package delivery

import (
	"encoding/json"
	"fmt"
	"strings"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

const (
	adaptiveCardSchema      = "http://adaptivecards.io/schemas/adaptive-card.json"
	adaptiveCardContentType = "application/vnd.microsoft.card.adaptive"
	adaptiveCardVersion     = "1.0"
	teamsPayloadLimit       = 28 * 1024 // 28 KB
)

// TeamsMessage is the Power Automate Workflows envelope.
// Uses the new Workflows webhook format (NOT legacy Office 365 MessageCard).
type TeamsMessage struct {
	Type        string            `json:"type"`
	Attachments []TeamsAttachment `json:"attachments"`
}

// TeamsAttachment wraps an Adaptive Card in the Workflows format.
type TeamsAttachment struct {
	ContentType string       `json:"contentType"`
	ContentURL  *string      `json:"contentUrl,omitempty"`
	Content     AdaptiveCard `json:"content"`
}

// AdaptiveCard represents a Microsoft Adaptive Card v1.0.
type AdaptiveCard struct {
	Schema  string        `json:"$schema"`
	Type    string        `json:"type"`
	Version string        `json:"version"`
	Body    []CardElement `json:"body"`
}

// CardElement is a polymorphic element in the Adaptive Card body.
// Supports TextBlock and FactSet types.
type CardElement struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Weight   string `json:"weight,omitempty"`
	Size     string `json:"size,omitempty"`
	Wrap     bool   `json:"wrap,omitempty"`
	Spacing  string `json:"spacing,omitempty"`
	IsSubtle bool   `json:"isSubtle,omitempty"`
	Color    string `json:"color,omitempty"`
	// FactSet fields
	Facts []CardFact `json:"facts,omitempty"`
}

// CardFact is a key-value pair in a FactSet element.
type CardFact struct {
	Title string `json:"title"`
	Value string `json:"value"`
}

// BuildTeamsPayload constructs a Power Automate Workflows message with an
// Adaptive Card from a NotificationRequest. The card layout varies by
// notification type (approval, status-update, escalation, completion, default).
func BuildTeamsPayload(notification *notificationv1alpha1.NotificationRequest) TeamsMessage {
	body := buildCardBody(notification)

	return TeamsMessage{
		Type: "message",
		Attachments: []TeamsAttachment{
			{
				ContentType: adaptiveCardContentType,
				Content: AdaptiveCard{
					Schema:  adaptiveCardSchema,
					Type:    "AdaptiveCard",
					Version: adaptiveCardVersion,
					Body:    body,
				},
			},
		},
	}
}

func buildCardBody(notification *notificationv1alpha1.NotificationRequest) []CardElement {
	var elements []CardElement

	elements = append(elements, buildHeaderSection(notification)...)
	elements = append(elements, buildBodySection(notification)...)
	elements = append(elements, buildFactsSection(notification)...)
	elements = append(elements, buildTypeSpecificSection(notification)...)
	elements = append(elements, buildCorrelationSection(notification)...)

	return elements
}

func buildCorrelationSection(notification *notificationv1alpha1.NotificationRequest) []CardElement {
	return []CardElement{
		{
			Type: "FactSet",
			Facts: []CardFact{
				{Title: "Correlation ID", Value: notification.Name},
			},
			Spacing: "small",
		},
	}
}

func buildHeaderSection(notification *notificationv1alpha1.NotificationRequest) []CardElement {
	priority := string(notification.Spec.Priority)
	prefix := ""
	color := ""

	switch notification.Spec.Priority {
	case notificationv1alpha1.NotificationPriorityCritical:
		prefix = "CRITICAL: "
		color = "attention"
	case notificationv1alpha1.NotificationPriorityHigh:
		prefix = "HIGH: "
		color = "warning"
	}

	header := CardElement{
		Type:   "TextBlock",
		Text:   prefix + notification.Spec.Subject,
		Weight: "bolder",
		Size:   "medium",
		Wrap:   true,
	}
	if color != "" {
		header.Color = color
	}

	return []CardElement{
		header,
		{
			Type:     "TextBlock",
			Text:     fmt.Sprintf("Priority: %s | Type: %s", strings.ToUpper(priority), notification.Spec.Type),
			IsSubtle: true,
			Spacing:  "none",
		},
	}
}

func buildBodySection(notification *notificationv1alpha1.NotificationRequest) []CardElement {
	if notification.Spec.Body == "" {
		return nil
	}
	return []CardElement{
		{
			Type:    "TextBlock",
			Text:    notification.Spec.Body,
			Wrap:    true,
			Spacing: "medium",
		},
	}
}

func buildFactsSection(notification *notificationv1alpha1.NotificationRequest) []CardElement {
	ctx := notification.Spec.Context
	if ctx == nil {
		return nil
	}

	var facts []CardFact

	if ctx.Analysis != nil && ctx.Analysis.RootCause != "" {
		facts = append(facts, CardFact{Title: "Root Cause", Value: ctx.Analysis.RootCause})
	}
	if ctx.Workflow != nil && ctx.Workflow.Confidence != "" {
		facts = append(facts, CardFact{Title: "Confidence", Value: ctx.Workflow.Confidence})
	}
	if ctx.Target != nil && ctx.Target.TargetResource != "" {
		facts = append(facts, CardFact{Title: "Affected Resource", Value: ctx.Target.TargetResource})
	}

	if len(facts) == 0 {
		return nil
	}

	return []CardElement{
		{
			Type:    "FactSet",
			Facts:   facts,
			Spacing: "medium",
		},
	}
}

func buildTypeSpecificSection(notification *notificationv1alpha1.NotificationRequest) []CardElement {
	switch notification.Spec.Type {
	case notificationv1alpha1.NotificationTypeApproval:
		return buildApprovalSection(notification)
	case notificationv1alpha1.NotificationTypeStatusUpdate:
		return buildVerificationSection(notification)
	case notificationv1alpha1.NotificationTypeEscalation:
		return buildEscalationSection(notification)
	case notificationv1alpha1.NotificationTypeCompletion:
		return buildVerificationSection(notification)
	default:
		return nil
	}
}

func buildApprovalSection(notification *notificationv1alpha1.NotificationRequest) []CardElement {
	var elements []CardElement

	ctx := notification.Spec.Context
	if ctx != nil && ctx.Lineage != nil && ctx.Lineage.RemediationRequest != "" {
		elements = append(elements, CardElement{
			Type:    "TextBlock",
			Text:    fmt.Sprintf("`kubectl kubernaut chat rar/%s -n %s`", ctx.Lineage.RemediationRequest, notification.Namespace),
			Wrap:    true,
			Spacing: "medium",
		})
	}

	return elements
}

func buildVerificationSection(notification *notificationv1alpha1.NotificationRequest) []CardElement {
	ctx := notification.Spec.Context
	if ctx == nil || ctx.Verification == nil {
		return nil
	}

	var facts []CardFact
	if ctx.Verification.Outcome != "" {
		facts = append(facts, CardFact{Title: "Verification Outcome", Value: ctx.Verification.Outcome})
	}
	if ctx.Verification.Reason != "" {
		facts = append(facts, CardFact{Title: "Verification Reason", Value: ctx.Verification.Reason})
	}

	if len(facts) == 0 {
		return nil
	}

	return []CardElement{
		{
			Type:    "FactSet",
			Facts:   facts,
			Spacing: "medium",
		},
	}
}

func buildEscalationSection(notification *notificationv1alpha1.NotificationRequest) []CardElement {
	return []CardElement{
		{
			Type:    "TextBlock",
			Text:    fmt.Sprintf("CRITICAL ESCALATION — Priority: %s", strings.ToUpper(string(notification.Spec.Priority))),
			Weight:  "bolder",
			Color:   "attention",
			Spacing: "medium",
		},
	}
}

// TruncateTeamsPayload reduces the message size to fit within the given limit
// by shortening the longest body text element. The correlation ID FactSet
// (added by BuildTeamsPayload) is always preserved.
func TruncateTeamsPayload(msg TeamsMessage, limit int) TeamsMessage {
	const marker = "[truncated -- full details in audit trail]"

	if len(msg.Attachments) == 0 {
		return msg
	}

	card := &msg.Attachments[0].Content

	longestIdx := -1
	longestLen := 0
	for i, el := range card.Body {
		if el.Type == "TextBlock" && len(el.Text) > longestLen {
			longestLen = len(el.Text)
			longestIdx = i
		}
	}

	if longestIdx < 0 {
		return msg
	}

	for attempts := 0; attempts < 3; attempts++ {
		raw, err := json.Marshal(msg)
		if err != nil || len(raw) <= limit {
			return msg
		}
		excess := len(raw) - limit
		text := card.Body[longestIdx].Text
		cutLen := excess + len(marker) + 2
		if len(text) > cutLen {
			card.Body[longestIdx].Text = text[:len(text)-cutLen] + " " + marker
		} else {
			card.Body[longestIdx].Text = marker
			return msg
		}
	}

	return msg
}
