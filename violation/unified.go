package violation

import censor "github.com/heibot/censor"

// Unified represents a platform-agnostic violation.
// This is the internal representation used for decision making.
type Unified struct {
	Domain          Domain           `json:"domain"`           // High-level category
	Tags            []Tag            `json:"tags"`             // Specific tags
	Severity        censor.RiskLevel `json:"severity"`         // Risk level
	Confidence      float64          `json:"confidence"`       // Confidence score (0-1)
	SourceProviders []string         `json:"source_providers"` // Which providers detected this
	OriginalLabels  []string         `json:"original_labels"`  // Original provider labels
}

// UnifiedList is a collection of unified violations.
type UnifiedList []Unified

// GetHighestSeverity returns the highest severity from a list of violations.
func (ul UnifiedList) GetHighestSeverity() censor.RiskLevel {
	highest := censor.RiskLow
	for _, v := range ul {
		if v.Severity > highest {
			highest = v.Severity
		}
	}
	return highest
}

// GetDomains returns all unique domains from a list of violations.
func (ul UnifiedList) GetDomains() []Domain {
	seen := make(map[Domain]bool)
	var domains []Domain
	for _, v := range ul {
		if !seen[v.Domain] {
			seen[v.Domain] = true
			domains = append(domains, v.Domain)
		}
	}
	return domains
}

// GetAllTags returns all unique tags from a list of violations.
func (ul UnifiedList) GetAllTags() []Tag {
	seen := make(map[Tag]bool)
	var tags []Tag
	for _, v := range ul {
		for _, t := range v.Tags {
			if !seen[t] {
				seen[t] = true
				tags = append(tags, t)
			}
		}
	}
	return tags
}

// HasDomain checks if any violation has the given domain.
func (ul UnifiedList) HasDomain(d Domain) bool {
	for _, v := range ul {
		if v.Domain == d {
			return true
		}
	}
	return false
}

// HasSeverityAtLeast checks if any violation has at least the given severity.
func (ul UnifiedList) HasSeverityAtLeast(level censor.RiskLevel) bool {
	for _, v := range ul {
		if v.Severity >= level {
			return true
		}
	}
	return false
}

// Filter returns violations matching the given predicate.
func (ul UnifiedList) Filter(predicate func(Unified) bool) UnifiedList {
	var result UnifiedList
	for _, v := range ul {
		if predicate(v) {
			result = append(result, v)
		}
	}
	return result
}

// DecideOutcome converts violations to a final outcome.
func (ul UnifiedList) DecideOutcome() censor.FinalOutcome {
	if len(ul) == 0 {
		return censor.FinalOutcome{
			Decision:      censor.DecisionPass,
			ReplacePolicy: censor.ReplacePolicyNone,
			RiskLevel:     censor.RiskLow,
		}
	}

	highestSeverity := ul.GetHighestSeverity()
	var decision censor.Decision
	var replacePolicy censor.ReplacePolicy

	switch highestSeverity {
	case censor.RiskSevere:
		decision = censor.DecisionBlock
		replacePolicy = censor.ReplacePolicyNone
	case censor.RiskHigh:
		decision = censor.DecisionBlock
		replacePolicy = censor.ReplacePolicyDefault
	case censor.RiskMedium:
		decision = censor.DecisionReview
		replacePolicy = censor.ReplacePolicyMask
	default:
		decision = censor.DecisionReview
		replacePolicy = censor.ReplacePolicyNone
	}

	// Convert violations to reasons
	var reasons []censor.Reason
	for _, v := range ul {
		reasons = append(reasons, censor.Reason{
			Code:    string(v.Domain),
			Message: GetDomainInfo(v.Domain).Description,
			HitTags: tagsToStrings(v.Tags),
		})
	}

	return censor.FinalOutcome{
		Decision:      decision,
		ReplacePolicy: replacePolicy,
		Reasons:       reasons,
		RiskLevel:     highestSeverity,
	}
}

func tagsToStrings(tags []Tag) []string {
	result := make([]string, len(tags))
	for i, t := range tags {
		result[i] = string(t)
	}
	return result
}
