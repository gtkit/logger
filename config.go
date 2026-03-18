package logger

import (
	"fmt"
	"io"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	defaultPath       = "./logs/"
	defaultMaxSize    = 512 // MB
	defaultMaxAge     = 7  // 天
	defaultMaxBackups = 50
)

// 包级全局变量.
// init() 中赋予安全默认值，NewZap() 中替换为正式配置.
var (
	zaplog  *zap.Logger
	sugar   *zap.SugaredLogger
	undofn  func()
	closers []io.Closer

	levelMap = map[string]zapcore.Level{
		"debug":  zapcore.DebugLevel,
		"info":   zapcore.InfoLevel,
		"warn":   zapcore.WarnLevel,
		"error":  zapcore.ErrorLevel,
		"dpanic": zapcore.DPanicLevel,
		"panic":  zapcore.PanicLevel,
		"fatal":  zapcore.FatalLevel,
	}
)

func init() {
	// 安全默认值：未调用 NewZap 时输出到 stderr，不会 nil panic.
	var err error

	zaplog, err = zap.NewDevelopment(zap.AddCallerSkip(1))
	if err != nil {
		zaplog = zap.NewNop()
	}

	sugar = zaplog.Sugar()
}

type logConfig struct {
	consoleStdout bool   // 是否输出到控制台
	fileStdout    bool   // 是否输出到文件
	outJSON       bool   // 是否输出为 JSON 格式
	division      string // 切割方式: "size" 或 "daily"
	path          string // 日志文件路径前缀
	compress      bool   // 是否压缩归档文件
	maxAge        int    // 最大保存天数
	maxBackups    int    // 最大备份数量
	maxSize       int    // 单文件最大大小 (MB)
	level         string // 日志级别
	messager      Messager
}

// NewZap 函数选项模式初始化全局 logger.
// 可重复调用，会自动清理上一次初始化的资源.
func NewZap(opts ...Option) {
	cfg := &logConfig{
		consoleStdout: false,
		fileStdout:    true,
		outJSON:       false,
		division:      "size",
		path:          defaultPath,
		compress:      true,
		maxAge:        defaultMaxAge,
		maxBackups:    defaultMaxBackups,
		maxSize:       defaultMaxSize,
		level:         "info",
	}

	for _, o := range opts {
		if err := o(cfg); err != nil {
			panic(fmt.Sprintf("logger: apply option: %v", err))
		}
	}

	msgr = cfg.messager

	initZap(cfg)
}

func initZap(cfg *logConfig) {
	// 清理上一次初始化的资源，防止重复调用 NewZap 导致泄漏.
	for _, c := range closers {
		_ = c.Close()
	}

	closers = closers[:0]

	level := getLoggerLevel(cfg.level)
	encoder := buildEncoder(cfg.outJSON)

	var writers []zapcore.WriteSyncer

	if cfg.consoleStdout {
		writers = append(writers, zapcore.Lock(os.Stdout))
	}

	if cfg.fileStdout {
		ws, cl := buildFileWriter(cfg)
		writers = append(writers, ws)
		closers = append(closers, cl...)
	}

	// 两者都未开启时，至少输出到 stdout，避免静默丢日志.
	if len(writers) == 0 {
		writers = append(writers, zapcore.Lock(os.Stdout))
	}

	core := zapcore.NewCore(
		encoder,
		zapcore.NewMultiWriteSyncer(writers...),
		zap.NewAtomicLevelAt(level),
	)

	zaplog = zap.New(
		core,
		zap.AddCaller(),
		zap.AddCallerSkip(1),
		zap.AddStacktrace(zap.ErrorLevel),
	)
	sugar = zaplog.Sugar()

	undofn = zap.ReplaceGlobals(zaplog)
}

func getLoggerLevel(lvl string) zapcore.Level {
	if level, ok := levelMap[lvl]; ok {
		return level
	}

	return zapcore.InfoLevel
}

// Sync 刷新缓冲区并关闭文件资源.
// 应在程序退出前调用: defer logger.Sync()
func Sync() {
	if undofn != nil {
		undofn()
	}

	_ = zaplog.Sync()

	for _, c := range closers {
		_ = c.Close()
	}
}

// Undo 恢复 zap 全局 logger 到替换前的状态.
func Undo() {
	if undofn != nil {
		undofn()
	}
}

// buildFileWriter 根据切割方式构建文件 WriteSyncer.
func buildFileWriter(cfg *logConfig) (zapcore.WriteSyncer, []io.Closer) {
	switch cfg.division {
	case "daily":
		return buildDailyWriter(cfg)
	default:
		return buildSizeWriter(cfg)
	}
}

// buildSizeWriter 按大小切割.
// 文件名格式: {path}-{level}.log
func buildSizeWriter(cfg *logConfig) (zapcore.WriteSyncer, []io.Closer) {
	logpath := cfg.path + "-" + cfg.level + ".log"

	lj := &lumberjack.Logger{
		Filename:   logpath,
		MaxSize:    cfg.maxSize,
		MaxAge:     cfg.maxAge,
		MaxBackups: cfg.maxBackups,
		Compress:   cfg.compress,
		LocalTime:  true,
	}

	return zapcore.AddSync(lj), []io.Closer{lj}
}

// buildDailyWriter 按天切割.
// 使用 dailyWriteSyncer 在写入时自动检测日期变化并切换文件.
func buildDailyWriter(cfg *logConfig) (zapcore.WriteSyncer, []io.Closer) {
	dw := newDailyWriteSyncer(cfg)
	return zapcore.AddSync(dw), []io.Closer{dw}
}

// buildEncoder 构建日志编码器.
func buildEncoder(outJSON bool) zapcore.Encoder {
	ec := zap.NewProductionEncoderConfig()

	ec.TimeKey = "time"
	ec.LevelKey = "level"
	ec.NameKey = "logger"
	ec.CallerKey = "caller"
	ec.MessageKey = "msg"
	ec.StacktraceKey = "stacktrace"

	ec.EncodeTime = zapcore.ISO8601TimeEncoder
	ec.LineEnding = zapcore.DefaultLineEnding
	ec.EncodeLevel = zapcore.CapitalLevelEncoder
	ec.EncodeDuration = zapcore.SecondsDurationEncoder
	ec.EncodeCaller = zapcore.ShortCallerEncoder

	if outJSON {
		return zapcore.NewJSONEncoder(ec)
	}

	return zapcore.NewConsoleEncoder(ec)
}
