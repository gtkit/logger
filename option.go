package logger

import (
	"errors"
	"fmt"
	"strings"
)

// Option configures the package-level logger.
type Option func(*logConfig) error

func WithConsole(b bool) Option {
	return func(c *logConfig) error {
		c.consoleStdout = b
		return nil
	}
}

func WithFile(b bool) Option {
	return func(c *logConfig) error {
		c.fileStdout = b
		return nil
	}
}

func WithDivision(d string) Option {
	return func(c *logConfig) error {
		if d != "size" && d != "daily" {
			return fmt.Errorf("logger: invalid division %q, must be \"size\" or \"daily\"", d)
		}
		c.division = d
		return nil
	}
}

func WithPath(p string) Option {
	return func(c *logConfig) error {
		if p == "" {
			return errors.New("logger: path must not be empty")
		}
		c.path = p
		return nil
	}
}

func WithOutJSON(b bool) Option {
	return func(c *logConfig) error {
		c.outJSON = b
		return nil
	}
}

func WithCompress(b bool) Option {
	return func(c *logConfig) error {
		c.compress = b
		return nil
	}
}

func WithMaxAge(days int) Option {
	return func(c *logConfig) error {
		if days <= 0 {
			return fmt.Errorf("logger: maxAge must be > 0, got %d", days)
		}
		c.maxAge = days
		return nil
	}
}

func WithMaxBackups(n int) Option {
	return func(c *logConfig) error {
		if n < 0 {
			return fmt.Errorf("logger: maxBackups must be >= 0, got %d", n)
		}
		c.maxBackups = n
		return nil
	}
}

func WithMaxSize(mb int) Option {
	return func(c *logConfig) error {
		if mb <= 0 {
			return fmt.Errorf("logger: maxSize must be > 0, got %d", mb)
		}
		c.maxSize = mb
		return nil
	}
}

func WithLevel(l string) Option {
	return func(c *logConfig) error {
		if _, ok := levelMap[l]; !ok {
			return fmt.Errorf("logger: invalid level %q", l)
		}
		c.level = l
		return nil
	}
}

func WithMessager(m Messager) Option {
	return func(c *logConfig) error {
		c.messager = m
		return nil
	}
}

func WithMessagerQueueSize(size int) Option {
	return func(c *logConfig) error {
		if size <= 0 {
			return fmt.Errorf("logger: messagerQueueSize must be > 0, got %d", size)
		}
		c.messagerQueueSize = size
		return nil
	}
}

func WithContextFields(fn ContextFieldsFunc) Option {
	return func(c *logConfig) error {
		c.contextFields = fn
		return nil
	}
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

		if c.channels == nil {
			c.channels = make(map[string]*channelConfig)
		}
		c.channels[trimmed] = cfg

		return nil
	}
}

func WithChannelPath(path string) ChannelOption {
	return func(c *channelConfig) error {
		if path == "" {
			return errors.New("logger: channel path must not be empty")
		}
		c.path = path
		return nil
	}
}

func WithChannelDuplicateToDefault(enabled bool) ChannelOption {
	return func(c *channelConfig) error {
		c.duplicateToDefault = enabled
		return nil
	}
}
