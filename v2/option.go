package logger

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap/zapcore"
)

// Option 配置 Logger 的构建参数。
type Option func(*Config) error

// WithConsole 设置是否输出到控制台（stdout），默认 false。
func WithConsole(b bool) Option {
	return func(c *Config) error {
		c.consoleStdout = b
		return nil
	}
}

// WithFile 设置是否输出到文件，默认 true。
func WithFile(b bool) Option {
	return func(c *Config) error {
		c.fileStdout = b
		return nil
	}
}

// WithDivision 设置日志切割方式，可选 "size"、"daily"、"both"，默认 "both"。
func WithDivision(d string) Option {
	return func(c *Config) error {
		switch rotationDivision(d) {
		case rotationSize, rotationDaily, rotationBoth:
			c.division = rotationDivision(d)
			return nil
		default:
			return fmt.Errorf("logger: invalid division %q, must be \"size\", \"daily\", or \"both\"", d)
		}
	}
}

// WithPath 设置日志文件路径前缀（最终文件名为 {path}-{level}.log），默认 "./logs/"。
func WithPath(p string) Option {
	return func(c *Config) error {
		if p == "" {
			// path 决定落盘位置，必须显式：空值静默回退相对默认路径会把日志
			// 刷进进程 cwd（零值容忍原则只适用于无文件系统副作用的配置项）。
			return errors.New("logger: path must not be empty")
		}
		c.path = p
		return nil
	}
}

// WithBasePath 设置相对日志路径（含 channel 路径）的锚定根目录：
// 构建时相对路径一律 Join 到 base 下，绝对路径原样使用；空串为不锚定（默认）。
// 用途：日志落盘位置不应依赖进程 cwd（如包测试的 cwd 是包目录，会把 logs/
// 刷进源码树），调用方传入自己的稳定写根（部署根目录、APP_ROOT 等）。
//
// 注意：本 option 仅做相对路径的解析锚定，不是安全隔离边界——
// 不校验 "../" 逃逸，也不拦截绝对路径。需要强制「日志必须落在某根目录下」
// 的边界约束时，应在应用配置层校验（如 gin-api 的 validateLogPathLocation）。
func WithBasePath(base string) Option {
	return func(c *Config) error {
		if base != "" && !filepath.IsAbs(base) {
			// 相对 base 仍随进程 cwd 漂移，违背「日志位置不依赖 cwd」的目标
			return fmt.Errorf("logger: basePath must be absolute, got %q", base)
		}
		c.basePath = base
		return nil
	}
}

// WithOutJSON 设置是否以 JSON 编码输出，默认 false（console 编码）。
func WithOutJSON(b bool) Option {
	return func(c *Config) error {
		c.outJSON = b
		return nil
	}
}

// WithDurationEncoder 设置 time.Duration 字段的编码方式，默认 zapcore.SecondsDurationEncoder。
func WithDurationEncoder(encoder zapcore.DurationEncoder) Option {
	return func(c *Config) error {
		if encoder == nil {
			return errors.New("logger: durationEncoder must not be nil")
		}
		c.durationEncoder = encoder
		return nil
	}
}

// WithCompress 设置是否压缩归档的历史日志文件，默认 true。
func WithCompress(b bool) Option {
	return func(c *Config) error {
		c.compress = b
		return nil
	}
}

// WithMaxAge 设置历史日志最大保留天数，0 表示不按时间清理，默认 7。
func WithMaxAge(days int) Option {
	return func(c *Config) error {
		if days < 0 {
			return fmt.Errorf("logger: maxAge must be >= 0, got %d", days)
		}
		c.maxAge = days
		return nil
	}
}

// WithMaxBackups 设置历史日志最大备份数量，0 表示不按数量清理，默认 50。
func WithMaxBackups(n int) Option {
	return func(c *Config) error {
		if n < 0 {
			return fmt.Errorf("logger: maxBackups must be >= 0, got %d", n)
		}
		c.maxBackups = n
		return nil
	}
}

// WithMaxSize 设置单个日志文件的最大体积（MB），默认 512。
func WithMaxSize(mb int) Option {
	return func(c *Config) error {
		if mb == 0 {
			// 零值视为未配置，保留默认值
			return nil
		}
		if mb < 0 {
			return fmt.Errorf("logger: maxSize must be > 0, got %d", mb)
		}
		c.maxSize = mb
		return nil
	}
}

// WithLevel 设置日志级别，支持 debug/info/warn/error/dpanic/panic/fatal，默认 "info"。
func WithLevel(l string) Option {
	return func(c *Config) error {
		if l == "" {
			// 零值视为未配置，保留默认级别
			return nil
		}
		if _, ok := levelMap[l]; !ok {
			return fmt.Errorf("logger: invalid level %q", l)
		}
		c.level = l
		return nil
	}
}

// WithMessager 设置外部消息推送实现，H 系列方法写日志后会通过它异步推送消息。
func WithMessager(m Messager) Option {
	return func(c *Config) error {
		c.messager = m
		return nil
	}
}

// WithMessagerQueueSize 设置异步推送队列大小，队列满时丢弃推送（可用 DroppedMessages 监控），默认 1024。
func WithMessagerQueueSize(size int) Option {
	return func(c *Config) error {
		if size <= 0 {
			return fmt.Errorf("logger: messagerQueueSize must be > 0, got %d", size)
		}
		c.messagerQueueSize = size
		return nil
	}
}

// WithContextFields 注册从 context.Context 提取日志字段的函数，供 *Ctx 系列方法自动合并 trace_id 等链路信息。
func WithContextFields(fn ContextFieldsFunc) Option {
	return func(c *Config) error {
		c.contextFields = fn
		return nil
	}
}

// WithBuffered 启用文件写入缓冲（BufferedWriteSyncer）。
// 缓冲区大小默认 256KB，刷写间隔默认 30 秒。
// 启用后可显著减少系统调用次数，提升高吞吐场景下的写入性能，
// 但进程异常退出时可能丢失缓冲区中未刷写的日志。
func WithBuffered(enabled bool) Option {
	return func(c *Config) error {
		c.buffered = enabled
		return nil
	}
}

// WithReplaceGlobals 控制构建时是否把该实例安装为 zap 全局 logger（zap.ReplaceGlobals），
// 默认 true（与既有行为一致）。进程内的辅助实例（如独立的 access 日志实例、
// 懒创建的兜底实例）应传 false，避免抢占全局后再靠 Undo 撤销。
func WithReplaceGlobals(enabled bool) Option {
	return func(c *Config) error {
		c.replaceGlobals = enabled
		return nil
	}
}

// WithBufferSize 设置缓冲区大小（字节），默认 256KB（256*1024）。
// 仅在 WithBuffered(true) 时生效。
func WithBufferSize(size int) Option {
	return func(c *Config) error {
		if size <= 0 {
			return fmt.Errorf("logger: bufferSize must be > 0, got %d", size)
		}
		c.bufferSize = size
		return nil
	}
}

// WithFlushInterval 设置缓冲区自动刷写间隔，默认 30 秒。
// 仅在 WithBuffered(true) 时生效。
func WithFlushInterval(d time.Duration) Option {
	return func(c *Config) error {
		if d <= 0 {
			return fmt.Errorf("logger: flushInterval must be > 0, got %v", d)
		}
		c.flushInterval = d
		return nil
	}
}

// WithSampling 启用日志采样，防止高频日志打爆磁盘 / 拖垮下游。
//
// 语义（zap 原生 NewSamplerWithOptions，tick 固定 1 秒）：在每个 1 秒窗口内，
// 对相同 level+message 的日志，先放行 first 条，之后每 thereafter 条放行一条，其余丢弃。
//   - first <= 0 时回退为 1（至少放行第一条）。
//   - thereafter == 0 表示首批之后全部丢弃。
//
// 默认不启用（不调用本 option 即不采样，所有日志原样输出）。channel 继承相同采样配置。
//
// 注意：采样按 message 文本去重，因此高频日志应使用**稳定的 message + 结构化字段**，
// 而不是把变量拼进 message（拼进 message 会让每条都不同，采样失效）。
func WithSampling(first, thereafter int) Option {
	return func(c *Config) error {
		if thereafter < 0 {
			return fmt.Errorf("logger: sampling thereafter must be >= 0, got %d", thereafter)
		}
		if first <= 0 {
			first = 1
		}
		c.samplingFirst = first
		c.samplingThereafter = thereafter
		return nil
	}
}

// WithRedactKeys 对指定字段名做脱敏：凡 Key 命中的结构化字段，其值统一替换为 "[REDACTED]"。
//
// 典型用途：屏蔽 password / token / authorization / id_card / phone 等敏感字段，避免落盘合规风险。
// 匹配区分大小写，按字段 Key 精确匹配。channel 继承相同脱敏规则。
//
// 不调用本 option 时零开销（不包装 core）。启用后每条日志会按字段数做一次集合查找，开销与字段数成正比。
//
// 仅作用于结构化字段（zap.String("password", x) 这类）；拼进 message 文本的敏感信息不受影响——
// 这也是推荐用结构化字段而非字符串拼接的又一理由。
func WithRedactKeys(keys ...string) Option {
	return func(c *Config) error {
		if redactor := newFieldRedactor(keys); redactor != nil {
			c.fieldRedactor = redactor
		}
		return nil
	}
}

func newFieldRedactor(keys []string) func([]zapcore.Field) []zapcore.Field {
	if len(keys) == 0 {
		return nil
	}

	set := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		if k != "" {
			set[k] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}

	return func(fields []zapcore.Field) []zapcore.Field {
		return redactFields(fields, set)
	}
}

func redactFields(fields []zapcore.Field, keys map[string]struct{}) []zapcore.Field {
	for i := range fields {
		if _, ok := keys[fields[i].Key]; ok {
			return redactFieldsFrom(fields, keys, i)
		}
	}
	return fields
}

func redactFieldsFrom(fields []zapcore.Field, keys map[string]struct{}, first int) []zapcore.Field {
	redacted := copyFields(fields)
	for i := first; i < len(redacted); i++ {
		if _, ok := keys[redacted[i].Key]; ok {
			redacted[i] = zapcore.Field{Key: redacted[i].Key, Type: zapcore.StringType, String: redactedValue}
		}
	}
	return redacted
}

// ChannelOption 配置单个 channel 路由。
type ChannelOption func(*channelConfig) error

// WithChannel 注册一个写入独立文件的 channel 路由；channel 继承全局切割与编码配置。
func WithChannel(name string, opts ...ChannelOption) Option {
	return func(c *Config) error {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return errors.New("logger: channel name must not be empty")
		}

		cfg := &channelConfig{
			duplicateToDefault: true,
		}
		for _, opt := range opts {
			if err := opt(cfg); err != nil {
				return err
			}
		}
		if cfg.path == "" {
			return fmt.Errorf("logger: channel %q path must not be empty", trimmed)
		}

		if c.channels == nil {
			c.channels = make(map[string]*channelConfig)
		}
		c.channels[trimmed] = cfg

		return nil
	}
}

// WithChannelPath 设置 channel 日志文件的路径前缀（必填）。
func WithChannelPath(path string) ChannelOption {
	return func(c *channelConfig) error {
		if path == "" {
			return errors.New("logger: channel path must not be empty")
		}
		c.path = path
		return nil
	}
}

// WithChannelDuplicateToDefault 设置 channel 日志是否同时写入默认输出，默认 true。
func WithChannelDuplicateToDefault(enabled bool) ChannelOption {
	return func(c *channelConfig) error {
		c.duplicateToDefault = enabled
		return nil
	}
}
