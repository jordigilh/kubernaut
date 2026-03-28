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
)

// Issue #453 Phase B: audit metadata shape matches Context + Extensions merge (BR-NOT-064),
// mirroring notification audit manager flatten behavior without calling unexported helpers.

var _ = Describe("IT-NOT-453B-002: Audit payload metadata preservation", Label("integration", "audit-context"), func() {
	It("should preserve all metadata keys for audit correlation after Context migration", func() {
		nr := &notificationv1alpha1.NotificationRequest{
			Spec: notificationv1alpha1.NotificationRequestSpec{
				Type:     notificationv1alpha1.NotificationTypeApproval,
				Priority: notificationv1alpha1.NotificationPriorityHigh,
				Severity: "critical",
				Subject:  "IT-453B-002: Audit Context",
				Body:     "Integration test for audit metadata preservation",
				Context: &notificationv1alpha1.NotificationContext{
					Lineage: &notificationv1alpha1.LineageContext{
						RemediationRequest: "rr-audit-001",
						AIAnalysis:         "ai-audit-001",
					},
					Workflow: &notificationv1alpha1.WorkflowContext{
						SelectedWorkflow: "restart-pod",
						Confidence:       "0.95",
					},
					Analysis: &notificationv1alpha1.AnalysisContext{
						ApprovalReason: "LowConfidence",
					},
				},
				Extensions: map[string]string{
					"cluster": "prod-east-1",
				},
			},
		}

		// Simulate what audit manager does: flatten Context + merge Extensions
		flatMeta := make(map[string]string)
		if nr.Spec.Context != nil {
			for k, v := range nr.Spec.Context.FlattenToMap() {
				flatMeta[k] = v
			}
		}
		for k, v := range nr.Spec.Extensions {
			flatMeta[k] = v
		}

		// Verify audit-critical keys are present
		Expect(flatMeta).To(HaveKeyWithValue("remediationRequest", "rr-audit-001"))
		Expect(flatMeta).To(HaveKeyWithValue("aiAnalysis", "ai-audit-001"))
		Expect(flatMeta).To(HaveKeyWithValue("selectedWorkflow", "restart-pod"))
		Expect(flatMeta).To(HaveKeyWithValue("confidence", "0.95"))
		Expect(flatMeta).To(HaveKeyWithValue("approvalReason", "LowConfidence"))
		Expect(flatMeta).To(HaveKeyWithValue("cluster", "prod-east-1"))
		// Redundant keys should NOT be present
		Expect(flatMeta).NotTo(HaveKey("severity"))
		Expect(flatMeta).NotTo(HaveKey("source"))
	})
})
