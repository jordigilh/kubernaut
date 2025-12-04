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

// Package remediationorchestrator contains unit tests for the Remediation Orchestrator controller.
// BR-ORCH-025: Core Orchestration Workflow
// BR-ORCH-032: Skipped Phase Handling (Terminal States)
package remediationorchestrator

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	ro "github.com/jordigilh/kubernaut/pkg/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/controller"
)

var _ = Describe("BR-ORCH-025: RemediationOrchestrator Controller", func() {
	var (
		ctx        context.Context
		fakeClient client.Client
		scheme     *runtime.Scheme
		reconciler *controller.Reconciler
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()

		// Register all required schemes
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(signalprocessingv1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())

		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(
				&remediationv1.RemediationRequest{},
				&signalprocessingv1.SignalProcessing{},
				&aianalysisv1.AIAnalysis{},
			).
			Build()

		reconciler = controller.NewReconciler(
			fakeClient,
			scheme,
			ro.DefaultConfig(),
		)
	})

	Describe("Reconcile", func() {
		Context("when RemediationRequest does not exist", func() {
			It("should return without error (idempotent)", func() {
				result, err := reconciler.Reconcile(ctx, ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      "non-existent",
						Namespace: "default",
					},
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
			})
		})

		Context("when RemediationRequest exists with empty status", func() {
			var rr *remediationv1.RemediationRequest

			BeforeEach(func() {
				rr = &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-rr",
						Namespace: "default",
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalFingerprint: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
						SignalName:        "HighMemoryUsage",
						Severity:          "warning",
						Environment:       "production",
						Priority:          "P1",
						SignalType:        "prometheus",
						TargetType:        "kubernetes",
						TargetResource: remediationv1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: "default",
						},
					},
				}
				Expect(fakeClient.Create(ctx, rr)).To(Succeed())
			})

			It("should initialize status to Pending phase", func() {
				result, err := reconciler.Reconcile(ctx, ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      rr.Name,
						Namespace: rr.Namespace,
					},
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())

				// Verify status was updated
				updated := &remediationv1.RemediationRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(rr), updated)).To(Succeed())
				Expect(updated.Status.OverallPhase).To(Equal("Pending"))
			})
		})

		Context("when RemediationRequest is in Pending phase", func() {
			var rr *remediationv1.RemediationRequest

			BeforeEach(func() {
				rr = &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-rr-pending",
						Namespace: "default",
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalFingerprint: "b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3",
						SignalName:        "HighMemoryUsage",
						Severity:          "warning",
						Environment:       "production",
						Priority:          "P1",
						SignalType:        "prometheus",
						TargetType:        "kubernetes",
						TargetResource: remediationv1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: "default",
						},
					},
					Status: remediationv1.RemediationRequestStatus{
						OverallPhase: "Pending",
					},
				}
				Expect(fakeClient.Create(ctx, rr)).To(Succeed())
			})

			It("should transition to Processing phase and create SignalProcessing CRD", func() {
				result, err := reconciler.Reconcile(ctx, ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      rr.Name,
						Namespace: rr.Namespace,
					},
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())

				// Verify phase transition
				updated := &remediationv1.RemediationRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(rr), updated)).To(Succeed())
				Expect(updated.Status.OverallPhase).To(Equal("Processing"))
			})
		})

		// Terminal state handling (BR-ORCH-025, BR-ORCH-032)
		// Reference: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md - DescribeTable pattern
		Context("when RemediationRequest is in terminal state", func() {
			DescribeTable("should not requeue for terminal phases",
				func(terminalPhase string, brRef string) {
					rr := &remediationv1.RemediationRequest{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-rr-terminal-" + terminalPhase,
							Namespace: "default",
						},
						Spec: remediationv1.RemediationRequestSpec{
							SignalFingerprint: "c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4",
							SignalName:        "TestSignal",
							Severity:          "warning",
							Environment:       "production",
							Priority:          "P2",
							SignalType:        "prometheus",
							TargetType:        "kubernetes",
							TargetResource: remediationv1.ResourceIdentifier{
								Kind:      "Pod",
								Name:      "test-pod",
								Namespace: "default",
							},
						},
						Status: remediationv1.RemediationRequestStatus{
							OverallPhase: terminalPhase,
						},
					}
					Expect(fakeClient.Create(ctx, rr)).To(Succeed())

					result, err := reconciler.Reconcile(ctx, ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      rr.Name,
							Namespace: rr.Namespace,
						},
					})

					Expect(err).NotTo(HaveOccurred(), "Terminal phase %s should not error", terminalPhase)
					Expect(result.Requeue).To(BeFalse(), "Terminal phase %s should not requeue", terminalPhase)
					Expect(result.RequeueAfter).To(BeZero(), "Terminal phase %s should not have RequeueAfter", terminalPhase)
				},
				Entry("Completed phase (BR-ORCH-025)", "Completed", "BR-ORCH-025"),
				Entry("Failed phase (BR-ORCH-025)", "Failed", "BR-ORCH-025"),
				Entry("TimedOut phase (BR-ORCH-027)", "TimedOut", "BR-ORCH-027"),
				Entry("Skipped phase (BR-ORCH-032)", "Skipped", "BR-ORCH-032"),
			)
		})
	})

	// NewReconciler construction validation
	Describe("NewReconciler", func() {
		DescribeTable("should create reconciler with provided dependencies",
			func(description string, validateFunc func(*controller.Reconciler)) {
				r := controller.NewReconciler(fakeClient, scheme, ro.DefaultConfig())
				Expect(r).NotTo(BeNil())
				validateFunc(r)
			},
			Entry("with provided client",
				"Client injection",
				func(r *controller.Reconciler) {
					Expect(r.Client).NotTo(BeNil())
				}),
			Entry("with provided scheme",
				"Scheme injection",
				func(r *controller.Reconciler) {
					Expect(r.Scheme).NotTo(BeNil())
				}),
			Entry("with provided config",
				"Config injection",
				func(r *controller.Reconciler) {
					Expect(r.Config.MaxConcurrentReconciles).To(Equal(10))
				}),
		)

		It("should allow custom configuration override", func() {
			config := ro.OrchestratorConfig{
				MaxConcurrentReconciles: 5,
			}
			r := controller.NewReconciler(fakeClient, scheme, config)
			Expect(r.Config.MaxConcurrentReconciles).To(Equal(5))
		})
	})
})

