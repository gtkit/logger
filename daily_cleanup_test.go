package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBothDivisionUsesDatedActiveFilename(t *testing.T) {
	logpath := filepath.Join(t.TempDir(), "app")

	NewZap(
		WithDivision("both"),
		WithPath(logpath),
		WithConsole(false),
		WithFile(true),
	)
	Info("both division")
	Sync()

	datedPath := logpath + "-info-" + time.Now().Format(time.DateOnly) + ".log"
	out := readLogFile(t, datedPath)
	if !strings.Contains(out, "both division") {
		t.Fatalf("both log missing message: %s", out)
	}

	if _, err := os.Stat(logpath + "-info.log"); err == nil {
		t.Fatalf("both mode should use dated active filename, found fixed file")
	}
}

func TestDefaultDivisionUsesBoth(t *testing.T) {
	logpath := filepath.Join(t.TempDir(), "default")

	NewZap(
		WithPath(logpath),
		WithConsole(false),
		WithFile(true),
	)
	Info("default both division")
	Sync()

	datedPath := logpath + "-info-" + time.Now().Format(time.DateOnly) + ".log"
	out := readLogFile(t, datedPath)
	if !strings.Contains(out, "default both division") {
		t.Fatalf("default both log missing message: %s", out)
	}
}
