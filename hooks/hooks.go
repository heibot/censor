// Package hooks provides the hook interface for handling censor events.
package hooks

import (
	"context"
)

// Hooks defines the interface for handling censor events.
// Implement this interface to receive notifications when decisions change.
type Hooks interface {
	// OnBizDecisionChanged is called when a business object's decision changes.
	OnBizDecisionChanged(ctx context.Context, e BizDecisionChangedEvent) error

	// OnResourceReviewed is called when a resource review completes.
	OnResourceReviewed(ctx context.Context, e ResourceReviewedEvent) error

	// OnViolationDetected is called when a violation is detected.
	OnViolationDetected(ctx context.Context, e ViolationDetectedEvent) error

	// OnManualReviewRequired is called when manual review is needed.
	OnManualReviewRequired(ctx context.Context, e ManualReviewRequiredEvent) error
}

// NopHooks is a no-op implementation of Hooks.
type NopHooks struct{}

// OnBizDecisionChanged does nothing.
func (NopHooks) OnBizDecisionChanged(ctx context.Context, e BizDecisionChangedEvent) error {
	return nil
}

// OnResourceReviewed does nothing.
func (NopHooks) OnResourceReviewed(ctx context.Context, e ResourceReviewedEvent) error {
	return nil
}

// OnViolationDetected does nothing.
func (NopHooks) OnViolationDetected(ctx context.Context, e ViolationDetectedEvent) error {
	return nil
}

// OnManualReviewRequired does nothing.
func (NopHooks) OnManualReviewRequired(ctx context.Context, e ManualReviewRequiredEvent) error {
	return nil
}

// Ensure NopHooks implements Hooks.
var _ Hooks = NopHooks{}

// ChainHooks chains multiple Hooks implementations.
type ChainHooks []Hooks

// OnBizDecisionChanged calls all hooks in order.
func (ch ChainHooks) OnBizDecisionChanged(ctx context.Context, e BizDecisionChangedEvent) error {
	for _, h := range ch {
		if err := h.OnBizDecisionChanged(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

// OnResourceReviewed calls all hooks in order.
func (ch ChainHooks) OnResourceReviewed(ctx context.Context, e ResourceReviewedEvent) error {
	for _, h := range ch {
		if err := h.OnResourceReviewed(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

// OnViolationDetected calls all hooks in order.
func (ch ChainHooks) OnViolationDetected(ctx context.Context, e ViolationDetectedEvent) error {
	for _, h := range ch {
		if err := h.OnViolationDetected(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

// OnManualReviewRequired calls all hooks in order.
func (ch ChainHooks) OnManualReviewRequired(ctx context.Context, e ManualReviewRequiredEvent) error {
	for _, h := range ch {
		if err := h.OnManualReviewRequired(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

// FuncHooks allows using functions as hooks.
type FuncHooks struct {
	OnBizDecisionChangedFunc   func(ctx context.Context, e BizDecisionChangedEvent) error
	OnResourceReviewedFunc     func(ctx context.Context, e ResourceReviewedEvent) error
	OnViolationDetectedFunc    func(ctx context.Context, e ViolationDetectedEvent) error
	OnManualReviewRequiredFunc func(ctx context.Context, e ManualReviewRequiredEvent) error
}

// OnBizDecisionChanged calls the function if set.
func (fh FuncHooks) OnBizDecisionChanged(ctx context.Context, e BizDecisionChangedEvent) error {
	if fh.OnBizDecisionChangedFunc != nil {
		return fh.OnBizDecisionChangedFunc(ctx, e)
	}
	return nil
}

// OnResourceReviewed calls the function if set.
func (fh FuncHooks) OnResourceReviewed(ctx context.Context, e ResourceReviewedEvent) error {
	if fh.OnResourceReviewedFunc != nil {
		return fh.OnResourceReviewedFunc(ctx, e)
	}
	return nil
}

// OnViolationDetected calls the function if set.
func (fh FuncHooks) OnViolationDetected(ctx context.Context, e ViolationDetectedEvent) error {
	if fh.OnViolationDetectedFunc != nil {
		return fh.OnViolationDetectedFunc(ctx, e)
	}
	return nil
}

// OnManualReviewRequired calls the function if set.
func (fh FuncHooks) OnManualReviewRequired(ctx context.Context, e ManualReviewRequiredEvent) error {
	if fh.OnManualReviewRequiredFunc != nil {
		return fh.OnManualReviewRequiredFunc(ctx, e)
	}
	return nil
}
