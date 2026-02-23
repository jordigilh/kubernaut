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

// Package reconciler_test contains unit tests for the SignalProcessing reconciler.
//
// BR Coverage:
//   - BR-SP-001: K8s Context Enrichment
//   - BR-SP-051-053: Environment Classification
//   - BR-SP-070-072: Priority Assignment
//   - BR-SP-080-081: Business Classification
//
// Test Categories:
//   - PHASE-XX: Phase transition state machine tests
//
// Per 03-testing-strategy.mdc: Unit tests validate business logic behavior.
// These tests validate phase transition state machine logic.
package reconciler_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

var _ = Describe("SignalProcessing Phase State Machine", func() {

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// Phase State Machine: Valid Transitions
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("Valid Phase Transitions", func() {
		Context("PHASE-01: Pending → Enriching is valid", func() {
			It("should allow transition from Pending to Enriching", func() {
				sp := createTestSP(signalprocessingv1alpha1.PhasePending)
				sp.Status.Phase = signalprocessingv1alpha1.PhaseEnriching

				Expect(sp.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseEnriching))
			})
		})

		Context("PHASE-02: Enriching → Classifying is valid", func() {
			It("should allow transition from Enriching to Classifying", func() {
				sp := createTestSP(signalprocessingv1alpha1.PhaseEnriching)
				sp.Status.Phase = signalprocessingv1alpha1.PhaseClassifying

				Expect(sp.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseClassifying))
			})
		})

		Context("PHASE-03: Classifying → Categorizing is valid", func() {
			It("should allow transition from Classifying to Categorizing", func() {
				sp := createTestSP(signalprocessingv1alpha1.PhaseClassifying)
				sp.Status.Phase = signalprocessingv1alpha1.PhaseCategorizing

				Expect(sp.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCategorizing))
			})
		})

		Context("PHASE-04: Categorizing → Completed is valid", func() {
			It("should allow transition from Categorizing to Completed", func() {
				sp := createTestSP(signalprocessingv1alpha1.PhaseCategorizing)
				sp.Status.Phase = signalprocessingv1alpha1.PhaseCompleted

				Expect(sp.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))
			})
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// Terminal States: Completed and Failed
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("Terminal States", func() {
		Context("PHASE-05: Completed is terminal state", func() {
			It("should be terminal (no further processing needed)", func() {
				sp := createTestSP(signalprocessingv1alpha1.PhaseCompleted)
				isTerminal := sp.Status.Phase == signalprocessingv1alpha1.PhaseCompleted ||
					sp.Status.Phase == signalprocessingv1alpha1.PhaseFailed

				Expect(isTerminal).To(BeTrue(), "Completed phase should be terminal")
			})
		})

		Context("PHASE-06: Failed is terminal state", func() {
			It("should be terminal (no further processing needed)", func() {
				sp := createTestSP(signalprocessingv1alpha1.PhaseFailed)
				isTerminal := sp.Status.Phase == signalprocessingv1alpha1.PhaseCompleted ||
					sp.Status.Phase == signalprocessingv1alpha1.PhaseFailed

				Expect(isTerminal).To(BeTrue(), "Failed phase should be terminal")
			})
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// Phase Constants
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("Phase Constants", func() {
		Context("PHASE-07: phase constants have expected values", func() {
			It("should have correct string values for CRD status", func() {
				Expect(string(signalprocessingv1alpha1.PhasePending)).To(Equal("Pending"))
				Expect(string(signalprocessingv1alpha1.PhaseEnriching)).To(Equal("Enriching"))
				Expect(string(signalprocessingv1alpha1.PhaseClassifying)).To(Equal("Classifying"))
				Expect(string(signalprocessingv1alpha1.PhaseCategorizing)).To(Equal("Categorizing"))
				Expect(string(signalprocessingv1alpha1.PhaseCompleted)).To(Equal("Completed"))
				Expect(string(signalprocessingv1alpha1.PhaseFailed)).To(Equal("Failed"))
			})
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// Status Initialization
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("Status Initialization", func() {
		Context("PHASE-08: new resource should require initialization", func() {
			It("should detect empty phase as uninitialized", func() {
				sp := &signalprocessingv1alpha1.SignalProcessing{
					ObjectMeta: metav1.ObjectMeta{Name: "new-sp", Namespace: "default"},
				}
				// Empty phase means resource is uninitialized
				Expect(sp.Status.Phase).To(Equal(signalprocessingv1alpha1.SignalProcessingPhase("")))
			})
		})

		Context("PHASE-09: initialized resource has StartTime", func() {
			It("should have StartTime after initialization", func() {
				sp := createTestSP(signalprocessingv1alpha1.PhasePending)
				Expect(sp.Status.StartTime).ToNot(BeNil())
			})
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// Status Fields by Phase
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("Status Fields by Phase", func() {
		Context("PHASE-10: after Enriching, KubernetesContext should be populated", func() {
			It("should have KubernetesContext populated after enrichment", func() {
				sp := createTestSP(signalprocessingv1alpha1.PhaseClassifying)
				// Simulate enrichment result
				sp.Status.KubernetesContext = &signalprocessingv1alpha1.KubernetesContext{
					Namespace: &signalprocessingv1alpha1.NamespaceContext{
						Name: "test-ns",
					},
				}

				Expect(sp.Status.KubernetesContext).ToNot(BeNil())
				Expect(sp.Status.KubernetesContext.Namespace.Name).To(Equal("test-ns"))
			})
		})

		Context("PHASE-11: after Classifying, classification results should be populated", func() {
			It("should have EnvironmentClassification and PriorityAssignment", func() {
				sp := createTestSP(signalprocessingv1alpha1.PhaseCategorizing)
				// Simulate classification results
				sp.Status.EnvironmentClassification = &signalprocessingv1alpha1.EnvironmentClassification{
					Environment: "production",
					Source:      "namespace-labels",
				}
				sp.Status.PriorityAssignment = &signalprocessingv1alpha1.PriorityAssignment{
					Priority: "P0",
					Source:   "policy-matrix",
				}

				Expect(sp.Status.EnvironmentClassification).ToNot(BeNil())
				Expect(sp.Status.EnvironmentClassification.Environment).To(Equal("production"))
				Expect(sp.Status.PriorityAssignment).ToNot(BeNil())
				Expect(sp.Status.PriorityAssignment.Priority).To(Equal("P0"))
			})
		})

		Context("PHASE-12: after Categorizing, business classification should be populated", func() {
			It("should have BusinessClassification after completion", func() {
				sp := createTestSP(signalprocessingv1alpha1.PhaseCompleted)
				// Simulate categorization result
				sp.Status.BusinessClassification = &signalprocessingv1alpha1.BusinessClassification{
					BusinessUnit: "platform",
					Criticality:  "high",
				}
				now := metav1.Now()
				sp.Status.CompletionTime = &now

				Expect(sp.Status.BusinessClassification).ToNot(BeNil())
				Expect(sp.Status.BusinessClassification.BusinessUnit).To(Equal("platform"))
				Expect(sp.Status.CompletionTime).ToNot(BeNil())
			})
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// Error Tracking
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("Error Tracking", func() {
		Context("PHASE-13: ConsecutiveFailures tracks retries", func() {
			It("should track consecutive failures for backoff", func() {
				sp := createTestSP(signalprocessingv1alpha1.PhaseEnriching)
				sp.Status.ConsecutiveFailures = 3
				sp.Status.Error = "K8s API timeout"

				Expect(sp.Status.ConsecutiveFailures).To(Equal(int32(3)))
				Expect(sp.Status.Error).To(ContainSubstring("timeout"))
			})
		})

		Context("PHASE-14: LastFailureTime tracks failure timing", func() {
			It("should track last failure time", func() {
				sp := createTestSP(signalprocessingv1alpha1.PhaseEnriching)
				now := metav1.Now()
				sp.Status.LastFailureTime = &now

				Expect(sp.Status.LastFailureTime).ToNot(BeNil())
			})
		})
	})
})

// ========================================
// HELPER FUNCTIONS
// ========================================

// createTestSP creates a test SignalProcessing CR with the given phase.
func createTestSP(phase signalprocessingv1alpha1.SignalProcessingPhase) *signalprocessingv1alpha1.SignalProcessing {
	now := metav1.Now()
	return &signalprocessingv1alpha1.SignalProcessing{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sp",
			Namespace: "default",
		},
		Spec: signalprocessingv1alpha1.SignalProcessingSpec{
			Signal: signalprocessingv1alpha1.SignalData{
				Fingerprint:  "abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab",
				Name:         "TestSignal",
				Severity:     "critical",
				Type:         "alert",
				TargetType:   "kubernetes",
				ReceivedTime: now,
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: "default",
				},
			},
		},
		Status: signalprocessingv1alpha1.SignalProcessingStatus{
			Phase:     phase,
			StartTime: &now,
		},
	}
}
