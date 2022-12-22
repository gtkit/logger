# logger

### 封装 zap 日志
#### 推荐使用函数选项模式传入参数, 实例化日志
```
NewZapWithOptions(
		WithConsole(true),
		WithDivision("size"),
		WithFile(true),
		WithSqlLog(false),
		WithPath("/home/www/logs"),
		WithLevel("info"),
	)
```

#### 也可以使用直接传参的模式
```
opt := &Option{
		Level         string // 日志级别
	    ConsoleStdout bool   // 日志是否输出到控制台
	    FileStdout    bool   // 日志是否输出到文件
	    Division      string // 日志切割方式, time:日期, size:大小, 默认按照大小分割
	    Path          string // 日志文件路径
	    SqlLog        bool   // 是否打印 sql 执行日志
	}
NewZap(opt)
```
