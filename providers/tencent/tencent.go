// Package tencent provides Tencent Cloud content moderation integration.
package tencent

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	ims "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ims/v20201229"
	tms "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/tms/v20201229"
	vm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/vm/v20201229"

	censor "github.com/heibot/censor"
	"github.com/heibot/censor/providers"
	"github.com/heibot/censor/violation"
)

const providerName = "tencent"

// Config holds the configuration for Tencent provider.
type Config struct {
	providers.ProviderConfig

	AppID       string
	CallbackURL string
	CallbackKey string
}

// DefaultConfig returns the default Tencent configuration.
func DefaultConfig() Config {
	return Config{
		ProviderConfig: providers.ProviderConfig{
			Region:   "ap-guangzhou",
			Endpoint: "tms.tencentcloudapi.com",
			Timeout:  30 * time.Second,
		},
	}
}

// Provider implements the Tencent content moderation provider.
type Provider struct {
	config     Config
	tmsClient  *tms.Client
	imsClient  *ims.Client
	vmClient   *vm.Client
	translator violation.Translator
	credential *common.Credential
}

// New creates a new Tencent provider.
func New(cfg Config) (*Provider, error) {
	p := &Provider{
		config:     cfg,
		translator: newTranslator(),
	}

	if err := p.initClients(); err != nil {
		return nil, fmt.Errorf("failed to init tencent clients: %w", err)
	}

	return p, nil
}

func (p *Provider) initClients() error {
	p.credential = common.NewCredential(p.config.AccessKeyID, p.config.AccessKeySecret)

	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = "tms.tencentcloudapi.com"

	tmsClient, err := tms.NewClient(p.credential, p.config.Region, cpf)
	if err != nil {
		return fmt.Errorf("failed to create tms client: %w", err)
	}
	p.tmsClient = tmsClient

	cpf.HttpProfile.Endpoint = "ims.tencentcloudapi.com"
	imsClient, err := ims.NewClient(p.credential, p.config.Region, cpf)
	if err != nil {
		return fmt.Errorf("failed to create ims client: %w", err)
	}
	p.imsClient = imsClient

	cpf.HttpProfile.Endpoint = "vm.tencentcloudapi.com"
	vmClient, err := vm.NewClient(p.credential, p.config.Region, cpf)
	if err != nil {
		return fmt.Errorf("failed to create vm client: %w", err)
	}
	p.vmClient = vmClient

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

// SceneCapability returns the detection scene capabilities for Tencent.
func (p *Provider) SceneCapability() providers.SceneCapability {
	return providers.SceneCapability{
		Provider: providerName,
		SupportedScenes: map[censor.ResourceType][]violation.UnifiedScene{
			censor.ResourceText: {
				violation.ScenePornography, violation.SceneTerrorism, violation.ScenePolitics,
				violation.SceneAbuse, violation.SceneAds, violation.SceneSpam, violation.SceneBan,
			},
			censor.ResourceImage: {
				violation.ScenePornography, violation.SceneTerrorism, violation.ScenePolitics,
				violation.SceneViolence, violation.SceneAds,
			},
			censor.ResourceVideo: {
				violation.ScenePornography, violation.SceneTerrorism, violation.ScenePolitics,
				violation.SceneViolence, violation.SceneAds,
			},
		},
		MaxTextLength:  10000,
		SyncSupported:  true,
		AsyncSupported: true,
	}
}

// TranslateScenes converts unified scenes to Tencent-specific scene codes.
func (p *Provider) TranslateScenes(scenes []violation.UnifiedScene, resourceType censor.ResourceType) []string {
	var sceneMap map[violation.UnifiedScene]string

	switch resourceType {
	case censor.ResourceText:
		sceneMap = map[violation.UnifiedScene]string{
			violation.ScenePornography: "Porn",
			violation.SceneTerrorism:   "Terror",
			violation.ScenePolitics:    "Polity",
			violation.SceneAbuse:       "Abuse",
			violation.SceneAds:         "Ad",
			violation.SceneSpam:        "Spam",
			violation.SceneBan:         "Illegal",
		}
	case censor.ResourceImage, censor.ResourceVideo:
		sceneMap = map[violation.UnifiedScene]string{
			violation.ScenePornography: "Porn",
			violation.SceneTerrorism:   "Terror",
			violation.ScenePolitics:    "Polity",
			violation.SceneViolence:    "Violence",
			violation.SceneAds:         "Ad",
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
	textReq := tms.NewTextModerationRequest()
	content := base64.StdEncoding.EncodeToString([]byte(req.Resource.ContentText))
	textReq.Content = &content

	if req.Biz.SubmitterID != "" {
		textReq.User = &tms.User{
			UserId: &req.Biz.SubmitterID,
		}
	}

	resp, err := p.tmsClient.TextModeration(textReq)
	if err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("text moderation failed: %w", err)
	}

	result := p.parseTextResponse(resp)
	taskID := ""
	if resp.Response != nil && resp.Response.RequestId != nil {
		taskID = *resp.Response.RequestId
	}

	return providers.SubmitResponse{
		Mode:      providers.ModeSync,
		TaskID:    taskID,
		Immediate: result,
		Raw: map[string]any{
			"requestId": taskID,
			"response":  resp.Response,
		},
	}, nil
}

func (p *Provider) parseTextResponse(resp *tms.TextModerationResponse) *censor.ReviewResult {
	result := &censor.ReviewResult{
		Decision:   censor.DecisionPass,
		Confidence: 1.0,
		Provider:   providerName,
		ReviewedAt: time.Now(),
	}

	if resp.Response == nil {
		return result
	}

	r := resp.Response

	// Parse suggestion
	if r.Suggestion != nil {
		switch *r.Suggestion {
		case "Block":
			result.Decision = censor.DecisionBlock
		case "Review":
			result.Decision = censor.DecisionReview
		case "Pass":
			result.Decision = censor.DecisionPass
		}
	}

	// Parse label
	if r.Label != nil {
		result.Reasons = append(result.Reasons, censor.Reason{
			Code:     *r.Label,
			Provider: providerName,
		})
	}

	// Parse detail results
	if r.DetailResults != nil {
		for _, detail := range r.DetailResults {
			if detail.Label != nil {
				reason := censor.Reason{
					Code:     *detail.Label,
					Provider: providerName,
				}
				if detail.Score != nil {
					score := float64(*detail.Score) / 100.0
					reason.Raw = map[string]any{
						"score": score,
					}
					if score > result.Confidence {
						result.Confidence = score
					}
				}
				result.Reasons = append(result.Reasons, reason)
			}
		}
	}

	return result
}

func (p *Provider) submitImage(ctx context.Context, req providers.SubmitRequest) (providers.SubmitResponse, error) {
	imageReq := ims.NewImageModerationRequest()
	imageReq.FileUrl = &req.Resource.ContentURL

	if req.Biz.SubmitterID != "" {
		imageReq.User = &ims.User{
			UserId: &req.Biz.SubmitterID,
		}
	}

	resp, err := p.imsClient.ImageModeration(imageReq)
	if err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("image moderation failed: %w", err)
	}

	result := p.parseImageResponse(resp)
	taskID := ""
	if resp.Response != nil && resp.Response.RequestId != nil {
		taskID = *resp.Response.RequestId
	}

	return providers.SubmitResponse{
		Mode:      providers.ModeSync,
		TaskID:    taskID,
		Immediate: result,
		Raw: map[string]any{
			"requestId": taskID,
			"response":  resp.Response,
		},
	}, nil
}

func (p *Provider) parseImageResponse(resp *ims.ImageModerationResponse) *censor.ReviewResult {
	result := &censor.ReviewResult{
		Decision:   censor.DecisionPass,
		Confidence: 1.0,
		Provider:   providerName,
		ReviewedAt: time.Now(),
	}

	if resp.Response == nil {
		return result
	}

	r := resp.Response

	// Parse suggestion
	if r.Suggestion != nil {
		switch *r.Suggestion {
		case "Block":
			result.Decision = censor.DecisionBlock
		case "Review":
			result.Decision = censor.DecisionReview
		case "Pass":
			result.Decision = censor.DecisionPass
		}
	}

	// Parse label
	if r.Label != nil {
		result.Reasons = append(result.Reasons, censor.Reason{
			Code:     *r.Label,
			Provider: providerName,
		})
	}

	// Parse sub labels
	if r.SubLabel != nil {
		result.Reasons = append(result.Reasons, censor.Reason{
			Code:     *r.SubLabel,
			Provider: providerName,
		})
	}

	// Parse score
	if r.Score != nil {
		result.Confidence = float64(*r.Score) / 100.0
	}

	return result
}

func (p *Provider) submitVideo(ctx context.Context, req providers.SubmitRequest) (providers.SubmitResponse, error) {
	videoReq := vm.NewCreateVideoModerationTaskRequest()
	taskType := "VIDEO"
	videoReq.Type = &taskType

	mediaType := "URL"
	videoReq.Tasks = []*vm.TaskInput{
		{
			Input: &vm.StorageInfo{
				Type: &mediaType,
				Url:  &req.Resource.ContentURL,
			},
		},
	}

	if p.config.CallbackURL != "" {
		videoReq.CallbackUrl = &p.config.CallbackURL
	}

	resp, err := p.vmClient.CreateVideoModerationTask(videoReq)
	if err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("video moderation failed: %w", err)
	}

	taskID := ""
	if resp.Response != nil && resp.Response.Results != nil && len(resp.Response.Results) > 0 {
		if resp.Response.Results[0].TaskId != nil {
			taskID = *resp.Response.Results[0].TaskId
		}
	}

	return providers.SubmitResponse{
		Mode:   providers.ModeAsync,
		TaskID: taskID,
		Raw: map[string]any{
			"requestId": taskID,
			"response":  resp.Response,
		},
	}, nil
}

// Query queries the status of an async task.
func (p *Provider) Query(ctx context.Context, taskID string) (providers.QueryResponse, error) {
	req := vm.NewDescribeTaskDetailRequest()
	req.TaskId = &taskID

	resp, err := p.vmClient.DescribeTaskDetail(req)
	if err != nil {
		return providers.QueryResponse{}, fmt.Errorf("query video result failed: %w", err)
	}

	result := &censor.ReviewResult{
		Decision:   censor.DecisionPass,
		Confidence: 1.0,
		Provider:   providerName,
		ReviewedAt: time.Now(),
	}

	done := false
	if resp.Response != nil {
		if resp.Response.Status != nil {
			switch *resp.Response.Status {
			case "FINISH":
				done = true
			case "RUNNING":
				done = false
			case "ERROR":
				done = true
				result.Decision = censor.DecisionError
			}
		}

		// Parse suggestion
		if done && resp.Response.Suggestion != nil {
			switch *resp.Response.Suggestion {
			case "Block":
				result.Decision = censor.DecisionBlock
			case "Review":
				result.Decision = censor.DecisionReview
			}
		}

		// Parse labels
		if resp.Response.Labels != nil {
			for _, label := range resp.Response.Labels {
				if label.Label != nil {
					result.Reasons = append(result.Reasons, censor.Reason{
						Code:     *label.Label,
						Provider: providerName,
					})
				}
			}
		}
	}

	return providers.QueryResponse{
		Done:   done,
		Result: result,
		Raw: map[string]any{
			"response": resp.Response,
		},
	}, nil
}

// VerifyCallback verifies the signature of a callback request.
func (p *Provider) VerifyCallback(ctx context.Context, headers map[string]string, body []byte) error {
	signature := headers["X-TC-Signature"]
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
		TaskID     string `json:"TaskId"`
		Status     string `json:"Status"`
		Suggestion string `json:"Suggestion"`
		Labels     []struct {
			Label string  `json:"Label"`
			Score float64 `json:"Score"`
		} `json:"Labels"`
	}

	if err := json.Unmarshal(body, &callback); err != nil {
		return providers.CallbackData{}, fmt.Errorf("failed to parse callback: %w", err)
	}

	decision := censor.DecisionPass
	switch callback.Suggestion {
	case "Block":
		decision = censor.DecisionBlock
	case "Review":
		decision = censor.DecisionReview
	}

	var reasons []censor.Reason
	var highestConf float64
	for _, label := range callback.Labels {
		reasons = append(reasons, censor.Reason{
			Code:     label.Label,
			Provider: providerName,
		})
		if label.Score > highestConf {
			highestConf = label.Score
		}
	}

	return providers.CallbackData{
		TaskID: callback.TaskID,
		Done:   callback.Status == "FINISH",
		Result: &censor.ReviewResult{
			Decision:   decision,
			Confidence: highestConf / 100.0,
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

// Tencent label mappings
var labelMappings = map[string]labelMapping{
	"Porn":     {domain: "pornography", tags: []string{"pornographic_act"}, severity: 4},
	"Sexy":     {domain: "sexual_hint", tags: []string{"nudity"}, severity: 2},
	"Sexual":   {domain: "pornography", tags: []string{"sexual_text"}, severity: 3},
	"Polity":   {domain: "politics", tags: []string{"political_sensitive"}, severity: 3},
	"Politics": {domain: "politics", tags: []string{"political_sensitive"}, severity: 3},
	"Terror":   {domain: "terrorism", tags: []string{}, severity: 4},
	"Violence": {domain: "violence", tags: []string{"gore"}, severity: 3},
	"Abuse":    {domain: "abuse", tags: []string{}, severity: 2},
	"Ad":       {domain: "ads", tags: []string{"spam_ads"}, severity: 1},
	"Spam":     {domain: "spam", tags: []string{}, severity: 1},
	"Illegal":  {domain: "illegal", tags: []string{}, severity: 4},
	"Fraud":    {domain: "fraud", tags: []string{"fraud_payment"}, severity: 3},
	"Minor":    {domain: "minor_safety", tags: []string{}, severity: 4},
	"Normal":   {domain: "", tags: []string{}, severity: 0},
	"Pass":     {domain: "", tags: []string{}, severity: 0},
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
