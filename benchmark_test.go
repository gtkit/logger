package logger

import (
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
