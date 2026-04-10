package util

import (
	"errors"
	"testing"
	"time"
)

func TestCircuitBreaker_InitialState(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	cb := NewCircuitBreaker(config)

	if cb.State() != StateClosed {
		t.Errorf("Initial state = %v, want StateClosed", cb.State())
	}
}

func TestCircuitBreaker_OpensAfterFailures(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          1 * time.Second,
	}
	cb := NewCircuitBreaker(config)

	testErr := errors.New("test error")

	// Generate failures up to threshold
	for i := 0; i < config.FailureThreshold; i++ {
		err := cb.Execute(func() error {
			return testErr
		})
		if err != testErr {
			t.Errorf("Expected test error, got %v", err)
		}
	}

	// Circuit should now be open
	if cb.State() != StateOpen {
		t.Errorf("State after failures = %v, want StateOpen", cb.State())
	}
}

func TestCircuitBreaker_RejectsWhenOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 1,
		Timeout:          1 * time.Hour, // Long timeout to stay open
	}
	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.Execute(func() error {
		return errors.New("test error")
	})

	// Next call should be rejected
	callCount := 0
	err := cb.Execute(func() error {
		callCount++
		return nil
	})

	if err != ErrCircuitOpen {
		t.Errorf("Expected ErrCircuitOpen, got %v", err)
	}
	if callCount != 0 {
		t.Error("Function should not be called when circuit is open")
	}
}

func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 1,
		Timeout:          50 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.Execute(func() error {
		return errors.New("test error")
	})

	if cb.State() != StateOpen {
		t.Fatal("Circuit should be open")
	}

	// Wait for timeout
	time.Sleep(100 * time.Millisecond)

	// State should report half-open (even though actual transition happens on Execute)
	if cb.State() != StateHalfOpen {
		t.Errorf("State after timeout = %v, want StateHalfOpen", cb.State())
	}

	// Execute should now transition to half-open and allow the call
	executed := false
	err := cb.Execute(func() error {
		executed = true
		return nil
	})

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !executed {
		t.Error("Function should have been executed")
	}
}

func TestCircuitBreaker_ClosesAfterSuccesses(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 2,
		Timeout:          50 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.Execute(func() error {
		return errors.New("test error")
	})

	// Wait for timeout to transition to half-open
	time.Sleep(100 * time.Millisecond)

	// First success in half-open
	err := cb.Execute(func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Should still be half-open (not enough successes yet)
	// Note: State() returns half-open when timeout has passed, so we need to check via Execute
	callCount := 0
	err = cb.Execute(func() error {
		callCount++
		return nil
	})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Error("Function should have been called in half-open state")
	}

	// Second success should close the circuit
	err = cb.Execute(func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Circuit should now be closed
	if cb.State() != StateClosed {
		t.Errorf("State after successes = %v, want StateClosed", cb.State())
	}
}

func TestCircuitBreaker_ResetsOnSuccess(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          1 * time.Second,
	}
	cb := NewCircuitBreaker(config)

	testErr := errors.New("test error")

	// Generate some failures but not enough to open
	for i := 0; i < 3; i++ {
		cb.Execute(func() error {
			return testErr
		})
	}

	if cb.State() != StateClosed {
		t.Fatal("Circuit should still be closed")
	}

	// Success should reset failures
	err := cb.Execute(func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Now we need 5 more failures to open
	for i := 0; i < 5; i++ {
		cb.Execute(func() error {
			return testErr
		})
	}

	// Circuit should now be open
	if cb.State() != StateOpen {
		t.Errorf("State after reset and failures = %v, want StateOpen", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenFailureReopens(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 3,
		Timeout:          50 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.Execute(func() error {
		return errors.New("test error")
	})

	// Wait for timeout
	time.Sleep(100 * time.Millisecond)

	// First success in half-open
	cb.Execute(func() error {
		return nil
	})

	// Then failure in half-open should immediately reopen
	err := cb.Execute(func() error {
		return errors.New("another error")
	})
	if err == nil {
		t.Error("Expected error")
	}

	// Circuit should be open again
	if cb.State() != StateOpen {
		t.Errorf("State after half-open failure = %v, want StateOpen", cb.State())
	}
}

func TestCircuitBreaker_StateString(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("State(%d).String() = %s, want %s", tt.state, got, tt.expected)
		}
	}
}

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	config := DefaultCircuitBreakerConfig()

	if config.FailureThreshold != 5 {
		t.Errorf("FailureThreshold = %d, want 5", config.FailureThreshold)
	}
	if config.SuccessThreshold != 2 {
		t.Errorf("SuccessThreshold = %d, want 2", config.SuccessThreshold)
	}
	if config.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", config.Timeout)
	}
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 100,
		SuccessThreshold: 10,
		Timeout:          1 * time.Second,
	}
	cb := NewCircuitBreaker(config)

	// Run concurrent executions
	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func() {
			cb.Execute(func() error {
				return nil
			})
			done <- true
		}()
	}

	// Wait for all to complete
	for i := 0; i < 100; i++ {
		<-done
	}

	// Circuit should still be closed
	if cb.State() != StateClosed {
		t.Errorf("State after concurrent successes = %v, want StateClosed", cb.State())
	}
}
