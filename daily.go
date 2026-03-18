package logger

import (
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

// dailyWriteSyncer 按天自动切换日志文件.
//
// 每次 Write 时比较 time.Now().Day() 与缓存的 day 值.
// 日期变化时关闭旧 lumberjack，创建新日期文件名的 lumberjack.
// 同一天内超过 MaxSize 时，lumberjack 仍会按大小 rotate.
type dailyWriteSyncer struct {
	mu  sync.Mutex
	cfg *logConfig
	lj  *lumberjack.Logger
	day int // 当前文件对应的 Day of month
}

func newDailyWriteSyncer(cfg *logConfig) *dailyWriteSyncer {
	now := time.Now()

	return &dailyWriteSyncer{
		cfg: cfg,
		lj:  newLumberjack(cfg, now),
		day: now.Day(),
	}
}

func newLumberjack(cfg *logConfig, t time.Time) *lumberjack.Logger {
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
	if now.Day() != d.day {
		_ = d.lj.Close()
		d.lj = newLumberjack(d.cfg, now)
		d.day = now.Day()
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
