package logging

import (
	"errors"
	"testing"
)

func TestNewSimpleLogger(t *testing.T) {
	l := NewSimpleLogger(false)
	if l == nil {
		t.Fatal("NewSimpleLogger returned nil")
	}
	// Smoke-test all log methods (output goes to stdout, no assertion needed)
	l.Info("info message", "key", "value")
	l.Warn("warn message")
	l.Error("error message", errors.New("test error"))
}

func TestNewSimpleLoggerDebug(t *testing.T) {
	l := NewSimpleLogger(true)
	if l == nil {
		t.Fatal("NewSimpleLogger(debug) returned nil")
	}
	l.Debug("debug message", "k", 1)
}

func TestNewSimpleLoggerWithPrefix(t *testing.T) {
	l := NewSimpleLoggerWithPrefix("[TestComponent]", false)
	if l == nil {
		t.Fatal("NewSimpleLoggerWithPrefix returned nil")
	}
	l.Info("prefixed info")
	l.Warn("prefixed warn", "x", 42)
	l.Error("prefixed error", errors.New("boom"))
}

func TestNewSimpleLoggerWithPrefixDebug(t *testing.T) {
	l := NewSimpleLoggerWithPrefix("[Debug]", true)
	l.Debug("prefixed debug")
}

func TestNoOpLogger(t *testing.T) {
	l := NewNoOpLogger()
	if l == nil {
		t.Fatal("NewNoOpLogger returned nil")
	}
	// All methods should be no-ops — just verify they don't panic
	l.Debug("noop debug")
	l.Info("noop info")
	l.Warn("noop warn")
	l.Error("noop error", errors.New("silent"))
}
