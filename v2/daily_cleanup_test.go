package logger

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeAgedFile 在 dir 下创建一个内容任意、修改时间为 age 之前的日志文件。
func writeAgedFile(t *testing.T, path string, age time.Duration) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
	mtime := time.Now().Add(-age)
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("chtimes %q: %v", path, err)
	}
}

func mustExist(t *testing.T, path string, want bool) {
	t.Helper()
	_, err := os.Stat(path)
	switch {
	case want && err != nil:
		t.Fatalf("expected %q to exist, got: %v", path, err)
	case !want && err == nil:
		t.Fatalf("expected %q to be removed, but it still exists", path)
	case !want && !os.IsNotExist(err):
		t.Fatalf("stat %q: unexpected error: %v", path, err)
	}
}

func TestCleanDailyFilesByMaxAge(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{path: filepath.Join(dir, "app"), level: "info", maxAge: 7}
	prefix := cfg.path + "-" + cfg.level + "-"

	current := prefix + "2026-06-02.log"
	recent := prefix + "2026-05-30.log"
	expired := prefix + "2026-01-01.log"
	expiredGz := prefix + "2026-01-02.log.gz"

	writeAgedFile(t, current, 0)
	writeAgedFile(t, recent, 2*24*time.Hour)
	writeAgedFile(t, expired, 30*24*time.Hour)
	writeAgedFile(t, expiredGz, 30*24*time.Hour)

	cleanDailyFiles(cfg, time.Now(), current)

	mustExist(t, current, true)
	mustExist(t, recent, true)
	mustExist(t, expired, false)
	mustExist(t, expiredGz, false)
}

func TestCleanDailyFilesByMaxBackups(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{path: filepath.Join(dir, "app"), level: "info", maxBackups: 2}
	prefix := cfg.path + "-" + cfg.level + "-"

	current := prefix + "2026-06-05.log"
	writeAgedFile(t, current, 0)

	files := []string{
		prefix + "2026-06-04.log",
		prefix + "2026-06-03.log",
		prefix + "2026-06-02.log",
		prefix + "2026-06-01.log",
		prefix + "2026-05-31.log",
	}
	for i, f := range files {
		writeAgedFile(t, f, time.Duration(i+1)*24*time.Hour)
	}

	cleanDailyFiles(cfg, time.Now(), current)

	mustExist(t, current, true)
	mustExist(t, files[0], true)
	mustExist(t, files[1], true)
	mustExist(t, files[2], false)
	mustExist(t, files[3], false)
	mustExist(t, files[4], false)
}

func TestCleanDailyFilesKeepsCurrentEvenIfOld(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{path: filepath.Join(dir, "app"), level: "info", maxAge: 1}
	prefix := cfg.path + "-" + cfg.level + "-"

	current := prefix + "2026-01-01.log"
	writeAgedFile(t, current, 100*24*time.Hour)

	cleanDailyFiles(cfg, time.Now(), current)

	mustExist(t, current, true)
}

func TestCleanDailyFilesDisabledWhenBothZero(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{path: filepath.Join(dir, "app"), level: "info", maxAge: 0, maxBackups: 0}
	prefix := cfg.path + "-" + cfg.level + "-"

	old := prefix + "2020-01-01.log"
	writeAgedFile(t, old, 1000*24*time.Hour)

	cleanDailyFiles(cfg, time.Now(), "")

	mustExist(t, old, true)
}

func TestDailyWriteSyncerCleansBacklogOnStartup(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{path: filepath.Join(dir, "app"), level: "info", maxAge: 7, maxBackups: 50, maxSize: 1}
	prefix := cfg.path + "-" + cfg.level + "-"

	expired := prefix + "2020-01-01.log"
	writeAgedFile(t, expired, 100*24*time.Hour)

	dw, err := newDailyWriteSyncer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := dw.Close(); err != nil {
		t.Fatal(err)
	}

	mustExist(t, expired, false)
}
