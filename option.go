package logger

import "fmt"

// Option 配置日志的选项函数.
type Option func(*logConfig) error

// WithConsole 设置是否输出到控制台.
func WithConsole(b bool) Option {
	return func(c *logConfig) error {
		c.consoleStdout = b
		return nil
	}
}

// WithFile 设置是否输出到文件.
func WithFile(b bool) Option {
	return func(c *logConfig) error {
		c.fileStdout = b
		return nil
	}
}

// WithDivision 设置日志切割方式: "size" 按大小, "daily" 按天.
func WithDivision(d string) Option {
	return func(c *logConfig) error {
		if d != "size" && d != "daily" {
			return fmt.Errorf("logger: invalid division %q, must be \"size\" or \"daily\"", d)
		}
		c.division = d
		return nil
	}
}

// WithPath 设置日志文件路径前缀.
func WithPath(p string) Option {
	return func(c *logConfig) error {
		if p == "" {
			return fmt.Errorf("logger: path must not be empty")
		}
		c.path = p
		return nil
	}
}

// WithOutJSON 设置是否输出为 JSON 格式.
func WithOutJSON(b bool) Option {
	return func(c *logConfig) error {
		c.outJSON = b
		return nil
	}
}

// WithCompress 设置是否压缩归档日志文件.
func WithCompress(b bool) Option {
	return func(c *logConfig) error {
		c.compress = b
		return nil
	}
}

// WithMaxAge 设置日志文件最大保存天数.
func WithMaxAge(days int) Option {
	return func(c *logConfig) error {
		if days <= 0 {
			return fmt.Errorf("logger: maxAge must be > 0, got %d", days)
		}
		c.maxAge = days
		return nil
	}
}

// WithMaxBackups 设置日志文件最大备份数量.
func WithMaxBackups(n int) Option {
	return func(c *logConfig) error {
		if n < 0 {
			return fmt.Errorf("logger: maxBackups must be >= 0, got %d", n)
		}
		c.maxBackups = n
		return nil
	}
}

// WithMaxSize 设置单个日志文件最大大小 (MB).
func WithMaxSize(mb int) Option {
	return func(c *logConfig) error {
		if mb <= 0 {
			return fmt.Errorf("logger: maxSize must be > 0, got %d", mb)
		}
		c.maxSize = mb
		return nil
	}
}

// WithLevel 设置日志级别: debug, info, warn, error, dpanic, panic, fatal.
func WithLevel(l string) Option {
	return func(c *logConfig) error {
		if _, ok := levelMap[l]; !ok {
			return fmt.Errorf("logger: invalid level %q", l)
		}
		c.level = l
		return nil
	}
}

// WithMessager 设置消息推送 Hook（飞书/钉钉/企微等通知）.
func WithMessager(m Messager) Option {
	return func(c *logConfig) error {
		c.messager = m
		return nil
	}
}
