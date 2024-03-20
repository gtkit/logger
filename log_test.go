package logger_test

import (
	"testing"

	"go.uber.org/zap"

	"github.com/gtkit/logger"
)

func TestLog(t *testing.T) {
	// 初始化日志
	logger.NewZap(
		logger.WithDivision("size"),
		logger.WithConsole(true),
		logger.WithFile(true),
		logger.WithPath("./testlogs"),
	)
	// 日志
	logger.Zlog().Named("xiaozhaofu").Info("test-log", zap.String("test", "test-log"))

	logger.ZInfo("[*test log*]",
		zap.String("test", "test-log"),
		zap.Int("age", 25),
		zap.Float64("weight", 65.5),
		zap.Bool("married", true),
		zap.String("address", "New York"),
	)

	logger.ZError("[*test error*]",
		zap.String("test", "test-error"),
		zap.Int("age", 25),
		zap.Float64("weight", 65.5),
		zap.Bool("married", true),
		zap.String("address", "New York"),
	)
}
