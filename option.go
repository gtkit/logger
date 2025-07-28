package logger

import (
	"strings"
)

type Options interface {
	apply(*logConfig) error
}

type news struct {
	newser Newser
}

func (n news) apply(opts *logConfig) error {
	if n.newser != nil {
		opts.newser = n.newser
	}

	return nil
}

func WithNews(n Newser) Options {
	return news{newser: n}
}

/**
 * consoleopt.
 */
type consoleopt bool

func (c consoleopt) apply(opts *logConfig) error {
	opts.consoleStdout = bool(c)

	return nil
}

// WithConsole # 设置日志是否打印到控制台.
func WithConsole(b bool) Options {
	return consoleopt(b)
}

/**
 * fileopt.
 */
type fileopt bool

func (f fileopt) apply(opts *logConfig) error {
	opts.fileStdout = bool(f)

	return nil
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

func (d divisionopt) apply(opts *logConfig) error {
	if d.division != "time" && d.division != "size" {
		return ErrDivision
	}

	opts.division = d.division

	return nil
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

func (p pathopt) apply(opts *logConfig) error {
	if p.path == "" {
		return ErrPath
	}

	opts.path = p.path

	return nil
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

func (c compressopt) apply(opts *logConfig) error {
	opts.compress = c.compress

	return nil
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

func (a maxageopt) apply(opts *logConfig) error {
	if a.age < 0 {
		return ErrMaxAge
	}

	opts.maxAge = a.age

	return nil
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

func (b maxbackupsopt) apply(opts *logConfig) error {
	if b.backups < 0 {
		return ErrMaxBackups
	}

	opts.maxBackups = b.backups

	return nil
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

func (s maxsizeopt) apply(opts *logConfig) error {
	if s.size < 0 {
		return ErrMaxSize
	}

	opts.maxSize = s.size

	return nil
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

func (l levelopt) apply(opts *logConfig) error {
	if l.level != "debug" && l.level != "info" && l.level != "warn" && l.level != "error" && l.level != "dpanic" && l.level != "panic" && l.level != "fatal" {
		return ErrLevel
	}

	opts.level = l.level

	return nil
}

// WithLevel # 设置日志输出级别.
// 日志级别: debug,info,warn,error,dpanic,panic,fatal.
func WithLevel(l string) Options {
	return levelopt{level: l}
}
