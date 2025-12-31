package censor

import (
	"time"
)

// Resource represents a content resource to be reviewed.
type Resource struct {
	ResourceID  string            `json:"resource_id"`  // Unique identifier for the resource
	Type        ResourceType      `json:"type"`         // text/image/video
	ContentText string            `json:"content_text"` // Text content (for text type)
	ContentURL  string            `json:"content_url"`  // URL for image/video
	ContentHash string            `json:"content_hash"` // Hash for deduplication
	Extra       map[string]string `json:"extra"`        // Additional metadata
}

// BizContext represents the business context for a review request.
type BizContext struct {
	BizType     BizType   `json:"biz_type"`     // Business type (user_avatar, note_body, etc.)
	BizID       string    `json:"biz_id"`       // Business object ID (userID, noteID, etc.)
	Field       string    `json:"field"`        // Specific field (title, body, avatar, etc.)
	SubmitterID string    `json:"submitter_id"` // Who submitted the content
	TraceID     string    `json:"trace_id"`     // Request trace ID for debugging
	CreatedAt   time.Time `json:"created_at"`   // When the content was created
}

// Reason represents the reason for a review decision.
type Reason struct {
	Code     string         `json:"code"`     // Reason code
	Message  string         `json:"message"`  // Human-readable message
	Provider string         `json:"provider"` // Which provider detected this
	HitTags  []string       `json:"hit_tags"` // Tags that were hit
	Raw      map[string]any `json:"raw"`      // Raw provider response (trimmed)
}

// ReviewResult represents the result from a single provider review.
type ReviewResult struct {
	Decision   Decision  `json:"decision"`    // pass/review/block/error
	Confidence float64   `json:"confidence"`  // Confidence score (0-1)
	Reasons    []Reason  `json:"reasons"`     // Reasons for the decision
	Provider   string    `json:"provider"`    // Provider name
	ReviewedAt time.Time `json:"reviewed_at"` // When the review was completed
}

// FinalOutcome represents the final decision after all provider reviews.
type FinalOutcome struct {
	Decision      Decision      `json:"decision"`       // Final decision
	ReplacePolicy ReplacePolicy `json:"replace_policy"` // How to handle if blocked
	ReplaceValue  string        `json:"replace_value"`  // Replacement value if applicable
	Reasons       []Reason      `json:"reasons"`        // All reasons from all providers
	RiskLevel     RiskLevel     `json:"risk_level"`     // Overall risk level
}

// BizReview represents a business-level review record.
type BizReview struct {
	ID          string       `json:"id" db:"id"`
	BizType     BizType      `json:"biz_type" db:"biz_type"`
	BizID       string       `json:"biz_id" db:"biz_id"`
	Field       string       `json:"field" db:"field"`
	SubmitterID string       `json:"submitter_id" db:"submitter_id"`
	TraceID     string       `json:"trace_id" db:"trace_id"`
	Decision    Decision     `json:"decision" db:"decision"`
	Status      ReviewStatus `json:"status" db:"status"`
	CreatedAt   int64        `json:"created_at" db:"created_at"`
	UpdatedAt   int64        `json:"updated_at" db:"updated_at"`
}

// ResourceReview represents a resource-level review record.
type ResourceReview struct {
	ID           string       `json:"id" db:"id"`
	BizReviewID  string       `json:"biz_review_id" db:"biz_review_id"`
	ResourceID   string       `json:"resource_id" db:"resource_id"`
	ResourceType ResourceType `json:"resource_type" db:"resource_type"`
	ContentHash  string       `json:"content_hash" db:"content_hash"`
	ContentText  string       `json:"content_text" db:"content_text"`
	ContentURL   string       `json:"content_url" db:"content_url"`
	Decision     Decision     `json:"decision" db:"decision"`
	OutcomeJSON  string       `json:"outcome_json" db:"outcome_json"`
	CreatedAt    int64        `json:"created_at" db:"created_at"`
	UpdatedAt    int64        `json:"updated_at" db:"updated_at"`
}

// ProviderTask represents a task submitted to a provider.
type ProviderTask struct {
	ID               string `json:"id" db:"id"`
	ResourceReviewID string `json:"resource_review_id" db:"resource_review_id"`
	Provider         string `json:"provider" db:"provider"`
	Mode             string `json:"mode" db:"mode"` // sync/async
	RemoteTaskID     string `json:"remote_task_id" db:"remote_task_id"`
	Done             bool   `json:"done" db:"done"`
	ResultJSON       string `json:"result_json" db:"result_json"`
	RawJSON          string `json:"raw_json" db:"raw_json"`
	CreatedAt        int64  `json:"created_at" db:"created_at"`
	UpdatedAt        int64  `json:"updated_at" db:"updated_at"`
}

// ViolationSnapshot stores the evidence for blocked/review content.
type ViolationSnapshot struct {
	ID           string `json:"id" db:"id"`
	BizType      string `json:"biz_type" db:"biz_type"`
	BizID        string `json:"biz_id" db:"biz_id"`
	Field        string `json:"field" db:"field"`
	ResourceID   string `json:"resource_id" db:"resource_id"`
	ResourceType string `json:"resource_type" db:"resource_type"`
	ContentHash  string `json:"content_hash" db:"content_hash"`
	ContentText  string `json:"content_text" db:"content_text"`
	ContentURL   string `json:"content_url" db:"content_url"`
	OutcomeJSON  string `json:"outcome_json" db:"outcome_json"`
	CreatedAt    int64  `json:"created_at" db:"created_at"`
}

// CensorBinding represents the current moderation state binding for a business field.
type CensorBinding struct {
	ID             string `json:"id" db:"id"`
	BizType        string `json:"biz_type" db:"biz_type"`
	BizID          string `json:"biz_id" db:"biz_id"`
	Field          string `json:"field" db:"field"`
	ResourceID     string `json:"resource_id" db:"resource_id"`
	ResourceType   string `json:"resource_type" db:"resource_type"`
	ContentHash    string `json:"content_hash" db:"content_hash"`
	ReviewID       string `json:"review_id" db:"review_id"`
	Decision       string `json:"decision" db:"decision"`
	ReplacePolicy  string `json:"replace_policy" db:"replace_policy"`
	ReplaceValue   string `json:"replace_value" db:"replace_value"`
	ViolationRefID string `json:"violation_ref_id" db:"violation_ref_id"`
	ReviewRevision int    `json:"review_revision" db:"review_revision"`
	UpdatedAt      int64  `json:"updated_at" db:"updated_at"`
}

// CensorBindingHistory represents historical moderation state changes.
type CensorBindingHistory struct {
	ID             string `json:"id" db:"id"`
	BizType        string `json:"biz_type" db:"biz_type"`
	BizID          string `json:"biz_id" db:"biz_id"`
	Field          string `json:"field" db:"field"`
	ResourceID     string `json:"resource_id" db:"resource_id"`
	ResourceType   string `json:"resource_type" db:"resource_type"`
	Decision       string `json:"decision" db:"decision"`
	ReplacePolicy  string `json:"replace_policy" db:"replace_policy"`
	ReplaceValue   string `json:"replace_value" db:"replace_value"`
	ViolationRefID string `json:"violation_ref_id" db:"violation_ref_id"`
	ReviewRevision int    `json:"review_revision" db:"review_revision"`
	ReasonJSON     string `json:"reason_json" db:"reason_json"`
	Source         string `json:"source" db:"source"`           // auto/manual/recheck/policy_upgrade/appeal
	ReviewerID     string `json:"reviewer_id" db:"reviewer_id"` // Who made the decision (for manual review)
	Comment        string `json:"comment" db:"comment"`         // Reviewer's comment
	CreatedAt      int64  `json:"created_at" db:"created_at"`
}

// TextMergeStrategy defines how to merge multiple text resources.
type TextMergeStrategy struct {
	MaxLen    int    // Maximum length for merged text
	Separator string // Separator between merged texts
}

// MergedText represents the result of merging multiple texts.
type MergedText struct {
	Merged string      // The merged text
	Parts  []string    // Original parts
	Index  []PartIndex // Index mapping for each part
}

// PartIndex represents the position of a part in the merged text.
type PartIndex struct {
	Start int // Start position in merged text
	End   int // End position in merged text
}

// PendingTask represents an async task waiting for result.
type PendingTask struct {
	ProviderTaskID string `json:"provider_task_id" db:"id"`
	Provider       string `json:"provider" db:"provider"`
	RemoteTaskID   string `json:"remote_task_id" db:"remote_task_id"`
}
