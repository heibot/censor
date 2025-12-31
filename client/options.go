// Package client provides the main censor client for submitting content reviews.
package client

import (
	censor "github.com/heibot/censor"
	"github.com/heibot/censor/hooks"
	"github.com/heibot/censor/providers"
	"github.com/heibot/censor/store"
	"github.com/heibot/censor/violation"
)

// Options configures the censor client.
type Options struct {
	// Store is the data storage backend (required).
	Store store.Store

	// Hooks receives notifications when decisions change.
	Hooks hooks.Hooks

	// Providers is the list of content moderation providers.
	Providers []providers.Provider

	// Pipeline defines the provider chain and merge strategy.
	Pipeline PipelineConfig

	// TextMerge defines the text merging strategy.
	TextMerge censor.TextMergeStrategy

	// EnableDedup enables content deduplication.
	EnableDedup bool

	// AsyncPollInterval is the interval for polling async tasks (seconds).
	AsyncPollInterval int

	// AsyncPollTimeout is the timeout for async polling (seconds).
	AsyncPollTimeout int
}

// DefaultOptions returns default options.
func DefaultOptions() Options {
	return Options{
		Hooks: hooks.NopHooks{},
		TextMerge: censor.TextMergeStrategy{
			MaxLen:    censor.DefaultTextMergeMaxLen,
			Separator: censor.DefaultTextMergeSeparator,
		},
		EnableDedup:       true,
		AsyncPollInterval: censor.DefaultAsyncPollInterval,
		AsyncPollTimeout:  censor.DefaultAsyncPollTimeout,
	}
}

// PipelineConfig configures the provider pipeline.
type PipelineConfig struct {
	// Primary is the primary provider name.
	Primary string

	// Secondary is the secondary provider name (optional).
	Secondary string

	// Trigger defines when to invoke the secondary provider.
	Trigger TriggerRule

	// Merge defines how to merge results from multiple providers.
	Merge MergePolicy
}

// TriggerRule defines when to trigger the secondary provider.
type TriggerRule struct {
	// OnDecisions triggers secondary provider on these decisions.
	OnDecisions map[censor.Decision]bool
}

// DefaultTriggerRule returns a default trigger rule.
func DefaultTriggerRule() TriggerRule {
	return TriggerRule{
		OnDecisions: map[censor.Decision]bool{
			censor.DecisionBlock:  true,
			censor.DecisionReview: true,
			censor.DecisionError:  true,
		},
	}
}

// ShouldTrigger checks if the decision should trigger secondary provider.
func (tr TriggerRule) ShouldTrigger(decision censor.Decision) bool {
	return tr.OnDecisions[decision]
}

// MergePolicy defines how to merge results from multiple providers.
type MergePolicy string

const (
	// MergeMostStrict takes the strictest decision (block > review > pass).
	MergeMostStrict MergePolicy = "most_strict"

	// MergeMajority takes the majority decision.
	MergeMajority MergePolicy = "majority"

	// MergeAny takes the first non-pass decision.
	MergeAny MergePolicy = "any"

	// MergeAll requires all providers to block/review.
	MergeAll MergePolicy = "all"
)

// SubmitInput is the input for submitting content for review.
type SubmitInput struct {
	// Biz is the business context.
	Biz censor.BizContext

	// Resources is the list of resources to review.
	Resources []censor.Resource

	// Scenes specifies which scenes to detect. If empty, uses BizType defaults.
	Scenes []violation.UnifiedScene

	// EnableTextMerge enables text merging for this submission.
	EnableTextMerge bool

	// SyncMode forces synchronous review even for async-capable providers.
	SyncMode bool

	// Priority is the review priority (higher = more urgent).
	Priority int
}

// SubmitResult is the result of submitting content for review.
type SubmitResult struct {
	// BizReviewID is the business review ID.
	BizReviewID string

	// ResourceReviewIDs maps resource ID to review ID.
	ResourceReviewIDs map[string]string

	// ImmediateResults contains results for sync reviews.
	ImmediateResults map[string]censor.FinalOutcome

	// PendingAsync is true if there are pending async tasks.
	PendingAsync bool
}

// QueryInput is the input for querying review status.
type QueryInput struct {
	// BizReviewID is the business review ID.
	BizReviewID string

	// ResourceReviewID is a specific resource review ID (optional).
	ResourceReviewID string
}

// QueryResult is the result of querying review status.
type QueryResult struct {
	// BizReview is the business review record.
	BizReview *censor.BizReview

	// ResourceReviews is the list of resource reviews.
	ResourceReviews []censor.ResourceReview

	// AllComplete is true if all reviews are complete.
	AllComplete bool

	// FinalOutcome is the final outcome (only if complete).
	FinalOutcome *censor.FinalOutcome
}
