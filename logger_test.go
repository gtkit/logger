package logger_test

import (
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/gtkit/logger"
)

// TestDefaultLogger 验证未调用 NewZap 时不会 panic.
func TestDefaultLogger(t *testing.T) {
	logger.Info("should not panic", zap.String("from", "default"))
	logger.Infof("formatted default: %s", "hello")
	logger.Debug("debug from default")
}

func TestNewZap(t *testing.T) {
	logger.NewZap(
		logger.WithDivision("size"),
		logger.WithConsole(true),
		logger.WithFile(true),
		logger.WithPath("./testlogs/app"),
		logger.WithOutJSON(true),
	)
	defer logger.Sync()

	// Structured 风格
	logger.Info("structured log",
		zap.String("module", "test"),
		zap.Int("age", 25),
		zap.Float64("weight", 65.5),
		zap.Bool("married", true),
		zap.String("address", "New York"),
	)

	// Sugar 风格
	logger.Infof("formatted: %s %d", "test", 42)
	logger.Infow("sugar kv", "key", "value", "count", 3)

	// Named sub-logger
	logger.Zap().Named("auth").Info("user logged in", zap.String("user", "alice"))
}

func TestDailyDivision(t *testing.T) {
	logger.NewZap(
		logger.WithDivision("daily"),
		logger.WithConsole(true),
		logger.WithFile(true),
		logger.WithPath("./testlogs/daily"),
	)
	defer logger.Sync()

	logger.Info("daily mode log", zap.String("test", "daily"))
	logger.Infof("daily formatted: %s", "hello")
}

func TestConsoleOnly(t *testing.T) {
	logger.NewZap(
		logger.WithConsole(true),
		logger.WithFile(false),
		logger.WithLevel("debug"),
	)
	defer logger.Sync()

	logger.Debug("debug message")
	logger.Warn("warning message")
	logger.Error("error message")
}

func TestRepeatedNewZap(t *testing.T) {
	logger.NewZap(
		logger.WithConsole(true),
		logger.WithFile(true),
		logger.WithPath("./testlogs/first"),
	)
	logger.Info("from first logger")

	// 重复调用，旧资源应被清理.
	logger.NewZap(
		logger.WithConsole(true),
		logger.WithFile(true),
		logger.WithPath("./testlogs/second"),
	)
	defer logger.Sync()

	logger.Info("from second logger")
}

func TestLogIf(t *testing.T) {
	logger.NewZap(
		logger.WithConsole(true),
		logger.WithFile(false),
	)
	defer logger.Sync()

	logger.LogIf(nil)
	logger.LogIf(errors.New("something went wrong"))
}

func TestAdapters(t *testing.T) {
	logger.NewZap(
		logger.WithConsole(true),
		logger.WithFile(false),
	)
	defer logger.Sync()

	cron := logger.NewCronAdapter()
	cron.Info("job executed", "duration", "1.5s")

	es := logger.NewESAdapter()
	es.Printf("connected to %s", "localhost:9200")

	resty := logger.NewRestyAdapter()
	resty.Debugf("request to %s", "https://api.example.com")
}

func TestInvalidOptionPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on invalid option")
		}
	}()

	logger.NewZap(
		logger.WithLevel("invalid_level"),
	)
}

func TestInvalidChannelOptionPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on invalid channel option")
		}
	}()

	logger.NewZap(
		logger.WithPath("./testlogs/app"),
		logger.WithChannel("order",
			logger.WithChannelPath("./testlogs/app"),
			logger.WithChannelDuplicateToDefault(true),
		),
	)
}
