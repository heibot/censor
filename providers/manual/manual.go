// Package manual provides a manual review provider for human moderation.
package manual

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	censor "github.com/heibot/censor"
	"github.com/heibot/censor/providers"
	"github.com/heibot/censor/violation"
)

const providerName = "manual"

// Config holds the configuration for manual review provider.
type Config struct {
	// QueueName is the name of the manual review queue.
	QueueName string

	// WebhookURL is the URL to notify when a task is created.
	WebhookURL string

	// WebhookSecret is the secret for webhook signatures.
	WebhookSecret string

	// DefaultTimeout is how long to wait for manual review.
	DefaultTimeout time.Duration

	// Store is an optional external storage for tasks.
	// If nil, uses in-memory storage (not recommended for production).
	Store TaskStore
}

// TaskStore is the interface for storing manual review tasks.
type TaskStore interface {
	SaveTask(ctx context.Context, task ManualTask) error
	GetTask(ctx context.Context, taskID string) (*ManualTask, error)
	UpdateTask(ctx context.Context, taskID string, result ManualResult) error
	ListPendingTasks(ctx context.Context, queueName string, limit int) ([]ManualTask, error)
}

// DefaultConfig returns the default manual review configuration.
func DefaultConfig() Config {
	return Config{
		QueueName:      "default",
		DefaultTimeout: 24 * time.Hour,
	}
}

// TaskHandler is called when a manual review task is created.
type TaskHandler func(ctx context.Context, task ManualTask) error

// ManualTask represents a task waiting for manual review.
type ManualTask struct {
	TaskID     string               `json:"task_id"`
	QueueName  string               `json:"queue_name"`
	Resource   censor.Resource      `json:"resource"`
	Biz        censor.BizContext    `json:"biz"`
	AutoResult *censor.ReviewResult `json:"auto_result,omitempty"` // Result from auto review
	Priority   int                  `json:"priority"`              // Higher = more urgent
	CreatedAt  time.Time            `json:"created_at"`
	ExpiresAt  time.Time            `json:"expires_at"`
	Result     *ManualResult        `json:"result,omitempty"` // Review result if done
	Done       bool                 `json:"done"`
}

// Provider implements the manual review provider.
type Provider struct {
	config     Config
	handler    TaskHandler
	store      TaskStore
	translator violation.Translator
}

// New creates a new manual review provider.
func New(cfg Config) *Provider {
	p := &Provider{
		config:     cfg,
		translator: newTranslator(),
	}

	// Use external store if provided, otherwise use in-memory store
	if cfg.Store != nil {
		p.store = cfg.Store
	} else {
		p.store = newMemoryStore()
	}

	return p
}

// WithHandler sets the task handler.
func (p *Provider) WithHandler(h TaskHandler) *Provider {
	p.handler = h
	return p
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return providerName
}

// Capabilities returns the supported capabilities.
// Manual review supports all types but only async mode.
func (p *Provider) Capabilities() []providers.Capability {
	return []providers.Capability{
		{
			ResourceType: censor.ResourceText,
			Modes:        []providers.Mode{providers.ModeAsync},
		},
		{
			ResourceType: censor.ResourceImage,
			Modes:        []providers.Mode{providers.ModeAsync},
		},
		{
			ResourceType: censor.ResourceVideo,
			Modes:        []providers.Mode{providers.ModeAsync},
		},
	}
}

// SceneCapability returns the detection scene capabilities.
// Manual review can handle all scenes.
func (p *Provider) SceneCapability() providers.SceneCapability {
	allScenes := []violation.UnifiedScene{
		violation.ScenePornography, violation.SceneTerrorism, violation.ScenePolitics,
		violation.SceneViolence, violation.SceneBan, violation.SceneAbuse,
		violation.SceneHateSpeech, violation.SceneHarassment, violation.SceneAds,
		violation.SceneSpam, violation.SceneFraud, violation.ScenePrivacy,
		violation.SceneMeaningless, violation.SceneFlood, violation.SceneMinor,
		violation.SceneMoan, violation.SceneQRCode, violation.SceneImageText,
		violation.SceneCustom, violation.SceneAdLaw, violation.ScenePublicFigure,
	}

	return providers.SceneCapability{
		Provider: providerName,
		SupportedScenes: map[censor.ResourceType][]violation.UnifiedScene{
			censor.ResourceText:  allScenes,
			censor.ResourceImage: allScenes,
			censor.ResourceVideo: allScenes,
		},
		MaxTextLength:  0, // No limit
		SyncSupported:  false,
		AsyncSupported: true,
	}
}

// TranslateScenes returns empty - manual review doesn't need scene translation.
func (p *Provider) TranslateScenes(scenes []violation.UnifiedScene, resourceType censor.ResourceType) []string {
	// Manual review doesn't use provider-specific scene codes
	return nil
}

// Submit creates a manual review task.
func (p *Provider) Submit(ctx context.Context, req providers.SubmitRequest) (providers.SubmitResponse, error) {
	taskID := fmt.Sprintf("manual_%s_%d", req.Resource.ResourceID, time.Now().UnixNano())

	task := ManualTask{
		TaskID:    taskID,
		QueueName: p.config.QueueName,
		Resource:  req.Resource,
		Biz:       req.Biz,
		Priority:  calculatePriority(req.Biz),
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(p.config.DefaultTimeout),
		Done:      false,
	}

	// Save task to store
	if err := p.store.SaveTask(ctx, task); err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("failed to save manual task: %w", err)
	}

	// Call handler if set (e.g., push to review queue, send notification)
	if p.handler != nil {
		if err := p.handler(ctx, task); err != nil {
			return providers.SubmitResponse{}, fmt.Errorf("failed to create manual task: %w", err)
		}
	}

	return providers.SubmitResponse{
		Mode:   providers.ModeAsync,
		TaskID: taskID,
		Raw: map[string]any{
			"task_id":    taskID,
			"queue":      p.config.QueueName,
			"status":     "pending",
			"expires_at": task.ExpiresAt.Unix(),
		},
	}, nil
}

// Query queries the status of a manual review task.
func (p *Provider) Query(ctx context.Context, taskID string) (providers.QueryResponse, error) {
	task, err := p.store.GetTask(ctx, taskID)
	if err != nil {
		return providers.QueryResponse{}, fmt.Errorf("failed to get task: %w", err)
	}

	if task == nil {
		return providers.QueryResponse{
			Done: false,
			Raw: map[string]any{
				"task_id": taskID,
				"status":  "not_found",
			},
		}, nil
	}

	if !task.Done {
		// Check if task has expired
		if time.Now().After(task.ExpiresAt) {
			return providers.QueryResponse{
				Done: true,
				Result: &censor.ReviewResult{
					Decision:   censor.DecisionReview, // Still needs review - escalate
					Confidence: 0,
					Provider:   providerName,
					ReviewedAt: time.Now(),
					Reasons: []censor.Reason{{
						Code:    "timeout",
						Message: "Manual review timed out",
					}},
				},
				Raw: map[string]any{
					"task_id": taskID,
					"status":  "timeout",
				},
			}, nil
		}

		return providers.QueryResponse{
			Done: false,
			Raw: map[string]any{
				"task_id": taskID,
				"status":  "pending",
			},
		}, nil
	}

	// Task is done, return the result
	result := &censor.ReviewResult{
		Decision:   task.Result.Decision,
		Confidence: 1.0, // Human review is authoritative
		Provider:   providerName,
		ReviewedAt: task.Result.ReviewedAt,
		Reasons:    task.Result.Reasons,
	}

	return providers.QueryResponse{
		Done:   true,
		Result: result,
		Raw: map[string]any{
			"task_id":     taskID,
			"status":      "completed",
			"reviewer_id": task.Result.ReviewerID,
			"comment":     task.Result.Comment,
		},
	}, nil
}

// SubmitResult submits a manual review result.
// This is called when a human reviewer completes the review.
func (p *Provider) SubmitResult(ctx context.Context, taskID string, result ManualResult) error {
	result.ReviewedAt = time.Now()
	return p.store.UpdateTask(ctx, taskID, result)
}

// GetPendingTasks returns pending tasks for a queue.
func (p *Provider) GetPendingTasks(ctx context.Context, limit int) ([]ManualTask, error) {
	return p.store.ListPendingTasks(ctx, p.config.QueueName, limit)
}

// ManualResult represents the result of a manual review.
type ManualResult struct {
	TaskID     string          `json:"task_id"`
	Decision   censor.Decision `json:"decision"`
	Reasons    []censor.Reason `json:"reasons"`
	ReviewerID string          `json:"reviewer_id"`
	Comment    string          `json:"comment"`
	ReviewedAt time.Time       `json:"reviewed_at"`
}

// VerifyCallback verifies callback signatures (not typically used for manual).
func (p *Provider) VerifyCallback(ctx context.Context, headers map[string]string, body []byte) error {
	return nil
}

// ParseCallback parses callback data (from internal review system).
func (p *Provider) ParseCallback(ctx context.Context, body []byte) (providers.CallbackData, error) {
	var callback struct {
		TaskID     string          `json:"task_id"`
		Decision   string          `json:"decision"`
		ReviewerID string          `json:"reviewer_id"`
		Comment    string          `json:"comment"`
		Reasons    []censor.Reason `json:"reasons"`
	}

	if err := json.Unmarshal(body, &callback); err != nil {
		return providers.CallbackData{}, fmt.Errorf("failed to parse callback: %w", err)
	}

	decision := censor.Decision(callback.Decision)

	return providers.CallbackData{
		TaskID: callback.TaskID,
		Done:   true,
		Result: &censor.ReviewResult{
			Decision:   decision,
			Confidence: 1.0,
			Reasons:    callback.Reasons,
			Provider:   providerName,
			ReviewedAt: time.Now(),
		},
		Raw: map[string]any{
			"reviewer_id": callback.ReviewerID,
			"comment":     callback.Comment,
		},
	}, nil
}

// Translator returns the violation translator for manual review.
func (p *Provider) Translator() violation.Translator {
	return p.translator
}

// calculatePriority determines task priority based on business context.
func calculatePriority(biz censor.BizContext) int {
	// Higher priority for user-facing content
	switch biz.BizType {
	case censor.BizUserNickname, censor.BizUserAvatar:
		return 10
	case censor.BizNoteTitle, censor.BizNoteBody:
		return 8
	case censor.BizChatMessage, censor.BizDanmaku:
		return 6
	default:
		return 5
	}
}

// ============================================================
// In-memory store implementation (for testing/development)
// ============================================================

type memoryStore struct {
	mu    sync.RWMutex
	tasks map[string]*ManualTask
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		tasks: make(map[string]*ManualTask),
	}
}

func (s *memoryStore) SaveTask(ctx context.Context, task ManualTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[task.TaskID] = &task
	return nil
}

func (s *memoryStore) GetTask(ctx context.Context, taskID string) (*ManualTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	task, ok := s.tasks[taskID]
	if !ok {
		return nil, nil
	}
	return task, nil
}

func (s *memoryStore) UpdateTask(ctx context.Context, taskID string, result ManualResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}
	task.Result = &result
	task.Done = true
	return nil
}

func (s *memoryStore) ListPendingTasks(ctx context.Context, queueName string, limit int) ([]ManualTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []ManualTask
	for _, task := range s.tasks {
		if !task.Done && task.QueueName == queueName {
			result = append(result, *task)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

// ============================================================
// Translator implementation
// ============================================================

// Manual review labels
var labelMappings = map[string]labelMapping{
	"pornography":  {domain: "pornography", tags: []string{}, severity: 4},
	"terrorism":    {domain: "terrorism", tags: []string{}, severity: 4},
	"politics":     {domain: "politics", tags: []string{}, severity: 3},
	"violence":     {domain: "violence", tags: []string{}, severity: 3},
	"abuse":        {domain: "abuse", tags: []string{}, severity: 2},
	"ads":          {domain: "ads", tags: []string{}, severity: 1},
	"spam":         {domain: "spam", tags: []string{}, severity: 1},
	"fraud":        {domain: "fraud", tags: []string{}, severity: 3},
	"illegal":      {domain: "illegal", tags: []string{}, severity: 4},
	"minor_safety": {domain: "minor_safety", tags: []string{}, severity: 4},
	"other":        {domain: "other", tags: []string{}, severity: 2},
	"normal":       {domain: "", tags: []string{}, severity: 0},
	"pass":         {domain: "", tags: []string{}, severity: 0},
}

type labelMapping struct {
	domain   string
	tags     []string
	severity int
}

func newTranslator() violation.Translator {
	labelMap := make(map[string]violation.LabelMapping)

	for label, mapping := range labelMappings {
		tags := make([]violation.Tag, len(mapping.tags))
		for i, t := range mapping.tags {
			tags[i] = violation.Tag(t)
		}

		labelMap[label] = violation.LabelMapping{
			Domain:     violation.Domain(mapping.domain),
			Tags:       tags,
			Severity:   censor.RiskLevel(mapping.severity),
			Confidence: 1.0, // Manual review is authoritative
		}
	}

	return violation.NewBaseTranslator(providerName, labelMap)
}
