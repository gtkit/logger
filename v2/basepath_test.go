package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWithBasePathAnchorsRelativePaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	l := MustNew(
		WithConsole(false), WithFile(true), WithOutJSON(true),
		WithPath("logs/app"),
		WithBasePath(root),
		WithChannel("order", WithChannelPath("logs/channels/order")),
		WithReplaceGlobals(false),
	)
	defer l.Sync()

	l.Info("root-anchored")
	l.Channel("order").Info("channel-anchored")
	l.Sync()

	rootLog := filepath.Join(root, "logs", "app-info.log")
	if !strings.Contains(readLogFile(t, rootLog), "root-anchored") {
		t.Fatalf("相对主路径应锚定到 basePath 下: %s", rootLog)
	}
	channelLog := filepath.Join(root, "logs", "channels", "order-info.log")
	if !strings.Contains(readLogFile(t, channelLog), "channel-anchored") {
		t.Fatalf("相对 channel 路径应锚定到 basePath 下: %s", channelLog)
	}
}

func TestWithBasePathKeepsAbsolutePaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	absPath := filepath.Join(t.TempDir(), "logs", "abs")
	l := MustNew(
		WithConsole(false), WithFile(true), WithOutJSON(true),
		WithPath(absPath),
		WithBasePath(root),
		WithReplaceGlobals(false),
	)
	defer l.Sync()

	l.Info("abs-untouched")
	l.Sync()

	if !strings.Contains(readLogFile(t, absPath+"-info.log"), "abs-untouched") {
		t.Fatal("绝对路径不应被 basePath 改写")
	}
	if _, err := os.Stat(filepath.Join(root, absPath)); err == nil {
		t.Fatal("绝对路径不应被 Join 到 basePath 下")
	}
}

func TestZeroValueOptionsFallBackToDefaults(t *testing.T) {
	t.Parallel()

	// 零值容忍仅限无文件系统副作用的配置：level/maxSize 零值 = 未配置 → 用默认
	l := MustNew(
		WithConsole(true), WithFile(false),
		WithLevel(""), WithMaxSize(0),
		WithReplaceGlobals(false),
	)
	defer l.Sync()

	if got := l.GetLevel(); got != "info" {
		t.Fatalf("WithLevel(\"\") 应保留默认级别 info, got %q", got)
	}

	// path 决定落盘位置，空值必须报错（静默回退相对默认路径会污染进程 cwd）
	if _, err := New(WithPath("")); err == nil {
		t.Fatal("WithPath(\"\") 应报错而非静默回退相对默认路径")
	}

	// 非零非法照旧报错
	if _, err := New(WithLevel("verbose")); err == nil {
		t.Fatal("非法级别应报错")
	}
	if _, err := New(WithMaxSize(-1)); err == nil {
		t.Fatal("负数 maxSize 应报错")
	}
}

// TestWithBasePathPreservesTrailingSlashSemantics 尾斜杠是「文件名前缀」语义的一部分：
// 锚定只能加根，不得改变目录结构（filepath.Join 会吞尾斜杠，必须补回）。
func TestWithBasePathPreservesTrailingSlashSemantics(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	// 显式尾斜杠前缀：文件应落在 root/logs/ 目录内（文件名 "-info.log"）
	l := MustNew(
		WithConsole(false), WithFile(true), WithOutJSON(true),
		WithPath("logs/"), WithBasePath(root),
		WithDivision("size"),
		WithReplaceGlobals(false),
	)
	l.Info("trailing-slash-check")
	l.Sync()

	inDir := filepath.Join(root, "logs", "-info.log")
	if !strings.Contains(readLogFile(t, inDir), "trailing-slash-check") {
		t.Fatalf("尾斜杠前缀应保持目录结构，期望文件 %s", inDir)
	}
	if _, err := os.Stat(filepath.Join(root, "logs-info.log")); err == nil {
		t.Fatal("锚定不得把 logs/ 目录坍缩成 logs- 文件名前缀")
	}

	// 库默认路径 "./logs/"（不显式 WithPath）走同一语义
	l2 := MustNew(
		WithConsole(false), WithFile(true), WithOutJSON(true),
		WithBasePath(root+"/d2"),
		WithDivision("size"),
		WithReplaceGlobals(false),
	)
	l2.Info("default-path-anchor-check")
	l2.Sync()
	defFile := filepath.Join(root, "d2", "logs", "-info.log")
	if !strings.Contains(readLogFile(t, defFile), "default-path-anchor-check") {
		t.Fatalf("默认路径经锚定应落在 %s", defFile)
	}
}

// TestWithBasePathRejectsRelativeBase 相对 base 仍随 cwd 漂移，必须 fail-fast。
func TestWithBasePathRejectsRelativeBase(t *testing.T) {
	t.Parallel()

	if _, err := New(WithBasePath("relative/root")); err == nil {
		t.Fatal("相对 basePath 应报错")
	}
	if _, err := New(WithConsole(true), WithFile(false), WithBasePath(""), WithReplaceGlobals(false)); err != nil {
		t.Fatalf("空 basePath（不锚定）应合法: %v", err)
	}
}
