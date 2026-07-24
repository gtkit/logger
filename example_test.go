package logger_test

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gtkit/logger"
	"go.uber.org/zap"
)

func ExampleNew() {
	dir, _ := os.MkdirTemp("", "logger-example")
	defer os.RemoveAll(dir)

	err := logger.New(
		logger.WithPath(filepath.Join(dir, "app")),
		logger.WithOutJSON(true),
		logger.WithLevel("info"),
	)
	defer logger.Sync()

	fmt.Println(err == nil)
	logger.Info("service started", zap.String("addr", ":8080"))
	// Output: true
}

func ExampleSetLevel() {
	dir, _ := os.MkdirTemp("", "logger-example")
	defer os.RemoveAll(dir)

	logger.NewZap(logger.WithPath(filepath.Join(dir, "app")))
	defer logger.Sync()

	logger.SetLevel("debug")
	fmt.Println(logger.GetLevel())
	// Output: debug
}

func ExampleChannel() {
	dir, _ := os.MkdirTemp("", "logger-example")
	defer os.RemoveAll(dir)

	logger.NewZap(
		logger.WithPath(filepath.Join(dir, "app")),
		logger.WithChannel("order",
			logger.WithChannelPath(filepath.Join(dir, "channels", "order")),
		),
	)
	defer logger.Sync()

	// 已注册的 channel 写入独立文件；推荐启动时派生一次并复用。
	orderLog := logger.Channel("order").Named("api").With(zap.String("service", "order"))
	orderLog.Info("order created", zap.String("order_id", "A100"))

	fmt.Println("ok")
	// Output: ok
}

func ExampleWithRedactKeys() {
	dir, _ := os.MkdirTemp("", "logger-example")
	defer os.RemoveAll(dir)

	logger.NewZap(
		logger.WithPath(filepath.Join(dir, "app")),
		logger.WithRedactKeys("password", "token"),
	)
	defer logger.Sync()

	// password 字段的值落盘时会被替换为 [REDACTED]。
	logger.Info("login", zap.String("user", "bob"), zap.String("password", "secret"))

	fmt.Println("ok")
	// Output: ok
}
