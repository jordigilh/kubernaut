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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
)

// ============================================================================
// EA CRD Helpers
// ============================================================================

// createEA creates an EffectivenessAssessment CRD in the given namespace.
// It sets reasonable defaults for fields not explicitly provided.
func createEA(namespace, name, correlationID string, opts ...eaOption) *eav1.EffectivenessAssessment {
	ea := &eav1.EffectivenessAssessment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: eav1.EffectivenessAssessmentSpec{
			CorrelationID:           correlationID,
			RemediationRequestPhase: "Completed",
			TargetResource: eav1.TargetResource{
				Kind:      "Pod",
				Name:      "target-pod",
				Namespace: namespace,
			},
			Config: eav1.EAConfig{
				StabilizationWindow: metav1.Duration{Duration: 10 * time.Second},
			},
		},
	}

	for _, opt := range opts {
		opt(ea)
	}

	GinkgoHelper()
	Expect(k8sClient.Create(ctx, ea)).To(Succeed(), "Failed to create EA %s/%s", namespace, name)
	return ea
}

// eaOption is a functional option for customizing EA creation.
type eaOption func(*eav1.EffectivenessAssessment)

// createExpiredEA creates an EA and patches its status with an already-expired
// ValidityDeadline via the status subresource. Kubernetes ignores status fields
// on Create(), so the deadline must be set via a separate Status().Update().
//
// A long StabilizationWindow (10m) ensures the reconciler's first reconcile
// requeues at the stabilization gate (Step 5) without reaching component
// assessment (Step 7+). The validity checker evaluates expiration BEFORE
// stabilization (validity.Check: "expired takes priority"), so once our
// status patch sets an expired deadline, the next reconcile completes the EA
// as expired with no component assessments — matching the ADR-EM-001 scenario:
// "On first reconcile after recovery, EM checks validity window. If expired,
// marks EA as expired without collecting any data."
//
// This mirrors the integration helper createExpiredEffectivenessAssessment
// but adapts for E2E where the controller runs asynchronously in a pod.
func createExpiredEA(namespace, name, correlationID string) *eav1.EffectivenessAssessment {
	ea := &eav1.EffectivenessAssessment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: eav1.EffectivenessAssessmentSpec{
			CorrelationID:           correlationID,
			RemediationRequestPhase: "Completed",
			TargetResource: eav1.TargetResource{
				Kind:      "Pod",
				Name:      "target-pod",
				Namespace: namespace,
			},
			Config: eav1.EAConfig{
				// Long stabilization ensures the reconciler's first reconcile
				// requeues at the stabilization gate (Step 5) without reaching
				// component assessment (Step 7+). Our status patch then triggers
				// a new reconcile that hits the expired path (Step 4).
				StabilizationWindow: metav1.Duration{Duration: 10 * time.Minute},
			},
		},
	}

	GinkgoHelper()
	Expect(k8sClient.Create(ctx, ea)).To(Succeed(), "Failed to create EA %s/%s", namespace, name)

	// Patch status with an expired ValidityDeadline via the status subresource.
	// Use Eventually to handle potential resourceVersion conflicts if the
	// reconciler updates status between our Get and Update.
	Eventually(func(g Gomega) {
		fetched := &eav1.EffectivenessAssessment{}
		g.Expect(apiReader.Get(ctx, client.ObjectKey{
			Namespace: namespace, Name: name,
		}, fetched)).To(Succeed())

		expired := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		fetched.Status.ValidityDeadline = &expired
		g.Expect(k8sClient.Status().Update(ctx, fetched)).To(Succeed())
	}, 10*time.Second, 200*time.Millisecond).Should(Succeed(),
		"Failed to patch expired ValidityDeadline on EA %s/%s", namespace, name)

	GinkgoWriter.Printf("✅ Created expired EA: %s/%s (correlationID: %s)\n",
		namespace, name, correlationID)
	return ea
}

// NOTE: withPrometheusDisabled and withAlertManagerDisabled were removed.
// Per ADR-EM-001 v1.4: PrometheusEnabled and AlertManagerEnabled are EM operational
// config (effectivenessmonitor.Config.External), NOT per-EA spec fields. Component isolation in E2E
// tests requires deploying the EM with a different ConfigMap, not per-EA options.

// withTargetPod sets the target pod name in the EA spec.
func withTargetPod(name string) eaOption {
	return func(ea *eav1.EffectivenessAssessment) {
		ea.Spec.TargetResource.Name = name
	}
}

// ============================================================================
// Pod Helpers
// ============================================================================

// createTargetPod creates a simple target pod in the given namespace.
// The pod runs a sleep container and becomes Ready.
func createTargetPod(namespace, name string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app": "target-workload",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "workload",
					Image:   "busybox:1.36",
					Command: []string{"sleep", "3600"},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("10m"),
							corev1.ResourceMemory: resource.MustParse("16Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("50m"),
							corev1.ResourceMemory: resource.MustParse("64Mi"),
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyAlways,
		},
	}

	GinkgoHelper()
	Expect(k8sClient.Create(ctx, pod)).To(Succeed(), "Failed to create target pod %s/%s", namespace, name)
	return pod
}

// waitForPodReady waits until the specified pod has a Ready condition.
func waitForPodReady(namespace, name string) {
	GinkgoHelper()
	Eventually(func() bool {
		pod := &corev1.Pod{}
		if err := apiReader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, pod); err != nil {
			return false
		}
		for _, c := range pod.Status.Conditions {
			if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
				return true
			}
		}
		return false
	}, timeout, interval).Should(BeTrue(), "Pod %s/%s did not become Ready", namespace, name)
}

// ============================================================================
// EA Status Helpers
// ============================================================================

// waitForEAPhase waits until the EA reaches the specified phase.
func waitForEAPhase(namespace, name, expectedPhase string) *eav1.EffectivenessAssessment {
	GinkgoHelper()
	ea := &eav1.EffectivenessAssessment{}
	Eventually(func() string {
		if err := apiReader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, ea); err != nil {
			return ""
		}
		return ea.Status.Phase
	}, timeout, interval).Should(Equal(expectedPhase),
		"EA %s/%s did not reach phase %s", namespace, name, expectedPhase)
	return ea
}

// ============================================================================
// Utility
// ============================================================================

// uniqueName generates a unique name for test resources using the Ginkgo process index.
func uniqueName(prefix string) string {
	return fmt.Sprintf("%s-p%d-%d", prefix, GinkgoParallelProcess(), time.Now().UnixNano()%100000)
}

