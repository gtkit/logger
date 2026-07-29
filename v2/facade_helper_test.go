package logger

// logThroughFacade 模拟外部 facade 的一层转发帧；配合 WithCallerSkip(1)，
// 期望日志 caller 指向 logThroughFacade 的调用行，而非本文件。
// 必须与断言测试放在不同文件：文件名断言才有鉴别力。
func logThroughFacade(l *Logger, msg string) {
	l.Info(msg)
}

// logThroughOuterFacade 模拟两层转发（facade 套 facade）；
// 配合 WithCallerSkip(1).WithCallerSkip(1)，caller 应指向本函数的调用行。
func logThroughOuterFacade(l *Logger, msg string) {
	logThroughFacade(l, msg)
}
