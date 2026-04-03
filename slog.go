package logger

import (
	"context"
	"log/slog"
	"runtime"
	"slices"
	"strconv"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// SlogHandler 返回一个 slog.Handler，将 slog 日志写入当前全局 logger 的 zap core。
// 用于统一第三方库的 slog 输出到同一个日志管道。
//
//	slog.SetDefault(slog.New(logger.SlogHandler()))
func SlogHandler() slog.Handler {
	return &zapSlogHandler{getCore: currentCore, addSource: true}
}

func currentCore() zapcore.Core {
	if state := snapshotLoggerState(); state != nil {
		return state.root.Core()
	}
	return zapcore.NewNopCore()
}

type zapSlogHandler struct {
	getCore   func() zapcore.Core
	addSource bool
	attrs     []zap.Field
	group     string
}

func (h *zapSlogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return h.getCore().Enabled(slogToZapLevel(level))
}

func (h *zapSlogHandler) Handle(_ context.Context, record slog.Record) error {
	core := h.getCore()
	fields := make([]zap.Field, 0, len(h.attrs)+record.NumAttrs()+1)
	fields = append(fields, h.attrs...)

	record.Attrs(func(a slog.Attr) bool {
		fields = append(fields, slogAttrToZapField(h.group, a))
		return true
	})

	if h.addSource && record.PC != 0 {
		frame, _ := runtime.CallersFrames([]uintptr{record.PC}).Next()
		if frame.File != "" {
			fields = append(fields, zap.String("source", frame.File+":"+strconv.Itoa(frame.Line)))
		}
	}

	if ce := core.Check(zapcore.Entry{
		Level:   slogToZapLevel(record.Level),
		Time:    record.Time,
		Message: record.Message,
	}, nil); ce != nil {
		ce.Write(fields...)
	}

	return nil
}

func (h *zapSlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	fields := make([]zap.Field, 0, len(attrs))
	for _, a := range attrs {
		fields = append(fields, slogAttrToZapField(h.group, a))
	}

	return &zapSlogHandler{
		getCore:   h.getCore,
		addSource: h.addSource,
		attrs:     append(slices.Clone(h.attrs), fields...),
		group:     h.group,
	}
}

func (h *zapSlogHandler) WithGroup(name string) slog.Handler {
	newGroup := name
	if h.group != "" {
		newGroup = h.group + "." + name
	}

	return &zapSlogHandler{
		getCore:   h.getCore,
		addSource: h.addSource,
		attrs:     slices.Clone(h.attrs),
		group:     newGroup,
	}
}

func slogToZapLevel(level slog.Level) zapcore.Level {
	switch {
	case level >= slog.LevelError:
		return zapcore.ErrorLevel
	case level >= slog.LevelWarn:
		return zapcore.WarnLevel
	case level >= slog.LevelInfo:
		return zapcore.InfoLevel
	default:
		return zapcore.DebugLevel
	}
}

func slogAttrToZapField(group string, attr slog.Attr) zap.Field {
	key := attr.Key
	if group != "" {
		key = group + "." + key
	}

	val := attr.Value.Resolve()

	switch val.Kind() {
	case slog.KindString:
		return zap.String(key, val.String())
	case slog.KindInt64:
		return zap.Int64(key, val.Int64())
	case slog.KindUint64:
		return zap.Uint64(key, val.Uint64())
	case slog.KindFloat64:
		return zap.Float64(key, val.Float64())
	case slog.KindBool:
		return zap.Bool(key, val.Bool())
	case slog.KindDuration:
		return zap.Duration(key, val.Duration())
	case slog.KindTime:
		return zap.Time(key, val.Time())
	case slog.KindGroup:
		attrs := val.Group()
		if len(attrs) == 0 {
			return zap.Skip()
		}
		return zap.Object(key, zapcore.ObjectMarshalerFunc(func(enc zapcore.ObjectEncoder) error {
			for _, a := range attrs {
				slogAttrToZapField("", a).AddTo(enc)
			}
			return nil
		}))
	default:
		return zap.Any(key, val.Any())
	}
}

