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

// Package classifier: Case Sensitivity Tests - Rego Policy Authoring Patterns
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

var _ = Describe("Severity Classifier - Case Sensitivity", func() {
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

	Context("Rego Policy Receives Original Casing", func() {
		It("should pass severity values to the Rego policy without modification", func() {
			// BUSINESS CONTEXT: The Rego policy is the authoritative mapping layer.
			// The classifier must not mutate severity values before the policy evaluates them.
			// VALUE: Policy authors have full control over casing semantics.
			// OUTCOME: Exact-match policies work correctly with original casing.

			policyWithOriginalCasing := `
package signalprocessing.severity

determine_severity := "critical" if {
    input.signal.severity == "SEV1"
} else := "high" if {
    input.signal.severity == "Warning"
} else := "low" if {
    true
}
`
			err := severityClassifier.LoadRegoPolicy(policyWithOriginalCasing)
			Expect(err).ToNot(HaveOccurred())

			testCases := []struct {
				input    string
				expected string
			}{
				{"SEV1", "critical"},    // Exact match (uppercase)
				{"Warning", "high"},     // Exact match (K8s event casing)
				{"sev1", "low"},         // Does NOT match "SEV1" → fallback
				{"warning", "low"},      // Does NOT match "Warning" → fallback
				{"UNMAPPED", "low"},     // Unknown → fallback
			}

			for _, tc := range testCases {
				sp := &signalprocessingv1alpha1.SignalProcessing{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-case-passthrough",
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
							Severity:     tc.input,
							Type:         "alert",
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
					fmt.Sprintf("Input %q should map to %q (original casing preserved)", tc.input, tc.expected))
				Expect(result.Source).To(Equal("rego-policy"),
					"Source should be rego-policy for all determinations")
			}
		})

		It("should allow operators to use Rego lower() for case-insensitive matching if desired", func() {
			// BUSINESS CONTEXT: Operators who want case-insensitive matching can use
			// Rego's built-in lower() function in their policy. This keeps the Go code
			// simple and gives full control to the policy author.
			// VALUE: Flexibility without Go-side mutation.
			// OUTCOME: Case-insensitive matching is a policy concern, not a runtime concern.

			policyWithLowerBuiltin := `
package signalprocessing.severity

determine_severity := "critical" if {
    lower(input.signal.severity) == "sev1"
} else := "high" if {
    lower(input.signal.severity) == "sev2"
} else := "low" if {
    true
}
`
			err := severityClassifier.LoadRegoPolicy(policyWithLowerBuiltin)
			Expect(err).ToNot(HaveOccurred())

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
				{"UNMAPPED", "low"},     // Fallback
			}

			for _, tc := range testCases {
				sp := &signalprocessingv1alpha1.SignalProcessing{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-lower-builtin",
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
							Name:         "TestLowerBuiltinAlert",
							Severity:     tc.input,
							Type:         "alert",
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
					fmt.Sprintf("Input %q should map to %q via Rego lower()", tc.input, tc.expected))
			}
		})
	})
})
