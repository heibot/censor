package violation

import censor "github.com/heibot/censor"

// UnifiedScene represents a platform-agnostic detection scene.
// This is "what we want to detect", not "what was detected".
type UnifiedScene string

const (
	// Basic compliance scenes
	ScenePornography UnifiedScene = "pornography" // 色情检测
	SceneTerrorism   UnifiedScene = "terrorism"   // 暴恐检测
	ScenePolitics    UnifiedScene = "politics"    // 涉政检测
	SceneViolence    UnifiedScene = "violence"    // 暴力检测
	SceneBan         UnifiedScene = "ban"         // 违禁检测

	// Community governance scenes
	SceneAbuse      UnifiedScene = "abuse"       // 辱骂检测
	SceneHateSpeech UnifiedScene = "hate_speech" // 仇恨言论检测
	SceneHarassment UnifiedScene = "harassment"  // 骚扰检测

	// Platform security scenes
	SceneAds     UnifiedScene = "ads"     // 广告检测
	SceneSpam    UnifiedScene = "spam"    // 垃圾信息检测
	SceneFraud   UnifiedScene = "fraud"   // 诈骗检测
	ScenePrivacy UnifiedScene = "privacy" // 隐私信息检测

	// Content quality scenes
	SceneMeaningless UnifiedScene = "meaningless" // 无意义内容检测
	SceneFlood       UnifiedScene = "flood"       // 灌水检测

	// Special scenes
	SceneMinor        UnifiedScene = "minor"         // 未成年人相关检测
	SceneMoan         UnifiedScene = "moan"          // 娇喘音频检测
	SceneQRCode       UnifiedScene = "qrcode"        // 二维码检测
	SceneImageText    UnifiedScene = "image_text"    // 图文违规检测
	SceneCustom       UnifiedScene = "custom"        // 自定义词库检测
	SceneAdLaw        UnifiedScene = "ad_law"        // 广告法合规检测
	ScenePublicFigure UnifiedScene = "public_figure" // 公众人物识别
)

// SceneInfo provides metadata about a scene.
type SceneInfo struct {
	Scene       UnifiedScene
	Name        string // Chinese name
	NameEN      string // English name
	Description string
	Domains     []Domain // Related violation domains
}

// SceneRegistry maps scenes to their metadata.
var SceneRegistry = map[UnifiedScene]SceneInfo{
	ScenePornography: {
		Scene:       ScenePornography,
		Name:        "色情检测",
		NameEN:      "Pornography Detection",
		Description: "检测色情、性感违规内容",
		Domains:     []Domain{DomainPornography, DomainSexualHint},
	},
	SceneTerrorism: {
		Scene:       SceneTerrorism,
		Name:        "暴恐检测",
		NameEN:      "Terrorism Detection",
		Description: "检测暴力恐怖相关内容",
		Domains:     []Domain{DomainTerrorism},
	},
	ScenePolitics: {
		Scene:       ScenePolitics,
		Name:        "涉政检测",
		NameEN:      "Politics Detection",
		Description: "检测政治敏感内容",
		Domains:     []Domain{DomainPolitics},
	},
	SceneViolence: {
		Scene:       SceneViolence,
		Name:        "暴力检测",
		NameEN:      "Violence Detection",
		Description: "检测暴力血腥内容",
		Domains:     []Domain{DomainViolence},
	},
	SceneBan: {
		Scene:       SceneBan,
		Name:        "违禁检测",
		NameEN:      "Contraband Detection",
		Description: "检测违禁物品和内容",
		Domains:     []Domain{DomainIllegal},
	},
	SceneAbuse: {
		Scene:       SceneAbuse,
		Name:        "辱骂检测",
		NameEN:      "Abuse Detection",
		Description: "检测辱骂、攻击性内容",
		Domains:     []Domain{DomainAbuse, DomainHarassment},
	},
	SceneAds: {
		Scene:       SceneAds,
		Name:        "广告检测",
		NameEN:      "Ads Detection",
		Description: "检测广告推广内容",
		Domains:     []Domain{DomainAds, DomainSpam},
	},
	SceneSpam: {
		Scene:       SceneSpam,
		Name:        "垃圾检测",
		NameEN:      "Spam Detection",
		Description: "检测垃圾信息和刷屏内容",
		Domains:     []Domain{DomainSpam},
	},
	SceneFraud: {
		Scene:       SceneFraud,
		Name:        "诈骗检测",
		NameEN:      "Fraud Detection",
		Description: "检测诈骗、钓鱼内容",
		Domains:     []Domain{DomainFraud, DomainScam},
	},
	ScenePrivacy: {
		Scene:       ScenePrivacy,
		Name:        "隐私检测",
		NameEN:      "Privacy Detection",
		Description: "检测隐私信息泄露",
		Domains:     []Domain{DomainAccountRisk},
	},
	SceneMeaningless: {
		Scene:       SceneMeaningless,
		Name:        "无意义检测",
		NameEN:      "Meaningless Detection",
		Description: "检测无意义、乱码内容",
		Domains:     []Domain{DomainSpam},
	},
	SceneMinor: {
		Scene:       SceneMinor,
		Name:        "未成年人检测",
		NameEN:      "Minor Safety Detection",
		Description: "检测涉及未成年人的不当内容",
		Domains:     []Domain{DomainMinorSafety},
	},
	SceneMoan: {
		Scene:       SceneMoan,
		Name:        "娇喘检测",
		NameEN:      "Moan Detection",
		Description: "检测娇喘等音频内容",
		Domains:     []Domain{DomainSexualHint},
	},
	SceneQRCode: {
		Scene:       SceneQRCode,
		Name:        "二维码检测",
		NameEN:      "QR Code Detection",
		Description: "检测二维码内容",
		Domains:     []Domain{DomainSpam, DomainAds},
	},
	SceneImageText: {
		Scene:       SceneImageText,
		Name:        "图文检测",
		NameEN:      "Image Text Detection",
		Description: "检测图片中的文字违规内容",
		Domains:     []Domain{DomainOther},
	},
}

// ReviewRequirement defines what scenes a business type requires.
type ReviewRequirement struct {
	Scenes   []UnifiedScene // Required detection scenes
	Strict   bool           // If true, all scenes must be supported
	Priority int            // Higher = more urgent
}

// BizReviewRequirements maps business types to their review requirements.
var BizReviewRequirements = map[censor.BizType]ReviewRequirement{
	// User profile - need basic safety checks
	censor.BizUserNickname: {
		Scenes: []UnifiedScene{ScenePornography, ScenePolitics, SceneAbuse, SceneAds},
	},
	censor.BizUserAvatar: {
		Scenes: []UnifiedScene{ScenePornography, ScenePolitics, SceneTerrorism},
	},
	censor.BizUserBio: {
		Scenes: []UnifiedScene{ScenePornography, ScenePolitics, SceneAbuse, SceneAds, ScenePrivacy},
	},

	// Notes/Posts - comprehensive checks
	censor.BizNoteTitle: {
		Scenes: []UnifiedScene{ScenePornography, ScenePolitics, SceneAbuse, SceneAds, SceneBan},
	},
	censor.BizNoteBody: {
		Scenes: []UnifiedScene{ScenePornography, ScenePolitics, SceneViolence, SceneTerrorism, SceneAbuse, SceneAds, SceneFraud, SceneBan},
	},
	censor.BizNoteImages: {
		Scenes: []UnifiedScene{ScenePornography, ScenePolitics, SceneTerrorism, SceneViolence, SceneAds, SceneQRCode, SceneImageText},
	},
	censor.BizNoteVideos: {
		Scenes: []UnifiedScene{ScenePornography, ScenePolitics, SceneTerrorism, SceneViolence, SceneMoan},
	},

	// Team - moderate checks
	censor.BizTeamName: {
		Scenes: []UnifiedScene{ScenePornography, ScenePolitics, SceneAbuse},
	},
	censor.BizTeamIntro: {
		Scenes: []UnifiedScene{ScenePornography, ScenePolitics, SceneAbuse, SceneAds},
	},
	censor.BizTeamBgImage: {
		Scenes: []UnifiedScene{ScenePornography, ScenePolitics, SceneTerrorism},
	},

	// Real-time communication - fast checks
	censor.BizChatMessage: {
		Scenes:   []UnifiedScene{ScenePornography, ScenePolitics, SceneTerrorism, SceneFraud},
		Priority: 10,
	},
	censor.BizDanmaku: {
		Scenes:   []UnifiedScene{ScenePornography, ScenePolitics, SceneAbuse},
		Priority: 10,
	},
	censor.BizComment: {
		Scenes: []UnifiedScene{ScenePornography, ScenePolitics, SceneAbuse, SceneSpam},
	},
}

// GetReviewRequirement returns the review requirement for a business type.
func GetReviewRequirement(bizType censor.BizType) ReviewRequirement {
	if req, ok := BizReviewRequirements[bizType]; ok {
		return req
	}
	// Default requirement
	return ReviewRequirement{
		Scenes: []UnifiedScene{ScenePornography, ScenePolitics},
	}
}

// SetReviewRequirement sets a custom review requirement for a business type.
func SetReviewRequirement(bizType censor.BizType, req ReviewRequirement) {
	BizReviewRequirements[bizType] = req
}
