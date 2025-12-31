package providers

import (
	"context"
	"errors"
	"testing"
	"time"

	censor "github.com/heibot/censor"
	"github.com/heibot/censor/violation"
)

// ============================================================
// Mock Provider Implementation
// ============================================================

type mockProvider struct {
	name         string
	capabilities []Capability
	sceneCap     SceneCapability
	submitResp   SubmitResponse
	submitErr    error
	queryResp    QueryResponse
	queryErr     error
	callbackErr  error
	callbackData CallbackData
}

func newMockProvider(name string) *mockProvider {
	return &mockProvider{
		name: name,
		capabilities: []Capability{
			{ResourceType: censor.ResourceText, Modes: []Mode{ModeSync, ModeAsync}},
			{ResourceType: censor.ResourceImage, Modes: []Mode{ModeAsync}},
		},
		sceneCap: SceneCapability{
			Provider: name,
			SupportedScenes: map[censor.ResourceType][]violation.UnifiedScene{
				censor.ResourceText:  {violation.ScenePornography, violation.ScenePolitics, violation.SceneAbuse},
				censor.ResourceImage: {violation.ScenePornography, violation.SceneTerrorism},
			},
			MaxTextLength:  10000,
			SyncSupported:  true,
			AsyncSupported: true,
		},
		submitResp: SubmitResponse{
			Mode:   ModeSync,
			TaskID: "task_123",
			Immediate: &censor.ReviewResult{
				Decision:   censor.DecisionPass,
				Confidence: 1.0,
				Provider:   name,
				ReviewedAt: time.Now(),
			},
		},
		queryResp: QueryResponse{
			Done: true,
			Result: &censor.ReviewResult{
				Decision:   censor.DecisionPass,
				Confidence: 1.0,
				Provider:   name,
			},
		},
		callbackData: CallbackData{
			TaskID: "callback_task",
			Done:   true,
			Result: &censor.ReviewResult{
				Decision: censor.DecisionPass,
				Provider: name,
			},
		},
	}
}

func (p *mockProvider) Name() string {
	return p.name
}

func (p *mockProvider) Capabilities() []Capability {
	return p.capabilities
}

func (p *mockProvider) SceneCapability() SceneCapability {
	return p.sceneCap
}

func (p *mockProvider) TranslateScenes(scenes []violation.UnifiedScene, resourceType censor.ResourceType) []string {
	var result []string
	for _, s := range scenes {
		result = append(result, string(s))
	}
	return result
}

func (p *mockProvider) Submit(ctx context.Context, req SubmitRequest) (SubmitResponse, error) {
	if p.submitErr != nil {
		return SubmitResponse{}, p.submitErr
	}
	return p.submitResp, nil
}

func (p *mockProvider) Query(ctx context.Context, taskID string) (QueryResponse, error) {
	if p.queryErr != nil {
		return QueryResponse{}, p.queryErr
	}
	return p.queryResp, nil
}

func (p *mockProvider) VerifyCallback(ctx context.Context, headers map[string]string, body []byte) error {
	return p.callbackErr
}

func (p *mockProvider) ParseCallback(ctx context.Context, body []byte) (CallbackData, error) {
	return p.callbackData, nil
}

func (p *mockProvider) Translator() violation.Translator {
	return nil
}

// ============================================================
// SceneCapability Tests
// ============================================================

func TestSceneCapability_CanHandle(t *testing.T) {
	sc := SceneCapability{
		Provider: "test",
		SupportedScenes: map[censor.ResourceType][]violation.UnifiedScene{
			censor.ResourceText: {violation.ScenePornography, violation.ScenePolitics},
		},
	}

	tests := []struct {
		name         string
		scenes       []violation.UnifiedScene
		resourceType censor.ResourceType
		want         bool
	}{
		{
			name:         "all scenes supported",
			scenes:       []violation.UnifiedScene{violation.ScenePornography, violation.ScenePolitics},
			resourceType: censor.ResourceText,
			want:         true,
		},
		{
			name:         "single scene supported",
			scenes:       []violation.UnifiedScene{violation.ScenePornography},
			resourceType: censor.ResourceText,
			want:         true,
		},
		{
			name:         "scene not supported",
			scenes:       []violation.UnifiedScene{violation.SceneTerrorism},
			resourceType: censor.ResourceText,
			want:         false,
		},
		{
			name:         "partial scenes supported",
			scenes:       []violation.UnifiedScene{violation.ScenePornography, violation.SceneTerrorism},
			resourceType: censor.ResourceText,
			want:         false,
		},
		{
			name:         "empty scenes",
			scenes:       []violation.UnifiedScene{},
			resourceType: censor.ResourceText,
			want:         true,
		},
		{
			name:         "unsupported resource type",
			scenes:       []violation.UnifiedScene{violation.ScenePornography},
			resourceType: censor.ResourceImage,
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sc.CanHandle(tt.scenes, tt.resourceType)
			if got != tt.want {
				t.Errorf("CanHandle() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSceneCapability_MissingScenes(t *testing.T) {
	sc := SceneCapability{
		Provider: "test",
		SupportedScenes: map[censor.ResourceType][]violation.UnifiedScene{
			censor.ResourceText: {violation.ScenePornography, violation.ScenePolitics},
		},
	}

	tests := []struct {
		name         string
		scenes       []violation.UnifiedScene
		resourceType censor.ResourceType
		wantCount    int
	}{
		{
			name:         "no missing scenes",
			scenes:       []violation.UnifiedScene{violation.ScenePornography},
			resourceType: censor.ResourceText,
			wantCount:    0,
		},
		{
			name:         "one missing scene",
			scenes:       []violation.UnifiedScene{violation.ScenePornography, violation.SceneTerrorism},
			resourceType: censor.ResourceText,
			wantCount:    1,
		},
		{
			name:         "all missing scenes",
			scenes:       []violation.UnifiedScene{violation.SceneTerrorism, violation.SceneAbuse},
			resourceType: censor.ResourceText,
			wantCount:    2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sc.MissingScenes(tt.scenes, tt.resourceType)
			if len(got) != tt.wantCount {
				t.Errorf("MissingScenes() count = %d, want %d", len(got), tt.wantCount)
			}
		})
	}
}

// ============================================================
// Provider Helper Function Tests
// ============================================================

func TestSupportsSync(t *testing.T) {
	p := newMockProvider("test")

	tests := []struct {
		name         string
		resourceType censor.ResourceType
		want         bool
	}{
		{
			name:         "text supports sync",
			resourceType: censor.ResourceText,
			want:         true,
		},
		{
			name:         "image does not support sync",
			resourceType: censor.ResourceImage,
			want:         false,
		},
		{
			name:         "unsupported type",
			resourceType: censor.ResourceVideo,
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SupportsSync(p, tt.resourceType)
			if got != tt.want {
				t.Errorf("SupportsSync() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSupportsAsync(t *testing.T) {
	p := newMockProvider("test")

	tests := []struct {
		name         string
		resourceType censor.ResourceType
		want         bool
	}{
		{
			name:         "text supports async",
			resourceType: censor.ResourceText,
			want:         true,
		},
		{
			name:         "image supports async",
			resourceType: censor.ResourceImage,
			want:         true,
		},
		{
			name:         "unsupported type",
			resourceType: censor.ResourceVideo,
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SupportsAsync(p, tt.resourceType)
			if got != tt.want {
				t.Errorf("SupportsAsync() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSupportsResourceType(t *testing.T) {
	p := newMockProvider("test")

	tests := []struct {
		name         string
		resourceType censor.ResourceType
		want         bool
	}{
		{
			name:         "text supported",
			resourceType: censor.ResourceText,
			want:         true,
		},
		{
			name:         "image supported",
			resourceType: censor.ResourceImage,
			want:         true,
		},
		{
			name:         "video not supported",
			resourceType: censor.ResourceVideo,
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SupportsResourceType(p, tt.resourceType)
			if got != tt.want {
				t.Errorf("SupportsResourceType() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ============================================================
// ResilientProvider Tests
// ============================================================

func TestResilientProvider_Submit(t *testing.T) {
	t.Run("success without retry", func(t *testing.T) {
		mp := newMockProvider("test")
		rp := WrapWithResilience(mp)

		resp, err := rp.Submit(context.Background(), SubmitRequest{
			Resource: censor.Resource{
				ResourceID:  "res_1",
				Type:        censor.ResourceText,
				ContentText: "Hello world",
			},
		})

		if err != nil {
			t.Fatalf("Submit() error = %v", err)
		}
		if resp.TaskID == "" {
			t.Error("Submit() TaskID is empty")
		}
	})

	t.Run("retry on retryable error", func(t *testing.T) {
		mp := newMockProvider("test")
		callCount := 0
		mp.submitErr = nil

		// Create custom provider that fails first, then succeeds
		originalSubmit := mp.submitResp
		retryErr := censor.NewProviderError("test", "rate_limit", "too many requests").
			WithStatusCode(429)

		// Use retryTestProvider to track calls and succeed on retry
		testProvider := &retryTestProvider{
			Provider:     mp,
			callCount:    &callCount,
			failUntil:    2,
			successResp:  originalSubmit,
			failErr:      retryErr,
		}

		rp2 := NewResilientProvider(testProvider, ResilientConfig{
			MaxRetries:    3,
			InitialDelay:  10 * time.Millisecond,
			MaxDelay:      100 * time.Millisecond,
			EnableRetry:   true,
			EnableLogging: false,
		})

		resp, err := rp2.Submit(context.Background(), SubmitRequest{
			Resource: censor.Resource{
				ResourceID:  "res_1",
				Type:        censor.ResourceText,
				ContentText: "Hello world",
			},
		})

		if err != nil {
			t.Fatalf("Submit() error = %v", err)
		}
		if resp.TaskID == "" {
			t.Error("Submit() TaskID is empty")
		}
		if callCount < 2 {
			t.Errorf("Expected at least 2 calls, got %d", callCount)
		}
	})

	t.Run("non-retryable error fails immediately", func(t *testing.T) {
		mp := newMockProvider("test")
		mp.submitErr = censor.NewProviderError("test", "invalid_param", "bad request").
			WithStatusCode(400)

		rp := NewResilientProvider(mp, ResilientConfig{
			MaxRetries:    3,
			InitialDelay:  10 * time.Millisecond,
			MaxDelay:      100 * time.Millisecond,
			EnableRetry:   true,
			EnableLogging: false,
		})

		_, err := rp.Submit(context.Background(), SubmitRequest{
			Resource: censor.Resource{
				ResourceID:  "res_1",
				Type:        censor.ResourceText,
				ContentText: "Hello world",
			},
		})

		if err == nil {
			t.Error("Submit() should return error")
		}
	})
}

func TestResilientProvider_Query(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mp := newMockProvider("test")
		rp := WrapWithResilience(mp)

		resp, err := rp.Query(context.Background(), "task_123")

		if err != nil {
			t.Fatalf("Query() error = %v", err)
		}
		if !resp.Done {
			t.Error("Query() Done should be true")
		}
	})

	t.Run("with error", func(t *testing.T) {
		mp := newMockProvider("test")
		mp.queryErr = errors.New("query failed")
		rp := WrapWithLogging(mp, NopLogger{})

		_, err := rp.Query(context.Background(), "task_123")

		if err == nil {
			t.Error("Query() should return error")
		}
	})
}

func TestResilientProvider_Callback(t *testing.T) {
	t.Run("verify and parse success", func(t *testing.T) {
		mp := newMockProvider("test")
		rp := WrapWithResilience(mp)

		err := rp.VerifyCallback(context.Background(), nil, nil)
		if err != nil {
			t.Fatalf("VerifyCallback() error = %v", err)
		}

		data, err := rp.ParseCallback(context.Background(), nil)
		if err != nil {
			t.Fatalf("ParseCallback() error = %v", err)
		}
		if data.TaskID == "" {
			t.Error("ParseCallback() TaskID is empty")
		}
	})
}

func TestResilientProvider_Unwrap(t *testing.T) {
	mp := newMockProvider("test")
	rp := WrapWithResilience(mp)

	unwrapped := rp.Unwrap()
	if unwrapped != mp {
		t.Error("Unwrap() should return original provider")
	}
}

// Helper provider for retry testing
type retryTestProvider struct {
	Provider
	callCount   *int
	failUntil   int
	successResp SubmitResponse
	failErr     error
}

func (p *retryTestProvider) Submit(ctx context.Context, req SubmitRequest) (SubmitResponse, error) {
	*p.callCount++
	if *p.callCount < p.failUntil {
		return SubmitResponse{}, p.failErr
	}
	return p.successResp, nil
}

// ============================================================
// Logger Tests
// ============================================================

func TestStandardLogger(t *testing.T) {
	t.Run("log entry sync", func(t *testing.T) {
		config := DefaultLoggerConfig()
		config.StdoutEnabled = false // Disable stdout for tests
		logger := NewStandardLogger(config)
		defer logger.Close()

		entry := APILogEntry{
			Provider:  "test",
			Operation: "submit",
			Success:   true,
			Duration:  100 * time.Millisecond,
		}

		// Should not panic
		logger.Log(context.Background(), entry)
	})

	t.Run("log entry async", func(t *testing.T) {
		config := DefaultLoggerConfig()
		config.StdoutEnabled = false
		logger := NewStandardLogger(config)

		entry := APILogEntry{
			Provider:  "test",
			Operation: "submit",
			Success:   true,
			Duration:  100 * time.Millisecond,
		}

		// Should not panic
		logger.LogAsync(context.Background(), entry)

		// Close waits for async logs to be processed
		logger.Close()
	})

	t.Run("nop logger", func(t *testing.T) {
		logger := NopLogger{}

		entry := APILogEntry{
			Provider:  "test",
			Operation: "submit",
		}

		// Should not panic
		logger.Log(context.Background(), entry)
		logger.LogAsync(context.Background(), entry)
	})
}

func TestLogTimer(t *testing.T) {
	t.Run("success logging", func(t *testing.T) {
		timer := StartLog(NopLogger{}, "test", "submit").
			WithResource(censor.ResourceText, "res_1").
			WithTaskID("task_123").
			WithRequest(map[string]string{"key": "value"}).
			WithRetryCount(2).
			WithExtra("custom", "data")

		// Should not panic
		timer.Success(context.Background(), map[string]string{"result": "ok"})
	})

	t.Run("error logging", func(t *testing.T) {
		timer := StartLog(NopLogger{}, "test", "submit").
			WithResource(censor.ResourceText, "res_1")

		err := censor.NewProviderError("test", "err_code", "error message")

		// Should not panic
		timer.Error(context.Background(), err, nil)
	})

	t.Run("error logging with non-provider error", func(t *testing.T) {
		timer := StartLog(NopLogger{}, "test", "submit")

		err := errors.New("generic error")

		// Should not panic
		timer.Error(context.Background(), err, nil)
	})
}

func TestGlobalLogger(t *testing.T) {
	t.Run("default is nop", func(t *testing.T) {
		_, ok := GlobalLogger.(NopLogger)
		if !ok {
			t.Error("Default GlobalLogger should be NopLogger")
		}
	})

	t.Run("set global logger", func(t *testing.T) {
		original := GlobalLogger
		defer func() { GlobalLogger = original }()

		config := DefaultLoggerConfig()
		config.StdoutEnabled = false
		newLogger := NewStandardLogger(config)
		defer newLogger.Close()

		SetGlobalLogger(newLogger)

		if GlobalLogger != newLogger {
			t.Error("SetGlobalLogger did not set the logger")
		}
	})
}

// ============================================================
// Wrapper Function Tests
// ============================================================

func TestWrapperFunctions(t *testing.T) {
	mp := newMockProvider("test")

	t.Run("WrapWithResilience", func(t *testing.T) {
		rp := WrapWithResilience(mp)
		if rp == nil {
			t.Error("WrapWithResilience returned nil")
		}
		if rp.Name() != mp.Name() {
			t.Error("Name should match underlying provider")
		}
	})

	t.Run("WrapWithRetry", func(t *testing.T) {
		rp := WrapWithRetry(mp, 5)
		if rp == nil {
			t.Error("WrapWithRetry returned nil")
		}
		if rp.config.MaxRetries != 5 {
			t.Errorf("MaxRetries = %d, want 5", rp.config.MaxRetries)
		}
		if rp.config.EnableLogging {
			t.Error("EnableLogging should be false")
		}
	})

	t.Run("WrapWithLogging", func(t *testing.T) {
		rp := WrapWithLogging(mp, NopLogger{})
		if rp == nil {
			t.Error("WrapWithLogging returned nil")
		}
		if rp.config.EnableRetry {
			t.Error("EnableRetry should be false")
		}
	})
}
