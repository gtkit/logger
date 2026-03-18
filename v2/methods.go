package logger

import "go.uber.org/zap"

// ============================================================
// 访问底层
// ============================================================

// Zap 返回底层 *zap.Logger.
func (l *Logger) Zap() *zap.Logger {
	return l.zap
}

// Sugar 返回底层 *zap.SugaredLogger.
func (l *Logger) Sugar() *zap.SugaredLogger {
	return l.sugar
}

// With 返回带有预设字段的新 Logger.
// 新 Logger 共享底层 closers 和 messager，但拥有独立的 zap/sugar 实例.
//
//	reqLog := log.With(zap.String("request_id", rid))
//	reqLog.Info("processing", zap.Int("status", 200))
func (l *Logger) With(fields ...zap.Field) *Logger {
	newZap := l.zap.With(fields...)

	return &Logger{
		zap:      newZap,
		sugar:    newZap.Sugar(),
		undo:     l.undo,
		messager: l.messager,
		closers:  l.closers,
	}
}

// Named 返回带有子 logger 名称的新 Logger.
func (l *Logger) Named(name string) *Logger {
	newZap := l.zap.Named(name)

	return &Logger{
		zap:      newZap,
		sugar:    newZap.Sugar(),
		undo:     l.undo,
		messager: l.messager,
		closers:  l.closers,
	}
}

// ============================================================
// 生命周期
// ============================================================

// Undo 恢复 zap 全局 logger 到替换前的状态.
func (l *Logger) Undo() {
	if l.undo != nil {
		l.undo()
	}
}

// Sync 刷新缓冲区并关闭文件资源.
// 应在程序退出前调用: defer log.Sync()
func (l *Logger) Sync() {
	_ = l.zap.Sync()
	for _, c := range l.closers {
		_ = c.Close()
	}
}

// ============================================================
// Structured 日志方法（zap.Field 风格，主推 API）
// 签名与 zap.Logger 一致: func(msg string, fields ...zap.Field)
// ============================================================

// Debug 记录 debug 级别日志.
func (l *Logger) Debug(msg string, fields ...zap.Field) {
	l.zap.Debug(msg, fields...)
}

// Info 记录 info 级别日志.
func (l *Logger) Info(msg string, fields ...zap.Field) {
	l.zap.Info(msg, fields...)
}

// Warn 记录 warn 级别日志.
func (l *Logger) Warn(msg string, fields ...zap.Field) {
	l.zap.Warn(msg, fields...)
}

// Error 记录 error 级别日志.
func (l *Logger) Error(msg string, fields ...zap.Field) {
	l.zap.Error(msg, fields...)
}

// DPanic 记录 dpanic 级别日志.
func (l *Logger) DPanic(msg string, fields ...zap.Field) {
	l.zap.DPanic(msg, fields...)
}

// Panic 记录 panic 级别日志，随后 panic.
func (l *Logger) Panic(msg string, fields ...zap.Field) {
	l.zap.Panic(msg, fields...)
}

// Fatal 记录 fatal 级别日志，随后调用 os.Exit(1).
func (l *Logger) Fatal(msg string, fields ...zap.Field) {
	l.zap.Fatal(msg, fields...)
}

// ============================================================
// Sugar 日志方法（格式化风格，便捷 API）
// ============================================================

// Debugf 记录 debug 级别格式化日志.
func (l *Logger) Debugf(format string, args ...any) {
	l.sugar.Debugf(format, args...)
}

// Infof 记录 info 级别格式化日志.
func (l *Logger) Infof(format string, args ...any) {
	l.sugar.Infof(format, args...)
}

// Infow 记录 info 级别带 key-value 的日志.
func (l *Logger) Infow(msg string, keysAndValues ...any) {
	l.sugar.Infow(msg, keysAndValues...)
}

// Warnf 记录 warn 级别格式化日志.
func (l *Logger) Warnf(format string, args ...any) {
	l.sugar.Warnf(format, args...)
}

// Errorf 记录 error 级别格式化日志.
func (l *Logger) Errorf(format string, args ...any) {
	l.sugar.Errorf(format, args...)
}

// DPanicf 记录 dpanic 级别格式化日志.
func (l *Logger) DPanicf(format string, args ...any) {
	l.sugar.DPanicf(format, args...)
}

// Panicf 记录 panic 级别格式化日志，随后 panic.
func (l *Logger) Panicf(format string, args ...any) {
	l.sugar.Panicf(format, args...)
}

// Fatalf 记录 fatal 级别格式化日志，随后调用 os.Exit(1).
func (l *Logger) Fatalf(format string, args ...any) {
	l.sugar.Fatalf(format, args...)
}

// ============================================================
// 便捷方法
// ============================================================

// LogIf 当 err != nil 时记录 error 级别日志.
func (l *Logger) LogIf(err error) {
	if err != nil {
		l.zap.Error("error occurred", zap.Error(err))
	}
}

// ============================================================
// Hook 消息方法（日志 + 外部推送）
// ============================================================

// HInfo 记录 info 日志，同时通过 Messager 推送消息.
func (l *Logger) HInfo(msg string, fields ...zap.Field) {
	l.zap.Info(msg, fields...)
	if l.messager != nil {
		l.messager.Send(msg)
	}
}

// HInfof 记录 info 格式化日志，同时推送消息.
func (l *Logger) HInfof(format string, args ...any) {
	l.sugar.Infof(format, args...)
	if l.messager != nil {
		l.messager.Send(formatMsg(format, args))
	}
}

// HInfoTo 记录 info 日志，同时推送消息到指定 URL.
func (l *Logger) HInfoTo(url, msg string, fields ...zap.Field) {
	l.zap.Info(msg, fields...)
	if l.messager != nil {
		l.messager.SendTo(url, msg)
	}
}

// HInfoTof 记录 info 格式化日志，同时推送消息到指定 URL.
func (l *Logger) HInfoTof(url, format string, args ...any) {
	l.sugar.Infof(format, args...)
	if l.messager != nil {
		l.messager.SendTo(url, formatMsg(format, args))
	}
}

// HError 记录 error 日志，同时推送消息.
func (l *Logger) HError(msg string, fields ...zap.Field) {
	l.zap.Error(msg, fields...)
	if l.messager != nil {
		l.messager.Send(msg)
	}
}

// HErrorf 记录 error 格式化日志，同时推送消息.
func (l *Logger) HErrorf(format string, args ...any) {
	l.sugar.Errorf(format, args...)
	if l.messager != nil {
		l.messager.Send(formatMsg(format, args))
	}
}

// HErrorTo 记录 error 日志，同时推送消息到指定 URL.
func (l *Logger) HErrorTo(url, msg string, fields ...zap.Field) {
	l.zap.Error(msg, fields...)
	if l.messager != nil {
		l.messager.SendTo(url, msg)
	}
}

// HErrorTof 记录 error 格式化日志，同时推送消息到指定 URL.
func (l *Logger) HErrorTof(url, format string, args ...any) {
	l.sugar.Errorf(format, args...)
	if l.messager != nil {
		l.messager.SendTo(url, formatMsg(format, args))
	}
}
