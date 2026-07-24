// Package logger（v2）是基于 go.uber.org/zap 的日志封装。与 v1 的包级全局函数不同，
// v2 以 Logger 实例为中心：New/MustNew 构建实例，With/Named/Channel 派生子实例。
//
//   - Structured / Sugar 双风格 API（Info / Infof / Infow 等）
//   - 文件切割（size / daily / both）、压缩与历史清理
//   - channel 分类路由：不同业务日志写入独立文件（Logger.Channel）
//   - Messager 消息推送 Hook（H 系列方法）
//   - Context 字段注入（*Ctx 系列）、动态级别（SetLevel）、采样（WithSampling）、
//     字段脱敏（WithRedactKeys）、slog 桥接（Logger.SlogHandler）
//
// 基本用法：
//
//	l := logger.MustNew(logger.WithPath("./logs/app"), logger.WithOutJSON(true))
//	defer l.Sync()
//	l.Info("hello", zap.String("k", "v"))
//
// 并发安全：Logger 的全部方法都可并发调用；派生实例与根实例共享底层资源，
// 由根实例的 Sync 统一释放（幂等），调用后所有关联实例不应再用于写日志。
package logger
