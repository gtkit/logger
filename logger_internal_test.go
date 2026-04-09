package logger

import (
	"context"
	"fmt"
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

	cfg := &logConfig{
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

	wantDate := time.Now().Format("2006-01-02")
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

func TestHInfoIncludesFieldsInMessager(t *testing.T) {
	msg := &testMessager{}
	state := newLoggerState(zap.NewNop(), nil, nil, msg, nil, nil, zap.NewAtomicLevelAt(zapcore.DebugLevel))

	globalMu.Lock()
	previous := currentState.Load()
	currentState.Store(state)
	globalMu.Unlock()
	defer func() {
		globalMu.Lock()
		currentState.Store(previous)
		globalMu.Unlock()
	}()

	HInfo("request failed",
		zap.String("request_id", "req-1"),
		zap.Int("status", 500),
	)

	if !strings.Contains(msg.lastMsg, "request_id") || !strings.Contains(msg.lastMsg, "req-1") {
		t.Fatalf("hook message %q does not include structured fields", msg.lastMsg)
	}
}

func TestGlobalLoggerConcurrentReconfigure(t *testing.T) {
	NewZap(WithConsole(false), WithFile(false))
	defer Sync()

	var wg sync.WaitGroup
	var stop atomic.Bool

	wg.Add(1)
	go func() {
		defer wg.Done()
		for !stop.Load() {
			Info("concurrent info", zap.String("k", "v"))
		}
	}()

	for range 20 {
		NewZap(WithConsole(false), WithFile(false))
	}

	stop.Store(true)
	wg.Wait()
}

func TestChannelUnconfiguredWritesOnlyDefaultFile(t *testing.T) {
	tempDir := t.TempDir()
	defaultPath := filepath.Join(tempDir, "default", "app")
	channelPath := filepath.Join(tempDir, "channels", "order", "order")

	NewZap(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(defaultPath),
	)
	defer Sync()

	Channel("order").Info("order created", zap.String("order_id", "A100"))

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

	NewZap(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(defaultPath),
		WithChannel("order",
			WithChannelPath(channelPath),
			WithChannelDuplicateToDefault(true),
		),
	)
	defer Sync()

	Channel("order").Info("order duplicated", zap.String("order_id", "A200"))

	defaultLog := readLogFile(t, defaultPath+"-info.log")
	channelLog := readLogFile(t, channelPath+"-info.log")

	for _, content := range []string{defaultLog, channelLog} {
		if !strings.Contains(content, "order duplicated") {
			t.Fatalf("log does not contain duplicated message: %s", content)
		}
		if !strings.Contains(content, `"channel":"order"`) {
			t.Fatalf("log does not contain channel field: %s", content)
		}
		if !strings.Contains(content, `"order_id":"A200"`) {
			t.Fatalf("log does not contain structured field: %s", content)
		}
	}
}

func TestChannelConfiguredCanWriteOnlyChannelFile(t *testing.T) {
	tempDir := t.TempDir()
	defaultPath := filepath.Join(tempDir, "default", "app")
	channelPath := filepath.Join(tempDir, "channels", "audit", "audit")

	NewZap(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(defaultPath),
		WithChannel("audit",
			WithChannelPath(channelPath),
			WithChannelDuplicateToDefault(false),
		),
	)
	defer Sync()

	Info("default event", zap.String("scope", "default"))
	Channel("audit").Warn("audit only", zap.String("actor", "system"))

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

func TestStoredChannelLoggerFollowsReconfigure(t *testing.T) {
	tempDir := t.TempDir()
	firstDefaultPath := filepath.Join(tempDir, "first", "app")
	firstChannelPath := filepath.Join(tempDir, "first", "channels", "order")
	secondDefaultPath := filepath.Join(tempDir, "second", "app")
	secondChannelPath := filepath.Join(tempDir, "second", "channels", "order")

	NewZap(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(firstDefaultPath),
		WithChannel("order",
			WithChannelPath(firstChannelPath),
			WithChannelDuplicateToDefault(true),
		),
	)

	orderLog := Channel("order").Named("api").With(zap.String("request_id", "req-1"))
	orderLog.Info("before reconfigure", zap.String("order_id", "A100"))

	NewZap(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(secondDefaultPath),
		WithChannel("order",
			WithChannelPath(secondChannelPath),
			WithChannelDuplicateToDefault(true),
		),
	)
	defer Sync()

	orderLog.Info("after reconfigure", zap.String("order_id", "A200"))

	secondDefaultLog := readLogFile(t, secondDefaultPath+"-info.log")
	secondChannelLog := readLogFile(t, secondChannelPath+"-info.log")

	for _, content := range []string{secondDefaultLog, secondChannelLog} {
		if !strings.Contains(content, "after reconfigure") {
			t.Fatalf("reconfigured log missing message: %s", content)
		}
		if !strings.Contains(content, `"channel":"order"`) {
			t.Fatalf("reconfigured log missing channel field: %s", content)
		}
		if !strings.Contains(content, `"request_id":"req-1"`) {
			t.Fatalf("reconfigured log missing preserved context: %s", content)
		}
		if !strings.Contains(content, `"logger":"api"`) {
			t.Fatalf("reconfigured log missing preserved logger name: %s", content)
		}
	}
}

func TestCallerSkipReportsUserCode(t *testing.T) {
	dir := t.TempDir()
	logpath := filepath.Join(dir, "test")

	NewZap(
		WithPath(logpath),
		WithFile(true),
		WithConsole(false),
		WithOutJSON(false),
	)
	defer Sync()

	Info("caller-check") // this line should appear in caller info

	data, err := os.ReadFile(logpath + "-info.log")
	if err != nil {
		t.Fatal(err)
	}
	line := string(data)

	if !strings.Contains(line, "logger_internal_test.go") {
		t.Errorf("caller should reference test file, got: %s", line)
	}
	if strings.Contains(line, "logger.go") || strings.Contains(line, "state.go") {
		t.Errorf("caller should not reference internal files, got: %s", line)
	}
}

func TestCallerSkipChannelReportsUserCode(t *testing.T) {
	dir := t.TempDir()
	defaultPath := filepath.Join(dir, "default", "app")
	channelPath := filepath.Join(dir, "channels", "pay")

	NewZap(
		WithPath(defaultPath),
		WithFile(true),
		WithConsole(false),
		WithOutJSON(false),
		WithChannel("pay",
			WithChannelPath(channelPath),
			WithChannelDuplicateToDefault(false),
		),
	)
	defer Sync()

	Channel("pay").Info("pay-check") // this line should appear in caller info

	data, err := os.ReadFile(channelPath + "-info.log")
	if err != nil {
		t.Fatal(err)
	}
	line := string(data)

	if !strings.Contains(line, "logger_internal_test.go") {
		t.Errorf("channel caller should reference test file, got: %s", line)
	}
	if strings.Contains(line, "channel.go") || strings.Contains(line, "state.go") {
		t.Errorf("channel caller should not reference internal files, got: %s", line)
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

// ---------------------------------------------------------------------------
// helpers for messager-based tests
// ---------------------------------------------------------------------------

// installTestState swaps in a loggerState with the given messager and returns a
// cleanup function that restores the previous state.
func installTestState(t *testing.T, msg Messager) {
	t.Helper()
	state := newLoggerState(zap.NewNop(), nil, nil, msg, nil, nil, zap.NewAtomicLevelAt(zapcore.DebugLevel))
	globalMu.Lock()
	previous := currentState.Load()
	currentState.Store(state)
	globalMu.Unlock()
	t.Cleanup(func() {
		globalMu.Lock()
		currentState.Store(previous)
		globalMu.Unlock()
	})
}

// chanMessager is a testMessager that uses channels for async verification.
type chanMessager struct {
	msgs chan string
	urls chan string
}

func newChanMessager(size int) *chanMessager {
	return &chanMessager{
		msgs: make(chan string, size),
		urls: make(chan string, size),
	}
}

func (m *chanMessager) Send(msg string) {
	m.msgs <- msg
}

func (m *chanMessager) SendTo(url, msg string) {
	m.urls <- url
	m.msgs <- msg
}

// ---------------------------------------------------------------------------
// 1. formatMsg tests
// ---------------------------------------------------------------------------

func TestFormatMsg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		template string
		args     []any
		want     string
	}{
		{"empty args returns template", "hello world", nil, "hello world"},
		{"template with args", "count=%d", []any{42}, "count=42"},
		{"empty template single string", "", []any{"only-arg"}, "only-arg"},
		{"empty template multi args", "", []any{1, "two", 3.0}, fmt.Sprint(1, "two", 3.0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatMsg(tt.template, tt.args)
			if got != tt.want {
				t.Fatalf("formatMsg(%q, %v) = %q, want %q", tt.template, tt.args, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 2. asyncMessager tests
// ---------------------------------------------------------------------------

func TestAsyncMessagerDelivery(t *testing.T) {
	inner := newChanMessager(10)
	am := newAsyncMessager(inner, 16)
	defer am.close()

	am.Send("hello")
	am.SendTo("http://example.com", "world")

	select {
	case msg := <-inner.msgs:
		if msg != "hello" {
			t.Fatalf("got %q, want %q", msg, "hello")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for Send")
	}

	select {
	case url := <-inner.urls:
		if url != "http://example.com" {
			t.Fatalf("got url %q, want %q", url, "http://example.com")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for SendTo url")
	}

	select {
	case msg := <-inner.msgs:
		if msg != "world" {
			t.Fatalf("got %q, want %q", msg, "world")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for SendTo msg")
	}
}

func TestAsyncMessagerCloseDrainsQueue(t *testing.T) {
	inner := newChanMessager(100)
	am := newAsyncMessager(inner, 100)

	for i := range 10 {
		am.Send(fmt.Sprintf("msg-%d", i))
	}
	am.close()

	// After close, all 10 messages should have been delivered.
	if len(inner.msgs) != 10 {
		t.Fatalf("expected 10 msgs, got %d", len(inner.msgs))
	}
}

func TestAsyncMessagerQueueFullDrops(t *testing.T) {
	// blockingMessager blocks on each Send to fill the queue.
	blocker := make(chan struct{})
	inner := &blockingMessager{block: blocker, started: make(chan struct{})}
	am := newAsyncMessager(inner, 1)

	// The worker goroutine is blocked on the first message.
	am.Send("first")  // picked up by worker, worker blocks
	<-inner.started    // wait for goroutine to start processing
	am.Send("fill-queue") // fills the queue (size=1)
	am.Send("dropped")    // should be silently dropped

	close(blocker) // unblock worker
	am.close()

	// We just verify no panic/deadlock; the "dropped" message is lost.
}

type blockingMessager struct {
	block       chan struct{}
	started     chan struct{}
	startedOnce sync.Once
	count       atomic.Int64
}

func (m *blockingMessager) Send(msg string) {
	m.startedOnce.Do(func() { close(m.started) })
	<-m.block
	m.count.Add(1)
}

func (m *blockingMessager) SendTo(url, msg string) {
	m.startedOnce.Do(func() { close(m.started) })
	<-m.block
	m.count.Add(1)
}

// ---------------------------------------------------------------------------
// 3. Zlog(), Sugar() return non-nil
// ---------------------------------------------------------------------------

func TestZlogAndSugarNonNil(t *testing.T) {
	NewZap(WithConsole(false), WithFile(false))
	defer Sync()

	if Zlog() == nil {
		t.Fatal("Zlog() returned nil")
	}
	if Sugar() == nil {
		t.Fatal("Sugar() returned nil")
	}
}

// ---------------------------------------------------------------------------
// 4. ZInfo, ZWarn, ZError aliases
// ---------------------------------------------------------------------------

func TestZInfoZWarnZError(t *testing.T) {
	dir := t.TempDir()
	logpath := filepath.Join(dir, "app")

	NewZap(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(logpath),
		WithLevel("debug"),
	)
	defer Sync()

	ZInfo("zinfo-msg", zap.String("k", "v"))
	ZWarn("zwarn-msg", zap.String("k", "v"))
	ZError("zerror-msg", zap.String("k", "v"))

	content := readLogFile(t, logpath+"-debug.log")
	for _, msg := range []string{"zinfo-msg", "zwarn-msg", "zerror-msg"} {
		if !strings.Contains(content, msg) {
			t.Fatalf("log missing %q: %s", msg, content)
		}
	}
}

// ---------------------------------------------------------------------------
// 5. DPanic, Warnf, Errorf, DPanicf
// ---------------------------------------------------------------------------

func TestDPanicWarnfErrorfDPanicf(t *testing.T) {
	dir := t.TempDir()
	logpath := filepath.Join(dir, "app")

	NewZap(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(logpath),
		WithLevel("debug"),
	)
	defer Sync()

	DPanic("dpanic-msg")
	Warnf("warnf-%s", "msg")
	Errorf("errorf-%s", "msg")
	DPanicf("dpanicf-%s", "msg")

	content := readLogFile(t, logpath+"-debug.log")
	for _, msg := range []string{"dpanic-msg", "warnf-msg", "errorf-msg", "dpanicf-msg"} {
		if !strings.Contains(content, msg) {
			t.Fatalf("log missing %q: %s", msg, content)
		}
	}
}

// ---------------------------------------------------------------------------
// 6. H* package-level methods with messager
// ---------------------------------------------------------------------------

func TestHInfofSendsMessage(t *testing.T) {
	msg := &testMessager{}
	installTestState(t, msg)

	HInfof("count=%d", 42)
	if msg.lastMsg != "count=42" {
		t.Fatalf("HInfof messager got %q, want %q", msg.lastMsg, "count=42")
	}
}

func TestHInfoToSendsToURL(t *testing.T) {
	msg := &testMessager{}
	installTestState(t, msg)

	HInfoTo("http://hook.example.com", "info-to-msg", zap.Int("code", 200))
	if msg.lastURL != "http://hook.example.com" {
		t.Fatalf("HInfoTo url = %q, want %q", msg.lastURL, "http://hook.example.com")
	}
	if !strings.Contains(msg.lastMsg, "info-to-msg") {
		t.Fatalf("HInfoTo msg = %q, missing 'info-to-msg'", msg.lastMsg)
	}
}

func TestHInfoTofSendsToURL(t *testing.T) {
	msg := &testMessager{}
	installTestState(t, msg)

	HInfoTof("http://hook.example.com", "hello %s", "world")
	if msg.lastURL != "http://hook.example.com" {
		t.Fatalf("HInfoTof url = %q", msg.lastURL)
	}
	if msg.lastMsg != "hello world" {
		t.Fatalf("HInfoTof msg = %q", msg.lastMsg)
	}
}

func TestHErrorSendsMessage(t *testing.T) {
	msg := &testMessager{}
	installTestState(t, msg)

	HError("err-msg", zap.String("detail", "oops"))
	if !strings.Contains(msg.lastMsg, "err-msg") {
		t.Fatalf("HError msg = %q", msg.lastMsg)
	}
}

func TestHErrorfSendsMessage(t *testing.T) {
	msg := &testMessager{}
	installTestState(t, msg)

	HErrorf("fail: %s", "reason")
	if msg.lastMsg != "fail: reason" {
		t.Fatalf("HErrorf msg = %q", msg.lastMsg)
	}
}

func TestHErrorToSendsToURL(t *testing.T) {
	msg := &testMessager{}
	installTestState(t, msg)

	HErrorTo("http://err.example.com", "error-to-msg")
	if msg.lastURL != "http://err.example.com" {
		t.Fatalf("HErrorTo url = %q", msg.lastURL)
	}
	if !strings.Contains(msg.lastMsg, "error-to-msg") {
		t.Fatalf("HErrorTo msg = %q", msg.lastMsg)
	}
}

func TestHErrorTofSendsToURL(t *testing.T) {
	msg := &testMessager{}
	installTestState(t, msg)

	HErrorTof("http://err.example.com", "fail %d", 500)
	if msg.lastURL != "http://err.example.com" {
		t.Fatalf("HErrorTof url = %q", msg.lastURL)
	}
	if msg.lastMsg != "fail 500" {
		t.Fatalf("HErrorTof msg = %q", msg.lastMsg)
	}
}

// ---------------------------------------------------------------------------
// 7. Context methods (DebugCtx, InfoCtx, WarnCtx, ErrorCtx)
// ---------------------------------------------------------------------------

type ctxKey struct{}

func TestContextFieldsInjection(t *testing.T) {
	dir := t.TempDir()
	logpath := filepath.Join(dir, "app")

	NewZap(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(logpath),
		WithLevel("debug"),
		WithContextFields(func(ctx context.Context) []zap.Field {
			if v, ok := ctx.Value(ctxKey{}).(string); ok {
				return []zap.Field{zap.String("trace_id", v)}
			}
			return nil
		}),
	)
	defer Sync()

	ctx := context.WithValue(context.Background(), ctxKey{}, "abc-123")
	DebugCtx(ctx, "debug-ctx-msg")
	InfoCtx(ctx, "info-ctx-msg")
	WarnCtx(ctx, "warn-ctx-msg")
	ErrorCtx(ctx, "error-ctx-msg")

	content := readLogFile(t, logpath+"-debug.log")
	for _, msg := range []string{"debug-ctx-msg", "info-ctx-msg", "warn-ctx-msg", "error-ctx-msg"} {
		if !strings.Contains(content, msg) {
			t.Fatalf("log missing %q", msg)
		}
	}
	if !strings.Contains(content, "abc-123") {
		t.Fatalf("log missing trace_id value: %s", content)
	}
}

// ---------------------------------------------------------------------------
// 8. Channel methods
// ---------------------------------------------------------------------------

func TestChannelMethodsVariety(t *testing.T) {
	dir := t.TempDir()
	defaultPath := filepath.Join(dir, "default", "app")
	channelPath := filepath.Join(dir, "channels", "test", "test")

	NewZap(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(defaultPath),
		WithLevel("debug"),
		WithChannel("test",
			WithChannelPath(channelPath),
			WithChannelDuplicateToDefault(false),
		),
	)
	defer Sync()

	ch := Channel("test")

	// Zap() and Sugar() should return non-nil.
	if ch.Zap() == nil {
		t.Fatal("channel Zap() is nil")
	}
	if ch.Sugar() == nil {
		t.Fatal("channel Sugar() is nil")
	}

	ch.Debug("ch-debug")
	ch.Error("ch-error", zap.String("k", "v"))
	ch.DPanic("ch-dpanic")
	ch.Debugf("ch-debugf-%s", "val")
	ch.Infof("ch-infof-%s", "val")
	ch.Infow("ch-infow", "key", "val")
	ch.Warnf("ch-warnf-%s", "val")
	ch.Errorf("ch-errorf-%s", "val")
	ch.DPanicf("ch-dpanicf-%s", "val")
	ch.LogIf(fmt.Errorf("test-logif-error"))

	content := readLogFile(t, channelPath+"-debug.log")
	for _, msg := range []string{
		"ch-debug", "ch-error", "ch-dpanic",
		"ch-debugf-val", "ch-infof-val", "ch-infow",
		"ch-warnf-val", "ch-errorf-val", "ch-dpanicf-val",
		"test-logif-error",
	} {
		if !strings.Contains(content, msg) {
			t.Fatalf("channel log missing %q: %s", msg, content)
		}
	}
}

// ---------------------------------------------------------------------------
// 9. Channel H* methods with messager
// ---------------------------------------------------------------------------

func TestChannelHMethodsWithMessager(t *testing.T) {
	msg := &testMessager{}
	state := newLoggerState(zap.NewNop(), nil, nil, msg, nil, nil, zap.NewAtomicLevelAt(zapcore.DebugLevel))

	globalMu.Lock()
	previous := currentState.Load()
	currentState.Store(state)
	globalMu.Unlock()
	defer func() {
		globalMu.Lock()
		currentState.Store(previous)
		globalMu.Unlock()
	}()

	ch := Channel("orders")

	ch.HInfo("hinfo-msg", zap.String("id", "1"))
	if !strings.Contains(msg.lastMsg, "hinfo-msg") || !strings.Contains(msg.lastMsg, "channel") {
		t.Fatalf("HInfo msg = %q", msg.lastMsg)
	}

	ch.HInfof("hinfof %d", 42)
	if !strings.Contains(msg.lastMsg, "hinfof 42") || !strings.Contains(msg.lastMsg, "orders") {
		t.Fatalf("HInfof msg = %q", msg.lastMsg)
	}

	ch.HInfoTo("http://a.com", "hinfoto-msg")
	if msg.lastURL != "http://a.com" {
		t.Fatalf("HInfoTo url = %q", msg.lastURL)
	}

	ch.HInfoTof("http://b.com", "hinfotof %s", "val")
	if msg.lastURL != "http://b.com" {
		t.Fatalf("HInfoTof url = %q", msg.lastURL)
	}
	if !strings.Contains(msg.lastMsg, "hinfotof val") {
		t.Fatalf("HInfoTof msg = %q", msg.lastMsg)
	}

	ch.HError("herror-msg")
	if !strings.Contains(msg.lastMsg, "herror-msg") {
		t.Fatalf("HError msg = %q", msg.lastMsg)
	}

	ch.HErrorf("herrorf %d", 500)
	if !strings.Contains(msg.lastMsg, "herrorf 500") {
		t.Fatalf("HErrorf msg = %q", msg.lastMsg)
	}

	ch.HErrorTo("http://c.com", "herrorto-msg")
	if msg.lastURL != "http://c.com" {
		t.Fatalf("HErrorTo url = %q", msg.lastURL)
	}

	ch.HErrorTof("http://d.com", "herrortof %s", "err")
	if msg.lastURL != "http://d.com" {
		t.Fatalf("HErrorTof url = %q", msg.lastURL)
	}
	if !strings.Contains(msg.lastMsg, "herrortof err") {
		t.Fatalf("HErrorTof msg = %q", msg.lastMsg)
	}
}

// ---------------------------------------------------------------------------
// 10. Channel Ctx methods
// ---------------------------------------------------------------------------

func TestChannelCtxMethods(t *testing.T) {
	dir := t.TempDir()
	defaultPath := filepath.Join(dir, "default", "app")
	channelPath := filepath.Join(dir, "channels", "ctx", "ctx")

	NewZap(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(defaultPath),
		WithLevel("debug"),
		WithChannel("ctx",
			WithChannelPath(channelPath),
			WithChannelDuplicateToDefault(false),
		),
		WithContextFields(func(ctx context.Context) []zap.Field {
			if v, ok := ctx.Value(ctxKey{}).(string); ok {
				return []zap.Field{zap.String("trace_id", v)}
			}
			return nil
		}),
	)
	defer Sync()

	ctx := context.WithValue(context.Background(), ctxKey{}, "trace-xyz")
	ch := Channel("ctx")

	ch.DebugCtx(ctx, "ch-debug-ctx")
	ch.InfoCtx(ctx, "ch-info-ctx")
	ch.WarnCtx(ctx, "ch-warn-ctx")
	ch.ErrorCtx(ctx, "ch-error-ctx")

	content := readLogFile(t, channelPath+"-debug.log")
	for _, msg := range []string{"ch-debug-ctx", "ch-info-ctx", "ch-warn-ctx", "ch-error-ctx"} {
		if !strings.Contains(content, msg) {
			t.Fatalf("channel ctx log missing %q", msg)
		}
	}
	if !strings.Contains(content, "trace-xyz") {
		t.Fatalf("channel ctx log missing trace_id: %s", content)
	}
}

// ---------------------------------------------------------------------------
// 11. withChannelField, formatChannelMsg
// ---------------------------------------------------------------------------

func TestWithChannelField(t *testing.T) {
	t.Parallel()

	// empty channel returns original fields
	original := []zap.Field{zap.String("k", "v")}
	result := withChannelField("", original)
	if len(result) != 1 {
		t.Fatalf("expected 1 field, got %d", len(result))
	}

	// non-empty channel prepends channel field
	result = withChannelField("orders", original)
	if len(result) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(result))
	}
	if result[0].Key != "channel" {
		t.Fatalf("first field key = %q, want 'channel'", result[0].Key)
	}
}

func TestFormatChannelMsg(t *testing.T) {
	t.Parallel()

	if got := formatChannelMsg("", "hello"); got != "hello" {
		t.Fatalf("empty channel: got %q", got)
	}

	got := formatChannelMsg("pay", "done")
	if got != "[channel=pay] done" {
		t.Fatalf("got %q, want %q", got, "[channel=pay] done")
	}
}

// ---------------------------------------------------------------------------
// 12. Options: WithCompress, WithMaxAge, WithMaxBackups, WithMaxSize,
//     WithMessager, WithMessagerQueueSize, WithContextFields
// ---------------------------------------------------------------------------

func TestOptionWithCompress(t *testing.T) {
	t.Parallel()
	cfg := &logConfig{}
	if err := WithCompress(true)(cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.compress {
		t.Fatal("compress not set")
	}
}

func TestOptionWithMaxAge(t *testing.T) {
	t.Parallel()
	cfg := &logConfig{}
	if err := WithMaxAge(30)(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.maxAge != 30 {
		t.Fatalf("maxAge = %d", cfg.maxAge)
	}
	if err := WithMaxAge(0)(cfg); err == nil {
		t.Fatal("expected error for 0")
	}
}

func TestOptionWithMaxBackups(t *testing.T) {
	t.Parallel()
	cfg := &logConfig{}
	if err := WithMaxBackups(5)(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.maxBackups != 5 {
		t.Fatalf("maxBackups = %d", cfg.maxBackups)
	}
	if err := WithMaxBackups(-1)(cfg); err == nil {
		t.Fatal("expected error for -1")
	}
}

func TestOptionWithMaxSize(t *testing.T) {
	t.Parallel()
	cfg := &logConfig{}
	if err := WithMaxSize(100)(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.maxSize != 100 {
		t.Fatalf("maxSize = %d", cfg.maxSize)
	}
	if err := WithMaxSize(0)(cfg); err == nil {
		t.Fatal("expected error for 0")
	}
}

func TestOptionWithMessager(t *testing.T) {
	t.Parallel()
	cfg := &logConfig{}
	m := &testMessager{}
	if err := WithMessager(m)(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.messager == nil {
		t.Fatal("messager not set")
	}
}

func TestOptionWithMessagerQueueSize(t *testing.T) {
	t.Parallel()
	cfg := &logConfig{}
	if err := WithMessagerQueueSize(512)(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.messagerQueueSize != 512 {
		t.Fatalf("messagerQueueSize = %d", cfg.messagerQueueSize)
	}
	if err := WithMessagerQueueSize(0)(cfg); err == nil {
		t.Fatal("expected error for 0")
	}
}

func TestOptionWithContextFields(t *testing.T) {
	t.Parallel()
	cfg := &logConfig{}
	fn := func(ctx context.Context) []zap.Field {
		return []zap.Field{zap.String("test", "val")}
	}
	if err := WithContextFields(fn)(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.contextFields == nil {
		t.Fatal("contextFields not set")
	}
}

// ---------------------------------------------------------------------------
// 13. Undo()
// ---------------------------------------------------------------------------

func TestUndo(t *testing.T) {
	// Save the current global logger.
	before := zap.L()

	NewZap(WithConsole(false), WithFile(false))

	// After NewZap, the global logger should differ from before (replaced).
	afterNewZap := zap.L()
	if afterNewZap == before {
		t.Log("global logger unchanged after NewZap (may be nop)")
	}

	Undo()

	afterUndo := zap.L()
	// After Undo, the global logger should be restored to baseGlobalLogger.
	if afterUndo != baseGlobalLogger {
		t.Fatal("Undo did not restore global logger")
	}

	// Cleanup
	Sync()
}

// ---------------------------------------------------------------------------
// 14. Adapters
// ---------------------------------------------------------------------------

func TestCronAdapterError(t *testing.T) {
	NewZap(WithConsole(false), WithFile(false))
	defer Sync()

	a := NewCronAdapter()
	// Should not panic.
	a.Info("cron info", "key", "val")
	a.Error(fmt.Errorf("cron err"), "cron error msg", "key", "val")
}

func TestRestyAdapterMethods(t *testing.T) {
	NewZap(WithConsole(false), WithFile(false))
	defer Sync()

	a := NewRestyAdapter()
	// Should not panic.
	a.Errorf("resty error %d", 500)
	a.Warnf("resty warn %s", "test")
	a.Debugf("resty debug %s", "test")
}

func TestESAdapterPrintf(t *testing.T) {
	NewZap(WithConsole(false), WithFile(false))
	defer Sync()

	a := NewESAdapter()
	// Should not panic.
	a.Printf("es query %s", "test")
}

// ---------------------------------------------------------------------------
// Extra: NewZap with messager integration (end-to-end)
// ---------------------------------------------------------------------------

func TestNewZapWithMessager(t *testing.T) {
	dir := t.TempDir()
	logpath := filepath.Join(dir, "app")

	m := newChanMessager(10)

	NewZap(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(logpath),
		WithMessager(m),
		WithMessagerQueueSize(16),
	)
	defer Sync()

	HInfo("integrated-msg", zap.String("k", "v"))

	select {
	case got := <-m.msgs:
		if !strings.Contains(got, "integrated-msg") {
			t.Fatalf("messager got %q, want to contain 'integrated-msg'", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for messager")
	}
}

// ---------------------------------------------------------------------------
// Extra: LogIf with nil error should not log
// ---------------------------------------------------------------------------

func TestLogIfNilError(t *testing.T) {
	NewZap(WithConsole(false), WithFile(false))
	defer Sync()

	// Should not panic.
	LogIf(nil)
}

func TestLogIfWithError(t *testing.T) {
	dir := t.TempDir()
	logpath := filepath.Join(dir, "app")

	NewZap(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(logpath),
		WithLevel("debug"),
	)
	defer Sync()

	LogIf(fmt.Errorf("test-err"))

	content := readLogFile(t, logpath+"-debug.log")
	if !strings.Contains(content, "test-err") {
		t.Fatalf("LogIf did not write error: %s", content)
	}
}

// ---------------------------------------------------------------------------
// 15. Dynamic log level: SetLevel / GetLevel
// ---------------------------------------------------------------------------

func TestGetLevelDefaultIsInfo(t *testing.T) {
	NewZap(WithConsole(false), WithFile(false))
	defer Sync()

	if got := GetLevel(); got != "info" {
		t.Fatalf("GetLevel() = %q, want %q", got, "info")
	}
}

func TestSetLevelChangesGetLevel(t *testing.T) {
	NewZap(WithConsole(false), WithFile(false))
	defer Sync()

	SetLevel("debug")
	if got := GetLevel(); got != "debug" {
		t.Fatalf("after SetLevel(debug): GetLevel() = %q, want %q", got, "debug")
	}
}

func TestSetLevelAffectsLogOutput(t *testing.T) {
	dir := t.TempDir()
	logpath := filepath.Join(dir, "app")

	NewZap(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(logpath),
		WithLevel("info"),
	)
	defer Sync()

	SetLevel("error")

	Info("should-not-appear")
	Error("should-appear")

	content := readLogFile(t, logpath+"-info.log")
	if strings.Contains(content, "should-not-appear") {
		t.Fatalf("Info() wrote to file after SetLevel(error): %s", content)
	}
	if !strings.Contains(content, "should-appear") {
		t.Fatalf("Error() did not write to file after SetLevel(error): %s", content)
	}
}

func TestSetLevelInvalidNoChange(t *testing.T) {
	NewZap(WithConsole(false), WithFile(false))
	defer Sync()

	SetLevel("info")
	SetLevel("not-a-level")
	if got := GetLevel(); got != "info" {
		t.Fatalf("after invalid SetLevel: GetLevel() = %q, want %q", got, "info")
	}
}

// ---------------------------------------------------------------------------
// 16. slog bridge: SlogHandler
// ---------------------------------------------------------------------------

func TestSlogHandlerNoPanic(t *testing.T) {
	NewZap(WithConsole(false), WithFile(false))
	defer Sync()

	sl := slog.New(SlogHandler())
	if sl == nil {
		t.Fatal("slog.New(SlogHandler()) returned nil")
	}
}

func TestSlogHandlerWritesToFile(t *testing.T) {
	dir := t.TempDir()
	logpath := filepath.Join(dir, "app")

	NewZap(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(logpath),
		WithLevel("info"),
	)
	defer Sync()

	sl := slog.New(SlogHandler())
	sl.Info("slog-bridge-test", "key", "val")

	content := readLogFile(t, logpath+"-info.log")
	if !strings.Contains(content, "slog-bridge-test") {
		t.Fatalf("slog.Info did not write to underlying log file: %s", content)
	}
}

func TestSlogHandlerRespectsLevel(t *testing.T) {
	NewZap(WithConsole(false), WithFile(false), WithLevel("error"))
	defer Sync()

	h := SlogHandler()
	if h.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("SlogHandler.Enabled(Info) should be false when level is error")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Fatal("SlogHandler.Enabled(Error) should be true when level is error")
	}
}

// ---------------------------------------------------------------------------
// 17. DroppedMessages
// ---------------------------------------------------------------------------

func TestDroppedMessagesZeroWithoutMessager(t *testing.T) {
	NewZap(WithConsole(false), WithFile(false))
	defer Sync()

	if got := DroppedMessages(); got != 0 {
		t.Fatalf("DroppedMessages() = %d, want 0", got)
	}
}

func TestDroppedMessagesCountsDrops(t *testing.T) {
	dir := t.TempDir()
	logpath := filepath.Join(dir, "app")

	blocker := make(chan struct{})
	inner := &blockingMessager{block: blocker, started: make(chan struct{})}

	NewZap(
		WithConsole(false),
		WithFile(true),
		WithPath(logpath),
		WithMessager(inner),
		WithMessagerQueueSize(1),
	)

	// First message is picked up by the worker goroutine, which blocks.
	HInfo("first")
	<-inner.started // wait for goroutine to start processing

	// Second message fills the queue (size=1).
	HInfo("fill-queue")

	// Third message should be dropped.
	HInfo("dropped")

	got := DroppedMessages()
	if got < 1 {
		t.Fatalf("DroppedMessages() = %d, want >= 1", got)
	}

	close(blocker)
	Sync()
}

// ---------------------------------------------------------------------------
// 18. New() returns error instead of panicking
// ---------------------------------------------------------------------------

func TestNewReturnsNilOnSuccess(t *testing.T) {
	err := New(WithConsole(false), WithFile(false))
	if err != nil {
		t.Fatalf("New() returned unexpected error: %v", err)
	}
	defer Sync()
}

func TestNewReturnsErrorOnInvalidOption(t *testing.T) {
	err := New(WithLevel("nonexistent"))
	if err == nil {
		t.Fatal("New() should return error for invalid level")
	}
}

// ---------------------------------------------------------------------------
// 19. Debugw / Warnw / Errorw (package-level)
// ---------------------------------------------------------------------------

func TestDebugwWarnwErrorw(t *testing.T) {
	dir := t.TempDir()
	logpath := filepath.Join(dir, "app")

	NewZap(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(logpath),
		WithLevel("debug"),
	)
	defer Sync()

	Debugw("debugw-msg", "key1", "val1")
	Warnw("warnw-msg", "key2", "val2")
	Errorw("errorw-msg", "key3", "val3")

	content := readLogFile(t, logpath+"-debug.log")
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

// ---------------------------------------------------------------------------
// 20. Channel Debugw / Warnw / Errorw
// ---------------------------------------------------------------------------

func TestChannelDebugwWarnwErrorw(t *testing.T) {
	dir := t.TempDir()
	defaultPath := filepath.Join(dir, "default", "app")
	channelPath := filepath.Join(dir, "channels", "test", "test")

	NewZap(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(defaultPath),
		WithLevel("debug"),
		WithChannel("test",
			WithChannelPath(channelPath),
			WithChannelDuplicateToDefault(false),
		),
	)
	defer Sync()

	ch := Channel("test")
	ch.Debugw("ch-debugw", "k1", "v1")
	ch.Warnw("ch-warnw", "k2", "v2")
	ch.Errorw("ch-errorw", "k3", "v3")

	content := readLogFile(t, channelPath+"-debug.log")
	for _, msg := range []string{"ch-debugw", "ch-warnw", "ch-errorw"} {
		if !strings.Contains(content, msg) {
			t.Fatalf("channel log missing %q: %s", msg, content)
		}
	}
}

// ---------------------------------------------------------------------------
// 21. slog KindGroup recursive handling
// ---------------------------------------------------------------------------

func TestSlogHandlerGroupAttr(t *testing.T) {
	dir := t.TempDir()
	logpath := filepath.Join(dir, "app")

	NewZap(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(logpath),
		WithLevel("info"),
	)
	defer Sync()

	sl := slog.New(SlogHandler())
	sl.Info("group-test",
		slog.Group("request",
			slog.String("method", "POST"),
			slog.Int("status", 201),
		),
	)

	content := readLogFile(t, logpath+"-info.log")
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

// ---------------------------------------------------------------------------
// 22. Dynamic channel cache limit
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// 23. BufferedWriteSyncer
// ---------------------------------------------------------------------------

func TestOptionWithBuffered(t *testing.T) {
	t.Parallel()
	cfg := &logConfig{}
	if err := WithBuffered(true)(cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.buffered {
		t.Fatal("buffered not set")
	}
}

func TestOptionWithBufferSize(t *testing.T) {
	t.Parallel()
	cfg := &logConfig{}
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
	cfg := &logConfig{}
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
	logpath := filepath.Join(dir, "app")

	NewZap(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(logpath),
		WithBuffered(true),
	)

	Info("buffered-test-msg", zap.String("key", "val"))

	// Sync flushes the buffer to disk.
	Sync()

	content := readLogFile(t, logpath+"-info.log")
	if !strings.Contains(content, "buffered-test-msg") {
		t.Fatalf("buffered message not found after Sync: %s", content)
	}
	if !strings.Contains(content, `"key":"val"`) {
		t.Fatalf("buffered field not found after Sync: %s", content)
	}
}

func TestBufferedDailyWriteActuallyLandsOnDisk(t *testing.T) {
	dir := t.TempDir()
	logpath := filepath.Join(dir, "daily")

	NewZap(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(logpath),
		WithDivision("daily"),
		WithBuffered(true),
	)

	Info("buffered-daily-msg", zap.String("mode", "daily"))
	Sync()

	today := time.Now().Format(time.DateOnly)
	content := readLogFile(t, logpath+"-info-"+today+".log")
	if !strings.Contains(content, "buffered-daily-msg") {
		t.Fatalf("buffered daily message not found: %s", content)
	}
}

func TestBufferedChannelWriteActuallyLandsOnDisk(t *testing.T) {
	dir := t.TempDir()
	defaultPath := filepath.Join(dir, "default", "app")
	channelPath := filepath.Join(dir, "channels", "order", "order")

	NewZap(
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

	Channel("order").Info("buffered-channel-msg", zap.String("order_id", "A100"))
	Sync()

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
func (f closerFunc) Close() error                 { return f() }

func TestBufferedWithCustomSize(t *testing.T) {
	dir := t.TempDir()
	logpath := filepath.Join(dir, "app")

	err := New(
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(logpath),
		WithBuffered(true),
		WithBufferSize(128*1024),
		WithFlushInterval(5*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}

	Info("custom-buffer-msg")
	Sync()

	content := readLogFile(t, logpath+"-info.log")
	if !strings.Contains(content, "custom-buffer-msg") {
		t.Fatalf("message not found: %s", content)
	}
}

func TestDynamicChannelCacheLimit(t *testing.T) {
	NewZap(WithConsole(false), WithFile(false))
	defer Sync()

	state := snapshotLoggerState()
	if state == nil {
		t.Fatal("state is nil")
	}

	// 写入超过上限的动态 channel
	for i := range maxDynamicChannels + 100 {
		name := fmt.Sprintf("dyn-ch-%d", i)
		Channel(name).Info("test")
	}

	if got := state.dynamicChannelCnt.Load(); got > maxDynamicChannels {
		t.Fatalf("dynamic channel count = %d, should not exceed %d", got, maxDynamicChannels)
	}
}
