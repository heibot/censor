package aliyun

import (
	censor "github.com/heibot/censor"
	"github.com/heibot/censor/violation"
)

type translator struct {
	*violation.BaseTranslator
}

func newTranslator() violation.Translator {
	labelMap := make(map[string]violation.LabelMapping)

	for label, mapping := range labelMappings {
		if mapping.domain == "" {
			continue // Skip pass labels
		}

		tags := make([]violation.Tag, len(mapping.tags))
		for i, t := range mapping.tags {
			tags[i] = violation.Tag(t)
		}

		labelMap[label] = violation.LabelMapping{
			Domain:     violation.Domain(mapping.domain),
			Tags:       tags,
			Severity:   censor.RiskLevel(mapping.severity),
			Confidence: 0.9,
		}
	}

	return &translator{
		BaseTranslator: violation.NewBaseTranslator(providerName, labelMap),
	}
}
