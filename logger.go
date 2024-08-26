// @Author xiaozhaofu 2022/11/23 00:48:00
package logger

import (
	"go.uber.org/zap"
)

// Zlog 获取 zap.Logger.
func Zlog() *zap.Logger {
	return zlog
}

// NewLog 获取 zap.Logger.
func ZapLog() *zap.Logger {
	return zlog
}

// Sugar 获取 zap.SugaredLogger.
func Sugar() *zap.SugaredLogger {
	return zlog.Sugar()
}

// Debug 记录 debug 等级的日志.
func Debug(args ...any) {
	zlog.Sugar().Debug(args...)
}

// Debugf 记录 debug 等级的日志.
func Debugf(format string, args ...any) {
	zlog.Sugar().Debugf(format, args...)
}

// Info 记录 info 等级的日志.
func Info(args ...any) {
	zlog.Sugar().Info(args...)
}

// ZInfo 记录 info 等级的日志.
func ZInfo(msg string, fields ...zap.Field) {
	zlog.Info(msg, fields...)
}

// Infof 记录 info 等级的日志.
func Infof(format string, args ...any) {
	zlog.Sugar().Infof(format, args...)
}

// Warn 记录 warn 等级的日志.
func Warn(args ...any) {
	zlog.Sugar().Warn(args...)
}

// ZWarn 记录 warn 等级的日志.
func ZWarn(moduleName string, fields ...zap.Field) {
	zlog.Warn(moduleName, fields...)
}

// Warnf 记录 warn 等级的日志.
func Warnf(format string, args ...any) {
	zlog.Sugar().Warnf(format, args...)
}

// Error 记录 error 等级的日志.
func Error(args ...any) {
	zlog.Sugar().Error(args...)
}

// ZError Error.
func ZError(moduleName string, fields ...zap.Field) {
	zlog.Error(moduleName, fields...)
}

// Errorf 记录 error 等级的日志.
func Errorf(format string, args ...any) {
	zlog.Sugar().Errorf(format, args...)
}

// DPanic 记录 dpanic 等级的日志.
func DPanic(args ...any) {
	zlog.Sugar().DPanic(args...)
}

// DPanicf 记录 dpanic 等级的日志.
func DPanicf(format string, args ...any) {
	zlog.Sugar().DPanicf(format, args...)
}

func Panic(args ...any) {
	zlog.Sugar().Panic(args...)
}

// Panic 级别同 Error(), 写完 log 后调用 os.Exit(1) 退出程序.
func Panicf(format string, args ...any) {
	zlog.Sugar().Panicf(format, args...)
}

// Fatal 级别同 Error(), 写完 log 后调用 os.Exit(1) 退出程序.
func Fatal(args ...any) {
	zlog.Sugar().Fatal(args...)
}

// Fatal 级别同 Error(), 写完 log 后调用 os.Exit(1) 退出程序.
func ZFatal(moduleName string, fields ...zap.Field) {
	zlog.Fatal(moduleName, fields...)
}

func Fatalf(format string, args ...any) {
	zlog.Sugar().Fatalf(format, args...)
}

func Infow(msg string, keysAndValues ...any) {
	zlog.Sugar().Infow(msg, keysAndValues...)
}

// LogIf 当 err != nil 时记录 error 等级的日志.
func LogIf(err error) {
	if err != nil {
		zlog.Error("Error Occurred:", zap.Error(err))
	}
}
func LogErrIf(err error) {
	if err != nil {
		zlog.Error("Error Occurred:", zap.Error(err))
	}
}

// LogInfoIf 当 err != nil 时记录 info 等级的日志.
func LogInfoIf(err error) {
	if err != nil {
		zlog.Info("Error Occurred:", zap.Error(err))
	}
}
