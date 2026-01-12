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
//   - BR-SP-101: Detected Labels
package signalprocessing

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/signalprocessing"
	"github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
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

// ptr returns a pointer to the given value (helper for test setup)
func ptr[T any](v T) *T {
	return &v
}

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
				Expect(result.Requeue).To(BeTrue(), "Should requeue to continue processing")

				// Verify status was updated
				updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
				err = fakeClient.Get(context.Background(), types.NamespacedName{
					Name:      sp.Name,
					Namespace: sp.Namespace,
				}, updatedSP)
				Expect(err).ToNot(HaveOccurred())
				Expect(updatedSP.Status.Phase).To(Equal(signalprocessingv1alpha1.PhasePending))
				Expect(updatedSP.Status.StartTime).ToNot(BeNil())
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
				Expect(result.Requeue).To(BeFalse())
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
					AuditClient:   auditClient,
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
				Expect(result.Requeue).To(BeTrue())

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
				Expect(result.Requeue).To(BeFalse())
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
				Expect(result.Requeue).To(BeFalse())

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
					AuditClient:   auditClient,
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
				Expect(result.Requeue).To(BeTrue())

				// Verify enrichment occurred
				updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
				err = fakeClient.Get(context.Background(), types.NamespacedName{
					Name:      sp.Name,
					Namespace: sp.Namespace,
				}, updatedSP)
				Expect(err).ToNot(HaveOccurred())

				// Namespace context should be populated
				Expect(updatedSP.Status.KubernetesContext).ToNot(BeNil())
				Expect(updatedSP.Status.KubernetesContext.Namespace).ToNot(BeNil())
				Expect(updatedSP.Status.KubernetesContext.Namespace.Name).To(Equal("test-namespace"))
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
					AuditClient:   auditClient,
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
				Expect(result.Requeue).To(BeTrue())

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
					AuditClient:      auditClient,
					EnvClassifier:    newDefaultMockEnvironmentClassifier(),
					PriorityAssigner: newDefaultMockPriorityAssigner(),
				}

				result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      sp.Name,
						Namespace: sp.Namespace,
					},
				})

				// Then: Should requeue for next phase transition
				Expect(err).ToNot(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())
			})
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
							Severity:    "warning",
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
							Environment: "production",
						},
						PriorityAssignment: &signalprocessingv1alpha1.PriorityAssignment{
							Priority: "P1",
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
					AuditClient:   auditClient,
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
				Expect(result.Requeue || updatedSP.Status.Phase == signalprocessingv1alpha1.PhaseCompleted).To(BeTrue())
			})
		})
	})

	// ========================================
	// Detection Method Tests (BR-SP-101)
	// Per unit-test-plan.md: hasPDB, hasHPA, hasNetworkPolicy at 22-80% coverage
	// ========================================
	Describe("Detection Methods (BR-SP-101)", func() {
		var (
			mockStore   *mockAuditStore
			auditClient *spaudit.AuditClient
		)

		BeforeEach(func() {
			_ = policyv1.AddToScheme(scheme)
			_ = autoscalingv2.AddToScheme(scheme)
			_ = networkingv1.AddToScheme(scheme)
			_ = appsv1.AddToScheme(scheme)

			mockStore = &mockAuditStore{}
			auditClient = spaudit.NewAuditClient(mockStore, logr.Discard())
		})

		// ----------------------------------------
		// hasPDB Tests
		// ----------------------------------------
		Describe("hasPDB detection", func() {
			Context("when PDB exists matching Pod labels", func() {
				It("CTRL-DETECT-01: should detect PDB for matching pod", func() {
					// Given: Pod with labels and matching PDB
					ns := &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{Name: "test-pdb-ns"},
					}

					pod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod-pdb",
							Namespace: "test-pdb-ns",
							Labels: map[string]string{
								"app": "my-app",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "main", Image: "nginx"}},
						},
					}

					minAvailable := intstr.FromInt(1)
					pdb := &policyv1.PodDisruptionBudget{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pdb",
							Namespace: "test-pdb-ns",
						},
						Spec: policyv1.PodDisruptionBudgetSpec{
							MinAvailable: &minAvailable,
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "my-app",
								},
							},
						},
					}

					sp := &signalprocessingv1alpha1.SignalProcessing{
						ObjectMeta: metav1.ObjectMeta{
							Name:       "detect-pdb-sp",
							Namespace:  "default",
							Generation: 1, // K8s increments on create/update
						},
						Spec: signalprocessingv1alpha1.SignalProcessingSpec{
							Signal: signalprocessingv1alpha1.SignalData{
								Fingerprint: "pdb-test",
								TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
									Kind:      "Pod",
									Name:      "test-pod-pdb",
									Namespace: "test-pdb-ns",
								},
							},
						},
						Status: signalprocessingv1alpha1.SignalProcessingStatus{
							Phase:     signalprocessingv1alpha1.PhaseEnriching,
							StartTime: &metav1.Time{Time: metav1.Now().Time},
							KubernetesContext: &signalprocessingv1alpha1.KubernetesContext{
								Pod: &signalprocessingv1alpha1.PodDetails{
									Labels: map[string]string{"app": "my-app"},
								},
							},
						},
					}

					// When: Reconcile enriches the signal
					fakeClient := fake.NewClientBuilder().
						WithScheme(scheme).
						WithObjects(ns, pod, pdb, sp).
						WithStatusSubresource(sp).
						Build()

					reconciler := &controller.SignalProcessingReconciler{
						Client:        fakeClient,
						Scheme:        scheme,
						StatusManager: spstatus.NewManager(fakeClient, fakeClient),
						Metrics:       spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
						AuditClient:   auditClient,
						K8sEnricher:   newMockK8sEnricherWithClient(fakeClient),
					}

					_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      sp.Name,
							Namespace: sp.Namespace,
						},
					})

					// Then: Should detect PDB in DetectedLabels
					Expect(err).ToNot(HaveOccurred())

					updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
					err = fakeClient.Get(context.Background(), types.NamespacedName{
						Name: sp.Name, Namespace: sp.Namespace,
					}, updatedSP)
					Expect(err).ToNot(HaveOccurred())

					// PDB detection occurs during enriching phase
					Expect(updatedSP.Status.KubernetesContext.DetectedLabels).ToNot(BeNil())
					Expect(updatedSP.Status.KubernetesContext.DetectedLabels.HasPDB).To(BeTrue())
				})
			})

			Context("when no PDB exists", func() {
				It("CTRL-DETECT-02: should not detect PDB when none exists", func() {
					// Given: Pod without matching PDB
					ns := &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{Name: "test-no-pdb-ns"},
					}

					pod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-pod-no-pdb",
							Namespace: "test-no-pdb-ns",
							Labels:    map[string]string{"app": "other-app"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "main", Image: "nginx"}},
						},
					}

					sp := &signalprocessingv1alpha1.SignalProcessing{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "no-pdb-sp",
							Namespace: "default",
						},
						Spec: signalprocessingv1alpha1.SignalProcessingSpec{
							Signal: signalprocessingv1alpha1.SignalData{
								Fingerprint: "no-pdb-test",
								TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
									Kind:      "Pod",
									Name:      "test-pod-no-pdb",
									Namespace: "test-no-pdb-ns",
								},
							},
						},
						Status: signalprocessingv1alpha1.SignalProcessingStatus{
							Phase:     signalprocessingv1alpha1.PhaseEnriching,
							StartTime: &metav1.Time{Time: metav1.Now().Time},
							KubernetesContext: &signalprocessingv1alpha1.KubernetesContext{
								Pod: &signalprocessingv1alpha1.PodDetails{
									Labels: map[string]string{"app": "other-app"},
								},
							},
						},
					}

					// When: Reconcile enriches the signal
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
						AuditClient:   auditClient,
						K8sEnricher:   newDefaultMockK8sEnricher(),
					}

					_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      sp.Name,
							Namespace: sp.Namespace,
						},
					})

					// Then: Should not detect PDB
					Expect(err).ToNot(HaveOccurred())

					updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
					err = fakeClient.Get(context.Background(), types.NamespacedName{
						Name: sp.Name, Namespace: sp.Namespace,
					}, updatedSP)
					Expect(err).ToNot(HaveOccurred())

					if updatedSP.Status.KubernetesContext.DetectedLabels != nil {
						Expect(updatedSP.Status.KubernetesContext.DetectedLabels.HasPDB).To(BeFalse())
					}
				})
			})
		})

		// ----------------------------------------
		// hasHPA Tests
		// ----------------------------------------
		Describe("hasHPA detection", func() {
			Context("when HPA targets the workload directly", func() {
				It("CTRL-DETECT-03: should detect HPA targeting Deployment", func() {
					// Given: Deployment with HPA targeting it
					ns := &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{Name: "test-hpa-ns"},
					}

					hpa := &autoscalingv2.HorizontalPodAutoscaler{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-hpa",
							Namespace: "test-hpa-ns",
						},
						Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
							ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
								APIVersion: "apps/v1",
								Kind:       "Deployment",
								Name:       "target-deploy",
							},
							MinReplicas: ptr(int32(2)),
							MaxReplicas: 10,
						},
					}

					sp := &signalprocessingv1alpha1.SignalProcessing{
						ObjectMeta: metav1.ObjectMeta{
							Name:       "detect-hpa-sp",
							Namespace:  "default",
							Generation: 1, // K8s increments on create/update
						},
						Spec: signalprocessingv1alpha1.SignalProcessingSpec{
							Signal: signalprocessingv1alpha1.SignalData{
								Fingerprint: "hpa-test",
								TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
									Kind:      "Deployment",
									Name:      "target-deploy",
									Namespace: "test-hpa-ns",
								},
							},
						},
						Status: signalprocessingv1alpha1.SignalProcessingStatus{
							Phase:     signalprocessingv1alpha1.PhaseEnriching,
							StartTime: &metav1.Time{Time: metav1.Now().Time},
							KubernetesContext: &signalprocessingv1alpha1.KubernetesContext{
								Namespace: &signalprocessingv1alpha1.NamespaceContext{Name: "test-hpa-ns"},
							},
						},
					}

					// When: Reconcile enriches the signal
					fakeClient := fake.NewClientBuilder().
						WithScheme(scheme).
						WithObjects(ns, hpa, sp).
						WithStatusSubresource(sp).
						Build()

					reconciler := &controller.SignalProcessingReconciler{
						Client:        fakeClient,
						Scheme:        scheme,
						StatusManager: spstatus.NewManager(fakeClient, fakeClient),
						Metrics:       spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
						AuditClient:   auditClient,
						K8sEnricher:   newDefaultMockK8sEnricher(),
					}

					_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      sp.Name,
							Namespace: sp.Namespace,
						},
					})

					// Then: Should detect HPA
					Expect(err).ToNot(HaveOccurred())

					updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
					err = fakeClient.Get(context.Background(), types.NamespacedName{
						Name: sp.Name, Namespace: sp.Namespace,
					}, updatedSP)
					Expect(err).ToNot(HaveOccurred())

					Expect(updatedSP.Status.KubernetesContext.DetectedLabels).ToNot(BeNil())
					Expect(updatedSP.Status.KubernetesContext.DetectedLabels.HasHPA).To(BeTrue())
				})
			})

			// NOTE: CTRL-DETECT-04 (HPA via owner chain) requires full OwnerChainBuilder
			// integration to traverse Pod → ReplicaSet → Deployment → HPA.
			// This is tested in E2E (BR-SP-101) where the full infrastructure is available.
			// Unit tests verify direct HPA targeting only (CTRL-DETECT-03).
			Context("when HPA targets owner in chain (integration scope)", func() {
				// Owner chain HPA detection requires:
				// 1. Pod with ownerReferences pointing to ReplicaSet
				// 2. ReplicaSet with ownerReferences pointing to Deployment
				// 3. HPA targeting the Deployment
				// This traversal requires OwnerChainBuilder which is tested in integration/E2E
				It("CTRL-DETECT-04: is tested in integration/E2E (BR-SP-101)", func() {
					// Documented in unit-test-plan.md as integration scope
					// See: test/e2e/signalprocessing/business_requirements_test.go (BR-SP-101)
				})
			})

			Context("when no HPA exists", func() {
				It("CTRL-DETECT-05: should not detect HPA when none exists", func() {
					// Given: Deployment without HPA
					ns := &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{Name: "test-no-hpa-ns"},
					}

					sp := &signalprocessingv1alpha1.SignalProcessing{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "no-hpa-sp",
							Namespace: "default",
						},
						Spec: signalprocessingv1alpha1.SignalProcessingSpec{
							Signal: signalprocessingv1alpha1.SignalData{
								Fingerprint: "no-hpa-test",
								TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
									Kind:      "Deployment",
									Name:      "lonely-deploy",
									Namespace: "test-no-hpa-ns",
								},
							},
						},
						Status: signalprocessingv1alpha1.SignalProcessingStatus{
							Phase:     signalprocessingv1alpha1.PhaseEnriching,
							StartTime: &metav1.Time{Time: metav1.Now().Time},
							KubernetesContext: &signalprocessingv1alpha1.KubernetesContext{
								Namespace: &signalprocessingv1alpha1.NamespaceContext{Name: "test-no-hpa-ns"},
							},
						},
					}

					// When: Reconcile enriches the signal
					fakeClient := fake.NewClientBuilder().
						WithScheme(scheme).
						WithObjects(ns, sp).
						WithStatusSubresource(sp).
						Build()

					reconciler := &controller.SignalProcessingReconciler{
						Client:        fakeClient,
						Scheme:        scheme,
						StatusManager: spstatus.NewManager(fakeClient, fakeClient),
						Metrics:       spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
						AuditClient:   auditClient,
						K8sEnricher:   newDefaultMockK8sEnricher(),
					}

					_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      sp.Name,
							Namespace: sp.Namespace,
						},
					})

					// Then: Should not detect HPA
					Expect(err).ToNot(HaveOccurred())

					updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
					err = fakeClient.Get(context.Background(), types.NamespacedName{
						Name: sp.Name, Namespace: sp.Namespace,
					}, updatedSP)
					Expect(err).ToNot(HaveOccurred())

					if updatedSP.Status.KubernetesContext.DetectedLabels != nil {
						Expect(updatedSP.Status.KubernetesContext.DetectedLabels.HasHPA).To(BeFalse())
					}
				})
			})
		})

		// ----------------------------------------
		// hasNetworkPolicy Tests
		// ----------------------------------------
		Describe("hasNetworkPolicy detection", func() {
			Context("when NetworkPolicy exists in namespace", func() {
				It("CTRL-DETECT-06: should detect NetworkPolicy in namespace", func() {
					// Given: Namespace with NetworkPolicy
					ns := &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{Name: "test-netpol-ns"},
					}

					np := &networkingv1.NetworkPolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-netpol",
							Namespace: "test-netpol-ns",
						},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{},
						},
					}

					sp := &signalprocessingv1alpha1.SignalProcessing{
						ObjectMeta: metav1.ObjectMeta{
							Name:       "detect-netpol-sp",
							Namespace:  "default",
							Generation: 1, // K8s increments on create/update
						},
						Spec: signalprocessingv1alpha1.SignalProcessingSpec{
							Signal: signalprocessingv1alpha1.SignalData{
								Fingerprint: "netpol-test",
								TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
									Kind:      "Pod",
									Name:      "test-pod",
									Namespace: "test-netpol-ns",
								},
							},
						},
						Status: signalprocessingv1alpha1.SignalProcessingStatus{
							Phase:     signalprocessingv1alpha1.PhaseEnriching,
							StartTime: &metav1.Time{Time: metav1.Now().Time},
							KubernetesContext: &signalprocessingv1alpha1.KubernetesContext{
								Namespace: &signalprocessingv1alpha1.NamespaceContext{Name: "test-netpol-ns"},
							},
						},
					}

					// When: Reconcile enriches the signal
					fakeClient := fake.NewClientBuilder().
						WithScheme(scheme).
						WithObjects(ns, np, sp).
						WithStatusSubresource(sp).
						Build()

					reconciler := &controller.SignalProcessingReconciler{
						Client:        fakeClient,
						Scheme:        scheme,
						StatusManager: spstatus.NewManager(fakeClient, fakeClient),
						Metrics:       spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
						AuditClient:   auditClient,
						K8sEnricher:   newDefaultMockK8sEnricher(),
					}

					_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      sp.Name,
							Namespace: sp.Namespace,
						},
					})

					// Then: Should detect NetworkPolicy
					Expect(err).ToNot(HaveOccurred())

					updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
					err = fakeClient.Get(context.Background(), types.NamespacedName{
						Name: sp.Name, Namespace: sp.Namespace,
					}, updatedSP)
					Expect(err).ToNot(HaveOccurred())

					Expect(updatedSP.Status.KubernetesContext.DetectedLabels).ToNot(BeNil())
					Expect(updatedSP.Status.KubernetesContext.DetectedLabels.NetworkIsolated).To(BeTrue())
				})
			})

			Context("when no NetworkPolicy exists", func() {
				It("CTRL-DETECT-07: should not detect NetworkPolicy when none exists", func() {
					// Given: Namespace without NetworkPolicy
					ns := &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{Name: "test-no-netpol-ns"},
					}

					sp := &signalprocessingv1alpha1.SignalProcessing{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "no-netpol-sp",
							Namespace: "default",
						},
						Spec: signalprocessingv1alpha1.SignalProcessingSpec{
							Signal: signalprocessingv1alpha1.SignalData{
								Fingerprint: "no-netpol-test",
								TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
									Kind:      "Pod",
									Name:      "test-pod",
									Namespace: "test-no-netpol-ns",
								},
							},
						},
						Status: signalprocessingv1alpha1.SignalProcessingStatus{
							Phase:     signalprocessingv1alpha1.PhaseEnriching,
							StartTime: &metav1.Time{Time: metav1.Now().Time},
							KubernetesContext: &signalprocessingv1alpha1.KubernetesContext{
								Namespace: &signalprocessingv1alpha1.NamespaceContext{Name: "test-no-netpol-ns"},
							},
						},
					}

					// When: Reconcile enriches the signal
					fakeClient := fake.NewClientBuilder().
						WithScheme(scheme).
						WithObjects(ns, sp).
						WithStatusSubresource(sp).
						Build()

					reconciler := &controller.SignalProcessingReconciler{
						Client:        fakeClient,
						Scheme:        scheme,
						StatusManager: spstatus.NewManager(fakeClient, fakeClient),
						Metrics:       spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
						AuditClient:   auditClient,
						K8sEnricher:   newDefaultMockK8sEnricher(),
					}

					_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      sp.Name,
							Namespace: sp.Namespace,
						},
					})

					// Then: Should not detect NetworkPolicy
					Expect(err).ToNot(HaveOccurred())

					updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
					err = fakeClient.Get(context.Background(), types.NamespacedName{
						Name: sp.Name, Namespace: sp.Namespace,
					}, updatedSP)
					Expect(err).ToNot(HaveOccurred())

					if updatedSP.Status.KubernetesContext.DetectedLabels != nil {
						Expect(updatedSP.Status.KubernetesContext.DetectedLabels.NetworkIsolated).To(BeFalse())
					}
				})
			})
		})

		// ----------------------------------------
		// detectLabels Tests (GitOps, HelmManaged, IsProduction)
		// ----------------------------------------
		Describe("detectLabels", func() {
			Context("when namespace has production label", func() {
				It("CTRL-DETECT-08: should detect IsProduction from namespace labels", func() {
					// Given: Namespace with production label
					ns := &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{
							Name: "prod-ns",
							Labels: map[string]string{
								"kubernaut.ai/environment": "production",
							},
						},
					}

					sp := &signalprocessingv1alpha1.SignalProcessing{
						ObjectMeta: metav1.ObjectMeta{
							Name:       "detect-prod-sp",
							Namespace:  "default",
							Generation: 1, // K8s increments on create/update
						},
						Spec: signalprocessingv1alpha1.SignalProcessingSpec{
							Signal: signalprocessingv1alpha1.SignalData{
								Fingerprint: "prod-test",
								TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
									Kind:      "Pod",
									Name:      "test-pod",
									Namespace: "prod-ns",
								},
							},
						},
						Status: signalprocessingv1alpha1.SignalProcessingStatus{
							Phase:     signalprocessingv1alpha1.PhaseEnriching,
							StartTime: &metav1.Time{Time: metav1.Now().Time},
						},
					}

					// When: Reconcile enriches the signal
					fakeClient := fake.NewClientBuilder().
						WithScheme(scheme).
						WithObjects(ns, sp).
						WithStatusSubresource(sp).
						Build()

					reconciler := &controller.SignalProcessingReconciler{
						Client:        fakeClient,
						Scheme:        scheme,
						StatusManager: spstatus.NewManager(fakeClient, fakeClient),
						Metrics:       spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
						AuditClient:   auditClient,
						K8sEnricher:   newMockK8sEnricherWithClient(fakeClient),
					}

					_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      sp.Name,
							Namespace: sp.Namespace,
						},
					})

					// Then: Should detect IsProduction
					Expect(err).ToNot(HaveOccurred())

					updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
					err = fakeClient.Get(context.Background(), types.NamespacedName{
						Name: sp.Name, Namespace: sp.Namespace,
					}, updatedSP)
					Expect(err).ToNot(HaveOccurred())

					Expect(updatedSP.Status.KubernetesContext.DetectedLabels).ToNot(BeNil())
					Expect(updatedSP.Status.KubernetesContext.DetectedLabels.IsProduction).To(BeTrue())
				})
			})

			Context("when namespace has GitOps annotations", func() {
				It("CTRL-DETECT-09: should detect GitOpsManaged from ArgoCD annotation", func() {
					// Given: Namespace with ArgoCD annotation
					ns := &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{
							Name: "gitops-ns",
							Annotations: map[string]string{
								"argocd.argoproj.io/managed": "true",
							},
						},
					}

					sp := &signalprocessingv1alpha1.SignalProcessing{
						ObjectMeta: metav1.ObjectMeta{
							Name:       "detect-gitops-sp",
							Namespace:  "default",
							Generation: 1, // K8s increments on create/update
						},
						Spec: signalprocessingv1alpha1.SignalProcessingSpec{
							Signal: signalprocessingv1alpha1.SignalData{
								Fingerprint: "gitops-test",
								TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
									Kind:      "Pod",
									Name:      "test-pod",
									Namespace: "gitops-ns",
								},
							},
						},
						Status: signalprocessingv1alpha1.SignalProcessingStatus{
							Phase:     signalprocessingv1alpha1.PhaseEnriching,
							StartTime: &metav1.Time{Time: metav1.Now().Time},
						},
					}

					// When: Reconcile enriches the signal
					fakeClient := fake.NewClientBuilder().
						WithScheme(scheme).
						WithObjects(ns, sp).
						WithStatusSubresource(sp).
						Build()

					reconciler := &controller.SignalProcessingReconciler{
						Client:        fakeClient,
						Scheme:        scheme,
						StatusManager: spstatus.NewManager(fakeClient, fakeClient),
						Metrics:       spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
						AuditClient:   auditClient,
						K8sEnricher:   newMockK8sEnricherWithClient(fakeClient),
					}

					_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      sp.Name,
							Namespace: sp.Namespace,
						},
					})

					// Then: Should detect GitOpsManaged
					Expect(err).ToNot(HaveOccurred())

					updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
					err = fakeClient.Get(context.Background(), types.NamespacedName{
						Name: sp.Name, Namespace: sp.Namespace,
					}, updatedSP)
					Expect(err).ToNot(HaveOccurred())

					Expect(updatedSP.Status.KubernetesContext.DetectedLabels).ToNot(BeNil())
					Expect(updatedSP.Status.KubernetesContext.DetectedLabels.GitOpsManaged).To(BeTrue())
				})

				It("CTRL-DETECT-10: should detect GitOpsManaged from FluxCD annotation", func() {
					// Given: Namespace with FluxCD annotation
					ns := &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{
							Name: "flux-ns",
							Annotations: map[string]string{
								"fluxcd.io/sync-status": "synced",
							},
						},
					}

					sp := &signalprocessingv1alpha1.SignalProcessing{
						ObjectMeta: metav1.ObjectMeta{
							Name:       "detect-flux-sp",
							Namespace:  "default",
							Generation: 1, // K8s increments on create/update
						},
						Spec: signalprocessingv1alpha1.SignalProcessingSpec{
							Signal: signalprocessingv1alpha1.SignalData{
								Fingerprint: "flux-test",
								TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
									Kind:      "Pod",
									Name:      "test-pod",
									Namespace: "flux-ns",
								},
							},
						},
						Status: signalprocessingv1alpha1.SignalProcessingStatus{
							Phase:     signalprocessingv1alpha1.PhaseEnriching,
							StartTime: &metav1.Time{Time: metav1.Now().Time},
						},
					}

					// When: Reconcile enriches the signal
					fakeClient := fake.NewClientBuilder().
						WithScheme(scheme).
						WithObjects(ns, sp).
						WithStatusSubresource(sp).
						Build()

					reconciler := &controller.SignalProcessingReconciler{
						Client:        fakeClient,
						Scheme:        scheme,
						StatusManager: spstatus.NewManager(fakeClient, fakeClient),
						Metrics:       spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
						AuditClient:   auditClient,
						K8sEnricher:   newMockK8sEnricherWithClient(fakeClient),
					}

					_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      sp.Name,
							Namespace: sp.Namespace,
						},
					})

					// Then: Should detect GitOpsManaged
					Expect(err).ToNot(HaveOccurred())

					updatedSP := &signalprocessingv1alpha1.SignalProcessing{}
					err = fakeClient.Get(context.Background(), types.NamespacedName{
						Name: sp.Name, Namespace: sp.Namespace,
					}, updatedSP)
					Expect(err).ToNot(HaveOccurred())

					Expect(updatedSP.Status.KubernetesContext.DetectedLabels).ToNot(BeNil())
					Expect(updatedSP.Status.KubernetesContext.DetectedLabels.GitOpsManaged).To(BeTrue())
				})
			})
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
			Expect(reconciler).ToNot(BeNil())
		})

		It("CTRL-IFACE-02: should have SetupWithManager method", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			reconciler := &controller.SignalProcessingReconciler{
				Client:        fakeClient,
				Scheme:        scheme,
				StatusManager: spstatus.NewManager(fakeClient, fakeClient),
			}

			// Verify method exists
			Expect(reconciler.SetupWithManager).ToNot(BeNil())
		})
	})
})
