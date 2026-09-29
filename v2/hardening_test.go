package logger

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

// newHardeningLogger 构建写临时文件的 JSON 实例，返回实例与 size 模式下的日志文件路径。
func newHardeningLogger(t *testing.T, opts ...Option) (*Logger, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app")
	base := append([]Option{
		WithFile(true), WithOutJSON(true), WithDivision("size"),
		WithPath(path), WithReplaceGlobals(false),
	}, opts...)
	l, err := New(base...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(l.Sync)
	return l, path + "-info.log"
}

// ============================================================
// 脱敏：多次调用取并集
// ============================================================

func TestRedactKeysAccumulateAcrossCalls(t *testing.T) {
	l, file := newHardeningLogger(t,
		WithRedactKeys("password"),
		WithRedactKeys("token", ""),
		WithRedactKeys(),
	)
	l.Info("redact-union",
		zap.String("password", "p-secret"),
		zap.String("token", "t-secret"),
		zap.String("user", "bob"),
	)
	l.Sync()

	line := logLineContaining(t, readLogFile(t, file), "redact-union")
	for _, secret := range []string{"p-secret", "t-secret"} {
		if strings.Contains(line, secret) {
			t.Fatalf("secret %q leaked: %s", secret, line)
		}
	}
	if got := strings.Count(line, redactedValue); got != 2 {
		t.Fatalf("redacted count = %d, want 2: %s", got, line)
	}
	if !strings.Contains(line, `"user":"bob"`) {
		t.Fatalf("non-sensitive field altered: %s", line)
	}
}

func TestRedactKeysUnionAppliesToChannelsAndHooks(t *testing.T) {
	msg := newSyncTestMessager(1)
	dir := t.TempDir()
	l := MustNew(
		WithFile(true), WithOutJSON(true), WithDivision("size"),
		WithPath(filepath.Join(dir, "app")), WithReplaceGlobals(false),
		WithMessager(msg),
		WithRedactKeys("password"), WithRedactKeys("token"),
		WithChannel("audit",
			WithChannelPath(filepath.Join(dir, "audit")),
			WithChannelDuplicateToDefault(false),
		),
	)
	l.Channel("audit").HInfo("channel-redact", zap.String("password", "p1"), zap.String("token", "t1"))
	l.Sync()

	line := logLineContaining(t, readLogFile(t, filepath.Join(dir, "audit-info.log")), "channel-redact")
	if strings.Contains(line, "p1") || strings.Contains(line, "t1") {
		t.Fatalf("channel file leaked secret: %s", line)
	}
	hook := <-msg.msgs
	if strings.Contains(hook, "p1") || strings.Contains(hook, "t1") {
		t.Fatalf("hook message leaked secret: %s", hook)
	}
}

func TestRedactKeysOnlyEmptyKeysKeepsCoreUnwrapped(t *testing.T) {
	cfg := defaultConfig()
	if err := WithRedactKeys("", "")(cfg); err != nil {
		t.Fatalf("WithRedactKeys: %v", err)
	}
	if cfg.redactKeys != nil {
		t.Fatalf("empty keys allocated redact set: %v", cfg.redactKeys)
	}
	if newFieldRedactor(cfg.redactKeys) != nil {
		t.Fatal("empty key set produced a redactor")
	}
}

// ============================================================
// Option 组合校验
// ============================================================

func TestNewRejectsInvalidOptionCombinations(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name    string
		opts    []Option
		wantSub string
	}{
		{
			name:    "no output",
			opts:    []Option{WithConsole(false), WithFile(false)},
			wantSub: "no output enabled",
		},
		{
			name: "duplicate channel",
			opts: []Option{
				WithPath(filepath.Join(dir, "d1")),
				WithChannel("x", WithChannelPath(filepath.Join(dir, "x1"))),
				WithChannel("x", WithChannelPath(filepath.Join(dir, "x2"))),
			},
			wantSub: `channel "x" registered more than once`,
		},
		{
			name: "duplicate channel after trim",
			opts: []Option{
				WithPath(filepath.Join(dir, "d2")),
				WithChannel("x", WithChannelPath(filepath.Join(dir, "y1"))),
				WithChannel("  x ", WithChannelPath(filepath.Join(dir, "y2"))),
			},
			wantSub: `channel "x" registered more than once`,
		},
		{
			name:    "invalid stacktrace level",
			opts:    []Option{WithStacktraceLevel("verbose")},
			wantSub: `invalid stacktrace level "verbose"`,
		},
		{
			name:    "zero drain timeout",
			opts:    []Option{WithMessagerDrainTimeout(0)},
			wantSub: "messagerDrainTimeout must be > 0",
		},
		{
			name:    "negative drain timeout",
			opts:    []Option{WithMessagerDrainTimeout(-time.Second)},
			wantSub: "messagerDrainTimeout must be > 0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, err := New(append([]Option{WithReplaceGlobals(false)}, tt.opts...)...)
			if err == nil {
				l.Sync()
				t.Fatal("New returned nil error")
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Fatalf("error = %q, want substring %q", err, tt.wantSub)
			}
		})
	}
}

func TestNewOptionErrorHasSinglePrefix(t *testing.T) {
	_, err := New(WithLevel("bogus"))
	if err == nil {
		t.Fatal("New(WithLevel(bogus)) returned nil error")
	}
	if got := strings.Count(err.Error(), "logger:"); got != 1 {
		t.Fatalf("error %q has %d 'logger:' prefixes, want 1", err, got)
	}
}

func TestNewOptionErrorPreservesChain(t *testing.T) {
	sentinel := errors.New("custom option failure")
	_, err := New(func(*Config) error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatalf("errors.Is(err, sentinel) = false, err = %v", err)
	}
}

// ============================================================
// channel 冲突按最终文件名判定
// ============================================================

func TestChannelRouteConflictKeyedByFilename(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name        string
		rootPath    string
		channelPath string
		wantErr     bool
	}{
		{"trailing slash differs", filepath.Join(dir, "a") + "/", filepath.Join(dir, "a"), false},
		{"identical", filepath.Join(dir, "b"), filepath.Join(dir, "b"), true},
		{"clean-equivalent", filepath.Join(dir, "c"), filepath.Join(dir, "x", "..", "c"), true},
		{"distinct", filepath.Join(dir, "d"), filepath.Join(dir, "e"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, err := New(
				WithReplaceGlobals(false), WithDivision("size"),
				WithPath(tt.rootPath),
				WithChannel("ch", WithChannelPath(tt.channelPath)),
			)
			if err == nil {
				l.Sync()
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestChannelTrailingSlashRoutesWriteDistinctFiles(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "logs")
	l := MustNew(
		WithReplaceGlobals(false), WithDivision("size"), WithOutJSON(true),
		WithPath(root+"/"),
		WithChannel("ch", WithChannelPath(root), WithChannelDuplicateToDefault(false)),
	)
	l.Info("to-root")
	l.Channel("ch").Info("to-channel")
	l.Sync()

	rootContent := readLogFile(t, filepath.Join(root, "-info.log"))
	chContent := readLogFile(t, root+"-info.log")
	if !strings.Contains(rootContent, "to-root") || strings.Contains(rootContent, "to-channel") {
		t.Fatalf("root file content wrong: %s", rootContent)
	}
	if !strings.Contains(chContent, "to-channel") || strings.Contains(chContent, "to-root") {
		t.Fatalf("channel file content wrong: %s", chContent)
	}
}

// ============================================================
// 输出全关：失败构建不得留下文件与 goroutine
// ============================================================

func TestNoOutputFailsBeforeTouchingFilesystem(t *testing.T) {
	dir := t.TempDir()
	_, err := New(
		WithReplaceGlobals(false), WithConsole(false), WithFile(false),
		WithPath(filepath.Join(dir, "app")),
		WithMessager(newSyncTestMessager(1)),
	)
	if err == nil {
		t.Fatal("New returned nil error")
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatalf("ReadDir: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("failed New created files: %v", entries)
	}
}

func TestConsoleOnlyAndFileOnlyBothSucceed(t *testing.T) {
	for _, tt := range []struct {
		name          string
		console, file bool
	}{
		{"console only", true, false},
		{"file only", false, true},
		{"both", true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			l, err := New(
				WithReplaceGlobals(false), WithConsole(tt.console), WithFile(tt.file),
				WithPath(filepath.Join(t.TempDir(), "app")),
			)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			l.Sync()
		})
	}
}

// ============================================================
// Messager 排空有界
// ============================================================

// TestSyncDrainIsBounded 反证：若 close 退回无界 <-done，Sync 永不返回，
// 外层 2s 截止触发失败而不是挂死测试进程。
func TestSyncDrainIsBounded(t *testing.T) {
	blocker := &blockingMessager{block: make(chan struct{}), started: make(chan struct{})}
	t.Cleanup(func() { close(blocker.block) }) // 释放后台推送协程，避免泄漏到后续测试

	const timeout = 100 * time.Millisecond
	l := MustNew(
		WithReplaceGlobals(false), WithConsole(true), WithFile(false),
		WithMessager(blocker), WithMessagerQueueSize(8),
		WithMessagerDrainTimeout(timeout),
	)
	l.HInfo("first") // 被 worker 取走并阻塞
	<-blocker.started
	for range 3 {
		l.HInfo("queued")
	}

	done := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		l.Sync()
		done <- time.Since(start)
	}()
	select {
	case elapsed := <-done:
		if elapsed < timeout {
			t.Fatalf("Sync returned after %v, before drain timeout %v", elapsed, timeout)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Sync did not return: messager drain is unbounded")
	}
	if got := l.DroppedMessages(); got != 3 {
		t.Fatalf("DroppedMessages = %d, want 3 (queued pushes abandoned on timeout)", got)
	}
}

func TestSyncDrainCompletesQueuedPushesWithinTimeout(t *testing.T) {
	msg := newSyncTestMessager(16)
	l := MustNew(
		WithReplaceGlobals(false), WithConsole(true), WithFile(false),
		WithMessager(msg), WithMessagerDrainTimeout(2*time.Second),
	)
	for range 10 {
		l.HInfo("drain-ok")
	}
	l.Sync()
	if got := len(msg.msgs); got != 10 {
		t.Fatalf("delivered %d pushes before Sync returned, want 10", got)
	}
	if got := l.DroppedMessages(); got != 0 {
		t.Fatalf("DroppedMessages = %d, want 0", got)
	}
}

func TestSyncDrainTimeoutIsIdempotent(t *testing.T) {
	blocker := &blockingMessager{block: make(chan struct{}), started: make(chan struct{})}
	t.Cleanup(func() { close(blocker.block) })
	l := MustNew(
		WithReplaceGlobals(false), WithConsole(true), WithFile(false),
		WithMessager(blocker), WithMessagerDrainTimeout(50*time.Millisecond),
	)
	l.HInfo("block")
	<-blocker.started
	l.HInfo("pending")

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(l.Sync)
	}
	wg.Wait()
	l.Sync()
	if got := l.DroppedMessages(); got != 1 {
		t.Fatalf("DroppedMessages = %d after repeated Sync, want 1", got)
	}
}

func TestSendAfterDrainTimeoutIsDiscarded(t *testing.T) {
	blocker := &blockingMessager{block: make(chan struct{}), started: make(chan struct{})}
	t.Cleanup(func() { close(blocker.block) })
	l := MustNew(
		WithReplaceGlobals(false), WithConsole(true), WithFile(false),
		WithMessager(blocker), WithMessagerDrainTimeout(20*time.Millisecond),
	)
	l.HInfo("block")
	<-blocker.started
	l.Sync()
	before := l.DroppedMessages()
	l.HError("after-close") // 已关闭：不得 panic，也不计入 dropped
	if got := l.DroppedMessages(); got != before {
		t.Fatalf("DroppedMessages changed after close: %d -> %d", before, got)
	}
}

// ============================================================
// stacktrace 级别
// ============================================================

func TestStacktraceLevel(t *testing.T) {
	tests := []struct {
		name      string
		level     string
		errTrace  bool
		warnTrace bool
	}{
		{"default", "", true, false},
		{"raised to fatal", "fatal", false, false},
		{"lowered to warn", "warn", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, file := newHardeningLogger(t, WithStacktraceLevel(tt.level))
			l.Warn("st-warn")
			l.Error("st-error")
			l.Sync()
			content := readLogFile(t, file)
			if got := strings.Contains(logLineContaining(t, content, "st-error"), `"stacktrace"`); got != tt.errTrace {
				t.Fatalf("error has stacktrace = %v, want %v", got, tt.errTrace)
			}
			if got := strings.Contains(logLineContaining(t, content, "st-warn"), `"stacktrace"`); got != tt.warnTrace {
				t.Fatalf("warn has stacktrace = %v, want %v", got, tt.warnTrace)
			}
		})
	}
}

func TestStacktraceLevelAppliesToChannels(t *testing.T) {
	dir := t.TempDir()
	l := MustNew(
		WithReplaceGlobals(false), WithOutJSON(true), WithDivision("size"),
		WithPath(filepath.Join(dir, "app")),
		WithStacktraceLevel("fatal"),
		WithChannel("order", WithChannelPath(filepath.Join(dir, "order")), WithChannelDuplicateToDefault(false)),
	)
	l.Channel("order").Error("ch-error")
	l.Channel("dynamic").Error("dyn-error")
	l.Sync()
	for file, marker := range map[string]string{
		filepath.Join(dir, "order-info.log"): "ch-error",
		filepath.Join(dir, "app-info.log"):   "dyn-error",
	} {
		if line := logLineContaining(t, readLogFile(t, file), marker); strings.Contains(line, `"stacktrace"`) {
			t.Fatalf("%s carries stacktrace: %s", marker, line)
		}
	}
}

// ============================================================
// SetLevel 错误返回
// ============================================================

func TestSetLevelReturnsErrorAndKeepsLevel(t *testing.T) {
	l, _ := newHardeningLogger(t, WithLevel("warn"))
	for _, bad := range []string{"", "bogus", "INFO", " info", "warning"} {
		err := l.SetLevel(bad)
		if err == nil {
			t.Fatalf("SetLevel(%q) returned nil error", bad)
		}
		if !strings.Contains(err.Error(), "invalid level") {
			t.Fatalf("SetLevel(%q) error = %q", bad, err)
		}
		if got := l.GetLevel(); got != "warn" {
			t.Fatalf("level after SetLevel(%q) = %q, want warn", bad, got)
		}
	}
	for name := range levelMap {
		if err := l.SetLevel(name); err != nil {
			t.Fatalf("SetLevel(%q): %v", name, err)
		}
		if got := l.GetLevel(); got != name {
			t.Fatalf("GetLevel after SetLevel(%q) = %q", name, got)
		}
	}
}

func TestSetLevelOnDerivedAffectsRoot(t *testing.T) {
	l, _ := newHardeningLogger(t)
	child := l.With(zap.String("k", "v")).Named("sub").Channel("dyn").WithCallerSkip(1)
	if err := child.SetLevel("error"); err != nil {
		t.Fatalf("SetLevel: %v", err)
	}
	if got := l.GetLevel(); got != "error" {
		t.Fatalf("root level = %q, want error", got)
	}
}

// ============================================================
// request_id 全方法族去重
// ============================================================

func TestRequestIDDedupAllRecordMethods(t *testing.T) {
	type call func(l *Logger, marker string)
	fieldCalls := map[string]call{
		"Debug": func(l *Logger, m string) { l.Debug(m, zap.String("request_id", "call")) },
		"Info":  func(l *Logger, m string) { l.Info(m, zap.String("request_id", "call")) },
		"Warn":  func(l *Logger, m string) { l.Warn(m, zap.String("request_id", "call")) },
		"Error": func(l *Logger, m string) { l.Error(m, zap.String("request_id", "call")) },
		"DPanic": func(l *Logger, m string) {
			l.DPanic(m, zap.String("request_id", "call"))
		},
		"Debugw": func(l *Logger, m string) { l.Debugw(m, "request_id", "call") },
		"Infow":  func(l *Logger, m string) { l.Infow(m, "request_id", "call") },
		"Warnw":  func(l *Logger, m string) { l.Warnw(m, "request_id", "call") },
		"Errorw": func(l *Logger, m string) { l.Errorw(m, "request_id", "call") },
		"InfowInlineField": func(l *Logger, m string) {
			l.Infow(m, zap.String("request_id", "call"))
		},
		"HInfo":    func(l *Logger, m string) { l.HInfo(m, zap.String("request_id", "call")) },
		"HInfoTo":  func(l *Logger, m string) { l.HInfoTo("u", m, zap.String("request_id", "call")) },
		"HError":   func(l *Logger, m string) { l.HError(m, zap.String("request_id", "call")) },
		"HErrorTo": func(l *Logger, m string) { l.HErrorTo("u", m, zap.String("request_id", "call")) },
	}

	for name, fn := range fieldCalls {
		t.Run("bound/"+name, func(t *testing.T) {
			l, file := newHardeningLogger(t, WithLevel("debug"), WithMessager(newSyncTestMessager(4)))
			fn(l.With(zap.String("request_id", "bound")), "m-"+name)
			l.Sync()
			line := logLineContaining(t, readLogFile(t, strings.Replace(file, "-info.log", "-debug.log", 1)), "m-"+name)
			if got := countKey(line, "request_id"); got != 1 {
				t.Fatalf("request_id count = %d, want 1: %s", got, line)
			}
			if !strings.Contains(line, `"request_id":"bound"`) {
				t.Fatalf("bound request_id must win: %s", line)
			}
		})
	}
}

func TestRequestIDCallSiteDuplicatesKeepLast(t *testing.T) {
	l, file := newHardeningLogger(t)
	l.Info("dup-fields",
		zap.String("request_id", "first"), zap.Int("n", 1), zap.String("request_id", "last"))
	l.Infow("dup-kv", "request_id", "first", "n", 1, zap.String("request_id", "last"))
	l.Sync()
	content := readLogFile(t, file)
	for _, marker := range []string{"dup-fields", "dup-kv"} {
		line := logLineContaining(t, content, marker)
		if countKey(line, "request_id") != 1 || !strings.Contains(line, `"request_id":"last"`) {
			t.Fatalf("%s: want single request_id=last: %s", marker, line)
		}
		if !strings.Contains(line, `"n":1`) {
			t.Fatalf("%s: unrelated field dropped: %s", marker, line)
		}
	}
}

func TestRequestIDDedupKVEdgeInputs(t *testing.T) {
	l, file := newHardeningLogger(t)
	bound := l.With(zap.String("request_id", "bound"))
	bound.Infow("kv-dangling", "request_id") // 奇数长度：悬空 key
	bound.Infow("kv-empty")
	l.Infow("kv-single", "request_id", "only")
	l.Sync()
	content := readLogFile(t, file)
	if line := logLineContaining(t, content, "kv-dangling"); countKey(line, "request_id") != 1 {
		t.Fatalf("dangling key produced duplicate: %s", line)
	}
	if line := logLineContaining(t, content, "kv-empty"); !strings.Contains(line, `"request_id":"bound"`) {
		t.Fatalf("bound id missing: %s", line)
	}
	if line := logLineContaining(t, content, "kv-single"); !strings.Contains(line, `"request_id":"only"`) {
		t.Fatalf("single id altered: %s", line)
	}
}

func TestRequestIDDedupDoesNotMutateCallerSlice(t *testing.T) {
	l, _ := newHardeningLogger(t)
	fields := []zap.Field{zap.String("request_id", "a"), zap.String("request_id", "b")}
	kv := []any{"request_id", "a", "request_id", "b"}
	l.Info("no-mutate", fields...)
	l.Infow("no-mutate", kv...)
	l.With(zap.String("request_id", "bound")).Info("no-mutate", fields...)
	if fields[0].String != "a" || fields[1].String != "b" || len(fields) != 2 {
		t.Fatalf("caller fields mutated: %+v", fields)
	}
	if len(kv) != 4 || kv[1] != "a" || kv[3] != "b" {
		t.Fatalf("caller kv mutated: %v", kv)
	}
}

func TestRequestIDSingleFieldPathIsAllocationFree(t *testing.T) {
	l := newDiscardCtxLogger()
	allocs := testing.AllocsPerRun(100, func() {
		_ = l.recordFields([]zap.Field{zap.String("request_id", "x")})
		_ = l.recordKV([]any{"k", "v"})
	})
	if allocs != 0 {
		t.Fatalf("recordFields/recordKV single-entry path allocs = %v, want 0", allocs)
	}
}

// ============================================================
// 派生链不再依赖 nil 防御：全部派生都共享同一 state
// ============================================================

func TestDerivedLoggersShareStateAndBase(t *testing.T) {
	l, _ := newHardeningLogger(t)
	derived := []*Logger{
		l.With(zap.String("a", "1")),
		l.Named("n"),
		l.Channel("dyn"),
		l.WithCallerSkip(1).With(zap.String("b", "2")).Channel("dyn2"),
	}
	for i, d := range derived {
		if d.state != l.state {
			t.Fatalf("derived[%d] has different state", i)
		}
		if d.base != l.base {
			t.Fatalf("derived[%d] has different canonical base", i)
		}
	}
}
