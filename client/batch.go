package client

import (
	"context"
	"strings"

	censor "github.com/heibot/censor"
	"github.com/heibot/censor/violation"
)

// BatchItem represents a single item in a batch submission.
type BatchItem struct {
	BizID       string      // Unique ID for this item (e.g., message_id, danmaku_id)
	SubmitterID string      // Who submitted this item (e.g., user_id)
	Text        string      // Content to review
	OnBlock     BlockAction // What to do if blocked
	Extra       map[string]string // Additional metadata
}

// BatchItemResult represents the review result for a single batch item.
type BatchItemResult struct {
	BizID       string          // Item ID
	Decision    censor.Decision // pass/review/block/error
	Reasons     []censor.Reason // Reasons for the decision
	Blocked     bool            // Convenience: true if Decision is Block or Review
	LocatedBy   string          // How the violation was located
	Confidence  float64         // Location confidence
}

// SubmitBatchInput is the input for batch submission.
type SubmitBatchInput struct {
	BizType     censor.BizType           // Business type (e.g., danmaku, comment)
	Items       []BatchItem              // Items to review
	Scenes      []violation.UnifiedScene // Detection scenes (optional)
	TraceID     string                   // Trace ID for debugging

	// FallbackThreshold controls when to use fallback.
	// If confidence < threshold when locating, use fallback.
	// Default: 0.8
	FallbackThreshold float64

	// DisableFallback disables fallback to individual item review.
	// If true, uses conservative approach (mark all items as blocked).
	DisableFallback bool

	// MaxMergeCount limits items per merged request.
	// If Items > MaxMergeCount, splits into multiple batches.
	// Default: 10
	MaxMergeCount int
}

// SubmitBatchResult is the result of batch submission.
type SubmitBatchResult struct {
	// Results maps BizID to its result.
	Results map[string]*BatchItemResult

	// OverallDecision is the strictest decision across all items.
	OverallDecision censor.Decision

	// BlockedCount is the number of blocked items.
	BlockedCount int

	// PassedCount is the number of passed items.
	PassedCount int

	// UsedFallback indicates if fallback (separate reviews) was used.
	UsedFallback bool

	// PendingAsync indicates if some items are pending async review.
	PendingAsync bool
}

// SubmitBatch submits multiple independent items for batch review.
// Uses the same hybrid strategy as SubmitFields:
// 1. Merge and submit as one request (cost-efficient)
// 2. If pass → all items pass
// 3. If block/review → try to locate which item(s) caused it
// 4. If location fails → fallback to separate reviews or conservative approach
func (c *Client) SubmitBatch(ctx context.Context, input SubmitBatchInput) (*SubmitBatchResult, error) {
	if len(input.Items) == 0 {
		return nil, censor.ErrNoResources
	}

	// Set defaults
	if input.FallbackThreshold == 0 {
		input.FallbackThreshold = 0.8
	}
	if input.MaxMergeCount == 0 {
		input.MaxMergeCount = 10
	}

	// Single item: no need for merge logic
	if len(input.Items) == 1 {
		return c.submitSingleBatchItem(ctx, input)
	}

	// If too many items, split into smaller batches
	if len(input.Items) > input.MaxMergeCount {
		return c.submitBatchInChunks(ctx, input)
	}

	// Multiple items: use hybrid strategy
	return c.submitBatchMerged(ctx, input)
}

// submitSingleBatchItem handles the simple case of a single item.
func (c *Client) submitSingleBatchItem(ctx context.Context, input SubmitBatchInput) (*SubmitBatchResult, error) {
	item := input.Items[0]

	biz := censor.BizContext{
		BizType:     input.BizType,
		BizID:       item.BizID,
		SubmitterID: item.SubmitterID,
		TraceID:     input.TraceID,
	}

	result, err := c.Submit(ctx, SubmitInput{
		Biz: biz,
		Resources: []censor.Resource{{
			ResourceID:  item.BizID,
			Type:        censor.ResourceText,
			ContentText: item.Text,
			Extra:       item.Extra,
		}},
		Scenes: input.Scenes,
	})
	if err != nil {
		return nil, err
	}

	itemResult := &BatchItemResult{
		BizID:      item.BizID,
		Confidence: 1.0,
	}

	if outcome, ok := result.ImmediateResults[item.BizID]; ok {
		itemResult.Decision = outcome.Decision
		itemResult.Reasons = outcome.Reasons
		itemResult.Blocked = outcome.Decision == censor.DecisionBlock || outcome.Decision == censor.DecisionReview
	} else {
		itemResult.Decision = censor.DecisionPending
	}

	batchResult := &SubmitBatchResult{
		Results:         map[string]*BatchItemResult{item.BizID: itemResult},
		OverallDecision: itemResult.Decision,
		PendingAsync:    result.PendingAsync,
	}
	if itemResult.Blocked {
		batchResult.BlockedCount = 1
	} else if itemResult.Decision == censor.DecisionPass {
		batchResult.PassedCount = 1
	}

	return batchResult, nil
}

// submitBatchInChunks splits large batches into smaller chunks.
func (c *Client) submitBatchInChunks(ctx context.Context, input SubmitBatchInput) (*SubmitBatchResult, error) {
	result := &SubmitBatchResult{
		Results:         make(map[string]*BatchItemResult),
		OverallDecision: censor.DecisionPass,
	}

	// Process in chunks
	for i := 0; i < len(input.Items); i += input.MaxMergeCount {
		end := i + input.MaxMergeCount
		if end > len(input.Items) {
			end = len(input.Items)
		}

		chunkInput := input
		chunkInput.Items = input.Items[i:end]

		chunkResult, err := c.submitBatchMerged(ctx, chunkInput)
		if err != nil {
			// On error, mark remaining items as error
			for j := i; j < len(input.Items); j++ {
				result.Results[input.Items[j].BizID] = &BatchItemResult{
					BizID:    input.Items[j].BizID,
					Decision: censor.DecisionError,
					Reasons:  []censor.Reason{{Code: "batch_error", Message: err.Error()}},
				}
			}
			result.OverallDecision = censor.DecisionError
			return result, nil
		}

		// Merge chunk results
		for bizID, itemResult := range chunkResult.Results {
			result.Results[bizID] = itemResult
			if itemResult.Blocked {
				result.BlockedCount++
			} else if itemResult.Decision == censor.DecisionPass {
				result.PassedCount++
			}
		}

		if chunkResult.UsedFallback {
			result.UsedFallback = true
		}
		if chunkResult.PendingAsync {
			result.PendingAsync = true
		}
		if decisionSeverity(chunkResult.OverallDecision) > decisionSeverity(result.OverallDecision) {
			result.OverallDecision = chunkResult.OverallDecision
		}
	}

	return result, nil
}

// submitBatchMerged merges items and submits as one request.
func (c *Client) submitBatchMerged(ctx context.Context, input SubmitBatchInput) (*SubmitBatchResult, error) {
	// Step 1: Merge texts
	merged, itemIndex := mergeBatchItems(input.Items)

	// Use first item's submitter for merged request
	biz := censor.BizContext{
		BizType:     input.BizType,
		BizID:       "_batch_",
		SubmitterID: input.Items[0].SubmitterID,
		TraceID:     input.TraceID,
	}

	// Step 2: Submit merged text
	submitResult, err := c.Submit(ctx, SubmitInput{
		Biz: biz,
		Resources: []censor.Resource{{
			ResourceID:  "_batch_",
			Type:        censor.ResourceText,
			ContentText: merged.Merged,
		}},
		Scenes: input.Scenes,
	})
	if err != nil {
		return nil, err
	}

	// Initialize result
	result := &SubmitBatchResult{
		Results:      make(map[string]*BatchItemResult),
		PendingAsync: submitResult.PendingAsync,
	}

	// Get merged outcome
	outcome, hasImmediate := submitResult.ImmediateResults["_batch_"]

	// Step 3: If pass, all items pass
	if hasImmediate && outcome.Decision == censor.DecisionPass {
		for _, item := range input.Items {
			result.Results[item.BizID] = &BatchItemResult{
				BizID:      item.BizID,
				Decision:   censor.DecisionPass,
				Confidence: 1.0,
			}
			result.PassedCount++
		}
		result.OverallDecision = censor.DecisionPass
		return result, nil
	}

	// Step 4: If pending async, mark all as pending
	if !hasImmediate {
		for _, item := range input.Items {
			result.Results[item.BizID] = &BatchItemResult{
				BizID:    item.BizID,
				Decision: censor.DecisionPending,
			}
		}
		result.OverallDecision = censor.DecisionPending
		return result, nil
	}

	// Step 5: Block/Review - try to locate which item(s)
	locatedItems, locateConfidence, locateMethod := locateBatchViolations(
		input.Items, itemIndex, outcome.Reasons,
	)

	// Step 6: High confidence location
	if locateConfidence >= input.FallbackThreshold && len(locatedItems) > 0 {
		return c.buildBatchResultFromLocation(input, locatedItems, outcome, locateMethod), nil
	}

	// Step 7: Fallback
	if input.DisableFallback {
		return c.buildConservativeBatchResult(input, outcome), nil
	}

	return c.submitBatchFallback(ctx, input, outcome)
}

// mergeBatchItems merges batch item texts with separators.
func mergeBatchItems(items []BatchItem) (*censor.MergedText, map[string]censor.PartIndex) {
	separator := "\n---\n"
	var parts []string
	var indices []censor.PartIndex
	itemIndex := make(map[string]censor.PartIndex)

	pos := 0
	for i, item := range items {
		if i > 0 {
			pos += len(separator)
		}

		start := pos
		end := start + len(item.Text)
		parts = append(parts, item.Text)
		idx := censor.PartIndex{Start: start, End: end}
		indices = append(indices, idx)
		itemIndex[item.BizID] = idx
		pos = end
	}

	merged := strings.Join(parts, separator)

	return &censor.MergedText{
		Merged: merged,
		Parts:  parts,
		Index:  indices,
	}, itemIndex
}

// locateBatchViolations tries to determine which item(s) caused the violation.
func locateBatchViolations(
	items []BatchItem,
	itemIndex map[string]censor.PartIndex,
	reasons []censor.Reason,
) ([]string, float64, string) {
	if len(reasons) == 0 {
		return nil, 0, ""
	}

	// Strategy 1: Position-based
	located, confidence := locateBatchByPosition(items, itemIndex, reasons)
	if confidence > 0 {
		return located, confidence, "position"
	}

	// Strategy 2: Keyword-based
	located, confidence = locateBatchByKeyword(items, reasons)
	if confidence > 0 {
		return located, confidence, "keyword"
	}

	return nil, 0, ""
}

// locateBatchByPosition uses position info to locate items.
func locateBatchByPosition(
	items []BatchItem,
	itemIndex map[string]censor.PartIndex,
	reasons []censor.Reason,
) ([]string, float64) {
	foundItems := make(map[string]bool)

	for _, reason := range reasons {
		if reason.Raw == nil {
			continue
		}

		var startPos, endPos int
		var hasPosition bool

		// Try different provider formats (same as fields.go)
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

		for _, item := range items {
			idx := itemIndex[item.BizID]
			if startPos >= idx.Start && endPos <= idx.End {
				foundItems[item.BizID] = true
			}
		}
	}

	if len(foundItems) > 0 {
		var result []string
		for bizID := range foundItems {
			result = append(result, bizID)
		}
		return result, 0.95
	}

	return nil, 0
}

// locateBatchByKeyword searches for violation keywords in each item.
func locateBatchByKeyword(items []BatchItem, reasons []censor.Reason) ([]string, float64) {
	foundItems := make(map[string]int)
	totalKeywords := 0

	for _, reason := range reasons {
		keywords := extractKeywords(reason)

		for _, keyword := range keywords {
			if keyword == "" {
				continue
			}
			totalKeywords++

			for _, item := range items {
				if strings.Contains(strings.ToLower(item.Text), strings.ToLower(keyword)) {
					foundItems[item.BizID]++
				}
			}
		}
	}

	if len(foundItems) > 0 && totalKeywords > 0 {
		var result []string
		maxMatches := 0
		for bizID, count := range foundItems {
			result = append(result, bizID)
			if count > maxMatches {
				maxMatches = count
			}
		}

		confidence := float64(maxMatches) / float64(totalKeywords)
		if confidence > 1 {
			confidence = 1
		}
		confidence *= 0.85

		return result, confidence
	}

	return nil, 0
}

// buildBatchResultFromLocation builds result when items are successfully located.
func (c *Client) buildBatchResultFromLocation(
	input SubmitBatchInput,
	locatedItems []string,
	outcome censor.FinalOutcome,
	locateMethod string,
) *SubmitBatchResult {
	locatedSet := make(map[string]bool)
	for _, bizID := range locatedItems {
		locatedSet[bizID] = true
	}

	result := &SubmitBatchResult{
		Results:         make(map[string]*BatchItemResult),
		OverallDecision: outcome.Decision,
	}

	for _, item := range input.Items {
		ir := &BatchItemResult{
			BizID:      item.BizID,
			LocatedBy:  locateMethod,
			Confidence: 1.0,
		}

		if locatedSet[item.BizID] {
			ir.Decision = outcome.Decision
			ir.Reasons = outcome.Reasons
			ir.Blocked = true
			result.BlockedCount++
		} else {
			ir.Decision = censor.DecisionPass
			result.PassedCount++
		}

		result.Results[item.BizID] = ir
	}

	return result
}

// buildConservativeBatchResult marks all items as blocked.
func (c *Client) buildConservativeBatchResult(
	input SubmitBatchInput,
	outcome censor.FinalOutcome,
) *SubmitBatchResult {
	result := &SubmitBatchResult{
		Results:         make(map[string]*BatchItemResult),
		OverallDecision: outcome.Decision,
		BlockedCount:    len(input.Items),
	}

	for _, item := range input.Items {
		result.Results[item.BizID] = &BatchItemResult{
			BizID:      item.BizID,
			Decision:   outcome.Decision,
			Reasons:    outcome.Reasons,
			Blocked:    true,
			LocatedBy:  "conservative",
			Confidence: 1.0,
		}
	}

	return result
}

// submitBatchFallback reviews each item separately.
func (c *Client) submitBatchFallback(
	ctx context.Context,
	input SubmitBatchInput,
	mergedOutcome censor.FinalOutcome,
) (*SubmitBatchResult, error) {
	result := &SubmitBatchResult{
		Results:         make(map[string]*BatchItemResult),
		OverallDecision: censor.DecisionPass,
		UsedFallback:    true,
	}

	for _, item := range input.Items {
		biz := censor.BizContext{
			BizType:     input.BizType,
			BizID:       item.BizID,
			SubmitterID: item.SubmitterID,
			TraceID:     input.TraceID,
		}

		submitResult, err := c.Submit(ctx, SubmitInput{
			Biz: biz,
			Resources: []censor.Resource{{
				ResourceID:  item.BizID,
				Type:        censor.ResourceText,
				ContentText: item.Text,
				Extra:       item.Extra,
			}},
			Scenes: input.Scenes,
		})

		ir := &BatchItemResult{
			BizID:     item.BizID,
			LocatedBy: "fallback",
		}

		if err != nil {
			ir.Decision = mergedOutcome.Decision
			ir.Reasons = mergedOutcome.Reasons
			ir.Blocked = true
			ir.LocatedBy = "fallback_error"
			result.BlockedCount++
		} else if itemOutcome, ok := submitResult.ImmediateResults[item.BizID]; ok {
			ir.Decision = itemOutcome.Decision
			ir.Reasons = itemOutcome.Reasons
			ir.Blocked = itemOutcome.Decision == censor.DecisionBlock || itemOutcome.Decision == censor.DecisionReview
			ir.Confidence = 1.0

			if ir.Blocked {
				result.BlockedCount++
			} else {
				result.PassedCount++
			}
		} else {
			ir.Decision = censor.DecisionPending
			result.PendingAsync = true
		}

		result.Results[item.BizID] = ir

		if decisionSeverity(ir.Decision) > decisionSeverity(result.OverallDecision) {
			result.OverallDecision = ir.Decision
		}
	}

	return result, nil
}
