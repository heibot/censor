package utils

import (
	"context"
	"errors"
	"testing"
	"time"

	censor "github.com/heibot/censor"
)

func TestRetryer_Do_Success(t *testing.T) {
	r := NewRetryer(RetryConfig{MaxRetries: 3})

	callCount := 0
	err := r.Do(context.Background(), func() error {
		callCount++
		return nil
	})

	if err != nil {
		t.Errorf("Do() error = %v, want nil", err)
	}
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1", callCount)
	}
}

func TestRetryer_Do_RetrySuccess(t *testing.T) {
	r := NewRetryer(RetryConfig{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
	})

	callCount := 0
	err := r.Do(context.Background(), func() error {
		callCount++
		if callCount < 3 {
			return censor.ErrTimeout // Retryable error
		}
		return nil
	})

	if err != nil {
		t.Errorf("Do() error = %v, want nil", err)
	}
	if callCount != 3 {
		t.Errorf("callCount = %d, want 3", callCount)
	}
}

func TestRetryer_Do_MaxRetriesExceeded(t *testing.T) {
	r := NewRetryer(RetryConfig{
		MaxRetries:   2,
		InitialDelay: 10 * time.Millisecond,
	})

	callCount := 0
	expectedErr := censor.ErrTimeout
	err := r.Do(context.Background(), func() error {
		callCount++
		return expectedErr
	})

	if !errors.Is(err, expectedErr) {
		t.Errorf("Do() error = %v, want %v", err, expectedErr)
	}
	// Initial call + 2 retries = 3 total calls
	if callCount != 3 {
		t.Errorf("callCount = %d, want 3", callCount)
	}
}

func TestRetryer_Do_NonRetryableError(t *testing.T) {
	r := NewRetryer(RetryConfig{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
	})

	callCount := 0
	nonRetryableErr := errors.New("non-retryable error")
	err := r.Do(context.Background(), func() error {
		callCount++
		return nonRetryableErr
	})

	if err != nonRetryableErr {
		t.Errorf("Do() error = %v, want %v", err, nonRetryableErr)
	}
	// Should only be called once since error is not retryable
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1", callCount)
	}
}

func TestRetryer_Do_ContextCanceled(t *testing.T) {
	r := NewRetryer(RetryConfig{
		MaxRetries:   10,
		InitialDelay: 100 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())

	callCount := 0
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := r.Do(ctx, func() error {
		callCount++
		return censor.ErrTimeout
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Do() error = %v, want context.Canceled", err)
	}
}

func TestRetryer_OnRetryCallback(t *testing.T) {
	retryAttempts := []int{}

	r := NewRetryer(RetryConfig{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		OnRetry: func(attempt int, err error, delay time.Duration) {
			retryAttempts = append(retryAttempts, attempt)
		},
	})

	r.Do(context.Background(), func() error {
		return censor.ErrTimeout
	})

	// Should have 3 retries (attempts 1, 2, 3)
	if len(retryAttempts) != 3 {
		t.Errorf("retryAttempts = %v, want 3 attempts", retryAttempts)
	}
}

func TestRetryer_ExponentialBackoff(t *testing.T) {
	r := NewRetryer(RetryConfig{
		MaxRetries:   3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
		Jitter:       0, // No jitter for predictable testing
	})

	delays := []time.Duration{}
	r.config.OnRetry = func(attempt int, err error, delay time.Duration) {
		delays = append(delays, delay)
	}

	start := time.Now()
	r.Do(context.Background(), func() error {
		return censor.ErrTimeout
	})
	elapsed := time.Since(start)

	// Expected delays: 100ms, 200ms, 400ms = 700ms total
	expectedMin := 600 * time.Millisecond
	expectedMax := 900 * time.Millisecond

	if elapsed < expectedMin || elapsed > expectedMax {
		t.Errorf("elapsed = %v, want between %v and %v", elapsed, expectedMin, expectedMax)
	}
}

func TestRetry_Convenience(t *testing.T) {
	callCount := 0
	err := Retry(context.Background(), 2, func() error {
		callCount++
		if callCount < 2 {
			return censor.ErrTimeout
		}
		return nil
	})

	if err != nil {
		t.Errorf("Retry() error = %v, want nil", err)
	}
	if callCount != 2 {
		t.Errorf("callCount = %d, want 2", callCount)
	}
}

func TestDoWithResult(t *testing.T) {
	r := NewRetryer(RetryConfig{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
	})

	callCount := 0
	result, err := DoWithResult(context.Background(), r, func() (string, error) {
		callCount++
		if callCount < 2 {
			return "", censor.ErrTimeout
		}
		return "success", nil
	})

	if err != nil {
		t.Errorf("DoWithResult() error = %v, want nil", err)
	}
	if result != "success" {
		t.Errorf("result = %q, want %q", result, "success")
	}
	if callCount != 2 {
		t.Errorf("callCount = %d, want 2", callCount)
	}
}

func TestRetryer_MaxDelayRespected(t *testing.T) {
	r := NewRetryer(RetryConfig{
		MaxRetries:   5,
		InitialDelay: 1 * time.Second,
		MaxDelay:     100 * time.Millisecond, // Very small max
		Multiplier:   10.0,
		Jitter:       0,
	})

	delays := []time.Duration{}
	r.config.OnRetry = func(attempt int, err error, delay time.Duration) {
		delays = append(delays, delay)
	}

	start := time.Now()
	r.Do(context.Background(), func() error {
		return censor.ErrTimeout
	})
	elapsed := time.Since(start)

	// All delays should be capped at 100ms
	for i, d := range delays {
		if d > 100*time.Millisecond {
			t.Errorf("delay[%d] = %v, want <= 100ms", i, d)
		}
	}

	// 5 retries * 100ms = 500ms max (plus some execution time)
	if elapsed > 700*time.Millisecond {
		t.Errorf("elapsed = %v, want < 700ms", elapsed)
	}
}
