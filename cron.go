// @Author 2023/11/20 17:31:00
package logger

import (
	"strings"
	"time"
)

type Cronlog struct {
}

func CronLog() *Cronlog {
	return &Cronlog{}
}

func (t *Cronlog) Info(msg string, keysAndValues ...any) {
	keysAndValues = formatTimes(keysAndValues)
	Infof(formatString("[* 定时任务 INFO]", len(keysAndValues)), append([]any{msg}, keysAndValues...)...)
}
func (t *Cronlog) Error(err error, msg string, keysAndValues ...any) {
	keysAndValues = formatTimes(keysAndValues)
	Infof(formatString("[*定时任务 ERROR]", len(keysAndValues)+2), append([]any{msg, "error", err}, keysAndValues...)...)
}

// formatString returns a logfmt-like format string for the number of
// key/values.
func formatString(prefix string, numKeysAndValues int) string {
	var sb strings.Builder
	sb.WriteString(prefix)
	sb.WriteString(": %s")
	if numKeysAndValues > 0 {
		sb.WriteString(", ")
	}
	for i := 0; i < numKeysAndValues/2; i++ {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("%v=%v")
	}
	return sb.String()
}

func formatTimes(keysAndValues []any) []any {
	var formattedArgs []any
	for _, arg := range keysAndValues {
		if t, ok := arg.(time.Time); ok {
			arg = t.Format(time.DateTime)
		}
		formattedArgs = append(formattedArgs, arg)
	}
	return formattedArgs
}
