# logger

基于 [zap](https://github.com/uber-go/zap) 的日志封装，提供 Structured / Sugar 双模式、文件切割、消息推送 Hook，以及按 `channel` 分类写入不同日志文件的能力。

## 安装

```bash
go get github.com/gtkit/logger@latest
```

## 快速开始

```go
package main

import (
	"github.com/gtkit/logger"
	"go.uber.org/zap"
)

func main() {
	// 方式一：返回 error，适合需要优雅处理错误的场景
	if err := logger.New(
		logger.WithPath("./logs/app"),
		logger.WithLevel("info"),
	); err != nil {
		panic(err)
	}

	// 方式二：失败时 panic，适合 main 函数或初始化阶段
	logger.NewZap(
		logger.WithPath("./logs/app"),
		logger.WithLevel("info"),
		logger.WithOutJSON(true),
		logger.WithConsole(true),
		logger.WithFile(true),
		logger.WithDivision("daily"),
		logger.WithChannel("order",
			logger.WithChannelPath("./logs/channels/order"),
			logger.WithChannelDuplicateToDefault(true),
		),
		logger.WithChannel("audit",
			logger.WithChannelPath("./logs/channels/audit"),
			logger.WithChannelDuplicateToDefault(false),
		),
	)
	defer logger.Sync()

	logger.Info("request processed",
		zap.String("method", "GET"),
		zap.Int("status", 200),
	)

	logger.Channel("order").Info("order created",
		zap.String("order_id", "A100"),
	)

	logger.Channel("audit").Warn("role changed",
		zap.String("operator", "admin"),
	)

	logger.HError("payment failed", zap.String("order_id", "A100"))
}
```

## 初始化

| 函数 | 说明 |
| --- | --- |
| `New(opts...) error` | 返回 error，适合需要优雅处理错误的场景 |
| `NewZap(opts...)` | 失败时 panic，适合 main 函数或初始化阶段 |

## 基本行为

- `logger.Info(...)` 只写默认日志输出。
- `logger.Channel("name").Info(...)` 会自动附加 `channel=name` 字段。
- 未显式配置的 channel 只写默认日志输出，不会自动创建独立文件。
- 显式配置的 channel 会写入自己的文件。
- 显式配置的 channel 默认会同时写入默认日志输出；可通过 `WithChannelDuplicateToDefault(false)` 改成只写 channel 文件。
- channel 文件继承全局日志级别、编码格式、切割方式、保留天数、备份数量和压缩策略。
- logger 会自动创建日志文件的父目录。
- 如果 channel 路径与默认路径相同，并且开启双写，初始化会直接失败，避免同一条日志被重复写入同一个文件。

## Channel 配置

### 注册一个 channel

```go
logger.NewZap(
	logger.WithPath("./logs/app"),
	logger.WithChannel("order",
		logger.WithChannelPath("./logs/channels/order"),
		logger.WithChannelDuplicateToDefault(true),
	),
)
```

生成的 channel 文件名仍然沿用现有规则：

- `size` 模式: `{channelPath}-{level}.log`
- `daily` 模式: `{channelPath}-{level}-2006-01-02.log`

### 未配置 channel

```go
logger.Channel("payment").Info("payment callback received")
```

这条日志只会进入默认输出，但日志内容里会带上 `channel=payment` 字段，方便检索。

### 配置后双写

```go
logger.WithChannel("order",
	logger.WithChannelPath("./logs/channels/order"),
	logger.WithChannelDuplicateToDefault(true),
)
```

这条 channel 日志会同时进入：

- 默认日志输出
- `./logs/channels/order-<level>.log`

### 配置后单写

```go
logger.WithChannel("audit",
	logger.WithChannelPath("./logs/channels/audit"),
	logger.WithChannelDuplicateToDefault(false),
)
```

这条 channel 日志只会进入 channel 文件，不会进入默认日志输出。

## 配置项

### 全局 Option

| Option | 说明 | 默认值 |
| --- | --- | --- |
| `WithPath(p)` | 默认日志文件路径前缀 | `./logs/` |
| `WithLevel(l)` | 日志级别 | `info` |
| `WithOutJSON(b)` | 是否输出 JSON | `false` |
| `WithConsole(b)` | 是否输出到控制台 | `false` |
| `WithFile(b)` | 是否输出到文件 | `true` |
| `WithDivision(d)` | 切割方式: `size` / `daily` | `size` |
| `WithMaxSize(mb)` | 单文件最大 MB | `512` |
| `WithMaxAge(days)` | 最大保留天数 | `7` |
| `WithMaxBackups(n)` | 最大备份数量 | `50` |
| `WithCompress(b)` | 是否压缩归档 | `true` |
| `WithMessager(m)` | 外部消息推送 Hook | `nil` |
| `WithMessagerQueueSize(n)` | 异步推送队列大小 | `1024` |
| `WithContextFields(fn)` | Context 字段提取函数 | `nil` |
| `WithChannel(name, ...opts)` | 注册独立 channel 文件路由 | 无 |

### ChannelOption

| Option | 说明 | 默认值 |
| --- | --- | --- |
| `WithChannelPath(path)` | channel 文件路径前缀 | 必填 |
| `WithChannelDuplicateToDefault(b)` | 是否同时写入默认日志输出 | `true` |

## 日志切割

### size 模式

文件名格式:

```text
{path}-{level}.log
```

由 lumberjack 按文件大小自动 rotate。

### daily 模式

文件名格式:

```text
{path}-{level}-2006-01-02.log
```

每天切换到新文件；同一天内超过 `MaxSize` 时，仍由 lumberjack 按大小继续 rotate。

## 第三方库适配器

```go
// robfig/cron
cron.New(cron.WithLogger(logger.NewCronAdapter()))

// elastic/go-elasticsearch
es, _ := elasticsearch.NewClient(elasticsearch.Config{
	Logger: logger.NewESAdapter(),
})

// go-resty/resty
client := resty.New().SetLogger(logger.NewRestyAdapter())
```

## 消息推送

```go
type Messager interface {
	Send(msg string)
	SendTo(url, msg string)
}

logger.NewZap(
	logger.WithMessager(myFeishuMessager),
)

logger.HError("payment failed", zap.String("order_id", "12345"))
```

消息推送默认异步执行（队列大小 1024），不会阻塞日志写入。可通过 `WithMessagerQueueSize` 调整队列大小：

```go
logger.NewZap(
	logger.WithMessager(myFeishuMessager),
	logger.WithMessagerQueueSize(4096),
)
```

队列满时推送静默丢弃（日志已写入文件，只丢通知），保证日志调用永不阻塞。

## Context 支持

通过 `WithContextFields` 注册提取函数，在日志中自动注入 trace_id、request_id 等链路追踪信息：

```go
logger.NewZap(
	logger.WithPath("./logs/app"),
	logger.WithContextFields(func(ctx context.Context) []zap.Field {
		var fields []zap.Field
		if traceID, ok := ctx.Value("trace_id").(string); ok {
			fields = append(fields, zap.String("trace_id", traceID))
		}
		if reqID, ok := ctx.Value("request_id").(string); ok {
			fields = append(fields, zap.String("request_id", reqID))
		}
		return fields
	}),
)

// 使用 Ctx 后缀的方法自动注入 context 字段
logger.InfoCtx(ctx, "order created", zap.String("order_id", "A100"))
logger.ErrorCtx(ctx, "payment failed", zap.String("reason", "timeout"))

// Channel 也支持
logger.Channel("order").InfoCtx(ctx, "order shipped")
```

可用方法：`DebugCtx`、`InfoCtx`、`WarnCtx`、`ErrorCtx`。

## API 方法一览

### Structured（高性能，类型安全）

`Debug`、`Info`、`Warn`、`Error`、`DPanic`、`Panic`、`Fatal`

### Sugar — fmt 风格

`Debugf`、`Infof`、`Warnf`、`Errorf`、`DPanicf`、`Panicf`、`Fatalf`

### Sugar — key-value 风格

`Debugw`、`Infow`、`Warnw`、`Errorw`

```go
logger.Infow("request processed", "method", "GET", "status", 200)
logger.Errorw("query failed", "table", "orders", "err", err)
```

以上所有方法在 `Channel` 上同样可用：

```go
logger.Channel("order").Infow("created", "order_id", "A100")
```

## 动态日志级别

运行时动态调整日志级别，无需重启服务，适合线上排查问题时临时开 Debug：

```go
// 查看当前级别
logger.GetLevel() // "info"

// 临时开启 Debug
logger.SetLevel("debug")

// 排查完毕，恢复
logger.SetLevel("info")
```

级别变更会立即生效，影响所有 logger（包括 channel）。支持的级别：`debug`、`info`、`warn`、`error`、`dpanic`、`panic`、`fatal`。

## slog 桥接

将 Go 标准库 `log/slog` 的日志统一写入 zap，适用于第三方库使用 slog 输出日志的场景：

```go
import "log/slog"

slog.SetDefault(slog.New(logger.SlogHandler()))

// 之后所有 slog 调用都会写入 zap 管道
slog.Info("third-party log", "key", "value")

// 支持 slog.Group 嵌套结构
slog.Info("request",
	slog.Group("request",
		slog.String("method", "POST"),
		slog.Int("status", 201),
	),
)
```

## 丢弃消息监控

当异步 Messager 队列满时，推送会被静默丢弃。可通过 `DroppedMessages()` 监控丢弃量：

```go
// 定期上报到监控系统
dropped := logger.DroppedMessages()
if dropped > 0 {
	metrics.Gauge("logger.messager.dropped", dropped)
}
```

## 使用 Channel 的利弊

### 优点

- 按业务分类查日志更快，例如 `order`、`payment`、`audit`。
- 某些高价值日志可以单独归档、单独采集。
- 配置为双写时，既保留主日志全量视角，又能拿到独立分类文件。

### 代价

- 双写会增加额外的编码和磁盘 I/O。
- channel 越多，打开的文件句柄和 rotate 管理成本越高。
- 如果把高基数维度当 channel，比如用户 ID、订单号、请求 ID，会迅速失控。
- 如果你已经有 ELK、Loki、Datadog 之类的集中日志系统，很多场景下直接打结构化字段会更合适。

## 生产环境建议

- 默认日志保留为全量日志，channel 只给少数稳定业务域使用。
- 推荐的 channel 类别是低基数、长期稳定的分类，例如 `order`、`payment`、`audit`、`security`。
- 不要把用户 ID、订单号、请求 ID、租户 ID 这类高基数值当作 channel。
- I/O 比较敏感时，优先使用未配置 channel 或把 `WithChannelDuplicateToDefault(false)` 用在确实需要独立文件的分类上。
- 如果项目后续会上集中日志平台，优先考虑“默认日志 + 结构化字段检索”，不要把 channel 文件拆分做成主路径。
- 建议显式设置 `WithPath("./logs/app")`，避免直接使用默认 `./logs/` 前缀带来不够直观的文件命名。
- 当前实现是同步写日志，不会在库内部起异步队列；这能保证语义清晰，但吞吐上限仍然受磁盘 I/O 约束。

## 高性能使用建议

在高频路径上，推荐在包级别或初始化阶段创建好 channel logger 并复用，避免每次请求都重新查找和构建：

```go
package order

import (
	"context"

	"github.com/gtkit/logger"
	"go.uber.org/zap"
)

// 推荐：启动时创建，全局复用
var orderLog = logger.Channel("order").Named("api").With(zap.String("service", "order"))

func HandleOrder(ctx context.Context) {
	orderLog.Info("created", zap.String("id", "A100"))
}
```

不推荐在每次请求中重复构建：

```go
func HandleOrder(ctx context.Context) {
	// 不推荐：每次调用都会重新查找 channel、创建 Named/With 派生 logger
	logger.Channel("order").Named("api").With(zap.String("service", "order")).Info("created", zap.String("id", "A100"))
}
```

其他建议：

- 如果某类 channel 需要独立文件，显式用 `WithChannel(...)` 注册；已配置 channel 的路由会在初始化阶段预建，热路径更稳定。
- 尽量复用稳定的字段组合，不要在热路径里为大量高基数分类动态创建 channel。

## 基准测试

可用下面的命令在本地验证当前版本的热路径开销：

```bash
go test -run ^$ -bench "Benchmark(Info|Channel)" -benchmem
```
