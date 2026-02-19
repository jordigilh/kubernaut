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

// Package signalprocessing contains unit tests for Signal Processing controller.
//
// DD-EVENT-001 v1.1: K8s Event Observability for SignalProcessing Controller
// BR-SP-095: All SignalProcessing lifecycle events must be emitted via Recorder.Event
package signalprocessing

import (
	"context"

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
	spaudit "github.com/jordigilh/kubernaut/pkg/signalprocessing/audit"
	spmetrics "github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
	spstatus "github.com/jordigilh/kubernaut/pkg/signalprocessing/status"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
	"github.com/prometheus/client_golang/prometheus"
)

// drainEvents reads all available events from the FakeRecorder channel.
func drainEvents(recorder *record.FakeRecorder) []string {
	var collected []string
	for {
		select {
		case evt := <-recorder.Events:
			collected = append(collected, evt)
		default:
			return collected
		}
	}
}

// containsEvent checks if any event string contains ALL the given substrings.
func containsEvent(eventList []string, substrings ...string) bool {
	for _, evt := range eventList {
		allMatch := true
		for _, sub := range substrings {
			if !containsSubstr(evt, sub) {
				allMatch = false
				break
			}
		}
		if allMatch {
			return true
		}
	}
	return false
}

func containsSubstr(s, substr string) bool {
	return len(s) >= len(substr) && findSubstr(s, substr)
}

func findSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// DD-EVENT-001 v1.1: K8s Event Observability for SignalProcessing Controller
var _ = Describe("SignalProcessing Controller K8s Events [DD-EVENT-001]", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(signalprocessingv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
	})

	// UT-SP-EVT-01: PhaseTransition event on Pending → Enriching
	Context("UT-SP-EVT-01: PhaseTransition event on Pending → Enriching", func() {
		It("should emit PhaseTransition Normal event when transitioning Pending to Enriching", func() {
			recorder := record.NewFakeRecorder(20)
			mockStore := &mockAuditStore{}
			auditClient := spaudit.NewAuditClient(mockStore, logr.Discard())

			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-sp-evt-01",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "test-fp-01",
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

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(sp).
				WithStatusSubresource(sp).
				Build()

			reconciler := &controller.SignalProcessingReconciler{
				Client:        fakeClient,
				Scheme:       scheme,
				Recorder:     recorder,
				StatusManager: spstatus.NewManager(fakeClient, fakeClient),
				Metrics:      spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				AuditClient:  auditClient,
				AuditManager: spaudit.NewManager(auditClient),
				K8sEnricher:  newDefaultMockK8sEnricher(),
			}

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "test-sp-evt-01", Namespace: "default"},
			}

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			evts := drainEvents(recorder)
			Expect(containsEvent(evts, "Normal", events.EventReasonPhaseTransition, "Pending", "Enriching")).
				To(BeTrue(), "Expected PhaseTransition event with Pending→Enriching, got: %v", evts)
		})
	})

	// UT-SP-EVT-02: SignalEnriched event on Enriching → Classifying
	Context("UT-SP-EVT-02: SignalEnriched event on Enriching → Classifying", func() {
		It("should emit SignalEnriched Normal event when K8s context enrichment completes", func() {
			recorder := record.NewFakeRecorder(20)
			mockStore := &mockAuditStore{}
			auditClient := spaudit.NewAuditClient(mockStore, logr.Discard())

			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-namespace",
					Labels: map[string]string{"env": "production"},
				},
			}

			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-sp-evt-02",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "test-fp-02",
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

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(ns, sp).
				WithStatusSubresource(sp).
				Build()

			reconciler := &controller.SignalProcessingReconciler{
				Client:        fakeClient,
				Scheme:       scheme,
				Recorder:     recorder,
				StatusManager: spstatus.NewManager(fakeClient, fakeClient),
				Metrics:      spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				AuditClient:  auditClient,
				AuditManager: spaudit.NewManager(auditClient),
				K8sEnricher:  newMockK8sEnricherWithClient(fakeClient),
			}

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "test-sp-evt-02", Namespace: "default"},
			}

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			evts := drainEvents(recorder)
			Expect(containsEvent(evts, "Normal", events.EventReasonSignalEnriched)).
				To(BeTrue(), "Expected SignalEnriched event, got: %v", evts)
			Expect(containsEvent(evts, "Normal", events.EventReasonPhaseTransition, "Enriching", "Classifying")).
				To(BeTrue(), "Expected PhaseTransition Enriching→Classifying, got: %v", evts)
		})
	})

	// UT-SP-EVT-03: EnrichmentDegraded Warning event
	Context("UT-SP-EVT-03: EnrichmentDegraded event on degraded enrichment", func() {
		It("should emit EnrichmentDegraded Warning event when enrichment returns partial/degraded results", func() {
			recorder := record.NewFakeRecorder(20)
			mockStore := &mockAuditStore{}
			auditClient := spaudit.NewAuditClient(mockStore, logr.Discard())

			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-sp-evt-03",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "test-fp-03",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "missing-pod",
							Namespace: "default",
						},
					},
				},
				Status: signalprocessingv1alpha1.SignalProcessingStatus{
					Phase:     signalprocessingv1alpha1.PhaseEnriching,
					StartTime: &metav1.Time{Time: metav1.Now().Time},
				},
			}

			// Mock enricher that returns degraded context (e.g., namespace not found)
			degradedEnricher := &mockK8sEnricher{
				EnrichFunc: func(_ context.Context, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.KubernetesContext, error) {
					return &signalprocessingv1alpha1.KubernetesContext{
						Namespace: &signalprocessingv1alpha1.NamespaceContext{
							Name:   signal.TargetResource.Namespace,
							Labels: signal.Labels,
						},
						DegradedMode: true,
					}, nil
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(sp).
				WithStatusSubresource(sp).
				Build()

			reconciler := &controller.SignalProcessingReconciler{
				Client:        fakeClient,
				Scheme:       scheme,
				Recorder:     recorder,
				StatusManager: spstatus.NewManager(fakeClient, fakeClient),
				Metrics:      spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				AuditClient:  auditClient,
				AuditManager: spaudit.NewManager(auditClient),
				K8sEnricher:  degradedEnricher,
			}

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "test-sp-evt-03", Namespace: "default"},
			}

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			evts := drainEvents(recorder)
			Expect(containsEvent(evts, "Warning", events.EventReasonEnrichmentDegraded)).
				To(BeTrue(), "Expected EnrichmentDegraded Warning event, got: %v", evts)
		})
	})

	// UT-SP-EVT-04: SignalProcessed event on Categorizing → Completed
	Context("UT-SP-EVT-04: SignalProcessed event on completion", func() {
		It("should emit SignalProcessed Normal event when signal enrichment and classification complete successfully", func() {
			recorder := record.NewFakeRecorder(20)
			mockStore := &mockAuditStore{}
			auditClient := spaudit.NewAuditClient(mockStore, logr.Discard())

			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-sp-evt-04",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "test-fp-04",
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
							Name:   "default",
						Labels: map[string]string{"env": "production"},
					},
				},
					EnvironmentClassification: &signalprocessingv1alpha1.EnvironmentClassification{
						Environment: "production",
						Source:      "mock",
					},
					PriorityAssignment: &signalprocessingv1alpha1.PriorityAssignment{
						Priority: "P1",
						Source:   "mock",
					},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(sp).
				WithStatusSubresource(sp).
				Build()

			reconciler := &controller.SignalProcessingReconciler{
				Client:           fakeClient,
				Scheme:          scheme,
				Recorder:        recorder,
				StatusManager:   spstatus.NewManager(fakeClient, fakeClient),
				Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				AuditClient:     auditClient,
				AuditManager:    spaudit.NewManager(auditClient),
				EnvClassifier:   newDefaultMockEnvironmentClassifier(),
				PriorityAssigner: newDefaultMockPriorityAssigner(),
			}

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "test-sp-evt-04", Namespace: "default"},
			}

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			evts := drainEvents(recorder)
			Expect(containsEvent(evts, "Normal", events.EventReasonSignalProcessed)).
				To(BeTrue(), "Expected SignalProcessed event, got: %v", evts)
			Expect(containsEvent(evts, "Normal", events.EventReasonPhaseTransition, "Categorizing", "Completed")).
				To(BeTrue(), "Expected PhaseTransition Categorizing→Completed, got: %v", evts)
		})
	})

	// UT-SP-EVT-05: Event constant verification
	Context("UT-SP-EVT-05: Event constant verification", func() {
		It("should have correct event reason constants for DD-EVENT-001", func() {
			Expect(events.EventReasonPolicyEvaluationFailed).To(Equal("PolicyEvaluationFailed"))
			Expect(events.EventReasonSignalProcessed).To(Equal("SignalProcessed"))
			Expect(events.EventReasonSignalEnriched).To(Equal("SignalEnriched"))
			Expect(events.EventReasonEnrichmentDegraded).To(Equal("EnrichmentDegraded"))
			Expect(events.EventReasonPhaseTransition).To(Equal("PhaseTransition"))
		})
	})
})
