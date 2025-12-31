package client

import (
	"context"
	"strings"

	censor "github.com/heibot/censor"
	"github.com/heibot/censor/utils"
	"github.com/heibot/censor/violation"
)

// BlockAction defines what to do when a field is blocked.
type BlockAction string

const (
	// ActionPassThrough lets the content through unchanged (for manual review later).
	ActionPassThrough BlockAction = "pass_through"

	// ActionReplace replaces the content with a specified value.
	ActionReplace BlockAction = "replace"

	// ActionHide hides the content entirely.
	ActionHide BlockAction = "hide"

	// ActionReject rejects the entire submission.
	ActionReject BlockAction = "reject"
)

// FieldInput represents a single field to be reviewed.
type FieldInput struct {
	Field       string      // Field name (e.g., "name", "desc", "title")
	Text        string      // Field content
	OnBlock     BlockAction // What to do if blocked
	ReplaceWith string      // Replacement value (for ActionReplace)
}

// FieldResult represents the review result for a single field.
type FieldResult struct {
	Field        string          // Field name
	Decision     censor.Decision // pass/review/block/error
	Reasons      []censor.Reason // Reasons for the decision
	FinalValue   string          // The final value (original or replaced)
	WasReplaced  bool            // Whether the value was replaced
	Confidence   float64         // Confidence score
	LocatedBy    string          // How the violation was located: "position", "keyword", "fallback"
}

// SubmitFieldsInput is the input for submitting multiple fields.
type SubmitFieldsInput struct {
	BizType     censor.BizType           // Business type
	BizID       string                   // Business ID
	SubmitterID string                   // Who submitted
	TraceID     string                   // Trace ID
	Fields      []FieldInput             // Fields to review
	Scenes      []violation.UnifiedScene // Detection scenes (optional)

	// FallbackThreshold controls when to use fallback.
	// If confidence < threshold when locating, use fallback.
	// Default: 0.8
	FallbackThreshold float64

	// DisableFallback disables fallback to individual field review.
	// If true, uses conservative approach (mark all fields as blocked).
	DisableFallback bool
}

// SubmitFieldsResult is the result of submitting multiple fields.
type SubmitFieldsResult struct {
	// BizReviewID is the business review ID.
	BizReviewID string

	// FieldResults maps field name to its result.
	FieldResults map[string]*FieldResult

	// OverallDecision is the strictest decision across all fields.
	OverallDecision censor.Decision

	// UsedFallback indicates if fallback (separate reviews) was used.
	UsedFallback bool

	// PendingAsync indicates if some fields are pending async review.
	PendingAsync bool
}

// SubmitFields submits multiple fields for review using the hybrid strategy:
// 1. Merge and submit as one request (cost-efficient)
// 2. If pass → all fields pass
// 3. If block/review → try to locate which field(s) caused it
// 4. If location fails → fallback to separate reviews
func (c *Client) SubmitFields(ctx context.Context, input SubmitFieldsInput) (*SubmitFieldsResult, error) {
	if len(input.Fields) == 0 {
		return nil, censor.ErrNoResources
	}

	// Set default threshold
	if input.FallbackThreshold == 0 {
		input.FallbackThreshold = 0.8
	}

	// Single field: no need for merge logic
	if len(input.Fields) == 1 {
		return c.submitSingleField(ctx, input)
	}

	// Multiple fields: use hybrid strategy
	return c.submitMultipleFields(ctx, input)
}

// submitSingleField handles the simple case of a single field.
func (c *Client) submitSingleField(ctx context.Context, input SubmitFieldsInput) (*SubmitFieldsResult, error) {
	field := input.Fields[0]

	// Create biz context
	biz := censor.BizContext{
		BizType:     input.BizType,
		BizID:       input.BizID,
		Field:       field.Field,
		SubmitterID: input.SubmitterID,
		TraceID:     input.TraceID,
	}

	// Submit
	result, err := c.Submit(ctx, SubmitInput{
		Biz: biz,
		Resources: []censor.Resource{{
			ResourceID:  field.Field,
			Type:        censor.ResourceText,
			ContentText: field.Text,
		}},
		Scenes: input.Scenes,
	})
	if err != nil {
		return nil, err
	}

	// Build field result
	fieldResult := &FieldResult{
		Field:      field.Field,
		FinalValue: field.Text,
	}

	if outcome, ok := result.ImmediateResults[field.Field]; ok {
		fieldResult.Decision = outcome.Decision
		fieldResult.Reasons = outcome.Reasons
		fieldResult.Confidence = 1.0

		// Apply block action
		if outcome.Decision == censor.DecisionBlock || outcome.Decision == censor.DecisionReview {
			fieldResult.FinalValue, fieldResult.WasReplaced = applyBlockAction(field, outcome.Decision)
		}
	} else {
		fieldResult.Decision = censor.DecisionPending
	}

	return &SubmitFieldsResult{
		BizReviewID: result.BizReviewID,
		FieldResults: map[string]*FieldResult{
			field.Field: fieldResult,
		},
		OverallDecision: fieldResult.Decision,
		PendingAsync:    result.PendingAsync,
	}, nil
}

// submitMultipleFields handles multiple fields with hybrid strategy.
func (c *Client) submitMultipleFields(ctx context.Context, input SubmitFieldsInput) (*SubmitFieldsResult, error) {
	// Step 1: Merge texts
	merged, fieldIndex := mergeFieldTexts(input.Fields)

	// Create biz context for merged review
	biz := censor.BizContext{
		BizType:     input.BizType,
		BizID:       input.BizID,
		Field:       "_merged_", // Special marker
		SubmitterID: input.SubmitterID,
		TraceID:     input.TraceID,
	}

	// Step 2: Submit merged text
	result, err := c.Submit(ctx, SubmitInput{
		Biz: biz,
		Resources: []censor.Resource{{
			ResourceID:  "_merged_",
			Type:        censor.ResourceText,
			ContentText: merged.Merged,
		}},
		Scenes: input.Scenes,
	})
	if err != nil {
		return nil, err
	}

	// Initialize result
	fieldsResult := &SubmitFieldsResult{
		BizReviewID:  result.BizReviewID,
		FieldResults: make(map[string]*FieldResult),
		PendingAsync: result.PendingAsync,
	}

	// Get merged outcome
	outcome, hasImmediate := result.ImmediateResults["_merged_"]

	// Step 3: If pass, all fields pass
	if hasImmediate && outcome.Decision == censor.DecisionPass {
		for _, field := range input.Fields {
			fieldsResult.FieldResults[field.Field] = &FieldResult{
				Field:      field.Field,
				Decision:   censor.DecisionPass,
				FinalValue: field.Text,
				Confidence: 1.0,
			}
		}
		fieldsResult.OverallDecision = censor.DecisionPass
		return fieldsResult, nil
	}

	// Step 4: If pending async, mark all as pending
	if !hasImmediate {
		for _, field := range input.Fields {
			fieldsResult.FieldResults[field.Field] = &FieldResult{
				Field:      field.Field,
				Decision:   censor.DecisionPending,
				FinalValue: field.Text,
			}
		}
		fieldsResult.OverallDecision = censor.DecisionPending
		return fieldsResult, nil
	}

	// Step 5: Block/Review - try to locate which field(s)
	locatedFields, locateConfidence, locateMethod := locateViolationFields(
		input.Fields, fieldIndex, outcome.Reasons,
	)

	// Step 6: High confidence location
	if locateConfidence >= input.FallbackThreshold && len(locatedFields) > 0 {
		return c.buildResultFromLocation(input, locatedFields, outcome, locateMethod), nil
	}

	// Step 7: Fallback - separate reviews or conservative approach
	if input.DisableFallback {
		// Conservative: mark all fields as blocked
		return c.buildConservativeResult(input, outcome), nil
	}

	// Fallback: review each field separately
	return c.submitFieldsFallback(ctx, input, outcome)
}

// mergeFieldTexts merges field texts with separators and tracks positions.
func mergeFieldTexts(fields []FieldInput) (*censor.MergedText, map[string]censor.PartIndex) {
	separator := "\n---\n"
	var parts []string
	var indices []censor.PartIndex
	fieldIndex := make(map[string]censor.PartIndex)

	pos := 0
	for i, field := range fields {
		if i > 0 {
			pos += len(separator)
		}

		start := pos
		end := start + len(field.Text)
		parts = append(parts, field.Text)
		idx := censor.PartIndex{Start: start, End: end}
		indices = append(indices, idx)
		fieldIndex[field.Field] = idx
		pos = end
	}

	merged := strings.Join(parts, separator)

	return &censor.MergedText{
		Merged: merged,
		Parts:  parts,
		Index:  indices,
	}, fieldIndex
}

// locateViolationFields tries to determine which field(s) caused the violation.
// Returns: located field names, confidence, method used.
func locateViolationFields(
	fields []FieldInput,
	fieldIndex map[string]censor.PartIndex,
	reasons []censor.Reason,
) ([]string, float64, string) {
	if len(reasons) == 0 {
		return nil, 0, ""
	}

	// Strategy 1: Position-based (if provider returns positions)
	located, confidence := locateByPosition(fields, fieldIndex, reasons)
	if confidence > 0 {
		return located, confidence, "position"
	}

	// Strategy 2: Keyword-based (search for hit keywords in each field)
	located, confidence = locateByKeyword(fields, reasons)
	if confidence > 0 {
		return located, confidence, "keyword"
	}

	return nil, 0, ""
}

// locateByPosition uses position info from reasons to locate fields.
func locateByPosition(
	fields []FieldInput,
	fieldIndex map[string]censor.PartIndex,
	reasons []censor.Reason,
) ([]string, float64) {
	foundFields := make(map[string]bool)

	for _, reason := range reasons {
		// Check if reason.Raw contains position info
		if reason.Raw == nil {
			continue
		}

		// Try to extract position (different providers have different formats)
		var startPos, endPos int
		var hasPosition bool

		// Aliyun format
		if positions, ok := reason.Raw["positions"].([]any); ok && len(positions) > 0 {
			if posMap, ok := positions[0].(map[string]any); ok {
				if start, ok := posMap["startPos"].(float64); ok {
					startPos = int(start)
					if end, ok := posMap["endPos"].(float64); ok {
						endPos = int(end)
						hasPosition = true
					}
				}
			}
		}

		// Huawei/Tencent format
		if start, ok := reason.Raw["start_position"].(float64); ok {
			startPos = int(start)
			if end, ok := reason.Raw["end_position"].(float64); ok {
				endPos = int(end)
				hasPosition = true
			}
		}

		if !hasPosition {
			continue
		}

		// Find which field contains this position
		for _, field := range fields {
			idx := fieldIndex[field.Field]
			if startPos >= idx.Start && endPos <= idx.End {
				foundFields[field.Field] = true
			}
		}
	}

	if len(foundFields) > 0 {
		var result []string
		for field := range foundFields {
			result = append(result, field)
		}
		// High confidence if we found positions
		return result, 0.95
	}

	return nil, 0
}

// locateByKeyword searches for violation keywords in each field.
func locateByKeyword(fields []FieldInput, reasons []censor.Reason) ([]string, float64) {
	foundFields := make(map[string]int) // field -> match count
	totalKeywords := 0

	for _, reason := range reasons {
		// Extract keywords from HitTags or Raw
		keywords := extractKeywords(reason)

		for _, keyword := range keywords {
			if keyword == "" {
				continue
			}
			totalKeywords++

			// Search in each field
			for _, field := range fields {
				if strings.Contains(strings.ToLower(field.Text), strings.ToLower(keyword)) {
					foundFields[field.Field]++
				}
			}
		}
	}

	if len(foundFields) > 0 && totalKeywords > 0 {
		var result []string
		maxMatches := 0
		for field, count := range foundFields {
			result = append(result, field)
			if count > maxMatches {
				maxMatches = count
			}
		}

		// Calculate confidence based on match ratio
		confidence := float64(maxMatches) / float64(totalKeywords)
		if confidence > 1 {
			confidence = 1
		}
		// Keyword matching is less reliable than position
		confidence *= 0.85

		return result, confidence
	}

	return nil, 0
}

// extractKeywords extracts violation keywords from a reason.
func extractKeywords(reason censor.Reason) []string {
	var keywords []string

	// From HitTags
	keywords = append(keywords, reason.HitTags...)

	// From Raw - common formats
	if reason.Raw != nil {
		// Aliyun: keywords, keywordTexts
		if kws, ok := reason.Raw["keywords"].([]any); ok {
			for _, kw := range kws {
				if s, ok := kw.(string); ok {
					keywords = append(keywords, s)
				}
			}
		}
		if kws, ok := reason.Raw["keywordTexts"].([]any); ok {
			for _, kw := range kws {
				if s, ok := kw.(string); ok {
					keywords = append(keywords, s)
				}
			}
		}
		// Huawei: segments, keyword
		if segs, ok := reason.Raw["segments"].([]any); ok {
			for _, seg := range segs {
				if segMap, ok := seg.(map[string]any); ok {
					if kw, ok := segMap["segment"].(string); ok {
						keywords = append(keywords, kw)
					}
				}
			}
		}
		// Tencent: Keywords
		if kws, ok := reason.Raw["Keywords"].([]any); ok {
			for _, kw := range kws {
				if s, ok := kw.(string); ok {
					keywords = append(keywords, s)
				}
			}
		}
	}

	return keywords
}

// buildResultFromLocation builds result when fields are successfully located.
func (c *Client) buildResultFromLocation(
	input SubmitFieldsInput,
	locatedFields []string,
	outcome censor.FinalOutcome,
	locateMethod string,
) *SubmitFieldsResult {
	locatedSet := make(map[string]bool)
	for _, f := range locatedFields {
		locatedSet[f] = true
	}

	result := &SubmitFieldsResult{
		FieldResults:    make(map[string]*FieldResult),
		OverallDecision: outcome.Decision,
	}

	for _, field := range input.Fields {
		fr := &FieldResult{
			Field:      field.Field,
			FinalValue: field.Text,
			LocatedBy:  locateMethod,
		}

		if locatedSet[field.Field] {
			// This field caused the violation
			fr.Decision = outcome.Decision
			fr.Reasons = outcome.Reasons
			fr.Confidence = 1.0
			fr.FinalValue, fr.WasReplaced = applyBlockAction(field, outcome.Decision)
		} else {
			// This field is clean
			fr.Decision = censor.DecisionPass
			fr.Confidence = 1.0
		}

		result.FieldResults[field.Field] = fr
	}

	return result
}

// buildConservativeResult marks all fields as blocked (conservative approach).
func (c *Client) buildConservativeResult(
	input SubmitFieldsInput,
	outcome censor.FinalOutcome,
) *SubmitFieldsResult {
	result := &SubmitFieldsResult{
		FieldResults:    make(map[string]*FieldResult),
		OverallDecision: outcome.Decision,
	}

	for _, field := range input.Fields {
		fr := &FieldResult{
			Field:      field.Field,
			Decision:   outcome.Decision,
			Reasons:    outcome.Reasons,
			Confidence: 1.0,
			LocatedBy:  "conservative",
		}
		fr.FinalValue, fr.WasReplaced = applyBlockAction(field, outcome.Decision)
		result.FieldResults[field.Field] = fr
	}

	return result
}

// submitFieldsFallback reviews each field separately as a fallback.
func (c *Client) submitFieldsFallback(
	ctx context.Context,
	input SubmitFieldsInput,
	mergedOutcome censor.FinalOutcome,
) (*SubmitFieldsResult, error) {
	result := &SubmitFieldsResult{
		FieldResults:    make(map[string]*FieldResult),
		OverallDecision: censor.DecisionPass,
		UsedFallback:    true,
	}

	// Submit each field separately
	for _, field := range input.Fields {
		biz := censor.BizContext{
			BizType:     input.BizType,
			BizID:       input.BizID,
			Field:       field.Field,
			SubmitterID: input.SubmitterID,
			TraceID:     input.TraceID,
		}

		submitResult, err := c.Submit(ctx, SubmitInput{
			Biz: biz,
			Resources: []censor.Resource{{
				ResourceID:  field.Field,
				Type:        censor.ResourceText,
				ContentText: field.Text,
			}},
			Scenes: input.Scenes,
		})
		if err != nil {
			// On error, use merged outcome for this field
			fr := &FieldResult{
				Field:      field.Field,
				Decision:   mergedOutcome.Decision,
				Reasons:    mergedOutcome.Reasons,
				Confidence: 1.0,
				LocatedBy:  "fallback_error",
			}
			fr.FinalValue, fr.WasReplaced = applyBlockAction(field, mergedOutcome.Decision)
			result.FieldResults[field.Field] = fr
			continue
		}

		// Update BizReviewID from first successful submission
		if result.BizReviewID == "" {
			result.BizReviewID = submitResult.BizReviewID
		}

		fr := &FieldResult{
			Field:      field.Field,
			FinalValue: field.Text,
			LocatedBy:  "fallback",
		}

		if fieldOutcome, ok := submitResult.ImmediateResults[field.Field]; ok {
			fr.Decision = fieldOutcome.Decision
			fr.Reasons = fieldOutcome.Reasons
			fr.Confidence = 1.0

			if fieldOutcome.Decision == censor.DecisionBlock || fieldOutcome.Decision == censor.DecisionReview {
				fr.FinalValue, fr.WasReplaced = applyBlockAction(field, fieldOutcome.Decision)
			}
		} else {
			fr.Decision = censor.DecisionPending
			result.PendingAsync = true
		}

		result.FieldResults[field.Field] = fr

		// Update overall decision (strictest)
		if decisionSeverity(fr.Decision) > decisionSeverity(result.OverallDecision) {
			result.OverallDecision = fr.Decision
		}
	}

	return result, nil
}

// applyBlockAction applies the block action and returns the final value.
func applyBlockAction(field FieldInput, decision censor.Decision) (string, bool) {
	switch field.OnBlock {
	case ActionReplace:
		if field.ReplaceWith != "" {
			return field.ReplaceWith, true
		}
		return "***", true
	case ActionHide:
		return "", true
	case ActionPassThrough, ActionReject:
		return field.Text, false
	default:
		return field.Text, false
	}
}

// Ensure we have access to utils
var _ = utils.HashText
