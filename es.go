// @Author 2023/11/20 17:51:00
package logger

type Eslogger struct {
}

func EsLogger() *Eslogger {
	return &Eslogger{}
}

func Es() *Eslogger {
	return &Eslogger{}
}

func (l Eslogger) Printf(format string, v ...interface{}) {
	Infof("[* ES] "+format, v...)
}
