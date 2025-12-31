package violation

import (
	"testing"

	censor "github.com/heibot/censor"
)

func TestUnifiedList_GetHighestSeverity(t *testing.T) {
	tests := []struct {
		name     string
		list     UnifiedList
		expected censor.RiskLevel
	}{
		{
			name:     "empty list",
			list:     UnifiedList{},
			expected: censor.RiskLow,
		},
		{
			name: "single item",
			list: UnifiedList{
				{Domain: DomainPornography, Severity: censor.RiskHigh},
			},
			expected: censor.RiskHigh,
		},
		{
			name: "multiple items",
			list: UnifiedList{
				{Domain: DomainPornography, Severity: censor.RiskMedium},
				{Domain: DomainViolence, Severity: censor.RiskSevere},
				{Domain: DomainAds, Severity: censor.RiskLow},
			},
			expected: censor.RiskSevere,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.list.GetHighestSeverity()
			if result != tt.expected {
				t.Errorf("GetHighestSeverity() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestUnifiedList_GetDomains(t *testing.T) {
	list := UnifiedList{
		{Domain: DomainPornography},
		{Domain: DomainViolence},
		{Domain: DomainPornography}, // duplicate
		{Domain: DomainAds},
	}

	domains := list.GetDomains()

	if len(domains) != 3 {
		t.Errorf("GetDomains() returned %d domains, want 3", len(domains))
	}

	domainSet := make(map[Domain]bool)
	for _, d := range domains {
		domainSet[d] = true
	}

	expectedDomains := []Domain{DomainPornography, DomainViolence, DomainAds}
	for _, d := range expectedDomains {
		if !domainSet[d] {
			t.Errorf("GetDomains() missing domain %v", d)
		}
	}
}

func TestUnifiedList_GetAllTags(t *testing.T) {
	list := UnifiedList{
		{Tags: []Tag{TagNudity, TagPornographicAct}},
		{Tags: []Tag{TagBloodContent, TagNudity}}, // TagNudity is duplicate
		{Tags: []Tag{TagSpamAds}},
	}

	tags := list.GetAllTags()

	if len(tags) != 4 {
		t.Errorf("GetAllTags() returned %d tags, want 4", len(tags))
	}
}

func TestUnifiedList_HasDomain(t *testing.T) {
	list := UnifiedList{
		{Domain: DomainPornography},
		{Domain: DomainViolence},
	}

	if !list.HasDomain(DomainPornography) {
		t.Error("HasDomain(DomainPornography) = false, want true")
	}

	if list.HasDomain(DomainAds) {
		t.Error("HasDomain(DomainAds) = true, want false")
	}
}

func TestUnifiedList_HasSeverityAtLeast(t *testing.T) {
	list := UnifiedList{
		{Severity: censor.RiskLow},
		{Severity: censor.RiskMedium},
	}

	if !list.HasSeverityAtLeast(censor.RiskMedium) {
		t.Error("HasSeverityAtLeast(RiskMedium) = false, want true")
	}

	if list.HasSeverityAtLeast(censor.RiskHigh) {
		t.Error("HasSeverityAtLeast(RiskHigh) = true, want false")
	}
}

func TestUnifiedList_Filter(t *testing.T) {
	list := UnifiedList{
		{Domain: DomainPornography, Severity: censor.RiskHigh},
		{Domain: DomainViolence, Severity: censor.RiskLow},
		{Domain: DomainAds, Severity: censor.RiskHigh},
	}

	filtered := list.Filter(func(u Unified) bool {
		return u.Severity == censor.RiskHigh
	})

	if len(filtered) != 2 {
		t.Errorf("Filter() returned %d items, want 2", len(filtered))
	}
}

func TestUnifiedList_DecideOutcome(t *testing.T) {
	tests := []struct {
		name             string
		list             UnifiedList
		expectedDecision censor.Decision
	}{
		{
			name:             "empty list - pass",
			list:             UnifiedList{},
			expectedDecision: censor.DecisionPass,
		},
		{
			name: "severe - block",
			list: UnifiedList{
				{Domain: DomainPornography, Severity: censor.RiskSevere},
			},
			expectedDecision: censor.DecisionBlock,
		},
		{
			name: "high - block",
			list: UnifiedList{
				{Domain: DomainViolence, Severity: censor.RiskHigh},
			},
			expectedDecision: censor.DecisionBlock,
		},
		{
			name: "medium - review",
			list: UnifiedList{
				{Domain: DomainAds, Severity: censor.RiskMedium},
			},
			expectedDecision: censor.DecisionReview,
		},
		{
			name: "low - review",
			list: UnifiedList{
				{Domain: DomainOther, Severity: censor.RiskLow},
			},
			expectedDecision: censor.DecisionReview,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outcome := tt.list.DecideOutcome()
			if outcome.Decision != tt.expectedDecision {
				t.Errorf("DecideOutcome().Decision = %v, want %v", outcome.Decision, tt.expectedDecision)
			}
		})
	}
}
