package logger

import (
	"context"
	"time"

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
)

var levelMap = map[string]zapcore.Level{
	"debug":  zapcore.DebugLevel,
	"info":   zapcore.InfoLevel,
	"warn":   zapcore.WarnLevel,
	"error":  zapcore.ErrorLevel,
	"dpanic": zapcore.DPanicLevel,
	"panic":  zapcore.PanicLevel,
	"fatal":  zapcore.FatalLevel,
}

// Config 是 v2 Logger 的内部配置容器。
//
// 字段全部不导出是有意为之——v2 采用 Functional Options 模式（New(opts ...Option)），
// Config 不应该被外部直接构造或反序列化（YAML/JSON 装配请通过 Option 函数）。
//
// Config 类型本身暴露在 godoc 中是为了让 IDE / 文档工具能定位选项作用对象，
// 而非作为公开 API 接受用户构造的实例。
//
// 反模式（禁止）：
//
//	cfg := &logger.Config{ ... } // ❌ 字段不导出，无法这样构造
//	_, _ = logger.New(cfg)        // ❌ New 不接受 Config，只接受 Option
//
// 正确用法：
//
//	l, err := logger.New(logger.WithPath("/var/log/app"), logger.WithOutJSON(true))
type Config struct {
	consoleStdout     bool
	fileStdout        bool
	outJSON           bool
	durationEncoder   zapcore.DurationEncoder
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
	buffered          bool
	bufferSize        int
	flushInterval     time.Duration
}

type channelConfig struct {
	path               string
	duplicateToDefault bool
}

func defaultConfig() *Config {
	return &Config{
		consoleStdout:     false,
		fileStdout:        true,
		outJSON:           false,
		durationEncoder:   zapcore.SecondsDurationEncoder,
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
}
