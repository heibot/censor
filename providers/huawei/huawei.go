// Package huawei provides Huawei Cloud content moderation integration.
package huawei

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	moderation "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/moderation/v3"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/moderation/v3/model"
	region "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/moderation/v3/region"

	censor "github.com/heibot/censor"
	"github.com/heibot/censor/providers"
	"github.com/heibot/censor/violation"
)

const providerName = "huawei"

// Config holds the configuration for Huawei provider.
type Config struct {
	providers.ProviderConfig

	ProjectID   string
	CallbackURL string
	CallbackKey string
}

// DefaultConfig returns the default Huawei configuration.
func DefaultConfig() Config {
	return Config{
		ProviderConfig: providers.ProviderConfig{
			Region:   "cn-north-4",
			Endpoint: "moderation.cn-north-4.myhuaweicloud.com",
			Timeout:  30 * time.Second,
		},
	}
}

// Provider implements the Huawei content moderation provider.
type Provider struct {
	config     Config
	client     *moderation.ModerationClient
	translator violation.Translator
}

// New creates a new Huawei provider.
func New(cfg Config) (*Provider, error) {
	p := &Provider{
		config:     cfg,
		translator: newTranslator(),
	}

	if err := p.initClient(); err != nil {
		return nil, fmt.Errorf("failed to init huawei client: %w", err)
	}

	return p, nil
}

func (p *Provider) initClient() error {
	auth := basic.NewCredentialsBuilder().
		WithAk(p.config.AccessKeyID).
		WithSk(p.config.AccessKeySecret).
		WithProjectId(p.config.ProjectID).
		Build()

	reg, err := region.SafeValueOf(p.config.Region)
	if err != nil {
		return fmt.Errorf("invalid region: %w", err)
	}

	client := moderation.NewModerationClient(
		moderation.ModerationClientBuilder().
			WithRegion(reg).
			WithCredential(auth).
			Build())

	p.client = client
	return nil
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return providerName
}

// Capabilities returns the supported capabilities.
func (p *Provider) Capabilities() []providers.Capability {
	return []providers.Capability{
		{
			ResourceType: censor.ResourceText,
			Modes:        []providers.Mode{providers.ModeSync},
		},
		{
			ResourceType: censor.ResourceImage,
			Modes:        []providers.Mode{providers.ModeSync, providers.ModeAsync},
		},
		{
			ResourceType: censor.ResourceVideo,
			Modes:        []providers.Mode{providers.ModeAsync},
		},
	}
}

// SceneCapability returns the detection scene capabilities for Huawei.
func (p *Provider) SceneCapability() providers.SceneCapability {
	return providers.SceneCapability{
		Provider: providerName,
		SupportedScenes: map[censor.ResourceType][]violation.UnifiedScene{
			censor.ResourceText: {
				violation.ScenePornography, violation.SceneTerrorism, violation.ScenePolitics,
				violation.SceneBan, violation.SceneAbuse, violation.SceneAds,
			},
			censor.ResourceImage: {
				violation.ScenePornography, violation.SceneTerrorism, violation.ScenePolitics,
				violation.SceneImageText,
			},
			censor.ResourceVideo: {
				violation.ScenePornography, violation.SceneTerrorism, violation.ScenePolitics,
				violation.SceneImageText, violation.SceneMoan, violation.SceneAds, violation.SceneAbuse,
			},
		},
		MaxTextLength:  5000,
		SyncSupported:  true,
		AsyncSupported: true,
	}
}

// TranslateScenes converts unified scenes to Huawei-specific scene codes.
func (p *Provider) TranslateScenes(scenes []violation.UnifiedScene, resourceType censor.ResourceType) []string {
	var sceneMap map[violation.UnifiedScene]string

	switch resourceType {
	case censor.ResourceText:
		sceneMap = map[violation.UnifiedScene]string{
			violation.ScenePornography: "porn",
			violation.SceneTerrorism:   "terrorism",
			violation.ScenePolitics:    "politics",
			violation.SceneBan:         "ban",
			violation.SceneAbuse:       "abuse",
			violation.SceneAds:         "ad",
		}
	case censor.ResourceImage:
		sceneMap = map[violation.UnifiedScene]string{
			violation.ScenePornography: "porn",
			violation.SceneTerrorism:   "terrorism",
			violation.ScenePolitics:    "politics",
			violation.SceneImageText:   "image_text",
		}
	case censor.ResourceVideo:
		sceneMap = map[violation.UnifiedScene]string{
			violation.ScenePornography: "porn",
			violation.SceneTerrorism:   "terrorism",
			violation.ScenePolitics:    "politics",
			violation.SceneImageText:   "image_text",
		}
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

// Submit submits content for review.
func (p *Provider) Submit(ctx context.Context, req providers.SubmitRequest) (providers.SubmitResponse, error) {
	switch req.Resource.Type {
	case censor.ResourceText:
		return p.submitText(ctx, req)
	case censor.ResourceImage:
		return p.submitImage(ctx, req)
	case censor.ResourceVideo:
		return p.submitVideo(ctx, req)
	default:
		return providers.SubmitResponse{}, censor.ErrUnsupportedType
	}
}

func (p *Provider) submitText(ctx context.Context, req providers.SubmitRequest) (providers.SubmitResponse, error) {
	eventType := p.getTextEventType(req.Biz.BizType)

	textReq := &model.RunTextModerationRequest{
		Body: &model.TextDetectionReq{
			EventType: &eventType,
			Data: &model.TextDetectionDataReq{
				Text: req.Resource.ContentText,
			},
		},
	}

	resp, err := p.client.RunTextModeration(textReq)
	if err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("text moderation failed: %w", err)
	}

	if resp.RequestId == nil {
		return providers.SubmitResponse{}, fmt.Errorf("invalid response from huawei")
	}

	result := p.parseTextResponse(resp)
	taskID := *resp.RequestId

	return providers.SubmitResponse{
		Mode:      providers.ModeSync,
		TaskID:    taskID,
		Immediate: result,
		Raw: map[string]any{
			"requestId": taskID,
			"result":    resp.Result,
		},
	}, nil
}

func (p *Provider) getTextEventType(bizType censor.BizType) string {
	switch bizType {
	case censor.BizUserNickname, censor.BizUserBio:
		return "nickname"
	case censor.BizComment, censor.BizDanmaku:
		return "comment"
	case censor.BizChatMessage:
		return "chat"
	default:
		return "comment"
	}
}

func (p *Provider) parseTextResponse(resp *model.RunTextModerationResponse) *censor.ReviewResult {
	result := &censor.ReviewResult{
		Decision:   censor.DecisionPass,
		Confidence: 1.0,
		Provider:   providerName,
		ReviewedAt: time.Now(),
	}

	if resp.Result == nil {
		return result
	}

	r := resp.Result

	// Parse suggestion - it's a pointer to string-like enum
	if r.Suggestion != nil {
		suggestion := string(*r.Suggestion)
		switch suggestion {
		case "block":
			result.Decision = censor.DecisionBlock
		case "review":
			result.Decision = censor.DecisionReview
		case "pass":
			result.Decision = censor.DecisionPass
		}
	}

	// Parse labels
	if r.Label != nil {
		result.Reasons = append(result.Reasons, censor.Reason{
			Code:     *r.Label,
			Provider: providerName,
		})
	}

	// Parse details
	if r.Details != nil {
		for _, detail := range *r.Details {
			if detail.Label != nil {
				reason := censor.Reason{
					Code:     *detail.Label,
					Provider: providerName,
				}
				if detail.Confidence != nil {
					conf := float64(*detail.Confidence)
					reason.Raw = map[string]any{
						"confidence": conf,
					}
					if conf > result.Confidence {
						result.Confidence = conf
					}
				}
				result.Reasons = append(result.Reasons, reason)
			}
		}
	}

	return result
}

func (p *Provider) submitImage(ctx context.Context, req providers.SubmitRequest) (providers.SubmitResponse, error) {
	categories := []string{"politics", "terrorism", "porn"}
	eventType := "head_image"

	imageReq := &model.CheckImageModerationRequest{
		Body: &model.ImageDetectionReq{
			EventType:  &eventType,
			Categories: &categories,
			Url:        &req.Resource.ContentURL,
		},
	}

	resp, err := p.client.CheckImageModeration(imageReq)
	if err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("image moderation failed: %w", err)
	}

	if resp.RequestId == nil {
		return providers.SubmitResponse{}, fmt.Errorf("invalid response from huawei")
	}

	result := p.parseImageResponse(resp)
	taskID := *resp.RequestId

	return providers.SubmitResponse{
		Mode:      providers.ModeSync,
		TaskID:    taskID,
		Immediate: result,
		Raw: map[string]any{
			"requestId": taskID,
			"result":    resp.Result,
		},
	}, nil
}

func (p *Provider) parseImageResponse(resp *model.CheckImageModerationResponse) *censor.ReviewResult {
	result := &censor.ReviewResult{
		Decision:   censor.DecisionPass,
		Confidence: 1.0,
		Provider:   providerName,
		ReviewedAt: time.Now(),
	}

	if resp.Result == nil {
		return result
	}

	r := resp.Result

	// Parse suggestion
	if r.Suggestion != nil {
		switch *r.Suggestion {
		case "block":
			result.Decision = censor.DecisionBlock
		case "review":
			result.Decision = censor.DecisionReview
		case "pass":
			result.Decision = censor.DecisionPass
		}
	}

	// Parse category
	if r.Category != nil {
		result.Reasons = append(result.Reasons, censor.Reason{
			Code:     *r.Category,
			Provider: providerName,
		})
	}

	// Parse details
	if r.Details != nil {
		for _, detail := range *r.Details {
			if detail.Label != nil {
				result.Reasons = append(result.Reasons, censor.Reason{
					Code:     *detail.Label,
					Provider: providerName,
				})
			}
		}
	}

	return result
}

func (p *Provider) submitVideo(ctx context.Context, req providers.SubmitRequest) (providers.SubmitResponse, error) {
	imageCategories := []model.VideoCreateRequestImageCategories{
		model.GetVideoCreateRequestImageCategoriesEnum().POLITICS,
		model.GetVideoCreateRequestImageCategoriesEnum().TERRORISM,
		model.GetVideoCreateRequestImageCategoriesEnum().PORN,
	}
	eventType := model.GetVideoCreateRequestEventTypeEnum().DEFAULT

	videoReq := &model.RunCreateVideoModerationJobRequest{
		Body: &model.VideoCreateRequest{
			Data: &model.VideoCreateRequestData{
				Url: req.Resource.ContentURL,
			},
			EventType:       &eventType,
			ImageCategories: &imageCategories,
		},
	}

	if p.config.CallbackURL != "" {
		videoReq.Body.Callback = &p.config.CallbackURL
	}

	resp, err := p.client.RunCreateVideoModerationJob(videoReq)
	if err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("video moderation failed: %w", err)
	}

	if resp.RequestId == nil || resp.JobId == nil {
		return providers.SubmitResponse{}, fmt.Errorf("invalid response from huawei")
	}

	return providers.SubmitResponse{
		Mode:   providers.ModeAsync,
		TaskID: *resp.JobId,
		Raw: map[string]any{
			"requestId": *resp.RequestId,
			"jobId":     *resp.JobId,
		},
	}, nil
}

// Query queries the status of an async task.
func (p *Provider) Query(ctx context.Context, taskID string) (providers.QueryResponse, error) {
	req := &model.RunQueryVideoModerationJobRequest{
		JobId: taskID,
	}

	resp, err := p.client.RunQueryVideoModerationJob(req)
	if err != nil {
		return providers.QueryResponse{}, fmt.Errorf("query video result failed: %w", err)
	}

	if resp.RequestId == nil {
		return providers.QueryResponse{}, fmt.Errorf("invalid response from huawei")
	}

	result := &censor.ReviewResult{
		Decision:   censor.DecisionPass,
		Confidence: 1.0,
		Provider:   providerName,
		ReviewedAt: time.Now(),
	}

	done := false
	if resp.Status != nil {
		switch resp.Status.Value() {
		case "succeeded":
			done = true
			if resp.Result != nil && resp.Result.Suggestion != nil {
				switch resp.Result.Suggestion.Value() {
				case "block":
					result.Decision = censor.DecisionBlock
				case "review":
					result.Decision = censor.DecisionReview
				}
			}
		case "failed":
			done = true
			result.Decision = censor.DecisionError
		case "running":
			done = false
		}
	}

	return providers.QueryResponse{
		Done:   done,
		Result: result,
		Raw: map[string]any{
			"requestId": *resp.RequestId,
			"status":    resp.Status,
		},
	}, nil
}

// VerifyCallback verifies the signature of a callback request.
func (p *Provider) VerifyCallback(ctx context.Context, headers map[string]string, body []byte) error {
	signature := headers["X-Hw-Signature"]
	if signature == "" {
		return censor.ErrCallbackInvalid
	}

	mac := hmac.New(sha256.New, []byte(p.config.CallbackKey))
	mac.Write(body)
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
		return censor.ErrCallbackInvalid
	}

	return nil
}

// ParseCallback parses a callback request body.
func (p *Provider) ParseCallback(ctx context.Context, body []byte) (providers.CallbackData, error) {
	var callback struct {
		JobID  string `json:"job_id"`
		Status string `json:"status"`
		Result struct {
			Suggestion string `json:"suggestion"`
			Categories []struct {
				Name       string  `json:"name"`
				Confidence float64 `json:"confidence"`
			} `json:"categories"`
		} `json:"result"`
	}

	if err := json.Unmarshal(body, &callback); err != nil {
		return providers.CallbackData{}, fmt.Errorf("failed to parse callback: %w", err)
	}

	decision := censor.DecisionPass
	switch callback.Result.Suggestion {
	case "block":
		decision = censor.DecisionBlock
	case "review":
		decision = censor.DecisionReview
	}

	var reasons []censor.Reason
	var highestConf float64
	for _, cat := range callback.Result.Categories {
		reasons = append(reasons, censor.Reason{
			Code:     cat.Name,
			Provider: providerName,
		})
		if cat.Confidence > highestConf {
			highestConf = cat.Confidence
		}
	}

	return providers.CallbackData{
		TaskID: callback.JobID,
		Done:   callback.Status == "succeeded",
		Result: &censor.ReviewResult{
			Decision:   decision,
			Confidence: highestConf,
			Reasons:    reasons,
			Provider:   providerName,
			ReviewedAt: time.Now(),
		},
		Raw: map[string]any{"raw": callback},
	}, nil
}

// Translator returns the violation translator.
func (p *Provider) Translator() violation.Translator {
	return p.translator
}

// Huawei label mappings
// Based on Huawei Cloud Content Moderation API documentation
var labelMappings = map[string]labelMapping{
	"porn":        {domain: "pornography", tags: []string{"pornographic_act"}, severity: 4},
	"sexy":        {domain: "sexual_hint", tags: []string{"nudity"}, severity: 2},
	"sexual_hint": {domain: "sexual_hint", tags: []string{}, severity: 2},
	"terrorism":   {domain: "terrorism", tags: []string{}, severity: 4},
	"violence":    {domain: "violence", tags: []string{"gore"}, severity: 3},
	"politics":    {domain: "politics", tags: []string{"political_sensitive"}, severity: 3},
	"leader":      {domain: "politics", tags: []string{"political_leader"}, severity: 3},
	"ban":         {domain: "illegal", tags: []string{}, severity: 4},
	"abuse":       {domain: "abuse", tags: []string{}, severity: 2},
	"ad":          {domain: "ads", tags: []string{"spam_ads"}, severity: 1},
	"qrcode":      {domain: "spam", tags: []string{"spam_contact"}, severity: 1},
	"image_text":  {domain: "other", tags: []string{"image_text_violation"}, severity: 2},
	"moan":        {domain: "sexual_hint", tags: []string{"sexual_text"}, severity: 3},
	"normal":      {domain: "", tags: []string{}, severity: 0},
	"pass":        {domain: "", tags: []string{}, severity: 0},
}

type labelMapping struct {
	domain   string
	tags     []string
	severity int
}

func newTranslator() violation.Translator {
	labelMap := make(map[string]violation.LabelMapping)

	for label, mapping := range labelMappings {
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

	return violation.NewBaseTranslator(providerName, labelMap)
}
