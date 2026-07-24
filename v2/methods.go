package logger

import (
	"context"
	"strings"

	"go.uber.org/zap"
)

// Zap 返回底层 *zap.Logger 供调用方直接使用或交给第三方库。
// 返回的 logger 已抵消内部包装层的 caller skip，直接调用时 caller 指向真实调用点。
func (l *Logger) Zap() *zap.Logger {
	return l.zap.WithOptions(zap.AddCallerSkip(-1))
}

// Sugar 返回底层 *zap.SugaredLogger 供调用方直接使用。
// 返回的 logger 已抵消内部包装层的 caller skip，直接调用时 caller 指向真实调用点。
func (l *Logger) Sugar() *zap.SugaredLogger {
	return l.zap.WithOptions(zap.AddCallerSkip(-1)).Sugar()
}

// With 返回附加了预绑定字段的新 Logger，原实例不受影响。
func (l *Logger) With(fields ...zap.Field) *Logger {
	combined := append(copyFields(l.fields), fields...)
	return l.rebuild(l.rootLogger(), l.name, l.channel, combined)
}

// Named 返回追加了 logger 名称段的新 Logger，名称以 "." 级联。
func (l *Logger) Named(name string) *Logger {
	return l.rebuild(l.rootLogger(), joinLoggerName(l.name, name), l.channel, l.fields)
}

// Channel 返回指定名称的 channel Logger；名称首尾空白会被去除，空名返回原实例。
// 未注册的 channel 写默认输出并自动附加 channel 字段。
func (l *Logger) Channel(name string) *Logger {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return l
	}

	if cached := l.cachedRootChannel(trimmed); cached != nil {
		return cached
	}

	return l.rebuild(l.rootLogger(), l.name, trimmed, l.fields)
}

// DroppedMessages 返回异步 Messager 因队列满而丢弃的推送消息数量。
// 如果未配置 Messager，始终返回 0。
func (l *Logger) DroppedMessages() int64 {
	if l.state != nil && l.state.asyncMsg != nil {
		return l.state.asyncMsg.dropped.Load()
	}
	return 0
}

// SetLevel 运行时动态调整日志级别，影响所有 logger（包括 channel）。
// 支持: debug, info, warn, error, dpanic, panic, fatal.
func (l *Logger) SetLevel(level string) {
	if lvl, ok := levelMap[level]; ok && l.state != nil {
		l.state.atomicLevel.SetLevel(lvl)
	}
}

// GetLevel 返回当前日志级别字符串。
func (l *Logger) GetLevel() string {
	if l.state != nil {
		return l.state.atomicLevel.Level().String()
	}
	return "info"
}

// Undo 恢复 New 之前的 zap 全局 logger（zap.L()/zap.S()），幂等。
func (l *Logger) Undo() {
	if l.state != nil {
		l.state.Undo()
	}
}

// Sync 恢复 zap 全局 logger、flush 缓冲日志并关闭文件等资源，幂等。
// 调用后本 Logger 及其派生实例不应再用于写日志。
func (l *Logger) Sync() {
	if l.state != nil {
		l.state.Sync()
	}
}

// Debug 以 Debug 级别记录结构化字段日志。
func (l *Logger) Debug(msg string, fields ...zap.Field) {
	l.zap.Debug(msg, fields...)
}

// Info 以 Info 级别记录结构化字段日志。
func (l *Logger) Info(msg string, fields ...zap.Field) {
	l.zap.Info(msg, fields...)
}

// Warn 以 Warn 级别记录结构化字段日志。
func (l *Logger) Warn(msg string, fields ...zap.Field) {
	l.zap.Warn(msg, fields...)
}

// Error 以 Error 级别记录结构化字段日志。
func (l *Logger) Error(msg string, fields ...zap.Field) {
	l.zap.Error(msg, fields...)
}

// DPanic 以 DPanic 级别记录结构化字段日志；development 模式下会 panic。
func (l *Logger) DPanic(msg string, fields ...zap.Field) {
	l.zap.DPanic(msg, fields...)
}

// Panic 以 Panic 级别记录结构化字段日志，随后 panic。
func (l *Logger) Panic(msg string, fields ...zap.Field) {
	l.zap.Panic(msg, fields...)
}

// Fatal 以 Fatal 级别记录结构化字段日志，随后调用 os.Exit(1)。
func (l *Logger) Fatal(msg string, fields ...zap.Field) {
	l.zap.Fatal(msg, fields...)
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
	l.sugar.Debugw(msg, keysAndValues...)
}

// Infow 以 Info 级别记录 Sugar 风格 key-value 日志。
func (l *Logger) Infow(msg string, keysAndValues ...any) {
	l.sugar.Infow(msg, keysAndValues...)
}

// Warnw 以 Warn 级别记录 Sugar 风格 key-value 日志。
func (l *Logger) Warnw(msg string, keysAndValues ...any) {
	l.sugar.Warnw(msg, keysAndValues...)
}

// Errorw 以 Error 级别记录 Sugar 风格 key-value 日志。
func (l *Logger) Errorw(msg string, keysAndValues ...any) {
	l.sugar.Errorw(msg, keysAndValues...)
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
	l.zap.Debug(msg, l.ctxFields(ctx, fields)...)
}

// InfoCtx 以 Info 级别记录结构化字段日志，并自动合并 ctx 字段。
func (l *Logger) InfoCtx(ctx context.Context, msg string, fields ...zap.Field) {
	l.zap.Info(msg, l.ctxFields(ctx, fields)...)
}

// WarnCtx 以 Warn 级别记录结构化字段日志，并自动合并 ctx 字段。
func (l *Logger) WarnCtx(ctx context.Context, msg string, fields ...zap.Field) {
	l.zap.Warn(msg, l.ctxFields(ctx, fields)...)
}

// ErrorCtx 以 Error 级别记录结构化字段日志，并自动合并 ctx 字段。
func (l *Logger) ErrorCtx(ctx context.Context, msg string, fields ...zap.Field) {
	l.zap.Error(msg, l.ctxFields(ctx, fields)...)
}

func (l *Logger) ctxFields(ctx context.Context, fields []zap.Field) []zap.Field {
	if l.contextFields == nil {
		return fields
	}
	extracted := l.contextFields(ctx)
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
	if l.contextFields == nil {
		return kv
	}
	extracted := l.contextFields(ctx)
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
	l.sugar.Debugw(msg, l.ctxKeysAndValues(ctx, keysAndValues)...)
}

// InfowCtx 以 Info 级别记录 Sugar 风格 key-value 日志，并自动合并 ctx 字段。
// 行为参见 DebugwCtx。
func (l *Logger) InfowCtx(ctx context.Context, msg string, keysAndValues ...any) {
	l.sugar.Infow(msg, l.ctxKeysAndValues(ctx, keysAndValues)...)
}

// WarnwCtx 以 Warn 级别记录 Sugar 风格 key-value 日志，并自动合并 ctx 字段。
// 行为参见 DebugwCtx。
func (l *Logger) WarnwCtx(ctx context.Context, msg string, keysAndValues ...any) {
	l.sugar.Warnw(msg, l.ctxKeysAndValues(ctx, keysAndValues)...)
}

// ErrorwCtx 以 Error 级别记录 Sugar 风格 key-value 日志，并自动合并 ctx 字段。
// 行为参见 DebugwCtx。
func (l *Logger) ErrorwCtx(ctx context.Context, msg string, keysAndValues ...any) {
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
	if err != nil {
		l.zap.Error("error occurred", l.ctxFields(ctx, []zap.Field{zap.Error(err)})...)
	}
}

// WarnIfCtx 在 err != nil 时以 Warn 级别记录日志，并合并 ctx 注入的字段。
func (l *Logger) WarnIfCtx(ctx context.Context, err error) {
	if err != nil {
		l.zap.Warn("warning occurred", l.ctxFields(ctx, []zap.Field{zap.Error(err)})...)
	}
}

// HInfo 以 Info 级别写日志，并通过 Messager 异步推送消息（未配置 Messager 时仅写日志）。
func (l *Logger) HInfo(msg string, fields ...zap.Field) {
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
	if l.state == nil {
		return formatHookFieldsMsg(msg, fields, nil)
	}
	return formatHookFieldsMsg(msg, fields, l.state.fieldRedactor)
}

func (l *Logger) rootLogger() *zap.Logger {
	if l.base != nil {
		return l.base
	}

	return l.zap
}

func (l *Logger) channelRoute(name string) *channelRoute {
	if l.state == nil || l.state.channelRoutes == nil {
		return nil
	}

	return l.state.channelRoutes[name]
}

func (l *Logger) rebuild(base *zap.Logger, name, channel string, fields []zap.Field) *Logger {
	z := l.baseForChannel(channel)
	if name != "" {
		z = z.Named(name)
	}
	if len(fields) > 0 {
		z = z.With(fields...)
	}

	return &Logger{
		base:          base,
		zap:           z,
		sugar:         z.Sugar(),
		state:         l.state,
		messager:      l.messager,
		contextFields: l.contextFields,
		channel:       channel,
		name:          name,
		fields:        copyFields(fields),
	}
}

func (l *Logger) baseForChannel(channel string) *zap.Logger {
	if channel == "" {
		return l.rootLogger()
	}

	if route := l.channelRoute(channel); route != nil {
		return route.logger
	}

	if l.state == nil {
		return l.rootLogger().With(zap.String("channel", channel))
	}

	if cached, ok := l.state.dynamicChannelBases.Load(channel); ok {
		if z, ok := cached.(*zap.Logger); ok {
			return z
		}
	}

	logger := l.rootLogger().With(zap.String("channel", channel))

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

	if l.state == nil || l.state.rootChannels == nil {
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
