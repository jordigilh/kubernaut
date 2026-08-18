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

// Package signalprocessing_test: DD-TIMEOUT-002 / Issue #2176 unit tests for
// SignalProcessing's Reconcile-level self-enforcement of Spec.TimesOutAt,
// RO's authoritative absolute deadline propagated from
// RemediationRequest.Status.TimeoutConfig.Processing.
package signalprocessing_test

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/signalprocessing"
	spconditions "github.com/jordigilh/kubernaut/pkg/signalprocessing"
	spaudit "github.com/jordigilh/kubernaut/pkg/signalprocessing/audit"
	spmetrics "github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
	spstatus "github.com/jordigilh/kubernaut/pkg/signalprocessing/status"
	"github.com/prometheus/client_golang/prometheus"
)

// buildTimeoutTestSP builds a SignalProcessing already mid-flight in the
// Enriching phase (RemediationRequestRef set so the audit path doesn't
// short-circuit), with the given Spec.TimesOutAt.
func buildTimeoutTestSP(name string, timesOutAt *metav1.Time) *signalprocessingv1alpha1.SignalProcessing {
	return buildTimeoutTestSPWithPhase(name, timesOutAt, signalprocessingv1alpha1.PhaseEnriching)
}

// buildTimeoutTestSPWithPhase is buildTimeoutTestSP with an explicit
// Status.Phase, so setActivePhaseConditionFalse's per-phase switch (DD-SP-002)
// can be exercised for each active phase (Enriching/Classifying/Categorizing)
// as well as the no-op group (Pending/Completed/Failed).
func buildTimeoutTestSPWithPhase(name string, timesOutAt *metav1.Time, phase signalprocessingv1alpha1.SignalProcessingPhase) *signalprocessingv1alpha1.SignalProcessing {
	return &signalprocessingv1alpha1.SignalProcessing{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  "default",
			Generation: 1,
		},
		Spec: signalprocessingv1alpha1.SignalProcessingSpec{
			RemediationRequestRef: signalprocessingv1alpha1.ObjectReference{
				Name:      name + "-rr",
				Namespace: "default",
			},
			Signal: signalprocessingv1alpha1.SignalData{
				Fingerprint: "test-fingerprint-2176",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: "default",
				},
			},
			// DD-TIMEOUT-002 / Issue #2176: propagated by RO from
			// Status.TimeoutConfig.Processing.
			TimesOutAt: timesOutAt,
		},
		Status: signalprocessingv1alpha1.SignalProcessingStatus{
			Phase:     phase,
			StartTime: &metav1.Time{Time: time.Now().Add(-1 * time.Minute)},
		},
	}
}

var _ = Describe("DD-TIMEOUT-002: SignalProcessing self-enforces Spec.TimesOutAt", func() {
	var scheme *runtime.Scheme

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		_ = signalprocessingv1alpha1.AddToScheme(scheme)
		_ = corev1.AddToScheme(scheme)
	})

	It("UT-SP-2176-001: fails the SignalProcessing when Spec.TimesOutAt has already passed", func() {
		pastDeadline := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		sp := buildTimeoutTestSP("sp-timeout-past", &pastDeadline)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(sp).
			WithStatusSubresource(sp).
			Build()

		mockStore := &mockAuditStore{}
		auditClient := spaudit.NewAuditClient(mockStore, logr.Discard())
		recorder := record.NewFakeRecorder(10)

		reconciler := &controller.SignalProcessingReconciler{
			Client:          fakeClient,
			Scheme:          scheme,
			StatusManager:   spstatus.NewManager(fakeClient, fakeClient),
			Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			AuditManager:    spaudit.NewManager(auditClient),
			Recorder:        recorder,
			K8sEnricher:     newDefaultMockK8sEnricher(),
			PolicyEvaluator: newDefaultMockPolicyEvaluator(),
		}

		_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
			NamespacedName: types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace},
		})
		Expect(err).ToNot(HaveOccurred())

		updated := &signalprocessingv1alpha1.SignalProcessing{}
		Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace}, updated)).To(Succeed())
		Expect(updated.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseFailed),
			"DD-TIMEOUT-002: an already-past Spec.TimesOutAt must fail SP regardless of which active phase it's in")

		processingComplete := spconditions.GetCondition(updated, spconditions.ConditionProcessingComplete)
		Expect(processingComplete).ToNot(BeNil())
		Expect(processingComplete.Status).To(Equal(metav1.ConditionFalse))

		Expect(mockStore.events).ToNot(BeEmpty(),
			"BR-AUDIT-005 / AU-2/AU-3: a self-enforced timeout must emit a failure audit event")

		Eventually(recorder.Events).Should(Receive(ContainSubstring("imed out")))
	})

	It("UT-SP-2176-002: leaves phase untouched (proceeds normally) when Spec.TimesOutAt is nil", func() {
		sp := buildTimeoutTestSP("sp-timeout-nil", nil)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(sp).
			WithStatusSubresource(sp).
			Build()

		mockStore := &mockAuditStore{}
		auditClient := spaudit.NewAuditClient(mockStore, logr.Discard())

		reconciler := &controller.SignalProcessingReconciler{
			Client:          fakeClient,
			Scheme:          scheme,
			StatusManager:   spstatus.NewManager(fakeClient, fakeClient),
			Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			AuditManager:    spaudit.NewManager(auditClient),
			K8sEnricher:     newDefaultMockK8sEnricher(),
			PolicyEvaluator: newDefaultMockPolicyEvaluator(),
		}

		_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
			NamespacedName: types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace},
		})
		Expect(err).ToNot(HaveOccurred())

		updated := &signalprocessingv1alpha1.SignalProcessing{}
		Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace}, updated)).To(Succeed())
		Expect(updated.Status.Phase).ToNot(Equal(signalprocessingv1alpha1.PhaseFailed),
			"no authoritative Processing timeout is available, so SP must rely solely on RO's outer backstop")
	})

	It("UT-SP-2176-003: leaves phase untouched (proceeds normally) when Spec.TimesOutAt has not yet passed", func() {
		futureDeadline := metav1.NewTime(time.Now().Add(1 * time.Hour))
		sp := buildTimeoutTestSP("sp-timeout-future", &futureDeadline)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(sp).
			WithStatusSubresource(sp).
			Build()

		mockStore := &mockAuditStore{}
		auditClient := spaudit.NewAuditClient(mockStore, logr.Discard())

		reconciler := &controller.SignalProcessingReconciler{
			Client:          fakeClient,
			Scheme:          scheme,
			StatusManager:   spstatus.NewManager(fakeClient, fakeClient),
			Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			AuditManager:    spaudit.NewManager(auditClient),
			K8sEnricher:     newDefaultMockK8sEnricher(),
			PolicyEvaluator: newDefaultMockPolicyEvaluator(),
		}

		_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
			NamespacedName: types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace},
		})
		Expect(err).ToNot(HaveOccurred())

		updated := &signalprocessingv1alpha1.SignalProcessing{}
		Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace}, updated)).To(Succeed())
		Expect(updated.Status.Phase).ToNot(Equal(signalprocessingv1alpha1.PhaseFailed),
			"DD-TIMEOUT-002: a future Spec.TimesOutAt must not fail SP prematurely")
	})

	// DD-SP-002: setActivePhaseConditionFalse mirrors the active phase into a
	// dedicated phase-specific condition so `kubectl describe` shows exactly
	// which phase was in flight when RO's deadline hit. Each active phase
	// (Enriching/Classifying/Categorizing) is its own switch branch.
	DescribeTable("UT-SP-2176-004: sets the phase-specific condition matching whichever phase was active at timeout",
		func(phase signalprocessingv1alpha1.SignalProcessingPhase, condition string) {
			pastDeadline := metav1.NewTime(time.Now().Add(-1 * time.Minute))
			sp := buildTimeoutTestSPWithPhase("sp-timeout-phase-"+string(phase), &pastDeadline, phase)

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(sp).
				WithStatusSubresource(sp).
				Build()

			mockStore := &mockAuditStore{}
			auditClient := spaudit.NewAuditClient(mockStore, logr.Discard())

			reconciler := &controller.SignalProcessingReconciler{
				Client:          fakeClient,
				Scheme:          scheme,
				StatusManager:   spstatus.NewManager(fakeClient, fakeClient),
				Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				AuditManager:    spaudit.NewManager(auditClient),
				K8sEnricher:     newDefaultMockK8sEnricher(),
				PolicyEvaluator: newDefaultMockPolicyEvaluator(),
			}

			_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
				NamespacedName: types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace},
			})
			Expect(err).ToNot(HaveOccurred())

			updated := &signalprocessingv1alpha1.SignalProcessing{}
			Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace}, updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseFailed))

			phaseCondition := spconditions.GetCondition(updated, condition)
			Expect(phaseCondition).ToNot(BeNil(), "phase %s must set its own %s condition on timeout", phase, condition)
			Expect(phaseCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(phaseCondition.Reason).To(Equal(spconditions.ReasonTimedOut))
		},
		Entry("Enriching -> EnrichmentComplete", signalprocessingv1alpha1.PhaseEnriching, spconditions.ConditionEnrichmentComplete),
		Entry("Classifying -> ClassificationComplete", signalprocessingv1alpha1.PhaseClassifying, spconditions.ConditionClassificationComplete),
		Entry("Categorizing -> CategorizationComplete", signalprocessingv1alpha1.PhaseCategorizing, spconditions.ConditionCategorizationComplete),
	)

	It("UT-SP-2176-005: a timed-out Pending SignalProcessing fails without setting any phase-specific condition", func() {
		// Pending precedes any active phase (DD-SP-002): if RO's deadline
		// already passed before SP even started enriching, there is no
		// phase-specific condition to set, only ProcessingComplete/Ready.
		pastDeadline := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		sp := buildTimeoutTestSPWithPhase("sp-timeout-pending", &pastDeadline, signalprocessingv1alpha1.PhasePending)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(sp).
			WithStatusSubresource(sp).
			Build()

		mockStore := &mockAuditStore{}
		auditClient := spaudit.NewAuditClient(mockStore, logr.Discard())

		reconciler := &controller.SignalProcessingReconciler{
			Client:          fakeClient,
			Scheme:          scheme,
			StatusManager:   spstatus.NewManager(fakeClient, fakeClient),
			Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			AuditManager:    spaudit.NewManager(auditClient),
			K8sEnricher:     newDefaultMockK8sEnricher(),
			PolicyEvaluator: newDefaultMockPolicyEvaluator(),
		}

		_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
			NamespacedName: types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace},
		})
		Expect(err).ToNot(HaveOccurred())

		updated := &signalprocessingv1alpha1.SignalProcessing{}
		Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace}, updated)).To(Succeed())
		Expect(updated.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseFailed))

		Expect(spconditions.GetCondition(updated, spconditions.ConditionEnrichmentComplete)).To(BeNil())
		Expect(spconditions.GetCondition(updated, spconditions.ConditionClassificationComplete)).To(BeNil())
		Expect(spconditions.GetCondition(updated, spconditions.ConditionCategorizationComplete)).To(BeNil())

		processingComplete := spconditions.GetCondition(updated, spconditions.ConditionProcessingComplete)
		Expect(processingComplete).ToNot(BeNil())
		Expect(processingComplete.Status).To(Equal(metav1.ConditionFalse))
	})

	It("UT-SP-2176-006: propagates the error when the status update itself fails", func() {
		// failOnTimeout's AtomicStatusUpdate error path (distinct from the
		// audit fail-open path below): a failed status write is NOT
		// fail-open, since the caller (checkTerminalOrTimeout -> Reconcile)
		// needs to know the Failed phase never actually got persisted, so
		// controller-runtime retries the reconcile.
		pastDeadline := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		sp := buildTimeoutTestSP("sp-timeout-status-update-err", &pastDeadline)

		statusUpdateErr := fmt.Errorf("simulated etcd write failure")
		errFuncs := interceptor.Funcs{
			SubResourceUpdate: func(ctx context.Context, c client.Client, subResourceName string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				if subResourceName == "status" {
					return statusUpdateErr
				}
				return c.SubResource(subResourceName).Update(ctx, obj, opts...)
			},
		}
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(sp).
			WithStatusSubresource(sp).
			WithInterceptorFuncs(errFuncs).
			Build()

		mockStore := &mockAuditStore{}
		auditClient := spaudit.NewAuditClient(mockStore, logr.Discard())

		reconciler := &controller.SignalProcessingReconciler{
			Client:          fakeClient,
			Scheme:          scheme,
			StatusManager:   spstatus.NewManager(fakeClient, fakeClient),
			Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			AuditManager:    spaudit.NewManager(auditClient),
			K8sEnricher:     newDefaultMockK8sEnricher(),
			PolicyEvaluator: newDefaultMockPolicyEvaluator(),
		}

		_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
			NamespacedName: types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace},
		})
		Expect(err).To(HaveOccurred(), "a failed status write must propagate so controller-runtime retries the reconcile")
		Expect(err.Error()).To(ContainSubstring("simulated etcd write failure"))
	})

	It("UT-SP-2176-007: still fails the SignalProcessing (fail-open) when AuditManager.RecordError itself errors", func() {
		// Distinct from UT-SP-2176-006: an audit-recording failure must NOT
		// block the timeout failure itself from being applied and returned
		// as success -- BR-AUDIT-005 requires the *attempt* (ADR-032), but
		// audit unavailability must not prevent the underlying safety action.
		// spaudit.Manager.RecordError only returns a non-nil error when its
		// own AuditClient is nil (ADR-032 mandatory-audit guard) -- the one
		// reachable way to force that branch without an unused error return
		// from the fire-and-forget AuditClient.RecordError itself.
		pastDeadline := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		sp := buildTimeoutTestSP("sp-timeout-audit-fail-open", &pastDeadline)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(sp).
			WithStatusSubresource(sp).
			Build()

		reconciler := &controller.SignalProcessingReconciler{
			Client:          fakeClient,
			Scheme:          scheme,
			StatusManager:   spstatus.NewManager(fakeClient, fakeClient),
			Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			AuditManager:    spaudit.NewManager(nil),
			K8sEnricher:     newDefaultMockK8sEnricher(),
			PolicyEvaluator: newDefaultMockPolicyEvaluator(),
		}

		_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
			NamespacedName: types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace},
		})
		Expect(err).ToNot(HaveOccurred(),
			"a failing audit record must not surface as a reconcile error (fail-open safety)")

		updated := &signalprocessingv1alpha1.SignalProcessing{}
		Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace}, updated)).To(Succeed())
		Expect(updated.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseFailed),
			"the timeout failure itself must still be applied even when its audit record fails")
	})
})
