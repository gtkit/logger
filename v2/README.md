# logger/v2

基于 [zap](https://github.com/uber-go/zap) 的实例化日志封装，提供 Structured / Sugar 双模式、文件切割、消息推送 Hook，以及按 `channel` 分类写入不同日志文件的能力。

## 安装

```bash
go get github.com/gtkit/logger/v2@latest
```

## 发布规范

- `v2/go.mod` 的模块路径必须保持为 `github.com/gtkit/logger/v2`。
- 当前仓库采用“仓库根目录 + major version 子目录 `./v2`”布局，发布 tag 必须使用 `v2.x.y`，不能使用 `v2/v2.x.y`。
- `v2/Makefile` 中的 `make tag` 会创建正确的 `v2.x.y` tag。
- 历史上如果误打了 `v2/v2.x.y`，不要删除或改写旧 tag；应补充创建同提交上的规范 tag。可先在本地执行 `make fix-tags` 生成缺失的 `v2.x.y` tag，再按需推送。

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
| `WithBuffered(b)` | 是否启用缓冲写入（BufferedWriteSyncer） | `false` |
| `WithBufferSize(n)` | 缓冲区大小（字节），仅 `WithBuffered(true)` 时生效 | `256KB` |
| `WithFlushInterval(d)` | 缓冲区自动刷写间隔，仅 `WithBuffered(true)` 时生效 | `30s` |
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

注意：适配器参数 `log` 不能为 nil，否则会 panic。

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

## slog 桥接

将 Go 标准库 `log/slog` 的日志统一写入 zap，适用于第三方库使用 slog 输出日志的场景：

```go
import "log/slog"

slog.SetDefault(slog.New(log.SlogHandler()))

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

## API 方法一览

## License

Apache-2.0. See [../LICENSE](../LICENSE).

### Structured（高性能，类型安全）

`Debug`、`Info`、`Warn`、`Error`、`DPanic`、`Panic`、`Fatal`

### Sugar — fmt 风格

`Debugf`、`Infof`、`Warnf`、`Errorf`、`DPanicf`、`Panicf`、`Fatalf`

### Sugar — key-value 风格

`Debugw`、`Infow`、`Warnw`、`Errorw`

```go
log.Infow("request processed", "method", "GET", "status", 200)
log.Errorw("query failed", "table", "orders", "err", err)
```

以上所有方法在 `Channel` 上同样可用：

```go
log.Channel("order").Infow("created", "order_id", "A100")
```

## 写入模式：WriteSyncer vs BufferedWriteSyncer

默认使用 `WriteSyncer`（同步写入），可通过 `WithBuffered(true)` 切换为 `BufferedWriteSyncer`（缓冲写入）。两者的核心区别在于日志数据从用户调用到真正落盘之间的路径不同。

### 内部原理

**WriteSyncer（同步写入）：**

```text
log.Info("msg") → zap 编码 → lumberjack.Write() → os.File.Write() → 内核缓冲区 → 磁盘
                               ↑ 每条日志都走一次完整的 write 系统调用
```

**BufferedWriteSyncer（缓冲写入）：**

```text
log.Info("msg") → zap 编码 → BufferedWriteSyncer.Write() → 内存缓冲区（用户态）
                                                                 │
                           缓冲区满 或 定时器到期 ──────────────────┘
                                                                 ↓
                           lumberjack.Write() → os.File.Write() → 内核缓冲区 → 磁盘
                           ↑ 多条日志合并为一次 write 系统调用
```

关键差异：BufferedWriteSyncer 在 zap 与底层 writer 之间插入了一层**用户态内存缓冲区**，将多次小写入合并为少量大写入，从而减少系统调用次数。

### 全维度对比

| 对比项 | WriteSyncer（默认） | BufferedWriteSyncer |
| --- | --- | --- |
| **写入方式** | 每条日志立即写入磁盘 | 先写入内存缓冲区，满或到期后批量刷盘 |
| **系统调用** | 每条日志 1 次 `write` syscall | N 条日志合并为 1 次 `write` syscall |
| **写入延迟** | 无——调用返回即已写入内核缓冲区 | 有——取决于缓冲区大小和刷写间隔（默认最多 30 秒） |
| **写入性能** | 高频写入时 I/O 开销大 | 高吞吐场景性能提升约 3-4 倍 |
| **内存占用** | 无额外内存 | 额外占用缓冲区大小的内存（默认 256KB） |
| **正常退出** | `Sync()` 调用 `os.File.Sync()`，数据已在磁盘 | `Sync()` 先 flush 缓冲区再 sync，数据落盘，**不丢日志** |
| **异常退出** | 已写入内核缓冲区的数据通常不丢 | 用户态缓冲区中未 flush 的数据**会丢失** |
| **丢失窗口** | 几乎为零 | 最多丢失 1 个缓冲区周期的日志（默认最多 30 秒或 256KB） |
| **线程安全** | 由底层 lumberjack 的 mutex 保证 | BufferedWriteSyncer 自带 mutex，再调用底层 writer |
| **适用场景** | 大多数服务——日志量适中，数据安全优先 | 高频日志——追求吞吐量，可容忍极端情况丢少量日志 |

### 异常退出场景详解

| 退出方式 | WriteSyncer | BufferedWriteSyncer |
| --- | --- | --- |
| `Sync()` 后正常退出 | 不丢 | 不丢（Sync 会 flush 缓冲区） |
| `os.Exit(0)` 未调 `Sync()` | 不丢（已在内核缓冲区） | **可能丢**（用户态缓冲区未 flush） |
| `kill -15`（SIGTERM）+ 信号处理调 `Sync()` | 不丢 | 不丢 |
| `kill -9`（SIGKILL） | 不丢（已在内核缓冲区） | **丢失缓冲区中的数据** |
| OOM Killer | 不丢（已在内核缓冲区） | **丢失缓冲区中的数据** |
| `panic` 未 recover | 不丢（已在内核缓冲区） | **可能丢**（取决于 panic 时是否执行了 defer Sync） |

### WriteSyncer（默认模式）

不需要额外配置，默认即为同步写入：

```go
log := logger.MustNew(
    logger.WithPath("./logs/app"),
    logger.WithLevel("info"),
)
defer log.Sync()
```

**优点：**
- 每条日志写入后立即进入内核缓冲区，数据安全性高
- 进程崩溃、被 kill、OOM 等异常退出几乎不丢日志
- 零额外内存开销
- 行为直观，适合绝大多数业务场景

**缺点：**
- 每条日志都触发系统调用（`write` syscall），高频写入时 I/O 开销大
- 在日志量极大的服务中（如每秒万条以上）可能成为性能瓶颈

### BufferedWriteSyncer（缓冲模式）

通过 `WithBuffered(true)` 启用，日志先写入内存缓冲区，当缓冲区满或达到刷写间隔时批量写入磁盘：

```go
log := logger.MustNew(
    logger.WithPath("./logs/app"),
    logger.WithLevel("info"),
    logger.WithBuffered(true),                    // 启用缓冲
    logger.WithBufferSize(512*1024),              // 可选：缓冲区 512KB（默认 256KB）
    logger.WithFlushInterval(10*time.Second),     // 可选：每 10 秒刷写（默认 30 秒）
)
defer log.Sync() // 重要：确保退出时 flush 缓冲区
```

**优点：**
- 批量写入大幅减少系统调用次数，高吞吐场景下性能提升显著（约 3-4 倍）
- 适合日志量大的网关、数据管道、批处理等服务
- 减少磁盘 I/O 竞争，对同机其他服务更友好

**缺点：**
- 进程异常退出时（kill -9、OOM、panic 未 recover）可能丢失缓冲区中未 flush 的日志
- 日志写入到实际落盘之间有延迟，`tail -f` 看日志会有滞后感
- 额外占用缓冲区大小的内存（默认 256KB，每个文件 writer 独立分配）
- 必须确保程序退出时调用 `Sync()` flush 残留数据

### 推荐：大多数场景使用默认的 WriteSyncer

**推荐绝大多数服务使用默认的 `WriteSyncer`（不开启缓冲）。** 理由：

1. **日志的首要职责是可靠记录**——丢失日志的代价通常远高于多几次系统调用的开销
2. **大多数服务的日志量不会成为瓶颈**——每秒几百到几千条日志，同步写入完全够用
3. **排查线上问题时需要实时看日志**——缓冲延迟会影响 `tail -f` 的实时性
4. **减少心智负担**——不需要担心异常退出丢日志，不需要确保每个退出路径都调了 `Sync()`

只在以下场景考虑开启 `WithBuffered(true)`：

| 场景 | 说明 |
| --- | --- |
| **高吞吐网关 / 代理** | 每秒数万条日志，同步写入成为 CPU/IO 瓶颈 |
| **数据管道 / 批处理** | 大量日志密集写入，但任务结束时会正常 `Sync()` |
| **非关键日志路径** | 如 access log、debug trace，丢少量可接受 |

以下场景**必须使用默认的 WriteSyncer**：

| 场景 | 说明 |
| --- | --- |
| **金融交易 / 支付** | 每笔交易日志都是审计证据，不能丢 |
| **审计 / 合规** | 监管要求完整记录，丢失即违规 |
| **安全事件** | 入侵检测、权限变更等日志丢失会影响事后溯源 |
| **线上排障依赖实时日志** | `tail -f` 需要即时看到输出 |

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
- 默认同步写日志，保证语义清晰；高吞吐场景可通过 `WithBuffered(true)` 启用缓冲写入提升性能，详见上方「写入模式」章节。

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
