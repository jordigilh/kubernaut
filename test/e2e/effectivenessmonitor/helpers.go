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
			Labels: map[string]string{
				"kubernaut.ai/correlation-id": correlationID,
			},
		},
		Spec: eav1.EffectivenessAssessmentSpec{
			CorrelationID: correlationID,
			TargetResource: eav1.TargetResource{
				Kind:      "Pod",
				Name:      "target-pod",
				Namespace: namespace,
			},
			Config: eav1.EAConfig{
				StabilizationWindow: metav1.Duration{Duration: 10 * time.Second},
				ValidityDeadline:    metav1.Time{Time: time.Now().Add(5 * time.Minute)},
				ScoringThreshold:    0.5,
				PrometheusEnabled:   true,
				AlertManagerEnabled: true,
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

// withValidityDeadline sets the validity deadline for the EA.
func withValidityDeadline(deadline time.Time) eaOption {
	return func(ea *eav1.EffectivenessAssessment) {
		ea.Spec.Config.ValidityDeadline = metav1.Time{Time: deadline}
	}
}

// withPrometheusDisabled disables Prometheus in the EA config.
func withPrometheusDisabled() eaOption {
	return func(ea *eav1.EffectivenessAssessment) {
		ea.Spec.Config.PrometheusEnabled = false
	}
}

// withAlertManagerDisabled disables AlertManager in the EA config.
func withAlertManagerDisabled() eaOption {
	return func(ea *eav1.EffectivenessAssessment) {
		ea.Spec.Config.AlertManagerEnabled = false
	}
}

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

