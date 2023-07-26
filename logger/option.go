// @Author xiaozhaofu 2022/12/22 17:20:00
package logger

type options struct {
	level         string // 日志级别
	consolestdout bool   // 日志是否输出到控制台
	filestdout    bool   // 日志是否输出到文件
	division      string // 日志切割方式, time:日期, size:大小, 默认按照大小分割
	path          string // 日志文件路径
	sqllog        bool   // 是否打印 sql 执行日志
}
type Options interface {
	apply(*options)
}

type levelopt struct {
	level string
}

func (l levelopt) apply(opts *options) {
	opts.level = l.level
}

// #设置日志级别
func WithLevel(l string) Options {
	return levelopt{level: l}
}

type consoleopt bool

func (c consoleopt) apply(opts *options) {
	opts.consolestdout = bool(c)
}

// # 设置日志是否打印到控制台
func WithConsole(b bool) Options {
	return consoleopt(b)
}

type fileopt bool

func (f fileopt) apply(opts *options) {
	opts.filestdout = bool(f)
}

// # 设置日志是否输出到文件
func WithFile(b bool) Options {
	return fileopt(b)
}

type divisionopt struct {
	division string
}

func (d divisionopt) apply(opts *options) {
	opts.division = d.division
}

// # 设置日志切割模式,time:按时间切割, size:按大小切割
func WithDivision(d string) Options {
	return divisionopt{division: d}
}

type pathopt struct {
	path string
}

func (p pathopt) apply(opts *options) {
	opts.path = p.path
}

// # 设置日志输出路径
func WithPath(p string) Options {
	return pathopt{path: p}
}

type sqllogopt bool

func (s sqllogopt) apply(opts *options) {
	opts.sqllog = bool(s)
}

// # 设置日志是否打印sql执行日志,用于gorm日志中
func WithSqlLog(b bool) Options {
	return sqllogopt(b)
}
