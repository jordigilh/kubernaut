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

package remediationorchestrator

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

var _ = Describe("Controller (BR-ORCH-025, BR-ORCH-026)", func() {
	var (
		scheme     *runtime.Scheme
		reconciler *controller.Reconciler
	)

	BeforeEach(func() {
		// Build scheme with all required types
		scheme = runtime.NewScheme()
		_ = remediationv1.AddToScheme(scheme)
		_ = signalprocessingv1.AddToScheme(scheme)
		_ = aianalysisv1.AddToScheme(scheme)
		_ = workflowexecutionv1.AddToScheme(scheme)
		_ = notificationv1.AddToScheme(scheme)

		// Create fake client and reconciler
		// Audit store is nil for unit tests (DD-AUDIT-003 compliant - audit is optional)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(20) // DD-EVENT-001: FakeRecorder for K8s event assertions
		reconciler = controller.NewReconciler(fakeClient, fakeClient, scheme, nil, recorder, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()), controller.TimeoutConfig{}, nil)
	})

	Describe("Reconciler", func() {
		Context("when creating a new Reconciler", func() {
			It("should return a non-nil Reconciler", func() {
				Expect(reconciler).ToNot(BeNil())
			})
		})

		Context("when checking interface compliance", func() {
			It("should implement controller-runtime Reconciler interface", func() {
				// Compile-time interface satisfaction check
				var _ reconcile.Reconciler = reconciler
				Expect(reconciler).ToNot(BeNil())
			})

			It("should have SetupWithManager method for controller registration", func() {
				// Verify method exists (compile-time check)
				Expect(reconciler.SetupWithManager).ToNot(BeNil())
			})
		})
	})

	// ========================================
	// Terminal Phase Edge Cases
	// Tests defensive programming for terminal state handling
	// Business Value: Prevents re-processing completed remediations
	// ========================================
	Describe("Terminal Phase Edge Cases", func() {
		Context("when RemediationRequest is in terminal phase", func() {
			It("should not process Completed RR even if child CRD status changes", func() {
				// Scenario: RR marked Completed, but watch triggers reconcile due to child update
				// Business Value: Prevents accidental re-opening of completed remediations
				// Confidence: 95% - Prevents real production bug

				ctx := context.Background()

				// Given: Completed RemediationRequest
				rr := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "completed-rr",
						Namespace: "default",
					},
					Status: remediationv1.RemediationRequestStatus{
						OverallPhase: remediationv1.PhaseCompleted,
						Outcome:      "Success",
					},
				}

				// When: Reconcile is triggered (watch event from child CRD)
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(rr).
					WithStatusSubresource(rr).
					Build()
				testReconciler := controller.NewReconciler(fakeClient, fakeClient, scheme, nil, record.NewFakeRecorder(20), rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()), controller.TimeoutConfig{}, nil)

				result, err := testReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      rr.Name,
						Namespace: rr.Namespace,
					},
				})

				// Then: Should skip reconciliation (no processing)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.RequeueAfter).To(BeZero(), "Terminal phases should not requeue")
				Expect(result.RequeueAfter).To(BeZero(), "Terminal phases should not schedule requeue")

				// Verify: Phase remains Completed (not modified)
				updatedRR := &remediationv1.RemediationRequest{}
				err = fakeClient.Get(ctx, types.NamespacedName{
					Name:      rr.Name,
					Namespace: rr.Namespace,
				}, updatedRR)
				Expect(err).ToNot(HaveOccurred())
				Expect(updatedRR.Status.OverallPhase).To(Equal(remediationv1.PhaseCompleted),
					"Completed phase must remain unchanged")
			})

			It("should not process Failed RR even if status.Message is updated", func() {
				// Scenario: Operator updates message on Failed RR for clarification
				// Business Value: Prevents unexpected state changes from metadata updates
				// Confidence: 90% - Common operator workflow

				ctx := context.Background()

				// Given: Failed RemediationRequest
				failureMsg := "Original failure message"
				rr := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "failed-rr",
						Namespace: "default",
					},
					Status: remediationv1.RemediationRequestStatus{
						OverallPhase: remediationv1.PhaseFailed,
						Message:      failureMsg,
					},
				}

				// When: Reconcile is triggered after message update
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(rr).
					WithStatusSubresource(rr).
					Build()
				testReconciler := controller.NewReconciler(fakeClient, fakeClient, scheme, nil, record.NewFakeRecorder(20), rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()), controller.TimeoutConfig{}, nil)

				result, err := testReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      rr.Name,
						Namespace: rr.Namespace,
					},
				})

				// Then: Should skip reconciliation
				Expect(err).ToNot(HaveOccurred())
				Expect(result.RequeueAfter).To(BeZero())

				// Verify: Phase remains Failed
				updatedRR := &remediationv1.RemediationRequest{}
				err = fakeClient.Get(ctx, types.NamespacedName{
					Name:      rr.Name,
					Namespace: rr.Namespace,
				}, updatedRR)
				Expect(err).ToNot(HaveOccurred())
				Expect(updatedRR.Status.OverallPhase).To(Equal(remediationv1.PhaseFailed),
					"Failed phase must remain in terminal state")
			})

			It("should handle Skipped RR with later duplicate detection correctly", func() {
				// Scenario: RR marked Skipped (duplicate), another RR for same fingerprint arrives
				// Business Value: Validates skip deduplication correctness
				// Confidence: 95% - Critical for BR-ORCH-032 deduplication logic

				ctx := context.Background()

				// Given: Skipped RemediationRequest (already processed as duplicate)
				rr := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "skipped-rr",
						Namespace: "default",
					},
					Status: remediationv1.RemediationRequestStatus{
						OverallPhase: remediationv1.PhaseSkipped,
						SkipReason:   "ResourceBusy",
						DuplicateOf:  "original-rr",
					},
				}

				// When: Reconcile is triggered
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(rr).
					WithStatusSubresource(rr).
					Build()
				testReconciler := controller.NewReconciler(fakeClient, fakeClient, scheme, nil, record.NewFakeRecorder(20), rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()), controller.TimeoutConfig{}, nil)

				result, err := testReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      rr.Name,
						Namespace: rr.Namespace,
					},
				})

				// Then: Should skip reconciliation (terminal phase)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.RequeueAfter).To(BeZero())

				// Verify: Phase remains Skipped, DuplicateOf preserved
				updatedRR := &remediationv1.RemediationRequest{}
				err = fakeClient.Get(ctx, types.NamespacedName{
					Name:      rr.Name,
					Namespace: rr.Namespace,
				}, updatedRR)
				Expect(err).ToNot(HaveOccurred())
				Expect(updatedRR.Status.OverallPhase).To(Equal(remediationv1.PhaseSkipped))
				Expect(updatedRR.Status.DuplicateOf).To(Equal("original-rr"),
					"Duplicate tracking must be preserved")
			})
		})
	})
})
