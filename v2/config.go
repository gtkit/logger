package logger

import "go.uber.org/zap/zapcore"

const (
	defaultPath       = "./logs/"
	defaultMaxSize    = 512 // MB
	defaultMaxAge     = 7  // 天
	defaultMaxBackups = 50
)

// levelMap 日志级别映射.
var levelMap = map[string]zapcore.Level{
	"debug":  zapcore.DebugLevel,
	"info":   zapcore.InfoLevel,
	"warn":   zapcore.WarnLevel,
	"error":  zapcore.ErrorLevel,
	"dpanic": zapcore.DPanicLevel,
	"panic":  zapcore.PanicLevel,
	"fatal":  zapcore.FatalLevel,
}

// Config 日志配置，通过 Option 函数设置.
type Config struct {
	consoleStdout bool   // 是否输出到控制台
	fileStdout    bool   // 是否输出到文件
	outJSON       bool   // 是否输出为 JSON 格式
	division      string // 切割方式: "size" 或 "daily"
	path          string // 日志文件路径前缀
	compress      bool   // 是否压缩归档文件
	maxAge        int    // 最大保存天数
	maxBackups    int    // 最大备份数量
	maxSize       int    // 单文件最大大小 (MB)
	level         string // 日志级别
	messager      Messager
}

func defaultConfig() *Config {
	return &Config{
		consoleStdout: false,
		fileStdout:    true,
		outJSON:       false,
		division:      "size",
		path:          defaultPath,
		compress:      true,
		maxAge:        defaultMaxAge,
		maxBackups:    defaultMaxBackups,
		maxSize:       defaultMaxSize,
		level:         "info",
	}
}
