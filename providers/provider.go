// Package providers defines the provider interface and common types
// for content moderation cloud providers.
package providers

import (
	"context"
	"time"

	censor "github.com/heibot/censor"
	"github.com/heibot/censor/violation"
)

// Mode represents the review mode.
type Mode string

const (
	ModeSync  Mode = "sync"  // Synchronous review
	ModeAsync Mode = "async" // Asynchronous review
)

// Capability represents what a provider can do.
type Capability struct {
	ResourceType censor.ResourceType
	Modes        []Mode
}

// SceneCapability declares what detection scenes a provider supports.
type SceneCapability struct {
	Provider        string
	SupportedScenes map[censor.ResourceType][]violation.UnifiedScene
	MaxTextLength   int  // Maximum text length per request
	SyncSupported   bool // Supports synchronous review
	AsyncSupported  bool // Supports asynchronous review
}

// CanHandle checks if the provider can handle all required scenes.
func (sc SceneCapability) CanHandle(scenes []violation.UnifiedScene, resourceType censor.ResourceType) bool {
	supported := sc.SupportedScenes[resourceType]
	supportedSet := make(map[violation.UnifiedScene]bool)
	for _, s := range supported {
		supportedSet[s] = true
	}

	for _, scene := range scenes {
		if !supportedSet[scene] {
			return false
		}
	}
	return true
}

// MissingScenes returns scenes that the provider cannot handle.
func (sc SceneCapability) MissingScenes(scenes []violation.UnifiedScene, resourceType censor.ResourceType) []violation.UnifiedScene {
	supported := sc.SupportedScenes[resourceType]
	supportedSet := make(map[violation.UnifiedScene]bool)
	for _, s := range supported {
		supportedSet[s] = true
	}

	var missing []violation.UnifiedScene
	for _, scene := range scenes {
		if !supportedSet[scene] {
			missing = append(missing, scene)
		}
	}
	return missing
}

// SubmitRequest represents a request to submit content for review.
type SubmitRequest struct {
	Resource censor.Resource
	Biz      censor.BizContext
	Scenes   []violation.UnifiedScene // Required detection scenes
	Timeout  time.Duration
}

// SubmitResponse represents the response from submitting content.
type SubmitResponse struct {
	Mode      Mode                 // sync or async
	TaskID    string               // Provider-side task ID
	Immediate *censor.ReviewResult // Immediate result for sync mode
	Raw       map[string]any       // Raw provider response
}

// QueryResponse represents the response from querying a task.
type QueryResponse struct {
	Done   bool                 // Whether the task is complete
	Result *censor.ReviewResult // Review result if done
	Raw    map[string]any       // Raw provider response
}

// CallbackData represents data received from a provider callback.
type CallbackData struct {
	TaskID string
	Done   bool
	Result *censor.ReviewResult
	Raw    map[string]any
}

// Provider defines the interface for content moderation providers.
type Provider interface {
	// Name returns the provider name (e.g., "aliyun", "huawei", "tencent").
	Name() string

	// Capabilities returns the supported resource types and modes.
	Capabilities() []Capability

	// SceneCapability returns the detection scene capabilities.
	SceneCapability() SceneCapability

	// TranslateScenes converts unified scenes to provider-specific scene codes.
	TranslateScenes(scenes []violation.UnifiedScene, resourceType censor.ResourceType) []string

	// Submit submits content for review.
	Submit(ctx context.Context, req SubmitRequest) (SubmitResponse, error)

	// Query queries the status of an async task.
	Query(ctx context.Context, taskID string) (QueryResponse, error)

	// VerifyCallback verifies the signature of a callback request.
	VerifyCallback(ctx context.Context, headers map[string]string, body []byte) error

	// ParseCallback parses a callback request body.
	ParseCallback(ctx context.Context, body []byte) (CallbackData, error)

	// Translator returns the violation translator for this provider.
	Translator() violation.Translator
}

// ProviderConfig is the base configuration for providers.
type ProviderConfig struct {
	AccessKeyID     string
	AccessKeySecret string
	Region          string
	Endpoint        string
	Timeout         time.Duration
}

// SupportsSync checks if a provider supports sync mode for a resource type.
func SupportsSync(p Provider, rt censor.ResourceType) bool {
	for _, cap := range p.Capabilities() {
		if cap.ResourceType == rt {
			for _, mode := range cap.Modes {
				if mode == ModeSync {
					return true
				}
			}
		}
	}
	return false
}

// SupportsAsync checks if a provider supports async mode for a resource type.
func SupportsAsync(p Provider, rt censor.ResourceType) bool {
	for _, cap := range p.Capabilities() {
		if cap.ResourceType == rt {
			for _, mode := range cap.Modes {
				if mode == ModeAsync {
					return true
				}
			}
		}
	}
	return false
}

// SupportsResourceType checks if a provider supports a resource type.
func SupportsResourceType(p Provider, rt censor.ResourceType) bool {
	for _, cap := range p.Capabilities() {
		if cap.ResourceType == rt {
			return true
		}
	}
	return false
}
