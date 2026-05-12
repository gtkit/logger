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
	cfg         *Config
	lj          *lumberjack.Logger
	currentDate string
}

func newDailyWriteSyncer(cfg *Config) (*dailyWriteSyncer, error) {
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

func newLumberjack(cfg *Config, t time.Time) (*lumberjack.Logger, error) {
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

// Sync 实现 zapcore.WriteSyncer.Sync——参见 v1 daily.go 中相同方法的详细注释。
// 简言之：返回 nil 是有意为之，dailyWriteSyncer 无缓冲，且 lumberjack.Logger 不暴露
// Sync/fsync 能力。zap 体系下所有 lumberjack 包装器在此点的行为一致。
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
