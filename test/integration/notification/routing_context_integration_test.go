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

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/routing"
)

// Issue #453 Phase B: routing resolver consumes typed Context + Extensions (BR-NOT-065).

var _ = Describe("IT-NOT-453B-001: Routing with typed Context and Extensions", Label("integration", "routing-context"), func() {
	It("should produce routing attributes from Context.FlattenToMap and Extensions", func() {
		nr := &notificationv1alpha1.NotificationRequest{
			Spec: notificationv1alpha1.NotificationRequestSpec{
				Type:         notificationv1alpha1.NotificationTypeManualReview,
				Priority:     notificationv1alpha1.NotificationPriorityHigh,
				Severity:     "critical",
				ReviewSource: notificationv1alpha1.ReviewSourceAIAnalysis,
				Subject:      "IT-453B-001: Routing Context Test",
				Body:         "Integration test for routing with typed Context",
				Context: &notificationv1alpha1.NotificationContext{
					Lineage: &notificationv1alpha1.LineageContext{
						RemediationRequest: "rr-routing-001",
						AIAnalysis:         "ai-routing-001",
					},
					Review: &notificationv1alpha1.ReviewContext{
						Reason:    "WorkflowResolutionFailed",
						SubReason: "WorkflowNotFound",
					},
				},
				Extensions: map[string]string{
					"environment": "production",
					"skip-reason": "RecentlyRemediated",
				},
			},
		}
		attrs := routing.RoutingAttributesFromSpec(nr)

		// Spec-derived attributes
		Expect(attrs["type"]).To(Equal("ManualReview"))
		Expect(attrs["severity"]).To(Equal("critical"))
		Expect(attrs["priority"]).To(Equal("High"))
		Expect(attrs["review-source"]).To(Equal("AIAnalysis"))

		// Context-derived attributes (via FlattenToMap)
		Expect(attrs["remediationRequest"]).To(Equal("rr-routing-001"))
		Expect(attrs["aiAnalysis"]).To(Equal("ai-routing-001"))
		Expect(attrs["reason"]).To(Equal("WorkflowResolutionFailed"))
		Expect(attrs["subReason"]).To(Equal("WorkflowNotFound"))

		// Extensions-derived attributes
		Expect(attrs["environment"]).To(Equal("production"))
		Expect(attrs["skip-reason"]).To(Equal("RecentlyRemediated"))
	})
})
