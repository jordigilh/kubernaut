/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package formatting

import (
	"encoding/json"
	"fmt"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/slack-go/slack"
)

// SlackFormatter formats notifications for Slack Block Kit
type SlackFormatter struct{}

// NewSlackFormatter creates a new Slack formatter
func NewSlackFormatter() *SlackFormatter {
	return &SlackFormatter{}
}

// Format formats a notification for Slack Block Kit.
// Returns a Slack WebhookMessage with Block Kit blocks, including:
//   - Header block with priority emoji + subject
//   - Section block with body text (converted from Markdown to mrkdwn)
//   - Context block with metadata (priority + type)
//
// The body text is automatically converted from standard Markdown to Slack's
// mrkdwn syntax using MarkdownToMrkdwn (Issue #48).
//
// Reference: https://api.slack.com/block-kit
//
// Business Requirements:
//   - BR-NOT-051: Multi-channel delivery (Slack Block Kit format)
//   - BR-NOT-055: Message formatting with priority indicators
func (f *SlackFormatter) Format(notification *notificationv1alpha1.NotificationRequest) (interface{}, error) {
	if notification == nil {
		return nil, fmt.Errorf("notification must not be nil")
	}

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

		// Section block with body text (Markdown → mrkdwn per Issue #48)
		slack.NewSectionBlock(
			&slack.TextBlockObject{
				Type: slack.MarkdownType,
				Text: MarkdownToMrkdwn(notification.Spec.Body),
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

	// Build webhook message
	msg := slack.WebhookMessage{
		Blocks: &slack.Blocks{
			BlockSet: blocks,
		},
	}

	// Enforce Slack's 40KB payload limit
	payload, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Slack payload: %w", err)
	}

	const maxPayloadBytes = 40 * 1024 // 40KB Slack limit
	if len(payload) > maxPayloadBytes {
		return nil, fmt.Errorf("slack payload exceeds 40KB limit (%d bytes)", len(payload))
	}

	return msg, nil
}
