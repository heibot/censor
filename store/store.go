// Package store provides the data storage interface for the censor system.
package store

import (
	"context"
	"time"

	censor "github.com/heibot/censor"
)

// Store defines the interface for censor data storage.
type Store interface {
	// BizReview operations
	CreateBizReview(ctx context.Context, biz censor.BizContext) (bizReviewID string, err error)
	GetBizReview(ctx context.Context, bizReviewID string) (*censor.BizReview, error)
	UpdateBizDecision(ctx context.Context, bizReviewID string, decision censor.Decision) (changed bool, err error)
	UpdateBizStatus(ctx context.Context, bizReviewID string, status censor.ReviewStatus) error

	// ResourceReview operations
	CreateResourceReview(ctx context.Context, bizReviewID string, r censor.Resource) (resourceReviewID string, err error)
	GetResourceReview(ctx context.Context, resourceReviewID string) (*censor.ResourceReview, error)
	UpdateResourceOutcome(ctx context.Context, resourceReviewID string, outcome censor.FinalOutcome) error
	ListResourceReviewsByBizReview(ctx context.Context, bizReviewID string) ([]censor.ResourceReview, error)

	// ProviderTask operations
	CreateProviderTask(ctx context.Context, resourceReviewID, provider, mode, remoteTaskID string, raw map[string]any) (taskID string, err error)
	GetProviderTask(ctx context.Context, taskID string) (*censor.ProviderTask, error)
	GetProviderTaskByRemoteID(ctx context.Context, provider, remoteTaskID string) (*censor.ProviderTask, error)
	UpdateProviderTaskResult(ctx context.Context, taskID string, done bool, result *censor.ReviewResult, raw map[string]any) error
	ListPendingAsyncTasks(ctx context.Context, provider string, limit int) ([]censor.PendingTask, error)

	// CensorBinding operations (current state)
	GetBinding(ctx context.Context, bizType, bizID, field string) (*censor.CensorBinding, error)
	UpsertBinding(ctx context.Context, binding censor.CensorBinding) error
	ListBindingsByBiz(ctx context.Context, bizType, bizID string) ([]censor.CensorBinding, error)

	// CensorBindingHistory operations (historical state)
	CreateBindingHistory(ctx context.Context, history censor.CensorBindingHistory) error
	ListBindingHistory(ctx context.Context, bizType, bizID, field string, limit int) ([]censor.CensorBindingHistory, error)

	// ViolationSnapshot operations
	SaveViolationSnapshot(ctx context.Context, biz censor.BizContext, r censor.Resource, outcome censor.FinalOutcome) (snapshotID string, err error)
	GetViolationSnapshot(ctx context.Context, snapshotID string) (*censor.ViolationSnapshot, error)
	ListViolationsByBiz(ctx context.Context, bizType, bizID string, limit int) ([]censor.ViolationSnapshot, error)

	// Utility
	Now() time.Time

	// Transaction support
	WithTx(ctx context.Context, fn func(Store) error) error

	// Health check
	Ping(ctx context.Context) error
	Close() error
}

// QueryOptions provides common query options.
type QueryOptions struct {
	Limit  int
	Offset int
	Since  *time.Time
	Until  *time.Time
}

// BindingChange represents a change in binding state.
type BindingChange struct {
	Old *censor.CensorBinding
	New censor.CensorBinding
}

// HasChanged checks if the binding state has changed.
func (c BindingChange) HasChanged() bool {
	if c.Old == nil {
		return true
	}
	return c.Old.Decision != c.New.Decision ||
		c.Old.ReplacePolicy != c.New.ReplacePolicy ||
		c.Old.ReplaceValue != c.New.ReplaceValue ||
		c.Old.ViolationRefID != c.New.ViolationRefID
}
