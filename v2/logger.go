package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type channelRoute struct {
	logger *zap.Logger
}

type lifecycleState struct {
	root                *zap.Logger
	undo                func()
	closers             []io.Closer
	asyncMsg            *asyncMessager
	atomicLevel         zap.AtomicLevel
	channelRoutes       map[string]*channelRoute
	rootChannels        map[string]*Logger
	dynamicChannelBases sync.Map

	undoOnce  sync.Once
	closeOnce sync.Once
}

func (s *lifecycleState) Undo() {
	if s == nil {
		return
	}

	s.undoOnce.Do(func() {
		if s.undo != nil {
			s.undo()
		}
	})
}

func (s *lifecycleState) Sync() {
	if s == nil {
		return
	}

	s.closeOnce.Do(func() {
		s.Undo()
		if s.asyncMsg != nil {
			s.asyncMsg.close()
		}
		if s.root != nil {
			_ = s.root.Sync()
		}
		for _, c := range s.closers {
			_ = c.Close()
		}
	})
}

type Logger struct {
	base          *zap.Logger
	zap           *zap.Logger
	sugar         *zap.SugaredLogger
	state         *lifecycleState
	messager      Messager
	contextFields ContextFieldsFunc
	channel       string
	name          string
	fields        []zap.Field
}

func New(opts ...Option) (*Logger, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		if err := opt(cfg); err != nil {
			return nil, fmt.Errorf("logger: apply option: %w", err)
		}
	}

	return build(cfg)
}

func MustNew(opts ...Option) *Logger {
	l, err := New(opts...)
	if err != nil {
		panic(err)
	}

	return l
}

func build(cfg *Config) (*Logger, error) {
	built, err := buildLoggerSet(cfg)
	if err != nil {
		return nil, err
	}

	var msgr Messager
	var asyncMsg *asyncMessager
	if cfg.messager != nil {
		asyncMsg = newAsyncMessager(cfg.messager, cfg.messagerQueueSize)
		msgr = asyncMsg
	}

	undo := zap.ReplaceGlobals(built.root)
	state := &lifecycleState{
		root:          built.root,
		undo:          undo,
		closers:       built.closers,
		asyncMsg:      asyncMsg,
		atomicLevel:   built.atomicLevel,
		channelRoutes: built.channelRoutes,
		rootChannels:  make(map[string]*Logger, len(built.channelRoutes)),
	}

	rootLogger := &Logger{
		base:          built.root,
		zap:           built.root,
		sugar:         built.root.Sugar(),
		state:         state,
		messager:      msgr,
		contextFields: cfg.contextFields,
	}
	for name, route := range built.channelRoutes {
		state.rootChannels[name] = &Logger{
			base:          built.root,
			zap:           route.logger,
			sugar:         route.logger.Sugar(),
			state:         state,
			messager:      msgr,
			contextFields: cfg.contextFields,
			channel:       name,
		}
	}

	return rootLogger, nil
}

func buildFileWriter(cfg *Config) (zapcore.WriteSyncer, []io.Closer, error) {
	switch cfg.division {
	case "daily":
		dw, err := newDailyWriteSyncer(cfg)
		if err != nil {
			return nil, nil, err
		}
		return zapcore.AddSync(dw), []io.Closer{dw}, nil
	default:
		return buildSizeWriter(cfg)
	}
}

func buildSizeWriter(cfg *Config) (zapcore.WriteSyncer, []io.Closer, error) {
	logpath := cfg.path + "-" + cfg.level + ".log"
	if err := ensureLogDir(logpath); err != nil {
		return nil, nil, err
	}

	lj := &lumberjack.Logger{
		Filename:   logpath,
		MaxSize:    cfg.maxSize,
		MaxAge:     cfg.maxAge,
		MaxBackups: cfg.maxBackups,
		Compress:   cfg.compress,
		LocalTime:  true,
	}

	return zapcore.AddSync(lj), []io.Closer{lj}, nil
}

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

type builtLoggerSet struct {
	root          *zap.Logger
	closers       []io.Closer
	atomicLevel   zap.AtomicLevel
	channelRoutes map[string]*channelRoute
}

func buildLoggerSet(cfg *Config) (*builtLoggerSet, error) {
	level, ok := levelMap[cfg.level]
	if !ok {
		level = zapcore.InfoLevel
	}
	atomicLevel := zap.NewAtomicLevelAt(level)

	defaultCore, defaultClosers, err := buildCore(cfg, atomicLevel)
	if err != nil {
		return nil, err
	}

	allClosers := append([]io.Closer{}, defaultClosers...)
	channelRoutes := make(map[string]*channelRoute, len(cfg.channels))

	for name, channelCfg := range cfg.channels {
		if err := validateChannelRoute(cfg, name, channelCfg); err != nil {
			closeClosers(allClosers)
			return nil, err
		}

		channelCore, channelClosers, buildErr := buildChannelCore(cfg, channelCfg, atomicLevel)
		if buildErr != nil {
			closeClosers(allClosers)
			return nil, buildErr
		}

		allClosers = append(allClosers, channelClosers...)
		routedCore := channelCore
		if channelCfg.duplicateToDefault {
			routedCore = zapcore.NewTee(defaultCore, channelCore)
		}
		channelRoutes[name] = &channelRoute{
			logger: newZapLogger(routedCore).With(zap.String("channel", name)),
		}
	}

	return &builtLoggerSet{
		root:          newZapLogger(defaultCore),
		closers:       allClosers,
		atomicLevel:   atomicLevel,
		channelRoutes: channelRoutes,
	}, nil
}

func buildCore(cfg *Config, lvl zap.AtomicLevel) (zapcore.Core, []io.Closer, error) {
	var (
		writers []zapcore.WriteSyncer
		closers []io.Closer
	)

	if cfg.consoleStdout {
		writers = append(writers, zapcore.Lock(os.Stdout))
	}

	if cfg.fileStdout {
		ws, cl, err := buildFileWriter(cfg)
		if err != nil {
			return nil, nil, err
		}
		writers = append(writers, ws)
		closers = append(closers, cl...)
	}

	if len(writers) == 0 {
		writers = append(writers, zapcore.Lock(os.Stdout))
	}

	core := zapcore.NewCore(
		buildEncoder(cfg.outJSON),
		zapcore.NewMultiWriteSyncer(writers...),
		lvl,
	)

	return core, closers, nil
}

func buildChannelCore(root *Config, channel *channelConfig, lvl zap.AtomicLevel) (zapcore.Core, []io.Closer, error) {
	cfg := *root
	cfg.consoleStdout = false
	cfg.fileStdout = true
	cfg.path = channel.path
	cfg.channels = nil

	return buildCore(&cfg, lvl)
}

func newZapLogger(core zapcore.Core) *zap.Logger {
	return zap.New(
		core,
		zap.AddCaller(),
		zap.AddCallerSkip(1),
		zap.AddStacktrace(zap.ErrorLevel),
	)
}

func ensureLogDir(logpath string) error {
	dir := filepath.Dir(logpath)
	if dir == "." || dir == "" {
		return nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("logger: create log dir %q: %w", dir, err)
	}
	return nil
}

func closeClosers(closers []io.Closer) {
	for _, c := range closers {
		_ = c.Close()
	}
}

func validateChannelRoute(root *Config, name string, channel *channelConfig) error {
	if !channel.duplicateToDefault {
		return nil
	}

	if sameLogPath(root.path, channel.path) {
		return fmt.Errorf("logger: channel %q path must differ from default path when duplicate-to-default is enabled", name)
	}

	return nil
}

func sameLogPath(left, right string) bool {
	cleanLeft := filepath.Clean(left)
	cleanRight := filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(cleanLeft, cleanRight)
	}

	return cleanLeft == cleanRight
}
