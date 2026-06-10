package logger

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gtkit/logrotate"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ContextFieldsFunc 从 context.Context 中提取需要注入日志的字段。
// 典型用法：提取 trace_id、request_id 等链路追踪信息。
type ContextFieldsFunc func(ctx context.Context) []zap.Field

const (
	defaultPath       = "./logs/"
	defaultMaxSize    = 512 // MB
	defaultMaxAge     = 7   // days
	defaultMaxBackups = 50
	noSizeRotationMB  = 1 << 30
)

type rotationDivision string

const (
	rotationSize  rotationDivision = "size"
	rotationDaily rotationDivision = "daily"
	rotationBoth  rotationDivision = "both"
)

var (
	baseGlobalLogger = zap.L()
	globalMu         sync.Mutex
	currentState     atomic.Pointer[loggerState]

	// loggerInitialized 在首次成功 New() 后置为 true，用于区分"从未初始化"与"已初始化"。
	// fallback 告警只在从未 New() 的情况下触发——Sync() 之后的 fallback 不再告警。
	loggerInitialized atomic.Bool

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
	consoleStdout      bool
	fileStdout         bool
	outJSON            bool
	durationEncoder    zapcore.DurationEncoder
	division           rotationDivision
	path               string
	compress           bool
	maxAge             int
	maxBackups         int
	maxSize            int
	level              string
	messager           Messager
	messagerQueueSize  int
	contextFields      ContextFieldsFunc
	channels           map[string]*channelConfig
	buffered           bool
	bufferSize         int
	flushInterval      time.Duration
	samplingFirst      int
	samplingThereafter int
	fieldRedactor      func([]zapcore.Field) []zapcore.Field
}

type channelConfig struct {
	path               string
	duplicateToDefault bool
}

// New initializes the package-level logger, returning an error on failure.
// It is safe to call repeatedly.
func New(opts ...Option) error {
	cfg := &logConfig{
		consoleStdout:     false,
		fileStdout:        true,
		outJSON:           false,
		durationEncoder:   zapcore.SecondsDurationEncoder,
		division:          rotationBoth,
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
			return fmt.Errorf("logger: apply option: %w", err)
		}
	}

	state, err := buildLoggerState(cfg)
	if err != nil {
		return err
	}

	previous := swapLoggerState(state, true)
	retireLoggerState(previous)
	loggerInitialized.Store(true)
	return nil
}

// NewZap initializes the package-level logger. It panics on failure.
// It is safe to call repeatedly.
func NewZap(opts ...Option) {
	if err := New(opts...); err != nil {
		panic(err)
	}
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
	lr := &logrotate.Logger{
		Filename:   logFilename(cfg),
		MaxSize:    logrotateMaxSize(cfg),
		MaxAge:     cfg.maxAge,
		MaxBackups: cfg.maxBackups,
		Compress:   cfg.compress,
		LocalTime:  true,
	}

	if cfg.division == rotationDaily || cfg.division == rotationBoth {
		lr.DailyFilename = true
	}

	return wrapWriter(cfg, zapcore.AddSync(lr), lr)
}

func logFilename(cfg *logConfig) string {
	return cfg.path + "-" + cfg.level + ".log"
}

func logrotateMaxSize(cfg *logConfig) int {
	if cfg.division == rotationDaily {
		return noSizeRotationMB
	}
	return cfg.maxSize
}

const (
	defaultBufferSize    = 256 * 1024 // 256KB
	defaultFlushInterval = 30 * time.Second
)

// wrapWriter 根据配置决定是否用 BufferedWriteSyncer 包装底层 WriteSyncer。
func wrapWriter(cfg *logConfig, ws zapcore.WriteSyncer, underlying io.Closer) (zapcore.WriteSyncer, []io.Closer, error) {
	if !cfg.buffered {
		return ws, []io.Closer{underlying}, nil
	}

	bufSize := cfg.bufferSize
	if bufSize <= 0 {
		bufSize = defaultBufferSize
	}
	flushInterval := cfg.flushInterval
	if flushInterval <= 0 {
		flushInterval = defaultFlushInterval
	}

	bws := &zapcore.BufferedWriteSyncer{
		WS:            ws,
		Size:          bufSize,
		FlushInterval: flushInterval,
	}
	// BufferedWriteSyncer.Stop() 只 flush + sync，不会 close 底层 writer，
	// 所以 stopCloser 需要先 Stop 再 Close underlying，确保文件句柄释放。
	return bws, []io.Closer{&stopCloser{bws: bws, underlying: underlying}}, nil
}

// stopCloser 将 BufferedWriteSyncer 的 Stop() 适配为 io.Closer，
// 并负责关闭底层 writer。
type stopCloser struct {
	bws        *zapcore.BufferedWriteSyncer
	underlying io.Closer
}

func (s *stopCloser) Close() error {
	err := s.bws.Stop()
	if err2 := s.underlying.Close(); err == nil {
		err = err2
	}
	return err
}

func buildEncoder(outJSON bool, durationEncoder zapcore.DurationEncoder) zapcore.Encoder {
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
	if durationEncoder == nil {
		durationEncoder = zapcore.SecondsDurationEncoder
	}
	ec.EncodeDuration = durationEncoder
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

	// 全配对路径冲突检查——在分配 channel core 之前做，避免 fail-after-allocate。
	if err := validateChannelRoutes(cfg); err != nil {
		closeClosers(defaultClosers)
		return nil, err
	}

	allClosers := append([]io.Closer{}, defaultClosers...)
	root := newZapLogger(defaultCore)
	channelBases := make(map[string]*zap.Logger, len(cfg.channels))

	for name, channelCfg := range cfg.channels {
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
		buildEncoder(cfg.outJSON, cfg.durationEncoder),
		zapcore.NewMultiWriteSyncer(writers...),
		lvl,
	)

	// 字段脱敏：包在采样之内，With() 预绑定字段经 sampler.With 透传后同样过脱敏。
	// nil 时不包装（零开销）。
	if cfg.fieldRedactor != nil {
		core = newRedactCore(core, cfg.fieldRedactor)
	}

	// 采样：同一 tick 内（1s）相同 level+message 先放行 first 条，之后每 thereafter 条放行一条。
	// 防止热循环里的高频日志打爆磁盘 / 拖垮下游。两值均为 0 时不包装（默认关闭）。
	//
	// sampler 必须包在最外层：zap 的采样判定全部在 sampler.Check 里完成，而装饰器型 core
	// （如 redactCore）的 Check 会把自身 AddCore 进 CheckedEntry、不再调用内层 Check——
	// 若 sampler 被包在内层，其 Check 永远不会执行，采样将静默失效。
	if cfg.samplingFirst > 0 || cfg.samplingThereafter > 0 {
		core = zapcore.NewSamplerWithOptions(core, time.Second, cfg.samplingFirst, cfg.samplingThereafter)
	}

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

	state := newLoggerState(logger, nil, nil, nil, nil, nil, zap.NewAtomicLevelAt(zapcore.DebugLevel))
	state.fallback = true
	return state
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

func closeClosers(closers []io.Closer) {
	for _, c := range closers {
		if err := c.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "logger: close resource: %v\n", err)
		}
	}
}

// validateChannelRoutes 对 root + 所有 channel 做全配对路径冲突检查。
//
// 冲突规则：
//   - 任一 channel 与 root 同路径 → 错误（duplicate=true 时双写同一文件；duplicate=false 时
//     两个独立 writer 竞争同一文件——两种情况都会引起 rotate/写入竞态）
//   - 两个 channel 同路径 → 错误（不论是否 duplicate，都会引起 rotator 实例间竞态）
//
// 命名按字典序遍历，确保错误信息确定性（便于测试与排错）。
func validateChannelRoutes(root *logConfig) error {
	rootKey := normalizedPathKey(root.path)
	seen := map[string]string{rootKey: ""} // pathKey -> channelName (""=root)

	names := make([]string, 0, len(root.channels))
	for name := range root.channels {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		ch := root.channels[name]
		key := normalizedPathKey(ch.path)
		owner, exists := seen[key]
		if !exists {
			seen[key] = name
			continue
		}
		if owner == "" {
			if ch.duplicateToDefault {
				return fmt.Errorf("logger: channel %q path must differ from default path when duplicate-to-default is enabled", name)
			}
			return fmt.Errorf("logger: channel %q path %q overlaps default path; multiple writers would race on the same file", name, ch.path)
		}
		return fmt.Errorf("logger: channel %q path conflicts with channel %q (both resolve to %q)", name, owner, ch.path)
	}
	return nil
}

// normalizedPathKey 把路径规范化为可比较的 key——Windows 大小写不敏感，其他平台敏感。
func normalizedPathKey(path string) string {
	clean := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(clean)
	}
	return clean
}
