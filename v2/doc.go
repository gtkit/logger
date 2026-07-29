// Package logger（v2）是基于 go.uber.org/zap 的日志封装。
// v2 以 Logger 实例为中心：New/MustNew 构建实例，With/Named/Channel 派生子实例；
// 同时提供 log/slog 同款的进程默认实例（SetDefault/Default）与包级转发函数，
// 让常规调用点保持 logger.Info(...) 的简洁形态。
//
//   - Structured / Sugar 双风格 API（Info / Infof / Infow 等）
//   - 文件切割（size / daily / both）、压缩与历史清理
//   - channel 分类路由：不同业务日志写入独立文件（Logger.Channel）
//   - Messager 消息推送 Hook（H 系列方法）
//   - Context 字段注入（*Ctx 系列）、动态级别（SetLevel）、采样（WithSampling）、
//     字段脱敏（WithRedactKeys）
//
// 基本用法：
//
//	l := logger.MustNew(logger.WithPath("./logs/app"), logger.WithOutJSON(true))
//	defer l.Sync()
//	l.Info("hello", zap.String("k", "v"))
//
// 默认实例与包级函数（进程内单主日志的推荐形态）：
//
//	logger.SetDefault(logger.MustNew(logger.WithPath("./logs/app")))
//	defer logger.Sync()
//	logger.Info("hello", zap.String("k", "v")) // caller 指向本行
//
// 未经 SetDefault 时，包级函数写入懒创建的纯控制台兜底实例（不落盘、
// 不替换 zap 全局）。辅助实例（如独立的 access 日志实例）用
// WithReplaceGlobals(false) 构建，避免抢占 zap 全局。
//
// 并发安全：Logger 的全部记录方法都可并发调用（Sync/Undo 为生命周期操作，
// 须在停止新写入后串行执行）；派生实例与根实例共享底层资源，
// 由根实例的 Sync 统一释放（幂等），调用后所有关联实例不应再用于写日志。
package logger
