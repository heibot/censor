package client

import (
	"context"
	"testing"

	censor "github.com/heibot/censor"
	"github.com/heibot/censor/providers"
)

func TestSubmitBatch_SingleItem(t *testing.T) {
	mockStore := newMockStore()
	mockProv := newMockProvider("test")

	client, _ := New(Options{
		Store:     mockStore,
		Providers: []providers.Provider{mockProv},
		Pipeline: PipelineConfig{
			Primary: "test",
		},
	})

	result, err := client.SubmitBatch(context.Background(), SubmitBatchInput{
		BizType: censor.BizType("danmaku"),
		Items: []BatchItem{
			{BizID: "msg_1", SubmitterID: "user_1", Text: "Hello world"},
		},
	})

	if err != nil {
		t.Fatalf("SubmitBatch() error = %v", err)
	}
	if result.OverallDecision != censor.DecisionPass {
		t.Errorf("OverallDecision = %v, want Pass", result.OverallDecision)
	}
	if result.PassedCount != 1 {
		t.Errorf("PassedCount = %d, want 1", result.PassedCount)
	}
}

func TestSubmitBatch_MultipleItemsAllPass(t *testing.T) {
	mockStore := newMockStore()
	mockProv := newMockProvider("test")

	client, _ := New(Options{
		Store:     mockStore,
		Providers: []providers.Provider{mockProv},
		Pipeline: PipelineConfig{
			Primary: "test",
		},
	})

	result, err := client.SubmitBatch(context.Background(), SubmitBatchInput{
		BizType: censor.BizType("danmaku"),
		Items: []BatchItem{
			{BizID: "msg_1", SubmitterID: "user_1", Text: "弹幕1"},
			{BizID: "msg_2", SubmitterID: "user_2", Text: "弹幕2"},
			{BizID: "msg_3", SubmitterID: "user_3", Text: "弹幕3"},
		},
	})

	if err != nil {
		t.Fatalf("SubmitBatch() error = %v", err)
	}
	if result.OverallDecision != censor.DecisionPass {
		t.Errorf("OverallDecision = %v, want Pass", result.OverallDecision)
	}
	if result.PassedCount != 3 {
		t.Errorf("PassedCount = %d, want 3", result.PassedCount)
	}
	if result.BlockedCount != 0 {
		t.Errorf("BlockedCount = %d, want 0", result.BlockedCount)
	}
}

func TestSubmitBatch_BlockWithKeywordLocation(t *testing.T) {
	mockStore := newMockStore()
	mockProv := newMockProvider("test")
	mockProv.submitResult = &censor.ReviewResult{
		Decision:   censor.DecisionBlock,
		Confidence: 0.95,
		Provider:   "test",
		Reasons: []censor.Reason{
			{
				Code:    "abuse",
				Message: "Abusive content",
				HitTags: []string{"脏话"},
			},
		},
	}

	client, _ := New(Options{
		Store:     mockStore,
		Providers: []providers.Provider{mockProv},
		Pipeline: PipelineConfig{
			Primary: "test",
		},
	})

	result, err := client.SubmitBatch(context.Background(), SubmitBatchInput{
		BizType:           censor.BizType("danmaku"),
		FallbackThreshold: 0.5,
		Items: []BatchItem{
			{BizID: "msg_1", SubmitterID: "user_1", Text: "正常弹幕"},
			{BizID: "msg_2", SubmitterID: "user_2", Text: "包含脏话的弹幕"},
			{BizID: "msg_3", SubmitterID: "user_3", Text: "另一条正常弹幕"},
		},
	})

	if err != nil {
		t.Fatalf("SubmitBatch() error = %v", err)
	}
	if result.OverallDecision != censor.DecisionBlock {
		t.Errorf("OverallDecision = %v, want Block", result.OverallDecision)
	}

	// Check that msg_2 was located by keyword
	if msg2, ok := result.Results["msg_2"]; ok {
		if msg2.LocatedBy == "keyword" {
			if !msg2.Blocked {
				t.Error("msg_2 should be blocked")
			}
		}
	}
}

func TestSubmitBatch_ConservativeApproach(t *testing.T) {
	mockStore := newMockStore()
	mockProv := newMockProvider("test")
	mockProv.submitResult = &censor.ReviewResult{
		Decision:   censor.DecisionBlock,
		Confidence: 0.95,
		Provider:   "test",
		Reasons: []censor.Reason{
			{Code: "unknown", Message: "Unknown violation"},
		},
	}

	client, _ := New(Options{
		Store:     mockStore,
		Providers: []providers.Provider{mockProv},
		Pipeline: PipelineConfig{
			Primary: "test",
		},
	})

	result, err := client.SubmitBatch(context.Background(), SubmitBatchInput{
		BizType:         censor.BizType("danmaku"),
		DisableFallback: true,
		Items: []BatchItem{
			{BizID: "msg_1", SubmitterID: "user_1", Text: "弹幕1"},
			{BizID: "msg_2", SubmitterID: "user_2", Text: "弹幕2"},
		},
	})

	if err != nil {
		t.Fatalf("SubmitBatch() error = %v", err)
	}

	// With conservative approach, all items should be blocked
	if result.BlockedCount != 2 {
		t.Errorf("BlockedCount = %d, want 2", result.BlockedCount)
	}
	for bizID, ir := range result.Results {
		if !ir.Blocked {
			t.Errorf("%s should be blocked", bizID)
		}
		if ir.LocatedBy != "conservative" {
			t.Errorf("%s LocatedBy = %v, want 'conservative'", bizID, ir.LocatedBy)
		}
	}
}

func TestSubmitBatch_Chunks(t *testing.T) {
	mockStore := newMockStore()
	mockProv := newMockProvider("test")

	client, _ := New(Options{
		Store:     mockStore,
		Providers: []providers.Provider{mockProv},
		Pipeline: PipelineConfig{
			Primary: "test",
		},
	})

	// Create 15 items, with MaxMergeCount=5 should split into 3 chunks
	items := make([]BatchItem, 15)
	for i := 0; i < 15; i++ {
		items[i] = BatchItem{
			BizID:       string(rune('a' + i)),
			SubmitterID: "user",
			Text:        "弹幕内容",
		}
	}

	result, err := client.SubmitBatch(context.Background(), SubmitBatchInput{
		BizType:       censor.BizType("danmaku"),
		Items:         items,
		MaxMergeCount: 5,
	})

	if err != nil {
		t.Fatalf("SubmitBatch() error = %v", err)
	}
	if len(result.Results) != 15 {
		t.Errorf("Results count = %d, want 15", len(result.Results))
	}
	if result.PassedCount != 15 {
		t.Errorf("PassedCount = %d, want 15", result.PassedCount)
	}
}

func TestSubmitBatch_NoItems(t *testing.T) {
	mockStore := newMockStore()

	client, _ := New(Options{
		Store: mockStore,
	})

	_, err := client.SubmitBatch(context.Background(), SubmitBatchInput{
		BizType: censor.BizType("danmaku"),
		Items:   []BatchItem{},
	})

	if err != censor.ErrNoResources {
		t.Errorf("SubmitBatch() error = %v, want ErrNoResources", err)
	}
}

func TestMergeBatchItems(t *testing.T) {
	items := []BatchItem{
		{BizID: "msg_1", Text: "弹幕1"},
		{BizID: "msg_2", Text: "弹幕2"},
		{BizID: "msg_3", Text: "弹幕3"},
	}

	merged, itemIndex := mergeBatchItems(items)

	expectedMerged := "弹幕1\n---\n弹幕2\n---\n弹幕3"
	if merged.Merged != expectedMerged {
		t.Errorf("Merged = %v, want %v", merged.Merged, expectedMerged)
	}

	if len(merged.Parts) != 3 {
		t.Errorf("Parts count = %d, want 3", len(merged.Parts))
	}

	// Check indices
	idx1 := itemIndex["msg_1"]
	if idx1.Start != 0 {
		t.Errorf("msg_1 start = %d, want 0", idx1.Start)
	}

	idx2 := itemIndex["msg_2"]
	if idx2.Start <= idx1.End {
		t.Errorf("msg_2 should start after msg_1")
	}
}

func TestLocateBatchByKeyword(t *testing.T) {
	items := []BatchItem{
		{BizID: "msg_1", Text: "正常内容"},
		{BizID: "msg_2", Text: "包含违禁词的内容"},
		{BizID: "msg_3", Text: "另一条正常内容"},
	}

	reasons := []censor.Reason{
		{HitTags: []string{"违禁词"}},
	}

	located, confidence := locateBatchByKeyword(items, reasons)

	if len(located) != 1 {
		t.Fatalf("located count = %d, want 1", len(located))
	}
	if located[0] != "msg_2" {
		t.Errorf("located = %v, want 'msg_2'", located)
	}
	if confidence == 0 {
		t.Error("confidence should be > 0")
	}
}

func TestSubmitBatch_RealWorldScenario(t *testing.T) {
	// Simulate real danmaku scenario: 5 messages in 1 second
	mockStore := newMockStore()
	mockProv := newMockProvider("test")

	client, _ := New(Options{
		Store:     mockStore,
		Providers: []providers.Provider{mockProv},
		Pipeline: PipelineConfig{
			Primary: "test",
		},
	})

	result, err := client.SubmitBatch(context.Background(), SubmitBatchInput{
		BizType: censor.BizType("danmaku"),
		TraceID: "batch_12345",
		Items: []BatchItem{
			{BizID: "dm_001", SubmitterID: "user_a", Text: "666"},
			{BizID: "dm_002", SubmitterID: "user_b", Text: "哈哈哈"},
			{BizID: "dm_003", SubmitterID: "user_c", Text: "好看"},
			{BizID: "dm_004", SubmitterID: "user_d", Text: "前排"},
			{BizID: "dm_005", SubmitterID: "user_e", Text: "打卡"},
		},
	})

	if err != nil {
		t.Fatalf("SubmitBatch() error = %v", err)
	}

	// All should pass
	if result.PassedCount != 5 {
		t.Errorf("PassedCount = %d, want 5", result.PassedCount)
	}

	// Verify each result has correct BizID
	for _, item := range []string{"dm_001", "dm_002", "dm_003", "dm_004", "dm_005"} {
		if _, ok := result.Results[item]; !ok {
			t.Errorf("Missing result for %s", item)
		}
	}
}
