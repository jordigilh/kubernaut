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

// Issue #573: EM ADR-EM-001 Implementation Gaps — Integration Tests
//
// IT-EM-573-010: Failed phase (empty correlationID → PhaseFailed)
// IT-EM-573-012: Stabilizing scheduled event timing
// IT-EM-573-013: Partial assessment when WFE started but not completed
// IT-EM-573-014: Full assessment when WFE started AND completed
//
// Tests 010 and 012 use the suite's manager-driven reconciler (envtest).
// Tests 013 and 014 use a locally-created reconciler with a mock DSQuerier
// and an isolated fake client, calling Reconcile() directly without the manager.
package effectivenessmonitor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prometheus "github.com/prometheus/client_golang/prometheus"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/effectivenessmonitor"
	emclient "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/client"
	emmetrics "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/metrics"
)

var _ = Describe("Issue #573: ADR-EM-001 Implementation Gaps", func() {

	// ========================================================================
	// IT-EM-573-010: CRD-level defense — empty correlationID rejected at admission
	//
	// The CRD schema has MinLength=1 on spec.correlationID, so the API server
	// rejects the create before the reconciler ever sees the EA. This test
	// validates that defense-in-depth works at the CRD layer.
	// The reconciler's validateEASpec guard is covered by unit tests
	// (UT-EM-573-001/002).
	// ========================================================================
	Describe("G1 — Failed Phase (ADR-EM-001 §11)", func() {
		It("IT-EM-573-010: should reject EA with empty correlationID at CRD validation", func() {
			ns := createTestNamespace("em-573-010")
			defer deleteTestNamespace(ns)

			By("Attempting to create an EA with empty correlationID")
			ea := &eav1.EffectivenessAssessment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ea-573-010",
					Namespace: ns,
				},
				Spec: eav1.EffectivenessAssessmentSpec{
					CorrelationID:           "",
					RemediationRequestPhase: "Completed",
					SignalTarget: eav1.TargetResource{
						Kind:      "Deployment",
						Name:      "test-app",
						Namespace: ns,
					},
					RemediationTarget: eav1.TargetResource{
						Kind:      "Deployment",
						Name:      "test-app",
						Namespace: ns,
					},
					Config: eav1.EAConfig{
						StabilizationWindow: metav1.Duration{Duration: 1 * time.Second},
					},
				},
			}

			By("Verifying the API server rejects the create with a validation error")
			err := k8sClient.Create(ctx, ea)
			Expect(err).To(HaveOccurred(),
				"IT-EM-573-010: CRD should reject empty correlationID")
			Expect(apierrors.IsInvalid(err)).To(BeTrue(),
				"IT-EM-573-010: error should be a validation error")
			Expect(err.Error()).To(ContainSubstring("correlationID"),
				"IT-EM-573-010: error should mention correlationID")
		})
	})

	// ========================================================================
	// IT-EM-573-012: Scheduled event on Stabilizing transition
	// ========================================================================
	Describe("G2 — Scheduled Event on Stabilizing (ADR-EM-001 §9.2.0)", func() {
		It("IT-EM-573-012: should emit AssessmentScheduled event on Stabilizing transition", func() {
			ns := createTestNamespace("em-573-012")
			defer deleteTestNamespace(ns)

			By("Creating an EA with a stabilization window long enough to observe Stabilizing phase")
			ea := &eav1.EffectivenessAssessment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ea-573-012",
					Namespace: ns,
				},
				Spec: eav1.EffectivenessAssessmentSpec{
					CorrelationID:           "rr-573-012",
					RemediationRequestPhase: "Completed",
					SignalTarget: eav1.TargetResource{
						Kind:      "Deployment",
						Name:      "test-app",
						Namespace: ns,
					},
					RemediationTarget: eav1.TargetResource{
						Kind:      "Deployment",
						Name:      "test-app",
						Namespace: ns,
					},
					Config: eav1.EAConfig{
						StabilizationWindow: metav1.Duration{Duration: 5 * time.Second},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ea)).To(Succeed())

			By("Waiting for Stabilizing phase (or later) with ValidityDeadline set")
			fetchedEA := &eav1.EffectivenessAssessment{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name: ea.Name, Namespace: ea.Namespace,
				}, fetchedEA)).To(Succeed())
				g.Expect(fetchedEA.Status.ValidityDeadline).NotTo(BeNil(),
					"IT-EM-573-012: ValidityDeadline should be set during Stabilizing")
			}, timeout, interval).Should(Succeed())

			By("Verifying AssessmentScheduled K8s event is emitted")
			Eventually(func() bool {
				evts := listEventsForObject(ctx, k8sClient, ea.Name, ns)
				return containsReason(eventReasons(evts), "AssessmentScheduled")
			}, 10*time.Second, interval).Should(BeTrue(),
				"IT-EM-573-012: should emit AssessmentScheduled event on Stabilizing transition")
		})
	})

	// ========================================================================
	// IT-EM-573-013 / IT-EM-573-014: Assessment path differentiation
	//
	// These tests create a local reconciler with a mock DSQuerier (httptest)
	// and an isolated fake client (fakeclient.NewClientBuilder). The fake client
	// prevents resource-version conflicts with the suite's manager-driven
	// reconciler which watches all EAs on the shared envtest API server.
	// ========================================================================
	Describe("G4 — Assessment Path Differentiation (ADR-EM-001 §5)", func() {
		var (
			dsServer     *httptest.Server
			localMetrics *emmetrics.Metrics
			fakeRecorder *record.FakeRecorder
		)

		AfterEach(func() {
			if dsServer != nil {
				dsServer.Close()
			}
		})

		// newLocalReconciler builds a reconciler with a mock DSQuerier using a fake client.
		// The fake client isolates these tests from the suite's manager-driven reconciler
		// so there are no resource-version conflicts on status updates.
		newLocalReconciler := func(dsURL string, initObjects ...client.Object) (*controller.Reconciler, client.Client) {
			localMetrics = emmetrics.NewMetricsWithRegistry(prometheus.NewRegistry())
			fakeRecorder = record.NewFakeRecorder(20)
			dsQuerier, err := emclient.NewOgenDataStorageQuerier(dsURL, 10*time.Second)
			Expect(err).ToNot(HaveOccurred())

			s := runtime.NewScheme()
			Expect(eav1.AddToScheme(s)).To(Succeed())
			fc := fakeclient.NewClientBuilder().
				WithScheme(s).
				WithObjects(initObjects...).
				WithStatusSubresource(&eav1.EffectivenessAssessment{}).
				Build()

			r := controller.NewReconciler(
				fc,
				fc,
				s,
				fakeRecorder,
				localMetrics,
				nil, // no Prometheus (not needed for partial path)
				nil, // no AlertManager
				nil, // no AuditManager
				dsQuerier,
				func() controller.ReconcilerConfig {
					c := controller.DefaultReconcilerConfig()
					c.PrometheusEnabled = false
					c.AlertManagerEnabled = false
					return c
				}(),
			)
			return r, fc
		}

		// ogenCompliantEvent builds a schema-compliant AuditEvent JSON map with
		// all 8 required fields plus event_type discriminator inside event_data.
		ogenCompliantEvent := func(eventType, correlationID string) map[string]interface{} {
			return map[string]interface{}{
				"version":         "1.0",
				"event_type":      eventType,
				"event_timestamp": "2026-01-01T00:00:00Z",
				"event_category":  "workflowexecution",
				"event_action":    "test_action",
				"event_outcome":   "success",
				"correlation_id":  correlationID,
				"event_data": map[string]interface{}{
					"event_type":       eventType,
					"workflow_id":      "test-wf",
					"workflow_version": "1.0.0",
					"target_resource":  "default/Deployment/test-app",
					"phase":            "Running",
					"container_image":  "test:latest",
					"execution_name":   "test-exec",
				},
			}
		}

		// mockDSHandler creates an HTTP handler that responds to HasWorkflowStarted
		// and HasWorkflowCompleted queries with ogen-compliant AuditEvent JSON.
		mockDSHandler := func(started, completed bool) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				eventType := r.URL.Query().Get("event_type")
				correlationID := r.URL.Query().Get("correlation_id")
				w.Header().Set("Content-Type", "application/json")

				events := make([]map[string]interface{}, 0)
				switch eventType {
				case "workflowexecution.execution.started":
					if started {
						events = append(events, ogenCompliantEvent(eventType, correlationID))
					}
				case "workflowexecution.workflow.completed":
					if completed {
						events = append(events, ogenCompliantEvent(eventType, correlationID))
					}
				}
				envelope := map[string]interface{}{"data": events}
				_ = json.NewEncoder(w).Encode(envelope)
			}
		}

		It("IT-EM-573-013: should complete with reason=partial when WFE started but not completed", func() {
			By("Setting up DS mock: started=true, completed=false")
			dsServer = httptest.NewServer(mockDSHandler(true, false))

			ea := &eav1.EffectivenessAssessment{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "ea-573-013",
					Namespace:         "default",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(-60 * time.Second)},
				},
				Spec: eav1.EffectivenessAssessmentSpec{
					CorrelationID:           "rr-573-013-partial",
					RemediationRequestPhase: "Completed",
					SignalTarget: eav1.TargetResource{
						Kind:      "Deployment",
						Name:      "test-app",
						Namespace: "default",
					},
					RemediationTarget: eav1.TargetResource{
						Kind:      "Deployment",
						Name:      "test-app",
						Namespace: "default",
					},
					Config: eav1.EAConfig{
						StabilizationWindow: metav1.Duration{Duration: 0},
					},
				},
			}

			By("Creating reconciler with fake client seeded with the EA")
			reconciler, fc := newLocalReconciler(dsServer.URL, ea)

			By("Calling Reconcile() directly with the local reconciler")
			req := ctrl.Request{NamespacedName: types.NamespacedName{Name: ea.Name, Namespace: "default"}}
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Requeue).To(BeFalse(), "IT-EM-573-013: should not requeue after partial completion")

			By("Fetching the EA and verifying partial assessment")
			fetchedEA := &eav1.EffectivenessAssessment{}
			Expect(fc.Get(ctx, req.NamespacedName, fetchedEA)).To(Succeed())

			Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted),
				"IT-EM-573-013: phase should be Completed")
			Expect(fetchedEA.Status.AssessmentReason).To(Equal("partial"),
				"IT-EM-573-013: reason should be partial")
			Expect(fetchedEA.Status.Components.HealthAssessed).To(BeTrue(),
				"IT-EM-573-013: health should be assessed")
			Expect(fetchedEA.Status.Components.HashComputed).To(BeTrue(),
				"IT-EM-573-013: hash should be computed")
			Expect(fetchedEA.Status.Components.MetricsAssessed).To(BeFalse(),
				"IT-EM-573-013: metrics should NOT be assessed (partial path)")
			Expect(fetchedEA.Status.Components.AlertAssessed).To(BeFalse(),
				"IT-EM-573-013: alert should NOT be assessed (partial path)")
		})

		It("IT-EM-573-014: should perform full assessment when WFE started AND completed", func() {
			By("Setting up DS mock: started=true, completed=true")
			dsServer = httptest.NewServer(mockDSHandler(true, true))

			ea := &eav1.EffectivenessAssessment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ea-573-014",
					Namespace: "default",
				},
				Spec: eav1.EffectivenessAssessmentSpec{
					CorrelationID:           "rr-573-014-full",
					RemediationRequestPhase: "Completed",
					SignalTarget: eav1.TargetResource{
						Kind:      "Deployment",
						Name:      "test-app",
						Namespace: "default",
					},
					RemediationTarget: eav1.TargetResource{
						Kind:      "Deployment",
						Name:      "test-app",
						Namespace: "default",
					},
					Config: eav1.EAConfig{
						StabilizationWindow: metav1.Duration{Duration: 0},
					},
				},
			}

			By("Creating reconciler with fake client seeded with the EA")
			reconciler, fc := newLocalReconciler(dsServer.URL, ea)

			By("Calling Reconcile() directly with the local reconciler")
			req := ctrl.Request{NamespacedName: types.NamespacedName{Name: ea.Name, Namespace: "default"}}
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			By("Fetching the EA and verifying full assessment path")
			fetchedEA := &eav1.EffectivenessAssessment{}
			Expect(fc.Get(ctx, req.NamespacedName, fetchedEA)).To(Succeed())

			// With Prometheus/AlertManager disabled, full path completes with health+hash only,
			// but the reconciler takes the full path (not the partial early-exit).
			// The distinction: partial path exits at Step 7a before alert/metrics;
			// full path reaches allComponentsDone at Step 8.
			Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted),
				"IT-EM-573-014: phase should be Completed")
			Expect(fetchedEA.Status.AssessmentReason).NotTo(Equal("partial"),
				"IT-EM-573-014: reason should NOT be partial (full assessment path)")
			Expect(fetchedEA.Status.Components.HealthAssessed).To(BeTrue(),
				"IT-EM-573-014: health should be assessed")
			Expect(fetchedEA.Status.Components.HashComputed).To(BeTrue(),
				"IT-EM-573-014: hash should be computed")

			// Prometheus and AlertManager are disabled, so these are marked as assessed (skipped)
			// via the "disabled" path — but critically, the reconciler did NOT take the partial
			// early-exit at Step 7a.
			Expect(result.RequeueAfter).To(BeZero(),
				"IT-EM-573-014: should not requeue after full completion")
		})
	})
})
