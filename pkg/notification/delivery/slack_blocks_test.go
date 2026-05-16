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

// Issue #588 Bug 5: Direct unit tests for FormatSlackBlocks.
// The active Slack formatting path had no direct tests — coverage came only through
// the deprecated FormatSlackPayload wrapper or E2E.
//
// Business Requirements:
//   - BR-NOT-051: Multi-channel delivery (Slack Block Kit format)
//   - BR-NOT-055: Message formatting with priority indicators
package delivery_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/slack-go/slack"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
)

var _ = Describe("Issue #588: FormatSlackBlocks Direct Tests", func() {

	buildNotification := func(subject, body string, priority notificationv1alpha1.NotificationPriority, notifType notificationv1alpha1.NotificationType) *notificationv1alpha1.NotificationRequest {
		return &notificationv1alpha1.NotificationRequest{
			Spec: notificationv1alpha1.NotificationRequestSpec{
				Subject:  subject,
				Body:     body,
				Priority: priority,
				Type:     notifType,
			},
		}
	}

	It("UT-NOT-588-005: should return exactly 3 blocks (header, section, context)", func() {
		notification := buildNotification(
			"Manual Review Required",
			"Signal: ContainerOOMKilling",
			notificationv1alpha1.NotificationPriorityHigh,
			notificationv1alpha1.NotificationTypeManualReview,
		)

		blocks := delivery.FormatSlackBlocks(notification)

		Expect(blocks).To(HaveLen(3), "Expected 3 blocks: header, section, context")

		// Block 0: Header
		headerBlock, ok := blocks[0].(*slack.HeaderBlock)
		Expect(ok).To(BeTrue(), "First block must be a HeaderBlock")
		Expect(headerBlock.Text.Type).To(Equal(slack.PlainTextType))
		Expect(headerBlock.Text.Text).To(ContainSubstring("Manual Review Required"))

		// Block 1: Section with body
		sectionBlock, ok := blocks[1].(*slack.SectionBlock)
		Expect(ok).To(BeTrue(), "Second block must be a SectionBlock")
		Expect(sectionBlock.Text.Type).To(Equal(slack.MarkdownType))
		Expect(sectionBlock.Text.Text).To(ContainSubstring("Signal: ContainerOOMKilling"))

		// Block 2: Context with metadata
		_, ok = blocks[2].(*slack.ContextBlock)
		Expect(ok).To(BeTrue(), "Third block must be a ContextBlock")
	})

	It("UT-NOT-588-006: section block body applies MarkdownToMrkdwn conversion", func() {
		notification := buildNotification(
			"Test Subject",
			"**Status**: Success and [link](https://example.com)",
			notificationv1alpha1.NotificationPriorityMedium,
			notificationv1alpha1.NotificationTypeManualReview,
		)

		blocks := delivery.FormatSlackBlocks(notification)

		sectionBlock := blocks[1].(*slack.SectionBlock)
		Expect(sectionBlock.Text.Text).To(ContainSubstring("*Status*"),
			"**bold** must be converted to *bold* via MarkdownToMrkdwn")
		Expect(sectionBlock.Text.Text).ToNot(ContainSubstring("**Status**"),
			"Markdown bold must not remain in mrkdwn output")
		Expect(sectionBlock.Text.Text).To(ContainSubstring("<https://example.com|link>"),
			"Markdown link must be converted to Slack link format")
	})

	It("UT-NOT-588-007: all priority levels map to correct emoji in header", func() {
		type emojiCase struct {
			priority notificationv1alpha1.NotificationPriority
			emoji    string
		}

		cases := []emojiCase{
			{notificationv1alpha1.NotificationPriorityCritical, "🚨"},
			{notificationv1alpha1.NotificationPriorityHigh, "⚠️"},
			{notificationv1alpha1.NotificationPriorityMedium, "ℹ️"},
			{notificationv1alpha1.NotificationPriorityLow, "💬"},
		}

		for _, tc := range cases {
			notification := buildNotification(
				"Test",
				"Body",
				tc.priority,
				notificationv1alpha1.NotificationTypeManualReview,
			)

			blocks := delivery.FormatSlackBlocks(notification)
			headerBlock := blocks[0].(*slack.HeaderBlock)
			Expect(headerBlock.Text.Text).To(HavePrefix(tc.emoji),
				"Priority %s must map to emoji %s", tc.priority, tc.emoji)
		}
	})
})
