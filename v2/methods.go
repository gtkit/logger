package logger

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Zap 返回底层 *zap.Logger 供调用方直接使用或交给第三方库。
// 返回的 logger 已抵消内部包装层与 WithCallerSkip 的全部偏移，
// 直接调用时 caller 恒指向真实调用点。
func (l *Logger) Zap() *zap.Logger {
	return l.zap.WithOptions(zap.AddCallerSkip(-1 - l.callerSkip))
}

// Sugar 返回底层 *zap.SugaredLogger 供调用方直接使用。
// 返回的 logger 已抵消内部包装层与 WithCallerSkip 的全部偏移，
// 直接调用时 caller 恒指向真实调用点。
func (l *Logger) Sugar() *zap.SugaredLogger {
	return l.zap.WithOptions(zap.AddCallerSkip(-1 - l.callerSkip)).Sugar()
}

// WithCallerSkip 返回 caller skip 增加 delta 的派生 Logger，原实例不受影响。
// 供在本库外再包一层转发的 facade 使用：每包一层转发函数，skip +1，
// 使日志 caller 指向 facade 的调用方而非 facade 本身。
//
// 偏移以元数据形式保存：经 With/Named/Channel（含注册与动态 channel）继续派生
// 均保留偏移，且共享的 channel 缓存永远只存零偏移的 canonical 实例，
// 不会被任何调用方的偏移污染；Zap()/Sugar() 返回值的 caller 恒指向真实调用点。
func (l *Logger) WithCallerSkip(delta int) *Logger {
	if delta == 0 {
		return l
	}

	// l.zap 当前视图 = canonical(decorated) + l.callerSkip，叠加 delta 即为新视图；
	// base 恒为 canonical，不携带任何偏移。
	z := l.zap.WithOptions(zap.AddCallerSkip(delta))

	return &Logger{
		base:           l.base,
		zap:            z,
		sugar:          z.Sugar(),
		state:          l.state,
		messager:       l.messager,
		contextFields:  l.contextFields,
		channel:        l.channel,
		name:           l.name,
		fields:         copyFields(l.fields),
		callerSkip:     l.callerSkip + delta,
		boundRequestID: l.boundRequestID,
	}
}

// With 返回附加了预绑定字段的新 Logger，原实例不受影响。
func (l *Logger) With(fields ...zap.Field) *Logger {
	combined := append(copyFields(l.fields), fields...)
	return l.rebuild(l.name, l.channel, combined)
}

// Named 返回追加了 logger 名称段的新 Logger，名称以 "." 级联。
func (l *Logger) Named(name string) *Logger {
	return l.rebuild(joinLoggerName(l.name, name), l.channel, l.fields)
}

// Channel 返回指定名称的 channel Logger；名称首尾空白会被去除，空名返回原实例。
// 未注册的 channel 写默认输出并自动附加 channel 字段。
func (l *Logger) Channel(name string) *Logger {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return l
	}

	if cached := l.cachedRootChannel(trimmed); cached != nil {
		// 缓存里是 canonical（零偏移）实例；本实例带偏移时在其上重新应用，
		// 缓存本身永不携带调用方偏移。
		if l.callerSkip == 0 {
			return cached
		}
		return cached.WithCallerSkip(l.callerSkip)
	}

	return l.rebuild(l.name, trimmed, l.fields)
}

// DroppedMessages 返回异步 Messager 因队列满或 Sync 排空超时而丢弃的推送消息数量。
// 如果未配置 Messager，始终返回 0。
func (l *Logger) DroppedMessages() int64 {
	if l.state.asyncMsg != nil {
		return l.state.asyncMsg.dropped.Load()
	}
	return 0
}

// SetLevel 运行时动态调整日志级别，影响所有 logger（包括 channel）。
// 支持: debug, info, warn, error, dpanic, panic, fatal；未知级别返回错误且当前级别不变。
func (l *Logger) SetLevel(level string) error {
	lvl, ok := levelMap[level]
	if !ok {
		return fmt.Errorf("logger: invalid level %q", level)
	}
	l.state.atomicLevel.SetLevel(lvl)
	return nil
}

// GetLevel 返回当前日志级别字符串。
func (l *Logger) GetLevel() string {
	return l.state.atomicLevel.Level().String()
}

// Undo 恢复 New 之前的 zap 全局 logger（zap.L()/zap.S()），幂等；
// 以 WithReplaceGlobals(false) 构建的实例调用为安全 no-op。
//
// 契约：进程内只应有一个实例以默认方式（安装全局）构建（owner），其余实例
// 必须 WithReplaceGlobals(false)。多 owner 先后安装、乱序关闭属未定义行为——
// undo 链会恢复过期甚至已关闭的实例。「全局已被后来者替换则跳过恢复」的
// 指针判断仅是对该误用最常见形态的缓解，不构成热替换安全承诺。
func (l *Logger) Undo() {
	l.state.Undo()
}

// Sync flush 缓冲日志并关闭文件等资源，幂等；若构建时安装过 zap 全局
// （默认行为，见 WithReplaceGlobals）则按 Undo 的契约与缓解语义处理全局恢复。
// 调用后本 Logger 及其派生实例不应再用于写日志。
func (l *Logger) Sync() {
	l.state.Sync()
}

// Debug 以 Debug 级别记录结构化字段日志。
func (l *Logger) Debug(msg string, fields ...zap.Field) {
	l.zap.Debug(msg, l.recordFields(fields)...)
}

// Info 以 Info 级别记录结构化字段日志。
func (l *Logger) Info(msg string, fields ...zap.Field) {
	l.zap.Info(msg, l.recordFields(fields)...)
}

// Warn 以 Warn 级别记录结构化字段日志。
func (l *Logger) Warn(msg string, fields ...zap.Field) {
	l.zap.Warn(msg, l.recordFields(fields)...)
}

// Error 以 Error 级别记录结构化字段日志。
func (l *Logger) Error(msg string, fields ...zap.Field) {
	l.zap.Error(msg, l.recordFields(fields)...)
}

// DPanic 以 DPanic 级别记录结构化字段日志；development 模式下会 panic。
func (l *Logger) DPanic(msg string, fields ...zap.Field) {
	l.zap.DPanic(msg, l.recordFields(fields)...)
}

// Panic 以 Panic 级别记录结构化字段日志，随后 panic。
func (l *Logger) Panic(msg string, fields ...zap.Field) {
	l.zap.Panic(msg, l.recordFields(fields)...)
}

// Fatal 以 Fatal 级别记录结构化字段日志，随后调用 os.Exit(1)。
func (l *Logger) Fatal(msg string, fields ...zap.Field) {
	l.zap.Fatal(msg, l.recordFields(fields)...)
}

// Debugf 以 Debug 级别记录 fmt 风格格式化日志。
func (l *Logger) Debugf(format string, args ...any) {
	l.sugar.Debugf(format, args...)
}

// Infof 以 Info 级别记录 fmt 风格格式化日志。
func (l *Logger) Infof(format string, args ...any) {
	l.sugar.Infof(format, args...)
}

// Debugw 以 Debug 级别记录 Sugar 风格 key-value 日志。
func (l *Logger) Debugw(msg string, keysAndValues ...any) {
	l.sugar.Debugw(msg, l.recordKV(keysAndValues)...)
}

// Infow 以 Info 级别记录 Sugar 风格 key-value 日志。
func (l *Logger) Infow(msg string, keysAndValues ...any) {
	l.sugar.Infow(msg, l.recordKV(keysAndValues)...)
}

// Warnw 以 Warn 级别记录 Sugar 风格 key-value 日志。
func (l *Logger) Warnw(msg string, keysAndValues ...any) {
	l.sugar.Warnw(msg, l.recordKV(keysAndValues)...)
}

// Errorw 以 Error 级别记录 Sugar 风格 key-value 日志。
func (l *Logger) Errorw(msg string, keysAndValues ...any) {
	l.sugar.Errorw(msg, l.recordKV(keysAndValues)...)
}

// Warnf 以 Warn 级别记录 fmt 风格格式化日志。
func (l *Logger) Warnf(format string, args ...any) {
	l.sugar.Warnf(format, args...)
}

// Errorf 以 Error 级别记录 fmt 风格格式化日志。
func (l *Logger) Errorf(format string, args ...any) {
	l.sugar.Errorf(format, args...)
}

// DPanicf 以 DPanic 级别记录 fmt 风格格式化日志；development 模式下会 panic。
func (l *Logger) DPanicf(format string, args ...any) {
	l.sugar.DPanicf(format, args...)
}

// Panicf 以 Panic 级别记录 fmt 风格格式化日志，随后 panic。
func (l *Logger) Panicf(format string, args ...any) {
	l.sugar.Panicf(format, args...)
}

// Fatalf 以 Fatal 级别记录 fmt 风格格式化日志，随后调用 os.Exit(1)。
func (l *Logger) Fatalf(format string, args ...any) {
	l.sugar.Fatalf(format, args...)
}

// DebugCtx 以 Debug 级别记录结构化字段日志，并自动合并 ContextFieldsFunc 从 ctx 提取的字段。
func (l *Logger) DebugCtx(ctx context.Context, msg string, fields ...zap.Field) {
	if !l.levelEnabled(zapcore.DebugLevel) {
		return
	}
	l.zap.Debug(msg, l.ctxFields(ctx, fields)...)
}

// InfoCtx 以 Info 级别记录结构化字段日志，并自动合并 ctx 字段。
func (l *Logger) InfoCtx(ctx context.Context, msg string, fields ...zap.Field) {
	if !l.levelEnabled(zapcore.InfoLevel) {
		return
	}
	l.zap.Info(msg, l.ctxFields(ctx, fields)...)
}

// WarnCtx 以 Warn 级别记录结构化字段日志，并自动合并 ctx 字段。
func (l *Logger) WarnCtx(ctx context.Context, msg string, fields ...zap.Field) {
	if !l.levelEnabled(zapcore.WarnLevel) {
		return
	}
	l.zap.Warn(msg, l.ctxFields(ctx, fields)...)
}

// ErrorCtx 以 Error 级别记录结构化字段日志，并自动合并 ctx 字段。
func (l *Logger) ErrorCtx(ctx context.Context, msg string, fields ...zap.Field) {
	if !l.levelEnabled(zapcore.ErrorLevel) {
		return
	}
	l.zap.Error(msg, l.ctxFields(ctx, fields)...)
}

// levelEnabled 供 *Ctx 方法在提取 ctx 字段前做级别短路：
// 级别关闭时直接返回，避免为注定丢弃的日志构造字段（热路径零额外分配）。
func (l *Logger) levelEnabled(lvl zapcore.Level) bool {
	return l.zap.Core().Enabled(lvl)
}

// recordFields 对调用点字段做 request_id 去重，所有记录方法共用：
// With 预绑定已烧进 zap 视图不可移除，其存在时剔除调用点同名字段；
// 否则同源重复只保留最后一个。至多一个命中时零分配原样返回。
func (l *Logger) recordFields(fields []zap.Field) []zap.Field {
	if l.boundRequestID {
		return filterOutRequestID(fields)
	}
	return normalizeRequestID(fields)
}

// recordKV 是 recordFields 的 Sugar key-value 形态。
func (l *Logger) recordKV(kv []any) []any {
	if l.boundRequestID {
		return filterOutRequestIDKV(kv)
	}
	return normalizeRequestIDKV(kv)
}

func (l *Logger) ctxFields(ctx context.Context, fields []zap.Field) []zap.Field {
	// request_id 四级优先级：With 预绑定 > 调用点 > 自定义 ctx 字段 > 内建。
	fields = l.recordFields(fields)
	extracted := l.extractCtxFields(ctx, l.boundRequestID || hasRequestIDField(fields))
	if len(extracted) == 0 {
		return fields
	}
	merged := make([]zap.Field, 0, len(extracted)+len(fields))
	merged = append(merged, extracted...)
	merged = append(merged, fields...)
	return merged
}

// ctxKeysAndValues 把 contextFields 提取的 zap.Field 前置到 Sugar 风格的 keysAndValues。
// Sugar 的 *w 系列方法识别 zap.Field 类型，因此以原 Field 形式注入即可。
func (l *Logger) ctxKeysAndValues(ctx context.Context, kv []any) []any {
	// 与 ctxFields 相同的四级优先级；kv 形态下剔除「字符串 key+值」与内联 Field 两种同名项。
	kv = l.recordKV(kv)
	extracted := l.extractCtxFields(ctx, l.boundRequestID || hasRequestIDKey(kv))
	if len(extracted) == 0 {
		return kv
	}
	merged := make([]any, 0, len(extracted)+len(kv))
	for _, f := range extracted {
		merged = append(merged, f)
	}
	merged = append(merged, kv...)
	return merged
}

// DebugwCtx 以 Debug 级别记录 Sugar 风格 key-value 日志，并自动合并 ContextFieldsFunc 从 ctx 提取的字段。
//
// 与 Debugw 的差别：在调用 zap Sugar 之前，会通过 ctxKeysAndValues 把 contextFields(ctx) 提取的
// zap.Field 前置到 keysAndValues。未配置 WithContextFields 时，行为等价于 Debugw。
//
// 用法：
//
//	log.DebugwCtx(ctx, "cache miss", "key", "user:42", "tier", "L2")
func (l *Logger) DebugwCtx(ctx context.Context, msg string, keysAndValues ...any) {
	if !l.levelEnabled(zapcore.DebugLevel) {
		return
	}
	l.sugar.Debugw(msg, l.ctxKeysAndValues(ctx, keysAndValues)...)
}

// InfowCtx 以 Info 级别记录 Sugar 风格 key-value 日志，并自动合并 ctx 字段。
// 行为参见 DebugwCtx。
func (l *Logger) InfowCtx(ctx context.Context, msg string, keysAndValues ...any) {
	if !l.levelEnabled(zapcore.InfoLevel) {
		return
	}
	l.sugar.Infow(msg, l.ctxKeysAndValues(ctx, keysAndValues)...)
}

// WarnwCtx 以 Warn 级别记录 Sugar 风格 key-value 日志，并自动合并 ctx 字段。
// 行为参见 DebugwCtx。
func (l *Logger) WarnwCtx(ctx context.Context, msg string, keysAndValues ...any) {
	if !l.levelEnabled(zapcore.WarnLevel) {
		return
	}
	l.sugar.Warnw(msg, l.ctxKeysAndValues(ctx, keysAndValues)...)
}

// ErrorwCtx 以 Error 级别记录 Sugar 风格 key-value 日志，并自动合并 ctx 字段。
// 行为参见 DebugwCtx。
func (l *Logger) ErrorwCtx(ctx context.Context, msg string, keysAndValues ...any) {
	if !l.levelEnabled(zapcore.ErrorLevel) {
		return
	}
	l.sugar.Errorw(msg, l.ctxKeysAndValues(ctx, keysAndValues)...)
}

// LogIf 在 err != nil 时以 Error 级别记录一条日志；err 为 nil 时什么都不做。
func (l *Logger) LogIf(err error) {
	if err != nil {
		l.zap.Error("error occurred", zap.Error(err))
	}
}

// WarnIf 在 err != nil 时以 Warn 级别记录一条日志。
func (l *Logger) WarnIf(err error) {
	if err != nil {
		l.zap.Warn("warning occurred", zap.Error(err))
	}
}

// LogIfCtx 在 err != nil 时以 Error 级别记录日志，并合并 ctx 注入的字段。
func (l *Logger) LogIfCtx(ctx context.Context, err error) {
	if err != nil && l.levelEnabled(zapcore.ErrorLevel) {
		l.zap.Error("error occurred", l.ctxFields(ctx, []zap.Field{zap.Error(err)})...)
	}
}

// WarnIfCtx 在 err != nil 时以 Warn 级别记录日志，并合并 ctx 注入的字段。
func (l *Logger) WarnIfCtx(ctx context.Context, err error) {
	if err != nil && l.levelEnabled(zapcore.WarnLevel) {
		l.zap.Warn("warning occurred", l.ctxFields(ctx, []zap.Field{zap.Error(err)})...)
	}
}

// HInfo 以 Info 级别写日志，并通过 Messager 异步推送消息（未配置 Messager 时仅写日志）。
func (l *Logger) HInfo(msg string, fields ...zap.Field) {
	fields = l.recordFields(fields)
	l.zap.Info(msg, fields...)
	if l.messager != nil {
		l.messager.Send(l.formatHookFieldsMsg(msg, withChannelField(l.channel, fields)))
	}
}

// HInfof 以 Info 级别写 fmt 风格日志，并通过 Messager 异步推送消息。
func (l *Logger) HInfof(format string, args ...any) {
	l.sugar.Infof(format, args...)
	if l.messager != nil {
		l.messager.Send(formatChannelMsg(l.channel, formatMsg(format, args)))
	}
}

// HInfoTo 以 Info 级别写日志，并通过 Messager 异步推送消息到指定 URL。
func (l *Logger) HInfoTo(url, msg string, fields ...zap.Field) {
	fields = l.recordFields(fields)
	l.zap.Info(msg, fields...)
	if l.messager != nil {
		l.messager.SendTo(url, l.formatHookFieldsMsg(msg, withChannelField(l.channel, fields)))
	}
}

// HInfoTof 以 Info 级别写 fmt 风格日志，并通过 Messager 异步推送消息到指定 URL。
func (l *Logger) HInfoTof(url, format string, args ...any) {
	l.sugar.Infof(format, args...)
	if l.messager != nil {
		l.messager.SendTo(url, formatChannelMsg(l.channel, formatMsg(format, args)))
	}
}

// HError 以 Error 级别写日志，并通过 Messager 异步推送消息（未配置 Messager 时仅写日志）。
func (l *Logger) HError(msg string, fields ...zap.Field) {
	fields = l.recordFields(fields)
	l.zap.Error(msg, fields...)
	if l.messager != nil {
		l.messager.Send(l.formatHookFieldsMsg(msg, withChannelField(l.channel, fields)))
	}
}

// HErrorf 以 Error 级别写 fmt 风格日志，并通过 Messager 异步推送消息。
func (l *Logger) HErrorf(format string, args ...any) {
	l.sugar.Errorf(format, args...)
	if l.messager != nil {
		l.messager.Send(formatChannelMsg(l.channel, formatMsg(format, args)))
	}
}

// HErrorTo 以 Error 级别写日志，并通过 Messager 异步推送消息到指定 URL。
func (l *Logger) HErrorTo(url, msg string, fields ...zap.Field) {
	fields = l.recordFields(fields)
	l.zap.Error(msg, fields...)
	if l.messager != nil {
		l.messager.SendTo(url, l.formatHookFieldsMsg(msg, withChannelField(l.channel, fields)))
	}
}

// HErrorTof 以 Error 级别写 fmt 风格日志，并通过 Messager 异步推送消息到指定 URL。
func (l *Logger) HErrorTof(url, format string, args ...any) {
	l.sugar.Errorf(format, args...)
	if l.messager != nil {
		l.messager.SendTo(url, formatChannelMsg(l.channel, formatMsg(format, args)))
	}
}

func (l *Logger) formatHookFieldsMsg(msg string, fields []zap.Field) string {
	return formatHookFieldsMsg(msg, fields, l.state.fieldRedactor)
}

func (l *Logger) rebuild(name, channel string, fields []zap.Field) *Logger {
	// 预绑定字段构建期同源归一化（链式 With 重复 request_id 保留最后一个）——
	// 字段在此烧进 zap 视图，归一化后所有方法（含非 Ctx）都不会输出重复 key。
	fields = normalizeRequestID(fields)
	// z 全程沿 canonical 链路构建（base 与 channel 缓存均为零偏移），
	// 偏移作为元数据在最后一步统一应用，保证任何派生路径都不污染共享缓存。
	z := l.baseForChannel(channel)
	if name != "" {
		z = z.Named(name)
	}
	if len(fields) > 0 {
		z = z.With(fields...)
	}
	if l.callerSkip != 0 {
		z = z.WithOptions(zap.AddCallerSkip(l.callerSkip))
	}

	return &Logger{
		base:           l.base,
		zap:            z,
		sugar:          z.Sugar(),
		state:          l.state,
		messager:       l.messager,
		contextFields:  l.contextFields,
		channel:        channel,
		name:           name,
		fields:         copyFields(fields),
		callerSkip:     l.callerSkip,
		boundRequestID: hasRequestIDField(fields),
	}
}

func (l *Logger) baseForChannel(channel string) *zap.Logger {
	if channel == "" {
		return l.base
	}

	if route := l.state.channelRoutes[channel]; route != nil {
		return route.logger
	}

	if cached, ok := l.state.dynamicChannelBases.Load(channel); ok {
		if z, ok := cached.(*zap.Logger); ok {
			return z
		}
	}

	logger := l.base.With(zap.String("channel", channel))

	// CAS 预留缓存 slot，确保计数不超过上限。
	for {
		cnt := l.state.dynamicChannelBasesCnt.Load()
		if cnt >= maxDynamicChannels {
			return logger
		}
		if l.state.dynamicChannelBasesCnt.CompareAndSwap(cnt, cnt+1) {
			break
		}
	}

	actual, loaded := l.state.dynamicChannelBases.LoadOrStore(channel, logger)
	if loaded {
		// 已有缓存，释放预留的 slot。
		l.state.dynamicChannelBasesCnt.Add(-1)
	}

	if z, ok := actual.(*zap.Logger); ok {
		return z
	}

	return logger
}

func (l *Logger) cachedRootChannel(channel string) *Logger {
	if !l.isRootContext() {
		return nil
	}

	return l.state.rootChannels[channel]
}

func (l *Logger) isRootContext() bool {
	return l.channel == "" && l.name == "" && len(l.fields) == 0
}

func copyFields(fields []zap.Field) []zap.Field {
	if len(fields) == 0 {
		return nil
	}

	cloned := make([]zap.Field, len(fields))
	copy(cloned, fields)

	return cloned
}

func joinLoggerName(current, next string) string {
	if current == "" {
		return next
	}
	if next == "" {
		return current
	}

	return current + "." + next
}

func withChannelField(channel string, fields []zap.Field) []zap.Field {
	if channel == "" {
		return fields
	}

	enriched := make([]zap.Field, 0, len(fields)+1)
	enriched = append(enriched, zap.String("channel", channel))
	enriched = append(enriched, fields...)

	return enriched
}

func formatChannelMsg(channel, msg string) string {
	if channel == "" {
		return msg
	}

	return "[channel=" + channel + "] " + msg
}
