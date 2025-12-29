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

package remediationapprovalrequest

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationapprovalrequest"
)

func TestRemediationApprovalRequestConditions(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RemediationApprovalRequest Conditions Suite")
}

var _ = Describe("RemediationApprovalRequest Conditions", func() {
	var rar *remediationv1.RemediationApprovalRequest

	BeforeEach(func() {
		rar = &remediationv1.RemediationApprovalRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-rar",
				Namespace: "default",
			},
			Status: remediationv1.RemediationApprovalRequestStatus{
				Conditions: []metav1.Condition{},
			},
		}
	})

	// ========================================
	// CONDITION CONSTANTS TESTS
	// ========================================

	Describe("Condition Type Constants", func() {
		It("should define all 3 condition types per DD-CRD-002-remediationapprovalrequest-conditions", func() {
			Expect(remediationapprovalrequest.ConditionApprovalPending).To(Equal("ApprovalPending"))
			Expect(remediationapprovalrequest.ConditionApprovalDecided).To(Equal("ApprovalDecided"))
			Expect(remediationapprovalrequest.ConditionApprovalExpired).To(Equal("ApprovalExpired"))
		})
	})

	Describe("Condition Reason Constants", func() {
		It("should define ApprovalPending reasons", func() {
			Expect(remediationapprovalrequest.ReasonAwaitingDecision).To(Equal("AwaitingDecision"))
			Expect(remediationapprovalrequest.ReasonDecisionMade).To(Equal("DecisionMade"))
		})

		It("should define ApprovalDecided reasons", func() {
			Expect(remediationapprovalrequest.ReasonApproved).To(Equal("Approved"))
			Expect(remediationapprovalrequest.ReasonRejected).To(Equal("Rejected"))
			Expect(remediationapprovalrequest.ReasonPendingDecision).To(Equal("PendingDecision"))
		})

		It("should define ApprovalExpired reasons", func() {
			Expect(remediationapprovalrequest.ReasonTimeout).To(Equal("Timeout"))
			Expect(remediationapprovalrequest.ReasonNotExpired).To(Equal("NotExpired"))
		})
	})

	// ========================================
	// GENERIC CONDITION FUNCTIONS TESTS
	// ========================================

	Describe("SetCondition", func() {
		It("should set a new condition using meta.SetStatusCondition", func() {
			remediationapprovalrequest.SetCondition(rar, remediationapprovalrequest.ConditionApprovalPending,
				metav1.ConditionTrue, remediationapprovalrequest.ReasonAwaitingDecision, "Waiting for approval", nil)

			Expect(rar.Status.Conditions).To(HaveLen(1))
			cond := rar.Status.Conditions[0]
			Expect(cond.Type).To(Equal(remediationapprovalrequest.ConditionApprovalPending))
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(remediationapprovalrequest.ReasonAwaitingDecision))
			Expect(cond.Message).To(Equal("Waiting for approval"))
			Expect(cond.LastTransitionTime).ToNot(BeZero())
		})

		It("should update existing condition without duplicating", func() {
			// Set initial condition
			remediationapprovalrequest.SetCondition(rar, remediationapprovalrequest.ConditionApprovalPending,
				metav1.ConditionTrue, remediationapprovalrequest.ReasonAwaitingDecision, "Initial", nil)

			// Update same condition type
			remediationapprovalrequest.SetCondition(rar, remediationapprovalrequest.ConditionApprovalPending,
				metav1.ConditionFalse, remediationapprovalrequest.ReasonDecisionMade, "Updated", nil)

			// Should still have only 1 condition (no duplicates)
			Expect(rar.Status.Conditions).To(HaveLen(1))
			cond := rar.Status.Conditions[0]
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(remediationapprovalrequest.ReasonDecisionMade))
		})
	})

	Describe("GetCondition", func() {
		It("should return nil when condition doesn't exist", func() {
			cond := remediationapprovalrequest.GetCondition(rar, remediationapprovalrequest.ConditionApprovalExpired)
			Expect(cond).To(BeNil())
		})

		It("should return existing condition", func() {
			remediationapprovalrequest.SetCondition(rar, remediationapprovalrequest.ConditionApprovalDecided,
				metav1.ConditionTrue, remediationapprovalrequest.ReasonApproved, "Approved by admin", nil)

			cond := remediationapprovalrequest.GetCondition(rar, remediationapprovalrequest.ConditionApprovalDecided)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Type).To(Equal(remediationapprovalrequest.ConditionApprovalDecided))
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	// ========================================
	// TYPE-SPECIFIC SETTER TESTS
	// ========================================

	Describe("SetApprovalPending", func() {
		It("should set True with AwaitingDecision reason when pending", func() {
			remediationapprovalrequest.SetApprovalPending(rar, true, "Expires in 30m", nil)

			cond := remediationapprovalrequest.GetCondition(rar, remediationapprovalrequest.ConditionApprovalPending)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(remediationapprovalrequest.ReasonAwaitingDecision))
			Expect(cond.Message).To(Equal("Expires in 30m"))
		})

		It("should set False with DecisionMade reason when decided", func() {
			remediationapprovalrequest.SetApprovalPending(rar, false, "Decision received", nil)

			cond := remediationapprovalrequest.GetCondition(rar, remediationapprovalrequest.ConditionApprovalPending)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(remediationapprovalrequest.ReasonDecisionMade))
		})
	})

	Describe("SetApprovalDecided", func() {
		It("should set True with Approved reason when approved", func() {
			remediationapprovalrequest.SetApprovalDecided(rar, true,
				remediationapprovalrequest.ReasonApproved, "Approved by admin@example.com", nil)

			cond := remediationapprovalrequest.GetCondition(rar, remediationapprovalrequest.ConditionApprovalDecided)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(remediationapprovalrequest.ReasonApproved))
		})

		It("should set True with Rejected reason when rejected", func() {
			remediationapprovalrequest.SetApprovalDecided(rar, true,
				remediationapprovalrequest.ReasonRejected, "Rejected: risky workflow", nil)

			cond := remediationapprovalrequest.GetCondition(rar, remediationapprovalrequest.ConditionApprovalDecided)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(remediationapprovalrequest.ReasonRejected))
		})

		It("should set False with PendingDecision reason when not yet decided", func() {
			remediationapprovalrequest.SetApprovalDecided(rar, false,
				remediationapprovalrequest.ReasonPendingDecision, "No decision yet", nil)

			cond := remediationapprovalrequest.GetCondition(rar, remediationapprovalrequest.ConditionApprovalDecided)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(remediationapprovalrequest.ReasonPendingDecision))
		})
	})

	Describe("SetApprovalExpired", func() {
		It("should set True with Timeout reason when expired", func() {
			remediationapprovalrequest.SetApprovalExpired(rar, true, "Expired after 30m without decision", nil)

			cond := remediationapprovalrequest.GetCondition(rar, remediationapprovalrequest.ConditionApprovalExpired)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(remediationapprovalrequest.ReasonTimeout))
		})

		It("should set False with NotExpired reason when not expired", func() {
			remediationapprovalrequest.SetApprovalExpired(rar, false, "15m remaining", nil)

			cond := remediationapprovalrequest.GetCondition(rar, remediationapprovalrequest.ConditionApprovalExpired)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(remediationapprovalrequest.ReasonNotExpired))
		})
	})

	// ========================================
	// EDGE CASES
	// ========================================

	Describe("Edge Cases", func() {
		It("should handle nil conditions slice gracefully", func() {
			rarNil := &remediationv1.RemediationApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nil",
					Namespace: "default",
				},
				// Status.Conditions is nil by default
			}

			// Should not panic
			remediationapprovalrequest.SetCondition(rarNil, remediationapprovalrequest.ConditionApprovalPending,
				metav1.ConditionTrue, remediationapprovalrequest.ReasonAwaitingDecision, "Starting", nil)

			Expect(rarNil.Status.Conditions).To(HaveLen(1))
		})
	})
})
