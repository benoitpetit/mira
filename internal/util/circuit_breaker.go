// Package util provides shared utility functions and resilience patterns.
package util

import (
	"errors"
	"sync"
	"time"
)

// State represents the circuit breaker state
type State int

const (
	StateClosed State = iota // Normal operation
	StateOpen                // Failing, reject requests
	StateHalfOpen            // Testing if service recovered
)

// String returns a human-readable state name
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig configures circuit breaker behavior
type CircuitBreakerConfig struct {
	FailureThreshold int           // Number of failures before opening (default: 5)
	SuccessThreshold int           // Number of successes in half-open to close (default: 2)
	Timeout          time.Duration // Duration to stay open before half-open (default: 30s)
}

// DefaultCircuitBreakerConfig returns sensible defaults
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          30 * time.Second,
	}
}

// CircuitBreaker prevents cascade failures
type CircuitBreaker struct {
	config      CircuitBreakerConfig
	state       State
	failures    int
	successes   int
	lastFailure time.Time
	mu          sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		config: config,
		state:  StateClosed,
	}
}

// Execute runs the function with circuit breaker protection
func (cb *CircuitBreaker) Execute(fn func() error) error {
	cb.mu.Lock()

	// Check if we should transition from open to half-open
	if cb.state == StateOpen {
		if time.Since(cb.lastFailure) >= cb.config.Timeout {
			cb.state = StateHalfOpen
			cb.failures = 0
			cb.successes = 0
		} else {
			cb.mu.Unlock()
			return ErrCircuitOpen
		}
	}

	cb.mu.Unlock()

	// Execute the function
	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		// Handle failure
		cb.failures++
		cb.lastFailure = time.Now()

		if cb.state == StateHalfOpen {
			// In half-open state, any failure immediately opens the circuit
			cb.state = StateOpen
		} else if cb.failures >= cb.config.FailureThreshold {
			// In closed state, reach threshold to open
			cb.state = StateOpen
		}

		return err
	}

	// Handle success
	if cb.state == StateHalfOpen {
		cb.successes++
		if cb.successes >= cb.config.SuccessThreshold {
			// Success threshold reached, close the circuit
			cb.state = StateClosed
			cb.failures = 0
			cb.successes = 0
		}
	} else {
		// In closed state, reset failures on success
		cb.failures = 0
	}

	return nil
}

// State returns current state (for monitoring)
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	// Check if we should update state (open -> half-open) for accurate reporting
	if cb.state == StateOpen && time.Since(cb.lastFailure) >= cb.config.Timeout {
		// Don't modify state here, just report what it would be
		// The actual transition happens in Execute
		return StateHalfOpen
	}

	return cb.state
}

// ErrCircuitOpen is returned when circuit breaker is open
var ErrCircuitOpen = errors.New("circuit breaker is open")
