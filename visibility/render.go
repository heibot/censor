package visibility

import (
	censor "github.com/heibot/censor"
)

// RenderResult represents the result of rendering a business object.
type RenderResult struct {
	Visible bool                     // Whether the object is visible at all
	Fields  map[string]RenderedField // Field-level rendering results
	Message string                   // Optional message (e.g., "Content under review")
}

// RenderedField represents a rendered field value.
type RenderedField struct {
	Visible      bool   // Whether this field is visible
	Value        string // The value to display (may be replaced)
	IsReplaced   bool   // Whether the value was replaced
	OriginalHash string // Hash of original value (for verification)
	Message      string // Optional message for this field
}

// Renderer handles content rendering based on visibility policies.
type Renderer struct {
	defaultReplacements map[censor.BizType]string
}

// NewRenderer creates a new renderer.
func NewRenderer() *Renderer {
	return &Renderer{
		defaultReplacements: map[censor.BizType]string{
			censor.BizUserNickname: "用户",
			censor.BizUserBio:      "",
			censor.BizNoteTitle:    "内容已屏蔽",
			censor.BizNoteBody:     "该内容因违反社区规定已被屏蔽",
			censor.BizTeamName:     "队伍",
			censor.BizChatMessage:  "[消息已屏蔽]",
			censor.BizDanmaku:      "",
			censor.BizComment:      "[评论已屏蔽]",
		},
	}
}

// SetDefaultReplacement sets a default replacement for a business type.
func (r *Renderer) SetDefaultReplacement(bizType censor.BizType, value string) {
	r.defaultReplacements[bizType] = value
}

// RenderContext provides context for rendering.
type RenderContext struct {
	BizType  censor.BizType
	BizID    string
	Viewer   ViewerRole
	ViewerID string // For creator check
}

// FieldData represents raw field data to be rendered.
type FieldData struct {
	Field    string
	RawValue string
	Binding  *censor.CensorBinding
}

// Render renders a business object based on its bindings.
func (r *Renderer) Render(ctx RenderContext, fields []FieldData) RenderResult {
	policy := GetPolicy(ctx.BizType)

	// Build bindings for outcome calculation
	var bindings []censor.CensorBinding
	for _, f := range fields {
		if f.Binding != nil {
			bindings = append(bindings, *f.Binding)
		}
	}

	outcome := ComputeBizOutcome(bindings)

	// Check if object is visible at all
	visible := CanView(policy, outcome, ctx.Viewer)

	result := RenderResult{
		Visible: visible,
		Fields:  make(map[string]RenderedField),
	}

	if !visible {
		result.Message = r.getBlockedMessage(ctx.BizType)
		return result
	}

	// Render each field
	for _, f := range fields {
		result.Fields[f.Field] = r.renderField(ctx, f, policy)
	}

	// Set message if under review
	if outcome.OverallDecision == censor.DecisionReview {
		result.Message = "内容审核中"
	}

	return result
}

// renderField renders a single field.
func (r *Renderer) renderField(ctx RenderContext, field FieldData, policy Policy) RenderedField {
	// Check if field is visible
	visible := CanViewField(field.Binding, ctx.Viewer, policy)

	if visible && field.Binding == nil {
		// No binding, show original
		return RenderedField{
			Visible: true,
			Value:   field.RawValue,
		}
	}

	if visible && field.Binding != nil {
		decision := censor.Decision(field.Binding.Decision)

		switch decision {
		case censor.DecisionPass, censor.DecisionPending:
			return RenderedField{
				Visible:      true,
				Value:        field.RawValue,
				OriginalHash: field.Binding.ContentHash,
			}

		case censor.DecisionReview:
			if ctx.Viewer == ViewerCreator {
				return RenderedField{
					Visible:      true,
					Value:        field.RawValue,
					OriginalHash: field.Binding.ContentHash,
					Message:      "审核中",
				}
			}
			return r.applyReplacement(ctx.BizType, field)

		case censor.DecisionBlock:
			return r.applyReplacement(ctx.BizType, field)
		}
	}

	// Not visible
	return RenderedField{
		Visible: false,
		Message: "内容不可见",
	}
}

// applyReplacement applies the replacement policy.
func (r *Renderer) applyReplacement(bizType censor.BizType, field FieldData) RenderedField {
	if field.Binding == nil {
		return RenderedField{Visible: false}
	}

	policy := censor.ReplacePolicy(field.Binding.ReplacePolicy)

	switch policy {
	case censor.ReplacePolicyNone:
		return RenderedField{
			Visible: false,
			Message: "内容已屏蔽",
		}

	case censor.ReplacePolicyDefault:
		value := field.Binding.ReplaceValue
		if value == "" {
			value = r.defaultReplacements[bizType]
		}
		return RenderedField{
			Visible:      true,
			Value:        value,
			IsReplaced:   true,
			OriginalHash: field.Binding.ContentHash,
		}

	case censor.ReplacePolicyMask:
		return RenderedField{
			Visible:      true,
			Value:        r.maskValue(field.RawValue),
			IsReplaced:   true,
			OriginalHash: field.Binding.ContentHash,
		}

	default:
		return RenderedField{Visible: false}
	}
}

// maskValue masks a value.
func (r *Renderer) maskValue(value string) string {
	runes := []rune(value)
	if len(runes) <= 2 {
		return "**"
	}
	for i := 1; i < len(runes)-1; i++ {
		runes[i] = '*'
	}
	return string(runes)
}

// getBlockedMessage returns the blocked message for a business type.
func (r *Renderer) getBlockedMessage(bizType censor.BizType) string {
	switch bizType {
	case censor.BizNoteTitle, censor.BizNoteBody:
		return "该内容因违反社区规定已被屏蔽"
	case censor.BizChatMessage:
		return "消息已被屏蔽"
	case censor.BizComment:
		return "评论已被屏蔽"
	default:
		return "内容不可用"
	}
}

// RenderUserProfile is a convenience function for rendering user profiles.
func (r *Renderer) RenderUserProfile(viewer ViewerRole, viewerID, targetUserID string,
	nickname, bio, avatarURL string,
	bindings map[string]*censor.CensorBinding) RenderResult {

	ctx := RenderContext{
		BizType:  censor.BizUserNickname, // Use any user-related type
		BizID:    targetUserID,
		Viewer:   viewer,
		ViewerID: viewerID,
	}

	fields := []FieldData{
		{Field: "nickname", RawValue: nickname, Binding: bindings["nickname"]},
		{Field: "bio", RawValue: bio, Binding: bindings["bio"]},
		{Field: "avatar", RawValue: avatarURL, Binding: bindings["avatar"]},
	}

	return r.Render(ctx, fields)
}
