// @Author xiaozhaofu 2023/6/30 20:16:00
package logger

import (
	"os"
	"strings"
	"time"

	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/natefinch/lumberjack"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	DayHour = 24
	WeekDay = 7
	MaxSize = 512

	DefaultPath       = "./logs/" // 默认保存目录
	DefaultMaxSize    = 512       // 默认 512M
	DefaultMaxAge     = 7         // 默认保存七天
	DefaultMaxBackups = 50        // 默认5个备份
	DefaultCompress   = true      // 默认压缩
	DefaultConsole    = false
)

type logConfig struct {
	consoleStdout bool   // 日志是否输出到控制台
	fileStdout    bool   // 日志是否输出到文件
	division      string // 日志切割方式, time:日期, size:大小, 默认按照大小分割
	path          string // 日志文件路径
	compress      bool   // 是否压缩日志文件
	maxAge        int    // 日志文件最大保存时间,单位天
	maxBackups    int    // 日志文件最大备份数
	maxSize       int    // 日志文件最大大小,单位M
}

var (
	zlog   *zap.Logger
	config *logConfig
	undo   func()
)

// NewZap 函数选项模式实例化zap.
func NewZap(opts ...Options) {
	config = &logConfig{
		consoleStdout: DefaultConsole,
		fileStdout:    true,
		division:      "size",
		path:          DefaultPath,
		compress:      DefaultCompress,
		maxAge:        DefaultMaxAge,
		maxBackups:    DefaultMaxBackups,
		maxSize:       DefaultMaxSize,
	}
	for _, o := range opts {
		o.apply(config)
	}
	// fmt.Printf("logConfig:%+v\n", logConfig)
	initzap()
}

func initzap() {
	var (
		syncInfoWriters  []zapcore.WriteSyncer
		syncErrorWriters []zapcore.WriteSyncer
	)
	// level := getLoggerLevel(logConfig.Level)

	infoPath := getFileConfig("info")   // 获取日志写入的路径
	errorPath := getFileConfig("error") // 获取错误日志写入的路径
	encoder := getEncoder()             // 编码配置

	// 控制台打印, info, error日志同时输出
	if config.consoleStdout {
		syncInfoWriters = append(syncInfoWriters, zapcore.AddSync(os.Stdout))
		syncErrorWriters = append(syncErrorWriters, zapcore.AddSync(os.Stdout))
	}
	/**
		原生打印到文件
		file, _ := os.Create("./test.log")
	    ori_writeSyncer := zapcore.AddSync(file)
	*/
	if config.fileStdout {
		syncInfoWriters = append(
			syncInfoWriters,
			zapcore.AddSync(infoPath), // 打印到 info 文件
			// ori_writeSyncer,
		)
		syncErrorWriters = append(
			syncErrorWriters,
			zapcore.AddSync(errorPath), // 打印到 error 文件
		)
	}

	// 日志级别为 Info
	infoCore := zapcore.NewCore(
		encoder,
		zapcore.NewMultiWriteSyncer(syncInfoWriters...),
		zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
			return lvl < zapcore.WarnLevel
		}),
	)
	// 错误日志级别为 Error
	// warnlevel及以上归到warn日志
	errorCore := zapcore.NewCore(
		encoder,
		zapcore.NewMultiWriteSyncer(syncErrorWriters...),
		zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
			return lvl >= zapcore.WarnLevel
		}),
	)

	/**
	 *  WrapCore(f func(zapcore.Core) zapcore.Core): 使用一个新的 zapcore.Core 替换掉 Logger 内部原有的的 zapcore.Core 属性。
	 *  Hooks(hooks ...func(zapcore.Entry) error): 注册钩子函数，用来在日志打印时同时调用注册的钩子函数。
	 *  Fields(fs ...Field): 添加公共字段。
	 *  ErrorOutput(w zapcore.WriteSyncer): 指定日志组件内部出现异常时的输出位置。
	 *  Development(): 将日志记录器设为开发模式，这将使 DPanic 级别日志记录错误后执行 panic()。
	 *  AddCaller(): 与 WithCaller(true) 等价。
	 *  WithCaller(enabled bool): 指定是否在日志输出内容中增加调用信息，即文件名和行号。
	 *  AddCallerSkip(skip int): 指定在通过调用栈获取文件名和行号时跳过的调用深度。
	 *  AddStacktrace(lvl zapcore.LevelEnabler): 用来指定某个日志级别及以上级别输出调用堆栈。
	 *  IncreaseLevel(lvl zapcore.LevelEnabler): 提高日志级别，如果传入的 lvl 比现有级别低，则不会改变日志级别。
	 *  WithFatalHook(hook zapcore.CheckWriteHook): 当出现 Fatal 级别日志时调用的钩子函数。
	 *  WithClock(clock zapcore.Clock): 指定日志记录器用来确定当前时间的 zapcore.Clock 对象，默认为 time.Now 的系统时钟。
	 */
	zlog = zap.New(
		zapcore.NewTee(infoCore, errorCore),
		zap.AddCaller(),                   // zap.AddCaller 打印日志的代码所在的位置信息
		zap.AddCallerSkip(1),              // AddCallerSkip 显示调用打印日志的是哪一行的 code 行数
		zap.AddStacktrace(zap.ErrorLevel), // Error 时才会显示 stacktrace
		// zap.Hooks(func(entry zapcore.Entry) error {
		// 	entry.Message = "[" + entry.Level.String() + "] " + entry.Message
		// 	return nil
		// }),

	)

	undo = zap.ReplaceGlobals(zlog) // ReplaceGlobals来将全局的 logger 替换为我们通过配置定制的 logger
}

func Sync() {
	undo()
	if err := zlog.Sync(); err != nil {
		Info("logger sync error: ", err)
	}
}

func UnDo() {
	undo()
}

func getFileConfig(level string) zapcore.WriteSyncer {
	var filehook zapcore.WriteSyncer

	switch config.division {
	case "size":
		filehook = getFileSizeConfig(level)
	case "daily":
		filehook = getFileDailyConfig(level)
	default:
		filehook = getFileSizeConfig(level)
	}
	return filehook
}

func getFileSizeConfig(level string) zapcore.WriteSyncer {
	logname := time.Now().Format("2006-01-02.log")
	var builder strings.Builder
	builder.WriteString(config.path)
	builder.WriteString("/")
	builder.WriteString(level)
	builder.WriteString("_")
	builder.WriteString(logname)
	lumberJackLogger := &lumberjack.Logger{
		Filename:   builder.String(),  // 日志文件路径
		MaxSize:    MaxSize,           // 日志文件大小,单个文件最大尺寸，默认单位 M
		MaxAge:     config.maxAge,     // 最长保存天数
		MaxBackups: config.maxBackups, // 最多备份几个
		Compress:   config.compress,   // 是否压缩文件，使用gzip
		LocalTime:  true,              // 使用本地时间
	}
	return zapcore.AddSync(lumberJackLogger)
}

func getFileDailyConfig(level string) zapcore.WriteSyncer {
	// 生成rotatelogs的Logger 实际生成的文件名 demo.log.YYmmddHH
	// demo.log是指向最新日志的链接
	// 保存7天内的日志，每1小时(整点)分割一次日志
	var builder strings.Builder
	builder.WriteString(config.path)
	builder.WriteString("/")
	builder.WriteString(level)
	builder.WriteString("_%Y-%m-%d.log")
	hook, err := rotatelogs.New(
		builder.String(), // 没有使用go风格反人类的format格式
		// 为最新的日志建立软连接，指向最新日志文件
		rotatelogs.WithLinkName(config.path+level+".log"),
		// 清理条件： 将已切割的日志文件按条件(数量or时间)直接删除
		// --- MaxAge and RotationCount cannot be both set  两者不能同时设置
		// --- RotationCount用来设置最多切割的文件数(超过的会被 从旧到新 清理)
		// --- MaxAge 是设置文件清理前的最长保存时间 最小分钟为单位
		// --- if both are 0, give maxAge a default 7 * 24 * time.Hour
		// WithRotationCount和WithMaxAge两个选项不能共存，只能设置一个(都设置编译时不会出错，但运行时会报错。也是为了防止影响切分的处理逻辑)
		// rotatelogs.WithRotationCount(10),       // 超过这个数的文件会被清掉
		rotatelogs.WithMaxAge(time.Hour*DayHour*WeekDay),
		rotatelogs.WithRotationTime(time.Hour*DayHour),
		// rotatelogs.WithRotationSize(), // 按文件大小切割日志,单位为 bytes
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
