// @Author xiaozhaofu 2022/11/23 00:48:00
package logger

import (
	"github.com/gtkit/news"
	"go.uber.org/zap"
)

func Sql() bool {
	return zlogoption.SqlLog
}
func Zlog() *zap.Logger {
	return zlog
}
func NewLog() *zap.Logger {
	return zlog
}
func NewSugar() *zap.SugaredLogger {
	return zlog.Sugar()
}
func Sugar() *zap.SugaredLogger {
	return zlog.Sugar()
}

func Debug(args ...interface{}) {
	zlog.Sugar().Debug(args...)
}

func Debugf(format string, args ...interface{}) {
	zlog.Sugar().Debugf(format, args...)
}

func Info(args ...interface{}) {
	zlog.Sugar().Info(args...)
}

func ZInfo(msg string, fields ...zap.Field) {
	zlog.Info(msg, fields...)
}

// LogInfoIf 当 err != nil 时记录 info 等级的日志
func LogInfoIf(err error) {
	if err != nil {
		zlog.Info("Error Occurred:", zap.Error(err))
	}
}

func FsWarn(err error, fsurl string) {
	if err != nil {
		zlog.Info("Error Occurred:", zap.Error(err))
		news.FsWarnText(fsurl, err.Error())
	}
}

func Infof(format string, args ...interface{}) {
	zlog.Sugar().Infof(format, args...)
}
func Warn(args ...interface{}) {
	zlog.Sugar().Warn(args...)
}
func ZWarn(moduleName string, fields ...zap.Field) {
	zlog.Warn(moduleName, fields...)
}
func Warnf(format string, args ...interface{}) {
	zlog.Sugar().Warnf(format, args...)
}

func Error(args ...interface{}) {
	zlog.Sugar().Error(args...)
}

// Error
func ZError(moduleName string, fields ...zap.Field) {
	zlog.Error(moduleName, fields...)
}

func Errorf(format string, args ...interface{}) {
	zlog.Sugar().Errorf(format, args...)
}

func DPanic(args ...interface{}) {
	zlog.Sugar().DPanic(args...)
}

func DPanicf(format string, args ...interface{}) {
	zlog.Sugar().DPanicf(format, args...)
}

func Panic(args ...interface{}) {
	zlog.Sugar().Panic(args...)
}

func Panicf(format string, args ...interface{}) {
	zlog.Sugar().Panicf(format, args...)
}

func Fatal(args ...interface{}) {
	zlog.Sugar().Fatal(args...)
}

// Fatal 级别同 Error(), 写完 log 后调用 os.Exit(1) 退出程序
func ZFatal(moduleName string, fields ...zap.Field) {
	zlog.Fatal(moduleName, fields...)
}

func Fatalf(format string, args ...interface{}) {
	zlog.Sugar().Fatalf(format, args...)
}

func Infow(msg string, keysAndValues ...interface{}) {
	zlog.Sugar().Infow(msg, keysAndValues...)
}

// LogIf 当 err != nil 时记录 error 等级的日志
func LogIf(err error) {
	if err != nil {
		zlog.Error("Error Occurred:", zap.Error(err))
	}
}
