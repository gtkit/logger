package logger

import "go.uber.org/zap/zapcore"

// redactedValue 是脱敏字段统一替换后的占位值。
const redactedValue = "[REDACTED]"

// redactCore 包装一个 zapcore.Core，在写入前对字段执行脱敏函数。
//
// 包在最外层（采样/leveled core 之外），确保通过 Logger.With() 预绑定的字段
// 同样经过脱敏——zap 在 With 时把字段下推到 core.With，这里一并处理。
type redactCore struct {
	zapcore.Core
	redact func([]zapcore.Field) []zapcore.Field
}

func newRedactCore(inner zapcore.Core, redact func([]zapcore.Field) []zapcore.Field) zapcore.Core {
	return &redactCore{Core: inner, redact: redact}
}

func (c *redactCore) With(fields []zapcore.Field) zapcore.Core {
	return &redactCore{Core: c.Core.With(c.redact(fields)), redact: c.redact}
}

func (c *redactCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

func (c *redactCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	//nolint:wrapcheck // 透明装饰器：必须原样透传底层 core 的错误，包裹会破坏 zap 的错误处理语义。
	return c.Core.Write(ent, c.redact(fields))
}
