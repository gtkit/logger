# Changelog

本仓库托管两个独立 Go module：

- `github.com/gtkit/logger`（v1，仓库根目录）
- `github.com/gtkit/logger/v2`（v2 子目录）

每个 module 各自维护版本号；本文件按 module + 版本号倒序记录变更。

格式参考 [Keep a Changelog](https://keepachangelog.com)，版本号遵循 [Semantic Versioning](https://semver.org)。

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
