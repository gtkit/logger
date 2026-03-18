package logger

import "go.uber.org/zap"

// ============================================================
// 访问底层
// ============================================================

// Zap 返回底层 *zap.Logger.
func Zap() *zap.Logger {
	return zaplog
}

func Zlog() *zap.Logger {
	return zaplog
}

// Sugar 返回底层 *zap.SugaredLogger.
func Sugar() *zap.SugaredLogger {
	return sugar
}

// ============================================================
// Structured 日志方法（zap.Field 风格，主推 API）
// 签名与 zap.Logger 一致: func(msg string, fields ...zap.Field)
// ============================================================

// Debug 记录 debug 级别日志.
func Debug(msg string, fields ...zap.Field) {
	zaplog.Debug(msg, fields...)
}

// Info 记录 info 级别日志.
func Info(msg string, fields ...zap.Field) {
	zaplog.Info(msg, fields...)
}

func ZInfo(msg string, fields ...zap.Field) {
	zaplog.Info(msg, fields...)
}

// Warn 记录 warn 级别日志.
func Warn(msg string, fields ...zap.Field) {
	zaplog.Warn(msg, fields...)
}

func ZWarn(msg string, fields ...zap.Field) {
	zaplog.Warn(msg, fields...)
}

// Error 记录 error 级别日志.
func Error(msg string, fields ...zap.Field) {
	zaplog.Error(msg, fields...)
}

func ZError(msg string, fields ...zap.Field) {
	zaplog.Error(msg, fields...)
}

// DPanic 记录 dpanic 级别日志.
func DPanic(msg string, fields ...zap.Field) {
	zaplog.DPanic(msg, fields...)
}

// Panic 记录 panic 级别日志，随后 panic.
func Panic(msg string, fields ...zap.Field) {
	zaplog.Panic(msg, fields...)
}

// Fatal 记录 fatal 级别日志，随后调用 os.Exit(1).
func Fatal(msg string, fields ...zap.Field) {
	zaplog.Fatal(msg, fields...)
}

// ============================================================
// Sugar 日志方法（格式化风格，便捷 API）
// ============================================================

// Debugf 记录 debug 级别格式化日志.
func Debugf(format string, args ...any) {
	sugar.Debugf(format, args...)
}

// Infof 记录 info 级别格式化日志.
func Infof(format string, args ...any) {
	sugar.Infof(format, args...)
}

// Infow 记录 info 级别带 key-value 的日志.
func Infow(msg string, keysAndValues ...any) {
	sugar.Infow(msg, keysAndValues...)
}

// Warnf 记录 warn 级别格式化日志.
func Warnf(format string, args ...any) {
	sugar.Warnf(format, args...)
}

// Errorf 记录 error 级别格式化日志.
func Errorf(format string, args ...any) {
	sugar.Errorf(format, args...)
}

// DPanicf 记录 dpanic 级别格式化日志.
func DPanicf(format string, args ...any) {
	sugar.DPanicf(format, args...)
}

// Panicf 记录 panic 级别格式化日志，随后 panic.
func Panicf(format string, args ...any) {
	sugar.Panicf(format, args...)
}

// Fatalf 记录 fatal 级别格式化日志，随后调用 os.Exit(1).
func Fatalf(format string, args ...any) {
	sugar.Fatalf(format, args...)
}

// ============================================================
// 便捷方法
// ============================================================

// LogIf 当 err != nil 时记录 error 级别日志.
func LogIf(err error) {
	if err != nil {
		zaplog.Error("error occurred", zap.Error(err))
	}
}

// ============================================================
// Hook 消息方法（日志 + 外部推送）
// ============================================================

// HInfo 记录 info 日志，同时通过 Messager 推送消息.
func HInfo(msg string, fields ...zap.Field) {
	zaplog.Info(msg, fields...)
	if msgr != nil {
		msgr.Send(msg)
	}
}

// HInfof 记录 info 格式化日志，同时推送消息.
func HInfof(format string, args ...any) {
	sugar.Infof(format, args...)
	if msgr != nil {
		msgr.Send(formatMsg(format, args))
	}
}

// HInfoTo 记录 info 日志，同时推送消息到指定 URL.
func HInfoTo(url, msg string, fields ...zap.Field) {
	zaplog.Info(msg, fields...)
	if msgr != nil {
		msgr.SendTo(url, msg)
	}
}

// HInfoTof 记录 info 格式化日志，同时推送消息到指定 URL.
func HInfoTof(url, format string, args ...any) {
	sugar.Infof(format, args...)
	if msgr != nil {
		msgr.SendTo(url, formatMsg(format, args))
	}
}

// HError 记录 error 日志，同时推送消息.
func HError(msg string, fields ...zap.Field) {
	zaplog.Error(msg, fields...)
	if msgr != nil {
		msgr.Send(msg)
	}
}

// HErrorf 记录 error 格式化日志，同时推送消息.
func HErrorf(format string, args ...any) {
	sugar.Errorf(format, args...)
	if msgr != nil {
		msgr.Send(formatMsg(format, args))
	}
}

// HErrorTo 记录 error 日志，同时推送消息到指定 URL.
func HErrorTo(url, msg string, fields ...zap.Field) {
	zaplog.Error(msg, fields...)
	if msgr != nil {
		msgr.SendTo(url, msg)
	}
}

// HErrorTof 记录 error 格式化日志，同时推送消息到指定 URL.
func HErrorTof(url, format string, args ...any) {
	sugar.Errorf(format, args...)
	if msgr != nil {
		msgr.SendTo(url, formatMsg(format, args))
	}
}
