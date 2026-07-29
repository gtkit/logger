package logger

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestRequestIDContextRoundTrip(t *testing.T) {
	t.Parallel()

	base := t.Context()
	if got := RequestIDFromContext(base); got != "" {
		t.Fatalf("空 context 应返回空串, got %q", got)
	}
	if ctx := ContextWithRequestID(base, ""); ctx != base {
		t.Fatal("空 id 应原样返回 context")
	}

	ctx := ContextWithRequestID(base, "req-42")
	if got := RequestIDFromContext(ctx); got != "req-42" {
		t.Fatalf("RequestIDFromContext = %q, want req-42", got)
	}
}

func TestCtxMethodsAutoMergeRequestID(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "logs", "app")
	l := MustNew(
		WithConsole(false), WithFile(true), WithOutJSON(true),
		WithPath(path), WithReplaceGlobals(false),
	)
	defer l.Sync()

	ctx := ContextWithRequestID(t.Context(), "req-auto-1")
	l.InfoCtx(ctx, "auto-merge-fields")
	l.InfowCtx(ctx, "auto-merge-kv", "k", "v")
	l.LogIfCtx(ctx, errAutoMerge)
	l.InfoCtx(t.Context(), "no-request-id")
	l.Sync()

	content := readLogFile(t, path+"-info.log")
	for _, msg := range []string{"auto-merge-fields", "auto-merge-kv", "auto-merge-error"} {
		line := logLineContaining(t, content, msg)
		if !strings.Contains(line, "req-auto-1") {
			t.Errorf("%s: 未配置 WithContextFields 时 *Ctx 也应自动合并 request_id, got: %s", msg, line)
		}
	}
	if line := logLineContaining(t, content, "no-request-id"); strings.Contains(line, "request_id") {
		t.Errorf("ctx 无 requestId 时不应出现 request_id 字段: %s", line)
	}
}

var errAutoMerge = errors.New("auto-merge-error")

func TestCtxMethodsMergeRequestIDWithCustomContextFields(t *testing.T) {
	t.Parallel()

	type tenantKey struct{}
	path := filepath.Join(t.TempDir(), "logs", "app")
	l := MustNew(
		WithConsole(false), WithFile(true), WithOutJSON(true),
		WithPath(path), WithReplaceGlobals(false),
		WithContextFields(func(ctx context.Context) []zap.Field {
			if v, ok := ctx.Value(tenantKey{}).(string); ok {
				return []zap.Field{zap.String("tenant", v)}
			}
			return nil
		}),
	)
	defer l.Sync()

	ctx := ContextWithRequestID(context.WithValue(t.Context(), tenantKey{}, "acme"), "req-both")
	l.InfoCtx(ctx, "merge-both-check")
	l.Sync()

	line := logLineContaining(t, readLogFile(t, path+"-info.log"), "merge-both-check")
	if !strings.Contains(line, "req-both") || !strings.Contains(line, "acme") {
		t.Fatalf("内建 request_id 与自定义 ContextFields 应同时合并, got: %s", line)
	}
}

func TestPackageCtxFunctionsAutoMergeRequestID(t *testing.T) {
	resetDefaultForTest()
	l, path := newDefaultFileLogger(t)
	SetDefault(l)

	InfoCtx(ContextWithRequestID(t.Context(), "req-pkg-9"), "pkg-auto-merge")
	Sync()

	line := logLineContaining(t, readLogFile(t, path+"-info.log"), "pkg-auto-merge")
	if !strings.Contains(line, "req-pkg-9") {
		t.Fatalf("包级 *Ctx 函数应自动合并 request_id, got: %s", line)
	}
}
