package util

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"
)

func TestRetry_Success(t *testing.T) {
	// Test que Retry retourne nil quand fn réussit du premier coup
	callCount := 0
	fn := func() error {
		callCount++
		return nil
	}

	config := DefaultRetryConfig()
	ctx := context.Background()

	err := Retry(ctx, config, fn)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 call, got: %d", callCount)
	}
}

func TestRetry_SuccessAfterRetries(t *testing.T) {
	// Test que Retry réessaye et finit par réussir
	callCount := 0
	fn := func() error {
		callCount++
		if callCount < 3 {
			return errors.New("temporary error")
		}
		return nil
	}

	config := RetryConfig{
		MaxAttempts:  5,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}
	ctx := context.Background()

	err := Retry(ctx, config, fn)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if callCount != 3 {
		t.Fatalf("expected 3 calls, got: %d", callCount)
	}
}

func TestRetry_MaxAttemptsExceeded(t *testing.T) {
	// Test que Retry retourne l'erreur après MaxAttempts échecs
	callCount := 0
	expectedErr := errors.New("persistent error")
	fn := func() error {
		callCount++
		return expectedErr
	}

	config := RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}
	ctx := context.Background()

	err := Retry(ctx, config, fn)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if callCount != 3 {
		t.Fatalf("expected 3 calls, got: %d", callCount)
	}
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error to wrap %v, got: %v", expectedErr, err)
	}
}

func TestRetry_ContextCancellation(t *testing.T) {
	// Test que Retry respecte la cancellation du context
	callCount := 0
	fn := func() error {
		callCount++
		return errors.New("error")
	}

	config := RetryConfig{
		MaxAttempts:  10,
		InitialDelay: 1 * time.Second, // Long delay to ensure cancellation happens first
		MaxDelay:     5 * time.Second,
		Multiplier:   2.0,
	}

	ctx, cancel := context.WithCancel(context.Background())
	
	// Cancel context immediately
	cancel()

	err := Retry(ctx, config, fn)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if callCount != 1 {
		t.Fatalf("expected 1 call before cancellation, got: %d", callCount)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled error, got: %v", err)
	}
}

func TestRetry_ContextTimeout(t *testing.T) {
	// Test que Retry respecte le timeout du context
	callCount := 0
	fn := func() error {
		callCount++
		return errors.New("error")
	}

	config := RetryConfig{
		MaxAttempts:  10,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	err := Retry(ctx, config, fn)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Should have made 1 call and started waiting for the second, but timeout happens
	if callCount < 1 {
		t.Fatalf("expected at least 1 call, got: %d", callCount)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded error, got: %v", err)
	}
}

func TestRetry_ExponentialBackoff(t *testing.T) {
	// Test que les délais augmentent exponentiellement
	delays := []time.Duration{}
	startTimes := []time.Time{}

	fn := func() error {
		startTimes = append(startTimes, time.Now())
		return errors.New("error")
	}

	config := RetryConfig{
		MaxAttempts:  4,
		InitialDelay: 50 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
	}
	ctx := context.Background()

	Retry(ctx, config, fn)

	// Calculate delays between attempts
	for i := 1; i < len(startTimes); i++ {
		delays = append(delays, startTimes[i].Sub(startTimes[i-1]))
	}

	// Check that delays increase exponentially
	// First delay should be around 50ms, second around 100ms, third around 200ms
	if len(delays) < 2 {
		t.Fatalf("expected at least 2 delays, got: %d", len(delays))
	}

	// Allow for some timing variance (±30%)
	for i, delay := range delays {
		expectedDelay := time.Duration(float64(config.InitialDelay) * math.Pow(config.Multiplier, float64(i)))
		tolerance := time.Duration(float64(expectedDelay) * 0.3)
		
		if delay < expectedDelay-tolerance || delay > expectedDelay+tolerance {
			t.Errorf("delay %d: expected ~%v, got %v (tolerance: ±%v)", i+1, expectedDelay, delay, tolerance)
		}
	}
}

func TestRetry_NonRetryableError(t *testing.T) {
	// Test que Retry s'arrête immédiatement sur une erreur non retryable
	retryableErr := errors.New("retryable")
	nonRetryableErr := errors.New("non-retryable")

	callCount := 0
	fn := func() error {
		callCount++
		return nonRetryableErr
	}

	config := RetryConfig{
		MaxAttempts:     5,
		InitialDelay:    10 * time.Millisecond,
		MaxDelay:        100 * time.Millisecond,
		Multiplier:      2.0,
		RetryableErrors: []error{retryableErr},
	}
	ctx := context.Background()

	err := Retry(ctx, config, fn)
	if err != nonRetryableErr {
		t.Fatalf("expected nonRetryableErr, got: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 call (immediate stop on non-retryable error), got: %d", callCount)
	}
}

func TestRetry_RetryableError(t *testing.T) {
	// Test que Retry continue sur une erreur retryable
	retryableErr := errors.New("retryable")

	callCount := 0
	fn := func() error {
		callCount++
		if callCount < 3 {
			return retryableErr
		}
		return nil
	}

	config := RetryConfig{
		MaxAttempts:     5,
		InitialDelay:    10 * time.Millisecond,
		MaxDelay:        100 * time.Millisecond,
		Multiplier:      2.0,
		RetryableErrors: []error{retryableErr},
	}
	ctx := context.Background()

	err := Retry(ctx, config, fn)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if callCount != 3 {
		t.Fatalf("expected 3 calls, got: %d", callCount)
	}
}

func TestRetry_DefaultValues(t *testing.T) {
	// Test que les valeurs par défaut sont appliquées correctement
	callCount := 0
	fn := func() error {
		callCount++
		return errors.New("error")
	}

	config := RetryConfig{
		// Leave all fields at zero values
	}
	ctx := context.Background()

	err := Retry(ctx, config, fn)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Should use default MaxAttempts = 3
	if callCount != 3 {
		t.Fatalf("expected 3 calls (default MaxAttempts), got: %d", callCount)
	}
}

func TestRetryWithResult_Success(t *testing.T) {
	// Test RetryWithResult avec succès
	callCount := 0
	expectedResult := "success"
	fn := func() (string, error) {
		callCount++
		return expectedResult, nil
	}

	config := DefaultRetryConfig()
	ctx := context.Background()

	result, err := RetryWithResult(ctx, config, fn)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if result != expectedResult {
		t.Fatalf("expected result %q, got: %q", expectedResult, result)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 call, got: %d", callCount)
	}
}

func TestRetryWithResult_SuccessAfterRetries(t *testing.T) {
	// Test RetryWithResult qui réussit après plusieurs tentatives
	callCount := 0
	expectedResult := 42
	fn := func() (int, error) {
		callCount++
		if callCount < 3 {
			return 0, errors.New("temporary error")
		}
		return expectedResult, nil
	}

	config := RetryConfig{
		MaxAttempts:  5,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}
	ctx := context.Background()

	result, err := RetryWithResult(ctx, config, fn)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if result != expectedResult {
		t.Fatalf("expected result %d, got: %d", expectedResult, result)
	}
	if callCount != 3 {
		t.Fatalf("expected 3 calls, got: %d", callCount)
	}
}

func TestRetryWithResult_MaxAttemptsExceeded(t *testing.T) {
	// Test RetryWithResult qui échoue après MaxAttempts
	callCount := 0
	expectedErr := errors.New("persistent error")
	fn := func() (string, error) {
		callCount++
		return "", expectedErr
	}

	config := RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}
	ctx := context.Background()

	result, err := RetryWithResult(ctx, config, fn)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != "" {
		t.Fatalf("expected empty result, got: %q", result)
	}
	if callCount != 3 {
		t.Fatalf("expected 3 calls, got: %d", callCount)
	}
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error to wrap %v, got: %v", expectedErr, err)
	}
}

func TestIsRetryable(t *testing.T) {
	retryableErr := errors.New("retryable error")
	otherErr := errors.New("other error")

	tests := []struct {
		name            string
		err             error
		retryableErrors []error
		want            bool
	}{
		{
			name:            "nil error",
			err:             nil,
			retryableErrors: []error{retryableErr},
			want:            false,
		},
		{
			name:            "retryable error",
			err:             retryableErr,
			retryableErrors: []error{retryableErr},
			want:            true,
		},
		{
			name:            "non-retryable error",
			err:             otherErr,
			retryableErrors: []error{retryableErr},
			want:            false,
		},
		{
			name:            "wrapped retryable error",
			err:             fmt.Errorf("wrapped: %w", retryableErr),
			retryableErrors: []error{retryableErr},
			want:            true,
		},
		{
			name:            "empty retryable list",
			err:             errors.New("any"),
			retryableErrors: []error{},
			want:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRetryable(tt.err, tt.retryableErrors)
			if got != tt.want {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCalculateDelay(t *testing.T) {
	tests := []struct {
		name         string
		initialDelay time.Duration
		maxDelay     time.Duration
		multiplier   float64
		attempt      int
		want         time.Duration
	}{
		{
			name:         "first attempt",
			initialDelay: 100 * time.Millisecond,
			maxDelay:     30 * time.Second,
			multiplier:   2.0,
			attempt:      0,
			want:         100 * time.Millisecond,
		},
		{
			name:         "second attempt",
			initialDelay: 100 * time.Millisecond,
			maxDelay:     30 * time.Second,
			multiplier:   2.0,
			attempt:      1,
			want:         200 * time.Millisecond,
		},
		{
			name:         "third attempt",
			initialDelay: 100 * time.Millisecond,
			maxDelay:     30 * time.Second,
			multiplier:   2.0,
			attempt:      2,
			want:         400 * time.Millisecond,
		},
		{
			name:         "capped at maxDelay",
			initialDelay: 10 * time.Second,
			maxDelay:     15 * time.Second,
			multiplier:   2.0,
			attempt:      1,
			want:         15 * time.Second,
		},
		{
			name:         "custom multiplier",
			initialDelay: 100 * time.Millisecond,
			maxDelay:     30 * time.Second,
			multiplier:   3.0,
			attempt:      2,
			want:         900 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateDelay(tt.initialDelay, tt.maxDelay, tt.multiplier, tt.attempt)
			if got != tt.want {
				t.Errorf("calculateDelay() = %v, want %v", got, tt.want)
			}
		})
	}
}
