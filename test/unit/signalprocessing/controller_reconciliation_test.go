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

// Package signalprocessing contains unit tests for the SignalProcessing controller.
//
// Per ADR-004: Fake Kubernetes Client for Unit Testing
// These tests use controller-runtime's fake client to test controller reconciliation.
//
// Test Tier: UNIT (per TESTING_GUIDELINES.md)
// Coverage Target: 70%+ for controller code
//
// BR Coverage:
//   - BR-SP-001: K8s Context Enrichment
//   - BR-SP-051-053: Environment Classification
//   - BR-SP-070-072: Priority Assignment
//   - BR-SP-090: Categorization Audit Trail
//   - BR-SP-100: Owner Chain Traversal
//   - BR-SP-102: Custom Labels
package signalprocessing

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/signalprocessing"
	"github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/evaluator"
	spaudit "github.com/jordigilh/kubernaut/pkg/signalprocessing/audit"
	spmetrics "github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
	spstatus "github.com/jordigilh/kubernaut/pkg/signalprocessing/status"
	"github.com/prometheus/client_golang/prometheus"
)

// mockAuditStore implements audit.AuditStore for testing
type mockAuditStore struct {
	events []*ogenclient.AuditEventRequest
}

func (m *mockAuditStore) StoreAudit(_ context.Context, event *ogenclient.AuditEventRequest) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockAuditStore) Flush(ctx context.Context) error {
	// Mock: no-op - events already stored synchronously
	return nil
}

func (m *mockAuditStore) Close() error {
	return nil
}

var _ audit.AuditStore = &mockAuditStore{}



var _ = Describe("SignalProcessing Controller Reconciliation (ADR-004)", func() {
	var (
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		_ = signalprocessingv1alpha1.AddToScheme(scheme)
		_ = corev1.AddToScheme(scheme)
	})

	// ========================================
	// Phase Initialization Tests
	// Per unit-test-plan.md: Controller initializes status correctly
	// ========================================
	Describe("Phase Initialization", func() {
		Context("when SignalProcessing is created without status", func() {
			It("CTRL-INIT-01: should initialize phase to Pending", func() {
				// Given: SignalProcessing with empty status
				sp := &signalprocessingv1alpha1.SignalProcessing{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-sp",
						Namespace:  "default",
						Generation: 1, // K8s increments on create/update
					},
					Spec: signalprocessingv1alpha1.SignalProcessingSpec{
						Signal: signalprocessingv1alpha1.SignalData{
							Fingerprint: "test-fingerprint-001",
							TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
								Kind:      "Pod",
								Name:      "test-pod",
								Namespace: "default",
							},
						},
					},
				}

				// When: Reconcile is triggered
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(sp).
					WithStatusSubresource(sp).
					Build()

				reconciler := &controller.SignalProcessingReconciler{
					Client:        fakeClient,
					Scheme:        scheme,
					StatusManager: spstatus.NewManager(fakeClient, fakeClient),
					Metrics:       spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				}

				result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      sp.Name,
						Namespace: sp.Namespace,
					},
				})

				// Then: Phase should be initialized to Pending
				Expect(err).ToNot(HaveOccurred())
				Expect(result.RequeueAfter).To(BeNumerically(">", 0), "Should requeue to continue processing")

				// Verify status was updated
				updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
				err = fakeClient.Get(context.Background(), types.NamespacedName{
					Name:      sp.Name,
					Namespace: sp.Namespace,
				}, updatedSP)
				Expect(err).ToNot(HaveOccurred())
				Expect(updatedSP.Status.Phase).To(Equal(signalprocessingv1alpha1.PhasePending))
				Expect(updatedSP.Status.StartTime).To(Not(BeNil()), "StartTime must be set on first reconciliation")
			})
		})

		Context("when SignalProcessing is not found", func() {
			It("CTRL-INIT-02: should return without error (deleted)", func() {
				// Given: Empty cluster (no SignalProcessing)
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					Build()

				reconciler := &controller.SignalProcessingReconciler{
					Client:        fakeClient,
					Scheme:        scheme,
					StatusManager: spstatus.NewManager(fakeClient, fakeClient),
					Metrics:       spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				}

				// When: Reconcile is triggered for non-existent resource
				result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "non-existent",
						Namespace: "default",
					},
				})

				// Then: Should return without error (resource was deleted)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.RequeueAfter).To(BeZero())
			})
		})
	})

	// ========================================
	// Pending → Enriching Transition Tests
	// Per unit-test-plan.md: reconcilePending at 0% coverage
	// ========================================
	Describe("reconcilePending", func() {
		Context("when SignalProcessing is in Pending phase", func() {
			It("CTRL-PEND-01: should transition to Enriching phase", func() {
				// Given: SignalProcessing in Pending phase
				sp := &signalprocessingv1alpha1.SignalProcessing{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-sp-pending",
						Namespace:  "default",
						Generation: 1, // K8s increments on create/update
					},
					Spec: signalprocessingv1alpha1.SignalProcessingSpec{
						Signal: signalprocessingv1alpha1.SignalData{
							Fingerprint: "test-fingerprint-002",
							TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
								Kind:      "Deployment",
								Name:      "test-deploy",
								Namespace: "default",
							},
						},
					},
					Status: signalprocessingv1alpha1.SignalProcessingStatus{
						Phase:     signalprocessingv1alpha1.PhasePending,
						StartTime: &metav1.Time{Time: metav1.Now().Time},
					},
				}

				// When: Reconcile is triggered
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(sp).
					WithStatusSubresource(sp).
					Build()

				// Setup audit client (ADR-032: audit is MANDATORY)
				mockStore := &mockAuditStore{}
				auditClient := spaudit.NewAuditClient(mockStore, logr.Discard())

				reconciler := &controller.SignalProcessingReconciler{
					Client:        fakeClient,
					Scheme:        scheme,
					StatusManager: spstatus.NewManager(fakeClient, fakeClient),
					Metrics:       spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
					AuditManager:  spaudit.NewManager(auditClient),
					K8sEnricher:   newDefaultMockK8sEnricher(),
				}

				result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      sp.Name,
						Namespace: sp.Namespace,
					},
				})

				// Then: Should transition to Enriching and requeue
				Expect(err).ToNot(HaveOccurred())
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

				// Verify phase transition
				updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
				err = fakeClient.Get(context.Background(), types.NamespacedName{
					Name:      sp.Name,
					Namespace: sp.Namespace,
				}, updatedSP)
				Expect(err).ToNot(HaveOccurred())
				Expect(updatedSP.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseEnriching))
			})
		})
	})

	// ========================================
	// Terminal Phase Tests
	// Prevents re-processing of completed signals
	// ========================================
	Describe("Terminal Phase Handling", func() {
		Context("when SignalProcessing is in Completed phase", func() {
			It("CTRL-TERM-01: should not reprocess Completed signals", func() {
				// Given: Completed SignalProcessing
				sp := &signalprocessingv1alpha1.SignalProcessing{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "completed-sp",
						Namespace:  "default",
						Generation: 1, // K8s increments on create/update
					},
					Spec: signalprocessingv1alpha1.SignalProcessingSpec{
						Signal: signalprocessingv1alpha1.SignalData{
							Fingerprint: "test-fingerprint-complete",
							TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
								Kind:      "Pod",
								Name:      "test-pod",
								Namespace: "default",
							},
						},
					},
					Status: signalprocessingv1alpha1.SignalProcessingStatus{
						Phase:     signalprocessingv1alpha1.PhaseCompleted,
						StartTime: &metav1.Time{Time: metav1.Now().Time},
					},
				}

				// When: Reconcile is triggered
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(sp).
					WithStatusSubresource(sp).
					Build()

				reconciler := &controller.SignalProcessingReconciler{
					Client:        fakeClient,
					Scheme:        scheme,
					StatusManager: spstatus.NewManager(fakeClient, fakeClient),
					Metrics:       spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				}

				result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      sp.Name,
						Namespace: sp.Namespace,
					},
				})

				// Then: Should not requeue (terminal state)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.RequeueAfter).To(BeZero())
				Expect(result.RequeueAfter).To(BeZero())

				// Verify phase unchanged
				updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
				err = fakeClient.Get(context.Background(), types.NamespacedName{
					Name:      sp.Name,
					Namespace: sp.Namespace,
				}, updatedSP)
				Expect(err).ToNot(HaveOccurred())
				Expect(updatedSP.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))
			})
		})

		Context("when SignalProcessing is in Failed phase", func() {
			It("CTRL-TERM-02: should not reprocess Failed signals", func() {
				// Given: Failed SignalProcessing
				sp := &signalprocessingv1alpha1.SignalProcessing{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "failed-sp",
						Namespace:  "default",
						Generation: 1, // K8s increments on create/update
					},
					Spec: signalprocessingv1alpha1.SignalProcessingSpec{
						Signal: signalprocessingv1alpha1.SignalData{
							Fingerprint: "test-fingerprint-failed",
							TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
								Kind:      "Pod",
								Name:      "test-pod",
								Namespace: "default",
							},
						},
					},
					Status: signalprocessingv1alpha1.SignalProcessingStatus{
						Phase:     signalprocessingv1alpha1.PhaseFailed,
						StartTime: &metav1.Time{Time: metav1.Now().Time},
					},
				}

				// When: Reconcile is triggered
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(sp).
					WithStatusSubresource(sp).
					Build()

				reconciler := &controller.SignalProcessingReconciler{
					Client:        fakeClient,
					Scheme:        scheme,
					StatusManager: spstatus.NewManager(fakeClient, fakeClient),
					Metrics:       spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				}

				result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      sp.Name,
						Namespace: sp.Namespace,
					},
				})

				// Then: Should not requeue (terminal state)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.RequeueAfter).To(BeZero())

				// Verify phase unchanged
				updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
				err = fakeClient.Get(context.Background(), types.NamespacedName{
					Name:      sp.Name,
					Namespace: sp.Namespace,
				}, updatedSP)
				Expect(err).ToNot(HaveOccurred())
				Expect(updatedSP.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseFailed))
			})
		})
	})

	// ========================================
	// Enriching Phase Tests (BR-SP-001)
	// Per unit-test-plan.md: enrichDeployment, enrichStatefulSet at 0%
	// ========================================
	Describe("reconcileEnriching (BR-SP-001)", func() {
		var (
			mockStore   *mockAuditStore
			auditClient *spaudit.AuditClient
		)

		BeforeEach(func() {
			mockStore = &mockAuditStore{}
			auditClient = spaudit.NewAuditClient(mockStore, logr.Discard())
		})

		Context("when enriching Pod signal", func() {
			It("CTRL-ENRICH-01: should enrich namespace context for Pod", func() {
				// Given: SignalProcessing in Enriching phase with Pod target
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-namespace",
						Labels: map[string]string{
							"env": "production",
						},
					},
				}

				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "test-namespace",
						Labels: map[string]string{
							"app": "test-app",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "main", Image: "nginx:latest"},
						},
					},
				}

				sp := &signalprocessingv1alpha1.SignalProcessing{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "enrich-pod-sp",
						Namespace:  "default",
						Generation: 1, // K8s increments on create/update
					},
					Spec: signalprocessingv1alpha1.SignalProcessingSpec{
						Signal: signalprocessingv1alpha1.SignalData{
							Fingerprint: "test-fingerprint-enrich",
							TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
								Kind:      "Pod",
								Name:      "test-pod",
								Namespace: "test-namespace",
							},
						},
					},
					Status: signalprocessingv1alpha1.SignalProcessingStatus{
						Phase:     signalprocessingv1alpha1.PhaseEnriching,
						StartTime: &metav1.Time{Time: metav1.Now().Time},
					},
				}

				// When: Reconcile is triggered
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(ns, pod, sp).
					WithStatusSubresource(sp).
					Build()

				reconciler := &controller.SignalProcessingReconciler{
					Client:        fakeClient,
					Scheme:        scheme,
					StatusManager: spstatus.NewManager(fakeClient, fakeClient),
					Metrics:       spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
					AuditManager:  spaudit.NewManager(auditClient),
					K8sEnricher:   newMockK8sEnricherWithClient(fakeClient),
				}

				result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      sp.Name,
						Namespace: sp.Namespace,
					},
				})

				// Then: Should continue processing (requeue for next phase)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

				// Verify enrichment occurred
				updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
				err = fakeClient.Get(context.Background(), types.NamespacedName{
					Name:      sp.Name,
					Namespace: sp.Namespace,
				}, updatedSP)
				Expect(err).ToNot(HaveOccurred())

				// Namespace context should be populated
				Expect(updatedSP.Status.KubernetesContext).To(Not(BeNil()))
				Expect(updatedSP.Status.KubernetesContext.Namespace).To(And(Not(BeNil()), HaveField("Name", Equal("test-namespace"))))
				Expect(updatedSP.Status.KubernetesContext.Namespace.Labels["env"]).To(Equal("production"))
			})
		})

		Context("when namespace does not exist", func() {
			It("CTRL-ENRICH-02: should handle missing namespace gracefully", func() {
				// Given: SignalProcessing targeting non-existent namespace
				sp := &signalprocessingv1alpha1.SignalProcessing{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "enrich-missing-ns",
						Namespace:  "default",
						Generation: 1, // K8s increments on create/update
					},
					Spec: signalprocessingv1alpha1.SignalProcessingSpec{
						Signal: signalprocessingv1alpha1.SignalData{
							Fingerprint: "test-fingerprint-missing-ns",
							TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
								Kind:      "Pod",
								Name:      "test-pod",
								Namespace: "non-existent-namespace",
							},
						},
					},
					Status: signalprocessingv1alpha1.SignalProcessingStatus{
						Phase:     signalprocessingv1alpha1.PhaseEnriching,
						StartTime: &metav1.Time{Time: metav1.Now().Time},
					},
				}

				// When: Reconcile is triggered (no namespace in cluster)
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(sp).
					WithStatusSubresource(sp).
					Build()

				reconciler := &controller.SignalProcessingReconciler{
					Client:        fakeClient,
					Scheme:        scheme,
					StatusManager: spstatus.NewManager(fakeClient, fakeClient),
					Metrics:       spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
					AuditManager:  spaudit.NewManager(auditClient),
					K8sEnricher:   newDefaultMockK8sEnricher(),
				}

				result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      sp.Name,
						Namespace: sp.Namespace,
					},
				})

				// Then: Should continue processing (graceful degradation)
				Expect(err).ToNot(HaveOccurred())
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

				// Verify enrichment continued (degraded mode)
				updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
				err = fakeClient.Get(context.Background(), types.NamespacedName{
					Name:      sp.Name,
					Namespace: sp.Namespace,
				}, updatedSP)
				Expect(err).ToNot(HaveOccurred())
				// Controller should still transition to next phase
				Expect(updatedSP.Status.Phase).ToNot(Equal(signalprocessingv1alpha1.PhaseFailed))
			})
		})
	})

	// ========================================
	// Classifying Phase Tests (BR-SP-051-053)
	// Per unit-test-plan.md: reconcileClassifying at 0% coverage
	// ========================================
	Describe("reconcileClassifying (BR-SP-051-053)", func() {
		var (
			mockStore   *mockAuditStore
			auditClient *spaudit.AuditClient
		)

		BeforeEach(func() {
			mockStore = &mockAuditStore{}
			auditClient = spaudit.NewAuditClient(mockStore, logr.Discard())
		})

		Context("when SignalProcessing is in Classifying phase", func() {
			It("CTRL-CLASS-01: should attempt classification and transition", func() {
				// Given: SignalProcessing in Classifying phase with enriched context
				sp := &signalprocessingv1alpha1.SignalProcessing{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "classify-sp",
						Namespace:  "default",
						Generation: 1, // K8s increments on create/update
					},
					Spec: signalprocessingv1alpha1.SignalProcessingSpec{
						Signal: signalprocessingv1alpha1.SignalData{
							Fingerprint: "test-fingerprint-classify",
							Severity:    "critical",
							TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
								Kind:      "Deployment",
								Name:      "test-deploy",
								Namespace: "default",
							},
						},
					},
					Status: signalprocessingv1alpha1.SignalProcessingStatus{
						Phase:     signalprocessingv1alpha1.PhaseClassifying,
						StartTime: &metav1.Time{Time: metav1.Now().Time},
						KubernetesContext: &signalprocessingv1alpha1.KubernetesContext{
							Namespace: &signalprocessingv1alpha1.NamespaceContext{
								Name: "default",
								Labels: map[string]string{
									"env": "production",
								},
							},
						},
					},
				}

				// When: Reconcile is triggered with mock classifiers
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(sp).
					WithStatusSubresource(sp).
					Build()

				reconciler := &controller.SignalProcessingReconciler{
					Client:           fakeClient,
					Scheme:           scheme,
					StatusManager:    spstatus.NewManager(fakeClient, fakeClient),
					Metrics:          spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
					AuditManager:     spaudit.NewManager(auditClient),
					PolicyEvaluator:  newDefaultMockPolicyEvaluator(),
				}

				result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      sp.Name,
						Namespace: sp.Namespace,
					},
				})

				// Then: Should requeue for next phase transition
				Expect(err).ToNot(HaveOccurred())
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))
			})
		})
	})

	// ========================================
	// Issue #437: Informer cache staleness defensive guard
	// reconcileClassifying must use FreshGet and requeue if
	// KubernetesContext is not yet visible (enrichment race).
	// ========================================
	Describe("Issue #437: Classifying with stale informer cache", func() {
		var (
			mockStore   *mockAuditStore
			auditClient *spaudit.AuditClient
		)

		BeforeEach(func() {
			mockStore = &mockAuditStore{}
			auditClient = spaudit.NewAuditClient(mockStore, logr.Discard())
		})

		It("UT-SP-437-001: should requeue when KubernetesContext is nil (enrichment not propagated)", func() {
			staleSP := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "sp-437-nil-ctx",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "fp-437-nil-ctx",
						Name:        "HighCPU",
						Severity:    "critical",
						Type:        "alert",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "api-server-xyz",
							Namespace: "production-ns",
						},
					},
				},
				Status: signalprocessingv1alpha1.SignalProcessingStatus{
					Phase:             signalprocessingv1alpha1.PhaseClassifying,
					StartTime:         &metav1.Time{Time: time.Now()},
					KubernetesContext: nil,
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(staleSP).
				WithStatusSubresource(staleSP).
				Build()

			envCalled := false
			mockEval := &mockPolicyEvaluator{
				EvaluateEnvironmentFunc: func(_ context.Context, _ evaluator.PolicyInput) (*signalprocessingv1alpha1.EnvironmentClassification, error) {
					envCalled = true
					return &signalprocessingv1alpha1.EnvironmentClassification{Environment: signalprocessingv1alpha1.EnvironmentUnknown, Source: "default", ClassifiedAt: metav1.Now()}, nil
				},
			}

			reconciler := &controller.SignalProcessingReconciler{
				Client:          fakeClient,
				Scheme:          scheme,
				StatusManager:   spstatus.NewManager(fakeClient, fakeClient),
				Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				AuditManager:    spaudit.NewManager(auditClient),
				PolicyEvaluator: mockEval,
				Recorder:        record.NewFakeRecorder(20),
			}

			result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
				NamespacedName: types.NamespacedName{Name: staleSP.Name, Namespace: staleSP.Namespace},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">=", 500*time.Millisecond),
				"UT-SP-437-001: Should requeue with >= 500ms delay when KubernetesContext is nil")
			Expect(envCalled).To(BeFalse(),
				"UT-SP-437-001: PolicyEvaluator must NOT be called when KubernetesContext is nil")
		})

		It("UT-SP-437-002: should requeue when Namespace is nil inside KubernetesContext", func() {
			staleSP := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "sp-437-nil-ns",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "fp-437-nil-ns",
						Name:        "HighCPU",
						Severity:    "critical",
						Type:        "alert",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "api-server-xyz",
							Namespace: "production-ns",
						},
					},
				},
				Status: signalprocessingv1alpha1.SignalProcessingStatus{
					Phase:     signalprocessingv1alpha1.PhaseClassifying,
					StartTime: &metav1.Time{Time: time.Now()},
					KubernetesContext: &signalprocessingv1alpha1.KubernetesContext{
						Namespace: nil,
					},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(staleSP).
				WithStatusSubresource(staleSP).
				Build()

			envCalled := false
			mockEval := &mockPolicyEvaluator{
				EvaluateEnvironmentFunc: func(_ context.Context, _ evaluator.PolicyInput) (*signalprocessingv1alpha1.EnvironmentClassification, error) {
					envCalled = true
					return &signalprocessingv1alpha1.EnvironmentClassification{Environment: signalprocessingv1alpha1.EnvironmentUnknown, Source: "default", ClassifiedAt: metav1.Now()}, nil
				},
			}

			reconciler := &controller.SignalProcessingReconciler{
				Client:          fakeClient,
				Scheme:          scheme,
				StatusManager:   spstatus.NewManager(fakeClient, fakeClient),
				Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				AuditManager:    spaudit.NewManager(auditClient),
				PolicyEvaluator: mockEval,
				Recorder:        record.NewFakeRecorder(20),
			}

			result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
				NamespacedName: types.NamespacedName{Name: staleSP.Name, Namespace: staleSP.Namespace},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">=", 500*time.Millisecond),
				"UT-SP-437-002: Should requeue when Namespace is nil in KubernetesContext")
			Expect(envCalled).To(BeFalse(),
				"UT-SP-437-002: PolicyEvaluator must NOT be called when Namespace is nil")
		})

		It("UT-SP-437-003: should proceed after safety valve timeout (30s)", func() {
			staleSP := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "sp-437-timeout",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "fp-437-timeout",
						Name:        "HighCPU",
						Severity:    "critical",
						Type:        "alert",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "api-server-xyz",
							Namespace: "production-ns",
						},
					},
				},
				Status: signalprocessingv1alpha1.SignalProcessingStatus{
					Phase:             signalprocessingv1alpha1.PhaseClassifying,
					StartTime:         &metav1.Time{Time: time.Now().Add(-60 * time.Second)},
					KubernetesContext: nil,
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(staleSP).
				WithStatusSubresource(staleSP).
				Build()

			envCalled := false
			mockEval := &mockPolicyEvaluator{
				EvaluateEnvironmentFunc: func(_ context.Context, _ evaluator.PolicyInput) (*signalprocessingv1alpha1.EnvironmentClassification, error) {
					envCalled = true
					return &signalprocessingv1alpha1.EnvironmentClassification{Environment: signalprocessingv1alpha1.EnvironmentUnknown, Source: "default", ClassifiedAt: metav1.Now()}, nil
				},
			}

			reconciler := &controller.SignalProcessingReconciler{
				Client:          fakeClient,
				Scheme:          scheme,
				StatusManager:   spstatus.NewManager(fakeClient, fakeClient),
				Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				AuditManager:    spaudit.NewManager(auditClient),
				PolicyEvaluator: mockEval,
				Recorder:        record.NewFakeRecorder(20),
			}

			_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
				NamespacedName: types.NamespacedName{Name: staleSP.Name, Namespace: staleSP.Namespace},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(envCalled).To(BeTrue(),
				"UT-SP-437-003: PolicyEvaluator MUST be called after safety valve timeout")
		})

		It("UT-SP-437-004: should proceed normally with complete KubernetesContext (no regression)", func() {
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "sp-437-normal",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "fp-437-normal",
						Name:        "HighCPU",
						Severity:    "critical",
						Type:        "alert",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "api-server-xyz",
							Namespace: "production-ns",
						},
					},
				},
				Status: signalprocessingv1alpha1.SignalProcessingStatus{
					Phase:     signalprocessingv1alpha1.PhaseClassifying,
					StartTime: &metav1.Time{Time: time.Now()},
					KubernetesContext: &signalprocessingv1alpha1.KubernetesContext{
						Namespace: &signalprocessingv1alpha1.NamespaceContext{
							Name: "production-ns",
							Labels: map[string]string{
								"kubernaut.ai/environment": "production",
								"kubernaut.ai/managed":     "true",
							},
						},
						DegradedMode: true,
					},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(sp).
				WithStatusSubresource(sp).
				Build()

			envCalled := false
			mockEval := &mockPolicyEvaluator{
				EvaluateEnvironmentFunc: func(_ context.Context, input evaluator.PolicyInput) (*signalprocessingv1alpha1.EnvironmentClassification, error) {
					envCalled = true
					Expect(input.Namespace.Labels).To(HaveKeyWithValue("kubernaut.ai/environment", "production"))
					return &signalprocessingv1alpha1.EnvironmentClassification{
						Environment:  signalprocessingv1alpha1.EnvironmentProduction,
						Source:       "namespace-labels",
						ClassifiedAt: metav1.Now(),
					}, nil
				},
			}

			reconciler := &controller.SignalProcessingReconciler{
				Client:          fakeClient,
				Scheme:          scheme,
				StatusManager:   spstatus.NewManager(fakeClient, fakeClient),
				Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				AuditManager:    spaudit.NewManager(auditClient),
				PolicyEvaluator: mockEval,
				Recorder:        record.NewFakeRecorder(20),
			}

			result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
				NamespacedName: types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0),
				"UT-SP-437-004: Should requeue for next phase transition")
			Expect(envCalled).To(BeTrue(),
				"UT-SP-437-004: PolicyEvaluator MUST be called with complete KubernetesContext")
		})
	})

	// ========================================
	// Categorizing Phase Tests (BR-SP-080-081)
	// Per unit-test-plan.md: reconcileCategorizing at 0% coverage
	// ========================================
	Describe("reconcileCategorizing (BR-SP-080-081)", func() {
		var (
			mockStore   *mockAuditStore
			auditClient *spaudit.AuditClient
		)

		BeforeEach(func() {
			mockStore = &mockAuditStore{}
			auditClient = spaudit.NewAuditClient(mockStore, logr.Discard())
		})

		Context("when SignalProcessing is in Categorizing phase", func() {
			It("CTRL-CAT-01: should transition to Completed", func() {
				// Given: SignalProcessing in Categorizing phase with classifications
				sp := &signalprocessingv1alpha1.SignalProcessing{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "categorize-sp",
						Namespace:  "default",
						Generation: 1, // K8s increments on create/update
					},
					Spec: signalprocessingv1alpha1.SignalProcessingSpec{
						Signal: signalprocessingv1alpha1.SignalData{
							Fingerprint: "test-fingerprint-categorize",
							Severity: "high",
							TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
								Kind:      "Pod",
								Name:      "test-pod",
								Namespace: "default",
							},
						},
					},
					Status: signalprocessingv1alpha1.SignalProcessingStatus{
						Phase:     signalprocessingv1alpha1.PhaseCategorizing,
						StartTime: &metav1.Time{Time: metav1.Now().Time},
						KubernetesContext: &signalprocessingv1alpha1.KubernetesContext{
							Namespace: &signalprocessingv1alpha1.NamespaceContext{
								Name: "default",
							},
						},
						EnvironmentClassification: &signalprocessingv1alpha1.EnvironmentClassification{
							Environment: signalprocessingv1alpha1.EnvironmentProduction,
						},
						PriorityAssignment: &signalprocessingv1alpha1.PriorityAssignment{
							Priority: signalprocessingv1alpha1.PriorityP1,
						},
					},
				}

				// When: Reconcile is triggered
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(sp).
					WithStatusSubresource(sp).
					Build()

				reconciler := &controller.SignalProcessingReconciler{
					Client:        fakeClient,
					Scheme:        scheme,
					StatusManager: spstatus.NewManager(fakeClient, fakeClient),
					Metrics:       spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
					AuditManager:  spaudit.NewManager(auditClient),
					K8sEnricher:   newDefaultMockK8sEnricher(),
				}

				result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      sp.Name,
						Namespace: sp.Namespace,
					},
				})

				// Then: Should complete (Categorizing → Completed)
				Expect(err).ToNot(HaveOccurred())

				// Verify transition occurred
				updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
				err = fakeClient.Get(context.Background(), types.NamespacedName{
					Name:      sp.Name,
					Namespace: sp.Namespace,
				}, updatedSP)
				Expect(err).ToNot(HaveOccurred())

				// Should transition or continue processing
				Expect(result.RequeueAfter != 0 || updatedSP.Status.Phase == signalprocessingv1alpha1.PhaseCompleted).To(BeTrue())
			})
		})
	})

	// Detection Methods (BR-SP-101) removed - ADR-056: DetectedLabels relocated to HAPI
	// See: holmesgpt-api/src/detection/labels.py for post-RCA label detection
	// BR-SP-101 tests relocated to: holmesgpt-api/tests/unit/test_label_detector.py
	//
	// The following detection tests were removed as part of ADR-056 cleanup:
	// - CTRL-DETECT-01 through CTRL-DETECT-09 (hasPDB, hasHPA, NetworkPolicy, IsProduction, GitOps)
	// All detection is now performed post-RCA in HAPI.

	// ========================================
	// ObservedGeneration Tracking (DD-CONTROLLER-001)
	// Verifies ObservedGeneration is persisted on terminal phase transitions.
	// Issue #118: E2E validator expects ObservedGeneration > 0 for completed SPs.
	// Bug: assignment at line 773 was outside AtomicStatusUpdate callback,
	// lost during refetch.
	// ========================================
	Describe("ObservedGeneration tracking (DD-CONTROLLER-001)", func() {
		var (
			mockStore   *mockAuditStore
			auditClient *spaudit.AuditClient
		)

		BeforeEach(func() {
			mockStore = &mockAuditStore{}
			auditClient = spaudit.NewAuditClient(mockStore, logr.Discard())
		})

		Context("when SP reaches PhaseCompleted via reconcileCategorizing", func() {
			It("UT-SP-OG-001: should persist ObservedGeneration > 0 through AtomicStatusUpdate", func() {
				// Given: SP in Categorizing phase, ready to transition to Completed
				sp := &signalprocessingv1alpha1.SignalProcessing{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "og-completed-sp",
						Namespace:  "default",
						Generation: 2,
					},
					Spec: signalprocessingv1alpha1.SignalProcessingSpec{
						Signal: signalprocessingv1alpha1.SignalData{
							Fingerprint: "og-test-fingerprint",
							Severity:    "high",
							TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
								Kind:      "Pod",
								Name:      "test-pod",
								Namespace: "default",
							},
						},
					},
					Status: signalprocessingv1alpha1.SignalProcessingStatus{
						Phase:     signalprocessingv1alpha1.PhaseCategorizing,
						StartTime: &metav1.Time{Time: metav1.Now().Time},
						KubernetesContext: &signalprocessingv1alpha1.KubernetesContext{
							Namespace: &signalprocessingv1alpha1.NamespaceContext{
								Name: "default",
							},
						},
						EnvironmentClassification: &signalprocessingv1alpha1.EnvironmentClassification{
							Environment: signalprocessingv1alpha1.EnvironmentProduction,
						},
						PriorityAssignment: &signalprocessingv1alpha1.PriorityAssignment{
							Priority: signalprocessingv1alpha1.PriorityP1,
						},
					},
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(sp).
					WithStatusSubresource(sp).
					Build()

				reconciler := &controller.SignalProcessingReconciler{
					Client:        fakeClient,
					Scheme:        scheme,
					StatusManager: spstatus.NewManager(fakeClient, fakeClient),
					Metrics:       spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
					AuditManager:  spaudit.NewManager(auditClient),
					K8sEnricher:   newDefaultMockK8sEnricher(),
				}

				// When: Reconcile triggers Categorizing → Completed
				_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      sp.Name,
						Namespace: sp.Namespace,
					},
				})
				Expect(err).ToNot(HaveOccurred())

				// Then: ObservedGeneration must be persisted (survives AtomicStatusUpdate refetch)
				updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
				err = fakeClient.Get(context.Background(), types.NamespacedName{
					Name:      sp.Name,
					Namespace: sp.Namespace,
				}, updatedSP)
				Expect(err).ToNot(HaveOccurred())
				Expect(updatedSP.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))
				Expect(updatedSP.Status.ObservedGeneration).To(Equal(int64(2)),
					"ObservedGeneration must be persisted through AtomicStatusUpdate to match Generation")
			})
		})
	})

	// ========================================
	// SP-CACHE-002: Informer cache staleness regression test
	// Reproduces the E2E bug where reconcileClassifying reads stale sp from the
	// informer cache (missing KubernetesContext), causing environment=unknown.
	// ========================================
	Describe("SP-CACHE-002: Classifying with stale informer cache", func() {
		var (
			mockStore   *mockAuditStore
			auditClient *spaudit.AuditClient
		)

		BeforeEach(func() {
			mockStore = &mockAuditStore{}
			auditClient = spaudit.NewAuditClient(mockStore, logr.Discard())
		})

		It("should classify environment correctly even when informer cache lacks KubernetesContext", func() {
			// This test reproduces the exact bug observed in E2E:
			// 1. Enriching phase writes KubernetesContext + Phase=Classifying to API server
			// 2. Informer cache syncs Phase=Classifying but NOT KubernetesContext (stale)
			// 3. reconcileClassifying reads k8sCtx from stale sp → nil → environment=unknown
			//
			// We simulate this by using two fake clients:
			// - cacheClient: the reconciler's Client (r.Get) — has Phase=Classifying but NO KubernetesContext
			// - apiClient:   the StatusManager's apiReader — has the FULL status with KubernetesContext

			enrichedK8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "production-ns",
					Labels: map[string]string{
						"kubernaut.ai/environment": "production",
						"kubernaut.ai/managed":     "true",
					},
				},
				DegradedMode: true, // Pod not found → degraded mode (fast path)
			}

			// The "stale cache" SP: has Phase=Classifying but KubernetesContext is nil.
			// This is what the informer cache returns before it syncs the enrichment data.
			staleSP := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "sp-cache-stale",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "cache-stale-fp-001",
						Name:        "HighCPU",
						Severity:    "critical",
						Type:        "alert",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "api-server-xyz",
							Namespace: "production-ns",
						},
					},
				},
				Status: signalprocessingv1alpha1.SignalProcessingStatus{
					Phase:     signalprocessingv1alpha1.PhaseClassifying,
					StartTime: &metav1.Time{Time: metav1.Now().Time},
					// KubernetesContext is deliberately nil — this is the stale cache state
				},
			}

			// The "fresh API" SP: has Phase=Classifying AND KubernetesContext populated.
			// This is the ground truth on the API server after the enriching phase wrote it.
			freshSP := staleSP.DeepCopy()
			freshSP.Status.KubernetesContext = enrichedK8sCtx

			// Build two fake clients to simulate informer cache vs API server divergence
			cacheClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(staleSP).
				WithStatusSubresource(staleSP).
				Build()

			apiClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(freshSP).
				WithStatusSubresource(freshSP).
				Build()

			// Track what the PolicyEvaluator receives to verify it got the labels
			var capturedEnvironmentInput *evaluator.PolicyInput
			policyEval := &mockPolicyEvaluator{
				EvaluateEnvironmentFunc: func(_ context.Context, input evaluator.PolicyInput) (*signalprocessingv1alpha1.EnvironmentClassification, error) {
					capturedEnvironmentInput = &input
					// Return classification based on actual namespace labels (like the real Rego policy)
					if env, ok := input.Namespace.Labels["kubernaut.ai/environment"]; ok && env != "" {
						var classified signalprocessingv1alpha1.Environment
						switch env {
						case "production":
							classified = signalprocessingv1alpha1.EnvironmentProduction
						case "staging":
							classified = signalprocessingv1alpha1.EnvironmentStaging
						case "development":
							classified = signalprocessingv1alpha1.EnvironmentDevelopment
						case "test":
							classified = signalprocessingv1alpha1.EnvironmentTest
						default:
							classified = signalprocessingv1alpha1.Environment(env)
						}
						return &signalprocessingv1alpha1.EnvironmentClassification{
							Environment:  classified,
							Source:       "namespace-labels",
							ClassifiedAt: metav1.Now(),
						}, nil
					}
					return &signalprocessingv1alpha1.EnvironmentClassification{
						Environment:  signalprocessingv1alpha1.EnvironmentUnknown,
						Source:       "default",
						ClassifiedAt: metav1.Now(),
					}, nil
				},
			}

			reconciler := &controller.SignalProcessingReconciler{
				Client:          cacheClient, // Simulates informer cache (stale)
				Scheme:          scheme,
				StatusManager:   spstatus.NewManager(apiClient, apiClient), // apiReader is fresh
				Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				AuditManager:    spaudit.NewManager(auditClient),
				PolicyEvaluator: policyEval,
				Recorder:        record.NewFakeRecorder(20),
			}

			_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      staleSP.Name,
					Namespace: staleSP.Namespace,
				},
			})
			Expect(err).ToNot(HaveOccurred())

			// The PolicyEvaluator MUST have received the production label from the
			// fresh API data, not from the stale cache data.
			Expect(capturedEnvironmentInput).ToNot(BeNil(),
				"PolicyEvaluator.EvaluateEnvironment should have been called")
			Expect(capturedEnvironmentInput.Namespace.Labels).To(HaveKeyWithValue(
				"kubernaut.ai/environment", "production"),
				"SP-CACHE-002: Classifier must use fresh namespace labels from API, not stale cache")

			// Verify the classification result written to the API server is correct
			resultSP := &signalprocessingv1alpha1.SignalProcessing{}
			err = apiClient.Get(context.Background(), types.NamespacedName{
				Name:      staleSP.Name,
				Namespace: staleSP.Namespace,
			}, resultSP)
			Expect(err).ToNot(HaveOccurred())
			Expect(resultSP.Status.EnvironmentClassification.Environment).To(Equal(signalprocessingv1alpha1.EnvironmentProduction),
				"SP-CACHE-002: Environment must be Production, not unknown from stale cache")
		})
	})

	// ========================================
	// Interface Compliance Tests
	// Verifies controller implements required interfaces
	// ========================================
	Describe("Interface Compliance", func() {
		It("CTRL-IFACE-01: should implement controller-runtime Reconciler interface", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			reconciler := &controller.SignalProcessingReconciler{
				Client:        fakeClient,
				Scheme:        scheme,
				StatusManager: spstatus.NewManager(fakeClient, fakeClient),
			}

			// Compile-time interface check
			var _ reconcile.Reconciler = reconciler
			Expect(reconciler).To(Not(BeNil()), "reconciler must be constructible")
		})

		It("CTRL-IFACE-02: should have SetupWithManager method", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			reconciler := &controller.SignalProcessingReconciler{
				Client:        fakeClient,
				Scheme:        scheme,
				StatusManager: spstatus.NewManager(fakeClient, fakeClient),
			}

			// Verify method exists
			Expect(reconciler.SetupWithManager).To(Not(BeNil()), "SetupWithManager method must be defined")
		})
	})
})
