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

package workflowexecution

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	weconditions "github.com/jordigilh/kubernaut/pkg/workflowexecution"
)

// ========================================
// BR-WE-006: Kubernetes Conditions Infrastructure
// Unit Tests for conditions.go
// Per TESTING_GUIDELINES.md: These are UNIT tests (implementation correctness), NOT BR tests
// ========================================

var _ = Describe("Conditions Infrastructure", func() {
	var wfe *workflowexecutionv1alpha1.WorkflowExecution

	BeforeEach(func() {
		wfe = &workflowexecutionv1alpha1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-wfe",
				Namespace: "default",
			},
		}
	})

	// ========================================
	// Test Condition Setters (5 conditions × 2 tests each = 10 tests)
	// ========================================

	Describe("SetExecutionCreated", func() {
		Context("when execution resource created successfully", func() {
			It("should set condition to True with ExecutionCreated reason", func() {
				weconditions.SetExecutionCreated(wfe, true,
					weconditions.ReasonExecutionCreated,
					"PipelineRun test-pr created in kubernaut-workflows")

				Expect(wfe.Status.Conditions).To(HaveLen(1))
				condition := wfe.Status.Conditions[0]
				Expect(condition.Type).To(Equal(weconditions.ConditionExecutionCreated))
				Expect(condition.Status).To(Equal(metav1.ConditionTrue))
				Expect(condition.Reason).To(Equal(weconditions.ReasonExecutionCreated))
				Expect(condition.Message).To(ContainSubstring("test-pr"))
				Expect(condition.LastTransitionTime).ToNot(BeNil())
			})
		})

		Context("when execution resource creation fails", func() {
			It("should set condition to False with QuotaExceeded reason", func() {
				weconditions.SetExecutionCreated(wfe, false,
					weconditions.ReasonQuotaExceeded,
					"Failed to create PipelineRun: pods exceeded quota")

				condition := weconditions.GetCondition(wfe, weconditions.ConditionExecutionCreated)
				Expect(condition).ToNot(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionFalse))
				Expect(condition.Reason).To(Equal(weconditions.ReasonQuotaExceeded))
				Expect(condition.Message).To(ContainSubstring("quota"))
			})
		})
	})

	Describe("SetExecutionRunning", func() {
		Context("when execution starts", func() {
			It("should set condition to True with ExecutionStarted reason", func() {
				weconditions.SetExecutionRunning(wfe, true,
					weconditions.ReasonExecutionStarted,
					"Pipeline executing task 2 of 5")

				condition := weconditions.GetCondition(wfe, weconditions.ConditionExecutionRunning)
				Expect(condition).ToNot(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionTrue))
				Expect(condition.Reason).To(Equal(weconditions.ReasonExecutionStarted))
				Expect(condition.Message).To(ContainSubstring("task 2 of 5"))
			})
		})

		Context("when execution fails to start", func() {
			It("should set condition to False with ExecutionFailedToStart reason", func() {
				weconditions.SetExecutionRunning(wfe, false,
					weconditions.ReasonExecutionFailedToStart,
					"Pipeline stuck in pending: node pressure")

				condition := weconditions.GetCondition(wfe, weconditions.ConditionExecutionRunning)
				Expect(condition).ToNot(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionFalse))
				Expect(condition.Reason).To(Equal(weconditions.ReasonExecutionFailedToStart))
			})
		})
	})

	Describe("SetExecutionComplete", func() {
		Context("when execution succeeds", func() {
			It("should set condition to True with ExecutionSucceeded reason", func() {
				weconditions.SetExecutionComplete(wfe, true,
					weconditions.ReasonExecutionSucceeded,
					"All 5 tasks completed successfully in 45s")

				condition := weconditions.GetCondition(wfe, weconditions.ConditionExecutionComplete)
				Expect(condition).ToNot(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionTrue))
				Expect(condition.Reason).To(Equal(weconditions.ReasonExecutionSucceeded))
				Expect(condition.Message).To(ContainSubstring("45s"))
			})
		})

		Context("when execution fails", func() {
			It("should set condition to False with TaskFailed reason", func() {
				weconditions.SetExecutionComplete(wfe, false,
					weconditions.ReasonTaskFailed,
					"Task apply-memory-increase failed: kubectl apply failed")

				condition := weconditions.GetCondition(wfe, weconditions.ConditionExecutionComplete)
				Expect(condition).ToNot(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionFalse))
				Expect(condition.Reason).To(Equal(weconditions.ReasonTaskFailed))
			})
		})
	})

	Describe("SetAuditRecorded", func() {
		Context("when audit event recorded successfully", func() {
			It("should set condition to True with AuditSucceeded reason", func() {
				weconditions.SetAuditRecorded(wfe, true,
					weconditions.ReasonAuditSucceeded,
					"Audit event workflowexecution.workflow.completed recorded")

				condition := weconditions.GetCondition(wfe, weconditions.ConditionAuditRecorded)
				Expect(condition).ToNot(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionTrue))
				Expect(condition.Reason).To(Equal(weconditions.ReasonAuditSucceeded))
			})
		})

		Context("when audit event fails", func() {
			It("should set condition to False with AuditFailed reason", func() {
				weconditions.SetAuditRecorded(wfe, false,
					weconditions.ReasonAuditFailed,
					"Failed to record audit event: DataStorage unavailable")

				condition := weconditions.GetCondition(wfe, weconditions.ConditionAuditRecorded)
				Expect(condition).ToNot(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionFalse))
				Expect(condition.Reason).To(Equal(weconditions.ReasonAuditFailed))
			})
		})
	})

	Describe("SetResourceLocked", func() {
		Context("when target resource is busy", func() {
			It("should set condition to True with TargetResourceBusy reason", func() {
				weconditions.SetResourceLocked(wfe, true,
					weconditions.ReasonTargetResourceBusy,
					"Another workflow is executing on target deployment/payment-api")

				condition := weconditions.GetCondition(wfe, weconditions.ConditionResourceLocked)
				Expect(condition).ToNot(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionTrue))
				Expect(condition.Reason).To(Equal(weconditions.ReasonTargetResourceBusy))
			})
		})

		Context("when target recently remediated", func() {
			It("should set condition to True with RecentlyRemediated reason", func() {
				weconditions.SetResourceLocked(wfe, true,
					weconditions.ReasonRecentlyRemediated,
					"Same workflow executed on target 30s ago (cooldown: 5m)")

				condition := weconditions.GetCondition(wfe, weconditions.ConditionResourceLocked)
				Expect(condition).ToNot(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionTrue))
				Expect(condition.Reason).To(Equal(weconditions.ReasonRecentlyRemediated))
			})
		})
	})

	// ========================================
	// Test Utility Functions (3 utility functions = 5 tests)
	// ========================================

	Describe("GetCondition", func() {
		Context("when condition exists", func() {
			It("should return the condition", func() {
				weconditions.SetExecutionCreated(wfe, true,
					weconditions.ReasonExecutionCreated, "Test")

				condition := weconditions.GetCondition(wfe, weconditions.ConditionExecutionCreated)
				Expect(condition).ToNot(BeNil())
				Expect(condition.Type).To(Equal(weconditions.ConditionExecutionCreated))
			})
		})

		Context("when condition doesn't exist", func() {
			It("should return nil", func() {
				condition := weconditions.GetCondition(wfe, weconditions.ConditionExecutionCreated)
				Expect(condition).To(BeNil())
			})
		})
	})

	Describe("IsConditionTrue", func() {
		Context("when condition exists and is True", func() {
			It("should return true", func() {
				weconditions.SetExecutionCreated(wfe, true,
					weconditions.ReasonExecutionCreated, "Test")

				isTrue := weconditions.IsConditionTrue(wfe, weconditions.ConditionExecutionCreated)
				Expect(isTrue).To(BeTrue())
			})
		})

		Context("when condition exists but is False", func() {
			It("should return false", func() {
				weconditions.SetExecutionCreated(wfe, false,
					weconditions.ReasonExecutionCreationFailed, "Test")

				isTrue := weconditions.IsConditionTrue(wfe, weconditions.ConditionExecutionCreated)
				Expect(isTrue).To(BeFalse())
			})
		})

		Context("when condition doesn't exist", func() {
			It("should return false", func() {
				isTrue := weconditions.IsConditionTrue(wfe, weconditions.ConditionExecutionCreated)
				Expect(isTrue).To(BeFalse())
			})
		})
	})

	// ========================================
	// Test Condition Transitions (3 tests)
	// ========================================

	Describe("Condition Transitions", func() {
		It("should update lastTransitionTime on status change", func() {
			// Set condition to True
			weconditions.SetExecutionCreated(wfe, true,
				weconditions.ReasonExecutionCreated, "Created")
			condition1 := weconditions.GetCondition(wfe, weconditions.ConditionExecutionCreated)
			time1 := condition1.LastTransitionTime

			// Wait brief moment (acceptable use of time.Sleep for timing test)
			time.Sleep(10 * time.Millisecond)

			// Change condition to False
			weconditions.SetExecutionCreated(wfe, false,
				weconditions.ReasonExecutionCreationFailed, "Failed")
			condition2 := weconditions.GetCondition(wfe, weconditions.ConditionExecutionCreated)
			time2 := condition2.LastTransitionTime

			// Verify timestamp updated
			Expect(time2.After(time1.Time)).To(BeTrue(),
				"LastTransitionTime should be updated on status change")
		})

		It("should preserve message and reason on each update", func() {
			// First update
			weconditions.SetExecutionCreated(wfe, true,
				weconditions.ReasonExecutionCreated, "First message")

			condition1 := weconditions.GetCondition(wfe, weconditions.ConditionExecutionCreated)
			Expect(condition1.Message).To(Equal("First message"))

			// Second update
			weconditions.SetExecutionCreated(wfe, true,
				weconditions.ReasonExecutionCreated, "Updated message")

			condition2 := weconditions.GetCondition(wfe, weconditions.ConditionExecutionCreated)
			Expect(condition2.Message).To(Equal("Updated message"),
				"Message should be updated on subsequent SetCondition calls")
		})

		It("should maintain multiple conditions independently", func() {
			// Set multiple conditions
			weconditions.SetExecutionCreated(wfe, true,
				weconditions.ReasonExecutionCreated, "Pipeline created")
			weconditions.SetExecutionRunning(wfe, true,
				weconditions.ReasonExecutionStarted, "Pipeline started")
			weconditions.SetAuditRecorded(wfe, true,
				weconditions.ReasonAuditSucceeded, "Audit recorded")

			// Verify all 3 conditions exist
			Expect(wfe.Status.Conditions).To(HaveLen(3))

			// Verify each condition independently
			Expect(weconditions.IsConditionTrue(wfe, weconditions.ConditionExecutionCreated)).To(BeTrue())
			Expect(weconditions.IsConditionTrue(wfe, weconditions.ConditionExecutionRunning)).To(BeTrue())
			Expect(weconditions.IsConditionTrue(wfe, weconditions.ConditionAuditRecorded)).To(BeTrue())

			// Update one condition shouldn't affect others
			weconditions.SetExecutionRunning(wfe, false,
				weconditions.ReasonExecutionFailedToStart, "Failed to start")

			Expect(wfe.Status.Conditions).To(HaveLen(3),
				"Updating one condition shouldn't change condition count")
			Expect(weconditions.IsConditionTrue(wfe, weconditions.ConditionExecutionCreated)).To(BeTrue(),
				"Other conditions should remain unchanged")
			Expect(weconditions.IsConditionTrue(wfe, weconditions.ConditionExecutionRunning)).To(BeFalse(),
				"Updated condition should reflect new status")
		})
	})

	// ========================================
	// Test Condition Reason Constants (validate they're used correctly)
	// ========================================

	Describe("Condition Reason Mapping", func() {
		It("should support all ExecutionCreated failure reasons", func() {
			// Test quota exceeded
			weconditions.SetExecutionCreated(wfe, false,
				weconditions.ReasonQuotaExceeded, "Quota exceeded")
			condition := weconditions.GetCondition(wfe, weconditions.ConditionExecutionCreated)
			Expect(condition.Reason).To(Equal(weconditions.ReasonQuotaExceeded))

			// Test RBAC denied
			weconditions.SetExecutionCreated(wfe, false,
				weconditions.ReasonRBACDenied, "RBAC denied")
			condition = weconditions.GetCondition(wfe, weconditions.ConditionExecutionCreated)
			Expect(condition.Reason).To(Equal(weconditions.ReasonRBACDenied))

			// Test image pull failed
			weconditions.SetExecutionCreated(wfe, false,
				weconditions.ReasonImagePullFailed, "Image pull failed")
			condition = weconditions.GetCondition(wfe, weconditions.ConditionExecutionCreated)
			Expect(condition.Reason).To(Equal(weconditions.ReasonImagePullFailed))
		})

		It("should support all ExecutionComplete failure reasons", func() {
			// Test task failed
			weconditions.SetExecutionComplete(wfe, false,
				weconditions.ReasonTaskFailed, "Task failed")
			condition := weconditions.GetCondition(wfe, weconditions.ConditionExecutionComplete)
			Expect(condition.Reason).To(Equal(weconditions.ReasonTaskFailed))

			// Test deadline exceeded
			weconditions.SetExecutionComplete(wfe, false,
				weconditions.ReasonDeadlineExceeded, "Timeout")
			condition = weconditions.GetCondition(wfe, weconditions.ConditionExecutionComplete)
			Expect(condition.Reason).To(Equal(weconditions.ReasonDeadlineExceeded))

			// Test OOM killed
			weconditions.SetExecutionComplete(wfe, false,
				weconditions.ReasonOOMKilled, "Out of memory")
			condition = weconditions.GetCondition(wfe, weconditions.ConditionExecutionComplete)
			Expect(condition.Reason).To(Equal(weconditions.ReasonOOMKilled))
		})

		It("should support all ResourceLocked reasons", func() {
			// Test target resource busy
			weconditions.SetResourceLocked(wfe, true,
				weconditions.ReasonTargetResourceBusy, "Resource busy")
			condition := weconditions.GetCondition(wfe, weconditions.ConditionResourceLocked)
			Expect(condition.Reason).To(Equal(weconditions.ReasonTargetResourceBusy))

			// Test recently remediated
			weconditions.SetResourceLocked(wfe, true,
				weconditions.ReasonRecentlyRemediated, "Recently remediated")
			condition = weconditions.GetCondition(wfe, weconditions.ConditionResourceLocked)
			Expect(condition.Reason).To(Equal(weconditions.ReasonRecentlyRemediated))

			// Test previous execution failed
			weconditions.SetResourceLocked(wfe, true,
				weconditions.ReasonPreviousExecutionFailed, "Previous failed")
			condition = weconditions.GetCondition(wfe, weconditions.ConditionResourceLocked)
			Expect(condition.Reason).To(Equal(weconditions.ReasonPreviousExecutionFailed))
		})
	})

	// ========================================
	// Test Complete Lifecycle (integration-style unit test)
	// ========================================

	Describe("Complete Condition Lifecycle", func() {
		It("should track full workflow execution lifecycle via conditions", func() {
			// 1. PipelineRun created
			weconditions.SetExecutionCreated(wfe, true,
				weconditions.ReasonExecutionCreated, "PipelineRun created")
			Expect(weconditions.IsConditionTrue(wfe, weconditions.ConditionExecutionCreated)).To(BeTrue())

			// 2. Pipeline starts running
			weconditions.SetExecutionRunning(wfe, true,
				weconditions.ReasonExecutionStarted, "Pipeline started")
			Expect(weconditions.IsConditionTrue(wfe, weconditions.ConditionExecutionRunning)).To(BeTrue())

			// 3. Pipeline completes successfully
			weconditions.SetExecutionComplete(wfe, true,
				weconditions.ReasonExecutionSucceeded, "All tasks completed")
			Expect(weconditions.IsConditionTrue(wfe, weconditions.ConditionExecutionComplete)).To(BeTrue())

			// 4. Audit event recorded
			weconditions.SetAuditRecorded(wfe, true,
				weconditions.ReasonAuditSucceeded, "Audit event recorded")
			Expect(weconditions.IsConditionTrue(wfe, weconditions.ConditionAuditRecorded)).To(BeTrue())

			// Verify all conditions present
			Expect(wfe.Status.Conditions).To(HaveLen(4),
				"Complete lifecycle should have 4 conditions")

			// Verify each condition is True
			for _, condition := range wfe.Status.Conditions {
				Expect(condition.Status).To(Equal(metav1.ConditionTrue),
					"All conditions should be True in success scenario")
			}
		})

		It("should track resource lock condition (legacy - V1.0: RO prevents locked WFE creation)", func() {
			// V1.0 NOTE: In V1.0, RO prevents creation of WFEs on locked resources (DD-RO-002)
			// This test validates the condition infrastructure still works for edge cases
			// where a WFE might be created before RO's routing decision completes

			// Resource lock detected
			weconditions.SetResourceLocked(wfe, true,
				weconditions.ReasonTargetResourceBusy,
				"Another workflow executing on target")

			// Audit event for workflow state change
			weconditions.SetAuditRecorded(wfe, true,
				weconditions.ReasonAuditSucceeded,
				"Audit event for workflow state recorded")

			// Verify resource lock condition tracked
			Expect(wfe.Status.Conditions).To(HaveLen(2))
			Expect(weconditions.IsConditionTrue(wfe, weconditions.ConditionResourceLocked)).To(BeTrue())
			Expect(weconditions.IsConditionTrue(wfe, weconditions.ConditionAuditRecorded)).To(BeTrue())

			// ExecutionCreated should NOT be set (no execution resource created when locked)
			Expect(weconditions.GetCondition(wfe, weconditions.ConditionExecutionCreated)).To(BeNil())
		})
	})
})
