package logger

import (
	"context"
	"strings"

	"go.uber.org/zap"
)

func (l *Logger) Zap() *zap.Logger {
	return l.zap
}

func (l *Logger) Sugar() *zap.SugaredLogger {
	return l.sugar
}

func (l *Logger) With(fields ...zap.Field) *Logger {
	combined := append(copyFields(l.fields), fields...)
	return l.rebuild(l.rootLogger(), l.name, l.channel, combined)
}

func (l *Logger) Named(name string) *Logger {
	return l.rebuild(l.rootLogger(), joinLoggerName(l.name, name), l.channel, l.fields)
}

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

func (l *Logger) Undo() {
	if l.state != nil {
		l.state.Undo()
	}
}

func (l *Logger) Sync() {
	if l.state != nil {
		l.state.Sync()
	}
}

func (l *Logger) Debug(msg string, fields ...zap.Field) {
	l.zap.Debug(msg, fields...)
}

func (l *Logger) Info(msg string, fields ...zap.Field) {
	l.zap.Info(msg, fields...)
}

func (l *Logger) Warn(msg string, fields ...zap.Field) {
	l.zap.Warn(msg, fields...)
}

func (l *Logger) Error(msg string, fields ...zap.Field) {
	l.zap.Error(msg, fields...)
}

func (l *Logger) DPanic(msg string, fields ...zap.Field) {
	l.zap.DPanic(msg, fields...)
}

func (l *Logger) Panic(msg string, fields ...zap.Field) {
	l.zap.Panic(msg, fields...)
}

func (l *Logger) Fatal(msg string, fields ...zap.Field) {
	l.zap.Fatal(msg, fields...)
}

func (l *Logger) Debugf(format string, args ...any) {
	l.sugar.Debugf(format, args...)
}

func (l *Logger) Infof(format string, args ...any) {
	l.sugar.Infof(format, args...)
}

func (l *Logger) Debugw(msg string, keysAndValues ...any) {
	l.sugar.Debugw(msg, keysAndValues...)
}

func (l *Logger) Infow(msg string, keysAndValues ...any) {
	l.sugar.Infow(msg, keysAndValues...)
}

func (l *Logger) Warnw(msg string, keysAndValues ...any) {
	l.sugar.Warnw(msg, keysAndValues...)
}

func (l *Logger) Errorw(msg string, keysAndValues ...any) {
	l.sugar.Errorw(msg, keysAndValues...)
}

func (l *Logger) Warnf(format string, args ...any) {
	l.sugar.Warnf(format, args...)
}

func (l *Logger) Errorf(format string, args ...any) {
	l.sugar.Errorf(format, args...)
}

func (l *Logger) DPanicf(format string, args ...any) {
	l.sugar.DPanicf(format, args...)
}

func (l *Logger) Panicf(format string, args ...any) {
	l.sugar.Panicf(format, args...)
}

func (l *Logger) Fatalf(format string, args ...any) {
	l.sugar.Fatalf(format, args...)
}

func (l *Logger) DebugCtx(ctx context.Context, msg string, fields ...zap.Field) {
	l.zap.Debug(msg, l.ctxFields(ctx, fields)...)
}

func (l *Logger) InfoCtx(ctx context.Context, msg string, fields ...zap.Field) {
	l.zap.Info(msg, l.ctxFields(ctx, fields)...)
}

func (l *Logger) WarnCtx(ctx context.Context, msg string, fields ...zap.Field) {
	l.zap.Warn(msg, l.ctxFields(ctx, fields)...)
}

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

func (l *Logger) HInfo(msg string, fields ...zap.Field) {
	l.zap.Info(msg, fields...)
	if l.messager != nil {
		l.messager.Send(formatFieldsMsg(msg, withChannelField(l.channel, fields)))
	}
}

func (l *Logger) HInfof(format string, args ...any) {
	l.sugar.Infof(format, args...)
	if l.messager != nil {
		l.messager.Send(formatChannelMsg(l.channel, formatMsg(format, args)))
	}
}

func (l *Logger) HInfoTo(url, msg string, fields ...zap.Field) {
	l.zap.Info(msg, fields...)
	if l.messager != nil {
		l.messager.SendTo(url, formatFieldsMsg(msg, withChannelField(l.channel, fields)))
	}
}

func (l *Logger) HInfoTof(url, format string, args ...any) {
	l.sugar.Infof(format, args...)
	if l.messager != nil {
		l.messager.SendTo(url, formatChannelMsg(l.channel, formatMsg(format, args)))
	}
}

func (l *Logger) HError(msg string, fields ...zap.Field) {
	l.zap.Error(msg, fields...)
	if l.messager != nil {
		l.messager.Send(formatFieldsMsg(msg, withChannelField(l.channel, fields)))
	}
}

func (l *Logger) HErrorf(format string, args ...any) {
	l.sugar.Errorf(format, args...)
	if l.messager != nil {
		l.messager.Send(formatChannelMsg(l.channel, formatMsg(format, args)))
	}
}

func (l *Logger) HErrorTo(url, msg string, fields ...zap.Field) {
	l.zap.Error(msg, fields...)
	if l.messager != nil {
		l.messager.SendTo(url, formatFieldsMsg(msg, withChannelField(l.channel, fields)))
	}
}

func (l *Logger) HErrorTof(url, format string, args ...any) {
	l.sugar.Errorf(format, args...)
	if l.messager != nil {
		l.messager.SendTo(url, formatChannelMsg(l.channel, formatMsg(format, args)))
	}
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
