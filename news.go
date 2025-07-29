package logger

import (
	"fmt"
)

var newser Newser

type Newser interface {
	Hook(args ...any)
	HookF(template string, args ...any)
	HookWithURL(url string, args ...any)
	HookWithURLF(url, template string, args ...any)
}

// Message format with Sprint, Sprintf, or neither.
func Message(template string, fmtArgs []any) string {
	if len(fmtArgs) == 0 {
		return template
	}

	if template != "" {
		return fmt.Sprintf(template, fmtArgs...)
	}

	if len(fmtArgs) == 1 {
		if str, ok := fmtArgs[0].(string); ok {
			return str
		}
	}
	return fmt.Sprint(fmtArgs...)
}
