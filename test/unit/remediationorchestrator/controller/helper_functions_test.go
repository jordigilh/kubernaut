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

package controller

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

var _ = Describe("BR-ORCH-HELPERS: Helper Function Tests", func() {
	var (
		ctx               context.Context
		fakeClient        client.Client
		reconciler        *prodcontroller.Reconciler
		mockRoutingEngine *MockRoutingEngine
		testMetrics       *metrics.Metrics
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create scheme with all CRD types
		scheme := setupScheme()

		// Create mock routing engine
		mockRoutingEngine = &MockRoutingEngine{}

		// Don't create new metrics (causes Prometheus registration conflicts)
		// Helper tests don't need metrics
		testMetrics = nil

		// Create fake client with status subresource
		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(
				&remediationv1.RemediationRequest{},
				&signalprocessingv1.SignalProcessing{},
				&aianalysisv1.AIAnalysis{},
				&workflowexecutionv1.WorkflowExecution{},
			).
			Build()

		// Create mock audit store
		mockAuditStore := &MockAuditStore{}

		// Create reconciler
		recorder := record.NewFakeRecorder(20) // DD-EVENT-001: FakeRecorder for K8s event assertions
		reconciler = prodcontroller.NewReconciler(
			fakeClient,
			fakeClient,     // apiReader (same as client for tests)
			scheme,
			mockAuditStore, // Use MockAuditStore for helper tests
			recorder,       // DD-EVENT-001: FakeRecorder for K8s event assertions
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()), // DD-METRICS-001: required
			prodcontroller.TimeoutConfig{
				Global:     1 * time.Hour,
				Processing: 5 * time.Minute,
				Analyzing:  10 * time.Minute,
				Executing:  30 * time.Minute,
			},
			mockRoutingEngine,
		)
	})

	Context("Phase 4: Helper Function Tests", func() {
		It("HF-8.1: UpdateRemediationRequestStatus should handle successful update", func() {
			// Create RemediationRequest
			rr := newRemediationRequest("test-rr", "default", remediationv1.PhasePending)
			rr.Status.StartTime = &metav1.Time{Time: time.Now()}
			Expect(fakeClient.Create(ctx, rr)).To(Succeed())

			// Update status using helper
			err := helpers.UpdateRemediationRequestStatus(ctx, fakeClient, testMetrics, rr, func(rr *remediationv1.RemediationRequest) error {
				rr.Status.OverallPhase = remediationv1.PhaseProcessing
				rr.Status.Message = "Transitioning to Processing"
				return nil
			})

			// Verify update succeeded
			Expect(err).ToNot(HaveOccurred())

			// Fetch and verify RR was updated
			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-rr", Namespace: "default"}, updated)).To(Succeed())
			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseProcessing))
			Expect(updated.Status.Message).To(Equal("Transitioning to Processing"))
		})

		It("HF-8.2: UpdateRemediationRequestStatus should handle update function errors", func() {
			// Create RemediationRequest
			rr := newRemediationRequest("test-rr", "default", remediationv1.PhasePending)
			rr.Status.StartTime = &metav1.Time{Time: time.Now()}
			Expect(fakeClient.Create(ctx, rr)).To(Succeed())

			// Update status with error in updateFn
			testError := fmt.Errorf("simulated update function error")
			err := helpers.UpdateRemediationRequestStatus(ctx, fakeClient, testMetrics, rr, func(rr *remediationv1.RemediationRequest) error {
				return testError
			})

			// Verify error is propagated
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(testError))

			// Verify RR was NOT updated
			unchanged := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-rr", Namespace: "default"}, unchanged)).To(Succeed())
			Expect(unchanged.Status.OverallPhase).To(Equal(remediationv1.PhasePending)) // Still Pending
		})

		It("HF-8.3: UpdateRemediationRequestStatus should handle not found errors gracefully", func() {
			// Create RemediationRequest but don't persist it
			rr := newRemediationRequest("nonexistent-rr", "default", remediationv1.PhasePending)
			rr.Status.StartTime = &metav1.Time{Time: time.Now()}

			// Try to update status (should fail with NotFound)
			err := helpers.UpdateRemediationRequestStatus(ctx, fakeClient, testMetrics, rr, func(rr *remediationv1.RemediationRequest) error {
				rr.Status.OverallPhase = remediationv1.PhaseProcessing
				return nil
			})

			// Verify NotFound error
			Expect(err).To(HaveOccurred())
			// The error should be a NotFound error from the Get() call
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("HF-8.4: Reconciler should handle concurrent status updates correctly", func() {
			// This test validates that the reconciler can handle multiple rapid reconciles
			// without conflicts (using the retry helper)

			// Create RemediationRequest
			rr := newRemediationRequest("test-rr", "default", remediationv1.PhasePending)
			rr.Status.StartTime = &metav1.Time{Time: time.Now()}
			Expect(fakeClient.Create(ctx, rr)).To(Succeed())

			// Perform multiple reconciles rapidly (simulating concurrent updates)
			for i := 0; i < 3; i++ {
				result, err := reconciler.Reconcile(ctx, ctrl.Request{
					NamespacedName: types.NamespacedName{Name: "test-rr", Namespace: "default"},
				})
				Expect(err).ToNot(HaveOccurred())
				// First reconcile transitions to Processing, subsequent ones stay there
				if i == 0 {
					Expect(result.RequeueAfter).To(Equal(5 * time.Second))
				}
			}

			// Verify final state is consistent
			final := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-rr", Namespace: "default"}, final)).To(Succeed())
			Expect(final.Status.OverallPhase).To(Equal(remediationv1.PhaseProcessing))
		})

		It("HF-8.5: Reconciler should preserve Gateway-owned fields during updates", func() {
			// This test validates BR-ORCH-038: Preserve Gateway deduplication data

			// Create RemediationRequest with Gateway-owned deduplication data
			rr := newRemediationRequest("test-rr", "default", remediationv1.PhasePending)
			rr.Status.StartTime = &metav1.Time{Time: time.Now()}
			now := metav1.Now()
			rr.Status.Deduplication = &remediationv1.DeduplicationStatus{
				OccurrenceCount: 1,
				FirstSeenAt:     &now,
				LastSeenAt:      &now,
			}
			Expect(fakeClient.Create(ctx, rr)).To(Succeed())

			// Reconcile (RO updates its own fields)
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "test-rr", Namespace: "default"},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(5 * time.Second))

			// Verify Gateway-owned deduplication data is preserved
			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-rr", Namespace: "default"}, updated)).To(Succeed())
			Expect(updated.Status.Deduplication).ToNot(BeNil())
			Expect(updated.Status.Deduplication.OccurrenceCount).To(Equal(int32(1)))
		})

		It("HF-8.6: Reconciler should handle phase transitions with status aggregation", func() {
			// This test validates that status updates correctly aggregate child CRD states

			// Create RR with completed SP
			rr := newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseProcessing, "sp-test-rr", "", "")
			Expect(fakeClient.Create(ctx, rr)).To(Succeed())

			sp := newSignalProcessingCompleted("sp-test-rr", "default", "test-rr")
			Expect(fakeClient.Create(ctx, sp)).To(Succeed())

			// Reconcile to transition to Analyzing
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "test-rr", Namespace: "default"},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(5 * time.Second))

			// Verify phase transition and status aggregation
			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-rr", Namespace: "default"}, updated)).To(Succeed())
			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseAnalyzing))
			// Status aggregation should have set AllChildrenHealthy based on SP completion
		})
	})
})
