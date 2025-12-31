package manual

import (
	"context"
	"testing"
	"time"

	censor "github.com/heibot/censor"
	"github.com/heibot/censor/providers"
)

func TestNew(t *testing.T) {
	cfg := DefaultConfig()
	provider := New(cfg)

	if provider == nil {
		t.Fatal("New() returned nil")
	}

	if provider.Name() != "manual" {
		t.Errorf("Name() = %q, want %q", provider.Name(), "manual")
	}
}

func TestProvider_Capabilities(t *testing.T) {
	provider := New(DefaultConfig())
	caps := provider.Capabilities()

	if len(caps) != 3 {
		t.Errorf("Capabilities() returned %d items, want 3", len(caps))
	}

	// All capabilities should be async only
	for _, cap := range caps {
		if len(cap.Modes) != 1 || cap.Modes[0] != providers.ModeAsync {
			t.Errorf("Capability for %v should only support async mode", cap.ResourceType)
		}
	}
}

func TestProvider_SceneCapability(t *testing.T) {
	provider := New(DefaultConfig())
	sceneCap := provider.SceneCapability()

	if sceneCap.Provider != "manual" {
		t.Errorf("SceneCapability().Provider = %q, want %q", sceneCap.Provider, "manual")
	}

	if sceneCap.SyncSupported {
		t.Error("SceneCapability().SyncSupported = true, want false")
	}

	if !sceneCap.AsyncSupported {
		t.Error("SceneCapability().AsyncSupported = false, want true")
	}

	// Should support text, image, video
	if len(sceneCap.SupportedScenes) != 3 {
		t.Errorf("SceneCapability().SupportedScenes has %d types, want 3", len(sceneCap.SupportedScenes))
	}
}

func TestProvider_Submit(t *testing.T) {
	provider := New(DefaultConfig())

	req := providers.SubmitRequest{
		Resource: censor.Resource{
			ResourceID:  "res_123",
			Type:        censor.ResourceText,
			ContentText: "test content",
		},
		Biz: censor.BizContext{
			BizID:   "biz_123",
			BizType: censor.BizNoteBody,
		},
	}

	resp, err := provider.Submit(context.Background(), req)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	if resp.Mode != providers.ModeAsync {
		t.Errorf("Submit().Mode = %v, want async", resp.Mode)
	}

	if resp.TaskID == "" {
		t.Error("Submit().TaskID is empty")
	}

	// Task should be in pending state
	task, err := provider.store.GetTask(context.Background(), resp.TaskID)
	if err != nil {
		t.Fatalf("GetTask() error = %v", err)
	}

	if task == nil {
		t.Fatal("Task not found in store")
	}

	if task.Done {
		t.Error("Task should not be done yet")
	}
}

func TestProvider_Query_NotFound(t *testing.T) {
	provider := New(DefaultConfig())

	resp, err := provider.Query(context.Background(), "nonexistent_task")
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}

	if resp.Done {
		t.Error("Query() for nonexistent task should return Done=false")
	}
}

func TestProvider_Query_Pending(t *testing.T) {
	provider := New(DefaultConfig())

	// Submit a task
	req := providers.SubmitRequest{
		Resource: censor.Resource{
			ResourceID: "res_123",
			Type:       censor.ResourceText,
		},
		Biz: censor.BizContext{
			BizID: "biz_123",
		},
	}

	submitResp, _ := provider.Submit(context.Background(), req)

	// Query before completion
	resp, err := provider.Query(context.Background(), submitResp.TaskID)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}

	if resp.Done {
		t.Error("Query() should return Done=false for pending task")
	}
}

func TestProvider_SubmitResult(t *testing.T) {
	provider := New(DefaultConfig())

	// Submit a task
	req := providers.SubmitRequest{
		Resource: censor.Resource{
			ResourceID: "res_123",
			Type:       censor.ResourceText,
		},
		Biz: censor.BizContext{
			BizID: "biz_123",
		},
	}

	submitResp, _ := provider.Submit(context.Background(), req)

	// Submit result
	result := ManualResult{
		TaskID:     submitResp.TaskID,
		Decision:   censor.DecisionBlock,
		ReviewerID: "reviewer_1",
		Comment:    "Blocked for policy violation",
		Reasons: []censor.Reason{
			{Code: "pornography", Message: "Contains explicit content"},
		},
	}

	err := provider.SubmitResult(context.Background(), submitResp.TaskID, result)
	if err != nil {
		t.Fatalf("SubmitResult() error = %v", err)
	}

	// Query should now return done
	resp, err := provider.Query(context.Background(), submitResp.TaskID)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}

	if !resp.Done {
		t.Error("Query() should return Done=true after SubmitResult")
	}

	if resp.Result == nil {
		t.Fatal("Query().Result is nil")
	}

	if resp.Result.Decision != censor.DecisionBlock {
		t.Errorf("Query().Result.Decision = %v, want Block", resp.Result.Decision)
	}

	if resp.Result.Confidence != 1.0 {
		t.Errorf("Query().Result.Confidence = %v, want 1.0", resp.Result.Confidence)
	}
}

func TestProvider_Query_Timeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DefaultTimeout = 1 * time.Millisecond // Very short timeout for testing
	provider := New(cfg)

	// Submit a task
	req := providers.SubmitRequest{
		Resource: censor.Resource{
			ResourceID: "res_123",
			Type:       censor.ResourceText,
		},
		Biz: censor.BizContext{
			BizID: "biz_123",
		},
	}

	submitResp, _ := provider.Submit(context.Background(), req)

	// Wait for timeout
	time.Sleep(10 * time.Millisecond)

	// Query should return timeout
	resp, err := provider.Query(context.Background(), submitResp.TaskID)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}

	if !resp.Done {
		t.Error("Query() should return Done=true for timed out task")
	}

	if resp.Result == nil {
		t.Fatal("Query().Result is nil")
	}

	if resp.Result.Decision != censor.DecisionReview {
		t.Errorf("Query().Result.Decision = %v, want Review (for timeout)", resp.Result.Decision)
	}
}

func TestProvider_GetPendingTasks(t *testing.T) {
	provider := New(DefaultConfig())

	// Submit multiple tasks
	for i := 0; i < 5; i++ {
		req := providers.SubmitRequest{
			Resource: censor.Resource{
				ResourceID: "res_" + string(rune('0'+i)),
				Type:       censor.ResourceText,
			},
			Biz: censor.BizContext{
				BizID: "biz_" + string(rune('0'+i)),
			},
		}
		provider.Submit(context.Background(), req)
	}

	// Get pending tasks
	tasks, err := provider.GetPendingTasks(context.Background(), 10)
	if err != nil {
		t.Fatalf("GetPendingTasks() error = %v", err)
	}

	if len(tasks) != 5 {
		t.Errorf("GetPendingTasks() returned %d tasks, want 5", len(tasks))
	}
}

func TestProvider_GetPendingTasks_Limit(t *testing.T) {
	provider := New(DefaultConfig())

	// Submit multiple tasks
	for i := 0; i < 10; i++ {
		req := providers.SubmitRequest{
			Resource: censor.Resource{
				ResourceID: "res_" + string(rune('0'+i)),
				Type:       censor.ResourceText,
			},
			Biz: censor.BizContext{
				BizID: "biz_" + string(rune('0'+i)),
			},
		}
		provider.Submit(context.Background(), req)
	}

	// Get pending tasks with limit
	tasks, err := provider.GetPendingTasks(context.Background(), 3)
	if err != nil {
		t.Fatalf("GetPendingTasks() error = %v", err)
	}

	if len(tasks) != 3 {
		t.Errorf("GetPendingTasks() returned %d tasks, want 3", len(tasks))
	}
}

func TestProvider_WithHandler(t *testing.T) {
	provider := New(DefaultConfig())

	handlerCalled := false
	provider.WithHandler(func(ctx context.Context, task ManualTask) error {
		handlerCalled = true
		return nil
	})

	req := providers.SubmitRequest{
		Resource: censor.Resource{
			ResourceID: "res_123",
			Type:       censor.ResourceText,
		},
		Biz: censor.BizContext{
			BizID: "biz_123",
		},
	}

	provider.Submit(context.Background(), req)

	if !handlerCalled {
		t.Error("Handler was not called on Submit")
	}
}

func TestProvider_ParseCallback(t *testing.T) {
	provider := New(DefaultConfig())

	body := []byte(`{
		"task_id": "manual_123",
		"decision": "block",
		"reviewer_id": "admin",
		"comment": "Policy violation",
		"reasons": [{"code": "porn", "message": "Explicit content"}]
	}`)

	data, err := provider.ParseCallback(context.Background(), body)
	if err != nil {
		t.Fatalf("ParseCallback() error = %v", err)
	}

	if data.TaskID != "manual_123" {
		t.Errorf("ParseCallback().TaskID = %q, want %q", data.TaskID, "manual_123")
	}

	if !data.Done {
		t.Error("ParseCallback().Done = false, want true")
	}

	if data.Result.Decision != censor.DecisionBlock {
		t.Errorf("ParseCallback().Result.Decision = %v, want Block", data.Result.Decision)
	}
}

func TestProvider_Translator(t *testing.T) {
	provider := New(DefaultConfig())

	translator := provider.Translator()
	if translator == nil {
		t.Fatal("Translator() returned nil")
	}

	if translator.Provider() != "manual" {
		t.Errorf("Translator().Provider() = %q, want %q", translator.Provider(), "manual")
	}
}

func TestCalculatePriority(t *testing.T) {
	tests := []struct {
		bizType  censor.BizType
		expected int
	}{
		{censor.BizUserNickname, 10},
		{censor.BizUserAvatar, 10},
		{censor.BizNoteTitle, 8},
		{censor.BizNoteBody, 8},
		{censor.BizChatMessage, 6},
		{censor.BizDanmaku, 6},
		{censor.BizType("unknown"), 5},
	}

	for _, tt := range tests {
		t.Run(string(tt.bizType), func(t *testing.T) {
			biz := censor.BizContext{BizType: tt.bizType}
			result := calculatePriority(biz)
			if result != tt.expected {
				t.Errorf("calculatePriority(%v) = %d, want %d", tt.bizType, result, tt.expected)
			}
		})
	}
}
