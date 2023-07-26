// @Author xiaozhaofu 2023/7/26 13:44:00
package logger

import (
	"math/rand"
	"strconv"
)

// RandomColor generates a random color.
func RandomColor() {
	Infof("#%s", strconv.FormatInt(int64(rand.Intn(16777216)), 16))
}

// Yellow ...
func Yellow(msg string) {
	Infof("\x1b[33m%s\x1b[0m", msg)
}

// Red ...
func Red(msg string) {
	Infof("\x1b[31m%s\x1b[0m", msg)
}

// Redf ...
func Redf(msg string, arg interface{}) {
	Infof("\x1b[31m%s\x1b[0m %+v\n", msg, arg)
}

// Blue ...
func Blue(msg string) {
	Infof("\x1b[34m%s\x1b[0m", msg)
}

// Green ...
func Green(msg string) {
	Infof("\x1b[32m%s\x1b[0m", msg)
}

// Greenf ...
func Greenf(msg string, arg interface{}) {
	Infof("\x1b[32m%s\x1b[0m %+v\n", msg, arg)
}
