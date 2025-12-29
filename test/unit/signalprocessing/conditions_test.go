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

package signalprocessing

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	spv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	spconditions "github.com/jordigilh/kubernaut/pkg/signalprocessing"
)

// ========================================
// BR-SP-110: KUBERNETES CONDITIONS UNIT TESTS
// ========================================
// Design Decisions:
//   - DD-SP-002: Kubernetes Conditions Specification
//   - DD-CRD-002: Cross-Service Conditions Standard
// ========================================

var _ = Describe("SignalProcessing Conditions (BR-SP-110)", func() {
	var sp *spv1.SignalProcessing

	BeforeEach(func() {
		sp = &spv1.SignalProcessing{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-sp",
				Namespace: "default",
			},
			Status: spv1.SignalProcessingStatus{
				Conditions: []metav1.Condition{},
			},
		}
	})

	// ========================================
	// Generic Condition Helpers
	// ========================================

	Context("SetCondition", func() {
		It("should set a new condition on empty status", func() {
			spconditions.SetCondition(sp, spconditions.ConditionEnrichmentComplete,
				metav1.ConditionTrue, spconditions.ReasonEnrichmentSucceeded, "Test message")

			Expect(sp.Status.Conditions).To(HaveLen(1))
			Expect(sp.Status.Conditions[0].Type).To(Equal(spconditions.ConditionEnrichmentComplete))
			Expect(sp.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(sp.Status.Conditions[0].Reason).To(Equal(spconditions.ReasonEnrichmentSucceeded))
			Expect(sp.Status.Conditions[0].Message).To(Equal("Test message"))
			Expect(sp.Status.Conditions[0].LastTransitionTime).ToNot(BeZero())
		})

		It("should update existing condition", func() {
			// Set initial condition
			spconditions.SetCondition(sp, spconditions.ConditionEnrichmentComplete,
				metav1.ConditionTrue, spconditions.ReasonEnrichmentSucceeded, "First message")

			// Update same condition
			spconditions.SetCondition(sp, spconditions.ConditionEnrichmentComplete,
				metav1.ConditionFalse, spconditions.ReasonEnrichmentFailed, "Second message")

			Expect(sp.Status.Conditions).To(HaveLen(1))
			Expect(sp.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
			Expect(sp.Status.Conditions[0].Reason).To(Equal(spconditions.ReasonEnrichmentFailed))
			Expect(sp.Status.Conditions[0].Message).To(Equal("Second message"))
		})

		It("should add multiple different conditions", func() {
			spconditions.SetCondition(sp, spconditions.ConditionEnrichmentComplete,
				metav1.ConditionTrue, spconditions.ReasonEnrichmentSucceeded, "Enriched")
			spconditions.SetCondition(sp, spconditions.ConditionClassificationComplete,
				metav1.ConditionTrue, spconditions.ReasonClassificationSucceeded, "Classified")

			Expect(sp.Status.Conditions).To(HaveLen(2))
		})
	})

	Context("GetCondition", func() {
		It("should return nil for non-existent condition", func() {
			cond := spconditions.GetCondition(sp, spconditions.ConditionEnrichmentComplete)
			Expect(cond).To(BeNil())
		})

		It("should return existing condition", func() {
			spconditions.SetCondition(sp, spconditions.ConditionEnrichmentComplete,
				metav1.ConditionTrue, spconditions.ReasonEnrichmentSucceeded, "Test")

			cond := spconditions.GetCondition(sp, spconditions.ConditionEnrichmentComplete)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Type).To(Equal(spconditions.ConditionEnrichmentComplete))
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("IsConditionTrue", func() {
		It("should return false for non-existent condition", func() {
			Expect(spconditions.IsConditionTrue(sp, spconditions.ConditionEnrichmentComplete)).To(BeFalse())
		})

		It("should return true when condition is True", func() {
			spconditions.SetCondition(sp, spconditions.ConditionEnrichmentComplete,
				metav1.ConditionTrue, spconditions.ReasonEnrichmentSucceeded, "Test")

			Expect(spconditions.IsConditionTrue(sp, spconditions.ConditionEnrichmentComplete)).To(BeTrue())
		})

		It("should return false when condition is False", func() {
			spconditions.SetCondition(sp, spconditions.ConditionEnrichmentComplete,
				metav1.ConditionFalse, spconditions.ReasonEnrichmentFailed, "Test")

			Expect(spconditions.IsConditionTrue(sp, spconditions.ConditionEnrichmentComplete)).To(BeFalse())
		})

		It("should return false when condition is Unknown", func() {
			spconditions.SetCondition(sp, spconditions.ConditionEnrichmentComplete,
				metav1.ConditionUnknown, "Unknown", "Test")

			Expect(spconditions.IsConditionTrue(sp, spconditions.ConditionEnrichmentComplete)).To(BeFalse())
		})
	})

	// ========================================
	// SetEnrichmentComplete (BR-SP-001)
	// ========================================

	Context("SetEnrichmentComplete", func() {
		It("should set True with default reason on success", func() {
			spconditions.SetEnrichmentComplete(sp, true, "", "K8s context enriched: Pod nginx")

			cond := spconditions.GetCondition(sp, spconditions.ConditionEnrichmentComplete)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(spconditions.ReasonEnrichmentSucceeded))
			Expect(cond.Message).To(Equal("K8s context enriched: Pod nginx"))
		})

		It("should set False with default reason on failure", func() {
			spconditions.SetEnrichmentComplete(sp, false, "", "Enrichment failed: timeout")

			cond := spconditions.GetCondition(sp, spconditions.ConditionEnrichmentComplete)
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(spconditions.ReasonEnrichmentFailed))
		})

		It("should use K8sAPITimeout reason when specified", func() {
			spconditions.SetEnrichmentComplete(sp, false, spconditions.ReasonK8sAPITimeout,
				"K8s API call timed out after 5s")

			cond := spconditions.GetCondition(sp, spconditions.ConditionEnrichmentComplete)
			Expect(cond.Reason).To(Equal(spconditions.ReasonK8sAPITimeout))
		})

		It("should use ResourceNotFound reason when specified", func() {
			spconditions.SetEnrichmentComplete(sp, false, spconditions.ReasonResourceNotFound,
				"Pod nginx not found in namespace default")

			cond := spconditions.GetCondition(sp, spconditions.ConditionEnrichmentComplete)
			Expect(cond.Reason).To(Equal(spconditions.ReasonResourceNotFound))
		})

		It("should use RBACDenied reason when specified", func() {
			spconditions.SetEnrichmentComplete(sp, false, spconditions.ReasonRBACDenied,
				"Controller lacks permission to get pods")

			cond := spconditions.GetCondition(sp, spconditions.ConditionEnrichmentComplete)
			Expect(cond.Reason).To(Equal(spconditions.ReasonRBACDenied))
		})

		It("should set DegradedMode reason with True status", func() {
			// DegradedMode is a True condition - enrichment completed but with partial context
			spconditions.SetEnrichmentComplete(sp, true, spconditions.ReasonDegradedMode,
				"Enrichment completed in degraded mode (K8s API unavailable)")

			cond := spconditions.GetCondition(sp, spconditions.ConditionEnrichmentComplete)
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(spconditions.ReasonDegradedMode))
		})
	})

	// ========================================
	// SetClassificationComplete (BR-SP-051, BR-SP-070)
	// ========================================

	Context("SetClassificationComplete", func() {
		It("should set True with default reason on success", func() {
			spconditions.SetClassificationComplete(sp, true, "",
				"Classified: environment=production (source=rego-policy), priority=P0 (source=rego-policy)")

			cond := spconditions.GetCondition(sp, spconditions.ConditionClassificationComplete)
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(spconditions.ReasonClassificationSucceeded))
		})

		It("should set False with default reason on failure", func() {
			spconditions.SetClassificationComplete(sp, false, "", "Classification failed")

			cond := spconditions.GetCondition(sp, spconditions.ConditionClassificationComplete)
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(spconditions.ReasonClassificationFailed))
		})

		It("should use RegoEvaluationError reason when specified", func() {
			spconditions.SetClassificationComplete(sp, false, spconditions.ReasonRegoEvaluationError,
				"Rego policy evaluation failed: undefined rule")

			cond := spconditions.GetCondition(sp, spconditions.ConditionClassificationComplete)
			Expect(cond.Reason).To(Equal(spconditions.ReasonRegoEvaluationError))
		})

		It("should use SeverityFallback reason for BR-SP-071 fallback", func() {
			// Per BR-SP-071: Severity-based fallback when Rego fails
			spconditions.SetClassificationComplete(sp, true, spconditions.ReasonSeverityFallback,
				"Classified using severity fallback: priority=P1 (critical)")

			cond := spconditions.GetCondition(sp, spconditions.ConditionClassificationComplete)
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(spconditions.ReasonSeverityFallback))
		})

		It("should use PolicyNotFound reason when policy file missing", func() {
			spconditions.SetClassificationComplete(sp, true, spconditions.ReasonPolicyNotFound,
				"Rego policy not found, using ConfigMap fallback")

			cond := spconditions.GetCondition(sp, spconditions.ConditionClassificationComplete)
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(spconditions.ReasonPolicyNotFound))
		})
	})

	// ========================================
	// SetCategorizationComplete (BR-SP-002, BR-SP-080)
	// ========================================

	Context("SetCategorizationComplete", func() {
		It("should set True with default reason on success", func() {
			spconditions.SetCategorizationComplete(sp, true, "",
				"Categorized: businessUnit=payments, criticality=high, sla=platinum")

			cond := spconditions.GetCondition(sp, spconditions.ConditionCategorizationComplete)
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(spconditions.ReasonCategorizationSucceeded))
		})

		It("should set False with default reason on failure", func() {
			spconditions.SetCategorizationComplete(sp, false, "", "Categorization failed")

			cond := spconditions.GetCondition(sp, spconditions.ConditionCategorizationComplete)
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(spconditions.ReasonCategorizationFailed))
		})

		It("should use InvalidBusinessUnit reason when specified", func() {
			spconditions.SetCategorizationComplete(sp, false, spconditions.ReasonInvalidBusinessUnit,
				"Business unit 'unknown-unit' not recognized")

			cond := spconditions.GetCondition(sp, spconditions.ConditionCategorizationComplete)
			Expect(cond.Reason).To(Equal(spconditions.ReasonInvalidBusinessUnit))
		})

		It("should use InvalidSLATier reason when specified", func() {
			spconditions.SetCategorizationComplete(sp, false, spconditions.ReasonInvalidSLATier,
				"SLA tier 'diamond' not valid")

			cond := spconditions.GetCondition(sp, spconditions.ConditionCategorizationComplete)
			Expect(cond.Reason).To(Equal(spconditions.ReasonInvalidSLATier))
		})
	})

	// ========================================
	// SetProcessingComplete (Terminal State)
	// ========================================

	Context("SetProcessingComplete", func() {
		It("should set True with default reason on success", func() {
			spconditions.SetProcessingComplete(sp, true, "",
				"Signal processed successfully in 1.23s: P0 production alert ready for remediation")

			cond := spconditions.GetCondition(sp, spconditions.ConditionProcessingComplete)
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(spconditions.ReasonProcessingSucceeded))
		})

		It("should set False with default reason on failure", func() {
			spconditions.SetProcessingComplete(sp, false, "", "Processing failed")

			cond := spconditions.GetCondition(sp, spconditions.ConditionProcessingComplete)
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(spconditions.ReasonProcessingFailed))
		})

		It("should use AuditWriteFailed reason when specified", func() {
			// Per user clarification: Audit failures are logged but don't block processing
			spconditions.SetProcessingComplete(sp, true, spconditions.ReasonAuditWriteFailed,
				"Processing succeeded but audit write failed (logged)")

			cond := spconditions.GetCondition(sp, spconditions.ConditionProcessingComplete)
			// Processing still succeeds even if audit fails
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(spconditions.ReasonAuditWriteFailed))
		})

		It("should use ValidationFailed reason when specified", func() {
			spconditions.SetProcessingComplete(sp, false, spconditions.ReasonValidationFailed,
				"CRD validation failed: missing required field")

			cond := spconditions.GetCondition(sp, spconditions.ConditionProcessingComplete)
			Expect(cond.Reason).To(Equal(spconditions.ReasonValidationFailed))
		})
	})

	// ========================================
	// Full Processing Lifecycle Test
	// ========================================

	Context("Full Processing Lifecycle", func() {
		It("should accumulate all conditions on successful processing", func() {
			// Phase 1: Enrichment complete
			spconditions.SetEnrichmentComplete(sp, true, "",
				"K8s context enriched: Pod nginx in namespace production")

			// Phase 2: Classification complete
			spconditions.SetClassificationComplete(sp, true, "",
				"Classified: environment=production, priority=P0")

			// Phase 3: Categorization complete
			spconditions.SetCategorizationComplete(sp, true, "",
				"Categorized: businessUnit=payments, sla=platinum")

			// Phase 4: Processing complete (terminal)
			spconditions.SetProcessingComplete(sp, true, "",
				"Signal processed successfully")

			// Verify all 4 conditions exist and are True
			Expect(sp.Status.Conditions).To(HaveLen(4))
			Expect(spconditions.IsConditionTrue(sp, spconditions.ConditionEnrichmentComplete)).To(BeTrue())
			Expect(spconditions.IsConditionTrue(sp, spconditions.ConditionClassificationComplete)).To(BeTrue())
			Expect(spconditions.IsConditionTrue(sp, spconditions.ConditionCategorizationComplete)).To(BeTrue())
			Expect(spconditions.IsConditionTrue(sp, spconditions.ConditionProcessingComplete)).To(BeTrue())
		})

		It("should show partial progress on failure", func() {
			// Enrichment succeeds
			spconditions.SetEnrichmentComplete(sp, true, "", "Enriched")

			// Classification fails
			spconditions.SetClassificationComplete(sp, false, spconditions.ReasonRegoEvaluationError,
				"Rego evaluation failed")

			// Processing fails (terminal)
			spconditions.SetProcessingComplete(sp, false, spconditions.ReasonProcessingFailed,
				"Processing failed at classification phase")

			Expect(sp.Status.Conditions).To(HaveLen(3))
			Expect(spconditions.IsConditionTrue(sp, spconditions.ConditionEnrichmentComplete)).To(BeTrue())
			Expect(spconditions.IsConditionTrue(sp, spconditions.ConditionClassificationComplete)).To(BeFalse())
			Expect(spconditions.IsConditionTrue(sp, spconditions.ConditionProcessingComplete)).To(BeFalse())
		})

		It("should handle degraded mode lifecycle", func() {
			// Enrichment in degraded mode (True with DegradedMode reason)
			spconditions.SetEnrichmentComplete(sp, true, spconditions.ReasonDegradedMode,
				"K8s API unavailable, using signal labels")

			// Classification with severity fallback
			spconditions.SetClassificationComplete(sp, true, spconditions.ReasonSeverityFallback,
				"Using severity fallback: priority=P1")

			// Categorization succeeds
			spconditions.SetCategorizationComplete(sp, true, "", "Categorized")

			// Processing completes
			spconditions.SetProcessingComplete(sp, true, "",
				"Processing completed in degraded mode")

			// All conditions True (degraded mode is still success)
			Expect(spconditions.IsConditionTrue(sp, spconditions.ConditionEnrichmentComplete)).To(BeTrue())
			Expect(spconditions.IsConditionTrue(sp, spconditions.ConditionClassificationComplete)).To(BeTrue())
			Expect(spconditions.IsConditionTrue(sp, spconditions.ConditionCategorizationComplete)).To(BeTrue())
			Expect(spconditions.IsConditionTrue(sp, spconditions.ConditionProcessingComplete)).To(BeTrue())

			// But reasons indicate degraded mode
			cond := spconditions.GetCondition(sp, spconditions.ConditionEnrichmentComplete)
			Expect(cond.Reason).To(Equal(spconditions.ReasonDegradedMode))
		})
	})

	// ========================================
	// Condition Constants Validation
	// ========================================

	Context("Condition Constants", func() {
		It("should have correct condition type values", func() {
			Expect(spconditions.ConditionEnrichmentComplete).To(Equal("EnrichmentComplete"))
			Expect(spconditions.ConditionClassificationComplete).To(Equal("ClassificationComplete"))
			Expect(spconditions.ConditionCategorizationComplete).To(Equal("CategorizationComplete"))
			Expect(spconditions.ConditionProcessingComplete).To(Equal("ProcessingComplete"))
		})

		It("should have correct reason values for enrichment", func() {
			Expect(spconditions.ReasonEnrichmentSucceeded).To(Equal("EnrichmentSucceeded"))
			Expect(spconditions.ReasonEnrichmentFailed).To(Equal("EnrichmentFailed"))
			Expect(spconditions.ReasonK8sAPITimeout).To(Equal("K8sAPITimeout"))
			Expect(spconditions.ReasonResourceNotFound).To(Equal("ResourceNotFound"))
			Expect(spconditions.ReasonRBACDenied).To(Equal("RBACDenied"))
			Expect(spconditions.ReasonDegradedMode).To(Equal("DegradedMode"))
		})

		It("should have correct reason values for classification", func() {
			Expect(spconditions.ReasonClassificationSucceeded).To(Equal("ClassificationSucceeded"))
			Expect(spconditions.ReasonClassificationFailed).To(Equal("ClassificationFailed"))
			Expect(spconditions.ReasonRegoEvaluationError).To(Equal("RegoEvaluationError"))
			Expect(spconditions.ReasonPolicyNotFound).To(Equal("PolicyNotFound"))
			Expect(spconditions.ReasonSeverityFallback).To(Equal("SeverityFallback"))
		})

		It("should have correct reason values for categorization", func() {
			Expect(spconditions.ReasonCategorizationSucceeded).To(Equal("CategorizationSucceeded"))
			Expect(spconditions.ReasonCategorizationFailed).To(Equal("CategorizationFailed"))
			Expect(spconditions.ReasonInvalidBusinessUnit).To(Equal("InvalidBusinessUnit"))
			Expect(spconditions.ReasonInvalidSLATier).To(Equal("InvalidSLATier"))
		})

		It("should have correct reason values for processing", func() {
			Expect(spconditions.ReasonProcessingSucceeded).To(Equal("ProcessingSucceeded"))
			Expect(spconditions.ReasonProcessingFailed).To(Equal("ProcessingFailed"))
			Expect(spconditions.ReasonAuditWriteFailed).To(Equal("AuditWriteFailed"))
			Expect(spconditions.ReasonValidationFailed).To(Equal("ValidationFailed"))
		})
	})
})
