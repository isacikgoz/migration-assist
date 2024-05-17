package logger

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type LogInterface interface {
	Printf(format string, v ...interface{})
	Println(v ...any)
}

type Options struct {
	Timestamps bool
}

func NewLogger(w io.Writer, opts Options) *Logger {
	if w == nil {
		w = os.Stderr
	}
	return &Logger{
		Writer:     w,
		Timestamps: opts.Timestamps,
	}
}

type Logger struct {
	Writer     io.Writer
	Timestamps bool
}

func (l *Logger) Printf(format string, v ...interface{}) {
	if l.Timestamps {
		format = time.Now().Format(time.DateTime) + " " + format
	}
	if !strings.HasSuffix(format, "\n") {
		format += "\n"
	}
	fmt.Fprintf(l.Writer, format, v...)
}

func (l *Logger) Println(v ...any) {
	if l.Timestamps {
		v = append([]any{time.Now().Format(time.DateTime)}, v...)
	}
	fmt.Fprintln(l.Writer, v...)
}
