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

package aianalysis

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
)

// Issue #79 Phase 6: AIAnalysis conditions unit tests
// BR-AA-*: Kubernetes Conditions for AIAnalysis CRD
var _ = Describe("AIAnalysis Conditions", func() {
	var analysis *aianalysisv1.AIAnalysis

	BeforeEach(func() {
		analysis = &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-aa",
				Namespace:  "default",
				Generation: 1,
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Conditions: []metav1.Condition{},
			},
		}
	})

	// ========================================
	// SetCondition
	// ========================================

	Context("SetCondition", func() {
		It("should set condition with ObservedGeneration", func() {
			aianalysis.SetCondition(analysis, aianalysis.ConditionInvestigationComplete,
				metav1.ConditionTrue, aianalysis.ReasonInvestigationSucceeded, "Investigation done")

			Expect(analysis.Status.Conditions).To(HaveLen(1))
			c := analysis.Status.Conditions[0]
			Expect(c.Type).To(Equal(aianalysis.ConditionInvestigationComplete))
			Expect(c.Status).To(Equal(metav1.ConditionTrue))
			Expect(c.Reason).To(Equal(aianalysis.ReasonInvestigationSucceeded))
			Expect(c.Message).To(Equal("Investigation done"))
			Expect(c.ObservedGeneration).To(Equal(int64(1)))
			Expect(c.LastTransitionTime).NotTo(BeZero())
		})

		It("should update existing condition in place", func() {
			aianalysis.SetCondition(analysis, "TestCond", metav1.ConditionTrue, "R1", "first")
			aianalysis.SetCondition(analysis, "TestCond", metav1.ConditionFalse, "R2", "second")

			Expect(analysis.Status.Conditions).To(HaveLen(1))
			c := analysis.Status.Conditions[0]
			Expect(c.Status).To(Equal(metav1.ConditionFalse))
			Expect(c.Reason).To(Equal("R2"))
			Expect(c.Message).To(Equal("second"))
		})
	})

	// ========================================
	// SetReady
	// ========================================

	Context("SetReady", func() {
		It("should set Ready=true with correct status", func() {
			aianalysis.SetReady(analysis, true, aianalysis.ReasonReady, "Analysis ready")

			c := aianalysis.GetCondition(analysis, aianalysis.ConditionReady)
			Expect(c).NotTo(BeNil())
			Expect(c.Status).To(Equal(metav1.ConditionTrue))
			Expect(c.Reason).To(Equal(aianalysis.ReasonReady))
			Expect(c.Message).To(Equal("Analysis ready"))
			Expect(c.ObservedGeneration).To(Equal(int64(1)))
		})

		It("should set Ready=false with correct status", func() {
			aianalysis.SetReady(analysis, false, aianalysis.ReasonNotReady, "Waiting for session")

			c := aianalysis.GetCondition(analysis, aianalysis.ConditionReady)
			Expect(c).NotTo(BeNil())
			Expect(c.Status).To(Equal(metav1.ConditionFalse))
			Expect(c.Reason).To(Equal(aianalysis.ReasonNotReady))
			Expect(c.Message).To(Equal("Waiting for session"))
		})
	})

	// ========================================
	// SetInvestigationComplete
	// ========================================

	Context("SetInvestigationComplete", func() {
		It("should set succeeded with default reason", func() {
			aianalysis.SetInvestigationComplete(analysis, true, "Investigation completed")

			c := aianalysis.GetCondition(analysis, aianalysis.ConditionInvestigationComplete)
			Expect(c).NotTo(BeNil())
			Expect(c.Status).To(Equal(metav1.ConditionTrue))
			Expect(c.Reason).To(Equal(aianalysis.ReasonInvestigationSucceeded))
			Expect(c.Message).To(Equal("Investigation completed"))
		})

		It("should set failed with default reason", func() {
			aianalysis.SetInvestigationComplete(analysis, false, "HAPI session lost")

			c := aianalysis.GetCondition(analysis, aianalysis.ConditionInvestigationComplete)
			Expect(c).NotTo(BeNil())
			Expect(c.Status).To(Equal(metav1.ConditionFalse))
			Expect(c.Reason).To(Equal(aianalysis.ReasonInvestigationFailed))
			Expect(c.Message).To(Equal("HAPI session lost"))
		})
	})

	// ========================================
	// SetAnalysisComplete
	// ========================================

	Context("SetAnalysisComplete", func() {
		It("should set succeeded with default reason", func() {
			aianalysis.SetAnalysisComplete(analysis, true, "Analysis completed")

			c := aianalysis.GetCondition(analysis, aianalysis.ConditionAnalysisComplete)
			Expect(c).NotTo(BeNil())
			Expect(c.Status).To(Equal(metav1.ConditionTrue))
			Expect(c.Reason).To(Equal(aianalysis.ReasonAnalysisSucceeded))
			Expect(c.Message).To(Equal("Analysis completed"))
		})

		It("should set failed with default reason", func() {
			aianalysis.SetAnalysisComplete(analysis, false, "LLM returned error")

			c := aianalysis.GetCondition(analysis, aianalysis.ConditionAnalysisComplete)
			Expect(c).NotTo(BeNil())
			Expect(c.Status).To(Equal(metav1.ConditionFalse))
			Expect(c.Reason).To(Equal(aianalysis.ReasonAnalysisFailed))
			Expect(c.Message).To(Equal("LLM returned error"))
		})
	})

	// ========================================
	// GetCondition
	// ========================================

	Context("GetCondition", func() {
		It("should return condition when it exists", func() {
			aianalysis.SetCondition(analysis, aianalysis.ConditionReady,
				metav1.ConditionTrue, aianalysis.ReasonReady, "Ready")

			c := aianalysis.GetCondition(analysis, aianalysis.ConditionReady)
			Expect(c).NotTo(BeNil())
			Expect(c.Type).To(Equal(aianalysis.ConditionReady))
			Expect(c.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should return nil when condition does not exist", func() {
			c := aianalysis.GetCondition(analysis, aianalysis.ConditionReady)
			Expect(c).To(BeNil())
		})
	})
})
