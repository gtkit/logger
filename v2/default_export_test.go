package logger

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
)

// TestAllPackageFunctionsEmit 全量执行 default.go 的日志类包级函数，
// 确保每个导出转发都真实可用（覆盖率不以总量数字代替导出面验证）。
// Fatal/Fatalf 因 os.Exit 语义在 TestPackageFatalFunctions 以子进程覆盖。
func TestAllPackageFunctionsEmit(t *testing.T) {
	resetDefaultForTest()
	path := filepath.Join(t.TempDir(), "logs", "app")
	l := MustNew(
		WithConsole(false), WithFile(true), WithOutJSON(true),
		WithPath(path), WithLevel("debug"), WithReplaceGlobals(false),
	)
	t.Cleanup(l.Sync)
	SetDefault(l)

	ctx := ContextWithRequestID(t.Context(), "req-smoke")
	err := errors.New("smoke-err")

	Debug("m-debug", zap.Int("i", 1))
	Info("m-info")
	Warn("m-warn")
	Error("m-error")
	DPanic("m-dpanic") // 非 development 模式：仅记录，不 panic

	Debugf("m-debugf %d", 1)
	Infof("m-infof %d", 1)
	Warnf("m-warnf %d", 1)
	Errorf("m-errorf %d", 1)
	DPanicf("m-dpanicf %d", 1)

	Debugw("m-debugw", "k", "v")
	Infow("m-infow", "k", "v")
	Warnw("m-warnw", "k", "v")
	Errorw("m-errorw", "k", "v")

	DebugCtx(ctx, "m-debugctx")
	InfoCtx(ctx, "m-infoctx")
	WarnCtx(ctx, "m-warnctx")
	ErrorCtx(ctx, "m-errorctx")

	DebugwCtx(ctx, "m-debugwctx", "k", "v")
	InfowCtx(ctx, "m-infowctx", "k", "v")
	WarnwCtx(ctx, "m-warnwctx", "k", "v")
	ErrorwCtx(ctx, "m-errorwctx", "k", "v")

	LogIf(err)
	WarnIf(err)
	LogIfCtx(ctx, err)
	WarnIfCtx(ctx, err)
	LogIf(nil)  // nil 分支：不输出
	WarnIf(nil) // nil 分支：不输出

	HInfo("m-hinfo")
	HInfof("m-hinfof %d", 1)
	HError("m-herror")
	HErrorf("m-herrorf %d", 1)
	HInfoTo("https://example.invalid/hook", "m-hinfoto")
	HInfoTof("https://example.invalid/hook", "m-hinfotof %d", 1)
	HErrorTo("https://example.invalid/hook", "m-herrorto")
	HErrorTof("https://example.invalid/hook", "m-herrortof %d", 1)

	// Panic/Panicf：recover 验证既 panic 又落日志
	func() {
		defer func() {
			if recover() == nil {
				t.Error("Panic 应触发 panic")
			}
		}()
		Panic("m-panic")
	}()
	func() {
		defer func() {
			if recover() == nil {
				t.Error("Panicf 应触发 panic")
			}
		}()
		Panicf("m-panicf %d", 1)
	}()

	Sync()
	content := readLogFile(t, path+"-debug.log")

	markers := []string{
		"m-debug", "m-info", "m-warn", "m-error", "m-dpanic",
		"m-debugf 1", "m-infof 1", "m-warnf 1", "m-errorf 1", "m-dpanicf 1",
		"m-debugw", "m-infow", "m-warnw", "m-errorw",
		"m-debugctx", "m-infoctx", "m-warnctx", "m-errorctx",
		"m-debugwctx", "m-infowctx", "m-warnwctx", "m-errorwctx",
		"m-hinfo", "m-hinfof 1", "m-herror", "m-herrorf 1",
		"m-hinfoto", "m-hinfotof 1", "m-herrorto", "m-herrortof 1",
		"m-panic", "m-panicf 1",
	}
	for _, m := range markers {
		if !strings.Contains(content, m) {
			t.Errorf("日志缺少 %q", m)
		}
	}
	// LogIf/WarnIf 家族共用 "error occurred"/"warning occurred" 消息
	if strings.Count(content, "smoke-err") != 4 {
		t.Errorf("LogIf/WarnIf(+Ctx) 应各输出一次 smoke-err，共 4 次，got %d", strings.Count(content, "smoke-err"))
	}
	// *Ctx 全家族应携带内建 request_id
	for _, m := range []string{"m-infoctx", "m-warnwctx", "m-debugctx"} {
		if line := logLineContaining(t, content, m); !strings.Contains(line, "req-smoke") {
			t.Errorf("%s: 应自动携带 request_id, got %s", m, line)
		}
	}
}

// TestPackageFatalFunctions 以子进程覆盖 Fatal/Fatalf（os.Exit(1) 语义）。
func TestPackageFatalFunctions(t *testing.T) {
	switch os.Getenv("LOGGER_V2_TEST_FATAL") {
	case "fatal":
		SetDefault(MustNew(WithConsole(true), WithFile(false), WithReplaceGlobals(false)))
		Fatal("subprocess-fatal-marker")
		return
	case "fatalf":
		SetDefault(MustNew(WithConsole(true), WithFile(false), WithReplaceGlobals(false)))
		Fatalf("subprocess-fatalf-marker %d", 1)
		return
	}

	for _, mode := range []struct {
		name   string
		marker string
	}{
		{name: "fatal", marker: "subprocess-fatal-marker"},
		{name: "fatalf", marker: "subprocess-fatalf-marker 1"},
	} {
		t.Run(mode.name, func(t *testing.T) {
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestPackageFatalFunctions$")
			cmd.Env = append(os.Environ(), "LOGGER_V2_TEST_FATAL="+mode.name)
			out, err := cmd.CombinedOutput()

			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("子进程应以非零码退出, err=%v, out=%s", err, out)
			}
			if code := exitErr.ExitCode(); code != 1 {
				t.Fatalf("退出码 = %d, want 1", code)
			}
			if !strings.Contains(string(out), mode.marker) {
				t.Fatalf("子进程输出缺少 %q:\n%s", mode.marker, out)
			}
		})
	}
}
