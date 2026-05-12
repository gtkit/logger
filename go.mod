module github.com/gtkit/logger

go 1.26

retract (
	v1.6.2 // bad release on abandoned version line
	v1.6.1 // bad release on abandoned version line
)

require (
	go.uber.org/zap v1.28.0
	gopkg.in/natefinch/lumberjack.v2 v2.2.1
)

require go.uber.org/multierr v1.11.0 // indirect
