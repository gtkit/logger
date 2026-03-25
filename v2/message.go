package logger

import (
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Messager 消息推送接口.
// 实现此接口以将日志消息发送到外部平台（飞书/钉钉/企微等）.
type Messager interface {
	// Send 发送消息到默认地址.
	Send(msg string)
	// SendTo 发送消息到指定 URL.
	SendTo(url, msg string)
}

// formatMsg 格式化消息内容.
func formatMsg(template string, fmtArgs []any) string {
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

func formatFieldsMsg(msg string, fields []zap.Field) string {
	if len(fields) == 0 {
		return msg
	}

	enc := zapcore.NewMapObjectEncoder()
	for _, field := range fields {
		field.AddTo(enc)
	}

	data, err := json.Marshal(enc.Fields)
	if err != nil {
		return msg
	}

	return fmt.Sprintf("%s %s", msg, data)
}
