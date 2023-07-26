// @Author xiaozhaofu 2022/11/27 00:30:00
package test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"logger/logger"
)

func TestNewZap(t *testing.T) {
	assert2 := assert.New(t)
	type args struct {
		option *logger.Option
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "logger",
			args: struct{ option *logger.Option }{option: &logger.Option{
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
			logger.NewZap(tt.args.option)

			if assert2.NotNil(logger.Zlog()) {
				logger.Info("--- zap log success ----")
			}
		})
	}
}

func TestNewZapWithOptions(t *testing.T) {
	type args struct {
		opts []logger.Options
	}
	tests := []struct {
		name string
		args args
	}{
		// TODO: Add test cases.
		{
			name: "NewZapWithOptions",
			args: args{opts: []logger.Options{
				logger.WithConsole(true),
				logger.WithDivision("size"),
				logger.WithFile(true),
				logger.WithSqlLog(true),
				logger.WithLevel("info"),
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger.NewZapWithOptions(tt.args.opts...)
			logger.Info(tt.name + "--- zap log with options success ----")
		})
	}
}
