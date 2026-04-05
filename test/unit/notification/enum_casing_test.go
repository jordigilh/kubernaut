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

package notification

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/slack-go/slack"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
	"github.com/jordigilh/kubernaut/pkg/notification/formatting"
	"github.com/jordigilh/kubernaut/pkg/notification/routing"
)

var _ = Describe("Issue #640: NotificationRequest PascalCase Enum Migration", func() {

	Context("NotificationPriority enum values", func() {
		It("UT-NOT-640-001: all 4 NotificationPriority constants use PascalCase", func() {
			Expect(string(notificationv1.NotificationPriorityCritical)).To(Equal("Critical"))
			Expect(string(notificationv1.NotificationPriorityHigh)).To(Equal("High"))
			Expect(string(notificationv1.NotificationPriorityMedium)).To(Equal("Medium"))
			Expect(string(notificationv1.NotificationPriorityLow)).To(Equal("Low"))
		})
	})

	Context("NotificationType enum values", func() {
		It("UT-NOT-640-002: all 6 NotificationType constants use PascalCase", func() {
			Expect(string(notificationv1.NotificationTypeEscalation)).To(Equal("Escalation"))
			Expect(string(notificationv1.NotificationTypeSimple)).To(Equal("Simple"))
			Expect(string(notificationv1.NotificationTypeStatusUpdate)).To(Equal("StatusUpdate"))
			Expect(string(notificationv1.NotificationTypeApproval)).To(Equal("Approval"))
			Expect(string(notificationv1.NotificationTypeManualReview)).To(Equal("ManualReview"))
			Expect(string(notificationv1.NotificationTypeCompletion)).To(Equal("Completion"))
		})
	})

	Context("Routing attribute extraction", func() {
		It("UT-NOT-640-003: RoutingAttributesFromSpec extracts PascalCase type and priority", func() {
			nr := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "test-640-003", Namespace: "default"},
				Spec: notificationv1.NotificationRequestSpec{
					Type:     notificationv1.NotificationTypeManualReview,
					Priority: notificationv1.NotificationPriorityHigh,
					Subject:  "test",
					Body:     "test",
				},
			}
			attrs := routing.RoutingAttributesFromSpec(nr)
			Expect(attrs["type"]).To(Equal("ManualReview"))
			Expect(attrs["priority"]).To(Equal("High"))
		})
	})

	Context("Routing constants alignment", func() {
		It("UT-NOT-640-004: routing constants that mirror CRD values use PascalCase", func() {
			Expect(routing.NotificationTypeEscalation).To(Equal("Escalation"))
			Expect(routing.NotificationTypeManualReview).To(Equal("ManualReview"))
		})
	})

	Context("Audit payload fidelity", func() {
		It("UT-NOT-640-005: string cast of typed enums produces PascalCase for audit payloads", func() {
			typeStr := string(notificationv1.NotificationTypeApproval)
			priorityStr := string(notificationv1.NotificationPriorityLow)
			Expect(typeStr).To(Equal("Approval"))
			Expect(priorityStr).To(Equal("Low"))
		})
	})

	Context("Enum completeness", func() {
		It("UT-NOT-640-006: all 6 NotificationType values are distinct and non-empty PascalCase", func() {
			allTypes := []notificationv1.NotificationType{
				notificationv1.NotificationTypeEscalation,
				notificationv1.NotificationTypeSimple,
				notificationv1.NotificationTypeStatusUpdate,
				notificationv1.NotificationTypeApproval,
				notificationv1.NotificationTypeManualReview,
				notificationv1.NotificationTypeCompletion,
			}
			seen := make(map[notificationv1.NotificationType]bool)
			for _, t := range allTypes {
				Expect(string(t)).NotTo(BeEmpty(), fmt.Sprintf("type %v should not be empty", t))
				firstChar := string(t)[0]
				Expect(firstChar >= 'A' && firstChar <= 'Z').To(BeTrue(),
					fmt.Sprintf("type %q should start with uppercase", t))
				Expect(strings.Contains(string(t), "-")).To(BeFalse(),
					fmt.Sprintf("type %q should not contain hyphens", t))
				seen[t] = true
			}
			Expect(seen).To(HaveLen(6))
		})
	})

	Context("Console display formatting", func() {
		It("UT-NOT-640-007: ConsoleFormatter resolves priorityEmoji for all PascalCase priorities", func() {
			formatter := formatting.NewConsoleFormatter()
			priorities := []notificationv1.NotificationPriority{
				notificationv1.NotificationPriorityCritical,
				notificationv1.NotificationPriorityHigh,
				notificationv1.NotificationPriorityMedium,
				notificationv1.NotificationPriorityLow,
			}
			for _, p := range priorities {
				nr := &notificationv1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{Name: "test-640-007", Namespace: "default"},
					Spec: notificationv1.NotificationRequestSpec{
						Type:     notificationv1.NotificationTypeSimple,
						Priority: p,
						Subject:  "Test Subject",
						Body:     "Test Body",
					},
				}
				output, err := formatter.Format(nr)
				Expect(err).NotTo(HaveOccurred())
				Expect(output).NotTo(ContainSubstring("📢"),
					fmt.Sprintf("priority %q should resolve to a specific emoji, not fallback", p))
			}
		})
	})

	Context("Slack display formatting", func() {
		It("UT-NOT-640-008: FormatSlackBlocks resolves priorityEmoji for all PascalCase priorities", func() {
			expectedEmojis := map[notificationv1.NotificationPriority]string{
				notificationv1.NotificationPriorityCritical: "🚨",
				notificationv1.NotificationPriorityHigh:     "⚠️",
				notificationv1.NotificationPriorityMedium:   "ℹ️",
				notificationv1.NotificationPriorityLow:      "💬",
			}
			for p, emoji := range expectedEmojis {
				nr := &notificationv1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{Name: "test-640-008", Namespace: "default"},
					Spec: notificationv1.NotificationRequestSpec{
						Type:     notificationv1.NotificationTypeSimple,
						Priority: p,
						Subject:  "Test Subject",
						Body:     "Test Body",
					},
				}
				blocks := delivery.FormatSlackBlocks(nr)
				Expect(blocks).To(HaveLen(3))
				headerBlock, ok := blocks[0].(*slack.HeaderBlock)
				Expect(ok).To(BeTrue(), "first block must be HeaderBlock")
				Expect(headerBlock.Text.Text).To(ContainSubstring(emoji),
					fmt.Sprintf("priority %q should produce emoji %s in header", p, emoji))
			}
		})
	})
})
