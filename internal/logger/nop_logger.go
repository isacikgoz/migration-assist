package logger

func NewNopLogger() *NopLogger {
	return &NopLogger{}
}

type NopLogger struct{}

func (l *NopLogger) Printf(format string, v ...interface{}) {}

func (l *NopLogger) Println(v ...interface{}) {}
