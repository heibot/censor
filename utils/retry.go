package utils

import (
	"context"
	"math"
	"math/rand"
	"time"

	censor "github.com/heibot/censor"
)

// RetryConfig configures the retry behavior.
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts (0 means no retries).
	MaxRetries int

	// InitialDelay is the initial delay before the first retry.
	InitialDelay time.Duration

	// MaxDelay is the maximum delay between retries.
	MaxDelay time.Duration

	// Multiplier is the factor by which the delay increases after each retry.
	Multiplier float64

	// Jitter adds randomness to the delay to prevent thundering herd.
	// Value between 0 and 1, where 0.1 means ±10% jitter.
	Jitter float64

	// RetryIf is a function that determines if an error is retryable.
	// If nil, uses censor.IsRetryable.
	RetryIf func(error) bool

	// OnRetry is called before each retry attempt.
	OnRetry func(attempt int, err error, delay time.Duration)
}

// DefaultRetryConfig returns sensible defaults for retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:   3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.1,
		RetryIf:      censor.IsRetryable,
	}
}

// Retryer provides retry functionality with exponential backoff.
type Retryer struct {
	config RetryConfig
}

// NewRetryer creates a new retryer with the given configuration.
func NewRetryer(config RetryConfig) *Retryer {
	if config.RetryIf == nil {
		config.RetryIf = censor.IsRetryable
	}
	if config.InitialDelay == 0 {
		config.InitialDelay = 1 * time.Second
	}
	if config.MaxDelay == 0 {
		config.MaxDelay = 30 * time.Second
	}
	if config.Multiplier == 0 {
		config.Multiplier = 2.0
	}
	return &Retryer{config: config}
}

// RetryResult contains the result of a retry operation.
type RetryResult[T any] struct {
	Value    T
	Attempts int
	Errors   []error
}

// Do executes the function with retry logic.
func (r *Retryer) Do(ctx context.Context, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		// Execute the function
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if we should retry
		if attempt >= r.config.MaxRetries || !r.config.RetryIf(err) {
			break
		}

		// Calculate delay with exponential backoff
		delay := r.calculateDelay(attempt)

		// Call OnRetry callback
		if r.config.OnRetry != nil {
			r.config.OnRetry(attempt+1, err, delay)
		}

		// Wait before retry
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}

	return lastErr
}

// DoWithResult executes the function with retry logic and returns the result.
func DoWithResult[T any](ctx context.Context, r *Retryer, fn func() (T, error)) (T, error) {
	var result T
	var lastErr error

	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		// Execute the function
		val, err := fn()
		if err == nil {
			return val, nil
		}

		lastErr = err

		// Check if we should retry
		if attempt >= r.config.MaxRetries || !r.config.RetryIf(err) {
			break
		}

		// Calculate delay with exponential backoff
		delay := r.calculateDelay(attempt)

		// Call OnRetry callback
		if r.config.OnRetry != nil {
			r.config.OnRetry(attempt+1, err, delay)
		}

		// Wait before retry
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(delay):
		}
	}

	return result, lastErr
}

// calculateDelay calculates the delay for a given attempt using exponential backoff.
func (r *Retryer) calculateDelay(attempt int) time.Duration {
	// Calculate base delay: initialDelay * (multiplier ^ attempt)
	delay := float64(r.config.InitialDelay) * math.Pow(r.config.Multiplier, float64(attempt))

	// Apply jitter
	if r.config.Jitter > 0 {
		jitterRange := delay * r.config.Jitter
		jitter := (rand.Float64()*2 - 1) * jitterRange // Random between -jitterRange and +jitterRange
		delay += jitter
	}

	// Cap at max delay
	if delay > float64(r.config.MaxDelay) {
		delay = float64(r.config.MaxDelay)
	}

	return time.Duration(delay)
}

// Retry is a convenience function for simple retry operations.
func Retry(ctx context.Context, maxRetries int, fn func() error) error {
	r := NewRetryer(RetryConfig{MaxRetries: maxRetries})
	return r.Do(ctx, fn)
}

// RetryWithBackoff retries with configurable backoff parameters.
func RetryWithBackoff(ctx context.Context, maxRetries int, initialDelay, maxDelay time.Duration, fn func() error) error {
	r := NewRetryer(RetryConfig{
		MaxRetries:   maxRetries,
		InitialDelay: initialDelay,
		MaxDelay:     maxDelay,
		Multiplier:   2.0,
		Jitter:       0.1,
	})
	return r.Do(ctx, fn)
}

// RetryWithCallback retries with a callback on each retry.
func RetryWithCallback(ctx context.Context, config RetryConfig, fn func() error) error {
	r := NewRetryer(config)
	return r.Do(ctx, fn)
}
