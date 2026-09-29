package logger_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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

// ExampleSetDefault 展示进程默认实例形态：启动期 SetDefault 一次，
// 其余调用点直接用包级函数（日志写入文件，示例打印确定性状态供验证）。
func ExampleSetDefault() {
	dir, _ := os.MkdirTemp("", "logger-example")
	defer os.RemoveAll(dir)

	l := logger.MustNew(
		logger.WithPath(filepath.Join(dir, "app")),
		logger.WithLevel("info"),
		logger.WithReplaceGlobals(false),
	)
	defer l.Sync()
	logger.SetDefault(l)

	ctx := logger.ContextWithRequestID(context.Background(), "req-1")
	logger.Info("request processed", zap.Int("status", 200))
	logger.InfoCtx(ctx, "order created") // 自动携带 request_id

	fmt.Println(logger.Default() == l)
	fmt.Println(logger.RequestIDFromContext(ctx))
	// Output:
	// true
	// req-1
}

// ExampleLogger_WithCallerSkip 展示自建 facade 再包一层转发时的 caller 校准：
// 写入文件后回读，验证 caller 指向本示例文件（转发方的调用行）。
func ExampleLogger_WithCallerSkip() {
	dir, _ := os.MkdirTemp("", "logger-example")
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "app")

	log := logger.MustNew(
		logger.WithPath(path),
		logger.WithDivision("size"), // size 模式文件名不带日期，便于示例确定性回读
		logger.WithReplaceGlobals(false),
	)
	facade := log.WithCallerSkip(1)
	logViaFacade := func(msg string) {
		facade.Info(msg) // caller 指向 logViaFacade 的调用行，而非本行
	}
	logViaFacade("hello")
	log.Sync()

	data, _ := os.ReadFile(path + "-info.log")
	fmt.Println(strings.Contains(string(data), "example_test.go"))
	// Output:
	// true
}

// ExampleWithReplaceGlobals 展示辅助实例不抢占 zap 全局：
// 以 WithReplaceGlobals(false) 构建前后，zap.L() 保持不变。
func ExampleWithReplaceGlobals() {
	dir, _ := os.MkdirTemp("", "logger-example")
	defer os.RemoveAll(dir)

	before := zap.L()
	access := logger.MustNew(
		logger.WithPath(filepath.Join(dir, "access")),
		logger.WithBuffered(true),
		logger.WithReplaceGlobals(false), // 不替换 zap.L()/zap.S()
	)
	defer access.Sync()

	access.Info("GET /health 200")
	fmt.Println(zap.L() == before)
	// Output:
	// true
}

// ExampleWithBasePath 展示相对日志路径锚定到稳定写根：
// 相对前缀 Join 到 base 下（尾斜杠语义保留），日志位置不随进程 cwd 漂移。
func ExampleWithBasePath() {
	base, _ := os.MkdirTemp("", "logger-example")
	defer os.RemoveAll(base)

	log := logger.MustNew(
		logger.WithPath("logs/app"),      // 相对前缀
		logger.WithBasePath(base),        // 锚定根（必须是绝对路径）
		logger.WithDivision("size"),      // size 模式文件名不带日期，便于示例确定性回读
		logger.WithReplaceGlobals(false), // 辅助实例不抢占 zap 全局
	)
	log.Info("anchored")
	log.Sync()

	data, _ := os.ReadFile(filepath.Join(base, "logs", "app-info.log"))
	fmt.Println(strings.Contains(string(data), "anchored"))
	// Output:
	// true
}

// ExampleWithStacktraceLevel 把 stacktrace 门槛提到 dpanic：高频 Error 日志不再采栈。
func ExampleWithStacktraceLevel() {
	dir, _ := os.MkdirTemp("", "logger-example")
	defer os.RemoveAll(dir)

	l := logger.MustNew(
		logger.WithPath(filepath.Join(dir, "app")),
		logger.WithStacktraceLevel("dpanic"),
	)
	defer l.Sync()

	l.Error("upstream timeout", zap.String("upstream", "payment"))
	fmt.Println("ok")
	// Output: ok
}

// ExampleWithMessagerDrainTimeout 限定 Sync 等待推送队列排空的上限，外部推送挂起时进程退出不被拖住。
func ExampleWithMessagerDrainTimeout() {
	dir, _ := os.MkdirTemp("", "logger-example")
	defer os.RemoveAll(dir)

	l := logger.MustNew(
		logger.WithPath(filepath.Join(dir, "app")),
		logger.WithMessager(printMessager{}),
		logger.WithMessagerDrainTimeout(2*time.Second),
	)

	l.HError("payment failed")
	l.Sync() // 排空推送队列后返回，最多等 2 秒
	fmt.Println("dropped:", l.DroppedMessages())
	// Output:
	// push: payment failed
	// dropped: 0
}

type printMessager struct{}

func (printMessager) Send(msg string)      { fmt.Println("push:", msg) }
func (printMessager) SendTo(_, msg string) { fmt.Println("push:", msg) }

// ExampleLogger_SetLevel_invalid 未知级别返回错误，当前级别保持不变。
func ExampleLogger_SetLevel_invalid() {
	dir, _ := os.MkdirTemp("", "logger-example")
	defer os.RemoveAll(dir)

	l := logger.MustNew(logger.WithPath(filepath.Join(dir, "app")))
	defer l.Sync()

	fmt.Println(l.SetLevel("verbose"))
	fmt.Println(l.GetLevel())
	// Output:
	// logger: invalid level "verbose"
	// info
}
