package logger

import (
	"context"
	"os"
	"sync/atomic"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// 进程默认实例与包级转发函数（log/slog 同款形态）：
// 完整的实例 API 之外，提供一个可显式替换的进程默认实例（SetDefault），
// 以及一组转发到默认实例的包级函数，使调用点写成 logger.Info(...)。
//
// 与旧版“库隐式全局”的区别：默认实例只能经 SetDefault 显式设定；
// 未设定时包级函数写入懒创建的纯控制台兜底实例（不落盘、不抢占 zap 全局），
// 多实例场景（如独立的 access 日志实例）不受默认实例影响。

type defaultState struct {
	instance *Logger
	// skipped 是 +1 caller skip 的派生视图：包级函数多一层转发帧，
	// 经它记录才能让 caller 指向真实调用点。
	skipped *Logger
}

var defaultHolder atomic.Pointer[defaultState]

// SetDefault 把 l 设为进程默认实例（包级函数的输出目标）。nil 被忽略。
// 并发安全；启动期设置一次，不支持运行期热替换（全局 owner 契约见 [Logger.Undo]）。
//
// 资源所有权：SetDefault 不会 Sync/Undo 被替换的旧默认实例。确需多实例轮换的
// 高级场景，全部实例必须以 WithReplaceGlobals(false) 构建，并由应用层先完成
// 流量排空（停止新日志、等待在途调用结束）再关闭旧实例。
func SetDefault(l *Logger) {
	if l == nil {
		return
	}
	defaultHolder.Store(&defaultState{instance: l, skipped: l.WithCallerSkip(1)})
}

// Default 返回进程默认实例；未经 SetDefault 设定时，返回懒创建的
// 纯控制台兜底实例：只写 stderr（不落盘——兜底若写文件会以进程 cwd 为锚
// 在任意目录刷出日志；写 stderr 而非 stdout 避免污染 CLI 协议输出）、
// 不替换 zap 全局。
func Default() *Logger {
	return loadDefault().instance
}

// loadDefault 用 CAS 循环解决「兜底懒创建」与并发 SetDefault 的竞争：
// 竞争失败方 Sync 释放自己的候选实例后采用胜者。不依赖一次性状态（sync.Once），
// 任何时刻把 holder 置回 nil（如测试重置）后都能安全重建兜底。
func loadDefault() *defaultState {
	for {
		if s := defaultHolder.Load(); s != nil {
			return s
		}
		fb := MustNew(WithConsole(true), WithFile(false), WithReplaceGlobals(false), withConsoleStderr())
		candidate := &defaultState{instance: fb, skipped: fb.WithCallerSkip(1)}
		if defaultHolder.CompareAndSwap(nil, candidate) {
			return candidate
		}
		// 竞争失败：释放候选实例，下一轮读取胜者。
		fb.Sync()
	}
}

func defaultSkipped() *Logger {
	return loadDefault().skipped
}

// withConsoleStderr 让控制台输出走 stderr（仅库内部供兜底实例使用）：
// 兜底日志属诊断输出，写 stdout 会污染 CLI 的 JSON/管道/协议输出。
func withConsoleStderr() Option {
	return func(c *Config) error {
		c.consoleWriter = zapcore.Lock(os.Stderr)
		return nil
	}
}

// Sync 刷写默认实例（含异步 Messager 队列与文件资源）。进程退出前调用。
func Sync() { Default().Sync() }

// ---------------------------------------------------------------------------
// 结构化字段
// ---------------------------------------------------------------------------

// Debug 以 Debug 级别经默认实例记录结构化字段日志。
func Debug(msg string, fields ...zap.Field) { defaultSkipped().Debug(msg, fields...) }

// Info 以 Info 级别经默认实例记录结构化字段日志。
func Info(msg string, fields ...zap.Field) { defaultSkipped().Info(msg, fields...) }

// Warn 以 Warn 级别经默认实例记录结构化字段日志。
func Warn(msg string, fields ...zap.Field) { defaultSkipped().Warn(msg, fields...) }

// Error 以 Error 级别经默认实例记录结构化字段日志。
func Error(msg string, fields ...zap.Field) { defaultSkipped().Error(msg, fields...) }

// DPanic 以 DPanic 级别经默认实例记录日志（development 模式下 panic）。
func DPanic(msg string, fields ...zap.Field) { defaultSkipped().DPanic(msg, fields...) }

// Panic 经默认实例记录日志后 panic。
func Panic(msg string, fields ...zap.Field) { defaultSkipped().Panic(msg, fields...) }

// Fatal 经默认实例记录日志后 os.Exit(1)。
func Fatal(msg string, fields ...zap.Field) { defaultSkipped().Fatal(msg, fields...) }

// ---------------------------------------------------------------------------
// printf 风格
// ---------------------------------------------------------------------------

// Debugf 以 Debug 级别经默认实例记录格式化日志。
func Debugf(format string, args ...any) { defaultSkipped().Debugf(format, args...) }

// Infof 以 Info 级别经默认实例记录格式化日志。
func Infof(format string, args ...any) { defaultSkipped().Infof(format, args...) }

// Warnf 以 Warn 级别经默认实例记录格式化日志。
func Warnf(format string, args ...any) { defaultSkipped().Warnf(format, args...) }

// Errorf 以 Error 级别经默认实例记录格式化日志。
func Errorf(format string, args ...any) { defaultSkipped().Errorf(format, args...) }

// DPanicf 以 DPanic 级别经默认实例记录格式化日志。
func DPanicf(format string, args ...any) { defaultSkipped().DPanicf(format, args...) }

// Panicf 经默认实例记录格式化日志后 panic。
func Panicf(format string, args ...any) { defaultSkipped().Panicf(format, args...) }

// Fatalf 经默认实例记录格式化日志后 os.Exit(1)。
func Fatalf(format string, args ...any) { defaultSkipped().Fatalf(format, args...) }

// ---------------------------------------------------------------------------
// 键值对风格
// ---------------------------------------------------------------------------

// Debugw 以 Debug 级别经默认实例记录键值对日志。
func Debugw(msg string, keysAndValues ...any) { defaultSkipped().Debugw(msg, keysAndValues...) }

// Infow 以 Info 级别经默认实例记录键值对日志。
func Infow(msg string, keysAndValues ...any) { defaultSkipped().Infow(msg, keysAndValues...) }

// Warnw 以 Warn 级别经默认实例记录键值对日志。
func Warnw(msg string, keysAndValues ...any) { defaultSkipped().Warnw(msg, keysAndValues...) }

// Errorw 以 Error 级别经默认实例记录键值对日志。
func Errorw(msg string, keysAndValues ...any) { defaultSkipped().Errorw(msg, keysAndValues...) }

// ---------------------------------------------------------------------------
// Ctx 变体（合并 WithContextFields 注入的字段）
// ---------------------------------------------------------------------------

// DebugCtx 以 Debug 级别记录日志并合并 ctx 注入字段。
func DebugCtx(ctx context.Context, msg string, fields ...zap.Field) {
	defaultSkipped().DebugCtx(ctx, msg, fields...)
}

// InfoCtx 以 Info 级别记录日志并合并 ctx 注入字段。
func InfoCtx(ctx context.Context, msg string, fields ...zap.Field) {
	defaultSkipped().InfoCtx(ctx, msg, fields...)
}

// WarnCtx 以 Warn 级别记录日志并合并 ctx 注入字段。
func WarnCtx(ctx context.Context, msg string, fields ...zap.Field) {
	defaultSkipped().WarnCtx(ctx, msg, fields...)
}

// ErrorCtx 以 Error 级别记录日志并合并 ctx 注入字段。
func ErrorCtx(ctx context.Context, msg string, fields ...zap.Field) {
	defaultSkipped().ErrorCtx(ctx, msg, fields...)
}

// DebugwCtx 以 Debug 级别记录键值对日志并合并 ctx 注入字段。
func DebugwCtx(ctx context.Context, msg string, keysAndValues ...any) {
	defaultSkipped().DebugwCtx(ctx, msg, keysAndValues...)
}

// InfowCtx 以 Info 级别记录键值对日志并合并 ctx 注入字段。
func InfowCtx(ctx context.Context, msg string, keysAndValues ...any) {
	defaultSkipped().InfowCtx(ctx, msg, keysAndValues...)
}

// WarnwCtx 以 Warn 级别记录键值对日志并合并 ctx 注入字段。
func WarnwCtx(ctx context.Context, msg string, keysAndValues ...any) {
	defaultSkipped().WarnwCtx(ctx, msg, keysAndValues...)
}

// ErrorwCtx 以 Error 级别记录键值对日志并合并 ctx 注入字段。
func ErrorwCtx(ctx context.Context, msg string, keysAndValues ...any) {
	defaultSkipped().ErrorwCtx(ctx, msg, keysAndValues...)
}

// ---------------------------------------------------------------------------
// 条件与推送
// ---------------------------------------------------------------------------

// LogIf 在 err != nil 时以 Error 级别经默认实例记录日志。
func LogIf(err error) { defaultSkipped().LogIf(err) }

// WarnIf 在 err != nil 时以 Warn 级别经默认实例记录日志。
func WarnIf(err error) { defaultSkipped().WarnIf(err) }

// LogIfCtx 在 err != nil 时以 Error 级别记录日志并合并 ctx 注入字段。
func LogIfCtx(ctx context.Context, err error) { defaultSkipped().LogIfCtx(ctx, err) }

// WarnIfCtx 在 err != nil 时以 Warn 级别记录日志并合并 ctx 注入字段。
func WarnIfCtx(ctx context.Context, err error) { defaultSkipped().WarnIfCtx(ctx, err) }

// HInfo 以 Info 级别记录日志并经 Messager 异步推送。
func HInfo(msg string, fields ...zap.Field) { defaultSkipped().HInfo(msg, fields...) }

// HInfof 以 Info 级别记录格式化日志并经 Messager 异步推送。
func HInfof(format string, args ...any) { defaultSkipped().HInfof(format, args...) }

// HError 以 Error 级别记录日志并经 Messager 异步推送。
func HError(msg string, fields ...zap.Field) { defaultSkipped().HError(msg, fields...) }

// HErrorf 以 Error 级别记录格式化日志并经 Messager 异步推送。
func HErrorf(format string, args ...any) { defaultSkipped().HErrorf(format, args...) }

// HInfoTo 以 Info 级别记录日志并经 Messager 异步推送到指定 URL。
func HInfoTo(url, msg string, fields ...zap.Field) { defaultSkipped().HInfoTo(url, msg, fields...) }

// HInfoTof 以 Info 级别记录格式化日志并经 Messager 异步推送到指定 URL。
func HInfoTof(url, format string, args ...any) { defaultSkipped().HInfoTof(url, format, args...) }

// HErrorTo 以 Error 级别记录日志并经 Messager 异步推送到指定 URL。
func HErrorTo(url, msg string, fields ...zap.Field) { defaultSkipped().HErrorTo(url, msg, fields...) }

// HErrorTof 以 Error 级别记录格式化日志并经 Messager 异步推送到指定 URL。
func HErrorTof(url, format string, args ...any) { defaultSkipped().HErrorTof(url, format, args...) }
