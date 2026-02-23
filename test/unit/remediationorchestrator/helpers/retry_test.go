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

// Business Requirement: REFACTOR-RO-001
// Purpose: Validates retry helper implementation mechanics
package helpers

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	prodhelpers "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
)

// Helper to generate random string for testing
func randString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[i%len(charset)]
	}
	return string(b)
}

func TestHelpers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RemediationOrchestrator Helpers Suite")
}

var _ = Describe("UpdateRemediationRequestStatus", func() {
	var (
		ctx        context.Context
		fakeClient client.Client
		scheme     *runtime.Scheme
		rr         *remediationv1.RemediationRequest
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		_ = remediationv1.AddToScheme(scheme)

		// Create a test RemediationRequest
		rr = &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "test-rr",
				Namespace:       "default",
				UID:             types.UID(randString(10)),
				ResourceVersion: "1",
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: randString(64),
				Severity:          "critical",
				TargetType:        "kubernetes",
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhasePending,
			},
		}

		fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(rr).WithObjects(rr).Build()
	})

	Context("REFACTOR-RO-001: Successful updates", func() {
		It("should update status fields successfully", func() {
			// Update phase and message
			err := prodhelpers.UpdateRemediationRequestStatus(ctx, fakeClient, nil, rr, func(rr *remediationv1.RemediationRequest) error {
				rr.Status.OverallPhase = remediationv1.PhaseProcessing
				rr.Status.Message = "Processing signal"
				return nil
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(rr.Status.OverallPhase).To(Equal(remediationv1.PhaseProcessing))
			Expect(rr.Status.Message).To(Equal("Processing signal"))
		})

		It("should update multiple status fields in single call", func() {
			err := prodhelpers.UpdateRemediationRequestStatus(ctx, fakeClient, nil, rr, func(rr *remediationv1.RemediationRequest) error {
				rr.Status.OverallPhase = remediationv1.PhaseSkipped
				rr.Status.SkipReason = "ResourceBusy"
				rr.Status.DuplicateOf = "parent-rr"
				rr.Status.Message = "Skipped due to resource lock"
				return nil
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(rr.Status.OverallPhase).To(Equal(remediationv1.PhaseSkipped))
			Expect(rr.Status.SkipReason).To(Equal("ResourceBusy"))
			Expect(rr.Status.DuplicateOf).To(Equal("parent-rr"))
			Expect(rr.Status.Message).To(Equal("Skipped due to resource lock"))
		})

		It("should refetch latest state before applying updates", func() {
			// Simulate external update by modifying RR directly
			rr.Status.Message = "External update"
			Expect(fakeClient.Status().Update(ctx, rr)).To(Succeed())

			// Fetch to get latest state
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)).To(Succeed())

			// Update RO fields - helper should refetch and see "External update"
			err := prodhelpers.UpdateRemediationRequestStatus(ctx, fakeClient, nil, rr, func(rr *remediationv1.RemediationRequest) error {
				// Verify refetch happened (should see external update)
				Expect(rr.Status.Message).To(Equal("External update"))

				// Apply our update
				rr.Status.OverallPhase = remediationv1.PhaseAnalyzing
				return nil
			})

			Expect(err).ToNot(HaveOccurred())

			// Verify both updates applied
			Expect(rr.Status.Message).To(Equal("External update"))                 // Preserved
			Expect(rr.Status.OverallPhase).To(Equal(remediationv1.PhaseAnalyzing)) // Updated
		})
	})

	Context("REFACTOR-RO-001: Error handling", func() {
		It("should return error when updateFn returns error", func() {
			err := prodhelpers.UpdateRemediationRequestStatus(ctx, fakeClient, nil, rr, func(rr *remediationv1.RemediationRequest) error {
				return fmt.Errorf("simulated update error")
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("simulated update error"))
		})

		It("should return error when RR not found", func() {
			// Delete the RR
			Expect(fakeClient.Delete(ctx, rr)).To(Succeed())

			err := prodhelpers.UpdateRemediationRequestStatus(ctx, fakeClient, nil, rr, func(rr *remediationv1.RemediationRequest) error {
				rr.Status.OverallPhase = remediationv1.PhaseCompleted
				return nil
			})

			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("should handle nil updateFn gracefully", func() {
			// Note: This would panic in current implementation (by design)
			// We expect caller to always provide valid updateFn
			// This test documents expected behavior
			Expect(func() {
				_ = prodhelpers.UpdateRemediationRequestStatus(ctx, fakeClient, nil, rr, nil)
			}).To(Panic())
		})
	})

	Context("Issue #118 Gap 5: SelectedWorkflowRef population", func() {
		It("UT-RR-SWR-001: should persist SelectedWorkflowRef when set in status update callback", func() {
			err := prodhelpers.UpdateRemediationRequestStatus(ctx, fakeClient, nil, rr, func(rr *remediationv1.RemediationRequest) error {
				rr.Status.SelectedWorkflowRef = &remediationv1.WorkflowReference{
					WorkflowID:      "wf-restart-pod",
					Version:         "v1.0.0",
					ExecutionBundle: "kubernaut.io/workflows/restart:v1.0.0",
				}
				return nil
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(rr.Status.SelectedWorkflowRef).NotTo(BeNil(),
				"SelectedWorkflowRef must be persisted through UpdateRemediationRequestStatus")
			Expect(rr.Status.SelectedWorkflowRef.WorkflowID).To(Equal("wf-restart-pod"))
			Expect(rr.Status.SelectedWorkflowRef.Version).To(Equal("v1.0.0"))
			Expect(rr.Status.SelectedWorkflowRef.ExecutionBundle).To(Equal("kubernaut.io/workflows/restart:v1.0.0"))
		})
	})

	Context("REFACTOR-RO-001: Refetch behavior", func() {
		It("should refetch RR before applying updates", func() {
			// Update RR externally (simulating concurrent update)
			externalRR := rr.DeepCopy()
			externalRR.Status.Message = "External update"
			Expect(fakeClient.Status().Update(ctx, externalRR)).To(Succeed())

			// Now update with helper - should see external update
			err := prodhelpers.UpdateRemediationRequestStatus(ctx, fakeClient, nil, rr, func(rr *remediationv1.RemediationRequest) error {
				// At this point, rr should have latest state including "External update"
				Expect(rr.Status.Message).To(Equal("External update"))

				// Apply our update
				rr.Status.OverallPhase = remediationv1.PhaseAnalyzing
				return nil
			})

			Expect(err).ToNot(HaveOccurred())

			// Verify both updates applied
			Expect(rr.Status.Message).To(Equal("External update"))                 // Preserved
			Expect(rr.Status.OverallPhase).To(Equal(remediationv1.PhaseAnalyzing)) // Updated
		})
	})
})
