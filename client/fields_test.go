package client

import (
	"context"
	"testing"

	censor "github.com/heibot/censor"
	"github.com/heibot/censor/providers"
)

func TestSubmitFields_SingleField(t *testing.T) {
	mockStore := newMockStore()
	mockProv := newMockProvider("test")

	client, _ := New(Options{
		Store:     mockStore,
		Providers: []providers.Provider{mockProv},
		Pipeline: PipelineConfig{
			Primary: "test",
		},
	})

	result, err := client.SubmitFields(context.Background(), SubmitFieldsInput{
		BizType: censor.BizNoteBody,
		BizID:   "note_123",
		Fields: []FieldInput{
			{Field: "title", Text: "Hello World", OnBlock: ActionReplace, ReplaceWith: "标题违规"},
		},
	})

	if err != nil {
		t.Fatalf("SubmitFields() error = %v", err)
	}
	if result == nil {
		t.Fatal("SubmitFields() returned nil result")
	}
	if result.OverallDecision != censor.DecisionPass {
		t.Errorf("OverallDecision = %v, want Pass", result.OverallDecision)
	}
	if fr, ok := result.FieldResults["title"]; !ok {
		t.Error("Missing field result for 'title'")
	} else if fr.Decision != censor.DecisionPass {
		t.Errorf("title Decision = %v, want Pass", fr.Decision)
	}
}

func TestSubmitFields_MultipleFieldsAllPass(t *testing.T) {
	mockStore := newMockStore()
	mockProv := newMockProvider("test")

	client, _ := New(Options{
		Store:     mockStore,
		Providers: []providers.Provider{mockProv},
		Pipeline: PipelineConfig{
			Primary: "test",
		},
	})

	result, err := client.SubmitFields(context.Background(), SubmitFieldsInput{
		BizType: censor.BizNoteBody,
		BizID:   "note_123",
		Fields: []FieldInput{
			{Field: "name", Text: "队伍名称", OnBlock: ActionReplace, ReplaceWith: "名称违规"},
			{Field: "desc", Text: "队伍描述", OnBlock: ActionReplace, ReplaceWith: "描述违规"},
		},
	})

	if err != nil {
		t.Fatalf("SubmitFields() error = %v", err)
	}
	if result.OverallDecision != censor.DecisionPass {
		t.Errorf("OverallDecision = %v, want Pass", result.OverallDecision)
	}
	if len(result.FieldResults) != 2 {
		t.Errorf("FieldResults count = %d, want 2", len(result.FieldResults))
	}
	for field, fr := range result.FieldResults {
		if fr.Decision != censor.DecisionPass {
			t.Errorf("%s Decision = %v, want Pass", field, fr.Decision)
		}
		if fr.WasReplaced {
			t.Errorf("%s should not be replaced when passing", field)
		}
	}
}

func TestSubmitFields_BlockWithKeywordLocation(t *testing.T) {
	mockStore := newMockStore()
	mockProv := newMockProvider("test")
	mockProv.submitResult = &censor.ReviewResult{
		Decision:   censor.DecisionBlock,
		Confidence: 0.95,
		Provider:   "test",
		Reasons: []censor.Reason{
			{
				Code:    "porn",
				Message: "Explicit content",
				HitTags: []string{"违禁词"},
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

	result, err := client.SubmitFields(context.Background(), SubmitFieldsInput{
		BizType:           censor.BizNoteBody,
		BizID:             "note_123",
		FallbackThreshold: 0.5, // Lower threshold for testing
		Fields: []FieldInput{
			{Field: "name", Text: "正常名称", OnBlock: ActionReplace, ReplaceWith: "名称违规"},
			{Field: "desc", Text: "包含违禁词的描述", OnBlock: ActionReplace, ReplaceWith: "描述违规"},
		},
	})

	if err != nil {
		t.Fatalf("SubmitFields() error = %v", err)
	}
	if result.OverallDecision != censor.DecisionBlock {
		t.Errorf("OverallDecision = %v, want Block", result.OverallDecision)
	}

	// Check that desc was blocked and replaced (keyword located)
	if descResult, ok := result.FieldResults["desc"]; ok {
		if descResult.LocatedBy == "keyword" {
			if descResult.Decision != censor.DecisionBlock {
				t.Errorf("desc Decision = %v, want Block", descResult.Decision)
			}
			if !descResult.WasReplaced {
				t.Error("desc should be replaced")
			}
			if descResult.FinalValue != "描述违规" {
				t.Errorf("desc FinalValue = %v, want '描述违规'", descResult.FinalValue)
			}
		}
	}
}

func TestSubmitFields_ConservativeApproach(t *testing.T) {
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

	result, err := client.SubmitFields(context.Background(), SubmitFieldsInput{
		BizType:         censor.BizNoteBody,
		BizID:           "note_123",
		DisableFallback: true, // Use conservative approach
		Fields: []FieldInput{
			{Field: "name", Text: "名称", OnBlock: ActionReplace, ReplaceWith: "名称违规"},
			{Field: "desc", Text: "描述", OnBlock: ActionReplace, ReplaceWith: "描述违规"},
		},
	})

	if err != nil {
		t.Fatalf("SubmitFields() error = %v", err)
	}

	// With conservative approach, all fields should be blocked
	for field, fr := range result.FieldResults {
		if fr.Decision != censor.DecisionBlock {
			t.Errorf("%s Decision = %v, want Block", field, fr.Decision)
		}
		if fr.LocatedBy != "conservative" {
			t.Errorf("%s LocatedBy = %v, want 'conservative'", field, fr.LocatedBy)
		}
		if !fr.WasReplaced {
			t.Errorf("%s should be replaced", field)
		}
	}
}

func TestSubmitFields_NoFields(t *testing.T) {
	mockStore := newMockStore()

	client, _ := New(Options{
		Store: mockStore,
	})

	_, err := client.SubmitFields(context.Background(), SubmitFieldsInput{
		BizType: censor.BizNoteBody,
		BizID:   "note_123",
		Fields:  []FieldInput{},
	})

	if err != censor.ErrNoResources {
		t.Errorf("SubmitFields() error = %v, want ErrNoResources", err)
	}
}

func TestBlockAction_Replace(t *testing.T) {
	field := FieldInput{
		Field:       "name",
		Text:        "原始内容",
		OnBlock:     ActionReplace,
		ReplaceWith: "替换内容",
	}

	finalValue, wasReplaced := applyBlockAction(field, censor.DecisionBlock)

	if !wasReplaced {
		t.Error("Should be replaced")
	}
	if finalValue != "替换内容" {
		t.Errorf("FinalValue = %v, want '替换内容'", finalValue)
	}
}

func TestBlockAction_Hide(t *testing.T) {
	field := FieldInput{
		Field:   "name",
		Text:    "原始内容",
		OnBlock: ActionHide,
	}

	finalValue, wasReplaced := applyBlockAction(field, censor.DecisionBlock)

	if !wasReplaced {
		t.Error("Should be replaced")
	}
	if finalValue != "" {
		t.Errorf("FinalValue = %v, want empty string", finalValue)
	}
}

func TestBlockAction_PassThrough(t *testing.T) {
	field := FieldInput{
		Field:   "name",
		Text:    "原始内容",
		OnBlock: ActionPassThrough,
	}

	finalValue, wasReplaced := applyBlockAction(field, censor.DecisionBlock)

	if wasReplaced {
		t.Error("Should not be replaced")
	}
	if finalValue != "原始内容" {
		t.Errorf("FinalValue = %v, want '原始内容'", finalValue)
	}
}

func TestBlockAction_ReplaceWithDefault(t *testing.T) {
	field := FieldInput{
		Field:       "name",
		Text:        "原始内容",
		OnBlock:     ActionReplace,
		ReplaceWith: "", // Empty replacement
	}

	finalValue, wasReplaced := applyBlockAction(field, censor.DecisionBlock)

	if !wasReplaced {
		t.Error("Should be replaced")
	}
	if finalValue != "***" {
		t.Errorf("FinalValue = %v, want '***'", finalValue)
	}
}

func TestMergeFieldTexts(t *testing.T) {
	fields := []FieldInput{
		{Field: "name", Text: "队伍名称"},
		{Field: "desc", Text: "队伍描述"},
	}

	merged, fieldIndex := mergeFieldTexts(fields)

	if merged.Merged != "队伍名称\n---\n队伍描述" {
		t.Errorf("Merged = %v", merged.Merged)
	}
	if len(merged.Parts) != 2 {
		t.Errorf("Parts count = %d, want 2", len(merged.Parts))
	}

	// Check indices
	nameIdx := fieldIndex["name"]
	if nameIdx.Start != 0 || nameIdx.End != len("队伍名称") {
		t.Errorf("name index = %+v", nameIdx)
	}

	descIdx := fieldIndex["desc"]
	expectedStart := len("队伍名称") + len("\n---\n")
	if descIdx.Start != expectedStart {
		t.Errorf("desc start = %d, want %d", descIdx.Start, expectedStart)
	}
}

func TestLocateByKeyword(t *testing.T) {
	fields := []FieldInput{
		{Field: "name", Text: "正常名称"},
		{Field: "desc", Text: "包含敏感词的描述"},
	}

	reasons := []censor.Reason{
		{
			HitTags: []string{"敏感词"},
		},
	}

	located, confidence := locateByKeyword(fields, reasons)

	if len(located) != 1 {
		t.Fatalf("located count = %d, want 1", len(located))
	}
	if located[0] != "desc" {
		t.Errorf("located = %v, want 'desc'", located)
	}
	if confidence == 0 {
		t.Error("confidence should be > 0")
	}
}

func TestExtractKeywords(t *testing.T) {
	tests := []struct {
		name     string
		reason   censor.Reason
		wantLen  int
	}{
		{
			name: "from HitTags",
			reason: censor.Reason{
				HitTags: []string{"keyword1", "keyword2"},
			},
			wantLen: 2,
		},
		{
			name: "from Raw keywords",
			reason: censor.Reason{
				Raw: map[string]any{
					"keywords": []any{"kw1", "kw2"},
				},
			},
			wantLen: 2,
		},
		{
			name:    "empty",
			reason:  censor.Reason{},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keywords := extractKeywords(tt.reason)
			if len(keywords) != tt.wantLen {
				t.Errorf("extractKeywords() count = %d, want %d", len(keywords), tt.wantLen)
			}
		})
	}
}
