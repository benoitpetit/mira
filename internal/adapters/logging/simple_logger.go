// Package logging provides structured logging adapters.
// Simple logger implementation using standard log/slog package
package logging

import (
	"log/slog"
	"os"
)

// SlogLogger implements ports.Logger using the standard slog package (Go 1.21+)
type SlogLogger struct {
	logger *slog.Logger
}

// NewSimpleLogger creates a new slog-based logger
func NewSimpleLogger(debug bool) *SlogLogger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	if debug {
		opts.Level = slog.LevelDebug
	}
	return &SlogLogger{
		logger: slog.New(slog.NewJSONHandler(os.Stdout, opts)),
	}
}

// NewSimpleLoggerWithPrefix creates a new slog-based logger with a component prefix
func NewSimpleLoggerWithPrefix(prefix string, debug bool) *SlogLogger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	if debug {
		opts.Level = slog.LevelDebug
	}
	return &SlogLogger{
		logger: slog.New(slog.NewJSONHandler(os.Stdout, opts)).With("component", prefix),
	}
}

// Debug logs a debug message
func (l *SlogLogger) Debug(msg string, keysAndValues ...interface{}) {
	l.logger.Debug(msg, keysAndValues...)
}

// Info logs an info message
func (l *SlogLogger) Info(msg string, keysAndValues ...interface{}) {
	l.logger.Info(msg, keysAndValues...)
}

// Warn logs a warning message
func (l *SlogLogger) Warn(msg string, keysAndValues ...interface{}) {
	l.logger.Warn(msg, keysAndValues...)
}

// Error logs an error message
func (l *SlogLogger) Error(msg string, err error, keysAndValues ...interface{}) {
	args := append([]interface{}{"error", err}, keysAndValues...)
	l.logger.Error(msg, args...)
}

// NoOpLogger is a logger that does nothing (for testing)
type NoOpLogger struct{}

// NewNoOpLogger creates a new no-op logger
func NewNoOpLogger() *NoOpLogger {
	return &NoOpLogger{}
}

// Debug logs a debug message (no-op)
func (n *NoOpLogger) Debug(_ string, _ ...interface{}) {}

// Info logs an info message (no-op)
func (n *NoOpLogger) Info(_ string, _ ...interface{}) {}

// Warn logs a warning message (no-op)
func (n *NoOpLogger) Warn(_ string, _ ...interface{}) {}

// Error logs an error message (no-op)
func (n *NoOpLogger) Error(_ string, _ error, _ ...interface{}) {}
