// Package aliyun provides Alibaba Cloud content moderation integration.
package aliyun

import (
	"time"

	"github.com/heibot/censor/providers"
)

// Config holds the configuration for Aliyun provider.
type Config struct {
	providers.ProviderConfig

	// BizType is the business type for Aliyun Green SDK.
	BizType string

	// Scenes specifies which scenes to check (e.g., "antispam", "porn", "terrorism").
	Scenes []string

	// CallbackURL is the URL for async callback notifications.
	CallbackURL string

	// CallbackSeed is the secret for callback signature verification.
	CallbackSeed string
}

// DefaultConfig returns the default Aliyun configuration.
func DefaultConfig() Config {
	return Config{
		ProviderConfig: providers.ProviderConfig{
			Region:   "cn-shanghai",
			Endpoint: "green.cn-shanghai.aliyuncs.com",
			Timeout:  30 * time.Second,
		},
		Scenes: []string{"antispam"},
	}
}

// Aliyun label to internal mapping
// Based on Aliyun Green SDK documentation
// Text: https://help.aliyun.com/document_detail/70439.html
// Image: https://help.aliyun.com/document_detail/70292.html
var labelMappings = map[string]labelMapping{
	// ============================================================
	// Text Labels (文本标签)
	// ============================================================

	// Pornography (色情)
	"porn":          {domain: "pornography", tags: []string{"pornographic_act"}, severity: 4},
	"sexy":          {domain: "sexual_hint", tags: []string{"nudity"}, severity: 2},
	"sexual":        {domain: "pornography", tags: []string{"sexual_text"}, severity: 3},
	"adult_content": {domain: "pornography", tags: []string{"pornographic_act"}, severity: 4},
	"profanity":     {domain: "pornography", tags: []string{"sexual_text"}, severity: 3},

	// Violence & Terrorism (暴恐)
	"violence":  {domain: "violence", tags: []string{"gore"}, severity: 3},
	"bloody":    {domain: "violence", tags: []string{"blood_content"}, severity: 3},
	"terrorism": {domain: "terrorism", tags: []string{}, severity: 4},
	"terrorist": {domain: "terrorism", tags: []string{}, severity: 4},
	"extremism": {domain: "terrorism", tags: []string{"extremist_content"}, severity: 4},
	"militant":  {domain: "terrorism", tags: []string{}, severity: 4},

	// Politics (涉政)
	"politics":            {domain: "politics", tags: []string{"political_sensitive"}, severity: 3},
	"political_sensitive": {domain: "politics", tags: []string{"political_sensitive"}, severity: 3},
	"leader":              {domain: "politics", tags: []string{"political_leader"}, severity: 3},
	"political_figure":    {domain: "politics", tags: []string{"political_leader"}, severity: 3},
	"national_emblem":     {domain: "politics", tags: []string{"national_symbol"}, severity: 2},
	"national_flag":       {domain: "politics", tags: []string{"national_symbol"}, severity: 2},
	"historical_figure":   {domain: "politics", tags: []string{"historical_event"}, severity: 2},

	// Prohibited (违禁)
	"contraband": {domain: "illegal", tags: []string{}, severity: 4},
	"drug":       {domain: "illegal", tags: []string{"drug_related"}, severity: 4},
	"weapon":     {domain: "illegal", tags: []string{"weapon"}, severity: 3},
	"controlled": {domain: "illegal", tags: []string{}, severity: 3},

	// Spam & Ads (垃圾/广告)
	"spam":         {domain: "spam", tags: []string{}, severity: 1},
	"ad":           {domain: "ads", tags: []string{"spam_ads"}, severity: 1},
	"qrcode":       {domain: "spam", tags: []string{"spam_contact"}, severity: 1},
	"contact_info": {domain: "spam", tags: []string{"spam_contact"}, severity: 2},
	"promotion":    {domain: "ads", tags: []string{"spam_ads"}, severity: 1},
	"marketing":    {domain: "ads", tags: []string{"spam_ads"}, severity: 1},

	// Fraud & Scam (诈骗)
	"fraud":               {domain: "fraud", tags: []string{"fraud_payment"}, severity: 3},
	"gambling":            {domain: "gambling", tags: []string{}, severity: 3},
	"lottery":             {domain: "gambling", tags: []string{}, severity: 2},
	"illegal_transaction": {domain: "fraud", tags: []string{"fraud_payment"}, severity: 3},

	// Abuse & Harassment (辱骂/骚扰)
	"abuse":          {domain: "abuse", tags: []string{}, severity: 2},
	"insult":         {domain: "harassment", tags: []string{}, severity: 2},
	"hate":           {domain: "hate_speech", tags: []string{}, severity: 3},
	"racism":         {domain: "hate_speech", tags: []string{"hate_race"}, severity: 3},
	"discrimination": {domain: "hate_speech", tags: []string{}, severity: 3},
	"threat":         {domain: "harassment", tags: []string{}, severity: 3},

	// Minor Safety (未成年)
	"minor_sexual": {domain: "minor_safety", tags: []string{"minor_sexual"}, severity: 4},
	"child_abuse":  {domain: "minor_safety", tags: []string{"minor_sexual"}, severity: 4},

	// Content Quality (内容质量)
	"meaningless": {domain: "spam", tags: []string{"meaningless"}, severity: 1},
	"flood":       {domain: "spam", tags: []string{"flood"}, severity: 1},
	"gibberish":   {domain: "spam", tags: []string{"meaningless"}, severity: 1},

	// ============================================================
	// Image Labels (图片标签)
	// ============================================================

	// Image-specific pornography
	"nudity":         {domain: "pornography", tags: []string{"nudity"}, severity: 3},
	"partial_nudity": {domain: "sexual_hint", tags: []string{"nudity"}, severity: 2},
	"suggestive":     {domain: "sexual_hint", tags: []string{}, severity: 2},
	"underwear":      {domain: "sexual_hint", tags: []string{}, severity: 1},

	// Image-specific violence
	"gore":      {domain: "violence", tags: []string{"gore"}, severity: 4},
	"corpse":    {domain: "violence", tags: []string{"gore"}, severity: 4},
	"self_harm": {domain: "violence", tags: []string{}, severity: 4},

	// Image-specific politics
	"flag_desecration": {domain: "politics", tags: []string{"national_symbol"}, severity: 3},
	"sensitive_map":    {domain: "politics", tags: []string{"national_symbol"}, severity: 3},

	// Others
	"customized": {domain: "other", tags: []string{"custom"}, severity: 2},
	"logo":       {domain: "other", tags: []string{}, severity: 1},
	"normal":     {domain: "", tags: []string{}, severity: 0}, // Pass
	"pass":       {domain: "", tags: []string{}, severity: 0}, // Pass
}

type labelMapping struct {
	domain   string
	tags     []string
	severity int
}
