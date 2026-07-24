package logger_test

import (
	"fmt"
	"os"
	"path/filepath"

	logger "github.com/gtkit/logger/v2"
	"go.uber.org/zap"
)

func ExampleNew() {
	dir, _ := os.MkdirTemp("", "logger-example")
	defer os.RemoveAll(dir)

	l, err := logger.New(
		logger.WithPath(filepath.Join(dir, "app")),
		logger.WithOutJSON(true),
		logger.WithLevel("info"),
	)
	if err != nil {
		fmt.Println("init failed:", err)
		return
	}
	defer l.Sync()

	l.Info("service started", zap.String("addr", ":8080"))
	fmt.Println("ok")
	// Output: ok
}

func ExampleLogger_SetLevel() {
	dir, _ := os.MkdirTemp("", "logger-example")
	defer os.RemoveAll(dir)

	l := logger.MustNew(logger.WithPath(filepath.Join(dir, "app")))
	defer l.Sync()

	l.SetLevel("debug")
	fmt.Println(l.GetLevel())
	// Output: debug
}

func ExampleLogger_Channel() {
	dir, _ := os.MkdirTemp("", "logger-example")
	defer os.RemoveAll(dir)

	l := logger.MustNew(
		logger.WithPath(filepath.Join(dir, "app")),
		logger.WithChannel("order",
			logger.WithChannelPath(filepath.Join(dir, "channels", "order")),
		),
	)
	defer l.Sync()

	// 已注册的 channel 写入独立文件；推荐启动时派生一次并复用。
	orderLog := l.Channel("order").Named("api").With(zap.String("service", "order"))
	orderLog.Info("order created", zap.String("order_id", "A100"))

	fmt.Println("ok")
	// Output: ok
}

func ExampleWithRedactKeys() {
	dir, _ := os.MkdirTemp("", "logger-example")
	defer os.RemoveAll(dir)

	l := logger.MustNew(
		logger.WithPath(filepath.Join(dir, "app")),
		logger.WithRedactKeys("password", "token"),
	)
	defer l.Sync()

	// password 字段的值落盘时会被替换为 [REDACTED]。
	l.Info("login", zap.String("user", "bob"), zap.String("password", "secret"))

	fmt.Println("ok")
	// Output: ok
}
