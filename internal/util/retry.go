package util

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
)

// RetryConfig configures retry behavior
type RetryConfig struct {
	MaxAttempts     int           // Maximum number of attempts (default: 3)
	InitialDelay    time.Duration // Initial delay between retries (default: 100ms)
	MaxDelay        time.Duration // Maximum delay between retries (default: 30s)
	Multiplier      float64       // Exponential backoff multiplier (default: 2)
	RetryableErrors []error       // List of errors that should trigger a retry
}

// DefaultRetryConfig returns sensible default configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
	}
}

// RetryableFunc is the function signature for retryable operations
type RetryableFunc func() error

// Retry executes the given function with exponential backoff retry logic
func Retry(ctx context.Context, config RetryConfig, fn RetryableFunc) error {
	// Validate and set defaults
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 3
	}
	if config.InitialDelay <= 0 {
		config.InitialDelay = 100 * time.Millisecond
	}
	if config.MaxDelay <= 0 {
		config.MaxDelay = 30 * time.Second
	}
	if config.Multiplier <= 1 {
		config.Multiplier = 2.0
	}

	var lastErr error

	for attempt := 0; attempt < config.MaxAttempts; attempt++ {
		// Execute the function
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if len(config.RetryableErrors) > 0 && !IsRetryable(err, config.RetryableErrors) {
			return err
		}

		// If this is the last attempt, return the error
		if attempt == config.MaxAttempts-1 {
			break
		}

		// Calculate delay with exponential backoff
		delay := calculateDelay(config.InitialDelay, config.MaxDelay, config.Multiplier, attempt)

		// Wait for delay or context cancellation
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("retry cancelled: %w", ctx.Err())
		case <-timer.C:
			// Continue to next attempt
		}
	}

	return fmt.Errorf("retry failed after %d attempts: %w", config.MaxAttempts, lastErr)
}

// RetryWithResult is like Retry but returns a result
func RetryWithResult[T any](ctx context.Context, config RetryConfig, fn func() (T, error)) (T, error) {
	var zero T

	// Validate and set defaults
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 3
	}
	if config.InitialDelay <= 0 {
		config.InitialDelay = 100 * time.Millisecond
	}
	if config.MaxDelay <= 0 {
		config.MaxDelay = 30 * time.Second
	}
	if config.Multiplier <= 1 {
		config.Multiplier = 2.0
	}

	var lastErr error

	for attempt := 0; attempt < config.MaxAttempts; attempt++ {
		// Execute the function
		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Check if error is retryable
		if len(config.RetryableErrors) > 0 && !IsRetryable(err, config.RetryableErrors) {
			return zero, err
		}

		// If this is the last attempt, return the error
		if attempt == config.MaxAttempts-1 {
			break
		}

		// Calculate delay with exponential backoff
		delay := calculateDelay(config.InitialDelay, config.MaxDelay, config.Multiplier, attempt)

		// Wait for delay or context cancellation
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return zero, fmt.Errorf("retry cancelled: %w", ctx.Err())
		case <-timer.C:
			// Continue to next attempt
		}
	}

	return zero, fmt.Errorf("retry failed after %d attempts: %w", config.MaxAttempts, lastErr)
}

// IsRetryable checks if an error is retryable
func IsRetryable(err error, retryableErrors []error) bool {
	if err == nil {
		return false
	}

	for _, retryableErr := range retryableErrors {
		if errors.Is(err, retryableErr) || err == retryableErr {
			return true
		}
	}

	return false
}

// calculateDelay computes the delay for a given attempt using exponential backoff
func calculateDelay(initialDelay, maxDelay time.Duration, multiplier float64, attempt int) time.Duration {
	// Calculate: initialDelay * multiplier^attempt
	factor := math.Pow(multiplier, float64(attempt))
	delay := time.Duration(float64(initialDelay) * factor)

	// Cap at maxDelay
	if delay > maxDelay {
		delay = maxDelay
	}

	return delay
}
