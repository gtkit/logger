# Changelog

本仓库托管两个独立 Go module：

- `github.com/gtkit/logger`（v1，仓库根目录）
- `github.com/gtkit/logger/v2`（v2 子目录）

每个 module 各自维护版本号；本文件按 module + 版本号倒序记录变更。

格式参考 [Keep a Changelog](https://keepachangelog.com)，版本号遵循 [Semantic Versioning](https://semver.org)。

---

## logger v1.10.0 / v2.5.0 — 2026-09-30

### Added

- 导出哨兵错误 `ErrNoOutput`（v1 + v2）：`WithConsole(false)` 与 `WithFile(false)` 同时生效时 `New` 返回它，调用方用 `errors.Is` 判定后自行兜底，例如追加 `WithConsole(true)` 重建。错误文本不变，按文本匹配的既有调用方不受影响

---

## logger v1.9.0 / v2.4.0 — 2026-09-29

> ⚠ 破坏性变更（fail-closed）：输出全关时 `New` 报错；`SetLevel` 返回 `error`。迁移说明见下方 Changed 条目。

### Added

- `WithMessagerDrainTimeout(d)`（v1 + v2）：`Sync` 排空异步推送队列的最长等待，默认 5 秒；超时向 stderr 告警、未执行的推送计入 `DroppedMessages`，后台推送协程继续消费直到外部调用返回。此前排空无上限，外部 `Send` 挂起会卡死进程退出。`d <= 0` 报错
- `WithStacktraceLevel(level)`（v1 + v2）：附带 stacktrace 的最低级别，默认 `error` 与既有行为一致；空串保留默认，未知级别报错

### Changed

- ⚠ **`WithConsole(false)` 与 `WithFile(false)` 同时生效时 `New` 返回错误**（v1 + v2），取代此前静默回退 stdout。迁移：需要控制台输出的显式 `WithConsole(true)`；需要落盘的保留默认 `WithFile(true)`
- ⚠ **`SetLevel` 返回 `error`**（v1 包级函数 + v2 `Logger` 方法）：未知级别显式报错且当前级别不变，取代静默忽略。语句式调用 `logger.SetLevel("debug")` 无需改动；把它当方法值或接口赋值的调用方需要更新签名
- `WithRedactKeys` 多次调用取并集（v1 + v2）。此前后一次调用覆盖前一次，只有最后一组 key 被脱敏
- `WithChannel` 同名重复注册返回错误（v1 + v2）。此前后者静默覆盖前者
- channel 路径冲突检查改按最终文件名（`{path}-{level}.log`）比较（v1 + v2）：`./logs/` 与 `./logs` 这类只差尾斜杠的前缀产出不同文件，不再误报
- `New` 返回的 option 错误不再叠加 `logger: apply option:` 前缀（v1 + v2），错误文本只保留 option 自带的一层 `logger:` 前缀
- v2 全部记录方法统一 `request_id` 去重：结构化、Sugar key-value、H 系列与 `*Ctx` 系列的调用点字段都按「`With` 预绑定 > 调用点」处理，同源重复保留最后一个；此前仅 `*Ctx` 方法族去重，`l.With(request_id).Info(msg, request_id)` 会输出重复 JSON key
- v1 `ChannelLogger` 按当前 logger 状态缓存派生结果，热路径不再每条日志重新 `Named`/`With`；状态重配置后自动失效重建
- 测试文件按 `go fix` 现代化（`sync.WaitGroup.Go`）

---

## logger v2.3.0 — 2026-07-29

### Removed

- **删除 slog 桥接**（`Logger.SlogHandler` 及 `zapSlogHandler` 实现与全部配套测试）：不再提供 log/slog 兼容层。按严格 SemVer 属破坏性变更；因本库当前仅维护者自用、无外部消费方，经所有者决策随本次 minor 发布移除，不另开 /v3

### Added

- **进程默认实例与包级转发函数**（log/slog 同款形态）：`SetDefault(l)` 显式设定默认实例（不关闭被替换的旧实例，生命周期归调用方）、`Default()` 读取。包级函数精确清单：`Debug/Info/Warn/Error/DPanic/Panic/Fatal` 及 `f` 变体，`Debugw/Infow/Warnw/Errorw`，`DebugCtx/InfoCtx/WarnCtx/ErrorCtx` 及 `w` 变体，`LogIf/WarnIf/LogIfCtx/WarnIfCtx`，`HInfo/HInfof/HInfoTo/HInfoTof/HError/HErrorf/HErrorTo/HErrorTof`，`Sync`。caller skip 库内一次性校准，包级函数 caller 指向真实调用行（精确到行号的测试断言）。未 SetDefault 时写入懒创建的纯控制台兜底实例（不落盘、不替换 zap 全局；CAS 竞争安全，可反复重建）
- `Logger.WithCallerSkip(delta)`：偏移以元数据保存、base 与 channel 共享缓存恒为零偏移 canonical——经 With/Named/Channel（注册与动态两条缓存路径均已覆盖测试）继续派生保留偏移且不污染其他实例；`Zap()/Sugar()` 返回值的 caller 恒指向真实调用点
- `WithReplaceGlobals(enabled)` Option：控制构建时是否执行 `zap.ReplaceGlobals`，默认 true 与既有行为一致；辅助实例（独立 access 日志、兜底实例）传 false 后无需再用 `Undo()` 撤销全局抢占
- **requestId 约定内建**：`ContextWithRequestID(ctx, id)` / `RequestIDFromContext(ctx)`（库私有 key，两者对 nil ctx 均宽容不 panic）；所有 `*Ctx` 方法零配置自动合并 `request_id` 字段（有则带、无则略），与 `WithContextFields` 的自定义字段并存合并；同名 `request_id` 去重，优先级：`With` 预绑定 > 调用点字段 > 自定义上下文字段 > 内建（预绑定为作用域身份不可覆盖；同源内部重复亦归一化保留最后一个，链式 With 在构建期归一化、非 Ctx 方法一并受益；覆盖 fields 与 keysAndValues 双形态、全部两两冲突对、多源并存与同源重复组合；调用点/extractor 归一化仅作用于 *Ctx 方法族，避免重复 JSON key）。`*Ctx` 方法在级别关闭时不构造 ctx 字段（热路径零额外分配，附基准）。其他基础设施（如 GORM 日志适配器的 trace 提取）可复用 `RequestIDFromContext` 统一 **id 值**——注意字段名不同：logger 为 request_id、ormx/zlogger 为 trace_id
- `WithBasePath(base)` Option：相对日志路径（含 channel 路径）构建时锚定到 base 下，绝对路径原样；base 必须为绝对路径（否则报错），锚定保留尾斜杠前缀语义（默认路径 `./logs/` 锚定后仍是 `logs/` 目录而非 `logs-` 文件名前缀）；避免日志落盘位置依赖进程 cwd

### Changed

- **Option 零值容忍（仅限无文件系统副作用项）**：`WithLevel("")`、`WithMaxSize(0)` 由报错改为「零值 = 未配置，使用默认值」；`WithPath("")` **维持报错**——path 决定落盘位置，静默回退相对默认路径会把日志刷进进程 cwd。非零非法输入照旧报错
- **兜底实例控制台输出改走 stderr**：未 SetDefault 时包级函数写入的兜底日志属诊断输出，不再污染 CLI 的 stdout 协议/管道输出
- **v2/Makefile 发布守卫**：`make tag` 前置检查工作区干净、`go mod tidy -diff`、vet/lint/race、govulncheck/gosec、覆盖率 ≥80%、benchmark（-benchmem -count=3）、CHANGELOG 含目标版本段；`BUMP=patch|minor` 显式选择语义级别（major 走 /v3 流程不由脚本承载）；tag 说明改用简体中文
- **全局 owner 契约**：进程内只允许一个实例安装 zap 全局（owner），其余实例必须 `WithReplaceGlobals(false)`；多 owner 先后安装、乱序关闭属未定义行为。`Undo`/`Sync` 的条件化恢复（全局已被替换则跳过）仅为该误用最常见形态的缓解层，不构成热替换安全承诺
- `ContextFieldsFunc` GoDoc 明确实现必须并发安全；测试产物全部迁入 t.TempDir()，源码树不再残留 testlogs/

---

## logger v1.8.4 / v2.2.4 — 2026-07-27

### Changed

- **依赖升级**：`github.com/stretchr/testify v1.8.1 → v1.11.1`（v1 + v2，间接测试依赖）。仅更新依赖记录与校验和，不影响构建产物与运行时行为。

## logger v1.8.3 / v2.2.3 — 2026-07-24

### Fixed

- 修复 `Zap()`/`Sugar()`（v1 包级与 `Channel`、v2 `Logger` 方法）返回的 logger 以及 `zap.L()`/`zap.S()` 全局 logger 直接打日志时 caller 错位一帧的问题（此前显示 `runtime/proc.go` 等无效位置），现在 caller 指向调用方真实代码位置；经由本库包装方法打日志的 caller 行为不变。
- 修复 `SlogHandler` 违反 `slog.Handler` 契约的两处问题（v1 + v2）：零值 `slog.Attr` 现在会被忽略（不再输出空 key 字段），`WithGroup("")` 现在返回原 handler 本身。

### Changed

- **依赖升级**：`github.com/gtkit/logrotate v1.1.2 → v1.1.3`（v1 + v2）。上游修复夏令时时区跨天边界计算、同一毫秒多次轮转备份文件名冲突、`MaxSize` 为负数时写入永久失败等问题；结构体字面量接入方式不受影响，无需迁移。
- 补齐全部导出 API 的 GoDoc、包级文档（doc.go）与核心 API 的 Example 测试；README 新增「获取底层 zap logger」使用说明。

## logger v1.8.2 / v2.2.2 — 2026-07-21

### Fixed

- 修复异步 `Messager` 在发送与关闭并发时可能触发 panic 的问题；关闭操作现在具备幂等性，并会排空已进入队列的消息。
- 隔离外部 `Messager` 实现的 panic，单次推送异常不再终止异步消息工作协程。
- 修复 `WithRedactKeys` 可能修改调用方字段切片的问题，并确保 H 系列消息推送同样应用字段脱敏规则。
- 修复 v1 `SlogHandler` 与日志器重新配置并发执行时可能提前关闭旧日志资源的问题。
- 忽略终端和管道环境下无实际影响的 `Sync` 错误，同时继续报告真实文件同步错误。

### Changed

- 补充 v2 异步消息队列大小配置和丢弃消息计数的使用文档。

## logger v1.8.1 / v2.2.1 — 2026-06-12

### Changed

- **依赖升级**：`github.com/gtkit/logrotate v1.1.1 → v1.1.2`（v1 + v2）。本次 logrotate 升级保持结构体字面量配置兼容；现有 `&logrotate.Logger{...}` 接入不需要迁移到新 `logrotate.New(...)` API。

### Fixed

- **`WithMaxAge(0)` 现在合法**（v1 + v2），与 README 和底层 logrotate 语义对齐：`0` 表示不按时间删除历史日志，负数才返回配置错误。此前用户无法通过公开 Option 配出文档中的 `WithMaxAge(0) + WithMaxBackups(0)` 关闭清理组合。

---

## logger v1.8.0 / v2.2.0 — 2026-06-10

### Changed — 行为变更

- **轮转引擎从 lumberjack 迁移到自研 [`github.com/gtkit/logrotate`](https://github.com/gtkit/logrotate) v1.1.1**（v1 + v2）。删除内部 `dailyWriteSyncer`（daily.go）——日切、按大小轮转、gzip 压缩与过期清理统一由 logrotate 完成（后台清理挂入 writer 的 `Close` 生命周期，无 goroutine 逃逸）。
  - logrotate v1.1.1 修复了 `Close()` 后跨天重开仍写入旧日期文件的问题；已用本包的精确配置（DailyFilename + LocalTime + Compress）对跨天重开、在线日切、同日重开追加三个场景做过 `-race` 交叉验证。
- **`WithDivision` 新增 `"both"` 模式**（按天日切 + 单日内按 `MaxSize` 轮转同时生效），并且 ⚠ **默认值从 `"size"` 改为 `"both"`**。
  - **影响**：不显式调用 `WithDivision` 的用户，活跃日志文件名将从 `{path}-{level}.log` 变为 `{path}-{level}-2006-01-02.log`，filebeat / fluentd 等采集器的路径通配需同步调整。
  - 需保持旧文件名与旧行为，请显式 `WithDivision("size")`。

### Fixed — 修复

- **`WithSampling` 与 `WithRedactKeys` 同时启用时采样静默失效**（影响 v1.7.1 / v2.1.1，两 option 均在该版本引入）。
  - **原因**：`redactCore` 包装在 zap sampler 之外，其 `Check` 把自身 AddCore 进 CheckedEntry 而不调用内层 `Check`，绕过了 sampler 的采样判定——采样+脱敏同开时所有日志原样写入，采样保护失效。
  - **修复**：调整包装顺序为脱敏在内、sampler 在最外层（`With()` 预绑定字段经 `sampler.With` 透传，脱敏语义不变），并补充两者同开的组合回归测试。单独使用任一 option 的行为不受影响。

---

## logger v1.7.1 / v2.1.1 — 2026-06-08

### Added — 新增公开 API（向后兼容）

- `WithSampling(first, thereafter int)`（v1 + v2）—— 启用 zap 原生采样（tick 1s），同 level+message 每窗口先放行 `first` 条、之后每 `thereafter` 条放行一条，防高频日志打爆磁盘 / 拖垮下游。默认关闭，channel 继承。
- `WithRedactKeys(keys ...string)`（v1 + v2）—— 按字段名脱敏，命中字段值替换为 `[REDACTED]`，用于屏蔽 password / token / 手机号等敏感信息。默认不启用时零开销。
- **fallback 告警**（仅 v1）—— 在 `New()`/`NewZap()` 之前打日志（走开发期 console fallback、配置未生效）时，向 stderr 告警一次，便于发现 init 顺序错误。v2 为实例式 API（`New` 返回 `*Logger`），不存在该 footgun，故不涉及。

### Fixed — 修复

- **daily 模式历史文件不再无限堆积**。
  - **旧行为**：`WithDivision("daily")` 下，每天用不同文件名新建 lumberjack，而 lumberjack 的 `MaxAge`/`MaxBackups` 只清理「单个文件名派生的备份」，因此**昨天起的整份日切文件永远不会被删除**——`WithMaxAge` / `WithMaxBackups` 在 daily 模式下被静默忽略，磁盘持续增长。
  - **新行为**：在**进程启动**与**每次跨天切换**时，异步按 `MaxAge` / `MaxBackups` 回收 `{path}-{level}-*.log`（含 `.log.gz`）历史文件，使 daily 与 size 模式保留语义一致。清理在后台 goroutine 执行、不阻塞写入、单飞（同一时刻至多一个），并挂入 writer 的 `Close` 生命周期（`Close` 会等待在途回收完成，无 goroutine 逃逸）。
  - ⚠ **行为变更注意**：升级后 daily 模式会真正按 `MaxAge`（默认 7 天）/ `MaxBackups`（默认 50）删除旧文件。若此前依赖「daily 文件永久保留」，请显式 `WithMaxAge(0)` + `WithMaxBackups(0)` 关闭清理。
  - size 模式不受影响（其清理一直由 lumberjack 正常完成）。

---

## logger v1.7.0 — 2026-05-12

> ⚠ **本次发版跳过 v1.5.x / v1.6.x 整段废弃版本号**，从 v1.4.6 直接升至 v1.7.0。`v1.6.1` / `v1.6.2` 在 go.mod 中以 `retract` 指令标记为废弃，禁止使用。

### Added — 新增公开 API（向后兼容）

- `DebugwCtx(ctx, msg, keysAndValues...)` / `InfowCtx` / `WarnwCtx` / `ErrorwCtx`
  Sugar 风格 + 自动合并 `ContextFieldsFunc` 提取的字段（与已有的 `InfoCtx` 等结构化方法对齐）。
- `WarnIf(err)` — `err != nil` 时以 Warn 级别记录一条日志。语义对照 `LogIf`，仅级别不同。
- `LogIfCtx(ctx, err)` — `err != nil` 时以 Error 级别记录，合并 ctx 注入字段。
- `WarnIfCtx(ctx, err)` — `err != nil` 时以 Warn 级别记录，合并 ctx 注入字段。

### Changed — 行为变更（⚠ 升级注意）

- **Channel 路径冲突校验收紧**。`validateChannelRoutes` 现在对 root + 所有 channel 做**全配对**路径冲突检查：
  - **旧行为**：`channel.path == root.path` 且 `duplicate-to-default=false` 时**静默放行**——运行期两个 lumberjack 实例竞争同一文件，rotate 时会丢数据 / 写错文件。
  - **新行为**：任何 channel 与 root 同路径（不论 duplicate 标志），或任何两个 channel 同路径 → `NewZap()` 在初始化阶段直接返回 error，附带具体诊断信息。
  - **迁移**：如果升级后看到 `channel "X" path %q overlaps default path` 或 `channel "A" path conflicts with channel "B"` 报错，把对应 channel.path 改成独立目录即可。旧配置在新版本下原本就会数据竞争——新版本把这个 footgun 显式化为启动期错误。
- **依赖升级**：`go.uber.org/zap v1.27.1 → v1.28.0`（无 API 破坏，仅新增 `zapcore.CheckPreWriteHook` 扩展点，本库暂未使用）。

### Fixed — 修复

- `CronAdapter.Info` / `Error` 在奇数 `keysAndValues` 时输出末尾 `%!(EXTRA xxx)` 噪音 → 新增 `cronNormalizeKVs`，奇数尾元素以 `<MISSING>` 占位补齐，输出形如 `key=<MISSING>` 而非乱码。
- `currentLoggerState` 在 state 切换瞬间（旧 state retire 与新 state Store 之间的极小窗口）可能 tight-loop → 加 `runtime.Gosched()` 让出 P。

### Documented — 文档与注释

- `dailyWriteSyncer.Sync()` 返回 nil 的原因补充详细 godoc：lumberjack v2.2.x 不暴露 `Sync` 方法且其内部 `*os.File` 私有，zap `WriteSyncer.Sync` 语义本身是"flush 任意 buffered writer"而非"fsync 到磁盘"。本实现遵守该契约，不做磁盘级 fsync——这是 zap + lumberjack 体系下所有 rotator 包装的统一行为。
- README 新增 `H 系列方法一览` 章节，明确 "H 前缀方法 = 写日志 + Messager.Send" 语义；API 一览表同步加入 Ctx/If/Hook 三组方法。

### Internal — 仓库工程化

- 新增 `scripts/check-modules.sh` 多 module 发版审计脚本（遵循全局规则 4-PRE），同时检查 v1 和 v2 是否需要发版；挂入 `make release-check` target。

### Retracted

- 继续保留 `v1.6.1` / `v1.6.2` retract 标记（废弃版本号线上的 bad release，禁止使用）。

### Tests

- 新增并发竞态 / 配置校验 / 自动刷写测试：
  - `TestMultiChannelConcurrentWrites` — 多 channel `-race` 并发写入
  - `TestDailyWriteSyncerConcurrentCrossDayRotation` — daily 跨天切换竞态
  - `TestBufferedFlushIntervalAutoFlushes` — buffered 模式定时自动刷写
  - `TestValidateChannelRoutes_*` — 4 个路径冲突边界 case
  - `TestCronNormalizeKVs` — 奇偶 kv 长度对齐
- 整体覆盖率：79.9% → 80.2%。

---

## logger v2.1.0 — 2026-05-12

### Added — 新增公开 API（向后兼容）

- `(*Logger).DebugwCtx(ctx, msg, kv...)` / `InfowCtx` / `WarnwCtx` / `ErrorwCtx` — Sugar + Context 字段自动注入。
- `(*Logger).WarnIf(err)` — Warn 级条件日志。
- `(*Logger).LogIfCtx(ctx, err)` — Error 级条件日志，合并 ctx 字段。
- `(*Logger).WarnIfCtx(ctx, err)` — Warn 级条件日志，合并 ctx 字段。

### Changed — 行为变更（⚠ 升级注意）

- **Channel 路径冲突校验收紧**——与 v1.7.0 同样的语义，迁移方式相同。详见上文 v1.7.0 段落的说明。
- **依赖升级**：`go.uber.org/zap v1.27.1 → v1.28.0`。

### Fixed

- `CronAdapter` 奇数 kv 防御（同 v1）。

### Documented

- `Config` 类型新增 godoc：明确字段全部不导出是有意为之，应通过 Option 函数（`New(opts ...Option)`）构造，**禁止**反序列化为 `Config` 字面量。
- `dailyWriteSyncer.Sync` 同步补充注释（同 v1）。
- README 新增 `H 系列方法一览` 表 + Ctx/If 方法清单。

### Tests

- 与 v1 平行的并发 / 校验 / 自动刷写测试，整体覆盖率：84.7% → 85.2%。

---

## 在此版本之前

历次发版细节见 git log（`git log --oneline -- v2/` 或根目录）；本 CHANGELOG 文件起始于 v1.7.0 / v2.1.0。
