// Package logger 是基于 go.uber.org/zap 的日志封装，通过包级函数提供开箱即用的日志能力：
//
//   - Structured / Sugar 双风格 API（Info / Infof / Infow 等）
//   - 文件切割（size / daily / both）、压缩与历史清理
//   - channel 分类路由：不同业务日志写入独立文件（Channel）
//   - Messager 消息推送 Hook（H 系列方法）
//   - Context 字段注入（*Ctx 系列）、动态级别（SetLevel）、采样（WithSampling）、
//     字段脱敏（WithRedactKeys）、slog 桥接（SlogHandler）
//
// 基本用法：
//
//	logger.NewZap(logger.WithPath("./logs/app"), logger.WithOutJSON(true))
//	defer logger.Sync()
//	logger.Info("hello", zap.String("k", "v"))
//
// 并发安全：包级日志函数与 ChannelLogger 的全部方法都可并发调用；New/NewZap 可在
// 运行期重复调用以重新配置，进行中的日志调用不会写入已关闭的资源。Sync 会 flush
// 并关闭文件资源，调用后再打日志将回退到开发期 console logger。
package logger
