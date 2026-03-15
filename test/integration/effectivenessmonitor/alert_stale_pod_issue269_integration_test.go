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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/conditions"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// ============================================================================
// STALE POD ALERT FILTERING INTEGRATION TESTS (Issue #269)
//
// These tests verify that the EM reconciler correctly filters out stale
// AlertManager alerts for pods that were deleted during a rolling restart.
//
// Strategy: Create active pods labeled for a Deployment, configure the mock
// AlertManager to return alerts with pod labels (some matching active pods,
// some matching deleted pods), and verify the EA alert score reflects only
// the active pod alerts.
// ============================================================================

var _ = Describe("Alert Scoring — Issue #269: filter stale pod alerts after rolling restart", func() {

	// ========================================================================
	// IT-EM-269-001: AM has alert for deleted pod only -> score 1.0
	//
	// After a Deployment rolling restart, the old pod is deleted but AM
	// still has a firing alert for it. The scorer must filter it out.
	// ========================================================================
	It("IT-EM-269-001: should score 1.0 when AlertManager only has stale alert for deleted pod", func() {
		ns := createTestNamespace("em-269-001")
		defer deleteTestNamespace(ns)

		By("Creating active pods for Deployment 'leaky-app' (post-restart replicas)")
		for _, podName := range []string{"leaky-app-new-rs-abc", "leaky-app-new-rs-def"} {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      podName,
					Namespace: ns,
					Labels:    map[string]string{"app": "leaky-app"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "registry.k8s.io/pause:3.9"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())
			pod.Status = corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "main", Ready: true, RestartCount: 0},
				},
			}
			Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())
		}

		By("Configuring mock AM with a stale alert for a deleted pod")
		mockAM.SetAlertsResponse([]infrastructure.AMAlert{
			infrastructure.NewFiringAlert("ContainerMemoryExhaustionPredicted", map[string]string{
				"namespace": ns,
				"pod":       "leaky-app-old-rs-xyz",
			}),
		})

		By("Creating EA with SignalTarget=Deployment/leaky-app")
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-269-001",
				Namespace: ns,
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-269-001",
				SignalName:              "ContainerMemoryExhaustionPredicted",
				RemediationRequestPhase: "Completed",
				SignalTarget: eav1.TargetResource{
					Kind:      "Deployment",
					Name:      "leaky-app",
					Namespace: ns,
				},
				RemediationTarget: eav1.TargetResource{
					Kind:      "Deployment",
					Name:      "leaky-app",
					Namespace: ns,
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 1 * time.Second},
				},
			},
		}
		Expect(k8sClient.Create(ctx, ea)).To(Succeed())

		By("Waiting for EA to complete")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.Components.AlertAssessed).To(BeTrue())
		Expect(fetchedEA.Status.Components.AlertScore).NotTo(BeNil())
		Expect(*fetchedEA.Status.Components.AlertScore).To(Equal(1.0),
			"#269: stale alert for deleted pod leaky-app-old-rs-xyz must be filtered out; "+
				"only active pods [leaky-app-new-rs-abc, leaky-app-new-rs-def] should count")

		By("Restoring mock AM to default")
		mockAM.SetAlertsResponse([]infrastructure.AMAlert{})
	})

	// ========================================================================
	// IT-EM-269-002: AM has alert for active + deleted pod -> score 0.0
	//
	// After rolling restart, AM has alerts for both the deleted old pod and
	// an active new pod. The stale alert is filtered but the active one remains.
	// ========================================================================
	It("IT-EM-269-002: should score 0.0 when AlertManager has alert for both active and deleted pods", func() {
		ns := createTestNamespace("em-269-002")
		defer deleteTestNamespace(ns)

		By("Creating active pod for Deployment 'leaky-app'")
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "leaky-app-new-rs-abc",
				Namespace: ns,
				Labels:    map[string]string{"app": "leaky-app"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "main", Image: "registry.k8s.io/pause:3.9"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())
		pod.Status = corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "main", Ready: true, RestartCount: 0},
			},
		}
		Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

		By("Configuring mock AM with alerts for both deleted and active pods")
		mockAM.SetAlertsResponse([]infrastructure.AMAlert{
			infrastructure.NewFiringAlert("ContainerMemoryExhaustionPredicted", map[string]string{
				"namespace": ns,
				"pod":       "leaky-app-old-rs-xyz",
			}),
			infrastructure.NewFiringAlert("ContainerMemoryExhaustionPredicted", map[string]string{
				"namespace": ns,
				"pod":       "leaky-app-new-rs-abc",
			}),
		})

		By("Creating EA with SignalTarget=Deployment/leaky-app")
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-269-002",
				Namespace: ns,
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-269-002",
				SignalName:              "ContainerMemoryExhaustionPredicted",
				RemediationRequestPhase: "Completed",
				SignalTarget: eav1.TargetResource{
					Kind:      "Deployment",
					Name:      "leaky-app",
					Namespace: ns,
				},
				RemediationTarget: eav1.TargetResource{
					Kind:      "Deployment",
					Name:      "leaky-app",
					Namespace: ns,
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 1 * time.Second},
				},
			},
		}
		Expect(k8sClient.Create(ctx, ea)).To(Succeed())

		By("Waiting for alert score and decay detection (BR-EM-012, #369)")
		// Post-#369: isAlertDecay returns true when health > 0, hash computed,
		// and alert score == 0.0. The EA stays in Assessing (AlertAssessed=false)
		// and AlertDecayRetries is incremented — matching UT-EM-DECAY-001 spec.
		// We assert the intermediate state rather than waiting for PhaseCompleted
		// (which requires the 30m validity window to expire).
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Components.AlertScore).ToNot(BeNil(),
				"AlertScore should be set on first assessment cycle")
			g.Expect(*fetchedEA.Status.Components.AlertScore).To(Equal(0.0),
				"#269: stale alert for deleted pod filtered, active pod alert counts")
			g.Expect(fetchedEA.Status.Components.AlertDecayRetries).To(
				BeNumerically(">", 0),
				"BR-EM-012: alert decay should be detected (health OK + alert firing)")

			decayCond := meta.FindStatusCondition(fetchedEA.Status.Conditions, conditions.ConditionAlertDecayDetected)
			g.Expect(decayCond).ToNot(BeNil(),
				"BR-EM-012: AlertDecayDetected condition should be present during decay monitoring")
			g.Expect(decayCond.Status).To(Equal(metav1.ConditionTrue),
				"AlertDecayDetected should be True (decay actively monitored)")
			g.Expect(decayCond.Reason).To(Equal(conditions.ReasonDecayActive),
				"Reason should be DecayActive")
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.Components.AlertAssessed).To(BeFalse(),
			"BR-EM-012: AlertAssessed must be false during alert decay monitoring")

		By("Restoring mock AM to default")
		mockAM.SetAlertsResponse([]infrastructure.AMAlert{})
	})
})
