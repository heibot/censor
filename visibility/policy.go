// Package visibility provides visibility policies for content rendering.
package visibility

import (
	censor "github.com/heibot/censor"
)

// Policy defines how content should be displayed based on review status.
type Policy string

const (
	// PolicyAllOrNothing requires all resources to pass for the object to be visible.
	PolicyAllOrNothing Policy = "all_or_nothing"

	// PolicyPartialAllowed allows partial visibility when some resources fail.
	PolicyPartialAllowed Policy = "partial_allowed"

	// PolicyCreatorOnlyDuringReview shows content to creator only during review.
	PolicyCreatorOnlyDuringReview Policy = "creator_only_during_review"

	// PolicyAlwaysVisible always shows content (with replacement if needed).
	PolicyAlwaysVisible Policy = "always_visible"
)

// ViewerRole represents who is viewing the content.
type ViewerRole string

const (
	ViewerCreator ViewerRole = "creator" // Content creator
	ViewerPublic  ViewerRole = "public"  // General public
	ViewerAdmin   ViewerRole = "admin"   // Administrator
)

// BizPolicyRegistry maps business types to their visibility policies.
var BizPolicyRegistry = map[censor.BizType]Policy{
	// User profile - partial allowed (each field independent)
	censor.BizUserAvatar:   PolicyPartialAllowed,
	censor.BizUserNickname: PolicyPartialAllowed,
	censor.BizUserBio:      PolicyPartialAllowed,

	// Notes - partial allowed (title+body can have different status)
	censor.BizNoteTitle:  PolicyPartialAllowed,
	censor.BizNoteBody:   PolicyPartialAllowed,
	censor.BizNoteImages: PolicyPartialAllowed,
	censor.BizNoteVideos: PolicyPartialAllowed,

	// Team - partial allowed
	censor.BizTeamName:    PolicyPartialAllowed,
	censor.BizTeamIntro:   PolicyPartialAllowed,
	censor.BizTeamBgImage: PolicyPartialAllowed,

	// Real-time communication - creator visible during review
	censor.BizChatMessage: PolicyCreatorOnlyDuringReview,
	censor.BizDanmaku:     PolicyCreatorOnlyDuringReview,
	censor.BizComment:     PolicyCreatorOnlyDuringReview,
}

// GetPolicy returns the visibility policy for a business type.
func GetPolicy(bizType censor.BizType) Policy {
	if policy, ok := BizPolicyRegistry[bizType]; ok {
		return policy
	}
	return PolicyAllOrNothing // Default to strictest
}

// SetPolicy sets a custom visibility policy for a business type.
func SetPolicy(bizType censor.BizType, policy Policy) {
	BizPolicyRegistry[bizType] = policy
}

// BizOutcome represents the aggregated outcome for a business object.
type BizOutcome struct {
	OverallDecision censor.Decision
	BlockedCount    int
	ReviewCount     int
	PassedCount     int
	TotalResources  int
}

// ComputeBizOutcome computes the aggregated outcome from bindings.
func ComputeBizOutcome(bindings []censor.CensorBinding) BizOutcome {
	outcome := BizOutcome{
		TotalResources:  len(bindings),
		OverallDecision: censor.DecisionPass,
	}

	for _, b := range bindings {
		switch censor.Decision(b.Decision) {
		case censor.DecisionBlock:
			outcome.BlockedCount++
		case censor.DecisionReview:
			outcome.ReviewCount++
		case censor.DecisionPass:
			outcome.PassedCount++
		}
	}

	// Determine overall decision
	if outcome.BlockedCount > 0 {
		outcome.OverallDecision = censor.DecisionBlock
	} else if outcome.ReviewCount > 0 {
		outcome.OverallDecision = censor.DecisionReview
	}

	return outcome
}

// CanView determines if a viewer can see the business object.
func CanView(policy Policy, outcome BizOutcome, viewer ViewerRole) bool {
	// Admins can always view
	if viewer == ViewerAdmin {
		return true
	}

	switch policy {
	case PolicyAllOrNothing:
		return outcome.OverallDecision == censor.DecisionPass

	case PolicyPartialAllowed:
		// Visible as long as not all blocked
		return outcome.BlockedCount < outcome.TotalResources

	case PolicyCreatorOnlyDuringReview:
		if outcome.OverallDecision == censor.DecisionPass {
			return true
		}
		if outcome.OverallDecision == censor.DecisionReview && viewer == ViewerCreator {
			return true
		}
		return false

	case PolicyAlwaysVisible:
		return true

	default:
		return outcome.OverallDecision == censor.DecisionPass
	}
}

// CanViewField determines if a specific field is visible.
func CanViewField(binding *censor.CensorBinding, viewer ViewerRole, policy Policy) bool {
	if binding == nil {
		return true // No binding means no restriction
	}

	// Admins can always view
	if viewer == ViewerAdmin {
		return true
	}

	decision := censor.Decision(binding.Decision)

	switch decision {
	case censor.DecisionPass, censor.DecisionPending:
		return true

	case censor.DecisionReview:
		if policy == PolicyCreatorOnlyDuringReview && viewer == ViewerCreator {
			return true
		}
		return policy == PolicyPartialAllowed || policy == PolicyAlwaysVisible

	case censor.DecisionBlock:
		return policy == PolicyAlwaysVisible

	default:
		return false
	}
}
