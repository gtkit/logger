package logger

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"go.uber.org/zap"
)

// resetDefaultForTest 清空默认实例状态；默认实例是进程级单例，
// 相关测试串行执行（不加 t.Parallel）以避免互相污染。
func resetDefaultForTest() {
	defaultHolder.Store(nil)
}

func newDefaultFileLogger(t *testing.T, opts ...Option) (*Logger, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "logs", "app")
	base := make([]Option, 0, 5+len(opts))
	base = append(base,
		WithConsole(false),
		WithFile(true),
		WithOutJSON(true),
		WithPath(path),
		WithReplaceGlobals(false),
	)
	l := MustNew(append(base, opts...)...)
	t.Cleanup(l.Sync)
	return l, path
}

func TestSetDefaultRoutesPackageFunctions(t *testing.T) {
	resetDefaultForTest()
	l, path := newDefaultFileLogger(t)
	SetDefault(l)

	Info("pkg-info-check", zap.String("k", "v"))
	Warnf("pkg-warnf-check %d", 42)
	Errorw("pkg-errorw-check", "key", "val")
	LogIf(errors.New("pkg-logif-check"))
	Sync()

	content := readLogFile(t, path+"-info.log")
	for _, msg := range []string{"pkg-info-check", "pkg-warnf-check 42", "pkg-errorw-check", "pkg-logif-check"} {
		if !strings.Contains(content, msg) {
			t.Errorf("默认实例日志缺少 %q\n---\n%s", msg, content)
		}
	}

	if Default() != l {
		t.Fatal("Default() 应返回 SetDefault 设定的实例")
	}
}

func TestPackageFunctionsReportUserCaller(t *testing.T) {
	resetDefaultForTest()
	l, path := newDefaultFileLogger(t)
	SetDefault(l)

	Info("caller-info-check")
	Infof("caller-infof-check")
	Infow("caller-infow-check")
	InfoCtx(t.Context(), "caller-infoctx-check")
	Sync()

	content := readLogFile(t, path+"-info.log")
	for _, msg := range []string{
		"caller-info-check",
		"caller-infof-check",
		"caller-infow-check",
		"caller-infoctx-check",
	} {
		line := logLineContaining(t, content, msg)
		if !strings.Contains(line, "default_test.go") {
			t.Errorf("%s: 包级函数的 caller 应指向调用方测试文件, got: %s", msg, line)
		}
	}
}

func TestDefaultFallbackIsConsoleOnlyAndKeepsZapGlobals(t *testing.T) {
	resetDefaultForTest()

	before := zap.L()
	fb := Default()
	if fb == nil {
		t.Fatal("未 SetDefault 时 Default() 应返回兜底实例而非 nil")
	}
	if zap.L() != before {
		t.Fatal("兜底实例不得替换 zap 全局 logger")
	}
	// 包级函数经兜底实例调用不 panic（输出到控制台）
	Info("fallback-write-check")

	// 兜底之后 SetDefault 仍可接管
	l, path := newDefaultFileLogger(t)
	SetDefault(l)
	Info("takeover-check")
	Sync()
	if !strings.Contains(readLogFile(t, path+"-info.log"), "takeover-check") {
		t.Fatal("SetDefault 应可从兜底实例接管默认输出")
	}
}

func TestPackageCtxFunctionsInjectContextFields(t *testing.T) {
	resetDefaultForTest()

	type ridKey struct{}
	l, path := newDefaultFileLogger(t, WithContextFields(func(ctx context.Context) []zap.Field {
		if id, ok := ctx.Value(ridKey{}).(string); ok && id != "" {
			return []zap.Field{zap.String("request_id", id)}
		}
		return nil
	}))
	SetDefault(l)

	ctx := context.WithValue(t.Context(), ridKey{}, "req-777")
	InfoCtx(ctx, "ctx-fields-check")
	ErrorwCtx(ctx, "ctx-errorw-check", "key", "val")
	WarnIfCtx(ctx, errors.New("ctx-warnif-check"))
	Sync()

	content := readLogFile(t, path+"-info.log")
	for _, msg := range []string{"ctx-fields-check", "ctx-errorw-check", "ctx-warnif-check"} {
		line := logLineContaining(t, content, msg)
		if !strings.Contains(line, "req-777") {
			t.Errorf("%s: 应携带 ctx 注入的 request_id, got: %s", msg, line)
		}
	}
}

func TestSetDefaultConcurrentWithPackageFunctions(t *testing.T) {
	resetDefaultForTest()
	l1, _ := newDefaultFileLogger(t)
	l2, _ := newDefaultFileLogger(t)
	SetDefault(l1)

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for range 50 {
				Info("concurrent-write")
			}
		}()
		go func() {
			defer wg.Done()
			for range 25 {
				SetDefault(l1)
				SetDefault(l2)
			}
		}()
	}
	wg.Wait()
}

func TestSetDefaultIgnoresNil(t *testing.T) {
	resetDefaultForTest()
	l, _ := newDefaultFileLogger(t)
	SetDefault(l)
	SetDefault(nil)
	if Default() != l {
		t.Fatal("SetDefault(nil) 应被忽略")
	}
}

func TestWithCallerSkipDerivedFacade(t *testing.T) {
	resetDefaultForTest()
	l, path := newDefaultFileLogger(t)
	facade := l.WithCallerSkip(1)

	logViaFacade := func(msg string) { facade.Info(msg) }
	logViaFacade("facade-caller-check")
	l.Sync()

	line := logLineContaining(t, readLogFile(t, path+"-info.log"), "facade-caller-check")
	if !strings.Contains(line, "default_test.go") {
		t.Errorf("WithCallerSkip(1) 经一层转发后 caller 应指向本测试文件, got: %s", line)
	}
	if l.WithCallerSkip(0) != l {
		t.Error("WithCallerSkip(0) 应返回原实例")
	}
}

func TestWithReplaceGlobalsFalseDoesNotTouchZapGlobals(t *testing.T) {
	resetDefaultForTest()

	before := zap.L()
	l := MustNew(WithConsole(true), WithFile(false), WithReplaceGlobals(false))
	t.Cleanup(l.Sync)

	if zap.L() != before {
		t.Fatal("WithReplaceGlobals(false) 不得替换 zap 全局 logger")
	}
	// Undo 对未替换全局的实例应为安全 no-op
	l.Undo()
	if zap.L() != before {
		t.Fatal("Undo 后 zap 全局不应变化")
	}
}

// TestResetRebuildFallbackRepeatedly 回归：重置后兜底可反复重建
// （原实现依赖 sync.Once，第二轮 Default() 曾空指针 panic）。
func TestResetRebuildFallbackRepeatedly(t *testing.T) {
	for i := range 3 {
		resetDefaultForTest()
		if Default() == nil {
			t.Fatalf("第 %d 轮重置后 Default() 不应为 nil", i+1)
		}
		Info("reset-rebuild-write")
	}
	resetDefaultForTest()
}

// TestFallbackDoesNotWriteWorkingDirectory 验证兜底实例的核心承诺：不落盘。
// 切换到临时 cwd 后触发兜底写入，断言当前目录不产生 logs/ 或任何日志文件。
func TestFallbackDoesNotWriteWorkingDirectory(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	resetDefaultForTest()

	Info("fallback-cwd-check")
	Errorf("fallback-cwd-errorf %d", 1)
	InfoCtx(ContextWithRequestID(t.Context(), "req-cwd"), "fallback-cwd-ctx")

	if _, err := os.Stat(filepath.Join(tmp, "logs")); !os.IsNotExist(err) {
		t.Fatalf("兜底实例不得在 cwd 创建 logs/ 目录: %v", err)
	}
	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".log") || strings.HasSuffix(e.Name(), ".log.gz") {
			t.Fatalf("兜底实例不得在 cwd 写日志文件: %s", e.Name())
		}
	}
}

// TestConcurrentFallbackAndSetDefaultFromNil 从 nil 状态并发触达：
// 一半 goroutine 经包级函数触发兜底 CAS，一半并发 SetDefault，
// 验证 CAS 循环无 panic、无 race、终态非 nil。
func TestConcurrentFallbackAndSetDefaultFromNil(t *testing.T) {
	resetDefaultForTest()
	l, _ := newDefaultFileLogger(t)

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for range 20 {
				Info("nil-race-write")
				_ = Default()
			}
		}()
		go func() {
			defer wg.Done()
			for range 10 {
				SetDefault(l)
			}
		}()
	}
	wg.Wait()

	if Default() == nil {
		t.Fatal("并发竞争后 Default() 不应为 nil")
	}
	resetDefaultForTest()
}

// TestFallbackWritesStderrNotStdout 兜底实例的控制台输出必须走 stderr：
// 写 stdout 会污染 CLI 的 JSON/管道/协议输出。
func TestFallbackWritesStderrNotStdout(t *testing.T) {
	resetDefaultForTest()

	origStdout, origStderr := os.Stdout, os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout, os.Stderr = wOut, wErr
	defer func() { os.Stdout, os.Stderr = origStdout, origStderr }()

	// 兜底实例在替换后的 stderr 上创建并写入
	Info("fallback-stderr-marker")

	os.Stdout, os.Stderr = origStdout, origStderr
	_ = wOut.Close()
	_ = wErr.Close()

	bufOut := make([]byte, 4096)
	nOut, _ := rOut.Read(bufOut)
	bufErr := make([]byte, 4096)
	nErr, _ := rErr.Read(bufErr)
	_ = rOut.Close()
	_ = rErr.Close()

	if !strings.Contains(string(bufErr[:nErr]), "fallback-stderr-marker") {
		t.Fatalf("兜底日志应写入 stderr, stderr=%q", string(bufErr[:nErr]))
	}
	if strings.Contains(string(bufOut[:nOut]), "fallback-stderr-marker") {
		t.Fatalf("兜底日志不得写入 stdout, stdout=%q", string(bufOut[:nOut]))
	}
	resetDefaultForTest()
}

// TestDelayedSyncDoesNotClobberSuccessorGlobals 误用缓解层（defense-in-depth）：
// 多 owner 先后安装全局属契约外的未定义行为（正例见 TestSingleOwnerContractLifecycle），
// 本用例仅锁定缓解层对「延迟关闭的旧实例立即踩掉现任」这一最常见形态的拦截；
// 链式乱序关闭的最终全局状态（如随后 B.Sync 会恢复 A 的过期视图）不作断言、不作承诺。
func TestDelayedSyncDoesNotClobberSuccessorGlobals(t *testing.T) {
	resetDefaultForTest()

	a := MustNew(WithConsole(true), WithFile(false)) // 安装全局
	b := MustNew(WithConsole(true), WithFile(false)) // 替换全局
	SetDefault(b)

	globalUnderB := zap.L()
	a.Sync() // 延迟关闭旧实例：不得把全局从 B 踩回 A 之前的状态

	if zap.L() != globalUnderB {
		t.Fatal("延迟 Sync 旧实例踩掉了现任 zap 全局")
	}

	// 单实例正常路径：全局仍属自己时 Undo 照常恢复
	before := zap.L()
	c := MustNew(WithConsole(true), WithFile(false))
	c.Undo()
	if zap.L() != before {
		t.Fatal("全局仍属自己时 Undo 应正常恢复")
	}
	b.Sync()
	resetDefaultForTest()
}

// TestSingleOwnerContractLifecycle 单 owner 契约的正例：进程内只有一个实例
// 安装 zap 全局，其余辅助实例一律 WithReplaceGlobals(false)——任意关闭顺序下
// 全局状态始终确定：owner 存活期间不变，owner 关闭后恢复为其安装前的全局。
func TestSingleOwnerContractLifecycle(t *testing.T) {
	resetDefaultForTest()

	before := zap.L()
	owner := MustNew(WithConsole(true), WithFile(false)) // 唯一 owner
	underOwner := zap.L()

	aux1 := MustNew(WithConsole(true), WithFile(false), WithReplaceGlobals(false))
	aux2 := MustNew(WithConsole(true), WithFile(false), WithReplaceGlobals(false))
	SetDefault(owner)

	aux1.Sync() // 辅助实例任意顺序关闭，不得影响全局
	if zap.L() != underOwner {
		t.Fatal("辅助实例关闭不得改变 zap 全局")
	}
	aux2.Sync()
	if zap.L() != underOwner {
		t.Fatal("辅助实例关闭不得改变 zap 全局")
	}

	owner.Sync() // owner 关闭：恢复安装前的全局
	if zap.L() != before {
		t.Fatal("owner 关闭后应恢复其安装前的 zap 全局")
	}
	resetDefaultForTest()
}
