package logger

import (
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestRedactKeysMasksValues(t *testing.T) {
	logpath := filepath.Join(t.TempDir(), "app")

	NewZap(
		WithPath(logpath),
		WithFile(true),
		WithConsole(false),
		WithOutJSON(true),
		WithRedactKeys("password", "token"),
	)
	defer Sync()

	Info("login",
		zap.String("user", "bob"),
		zap.String("password", "supersecret"),
		zap.String("token", "abc123"),
	)
	Sync() // flush before读取

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

	NewZap(
		WithPath(logpath),
		WithFile(true),
		WithConsole(false),
		WithOutJSON(true),
		WithSampling(2, 0), // 同 tick 内首 2 条放行，其余丢弃
	)
	defer Sync()

	for range 8 {
		Info("same-message", zap.Int("n", 1))
	}
	Sync()

	out := readLogFile(t, logpath+"-info.log")
	got := strings.Count(out, "same-message")
	if got != 2 {
		t.Fatalf("采样后期望 2 条，实际 %d 条:\n%s", got, out)
	}
}

// TestSamplingWithRedactKeys 回归测试：采样与脱敏同时启用时，两者都必须生效。
// 历史 bug：redactCore 曾包在 sampler 之外，其 Check 绕过 sampler.Check，导致采样静默失效。
func TestSamplingWithRedactKeys(t *testing.T) {
	logpath := filepath.Join(t.TempDir(), "app")

	NewZap(
		WithPath(logpath),
		WithFile(true),
		WithConsole(false),
		WithOutJSON(true),
		WithSampling(2, 0),
		WithRedactKeys("password"),
	)
	defer Sync()

	for range 8 {
		Info("hot-message", zap.String("password", "supersecret"))
	}
	Sync()

	out := readLogFile(t, logpath+"-info.log")
	if got := strings.Count(out, "hot-message"); got != 2 {
		t.Fatalf("采样+脱敏同开时采样失效: 期望 2 条，实际 %d 条:\n%s", got, out)
	}
	if strings.Contains(out, "supersecret") || !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("采样+脱敏同开时脱敏失效:\n%s", out)
	}
}

func TestSamplingDisabledByDefault(t *testing.T) {
	logpath := filepath.Join(t.TempDir(), "app")

	NewZap(
		WithPath(logpath),
		WithFile(true),
		WithConsole(false),
		WithOutJSON(true),
	)
	defer Sync()

	for range 5 {
		Info("no-sampling", zap.Int("n", 1))
	}
	Sync()

	out := readLogFile(t, logpath+"-info.log")
	if got := strings.Count(out, "no-sampling"); got != 5 {
		t.Fatalf("未采样应保留全部 5 条，实际 %d 条", got)
	}
}

func TestWithSamplingValidatesThereafter(t *testing.T) {
	if err := New(WithSampling(1, -1)); err == nil {
		t.Fatal("thereafter < 0 应返回错误")
	}
	// 复位为可用状态，避免污染后续测试的全局 logger。
	NewZap(WithConsole(false), WithFile(false))
	Sync()
}
