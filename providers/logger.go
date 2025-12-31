package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	censor "github.com/heibot/censor"
)

// APILogEntry represents a single API call log entry.
type APILogEntry struct {
	ID           string         `json:"id"`
	Timestamp    time.Time      `json:"timestamp"`
	Provider     string         `json:"provider"`
	Operation    string         `json:"operation"` // submit_text, submit_image, query, callback
	ResourceType string         `json:"resource_type,omitempty"`
	ResourceID   string         `json:"resource_id,omitempty"`
	TaskID       string         `json:"task_id,omitempty"`
	Duration     time.Duration  `json:"duration_ms"`
	Success      bool           `json:"success"`
	StatusCode   int            `json:"status_code,omitempty"`
	ErrorCode    string         `json:"error_code,omitempty"`
	ErrorMessage string         `json:"error_message,omitempty"`
	RetryCount   int            `json:"retry_count,omitempty"`
	RequestSize  int            `json:"request_size,omitempty"`
	ResponseSize int            `json:"response_size,omitempty"`
	Request      any            `json:"request,omitempty"`  // Sanitized request (no sensitive data)
	Response     any            `json:"response,omitempty"` // Sanitized response
	Extra        map[string]any `json:"extra,omitempty"`
}

// APILogger defines the interface for logging API calls.
type APILogger interface {
	// Log records an API log entry.
	Log(ctx context.Context, entry APILogEntry)

	// LogAsync records an API log entry asynchronously.
	LogAsync(ctx context.Context, entry APILogEntry)
}

// APILogStore defines the interface for persisting API logs.
type APILogStore interface {
	// SaveAPILog saves an API log entry to persistent storage.
	SaveAPILog(ctx context.Context, entry APILogEntry) error

	// ListAPILogs retrieves API logs with optional filtering.
	ListAPILogs(ctx context.Context, filter APILogFilter, limit, offset int) ([]APILogEntry, error)
}

// APILogFilter defines filtering options for API logs.
type APILogFilter struct {
	Provider    string
	Operation   string
	ResourceID  string
	TaskID      string
	SuccessOnly *bool
	StartTime   *time.Time
	EndTime     *time.Time
	MinDuration *time.Duration
}

// LogLevel defines the logging verbosity level.
type LogLevel int

const (
	LogLevelNone  LogLevel = iota // No logging
	LogLevelError                 // Only errors
	LogLevelInfo                  // Errors and basic info
	LogLevelDebug                 // Detailed logging including request/response
)

// LoggerConfig configures the API logger behavior.
type LoggerConfig struct {
	// Level controls the verbosity of logging.
	Level LogLevel

	// Store is an optional persistent storage for logs.
	Store APILogStore

	// LogRequest controls whether to log request bodies.
	LogRequest bool

	// LogResponse controls whether to log response bodies.
	LogResponse bool

	// SanitizeFunc is a function to sanitize sensitive data before logging.
	SanitizeFunc func(data any) any

	// AsyncBufferSize is the buffer size for async logging.
	AsyncBufferSize int

	// StdoutEnabled controls whether to print logs to stdout.
	StdoutEnabled bool
}

// DefaultLoggerConfig returns sensible defaults for logger configuration.
func DefaultLoggerConfig() LoggerConfig {
	return LoggerConfig{
		Level:           LogLevelInfo,
		LogRequest:      false,
		LogResponse:     false,
		AsyncBufferSize: 1000,
		StdoutEnabled:   true,
	}
}

// StandardLogger is the default implementation of APILogger.
type StandardLogger struct {
	config    LoggerConfig
	asyncChan chan APILogEntry
	wg        sync.WaitGroup
	closed    bool
	mu        sync.RWMutex
}

// NewStandardLogger creates a new standard logger.
func NewStandardLogger(config LoggerConfig) *StandardLogger {
	if config.AsyncBufferSize == 0 {
		config.AsyncBufferSize = 1000
	}

	l := &StandardLogger{
		config:    config,
		asyncChan: make(chan APILogEntry, config.AsyncBufferSize),
	}

	// Start async log processor
	l.wg.Add(1)
	go l.processAsyncLogs()

	return l
}

// Log records an API log entry synchronously.
func (l *StandardLogger) Log(ctx context.Context, entry APILogEntry) {
	l.logEntry(ctx, entry)
}

// LogAsync records an API log entry asynchronously.
func (l *StandardLogger) LogAsync(ctx context.Context, entry APILogEntry) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.closed {
		return
	}

	select {
	case l.asyncChan <- entry:
	default:
		// Buffer full, log synchronously
		l.logEntry(ctx, entry)
	}
}

func (l *StandardLogger) logEntry(ctx context.Context, entry APILogEntry) {
	// Check log level
	if l.config.Level == LogLevelNone {
		return
	}
	if l.config.Level == LogLevelError && entry.Success {
		return
	}

	// Generate ID if not set
	if entry.ID == "" {
		entry.ID = fmt.Sprintf("%s_%s_%d", entry.Provider, entry.Operation, time.Now().UnixNano())
	}

	// Sanitize data if needed
	if l.config.SanitizeFunc != nil {
		if entry.Request != nil {
			entry.Request = l.config.SanitizeFunc(entry.Request)
		}
		if entry.Response != nil {
			entry.Response = l.config.SanitizeFunc(entry.Response)
		}
	}

	// Remove request/response if not configured
	if !l.config.LogRequest {
		entry.Request = nil
	}
	if !l.config.LogResponse {
		entry.Response = nil
	}

	// Print to stdout if enabled
	if l.config.StdoutEnabled {
		l.printLog(entry)
	}

	// Save to store if configured
	if l.config.Store != nil {
		if err := l.config.Store.SaveAPILog(ctx, entry); err != nil {
			log.Printf("[APILogger] Failed to save log: %v", err)
		}
	}
}

func (l *StandardLogger) printLog(entry APILogEntry) {
	status := "✅"
	if !entry.Success {
		status = "❌"
	}

	durationMs := entry.Duration.Milliseconds()

	if entry.Success {
		log.Printf("[%s] %s %s/%s taskID=%s duration=%dms",
			entry.Provider, status, entry.Operation, entry.ResourceType,
			entry.TaskID, durationMs)
	} else {
		log.Printf("[%s] %s %s/%s taskID=%s duration=%dms error=[%s] %s",
			entry.Provider, status, entry.Operation, entry.ResourceType,
			entry.TaskID, durationMs, entry.ErrorCode, entry.ErrorMessage)
	}

	// Log extra details in debug mode
	if l.config.Level >= LogLevelDebug {
		if entry.Request != nil {
			if data, err := json.Marshal(entry.Request); err == nil {
				log.Printf("[%s] Request: %s", entry.Provider, string(data))
			}
		}
		if entry.Response != nil {
			if data, err := json.Marshal(entry.Response); err == nil {
				log.Printf("[%s] Response: %s", entry.Provider, string(data))
			}
		}
	}
}

func (l *StandardLogger) processAsyncLogs() {
	defer l.wg.Done()

	for entry := range l.asyncChan {
		l.logEntry(context.Background(), entry)
	}
}

// Close shuts down the logger and waits for pending logs to be processed.
func (l *StandardLogger) Close() {
	l.mu.Lock()
	l.closed = true
	l.mu.Unlock()

	close(l.asyncChan)
	l.wg.Wait()
}

// LogTimer is a helper for timing API calls.
type LogTimer struct {
	entry     APILogEntry
	startTime time.Time
	logger    APILogger
}

// StartLog starts timing an API call and returns a LogTimer.
func StartLog(logger APILogger, provider, operation string) *LogTimer {
	return &LogTimer{
		entry: APILogEntry{
			Provider:  provider,
			Operation: operation,
			Timestamp: time.Now(),
		},
		startTime: time.Now(),
		logger:    logger,
	}
}

// WithResource sets the resource information.
func (t *LogTimer) WithResource(resourceType censor.ResourceType, resourceID string) *LogTimer {
	t.entry.ResourceType = string(resourceType)
	t.entry.ResourceID = resourceID
	return t
}

// WithTaskID sets the task ID.
func (t *LogTimer) WithTaskID(taskID string) *LogTimer {
	t.entry.TaskID = taskID
	return t
}

// WithRequest sets the request data.
func (t *LogTimer) WithRequest(req any) *LogTimer {
	t.entry.Request = req
	return t
}

// WithRetryCount sets the retry count.
func (t *LogTimer) WithRetryCount(count int) *LogTimer {
	t.entry.RetryCount = count
	return t
}

// WithExtra adds extra metadata.
func (t *LogTimer) WithExtra(key string, value any) *LogTimer {
	if t.entry.Extra == nil {
		t.entry.Extra = make(map[string]any)
	}
	t.entry.Extra[key] = value
	return t
}

// Success logs a successful API call.
func (t *LogTimer) Success(ctx context.Context, response any) {
	t.entry.Duration = time.Since(t.startTime)
	t.entry.Success = true
	t.entry.Response = response
	t.logger.LogAsync(ctx, t.entry)
}

// Error logs a failed API call.
func (t *LogTimer) Error(ctx context.Context, err error, response any) {
	t.entry.Duration = time.Since(t.startTime)
	t.entry.Success = false
	t.entry.Response = response

	// Extract error details
	var pe *censor.ProviderError
	if censor.IsProviderError(err) {
		if err, ok := err.(*censor.ProviderError); ok {
			pe = err
		}
	}

	if pe != nil {
		t.entry.ErrorCode = pe.Code
		t.entry.ErrorMessage = pe.Message
		t.entry.StatusCode = pe.StatusCode
	} else if err != nil {
		t.entry.ErrorCode = string(censor.GetErrorCategory(err))
		t.entry.ErrorMessage = err.Error()
	}

	t.logger.LogAsync(ctx, t.entry)
}

// NopLogger is a no-op logger that discards all logs.
type NopLogger struct{}

func (NopLogger) Log(ctx context.Context, entry APILogEntry)      {}
func (NopLogger) LogAsync(ctx context.Context, entry APILogEntry) {}

// GlobalLogger is the default global logger instance.
var GlobalLogger APILogger = NopLogger{}

// SetGlobalLogger sets the global logger instance.
func SetGlobalLogger(logger APILogger) {
	GlobalLogger = logger
}
