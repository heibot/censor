package violation

// Tag represents fine-grained violation labels.
// Tags provide more specific information than domains.
type Tag string

const (
	// Pornography related
	TagNudity          Tag = "nudity"
	TagPornographicAct Tag = "pornographic_act"
	TagMinorSexual     Tag = "minor_sexual"
	TagSexualText      Tag = "sexual_text"

	// Violence related
	TagGore         Tag = "gore"
	TagWeapon       Tag = "weapon"
	TagBloodContent Tag = "blood_content"
	TagSelfHarm     Tag = "self_harm"

	// Politics related
	TagPoliticalSensitive Tag = "political_sensitive"
	TagPoliticalRumor     Tag = "political_rumor"
	TagPoliticalLeader    Tag = "political_leader"
	TagPoliticalSymbol    Tag = "political_symbol"

	// Fraud related
	TagScamImpersonation Tag = "scam_impersonation"
	TagFraudPayment      Tag = "fraud_payment"
	TagPhishing          Tag = "phishing"
	TagFakeInfo          Tag = "fake_info"

	// Hate speech related
	TagHateRace     Tag = "hate_race"
	TagHateGender   Tag = "hate_gender"
	TagHateReligion Tag = "hate_religion"
	TagHateDisabled Tag = "hate_disabled"

	// Spam related
	TagSpamAds     Tag = "spam_ads"
	TagSpamContact Tag = "spam_contact"
	TagSpamLink    Tag = "spam_link"
	TagSpamRepeat  Tag = "spam_repeat"

	// Minor safety
	TagMinorAbuse        Tag = "minor_abuse"
	TagMinorExploitation Tag = "minor_exploitation"

	// Drugs
	TagDrugSale  Tag = "drug_sale"
	TagDrugUse   Tag = "drug_use"
	TagDrugPromo Tag = "drug_promo"

	// Other
	TagCustom Tag = "custom"
)

// TagInfo provides metadata about a tag.
type TagInfo struct {
	Tag         Tag
	Name        string
	Description string
	Domain      Domain // Primary domain this tag belongs to
}

// TagRegistry maps tags to their metadata.
var TagRegistry = map[Tag]TagInfo{
	TagNudity: {
		Tag:         TagNudity,
		Name:        "Nudity",
		Description: "Nude or partially nude content",
		Domain:      DomainPornography,
	},
	TagPornographicAct: {
		Tag:         TagPornographicAct,
		Name:        "Pornographic Act",
		Description: "Explicit sexual acts",
		Domain:      DomainPornography,
	},
	TagMinorSexual: {
		Tag:         TagMinorSexual,
		Name:        "Minor Sexual",
		Description: "Sexual content involving minors",
		Domain:      DomainMinorSafety,
	},
	TagPoliticalSensitive: {
		Tag:         TagPoliticalSensitive,
		Name:        "Political Sensitive",
		Description: "Politically sensitive content",
		Domain:      DomainPolitics,
	},
	TagHateRace: {
		Tag:         TagHateRace,
		Name:        "Racial Hate",
		Description: "Racially discriminatory content",
		Domain:      DomainHateSpeech,
	},
	TagSpamAds: {
		Tag:         TagSpamAds,
		Name:        "Spam Ads",
		Description: "Spam advertising content",
		Domain:      DomainSpam,
	},
}

// GetTagDomain returns the primary domain for a tag.
func GetTagDomain(t Tag) Domain {
	if info, ok := TagRegistry[t]; ok {
		return info.Domain
	}
	return DomainOther
}
