package logger

var newser Newser

type Newser interface {
	Text(args ...any)
	TextF(template string, args ...any)
}
