// Simple logger implementation using standard log package
package logging

import (
	"fmt"
	"log"
	"os"
)

// SimpleLogger implements ports.Logger using the standard log package
type SimpleLogger struct {
	logger *log.Logger
	debug  bool
}

// NewSimpleLogger creates a new simple logger
func NewSimpleLogger(debug bool) *SimpleLogger {
	return &SimpleLogger{
		logger: log.New(os.Stdout, "", log.LstdFlags),
		debug:  debug,
	}
}

// NewSimpleLoggerWithPrefix creates a new simple logger with a prefix
func NewSimpleLoggerWithPrefix(prefix string, debug bool) *SimpleLogger {
	return &SimpleLogger{
		logger: log.New(os.Stdout, prefix+" ", log.LstdFlags),
		debug:  debug,
	}
}

// Debug logs a debug message
func (l *SimpleLogger) Debug(msg string, keysAndValues ...interface{}) {
	if !l.debug {
		return
	}
	l.logger.Printf("[DEBUG] %s %s", msg, formatKeyValues(keysAndValues...))
}

// Info logs an info message
func (l *SimpleLogger) Info(msg string, keysAndValues ...interface{}) {
	l.logger.Printf("[INFO] %s %s", msg, formatKeyValues(keysAndValues...))
}

// Warn logs a warning message
func (l *SimpleLogger) Warn(msg string, keysAndValues ...interface{}) {
	l.logger.Printf("[WARN] %s %s", msg, formatKeyValues(keysAndValues...))
}

// Error logs an error message
func (l *SimpleLogger) Error(msg string, err error, keysAndValues ...interface{}) {
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	l.logger.Printf("[ERROR] %s: %s %s", msg, errStr, formatKeyValues(keysAndValues...))
}

// formatKeyValues formats key-value pairs into a string
func formatKeyValues(keysAndValues ...interface{}) string {
	if len(keysAndValues) == 0 {
		return ""
	}
	result := ""
	for i := 0; i < len(keysAndValues)-1; i += 2 {
		key := fmt.Sprintf("%v", keysAndValues[i])
		value := fmt.Sprintf("%v", keysAndValues[i+1])
		if result != "" {
			result += " "
		}
		result += fmt.Sprintf("%s=%s", key, value)
	}
	return result
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
