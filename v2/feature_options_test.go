package logger

import (
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestRedactKeysMasksValues(t *testing.T) {
	logpath := filepath.Join(t.TempDir(), "app")

	l, err := New(
		WithPath(logpath),
		WithFile(true),
		WithConsole(false),
		WithOutJSON(true),
		WithRedactKeys("password", "token"),
	)
	if err != nil {
		t.Fatal(err)
	}
	l.Info("login",
		zap.String("user", "bob"),
		zap.String("password", "supersecret"),
		zap.String("token", "abc123"),
	)
	l.Sync()

	out := readLogFile(t, logpath+"-info.log")
	if strings.Contains(out, "supersecret") || strings.Contains(out, "abc123") {
		t.Fatalf("敏感值未脱敏: %s", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("缺少脱敏占位符: %s", out)
	}
	if !strings.Contains(out, "bob") {
		t.Fatalf("非敏感字段被误删: %s", out)
	}
}

func TestSamplingDropsRepeats(t *testing.T) {
	logpath := filepath.Join(t.TempDir(), "app")

	l, err := New(
		WithPath(logpath),
		WithFile(true),
		WithConsole(false),
		WithOutJSON(true),
		WithSampling(2, 0),
	)
	if err != nil {
		t.Fatal(err)
	}
	for range 8 {
		l.Info("same-message", zap.Int("n", 1))
	}
	l.Sync()

	out := readLogFile(t, logpath+"-info.log")
	if got := strings.Count(out, "same-message"); got != 2 {
		t.Fatalf("采样后期望 2 条，实际 %d 条:\n%s", got, out)
	}
}

func TestSamplingDisabledByDefault(t *testing.T) {
	logpath := filepath.Join(t.TempDir(), "app")

	l, err := New(
		WithPath(logpath),
		WithFile(true),
		WithConsole(false),
		WithOutJSON(true),
	)
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		l.Info("no-sampling", zap.Int("n", 1))
	}
	l.Sync()

	out := readLogFile(t, logpath+"-info.log")
	if got := strings.Count(out, "no-sampling"); got != 5 {
		t.Fatalf("未采样应保留全部 5 条，实际 %d 条", got)
	}
}

func TestWithSamplingValidatesThereafter(t *testing.T) {
	if _, err := New(WithSampling(1, -1)); err == nil {
		t.Fatal("thereafter < 0 应返回错误")
	}
}
