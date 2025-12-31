package censor

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

// ErrorCategory represents the category of an error for handling decisions.
type ErrorCategory string

const (
	ErrorCategoryNetwork    ErrorCategory = "network"    // Network connectivity issues
	ErrorCategoryRateLimit  ErrorCategory = "rate_limit" // Rate limiting
	ErrorCategoryTimeout    ErrorCategory = "timeout"    // Request timeout
	ErrorCategoryAuth       ErrorCategory = "auth"       // Authentication/authorization
	ErrorCategoryConfig     ErrorCategory = "config"     // Configuration issues
	ErrorCategoryValidation ErrorCategory = "validation" // Input validation
	ErrorCategoryProvider   ErrorCategory = "provider"   // Provider-specific errors
	ErrorCategoryInternal   ErrorCategory = "internal"   // Internal errors
)

// Common errors
var (
	ErrNoResources        = errors.New("censor: no resources provided")
	ErrInvalidResource    = errors.New("censor: invalid resource")
	ErrProviderNotFound   = errors.New("censor: provider not found")
	ErrStoreNotConfigured = errors.New("censor: store not configured")
	ErrTaskNotFound       = errors.New("censor: task not found")
	ErrCallbackInvalid    = errors.New("censor: callback signature invalid")
	ErrTimeout            = errors.New("censor: operation timeout")
	ErrRateLimited        = errors.New("censor: rate limited by provider")
	ErrContentTooLarge    = errors.New("censor: content exceeds size limit")
	ErrUnsupportedType    = errors.New("censor: unsupported resource type")
	ErrDuplicateSubmit    = errors.New("censor: duplicate submission")
	ErrRevisionConflict   = errors.New("censor: revision conflict, stale update")

	// Network errors
	ErrNetworkUnreachable = errors.New("censor: network unreachable")
	ErrConnectionRefused  = errors.New("censor: connection refused")
	ErrDNSResolution      = errors.New("censor: DNS resolution failed")

	// Auth errors
	ErrAuthFailed        = errors.New("censor: authentication failed")
	ErrPermissionDenied  = errors.New("censor: permission denied")
	ErrInvalidCredential = errors.New("censor: invalid credentials")

	// Config errors
	ErrMissingConfig    = errors.New("censor: missing required configuration")
	ErrInvalidConfig    = errors.New("censor: invalid configuration")
	ErrProviderDisabled = errors.New("censor: provider is disabled")
)

// ProviderError represents an error from a cloud provider.
type ProviderError struct {
	Provider   string        // Provider name (aliyun, huawei, tencent)
	Code       string        // Error code from provider
	Message    string        // Error message
	StatusCode int           // HTTP status code if applicable
	Category   ErrorCategory // Error category for handling
	Retryable  bool          // Whether this error is retryable
	Raw        any           // Raw error response
	Err        error         // Underlying error
}

func (e *ProviderError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("censor: provider %s error [%d/%s]: %s", e.Provider, e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("censor: provider %s error [%s]: %s", e.Provider, e.Code, e.Message)
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}

// NewProviderError creates a new provider error.
func NewProviderError(provider, code, message string) *ProviderError {
	pe := &ProviderError{
		Provider: provider,
		Code:     code,
		Message:  message,
		Category: ErrorCategoryProvider,
	}
	pe.Retryable = pe.isRetryable()
	return pe
}

// WithStatusCode sets the HTTP status code.
func (e *ProviderError) WithStatusCode(code int) *ProviderError {
	e.StatusCode = code
	e.Category = categorizeByStatusCode(code)
	e.Retryable = e.isRetryable()
	return e
}

// WithCategory sets the error category.
func (e *ProviderError) WithCategory(cat ErrorCategory) *ProviderError {
	e.Category = cat
	e.Retryable = e.isRetryable()
	return e
}

// WithRaw sets the raw error response.
func (e *ProviderError) WithRaw(raw any) *ProviderError {
	e.Raw = raw
	return e
}

// WithCause sets the underlying error.
func (e *ProviderError) WithCause(err error) *ProviderError {
	e.Err = err
	return e
}

func (e *ProviderError) isRetryable() bool {
	switch e.Category {
	case ErrorCategoryNetwork, ErrorCategoryRateLimit, ErrorCategoryTimeout:
		return true
	}
	switch e.StatusCode {
	case 429, 500, 502, 503, 504:
		return true
	}
	return false
}

func categorizeByStatusCode(code int) ErrorCategory {
	switch {
	case code == 401 || code == 403:
		return ErrorCategoryAuth
	case code == 429:
		return ErrorCategoryRateLimit
	case code == 408 || code == 504:
		return ErrorCategoryTimeout
	case code >= 500:
		return ErrorCategoryInternal
	default:
		return ErrorCategoryProvider
	}
}

// ValidationError represents a validation error.
type ValidationError struct {
	Field   string // Field that failed validation
	Message string // Validation error message
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("censor: validation error on %s: %s", e.Field, e.Message)
}

// NewValidationError creates a new validation error.
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Message: message,
	}
}

// StoreError represents a database/store error.
type StoreError struct {
	Operation string // Operation that failed (create, update, query)
	Table     string // Table/collection name
	Err       error  // Underlying error
}

func (e *StoreError) Error() string {
	return fmt.Sprintf("censor: store error during %s on %s: %v", e.Operation, e.Table, e.Err)
}

func (e *StoreError) Unwrap() error {
	return e.Err
}

// NewStoreError creates a new store error.
func NewStoreError(operation, table string, err error) *StoreError {
	return &StoreError{
		Operation: operation,
		Table:     table,
		Err:       err,
	}
}

// IsProviderError checks if an error is a provider error.
func IsProviderError(err error) bool {
	var pe *ProviderError
	return errors.As(err, &pe)
}

// IsValidationError checks if an error is a validation error.
func IsValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}

// IsStoreError checks if an error is a store error.
func IsStoreError(err error) bool {
	var se *StoreError
	return errors.As(err, &se)
}

// IsRetryable checks if an error is retryable.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Check sentinel errors
	if errors.Is(err, ErrTimeout) || errors.Is(err, ErrRateLimited) ||
		errors.Is(err, ErrNetworkUnreachable) || errors.Is(err, ErrConnectionRefused) {
		return true
	}

	// Check provider error
	var pe *ProviderError
	if errors.As(err, &pe) {
		return pe.Retryable
	}

	// Check for network errors
	if IsNetworkError(err) {
		return true
	}

	return false
}

// IsNetworkError checks if an error is a network-related error.
func IsNetworkError(err error) bool {
	if err == nil {
		return false
	}

	// Check sentinel errors
	if errors.Is(err, ErrNetworkUnreachable) || errors.Is(err, ErrConnectionRefused) ||
		errors.Is(err, ErrDNSResolution) {
		return true
	}

	// Check for net.Error
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	// Check for common network error patterns in message
	msg := strings.ToLower(err.Error())
	networkPatterns := []string{
		"connection refused",
		"connection reset",
		"no such host",
		"network is unreachable",
		"i/o timeout",
		"connection timed out",
		"dial tcp",
		"dial udp",
	}
	for _, pattern := range networkPatterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}

	return false
}

// IsAuthError checks if an error is an authentication/authorization error.
func IsAuthError(err error) bool {
	if errors.Is(err, ErrAuthFailed) || errors.Is(err, ErrPermissionDenied) ||
		errors.Is(err, ErrInvalidCredential) {
		return true
	}

	var pe *ProviderError
	if errors.As(err, &pe) {
		return pe.Category == ErrorCategoryAuth
	}

	return false
}

// IsConfigError checks if an error is a configuration error.
func IsConfigError(err error) bool {
	if errors.Is(err, ErrMissingConfig) || errors.Is(err, ErrInvalidConfig) ||
		errors.Is(err, ErrProviderDisabled) {
		return true
	}

	var pe *ProviderError
	if errors.As(err, &pe) {
		return pe.Category == ErrorCategoryConfig
	}

	return false
}

// IsRateLimitError checks if an error is a rate limit error.
func IsRateLimitError(err error) bool {
	if errors.Is(err, ErrRateLimited) {
		return true
	}

	var pe *ProviderError
	if errors.As(err, &pe) {
		return pe.Category == ErrorCategoryRateLimit || pe.StatusCode == 429
	}

	return false
}

// GetErrorCategory returns the category of an error.
func GetErrorCategory(err error) ErrorCategory {
	if err == nil {
		return ""
	}

	var pe *ProviderError
	if errors.As(err, &pe) {
		return pe.Category
	}

	if IsNetworkError(err) {
		return ErrorCategoryNetwork
	}
	if errors.Is(err, ErrTimeout) {
		return ErrorCategoryTimeout
	}
	if errors.Is(err, ErrRateLimited) {
		return ErrorCategoryRateLimit
	}
	if IsAuthError(err) {
		return ErrorCategoryAuth
	}
	if IsConfigError(err) {
		return ErrorCategoryConfig
	}

	var ve *ValidationError
	if errors.As(err, &ve) {
		return ErrorCategoryValidation
	}

	return ErrorCategoryInternal
}

// WrapNetworkError wraps a network error with appropriate sentinel error.
func WrapNetworkError(err error) error {
	if err == nil {
		return nil
	}

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "connection refused") {
		return fmt.Errorf("%w: %v", ErrConnectionRefused, err)
	}
	if strings.Contains(msg, "no such host") || strings.Contains(msg, "dns") {
		return fmt.Errorf("%w: %v", ErrDNSResolution, err)
	}
	if strings.Contains(msg, "network is unreachable") {
		return fmt.Errorf("%w: %v", ErrNetworkUnreachable, err)
	}
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "timed out") {
		return fmt.Errorf("%w: %v", ErrTimeout, err)
	}

	return err
}
