package logger

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.uber.org/zap"
)

// callerRef 返回相对本调用点偏移 offset 行的 "file.go:line" 引用，供精确断言。
func callerRef(t *testing.T, offset int) string {
	t.Helper()
	_, file, line, ok := runtime.Caller(1)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return fmt.Sprintf("%s:%d", filepath.Base(file), line+offset)
}

func newMatrixLogger(t *testing.T) (*Logger, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "logs", "app")
	l := MustNew(
		WithConsole(false), WithFile(true), WithOutJSON(true),
		WithPath(path),
		WithChannel("reg",
			WithChannelPath(filepath.Join(filepath.Dir(path), "reg")),
			WithChannelDuplicateToDefault(true),
		),
		WithReplaceGlobals(false),
	)
	t.Cleanup(l.Sync)
	return l, path
}

func assertCallerAt(t *testing.T, content, marker, want string) {
	t.Helper()
	line := logLineContaining(t, content, marker)
	if !strings.Contains(line, want) {
		t.Errorf("%s: caller 应为 %s, got: %s", marker, want, line)
	}
}

// TestCallerSkipMatrixSkipFirst：skip 实例先触达各派生路径（含动态 channel 首建），
// 之后 canonical 实例的 caller 必须不被污染。
func TestCallerSkipMatrixSkipFirst(t *testing.T) {
	l, path := newMatrixLogger(t)
	skipped := l.WithCallerSkip(1)

	wantDirect := callerRef(t, 1)
	logThroughFacade(skipped, "sf-direct")
	wantWith := callerRef(t, 1)
	logThroughFacade(skipped.With(zap.String("k", "v")), "sf-with")
	wantNamed := callerRef(t, 1)
	logThroughFacade(skipped.Named("api"), "sf-named")
	wantChReg := callerRef(t, 1)
	logThroughFacade(skipped.Channel("reg"), "sf-ch-reg")
	wantChDyn := callerRef(t, 1)
	logThroughFacade(skipped.Channel("dyn"), "sf-ch-dyn") // 动态 channel 由 skip 实例首建

	// 污染检查：canonical 实例随后走同名 channel 与根路径，caller 必须指向自身调用行
	wantCanonDyn := callerRef(t, 1)
	l.Channel("dyn").Info("sf-canon-dyn")
	wantCanonReg := callerRef(t, 1)
	l.Channel("reg").Info("sf-canon-reg")
	wantCanonRoot := callerRef(t, 1)
	l.Info("sf-canon-root")

	// skip 实例的 Zap()/Sugar() 直接使用也应指向真实调用行
	wantZap := callerRef(t, 1)
	skipped.Zap().Info("sf-zap-direct")
	wantSugar := callerRef(t, 1)
	skipped.Sugar().Infof("sf-sugar-direct")

	l.Sync()
	content := readLogFile(t, path+"-info.log")

	assertCallerAt(t, content, "sf-direct", wantDirect)
	assertCallerAt(t, content, "sf-with", wantWith)
	assertCallerAt(t, content, "sf-named", wantNamed)
	assertCallerAt(t, content, "sf-ch-reg", wantChReg)
	assertCallerAt(t, content, "sf-ch-dyn", wantChDyn)
	assertCallerAt(t, content, "sf-canon-dyn", wantCanonDyn)
	assertCallerAt(t, content, "sf-canon-reg", wantCanonReg)
	assertCallerAt(t, content, "sf-canon-root", wantCanonRoot)
	assertCallerAt(t, content, "sf-zap-direct", wantZap)
	assertCallerAt(t, content, "sf-sugar-direct", wantSugar)
}

// TestCallerSkipMatrixCanonicalFirst：canonical 先建缓存（注册 + 动态），
// skip 实例随后经缓存路径派生仍必须保留偏移。
func TestCallerSkipMatrixCanonicalFirst(t *testing.T) {
	l, path := newMatrixLogger(t)

	wantCanonDyn := callerRef(t, 1)
	l.Channel("dyn").Info("cf-canon-dyn") // canonical 首建动态 channel
	wantCanonReg := callerRef(t, 1)
	l.Channel("reg").Info("cf-canon-reg")

	skipped := l.WithCallerSkip(1)
	wantChReg := callerRef(t, 1)
	logThroughFacade(skipped.Channel("reg"), "cf-ch-reg") // 命中注册缓存后仍需偏移
	wantChDyn := callerRef(t, 1)
	logThroughFacade(skipped.Channel("dyn"), "cf-ch-dyn") // 命中动态缓存后仍需偏移

	// skip 派生后 canonical 再用，二次确认无残留污染
	wantCanonDyn2 := callerRef(t, 1)
	l.Channel("dyn").Info("cf-canon-dyn2")

	l.Sync()
	content := readLogFile(t, path+"-info.log")

	assertCallerAt(t, content, "cf-canon-dyn", wantCanonDyn)
	assertCallerAt(t, content, "cf-canon-reg", wantCanonReg)
	assertCallerAt(t, content, "cf-ch-reg", wantChReg)
	assertCallerAt(t, content, "cf-ch-dyn", wantChDyn)
	assertCallerAt(t, content, "cf-canon-dyn2", wantCanonDyn2)
}

// TestCallerSkipAccumulates：偏移叠加（facade 套 facade）与 0 偏移恒等。
func TestCallerSkipAccumulates(t *testing.T) {
	l, path := newMatrixLogger(t)

	if l.WithCallerSkip(0) != l {
		t.Fatal("WithCallerSkip(0) 应返回原实例")
	}

	doubleWrapped := l.WithCallerSkip(1).WithCallerSkip(1)
	wantOuter := callerRef(t, 1)
	logThroughOuterFacade(doubleWrapped, "acc-double")

	l.Sync()
	assertCallerAt(t, readLogFile(t, path+"-info.log"), "acc-double", wantOuter)
}

// TestPackageFunctionsExactCaller：包级函数（内建 +1 偏移）的精确行号断言。
func TestPackageFunctionsExactCaller(t *testing.T) {
	resetDefaultForTest()
	l, path := newDefaultFileLogger(t)
	SetDefault(l)

	wantInfo := callerRef(t, 1)
	Info("pkg-exact-info")
	wantInfof := callerRef(t, 1)
	Infof("pkg-exact-infof %d", 1)
	wantCtx := callerRef(t, 1)
	InfoCtx(t.Context(), "pkg-exact-ctx")

	Sync()
	content := readLogFile(t, path+"-info.log")
	assertCallerAt(t, content, "pkg-exact-info", wantInfo)
	assertCallerAt(t, content, "pkg-exact-infof", wantInfof)
	assertCallerAt(t, content, "pkg-exact-ctx", wantCtx)
}
