package violation

import censor "github.com/heibot/censor"

// SceneTranslator translates unified scenes to provider-specific scenes.
type SceneTranslator interface {
	// Provider returns the provider name.
	Provider() string

	// TranslateScenes converts unified scenes to provider-specific scene codes.
	TranslateScenes(scenes []UnifiedScene, resourceType censor.ResourceType) []string

	// SupportedScenes returns all scenes this provider supports.
	SupportedScenes(resourceType censor.ResourceType) []UnifiedScene
}

// ProviderCapability declares what a provider can detect.
type ProviderCapability struct {
	Provider        string
	SupportedScenes map[censor.ResourceType][]UnifiedScene
	MaxTextLength   int  // Max text length per request
	SyncSupported   bool // Supports synchronous review
	AsyncSupported  bool // Supports asynchronous review
}

// CanHandle checks if the provider can handle all required scenes.
func (pc ProviderCapability) CanHandle(scenes []UnifiedScene, resourceType censor.ResourceType) bool {
	supported := pc.SupportedScenes[resourceType]
	supportedSet := make(map[UnifiedScene]bool)
	for _, s := range supported {
		supportedSet[s] = true
	}

	for _, scene := range scenes {
		if !supportedSet[scene] {
			return false
		}
	}
	return true
}

// MissingScenes returns scenes that the provider cannot handle.
func (pc ProviderCapability) MissingScenes(scenes []UnifiedScene, resourceType censor.ResourceType) []UnifiedScene {
	supported := pc.SupportedScenes[resourceType]
	supportedSet := make(map[UnifiedScene]bool)
	for _, s := range supported {
		supportedSet[s] = true
	}

	var missing []UnifiedScene
	for _, scene := range scenes {
		if !supportedSet[scene] {
			missing = append(missing, scene)
		}
	}
	return missing
}

// ============================================================
// Huawei Cloud Scene Translator
// ============================================================

// HuaweiSceneTranslator translates scenes for Huawei Cloud.
type HuaweiSceneTranslator struct{}

// HuaweiTextScenes maps unified scenes to Huawei text categories.
var HuaweiTextScenes = map[UnifiedScene]string{
	ScenePornography: "porn",
	SceneTerrorism:   "terrorism",
	ScenePolitics:    "politics",
	SceneBan:         "ban",
	SceneAbuse:       "abuse",
	SceneAds:         "ad",
}

// HuaweiImageScenes maps unified scenes to Huawei image categories.
var HuaweiImageScenes = map[UnifiedScene]string{
	ScenePornography: "porn",
	SceneTerrorism:   "terrorism",
	ScenePolitics:    "politics",
	SceneImageText:   "image_text",
}

// HuaweiVideoImageScenes maps unified scenes to Huawei video image categories.
var HuaweiVideoImageScenes = map[UnifiedScene]string{
	ScenePornography: "porn",
	SceneTerrorism:   "terrorism",
	ScenePolitics:    "politics",
	SceneImageText:   "image_text",
}

// HuaweiVideoAudioScenes maps unified scenes to Huawei video audio categories.
var HuaweiVideoAudioScenes = map[UnifiedScene]string{
	ScenePornography: "porn",
	ScenePolitics:    "politics",
	SceneAds:         "ad",
	SceneMoan:        "moan",
	SceneAbuse:       "abuse",
}

func (t *HuaweiSceneTranslator) Provider() string {
	return "huawei"
}

func (t *HuaweiSceneTranslator) TranslateScenes(scenes []UnifiedScene, resourceType censor.ResourceType) []string {
	var sceneMap map[UnifiedScene]string

	switch resourceType {
	case censor.ResourceText:
		sceneMap = HuaweiTextScenes
	case censor.ResourceImage:
		sceneMap = HuaweiImageScenes
	case censor.ResourceVideo:
		sceneMap = HuaweiVideoImageScenes // Use image scenes for video
	default:
		return nil
	}

	seen := make(map[string]bool)
	var result []string
	for _, scene := range scenes {
		if code, ok := sceneMap[scene]; ok && !seen[code] {
			seen[code] = true
			result = append(result, code)
		}
	}
	return result
}

func (t *HuaweiSceneTranslator) SupportedScenes(resourceType censor.ResourceType) []UnifiedScene {
	var sceneMap map[UnifiedScene]string

	switch resourceType {
	case censor.ResourceText:
		sceneMap = HuaweiTextScenes
	case censor.ResourceImage:
		sceneMap = HuaweiImageScenes
	case censor.ResourceVideo:
		sceneMap = HuaweiVideoImageScenes
	default:
		return nil
	}

	scenes := make([]UnifiedScene, 0, len(sceneMap))
	for scene := range sceneMap {
		scenes = append(scenes, scene)
	}
	return scenes
}

// HuaweiCapability returns Huawei Cloud's provider capability.
func HuaweiCapability() ProviderCapability {
	return ProviderCapability{
		Provider: "huawei",
		SupportedScenes: map[censor.ResourceType][]UnifiedScene{
			censor.ResourceText: {
				ScenePornography, SceneTerrorism, ScenePolitics,
				SceneBan, SceneAbuse, SceneAds,
			},
			censor.ResourceImage: {
				ScenePornography, SceneTerrorism, ScenePolitics, SceneImageText,
			},
			censor.ResourceVideo: {
				ScenePornography, SceneTerrorism, ScenePolitics,
				SceneImageText, SceneMoan, SceneAds, SceneAbuse,
			},
		},
		MaxTextLength:  5000,
		SyncSupported:  true,
		AsyncSupported: true,
	}
}

// ============================================================
// Aliyun Scene Translator
// ============================================================

// AliyunSceneTranslator translates scenes for Aliyun.
type AliyunSceneTranslator struct{}

// AliyunTextScenes - Aliyun uses "antispam" as unified scene, labels for filtering.
var AliyunTextScenes = map[UnifiedScene]string{
	ScenePornography: "porn",
	SceneTerrorism:   "terrorism",
	ScenePolitics:    "politics",
	SceneAbuse:       "abuse",
	SceneAds:         "ad",
	SceneSpam:        "spam",
	SceneBan:         "contraband",
	SceneMeaningless: "meaningless",
	SceneFlood:       "flood",
}

// AliyunImageScenes maps unified scenes to Aliyun image scenes.
var AliyunImageScenes = map[UnifiedScene]string{
	ScenePornography: "porn",
	SceneTerrorism:   "terrorism",
	ScenePolitics:    "politics", // Combined in baseline-check
	SceneAds:         "ad",
	SceneQRCode:      "qrcode",
}

func (t *AliyunSceneTranslator) Provider() string {
	return "aliyun"
}

func (t *AliyunSceneTranslator) TranslateScenes(scenes []UnifiedScene, resourceType censor.ResourceType) []string {
	// Aliyun text uses "antispam" scene, filtering happens via labels
	if resourceType == censor.ResourceText {
		return []string{"antispam"}
	}

	var sceneMap map[UnifiedScene]string
	switch resourceType {
	case censor.ResourceImage:
		sceneMap = AliyunImageScenes
	case censor.ResourceVideo:
		// Video uses same scenes as image
		sceneMap = AliyunImageScenes
	default:
		return nil
	}

	seen := make(map[string]bool)
	var result []string
	for _, scene := range scenes {
		if code, ok := sceneMap[scene]; ok && !seen[code] {
			seen[code] = true
			result = append(result, code)
		}
	}
	return result
}

func (t *AliyunSceneTranslator) SupportedScenes(resourceType censor.ResourceType) []UnifiedScene {
	switch resourceType {
	case censor.ResourceText:
		return []UnifiedScene{
			ScenePornography, SceneTerrorism, ScenePolitics, SceneAbuse,
			SceneAds, SceneSpam, SceneBan, SceneMeaningless, SceneFlood,
		}
	case censor.ResourceImage, censor.ResourceVideo:
		return []UnifiedScene{
			ScenePornography, SceneTerrorism, ScenePolitics, SceneAds, SceneQRCode,
		}
	default:
		return nil
	}
}

// AliyunCapability returns Aliyun's provider capability.
func AliyunCapability() ProviderCapability {
	return ProviderCapability{
		Provider: "aliyun",
		SupportedScenes: map[censor.ResourceType][]UnifiedScene{
			censor.ResourceText: {
				ScenePornography, SceneTerrorism, ScenePolitics, SceneAbuse,
				SceneAds, SceneSpam, SceneBan, SceneMeaningless, SceneFlood,
			},
			censor.ResourceImage: {
				ScenePornography, SceneTerrorism, ScenePolitics, SceneAds, SceneQRCode,
			},
			censor.ResourceVideo: {
				ScenePornography, SceneTerrorism, ScenePolitics, SceneAds,
			},
		},
		MaxTextLength:  10000,
		SyncSupported:  true,
		AsyncSupported: true,
	}
}

// ============================================================
// Tencent Scene Translator
// ============================================================

// TencentSceneTranslator translates scenes for Tencent Cloud.
type TencentSceneTranslator struct{}

// TencentTextScenes maps unified scenes to Tencent text scenes.
var TencentTextScenes = map[UnifiedScene]string{
	ScenePornography: "Porn",
	SceneTerrorism:   "Terror",
	ScenePolitics:    "Polity",
	SceneAbuse:       "Abuse",
	SceneAds:         "Ad",
	SceneSpam:        "Spam",
	SceneBan:         "Illegal",
}

// TencentImageScenes maps unified scenes to Tencent image scenes.
var TencentImageScenes = map[UnifiedScene]string{
	ScenePornography: "Porn",
	SceneTerrorism:   "Terror",
	ScenePolitics:    "Polity",
	SceneViolence:    "Violence",
	SceneAds:         "Ad",
}

func (t *TencentSceneTranslator) Provider() string {
	return "tencent"
}

func (t *TencentSceneTranslator) TranslateScenes(scenes []UnifiedScene, resourceType censor.ResourceType) []string {
	var sceneMap map[UnifiedScene]string

	switch resourceType {
	case censor.ResourceText:
		sceneMap = TencentTextScenes
	case censor.ResourceImage, censor.ResourceVideo:
		sceneMap = TencentImageScenes
	default:
		return nil
	}

	seen := make(map[string]bool)
	var result []string
	for _, scene := range scenes {
		if code, ok := sceneMap[scene]; ok && !seen[code] {
			seen[code] = true
			result = append(result, code)
		}
	}
	return result
}

func (t *TencentSceneTranslator) SupportedScenes(resourceType censor.ResourceType) []UnifiedScene {
	switch resourceType {
	case censor.ResourceText:
		return []UnifiedScene{
			ScenePornography, SceneTerrorism, ScenePolitics, SceneAbuse, SceneAds, SceneSpam, SceneBan,
		}
	case censor.ResourceImage, censor.ResourceVideo:
		return []UnifiedScene{
			ScenePornography, SceneTerrorism, ScenePolitics, SceneViolence, SceneAds,
		}
	default:
		return nil
	}
}

// TencentCapability returns Tencent Cloud's provider capability.
func TencentCapability() ProviderCapability {
	return ProviderCapability{
		Provider: "tencent",
		SupportedScenes: map[censor.ResourceType][]UnifiedScene{
			censor.ResourceText: {
				ScenePornography, SceneTerrorism, ScenePolitics, SceneAbuse, SceneAds, SceneSpam, SceneBan,
			},
			censor.ResourceImage: {
				ScenePornography, SceneTerrorism, ScenePolitics, SceneViolence, SceneAds,
			},
			censor.ResourceVideo: {
				ScenePornography, SceneTerrorism, ScenePolitics, SceneViolence, SceneAds,
			},
		},
		MaxTextLength:  10000,
		SyncSupported:  true,
		AsyncSupported: true,
	}
}

// ============================================================
// Shumei (数美) Scene Translator
// ============================================================

// ShumeiSceneTranslator translates scenes for Shumei.
type ShumeiSceneTranslator struct{}

// ShumeiTextScenes maps unified scenes to Shumei text types.
var ShumeiTextScenes = map[UnifiedScene]string{
	ScenePornography:  "EROTIC",
	SceneTerrorism:    "VIOLENT",
	ScenePolitics:     "POLITY",
	SceneBan:          "BAN",
	SceneAbuse:        "DIRTY",
	SceneAds:          "ADVERT",
	ScenePrivacy:      "PRIVACY",
	SceneAdLaw:        "ADLAW",
	SceneMeaningless:  "MEANINGLESS",
	SceneFraud:        "FRUAD",
	SceneMinor:        "TEXTMINOR",
	ScenePublicFigure: "PUBLICFIGURE",
}

// ShumeiImageScenes maps unified scenes to Shumei image types.
var ShumeiImageScenes = map[UnifiedScene]string{
	ScenePornography: "EROTIC",
	SceneTerrorism:   "VIOLENT",
	ScenePolitics:    "POLITY",
	SceneAds:         "ADVERT",
	SceneQRCode:      "QRCODE",
	SceneImageText:   "IMGTEXTRISK",
}

func (t *ShumeiSceneTranslator) Provider() string {
	return "shumei"
}

func (t *ShumeiSceneTranslator) TranslateScenes(scenes []UnifiedScene, resourceType censor.ResourceType) []string {
	var sceneMap map[UnifiedScene]string

	switch resourceType {
	case censor.ResourceText:
		sceneMap = ShumeiTextScenes
	case censor.ResourceImage, censor.ResourceVideo:
		sceneMap = ShumeiImageScenes
	default:
		return nil
	}

	seen := make(map[string]bool)
	var result []string
	for _, scene := range scenes {
		if code, ok := sceneMap[scene]; ok && !seen[code] {
			seen[code] = true
			result = append(result, code)
		}
	}
	return result
}

func (t *ShumeiSceneTranslator) SupportedScenes(resourceType censor.ResourceType) []UnifiedScene {
	switch resourceType {
	case censor.ResourceText:
		return []UnifiedScene{
			ScenePornography, SceneTerrorism, ScenePolitics, SceneBan, SceneAbuse,
			SceneAds, ScenePrivacy, SceneAdLaw, SceneMeaningless, SceneFraud,
			SceneMinor, ScenePublicFigure,
		}
	case censor.ResourceImage, censor.ResourceVideo:
		return []UnifiedScene{
			ScenePornography, SceneTerrorism, ScenePolitics, SceneAds, SceneQRCode, SceneImageText,
		}
	default:
		return nil
	}
}

// ShumeiCapability returns Shumei's provider capability.
func ShumeiCapability() ProviderCapability {
	return ProviderCapability{
		Provider: "shumei",
		SupportedScenes: map[censor.ResourceType][]UnifiedScene{
			censor.ResourceText: {
				ScenePornography, SceneTerrorism, ScenePolitics, SceneBan, SceneAbuse,
				SceneAds, ScenePrivacy, SceneAdLaw, SceneMeaningless, SceneFraud,
				SceneMinor, ScenePublicFigure,
			},
			censor.ResourceImage: {
				ScenePornography, SceneTerrorism, ScenePolitics, SceneAds, SceneQRCode, SceneImageText,
			},
			censor.ResourceVideo: {
				ScenePornography, SceneTerrorism, ScenePolitics, SceneAds, SceneImageText,
			},
		},
		MaxTextLength:  10000,
		SyncSupported:  true,
		AsyncSupported: true,
	}
}
