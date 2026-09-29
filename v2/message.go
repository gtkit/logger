package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

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

// asyncMessager 将推送操作放入有界队列，由独立 goroutine 执行，避免阻塞日志调用。
// 队列满时静默丢弃推送（日志本身已写入文件，只丢通知）。
type asyncMessager struct {
	inner        Messager
	queue        chan func()
	done         chan struct{}
	drainTimeout time.Duration
	mu           sync.RWMutex
	closeOnce    sync.Once
	closed       bool
	dropped      atomic.Int64
}

func newAsyncMessager(m Messager, size int, drainTimeout time.Duration) *asyncMessager {
	am := &asyncMessager{
		inner:        m,
		queue:        make(chan func(), size),
		done:         make(chan struct{}),
		drainTimeout: drainTimeout,
	}
	go am.run()
	return am
}

func (am *asyncMessager) run() {
	defer close(am.done)
	for fn := range am.queue {
		runMessagerFunc(fn)
	}
}

func runMessagerFunc(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "logger: messager panic recovered: %v\n", r)
		}
	}()
	fn()
}

func (am *asyncMessager) Send(msg string) {
	am.enqueue(func() { am.inner.Send(msg) })
}

func (am *asyncMessager) SendTo(url, msg string) {
	am.enqueue(func() { am.inner.SendTo(url, msg) })
}

func (am *asyncMessager) enqueue(fn func()) {
	am.mu.RLock()
	if am.closed {
		am.mu.RUnlock()
		return
	}
	select {
	case am.queue <- fn:
	default:
		am.dropped.Add(1)
	}
	am.mu.RUnlock()
}

// close 关闭队列并等待待处理推送完成，最多等 drainTimeout：
// 外部 Send 挂起（网络黑洞）时不能拖住进程退出，超时后把未执行的推送计入 dropped 并告警，
// 后台协程继续消费直到外部调用返回。
func (am *asyncMessager) close() {
	am.closeOnce.Do(func() {
		am.mu.Lock()
		am.closed = true
		close(am.queue)
		am.mu.Unlock()

		timer := time.NewTimer(am.drainTimeout)
		defer timer.Stop()
		select {
		case <-am.done:
		case <-timer.C:
			pending := int64(len(am.queue))
			am.dropped.Add(pending)
			fmt.Fprintf(os.Stderr, "logger: messager drain timed out after %v, %d pending push(es) dropped\n", am.drainTimeout, pending)
		}
	})
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

// formatFieldsMsg 把结构化字段序列化后追加到消息文本，用于 Messager 推送。
// 这里用标准库 encoding/json 而非 gtkit/json：仅服务于非热路径的 Hook 消息格式化，
// 不值得为此引入额外依赖（取舍记录，见项目 JSON 选型规则）。
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

func formatHookFieldsMsg(msg string, fields []zap.Field, redact func([]zapcore.Field) []zapcore.Field) string {
	if redact != nil {
		fields = redact(fields)
	}
	return formatFieldsMsg(msg, fields)
}
