package gormlog

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// GormLogger 操作对象，实现 gormlogger.Interface
type GormZapLogger struct {
	ZapLogger     *zap.Logger
	SlowThreshold time.Duration
	SqlLog        bool
}

func ZapLogger(logger *zap.Logger, sqlLog bool) GormZapLogger {
	// if logger.Zlog() == nil {
	// 	logger.NewZap(&logger.Option{
	// 		FileStdout: true,
	// 		Division:   "size",
	// 	})
	// }

	return GormZapLogger{
		ZapLogger:     logger,                 // 使用全局的 Logger 对象
		SlowThreshold: 200 * time.Millisecond, // 慢查询阈值，单位为千分之一秒
		SqlLog:        sqlLog,
	}
}

// LogMode 实现 gormlogger.Interface 的 LogMode 方法
func (l GormZapLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	return GormZapLogger{
		ZapLogger:     l.ZapLogger,
		SlowThreshold: l.SlowThreshold,
		SqlLog:        l.SqlLog,
	}
}

func (l GormZapLogger) Info(ctx context.Context, s string, i ...interface{}) {
	l.logger().Sugar().Debugf(s, i...)
}

func (l GormZapLogger) Warn(ctx context.Context, s string, i ...interface{}) {
	l.logger().Sugar().Warnf(s, i...)
}

func (l GormZapLogger) Error(ctx context.Context, s string, i ...interface{}) {
	l.logger().Sugar().Errorf(s, i...)
}

func (l GormZapLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	// 获取运行时间
	elapsed := time.Since(begin)
	// 获取 SQL 请求和返回条数
	sql, rows := fc()

	// 通用字段
	logFields := []zap.Field{
		zap.String("sql", sql),
		zap.String("time", fmt.Sprintf("%.3fms", float64(elapsed.Nanoseconds())/1e6)),
		zap.Int64("rows", rows),
	}
	// Gorm 错误
	if err != nil {
		// 记录未找到的错误使用 warning 等级
		if errors.Is(err, gorm.ErrRecordNotFound) {
			l.logger().Warn("Database ErrRecordNotFound", logFields...)
		} else {
			// 其他错误使用 error 等级
			logFields = append(logFields, zap.Error(err))
			l.logger().Error("Database Error", logFields...)
		}
	}

	// 慢查询日志
	if l.SlowThreshold != 0 && elapsed > l.SlowThreshold {
		l.logger().Warn("Database Slow Log", logFields...)
	}

	// 记录所有 SQL 请求
	if l.SqlLog {
		l.logger().Debug("Database Query", logFields...)
	}

}
func (l GormZapLogger) logger() *zap.Logger {

	// 跳过 gorm 内置的调用
	var (
		gormPackage    = filepath.Join("gorm.io", "gorm")
		zapgormPackage = filepath.Join("moul.io", "zapgorm2")
	)

	// 减去一次封装，以及一次在 logger 初始化里添加 zap.AddCallerSkip(1)
	clone := l.ZapLogger.WithOptions(zap.AddCallerSkip(-2))

	for i := 2; i < 15; i++ {
		_, file, _, ok := runtime.Caller(i)
		switch {
		case !ok:
		case strings.HasSuffix(file, "_test.go"):
		case strings.Contains(file, gormPackage):
		case strings.Contains(file, zapgormPackage):
		default:
			// 返回一个附带跳过行号的新的 zap logger
			return clone.WithOptions(zap.AddCallerSkip(i))
		}
	}
	return l.ZapLogger
}
