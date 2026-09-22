// Package observability provides minimal logging for the HTTP contour.
package observability

import (
	"io"
	"log"
	"os"
)

// Logger is a minimal leveled logger used across the HTTP layer.
type Logger struct {
	info  *log.Logger
	error *log.Logger
}

// NewLogger returns a Logger writing to stderr.
func NewLogger() *Logger {
	return NewLoggerTo(os.Stderr)
}

// NewLoggerTo returns a Logger writing to w. It is used by tests to capture
// output.
func NewLoggerTo(w io.Writer) *Logger {
	return &Logger{
		info:  log.New(w, "INFO  ", log.LstdFlags),
		error: log.New(w, "ERROR ", log.LstdFlags),
	}
}

// Info logs an informational message.
func (l *Logger) Info(msg string, kv ...any) {
	l.info.Println(append([]any{msg}, kv...)...)
}

// Error logs an error message.
func (l *Logger) Error(msg string, kv ...any) {
	l.error.Println(append([]any{msg}, kv...)...)
}