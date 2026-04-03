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

func Channel(name string) *ChannelLogger {
	return &ChannelLogger{channel: strings.TrimSpace(name)}
}

func (l *ChannelLogger) Zap() *zap.Logger {
	state := snapshotLoggerState()
	if state == nil {
		return zap.NewNop()
	}

	return l.derive(state)
}

func (l *ChannelLogger) Sugar() *zap.SugaredLogger {
	return l.Zap().Sugar()
}

func (l *ChannelLogger) With(fields ...zap.Field) *ChannelLogger {
	combined := append(copyFields(l.fields), fields...)

	return &ChannelLogger{
		channel: l.channel,
		name:    l.name,
		fields:  combined,
	}
}

func (l *ChannelLogger) Named(name string) *ChannelLogger {
	return &ChannelLogger{
		channel: l.channel,
		name:    joinLoggerName(l.name, name),
		fields:  copyFields(l.fields),
	}
}

func (l *ChannelLogger) Debug(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Debug(msg, fields...)
}

func (l *ChannelLogger) Info(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Info(msg, fields...)
}

func (l *ChannelLogger) Warn(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Warn(msg, fields...)
}

func (l *ChannelLogger) Error(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Error(msg, fields...)
}

func (l *ChannelLogger) DPanic(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).DPanic(msg, fields...)
}

func (l *ChannelLogger) Panic(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Panic(msg, fields...)
}

func (l *ChannelLogger) Fatal(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Fatal(msg, fields...)
}

func (l *ChannelLogger) Debugf(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Sugar().Debugf(format, args...)
}

func (l *ChannelLogger) Infof(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Sugar().Infof(format, args...)
}

func (l *ChannelLogger) Debugw(msg string, keysAndValues ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Sugar().Debugw(msg, keysAndValues...)
}

func (l *ChannelLogger) Infow(msg string, keysAndValues ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Sugar().Infow(msg, keysAndValues...)
}

func (l *ChannelLogger) Warnw(msg string, keysAndValues ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Sugar().Warnw(msg, keysAndValues...)
}

func (l *ChannelLogger) Errorw(msg string, keysAndValues ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Sugar().Errorw(msg, keysAndValues...)
}

func (l *ChannelLogger) Warnf(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Sugar().Warnf(format, args...)
}

func (l *ChannelLogger) Errorf(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Sugar().Errorf(format, args...)
}

func (l *ChannelLogger) DPanicf(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Sugar().DPanicf(format, args...)
}

func (l *ChannelLogger) Panicf(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Sugar().Panicf(format, args...)
}

func (l *ChannelLogger) Fatalf(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Sugar().Fatalf(format, args...)
}

func (l *ChannelLogger) DebugCtx(ctx context.Context, msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Debug(msg, ctxFields(ctx, state, fields)...)
}

func (l *ChannelLogger) InfoCtx(ctx context.Context, msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Info(msg, ctxFields(ctx, state, fields)...)
}

func (l *ChannelLogger) WarnCtx(ctx context.Context, msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Warn(msg, ctxFields(ctx, state, fields)...)
}

func (l *ChannelLogger) ErrorCtx(ctx context.Context, msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Error(msg, ctxFields(ctx, state, fields)...)
}

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

func (l *ChannelLogger) HInfo(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Info(msg, fields...)
	if state.messager != nil {
		state.messager.Send(formatFieldsMsg(msg, withChannelField(l.channel, fields)))
	}
}

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

func (l *ChannelLogger) HInfoTo(url, msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Info(msg, fields...)
	if state.messager != nil {
		state.messager.SendTo(url, formatFieldsMsg(msg, withChannelField(l.channel, fields)))
	}
}

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

func (l *ChannelLogger) HError(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Error(msg, fields...)
	if state.messager != nil {
		state.messager.Send(formatFieldsMsg(msg, withChannelField(l.channel, fields)))
	}
}

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

func (l *ChannelLogger) HErrorTo(url, msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	l.derive(state).Error(msg, fields...)
	if state.messager != nil {
		state.messager.SendTo(url, formatFieldsMsg(msg, withChannelField(l.channel, fields)))
	}
}

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
