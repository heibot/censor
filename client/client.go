package client

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	censor "github.com/heibot/censor"
	"github.com/heibot/censor/hooks"
	"github.com/heibot/censor/providers"
	"github.com/heibot/censor/store"
	"github.com/heibot/censor/utils"
	"github.com/heibot/censor/violation"
)

// Client is the main censor client.
type Client struct {
	store    store.Store
	hooks    hooks.Hooks
	pipeline *pipelineExecutor
	opts     Options
}

// New creates a new censor client.
func New(opts Options) (*Client, error) {
	if opts.Store == nil {
		return nil, censor.ErrStoreNotConfigured
	}

	if opts.Hooks == nil {
		opts.Hooks = hooks.NopHooks{}
	}

	pe := newPipelineExecutor(opts.Providers, opts.Pipeline)

	return &Client{
		store:    opts.Store,
		hooks:    opts.Hooks,
		pipeline: pe,
		opts:     opts,
	}, nil
}

// Submit submits content for review.
func (c *Client) Submit(ctx context.Context, input SubmitInput) (*SubmitResult, error) {
	if len(input.Resources) == 0 {
		return nil, censor.ErrNoResources
	}

	// Get required scenes from input or BizType defaults
	scenes := input.Scenes
	if len(scenes) == 0 {
		scenes = c.getScenesForBiz(input.Biz.BizType)
	}

	// Create biz review record
	bizReviewID, err := c.store.CreateBizReview(ctx, input.Biz)
	if err != nil {
		return nil, fmt.Errorf("failed to create biz review: %w", err)
	}

	// Update status to running
	if err := c.store.UpdateBizStatus(ctx, bizReviewID, censor.StatusRunning); err != nil {
		return nil, fmt.Errorf("failed to update biz status: %w", err)
	}

	result := &SubmitResult{
		BizReviewID:       bizReviewID,
		ResourceReviewIDs: make(map[string]string),
		ImmediateResults:  make(map[string]censor.FinalOutcome),
	}

	// Handle text merging if enabled
	resources := input.Resources
	if input.EnableTextMerge {
		resources = c.mergeTextResources(input.Resources)
	}

	// Process each resource
	for _, resource := range resources {
		// Compute hash if not set
		if resource.ContentHash == "" {
			resource.ContentHash = c.computeHash(resource)
		}

		// Check for deduplication
		if c.opts.EnableDedup {
			if existing := c.checkDedup(ctx, input.Biz, resource); existing != nil {
				result.ResourceReviewIDs[resource.ResourceID] = existing.ID
				result.ImmediateResults[resource.ResourceID] = c.parseOutcome(existing.OutcomeJSON)
				continue
			}
		}

		// Create resource review record
		resourceReviewID, err := c.store.CreateResourceReview(ctx, bizReviewID, resource)
		if err != nil {
			return nil, fmt.Errorf("failed to create resource review: %w", err)
		}
		result.ResourceReviewIDs[resource.ResourceID] = resourceReviewID

		// Execute pipeline with scenes
		pipelineResult, err := c.pipeline.execute(ctx, providers.SubmitRequest{
			Resource: resource,
			Biz:      input.Biz,
			Scenes:   scenes,
		})
		if err != nil {
			// Record error but continue
			c.recordError(ctx, resourceReviewID, err)
			continue
		}

		// Create provider task records
		if err := c.createProviderTasks(ctx, resourceReviewID, pipelineResult); err != nil {
			return nil, fmt.Errorf("failed to create provider tasks: %w", err)
		}

		// Handle immediate results
		if pipelineResult.isComplete() {
			outcome := *pipelineResult.finalOutcome
			result.ImmediateResults[resource.ResourceID] = outcome

			// Update resource review
			if err := c.store.UpdateResourceOutcome(ctx, resourceReviewID, outcome); err != nil {
				return nil, fmt.Errorf("failed to update resource outcome: %w", err)
			}

			// Handle violations
			if outcome.Decision == censor.DecisionBlock || outcome.Decision == censor.DecisionReview {
				snapshotID, err := c.handleViolation(ctx, input.Biz, resource, outcome)
				if err != nil {
					// Log but don't fail
				}

				// Fire violation detected hook
				c.fireViolationDetectedHook(ctx, input.Biz, resource, outcome, snapshotID)
			}

			// Fire hooks
			c.fireResourceReviewedHook(ctx, input.Biz, resource, pipelineResult, resourceReviewID, bizReviewID)
		} else {
			result.PendingAsync = true
		}
	}

	// Aggregate biz decision
	if err := c.aggregateBizDecision(ctx, bizReviewID, input.Biz); err != nil {
		return nil, fmt.Errorf("failed to aggregate biz decision: %w", err)
	}

	return result, nil
}

// Query queries the status of a review.
func (c *Client) Query(ctx context.Context, input QueryInput) (*QueryResult, error) {
	bizReview, err := c.store.GetBizReview(ctx, input.BizReviewID)
	if err != nil {
		return nil, err
	}

	resourceReviews, err := c.store.ListResourceReviewsByBizReview(ctx, input.BizReviewID)
	if err != nil {
		return nil, err
	}

	allComplete := true
	for _, rr := range resourceReviews {
		if rr.Decision == censor.DecisionPending {
			allComplete = false
			break
		}
	}

	result := &QueryResult{
		BizReview:       bizReview,
		ResourceReviews: resourceReviews,
		AllComplete:     allComplete,
	}

	if allComplete && len(resourceReviews) > 0 {
		// Parse final outcome from first resource (or aggregate)
		if resourceReviews[0].OutcomeJSON != "" {
			var outcome censor.FinalOutcome
			if err := json.Unmarshal([]byte(resourceReviews[0].OutcomeJSON), &outcome); err == nil {
				result.FinalOutcome = &outcome
			}
		}
	}

	return result, nil
}

// HandleCallback handles a provider callback.
func (c *Client) HandleCallback(ctx context.Context, providerName string, headers map[string]string, body []byte) error {
	provider, ok := c.pipeline.providers[providerName]
	if !ok {
		return censor.ErrProviderNotFound
	}

	// Verify callback
	if err := provider.VerifyCallback(ctx, headers, body); err != nil {
		return err
	}

	// Parse callback
	callbackData, err := provider.ParseCallback(ctx, body)
	if err != nil {
		return err
	}

	// Find provider task
	task, err := c.store.GetProviderTaskByRemoteID(ctx, providerName, callbackData.TaskID)
	if err != nil {
		return err
	}

	// Update provider task
	if err := c.store.UpdateProviderTaskResult(ctx, task.ID, callbackData.Done, callbackData.Result, callbackData.Raw); err != nil {
		return err
	}

	if callbackData.Done {
		// Process completion
		if err := c.processAsyncCompletion(ctx, task, callbackData.Result); err != nil {
			return err
		}
	}

	return nil
}

// GetBinding gets the current binding state for a business field.
func (c *Client) GetBinding(ctx context.Context, bizType, bizID, field string) (*censor.CensorBinding, error) {
	return c.store.GetBinding(ctx, bizType, bizID, field)
}

// GetBindings gets all bindings for a business object.
func (c *Client) GetBindings(ctx context.Context, bizType, bizID string) ([]censor.CensorBinding, error) {
	return c.store.ListBindingsByBiz(ctx, bizType, bizID)
}

// GetBindingHistory gets the history of binding changes.
func (c *Client) GetBindingHistory(ctx context.Context, bizType, bizID, field string, limit int) ([]censor.CensorBindingHistory, error) {
	return c.store.ListBindingHistory(ctx, bizType, bizID, field, limit)
}

// mergeTextResources merges text resources for efficient review.
func (c *Client) mergeTextResources(resources []censor.Resource) []censor.Resource {
	var textResources []censor.Resource
	var otherResources []censor.Resource

	for _, r := range resources {
		if r.Type == censor.ResourceText {
			textResources = append(textResources, r)
		} else {
			otherResources = append(otherResources, r)
		}
	}

	if len(textResources) <= 1 {
		return resources
	}

	// Try to merge texts
	var texts []string
	for _, r := range textResources {
		texts = append(texts, r.ContentText)
	}

	merged, ok := utils.MergeTexts(texts, c.opts.TextMerge)
	if !ok {
		return resources
	}

	// Create merged resource
	mergedResource := censor.Resource{
		ResourceID:  textResources[0].ResourceID + "_merged",
		Type:        censor.ResourceText,
		ContentText: merged.Merged,
		ContentHash: utils.HashText(merged.Merged),
		Extra: map[string]string{
			"merged": "true",
			"count":  fmt.Sprintf("%d", len(textResources)),
		},
	}

	return append([]censor.Resource{mergedResource}, otherResources...)
}

// computeHash computes the content hash for a resource.
func (c *Client) computeHash(r censor.Resource) string {
	switch r.Type {
	case censor.ResourceText:
		return utils.HashText(r.ContentText)
	default:
		return utils.HashURL(r.ContentURL)
	}
}

// checkDedup checks for duplicate content.
func (c *Client) checkDedup(ctx context.Context, biz censor.BizContext, r censor.Resource) *censor.ResourceReview {
	// Check if we have a recent review with the same hash
	binding, err := c.store.GetBinding(ctx, string(biz.BizType), biz.BizID, biz.Field)
	if err != nil || binding == nil {
		return nil
	}

	if binding.ContentHash == r.ContentHash && binding.Decision != string(censor.DecisionPending) {
		// Return a synthetic resource review
		return &censor.ResourceReview{
			ID:          binding.ReviewID,
			Decision:    censor.Decision(binding.Decision),
			OutcomeJSON: fmt.Sprintf(`{"decision":"%s","replace_policy":"%s","replace_value":"%s"}`, binding.Decision, binding.ReplacePolicy, binding.ReplaceValue),
		}
	}

	return nil
}

// createProviderTasks creates provider task records.
func (c *Client) createProviderTasks(ctx context.Context, resourceReviewID string, pr *pipelineResult) error {
	// Create primary task
	_, err := c.store.CreateProviderTask(ctx, resourceReviewID, c.opts.Pipeline.Primary, string(pr.mode), pr.primaryTaskID, nil)
	if err != nil {
		return err
	}

	// Create secondary task if exists
	if pr.secondaryTaskID != "" {
		_, err = c.store.CreateProviderTask(ctx, resourceReviewID, c.opts.Pipeline.Secondary, string(pr.mode), pr.secondaryTaskID, nil)
		if err != nil {
			return err
		}
	}

	return nil
}

// handleViolation handles a violation detection and returns the snapshot ID.
func (c *Client) handleViolation(ctx context.Context, biz censor.BizContext, r censor.Resource, outcome censor.FinalOutcome) (string, error) {
	// Save violation snapshot
	snapshotID, err := c.store.SaveViolationSnapshot(ctx, biz, r, outcome)
	if err != nil {
		return "", err
	}

	// Update binding
	binding := censor.CensorBinding{
		BizType:        string(biz.BizType),
		BizID:          biz.BizID,
		Field:          biz.Field,
		ResourceID:     r.ResourceID,
		ResourceType:   string(r.Type),
		ContentHash:    r.ContentHash,
		Decision:       string(outcome.Decision),
		ReplacePolicy:  string(outcome.ReplacePolicy),
		ReplaceValue:   outcome.ReplaceValue,
		ViolationRefID: snapshotID,
		ReviewRevision: 1,
	}

	// Get existing binding to check for revision
	existing, _ := c.store.GetBinding(ctx, string(biz.BizType), biz.BizID, biz.Field)
	if existing != nil {
		binding.ReviewRevision = existing.ReviewRevision + 1

		// Create history if changed
		if existing.Decision != binding.Decision ||
			existing.ReplacePolicy != binding.ReplacePolicy ||
			existing.ViolationRefID != binding.ViolationRefID {

			reasonJSON, _ := json.Marshal(outcome.Reasons)
			history := censor.CensorBindingHistory{
				BizType:        binding.BizType,
				BizID:          binding.BizID,
				Field:          binding.Field,
				ResourceID:     binding.ResourceID,
				ResourceType:   binding.ResourceType,
				Decision:       binding.Decision,
				ReplacePolicy:  binding.ReplacePolicy,
				ReplaceValue:   binding.ReplaceValue,
				ViolationRefID: binding.ViolationRefID,
				ReviewRevision: binding.ReviewRevision,
				ReasonJSON:     string(reasonJSON),
				Source:         string(censor.SourceAuto),
			}
			if err := c.store.CreateBindingHistory(ctx, history); err != nil {
				// Log but don't fail
				_ = err
			}
		}
	}

	if err := c.store.UpsertBinding(ctx, binding); err != nil {
		return snapshotID, err
	}

	return snapshotID, nil
}

// aggregateBizDecision aggregates the decision for a biz review.
func (c *Client) aggregateBizDecision(ctx context.Context, bizReviewID string, biz censor.BizContext) error {
	reviews, err := c.store.ListResourceReviewsByBizReview(ctx, bizReviewID)
	if err != nil {
		return err
	}

	// Aggregate: take strictest decision
	finalDecision := censor.DecisionPass
	allComplete := true

	for _, rr := range reviews {
		if rr.Decision == censor.DecisionPending {
			allComplete = false
			continue
		}
		if decisionSeverity(rr.Decision) > decisionSeverity(finalDecision) {
			finalDecision = rr.Decision
		}
	}

	if !allComplete {
		finalDecision = censor.DecisionPending
	}

	// Update biz review
	changed, err := c.store.UpdateBizDecision(ctx, bizReviewID, finalDecision)
	if err != nil {
		return err
	}

	if allComplete {
		if err := c.store.UpdateBizStatus(ctx, bizReviewID, censor.StatusDone); err != nil {
			return err
		}
	}

	// Fire hook if decision changed
	if changed {
		c.fireBizDecisionChangedHook(ctx, biz, reviews, bizReviewID, finalDecision)
	}

	return nil
}

// processAsyncCompletion processes the completion of an async task.
func (c *Client) processAsyncCompletion(ctx context.Context, task *censor.ProviderTask, result *censor.ReviewResult) error {
	// Get resource review
	resourceReview, err := c.store.GetResourceReview(ctx, task.ResourceReviewID)
	if err != nil {
		return err
	}

	// Compute final outcome
	outcome := censor.FinalOutcome{
		Decision: result.Decision,
		Reasons:  result.Reasons,
	}

	// Update resource review
	if err := c.store.UpdateResourceOutcome(ctx, task.ResourceReviewID, outcome); err != nil {
		return err
	}

	// Get biz review for aggregation
	bizReview, err := c.store.GetBizReview(ctx, resourceReview.BizReviewID)
	if err != nil {
		return err
	}

	// Aggregate biz decision
	biz := censor.BizContext{
		BizType: bizReview.BizType,
		BizID:   bizReview.BizID,
		Field:   bizReview.Field,
	}

	return c.aggregateBizDecision(ctx, resourceReview.BizReviewID, biz)
}

// recordError records an error for a resource review.
func (c *Client) recordError(ctx context.Context, resourceReviewID string, err error) {
	outcome := censor.FinalOutcome{
		Decision: censor.DecisionError,
		Reasons: []censor.Reason{{
			Code:    "error",
			Message: err.Error(),
		}},
	}
	if updateErr := c.store.UpdateResourceOutcome(ctx, resourceReviewID, outcome); updateErr != nil {
		// Log error but don't propagate - we're already in error handling
		_ = updateErr
	}
}

// parseOutcome parses a FinalOutcome from JSON.
func (c *Client) parseOutcome(jsonStr string) censor.FinalOutcome {
	var outcome censor.FinalOutcome
	if jsonStr == "" {
		return outcome
	}
	if err := json.Unmarshal([]byte(jsonStr), &outcome); err != nil {
		// Return empty outcome on parse error
		return censor.FinalOutcome{
			Decision: censor.DecisionError,
			Reasons: []censor.Reason{{
				Code:    "parse_error",
				Message: "Failed to parse outcome JSON",
			}},
		}
	}
	return outcome
}

// fireResourceReviewedHook fires the resource reviewed hook.
func (c *Client) fireResourceReviewedHook(ctx context.Context, biz censor.BizContext, resource censor.Resource, pr *pipelineResult, resourceReviewID, bizReviewID string) {
	event := hooks.ResourceReviewedEvent{
		Resource:         resource,
		Biz:              biz,
		Result:           *pr.getReviewResult(),
		Outcome:          *pr.finalOutcome,
		Provider:         c.opts.Pipeline.Primary,
		BizReviewID:      bizReviewID,
		ResourceReviewID: resourceReviewID,
		TraceID:          biz.TraceID,
		Timestamp:        time.Now(),
	}
	c.hooks.OnResourceReviewed(ctx, event)
}

// fireBizDecisionChangedHook fires the biz decision changed hook.
func (c *Client) fireBizDecisionChangedHook(ctx context.Context, biz censor.BizContext, reviews []censor.ResourceReview, bizReviewID string, decision censor.Decision) {
	var outcome censor.FinalOutcome
	if len(reviews) > 0 && reviews[0].OutcomeJSON != "" {
		json.Unmarshal([]byte(reviews[0].OutcomeJSON), &outcome)
	}
	outcome.Decision = decision

	var resource censor.Resource
	if len(reviews) > 0 {
		resource = censor.Resource{
			ResourceID:  reviews[0].ResourceID,
			Type:        reviews[0].ResourceType,
			ContentHash: reviews[0].ContentHash,
		}
	}

	event := hooks.BizDecisionChangedEvent{
		Biz:         biz,
		Resource:    resource,
		Outcome:     outcome,
		BizReviewID: bizReviewID,
		TraceID:     biz.TraceID,
		Timestamp:   time.Now(),
	}
	c.hooks.OnBizDecisionChanged(ctx, event)
}

// getScenesForBiz returns the required scenes for a business type.
func (c *Client) getScenesForBiz(bizType censor.BizType) []violation.UnifiedScene {
	req := violation.GetReviewRequirement(bizType)
	return req.Scenes
}

// fireViolationDetectedHook fires the violation detected hook.
func (c *Client) fireViolationDetectedHook(ctx context.Context, biz censor.BizContext, resource censor.Resource, outcome censor.FinalOutcome, snapshotID string) {
	// Convert reasons to violations for the event
	var violations violation.UnifiedList
	for _, reason := range outcome.Reasons {
		violations = append(violations, violation.Unified{
			Domain:         violation.Domain(reason.Code),
			Severity:       censor.RiskLevel(3), // Default severity
			Confidence:     1.0,
			OriginalLabels: []string{reason.Code, reason.Message},
		})
	}

	event := hooks.ViolationDetectedEvent{
		Resource:   resource,
		Biz:        biz,
		Violations: violations,
		SnapshotID: snapshotID,
		Provider:   c.opts.Pipeline.Primary,
		TraceID:    biz.TraceID,
		Timestamp:  time.Now(),
	}
	c.hooks.OnViolationDetected(ctx, event)
}

// fireManualReviewRequiredHook fires the manual review required hook.
func (c *Client) fireManualReviewRequiredHook(ctx context.Context, biz censor.BizContext, resource censor.Resource, result censor.ReviewResult, bizReviewID, resourceReviewID, manualTaskID string, priority int, expiresAt time.Time) {
	event := hooks.ManualReviewRequiredEvent{
		Resource:         resource,
		Biz:              biz,
		AutoResult:       result,
		Priority:         priority,
		ExpiresAt:        expiresAt,
		BizReviewID:      bizReviewID,
		ResourceReviewID: resourceReviewID,
		ManualTaskID:     manualTaskID,
		TraceID:          biz.TraceID,
		Timestamp:        time.Now(),
	}
	c.hooks.OnManualReviewRequired(ctx, event)
}
