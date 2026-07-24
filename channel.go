package logger

import (
	"context"
	"strings"

	"go.uber.org/zap"
)

// ChannelLogger writes classified logs. Unconfigured channels fall back to the
// default outputs and still attach a `channel` field to each entry.
type ChannelLogger struct {
	channel string
	name    string
	fields  []zap.Field
}

// Channel 返回指定名称的 ChannelLogger；名称首尾空白会被去除。
// 未注册的 channel 写默认输出并自动附加 channel 字段。
func Channel(name string) *ChannelLogger {
	return &ChannelLogger{channel: strings.TrimSpace(name)}
}

// Zap 返回该 channel 的底层 *zap.Logger 供调用方直接使用。
// 返回的 logger 已抵消内部包装层的 caller skip，直接调用时 caller 指向真实调用点。
func (l *ChannelLogger) Zap() *zap.Logger {
	state := snapshotLoggerState()
	if state == nil {
		return zap.NewNop()
	}

	return l.derive(state).WithOptions(zap.AddCallerSkip(-1))
}

// Sugar 返回该 channel 的底层 *zap.SugaredLogger 供调用方直接使用，caller 语义同 Zap。
func (l *ChannelLogger) Sugar() *zap.SugaredLogger {
	return l.Zap().Sugar()
}

// With 返回附加了预绑定字段的新 ChannelLogger，原实例不受影响。
func (l *ChannelLogger) With(fields ...zap.Field) *ChannelLogger {
	combined := append(copyFields(l.fields), fields...)

	return &ChannelLogger{
		channel: l.channel,
		name:    l.name,
		fields:  combined,
	}
}

// Named 返回追加了 logger 名称段的新 ChannelLogger，名称以 "." 级联。
func (l *ChannelLogger) Named(name string) *ChannelLogger {
	return &ChannelLogger{
		channel: l.channel,
		name:    joinLoggerName(l.name, name),
		fields:  copyFields(l.fields),
	}
}

// Debug 以 Debug 级别记录结构化字段日志。
func (l *ChannelLogger) Debug(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Debug(msg, fields...)
}

// Info 以 Info 级别记录结构化字段日志。
func (l *ChannelLogger) Info(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Info(msg, fields...)
}

// Warn 以 Warn 级别记录结构化字段日志。
func (l *ChannelLogger) Warn(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Warn(msg, fields...)
}

// Error 以 Error 级别记录结构化字段日志。
func (l *ChannelLogger) Error(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Error(msg, fields...)
}

// DPanic 以 DPanic 级别记录结构化字段日志；development 模式下会 panic。
func (l *ChannelLogger) DPanic(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).DPanic(msg, fields...)
}

// Panic 以 Panic 级别记录结构化字段日志，随后 panic。
func (l *ChannelLogger) Panic(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Panic(msg, fields...)
}

// Fatal 以 Fatal 级别记录结构化字段日志，随后调用 os.Exit(1)。
func (l *ChannelLogger) Fatal(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Fatal(msg, fields...)
}

// Debugf 以 Debug 级别记录 fmt 风格格式化日志。
func (l *ChannelLogger) Debugf(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Sugar().Debugf(format, args...)
}

// Infof 以 Info 级别记录 fmt 风格格式化日志。
func (l *ChannelLogger) Infof(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Sugar().Infof(format, args...)
}

// Debugw 以 Debug 级别记录 Sugar 风格 key-value 日志。
func (l *ChannelLogger) Debugw(msg string, keysAndValues ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Sugar().Debugw(msg, keysAndValues...)
}

// Infow 以 Info 级别记录 Sugar 风格 key-value 日志。
func (l *ChannelLogger) Infow(msg string, keysAndValues ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Sugar().Infow(msg, keysAndValues...)
}

// Warnw 以 Warn 级别记录 Sugar 风格 key-value 日志。
func (l *ChannelLogger) Warnw(msg string, keysAndValues ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Sugar().Warnw(msg, keysAndValues...)
}

// Errorw 以 Error 级别记录 Sugar 风格 key-value 日志。
func (l *ChannelLogger) Errorw(msg string, keysAndValues ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Sugar().Errorw(msg, keysAndValues...)
}

// Warnf 以 Warn 级别记录 fmt 风格格式化日志。
func (l *ChannelLogger) Warnf(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Sugar().Warnf(format, args...)
}

// Errorf 以 Error 级别记录 fmt 风格格式化日志。
func (l *ChannelLogger) Errorf(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Sugar().Errorf(format, args...)
}

// DPanicf 以 DPanic 级别记录 fmt 风格格式化日志；development 模式下会 panic。
func (l *ChannelLogger) DPanicf(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Sugar().DPanicf(format, args...)
}

// Panicf 以 Panic 级别记录 fmt 风格格式化日志，随后 panic。
func (l *ChannelLogger) Panicf(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Sugar().Panicf(format, args...)
}

// Fatalf 以 Fatal 级别记录 fmt 风格格式化日志，随后调用 os.Exit(1)。
func (l *ChannelLogger) Fatalf(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Sugar().Fatalf(format, args...)
}

// DebugCtx 以 Debug 级别记录结构化字段日志，并自动合并 ContextFieldsFunc 从 ctx 提取的字段。
func (l *ChannelLogger) DebugCtx(ctx context.Context, msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Debug(msg, ctxFields(ctx, state, fields)...)
}

// InfoCtx 以 Info 级别记录结构化字段日志，并自动合并 ctx 字段。
func (l *ChannelLogger) InfoCtx(ctx context.Context, msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Info(msg, ctxFields(ctx, state, fields)...)
}

// WarnCtx 以 Warn 级别记录结构化字段日志，并自动合并 ctx 字段。
func (l *ChannelLogger) WarnCtx(ctx context.Context, msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Warn(msg, ctxFields(ctx, state, fields)...)
}

// ErrorCtx 以 Error 级别记录结构化字段日志，并自动合并 ctx 字段。
func (l *ChannelLogger) ErrorCtx(ctx context.Context, msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Error(msg, ctxFields(ctx, state, fields)...)
}

// LogIf 在 err != nil 时以 Error 级别记录一条日志；err 为 nil 时什么都不做。
func (l *ChannelLogger) LogIf(err error) {
	if err != nil {
		state := currentLoggerState()
		if state == nil {
			return
		}
		defer state.release()
		l.derive(state).Error("error occurred", zap.Error(err))
	}
}

// HInfo 以 Info 级别写日志，并通过 Messager 异步推送消息（未配置 Messager 时仅写日志）。
func (l *ChannelLogger) HInfo(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Info(msg, fields...)
	if state.messager != nil {
		state.messager.Send(formatHookFieldsMsg(msg, withChannelField(l.channel, fields), state.fieldRedactor))
	}
}

// HInfof 以 Info 级别写 fmt 风格日志，并通过 Messager 异步推送消息。
func (l *ChannelLogger) HInfof(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Sugar().Infof(format, args...)
	if state.messager != nil {
		state.messager.Send(formatChannelMsg(l.channel, formatMsg(format, args)))
	}
}

// HInfoTo 以 Info 级别写日志，并通过 Messager 异步推送消息到指定 URL。
func (l *ChannelLogger) HInfoTo(url, msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Info(msg, fields...)
	if state.messager != nil {
		state.messager.SendTo(url, formatHookFieldsMsg(msg, withChannelField(l.channel, fields), state.fieldRedactor))
	}
}

// HInfoTof 以 Info 级别写 fmt 风格日志，并通过 Messager 异步推送消息到指定 URL。
func (l *ChannelLogger) HInfoTof(url, format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Sugar().Infof(format, args...)
	if state.messager != nil {
		state.messager.SendTo(url, formatChannelMsg(l.channel, formatMsg(format, args)))
	}
}

// HError 以 Error 级别写日志，并通过 Messager 异步推送消息（未配置 Messager 时仅写日志）。
func (l *ChannelLogger) HError(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Error(msg, fields...)
	if state.messager != nil {
		state.messager.Send(formatHookFieldsMsg(msg, withChannelField(l.channel, fields), state.fieldRedactor))
	}
}

// HErrorf 以 Error 级别写 fmt 风格日志，并通过 Messager 异步推送消息。
func (l *ChannelLogger) HErrorf(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Sugar().Errorf(format, args...)
	if state.messager != nil {
		state.messager.Send(formatChannelMsg(l.channel, formatMsg(format, args)))
	}
}

// HErrorTo 以 Error 级别写日志，并通过 Messager 异步推送消息到指定 URL。
func (l *ChannelLogger) HErrorTo(url, msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Error(msg, fields...)
	if state.messager != nil {
		state.messager.SendTo(url, formatHookFieldsMsg(msg, withChannelField(l.channel, fields), state.fieldRedactor))
	}
}

// HErrorTof 以 Error 级别写 fmt 风格日志，并通过 Messager 异步推送消息到指定 URL。
func (l *ChannelLogger) HErrorTof(url, format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Sugar().Errorf(format, args...)
	if state.messager != nil {
		state.messager.SendTo(url, formatChannelMsg(l.channel, formatMsg(format, args)))
	}
}

func (l *ChannelLogger) derive(state *loggerState) *zap.Logger {
	logger := state.channelLogger(l.channel)
	if l.name != "" {
		logger = logger.Named(l.name)
	}
	if len(l.fields) > 0 {
		logger = logger.With(l.fields...)
	}

	return logger
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
