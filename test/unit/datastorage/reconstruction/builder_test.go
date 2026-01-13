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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	reconstructionpkg "github.com/jordigilh/kubernaut/pkg/datastorage/reconstruction"
)

// BR-AUDIT-006: RemediationRequest Reconstruction from Audit Traces
// Test Plan: docs/development/SOC2/SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md
// This test validates the builder component that constructs a complete RR CRD from reconstructed fields.
var _ = Describe("RemediationRequest Builder", func() {

	Context("BUILDER-01: Build complete RR from reconstructed fields", func() {
		It("should create RR with proper TypeMeta and ObjectMeta", func() {
			// Validates builder creates K8s-compliant CRD structure
			correlationID := "test-correlation-123"
			rrFields := &reconstructionpkg.ReconstructedRRFields{
				Spec: &remediationv1.RemediationRequestSpec{
					SignalName:        "HighCPU",
					SignalType:        "prometheus-alert",
					SignalLabels:      map[string]string{"alertname": "HighCPU"},
					SignalAnnotations: map[string]string{"summary": "CPU usage is high"},
				},
				Status: &remediationv1.RemediationRequestStatus{
					TimeoutConfig: &remediationv1.TimeoutConfig{
						Global: &metav1.Duration{Duration: 3600000000000}, // 1h
					},
				},
			}

			rr, err := reconstructionpkg.BuildRemediationRequest(correlationID, rrFields)

			Expect(err).ToNot(HaveOccurred())
			Expect(rr).ToNot(BeNil())

			// Validate TypeMeta
			Expect(rr.APIVersion).To(Equal("remediation.kubernaut.ai/v1alpha1"))
			Expect(rr.Kind).To(Equal("RemediationRequest"))

			// Validate ObjectMeta
			Expect(rr.Name).To(HavePrefix("rr-reconstructed-"))
			Expect(rr.Namespace).To(Equal("kubernaut-system"))
			Expect(rr.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "kubernaut-datastorage"))
			Expect(rr.Labels).To(HaveKeyWithValue("kubernaut.ai/reconstructed", "true"))
			Expect(rr.Labels).To(HaveKeyWithValue("kubernaut.ai/correlation-id", correlationID))

			// Validate Spec and Status are populated
			Expect(rr.Spec.SignalName).To(Equal("HighCPU"))
			Expect(rr.Status.TimeoutConfig).ToNot(BeNil())
		})

		It("should return error for nil rrFields", func() {
			// Validates error handling for nil input
			correlationID := "test-correlation-123"

			_, err := reconstructionpkg.BuildRemediationRequest(correlationID, nil)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("rrFields cannot be nil"))
		})

		It("should return error for empty correlation ID", func() {
			// Validates correlation ID is required
			rrFields := &reconstructionpkg.ReconstructedRRFields{
				Spec: &remediationv1.RemediationRequestSpec{
					SignalName: "HighCPU",
				},
			}

			_, err := reconstructionpkg.BuildRemediationRequest("", rrFields)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("correlation ID is required"))
		})
	})

	Context("BUILDER-02: Add reconstruction metadata", func() {
		It("should add annotations with reconstruction timestamp and source", func() {
			// Validates builder adds metadata for audit trail
			correlationID := "test-correlation-456"
			rrFields := &reconstructionpkg.ReconstructedRRFields{
				Spec: &remediationv1.RemediationRequestSpec{
					SignalName: "HighMemory",
				},
			}

			rr, err := reconstructionpkg.BuildRemediationRequest(correlationID, rrFields)

			Expect(err).ToNot(HaveOccurred())
			Expect(rr.Annotations).To(HaveKey("kubernaut.ai/reconstructed-at"))
			Expect(rr.Annotations).To(HaveKeyWithValue("kubernaut.ai/reconstruction-source", "audit-trail"))
			Expect(rr.Annotations).To(HaveKeyWithValue("kubernaut.ai/correlation-id", correlationID))
		})

		It("should add finalizer for audit retention", func() {
			// Validates builder adds finalizer to prevent premature deletion
			correlationID := "test-correlation-789"
			rrFields := &reconstructionpkg.ReconstructedRRFields{
				Spec: &remediationv1.RemediationRequestSpec{
					SignalName: "DiskFull",
				},
			}

			rr, err := reconstructionpkg.BuildRemediationRequest(correlationID, rrFields)

			Expect(err).ToNot(HaveOccurred())
			Expect(rr.Finalizers).To(ContainElement("kubernaut.ai/audit-retention"))
		})
	})

	Context("BUILDER-03: Validate required fields presence", func() {
		It("should return error when SignalName is missing", func() {
			// Validates builder checks for required spec fields
			correlationID := "test-correlation-required"
			rrFields := &reconstructionpkg.ReconstructedRRFields{
				Spec: &remediationv1.RemediationRequestSpec{
					// SignalName missing - should error
					SignalType: "prometheus-alert",
				},
			}

			_, err := reconstructionpkg.BuildRemediationRequest(correlationID, rrFields)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("SignalName is required"))
		})

		It("should allow partial Status fields (optional)", func() {
			// Validates Status fields are optional (may not be in all audit events)
			correlationID := "test-correlation-partial"
			rrFields := &reconstructionpkg.ReconstructedRRFields{
				Spec: &remediationv1.RemediationRequestSpec{
					SignalName: "NodeNotReady",
					SignalType: "kubernetes-event",
				},
				Status: &remediationv1.RemediationRequestStatus{
					// TimeoutConfig missing - should succeed (optional)
				},
			}

			rr, err := reconstructionpkg.BuildRemediationRequest(correlationID, rrFields)

			Expect(err).ToNot(HaveOccurred())
			Expect(rr).ToNot(BeNil())
			Expect(rr.Spec.SignalName).To(Equal("NodeNotReady"))
		})
	})

	// NOTE: Additional builder tests for Gaps #4-7 will be added during GREEN phase
	// when we handle workflow selection, execution, and error data
})
