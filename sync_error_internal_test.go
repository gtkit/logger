package logger

import (
	"errors"
	"io/fs"
	"syscall"
	"testing"

	"go.uber.org/multierr"
)

func TestIsBenignSyncError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		benign bool
	}{
		{
			name:   "enotty on terminal stderr",
			err:    &fs.PathError{Op: "sync", Path: "/dev/stderr", Err: syscall.ENOTTY},
			benign: true,
		},
		{
			name:   "ebadf on piped stderr",
			err:    &fs.PathError{Op: "sync", Path: "/dev/stderr", Err: syscall.EBADF},
			benign: true,
		},
		{
			name:   "einval on pipe",
			err:    &fs.PathError{Op: "sync", Path: "/dev/stdout", Err: syscall.EINVAL},
			benign: true,
		},
		{
			name:   "real file sync failure",
			err:    &fs.PathError{Op: "sync", Path: "/var/log/app.log", Err: syscall.EIO},
			benign: false,
		},
		{
			name:   "plain error",
			err:    errors.New("disk full"),
			benign: false,
		},
		{
			name: "multierr all benign",
			err: multierr.Combine(
				&fs.PathError{Op: "sync", Path: "/dev/stdout", Err: syscall.ENOTTY},
				&fs.PathError{Op: "sync", Path: "/dev/stderr", Err: syscall.EBADF},
			),
			benign: true,
		},
		{
			name: "multierr benign mixed with real failure",
			err: multierr.Combine(
				&fs.PathError{Op: "sync", Path: "/dev/stdout", Err: syscall.ENOTTY},
				&fs.PathError{Op: "sync", Path: "/var/log/app.log", Err: syscall.EIO},
			),
			benign: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBenignSyncError(tt.err); got != tt.benign {
				t.Fatalf("isBenignSyncError(%v) = %v, want %v", tt.err, got, tt.benign)
			}
		})
	}
}
