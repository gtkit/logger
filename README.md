# logger

基于 [zap](https://github.com/uber-go/zap) 的日志封装包，提供 Structured / Sugar 双模式、文件切割（按大小/按天）、消息推送 Hook 等功能。

## 安装

```bash
go get github.com/gtkit/logger@latest
```

## 快速开始

```go
package main

import (
    "github.com/gtkit/logger"
    "go.uber.org/zap"
)

func main() {
    logger.NewZap(
        logger.WithPath("./logs/app"),
        logger.WithLevel("info"),
        logger.WithOutJSON(true),
        logger.WithConsole(true),
        logger.WithFile(true),
        logger.WithDivision("daily"),
    )
    defer logger.Sync()

    // Structured 风格（主推，高性能）
    logger.Info("request processed",
        zap.String("method", "GET"),
        zap.Int("status", 200),
    )

    // Sugar 风格（便捷）
    logger.Infof("user %s logged in from %s", username, ip)

    // 条件日志
    logger.LogIf(err)

    // Hook 消息推送（飞书/钉钉/企微等）
    logger.HError("payment failed", zap.String("order_id", oid))
}
```

> 即使不调用 `NewZap`，日志函数也不会 panic，会默认输出到 stderr.

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

## 日志切割

### size 模式（默认）

文件名: `{path}-{level}.log`，由 lumberjack 按大小自动 rotate.

### daily 模式

文件名: `{path}-{level}-2006-01-02.log`，每天自动切换到新文件。同一天内超过 MaxSize 时 lumberjack 仍会按大小 rotate.

## 第三方库适配器

```go
// robfig/cron
cron.New(cron.WithLogger(logger.NewCronAdapter()))

// elastic/go-elasticsearch
es, _ := elasticsearch.NewClient(elasticsearch.Config{
    Logger: logger.NewESAdapter(),
})

// go-resty/resty
client := resty.New().SetLogger(logger.NewRestyAdapter())
```

## 消息推送

实现 `Messager` 接口即可接入飞书/钉钉/企微等通知:

```go
type Messager interface {
    Send(msg string)
    SendTo(url, msg string)
}

logger.NewZap(
    logger.WithMessager(myFeishuMessager),
)

logger.HError("payment failed", zap.String("order_id", "12345"))
```
