package logger_test

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

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

// ExampleWithStacktraceLevel 把 stacktrace 门槛提到 dpanic：高频 Error 日志不再采栈。
func ExampleWithStacktraceLevel() {
	dir, _ := os.MkdirTemp("", "logger-example")
	defer os.RemoveAll(dir)

	err := logger.New(
		logger.WithPath(filepath.Join(dir, "app")),
		logger.WithStacktraceLevel("dpanic"),
	)
	defer logger.Sync()

	fmt.Println(err == nil)
	logger.Error("upstream timeout", zap.String("upstream", "payment"))
	// Output: true
}

// ExampleWithMessagerDrainTimeout 限定 Sync 等待推送队列排空的上限，外部推送挂起时进程退出不被拖住。
func ExampleWithMessagerDrainTimeout() {
	dir, _ := os.MkdirTemp("", "logger-example")
	defer os.RemoveAll(dir)

	logger.NewZap(
		logger.WithPath(filepath.Join(dir, "app")),
		logger.WithMessager(printMessager{}),
		logger.WithMessagerDrainTimeout(2*time.Second),
	)

	logger.HError("payment failed")
	logger.Sync() // 排空推送队列后返回，最多等 2 秒
	// Output: push: payment failed
}

type printMessager struct{}

func (printMessager) Send(msg string)      { fmt.Println("push:", msg) }
func (printMessager) SendTo(_, msg string) { fmt.Println("push:", msg) }

// ExampleSetLevel_invalid 未知级别返回错误，当前级别保持不变。
func ExampleSetLevel_invalid() {
	dir, _ := os.MkdirTemp("", "logger-example")
	defer os.RemoveAll(dir)

	logger.NewZap(logger.WithPath(filepath.Join(dir, "app")))
	defer logger.Sync()

	fmt.Println(logger.SetLevel("verbose"))
	fmt.Println(logger.GetLevel())
	// Output:
	// logger: invalid level "verbose"
	// info
}
