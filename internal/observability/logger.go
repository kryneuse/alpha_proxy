// Package observability provides structured logging for the HTTP contour using
// the standard log/slog package with a JSON handler. No external dependency is
// introduced.
package observability

import (
	"io"
	"log/slog"
	"os"
)

// Logger is a thin wrapper around *slog.Logger.
type Logger struct {
	l *slog.Logger
}

// NewLogger returns a Logger writing JSON to stderr.
func NewLogger() *Logger {
	return NewLoggerTo(os.Stderr)
}

// NewLoggerTo returns a Logger writing JSON to w. It is used by tests to capture
// output.
func NewLoggerTo(w io.Writer) *Logger {
	return &Logger{l: slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{}))}
}

// Info logs a structured message at Info level.
func (l *Logger) Info(msg string, args ...any) {
	l.l.Info(msg, args...)
}

// Error logs a structured message at Error level.
func (l *Logger) Error(msg string, args ...any) {
	l.l.Error(msg, args...)
}
