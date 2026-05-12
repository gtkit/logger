package logger

import (
	"fmt"
	"os"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

type dailyWriteSyncer struct {
	mu          sync.Mutex
	cfg         *logConfig
	lj          *lumberjack.Logger
	currentDate string
}

func newDailyWriteSyncer(cfg *logConfig) (*dailyWriteSyncer, error) {
	now := time.Now()
	lj, err := newLumberjack(cfg, now)
	if err != nil {
		return nil, err
	}

	return &dailyWriteSyncer{
		cfg:         cfg,
		lj:          lj,
		currentDate: now.Format(time.DateOnly),
	}, nil
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
	}

	n, err := d.lj.Write(p)
	if err != nil {
		return n, fmt.Errorf("logger: daily write: %w", err)
	}
	return n, nil
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
	d.mu.Lock()
	defer d.mu.Unlock()

	if err := d.lj.Close(); err != nil {
		return fmt.Errorf("logger: daily close: %w", err)
	}
	return nil
}
