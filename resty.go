// @Author 2023/11/20 17:49:00
package logger

var _ IRestyLogger = (*Restylogger)(nil)

type IRestyLogger interface {
	Errorf(format string, v ...interface{})
	Warnf(format string, v ...interface{})
	Debugf(format string, v ...interface{})
}
type Restylogger struct {
}

func RestyLogger() *Restylogger {
	return &Restylogger{}
}

func (l *Restylogger) Errorf(format string, v ...interface{}) {
	Errorf("--ERROR RESTY "+format, v)
}
func (l *Restylogger) Warnf(format string, v ...interface{}) {
	Warnf("--WARN RESTY "+format, v)
}
func (l *Restylogger) Debugf(format string, v ...interface{}) {
	Debugf("--DEBUG RESTY "+format, v)
}
