package logger

import (
	"context"

	"go.uber.org/zap"
)

func Zap() *zap.Logger {
	if state := snapshotLoggerState(); state != nil {
		return state.root
	}

	return zap.NewNop()
}

func Zlog() *zap.Logger {
	return Zap()
}

func Sugar() *zap.SugaredLogger {
	if state := snapshotLoggerState(); state != nil {
		return state.sugar
	}

	return zap.NewNop().Sugar()
}

// DroppedMessages 返回异步 Messager 因队列满而丢弃的推送消息数量。
// 如果未配置 Messager，始终返回 0。
func DroppedMessages() int64 {
	if state := snapshotLoggerState(); state != nil && state.asyncMsg != nil {
		return state.asyncMsg.dropped.Load()
	}
	return 0
}

// SetLevel 运行时动态调整日志级别，影响所有 logger（包括 channel）。
// 支持: debug, info, warn, error, dpanic, panic, fatal.
func SetLevel(level string) {
	if l, ok := levelMap[level]; ok {
		if state := snapshotLoggerState(); state != nil {
			state.atomicLevel.SetLevel(l)
		}
	}
}

// GetLevel 返回当前日志级别字符串。
func GetLevel() string {
	if state := snapshotLoggerState(); state != nil {
		return state.atomicLevel.Level().String()
	}
	return "info"
}

func Debug(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.Debug(msg, fields...)
}

func Info(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.Info(msg, fields...)
}

func ZInfo(msg string, fields ...zap.Field) {
	Info(msg, fields...)
}

func Warn(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.Warn(msg, fields...)
}

func ZWarn(msg string, fields ...zap.Field) {
	Warn(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.Error(msg, fields...)
}

func ZError(msg string, fields ...zap.Field) {
	Error(msg, fields...)
}

func DPanic(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.DPanic(msg, fields...)
}

func Panic(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.Panic(msg, fields...)
}

func Fatal(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.Fatal(msg, fields...)
}

func Debugf(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Debugf(format, args...)
}

func Infof(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Infof(format, args...)
}

func Debugw(msg string, keysAndValues ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Debugw(msg, keysAndValues...)
}

func Infow(msg string, keysAndValues ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Infow(msg, keysAndValues...)
}

func Warnw(msg string, keysAndValues ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Warnw(msg, keysAndValues...)
}

func Errorw(msg string, keysAndValues ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Errorw(msg, keysAndValues...)
}

func Warnf(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Warnf(format, args...)
}

func Errorf(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Errorf(format, args...)
}

func DPanicf(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.DPanicf(format, args...)
}

func Panicf(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Panicf(format, args...)
}

func Fatalf(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Fatalf(format, args...)
}

func DebugCtx(ctx context.Context, msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.Debug(msg, ctxFields(ctx, state, fields)...)
}

func InfoCtx(ctx context.Context, msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.Info(msg, ctxFields(ctx, state, fields)...)
}

func WarnCtx(ctx context.Context, msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.Warn(msg, ctxFields(ctx, state, fields)...)
}

func ErrorCtx(ctx context.Context, msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.Error(msg, ctxFields(ctx, state, fields)...)
}

func ctxFields(ctx context.Context, state *loggerState, fields []zap.Field) []zap.Field {
	if state.contextFields == nil {
		return fields
	}
	extracted := state.contextFields(ctx)
	if len(extracted) == 0 {
		return fields
	}
	merged := make([]zap.Field, 0, len(extracted)+len(fields))
	merged = append(merged, extracted...)
	merged = append(merged, fields...)
	return merged
}

func LogIf(err error) {
	if err != nil {
		state := currentLoggerState()
		if state == nil {
			return
		}
		defer state.release()
		state.root.Error("error occurred", zap.Error(err))
	}
}

func HInfo(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.Info(msg, fields...)
	if state.messager != nil {
		state.messager.Send(formatFieldsMsg(msg, fields))
	}
}

func HInfof(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Infof(format, args...)
	if state.messager != nil {
		state.messager.Send(formatMsg(format, args))
	}
}

func HInfoTo(url, msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.Info(msg, fields...)
	if state.messager != nil {
		state.messager.SendTo(url, formatFieldsMsg(msg, fields))
	}
}

func HInfoTof(url, format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Infof(format, args...)
	if state.messager != nil {
		state.messager.SendTo(url, formatMsg(format, args))
	}
}

func HError(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.Error(msg, fields...)
	if state.messager != nil {
		state.messager.Send(formatFieldsMsg(msg, fields))
	}
}

func HErrorf(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Errorf(format, args...)
	if state.messager != nil {
		state.messager.Send(formatMsg(format, args))
	}
}

func HErrorTo(url, msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.Error(msg, fields...)
	if state.messager != nil {
		state.messager.SendTo(url, formatFieldsMsg(msg, fields))
	}
}

func HErrorTof(url, format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Errorf(format, args...)
	if state.messager != nil {
		state.messager.SendTo(url, formatMsg(format, args))
	}
}
