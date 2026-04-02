# logger/v2

基于 [zap](https://github.com/uber-go/zap) 的实例化日志封装，提供 Structured / Sugar 双模式、文件切割、消息推送 Hook，以及按 `channel` 分类写入不同日志文件的能力。

## 安装

```bash
go get github.com/gtkit/logger/v2@latest
```

## 快速开始

```go
package main

import (
	"github.com/gtkit/logger/v2"
	"go.uber.org/zap"
)

func main() {
	log := logger.MustNew(
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
	defer log.Sync()

	log.Info("request processed",
		zap.String("method", "GET"),
		zap.Int("status", 200),
	)

	log.With(zap.String("request_id", "req-1")).
		Channel("order").
		Named("api").
		Info("order created", zap.String("order_id", "A100"))

	log.Channel("audit").Warn("role changed", zap.String("operator", "admin"))

	log.HError("payment failed", zap.String("order_id", "A100"))
}
```

## 基本行为

- `log.Info(...)` 只写默认日志输出。
- `log.Channel("name").Info(...)` 会自动附加 `channel=name` 字段。
- 未显式配置的 channel 只写默认日志输出，不会自动创建独立文件。
- 显式配置的 channel 会写入自己的文件。
- 显式配置的 channel 默认会同时写入默认日志输出；可通过 `WithChannelDuplicateToDefault(false)` 改成只写 channel 文件。
- channel 文件继承全局日志级别、编码格式、切割方式、保留天数、备份数量和压缩策略。
- logger 会自动创建日志文件的父目录。
- 如果 channel 路径与默认路径相同，并且开启双写，初始化会直接失败，避免同一条日志被重复写入同一个文件。

## Channel 配置

### 注册一个 channel

```go
log := logger.MustNew(
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
log.Channel("payment").Info("payment callback received")
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
cron.New(cron.WithLogger(logger.NewCronAdapter(log)))

// elastic/go-elasticsearch
es, _ := elasticsearch.NewClient(elasticsearch.Config{
	Logger: logger.NewESAdapter(log),
})

// go-resty/resty
client := resty.New().SetLogger(logger.NewRestyAdapter(log))
```

## 消息推送

```go
type Messager interface {
	Send(msg string)
	SendTo(url, msg string)
}

log := logger.MustNew(
	logger.WithMessager(myFeishuMessager),
)

log.HError("payment failed", zap.String("order_id", "12345"))
```

## 使用 Channel 的利弊

### 优点

- 按业务分类查日志更快，例如 `order`、`payment`、`audit`。
- 某些高价值日志可以单独归档、单独采集。
- 配置为双写时，既保留主日志全量视角，又能拿到独立分类文件。
- `With` 和 `Named` 的上下文会同时保留在默认日志和 channel 文件里。

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

- 对热点业务，优先把 `orderLog := log.Channel("order")` 这类 channel logger 初始化一次后复用。
- 已配置 channel 的基础 logger 会在 `New()` / `MustNew()` 时预建；对已配置 channel，直接 `log.Channel("order")` 的热路径成本已经较低。
- 如果链式叠加 `Named(...)`、`With(...)`、`Channel(...)`，建议把最终派生出来的 logger 缓存复用，而不是每次请求都重新组合。
- 尽量复用稳定的字段组合，不要在热路径里为大量高基数分类动态创建 channel。

## 基准测试

可用下面的命令在本地验证当前版本的热路径开销：

```bash
go test -run ^$ -bench "Benchmark(Info|Channel)" -benchmem
```
