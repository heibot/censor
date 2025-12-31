package providers

import (
	"context"
	"time"

	censor "github.com/heibot/censor"
	"github.com/heibot/censor/utils"
	"github.com/heibot/censor/violation"
)

// ResilientConfig configures the resilient provider wrapper.
type ResilientConfig struct {
	// Retry configuration
	MaxRetries   int
	InitialDelay time.Duration
	MaxDelay     time.Duration

	// Logger for API calls
	Logger APILogger

	// EnableRetry controls whether retry is enabled.
	EnableRetry bool

	// EnableLogging controls whether logging is enabled.
	EnableLogging bool
}

// DefaultResilientConfig returns sensible defaults.
func DefaultResilientConfig() ResilientConfig {
	return ResilientConfig{
		MaxRetries:    3,
		InitialDelay:  1 * time.Second,
		MaxDelay:      30 * time.Second,
		EnableRetry:   true,
		EnableLogging: true,
	}
}

// ResilientProvider wraps a provider with retry and logging capabilities.
type ResilientProvider struct {
	provider Provider
	config   ResilientConfig
	retryer  *utils.Retryer
	logger   APILogger
}

// NewResilientProvider creates a new resilient provider wrapper.
func NewResilientProvider(provider Provider, config ResilientConfig) *ResilientProvider {
	rp := &ResilientProvider{
		provider: provider,
		config:   config,
	}

	// Setup retryer
	if config.EnableRetry {
		rp.retryer = utils.NewRetryer(utils.RetryConfig{
			MaxRetries:   config.MaxRetries,
			InitialDelay: config.InitialDelay,
			MaxDelay:     config.MaxDelay,
			Multiplier:   2.0,
			Jitter:       0.1,
		})
	}

	// Setup logger
	if config.EnableLogging {
		if config.Logger != nil {
			rp.logger = config.Logger
		} else {
			rp.logger = GlobalLogger
		}
	} else {
		rp.logger = NopLogger{}
	}

	return rp
}

// Name returns the provider name.
func (rp *ResilientProvider) Name() string {
	return rp.provider.Name()
}

// Capabilities returns the supported capabilities.
func (rp *ResilientProvider) Capabilities() []Capability {
	return rp.provider.Capabilities()
}

// SceneCapability returns the detection scene capabilities.
func (rp *ResilientProvider) SceneCapability() SceneCapability {
	return rp.provider.SceneCapability()
}

// TranslateScenes converts unified scenes to provider-specific scene codes.
func (rp *ResilientProvider) TranslateScenes(scenes []violation.UnifiedScene, resourceType censor.ResourceType) []string {
	return rp.provider.TranslateScenes(scenes, resourceType)
}

// Submit submits content for review with retry and logging.
func (rp *ResilientProvider) Submit(ctx context.Context, req SubmitRequest) (SubmitResponse, error) {
	timer := StartLog(rp.logger, rp.provider.Name(), "submit").
		WithResource(req.Resource.Type, req.Resource.ResourceID).
		WithRequest(sanitizeRequest(req))

	var resp SubmitResponse
	var retryCount int

	executeSubmit := func() error {
		var err error
		resp, err = rp.provider.Submit(ctx, req)
		if err != nil {
			retryCount++
			return err
		}
		return nil
	}

	if rp.retryer != nil {
		err := rp.retryer.Do(ctx, executeSubmit)
		if err != nil {
			timer.WithRetryCount(retryCount).Error(ctx, err, nil)
			return SubmitResponse{}, err
		}
	} else {
		if err := executeSubmit(); err != nil {
			timer.Error(ctx, err, nil)
			return SubmitResponse{}, err
		}
	}

	timer.WithTaskID(resp.TaskID).WithRetryCount(retryCount).Success(ctx, sanitizeResponse(resp))
	return resp, nil
}

// Query queries the status of an async task with retry and logging.
func (rp *ResilientProvider) Query(ctx context.Context, taskID string) (QueryResponse, error) {
	timer := StartLog(rp.logger, rp.provider.Name(), "query").
		WithTaskID(taskID)

	var resp QueryResponse
	var retryCount int

	executeQuery := func() error {
		var err error
		resp, err = rp.provider.Query(ctx, taskID)
		if err != nil {
			retryCount++
			return err
		}
		return nil
	}

	if rp.retryer != nil {
		err := rp.retryer.Do(ctx, executeQuery)
		if err != nil {
			timer.WithRetryCount(retryCount).Error(ctx, err, nil)
			return QueryResponse{}, err
		}
	} else {
		if err := executeQuery(); err != nil {
			timer.Error(ctx, err, nil)
			return QueryResponse{}, err
		}
	}

	timer.WithRetryCount(retryCount).
		WithExtra("done", resp.Done).
		Success(ctx, nil)
	return resp, nil
}

// VerifyCallback verifies the signature of a callback request.
func (rp *ResilientProvider) VerifyCallback(ctx context.Context, headers map[string]string, body []byte) error {
	return rp.provider.VerifyCallback(ctx, headers, body)
}

// ParseCallback parses a callback request body with logging.
func (rp *ResilientProvider) ParseCallback(ctx context.Context, body []byte) (CallbackData, error) {
	timer := StartLog(rp.logger, rp.provider.Name(), "callback")

	data, err := rp.provider.ParseCallback(ctx, body)
	if err != nil {
		timer.Error(ctx, err, nil)
		return CallbackData{}, err
	}

	timer.WithTaskID(data.TaskID).
		WithExtra("done", data.Done).
		Success(ctx, nil)
	return data, nil
}

// Translator returns the violation translator.
func (rp *ResilientProvider) Translator() violation.Translator {
	return rp.provider.Translator()
}

// Unwrap returns the underlying provider.
func (rp *ResilientProvider) Unwrap() Provider {
	return rp.provider
}

// sanitizeRequest removes sensitive data from request for logging.
func sanitizeRequest(req SubmitRequest) map[string]any {
	return map[string]any{
		"resource_id":   req.Resource.ResourceID,
		"resource_type": req.Resource.Type,
		"biz_type":      req.Biz.BizType,
		"biz_id":        req.Biz.BizID,
		"has_text":      req.Resource.ContentText != "",
		"has_url":       req.Resource.ContentURL != "",
	}
}

// sanitizeResponse removes sensitive data from response for logging.
func sanitizeResponse(resp SubmitResponse) map[string]any {
	result := map[string]any{
		"mode":    resp.Mode,
		"task_id": resp.TaskID,
	}
	if resp.Immediate != nil {
		result["decision"] = resp.Immediate.Decision
		result["confidence"] = resp.Immediate.Confidence
	}
	return result
}

// WrapWithResilience wraps a provider with default resilience configuration.
func WrapWithResilience(provider Provider) *ResilientProvider {
	return NewResilientProvider(provider, DefaultResilientConfig())
}

// WrapWithRetry wraps a provider with retry only.
func WrapWithRetry(provider Provider, maxRetries int) *ResilientProvider {
	return NewResilientProvider(provider, ResilientConfig{
		MaxRetries:    maxRetries,
		InitialDelay:  1 * time.Second,
		MaxDelay:      30 * time.Second,
		EnableRetry:   true,
		EnableLogging: false,
	})
}

// WrapWithLogging wraps a provider with logging only.
func WrapWithLogging(provider Provider, logger APILogger) *ResilientProvider {
	return NewResilientProvider(provider, ResilientConfig{
		Logger:        logger,
		EnableRetry:   false,
		EnableLogging: true,
	})
}
