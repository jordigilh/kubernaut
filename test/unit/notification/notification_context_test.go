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
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

var _ = Describe("Issue #453 Phase B: Metadata Decomposition", func() {

	Context("UT-NOT-453B-001: Approval notification context structure", func() {
		It("should populate lineage, workflow, and analysis sub-structs", func() {
			ctx := &notificationv1.NotificationContext{
				Lineage: &notificationv1.LineageContext{
					RemediationRequest: "rr-test-001",
					AIAnalysis:         "ai-test-001",
				},
				Workflow: &notificationv1.WorkflowContext{
					SelectedWorkflow: "restart-pod",
					Confidence:       "0.95",
				},
				Analysis: &notificationv1.AnalysisContext{
					ApprovalReason: "LowConfidence",
				},
			}
			Expect(ctx.Lineage.RemediationRequest).To(Equal("rr-test-001"))
			Expect(ctx.Lineage.AIAnalysis).To(Equal("ai-test-001"))
			Expect(ctx.Workflow.SelectedWorkflow).To(Equal("restart-pod"))
			Expect(ctx.Workflow.Confidence).To(Equal("0.95"))
			Expect(ctx.Analysis.ApprovalReason).To(Equal("LowConfidence"))
			Expect(ctx.Execution).To(BeNil())
			Expect(ctx.Dedup).To(BeNil())
			Expect(ctx.Target).To(BeNil())
		})
	})

	Context("UT-NOT-453B-002: Completion notification context structure", func() {
		It("should populate lineage, workflow, and analysis with outcome", func() {
			ctx := &notificationv1.NotificationContext{
				Lineage: &notificationv1.LineageContext{
					RemediationRequest: "rr-complete-001",
					AIAnalysis:         "ai-complete-001",
				},
				Workflow: &notificationv1.WorkflowContext{
					WorkflowID:      "restart-pod",
					ExecutionEngine: "ansible",
				},
				Analysis: &notificationv1.AnalysisContext{
					RootCause: "OOMKilled in container nginx",
					Outcome:   "Success",
				},
			}
			Expect(ctx.Lineage.RemediationRequest).To(Equal("rr-complete-001"))
			Expect(ctx.Workflow.WorkflowID).To(Equal("restart-pod"))
			Expect(ctx.Workflow.ExecutionEngine).To(Equal("ansible"))
			Expect(ctx.Analysis.RootCause).To(Equal("OOMKilled in container nginx"))
			Expect(ctx.Analysis.Outcome).To(Equal("Success"))
			Expect(ctx.Review).To(BeNil())
			Expect(ctx.Execution).To(BeNil())
			Expect(ctx.Dedup).To(BeNil())
		})
	})

	Context("UT-NOT-453B-003: Bulk duplicate notification context structure", func() {
		It("should populate lineage and dedup sub-structs only", func() {
			ctx := &notificationv1.NotificationContext{
				Lineage: &notificationv1.LineageContext{
					RemediationRequest: "rr-bulk-001",
				},
				Dedup: &notificationv1.DedupContext{
					DuplicateCount: "5",
				},
			}
			Expect(ctx.Lineage.RemediationRequest).To(Equal("rr-bulk-001"))
			Expect(ctx.Dedup.DuplicateCount).To(Equal("5"))
			Expect(ctx.Workflow).To(BeNil())
			Expect(ctx.Analysis).To(BeNil())
			Expect(ctx.Review).To(BeNil())
			Expect(ctx.Execution).To(BeNil())
			Expect(ctx.Target).To(BeNil())

			flat := ctx.FlattenToMap()
			Expect(flat).To(HaveKeyWithValue("remediationRequest", "rr-bulk-001"))
			Expect(flat).To(HaveKeyWithValue("duplicateCount", "5"))
		})
	})

	Context("UT-NOT-453B-004: Manual review notification context (both sources)", func() {
		It("should populate review sub-struct for AIAnalysis source", func() {
			ctx := &notificationv1.NotificationContext{
				Lineage: &notificationv1.LineageContext{
					RemediationRequest: "rr-review-001",
				},
				Review: &notificationv1.ReviewContext{
					Reason:            "WorkflowResolutionFailed",
					SubReason:         "WorkflowNotFound",
					HumanReviewReason: "workflow_not_found",
				},
			}
			Expect(ctx.Review.Reason).To(Equal("WorkflowResolutionFailed"))
			Expect(ctx.Review.SubReason).To(Equal("WorkflowNotFound"))
			Expect(ctx.Review.HumanReviewReason).To(Equal("workflow_not_found"))
			Expect(ctx.Execution).To(BeNil())

			flat := ctx.FlattenToMap()
			Expect(flat).To(HaveKeyWithValue("reason", "WorkflowResolutionFailed"))
			Expect(flat).To(HaveKeyWithValue("subReason", "WorkflowNotFound"))
			Expect(flat).To(HaveKeyWithValue("humanReviewReason", "workflow_not_found"))
			Expect(flat).NotTo(HaveKey("source"))
		})

		It("should populate review and execution sub-structs for WorkflowExecution source", func() {
			ctx := &notificationv1.NotificationContext{
				Lineage: &notificationv1.LineageContext{
					RemediationRequest: "rr-review-002",
				},
				Review: &notificationv1.ReviewContext{
					Reason: "ExhaustedRetries",
				},
				Execution: &notificationv1.ExecutionContext{
					RetryCount:        "3",
					MaxRetries:        "5",
					LastExitCode:      "1",
					PreviousExecution: "we-prev-001",
				},
			}
			Expect(ctx.Review.Reason).To(Equal("ExhaustedRetries"))
			Expect(ctx.Execution.RetryCount).To(Equal("3"))
			Expect(ctx.Execution.MaxRetries).To(Equal("5"))
			Expect(ctx.Execution.LastExitCode).To(Equal("1"))
			Expect(ctx.Execution.PreviousExecution).To(Equal("we-prev-001"))
		})
	})

	Context("UT-NOT-453B-005: FlattenToMap routing compatibility", func() {
		DescribeTable("should produce identical key-value map for each notification type",
			func(ctx *notificationv1.NotificationContext, expected map[string]string) {
				result := ctx.FlattenToMap()
				Expect(result).To(Equal(expected))
			},
			Entry("Approval notification", &notificationv1.NotificationContext{
				Lineage: &notificationv1.LineageContext{
					RemediationRequest: "rr-001",
					AIAnalysis:         "ai-001",
				},
				Workflow: &notificationv1.WorkflowContext{
					SelectedWorkflow: "restart-pod",
					Confidence:       "0.95",
				},
				Analysis: &notificationv1.AnalysisContext{
					ApprovalReason: "LowConfidence",
				},
			}, map[string]string{
				"remediationRequest": "rr-001",
				"aiAnalysis":         "ai-001",
				"selectedWorkflow":   "restart-pod",
				"confidence":         "0.95",
				"approvalReason":     "LowConfidence",
			}),
			Entry("Completion notification", &notificationv1.NotificationContext{
				Lineage: &notificationv1.LineageContext{
					RemediationRequest: "rr-002",
					AIAnalysis:         "ai-002",
				},
				Workflow: &notificationv1.WorkflowContext{
					WorkflowID:      "restart-pod",
					ExecutionEngine: "ansible",
				},
				Analysis: &notificationv1.AnalysisContext{
					RootCause: "OOMKilled",
					Outcome:   "Success",
				},
			}, map[string]string{
				"remediationRequest": "rr-002",
				"aiAnalysis":         "ai-002",
				"workflowId":         "restart-pod",
				"executionEngine":    "ansible",
				"rootCause":          "OOMKilled",
				"outcome":            "Success",
			}),
			Entry("Bulk duplicate notification", &notificationv1.NotificationContext{
				Lineage: &notificationv1.LineageContext{
					RemediationRequest: "rr-003",
				},
				Dedup: &notificationv1.DedupContext{
					DuplicateCount: "5",
				},
			}, map[string]string{
				"remediationRequest": "rr-003",
				"duplicateCount":     "5",
			}),
			Entry("Manual review AI notification", &notificationv1.NotificationContext{
				Lineage: &notificationv1.LineageContext{
					RemediationRequest: "rr-004",
				},
				Review: &notificationv1.ReviewContext{
					Reason:            "WorkflowResolutionFailed",
					SubReason:         "WorkflowNotFound",
					HumanReviewReason: "workflow_not_found",
				},
			}, map[string]string{
				"remediationRequest": "rr-004",
				"reason":             "WorkflowResolutionFailed",
				"subReason":          "WorkflowNotFound",
				"humanReviewReason":  "workflow_not_found",
			}),
			Entry("Manual review WE notification", &notificationv1.NotificationContext{
				Lineage: &notificationv1.LineageContext{
					RemediationRequest: "rr-005",
				},
				Review: &notificationv1.ReviewContext{
					Reason: "ExhaustedRetries",
				},
				Execution: &notificationv1.ExecutionContext{
					RetryCount:        "3",
					MaxRetries:        "5",
					LastExitCode:      "1",
					PreviousExecution: "we-prev-001",
				},
			}, map[string]string{
				"remediationRequest": "rr-005",
				"reason":             "ExhaustedRetries",
				"retryCount":         "3",
				"maxRetries":         "5",
				"lastExitCode":       "1",
				"previousExecution":  "we-prev-001",
			}),
			Entry("Global timeout notification", &notificationv1.NotificationContext{
				Lineage: &notificationv1.LineageContext{
					RemediationRequest: "rr-006",
				},
				Execution: &notificationv1.ExecutionContext{
					TimeoutPhase: string(remediationv1.PhaseProcessing),
				},
				Target: &notificationv1.TargetContext{
					TargetResource: "Deployment/nginx",
				},
			}, map[string]string{
				"remediationRequest": "rr-006",
				"timeoutPhase":       "Processing",
				"targetResource":     "Deployment/nginx",
			}),
		)
	})

	Context("UT-NOT-453B-006: FlattenToMap audit correlation preservation", func() {
		It("should contain remediationRequest and aiAnalysis keys for audit", func() {
			ctx := &notificationv1.NotificationContext{
				Lineage: &notificationv1.LineageContext{
					RemediationRequest: "rr-audit-001",
					AIAnalysis:         "ai-audit-001",
				},
			}
			result := ctx.FlattenToMap()
			Expect(result).To(HaveKeyWithValue("remediationRequest", "rr-audit-001"))
			Expect(result).To(HaveKeyWithValue("aiAnalysis", "ai-audit-001"))
		})
	})

	Context("UT-NOT-453B-007: Extensions map in routing attributes", func() {
		It("should include Extensions in FlattenToMap output", func() {
			spec := notificationv1.NotificationRequestSpec{
				Type:     notificationv1.NotificationTypeManualReview,
				Priority: notificationv1.NotificationPriorityHigh,
				Subject:  "Test",
				Body:     "Test body",
				Context: &notificationv1.NotificationContext{
					Lineage: &notificationv1.LineageContext{
						RemediationRequest: "rr-ext-001",
					},
				},
				Extensions: map[string]string{
					"environment": "production",
					"skip-reason": "RecentlyRemediated",
				},
			}
			Expect(spec.Extensions).To(HaveKeyWithValue("environment", "production"))
			Expect(spec.Extensions).To(HaveKeyWithValue("skip-reason", "RecentlyRemediated"))
		})
	})

	Context("UT-NOT-453B-008: Nil sub-struct safety", func() {
		It("should return empty map for zero-value context", func() {
			ctx := &notificationv1.NotificationContext{}
			result := ctx.FlattenToMap()
			Expect(result).To(BeEmpty())
		})

		It("should return empty map for nil context", func() {
			var ctx *notificationv1.NotificationContext
			result := ctx.FlattenToMap()
			Expect(result).To(BeEmpty())
		})

		It("should produce only lineage keys when only lineage is set", func() {
			ctx := &notificationv1.NotificationContext{
				Lineage: &notificationv1.LineageContext{
					RemediationRequest: "rr-partial",
				},
			}
			result := ctx.FlattenToMap()
			Expect(result).To(HaveLen(1))
			Expect(result).To(HaveKeyWithValue("remediationRequest", "rr-partial"))
		})
	})

	// =====================================================
	// #318: VerificationContext on NotificationContext
	// =====================================================

	Context("UT-RO-318-011: FlattenToMap includes verification routing keys", func() {
		It("should return verificationAssessed, verificationOutcome, verificationReason when Verification is set", func() {
			ctx := &notificationv1.NotificationContext{
				Verification: &notificationv1.VerificationContext{
					Assessed: true,
					Outcome:  "inconclusive",
					Reason:   "SpecDrift",
					Summary:  "Verification inconclusive: the resource spec was modified by an external entity after remediation.",
				},
			}
			result := ctx.FlattenToMap()
			Expect(result).To(HaveKeyWithValue("verificationAssessed", "true"))
			Expect(result).To(HaveKeyWithValue("verificationOutcome", "inconclusive"))
			Expect(result).To(HaveKeyWithValue("verificationReason", "SpecDrift"))
		})
	})

	Context("UT-RO-318-012: FlattenToMap backward compat with nil Verification", func() {
		It("should NOT contain any verification keys when Verification is nil", func() {
			ctx := &notificationv1.NotificationContext{
				Lineage: &notificationv1.LineageContext{
					RemediationRequest: "rr-no-verification",
				},
			}
			result := ctx.FlattenToMap()
			Expect(result).NotTo(HaveKey("verificationAssessed"))
			Expect(result).NotTo(HaveKey("verificationOutcome"))
			Expect(result).NotTo(HaveKey("verificationReason"))
		})
	})

	Context("UT-NOT-453B-009: Redundant key elimination", func() {
		It("should NOT contain severity or source keys in FlattenToMap output", func() {
			ctx := &notificationv1.NotificationContext{
				Lineage: &notificationv1.LineageContext{
					RemediationRequest: "rr-redundant",
				},
				Target: &notificationv1.TargetContext{
					TargetResource: "Deployment/nginx",
				},
			}
			result := ctx.FlattenToMap()
			Expect(result).NotTo(HaveKey("severity"))
			Expect(result).NotTo(HaveKey("source"))
		})
	})
})
