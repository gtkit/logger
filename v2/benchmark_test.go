package logger

import (
	"testing"

	"go.uber.org/zap"
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
