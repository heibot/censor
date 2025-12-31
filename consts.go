// Package censor provides a powerful content moderation system supporting
// multiple cloud providers (Aliyun, Huawei, Tencent), multiple content types
// (text, image, video), and flexible review policies.
package censor

// ResourceType represents the type of content being reviewed.
type ResourceType string

const (
	ResourceText  ResourceType = "text"
	ResourceImage ResourceType = "image"
	ResourceVideo ResourceType = "video"
)

// BizType represents different business scenarios for content review.
type BizType string

const (
	// User profile related
	BizUserAvatar   BizType = "user_avatar"
	BizUserNickname BizType = "user_nickname"
	BizUserBio      BizType = "user_bio"

	// Note/Post related
	BizNoteTitle  BizType = "note_title"
	BizNoteBody   BizType = "note_body"
	BizNoteImages BizType = "note_images"
	BizNoteVideos BizType = "note_videos"

	// Team related
	BizTeamName    BizType = "team_name"
	BizTeamIntro   BizType = "team_intro"
	BizTeamBgImage BizType = "team_bg_image"

	// Communication related
	BizChatMessage BizType = "chat_message"
	BizDanmaku     BizType = "danmaku"
	BizComment     BizType = "comment"
)

// Decision represents the review decision for a resource.
type Decision string

const (
	DecisionPending Decision = "pending" // Awaiting review
	DecisionPass    Decision = "pass"    // Content approved
	DecisionReview  Decision = "review"  // Needs manual review
	DecisionBlock   Decision = "block"   // Content blocked
	DecisionError   Decision = "error"   // Review failed with error
)

// ReplacePolicy defines how to handle blocked content.
type ReplacePolicy string

const (
	ReplacePolicyNone    ReplacePolicy = "none"          // No replacement, hide content
	ReplacePolicyDefault ReplacePolicy = "default_value" // Replace with default value
	ReplacePolicyMask    ReplacePolicy = "mask"          // Mask sensitive parts
)

// ReviewStatus represents the status of a review task.
type ReviewStatus string

const (
	StatusPending  ReviewStatus = "pending"
	StatusRunning  ReviewStatus = "running"
	StatusDone     ReviewStatus = "done"
	StatusFailed   ReviewStatus = "failed"
	StatusCanceled ReviewStatus = "canceled"
)

// RiskLevel represents the severity of a violation.
type RiskLevel int

const (
	RiskLow RiskLevel = iota + 1
	RiskMedium
	RiskHigh
	RiskSevere
)

// String returns the string representation of RiskLevel.
func (r RiskLevel) String() string {
	switch r {
	case RiskLow:
		return "low"
	case RiskMedium:
		return "medium"
	case RiskHigh:
		return "high"
	case RiskSevere:
		return "severe"
	default:
		return "unknown"
	}
}

// HistorySource represents the source of a review history entry.
type HistorySource string

const (
	SourceAuto          HistorySource = "auto"           // Automatic review
	SourceManual        HistorySource = "manual"         // Manual review
	SourceRecheck       HistorySource = "recheck"        // Re-review
	SourcePolicyUpgrade HistorySource = "policy_upgrade" // Policy upgrade triggered
	SourceAppeal        HistorySource = "appeal"         // User appeal
)

// Default configuration values
const (
	DefaultTextMergeMaxLen    = 1800
	DefaultTextMergeSeparator = "\n---\n"
	DefaultAsyncPollInterval  = 5  // seconds
	DefaultAsyncPollTimeout   = 60 // seconds
)
