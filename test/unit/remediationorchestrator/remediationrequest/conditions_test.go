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

package remediationrequest

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationrequest"
)

func TestRemediationRequestConditions(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RemediationRequest Conditions Suite")
}

var _ = Describe("RemediationRequest Conditions", func() {
	var rr *remediationv1.RemediationRequest

	BeforeEach(func() {
		rr = &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-rr",
				Namespace: "default",
			},
			Status: remediationv1.RemediationRequestStatus{
				Conditions: []metav1.Condition{},
			},
		}
	})

	// ========================================
	// CONDITION CONSTANTS TESTS
	// ========================================

	Describe("Condition Type Constants", func() {
		It("should define all 6 pipeline condition types per DD-CRD-002-remediationrequest-conditions", func() {
			// BR-ORCH-043: 6 pipeline conditions for orchestration visibility (Ready is aggregate)
			Expect(remediationrequest.ConditionSignalProcessingReady).To(Equal("SignalProcessingReady"))
			Expect(remediationrequest.ConditionSignalProcessingComplete).To(Equal("SignalProcessingComplete"))
			Expect(remediationrequest.ConditionAIAnalysisReady).To(Equal("AIAnalysisReady"))
			Expect(remediationrequest.ConditionAIAnalysisComplete).To(Equal("AIAnalysisComplete"))
			Expect(remediationrequest.ConditionWorkflowExecutionReady).To(Equal("WorkflowExecutionReady"))
			Expect(remediationrequest.ConditionWorkflowExecutionComplete).To(Equal("WorkflowExecutionComplete"))
		})
	})

	Describe("Condition Reason Constants", func() {
		It("should define SignalProcessing reasons", func() {
			Expect(remediationrequest.ReasonSignalProcessingCreated).To(Equal("SignalProcessingCreated"))
			Expect(remediationrequest.ReasonSignalProcessingCreationFailed).To(Equal("SignalProcessingCreationFailed"))
			Expect(remediationrequest.ReasonSignalProcessingSucceeded).To(Equal("SignalProcessingSucceeded"))
			Expect(remediationrequest.ReasonSignalProcessingFailed).To(Equal("SignalProcessingFailed"))
			Expect(remediationrequest.ReasonSignalProcessingTimeout).To(Equal("SignalProcessingTimeout"))
		})

		It("should define AIAnalysis reasons", func() {
			Expect(remediationrequest.ReasonAIAnalysisCreated).To(Equal("AIAnalysisCreated"))
			Expect(remediationrequest.ReasonAIAnalysisCreationFailed).To(Equal("AIAnalysisCreationFailed"))
			Expect(remediationrequest.ReasonAIAnalysisSucceeded).To(Equal("AIAnalysisSucceeded"))
			Expect(remediationrequest.ReasonAIAnalysisFailed).To(Equal("AIAnalysisFailed"))
			Expect(remediationrequest.ReasonAIAnalysisTimeout).To(Equal("AIAnalysisTimeout"))
			Expect(remediationrequest.ReasonNoWorkflowSelected).To(Equal("NoWorkflowSelected"))
		})

		It("should define WorkflowExecution reasons", func() {
			Expect(remediationrequest.ReasonWorkflowExecutionCreated).To(Equal("WorkflowExecutionCreated"))
			Expect(remediationrequest.ReasonWorkflowExecutionCreationFailed).To(Equal("WorkflowExecutionCreationFailed"))
			Expect(remediationrequest.ReasonWorkflowSucceeded).To(Equal("WorkflowSucceeded"))
			Expect(remediationrequest.ReasonWorkflowFailed).To(Equal("WorkflowFailed"))
			Expect(remediationrequest.ReasonWorkflowTimeout).To(Equal("WorkflowTimeout"))
			Expect(remediationrequest.ReasonApprovalPending).To(Equal("ApprovalPending"))
		})

	})

	// ========================================
	// GENERIC CONDITION FUNCTIONS TESTS
	// ========================================

	Describe("SetCondition", func() {
		It("should set a new condition using meta.SetStatusCondition", func() {
			remediationrequest.SetCondition(rr, remediationrequest.ConditionSignalProcessingReady,
				metav1.ConditionTrue, remediationrequest.ReasonSignalProcessingCreated, "SP created", nil)

			Expect(rr.Status.Conditions).To(HaveLen(1))
			cond := rr.Status.Conditions[0]
			Expect(cond.Type).To(Equal(remediationrequest.ConditionSignalProcessingReady))
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(remediationrequest.ReasonSignalProcessingCreated))
			Expect(cond.Message).To(Equal("SP created"))
			Expect(cond.LastTransitionTime).ToNot(BeZero())
		})

		It("should update existing condition without duplicating", func() {
			// Set initial condition
			remediationrequest.SetCondition(rr, remediationrequest.ConditionSignalProcessingReady,
				metav1.ConditionFalse, remediationrequest.ReasonSignalProcessingCreationFailed, "Initial failure", nil)

			// Update same condition type
			remediationrequest.SetCondition(rr, remediationrequest.ConditionSignalProcessingReady,
				metav1.ConditionTrue, remediationrequest.ReasonSignalProcessingCreated, "Now succeeded", nil)

			// Should still have only 1 condition (no duplicates)
			Expect(rr.Status.Conditions).To(HaveLen(1))
			cond := rr.Status.Conditions[0]
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(remediationrequest.ReasonSignalProcessingCreated))
			Expect(cond.Message).To(Equal("Now succeeded"))
		})

		It("should support multiple different condition types", func() {
			remediationrequest.SetCondition(rr, remediationrequest.ConditionSignalProcessingReady,
				metav1.ConditionTrue, remediationrequest.ReasonSignalProcessingCreated, "SP ready", nil)
			remediationrequest.SetCondition(rr, remediationrequest.ConditionAIAnalysisReady,
				metav1.ConditionTrue, remediationrequest.ReasonAIAnalysisCreated, "AI ready", nil)
			remediationrequest.SetCondition(rr, remediationrequest.ConditionWorkflowExecutionReady,
				metav1.ConditionTrue, remediationrequest.ReasonWorkflowExecutionCreated, "WE ready", nil)

			Expect(rr.Status.Conditions).To(HaveLen(3))
		})
	})

	Describe("GetCondition", func() {
		It("should return nil when condition doesn't exist", func() {
			cond := remediationrequest.GetCondition(rr, remediationrequest.ConditionWorkflowExecutionComplete)
			Expect(cond).To(BeNil())
		})

		It("should return existing condition", func() {
			remediationrequest.SetCondition(rr, remediationrequest.ConditionWorkflowExecutionComplete,
				metav1.ConditionTrue, remediationrequest.ReasonWorkflowSucceeded, "Done", nil)

			cond := remediationrequest.GetCondition(rr, remediationrequest.ConditionWorkflowExecutionComplete)
			Expect(cond.Type).To(Equal(remediationrequest.ConditionWorkflowExecutionComplete))
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	// ========================================
	// TYPE-SPECIFIC SETTER TESTS
	// ========================================

	Describe("SetSignalProcessingReady", func() {
		It("should set True with SignalProcessingCreated reason on success", func() {
			remediationrequest.SetSignalProcessingReady(rr, true, "SP CRD sp-test created", nil)

			cond := remediationrequest.GetCondition(rr, remediationrequest.ConditionSignalProcessingReady)
			Expect(cond.Type).To(Equal(remediationrequest.ConditionSignalProcessingReady))
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(remediationrequest.ReasonSignalProcessingCreated))
			Expect(cond.Message).To(Equal("SP CRD sp-test created"))
		})

		It("should set False with SignalProcessingCreationFailed reason on failure", func() {
			remediationrequest.SetSignalProcessingReady(rr, false, "Failed to create SP: timeout", nil)

			cond := remediationrequest.GetCondition(rr, remediationrequest.ConditionSignalProcessingReady)
			Expect(cond.Type).To(Equal(remediationrequest.ConditionSignalProcessingReady))
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(remediationrequest.ReasonSignalProcessingCreationFailed))
		})
	})

	Describe("SetSignalProcessingComplete", func() {
		It("should set True with SignalProcessingSucceeded reason on success", func() {
			remediationrequest.SetSignalProcessingComplete(rr, true,
				remediationrequest.ReasonSignalProcessingSucceeded, "Completed (env: prod)", nil)

			cond := remediationrequest.GetCondition(rr, remediationrequest.ConditionSignalProcessingComplete)
			Expect(cond.Type).To(Equal(remediationrequest.ConditionSignalProcessingComplete))
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(remediationrequest.ReasonSignalProcessingSucceeded))
		})

		It("should set False with custom reason on failure", func() {
			remediationrequest.SetSignalProcessingComplete(rr, false,
				remediationrequest.ReasonSignalProcessingTimeout, "Timed out after 5m", nil)

			cond := remediationrequest.GetCondition(rr, remediationrequest.ConditionSignalProcessingComplete)
			Expect(cond.Type).To(Equal(remediationrequest.ConditionSignalProcessingComplete))
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(remediationrequest.ReasonSignalProcessingTimeout))
		})
	})

	Describe("SetAIAnalysisReady", func() {
		It("should set True with AIAnalysisCreated reason on success", func() {
			remediationrequest.SetAIAnalysisReady(rr, true, "AI CRD ai-test created", nil)

			cond := remediationrequest.GetCondition(rr, remediationrequest.ConditionAIAnalysisReady)
			Expect(cond.Type).To(Equal(remediationrequest.ConditionAIAnalysisReady))
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(remediationrequest.ReasonAIAnalysisCreated))
		})

		It("should set False with AIAnalysisCreationFailed reason on failure", func() {
			remediationrequest.SetAIAnalysisReady(rr, false, "Failed to create AI: resource quota exceeded", nil)

			cond := remediationrequest.GetCondition(rr, remediationrequest.ConditionAIAnalysisReady)
			Expect(cond.Type).To(Equal(remediationrequest.ConditionAIAnalysisReady))
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(remediationrequest.ReasonAIAnalysisCreationFailed))
		})
	})

	Describe("SetAIAnalysisComplete", func() {
		It("should set True with AIAnalysisSucceeded reason on success", func() {
			remediationrequest.SetAIAnalysisComplete(rr, true,
				remediationrequest.ReasonAIAnalysisSucceeded, "Completed (workflow: restart-pod)", nil)

			cond := remediationrequest.GetCondition(rr, remediationrequest.ConditionAIAnalysisComplete)
			Expect(cond.Type).To(Equal(remediationrequest.ConditionAIAnalysisComplete))
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(remediationrequest.ReasonAIAnalysisSucceeded))
		})

		It("should set False with NoWorkflowSelected reason when no workflow", func() {
			remediationrequest.SetAIAnalysisComplete(rr, false,
				remediationrequest.ReasonNoWorkflowSelected, "Analysis complete but no workflow selected", nil)

			cond := remediationrequest.GetCondition(rr, remediationrequest.ConditionAIAnalysisComplete)
			Expect(cond.Type).To(Equal(remediationrequest.ConditionAIAnalysisComplete))
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(remediationrequest.ReasonNoWorkflowSelected))
		})
	})

	Describe("SetWorkflowExecutionReady", func() {
		It("should set True with WorkflowExecutionCreated reason on success", func() {
			remediationrequest.SetWorkflowExecutionReady(rr, true, "WE CRD we-test created", nil)

			cond := remediationrequest.GetCondition(rr, remediationrequest.ConditionWorkflowExecutionReady)
			Expect(cond.Type).To(Equal(remediationrequest.ConditionWorkflowExecutionReady))
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(remediationrequest.ReasonWorkflowExecutionCreated))
		})

		It("should set False with ApprovalPending reason when waiting for approval", func() {
			remediationrequest.SetWorkflowExecutionReady(rr, false, "Waiting for approval", nil)

			cond := remediationrequest.GetCondition(rr, remediationrequest.ConditionWorkflowExecutionReady)
			Expect(cond.Type).To(Equal(remediationrequest.ConditionWorkflowExecutionReady))
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			// Default failure reason for WE ready is ApprovalPending
			Expect(cond.Reason).To(Equal(remediationrequest.ReasonWorkflowExecutionCreationFailed))
		})
	})

	Describe("SetWorkflowExecutionComplete", func() {
		It("should set True with WorkflowSucceeded reason on success", func() {
			remediationrequest.SetWorkflowExecutionComplete(rr, true,
				remediationrequest.ReasonWorkflowSucceeded, "Workflow executed successfully", nil)

			cond := remediationrequest.GetCondition(rr, remediationrequest.ConditionWorkflowExecutionComplete)
			Expect(cond.Type).To(Equal(remediationrequest.ConditionWorkflowExecutionComplete))
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(remediationrequest.ReasonWorkflowSucceeded))
		})

		It("should set False with WorkflowTimeout reason on timeout", func() {
			remediationrequest.SetWorkflowExecutionComplete(rr, false,
				remediationrequest.ReasonWorkflowTimeout, "Workflow timed out after 10m", nil)

			cond := remediationrequest.GetCondition(rr, remediationrequest.ConditionWorkflowExecutionComplete)
			Expect(cond.Type).To(Equal(remediationrequest.ConditionWorkflowExecutionComplete))
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(remediationrequest.ReasonWorkflowTimeout))
		})
	})

	// ========================================
	// EDGE CASES
	// ========================================

	Describe("Edge Cases", func() {
		It("should handle nil conditions slice gracefully", func() {
			rrNil := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nil",
					Namespace: "default",
				},
				// Status.Conditions is nil by default
			}

			// Should not panic
			remediationrequest.SetCondition(rrNil, remediationrequest.ConditionReady,
				metav1.ConditionFalse, remediationrequest.ReasonNotReady, "Starting", nil)

			Expect(rrNil.Status.Conditions).To(HaveLen(1))
		})

		It("should handle empty message", func() {
			remediationrequest.SetCondition(rr, remediationrequest.ConditionReady,
				metav1.ConditionTrue, remediationrequest.ReasonReady, "", nil)

			cond := remediationrequest.GetCondition(rr, remediationrequest.ConditionReady)
			Expect(cond.Type).To(Equal(remediationrequest.ConditionReady))
			Expect(cond.Message).To(Equal(""))
		})
	})
})
