package logger

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// ContextFieldsFunc 从 context.Context 中提取需要注入日志的字段。
// 典型用法：提取 trace_id、request_id 等链路追踪信息。
type ContextFieldsFunc func(ctx context.Context) []zap.Field

const (
	defaultPath       = "./logs/"
	defaultMaxSize    = 512 // MB
	defaultMaxAge     = 7   // days
	defaultMaxBackups = 50
)

var (
	baseGlobalLogger = zap.L()
	globalMu         sync.Mutex
	currentState     atomic.Pointer[loggerState]

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
	currentState.Store(newFallbackState())
}

type logConfig struct {
	consoleStdout     bool
	fileStdout        bool
	outJSON           bool
	division          string
	path              string
	compress          bool
	maxAge            int
	maxBackups        int
	maxSize           int
	level             string
	messager          Messager
	messagerQueueSize int
	contextFields     ContextFieldsFunc
	channels          map[string]*channelConfig
}

type channelConfig struct {
	path               string
	duplicateToDefault bool
}

// NewZap initializes the package-level logger. It is safe to call repeatedly.
func NewZap(opts ...Option) {
	cfg := &logConfig{
		consoleStdout:     false,
		fileStdout:        true,
		outJSON:           false,
		division:          "size",
		path:              defaultPath,
		compress:          true,
		maxAge:            defaultMaxAge,
		maxBackups:        defaultMaxBackups,
		maxSize:           defaultMaxSize,
		level:             "info",
		messagerQueueSize: 1024,
		channels:          make(map[string]*channelConfig),
	}

	for _, o := range opts {
		if err := o(cfg); err != nil {
			panic(fmt.Sprintf("logger: apply option: %v", err))
		}
	}

	initZap(cfg)
}

func initZap(cfg *logConfig) {
	state, err := buildLoggerState(cfg)
	if err != nil {
		panic(fmt.Sprintf("logger: build logger: %v", err))
	}

	previous := swapLoggerState(state, true)
	retireLoggerState(previous)
}

func getLoggerLevel(lvl string) zapcore.Level {
	if level, ok := levelMap[lvl]; ok {
		return level
	}

	return zapcore.InfoLevel
}

// Sync flushes buffered logs and closes file resources.
func Sync() {
	fallback := newFallbackState()
	previous := swapLoggerState(fallback, false)
	retireLoggerState(previous)
}

// Undo restores the previous global zap logger.
func Undo() {
	globalMu.Lock()
	defer globalMu.Unlock()

	state := currentState.Load()
	if state != nil && state.undo != nil {
		state.undo()
		state.undo = nil
		return
	}

	zap.ReplaceGlobals(baseGlobalLogger)
}

func buildFileWriter(cfg *logConfig) (zapcore.WriteSyncer, []io.Closer, error) {
	switch cfg.division {
	case "daily":
		return buildDailyWriter(cfg)
	default:
		return buildSizeWriter(cfg)
	}
}

func buildSizeWriter(cfg *logConfig) (zapcore.WriteSyncer, []io.Closer, error) {
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

func buildDailyWriter(cfg *logConfig) (zapcore.WriteSyncer, []io.Closer, error) {
	dw, err := newDailyWriteSyncer(cfg)
	if err != nil {
		return nil, nil, err
	}

	return zapcore.AddSync(dw), []io.Closer{dw}, nil
}

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

func buildLoggerState(cfg *logConfig) (*loggerState, error) {
	atomicLevel := zap.NewAtomicLevelAt(getLoggerLevel(cfg.level))

	defaultCore, defaultClosers, err := buildCore(cfg, atomicLevel)
	if err != nil {
		return nil, err
	}

	allClosers := append([]io.Closer{}, defaultClosers...)
	root := newZapLogger(defaultCore)
	channelBases := make(map[string]*zap.Logger, len(cfg.channels))

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

		channelBases[name] = newZapLogger(routedCore).With(zap.String("channel", name))
	}

	var msgr Messager
	var asyncMsg *asyncMessager
	if cfg.messager != nil {
		asyncMsg = newAsyncMessager(cfg.messager, cfg.messagerQueueSize)
		msgr = asyncMsg
	}

	return newLoggerState(root, channelBases, allClosers, msgr, asyncMsg, cfg.contextFields, atomicLevel), nil
}

func buildCore(cfg *logConfig, lvl zap.AtomicLevel) (zapcore.Core, []io.Closer, error) {
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

func buildChannelCore(root *logConfig, channel *channelConfig, lvl zap.AtomicLevel) (zapcore.Core, []io.Closer, error) {
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

func newFallbackState() *loggerState {
	logger, err := zap.NewDevelopment(zap.AddCallerSkip(1))
	if err != nil {
		logger = zap.NewNop()
	}

	return newLoggerState(logger, nil, nil, nil, nil, nil, zap.NewAtomicLevelAt(zapcore.DebugLevel))
}

func swapLoggerState(next *loggerState, replaceGlobals bool) *loggerState {
	globalMu.Lock()
	defer globalMu.Unlock()

	previous := currentState.Swap(next)
	if previous != nil && previous.undo != nil {
		previous.undo()
		previous.undo = nil
	} else if previous == nil && !replaceGlobals {
		zap.ReplaceGlobals(baseGlobalLogger)
	}

	if replaceGlobals {
		next.undo = zap.ReplaceGlobals(next.root)
	}

	return previous
}

func retireLoggerState(state *loggerState) {
	if state == nil {
		return
	}

	state.retire()
	state.wait()
	state.closeResources()
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

func validateChannelRoute(root *logConfig, name string, channel *channelConfig) error {
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
