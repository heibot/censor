package utils

import (
	"testing"

	censor "github.com/heibot/censor"
)

func TestMergeTexts(t *testing.T) {
	strategy := censor.TextMergeStrategy{
		Separator: "\n---\n",
		MaxLen:    1000,
	}

	tests := []struct {
		name          string
		parts         []string
		expectedOk    bool
		expectedMerge string
	}{
		{
			name:       "empty parts",
			parts:      []string{},
			expectedOk: false,
		},
		{
			name:          "single part",
			parts:         []string{"hello"},
			expectedOk:    true,
			expectedMerge: "hello",
		},
		{
			name:          "two parts",
			parts:         []string{"hello", "world"},
			expectedOk:    true,
			expectedMerge: "hello\n---\nworld",
		},
		{
			name:          "three parts",
			parts:         []string{"one", "two", "three"},
			expectedOk:    true,
			expectedMerge: "one\n---\ntwo\n---\nthree",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := MergeTexts(tt.parts, strategy)

			if ok != tt.expectedOk {
				t.Errorf("MergeTexts() ok = %v, want %v", ok, tt.expectedOk)
			}

			if ok && result.Merged != tt.expectedMerge {
				t.Errorf("MergeTexts() merged = %q, want %q", result.Merged, tt.expectedMerge)
			}
		})
	}
}

func TestMergeTexts_MaxLength(t *testing.T) {
	strategy := censor.TextMergeStrategy{
		Separator: "|",
		MaxLen:    10,
	}

	// "aaa|bbb|ccc" = 11 chars, exceeds max
	parts := []string{"aaa", "bbb", "ccc"}

	result, ok := MergeTexts(parts, strategy)

	if !ok {
		t.Fatal("MergeTexts() should succeed with partial merge")
	}

	// Should only include first two parts: "aaa|bbb" = 7 chars
	if result.Merged != "aaa|bbb" {
		t.Errorf("MergeTexts() merged = %q, want %q", result.Merged, "aaa|bbb")
	}

	if len(result.Parts) != 2 {
		t.Errorf("MergeTexts() included %d parts, want 2", len(result.Parts))
	}
}

func TestMergeTexts_CannotMerge(t *testing.T) {
	strategy := censor.TextMergeStrategy{
		Separator: "|",
		MaxLen:    5,
	}

	// Each part is too long to fit even two
	parts := []string{"aaaa", "bbbb"}

	_, ok := MergeTexts(parts, strategy)

	if ok {
		t.Error("MergeTexts() should fail when cannot fit at least 2 parts")
	}
}

func TestMergeTexts_IndexMapping(t *testing.T) {
	strategy := censor.TextMergeStrategy{
		Separator: "|",
		MaxLen:    100,
	}

	parts := []string{"hello", "world", "test"}

	result, ok := MergeTexts(parts, strategy)
	if !ok {
		t.Fatal("MergeTexts() failed")
	}

	// Verify index mapping
	// "hello|world|test"
	// hello: 0-5
	// world: 6-11
	// test: 12-16

	expected := []PartIndex{
		{Start: 0, End: 5},
		{Start: 6, End: 11},
		{Start: 12, End: 16},
	}

	for i, idx := range result.Index {
		if idx.Start != expected[i].Start || idx.End != expected[i].End {
			t.Errorf("Index[%d] = {%d, %d}, want {%d, %d}",
				i, idx.Start, idx.End, expected[i].Start, expected[i].End)
		}
	}
}

func TestSplitMergedText(t *testing.T) {
	merged := MergedText{
		Merged: "hello|world|test",
		Parts:  []string{"hello", "world", "test"},
		Index: []PartIndex{
			{Start: 0, End: 5},
			{Start: 6, End: 11},
			{Start: 12, End: 16},
		},
	}

	result := SplitMergedText(merged)

	if len(result) != 3 {
		t.Fatalf("SplitMergedText() returned %d parts, want 3", len(result))
	}

	expected := []string{"hello", "world", "test"}
	for i, part := range result {
		if part != expected[i] {
			t.Errorf("SplitMergedText()[%d] = %q, want %q", i, part, expected[i])
		}
	}
}

func TestFindViolatingParts(t *testing.T) {
	merged := MergedText{
		Merged: "hello|world|test",
		Index: []PartIndex{
			{Start: 0, End: 5},   // hello
			{Start: 6, End: 11},  // world
			{Start: 12, End: 16}, // test
		},
	}

	tests := []struct {
		name           string
		violationStart int
		violationEnd   int
		expected       []int
	}{
		{
			name:           "violation in first part",
			violationStart: 1,
			violationEnd:   3,
			expected:       []int{0},
		},
		{
			name:           "violation in second part",
			violationStart: 7,
			violationEnd:   10,
			expected:       []int{1},
		},
		{
			name:           "violation spanning two parts",
			violationStart: 4,
			violationEnd:   8,
			expected:       []int{0, 1},
		},
		{
			name:           "violation in separator (no parts)",
			violationStart: 5,
			violationEnd:   6,
			expected:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FindViolatingParts(merged, tt.violationStart, tt.violationEnd)

			if len(result) != len(tt.expected) {
				t.Errorf("FindViolatingParts() returned %d parts, want %d", len(result), len(tt.expected))
				return
			}

			for i, part := range result {
				if part != tt.expected[i] {
					t.Errorf("FindViolatingParts()[%d] = %d, want %d", i, part, tt.expected[i])
				}
			}
		})
	}
}

func TestTruncateText(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		maxLen   int
		expected string
	}{
		{
			name:     "shorter than max",
			text:     "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "equal to max",
			text:     "hello",
			maxLen:   5,
			expected: "hello",
		},
		{
			name:     "longer than max",
			text:     "hello world",
			maxLen:   8,
			expected: "hello...",
		},
		{
			name:     "very short max",
			text:     "hello",
			maxLen:   3,
			expected: "hel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateText(tt.text, tt.maxLen)
			if result != tt.expected {
				t.Errorf("TruncateText(%q, %d) = %q, want %q", tt.text, tt.maxLen, result, tt.expected)
			}
		})
	}
}

func TestMaskText(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		start    int
		end      int
		maskChar rune
		expected string
	}{
		{
			name:     "mask middle",
			text:     "hello",
			start:    1,
			end:      4,
			maskChar: '*',
			expected: "h***o",
		},
		{
			name:     "mask all",
			text:     "hello",
			start:    0,
			end:      5,
			maskChar: '*',
			expected: "*****",
		},
		{
			name:     "negative start",
			text:     "hello",
			start:    -1,
			end:      2,
			maskChar: '*',
			expected: "**llo",
		},
		{
			name:     "end beyond length",
			text:     "hello",
			start:    3,
			end:      10,
			maskChar: '*',
			expected: "hel**",
		},
		{
			name:     "start >= end",
			text:     "hello",
			start:    3,
			end:      2,
			maskChar: '*',
			expected: "hello",
		},
		{
			name:     "unicode text",
			text:     "你好世界",
			start:    1,
			end:      3,
			maskChar: '*',
			expected: "你**界",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskText(tt.text, tt.start, tt.end, tt.maskChar)
			if result != tt.expected {
				t.Errorf("MaskText(%q, %d, %d, %q) = %q, want %q",
					tt.text, tt.start, tt.end, string(tt.maskChar), result, tt.expected)
			}
		})
	}
}
