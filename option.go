package logger

import (
	"strings"
)

type Options interface {
	apply(*logConfig)
}

/**
 * consoleopt.
 */
type consoleopt bool

func (c consoleopt) apply(opts *logConfig) {
	opts.consoleStdout = bool(c)
}

// WithConsole # 设置日志是否打印到控制台.
func WithConsole(b bool) Options {
	return consoleopt(b)
}

/**
 * fileopt.
 */
type fileopt bool

func (f fileopt) apply(opts *logConfig) {
	opts.fileStdout = bool(f)
}

// WithFile # 设置日志是否输出到文件.
func WithFile(b bool) Options {
	return fileopt(b)
}

/**
 * divisionopt.
 */
type divisionopt struct {
	division string
}

func (d divisionopt) apply(opts *logConfig) {
	opts.division = d.division
}

// WithDivision # 设置日志切割模式,time:按时间切割, size:按大小切割.
func WithDivision(d string) Options {
	return divisionopt{division: d}
}

/**
 * pathopt.
 */
type pathopt struct {
	path string
}

func (p pathopt) apply(opts *logConfig) {
	opts.path = p.path
}

// WithPath # 设置日志输出路径.
func WithPath(p string) Options {
	if strings.HasSuffix(p, "/") {
		return pathopt{path: p + "logger"}
	}
	return pathopt{path: p}

}

/**
 * compressopt.
 */
type compressopt struct {
	compress bool
}

func (c compressopt) apply(opts *logConfig) {
	opts.compress = c.compress
}
func WithCompress(c bool) Options {
	return compressopt{compress: c}
}

/**
 * maxageopt.
 */
type maxageopt struct {
	age int
}

func (a maxageopt) apply(opts *logConfig) {
	opts.maxAge = a.age
}

// WithMaxAge # 设置日志文件最大保存时间,单位:天.
func WithMaxAge(a int) Options {
	return maxageopt{age: a}
}

/**
 * maxbackupsopt.
 */
type maxbackupsopt struct {
	backups int
}

func (b maxbackupsopt) apply(opts *logConfig) {
	opts.maxBackups = b.backups
}

// WithMaxBackups # 设置日志文件最大保存数量.
func WithMaxBackups(b int) Options {
	return maxbackupsopt{backups: b}
}

/**
 * maxsizeopt.
 */
type maxsizeopt struct {
	size int
}

func (s maxsizeopt) apply(opts *logConfig) {
	opts.maxSize = s.size
}

func WithMaxSize(s int) Options {
	return maxsizeopt{size: s}
}

/**
 * levelopt.
 * 日志级别: debug,info,warn,error,dpanic,panic,fatal.
 */
type levelopt struct {
	level string
}

func (l levelopt) apply(opts *logConfig) {
	opts.level = l.level
}

// WithLevel # 设置日志输出级别.
// 日志级别: debug,info,warn,error,dpanic,panic,fatal.
func WithLevel(l string) Options {
	return levelopt{level: l}
}
