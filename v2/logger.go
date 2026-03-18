package logger

import (
	"fmt"
	"io"
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Logger 封装 zap.Logger，提供 Structured 和 Sugar 双模式日志.
// 通过 New 或 MustNew 创建实例.
type Logger struct {
	zap      *zap.Logger
	sugar    *zap.SugaredLogger
	undo     func()
	messager Messager
	closers  []io.Closer
}

// New 创建 Logger 实例.
//
//	log, err := logger.New(
//	    logger.WithPath("./logs/app"),
//	    logger.WithLevel("info"),
//	    logger.WithOutJSON(true),
//	)
//	if err != nil {
//	    panic(err)
//	}
//	defer log.Sync()
func New(opts ...Option) (*Logger, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		if err := opt(cfg); err != nil {
			return nil, fmt.Errorf("logger: apply option: %w", err)
		}
	}

	return build(cfg)
}

// MustNew 同 New，但配置错误时 panic.
// 适用于 main 函数中初始化.
func MustNew(opts ...Option) *Logger {
	l, err := New(opts...)
	if err != nil {
		panic(err)
	}

	return l
}

// build 根据配置构建 Logger.
func build(cfg *Config) (*Logger, error) {
	level, ok := levelMap[cfg.level]
	if !ok {
		level = zapcore.InfoLevel
	}

	encoder := buildEncoder(cfg.outJSON)

	var (
		writers []zapcore.WriteSyncer
		closers []io.Closer
	)

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

	// CallerSkip(1): 跳过本包的封装层，显示调用者真实位置.
	zlog := zap.New(
		core,
		zap.AddCaller(),
		zap.AddCallerSkip(1),
		zap.AddStacktrace(zap.ErrorLevel),
	)

	undo := zap.ReplaceGlobals(zlog)

	return &Logger{
		zap:      zlog,
		sugar:    zlog.Sugar(),
		undo:     undo,
		messager: cfg.messager,
		closers:  closers,
	}, nil
}

// buildFileWriter 根据切割方式构建文件 WriteSyncer.
func buildFileWriter(cfg *Config) (zapcore.WriteSyncer, []io.Closer) {
	switch cfg.division {
	case "daily":
		dw := newDailyWriteSyncer(cfg)
		return zapcore.AddSync(dw), []io.Closer{dw}
	default:
		return buildSizeWriter(cfg)
	}
}

// buildSizeWriter 按大小切割.
// 文件名格式: {path}-{level}.log
func buildSizeWriter(cfg *Config) (zapcore.WriteSyncer, []io.Closer) {
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

// buildEncoder 构建日志编码器.
func buildEncoder(outJSON bool) zapcore.Encoder {
	ec := zap.NewProductionEncoderConfig()

	ec.TimeKey = "time"
	ec.LevelKey = "level"
	ec.NameKey = "logger"
	ec.CallerKey = "caller"
	ec.MessageKey = "msg"
	ec.StacktraceKey = "stacktrace"

	ec.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.Format("2006-01-02T15:04:05.000Z0700"))
	}
	ec.LineEnding = zapcore.DefaultLineEnding
	ec.EncodeLevel = zapcore.CapitalLevelEncoder
	ec.EncodeDuration = zapcore.SecondsDurationEncoder
	ec.EncodeCaller = zapcore.ShortCallerEncoder

	if outJSON {
		return zapcore.NewJSONEncoder(ec)
	}

	return zapcore.NewConsoleEncoder(ec)
}
