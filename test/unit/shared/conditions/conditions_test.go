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

package conditions_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jordigilh/kubernaut/pkg/shared/conditions"
)

func TestConditions(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Shared Conditions Suite")
}

// statusPtr returns a pointer to the given ConditionStatus.
// Used by DescribeTable entries where nil means "condition not set".
func statusPtr(s metav1.ConditionStatus) *metav1.ConditionStatus {
	return &s
}

var _ = Describe("Shared Conditions Utilities", func() {
	var conditionsList []metav1.Condition

	BeforeEach(func() {
		conditionsList = []metav1.Condition{}
	})

	Describe("Set", func() {
		It("should set a new condition", func() {
			conditions.Set(&conditionsList, "ValidationComplete", metav1.ConditionTrue, "ValidationSucceeded", "Validation passed")

			Expect(conditionsList).To(HaveLen(1))
			Expect(conditionsList[0].Type).To(Equal("ValidationComplete"))
			Expect(conditionsList[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(conditionsList[0].Reason).To(Equal("ValidationSucceeded"))
			Expect(conditionsList[0].Message).To(Equal("Validation passed"))
			Expect(conditionsList[0].LastTransitionTime).ToNot(BeZero())
		})

		It("should update an existing condition", func() {
			// Set initial condition
			conditions.Set(&conditionsList, "ValidationComplete", metav1.ConditionTrue, "ValidationSucceeded", "Initial message")
			Expect(conditionsList).To(HaveLen(1))
			firstTransitionTime := conditionsList[0].LastTransitionTime

			// Update the same condition
			conditions.Set(&conditionsList, "ValidationComplete", metav1.ConditionFalse, "ValidationFailed", "Updated message")

			// Should still have only one condition, but updated
			Expect(conditionsList).To(HaveLen(1))
			Expect(conditionsList[0].Type).To(Equal("ValidationComplete"))
			Expect(conditionsList[0].Status).To(Equal(metav1.ConditionFalse))
			Expect(conditionsList[0].Reason).To(Equal("ValidationFailed"))
			Expect(conditionsList[0].Message).To(Equal("Updated message"))
			Expect(conditionsList[0].LastTransitionTime).ToNot(Equal(firstTransitionTime))
		})

		It("should maintain multiple different conditions", func() {
			conditions.Set(&conditionsList, "ValidationComplete", metav1.ConditionTrue, "ValidationSucceeded", "Validation passed")
			conditions.Set(&conditionsList, "EnrichmentComplete", metav1.ConditionTrue, "EnrichmentSucceeded", "Enrichment passed")
			conditions.Set(&conditionsList, "ProcessingComplete", metav1.ConditionFalse, "ProcessingFailed", "Processing failed")

			Expect(conditionsList).To(HaveLen(3))
		})

		It("should handle empty message", func() {
			conditions.Set(&conditionsList, "ValidationComplete", metav1.ConditionTrue, "ValidationSucceeded", "")

			Expect(conditionsList).To(HaveLen(1))
			Expect(conditionsList[0].Message).To(Equal(""))
		})
	})

	Describe("Get", func() {
		It("should return existing condition", func() {
			conditions.Set(&conditionsList, "ValidationComplete", metav1.ConditionTrue, "ValidationSucceeded", "Validation passed")

			condition := conditions.Get(conditionsList, "ValidationComplete")
			Expect(condition).ToNot(BeNil())
			Expect(condition.Type).To(Equal("ValidationComplete"))
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should return nil for non-existent condition", func() {
			condition := conditions.Get(conditionsList, "EnrichmentComplete")
			Expect(condition).To(BeNil())
		})

		It("should handle empty conditions slice", func() {
			condition := conditions.Get(conditionsList, "ValidationComplete")
			Expect(condition).To(BeNil())
		})
	})

	// Status check functions: IsTrue, IsFalse, IsUnknown
	// Each follows the same contract: set a condition (or not) → query status → verify result.
	// DescribeTable consolidates 12 identical-structure tests into one table.
	DescribeTable("status check functions (IsTrue, IsFalse, IsUnknown)",
		func(checkFn func([]metav1.Condition, string) bool, status *metav1.ConditionStatus, expected bool) {
			if status != nil {
				conditions.Set(&conditionsList, "TestCondition", *status, "TestReason", "Test message")
			}
			Expect(checkFn(conditionsList, "TestCondition")).To(Equal(expected))
		},
		// --- IsTrue ---
		Entry("IsTrue: condition is True → true",
			conditions.IsTrue, statusPtr(metav1.ConditionTrue), true),
		Entry("IsTrue: condition is False → false",
			conditions.IsTrue, statusPtr(metav1.ConditionFalse), false),
		Entry("IsTrue: condition is Unknown → false",
			conditions.IsTrue, statusPtr(metav1.ConditionUnknown), false),
		Entry("IsTrue: condition not set → false",
			conditions.IsTrue, (*metav1.ConditionStatus)(nil), false),
		// --- IsFalse ---
		Entry("IsFalse: condition is False → true",
			conditions.IsFalse, statusPtr(metav1.ConditionFalse), true),
		Entry("IsFalse: condition is True → false",
			conditions.IsFalse, statusPtr(metav1.ConditionTrue), false),
		Entry("IsFalse: condition is Unknown → false",
			conditions.IsFalse, statusPtr(metav1.ConditionUnknown), false),
		Entry("IsFalse: condition not set → false",
			conditions.IsFalse, (*metav1.ConditionStatus)(nil), false),
		// --- IsUnknown ---
		Entry("IsUnknown: condition is Unknown → true",
			conditions.IsUnknown, statusPtr(metav1.ConditionUnknown), true),
		Entry("IsUnknown: condition is True → false",
			conditions.IsUnknown, statusPtr(metav1.ConditionTrue), false),
		Entry("IsUnknown: condition is False → false",
			conditions.IsUnknown, statusPtr(metav1.ConditionFalse), false),
		Entry("IsUnknown: condition not set → false",
			conditions.IsUnknown, (*metav1.ConditionStatus)(nil), false),
	)

	Describe("Integration Scenarios", func() {
		It("should handle typical lifecycle transitions", func() {
			// Phase 1: Validation starts (Unknown)
			conditions.Set(&conditionsList, "ValidationComplete", metav1.ConditionUnknown, "ValidationInProgress", "Validating input")
			Expect(conditions.IsUnknown(conditionsList, "ValidationComplete")).To(BeTrue())

			// Phase 2: Validation succeeds (True)
			conditions.Set(&conditionsList, "ValidationComplete", metav1.ConditionTrue, "ValidationSucceeded", "Validation passed")
			Expect(conditions.IsTrue(conditionsList, "ValidationComplete")).To(BeTrue())
			Expect(conditions.IsUnknown(conditionsList, "ValidationComplete")).To(BeFalse())

			// Phase 3: Enrichment starts (Unknown)
			conditions.Set(&conditionsList, "EnrichmentComplete", metav1.ConditionUnknown, "EnrichmentInProgress", "Enriching data")
			Expect(conditions.IsUnknown(conditionsList, "EnrichmentComplete")).To(BeTrue())

			// Phase 4: Enrichment fails (False)
			conditions.Set(&conditionsList, "EnrichmentComplete", metav1.ConditionFalse, "K8sAPITimeout", "Kubernetes API timed out")
			Expect(conditions.IsFalse(conditionsList, "EnrichmentComplete")).To(BeTrue())

			// Verify both conditions coexist
			Expect(conditionsList).To(HaveLen(2))
			Expect(conditions.IsTrue(conditionsList, "ValidationComplete")).To(BeTrue())
			Expect(conditions.IsFalse(conditionsList, "EnrichmentComplete")).To(BeTrue())
		})

		It("should work with multiple services pattern", func() {
			// Simulate WorkflowExecution conditions
			conditions.Set(&conditionsList, "TektonPipelineCreated", metav1.ConditionTrue, "PipelineCreated", "PipelineRun created")
			conditions.Set(&conditionsList, "TektonPipelineRunning", metav1.ConditionTrue, "PipelineStarted", "Pipeline executing")
			conditions.Set(&conditionsList, "TektonPipelineComplete", metav1.ConditionTrue, "PipelineSucceeded", "All tasks completed")
			conditions.Set(&conditionsList, "AuditRecorded", metav1.ConditionTrue, "AuditSucceeded", "Audit event recorded")

			Expect(conditionsList).To(HaveLen(4))
			Expect(conditions.IsTrue(conditionsList, "TektonPipelineCreated")).To(BeTrue())
			Expect(conditions.IsTrue(conditionsList, "TektonPipelineRunning")).To(BeTrue())
			Expect(conditions.IsTrue(conditionsList, "TektonPipelineComplete")).To(BeTrue())
			Expect(conditions.IsTrue(conditionsList, "AuditRecorded")).To(BeTrue())
		})
	})
})
