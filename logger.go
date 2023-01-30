// @Author xiaozhaofu 2022/11/23 00:48:00
package logger

import (
	"os"
	"time"

	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type Option struct {
	Level         string // 日志级别
	ConsoleStdout bool   // 日志是否输出到控制台
	FileStdout    bool   // 日志是否输出到文件
	Division      string // 日志切割方式, time:日期, size:大小, 默认按照大小分割
	Path          string // 日志文件路径
	SqlLog        bool   // 是否打印 sql 执行日志
}

var (
	zlog       *zap.Logger
	zlogoption *Option
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

// 实例化zap
func NewZap(option *Option, opts ...Options) {
	zlogoption = option
	if len(option.Path) == 0 {
		zlogoption.Path = "./logs/"
	}
	initzap()
}

// 函数选项模式实例化zap
func NewZapWithOptions(opts ...Options) {
	var op options
	for _, o := range opts {
		o.apply(&op)
	}

	zlogoption = &Option{
		Level:         op.level,
		ConsoleStdout: op.consolestdout,
		FileStdout:    op.filestdout,
		Division:      op.division,
		Path:          op.path,
		SqlLog:        op.sqllog,
	}
	if len(op.path) == 0 {
		zlogoption.Path = "./logs/"
	}

	// fmt.Printf("zlogoption:%+v\n", zlogoption)
	initzap()
}

func initzap() {
	var syncWriters []zapcore.WriteSyncer
	level := getLoggerLevel(zlogoption.Level)

	writeSyncer := getFileConfig() // 获取日志写入的路径
	encoder := getEncoder()        // 编码配置

	if zlogoption.ConsoleStdout {
		syncWriters = append(syncWriters, zapcore.AddSync(os.Stdout)) // 打印到控制台
	}
	/**
		原生打印到文件
		file, _ := os.Create("./test.log")
	    ori_writeSyncer := zapcore.AddSync(file)
	*/
	if zlogoption.FileStdout {
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
	defer logger.Sync()

	zap.ReplaceGlobals(logger) // ReplaceGlobals来将全局的 logger 替换为我们通过配置定制的 logger
	zlog = logger

}
func Zlog() *zap.Logger {
	return zlog
}

func getLoggerLevel(lvl string) zapcore.Level {
	if level, ok := levelMap[lvl]; ok {
		return level
	}
	return zapcore.InfoLevel
}

func getFileConfig() zapcore.WriteSyncer {
	var filehook zapcore.WriteSyncer

	switch zlogoption.Division {
	case SizeDivision:
		filehook = getFileSizeConfig()
	case TimeDivision:
		filehook = getFileTimeConfig()
	default:
		filehook = getFileSizeConfig()
	}
	return filehook
}

func getFileSizeConfig() zapcore.WriteSyncer {
	logname := time.Now().Format("2006-01-02.log")
	lumberJackLogger := &lumberjack.Logger{
		Filename:   zlogoption.Path + logname, // 日志文件路径
		MaxSize:    128,                       // 日志文件大小,单个文件最大尺寸，默认单位 M
		MaxAge:     30,                        // 最长保存天数
		MaxBackups: 300,                       // 最多备份几个
		Compress:   true,                      // 是否压缩文件，使用gzip
		LocalTime:  true,                      // 使用本地时间
	}
	return zapcore.AddSync(lumberJackLogger)
}

func getFileTimeConfig() zapcore.WriteSyncer {
	// 生成rotatelogs的Logger 实际生成的文件名 demo.log.YYmmddHH
	// demo.log是指向最新日志的链接
	// 保存7天内的日志，每1小时(整点)分割一次日志
	hook, err := rotatelogs.New(
		zlogoption.Path+"%Y%m%d.log", // 没有使用go风格反人类的format格式
		rotatelogs.WithLinkName(zlogoption.Path+"log.log"),
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
