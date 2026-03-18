# logger/v2

基于 [zap](https://github.com/uber-go/zap) 的日志封装包（实例模式），提供 Structured / Sugar 双模式、文件切割（按大小/按天）、消息推送 Hook 等功能。

## 安装

```bash
go get github.com/gtkit/logger/v2@latest
```

## 快速开始

```go
package main

import (
    "github.com/gtkit/logger/v2"
    "go.uber.org/zap"
)

func main() {
    log := logger.MustNew(
        logger.WithPath("./logs/app"),
        logger.WithLevel("info"),
        logger.WithOutJSON(true),
        logger.WithConsole(true),
        logger.WithFile(true),
        logger.WithDivision("daily"),
    )
    defer log.Sync()

    // Structured 风格（主推，高性能）
    log.Info("request processed",
        zap.String("method", "GET"),
        zap.Int("status", 200),
    )

    // Sugar 风格（便捷）
    log.Infof("user %s logged in from %s", username, ip)

    // 带预设字段的子 Logger
    reqLog := log.With(zap.String("request_id", rid))
    reqLog.Info("processing")

    // 带名称的子 Logger
    authLog := log.Named("auth")
    authLog.Warn("token expired", zap.String("user", "alice"))

    // 条件日志
    log.LogIf(err)

    // Hook 消息推送
    log.HError("payment failed", zap.String("order_id", oid))
}
```

## 配置选项

| Option | 说明 | 默认值 |
|--------|------|--------|
| `WithPath(p)` | 日志文件路径前缀 | `./logs/` |
| `WithLevel(l)` | 日志级别 | `info` |
| `WithOutJSON(b)` | 是否输出 JSON 格式 | `false` |
| `WithConsole(b)` | 是否输出到控制台 | `false` |
| `WithFile(b)` | 是否输出到文件 | `true` |
| `WithDivision(d)` | 切割方式 `size`/`daily` | `size` |
| `WithMaxSize(mb)` | 单文件最大 MB | `512` |
| `WithMaxAge(days)` | 最大保存天数 | `7` |
| `WithMaxBackups(n)` | 最大备份数 | `50` |
| `WithCompress(b)` | 是否压缩归档 | `true` |
| `WithMessager(m)` | 消息推送 Hook | `nil` |

## 第三方库适配器

```go
// robfig/cron
cron.New(cron.WithLogger(logger.NewCronAdapter(log)))

// elastic/go-elasticsearch
es, _ := elasticsearch.NewClient(elasticsearch.Config{
    Logger: logger.NewESAdapter(log),
})

// go-resty/resty
client := resty.New().SetLogger(logger.NewRestyAdapter(log))
```

## 消息推送

```go
type Messager interface {
    Send(msg string)
    SendTo(url, msg string)
}

log := logger.MustNew(
    logger.WithMessager(myFeishuMessager),
)

log.HError("payment failed", zap.String("order_id", "12345"))
```
