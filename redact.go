package logger

import "go.uber.org/zap/zapcore"

// redactedValue 是脱敏字段统一替换后的占位值。
const redactedValue = "[REDACTED]"

// redactCore 包装一个 zapcore.Core，在写入前对字段执行脱敏函数。
//
// 包装顺序（见 buildCore）：leveled core 之外、sampler 之内。Logger.With() 预绑定的
// 字段经 sampler.With 原样透传到这里的 With，同样过脱敏；而 sampler 必须留在最外层——
// 本装饰器的 Check 把自身 AddCore 进 CheckedEntry、不会调用内层 Check，若把 sampler
// 包在内层，其 Check 里的采样判定将被绕过（曾因此导致采样+脱敏同开时采样静默失效）。
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
