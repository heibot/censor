package client

import (
	"context"
	"encoding/json"
	"time"

	censor "github.com/heibot/censor"
	"github.com/heibot/censor/providers"
	"github.com/heibot/censor/violation"
)

// pipelineExecutor handles the provider pipeline execution.
type pipelineExecutor struct {
	providers map[string]providers.Provider
	config    PipelineConfig
}

// newPipelineExecutor creates a new pipeline executor.
func newPipelineExecutor(provs []providers.Provider, config PipelineConfig) *pipelineExecutor {
	provMap := make(map[string]providers.Provider)
	for _, p := range provs {
		provMap[p.Name()] = p
	}
	return &pipelineExecutor{
		providers: provMap,
		config:    config,
	}
}

// execute runs the pipeline for a resource.
func (pe *pipelineExecutor) execute(ctx context.Context, req providers.SubmitRequest) (*pipelineResult, error) {
	result := &pipelineResult{
		providerResults: make(map[string]*censor.ReviewResult),
	}

	// Get primary provider
	primary, ok := pe.providers[pe.config.Primary]
	if !ok {
		return nil, censor.ErrProviderNotFound
	}

	// Check if primary provider can handle all required scenes
	if len(req.Scenes) > 0 {
		capability := primary.SceneCapability()
		if missing := capability.MissingScenes(req.Scenes, req.Resource.Type); len(missing) > 0 {
			// Log warning but continue - provider may still handle some scenes
			result.missingScenes = missing
		}
	}

	// Submit to primary provider
	resp, err := primary.Submit(ctx, req)
	if err != nil {
		return nil, err
	}

	result.primaryTaskID = resp.TaskID
	result.mode = resp.Mode

	// Handle sync result
	if resp.Mode == providers.ModeSync && resp.Immediate != nil {
		result.providerResults[pe.config.Primary] = resp.Immediate

		// Check if we need to trigger secondary
		if pe.shouldTriggerSecondary(resp.Immediate.Decision) {
			if err := pe.runSecondary(ctx, req, result); err != nil {
				// Log but don't fail - primary result is enough
				result.secondaryError = err
			}
		}

		// Compute final outcome
		result.finalOutcome = pe.computeFinalOutcome(result.providerResults)
	}

	return result, nil
}

// shouldTriggerSecondary checks if secondary provider should be invoked.
func (pe *pipelineExecutor) shouldTriggerSecondary(decision censor.Decision) bool {
	if pe.config.Secondary == "" {
		return false
	}
	return pe.config.Trigger.ShouldTrigger(decision)
}

// runSecondary runs the secondary provider.
func (pe *pipelineExecutor) runSecondary(ctx context.Context, req providers.SubmitRequest, result *pipelineResult) error {
	secondary, ok := pe.providers[pe.config.Secondary]
	if !ok {
		return censor.ErrProviderNotFound
	}

	resp, err := secondary.Submit(ctx, req)
	if err != nil {
		return err
	}

	result.secondaryTaskID = resp.TaskID

	if resp.Mode == providers.ModeSync && resp.Immediate != nil {
		result.providerResults[pe.config.Secondary] = resp.Immediate
	}

	return nil
}

// computeFinalOutcome computes the final outcome from provider results.
func (pe *pipelineExecutor) computeFinalOutcome(results map[string]*censor.ReviewResult) *censor.FinalOutcome {
	if len(results) == 0 {
		return nil
	}

	// Collect all violations
	var allViolations violation.UnifiedList
	var allReasons []censor.Reason

	for providerName, result := range results {
		if result == nil {
			continue
		}

		// Translate provider labels to unified violations
		if p, ok := pe.providers[providerName]; ok && p.Translator() != nil {
			labels := extractLabels(result.Reasons)
			scores := extractScores(result.Reasons)
			violations := p.Translator().Translate(violation.TranslationContext{}, labels, scores)
			allViolations = append(allViolations, violations...)
		}

		allReasons = append(allReasons, result.Reasons...)
	}

	// Merge results based on policy
	finalDecision := pe.mergeDecisions(results)

	// Get outcome from violations
	outcome := allViolations.DecideOutcome()
	outcome.Decision = finalDecision
	outcome.Reasons = allReasons

	return &outcome
}

// mergeDecisions merges decisions based on the merge policy.
func (pe *pipelineExecutor) mergeDecisions(results map[string]*censor.ReviewResult) censor.Decision {
	switch pe.config.Merge {
	case MergeMostStrict:
		return pe.mergeMostStrict(results)
	case MergeMajority:
		return pe.mergeMajority(results)
	case MergeAny:
		return pe.mergeAny(results)
	case MergeAll:
		return pe.mergeAll(results)
	default:
		return pe.mergeMostStrict(results)
	}
}

// mergeMostStrict takes the strictest decision.
func (pe *pipelineExecutor) mergeMostStrict(results map[string]*censor.ReviewResult) censor.Decision {
	strictest := censor.DecisionPass

	for _, r := range results {
		if r == nil {
			continue
		}
		if decisionSeverity(r.Decision) > decisionSeverity(strictest) {
			strictest = r.Decision
		}
	}

	return strictest
}

// mergeMajority takes the majority decision.
func (pe *pipelineExecutor) mergeMajority(results map[string]*censor.ReviewResult) censor.Decision {
	counts := make(map[censor.Decision]int)

	for _, r := range results {
		if r == nil {
			continue
		}
		counts[r.Decision]++
	}

	// Find majority
	maxCount := 0
	majority := censor.DecisionPass
	for decision, count := range counts {
		if count > maxCount || (count == maxCount && decisionSeverity(decision) > decisionSeverity(majority)) {
			maxCount = count
			majority = decision
		}
	}

	return majority
}

// mergeAny takes the first non-pass decision.
func (pe *pipelineExecutor) mergeAny(results map[string]*censor.ReviewResult) censor.Decision {
	for _, r := range results {
		if r == nil {
			continue
		}
		if r.Decision != censor.DecisionPass {
			return r.Decision
		}
	}
	return censor.DecisionPass
}

// mergeAll requires all providers to agree.
func (pe *pipelineExecutor) mergeAll(results map[string]*censor.ReviewResult) censor.Decision {
	allBlock := true
	allReview := true
	hasResult := false

	for _, r := range results {
		if r == nil {
			continue
		}
		hasResult = true
		if r.Decision != censor.DecisionBlock {
			allBlock = false
		}
		if r.Decision != censor.DecisionReview && r.Decision != censor.DecisionBlock {
			allReview = false
		}
	}

	if !hasResult {
		return censor.DecisionPass
	}
	if allBlock {
		return censor.DecisionBlock
	}
	if allReview {
		return censor.DecisionReview
	}
	return censor.DecisionPass
}

func decisionSeverity(d censor.Decision) int {
	switch d {
	case censor.DecisionPass:
		return 0
	case censor.DecisionPending:
		return 1
	case censor.DecisionReview:
		return 2
	case censor.DecisionBlock:
		return 3
	case censor.DecisionError:
		return 4
	default:
		return 0
	}
}

func extractLabels(reasons []censor.Reason) []string {
	var labels []string
	for _, r := range reasons {
		if r.Code != "" {
			labels = append(labels, r.Code)
		}
		labels = append(labels, r.HitTags...)
	}
	return labels
}

func extractScores(reasons []censor.Reason) map[string]float64 {
	scores := make(map[string]float64)
	for _, r := range reasons {
		if r.Raw != nil {
			if score, ok := r.Raw["score"].(float64); ok {
				scores[r.Code] = score
			}
		}
	}
	return scores
}

// pipelineResult holds the result of a pipeline execution.
type pipelineResult struct {
	mode            providers.Mode
	primaryTaskID   string
	secondaryTaskID string
	providerResults map[string]*censor.ReviewResult
	finalOutcome    *censor.FinalOutcome
	secondaryError  error
	missingScenes   []violation.UnifiedScene // Scenes not supported by provider
}

// toJSON converts provider results to JSON for storage.
func (pr *pipelineResult) toJSON() (string, error) {
	data := map[string]any{
		"mode":             pr.mode,
		"primary_task_id":  pr.primaryTaskID,
		"provider_results": pr.providerResults,
	}
	if pr.secondaryTaskID != "" {
		data["secondary_task_id"] = pr.secondaryTaskID
	}
	if pr.secondaryError != nil {
		data["secondary_error"] = pr.secondaryError.Error()
	}

	b, err := json.Marshal(data)
	return string(b), err
}

// isComplete checks if the pipeline execution is complete.
func (pr *pipelineResult) isComplete() bool {
	return pr.mode == providers.ModeSync && pr.finalOutcome != nil
}

// getReviewResult returns the first available review result.
func (pr *pipelineResult) getReviewResult() *censor.ReviewResult {
	for _, r := range pr.providerResults {
		if r != nil {
			return r
		}
	}
	return &censor.ReviewResult{
		Decision:   censor.DecisionPending,
		ReviewedAt: time.Now(),
	}
}
