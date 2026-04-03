package logger

import (
	"strings"
	"time"
)

// ============================================================
// Cron 适配器 - 适配 robfig/cron 的日志接口
// ============================================================

// CronAdapter 适配 robfig/cron 的日志接口.
type CronAdapter struct{}

// NewCronAdapter 创建 Cron 日志适配器.
func NewCronAdapter() *CronAdapter {
	return &CronAdapter{}
}

// Info implements cron.Logger.
func (a *CronAdapter) Info(msg string, keysAndValues ...any) {
	keysAndValues = cronFormatTimes(keysAndValues)
	Infof(
		cronFormatString("[cron] INFO", len(keysAndValues)),
		append([]any{msg}, keysAndValues...)...,
	)
}

// Error implements cron.Logger.
func (a *CronAdapter) Error(err error, msg string, keysAndValues ...any) {
	keysAndValues = cronFormatTimes(keysAndValues)
	Errorf(
		cronFormatString("[cron] ERROR", len(keysAndValues)+2),
		append([]any{msg, "error", err}, keysAndValues...)...,
	)
}

func cronFormatString(prefix string, numKeysAndValues int) string {
	var sb strings.Builder

	sb.WriteString(prefix)
	sb.WriteString(": %s")

	if numKeysAndValues > 0 {
		sb.WriteString(", ")
	}

	for i := range numKeysAndValues / 2 {
		if i > 0 {
			sb.WriteString(", ")
		}

		sb.WriteString("%v=%v")
	}

	return sb.String()
}

func cronFormatTimes(keysAndValues []any) []any {
	formatted := make([]any, 0, len(keysAndValues))
	for _, arg := range keysAndValues {
		if t, ok := arg.(time.Time); ok {
			arg = t.Format(time.DateTime)
		}

		formatted = append(formatted, arg)
	}

	return formatted
}

// ============================================================
// Migrate 适配器 - 适配 gtkit/migrate/v2 的 migration.Logger 接口
// ============================================================

// MigrateAdapter 适配 gtkit/migrate/v2 的 migration.Logger 接口.
// 通过 SugaredLogger 的 key-value 方式输出结构化日志.
type MigrateAdapter struct{}

// NewMigrateAdapter 创建 Migrate 日志适配器.
func NewMigrateAdapter() *MigrateAdapter {
	return &MigrateAdapter{}
}

// Info implements migration.Logger.
func (a *MigrateAdapter) Info(msg string, keysAndValues ...any) {
	Infow("[migrate] "+msg, keysAndValues...)
}

// Warn implements migration.Logger.
func (a *MigrateAdapter) Warn(msg string, keysAndValues ...any) {
	Warnw("[migrate] "+msg, keysAndValues...)
}

// Error implements migration.Logger.
func (a *MigrateAdapter) Error(msg string, keysAndValues ...any) {
	Errorw("[migrate] "+msg, keysAndValues...)
}

// ============================================================
// ES 适配器 - 适配 Elasticsearch client 的日志接口
// ============================================================

// ESAdapter 适配 Elasticsearch client 的日志接口.
type ESAdapter struct{}

// NewESAdapter 创建 ES 日志适配器.
func NewESAdapter() *ESAdapter {
	return &ESAdapter{}
}

// Printf implements estransport.Logger.
func (a *ESAdapter) Printf(format string, v ...any) {
	Infof("[es] "+format, v...)
}

// ============================================================
// Resty 适配器 - 适配 go-resty 的日志接口
// ============================================================

// RestyAdapter 适配 go-resty 的日志接口.
type RestyAdapter struct{}

// NewRestyAdapter 创建 Resty 日志适配器.
func NewRestyAdapter() *RestyAdapter {
	return &RestyAdapter{}
}

// Errorf implements resty.Logger.
func (a *RestyAdapter) Errorf(format string, v ...any) {
	Errorf("[resty] "+format, v...)
}

// Warnf implements resty.Logger.
func (a *RestyAdapter) Warnf(format string, v ...any) {
	Warnf("[resty] "+format, v...)
}

// Debugf implements resty.Logger.
func (a *RestyAdapter) Debugf(format string, v ...any) {
	Debugf("[resty] "+format, v...)
}
