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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/routing"
)

var _ = Describe("Issue #453 Phase A: Typed Enum Fields", func() {

	Context("UT-NOT-453A-001: ReviewSourceType enum validation", func() {
		It("should define ReviewSourceAIAnalysis with value AIAnalysis", func() {
			Expect(string(notificationv1.ReviewSourceAIAnalysis)).To(Equal("AIAnalysis"))
		})

		It("should define ReviewSourceWorkflowExecution with value WorkflowExecution", func() {
			Expect(string(notificationv1.ReviewSourceWorkflowExecution)).To(Equal("WorkflowExecution"))
		})

		It("should be assignable from string literals in struct initializers", func() {
			spec := notificationv1.NotificationRequestSpec{
				Type:         notificationv1.NotificationTypeEscalation,
				Priority:     notificationv1.NotificationPriorityHigh,
				Subject:      "Test",
				Body:         "Test body",
				ReviewSource: "AIAnalysis",
			}
			Expect(spec.ReviewSource).To(Equal(notificationv1.ReviewSourceAIAnalysis))
		})
	})

	Context("UT-NOT-453A-002: DeliveryAttemptStatus enum validation", func() {
		It("should define DeliveryAttemptStatusSuccess with value success", func() {
			Expect(string(notificationv1.DeliveryAttemptStatusSuccess)).To(Equal("success"))
		})

		It("should define DeliveryAttemptStatusFailed with value failed", func() {
			Expect(string(notificationv1.DeliveryAttemptStatusFailed)).To(Equal("failed"))
		})

		It("should define DeliveryAttemptStatusTimeout with value timeout", func() {
			Expect(string(notificationv1.DeliveryAttemptStatusTimeout)).To(Equal("timeout"))
		})

		It("should define DeliveryAttemptStatusInvalid with value invalid", func() {
			Expect(string(notificationv1.DeliveryAttemptStatusInvalid)).To(Equal("invalid"))
		})
	})

	Context("UT-NOT-453A-003: NotificationStatusReason constants completeness", func() {
		It("should define AllDeliveriesSucceeded", func() {
			Expect(string(notificationv1.StatusReasonAllDeliveriesSucceeded)).To(Equal("AllDeliveriesSucceeded"))
		})

		It("should define PartialDeliverySuccess", func() {
			Expect(string(notificationv1.StatusReasonPartialDeliverySuccess)).To(Equal("PartialDeliverySuccess"))
		})

		It("should define AllDeliveriesFailed", func() {
			Expect(string(notificationv1.StatusReasonAllDeliveriesFailed)).To(Equal("AllDeliveriesFailed"))
		})

		It("should define NoChannelsResolved", func() {
			Expect(string(notificationv1.StatusReasonNoChannelsResolved)).To(Equal("NoChannelsResolved"))
		})

		It("should define PartialFailureRetrying", func() {
			Expect(string(notificationv1.StatusReasonPartialFailureRetrying)).To(Equal("PartialFailureRetrying"))
		})
	})

	Context("UT-NOT-453A-004: Routing attribute map with typed ReviewSource", func() {
		It("should produce review-source attribute identical to old string behavior", func() {
			nr := &notificationv1.NotificationRequest{
				Spec: notificationv1.NotificationRequestSpec{
					Type:         notificationv1.NotificationTypeManualReview,
					Priority:     notificationv1.NotificationPriorityHigh,
					Severity:     "critical",
					ReviewSource: notificationv1.ReviewSourceAIAnalysis,
					Subject:      "Test",
					Body:         "Test body",
				},
			}
			attrs := routing.RoutingAttributesFromSpec(nr)
			Expect(attrs["review-source"]).To(Equal("AIAnalysis"))
			Expect(attrs["type"]).To(Equal("ManualReview"))
			Expect(attrs["severity"]).To(Equal("critical"))
			Expect(attrs["priority"]).To(Equal("High"))
		})

		It("should produce review-source for WorkflowExecution source", func() {
			nr := &notificationv1.NotificationRequest{
				Spec: notificationv1.NotificationRequestSpec{
					Type:         notificationv1.NotificationTypeManualReview,
					Priority:     notificationv1.NotificationPriorityCritical,
					Severity:     "warning",
					ReviewSource: notificationv1.ReviewSourceWorkflowExecution,
					Subject:      "Test",
					Body:         "Test body",
				},
			}
			attrs := routing.RoutingAttributesFromSpec(nr)
			Expect(attrs["review-source"]).To(Equal("WorkflowExecution"))
		})
	})
})
