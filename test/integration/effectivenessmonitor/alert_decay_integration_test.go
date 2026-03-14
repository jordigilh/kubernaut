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

package effectivenessmonitor

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

var _ = Describe("Alert Decay Detection Integration (Issue #369, BR-EM-012)", func() {

	// ========================================
	// IT-EM-DECAY-001: End-to-end duplicate suppression during alert decay
	// Business outcome: System keeps EA open during alert decay, then
	// completes correctly when alert resolves — no manual intervention needed.
	// ========================================
	It("IT-EM-DECAY-001: should suppress duplicate RRs during alert decay window then complete on resolution", func() {
		ns := createTestNamespace("em-decay-001")
		defer deleteTestNamespace(ns)

		By("Creating a healthy target pod to enable positive health score")
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-decay",
				Namespace: ns,
				Labels:    map[string]string{"app": "test-app"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "main",
						Image: "registry.k8s.io/pause:3.9",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())

		By("Simulating pod readiness (Running + Ready)")
		pod.Status = corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "main",
					Ready:        true,
					RestartCount: 0,
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

		By("Configuring mock AlertManager with a firing alert (simulating decay)")
		mockAM.SetAlertsResponse([]infrastructure.AMAlert{
			infrastructure.NewFiringAlert("HighMemory", map[string]string{
				"namespace": ns,
			}),
		})

		By("Creating an EA with Verifying RR phase (normal post-remediation path)")
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-decay-001",
				Namespace: ns,
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-decay-001",
				RemediationRequestPhase: "Verifying",
				SignalName:              "HighMemory",
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
				RemediationCreatedAt: &metav1.Time{Time: time.Now().Add(-5 * time.Minute)},
			},
		}
		Expect(k8sClient.Create(ctx, ea)).To(Succeed())

		By("Verifying alert decay is detected: EA stays in Assessing with AlertDecayRetries > 0")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseAssessing),
				"EA should be in Assessing (not completed prematurely)")
			g.Expect(fetchedEA.Status.Components.AlertDecayRetries).To(BeNumerically(">", 0),
				"AlertDecayRetries should be > 0 (decay detection active)")
			g.Expect(fetchedEA.Status.Components.AlertAssessed).To(BeFalse(),
				"AlertAssessed should be false (EA kept open for re-check)")
		}, timeout, interval).Should(Succeed())

		GinkgoWriter.Printf("Alert decay detected after %d retries\n",
			fetchedEA.Status.Components.AlertDecayRetries)

		By("Switching mock AlertManager to resolved (simulating alert decay completion)")
		mockAM.SetAlertsResponse([]infrastructure.AMAlert{})

		By("Verifying EA completes with full reason and AlertScore=1.0")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.Components.AlertAssessed).To(BeTrue(),
			"AlertAssessed should be true after alert resolved")
		Expect(fetchedEA.Status.Components.AlertScore).To(HaveValue(Equal(1.0)),
			"AlertScore should be 1.0 (confirmed resolved)")
		Expect(fetchedEA.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonFull),
			"Reason should be 'full' (all components assessed)")
		Expect(fetchedEA.Status.Components.AlertDecayRetries).To(BeNumerically(">", 0),
			"AlertDecayRetries should be preserved for operator observability")

		GinkgoWriter.Printf("EA completed: reason=%s, alertScore=%.1f, decayRetries=%d\n",
			fetchedEA.Status.AssessmentReason,
			*fetchedEA.Status.Components.AlertScore,
			fetchedEA.Status.Components.AlertDecayRetries)

		By("Restoring mock AlertManager to default")
		mockAM.SetAlertsResponse([]infrastructure.AMAlert{})
	})

	// ========================================
	// IT-EM-DECAY-002: Proactive signal — metrics negative, alert is genuine
	// BR-EM-012: When metrics show no improvement (proactive signal) and the
	// alert is firing, the metrics gate in isAlertDecay prevents decay detection.
	// The EA completes with AlertScore=0.0 — the alert is genuine.
	// ========================================
	It("IT-EM-DECAY-002: should complete with AlertScore=0.0 when metrics are negative (proactive signal kills decay)", func() {
		ns := createTestNamespace("em-decay-002")
		defer deleteTestNamespace(ns)

		By("Creating a healthy target pod to enable positive health score")
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-decay-002",
				Namespace: ns,
				Labels:    map[string]string{"app": "test-app"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "main",
						Image: "registry.k8s.io/pause:3.9",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())

		By("Simulating pod readiness (Running + Ready)")
		pod.Status = corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "main",
					Ready:        true,
					RestartCount: 0,
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

		By("Configuring mock AlertManager with a firing alert")
		mockAM.SetAlertsResponse([]infrastructure.AMAlert{
			infrastructure.NewFiringAlert("HighMemory", map[string]string{
				"namespace": ns,
			}),
		})

		By("Configuring mock Prometheus with degraded metrics (all metrics show degradation)")
		now := float64(time.Now().Unix())
		preRemediationTime := now - 60
		// Use query-aware handler: throughput (HigherIsBetter) needs inverted
		// values so MetricsScore=0.0 triggers the metrics gate in isAlertDecay.
		mockProm.SetQueryRangeHandler(func(w http.ResponseWriter, r *http.Request) {
			query := r.FormValue("query")
			pre, post := "0.250000", "0.500000" // LowerIsBetter: higher post = degradation
			if strings.Contains(query, "http_requests_total") && !strings.Contains(query, "code") {
				pre, post = "0.500000", "0.250000" // HigherIsBetter (throughput): lower post = degradation
			}
			resp := infrastructure.NewPromMatrixResponse(
				map[string]string{"__name__": "metric", "namespace": ns},
				[][]interface{}{
					{preRemediationTime, pre},
					{now, post},
				},
			)
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				http.Error(w, fmt.Sprintf("encode: %v", err), http.StatusInternalServerError)
			}
		})

		By("Creating an EA with short stabilization for faster test execution")
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-decay-002",
				Namespace: ns,
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-decay-002",
				RemediationRequestPhase: "Verifying",
				SignalName:              "HighMemory",
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
				RemediationCreatedAt: &metav1.Time{Time: time.Now().Add(-5 * time.Minute)},
			},
		}
		Expect(k8sClient.Create(ctx, ea)).To(Succeed())

		By("Verifying EA completes with AlertScore=0.0 and reason=full (metrics gate killed decay)")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted),
				"EA should complete (metrics gate prevented decay detection)")
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.Components.AlertAssessed).To(BeTrue(),
			"AlertAssessed should be true (alert accepted at face value)")
		Expect(fetchedEA.Status.Components.AlertScore).To(HaveValue(Equal(0.0)),
			"AlertScore should be 0.0 (alert firing, remediation failed)")
		Expect(fetchedEA.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonFull),
			"AssessmentReason should be 'full' (all components assessed, not alert_decay_timeout)")
		Expect(fetchedEA.Status.Components.AlertDecayRetries).To(Equal(int32(1)),
			"AlertDecayRetries should be 1 (one decay pass before metrics gate killed hypothesis)")

		GinkgoWriter.Printf("EA completed: reason=%s, alertScore=%.1f, decayRetries=%d\n",
			fetchedEA.Status.AssessmentReason,
			*fetchedEA.Status.Components.AlertScore,
			fetchedEA.Status.Components.AlertDecayRetries)

		By("Restoring mock AlertManager and Prometheus to defaults")
		mockAM.SetAlertsResponse([]infrastructure.AMAlert{})
		mockProm.SetQueryRangeHandler(nil)
		now = float64(time.Now().Unix())
		preRemediationTime = now - 60
		mockProm.SetQueryRangeResponse(infrastructure.NewPromMatrixResponse(
			map[string]string{"__name__": "container_cpu_usage_seconds_total", "namespace": "default"},
			[][]interface{}{
				{preRemediationTime, "0.500000"},
				{now, "0.250000"},
			},
		))
	})
})
