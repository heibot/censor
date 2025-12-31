package violation

import censor "github.com/heibot/censor"

// TranslationContext provides context for translating provider results.
type TranslationContext struct {
	ResourceType censor.ResourceType
	BizType      censor.BizType
}

// Translator translates provider-specific labels to unified violations.
type Translator interface {
	// Provider returns the provider name this translator handles.
	Provider() string

	// Translate converts provider-specific labels to unified violations.
	Translate(ctx TranslationContext, labels []string, scores map[string]float64) UnifiedList
}

// BaseTranslator provides common translation functionality.
type BaseTranslator struct {
	providerName string
	labelMap     map[string]LabelMapping
}

// LabelMapping maps a provider label to unified domain and tags.
type LabelMapping struct {
	Domain     Domain
	Tags       []Tag
	Severity   censor.RiskLevel
	Confidence float64 // Default confidence if not provided
}

// NewBaseTranslator creates a new base translator.
func NewBaseTranslator(provider string, labelMap map[string]LabelMapping) *BaseTranslator {
	return &BaseTranslator{
		providerName: provider,
		labelMap:     labelMap,
	}
}

// Provider returns the provider name.
func (t *BaseTranslator) Provider() string {
	return t.providerName
}

// Translate converts labels to unified violations.
func (t *BaseTranslator) Translate(ctx TranslationContext, labels []string, scores map[string]float64) UnifiedList {
	var violations UnifiedList

	for _, label := range labels {
		mapping, ok := t.labelMap[label]
		if !ok {
			// Unknown label, map to other
			violations = append(violations, Unified{
				Domain:          DomainOther,
				Tags:            []Tag{TagCustom},
				Severity:        censor.RiskLow,
				Confidence:      0.5,
				SourceProviders: []string{t.providerName},
				OriginalLabels:  []string{label},
			})
			continue
		}

		confidence := mapping.Confidence
		if score, ok := scores[label]; ok {
			confidence = score
		}

		// Adjust severity based on context
		severity := t.adjustSeverity(mapping.Severity, ctx)

		violations = append(violations, Unified{
			Domain:          mapping.Domain,
			Tags:            mapping.Tags,
			Severity:        severity,
			Confidence:      confidence,
			SourceProviders: []string{t.providerName},
			OriginalLabels:  []string{label},
		})
	}

	return violations
}

// adjustSeverity adjusts severity based on context.
func (t *BaseTranslator) adjustSeverity(base censor.RiskLevel, ctx TranslationContext) censor.RiskLevel {
	// Stricter for user profile fields
	switch ctx.BizType {
	case censor.BizUserNickname, censor.BizUserAvatar, censor.BizUserBio:
		if base == censor.RiskMedium {
			return censor.RiskHigh
		}
	case censor.BizChatMessage, censor.BizDanmaku:
		// More lenient for real-time messages
		if base == censor.RiskHigh {
			return censor.RiskMedium
		}
	}
	return base
}

// MergeViolations merges violations from multiple providers.
func MergeViolations(lists ...UnifiedList) UnifiedList {
	domainMap := make(map[Domain]*Unified)

	for _, list := range lists {
		for _, v := range list {
			if existing, ok := domainMap[v.Domain]; ok {
				// Merge: take higher severity, merge tags and providers
				if v.Severity > existing.Severity {
					existing.Severity = v.Severity
				}
				if v.Confidence > existing.Confidence {
					existing.Confidence = v.Confidence
				}
				existing.Tags = mergeTags(existing.Tags, v.Tags)
				existing.SourceProviders = mergeStrings(existing.SourceProviders, v.SourceProviders)
				existing.OriginalLabels = mergeStrings(existing.OriginalLabels, v.OriginalLabels)
			} else {
				copy := v
				domainMap[v.Domain] = &copy
			}
		}
	}

	result := make(UnifiedList, 0, len(domainMap))
	for _, v := range domainMap {
		result = append(result, *v)
	}
	return result
}

func mergeTags(a, b []Tag) []Tag {
	seen := make(map[Tag]bool)
	for _, t := range a {
		seen[t] = true
	}
	result := append([]Tag{}, a...)
	for _, t := range b {
		if !seen[t] {
			result = append(result, t)
		}
	}
	return result
}

func mergeStrings(a, b []string) []string {
	seen := make(map[string]bool)
	for _, s := range a {
		seen[s] = true
	}
	result := append([]string{}, a...)
	for _, s := range b {
		if !seen[s] {
			result = append(result, s)
		}
	}
	return result
}
