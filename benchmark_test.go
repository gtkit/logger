package logger

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func BenchmarkInfo(b *testing.B) {
	restore := installBenchmarkState(b)
	defer restore()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		Info("bench info", zap.String("key", "value"))
	}
}

// ============================================================
// 新增方法的 benchmark——量化 Ctx 字段注入与条件日志的开销
// 用于对照 BenchmarkInfo 基线，验证新增 API 没有引入热路径退化。
// ============================================================

// BenchmarkInfoCtx_NoContextFields 量化 InfoCtx 在未配置 WithContextFields 时的开销，
// 对照 BenchmarkInfo——理想情况下两者应当几乎相等（多一次 nil check）。
func BenchmarkInfoCtx_NoContextFields(b *testing.B) {
	restore := installBenchmarkState(b)
	defer restore()

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		InfoCtx(ctx, "bench info ctx", zap.String("key", "value"))
	}
}

// BenchmarkInfowCtx_NoContextFields 量化 Sugar 风格 InfowCtx 在未配置 contextFields 时的开销。
// 与 BenchmarkInfo 对比可看出 Sugar 路径相对 Structured 路径的相对成本。
func BenchmarkInfowCtx_NoContextFields(b *testing.B) {
	restore := installBenchmarkState(b)
	defer restore()

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		InfowCtx(ctx, "bench infow ctx", "key", "value")
	}
}

// BenchmarkLogIfNil 量化 LogIf(nil) 的"无副作用"路径——理想情况下应当是单次 nil check 的成本。
func BenchmarkLogIfNil(b *testing.B) {
	restore := installBenchmarkState(b)
	defer restore()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		LogIf(nil)
	}
}

// BenchmarkLogIfNonNil 量化 LogIf(err) 在 err != nil 时的实际 Error() 写日志成本。
func BenchmarkLogIfNonNil(b *testing.B) {
	restore := installBenchmarkState(b)
	defer restore()

	err := errors.New("bench error")
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		LogIf(err)
	}
}

func BenchmarkChannelConfiguredLookupAndInfo(b *testing.B) {
	restore := installBenchmarkState(b)
	defer restore()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		Channel("order").Info("bench channel", zap.String("key", "value"))
	}
}

func BenchmarkChannelConfiguredReuse(b *testing.B) {
	restore := installBenchmarkState(b)
	defer restore()

	orderLog := Channel("order")

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		orderLog.Info("bench channel", zap.String("key", "value"))
	}
}

func BenchmarkChannelUnconfiguredReuse(b *testing.B) {
	restore := installBenchmarkState(b)
	defer restore()

	paymentLog := Channel("payment")

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		paymentLog.Info("bench channel", zap.String("key", "value"))
	}
}

func installBenchmarkState(b *testing.B) func() {
	b.Helper()

	root := zap.NewNop()
	state := newLoggerState(
		root,
		map[string]*zap.Logger{
			"order": root.With(zap.String("channel", "order")),
		},
		nil,
		nil,
		nil,
		nil,
		zap.NewAtomicLevelAt(zapcore.DebugLevel),
	)

	globalMu.Lock()
	previous := currentState.Load()
	currentState.Store(state)
	globalMu.Unlock()

	return func() {
		globalMu.Lock()
		currentState.Store(previous)
		globalMu.Unlock()
	}
}
