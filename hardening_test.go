package logger

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

// initHardeningLogger 初始化写临时文件的全局 JSON logger，返回 size 模式下的 info 日志文件路径。
func initHardeningLogger(t *testing.T, opts ...Option) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app")
	base := append([]Option{
		WithFile(true), WithOutJSON(true), WithDivision("size"), WithPath(path),
	}, opts...)
	if err := New(base...); err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(Sync)
	return path + "-info.log"
}

// ============================================================
// 脱敏：多次调用取并集
// ============================================================

func TestRedactKeysAccumulateAcrossCalls(t *testing.T) {
	file := initHardeningLogger(t,
		WithRedactKeys("password"),
		WithRedactKeys("token", ""),
		WithRedactKeys(),
	)
	Info("redact-union", zap.String("password", "p-secret"), zap.String("token", "t-secret"), zap.String("user", "bob"))
	Channel("dyn").Info("redact-union-ch", zap.String("password", "p-secret"), zap.String("token", "t-secret"))
	Sync()

	content := readLogFile(t, file)
	for _, marker := range []string{"redact-union", "redact-union-ch"} {
		line := logLineContaining(t, content, marker)
		if strings.Contains(line, "p-secret") || strings.Contains(line, "t-secret") {
			t.Fatalf("%s leaked secret: %s", marker, line)
		}
	}
	if !strings.Contains(content, `"user":"bob"`) {
		t.Fatalf("non-sensitive field altered: %s", content)
	}
}

func TestRedactKeysUnionAppliesToHooks(t *testing.T) {
	msg := newChanMessager(1)
	initHardeningLogger(t, WithMessager(msg), WithRedactKeys("password"), WithRedactKeys("token"))
	HInfo("hook-redact", zap.String("password", "p1"), zap.String("token", "t1"))
	Sync()
	hook := <-msg.msgs
	if strings.Contains(hook, "p1") || strings.Contains(hook, "t1") {
		t.Fatalf("hook message leaked secret: %s", hook)
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
		{"no output", []Option{WithConsole(false), WithFile(false)}, "no output enabled"},
		{"duplicate channel", []Option{
			WithPath(filepath.Join(dir, "d1")),
			WithChannel("x", WithChannelPath(filepath.Join(dir, "x1"))),
			WithChannel(" x ", WithChannelPath(filepath.Join(dir, "x2"))),
		}, `channel "x" registered more than once`},
		{"invalid stacktrace level", []Option{WithStacktraceLevel("verbose")}, `invalid stacktrace level "verbose"`},
		{"zero drain timeout", []Option{WithMessagerDrainTimeout(0)}, "messagerDrainTimeout must be > 0"},
		{"negative drain timeout", []Option{WithMessagerDrainTimeout(-time.Second)}, "messagerDrainTimeout must be > 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(tt.opts...)
			if err == nil {
				Sync()
				t.Fatal("New returned nil error")
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Fatalf("error = %q, want substring %q", err, tt.wantSub)
			}
		})
	}
}

// TestFailedNewKeepsPreviousLogger 失败的 New 不得替换或关闭当前生效的 logger。
func TestFailedNewKeepsPreviousLogger(t *testing.T) {
	file := initHardeningLogger(t)
	if err := New(WithConsole(false), WithFile(false)); err == nil {
		t.Fatal("New returned nil error")
	}
	Info("still-alive")
	Sync()
	logLineContaining(t, readLogFile(t, file), "still-alive")
}

func TestNewOptionErrorHasSinglePrefixAndChain(t *testing.T) {
	err := New(WithLevel("bogus"))
	if err == nil {
		t.Fatal("New returned nil error")
	}
	if got := strings.Count(err.Error(), "logger:"); got != 1 {
		t.Fatalf("error %q has %d 'logger:' prefixes, want 1", err, got)
	}

	sentinel := errors.New("custom option failure")
	if err := New(func(*logConfig) error { return sentinel }); !errors.Is(err, sentinel) {
		t.Fatalf("errors.Is(err, sentinel) = false, err = %v", err)
	}
}

// ============================================================
// channel 冲突按最终文件名判定
// ============================================================

func TestValidateChannelRoutesKeyedByFilename(t *testing.T) {
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
			cfg := newTestConfig()
			cfg.level = "info"
			cfg.path = tt.rootPath
			cfg.channels["ch"] = &channelConfig{path: tt.channelPath, duplicateToDefault: true}
			if err := validateChannelRoutes(cfg); (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestChannelTrailingSlashRoutesWriteDistinctFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "logs")
	if err := New(
		WithDivision("size"), WithOutJSON(true), WithPath(root+"/"),
		WithChannel("ch", WithChannelPath(root), WithChannelDuplicateToDefault(false)),
	); err != nil {
		t.Fatalf("New: %v", err)
	}
	defer Sync()
	Info("to-root")
	Channel("ch").Info("to-channel")
	Sync()

	rootContent := readLogFile(t, filepath.Join(root, "-info.log"))
	chContent := readLogFile(t, root+"-info.log")
	if !strings.Contains(rootContent, "to-root") || strings.Contains(rootContent, "to-channel") {
		t.Fatalf("root file content wrong: %s", rootContent)
	}
	if !strings.Contains(chContent, "to-channel") || strings.Contains(chContent, "to-root") {
		t.Fatalf("channel file content wrong: %s", chContent)
	}
}

func TestNoOutputFailsBeforeTouchingFilesystem(t *testing.T) {
	dir := t.TempDir()
	if err := New(WithConsole(false), WithFile(false), WithPath(filepath.Join(dir, "app"))); err == nil {
		Sync()
		t.Fatal("New returned nil error")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed New created files: %v", entries)
	}
}

// ============================================================
// Messager 排空有界
// ============================================================

func TestSyncDrainIsBounded(t *testing.T) {
	blocker := &blockingMessager{block: make(chan struct{}), started: make(chan struct{})}
	t.Cleanup(func() { close(blocker.block) })

	const timeout = 100 * time.Millisecond
	if err := New(
		WithConsole(true), WithFile(false),
		WithMessager(blocker), WithMessagerQueueSize(8), WithMessagerDrainTimeout(timeout),
	); err != nil {
		t.Fatalf("New: %v", err)
	}
	state := snapshotLoggerState()
	HInfo("first")
	<-blocker.started
	for range 3 {
		HInfo("queued")
	}

	done := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		Sync()
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
	if got := state.asyncMsg.dropped.Load(); got != 3 {
		t.Fatalf("dropped = %d, want 3", got)
	}
}

func TestSyncDrainCompletesQueuedPushes(t *testing.T) {
	msg := newChanMessager(16)
	if err := New(WithConsole(true), WithFile(false), WithMessager(msg), WithMessagerDrainTimeout(2*time.Second)); err != nil {
		t.Fatalf("New: %v", err)
	}
	state := snapshotLoggerState()
	for range 10 {
		HInfo("drain-ok")
	}
	Sync()
	if got := len(msg.msgs); got != 10 {
		t.Fatalf("delivered %d pushes before Sync returned, want 10", got)
	}
	if got := state.asyncMsg.dropped.Load(); got != 0 {
		t.Fatalf("dropped = %d, want 0", got)
	}
}

// TestReconfigureWithHungMessagerIsBounded 重配置会关闭旧 state 的推送队列，同样必须有界。
func TestReconfigureWithHungMessagerIsBounded(t *testing.T) {
	blocker := &blockingMessager{block: make(chan struct{}), started: make(chan struct{})}
	t.Cleanup(func() { close(blocker.block) })
	if err := New(WithConsole(true), WithFile(false), WithMessager(blocker), WithMessagerDrainTimeout(50*time.Millisecond)); err != nil {
		t.Fatalf("New: %v", err)
	}
	HInfo("block")
	<-blocker.started

	done := make(chan error, 1)
	go func() { done <- New(WithConsole(true), WithFile(false)) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("reconfigure: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reconfigure blocked on hung messager")
	}
	Sync()
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
			file := initHardeningLogger(t, WithStacktraceLevel(tt.level))
			Warn("st-warn")
			Error("st-error")
			Channel("dyn").Error("st-ch-error")
			Sync()
			content := readLogFile(t, file)
			if got := strings.Contains(logLineContaining(t, content, "st-error"), `"stacktrace"`); got != tt.errTrace {
				t.Fatalf("error has stacktrace = %v, want %v", got, tt.errTrace)
			}
			if got := strings.Contains(logLineContaining(t, content, "st-ch-error"), `"stacktrace"`); got != tt.errTrace {
				t.Fatalf("channel error has stacktrace = %v, want %v", got, tt.errTrace)
			}
			if got := strings.Contains(logLineContaining(t, content, "st-warn"), `"stacktrace"`); got != tt.warnTrace {
				t.Fatalf("warn has stacktrace = %v, want %v", got, tt.warnTrace)
			}
		})
	}
}

// ============================================================
// SetLevel 错误返回
// ============================================================

func TestSetLevelReturnsErrorAndKeepsLevel(t *testing.T) {
	initHardeningLogger(t, WithLevel("warn"))
	for _, bad := range []string{"", "bogus", "INFO", " info"} {
		if err := SetLevel(bad); err == nil || !strings.Contains(err.Error(), "invalid level") {
			t.Fatalf("SetLevel(%q) error = %v", bad, err)
		}
		if got := GetLevel(); got != "warn" {
			t.Fatalf("level after SetLevel(%q) = %q, want warn", bad, got)
		}
	}
	for name := range levelMap {
		if err := SetLevel(name); err != nil {
			t.Fatalf("SetLevel(%q): %v", name, err)
		}
		if got := GetLevel(); got != name {
			t.Fatalf("GetLevel after SetLevel(%q) = %q", name, got)
		}
	}
}

// ============================================================
// ChannelLogger 派生缓存
// ============================================================

func TestChannelLoggerDeriveIsCachedPerState(t *testing.T) {
	initHardeningLogger(t)
	cl := Channel("order").Named("api").With(zap.String("k", "v"))
	state := snapshotLoggerState()
	first := cl.derive(state)
	if second := cl.derive(state); second != first {
		t.Fatal("derive rebuilt logger for the same state")
	}
}

// TestChannelLoggerCacheInvalidatedOnReconfigure 反证：缓存若不按 state 失效，
// 重配置后写入会落到已关闭的旧 state，新文件里找不到日志。
func TestChannelLoggerCacheInvalidatedOnReconfigure(t *testing.T) {
	initHardeningLogger(t)
	cl := Channel("order").Named("api").With(zap.String("k", "v"))
	cl.Info("before-reconfigure")

	newFile := initHardeningLogger(t)
	cl.Info("after-reconfigure")
	Sync()

	line := logLineContaining(t, readLogFile(t, newFile), "after-reconfigure")
	for _, want := range []string{`"channel":"order"`, `"logger":"api"`, `"k":"v"`} {
		if !strings.Contains(line, want) {
			t.Fatalf("derived context %s lost after reconfigure: %s", want, line)
		}
	}
}

func TestChannelLoggerDerivedValuesAreIndependent(t *testing.T) {
	file := initHardeningLogger(t)
	parent := Channel("order").With(zap.String("scope", "parent"))
	parent.Info("warm-cache")
	child := parent.With(zap.String("extra", "child"))
	child.Info("child-line")
	parent.Info("parent-line")
	Sync()

	content := readLogFile(t, file)
	if line := logLineContaining(t, content, "child-line"); !strings.Contains(line, `"extra":"child"`) {
		t.Fatalf("child missing its field (inherited stale cache?): %s", line)
	}
	if line := logLineContaining(t, content, "parent-line"); strings.Contains(line, "extra") {
		t.Fatalf("parent picked up child field: %s", line)
	}
}

func TestChannelLoggerConcurrentUseAcrossReconfigure(t *testing.T) {
	initHardeningLogger(t)
	cl := Channel("order").Named("api").With(zap.String("k", "v"))

	var stop atomic.Bool
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for !stop.Load() {
				cl.Info("concurrent")
			}
		})
	}
	for range 10 {
		initHardeningLogger(t)
	}
	stop.Store(true)
	wg.Wait()

	file := initHardeningLogger(t)
	cl.Info("final")
	Sync()
	logLineContaining(t, readLogFile(t, file), "final")
}
