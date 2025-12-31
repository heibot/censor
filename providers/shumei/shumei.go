// Package shumei provides Shumei (数美) content moderation integration.
package shumei

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	censor "github.com/heibot/censor"
	"github.com/heibot/censor/providers"
	"github.com/heibot/censor/violation"
)

const providerName = "shumei"

// Config holds the configuration for Shumei provider.
type Config struct {
	providers.ProviderConfig

	// AppID is the Shumei application ID.
	AppID string

	// AccessKey is the API access key.
	AccessKey string

	// CallbackURL is the URL for async callback notifications.
	CallbackURL string
}

// DefaultConfig returns the default Shumei configuration.
func DefaultConfig() Config {
	return Config{
		ProviderConfig: providers.ProviderConfig{
			Endpoint: "api-text-bj.fengkongcloud.com",
			Timeout:  30 * time.Second,
		},
	}
}

// Provider implements the Shumei content moderation provider.
type Provider struct {
	config     Config
	httpClient *http.Client
	translator violation.Translator
}

// New creates a new Shumei provider.
func New(cfg Config) *Provider {
	return &Provider{
		config: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		translator: newTranslator(),
	}
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

// SceneCapability returns the detection scene capabilities for Shumei.
func (p *Provider) SceneCapability() providers.SceneCapability {
	return providers.SceneCapability{
		Provider: providerName,
		SupportedScenes: map[censor.ResourceType][]violation.UnifiedScene{
			censor.ResourceText: {
				violation.ScenePornography, violation.SceneTerrorism, violation.ScenePolitics,
				violation.SceneBan, violation.SceneAbuse, violation.SceneAds,
				violation.ScenePrivacy, violation.SceneAdLaw, violation.SceneMeaningless,
				violation.SceneFraud, violation.SceneMinor, violation.ScenePublicFigure,
			},
			censor.ResourceImage: {
				violation.ScenePornography, violation.SceneTerrorism, violation.ScenePolitics,
				violation.SceneAds, violation.SceneQRCode, violation.SceneImageText,
			},
			censor.ResourceVideo: {
				violation.ScenePornography, violation.SceneTerrorism, violation.ScenePolitics,
				violation.SceneAds, violation.SceneImageText,
			},
		},
		MaxTextLength:  10000,
		SyncSupported:  true,
		AsyncSupported: true,
	}
}

// TranslateScenes converts unified scenes to Shumei-specific type codes.
func (p *Provider) TranslateScenes(scenes []violation.UnifiedScene, resourceType censor.ResourceType) []string {
	var sceneMap map[violation.UnifiedScene]string

	switch resourceType {
	case censor.ResourceText:
		sceneMap = map[violation.UnifiedScene]string{
			violation.ScenePornography:  "EROTIC",
			violation.SceneTerrorism:    "VIOLENT",
			violation.ScenePolitics:     "POLITY",
			violation.SceneBan:          "BAN",
			violation.SceneAbuse:        "DIRTY",
			violation.SceneAds:          "ADVERT",
			violation.ScenePrivacy:      "PRIVACY",
			violation.SceneAdLaw:        "ADLAW",
			violation.SceneMeaningless:  "MEANINGLESS",
			violation.SceneFraud:        "FRUAD", // Note: Shumei typo in API
			violation.SceneMinor:        "TEXTMINOR",
			violation.ScenePublicFigure: "PUBLICFIGURE",
		}
	case censor.ResourceImage, censor.ResourceVideo:
		sceneMap = map[violation.UnifiedScene]string{
			violation.ScenePornography: "EROTIC",
			violation.SceneTerrorism:   "VIOLENT",
			violation.ScenePolitics:    "POLITY",
			violation.SceneAds:         "ADVERT",
			violation.SceneQRCode:      "QRCODE",
			violation.SceneImageText:   "IMGTEXTRISK",
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
	// Build request body
	scenes := p.TranslateScenes(req.Scenes, censor.ResourceText)
	if len(scenes) == 0 {
		scenes = []string{"EROTIC", "POLITY", "VIOLENT", "BAN", "DIRTY", "ADVERT"}
	}
	typeStr := strings.Join(scenes, "_")

	requestBody := map[string]interface{}{
		"accessKey": p.config.AccessKey,
		"appId":     p.config.AppID,
		"type":      typeStr,
		"data": map[string]interface{}{
			"text":    req.Resource.ContentText,
			"tokenId": req.Biz.SubmitterID,
		},
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Make HTTP request
	url := fmt.Sprintf("https://%s/v4/text", p.config.Endpoint)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("request failed: %w", err)
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var resp textResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("failed to parse response: %w", err)
	}

	if resp.Code != 1100 {
		return providers.SubmitResponse{}, fmt.Errorf("shumei error: code=%d, msg=%s", resp.Code, resp.Message)
	}

	result := p.parseTextResponse(&resp)

	return providers.SubmitResponse{
		Mode:      providers.ModeSync,
		TaskID:    resp.RequestID,
		Immediate: result,
		Raw: map[string]any{
			"requestId": resp.RequestID,
			"code":      resp.Code,
			"riskLevel": resp.RiskLevel,
		},
	}, nil
}

type textResponse struct {
	Code       int    `json:"code"`
	Message    string `json:"message"`
	RequestID  string `json:"requestId"`
	RiskLevel  string `json:"riskLevel"`
	RiskLabel1 string `json:"riskLabel1"`
	RiskLabel2 string `json:"riskLabel2"`
	RiskLabel3 string `json:"riskLabel3"`
	Score      int    `json:"score"`
}

func (p *Provider) parseTextResponse(resp *textResponse) *censor.ReviewResult {
	result := &censor.ReviewResult{
		Decision:   censor.DecisionPass,
		Confidence: float64(resp.Score) / 100.0,
		Provider:   providerName,
		ReviewedAt: time.Now(),
	}

	switch resp.RiskLevel {
	case "REJECT":
		result.Decision = censor.DecisionBlock
	case "REVIEW":
		result.Decision = censor.DecisionReview
	case "PASS":
		result.Decision = censor.DecisionPass
	}

	if resp.RiskLabel1 != "" && resp.RiskLabel1 != "normal" {
		result.Reasons = append(result.Reasons, censor.Reason{
			Code:     resp.RiskLabel1,
			Provider: providerName,
		})
	}

	return result
}

func (p *Provider) submitImage(ctx context.Context, req providers.SubmitRequest) (providers.SubmitResponse, error) {
	scenes := p.TranslateScenes(req.Scenes, censor.ResourceImage)
	if len(scenes) == 0 {
		scenes = []string{"EROTIC", "POLITY", "VIOLENT", "ADVERT", "QRCODE"}
	}
	typeStr := strings.Join(scenes, "_")

	requestBody := map[string]interface{}{
		"accessKey": p.config.AccessKey,
		"appId":     p.config.AppID,
		"type":      typeStr,
		"data": map[string]interface{}{
			"img":     req.Resource.ContentURL,
			"tokenId": req.Biz.SubmitterID,
		},
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("https://%s/image/v4", p.getImageEndpoint())
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("request failed: %w", err)
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var resp imageResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("failed to parse response: %w", err)
	}

	if resp.Code != 1100 {
		return providers.SubmitResponse{}, fmt.Errorf("shumei error: code=%d, msg=%s", resp.Code, resp.Message)
	}

	result := p.parseImageResponse(&resp)

	return providers.SubmitResponse{
		Mode:      providers.ModeSync,
		TaskID:    resp.RequestID,
		Immediate: result,
		Raw: map[string]any{
			"requestId": resp.RequestID,
			"code":      resp.Code,
			"riskLevel": resp.RiskLevel,
		},
	}, nil
}

type imageResponse struct {
	Code       int    `json:"code"`
	Message    string `json:"message"`
	RequestID  string `json:"requestId"`
	RiskLevel  string `json:"riskLevel"`
	RiskLabel1 string `json:"riskLabel1"`
	RiskLabel2 string `json:"riskLabel2"`
	RiskLabel3 string `json:"riskLabel3"`
	Score      int    `json:"score"`
}

func (p *Provider) getImageEndpoint() string {
	if strings.Contains(p.config.Endpoint, "text") {
		return strings.Replace(p.config.Endpoint, "text", "img", 1)
	}
	return "api-img-bj.fengkongcloud.com"
}

func (p *Provider) parseImageResponse(resp *imageResponse) *censor.ReviewResult {
	result := &censor.ReviewResult{
		Decision:   censor.DecisionPass,
		Confidence: float64(resp.Score) / 100.0,
		Provider:   providerName,
		ReviewedAt: time.Now(),
	}

	switch resp.RiskLevel {
	case "REJECT":
		result.Decision = censor.DecisionBlock
	case "REVIEW":
		result.Decision = censor.DecisionReview
	case "PASS":
		result.Decision = censor.DecisionPass
	}

	if resp.RiskLabel1 != "" && resp.RiskLabel1 != "normal" {
		result.Reasons = append(result.Reasons, censor.Reason{
			Code:     resp.RiskLabel1,
			Provider: providerName,
		})
	}

	return result
}

func (p *Provider) submitVideo(ctx context.Context, req providers.SubmitRequest) (providers.SubmitResponse, error) {
	scenes := p.TranslateScenes(req.Scenes, censor.ResourceVideo)
	if len(scenes) == 0 {
		scenes = []string{"EROTIC", "POLITY", "VIOLENT", "ADVERT"}
	}
	typeStr := strings.Join(scenes, "_")

	requestBody := map[string]interface{}{
		"accessKey": p.config.AccessKey,
		"appId":     p.config.AppID,
		"type":      typeStr,
		"data": map[string]interface{}{
			"url":     req.Resource.ContentURL,
			"tokenId": req.Biz.SubmitterID,
		},
	}

	if p.config.CallbackURL != "" {
		requestBody["callback"] = p.config.CallbackURL
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("https://%s/video/v4", p.getVideoEndpoint())
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("request failed: %w", err)
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var resp videoResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return providers.SubmitResponse{}, fmt.Errorf("failed to parse response: %w", err)
	}

	if resp.Code != 1100 {
		return providers.SubmitResponse{}, fmt.Errorf("shumei error: code=%d, msg=%s", resp.Code, resp.Message)
	}

	return providers.SubmitResponse{
		Mode:   providers.ModeAsync,
		TaskID: resp.RequestID,
		Raw: map[string]any{
			"requestId": resp.RequestID,
			"code":      resp.Code,
		},
	}, nil
}

type videoResponse struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId"`
}

func (p *Provider) getVideoEndpoint() string {
	if strings.Contains(p.config.Endpoint, "text") {
		return strings.Replace(p.config.Endpoint, "text", "video", 1)
	}
	return "api-video-bj.fengkongcloud.com"
}

// Query queries the status of an async task.
func (p *Provider) Query(ctx context.Context, taskID string) (providers.QueryResponse, error) {
	requestBody := map[string]interface{}{
		"accessKey": p.config.AccessKey,
		"requestId": taskID,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return providers.QueryResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("https://%s/video/query/v4", p.getVideoEndpoint())
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return providers.QueryResponse{}, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return providers.QueryResponse{}, fmt.Errorf("request failed: %w", err)
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return providers.QueryResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	var resp queryResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return providers.QueryResponse{}, fmt.Errorf("failed to parse response: %w", err)
	}

	result := &censor.ReviewResult{
		Decision:   censor.DecisionPass,
		Confidence: 1.0,
		Provider:   providerName,
		ReviewedAt: time.Now(),
	}

	done := resp.Code == 1100

	if done && resp.RiskLevel != "" {
		switch resp.RiskLevel {
		case "REJECT":
			result.Decision = censor.DecisionBlock
		case "REVIEW":
			result.Decision = censor.DecisionReview
		}

		if resp.RiskLabel1 != "" && resp.RiskLabel1 != "normal" {
			result.Reasons = append(result.Reasons, censor.Reason{
				Code:     resp.RiskLabel1,
				Provider: providerName,
			})
		}
	}

	return providers.QueryResponse{
		Done:   done,
		Result: result,
		Raw: map[string]any{
			"requestId": taskID,
			"code":      resp.Code,
			"riskLevel": resp.RiskLevel,
		},
	}, nil
}

type queryResponse struct {
	Code       int    `json:"code"`
	Message    string `json:"message"`
	RiskLevel  string `json:"riskLevel"`
	RiskLabel1 string `json:"riskLabel1"`
}

// VerifyCallback verifies the signature of a callback request.
func (p *Provider) VerifyCallback(ctx context.Context, headers map[string]string, body []byte) error {
	signature := headers["X-Shumei-Signature"]
	if signature == "" {
		return censor.ErrCallbackInvalid
	}

	// Shumei uses MD5(sorted_params + accessKey) for signature
	var params map[string]any
	if err := json.Unmarshal(body, &params); err != nil {
		return censor.ErrCallbackInvalid
	}

	expectedSig := p.computeSignature(params)
	if signature != expectedSig {
		return censor.ErrCallbackInvalid
	}

	return nil
}

func (p *Provider) computeSignature(params map[string]any) string {
	// Sort keys and concatenate values
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(fmt.Sprintf("%v", params[k]))
	}
	sb.WriteString(p.config.AccessKey)

	hash := md5.Sum([]byte(sb.String()))
	return hex.EncodeToString(hash[:])
}

// ParseCallback parses a callback request body.
func (p *Provider) ParseCallback(ctx context.Context, body []byte) (providers.CallbackData, error) {
	var callback struct {
		RequestID   string `json:"requestId"`
		Code        int    `json:"code"`
		RiskLevel   string `json:"riskLevel"`
		RiskLabel1  string `json:"riskLabel1"`
		RiskLabel2  string `json:"riskLabel2"`
		RiskLabel3  string `json:"riskLabel3"`
		Score       int    `json:"score"`
		Description string `json:"riskDescription"`
	}

	if err := json.Unmarshal(body, &callback); err != nil {
		return providers.CallbackData{}, fmt.Errorf("failed to parse callback: %w", err)
	}

	decision := censor.DecisionPass
	switch callback.RiskLevel {
	case "REJECT":
		decision = censor.DecisionBlock
	case "REVIEW":
		decision = censor.DecisionReview
	case "PASS":
		decision = censor.DecisionPass
	}

	var reasons []censor.Reason
	if callback.RiskLabel1 != "" {
		reasons = append(reasons, censor.Reason{
			Code:     callback.RiskLabel1,
			Message:  callback.Description,
			Provider: providerName,
		})
	}

	confidence := float64(callback.Score) / 100.0

	return providers.CallbackData{
		TaskID: callback.RequestID,
		Done:   callback.Code == 1100,
		Result: &censor.ReviewResult{
			Decision:   decision,
			Confidence: confidence,
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

// Shumei label mappings based on API documentation
// riskLabel1 -> domain mapping
var labelMappings = map[string]labelMapping{
	// Pornography (色情)
	"EROTIC": {domain: "pornography", tags: []string{"sexual_text"}, severity: 3},
	"SEXY":   {domain: "sexual_hint", tags: []string{"nudity"}, severity: 2},
	"SEX":    {domain: "pornography", tags: []string{"pornographic_act"}, severity: 4},
	"GENDER": {domain: "sexual_hint", tags: []string{"gender_related"}, severity: 1},

	// Violence (暴力)
	"VIOLENT": {domain: "violence", tags: []string{"gore"}, severity: 3},
	"WEAPON":  {domain: "violence", tags: []string{"weapon"}, severity: 2},
	"BLOODY":  {domain: "violence", tags: []string{"blood_content"}, severity: 3},

	// Politics (政治)
	"POLITY": {domain: "politics", tags: []string{"political_sensitive"}, severity: 3},
	"LEADER": {domain: "politics", tags: []string{"political_leader"}, severity: 3},
	"NATION": {domain: "politics", tags: []string{"national_event"}, severity: 2},

	// Prohibited (违禁)
	"BAN":      {domain: "illegal", tags: []string{}, severity: 4},
	"DRUG":     {domain: "illegal", tags: []string{"drug_related"}, severity: 4},
	"GAMBLING": {domain: "gambling", tags: []string{}, severity: 3},

	// Abuse (辱骂)
	"DIRTY":  {domain: "abuse", tags: []string{}, severity: 2},
	"INSULT": {domain: "harassment", tags: []string{}, severity: 2},
	"HATE":   {domain: "hate_speech", tags: []string{}, severity: 3},

	// Ads (广告)
	"ADVERT":  {domain: "ads", tags: []string{"spam_ads"}, severity: 1},
	"CONTACT": {domain: "spam", tags: []string{"spam_contact"}, severity: 2},
	"QRCODE":  {domain: "spam", tags: []string{"qrcode"}, severity: 1},

	// Privacy (隐私)
	"PRIVACY": {domain: "account_risk", tags: []string{"personal_info"}, severity: 2},
	"IDCARD":  {domain: "account_risk", tags: []string{"personal_info"}, severity: 3},
	"PHONE":   {domain: "spam", tags: []string{"spam_contact"}, severity: 2},

	// Fraud (诈骗)
	"FRUAD": {domain: "fraud", tags: []string{"fraud_payment"}, severity: 3},
	"SCAM":  {domain: "scam", tags: []string{}, severity: 3},

	// Minor (未成年)
	"TEXTMINOR": {domain: "minor_safety", tags: []string{}, severity: 4},
	"MINOR":     {domain: "minor_safety", tags: []string{}, severity: 4},

	// Public Figure (公众人物)
	"PUBLICFIGURE": {domain: "politics", tags: []string{"public_figure"}, severity: 2},

	// Meaningless (无意义)
	"MEANINGLESS": {domain: "spam", tags: []string{"meaningless"}, severity: 1},

	// Ad Law (广告法)
	"ADLAW": {domain: "ads", tags: []string{"ad_law_violation"}, severity: 2},

	// Image text risk
	"IMGTEXTRISK": {domain: "other", tags: []string{"image_text_violation"}, severity: 2},

	// Normal
	"NORMAL": {domain: "", tags: []string{}, severity: 0},
}

type labelMapping struct {
	domain   string
	tags     []string
	severity int
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

	return violation.NewBaseTranslator(providerName, labelMap)
}
