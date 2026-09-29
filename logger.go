package logger

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// Zap 返回底层 *zap.Logger 供调用方直接使用或交给第三方库。
// 返回的 logger 已抵消内部包装层的 caller skip，直接调用时 caller 指向真实调用点。
func Zap() *zap.Logger {
	if state := snapshotLoggerState(); state != nil {
		return state.root.WithOptions(zap.AddCallerSkip(-1))
	}

	return zap.NewNop()
}

// Zlog 是 Zap 的别名。
func Zlog() *zap.Logger {
	return Zap()
}

// Sugar 返回底层 *zap.SugaredLogger 供调用方直接使用。
// 返回的 logger 已抵消内部包装层的 caller skip，直接调用时 caller 指向真实调用点。
func Sugar() *zap.SugaredLogger {
	if state := snapshotLoggerState(); state != nil {
		return state.root.WithOptions(zap.AddCallerSkip(-1)).Sugar()
	}

	return zap.NewNop().Sugar()
}

// DroppedMessages 返回异步 Messager 因队列满或 Sync 排空超时而丢弃的推送消息数量。
// 如果未配置 Messager，始终返回 0。
func DroppedMessages() int64 {
	if state := snapshotLoggerState(); state != nil && state.asyncMsg != nil {
		return state.asyncMsg.dropped.Load()
	}
	return 0
}

// SetLevel 运行时动态调整日志级别，影响所有 logger（包括 channel）。
// 支持: debug, info, warn, error, dpanic, panic, fatal；未知级别返回错误且当前级别不变。
func SetLevel(level string) error {
	l, ok := levelMap[level]
	if !ok {
		return fmt.Errorf("logger: invalid level %q", level)
	}
	if state := snapshotLoggerState(); state != nil {
		state.atomicLevel.SetLevel(l)
	}
	return nil
}

// GetLevel 返回当前日志级别字符串。
func GetLevel() string {
	if state := snapshotLoggerState(); state != nil {
		return state.atomicLevel.Level().String()
	}
	return "info"
}

// Debug 以 Debug 级别记录结构化字段日志。
func Debug(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.Debug(msg, fields...)
}

// Info 以 Info 级别记录结构化字段日志。
func Info(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.Info(msg, fields...)
}

// ZInfo 是 Info 的别名。
func ZInfo(msg string, fields ...zap.Field) {
	Info(msg, fields...)
}

// Warn 以 Warn 级别记录结构化字段日志。
func Warn(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.Warn(msg, fields...)
}

// ZWarn 是 Warn 的别名。
func ZWarn(msg string, fields ...zap.Field) {
	Warn(msg, fields...)
}

// Error 以 Error 级别记录结构化字段日志。
func Error(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.Error(msg, fields...)
}

// ZError 是 Error 的别名。
func ZError(msg string, fields ...zap.Field) {
	Error(msg, fields...)
}

// DPanic 以 DPanic 级别记录结构化字段日志；development 模式下会 panic。
func DPanic(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.DPanic(msg, fields...)
}

// Panic 以 Panic 级别记录结构化字段日志，随后 panic。
func Panic(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.Panic(msg, fields...)
}

// Fatal 以 Fatal 级别记录结构化字段日志，随后调用 os.Exit(1)。
func Fatal(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.Fatal(msg, fields...)
}

// Debugf 以 Debug 级别记录 fmt 风格格式化日志。
func Debugf(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Debugf(format, args...)
}

// Infof 以 Info 级别记录 fmt 风格格式化日志。
func Infof(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Infof(format, args...)
}

// Debugw 以 Debug 级别记录 Sugar 风格 key-value 日志。
func Debugw(msg string, keysAndValues ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Debugw(msg, keysAndValues...)
}

// Infow 以 Info 级别记录 Sugar 风格 key-value 日志。
func Infow(msg string, keysAndValues ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Infow(msg, keysAndValues...)
}

// Warnw 以 Warn 级别记录 Sugar 风格 key-value 日志。
func Warnw(msg string, keysAndValues ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Warnw(msg, keysAndValues...)
}

// Errorw 以 Error 级别记录 Sugar 风格 key-value 日志。
func Errorw(msg string, keysAndValues ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Errorw(msg, keysAndValues...)
}

// Warnf 以 Warn 级别记录 fmt 风格格式化日志。
func Warnf(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Warnf(format, args...)
}

// Errorf 以 Error 级别记录 fmt 风格格式化日志。
func Errorf(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Errorf(format, args...)
}

// DPanicf 以 DPanic 级别记录 fmt 风格格式化日志；development 模式下会 panic。
func DPanicf(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.DPanicf(format, args...)
}

// Panicf 以 Panic 级别记录 fmt 风格格式化日志，随后 panic。
func Panicf(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Panicf(format, args...)
}

// Fatalf 以 Fatal 级别记录 fmt 风格格式化日志，随后调用 os.Exit(1)。
func Fatalf(format string, args ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Fatalf(format, args...)
}

// DebugCtx 以 Debug 级别记录结构化字段日志，并自动合并 ContextFieldsFunc 从 ctx 提取的字段。
func DebugCtx(ctx context.Context, msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.Debug(msg, ctxFields(ctx, state, fields)...)
}

// InfoCtx 以 Info 级别记录结构化字段日志，并自动合并 ctx 字段。
func InfoCtx(ctx context.Context, msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.Info(msg, ctxFields(ctx, state, fields)...)
}

// WarnCtx 以 Warn 级别记录结构化字段日志，并自动合并 ctx 字段。
func WarnCtx(ctx context.Context, msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.Warn(msg, ctxFields(ctx, state, fields)...)
}

// ErrorCtx 以 Error 级别记录结构化字段日志，并自动合并 ctx 字段。
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

// ctxKeysAndValues 把 contextFields 提取的 zap.Field 前置到 Sugar 风格的 keysAndValues。
// Sugar 的 Infow/Warnw 等方法接受 ...any，其中 zap.Field 类型会被原样识别为结构化字段，
// 因此这里以 zap.Field 形式注入，不展开为 k/v 对。
func ctxKeysAndValues(ctx context.Context, state *loggerState, kv []any) []any {
	if state.contextFields == nil {
		return kv
	}
	extracted := state.contextFields(ctx)
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
//	logger.DebugwCtx(ctx, "cache miss", "key", "user:42", "tier", "L2")
func DebugwCtx(ctx context.Context, msg string, keysAndValues ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Debugw(msg, ctxKeysAndValues(ctx, state, keysAndValues)...)
}

// InfowCtx 以 Info 级别记录 Sugar 风格 key-value 日志，并自动合并 ctx 字段。
// 行为参见 DebugwCtx。
func InfowCtx(ctx context.Context, msg string, keysAndValues ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Infow(msg, ctxKeysAndValues(ctx, state, keysAndValues)...)
}

// WarnwCtx 以 Warn 级别记录 Sugar 风格 key-value 日志，并自动合并 ctx 字段。
// 行为参见 DebugwCtx。
func WarnwCtx(ctx context.Context, msg string, keysAndValues ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Warnw(msg, ctxKeysAndValues(ctx, state, keysAndValues)...)
}

// ErrorwCtx 以 Error 级别记录 Sugar 风格 key-value 日志，并自动合并 ctx 字段。
// 行为参见 DebugwCtx。
func ErrorwCtx(ctx context.Context, msg string, keysAndValues ...any) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.sugar.Errorw(msg, ctxKeysAndValues(ctx, state, keysAndValues)...)
}

// LogIf 在 err != nil 时以 Error 级别记录一条日志；err 为 nil 时什么都不做。
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

// WarnIf 在 err != nil 时以 Warn 级别记录一条日志。语义与 LogIf 一致，仅级别不同。
func WarnIf(err error) {
	if err != nil {
		state := currentLoggerState()
		if state == nil {
			return
		}
		defer state.release()
		state.root.Warn("warning occurred", zap.Error(err))
	}
}

// LogIfCtx 在 err != nil 时以 Error 级别记录日志，并合并 ctx 注入的字段。
func LogIfCtx(ctx context.Context, err error) {
	if err != nil {
		state := currentLoggerState()
		if state == nil {
			return
		}
		defer state.release()
		state.root.Error("error occurred", ctxFields(ctx, state, []zap.Field{zap.Error(err)})...)
	}
}

// WarnIfCtx 在 err != nil 时以 Warn 级别记录日志，并合并 ctx 注入的字段。
func WarnIfCtx(ctx context.Context, err error) {
	if err != nil {
		state := currentLoggerState()
		if state == nil {
			return
		}
		defer state.release()
		state.root.Warn("warning occurred", ctxFields(ctx, state, []zap.Field{zap.Error(err)})...)
	}
}

// HInfo 以 Info 级别写日志，并通过 Messager 异步推送消息（未配置 Messager 时仅写日志）。
func HInfo(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.Info(msg, fields...)
	if state.messager != nil {
		state.messager.Send(formatHookFieldsMsg(msg, fields, state.fieldRedactor))
	}
}

// HInfof 以 Info 级别写 fmt 风格日志，并通过 Messager 异步推送消息。
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

// HInfoTo 以 Info 级别写日志，并通过 Messager 异步推送消息到指定 URL。
func HInfoTo(url, msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.Info(msg, fields...)
	if state.messager != nil {
		state.messager.SendTo(url, formatHookFieldsMsg(msg, fields, state.fieldRedactor))
	}
}

// HInfoTof 以 Info 级别写 fmt 风格日志，并通过 Messager 异步推送消息到指定 URL。
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

// HError 以 Error 级别写日志，并通过 Messager 异步推送消息（未配置 Messager 时仅写日志）。
func HError(msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.Error(msg, fields...)
	if state.messager != nil {
		state.messager.Send(formatHookFieldsMsg(msg, fields, state.fieldRedactor))
	}
}

// HErrorf 以 Error 级别写 fmt 风格日志，并通过 Messager 异步推送消息。
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

// HErrorTo 以 Error 级别写日志，并通过 Messager 异步推送消息到指定 URL。
func HErrorTo(url, msg string, fields ...zap.Field) {
	state := currentLoggerState()
	if state == nil {
		return
	}
	defer state.release()
	state.root.Error(msg, fields...)
	if state.messager != nil {
		state.messager.SendTo(url, formatHookFieldsMsg(msg, fields, state.fieldRedactor))
	}
}

// HErrorTof 以 Error 级别写 fmt 风格日志，并通过 Messager 异步推送消息到指定 URL。
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
