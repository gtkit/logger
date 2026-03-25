package logger

import (
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

// dailyWriteSyncer 按天自动切换日志文件.
//
// 每次 Write 时比较 time.Now().Format("2006-01-02") 与缓存的 currentDate.
// 日期变化时关闭旧 lumberjack，创建新日期文件名的 lumberjack.
// 同一天内超过 MaxSize 时，lumberjack 仍会按大小 rotate.
type dailyWriteSyncer struct {
	mu          sync.Mutex
	cfg         *Config
	lj          *lumberjack.Logger
	currentDate string // 当前文件对应的完整本地日期: 2006-01-02
}

func newDailyWriteSyncer(cfg *Config) *dailyWriteSyncer {
	now := time.Now()

	return &dailyWriteSyncer{
		cfg:         cfg,
		lj:          newLumberjack(cfg, now),
		currentDate: now.Format("2006-01-02"),
	}
}

func newLumberjack(cfg *Config, t time.Time) *lumberjack.Logger {
	// 文件名格式: {path}-{level}-2006-01-02.log
	logpath := cfg.path + "-" + cfg.level + "-" + t.Format("2006-01-02") + ".log"

	return &lumberjack.Logger{
		Filename:   logpath,
		MaxSize:    cfg.maxSize,
		MaxAge:     cfg.maxAge,
		MaxBackups: cfg.maxBackups,
		Compress:   cfg.compress,
		LocalTime:  true,
	}
}

// Write implements io.Writer.
func (d *dailyWriteSyncer) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	currentDate := now.Format("2006-01-02")
	if currentDate != d.currentDate {
		_ = d.lj.Close()
		d.lj = newLumberjack(d.cfg, now)
		d.currentDate = currentDate
	}

	return d.lj.Write(p)
}

// Sync implements zapcore.WriteSyncer.
func (d *dailyWriteSyncer) Sync() error {
	return nil
}

// Close implements io.Closer.
func (d *dailyWriteSyncer) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.lj.Close()
}
