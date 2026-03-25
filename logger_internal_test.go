package logger

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

type testMessager struct {
	lastMsg string
	lastURL string
}

func (m *testMessager) Send(msg string) {
	m.lastMsg = msg
}

func (m *testMessager) SendTo(url, msg string) {
	m.lastURL = url
	m.lastMsg = msg
}

func TestDailyWriteSyncerRotatesOnFullDateChange(t *testing.T) {
	t.Parallel()

	cfg := &logConfig{
		path:       t.TempDir() + "/daily",
		level:      "info",
		maxSize:    1,
		maxAge:     1,
		maxBackups: 1,
	}

	dw := newDailyWriteSyncer(cfg)
	oldWriter := dw.lj
	dw.currentDate = "2000-01-01"

	if _, err := dw.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}

	wantDate := time.Now().Format("2006-01-02")
	if dw.currentDate != wantDate {
		t.Fatalf("currentDate = %q, want %q", dw.currentDate, wantDate)
	}
	if dw.lj == oldWriter {
		t.Fatal("daily writer was not replaced after date change")
	}
	if !strings.Contains(dw.lj.Filename, wantDate) {
		t.Fatalf("filename %q does not contain current date %q", dw.lj.Filename, wantDate)
	}
}

func TestHInfoIncludesFieldsInMessager(t *testing.T) {
	globalMu.Lock()
	oldMsgr := msgr
	msgr = &testMessager{}
	globalMu.Unlock()
	defer func() {
		globalMu.Lock()
		msgr = oldMsgr
		globalMu.Unlock()
	}()

	HInfo("request failed",
		zap.String("request_id", "req-1"),
		zap.Int("status", 500),
	)

	globalMu.RLock()
	msg := msgr.(*testMessager).lastMsg
	globalMu.RUnlock()

	if !strings.Contains(msg, "request_id") || !strings.Contains(msg, "req-1") {
		t.Fatalf("hook message %q does not include structured fields", msg)
	}
}

func TestGlobalLoggerConcurrentReconfigure(t *testing.T) {
	NewZap(WithConsole(false), WithFile(false))
	defer Sync()

	var wg sync.WaitGroup
	var stop atomic.Bool

	wg.Add(1)
	go func() {
		defer wg.Done()
		for !stop.Load() {
			Info("concurrent info", zap.String("k", "v"))
		}
	}()

	for range 20 {
		NewZap(WithConsole(false), WithFile(false))
	}

	stop.Store(true)
	wg.Wait()
}
