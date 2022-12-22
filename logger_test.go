// @Author xiaozhaofu 2022/11/27 00:30:00
package logger

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewZap(t *testing.T) {
	assert2 := assert.New(t)
	type args struct {
		option *Option
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "logger",
			args: struct{ option *Option }{option: &Option{
				Level:         "info",
				ConsoleStdout: true,
				FileStdout:    true,
				Division:      "size",
				SqlLog:        true,
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			NewZap(tt.args.option)

			if assert2.NotNil(Zlog()) {
				Info("--- zap log success ----")
			}
		})
	}
}

func TestNewZapWithOptions(t *testing.T) {
	type args struct {
		opts []Options
	}
	tests := []struct {
		name string
		args args
	}{
		// TODO: Add test cases.
		{
			name: "NewZapWithOptions",
			args: args{opts: []Options{
				WithConsole(true),
				WithDivision("size"),
				WithFile(true),
				WithSqlLog(true),
				WithLevel("info"),
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			NewZapWithOptions(tt.args.opts...)
			Info(tt.name + "--- zap log with options success ----")
		})
	}
}
