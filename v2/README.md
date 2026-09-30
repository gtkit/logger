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

## 默认实例与包级函数

进程内单主日志的推荐形态（`log/slog` 同款）：构建实例后 `SetDefault`，其余调用点直接用包级函数，caller 指向真实调用行。

```go
func main() {
	logger.SetDefault(logger.MustNew(
		logger.WithPath("./logs/app"),
		logger.WithLevel("info"),
	))
	defer logger.Sync()

	ctx := logger.ContextWithRequestID(context.Background(), "req-1")

	logger.Info("request processed", zap.Int("status", 200))
	logger.Errorf("connect failed: %v", context.DeadlineExceeded)
	logger.InfoCtx(ctx, "order created") // 自动携带 request_id，并合并 WithContextFields 字段
}
```

行为约定：

- 默认实例只能经 `SetDefault` **显式**设定（nil 被忽略），并发安全；
- 未设定时包级函数写入懒创建的**纯控制台兜底实例**——不落盘（避免以进程 cwd 为锚在任意目录刷出日志文件）、不替换 zap 全局；
- 多实例场景不受影响：辅助实例（如独立的 access 日志实例）建议用 `WithReplaceGlobals(false)` 构建，避免抢占 zap 全局后再靠 `Undo()` 撤销；
- 进阶能力（`Channel`/`With`/`Zap()`/`SetLevel` 等）经 `logger.Default()` 或自行持有的实例使用；
- **全局 owner 契约**：进程内只应有**一个**实例以默认方式（安装 zap 全局）构建，其余实例一律 `WithReplaceGlobals(false)`。推荐生命周期：启动时构建 owner 并 `SetDefault` 一次 + `defer logger.Sync()`。**不支持运行期热替换**；多 owner 先后安装、乱序关闭属未定义行为（undo 链会恢复过期甚至已关闭的实例）。库侧的条件化 `Undo`（全局已被替换则跳过恢复）仅是对该误用最常见形态的缓解，不是热替换安全承诺。确需多实例轮换：全部 `WithReplaceGlobals(false)`，由应用层自行管理 zap 全局。
- 兜底实例的控制台输出走 **stderr**，不会污染 CLI 的 stdout 协议/管道输出。
- 自建 facade 再包一层转发时，用 `Logger.WithCallerSkip(1)` 派生实例校准 caller；偏移以元数据保存，经 `With`/`Named`/`Channel`（含注册与动态）继续派生均保留，共享的 channel 缓存永远只存零偏移实例，`Zap()`/`Sugar()` 返回值的 caller 恒指向真实调用点。
- 包级函数导出面（精确清单）：`Debug/Info/Warn/Error/DPanic/Panic/Fatal` 及其 `f` 变体；`Debugw/Infow/Warnw/Errorw`；`DebugCtx/InfoCtx/WarnCtx/ErrorCtx` 及其 `w` 变体；`LogIf/WarnIf/LogIfCtx/WarnIfCtx`；`HInfo/HInfof/HInfoTo/HInfoTof/HError/HErrorf/HErrorTo/HErrorTof`；`Sync`。`DPanicw/Panicw/Fatalw` 实例方法不存在，故无对应包级函数；`SetDefault` 不会关闭被替换的旧实例，其生命周期由调用方管理。

## 基本行为

- `log.Info(...)` 只写默认日志输出。
- `log.Channel("name").Info(...)` 会自动附加 `channel=name` 字段。
- 未显式配置的 channel 只写默认日志输出，不会自动创建独立文件。
- 显式配置的 channel 会写入自己的文件。
- 显式配置的 channel 默认会同时写入默认日志输出；可通过 `WithChannelDuplicateToDefault(false)` 改成只写 channel 文件。
- channel 文件继承全局日志级别、编码格式、切割方式、保留天数、备份数量和压缩策略。
- logger 会自动创建日志文件的父目录。
- channel 文件与默认文件、channel 文件之间按**最终文件名**做冲突检查（`{path}-{level}.log`），任一重合初始化直接失败，避免多个 writer 竞争同一个文件；`./logs/` 与 `./logs` 这类只差尾斜杠的前缀产出的是不同文件，不视为冲突。
- 同名 channel 重复注册（两次 `WithChannel("order", ...)`）初始化直接失败。
- `WithConsole` 与 `WithFile` 至少一个为 `true`，两者都关闭时 `New` 返回 `ErrNoOutput`。库不做回退，兜底由调用方用 `errors.Is` 判定后决定：

```go
l, err := logger.New(opts...)
if errors.Is(err, logger.ErrNoOutput) {
	l, err = logger.New(append(opts, logger.WithConsole(true))...)
}
```

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
- `daily` / `both` 模式: `{channelPath}-{level}-2006-01-02.log`

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
| `WithDurationEncoder(enc)` | 自定义 `time.Duration` 编码方式，例如 `zapcore.StringDurationEncoder` | `zapcore.SecondsDurationEncoder` |
| `WithConsole(b)` | 是否输出到控制台 | `false` |
| `WithFile(b)` | 是否输出到文件 | `true` |
| `WithDivision(d)` | 切割方式: `size` / `daily` / `both` | `both` |
| `WithMaxSize(mb)` | 单文件最大 MB | `512` |
| `WithMaxAge(days)` | 最大保留天数 | `7` |
| `WithMaxBackups(n)` | 最大备份数量 | `50` |
| `WithCompress(b)` | 是否压缩归档 | `true` |
| `WithMessager(m)` | 外部消息推送 Hook | `nil` |
| `WithMessagerQueueSize(n)` | 异步推送队列大小 | `1024` |
| `WithMessagerDrainTimeout(d)` | `Sync` 排空推送队列的最长等待，超时告警并计入 `DroppedMessages` | `5s` |
| `WithStacktraceLevel(l)` | 附带 stacktrace 的最低级别，空串保留默认 | `error` |
| `WithBuffered(b)` | 是否启用缓冲写入（BufferedWriteSyncer） | `false` |
| `WithBufferSize(n)` | 缓冲区大小（字节），仅 `WithBuffered(true)` 时生效 | `256KB` |
| `WithFlushInterval(d)` | 缓冲区自动刷写间隔，仅 `WithBuffered(true)` 时生效 | `30s` |
| `WithSampling(first, thereafter)` | 启用采样：每 1s 窗口内同 level+message 先放行 `first` 条，之后每 `thereafter` 条放行一条 | 关闭 |
| `WithRedactKeys(keys...)` | 对命中的结构化字段名脱敏，值替换为 `[REDACTED]`；多次调用取并集 | 无 |
| `WithChannel(name, ...opts)` | 注册独立 channel 文件路由；同名重复注册报错 | 无 |
| `WithBasePath(base)` | 相对日志路径（含 channel 路径）的锚定根目录；绝对路径原样 | 空（不锚定） |
| `WithReplaceGlobals(b)` | 构建时是否安装为 zap 全局 logger（`zap.L()/zap.S()`） | `true` |

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

由 `github.com/gtkit/logrotate` 按文件大小自动 rotate。

### daily 模式

文件名格式:

```text
{path}-{level}-2006-01-02.log
```

每天写入带日期的活跃文件；`daily` 只按天切割，不按大小切割。

### both 模式（默认）

文件名格式:

```text
{path}-{level}-2006-01-02.log
```

每天写入带日期的活跃文件；同一天内超过 `MaxSize` 时继续按大小切割。适合既要按天归档，又要限制单个日志文件大小的场景。

**历史文件清理**：由 `logrotate` 按 `WithMaxAge` / `WithMaxBackups` 清理过期或超量的历史文件（含 `.log.gz` 压缩档）。

- 清理在后台 goroutine 执行，不阻塞日志写入。
- `MaxAge` 与 `MaxBackups` 同时为 0 时，关闭清理（历史文件全部保留）。

## 采样（高频日志降噪）

热循环里某条日志每秒打几万次时，会打爆磁盘 IO、拖垮下游、瞬间填满异步推送队列。`WithSampling` 用 zap 原生采样器按 message 去重限流：

```go
log := logger.MustNew(
	logger.WithPath("./logs/app"),
	logger.WithSampling(100, 100), // 每秒同 message 先放行 100 条，之后每 100 条放行 1 条
)
```

- 默认关闭（不配置即全量输出）。
- `thereafter` 为 0 表示首批之后全部丢弃。
- channel 继承相同采样配置。
- ⚠ 采样按 **message 文本**去重，务必用稳定 message + 结构化字段，不要把变量拼进 message。

## 字段脱敏（PII / 密钥合规）

```go
log := logger.MustNew(
	logger.WithPath("./logs/app"),
	logger.WithRedactKeys("password", "token", "authorization", "id_card", "phone"),
)

log.Info("login", zap.String("user", "bob"), zap.String("password", "secret"))
// 输出: ... "user":"bob","password":"[REDACTED]"
```

- 按字段 Key 精确匹配（区分大小写），命中字段值替换为 `[REDACTED]`。
- 多次调用取并集：基础敏感集与业务追加集可以分开传入，空串 key 忽略。
- 仅作用于结构化字段；拼进 message 文本的敏感信息不受影响。
- 不配置时零开销（不包装 core）。

## Stacktrace 级别

默认 `error` 及以上级别的每条日志都附带 stacktrace（与 zap production 一致）。高频 Error 日志场景可以提升门槛省去采栈开销：

```go
log := logger.MustNew(
	logger.WithPath("./logs/app"),
	logger.WithStacktraceLevel("dpanic"), // 仅 dpanic/panic/fatal 附带 stacktrace
)
```

## 动态日志级别

```go
if err := log.SetLevel("debug"); err != nil { // 未知级别返回错误，当前级别不变
	return err
}
log.GetLevel() // "debug"
```

级别变更立即生效，影响所有派生实例与 channel。支持的级别：`debug`、`info`、`warn`、`error`、`dpanic`、`panic`、`fatal`。

## 强制团队正确使用（golangci-lint depguard）

为在 CI 层面强制「业务必须用 gtkit/logger、禁止 `log` / `log/slog` / `fmt.Print*` 打日志」，在**使用方项目**的 `.golangci.yml` 加入：

```yaml
linters:
  enable:
    - depguard
    - forbidigo
  settings:
    depguard:
      rules:
        main:
          deny:
            - pkg: "log"
              desc: "业务日志请用 github.com/gtkit/logger/v2"
            - pkg: "log/slog"
              desc: "禁止把 slog 作为主日志栈，请用 github.com/gtkit/logger/v2"
    forbidigo:
      forbid:
        - pattern: "^fmt\\.Print.*$"
          msg: "禁止用 fmt.Print* 打日志，请用 github.com/gtkit/logger/v2"
```

> 机械强制优于口头约定——把规则交给 lint，任何人都绕不过去。

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

消息推送默认异步执行（队列大小 1024），不会阻塞日志写入。可通过 `WithMessagerQueueSize` 调整队列大小：

```go
log := logger.MustNew(
	logger.WithMessager(myFeishuMessager),
	logger.WithMessagerQueueSize(4096),
)
```

队列满时推送静默丢弃（日志已写入文件，只丢通知），保证日志调用永不阻塞。

`Sync` 会等待队列中的推送执行完毕，最长等 `WithMessagerDrainTimeout`（默认 5 秒）：外部推送挂起时进程退出不会被拖住，超时后向 stderr 告警，尚未执行的推送计入 `DroppedMessages`，后台推送协程继续消费直到外部调用返回。

```go
log := logger.MustNew(
	logger.WithMessager(myFeishuMessager),
	logger.WithMessagerDrainTimeout(2*time.Second),
)
```

### H 系列方法一览

**`H` 前缀的方法 = 写日志 + 调用 Messager.Send()**。普通方法（`Info` / `Error` 等）只写日志；`H` 方法在写日志成功后额外触发消息推送：

| 方法 | 日志级别 | 推送目标 |
|---|---|---|
| `HInfo(msg, fields...)` | Info | `Messager.Send` |
| `HInfof(format, args...)` | Info | `Messager.Send` |
| `HInfoTo(url, msg, fields...)` | Info | `Messager.SendTo(url, ...)` |
| `HInfoTof(url, format, args...)` | Info | `Messager.SendTo(url, ...)` |
| `HError(msg, fields...)` | Error | `Messager.Send` |
| `HErrorf(format, args...)` | Error | `Messager.Send` |
| `HErrorTo(url, msg, fields...)` | Error | `Messager.SendTo(url, ...)` |
| `HErrorTof(url, format, args...)` | Error | `Messager.SendTo(url, ...)` |

未配置 `WithMessager` 时，`H` 方法等价于普通方法（推送部分静默跳过）。

## 丢弃消息监控

异步 Messager 队列满、或 `Sync` 排空超时时，推送会被丢弃。可通过 `DroppedMessages()` 监控丢弃量：

```go
dropped := log.DroppedMessages()
if dropped > 0 {
	metrics.Gauge("logger.messager.dropped", dropped)
}
```

## API 方法一览

### 获取底层 zap logger

`l.Zap()` / `l.Sugar()`（channel 派生实例同样可用）返回底层 `*zap.Logger` / `*zap.SugaredLogger`，可直接打日志或交给需要原生 zap 的第三方库（gorm、grpc 中间件等）。返回的 logger 已修正 caller skip：直接调用时 `caller` 指向你的真实代码位置。`New()` 成功后 `zap.L()` / `zap.S()` 同样可用且 caller 准确。若你要在它外面再包一层自己的封装，按 zap 惯例自行叠加 `WithOptions(zap.AddCallerSkip(1))`。

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

### Context 注入（自动合并 contextFields）

- 结构化字段：`DebugCtx`、`InfoCtx`、`WarnCtx`、`ErrorCtx`
- Sugar key-value：`DebugwCtx`、`InfowCtx`、`WarnwCtx`、`ErrorwCtx`

### 条件日志（err != nil 才记录）

- `LogIf(err)` / `WarnIf(err)`：Error / Warn 级别
- `LogIfCtx(ctx, err)` / `WarnIfCtx(ctx, err)`：带 ctx 字段注入

### Hook 推送（写日志 + Messager.Send）

`HInfo`、`HInfof`、`HInfoTo`、`HInfoTof`、`HError`、`HErrorf`、`HErrorTo`、`HErrorTof` —— 详见上文 "H 系列方法一览" 章节

## 写入模式：WriteSyncer vs BufferedWriteSyncer

默认使用 `WriteSyncer`（同步写入），可通过 `WithBuffered(true)` 切换为 `BufferedWriteSyncer`（缓冲写入）。两者的核心区别在于日志数据从用户调用到真正落盘之间的路径不同。

> **持久化边界（best-effort）**：`Sync()` 会 flush 缓冲并调用底层 fsync/close，但失败只输出到 stderr、调用方无法拿到 error——本库不提供事务级持久化保证。审计、计费、交易凭证等不允许丢失的数据，请使用数据库/可靠消息等事务性存储，不要把日志文件当作可靠存储。

### 内部原理

**WriteSyncer（同步写入）：**

```text
log.Info("msg") → zap 编码 → logrotate.Write() → os.File.Write() → 内核缓冲区 → 磁盘
                               ↑ 每条日志都走一次完整的 write 系统调用
```

**BufferedWriteSyncer（缓冲写入）：**

```text
log.Info("msg") → zap 编码 → BufferedWriteSyncer.Write() → 内存缓冲区（用户态）
                                                                 │
                           缓冲区满 或 定时器到期 ──────────────────┘
                                                                 ↓
                           logrotate.Write() → os.File.Write() → 内核缓冲区 → 磁盘
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
| **正常退出** | `Sync()` 调用 `os.File.Sync()`，尽力落盘 | `Sync()` 先 flush 缓冲区再 sync，尽力落盘 |
| **异常退出** | 已写入内核缓冲区的数据通常不丢 | 用户态缓冲区中未 flush 的数据**会丢失** |
| **丢失窗口** | 几乎为零 | 最多丢失 1 个缓冲区周期的日志（默认最多 30 秒或 256KB） |
| **线程安全** | 由底层 logrotate 的 mutex 保证 | BufferedWriteSyncer 自带 mutex，再调用底层 writer |
| **适用场景** | 大多数服务——日志量适中，数据安全优先 | 高频日志——追求吞吐量，可容忍极端情况丢少量日志 |

### 异常退出场景详解

| 退出方式 | WriteSyncer | BufferedWriteSyncer |
| --- | --- | --- |
| `Sync()` 后正常退出 | 尽力不丢（best-effort） | 尽力不丢（Sync 会 flush 缓冲区，best-effort） |
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
- 进程崩溃、被 kill、OOM 等异常退出几乎不丢日志（数据已交内核缓冲；机器断电/内核崩溃仍可能丢失未刷盘部分）
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

## License

Apache-2.0. See [../LICENSE](../LICENSE).

## requestId 贯穿（内建约定）

HTTP 中间件把 requestId 写入 context，所有 `*Ctx` 方法零配置自动合并 `request_id` 字段：

```go
// 中间件侧
ctx := logger.ContextWithRequestID(r.Context(), requestID)

// 任意层
logger.InfoCtx(ctx, "order created")        // 自动携带 request_id
logger.ErrorwCtx(ctx, "failed", "k", "v")

// 其他基础设施复用同一约定（如 GORM 日志适配器）
zlogger.WithTraceIDExtractor(logger.RequestIDFromContext)
```

自定义字段仍走 `WithContextFields`，与内建 request_id 并存合并。同名 `request_id` 只保留一个，优先级：**`With` 预绑定 > 调用点字段 > 自定义上下文字段 > 内建 RequestID**（避免同一条 JSON 出现重复 key）。`With` 预绑定的 request_id 是作用域身份、不可被调用点覆盖；**同源内部重复同样归一化，保留最后一个**。去重覆盖全部记录方法：结构化（`Info` 等）、Sugar key-value（`Infow` 等）、H 系列与 `*Ctx` 系列的调用点字段都会在写入前按上述优先级处理，链式 `With` 的归一化发生在构建期。

注意字段名：logger 侧输出 `request_id`；ormx/zlogger 侧输出 `trace_id`——**统一的是 id 值，不是检索字段名**，日志平台做关联查询时需对两个字段名做别名或分别检索。`*Ctx` 方法在级别关闭时不构造 ctx 字段（零额外分配）。

## 路径锚定与零值容忍

- `WithBasePath(base)`：相对日志路径（含 channel 路径）锚定到 base 下，绝对路径原样；**base 必须为绝对路径**（相对 base 仍随 cwd 漂移，option 校验拒绝），锚定保留尾斜杠语义（`logs/` 仍产出 `logs/` 目录内的文件，不会坍缩成 `logs-` 前缀）。**它只做相对路径解析锚定，不是安全隔离边界**：不校验 `../` 逃逸、不拦截绝对路径；需要强制"日志必须落在某根目录下"时在应用配置层校验。
- 零值 = 未配置（**仅限无文件系统副作用的配置项**）：`WithLevel("")` / `WithMaxSize(0)` 使用默认值而不报错，可无条件透传；`WithPath("")` **报错**——path 决定落盘位置，静默回退相对默认路径会把日志刷进进程 cwd；非零非法输入（拼错的级别、负数）照旧报错。
