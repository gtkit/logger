package logger

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type testCloser struct {
	count atomic.Int32
}

func (c *testCloser) Close() error {
	c.count.Add(1)
	return nil
}

type testMessager struct {
	lastMsg string
	lastURL string
}

func (m *testMessager) Send(msg string) {
	m.lastMsg = msg
}

func (m *testMessager) SendTo(url, msg string) {
	m.lastURL = url
	m.lastMsg = msg
}

func TestDailyWriteSyncerRotatesOnFullDateChange(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		path:       t.TempDir() + "/daily",
		level:      "info",
		maxSize:    1,
		maxAge:     1,
		maxBackups: 1,
	}

	dw, err := newDailyWriteSyncer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = dw.Close()
	})
	oldWriter := dw.lj
	dw.currentDate = "2000-01-01"

	if _, err = dw.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}

	wantDate := time.Now().Format(time.DateOnly)
	if dw.currentDate != wantDate {
		t.Fatalf("currentDate = %q, want %q", dw.currentDate, wantDate)
	}
	if dw.lj == oldWriter {
		t.Fatal("daily writer was not replaced after date change")
	}
	if !strings.Contains(dw.lj.Filename, wantDate) {
		t.Fatalf("filename %q does not contain current date %q", dw.lj.Filename, wantDate)
	}
}

func TestSyncIsIdempotentAcrossDerivedLoggers(t *testing.T) {
	closer := &testCloser{}
	var undoCalls atomic.Int32

	log := &Logger{
		zap:   zap.NewNop(),
		sugar: zap.NewNop().Sugar(),
		state: &lifecycleState{
			root: zap.NewNop(),
			undo: func() {
				undoCalls.Add(1)
			},
			closers: []io.Closer{closer},
		},
	}

	child := log.With(zap.String("request_id", "req-1"))
	child.Sync()
	log.Sync()
	child.Undo()

	if got := closer.count.Load(); got != 1 {
		t.Fatalf("closer closed %d times, want 1", got)
	}
	if got := undoCalls.Load(); got != 1 {
		t.Fatalf("undo called %d times, want 1", got)
	}
}

func TestHInfoIncludesFieldsInMessager(t *testing.T) {
	msg := &testMessager{}
	log := &Logger{
		zap:      zap.NewNop(),
		sugar:    zap.NewNop().Sugar(),
		state:    &lifecycleState{root: zap.NewNop()},
		messager: msg,
	}

	log.HInfo("request failed",
		zap.String("request_id", "req-1"),
		zap.Int("status", 500),
	)

	if !strings.Contains(msg.lastMsg, "request_id") || !strings.Contains(msg.lastMsg, "req-1") {
		t.Fatalf("hook message %q does not include structured fields", msg.lastMsg)
	}
}

func TestChannelUnconfiguredWritesOnlyDefaultFile(t *testing.T) {
	tempDir := t.TempDir()
	defaultPath := filepath.Join(tempDir, "default", "app")
	channelPath := filepath.Join(tempDir, "channels", "order", "order")

	log := MustNew(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(defaultPath),
	)
	defer log.Sync()

	log.Channel("order").Info("order created", zap.String("order_id", "A100"))

	defaultLog := readLogFile(t, defaultPath+"-info.log")
	if !strings.Contains(defaultLog, "order created") {
		t.Fatalf("default log does not contain channel message: %s", defaultLog)
	}
	if !strings.Contains(defaultLog, `"channel":"order"`) {
		t.Fatalf("default log does not contain channel field: %s", defaultLog)
	}
	if _, err := os.Stat(channelPath + "-info.log"); !os.IsNotExist(err) {
		t.Fatalf("unexpected unconfigured channel file state: %v", err)
	}
}

func TestChannelConfiguredCanDuplicateToDefaultFile(t *testing.T) {
	tempDir := t.TempDir()
	defaultPath := filepath.Join(tempDir, "default", "app")
	channelPath := filepath.Join(tempDir, "channels", "order", "order")

	log := MustNew(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(defaultPath),
		WithChannel("order",
			WithChannelPath(channelPath),
			WithChannelDuplicateToDefault(true),
		),
	)
	defer log.Sync()

	log.With(zap.String("request_id", "req-1")).Named("api").Channel("order").Info(
		"order duplicated",
		zap.String("order_id", "A200"),
	)

	defaultLog := readLogFile(t, defaultPath+"-info.log")
	channelLog := readLogFile(t, channelPath+"-info.log")

	for _, content := range []string{defaultLog, channelLog} {
		if !strings.Contains(content, "order duplicated") {
			t.Fatalf("log does not contain duplicated message: %s", content)
		}
		if !strings.Contains(content, `"channel":"order"`) {
			t.Fatalf("log does not contain channel field: %s", content)
		}
		if !strings.Contains(content, `"request_id":"req-1"`) {
			t.Fatalf("log does not contain inherited context field: %s", content)
		}
		if !strings.Contains(content, `"logger":"api"`) {
			t.Fatalf("log does not contain inherited logger name: %s", content)
		}
	}
}

func TestChannelConfiguredCanWriteOnlyChannelFile(t *testing.T) {
	tempDir := t.TempDir()
	defaultPath := filepath.Join(tempDir, "default", "app")
	channelPath := filepath.Join(tempDir, "channels", "audit", "audit")

	log := MustNew(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(defaultPath),
		WithChannel("audit",
			WithChannelPath(channelPath),
			WithChannelDuplicateToDefault(false),
		),
	)
	defer log.Sync()

	log.Info("default event", zap.String("scope", "default"))
	log.Channel("audit").Warn("audit only", zap.String("actor", "system"))

	defaultLog := readLogFile(t, defaultPath+"-info.log")
	channelLog := readLogFile(t, channelPath+"-info.log")

	if !strings.Contains(defaultLog, "default event") {
		t.Fatalf("default log missing base message: %s", defaultLog)
	}
	if strings.Contains(defaultLog, "audit only") {
		t.Fatalf("default log unexpectedly contains channel-only message: %s", defaultLog)
	}
	if !strings.Contains(channelLog, "audit only") {
		t.Fatalf("channel log missing message: %s", channelLog)
	}
	if !strings.Contains(channelLog, `"channel":"audit"`) {
		t.Fatalf("channel log missing channel field: %s", channelLog)
	}
}

func TestConfiguredChannelReuseDoesNotLeakContext(t *testing.T) {
	tempDir := t.TempDir()
	defaultPath := filepath.Join(tempDir, "default", "app")
	channelPath := filepath.Join(tempDir, "channels", "order", "order")

	log := MustNew(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(defaultPath),
		WithChannel("order",
			WithChannelPath(channelPath),
			WithChannelDuplicateToDefault(true),
		),
	)
	defer log.Sync()

	orderLog := log.Channel("order")
	orderLog.Named("worker").With(zap.String("request_id", "req-1")).Info(
		"first message",
		zap.String("order_id", "A100"),
	)
	orderLog.Info("second message", zap.String("order_id", "A200"))

	channelLog := readLogFile(t, channelPath+"-info.log")
	if !strings.Contains(channelLog, `"logger":"worker"`) {
		t.Fatalf("channel log missing named logger entry: %s", channelLog)
	}
	if !strings.Contains(channelLog, `"request_id":"req-1"`) {
		t.Fatalf("channel log missing contextual field entry: %s", channelLog)
	}
	if !strings.Contains(channelLog, `"msg":"second message"`) {
		t.Fatalf("channel log missing second entry: %s", channelLog)
	}
	if strings.Count(channelLog, `"request_id":"req-1"`) != 1 {
		t.Fatalf("context leaked into reused channel logger: %s", channelLog)
	}
}

func readLogFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file %q: %v", path, err)
	}

	return string(data)
}

// ============================================================
// formatMsg tests
// ============================================================

func TestFormatMsg_NoArgs(t *testing.T) {
	got := formatMsg("hello world", nil)
	if got != "hello world" {
		t.Fatalf("got %q, want %q", got, "hello world")
	}
}

func TestFormatMsg_TemplateWithArgs(t *testing.T) {
	got := formatMsg("count=%d name=%s", []any{42, "foo"})
	if got != "count=42 name=foo" {
		t.Fatalf("got %q, want %q", got, "count=42 name=foo")
	}
}

func TestFormatMsg_EmptyTemplateSingleString(t *testing.T) {
	got := formatMsg("", []any{"single"})
	if got != "single" {
		t.Fatalf("got %q, want %q", got, "single")
	}
}

func TestFormatMsg_EmptyTemplateMultipleArgs(t *testing.T) {
	got := formatMsg("", []any{"a", 1, true})
	// fmt.Sprint joins without spaces between non-string args
	want := "a1 true"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// ============================================================
// asyncMessager tests
// ============================================================

type syncTestMessager struct {
	msgs   chan string
	toMsgs chan [2]string
}

func newSyncTestMessager(size int) *syncTestMessager {
	return &syncTestMessager{
		msgs:   make(chan string, size),
		toMsgs: make(chan [2]string, size),
	}
}

func (m *syncTestMessager) Send(msg string) {
	m.msgs <- msg
}

func (m *syncTestMessager) SendTo(url, msg string) {
	m.toMsgs <- [2]string{url, msg}
}

func TestAsyncMessager_SendDelivered(t *testing.T) {
	inner := newSyncTestMessager(10)
	am := newAsyncMessager(inner, 10)

	am.Send("hello")
	am.SendTo("http://example.com", "world")
	am.close()

	select {
	case msg := <-inner.msgs:
		if msg != "hello" {
			t.Fatalf("got %q, want %q", msg, "hello")
		}
	default:
		t.Fatal("Send message not delivered")
	}

	select {
	case pair := <-inner.toMsgs:
		if pair[0] != "http://example.com" || pair[1] != "world" {
			t.Fatalf("got %v, want [http://example.com world]", pair)
		}
	default:
		t.Fatal("SendTo message not delivered")
	}
}

func TestAsyncMessager_NonBlocking(t *testing.T) {
	inner := newSyncTestMessager(10)
	am := newAsyncMessager(inner, 10)
	defer am.close()

	// Should not block even if inner is slow
	done := make(chan struct{})
	go func() {
		am.Send("msg1")
		am.SendTo("url", "msg2")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Send/SendTo blocked")
	}
}

func TestAsyncMessager_QueueFullDropsSilently(t *testing.T) {
	// Use a blocking inner to fill the queue
	blocker := &blockingMessager{block: make(chan struct{}), started: make(chan struct{})}
	am := newAsyncMessager(blocker, 2)

	// Fill the queue: first item is being processed (blocked), next 2 fill the buffer
	am.Send("1")
	<-blocker.started // wait for goroutine to start processing
	am.Send("2")
	am.Send("3")
	// This one should be silently dropped (queue full)
	am.Send("4")

	// Unblock and close
	close(blocker.block)
	am.close()
	// No panic = success
}

type blockingMessager struct {
	block       chan struct{}
	started     chan struct{}
	startedOnce sync.Once
}

func (m *blockingMessager) Send(msg string) {
	if m.started != nil {
		m.startedOnce.Do(func() { close(m.started) })
	}
	<-m.block
}

func (m *blockingMessager) SendTo(url, msg string) {
	if m.started != nil {
		m.startedOnce.Do(func() { close(m.started) })
	}
	<-m.block
}

func TestAsyncMessager_CloseDrainsQueue(t *testing.T) {
	inner := newSyncTestMessager(10)
	am := newAsyncMessager(inner, 10)

	am.Send("a")
	am.Send("b")
	am.Send("c")
	am.close()

	count := len(inner.msgs)
	if count != 3 {
		t.Fatalf("expected 3 messages drained, got %d", count)
	}
}

// ============================================================
// Zap() and Sugar() tests
// ============================================================

func TestZapAndSugar_NonNil(t *testing.T) {
	l := MustNew(WithConsole(false), WithFile(false))
	defer l.Sync()

	if l.Zap() == nil {
		t.Fatal("Zap() returned nil")
	}
	if l.Sugar() == nil {
		t.Fatal("Sugar() returned nil")
	}
}

// ============================================================
// DPanic test (development mode)
// ============================================================

func TestDPanic_WritesToFile(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "logs", "app")

	l := MustNew(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(path),
	)
	defer l.Sync()

	// DPanic in production mode (default) logs but does not panic
	l.DPanic("dpanic-test-msg", zap.String("key", "val"))

	content := readLogFile(t, path+"-info.log")
	if !strings.Contains(content, "dpanic-test-msg") {
		t.Fatalf("DPanic message not found in log: %s", content)
	}
}

// ============================================================
// Warnf, Errorf tests
// ============================================================

func TestWarnfAndErrorf_WriteToFile(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "logs", "app")

	l := MustNew(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(path),
	)
	defer l.Sync()

	l.Warnf("warn-%s-%d", "test", 1)
	l.Errorf("error-%s-%d", "test", 2)

	content := readLogFile(t, path+"-info.log")
	if !strings.Contains(content, "warn-test-1") {
		t.Fatalf("Warnf message not found: %s", content)
	}
	if !strings.Contains(content, "error-test-2") {
		t.Fatalf("Errorf message not found: %s", content)
	}
}

// ============================================================
// H* methods (log + messager) tests
// ============================================================

func TestHInfof_LogsAndCallsMessager(t *testing.T) {
	msg := &testMessager{}
	l := &Logger{
		zap:      zap.NewNop(),
		sugar:    zap.NewNop().Sugar(),
		state:    &lifecycleState{root: zap.NewNop()},
		messager: msg,
	}

	l.HInfof("count=%d", 42)
	if !strings.Contains(msg.lastMsg, "count=42") {
		t.Fatalf("messager got %q, want it to contain 'count=42'", msg.lastMsg)
	}
}

func TestHInfoTo_LogsAndCallsMessager(t *testing.T) {
	msg := &testMessager{}
	l := &Logger{
		zap:      zap.NewNop(),
		sugar:    zap.NewNop().Sugar(),
		state:    &lifecycleState{root: zap.NewNop()},
		messager: msg,
	}

	l.HInfoTo("http://hook", "info-to-msg", zap.String("k", "v"))
	if msg.lastURL != "http://hook" {
		t.Fatalf("url = %q, want %q", msg.lastURL, "http://hook")
	}
	if !strings.Contains(msg.lastMsg, "info-to-msg") {
		t.Fatalf("msg = %q, want to contain 'info-to-msg'", msg.lastMsg)
	}
}

func TestHInfoTof_LogsAndCallsMessager(t *testing.T) {
	msg := &testMessager{}
	l := &Logger{
		zap:      zap.NewNop(),
		sugar:    zap.NewNop().Sugar(),
		state:    &lifecycleState{root: zap.NewNop()},
		messager: msg,
	}

	l.HInfoTof("http://hook", "val=%d", 99)
	if msg.lastURL != "http://hook" {
		t.Fatalf("url = %q, want %q", msg.lastURL, "http://hook")
	}
	if !strings.Contains(msg.lastMsg, "val=99") {
		t.Fatalf("msg = %q, want to contain 'val=99'", msg.lastMsg)
	}
}

func TestHError_LogsAndCallsMessager(t *testing.T) {
	msg := &testMessager{}
	l := &Logger{
		zap:      zap.NewNop(),
		sugar:    zap.NewNop().Sugar(),
		state:    &lifecycleState{root: zap.NewNop()},
		messager: msg,
	}

	l.HError("err-msg", zap.Int("code", 500))
	if !strings.Contains(msg.lastMsg, "err-msg") {
		t.Fatalf("msg = %q, want to contain 'err-msg'", msg.lastMsg)
	}
	if !strings.Contains(msg.lastMsg, "code") {
		t.Fatalf("msg = %q, want to contain 'code'", msg.lastMsg)
	}
}

func TestHErrorf_LogsAndCallsMessager(t *testing.T) {
	msg := &testMessager{}
	l := &Logger{
		zap:      zap.NewNop(),
		sugar:    zap.NewNop().Sugar(),
		state:    &lifecycleState{root: zap.NewNop()},
		messager: msg,
	}

	l.HErrorf("fail-%s", "db")
	if !strings.Contains(msg.lastMsg, "fail-db") {
		t.Fatalf("msg = %q, want to contain 'fail-db'", msg.lastMsg)
	}
}

func TestHErrorTo_LogsAndCallsMessager(t *testing.T) {
	msg := &testMessager{}
	l := &Logger{
		zap:      zap.NewNop(),
		sugar:    zap.NewNop().Sugar(),
		state:    &lifecycleState{root: zap.NewNop()},
		messager: msg,
	}

	l.HErrorTo("http://err-hook", "err-to-msg", zap.String("k", "v"))
	if msg.lastURL != "http://err-hook" {
		t.Fatalf("url = %q, want %q", msg.lastURL, "http://err-hook")
	}
	if !strings.Contains(msg.lastMsg, "err-to-msg") {
		t.Fatalf("msg = %q, want to contain 'err-to-msg'", msg.lastMsg)
	}
}

func TestHErrorTof_LogsAndCallsMessager(t *testing.T) {
	msg := &testMessager{}
	l := &Logger{
		zap:      zap.NewNop(),
		sugar:    zap.NewNop().Sugar(),
		state:    &lifecycleState{root: zap.NewNop()},
		messager: msg,
	}

	l.HErrorTof("http://err-hook", "code=%d", 503)
	if msg.lastURL != "http://err-hook" {
		t.Fatalf("url = %q, want %q", msg.lastURL, "http://err-hook")
	}
	if !strings.Contains(msg.lastMsg, "code=503") {
		t.Fatalf("msg = %q, want to contain 'code=503'", msg.lastMsg)
	}
}

// ============================================================
// Ctx methods tests
// ============================================================

type ctxKey struct{}

func TestCtxMethods_InjectFields(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "logs", "app")

	ctxFn := func(ctx context.Context) []zap.Field {
		if v, ok := ctx.Value(ctxKey{}).(string); ok {
			return []zap.Field{zap.String("trace_id", v)}
		}
		return nil
	}

	l := MustNew(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(path),
		WithContextFields(ctxFn),
	)
	defer l.Sync()

	ctx := context.WithValue(context.Background(), ctxKey{}, "abc-123")

	l.InfoCtx(ctx, "info-ctx-msg")
	l.WarnCtx(ctx, "warn-ctx-msg")
	l.ErrorCtx(ctx, "error-ctx-msg")

	content := readLogFile(t, path+"-info.log")
	for _, msg := range []string{"info-ctx-msg", "warn-ctx-msg", "error-ctx-msg"} {
		if !strings.Contains(content, msg) {
			t.Fatalf("log missing %q: %s", msg, content)
		}
	}
	// Each entry should have the trace_id from context
	if count := strings.Count(content, `"trace_id":"abc-123"`); count < 3 {
		t.Fatalf("expected trace_id in all 3 entries, found %d times in: %s", count, content)
	}
}

func TestDebugCtx_InjectsFields(t *testing.T) {
	msg := &testMessager{}
	ctxFn := func(ctx context.Context) []zap.Field {
		if v, ok := ctx.Value(ctxKey{}).(string); ok {
			return []zap.Field{zap.String("trace_id", v)}
		}
		return nil
	}
	l := &Logger{
		zap:           zap.NewNop(),
		sugar:         zap.NewNop().Sugar(),
		state:         &lifecycleState{root: zap.NewNop()},
		messager:      msg,
		contextFields: ctxFn,
	}
	ctx := context.WithValue(context.Background(), ctxKey{}, "debug-trace")
	// Should not panic; covers DebugCtx path
	l.DebugCtx(ctx, "debug-ctx-msg")
}

// ============================================================
// formatChannelMsg tests
// ============================================================

func TestFormatChannelMsg_EmptyChannel(t *testing.T) {
	got := formatChannelMsg("", "hello")
	if got != "hello" {
		t.Fatalf("got %q, want %q", got, "hello")
	}
}

func TestFormatChannelMsg_WithChannel(t *testing.T) {
	got := formatChannelMsg("order", "placed")
	want := "[channel=order] placed"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// ============================================================
// Option tests: WithCompress, WithMessager, WithMessagerQueueSize, WithContextFields
// ============================================================

func TestWithCompress(t *testing.T) {
	cfg := defaultConfig()
	if err := WithCompress(false)(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.compress != false {
		t.Fatal("compress should be false")
	}
	if err := WithCompress(true)(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.compress != true {
		t.Fatal("compress should be true")
	}
}

func TestWithMessager(t *testing.T) {
	cfg := defaultConfig()
	m := &testMessager{}
	if err := WithMessager(m)(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.messager != m {
		t.Fatal("messager not set")
	}
}

func TestWithMessagerQueueSize(t *testing.T) {
	cfg := defaultConfig()
	if err := WithMessagerQueueSize(256)(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.messagerQueueSize != 256 {
		t.Fatalf("got %d, want 256", cfg.messagerQueueSize)
	}

	if err := WithMessagerQueueSize(0)(cfg); err == nil {
		t.Fatal("expected error for size <= 0")
	}
}

func TestWithContextFields(t *testing.T) {
	cfg := defaultConfig()
	fn := func(ctx context.Context) []zap.Field {
		return []zap.Field{zap.String("k", "v")}
	}
	if err := WithContextFields(fn)(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.contextFields == nil {
		t.Fatal("contextFields not set")
	}
}

// ============================================================
// Adapter tests
// ============================================================

func TestCronAdapter_Error(t *testing.T) {
	l := MustNew(WithConsole(false), WithFile(false))
	defer l.Sync()

	a := NewCronAdapter(l)
	// Should not panic
	a.Error(nil, "test cron error", "key", "val")
}

func TestCronNormalizeKVs(t *testing.T) {
	even := []any{"k1", "v1", "k2", "v2"}
	got := cronNormalizeKVs(even)
	if len(got) != 4 {
		t.Fatalf("even input should not change length, got %d", len(got))
	}

	odd := []any{"k1", "v1", "k2"}
	got = cronNormalizeKVs(odd)
	if len(got) != 4 {
		t.Fatalf("odd input should be padded to even, got len=%d", len(got))
	}
	if got[3] != "<MISSING>" {
		t.Fatalf("odd input should be padded with <MISSING>, got %v", got[3])
	}

	got = cronNormalizeKVs(nil)
	if len(got) != 0 {
		t.Fatalf("nil should stay empty, got %d", len(got))
	}
}

func TestRestyAdapter_ErrorfAndWarnf(t *testing.T) {
	l := MustNew(WithConsole(false), WithFile(false))
	defer l.Sync()

	a := NewRestyAdapter(l)
	// Should not panic
	a.Errorf("resty error %s", "test")
	a.Warnf("resty warn %s", "test")
}

// ============================================================
// Dynamic log level tests
// ============================================================

func TestSetLevel_InitialLevelIsInfo(t *testing.T) {
	l := MustNew(WithConsole(false), WithFile(false))
	defer l.Sync()

	if got := l.GetLevel(); got != "info" {
		t.Fatalf("initial level = %q, want %q", got, "info")
	}
}

func TestSetLevel_ChangesToDebug(t *testing.T) {
	l := MustNew(WithConsole(false), WithFile(false))
	defer l.Sync()

	l.SetLevel("debug")
	if got := l.GetLevel(); got != "debug" {
		t.Fatalf("level after SetLevel(debug) = %q, want %q", got, "debug")
	}
}

func TestSetLevel_ErrorSuppressesInfo(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "logs", "app")

	l := MustNew(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(path),
	)
	defer l.Sync()

	l.SetLevel("error")

	l.Info("should-not-appear")
	l.Error("should-appear")

	content := readLogFile(t, path+"-info.log")
	if strings.Contains(content, "should-not-appear") {
		t.Fatalf("Info message written at error level: %s", content)
	}
	if !strings.Contains(content, "should-appear") {
		t.Fatalf("Error message missing at error level: %s", content)
	}
}

func TestSetLevel_InvalidLevelNoChange(t *testing.T) {
	l := MustNew(WithConsole(false), WithFile(false))
	defer l.Sync()

	l.SetLevel("bogus")
	if got := l.GetLevel(); got != "info" {
		t.Fatalf("level after invalid SetLevel = %q, want %q", got, "info")
	}
}

// ============================================================
// slog bridge tests
// ============================================================

func TestSlogHandler_NoPanic(t *testing.T) {
	l := MustNew(WithConsole(false), WithFile(false))
	defer l.Sync()

	sl := slog.New(l.SlogHandler())
	if sl == nil {
		t.Fatal("slog.New returned nil")
	}
}

func TestSlogHandler_WritesToFile(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "logs", "app")

	l := MustNew(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(path),
	)
	defer l.Sync()

	sl := slog.New(l.SlogHandler())
	sl.Info("slog-bridge-test", "key", "val123")

	content := readLogFile(t, path+"-info.log")
	if !strings.Contains(content, "slog-bridge-test") {
		t.Fatalf("slog message not found in log: %s", content)
	}
	if !strings.Contains(content, "val123") {
		t.Fatalf("slog attr not found in log: %s", content)
	}
}

func TestSlogHandler_EnabledRespectsLevel(t *testing.T) {
	l := MustNew(WithConsole(false), WithFile(false), WithLevel("error"))
	defer l.Sync()

	h := l.SlogHandler()
	if h.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("slog handler should not enable Info when logger level is error")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Fatal("slog handler should enable Error when logger level is error")
	}
}

func TestDurationEncoderOption_UsesStringEncoder(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "logs", "app")

	l := MustNew(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(path),
		WithDurationEncoder(zapcore.StringDurationEncoder),
	)
	defer l.Sync()

	l.Info("duration-test", zap.Duration("elapsed", 1500*time.Millisecond))

	content := readLogFile(t, path+"-info.log")
	if !strings.Contains(content, `"elapsed":"1.5s"`) {
		t.Fatalf("duration encoder did not write string duration: %s", content)
	}
}

// ============================================================
// DroppedMessages tests
// ============================================================

func TestDroppedMessages_ZeroWithoutMessager(t *testing.T) {
	l := MustNew(WithConsole(false), WithFile(false))
	defer l.Sync()

	if got := l.DroppedMessages(); got != 0 {
		t.Fatalf("DroppedMessages() = %d, want 0", got)
	}
}

func TestDroppedMessages_CountsDrops(t *testing.T) {
	blocker := &blockingMessager{block: make(chan struct{}), started: make(chan struct{})}

	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "logs", "app")

	l := MustNew(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(path),
		WithMessager(blocker),
		WithMessagerQueueSize(2),
	)

	// First HInfo is picked up by the goroutine and blocks on blocker.
	l.HInfo("msg-1")
	<-blocker.started // wait for goroutine to start processing

	// Fill the queue buffer (size=2).
	l.HInfo("msg-2")
	l.HInfo("msg-3")

	// These should be dropped.
	l.HInfo("msg-drop-1")
	l.HInfo("msg-drop-2")

	dropped := l.DroppedMessages()
	if dropped < 2 {
		t.Fatalf("DroppedMessages() = %d, want >= 2", dropped)
	}

	// Unblock and clean up.
	close(blocker.block)
	l.Sync()
}

// ============================================================
// Debugw / Warnw / Errorw tests
// ============================================================

func TestDebugwWarnwErrorw(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "logs", "app")

	l := MustNew(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(path),
		WithLevel("debug"),
	)
	defer l.Sync()

	l.Debugw("debugw-msg", "key1", "val1")
	l.Warnw("warnw-msg", "key2", "val2")
	l.Errorw("errorw-msg", "key3", "val3")

	content := readLogFile(t, path+"-debug.log")
	for _, msg := range []string{"debugw-msg", "warnw-msg", "errorw-msg"} {
		if !strings.Contains(content, msg) {
			t.Fatalf("log missing %q: %s", msg, content)
		}
	}
	for _, kv := range []string{"key1", "val1", "key2", "val2", "key3", "val3"} {
		if !strings.Contains(content, kv) {
			t.Fatalf("log missing kv %q: %s", kv, content)
		}
	}
}

// ============================================================
// Adapter nil Logger panic tests
// ============================================================

func TestNewCronAdapterPanicsOnNil(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("NewCronAdapter(nil) should panic")
		}
	}()
	NewCronAdapter(nil)
}

func TestNewESAdapterPanicsOnNil(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("NewESAdapter(nil) should panic")
		}
	}()
	NewESAdapter(nil)
}

func TestNewRestyAdapterPanicsOnNil(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("NewRestyAdapter(nil) should panic")
		}
	}()
	NewRestyAdapter(nil)
}

// ============================================================
// slog KindGroup recursive handling
// ============================================================

func TestSlogHandlerGroupAttr(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "logs", "app")

	l := MustNew(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(path),
		WithLevel("info"),
	)
	defer l.Sync()

	sl := slog.New(l.SlogHandler())
	sl.Info("group-test",
		slog.Group("request",
			slog.String("method", "POST"),
			slog.Int("status", 201),
		),
	)

	content := readLogFile(t, path+"-info.log")
	if !strings.Contains(content, "group-test") {
		t.Fatalf("slog group message not found: %s", content)
	}
	if !strings.Contains(content, "request") {
		t.Fatalf("slog group key 'request' not found: %s", content)
	}
	if !strings.Contains(content, "method") {
		t.Fatalf("slog group nested field 'method' not found: %s", content)
	}
}

// ============================================================
// BufferedWriteSyncer
// ============================================================

func TestOptionWithBuffered(t *testing.T) {
	t.Parallel()
	cfg := defaultConfig()
	if err := WithBuffered(true)(cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.buffered {
		t.Fatal("buffered not set")
	}
}

func TestOptionWithBufferSize(t *testing.T) {
	t.Parallel()
	cfg := defaultConfig()
	if err := WithBufferSize(512 * 1024)(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.bufferSize != 512*1024 {
		t.Fatalf("bufferSize = %d", cfg.bufferSize)
	}
	if err := WithBufferSize(0)(cfg); err == nil {
		t.Fatal("expected error for 0")
	}
	if err := WithBufferSize(-1)(cfg); err == nil {
		t.Fatal("expected error for -1")
	}
}

func TestOptionWithFlushInterval(t *testing.T) {
	t.Parallel()
	cfg := defaultConfig()
	if err := WithFlushInterval(10 * time.Second)(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.flushInterval != 10*time.Second {
		t.Fatalf("flushInterval = %v", cfg.flushInterval)
	}
	if err := WithFlushInterval(0)(cfg); err == nil {
		t.Fatal("expected error for 0")
	}
	if err := WithFlushInterval(-1 * time.Second)(cfg); err == nil {
		t.Fatal("expected error for negative")
	}
}

func TestBufferedWriteActuallyLandsOnDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app")

	l := MustNew(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(path),
		WithBuffered(true),
	)

	l.Info("buffered-test-msg", zap.String("key", "val"))
	l.Sync()

	content := readLogFile(t, path+"-info.log")
	if !strings.Contains(content, "buffered-test-msg") {
		t.Fatalf("buffered message not found after Sync: %s", content)
	}
	if !strings.Contains(content, `"key":"val"`) {
		t.Fatalf("buffered field not found after Sync: %s", content)
	}
}

func TestBufferedDailyWriteActuallyLandsOnDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "daily")

	l := MustNew(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(path),
		WithDivision("daily"),
		WithBuffered(true),
	)

	l.Info("buffered-daily-msg", zap.String("mode", "daily"))
	l.Sync()

	today := time.Now().Format(time.DateOnly)
	content := readLogFile(t, path+"-info-"+today+".log")
	if !strings.Contains(content, "buffered-daily-msg") {
		t.Fatalf("buffered daily message not found: %s", content)
	}
}

func TestBufferedChannelWriteActuallyLandsOnDisk(t *testing.T) {
	dir := t.TempDir()
	defaultPath := filepath.Join(dir, "default", "app")
	channelPath := filepath.Join(dir, "channels", "order", "order")

	l := MustNew(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(defaultPath),
		WithBuffered(true),
		WithChannel("order",
			WithChannelPath(channelPath),
			WithChannelDuplicateToDefault(false),
		),
	)

	l.Channel("order").Info("buffered-channel-msg", zap.String("order_id", "A100"))
	l.Sync()

	content := readLogFile(t, channelPath+"-info.log")
	if !strings.Contains(content, "buffered-channel-msg") {
		t.Fatalf("buffered channel message not found: %s", content)
	}
	if !strings.Contains(content, `"channel":"order"`) {
		t.Fatalf("buffered channel field not found: %s", content)
	}
}

func TestStopCloserClosesUnderlyingWriter(t *testing.T) {
	t.Parallel()

	var closed atomic.Bool
	underlying := closerFunc(func() error {
		closed.Store(true)
		return nil
	})

	ws := zapcore.AddSync(underlying)
	bws := &zapcore.BufferedWriteSyncer{
		WS:            ws,
		Size:          4096,
		FlushInterval: time.Minute,
	}

	sc := &stopCloser{bws: bws, underlying: underlying}
	if err := sc.Close(); err != nil {
		t.Fatal(err)
	}
	if !closed.Load() {
		t.Fatal("underlying writer was not closed by stopCloser")
	}
}

// closerFunc is an io.WriteCloser backed by a function, for testing.
type closerFunc func() error

func (f closerFunc) Write(p []byte) (int, error) { return len(p), nil }
func (f closerFunc) Close() error                { return f() }

func TestBufferedWithCustomSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app")

	l, err := New(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(path),
		WithBuffered(true),
		WithBufferSize(128*1024),
		WithFlushInterval(5*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}

	l.Info("custom-buffer-msg")
	l.Sync()

	content := readLogFile(t, path+"-info.log")
	if !strings.Contains(content, "custom-buffer-msg") {
		t.Fatalf("message not found: %s", content)
	}
}

// ============================================================
// Dynamic channel cache limit
// ============================================================

func TestDynamicChannelCacheLimit(t *testing.T) {
	l := MustNew(WithConsole(false), WithFile(false))
	defer l.Sync()

	// 写入超过上限的动态 channel
	for i := range maxDynamicChannels + 100 {
		name := fmt.Sprintf("dyn-ch-%d", i)
		l.Channel(name).Info("test")
	}

	if got := l.state.dynamicChannelBasesCnt.Load(); got > maxDynamicChannels {
		t.Fatalf("dynamic channel count = %d, should not exceed %d", got, maxDynamicChannels)
	}
}

func TestSugarCtxMethods_InjectFields(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "logs", "app")

	ctxFn := func(ctx context.Context) []zap.Field {
		if v, ok := ctx.Value(ctxKey{}).(string); ok {
			return []zap.Field{zap.String("trace_id", v)}
		}
		return nil
	}

	l := MustNew(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(path),
		WithLevel("debug"),
		WithContextFields(ctxFn),
	)
	defer l.Sync()

	ctx := context.WithValue(context.Background(), ctxKey{}, "trace-xyz")
	l.DebugwCtx(ctx, "debug-w-ctx", "biz", "B1")
	l.InfowCtx(ctx, "info-w-ctx", "biz", "B2")
	l.WarnwCtx(ctx, "warn-w-ctx", "biz", "B3")
	l.ErrorwCtx(ctx, "error-w-ctx", "biz", "B4")

	content := readLogFile(t, path+"-debug.log")
	for _, msg := range []string{"debug-w-ctx", "info-w-ctx", "warn-w-ctx", "error-w-ctx"} {
		if !strings.Contains(content, msg) {
			t.Fatalf("log missing message %q: %s", msg, content)
		}
	}
	for _, kv := range []string{`"trace_id":"trace-xyz"`, `"biz":"B1"`, `"biz":"B2"`, `"biz":"B3"`, `"biz":"B4"`} {
		if !strings.Contains(content, kv) {
			t.Fatalf("log missing %q in:\n%s", kv, content)
		}
	}
}

func TestSugarCtxMethods_NoContextFields(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "logs", "app")

	l := MustNew(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(path),
		WithLevel("debug"),
	)
	defer l.Sync()

	l.InfowCtx(context.Background(), "no-ctx-fn", "biz", "OK")

	content := readLogFile(t, path+"-debug.log")
	if !strings.Contains(content, "no-ctx-fn") {
		t.Fatalf("log missing message: %s", content)
	}
	if !strings.Contains(content, `"biz":"OK"`) {
		t.Fatalf("log missing biz field: %s", content)
	}
	if strings.Contains(content, "trace_id") {
		t.Fatalf("log should not contain trace_id when contextFields not configured: %s", content)
	}
}

func TestWarnIfAndConditionalLoggers(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "logs", "app")

	ctxFn := func(ctx context.Context) []zap.Field {
		if v, ok := ctx.Value(ctxKey{}).(string); ok {
			return []zap.Field{zap.String("trace_id", v)}
		}
		return nil
	}

	l := MustNew(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(path),
		WithLevel("debug"),
		WithContextFields(ctxFn),
	)
	defer l.Sync()

	// nil err 全部 no-op
	l.WarnIf(nil)
	l.LogIfCtx(context.Background(), nil)
	l.WarnIfCtx(context.Background(), nil)

	l.WarnIf(errors.New("warn-only-error"))

	ctx := context.WithValue(context.Background(), ctxKey{}, "trc-cond")
	l.LogIfCtx(ctx, errors.New("err-with-ctx"))
	l.WarnIfCtx(ctx, errors.New("warn-with-ctx"))

	content := readLogFile(t, path+"-debug.log")
	for _, msg := range []string{"warn-only-error", "err-with-ctx", "warn-with-ctx"} {
		if !strings.Contains(content, msg) {
			t.Fatalf("log missing error message %q in:\n%s", msg, content)
		}
	}
	traceCount := strings.Count(content, `"trace_id":"trc-cond"`)
	if traceCount != 2 {
		t.Fatalf("expected trace_id in 2 ctx-aware entries, got %d in:\n%s", traceCount, content)
	}
	errCount := strings.Count(content, `"error":`)
	if errCount != 3 {
		t.Fatalf("expected 3 error entries (nil err is no-op), got %d in:\n%s", errCount, content)
	}
}

// ---------------------------------------------------------------------------
// 路径冲突校验测试（覆盖新加的 validateChannelRoutes）
// ---------------------------------------------------------------------------

func TestValidateChannelRoutes_ChannelOverlapsDefaultWithoutDuplicate(t *testing.T) {
	dir := t.TempDir()
	sharedPath := filepath.Join(dir, "app")

	cfg := defaultConfig()
	cfg.path = sharedPath
	cfg.channels = map[string]*channelConfig{
		"order": {path: sharedPath, duplicateToDefault: false},
	}

	if err := validateChannelRoutes(cfg); err == nil {
		t.Fatal("expected error when channel.path overlaps default path with duplicate=false")
	} else if !strings.Contains(err.Error(), "overlaps default path") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestValidateChannelRoutes_ChannelOverlapsDefaultWithDuplicate(t *testing.T) {
	dir := t.TempDir()
	sharedPath := filepath.Join(dir, "app")

	cfg := defaultConfig()
	cfg.path = sharedPath
	cfg.channels = map[string]*channelConfig{
		"order": {path: sharedPath, duplicateToDefault: true},
	}

	if err := validateChannelRoutes(cfg); err == nil {
		t.Fatal("expected error when channel.path overlaps default with duplicate=true")
	} else if !strings.Contains(err.Error(), "must differ from default path") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestValidateChannelRoutes_TwoChannelsSamePath(t *testing.T) {
	dir := t.TempDir()
	conflictPath := filepath.Join(dir, "channels", "shared")

	cfg := defaultConfig()
	cfg.path = filepath.Join(dir, "default", "app")
	cfg.channels = map[string]*channelConfig{
		"order": {path: conflictPath, duplicateToDefault: false},
		"audit": {path: conflictPath, duplicateToDefault: false},
	}

	err := validateChannelRoutes(cfg)
	if err == nil {
		t.Fatal("expected error when two channels share the same path")
	}
	if !strings.Contains(err.Error(), "conflicts with channel") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestValidateChannelRoutes_DistinctPathsOK(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultConfig()
	cfg.path = filepath.Join(dir, "default", "app")
	cfg.channels = map[string]*channelConfig{
		"order": {path: filepath.Join(dir, "ch", "order"), duplicateToDefault: true},
		"audit": {path: filepath.Join(dir, "ch", "audit"), duplicateToDefault: false},
	}

	if err := validateChannelRoutes(cfg); err != nil {
		t.Fatalf("expected no error for distinct paths, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 多 channel 并发写入竞态测试
// ---------------------------------------------------------------------------

func TestMultiChannelConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	defaultPath := filepath.Join(dir, "default", "app")
	chAPath := filepath.Join(dir, "ch_a", "a")
	chBPath := filepath.Join(dir, "ch_b", "b")
	chCPath := filepath.Join(dir, "ch_c", "c")

	l := MustNew(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(defaultPath),
		WithChannel("a", WithChannelPath(chAPath), WithChannelDuplicateToDefault(false)),
		WithChannel("b", WithChannelPath(chBPath), WithChannelDuplicateToDefault(false)),
		WithChannel("c", WithChannelPath(chCPath), WithChannelDuplicateToDefault(false)),
	)
	defer l.Sync()

	const writesPerChannel = 200
	var wg sync.WaitGroup
	for _, chName := range []string{"a", "b", "c"} {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			cl := l.Channel(name)
			for i := 0; i < writesPerChannel; i++ {
				cl.Info("concurrent-multi-channel",
					zap.String("channel_tag", name),
					zap.Int("seq", i),
				)
			}
		}(chName)
	}
	wg.Wait()

	for _, p := range []string{chAPath, chBPath, chCPath} {
		content := readLogFile(t, p+"-info.log")
		lines := strings.Count(content, "concurrent-multi-channel")
		if lines != writesPerChannel {
			t.Fatalf("channel %s: expected %d lines, got %d", p, writesPerChannel, lines)
		}
	}
}

// ---------------------------------------------------------------------------
// dailyWriteSyncer 跨天并发切换竞态测试
// ---------------------------------------------------------------------------

func TestDailyWriteSyncerConcurrentCrossDayRotation(t *testing.T) {
	cfg := &Config{
		path:       filepath.Join(t.TempDir(), "daily"),
		level:      "info",
		maxSize:    1,
		maxAge:     1,
		maxBackups: 1,
	}

	dw, err := newDailyWriteSyncer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dw.Close() })

	dw.currentDate = "2000-01-01"

	const goroutines = 32
	const writesPerG = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)
	var totalBytes atomic.Int64
	for g := 0; g < goroutines; g++ {
		go func(gi int) {
			defer wg.Done()
			line := []byte(fmt.Sprintf("g%d-write\n", gi))
			for i := 0; i < writesPerG; i++ {
				n, werr := dw.Write(line)
				if werr != nil {
					t.Errorf("concurrent write err: %v", werr)
					return
				}
				totalBytes.Add(int64(n))
			}
		}(g)
	}
	wg.Wait()

	if dw.currentDate == "2000-01-01" {
		t.Fatalf("currentDate not rotated, still: %q", dw.currentDate)
	}
	if totalBytes.Load() == 0 {
		t.Fatal("no bytes written despite concurrent goroutines")
	}
}

// ---------------------------------------------------------------------------
// Buffered FlushInterval 自动刷写验证
// ---------------------------------------------------------------------------

func TestBufferedFlushIntervalAutoFlushes(t *testing.T) {
	dir := t.TempDir()
	logpath := filepath.Join(dir, "buffered")

	l := MustNew(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(logpath),
		WithBuffered(true),
		WithBufferSize(1<<20),
		WithFlushInterval(50*time.Millisecond),
	)

	l.Info("auto-flush-probe", zap.String("marker", "Z42"))

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(logpath + "-info.log")
		if err == nil && strings.Contains(string(data), "Z42") {
			l.Sync()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	l.Sync()
	data, _ := os.ReadFile(logpath + "-info.log")
	t.Fatalf("auto-flush did not occur within 2s. File content after final Sync:\n%s", string(data))
}
