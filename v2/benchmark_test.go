package logger

import (
	"io"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func BenchmarkInfo(b *testing.B) {
	log := &Logger{
		base:  zap.NewNop(),
		zap:   zap.NewNop(),
		sugar: zap.NewNop().Sugar(),
		state: &lifecycleState{root: zap.NewNop()},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		log.Info("bench info", zap.String("key", "value"))
	}
}

func BenchmarkChannelConfiguredLookupAndInfo(b *testing.B) {
	log := newBenchmarkLogger()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		log.Channel("order").Info("bench channel", zap.String("key", "value"))
	}
}

func BenchmarkChannelConfiguredReuse(b *testing.B) {
	log := newBenchmarkLogger()
	orderLog := log.Channel("order")

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		orderLog.Info("bench channel", zap.String("key", "value"))
	}
}

func BenchmarkChannelUnconfiguredReuse(b *testing.B) {
	log := newBenchmarkLogger()
	paymentLog := log.Channel("payment")

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		paymentLog.Info("bench channel", zap.String("key", "value"))
	}
}

func newBenchmarkLogger() *Logger {
	root := zap.NewNop()
	order := root.With(zap.String("channel", "order"))
	state := &lifecycleState{
		root: root,
		channelRoutes: map[string]*channelRoute{
			"order": {
				logger: order,
			},
		},
		rootChannels: make(map[string]*Logger, 1),
	}
	state.rootChannels["order"] = &Logger{
		base:    root,
		zap:     order,
		sugar:   order.Sugar(),
		state:   state,
		channel: "order",
	}

	return &Logger{
		base:  root,
		zap:   root,
		sugar: root.Sugar(),
		state: state,
	}
}

// BenchmarkInfoCtxLevelDisabled 级别关闭时 *Ctx 的开销：
// 门控在 ctx 字段提取之前，无调用点字段时期望零分配。
func BenchmarkInfoCtxLevelDisabled(b *testing.B) {
	l := MustNew(
		WithConsole(false), WithFile(true), WithOutJSON(true),
		WithPath(filepath.Join(b.TempDir(), "logs", "bench")),
		WithLevel("error"), // Info 级别关闭
		WithReplaceGlobals(false),
	)
	defer l.Sync()
	ctx := ContextWithRequestID(b.Context(), "req-bench")

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		l.InfoCtx(ctx, "disabled")
	}
}

// BenchmarkInfoCtxLevelDisabledWithFields 级别关闭但调用点带字段：
// 变参切片在调用点构造并逃逸，这 1 次分配是 Go 变参 API 的固有地板，
// 门控消除的是 ctx 字段提取的那部分分配。
func BenchmarkInfoCtxLevelDisabledWithFields(b *testing.B) {
	l := MustNew(
		WithConsole(false), WithFile(true), WithOutJSON(true),
		WithPath(filepath.Join(b.TempDir(), "logs", "bench")),
		WithLevel("error"),
		WithReplaceGlobals(false),
	)
	defer l.Sync()
	ctx := ContextWithRequestID(b.Context(), "req-bench")

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		l.InfoCtx(ctx, "disabled", zap.Int("i", i))
	}
}

// BenchmarkInfoCtxLevelEnabled 级别开启时 *Ctx 的开销参照。
func BenchmarkInfoCtxLevelEnabled(b *testing.B) {
	l := MustNew(
		WithConsole(false), WithFile(true), WithOutJSON(true),
		WithPath(filepath.Join(b.TempDir(), "logs", "bench")),
		WithLevel("info"),
		WithReplaceGlobals(false),
	)
	defer l.Sync()
	ctx := ContextWithRequestID(b.Context(), "req-bench")

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		l.InfoCtx(ctx, "enabled", zap.Int("i", i))
	}
}

// newDiscardCtxLogger 构造写入 io.Discard 的实例：微基准只量 requestId 提取与
// 去重合并本身的开销，剥离磁盘 I/O（对照 BenchmarkInfoCtxLevelEnabled 端到端值）。
func newDiscardCtxLogger() *Logger {
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(io.Discard),
		zap.NewAtomicLevelAt(zapcore.DebugLevel),
	)
	z := zap.New(core)
	return &Logger{base: z, zap: z, sugar: z.Sugar()}
}

// BenchmarkInfoCtxDiscard 级别开启、写 discard core 的微基准（含内建 request_id 合并）。
func BenchmarkInfoCtxDiscard(b *testing.B) {
	l := newDiscardCtxLogger()
	ctx := ContextWithRequestID(b.Context(), "req-bench")

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		l.InfoCtx(ctx, "discard", zap.Int("i", i))
	}
}
