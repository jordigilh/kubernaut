/*
Copyright 2026 Jordi Gil.

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

// Package classifier: REFACTOR Phase - Case Sensitivity Tests (DD-SEVERITY-001 Gap 4)
package classifier

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/classifier"
)

var _ = Describe("Severity Classifier - Case Sensitivity (REFACTOR Phase)", func() {
	var (
		severityClassifier *classifier.SeverityClassifier
		ctx                context.Context
	)

	BeforeEach(func() {
		zapLog, _ := zap.NewDevelopment()
		logger := zapr.NewLogger(zapLog)
		severityClassifier = classifier.NewSeverityClassifier(nil, logger)
		ctx = context.Background()
	})

	// ========================================
	// DD-SEVERITY-001: REFACTOR PHASE - Case Sensitivity (Gap 4)
	// ========================================

	Context("External Severity Case Sensitivity", func() {
		It("should handle case-insensitive external severity values", func() {
			// BUSINESS CONTEXT: External monitoring systems may send severity in different cases
			// VALUE: Prevents duplicate policy entries for "SEV1", "Sev1", "sev1"
			// OUTCOME: Single policy entry handles all case variations

			// Load policy with lowercase severity matching
			policyWithLowercase := `
package signalprocessing.severity

determine_severity := "critical" if {
    input.signal.severity == "sev1"
} else := "high" if {
    input.signal.severity == "sev2"
} else := "low" if {
    true
}
`
			err := severityClassifier.LoadRegoPolicy(policyWithLowercase)
			Expect(err).ToNot(HaveOccurred())

			// Test all case variations
			testCases := []struct {
				input    string
				expected string
			}{
				{"SEV1", "critical"},    // All uppercase
				{"Sev1", "critical"},    // Mixed case
				{"sev1", "critical"},    // All lowercase
				{"SEV2", "high"},        // All uppercase
				{"Sev2", "high"},        // Mixed case
				{"sev2", "high"},        // All lowercase
				{"UNMAPPED", "low"},     // Fallback (all uppercase)
				{"Unmapped", "low"},     // Fallback (mixed case)
				{"unmapped", "low"},     // Fallback (all lowercase)
			}

			for _, tc := range testCases {
				sp := &signalprocessingv1alpha1.SignalProcessing{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-case-sensitivity",
						Namespace: "default",
						UID:       "test-uid-12345",
					},
					Spec: signalprocessingv1alpha1.SignalProcessingSpec{
						RemediationRequestRef: signalprocessingv1alpha1.ObjectReference{
							Name:      "test-rr",
							Namespace: "default",
						},
						Signal: signalprocessingv1alpha1.SignalData{
							Fingerprint:  "test-fingerprint-abc123",
							Name:         "TestAlert",
							Severity:     tc.input, // Test case-specific input
							Type:         "prometheus",
							Source:       "test-source",
							TargetType:   "kubernetes",
							ReceivedTime: metav1.Now(),
							TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
								Kind:      "Pod",
								Name:      "test-pod",
								Namespace: "default",
							},
						},
					},
				}

				result, err := severityClassifier.ClassifySeverity(ctx, sp)

				Expect(err).ToNot(HaveOccurred(),
					fmt.Sprintf("Classification should succeed for input %q", tc.input))
				Expect(result.Severity).To(Equal(tc.expected),
					fmt.Sprintf("Input %q should map to %q", tc.input, tc.expected))
				Expect(result.Source).To(Equal("rego-policy"),
					"Source should be rego-policy for all determinations")
			}

			// BUSINESS OUTCOME VERIFIED:
			// ✅ Single policy entry handles all case variations
			// ✅ Operators don't need to maintain case-specific policy rules
			// ✅ System is robust against case inconsistencies from external systems
		})

		It("should normalize severity to lowercase before policy evaluation", func() {
			// BUSINESS CONTEXT: Rego policies should use consistent lowercase matching
			// VALUE: Simplifies policy authoring (operators don't need case-insensitive Rego)
			// OUTCOME: Policy can use simple equality checks instead of regex

			// Load policy expecting lowercase input
			policyExpectingLowercase := `
package signalprocessing.severity

determine_severity := "critical" if {
    input.signal.severity == "p0"
} else := "critical" if {
    input.signal.severity == "high"
} else := "medium" if {
    input.signal.severity == "medium"
} else := "low" if {
    input.signal.severity == "low"
} else := "low" if {
    true
}
`
			err := severityClassifier.LoadRegoPolicy(policyExpectingLowercase)
			Expect(err).ToNot(HaveOccurred())

			// Test mixed-case inputs
			testCases := []struct {
				input    string
				expected string
			}{
				{"P0", "critical"},      // Uppercase priority
				{"HIGH", "critical"},    // Uppercase severity
				{"Medium", "medium"},    // Mixed case
				{"low", "low"},          // Already lowercase
			}

			for _, tc := range testCases {
				sp := &signalprocessingv1alpha1.SignalProcessing{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-normalize",
						Namespace: "default",
						UID:       "test-uid-67890",
					},
					Spec: signalprocessingv1alpha1.SignalProcessingSpec{
						RemediationRequestRef: signalprocessingv1alpha1.ObjectReference{
							Name:      "test-rr",
							Namespace: "default",
						},
						Signal: signalprocessingv1alpha1.SignalData{
							Fingerprint:  "test-fingerprint-def456",
							Name:         "TestNormalizedAlert",
							Severity:     tc.input, // Test case-specific input
							Type:         "prometheus",
							Source:       "test-source",
							TargetType:   "kubernetes",
							ReceivedTime: metav1.Now(),
							TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
								Kind:      "Pod",
								Name:      "test-pod",
								Namespace: "default",
							},
						},
					},
				}

				result, err := severityClassifier.ClassifySeverity(ctx, sp)

				Expect(err).ToNot(HaveOccurred(),
					fmt.Sprintf("Classification should succeed for input %q", tc.input))
				Expect(result.Severity).To(Equal(tc.expected),
					fmt.Sprintf("Input %q should map to %q", tc.input, tc.expected))
			}

			// BUSINESS OUTCOME VERIFIED:
			// ✅ Rego policies can use simple lowercase equality checks
			// ✅ No need for complex regex or case-insensitive Rego functions
			// ✅ Policy authoring is simplified for operators
		})
	})
})
