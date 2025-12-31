package hooks

import (
	"time"

	censor "github.com/heibot/censor"
	"github.com/heibot/censor/violation"
)

// BizDecisionChangedEvent is emitted when a business object's decision changes.
type BizDecisionChangedEvent struct {
	// Business context
	Biz censor.BizContext `json:"biz"`

	// Resource that triggered the change
	Resource censor.Resource `json:"resource"`

	// Final outcome after all providers
	Outcome censor.FinalOutcome `json:"outcome"`

	// Previous decision (empty if first review)
	PreviousDecision censor.Decision `json:"previous_decision,omitempty"`

	// Review IDs for tracing
	BizReviewID      string `json:"biz_review_id"`
	ResourceReviewID string `json:"resource_review_id,omitempty"`

	// Unified violations
	Violations violation.UnifiedList `json:"violations,omitempty"`

	// Tracing
	TraceID   string    `json:"trace_id"`
	Timestamp time.Time `json:"timestamp"`
}

// ResourceReviewedEvent is emitted when a single resource review completes.
type ResourceReviewedEvent struct {
	// Resource that was reviewed
	Resource censor.Resource `json:"resource"`

	// Business context
	Biz censor.BizContext `json:"biz"`

	// Review result
	Result censor.ReviewResult `json:"result"`

	// Final outcome
	Outcome censor.FinalOutcome `json:"outcome"`

	// Provider that produced the result
	Provider string `json:"provider"`

	// Review IDs
	BizReviewID      string `json:"biz_review_id"`
	ResourceReviewID string `json:"resource_review_id"`

	// Tracing
	TraceID   string    `json:"trace_id"`
	Timestamp time.Time `json:"timestamp"`
}

// ViolationDetectedEvent is emitted when a violation is detected.
type ViolationDetectedEvent struct {
	// Resource with violation
	Resource censor.Resource `json:"resource"`

	// Business context
	Biz censor.BizContext `json:"biz"`

	// Detected violations
	Violations violation.UnifiedList `json:"violations"`

	// Snapshot ID for evidence
	SnapshotID string `json:"snapshot_id,omitempty"`

	// Provider that detected the violation
	Provider string `json:"provider"`

	// Tracing
	TraceID   string    `json:"trace_id"`
	Timestamp time.Time `json:"timestamp"`
}

// ManualReviewRequiredEvent is emitted when manual review is needed.
type ManualReviewRequiredEvent struct {
	// Resource requiring review
	Resource censor.Resource `json:"resource"`

	// Business context
	Biz censor.BizContext `json:"biz"`

	// Auto review result that triggered manual review
	AutoResult censor.ReviewResult `json:"auto_result"`

	// Review priority (higher = more urgent)
	Priority int `json:"priority"`

	// Expires at (when auto-decision should be made if not reviewed)
	ExpiresAt time.Time `json:"expires_at"`

	// Review IDs
	BizReviewID      string `json:"biz_review_id"`
	ResourceReviewID string `json:"resource_review_id"`

	// Manual task ID (if using manual provider)
	ManualTaskID string `json:"manual_task_id,omitempty"`

	// Tracing
	TraceID   string    `json:"trace_id"`
	Timestamp time.Time `json:"timestamp"`
}

// DecisionChange represents a change in decision.
type DecisionChange struct {
	From censor.Decision `json:"from"`
	To   censor.Decision `json:"to"`
}

// IsEscalation returns true if the decision became stricter.
func (dc DecisionChange) IsEscalation() bool {
	return decisionSeverity(dc.To) > decisionSeverity(dc.From)
}

// IsDeescalation returns true if the decision became more lenient.
func (dc DecisionChange) IsDeescalation() bool {
	return decisionSeverity(dc.To) < decisionSeverity(dc.From)
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
