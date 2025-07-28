package logger

type Err string

var (
	ErrDivision   Err = "传入值应该为字符串 `time` 或者 `size`"
	ErrPath       Err = "传入路径不能为空"
	ErrMaxAge     Err = "传入的最大保存时间应该大于0"
	ErrMaxBackups Err = "传入的最大备份数量应该大于0"
	ErrMaxSize    Err = "传入的最大文件大小应该大于0"
	ErrLevel      Err = "传入的日志级别不正确"
)

func (e Err) Error() string {
	return string(e)
}
