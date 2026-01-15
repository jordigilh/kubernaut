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

package reconstruction

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	reconstructionpkg "github.com/jordigilh/kubernaut/pkg/datastorage/reconstruction"
)

// BR-AUDIT-006: RemediationRequest Reconstruction from Audit Traces
// Test Plan: docs/development/SOC2/SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md
// This test validates the validator component that checks reconstructed RR quality and completeness.
var _ = Describe("Reconstruction Validator", func() {

	Context("VALIDATOR-01: Validate required fields", func() {
		It("should pass validation for complete RR with all required fields", func() {
			// Validates validator accepts RR with all required fields
			// Updated Jan 14, 2026: Added Gaps #4, #5, #6 fields for comprehensive validation
			rr := &remediationv1.RemediationRequest{
				Spec: remediationv1.RemediationRequestSpec{
					SignalName:        "HighCPU",
					SignalType:        "prometheus-alert",
					SignalLabels:      map[string]string{"alertname": "HighCPU"},
					SignalAnnotations: map[string]string{"summary": "High CPU"},
					OriginalPayload:   []byte(`{"alert":"data"}`),
					ProviderData:      []byte(`{"incident_id":"test-123"}`), // Gap #4
				},
				Status: remediationv1.RemediationRequestStatus{
					SelectedWorkflowRef: &remediationv1.WorkflowReference{ // Gap #5
						WorkflowID:     "test-workflow-001",
						Version:        "v1.0.0",
						ContainerImage: "test/workflow:latest",
					},
					ExecutionRef: &corev1.ObjectReference{ // Gap #6
						Name:      "test-execution-001",
						Namespace: "default",
					},
					TimeoutConfig: &remediationv1.TimeoutConfig{
						Global: &metav1.Duration{Duration: 3600000000000},
					},
				},
			}

			result, err := reconstructionpkg.ValidateReconstructedRR(rr)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.IsValid).To(BeTrue())
			Expect(result.Errors).To(BeEmpty())
			Expect(result.Completeness).To(Equal(100)) // All 9 fields present = 100% complete
		})

		It("should fail validation when SignalName is missing", func() {
			// Validates validator rejects RR without required SignalName
			rr := &remediationv1.RemediationRequest{
				Spec: remediationv1.RemediationRequestSpec{
					SignalType: "prometheus-alert",
					// SignalName missing - should fail
				},
			}

			result, err := reconstructionpkg.ValidateReconstructedRR(rr)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.IsValid).To(BeFalse())
			Expect(result.Errors).To(ContainElement(ContainSubstring("SignalName is required")))
		})

		It("should fail validation when SignalType is missing", func() {
			// Validates validator rejects RR without required SignalType
			rr := &remediationv1.RemediationRequest{
				Spec: remediationv1.RemediationRequestSpec{
					SignalName: "HighCPU",
					// SignalType missing - should fail
				},
			}

			result, err := reconstructionpkg.ValidateReconstructedRR(rr)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(result.IsValid).To(BeFalse())
			Expect(result.Errors).To(ContainElement(ContainSubstring("SignalType is required")))
		})
	})

	Context("VALIDATOR-02: Calculate completeness percentage", func() {
		It("should calculate 100% completeness for fully populated RR", func() {
			// Validates completeness calculation for RR with all fields
			// Updated Jan 14, 2026: Added all 9 fields for true 100% completeness
			rr := &remediationv1.RemediationRequest{
				Spec: remediationv1.RemediationRequestSpec{
					SignalName:        "HighCPU",
					SignalType:        "prometheus-alert",
					SignalLabels:      map[string]string{"alertname": "HighCPU"},
					SignalAnnotations: map[string]string{"summary": "CPU usage is high"},
					OriginalPayload:   []byte(`{"alert":"data"}`),
					ProviderData:      []byte(`{"incident_id":"test-456","analysis":"complete"}`), // Gap #4
				},
				Status: remediationv1.RemediationRequestStatus{
					SelectedWorkflowRef: &remediationv1.WorkflowReference{ // Gap #5
						WorkflowID:      "workflow-002",
						Version:         "v2.1.0",
						ContainerImage:  "registry/workflow:v2.1.0",
						ContainerDigest: "sha256:abcdef123456",
					},
					ExecutionRef: &corev1.ObjectReference{ // Gap #6
						Name:      "execution-002",
						Namespace: "production",
						Kind:      "WorkflowExecution",
					},
					TimeoutConfig: &remediationv1.TimeoutConfig{
						Global: &metav1.Duration{Duration: 3600000000000},
					},
				},
			}

			result, err := reconstructionpkg.ValidateReconstructedRR(rr)

			Expect(err).ToNot(HaveOccurred())
			Expect(result.IsValid).To(BeTrue())
			Expect(result.Completeness).To(Equal(100)) // All 9 fields = 100% completeness
		})

		It("should calculate lower completeness for minimal RR", func() {
			// Validates completeness calculation for RR with only required fields
			// Updated Jan 14, 2026: Adjusted expectations for 9 total validation fields
			rr := &remediationv1.RemediationRequest{
				Spec: remediationv1.RemediationRequestSpec{
					SignalName: "HighCPU",
					SignalType: "prometheus-alert",
					// All 7 optional fields missing (Gaps #1-6, #8)
				},
			}

			result, err := reconstructionpkg.ValidateReconstructedRR(rr)

			Expect(err).ToNot(HaveOccurred())
			Expect(result.IsValid).To(BeTrue())
			Expect(result.Completeness).To(Equal(22)) // 2 required fields / 9 total = 22%
			Expect(result.Warnings).To(HaveLen(7))    // All 7 optional fields missing
		})
	})

	Context("VALIDATOR-03: Generate warnings for missing optional fields", func() {
		It("should warn when SignalLabels are missing", func() {
			// Validates warning for missing optional SignalLabels
			rr := &remediationv1.RemediationRequest{
				Spec: remediationv1.RemediationRequestSpec{
					SignalName: "HighCPU",
					SignalType: "prometheus-alert",
					// SignalLabels missing - should warn
				},
			}

			result, err := reconstructionpkg.ValidateReconstructedRR(rr)

			Expect(err).ToNot(HaveOccurred())
			Expect(result.IsValid).To(BeTrue()) // Still valid, just incomplete
			Expect(result.Warnings).To(ContainElement(ContainSubstring("SignalLabels")))
		})

		It("should warn when TimeoutConfig is missing", func() {
			// Validates warning for missing optional TimeoutConfig
			rr := &remediationv1.RemediationRequest{
				Spec: remediationv1.RemediationRequestSpec{
					SignalName: "HighCPU",
					SignalType: "prometheus-alert",
				},
				// Status.TimeoutConfig missing - should warn
			}

			result, err := reconstructionpkg.ValidateReconstructedRR(rr)

			Expect(err).ToNot(HaveOccurred())
			Expect(result.IsValid).To(BeTrue()) // Still valid, just incomplete
			Expect(result.Warnings).To(ContainElement(ContainSubstring("TimeoutConfig")))
		})

		It("should not warn when optional fields are present", func() {
			// Validates no warnings for complete RR
			// Updated Jan 14, 2026: Added all 9 fields (Gaps #1-8) for zero warnings
			rr := &remediationv1.RemediationRequest{
				Spec: remediationv1.RemediationRequestSpec{
					SignalName:        "HighCPU",
					SignalType:        "prometheus-alert",
					SignalLabels:      map[string]string{"alertname": "HighCPU"},
					SignalAnnotations: map[string]string{"summary": "CPU usage is high"},
					OriginalPayload:   []byte(`{"alert":"data"}`),
					ProviderData:      []byte(`{"incident_id":"test-789"}`), // Gap #4
				},
				Status: remediationv1.RemediationRequestStatus{
					SelectedWorkflowRef: &remediationv1.WorkflowReference{ // Gap #5
						WorkflowID:     "workflow-003",
						Version:        "v1.2.0",
						ContainerImage: "registry/workflow:v1.2.0",
					},
					ExecutionRef: &corev1.ObjectReference{ // Gap #6
						Name:      "execution-003",
						Namespace: "default",
					},
					TimeoutConfig: &remediationv1.TimeoutConfig{ // Gap #8
						Global: &metav1.Duration{Duration: 3600000000000},
					},
				},
			}

			result, err := reconstructionpkg.ValidateReconstructedRR(rr)

			Expect(err).ToNot(HaveOccurred())
			Expect(result.IsValid).To(BeTrue())
			Expect(result.Warnings).To(BeEmpty()) // All 9 fields present = no warnings
			Expect(result.Completeness).To(Equal(100))
		})
	})

	// NOTE: Additional validator tests for Gaps #4-7 will be added during GREEN phase
	// when we validate workflow selection, execution, and error data
})
