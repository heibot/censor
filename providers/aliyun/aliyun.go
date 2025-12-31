package aliyun

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	green "github.com/alibabacloud-go/green-20220302/v2/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"

	censor "github.com/heibot/censor"
	"github.com/heibot/censor/providers"
	"github.com/heibot/censor/violation"
)

const providerName = "aliyun"

// Provider implements the Aliyun content moderation provider.
type Provider struct {
	config     Config
	client     *green.Client
	translator violation.Translator
}

// New creates a new Aliyun provider.
func New(cfg Config) (*Provider, error) {
	p := &Provider{
		config:     cfg,
		translator: newTranslator(),
	}

	if err := p.initClient(); err != nil {
		return nil, fmt.Errorf("failed to init aliyun client: %w", err)
	}

	return p, nil
}

func (p *Provider) initClient() error {
	config := &openapi.Config{
		AccessKeyId:     tea.String(p.config.AccessKeyID),
		AccessKeySecret: tea.String(p.config.AccessKeySecret),
		RegionId:        tea.String(p.config.Region),
		Endpoint:        tea.String(p.config.Endpoint),
	}

	client, err := green.NewClient(config)
	if err != nil {
		return err
	}

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

// SceneCapability returns the detection scene capabilities for Aliyun.
func (p *Provider) SceneCapability() providers.SceneCapability {
	return providers.SceneCapability{
		Provider: providerName,
		SupportedScenes: map[censor.ResourceType][]violation.UnifiedScene{
			censor.ResourceText: {
				violation.ScenePornography, violation.SceneTerrorism, violation.ScenePolitics,
				violation.SceneAbuse, violation.SceneAds, violation.SceneSpam,
				violation.SceneBan, violation.SceneMeaningless, violation.SceneFlood,
			},
			censor.ResourceImage: {
				violation.ScenePornography, violation.SceneTerrorism, violation.ScenePolitics,
				violation.SceneAds, violation.SceneQRCode,
			},
			censor.ResourceVideo: {
				violation.ScenePornography, violation.SceneTerrorism, violation.ScenePolitics,
				violation.SceneAds,
			},
		},
		MaxTextLength:  10000,
		SyncSupported:  true,
		AsyncSupported: true,
	}
}

// TranslateScenes converts unified scenes to Aliyun-specific scene codes.
func (p *Provider) TranslateScenes(scenes []violation.UnifiedScene, resourceType censor.ResourceType) []string {
	// Aliyun text uses "antispam" scene, filtering happens via labels
	if resourceType == censor.ResourceText {
		return []string{"antispam"}
	}

	sceneMap := map[violation.UnifiedScene]string{
		violation.ScenePornography: "porn",
		violation.SceneTerrorism:   "terrorism",
		violation.ScenePolitics:    "politics",
		violation.SceneAds:         "ad",
		violation.SceneQRCode:      "qrcode",
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
	// Build service parameters
	serviceParams := map[string]interface{}{
		"content": req.Resource.ContentText,
	}
	if req.Biz.SubmitterID != "" {
		serviceParams["accountId"] = req.Biz.SubmitterID
	}

	serviceParamsJSON, err := json.Marshal(serviceParams)
	if err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("failed to marshal service params: %w", err)
	}

	// Determine service type based on business type
	service := p.getTextService(req.Biz.BizType)

	textReq := &green.TextModerationRequest{
		Service:           tea.String(service),
		ServiceParameters: tea.String(string(serviceParamsJSON)),
	}

	runtime := &util.RuntimeOptions{}
	resp, err := p.client.TextModerationWithOptions(textReq, runtime)
	if err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("text moderation failed: %w", err)
	}

	if resp.Body == nil || resp.Body.Code == nil {
		return providers.SubmitResponse{}, fmt.Errorf("invalid response from aliyun")
	}

	if *resp.Body.Code != 200 {
		return providers.SubmitResponse{}, fmt.Errorf("aliyun error: code=%d, msg=%s",
			*resp.Body.Code, tea.StringValue(resp.Body.Message))
	}

	// Parse response
	result := p.parseTextResponse(resp.Body)
	taskID := tea.StringValue(resp.Body.RequestId)

	return providers.SubmitResponse{
		Mode:      providers.ModeSync,
		TaskID:    taskID,
		Immediate: result,
		Raw: map[string]any{
			"requestId": taskID,
			"code":      tea.Int32Value(resp.Body.Code),
			"data":      resp.Body.Data,
		},
	}, nil
}

func (p *Provider) getTextService(bizType censor.BizType) string {
	// Map business type to Aliyun service type
	switch bizType {
	case censor.BizUserNickname:
		return "nickname_detection"
	case censor.BizComment:
		return "comment_detection"
	case censor.BizChatMessage:
		return "chat_detection"
	default:
		return "chat_detection"
	}
}

func (p *Provider) parseTextResponse(body *green.TextModerationResponseBody) *censor.ReviewResult {
	result := &censor.ReviewResult{
		Decision:   censor.DecisionPass,
		Confidence: 1.0,
		Provider:   providerName,
		ReviewedAt: time.Now(),
	}

	if body.Data == nil {
		return result
	}

	data := body.Data

	// Parse labels
	if data.Labels != nil && *data.Labels != "" {
		labels := *data.Labels
		result.Reasons = append(result.Reasons, censor.Reason{
			Code:     labels,
			Provider: providerName,
		})

		// Determine decision based on labels
		if labels != "" && labels != "normal" && labels != "nonLabel" {
			result.Decision = censor.DecisionBlock
		}
	}

	// Parse reason (contains detailed risk info)
	if data.Reason != nil && *data.Reason != "" {
		var reasonData map[string]interface{}
		if err := json.Unmarshal([]byte(*data.Reason), &reasonData); err == nil {
			// Extract risk level
			if riskLevel, ok := reasonData["riskLevel"].(string); ok {
				switch riskLevel {
				case "high":
					result.Decision = censor.DecisionBlock
					result.Confidence = 0.95
				case "medium":
					result.Decision = censor.DecisionReview
					result.Confidence = 0.75
				case "low":
					result.Confidence = 0.5
				}
			}
		}
	}

	return result
}

func (p *Provider) submitImage(ctx context.Context, req providers.SubmitRequest) (providers.SubmitResponse, error) {
	// Build service parameters
	serviceParams := map[string]interface{}{
		"imageUrl": req.Resource.ContentURL,
	}

	serviceParamsJSON, err := json.Marshal(serviceParams)
	if err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("failed to marshal service params: %w", err)
	}

	imageReq := &green.ImageModerationRequest{
		Service:           tea.String("baselineCheck"),
		ServiceParameters: tea.String(string(serviceParamsJSON)),
	}

	runtime := &util.RuntimeOptions{}
	resp, err := p.client.ImageModerationWithOptions(imageReq, runtime)
	if err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("image moderation failed: %w", err)
	}

	if resp.Body == nil || resp.Body.Code == nil {
		return providers.SubmitResponse{}, fmt.Errorf("invalid response from aliyun")
	}

	if *resp.Body.Code != 200 {
		return providers.SubmitResponse{}, fmt.Errorf("aliyun error: code=%d, msg=%s",
			*resp.Body.Code, tea.StringValue(resp.Body.Msg))
	}

	result := p.parseImageResponse(resp.Body)
	taskID := tea.StringValue(resp.Body.RequestId)

	return providers.SubmitResponse{
		Mode:      providers.ModeSync,
		TaskID:    taskID,
		Immediate: result,
		Raw: map[string]any{
			"requestId": taskID,
			"code":      tea.Int32Value(resp.Body.Code),
			"data":      resp.Body.Data,
		},
	}, nil
}

func (p *Provider) parseImageResponse(body *green.ImageModerationResponseBody) *censor.ReviewResult {
	result := &censor.ReviewResult{
		Decision:   censor.DecisionPass,
		Confidence: 1.0,
		Provider:   providerName,
		ReviewedAt: time.Now(),
	}

	if body.Data == nil {
		return result
	}

	data := body.Data

	// Parse result list
	if data.Result != nil {
		for _, item := range data.Result {
			if item.Label != nil && *item.Label != "" {
				label := *item.Label
				confidence := float64(0)
				if item.Confidence != nil {
					confidence = float64(*item.Confidence)
				}

				result.Reasons = append(result.Reasons, censor.Reason{
					Code:     label,
					Provider: providerName,
					Raw: map[string]any{
						"confidence": confidence,
					},
				})

				// Determine decision based on label
				if label != "normal" && label != "nonLabel" {
					if confidence >= 90 {
						result.Decision = censor.DecisionBlock
					} else if confidence >= 70 {
						result.Decision = censor.DecisionReview
					}
				}

				if confidence > result.Confidence {
					result.Confidence = confidence / 100.0
				}
			}
		}
	}

	return result
}

func (p *Provider) submitVideo(ctx context.Context, req providers.SubmitRequest) (providers.SubmitResponse, error) {
	// Build service parameters for async video moderation
	serviceParams := map[string]interface{}{
		"url": req.Resource.ContentURL,
	}
	if p.config.CallbackURL != "" {
		serviceParams["callback"] = p.config.CallbackURL
	}

	serviceParamsJSON, err := json.Marshal(serviceParams)
	if err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("failed to marshal service params: %w", err)
	}

	videoReq := &green.VideoModerationRequest{
		Service:           tea.String("videoAsyncManualReview"),
		ServiceParameters: tea.String(string(serviceParamsJSON)),
	}

	runtime := &util.RuntimeOptions{}
	resp, err := p.client.VideoModerationWithOptions(videoReq, runtime)
	if err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("video moderation failed: %w", err)
	}

	if resp.Body == nil || resp.Body.Code == nil {
		return providers.SubmitResponse{}, fmt.Errorf("invalid response from aliyun")
	}

	if *resp.Body.Code != 200 {
		return providers.SubmitResponse{}, fmt.Errorf("aliyun error: code=%d, msg=%s",
			*resp.Body.Code, tea.StringValue(resp.Body.Message))
	}

	taskID := ""
	if resp.Body.Data != nil && resp.Body.Data.TaskId != nil {
		taskID = *resp.Body.Data.TaskId
	}

	return providers.SubmitResponse{
		Mode:   providers.ModeAsync,
		TaskID: taskID,
		Raw: map[string]any{
			"requestId": tea.StringValue(resp.Body.RequestId),
			"code":      tea.Int32Value(resp.Body.Code),
			"taskId":    taskID,
		},
	}, nil
}

// Query queries the status of an async task.
func (p *Provider) Query(ctx context.Context, taskID string) (providers.QueryResponse, error) {
	req := &green.VideoModerationResultRequest{
		Service:           tea.String("videoAsyncManualReview"),
		ServiceParameters: tea.String(fmt.Sprintf(`{"taskId":"%s"}`, taskID)),
	}

	runtime := &util.RuntimeOptions{}
	resp, err := p.client.VideoModerationResultWithOptions(req, runtime)
	if err != nil {
		return providers.QueryResponse{}, fmt.Errorf("query video result failed: %w", err)
	}

	if resp.Body == nil || resp.Body.Code == nil {
		return providers.QueryResponse{}, fmt.Errorf("invalid response from aliyun")
	}

	if *resp.Body.Code != 200 {
		return providers.QueryResponse{}, fmt.Errorf("aliyun error: code=%d, msg=%s",
			*resp.Body.Code, tea.StringValue(resp.Body.Message))
	}

	result := &censor.ReviewResult{
		Decision:   censor.DecisionPass,
		Confidence: 1.0,
		Provider:   providerName,
		ReviewedAt: time.Now(),
	}

	done := false
	if resp.Body.Data != nil && resp.Body.Data.LiveId != nil {
		// Parse live_id to determine status
		done = true
	}

	return providers.QueryResponse{
		Done:   done,
		Result: result,
		Raw: map[string]any{
			"requestId": tea.StringValue(resp.Body.RequestId),
			"code":      tea.Int32Value(resp.Body.Code),
		},
	}, nil
}

// VerifyCallback verifies the signature of a callback request.
func (p *Provider) VerifyCallback(ctx context.Context, headers map[string]string, body []byte) error {
	signature := headers["X-Acs-Signature"]
	if signature == "" {
		return censor.ErrCallbackInvalid
	}

	// Verify HMAC signature
	mac := hmac.New(sha256.New, []byte(p.config.CallbackSeed))
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
		TaskID   string `json:"taskId"`
		Code     int    `json:"code"`
		DataID   string `json:"dataId"`
		Labels   string `json:"labels"`
		Reason   string `json:"reason"`
		RiskTips string `json:"riskTips"`
	}

	if err := json.Unmarshal(body, &callback); err != nil {
		return providers.CallbackData{}, fmt.Errorf("failed to parse callback: %w", err)
	}

	decision := censor.DecisionPass
	var reasons []censor.Reason
	var highestScore float64 = 1.0

	if callback.Labels != "" && callback.Labels != "normal" && callback.Labels != "nonLabel" {
		decision = censor.DecisionBlock
		reasons = append(reasons, censor.Reason{
			Code:     callback.Labels,
			Message:  callback.RiskTips,
			Provider: providerName,
		})
	}

	return providers.CallbackData{
		TaskID: callback.TaskID,
		Done:   true,
		Result: &censor.ReviewResult{
			Decision:   decision,
			Confidence: highestScore,
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
