# logger

### 封装 zap 日志
#### 推荐使用函数选项模式传入参数, 实例化日志
```
NewZap(
		WithConsole(true),
		WithDivision("size"),
		WithFile(true),
		WithSqlLog(false),
		WithPath("/home/www/logs"),
	)
```
