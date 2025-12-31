package violation

import (
	"testing"

	censor "github.com/heibot/censor"
)

func TestBaseTranslator_Provider(t *testing.T) {
	translator := NewBaseTranslator("test-provider", nil)

	if translator.Provider() != "test-provider" {
		t.Errorf("Provider() = %v, want %v", translator.Provider(), "test-provider")
	}
}

func TestBaseTranslator_Translate(t *testing.T) {
	labelMap := map[string]LabelMapping{
		"porn": {
			Domain:     DomainPornography,
			Tags:       []Tag{TagNudity},
			Severity:   censor.RiskHigh,
			Confidence: 0.9,
		},
		"violence": {
			Domain:     DomainViolence,
			Tags:       []Tag{TagBloodContent},
			Severity:   censor.RiskMedium,
			Confidence: 0.8,
		},
	}

	translator := NewBaseTranslator("test", labelMap)

	tests := []struct {
		name           string
		labels         []string
		scores         map[string]float64
		expectedCount  int
		expectedDomain Domain
	}{
		{
			name:           "known label",
			labels:         []string{"porn"},
			scores:         nil,
			expectedCount:  1,
			expectedDomain: DomainPornography,
		},
		{
			name:           "unknown label maps to other",
			labels:         []string{"unknown_label"},
			scores:         nil,
			expectedCount:  1,
			expectedDomain: DomainOther,
		},
		{
			name:          "multiple labels",
			labels:        []string{"porn", "violence"},
			scores:        nil,
			expectedCount: 2,
		},
		{
			name:          "empty labels",
			labels:        []string{},
			scores:        nil,
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := TranslationContext{
				ResourceType: censor.ResourceText,
				BizType:      censor.BizNoteBody,
			}

			result := translator.Translate(ctx, tt.labels, tt.scores)

			if len(result) != tt.expectedCount {
				t.Errorf("Translate() returned %d violations, want %d", len(result), tt.expectedCount)
			}

			if tt.expectedCount > 0 && tt.expectedDomain != "" {
				if result[0].Domain != tt.expectedDomain {
					t.Errorf("Translate() domain = %v, want %v", result[0].Domain, tt.expectedDomain)
				}
			}
		})
	}
}

func TestBaseTranslator_TranslateWithScores(t *testing.T) {
	labelMap := map[string]LabelMapping{
		"porn": {
			Domain:     DomainPornography,
			Tags:       []Tag{TagNudity},
			Severity:   censor.RiskHigh,
			Confidence: 0.5, // default confidence
		},
	}

	translator := NewBaseTranslator("test", labelMap)

	ctx := TranslationContext{
		ResourceType: censor.ResourceText,
		BizType:      censor.BizNoteBody,
	}

	scores := map[string]float64{
		"porn": 0.95,
	}

	result := translator.Translate(ctx, []string{"porn"}, scores)

	if len(result) != 1 {
		t.Fatalf("Translate() returned %d violations, want 1", len(result))
	}

	// Score should override default confidence
	if result[0].Confidence != 0.95 {
		t.Errorf("Confidence = %v, want 0.95", result[0].Confidence)
	}
}

func TestBaseTranslator_AdjustSeverity(t *testing.T) {
	labelMap := map[string]LabelMapping{
		"test": {
			Domain:     DomainOther,
			Tags:       []Tag{},
			Severity:   censor.RiskMedium,
			Confidence: 0.5,
		},
		"test_high": {
			Domain:     DomainOther,
			Tags:       []Tag{},
			Severity:   censor.RiskHigh,
			Confidence: 0.5,
		},
	}

	translator := NewBaseTranslator("test", labelMap)

	// User profile fields should be stricter
	ctx := TranslationContext{
		ResourceType: censor.ResourceText,
		BizType:      censor.BizUserNickname,
	}

	result := translator.Translate(ctx, []string{"test"}, nil)
	if result[0].Severity != censor.RiskHigh {
		t.Errorf("Severity for user nickname = %v, want RiskHigh", result[0].Severity)
	}

	// Chat messages should be more lenient
	ctx = TranslationContext{
		ResourceType: censor.ResourceText,
		BizType:      censor.BizChatMessage,
	}

	result = translator.Translate(ctx, []string{"test_high"}, nil)
	if result[0].Severity != censor.RiskMedium {
		t.Errorf("Severity for chat message = %v, want RiskMedium", result[0].Severity)
	}
}

func TestMergeViolations(t *testing.T) {
	list1 := UnifiedList{
		{
			Domain:          DomainPornography,
			Tags:            []Tag{TagPornographicAct},
			Severity:        censor.RiskMedium,
			Confidence:      0.7,
			SourceProviders: []string{"provider1"},
			OriginalLabels:  []string{"porn"},
		},
	}

	list2 := UnifiedList{
		{
			Domain:          DomainPornography,
			Tags:            []Tag{TagNudity},
			Severity:        censor.RiskHigh,
			Confidence:      0.9,
			SourceProviders: []string{"provider2"},
			OriginalLabels:  []string{"nude"},
		},
		{
			Domain:          DomainViolence,
			Tags:            []Tag{TagBloodContent},
			Severity:        censor.RiskLow,
			Confidence:      0.5,
			SourceProviders: []string{"provider2"},
			OriginalLabels:  []string{"blood"},
		},
	}

	merged := MergeViolations(list1, list2)

	if len(merged) != 2 {
		t.Fatalf("MergeViolations() returned %d violations, want 2", len(merged))
	}

	// Find the pornography violation
	var pornViolation *Unified
	for i := range merged {
		if merged[i].Domain == DomainPornography {
			pornViolation = &merged[i]
			break
		}
	}

	if pornViolation == nil {
		t.Fatal("MergeViolations() missing pornography violation")
	}

	// Should have higher severity
	if pornViolation.Severity != censor.RiskHigh {
		t.Errorf("Merged severity = %v, want RiskHigh", pornViolation.Severity)
	}

	// Should have higher confidence
	if pornViolation.Confidence != 0.9 {
		t.Errorf("Merged confidence = %v, want 0.9", pornViolation.Confidence)
	}

	// Should have merged tags
	if len(pornViolation.Tags) != 2 {
		t.Errorf("Merged tags count = %d, want 2", len(pornViolation.Tags))
	}

	// Should have merged providers
	if len(pornViolation.SourceProviders) != 2 {
		t.Errorf("Merged providers count = %d, want 2", len(pornViolation.SourceProviders))
	}
}

func TestMergeViolations_Empty(t *testing.T) {
	merged := MergeViolations()

	if len(merged) != 0 {
		t.Errorf("MergeViolations() with no args returned %d violations, want 0", len(merged))
	}
}
