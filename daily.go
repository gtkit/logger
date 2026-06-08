package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

type dailyWriteSyncer struct {
	mu          sync.Mutex
	cfg         *logConfig
	lj          *lumberjack.Logger
	currentDate string

	// cleaning 保证同一时刻只有一个 sweep goroutine 在跑；cleanWG 让 Close 等待
	// 在途 sweep 完成，避免清理 goroutine 逃逸出 dailyWriteSyncer 生命周期。
	cleaning atomic.Bool
	cleanWG  sync.WaitGroup
}

func newDailyWriteSyncer(cfg *logConfig) (*dailyWriteSyncer, error) {
	now := time.Now()
	lj, err := newLumberjack(cfg, now)
	if err != nil {
		return nil, err
	}

	d := &dailyWriteSyncer{
		cfg:         cfg,
		lj:          lj,
		currentDate: now.Format(time.DateOnly),
	}
	// 启动即清一次积压：进程重启 / 长期 idle 后，历史日切文件可能早已超期，
	// 不必等到下一次跨天才回收。
	d.maybeClean(now, lj.Filename)
	return d, nil
}

func newLumberjack(cfg *logConfig, t time.Time) (*lumberjack.Logger, error) {
	logpath := cfg.path + "-" + cfg.level + "-" + t.Format(time.DateOnly) + ".log"
	if err := ensureLogDir(logpath); err != nil {
		return nil, err
	}

	return &lumberjack.Logger{
		Filename:   logpath,
		MaxSize:    cfg.maxSize,
		MaxAge:     cfg.maxAge,
		MaxBackups: cfg.maxBackups,
		Compress:   cfg.compress,
		LocalTime:  true,
	}, nil
}

func (d *dailyWriteSyncer) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	currentDate := now.Format(time.DateOnly)
	if currentDate != d.currentDate {
		next, err := newLumberjack(d.cfg, now)
		if err != nil {
			return 0, err
		}
		if err := d.lj.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "logger: close rotated daily file: %v\n", err)
		}
		d.lj = next
		d.currentDate = currentDate
		// 跨天后回收过期的历史日切文件——这是 daily 模式的真正清理点，
		// lumberjack 的 MaxAge/MaxBackups 只覆盖单个 Filename，无法跨天删除。
		d.maybeClean(now, next.Filename)
	}

	n, err := d.lj.Write(p)
	if err != nil {
		return n, fmt.Errorf("logger: daily write: %w", err)
	}
	return n, nil
}

// maybeClean 在不阻塞写入的前提下异步触发一次历史日切文件回收。
//
// 用 cleaning(CAS) 做单飞：若已有 sweep 在跑则直接跳过——漏掉的这次清理会在
// 下一次跨天或下次进程启动时补上，不会无限堆积。cleanWG 让 Close 能等待在途 sweep。
func (d *dailyWriteSyncer) maybeClean(now time.Time, keepCurrent string) {
	if d.cfg.maxAge <= 0 && d.cfg.maxBackups <= 0 {
		return
	}
	if !d.cleaning.CompareAndSwap(false, true) {
		return
	}

	d.cleanWG.Go(func() {
		defer d.cleaning.Store(false)
		cleanDailyFiles(d.cfg, now, keepCurrent)
	})
}

// cleanDailyFiles 按 cfg.maxAge / cfg.maxBackups 回收历史日切文件，使 daily 模式
// 与 size 模式的保留语义一致。
//
// 匹配范围："{path}-{level}-*.log" 及其压缩档 "*.log.gz"，覆盖每日基础文件
// （path-level-DATE.log）与 lumberjack 当日 size 切割产生的备份。keepCurrent 是
// 当前活跃文件，永不删除。
func cleanDailyFiles(cfg *logConfig, now time.Time, keepCurrent string) {
	if cfg.maxAge <= 0 && cfg.maxBackups <= 0 {
		return
	}

	prefix := cfg.path + "-" + cfg.level + "-"
	matches, err := filepath.Glob(prefix + "*.log")
	if err != nil {
		return
	}
	if gz, gzErr := filepath.Glob(prefix + "*.log.gz"); gzErr == nil {
		matches = append(matches, gz...)
	}

	type logFile struct {
		path    string
		modTime time.Time
	}

	keepKey := normalizedPathKey(keepCurrent)
	candidates := make([]logFile, 0, len(matches))
	for _, m := range matches {
		if normalizedPathKey(m) == keepKey {
			continue
		}
		info, statErr := os.Stat(m)
		if statErr != nil {
			continue
		}
		candidates = append(candidates, logFile{path: m, modTime: info.ModTime()})
	}

	// maxAge：删除修改时间早于 (now - maxAge 天) 的文件。
	if cfg.maxAge > 0 {
		cutoff := now.Add(-time.Duration(cfg.maxAge) * 24 * time.Hour)
		kept := candidates[:0]
		for _, c := range candidates {
			if c.modTime.Before(cutoff) {
				removeLogFile(c.path)
				continue
			}
			kept = append(kept, c)
		}
		candidates = kept
	}

	// maxBackups：按修改时间降序保留最新 N 个，多余删除。
	if cfg.maxBackups > 0 && len(candidates) > cfg.maxBackups {
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].modTime.After(candidates[j].modTime)
		})
		for _, c := range candidates[cfg.maxBackups:] {
			removeLogFile(c.path)
		}
	}
}

func removeLogFile(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "logger: remove expired log %q: %v\n", path, err)
	}
}

// Sync 实现 zapcore.WriteSyncer.Sync。
//
// 返回 nil 是有意为之——zap WriteSyncer.Sync 的语义是"flush 任意 buffered writer"，
// dailyWriteSyncer 不持有自己的缓冲（Write 直接落到 lumberjack.Logger），所以无需任何动作。
//
// 注意：本实现**不做磁盘级 fsync**。lumberjack.Logger v2.2.x 不暴露公开 Sync 方法，
// 其内部 *os.File 也是私有的，所以无法在不引入反射/fork lumberjack 的前提下做 fsync。
// 这也是 zap 体系下所有 lumberjack 包装器的统一行为——zap 把"耐久性"留给上层选择
// （需要 fsync 的场景应替换底层 rotator，或在业务关键点显式 fsync 应用层数据）。
//
// 与 BufferedWriteSyncer 组合时：BufferedWriteSyncer.Stop/Sync 会先把自己的缓冲通过
// inner.Write 刷出，然后调用 inner.Sync——前者已经把数据交给了 lumberjack（再到 kernel），
// 后者在这里 no-op 不会引起数据丢失（kernel page cache 仍由 OS 兜底）。
func (d *dailyWriteSyncer) Sync() error {
	return nil
}

func (d *dailyWriteSyncer) Close() error {
	// 先等待在途 sweep 完成，确保没有清理 goroutine 逃逸出本 syncer 生命周期。
	d.cleanWG.Wait()

	d.mu.Lock()
	defer d.mu.Unlock()

	if err := d.lj.Close(); err != nil {
		return fmt.Errorf("logger: daily close: %w", err)
	}
	return nil
}
