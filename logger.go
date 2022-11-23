// @Author xiaozhaofu 2022/11/23 00:48:00
package logger

import (
	"os"
	"time"

	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type Option struct {
	Level         zapcore.Level
	ConsoleStdout bool
	FileStdout    bool
	Division      string // 日志切割方式, time:日期, size:大小, 默认按照大小分割
	Path          string // 日志文件路径
}

var (
	Zlog         *zap.Logger
	LoggerOption *Option
)

const (
	TimeDivision = "time"
	SizeDivision = "size"
)

var levelMap = map[string]zapcore.Level{
	"debug":  zapcore.DebugLevel,
	"info":   zapcore.InfoLevel,
	"warn":   zapcore.WarnLevel,
	"error":  zapcore.ErrorLevel,
	"dpanic": zapcore.DPanicLevel,
	"panic":  zapcore.PanicLevel,
	"fatal":  zapcore.FatalLevel,
}

func Init() {
	var syncWriters []zapcore.WriteSyncer
	level := getLoggerLevel(viper.GetString("log.level"))

	writeSyncer := getFileConfig() // 获取日志写入的路径
	encoder := getEncoder()        // 编码配置

	if LoggerOption.ConsoleStdout {
		syncWriters = append(syncWriters, zapcore.AddSync(os.Stdout)) // 打印到控制台
	}
	/**
		原生打印到文件
		file, _ := os.Create("./test.log")
	    ori_writeSyncer := zapcore.AddSync(file)
	*/
	if LoggerOption.FileStdout {
		syncWriters = append(
			syncWriters,
			zapcore.AddSync(writeSyncer), // 打印到文件
			// ori_writeSyncer,
		)
	}

	// syncWriters... 切片被打散进行传递; WriteSyncer 指定日志写到哪里去
	core := zapcore.NewCore(
		encoder,
		zapcore.NewMultiWriteSyncer(syncWriters...),
		zap.NewAtomicLevelAt(level),
	)

	logger := zap.New(
		core,
		zap.AddCaller(),                   // zap.AddCaller 打印日志的代码所在的位置信息
		zap.AddCallerSkip(1),              // AddCallerSkip 显示调用打印日志的是哪一行的 code 行数
		zap.AddStacktrace(zap.ErrorLevel), // Error 时才会显示 stacktrace
	)

	zap.ReplaceGlobals(logger) // ReplaceGlobals来将全局的 logger 替换为我们通过配置定制的 logger
	Zlog = logger

}

func getLoggerLevel(lvl string) zapcore.Level {
	if level, ok := levelMap[lvl]; ok {
		return level
	}
	return zapcore.InfoLevel
}

func getFileConfig() zapcore.WriteSyncer {
	var filehook zapcore.WriteSyncer

	switch LoggerOption.Division {
	case SizeDivision:
		filehook = getFileSizeConfig(LoggerOption.Path)
	case TimeDivision:
		filehook = getFileTimeConfig(LoggerOption.Path)
	default:
		filehook = getFileSizeConfig(LoggerOption.Path)
	}
	return filehook
}

func getFileSizeConfig(path string) zapcore.WriteSyncer {

	lumberJackLogger := &lumberjack.Logger{
		Filename:   path + "log.log", // 日志文件路径
		MaxSize:    128,              // 日志文件大小,单个文件最大尺寸，默认单位 M
		MaxAge:     30,               // 最长保存天数
		MaxBackups: 300,              // 最多备份几个
		Compress:   true,             // 是否压缩文件，使用gzip
		LocalTime:  true,             // 使用本地时间
	}
	return zapcore.AddSync(lumberJackLogger)
}

func getFileTimeConfig(path string) zapcore.WriteSyncer {
	// 生成rotatelogs的Logger 实际生成的文件名 demo.log.YYmmddHH
	// demo.log是指向最新日志的链接
	// 保存7天内的日志，每1小时(整点)分割一次日志
	hook, err := rotatelogs.New(
		path+"%Y%m%d.log", // 没有使用go风格反人类的format格式
		rotatelogs.WithLinkName(path+"log.log"),
		rotatelogs.WithMaxAge(time.Hour*24*7),
		rotatelogs.WithRotationTime(time.Hour*24),
	)
	if err != nil {
		panic(err)
	}
	return zapcore.AddSync(hook)
}

func getEncoder() zapcore.Encoder {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.TimeKey = "time"
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	encoderConfig.EncodeDuration = zapcore.SecondsDurationEncoder
	encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	// return zapcore.NewJSONEncoder(encoderConfig)
	return zapcore.NewConsoleEncoder(encoderConfig)
}

func Debug(args ...interface{}) {
	Zlog.Sugar().Debug(args...)
}

func Debugf(format string, args ...interface{}) {
	Zlog.Sugar().Debugf(format, args...)
}

func Info(args ...interface{}) {
	Zlog.Sugar().Info(args...)
}

func Infof(format string, args ...interface{}) {
	Zlog.Sugar().Infof(format, args...)
}

func Warn(args ...interface{}) {
	Zlog.Sugar().Warn(args...)
}

func Warnf(format string, args ...interface{}) {
	Zlog.Sugar().Warnf(format, args...)
}

func Error(args ...interface{}) {
	Zlog.Sugar().Error(args...)
}

func Errorf(format string, args ...interface{}) {
	Zlog.Sugar().Errorf(format, args...)
}

func DPanic(args ...interface{}) {
	Zlog.Sugar().DPanic(args...)
}

func DPanicf(format string, args ...interface{}) {
	Zlog.Sugar().DPanicf(format, args...)
}

func Panic(args ...interface{}) {
	Zlog.Sugar().Panic(args...)
}

func Panicf(format string, args ...interface{}) {
	Zlog.Sugar().Panicf(format, args...)
}

func Fatal(args ...interface{}) {
	Zlog.Sugar().Fatal(args...)
}

func Fatalf(format string, args ...interface{}) {
	Zlog.Sugar().Fatalf(format, args...)
}

func Infow(msg string, keysAndValues ...interface{}) {
	Zlog.Sugar().Infow(msg, keysAndValues...)
}