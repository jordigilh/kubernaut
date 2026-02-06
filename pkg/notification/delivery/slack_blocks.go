package delivery

import (
	"fmt"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/formatting"
	"github.com/slack-go/slack"
)

// FormatSlackBlocks creates structured Slack Block Kit blocks using the github.com/slack-go/slack SDK
//
// Per DD-AUDIT-004 and 02-go-coding-standards.mdc: Use structured types instead of map[string]interface{}
//
// This replaces the manual map[string]interface{} construction with SDK-provided structured types
// for compile-time type safety and maintainability.
//
// Business Requirements:
// - BR-NOT-051: Multi-channel delivery (Slack Block Kit format)
// - BR-NOT-055: Message formatting with priority indicators
//
// Parameters:
//   - notification: The NotificationRequest CRD containing message details
//
// Returns:
//   - []slack.Block: Structured Block Kit blocks for Slack API
//
// DD-AUDIT-004: Structured Types for Audit Event Payloads (applies to all unstructured data)
func FormatSlackBlocks(notification *notificationv1alpha1.NotificationRequest) []slack.Block {
	// Priority emoji mapping
	priorityEmoji := map[notificationv1alpha1.NotificationPriority]string{
		notificationv1alpha1.NotificationPriorityCritical: "🚨",
		notificationv1alpha1.NotificationPriorityHigh:     "⚠️",
		notificationv1alpha1.NotificationPriorityMedium:   "ℹ️",
		notificationv1alpha1.NotificationPriorityLow:      "💬",
	}

	emoji := priorityEmoji[notification.Spec.Priority]
	if emoji == "" {
		emoji = "📢" // Default emoji
	}

	blocks := []slack.Block{
		// Header block with priority emoji + subject
		slack.NewHeaderBlock(
			&slack.TextBlockObject{
				Type: slack.PlainTextType,
				Text: fmt.Sprintf("%s %s", emoji, notification.Spec.Subject),
			},
		),

		// Section block with message body (markdown → mrkdwn converted per Issue #48)
		slack.NewSectionBlock(
			&slack.TextBlockObject{
				Type: slack.MarkdownType,
				Text: formatting.MarkdownToMrkdwn(notification.Spec.Body),
			},
			nil, // No fields
			nil, // No accessory
		),

		// Context block with metadata (priority + type)
		slack.NewContextBlock(
			"", // No block ID
			&slack.TextBlockObject{
				Type: slack.MarkdownType,
				Text: fmt.Sprintf("*Priority:* %s | *Type:* %s",
					notification.Spec.Priority,
					notification.Spec.Type),
			},
		),
	}

	return blocks
}
