package logger

import (
	"context"

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

type Config struct {
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

func defaultConfig() *Config {
	return &Config{
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
}
