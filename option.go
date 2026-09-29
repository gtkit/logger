package logger

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap/zapcore"
)

// Option configures the package-level logger.
type Option func(*logConfig) error

// WithConsole 设置是否输出到控制台（stdout），默认 false。
func WithConsole(b bool) Option {
	return func(c *logConfig) error {
		c.consoleStdout = b
		return nil
	}
}

// WithFile 设置是否输出到文件，默认 true。
func WithFile(b bool) Option {
	return func(c *logConfig) error {
		c.fileStdout = b
		return nil
	}
}

// WithDivision 设置日志切割方式，可选 "size"、"daily"、"both"，默认 "both"。
func WithDivision(d string) Option {
	return func(c *logConfig) error {
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
	return func(c *logConfig) error {
		if p == "" {
			return errors.New("logger: path must not be empty")
		}
		c.path = p
		return nil
	}
}

// WithOutJSON 设置是否以 JSON 编码输出，默认 false（console 编码）。
func WithOutJSON(b bool) Option {
	return func(c *logConfig) error {
		c.outJSON = b
		return nil
	}
}

// WithDurationEncoder 设置 time.Duration 字段的编码方式，默认 zapcore.SecondsDurationEncoder。
func WithDurationEncoder(encoder zapcore.DurationEncoder) Option {
	return func(c *logConfig) error {
		if encoder == nil {
			return errors.New("logger: durationEncoder must not be nil")
		}
		c.durationEncoder = encoder
		return nil
	}
}

// WithCompress 设置是否压缩归档的历史日志文件，默认 true。
func WithCompress(b bool) Option {
	return func(c *logConfig) error {
		c.compress = b
		return nil
	}
}

// WithMaxAge 设置历史日志最大保留天数，0 表示不按时间清理，默认 7。
func WithMaxAge(days int) Option {
	return func(c *logConfig) error {
		if days < 0 {
			return fmt.Errorf("logger: maxAge must be >= 0, got %d", days)
		}
		c.maxAge = days
		return nil
	}
}

// WithMaxBackups 设置历史日志最大备份数量，0 表示不按数量清理，默认 50。
func WithMaxBackups(n int) Option {
	return func(c *logConfig) error {
		if n < 0 {
			return fmt.Errorf("logger: maxBackups must be >= 0, got %d", n)
		}
		c.maxBackups = n
		return nil
	}
}

// WithMaxSize 设置单个日志文件的最大体积（MB），默认 512。
func WithMaxSize(mb int) Option {
	return func(c *logConfig) error {
		if mb <= 0 {
			return fmt.Errorf("logger: maxSize must be > 0, got %d", mb)
		}
		c.maxSize = mb
		return nil
	}
}

// WithLevel 设置日志级别，支持 debug/info/warn/error/dpanic/panic/fatal，默认 "info"。
func WithLevel(l string) Option {
	return func(c *logConfig) error {
		if _, ok := levelMap[l]; !ok {
			return fmt.Errorf("logger: invalid level %q", l)
		}
		c.level = l
		return nil
	}
}

// WithMessager 设置外部消息推送实现，H 系列方法写日志后会通过它异步推送消息。
func WithMessager(m Messager) Option {
	return func(c *logConfig) error {
		c.messager = m
		return nil
	}
}

// WithMessagerQueueSize 设置异步推送队列大小，队列满时丢弃推送（可用 DroppedMessages 监控），默认 1024。
func WithMessagerQueueSize(size int) Option {
	return func(c *logConfig) error {
		if size <= 0 {
			return fmt.Errorf("logger: messagerQueueSize must be > 0, got %d", size)
		}
		c.messagerQueueSize = size
		return nil
	}
}

// WithMessagerDrainTimeout 设置 Sync 排空异步推送队列的最长等待时间，默认 5 秒。
// 超时后 Sync 不再等待：向 stderr 告警，队列中尚未执行的推送计入 DroppedMessages，
// 后台推送协程继续消费直到外部 Messager 的调用返回。用于防止外部推送挂起时进程无法退出。
func WithMessagerDrainTimeout(d time.Duration) Option {
	return func(c *logConfig) error {
		if d <= 0 {
			return fmt.Errorf("logger: messagerDrainTimeout must be > 0, got %v", d)
		}
		c.messagerDrainTimeout = d
		return nil
	}
}

// WithStacktraceLevel 设置附带 stacktrace 的最低日志级别，支持 debug/info/warn/error/dpanic/panic/fatal，
// 默认 "error"；空串视为未配置。高频 Error 日志场景可提升到 "dpanic" 或 "fatal" 以省去采栈开销。
func WithStacktraceLevel(l string) Option {
	return func(c *logConfig) error {
		if l == "" {
			return nil
		}
		lvl, ok := levelMap[l]
		if !ok {
			return fmt.Errorf("logger: invalid stacktrace level %q", l)
		}
		c.stacktraceLevel = lvl
		return nil
	}
}

// WithContextFields 注册从 context.Context 提取日志字段的函数，供 *Ctx 系列方法自动合并 trace_id 等链路信息。
func WithContextFields(fn ContextFieldsFunc) Option {
	return func(c *logConfig) error {
		c.contextFields = fn
		return nil
	}
}

// WithBuffered 启用文件写入缓冲（BufferedWriteSyncer）。
// 缓冲区大小默认 256KB，刷写间隔默认 30 秒。
// 启用后可显著减少系统调用次数，提升高吞吐场景下的写入性能，
// 但进程异常退出时可能丢失缓冲区中未刷写的日志。
func WithBuffered(enabled bool) Option {
	return func(c *logConfig) error {
		c.buffered = enabled
		return nil
	}
}

// WithBufferSize 设置缓冲区大小（字节），默认 256KB（256*1024）。
// 仅在 WithBuffered(true) 时生效。
func WithBufferSize(size int) Option {
	return func(c *logConfig) error {
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
	return func(c *logConfig) error {
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
	return func(c *logConfig) error {
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
// 多次调用取并集（基础敏感集 + 业务追加集可分开传入），空串 key 忽略。
//
// 典型用途：屏蔽 password / token / authorization / id_card / phone 等敏感字段，避免落盘合规风险。
// 匹配区分大小写，按字段 Key 精确匹配。channel 继承相同脱敏规则。
//
// 不调用本 option 时零开销（不包装 core）。启用后每条日志会按字段数做一次集合查找，开销与字段数成正比。
//
// 仅作用于结构化字段（zap.String("password", x) 这类）；拼进 message 文本的敏感信息不受影响——
// 这也是推荐用结构化字段而非字符串拼接的又一理由。
func WithRedactKeys(keys ...string) Option {
	return func(c *logConfig) error {
		for _, k := range keys {
			if k == "" {
				continue
			}
			if c.redactKeys == nil {
				c.redactKeys = make(map[string]struct{}, len(keys))
			}
			c.redactKeys[k] = struct{}{}
		}
		return nil
	}
}

func newFieldRedactor(set map[string]struct{}) func([]zapcore.Field) []zapcore.Field {
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

// ChannelOption configures a single channel route.
type ChannelOption func(*channelConfig) error

// WithChannel registers a dedicated file route for a channel.
// Channel files inherit the global rotation and encoding settings.
func WithChannel(name string, opts ...ChannelOption) Option {
	return func(c *logConfig) error {
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

		if _, dup := c.channels[trimmed]; dup {
			return fmt.Errorf("logger: channel %q registered more than once", trimmed)
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
