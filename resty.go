// @Author 2023/11/20 17:49:00
package logger

var _ IRestyLogger = (*Restylogger)(nil)

type IRestyLogger interface {
	Errorf(format string, v ...any)
	Warnf(format string, v ...any)
	Debugf(format string, v ...any)
}
type Restylogger struct {
}

func RestyLogger() *Restylogger {
	return &Restylogger{}
}

func (l *Restylogger) Errorf(format string, v ...any) {
	Errorf("--ERROR RESTY "+format, v)
}
func (l *Restylogger) Warnf(format string, v ...any) {
	Warnf("--WARN RESTY "+format, v)
}
func (l *Restylogger) Debugf(format string, v ...any) {
	Debugf("--DEBUG RESTY "+format, v)
}
