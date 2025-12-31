package client

import (
	"context"
	"errors"
	"testing"
	"time"

	censor "github.com/heibot/censor"
	"github.com/heibot/censor/hooks"
	"github.com/heibot/censor/providers"
	"github.com/heibot/censor/store"
	"github.com/heibot/censor/violation"
)

// ============================================================
// Mock Store Implementation
// ============================================================

type mockStore struct {
	bizReviews       map[string]*censor.BizReview
	resourceReviews  map[string]*censor.ResourceReview
	providerTasks    map[string]*censor.ProviderTask
	bindings         map[string]*censor.CensorBinding
	violations       map[string]*censor.ViolationSnapshot
	idCounter        int
	createBizError   error
	createResError   error
}

func newMockStore() *mockStore {
	return &mockStore{
		bizReviews:      make(map[string]*censor.BizReview),
		resourceReviews: make(map[string]*censor.ResourceReview),
		providerTasks:   make(map[string]*censor.ProviderTask),
		bindings:        make(map[string]*censor.CensorBinding),
		violations:      make(map[string]*censor.ViolationSnapshot),
	}
}

func (m *mockStore) nextID() string {
	m.idCounter++
	return string(rune('a'+m.idCounter-1)) + "_" + time.Now().Format("20060102150405")
}

func (m *mockStore) CreateBizReview(ctx context.Context, biz censor.BizContext) (string, error) {
	if m.createBizError != nil {
		return "", m.createBizError
	}
	id := m.nextID()
	m.bizReviews[id] = &censor.BizReview{
		ID:          id,
		BizType:     biz.BizType,
		BizID:       biz.BizID,
		Field:       biz.Field,
		SubmitterID: biz.SubmitterID,
		TraceID:     biz.TraceID,
		Decision:    censor.DecisionPending,
		Status:      censor.StatusPending,
		CreatedAt:   time.Now().UnixMilli(),
	}
	return id, nil
}

func (m *mockStore) GetBizReview(ctx context.Context, bizReviewID string) (*censor.BizReview, error) {
	if br, ok := m.bizReviews[bizReviewID]; ok {
		return br, nil
	}
	return nil, censor.ErrTaskNotFound
}

func (m *mockStore) UpdateBizDecision(ctx context.Context, bizReviewID string, decision censor.Decision) (bool, error) {
	if br, ok := m.bizReviews[bizReviewID]; ok {
		changed := br.Decision != decision
		br.Decision = decision
		return changed, nil
	}
	return false, censor.ErrTaskNotFound
}

func (m *mockStore) UpdateBizStatus(ctx context.Context, bizReviewID string, status censor.ReviewStatus) error {
	if br, ok := m.bizReviews[bizReviewID]; ok {
		br.Status = status
		return nil
	}
	return censor.ErrTaskNotFound
}

func (m *mockStore) CreateResourceReview(ctx context.Context, bizReviewID string, r censor.Resource) (string, error) {
	if m.createResError != nil {
		return "", m.createResError
	}
	id := m.nextID()
	m.resourceReviews[id] = &censor.ResourceReview{
		ID:           id,
		BizReviewID:  bizReviewID,
		ResourceID:   r.ResourceID,
		ResourceType: r.Type,
		ContentHash:  r.ContentHash,
		ContentText:  r.ContentText,
		ContentURL:   r.ContentURL,
		Decision:     censor.DecisionPending,
		CreatedAt:    time.Now().UnixMilli(),
	}
	return id, nil
}

func (m *mockStore) GetResourceReview(ctx context.Context, resourceReviewID string) (*censor.ResourceReview, error) {
	if rr, ok := m.resourceReviews[resourceReviewID]; ok {
		return rr, nil
	}
	return nil, censor.ErrTaskNotFound
}

func (m *mockStore) UpdateResourceOutcome(ctx context.Context, resourceReviewID string, outcome censor.FinalOutcome) error {
	if rr, ok := m.resourceReviews[resourceReviewID]; ok {
		rr.Decision = outcome.Decision
		return nil
	}
	return censor.ErrTaskNotFound
}

func (m *mockStore) ListResourceReviewsByBizReview(ctx context.Context, bizReviewID string) ([]censor.ResourceReview, error) {
	var result []censor.ResourceReview
	for _, rr := range m.resourceReviews {
		if rr.BizReviewID == bizReviewID {
			result = append(result, *rr)
		}
	}
	return result, nil
}

func (m *mockStore) CreateProviderTask(ctx context.Context, resourceReviewID, provider, mode, remoteTaskID string, raw map[string]any) (string, error) {
	id := m.nextID()
	m.providerTasks[id] = &censor.ProviderTask{
		ID:               id,
		ResourceReviewID: resourceReviewID,
		Provider:         provider,
		Mode:             mode,
		RemoteTaskID:     remoteTaskID,
		Done:             false,
		CreatedAt:        time.Now().UnixMilli(),
	}
	return id, nil
}

func (m *mockStore) GetProviderTask(ctx context.Context, taskID string) (*censor.ProviderTask, error) {
	if pt, ok := m.providerTasks[taskID]; ok {
		return pt, nil
	}
	return nil, censor.ErrTaskNotFound
}

func (m *mockStore) GetProviderTaskByRemoteID(ctx context.Context, provider, remoteTaskID string) (*censor.ProviderTask, error) {
	for _, pt := range m.providerTasks {
		if pt.Provider == provider && pt.RemoteTaskID == remoteTaskID {
			return pt, nil
		}
	}
	return nil, censor.ErrTaskNotFound
}

func (m *mockStore) UpdateProviderTaskResult(ctx context.Context, taskID string, done bool, result *censor.ReviewResult, raw map[string]any) error {
	if pt, ok := m.providerTasks[taskID]; ok {
		pt.Done = done
		return nil
	}
	return censor.ErrTaskNotFound
}

func (m *mockStore) ListPendingAsyncTasks(ctx context.Context, provider string, limit int) ([]censor.PendingTask, error) {
	var result []censor.PendingTask
	for _, pt := range m.providerTasks {
		if pt.Provider == provider && !pt.Done {
			result = append(result, censor.PendingTask{
				ProviderTaskID: pt.ID,
				Provider:       pt.Provider,
				RemoteTaskID:   pt.RemoteTaskID,
			})
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (m *mockStore) GetBinding(ctx context.Context, bizType, bizID, field string) (*censor.CensorBinding, error) {
	key := bizType + "/" + bizID + "/" + field
	if b, ok := m.bindings[key]; ok {
		return b, nil
	}
	return nil, nil
}

func (m *mockStore) UpsertBinding(ctx context.Context, binding censor.CensorBinding) error {
	key := binding.BizType + "/" + binding.BizID + "/" + binding.Field
	m.bindings[key] = &binding
	return nil
}

func (m *mockStore) ListBindingsByBiz(ctx context.Context, bizType, bizID string) ([]censor.CensorBinding, error) {
	var result []censor.CensorBinding
	prefix := bizType + "/" + bizID + "/"
	for k, v := range m.bindings {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			result = append(result, *v)
		}
	}
	return result, nil
}

func (m *mockStore) CreateBindingHistory(ctx context.Context, history censor.CensorBindingHistory) error {
	return nil
}

func (m *mockStore) ListBindingHistory(ctx context.Context, bizType, bizID, field string, limit int) ([]censor.CensorBindingHistory, error) {
	return nil, nil
}

func (m *mockStore) SaveViolationSnapshot(ctx context.Context, biz censor.BizContext, r censor.Resource, outcome censor.FinalOutcome) (string, error) {
	id := m.nextID()
	m.violations[id] = &censor.ViolationSnapshot{
		ID:           id,
		BizType:      string(biz.BizType),
		BizID:        biz.BizID,
		Field:        biz.Field,
		ResourceID:   r.ResourceID,
		ResourceType: string(r.Type),
		CreatedAt:    time.Now().UnixMilli(),
	}
	return id, nil
}

func (m *mockStore) GetViolationSnapshot(ctx context.Context, snapshotID string) (*censor.ViolationSnapshot, error) {
	if v, ok := m.violations[snapshotID]; ok {
		return v, nil
	}
	return nil, censor.ErrTaskNotFound
}

func (m *mockStore) ListViolationsByBiz(ctx context.Context, bizType, bizID string, limit int) ([]censor.ViolationSnapshot, error) {
	return nil, nil
}

func (m *mockStore) Now() time.Time {
	return time.Now()
}

func (m *mockStore) WithTx(ctx context.Context, fn func(store.Store) error) error {
	return fn(m)
}

func (m *mockStore) Ping(ctx context.Context) error {
	return nil
}

func (m *mockStore) Close() error {
	return nil
}

// ============================================================
// Mock Provider Implementation
// ============================================================

type mockProvider struct {
	name         string
	submitResult *censor.ReviewResult
	submitError  error
	queryDone    bool
	queryResult  *censor.ReviewResult
}

func newMockProvider(name string) *mockProvider {
	return &mockProvider{
		name: name,
		submitResult: &censor.ReviewResult{
			Decision:   censor.DecisionPass,
			Confidence: 1.0,
			Provider:   name,
			ReviewedAt: time.Now(),
		},
	}
}

func (p *mockProvider) Name() string {
	return p.name
}

func (p *mockProvider) Capabilities() []providers.Capability {
	return []providers.Capability{
		{ResourceType: censor.ResourceText, Modes: []providers.Mode{providers.ModeSync}},
		{ResourceType: censor.ResourceImage, Modes: []providers.Mode{providers.ModeSync}},
	}
}

func (p *mockProvider) SceneCapability() providers.SceneCapability {
	return providers.SceneCapability{
		Provider: p.name,
		SupportedScenes: map[censor.ResourceType][]violation.UnifiedScene{
			censor.ResourceText:  {violation.ScenePornography, violation.ScenePolitics},
			censor.ResourceImage: {violation.ScenePornography, violation.ScenePolitics},
		},
	}
}

func (p *mockProvider) TranslateScenes(scenes []violation.UnifiedScene, resourceType censor.ResourceType) []string {
	return []string{"default"}
}

func (p *mockProvider) Submit(ctx context.Context, req providers.SubmitRequest) (providers.SubmitResponse, error) {
	if p.submitError != nil {
		return providers.SubmitResponse{}, p.submitError
	}
	return providers.SubmitResponse{
		Mode:      providers.ModeSync,
		TaskID:    "task_" + time.Now().Format("20060102150405"),
		Immediate: p.submitResult,
		Raw:       map[string]any{"mock": true},
	}, nil
}

func (p *mockProvider) Query(ctx context.Context, taskID string) (providers.QueryResponse, error) {
	return providers.QueryResponse{
		Done:   p.queryDone,
		Result: p.queryResult,
	}, nil
}

func (p *mockProvider) VerifyCallback(ctx context.Context, headers map[string]string, body []byte) error {
	return nil
}

func (p *mockProvider) ParseCallback(ctx context.Context, body []byte) (providers.CallbackData, error) {
	return providers.CallbackData{
		TaskID: "callback_task",
		Done:   true,
		Result: &censor.ReviewResult{
			Decision:   censor.DecisionPass,
			Confidence: 1.0,
			Provider:   p.name,
		},
	}, nil
}

func (p *mockProvider) Translator() violation.Translator {
	return nil
}

// ============================================================
// Tests
// ============================================================

func TestNew(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockStore := newMockStore()
		mockProv := newMockProvider("test")

		client, err := New(Options{
			Store:     mockStore,
			Providers: []providers.Provider{mockProv},
			Pipeline: PipelineConfig{
				Primary: "test",
			},
		})

		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		if client == nil {
			t.Fatal("New() returned nil client")
		}
	})

	t.Run("error without store", func(t *testing.T) {
		_, err := New(Options{})

		if !errors.Is(err, censor.ErrStoreNotConfigured) {
			t.Errorf("New() error = %v, want %v", err, censor.ErrStoreNotConfigured)
		}
	})

	t.Run("default hooks", func(t *testing.T) {
		mockStore := newMockStore()

		client, err := New(Options{
			Store: mockStore,
		})

		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		if client.hooks == nil {
			t.Error("Client hooks should not be nil")
		}
	})
}

func TestClient_Submit(t *testing.T) {
	t.Run("success with single resource", func(t *testing.T) {
		mockStore := newMockStore()
		mockProv := newMockProvider("test")

		client, _ := New(Options{
			Store:     mockStore,
			Providers: []providers.Provider{mockProv},
			Pipeline: PipelineConfig{
				Primary: "test",
			},
		})

		result, err := client.Submit(context.Background(), SubmitInput{
			Biz: censor.BizContext{
				BizType: censor.BizNoteBody,
				BizID:   "note_123",
				Field:   "body",
			},
			Resources: []censor.Resource{
				{
					ResourceID:  "res_1",
					Type:        censor.ResourceText,
					ContentText: "Hello world",
				},
			},
		})

		if err != nil {
			t.Fatalf("Submit() error = %v", err)
		}
		if result == nil {
			t.Fatal("Submit() returned nil result")
		}
		if result.BizReviewID == "" {
			t.Error("Submit() BizReviewID is empty")
		}
		if len(result.ResourceReviewIDs) != 1 {
			t.Errorf("Submit() ResourceReviewIDs count = %d, want 1", len(result.ResourceReviewIDs))
		}
	})

	t.Run("error with no resources", func(t *testing.T) {
		mockStore := newMockStore()

		client, _ := New(Options{
			Store: mockStore,
		})

		_, err := client.Submit(context.Background(), SubmitInput{
			Biz: censor.BizContext{
				BizType: censor.BizNoteBody,
				BizID:   "note_123",
			},
			Resources: []censor.Resource{},
		})

		if !errors.Is(err, censor.ErrNoResources) {
			t.Errorf("Submit() error = %v, want %v", err, censor.ErrNoResources)
		}
	})

	t.Run("multiple resources", func(t *testing.T) {
		mockStore := newMockStore()
		mockProv := newMockProvider("test")

		client, _ := New(Options{
			Store:     mockStore,
			Providers: []providers.Provider{mockProv},
			Pipeline: PipelineConfig{
				Primary: "test",
			},
		})

		result, err := client.Submit(context.Background(), SubmitInput{
			Biz: censor.BizContext{
				BizType: censor.BizNoteBody,
				BizID:   "note_123",
			},
			Resources: []censor.Resource{
				{ResourceID: "res_1", Type: censor.ResourceText, ContentText: "Text 1"},
				{ResourceID: "res_2", Type: censor.ResourceText, ContentText: "Text 2"},
				{ResourceID: "res_3", Type: censor.ResourceImage, ContentURL: "http://example.com/img.jpg"},
			},
		})

		if err != nil {
			t.Fatalf("Submit() error = %v", err)
		}
		if len(result.ResourceReviewIDs) != 3 {
			t.Errorf("Submit() ResourceReviewIDs count = %d, want 3", len(result.ResourceReviewIDs))
		}
	})

	t.Run("block decision triggers violation", func(t *testing.T) {
		mockStore := newMockStore()
		mockProv := newMockProvider("test")
		mockProv.submitResult = &censor.ReviewResult{
			Decision:   censor.DecisionBlock,
			Confidence: 0.95,
			Provider:   "test",
			Reasons: []censor.Reason{
				{Code: "porn", Message: "Explicit content"},
			},
		}

		client, _ := New(Options{
			Store:     mockStore,
			Providers: []providers.Provider{mockProv},
			Pipeline: PipelineConfig{
				Primary: "test",
			},
		})

		result, err := client.Submit(context.Background(), SubmitInput{
			Biz: censor.BizContext{
				BizType: censor.BizNoteBody,
				BizID:   "note_123",
			},
			Resources: []censor.Resource{
				{ResourceID: "res_1", Type: censor.ResourceText, ContentText: "Bad content"},
			},
		})

		if err != nil {
			t.Fatalf("Submit() error = %v", err)
		}

		// Check that violation was recorded
		if len(mockStore.violations) != 1 {
			t.Errorf("Expected 1 violation snapshot, got %d", len(mockStore.violations))
		}

		// Check immediate result
		if outcome, ok := result.ImmediateResults["res_1"]; ok {
			if outcome.Decision != censor.DecisionBlock {
				t.Errorf("Decision = %v, want Block", outcome.Decision)
			}
		} else {
			t.Error("Missing immediate result for res_1")
		}
	})

	t.Run("store error on biz review creation", func(t *testing.T) {
		mockStore := newMockStore()
		mockStore.createBizError = errors.New("database error")

		client, _ := New(Options{
			Store: mockStore,
		})

		_, err := client.Submit(context.Background(), SubmitInput{
			Biz: censor.BizContext{
				BizType: censor.BizNoteBody,
				BizID:   "note_123",
			},
			Resources: []censor.Resource{
				{ResourceID: "res_1", Type: censor.ResourceText, ContentText: "Test"},
			},
		})

		if err == nil {
			t.Error("Submit() should return error when store fails")
		}
	})
}

func TestClient_Query(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockStore := newMockStore()
		mockProv := newMockProvider("test")

		client, _ := New(Options{
			Store:     mockStore,
			Providers: []providers.Provider{mockProv},
			Pipeline: PipelineConfig{
				Primary: "test",
			},
		})

		// First submit
		submitResult, _ := client.Submit(context.Background(), SubmitInput{
			Biz: censor.BizContext{
				BizType: censor.BizNoteBody,
				BizID:   "note_123",
			},
			Resources: []censor.Resource{
				{ResourceID: "res_1", Type: censor.ResourceText, ContentText: "Test"},
			},
		})

		// Then query
		queryResult, err := client.Query(context.Background(), QueryInput{
			BizReviewID: submitResult.BizReviewID,
		})

		if err != nil {
			t.Fatalf("Query() error = %v", err)
		}
		if queryResult == nil {
			t.Fatal("Query() returned nil result")
		}
		if queryResult.BizReview == nil {
			t.Error("Query() BizReview is nil")
		}
	})

	t.Run("not found", func(t *testing.T) {
		mockStore := newMockStore()

		client, _ := New(Options{
			Store: mockStore,
		})

		_, err := client.Query(context.Background(), QueryInput{
			BizReviewID: "nonexistent",
		})

		if !errors.Is(err, censor.ErrTaskNotFound) {
			t.Errorf("Query() error = %v, want %v", err, censor.ErrTaskNotFound)
		}
	})
}

func TestClient_HandleCallback(t *testing.T) {
	t.Run("provider not found", func(t *testing.T) {
		mockStore := newMockStore()

		client, _ := New(Options{
			Store: mockStore,
		})

		err := client.HandleCallback(context.Background(), "unknown_provider", nil, nil)

		if !errors.Is(err, censor.ErrProviderNotFound) {
			t.Errorf("HandleCallback() error = %v, want %v", err, censor.ErrProviderNotFound)
		}
	})
}

func TestClient_WithHooks(t *testing.T) {
	t.Run("hooks are called", func(t *testing.T) {
		mockStore := newMockStore()
		mockProv := newMockProvider("test")

		hooksCalled := make(map[string]bool)
		testHooks := &testHooks{
			onResourceReviewed: func(ctx context.Context, event hooks.ResourceReviewedEvent) {
				hooksCalled["resource_reviewed"] = true
			},
		}

		client, _ := New(Options{
			Store:     mockStore,
			Providers: []providers.Provider{mockProv},
			Pipeline: PipelineConfig{
				Primary: "test",
			},
			Hooks: testHooks,
		})

		client.Submit(context.Background(), SubmitInput{
			Biz: censor.BizContext{
				BizType: censor.BizNoteBody,
				BizID:   "note_123",
			},
			Resources: []censor.Resource{
				{ResourceID: "res_1", Type: censor.ResourceText, ContentText: "Test"},
			},
		})

		if !hooksCalled["resource_reviewed"] {
			t.Error("OnResourceReviewed hook was not called")
		}
	})
}

// testHooks is a test implementation of hooks.Hooks
type testHooks struct {
	onBizDecisionChanged func(ctx context.Context, event hooks.BizDecisionChangedEvent)
	onResourceReviewed   func(ctx context.Context, event hooks.ResourceReviewedEvent)
	onViolationDetected  func(ctx context.Context, event hooks.ViolationDetectedEvent)
	onManualReviewNeeded func(ctx context.Context, event hooks.ManualReviewRequiredEvent)
}

func (h *testHooks) OnBizDecisionChanged(ctx context.Context, event hooks.BizDecisionChangedEvent) error {
	if h.onBizDecisionChanged != nil {
		h.onBizDecisionChanged(ctx, event)
	}
	return nil
}

func (h *testHooks) OnResourceReviewed(ctx context.Context, event hooks.ResourceReviewedEvent) error {
	if h.onResourceReviewed != nil {
		h.onResourceReviewed(ctx, event)
	}
	return nil
}

func (h *testHooks) OnViolationDetected(ctx context.Context, event hooks.ViolationDetectedEvent) error {
	if h.onViolationDetected != nil {
		h.onViolationDetected(ctx, event)
	}
	return nil
}

func (h *testHooks) OnManualReviewRequired(ctx context.Context, event hooks.ManualReviewRequiredEvent) error {
	if h.onManualReviewNeeded != nil {
		h.onManualReviewNeeded(ctx, event)
	}
	return nil
}
