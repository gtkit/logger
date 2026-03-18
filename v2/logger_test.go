package logger_test

import (
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/gtkit/logger/v2"
)

func TestNew(t *testing.T) {
	log, err := logger.New(
		logger.WithPath("./testlogs/app"),
		logger.WithLevel("info"),
		logger.WithOutJSON(true),
		logger.WithConsole(true),
		logger.WithFile(true),
		logger.WithDivision("size"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Sync()

	// Structured 风格
	log.Info("structured log",
		zap.String("module", "test"),
		zap.Int("age", 25),
		zap.Float64("weight", 65.5),
		zap.Bool("married", true),
		zap.String("address", "New York"),
	)

	// Sugar 风格
	log.Infof("formatted: %s %d", "test", 42)
	log.Infow("sugar kv", "key", "value", "count", 3)
}

func TestMustNew(t *testing.T) {
	log := logger.MustNew(
		logger.WithConsole(true),
		logger.WithFile(false),
		logger.WithLevel("debug"),
	)
	defer log.Sync()

	log.Debug("debug message")
	log.Warn("warning message")
	log.Error("error message")
}

func TestWith(t *testing.T) {
	log := logger.MustNew(
		logger.WithConsole(true),
		logger.WithFile(false),
	)
	defer log.Sync()

	// With 返回新 Logger，原 Logger 不受影响.
	reqLog := log.With(zap.String("request_id", "abc-123"))
	reqLog.Info("processing request", zap.Int("status", 200))

	// 原 Logger 无 request_id 字段.
	log.Info("original logger, no request_id")
}

func TestNamed(t *testing.T) {
	log := logger.MustNew(
		logger.WithConsole(true),
		logger.WithFile(false),
	)
	defer log.Sync()

	authLog := log.Named("auth")
	authLog.Info("user logged in", zap.String("user", "alice"))
}

func TestDailyDivision(t *testing.T) {
	log := logger.MustNew(
		logger.WithDivision("daily"),
		logger.WithConsole(true),
		logger.WithFile(true),
		logger.WithPath("./testlogs/daily"),
	)
	defer log.Sync()

	log.Info("daily mode log", zap.String("test", "daily"))
	log.Infof("daily formatted: %s", "hello")
}

func TestLogIf(t *testing.T) {
	log := logger.MustNew(
		logger.WithConsole(true),
		logger.WithFile(false),
	)
	defer log.Sync()

	// nil error — 不记录日志.
	log.LogIf(nil)

	// non-nil error — 记录日志.
	log.LogIf(errors.New("something went wrong"))
}

func TestAdapters(t *testing.T) {
	log := logger.MustNew(
		logger.WithConsole(true),
		logger.WithFile(false),
	)
	defer log.Sync()

	cron := logger.NewCronAdapter(log)
	cron.Info("job executed", "duration", "1.5s")

	es := logger.NewESAdapter(log)
	es.Printf("connected to %s", "localhost:9200")

	resty := logger.NewRestyAdapter(log)
	resty.Debugf("request to %s", "https://api.example.com")
}

func TestMustNewPanicsOnInvalidOption(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on invalid option")
		}
	}()

	_ = logger.MustNew(
		logger.WithLevel("invalid_level"),
	)
}

func TestNewInvalidOptions(t *testing.T) {
	tests := []struct {
		name string
		opt  logger.Option
	}{
		{"invalid division", logger.WithDivision("hourly")},
		{"empty path", logger.WithPath("")},
		{"negative maxAge", logger.WithMaxAge(-1)},
		{"negative maxBackups", logger.WithMaxBackups(-1)},
		{"zero maxSize", logger.WithMaxSize(0)},
		{"invalid level", logger.WithLevel("trace")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := logger.New(tt.opt)
			if err == nil {
				t.Fatalf("expected error for %s", tt.name)
			}
		})
	}
}
