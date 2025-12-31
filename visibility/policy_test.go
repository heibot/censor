package visibility

import (
	"testing"

	censor "github.com/heibot/censor"
)

func TestGetPolicy(t *testing.T) {
	tests := []struct {
		name     string
		bizType  censor.BizType
		expected Policy
	}{
		{
			name:     "user avatar - partial allowed",
			bizType:  censor.BizUserAvatar,
			expected: PolicyPartialAllowed,
		},
		{
			name:     "chat message - creator only during review",
			bizType:  censor.BizChatMessage,
			expected: PolicyCreatorOnlyDuringReview,
		},
		{
			name:     "unknown biz type - all or nothing",
			bizType:  censor.BizType("unknown"),
			expected: PolicyAllOrNothing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetPolicy(tt.bizType)
			if result != tt.expected {
				t.Errorf("GetPolicy(%v) = %v, want %v", tt.bizType, result, tt.expected)
			}
		})
	}
}

func TestSetPolicy(t *testing.T) {
	// Save original and restore after test
	original := BizPolicyRegistry[censor.BizUserAvatar]
	defer func() {
		BizPolicyRegistry[censor.BizUserAvatar] = original
	}()

	SetPolicy(censor.BizUserAvatar, PolicyAlwaysVisible)

	if GetPolicy(censor.BizUserAvatar) != PolicyAlwaysVisible {
		t.Error("SetPolicy did not update the policy")
	}
}

func TestComputeBizOutcome(t *testing.T) {
	tests := []struct {
		name             string
		bindings         []censor.CensorBinding
		expectedDecision censor.Decision
		expectedBlocked  int
		expectedReview   int
		expectedPassed   int
	}{
		{
			name:             "empty bindings",
			bindings:         []censor.CensorBinding{},
			expectedDecision: censor.DecisionPass,
			expectedBlocked:  0,
			expectedReview:   0,
			expectedPassed:   0,
		},
		{
			name: "all passed",
			bindings: []censor.CensorBinding{
				{Decision: string(censor.DecisionPass)},
				{Decision: string(censor.DecisionPass)},
			},
			expectedDecision: censor.DecisionPass,
			expectedBlocked:  0,
			expectedReview:   0,
			expectedPassed:   2,
		},
		{
			name: "one blocked",
			bindings: []censor.CensorBinding{
				{Decision: string(censor.DecisionPass)},
				{Decision: string(censor.DecisionBlock)},
			},
			expectedDecision: censor.DecisionBlock,
			expectedBlocked:  1,
			expectedReview:   0,
			expectedPassed:   1,
		},
		{
			name: "one review",
			bindings: []censor.CensorBinding{
				{Decision: string(censor.DecisionPass)},
				{Decision: string(censor.DecisionReview)},
			},
			expectedDecision: censor.DecisionReview,
			expectedBlocked:  0,
			expectedReview:   1,
			expectedPassed:   1,
		},
		{
			name: "block takes precedence over review",
			bindings: []censor.CensorBinding{
				{Decision: string(censor.DecisionReview)},
				{Decision: string(censor.DecisionBlock)},
			},
			expectedDecision: censor.DecisionBlock,
			expectedBlocked:  1,
			expectedReview:   1,
			expectedPassed:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outcome := ComputeBizOutcome(tt.bindings)

			if outcome.OverallDecision != tt.expectedDecision {
				t.Errorf("OverallDecision = %v, want %v", outcome.OverallDecision, tt.expectedDecision)
			}
			if outcome.BlockedCount != tt.expectedBlocked {
				t.Errorf("BlockedCount = %d, want %d", outcome.BlockedCount, tt.expectedBlocked)
			}
			if outcome.ReviewCount != tt.expectedReview {
				t.Errorf("ReviewCount = %d, want %d", outcome.ReviewCount, tt.expectedReview)
			}
			if outcome.PassedCount != tt.expectedPassed {
				t.Errorf("PassedCount = %d, want %d", outcome.PassedCount, tt.expectedPassed)
			}
		})
	}
}

func TestCanView(t *testing.T) {
	tests := []struct {
		name     string
		policy   Policy
		outcome  BizOutcome
		viewer   ViewerRole
		expected bool
	}{
		// Admin can always view
		{
			name:     "admin can view blocked content",
			policy:   PolicyAllOrNothing,
			outcome:  BizOutcome{OverallDecision: censor.DecisionBlock, BlockedCount: 1, TotalResources: 1},
			viewer:   ViewerAdmin,
			expected: true,
		},
		// PolicyAllOrNothing
		{
			name:     "all or nothing - pass",
			policy:   PolicyAllOrNothing,
			outcome:  BizOutcome{OverallDecision: censor.DecisionPass},
			viewer:   ViewerPublic,
			expected: true,
		},
		{
			name:     "all or nothing - block",
			policy:   PolicyAllOrNothing,
			outcome:  BizOutcome{OverallDecision: censor.DecisionBlock},
			viewer:   ViewerPublic,
			expected: false,
		},
		// PolicyPartialAllowed
		{
			name:     "partial allowed - some blocked",
			policy:   PolicyPartialAllowed,
			outcome:  BizOutcome{BlockedCount: 1, TotalResources: 3},
			viewer:   ViewerPublic,
			expected: true,
		},
		{
			name:     "partial allowed - all blocked",
			policy:   PolicyPartialAllowed,
			outcome:  BizOutcome{BlockedCount: 3, TotalResources: 3},
			viewer:   ViewerPublic,
			expected: false,
		},
		// PolicyCreatorOnlyDuringReview
		{
			name:     "creator only - creator can view during review",
			policy:   PolicyCreatorOnlyDuringReview,
			outcome:  BizOutcome{OverallDecision: censor.DecisionReview},
			viewer:   ViewerCreator,
			expected: true,
		},
		{
			name:     "creator only - public cannot view during review",
			policy:   PolicyCreatorOnlyDuringReview,
			outcome:  BizOutcome{OverallDecision: censor.DecisionReview},
			viewer:   ViewerPublic,
			expected: false,
		},
		{
			name:     "creator only - public can view passed",
			policy:   PolicyCreatorOnlyDuringReview,
			outcome:  BizOutcome{OverallDecision: censor.DecisionPass},
			viewer:   ViewerPublic,
			expected: true,
		},
		// PolicyAlwaysVisible
		{
			name:     "always visible - blocked content still visible",
			policy:   PolicyAlwaysVisible,
			outcome:  BizOutcome{OverallDecision: censor.DecisionBlock},
			viewer:   ViewerPublic,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CanView(tt.policy, tt.outcome, tt.viewer)
			if result != tt.expected {
				t.Errorf("CanView() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCanViewField(t *testing.T) {
	tests := []struct {
		name     string
		binding  *censor.CensorBinding
		viewer   ViewerRole
		policy   Policy
		expected bool
	}{
		{
			name:     "nil binding - visible",
			binding:  nil,
			viewer:   ViewerPublic,
			policy:   PolicyAllOrNothing,
			expected: true,
		},
		{
			name:     "admin can view blocked",
			binding:  &censor.CensorBinding{Decision: string(censor.DecisionBlock)},
			viewer:   ViewerAdmin,
			policy:   PolicyAllOrNothing,
			expected: true,
		},
		{
			name:     "pass decision - visible",
			binding:  &censor.CensorBinding{Decision: string(censor.DecisionPass)},
			viewer:   ViewerPublic,
			policy:   PolicyAllOrNothing,
			expected: true,
		},
		{
			name:     "pending decision - visible",
			binding:  &censor.CensorBinding{Decision: string(censor.DecisionPending)},
			viewer:   ViewerPublic,
			policy:   PolicyAllOrNothing,
			expected: true,
		},
		{
			name:     "review - creator can view with creator only policy",
			binding:  &censor.CensorBinding{Decision: string(censor.DecisionReview)},
			viewer:   ViewerCreator,
			policy:   PolicyCreatorOnlyDuringReview,
			expected: true,
		},
		{
			name:     "review - partial allowed shows to public",
			binding:  &censor.CensorBinding{Decision: string(censor.DecisionReview)},
			viewer:   ViewerPublic,
			policy:   PolicyPartialAllowed,
			expected: true,
		},
		{
			name:     "block - always visible policy shows",
			binding:  &censor.CensorBinding{Decision: string(censor.DecisionBlock)},
			viewer:   ViewerPublic,
			policy:   PolicyAlwaysVisible,
			expected: true,
		},
		{
			name:     "block - all or nothing hides",
			binding:  &censor.CensorBinding{Decision: string(censor.DecisionBlock)},
			viewer:   ViewerPublic,
			policy:   PolicyAllOrNothing,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CanViewField(tt.binding, tt.viewer, tt.policy)
			if result != tt.expected {
				t.Errorf("CanViewField() = %v, want %v", result, tt.expected)
			}
		})
	}
}
