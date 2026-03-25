package logger

import (
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

type testCloser struct {
	count atomic.Int32
}

func (c *testCloser) Close() error {
	c.count.Add(1)
	return nil
}

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

	cfg := &Config{
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

func TestSyncIsIdempotentAcrossDerivedLoggers(t *testing.T) {
	closer := &testCloser{}
	var undoCalls atomic.Int32

	log := &Logger{
		zap:   zap.NewNop(),
		sugar: zap.NewNop().Sugar(),
		state: &lifecycleState{
			root: zap.NewNop(),
			undo: func() {
				undoCalls.Add(1)
			},
			closers: []io.Closer{closer},
		},
	}

	child := log.With(zap.String("request_id", "req-1"))
	child.Sync()
	log.Sync()
	child.Undo()

	if got := closer.count.Load(); got != 1 {
		t.Fatalf("closer closed %d times, want 1", got)
	}
	if got := undoCalls.Load(); got != 1 {
		t.Fatalf("undo called %d times, want 1", got)
	}
}

func TestHInfoIncludesFieldsInMessager(t *testing.T) {
	msg := &testMessager{}
	log := &Logger{
		zap:      zap.NewNop(),
		sugar:    zap.NewNop().Sugar(),
		state:    &lifecycleState{root: zap.NewNop()},
		messager: msg,
	}

	log.HInfo("request failed",
		zap.String("request_id", "req-1"),
		zap.Int("status", 500),
	)

	if !strings.Contains(msg.lastMsg, "request_id") || !strings.Contains(msg.lastMsg, "req-1") {
		t.Fatalf("hook message %q does not include structured fields", msg.lastMsg)
	}
}
