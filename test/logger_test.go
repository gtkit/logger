// @Author xiaozhaofu 2022/11/27 00:30:00
package test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	logger2 "logger"
)

func TestNewZap(t *testing.T) {
	assert2 := assert.New(t)
	type args struct {
		option *logger2.Option
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "logger",
			args: struct{ option *logger2.Option }{option: &logger2.Option{
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
			logger2.NewZap(tt.args.option)

			if assert2.NotNil(logger2.Zlog()) {
				logger2.Info("--- zap log success ----")
			}
		})
	}
}

func TestNewZapWithOptions(t *testing.T) {
	type args struct {
		opts []logger2.Options
	}
	tests := []struct {
		name string
		args args
	}{
		// TODO: Add test cases.
		{
			name: "NewZapWithOptions",
			args: args{opts: []logger2.Options{
				logger2.WithConsole(true),
				logger2.WithDivision("size"),
				logger2.WithFile(true),
				logger2.WithSqlLog(true),
				logger2.WithLevel("info"),
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger2.NewZapWithOptions(tt.args.opts...)
			logger2.Info(tt.name + "--- zap log with options success ----")
		})
	}
}
