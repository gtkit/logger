// @Author 2023/11/20 17:51:00
package logger

var _ IEsLogger = (*Eslogger)(nil)

type IEsLogger interface {
	Printf(format string, v ...any)
}
type Eslogger struct {
}

func EsLogger() *Eslogger {
	return &Eslogger{}
}

func Es() *Eslogger {
	return &Eslogger{}
}

func (l Eslogger) Printf(format string, v ...any) {
	Infof("[* ES] "+format, v...)
}
