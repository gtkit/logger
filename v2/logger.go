package logger

import (
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

type channelRoute struct {
	logger *zap.Logger
}

// maxDynamicChannels 动态 channel 缓存上限，防止无限增长导致内存泄漏。
const maxDynamicChannels = 1024

type lifecycleState struct {
	root *zap.Logger
	undo func()
	// installed 是本实例安装进 zap 全局的视图指针；Undo 仅在全局仍指向它时
	// 才执行恢复，避免延迟 Sync 的旧实例把后来者安装的全局踩掉。
	installed              *zap.Logger
	closers                []io.Closer
	asyncMsg               *asyncMessager
	atomicLevel            zap.AtomicLevel
	fieldRedactor          func([]zapcore.Field) []zapcore.Field
	channelRoutes          map[string]*channelRoute
	rootChannels           map[string]*Logger
	dynamicChannelBases    sync.Map
	dynamicChannelBasesCnt atomic.Int64

	undoOnce  sync.Once
	closeOnce sync.Once
}

func (s *lifecycleState) Undo() {
	if s == nil {
		return
	}

	s.undoOnce.Do(func() {
		if s.undo == nil {
			return
		}
		// 全局已被后来者替换时放弃恢复：只收拾自己的摊位，不踩后来者。
		if s.installed != nil && zap.L() != s.installed {
			return
		}
		s.undo()
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
			// console(终端/管道)的 fsync 失败是平台噪音,静默;文件输出的真实失败照常告警。
			if err := s.root.Sync(); err != nil && !isBenignSyncError(err) {
				fmt.Fprintf(os.Stderr, "logger: sync root logger: %v\n", err)
			}
		}
		closeClosers(s.closers)
	})
}

// Logger 是 v2 的核心日志实例，封装 zap 并提供 channel 路由、消息推送与 ctx 字段注入能力。
// 由 New/MustNew 构建；With/Named/Channel 派生的实例共享底层资源。
// 全部记录方法并发安全；Sync/Undo 是生命周期操作，须在停止新写入、
// 在途调用结束后串行执行（关闭后的并发写入可能重新打开已关闭的文件句柄）。
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
	// callerSkip 是本实例相对 canonical 视图的 caller 偏移（facade 转发层数）。
	// 偏移只存在于 zap/sugar 视图与本元数据中；base 与所有共享缓存
	// （注册 channel、动态 channel base）永远保持 canonical（零偏移）。
	callerSkip int
	// boundRequestID 记录 With() 预绑定字段中是否含 request_id（构建期一次判定，
	// 热路径零扫描）。预绑定是作用域身份，request_id 去重优先级最高。
	boundRequestID bool
}

// New 按 Functional Options 构建 Logger；失败返回 error。
// 默认将实例安装为 zap 全局 logger（zap.L()/zap.S()），可经 WithReplaceGlobals(false) 关闭。
// 使用完毕通过 Sync 释放资源。
func New(opts ...Option) (*Logger, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		if err := opt(cfg); err != nil {
			return nil, fmt.Errorf("logger: apply option: %w", err)
		}
	}

	return build(cfg)
}

// MustNew 与 New 相同，但失败时 panic，适合 main/初始化阶段。
func MustNew(opts ...Option) *Logger {
	l, err := New(opts...)
	if err != nil {
		panic(err)
	}

	return l
}

func build(cfg *Config) (*Logger, error) {
	applyBasePath(cfg)

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

	// 抵消内部包装层的 caller skip：zap.L()/zap.S() 由调用方直接使用，
	// 不经过本库包装方法，原样安装 root 会导致 caller 多跳一帧。
	var undo func()
	var installed *zap.Logger
	if cfg.replaceGlobals {
		installed = built.root.WithOptions(zap.AddCallerSkip(-1))
		undo = zap.ReplaceGlobals(installed)
	}
	state := &lifecycleState{
		root:          built.root,
		undo:          undo,
		installed:     installed,
		closers:       built.closers,
		asyncMsg:      asyncMsg,
		atomicLevel:   built.atomicLevel,
		fieldRedactor: cfg.fieldRedactor,
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

// applyBasePath 把相对日志路径（含 channel 路径）锚定到 basePath；
// 绝对路径原样使用，未设置 basePath 时不做任何改写（与既有行为一致）。
func applyBasePath(cfg *Config) {
	if cfg.basePath == "" {
		return
	}
	cfg.path = anchorToBase(cfg.basePath, cfg.path)
	for _, ch := range cfg.channels {
		ch.path = anchorToBase(cfg.basePath, ch.path)
	}
}

func anchorToBase(base, path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}

	joined := filepath.Join(base, path)
	// path 是「文件名前缀」而非目录：尾分隔符参与前缀语义
	// （如默认 "./logs/" 产出 logs/ 目录内的 "-<level>.log"）。
	// filepath.Join 会吞掉尾分隔符，这里补回，锚定不得改变目录结构。
	if strings.HasSuffix(path, "/") || strings.HasSuffix(path, string(os.PathSeparator)) {
		joined += string(os.PathSeparator)
	}

	return joined
}

func buildFileWriter(cfg *Config) (zapcore.WriteSyncer, []io.Closer, error) {
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

func logFilename(cfg *Config) string {
	return cfg.path + "-" + cfg.level + ".log"
}

func logrotateMaxSize(cfg *Config) int {
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
func wrapWriter(cfg *Config, ws zapcore.WriteSyncer, underlying io.Closer) (zapcore.WriteSyncer, []io.Closer, error) {
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

	// 全配对路径冲突检查——在分配 channel core 之前做。
	if err := validateChannelRoutes(cfg); err != nil {
		closeClosers(defaultClosers)
		return nil, err
	}

	allClosers := append([]io.Closer{}, defaultClosers...)
	channelRoutes := make(map[string]*channelRoute, len(cfg.channels))

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

	consoleWS := cfg.consoleWriter
	if consoleWS == nil {
		consoleWS = zapcore.Lock(os.Stdout)
	}

	if cfg.consoleStdout {
		writers = append(writers, consoleWS)
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
		writers = append(writers, consoleWS)
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
//   - channel 与 root 同路径 → 错误（无论 duplicate-to-default 与否，都会引起竞态）
//   - 两个 channel 同路径 → 错误（rotator 实例间会竞争 rotate）
//
// 命名按字典序遍历，确保错误信息确定性。
func validateChannelRoutes(root *Config) error {
	rootKey := normalizedPathKey(root.path)
	seen := map[string]string{rootKey: ""}

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

// normalizedPathKey 路径规范化为可比较 key。Windows 大小写不敏感，其他平台敏感。
func normalizedPathKey(path string) string {
	clean := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(clean)
	}
	return clean
}
