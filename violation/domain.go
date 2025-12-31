// Package violation provides unified violation domain definitions
// that abstract away differences between cloud providers.
package violation

import censor "github.com/heibot/censor"

// Domain represents high-level violation categories.
// These are platform-agnostic and used for decision making.
type Domain string

const (
	// Basic compliance
	DomainPornography Domain = "pornography"
	DomainSexualHint  Domain = "sexual_hint"
	DomainViolence    Domain = "violence"
	DomainTerrorism   Domain = "terrorism"
	DomainPolitics    Domain = "politics"
	DomainIllegal     Domain = "illegal"
	DomainFraud       Domain = "fraud"
	DomainGambling    Domain = "gambling"
	DomainDrugs       Domain = "drugs"

	// Community governance
	DomainHateSpeech  Domain = "hate_speech"
	DomainHarassment  Domain = "harassment"
	DomainAbuse       Domain = "abuse"
	DomainMinorSafety Domain = "minor_safety"

	// Platform security
	DomainSpam        Domain = "spam"
	DomainAds         Domain = "ads"
	DomainScam        Domain = "scam"
	DomainAccountRisk Domain = "account_risk"

	// Fallback
	DomainOther Domain = "other"
)

// DomainInfo provides metadata about a violation domain.
type DomainInfo struct {
	Domain      Domain
	Name        string
	Description string
	DefaultRisk censor.RiskLevel
}

// DomainRegistry maps domains to their metadata.
var DomainRegistry = map[Domain]DomainInfo{
	DomainPornography: {
		Domain:      DomainPornography,
		Name:        "Pornography",
		Description: "Sexually explicit content",
		DefaultRisk: censor.RiskSevere,
	},
	DomainSexualHint: {
		Domain:      DomainSexualHint,
		Name:        "Sexual Hint",
		Description: "Suggestive or sexually implicit content",
		DefaultRisk: censor.RiskMedium,
	},
	DomainViolence: {
		Domain:      DomainViolence,
		Name:        "Violence",
		Description: "Violent or graphic content",
		DefaultRisk: censor.RiskHigh,
	},
	DomainTerrorism: {
		Domain:      DomainTerrorism,
		Name:        "Terrorism",
		Description: "Terrorist-related content",
		DefaultRisk: censor.RiskSevere,
	},
	DomainPolitics: {
		Domain:      DomainPolitics,
		Name:        "Politics",
		Description: "Politically sensitive content",
		DefaultRisk: censor.RiskHigh,
	},
	DomainIllegal: {
		Domain:      DomainIllegal,
		Name:        "Illegal",
		Description: "Content promoting illegal activities",
		DefaultRisk: censor.RiskSevere,
	},
	DomainFraud: {
		Domain:      DomainFraud,
		Name:        "Fraud",
		Description: "Fraudulent or deceptive content",
		DefaultRisk: censor.RiskHigh,
	},
	DomainGambling: {
		Domain:      DomainGambling,
		Name:        "Gambling",
		Description: "Gambling-related content",
		DefaultRisk: censor.RiskMedium,
	},
	DomainDrugs: {
		Domain:      DomainDrugs,
		Name:        "Drugs",
		Description: "Drug-related content",
		DefaultRisk: censor.RiskHigh,
	},
	DomainHateSpeech: {
		Domain:      DomainHateSpeech,
		Name:        "Hate Speech",
		Description: "Hateful or discriminatory content",
		DefaultRisk: censor.RiskHigh,
	},
	DomainHarassment: {
		Domain:      DomainHarassment,
		Name:        "Harassment",
		Description: "Harassing or bullying content",
		DefaultRisk: censor.RiskMedium,
	},
	DomainAbuse: {
		Domain:      DomainAbuse,
		Name:        "Abuse",
		Description: "Abusive content",
		DefaultRisk: censor.RiskHigh,
	},
	DomainMinorSafety: {
		Domain:      DomainMinorSafety,
		Name:        "Minor Safety",
		Description: "Content endangering minors",
		DefaultRisk: censor.RiskSevere,
	},
	DomainSpam: {
		Domain:      DomainSpam,
		Name:        "Spam",
		Description: "Spam or unsolicited content",
		DefaultRisk: censor.RiskLow,
	},
	DomainAds: {
		Domain:      DomainAds,
		Name:        "Ads",
		Description: "Unauthorized advertising",
		DefaultRisk: censor.RiskLow,
	},
	DomainScam: {
		Domain:      DomainScam,
		Name:        "Scam",
		Description: "Scam or phishing content",
		DefaultRisk: censor.RiskHigh,
	},
	DomainAccountRisk: {
		Domain:      DomainAccountRisk,
		Name:        "Account Risk",
		Description: "Account-level risk indicators",
		DefaultRisk: censor.RiskMedium,
	},
	DomainOther: {
		Domain:      DomainOther,
		Name:        "Other",
		Description: "Other violations",
		DefaultRisk: censor.RiskLow,
	},
}

// GetDomainInfo returns the metadata for a domain.
func GetDomainInfo(d Domain) DomainInfo {
	if info, ok := DomainRegistry[d]; ok {
		return info
	}
	return DomainRegistry[DomainOther]
}
