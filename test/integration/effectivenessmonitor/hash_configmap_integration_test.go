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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
)

// ========================================
// ConfigMap-Aware Spec Hash Integration Tests (#396, BR-EM-004)
//
// These tests validate that the EM reconciler includes ConfigMap content
// in the composite spec hash, both in assessHash (Step 3) and the
// drift guard (Step 6.5).
//
// Infrastructure: envtest (real K8s API), httptest mocks (Prom, AM)
// External deps: DataStorage (PostgreSQL, Redis)
// ========================================
var _ = Describe("ConfigMap-Aware Spec Hash Integration (#396)", func() {

	// IT-EM-396-001: Composite hash matches pre-hash when spec + ConfigMaps unchanged (Match=true)
	It("IT-EM-396-001: should match pre-hash when spec and ConfigMaps are unchanged", func() {
		ns := createTestNamespace("em-396-001")
		defer deleteTestNamespace(ns)

		By("Creating ConfigMap referenced by the Deployment")
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: ns},
			Data:       map[string]string{"config.yaml": "server:\n  port: 8080\n  debug: false"},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())

		By("Creating a Deployment with ConfigMap volume mount")
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "test-app", Namespace: ns},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To(int32(1)),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test-app"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test-app"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "app", Image: "nginx:1.25"}},
						Volumes: []corev1.Volume{{
							Name:         "config",
							VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "app-config"}}},
						}},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, dep)).To(Succeed())

		By("Creating first EA to capture the composite hash")
		ea1 := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{Name: "ea-396-001a", Namespace: ns},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-396-001a",
				RemediationRequestPhase: "Completed",
				SignalTarget:            eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: ns},
				RemediationTarget:       eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: ns},
				Config:                  eav1.EAConfig{StabilizationWindow: metav1.Duration{Duration: 1 * time.Second}},
			},
		}
		Expect(k8sClient.Create(ctx, ea1)).To(Succeed())

		fetchedEA1 := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ea1.Name, Namespace: ea1.Namespace}, fetchedEA1)).To(Succeed())
			g.Expect(fetchedEA1.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		hash1 := fetchedEA1.Status.Components.PostRemediationSpecHash
		Expect(hash1).To(HavePrefix("sha256:"))
		Expect(hash1).To(HaveLen(71))

		By("Creating second EA with pre-hash set to first EA's hash (same spec, same ConfigMap)")
		ea2 := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{Name: "ea-396-001b", Namespace: ns},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:            "rr-396-001b",
				RemediationRequestPhase:  "Completed",
				PreRemediationSpecHash:   hash1,
				SignalTarget:             eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: ns},
				RemediationTarget:        eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: ns},
				Config:                   eav1.EAConfig{StabilizationWindow: metav1.Duration{Duration: 1 * time.Second}},
			},
		}
		Expect(k8sClient.Create(ctx, ea2)).To(Succeed())

		fetchedEA2 := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ea2.Name, Namespace: ea2.Namespace}, fetchedEA2)).To(Succeed())
			g.Expect(fetchedEA2.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		By("Verifying Match=true: same spec + same ConfigMap → identical composite hashes")
		Expect(fetchedEA2.Status.Components.PostRemediationSpecHash).To(Equal(hash1),
			"composite hash must be identical when spec and ConfigMaps are unchanged")
	})

	// IT-EM-396-002: ConfigMap data change between assessments → different hashes
	It("IT-EM-396-002: should produce different hash when ConfigMap data changes", func() {
		ns := createTestNamespace("em-396-002")
		defer deleteTestNamespace(ns)

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: ns},
			Data:       map[string]string{"config.yaml": "version: v1"},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())

		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "test-app", Namespace: ns},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To(int32(1)),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test-app"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test-app"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "app", Image: "nginx:1.25"}},
						Volumes: []corev1.Volume{{
							Name:         "config",
							VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "app-config"}}},
						}},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, dep)).To(Succeed())

		By("Creating first EA with ConfigMap v1")
		ea1 := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{Name: "ea-396-002a", Namespace: ns},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-396-002a",
				RemediationRequestPhase: "Completed",
				SignalTarget:            eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: ns},
				RemediationTarget:       eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: ns},
				Config:                  eav1.EAConfig{StabilizationWindow: metav1.Duration{Duration: 1 * time.Second}},
			},
		}
		Expect(k8sClient.Create(ctx, ea1)).To(Succeed())

		fetchedEA1 := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ea1.Name, Namespace: ea1.Namespace}, fetchedEA1)).To(Succeed())
			g.Expect(fetchedEA1.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())
		hash1 := fetchedEA1.Status.Components.PostRemediationSpecHash

		By("Updating ConfigMap data")
		Eventually(func(g Gomega) {
			freshCM := &corev1.ConfigMap{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-config", Namespace: ns}, freshCM)).To(Succeed())
			freshCM.Data["config.yaml"] = "version: v2"
			g.Expect(k8sClient.Update(ctx, freshCM)).To(Succeed())
		}, timeout, interval).Should(Succeed())

		By("Creating second EA with ConfigMap v2")
		ea2 := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{Name: "ea-396-002b", Namespace: ns},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-396-002b",
				RemediationRequestPhase: "Completed",
				SignalTarget:            eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: ns},
				RemediationTarget:       eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: ns},
				Config:                  eav1.EAConfig{StabilizationWindow: metav1.Duration{Duration: 1 * time.Second}},
			},
		}
		Expect(k8sClient.Create(ctx, ea2)).To(Succeed())

		fetchedEA2 := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ea2.Name, Namespace: ea2.Namespace}, fetchedEA2)).To(Succeed())
			g.Expect(fetchedEA2.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())
		hash2 := fetchedEA2.Status.Components.PostRemediationSpecHash

		Expect(hash1).ToNot(Equal(hash2),
			"ConfigMap data change must produce different composite hash")
	})

	// IT-EM-396-003: Drift guard detects ConfigMap change during multi-reconcile assessment
	//
	// Production scenario: alert resolution is deferred (proactive signal with AlertCheckDelay),
	// so hash/health/metrics complete first and the EA requeues waiting for the alert window.
	// During that requeue, a ConfigMap is modified. On the next reconcile the drift guard
	// (Step 6.5) re-computes the composite hash via apiReader and detects the change.
	It("IT-EM-396-003: should detect drift when ConfigMap changes during deferred alert window", func() {
		ns := createTestNamespace("em-396-003")
		defer deleteTestNamespace(ns)

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: ns},
			Data:       map[string]string{"config.yaml": "version: stable"},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())

		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "test-app", Namespace: ns},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To(int32(1)),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test-app"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test-app"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "app", Image: "nginx:1.25"}},
						Volumes: []corev1.Volume{{
							Name:         "config",
							VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "app-config"}}},
						}},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, dep)).To(Succeed())

		By("Creating EA with AlertCheckDelay to force multi-reconcile assessment")
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{Name: "ea-396-003", Namespace: ns},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-396-003",
				RemediationRequestPhase: "Completed",
				SignalTarget:            eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: ns},
				RemediationTarget:       eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: ns},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 1 * time.Second},
					AlertCheckDelay:     &metav1.Duration{Duration: 30 * time.Second},
				},
			},
		}
		Expect(k8sClient.Create(ctx, ea)).To(Succeed())

		By("Waiting for hash to be computed (alert still deferred, EA not yet completed)")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ea.Name, Namespace: ea.Namespace}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Components.HashComputed).To(BeTrue())
			g.Expect(fetchedEA.Status.Phase).ToNot(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		By("Modifying ConfigMap data while alert check is deferred")
		Eventually(func(g Gomega) {
			freshCM := &corev1.ConfigMap{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-config", Namespace: ns}, freshCM)).To(Succeed())
			freshCM.Data["config.yaml"] = "version: drifted"
			g.Expect(k8sClient.Update(ctx, freshCM)).To(Succeed())
		}, timeout, interval).Should(Succeed())

		By("Waiting for EA to complete with spec_drift reason")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ea.Name, Namespace: ea.Namespace}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
			g.Expect(fetchedEA.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonSpecDrift))
		}, timeout, interval).Should(Succeed(),
			"ConfigMap change during deferred alert window must trigger spec drift detection")
	})

	// IT-EM-396-004: No ConfigMap refs → backward compatible hash
	It("IT-EM-396-004: should produce standard hash when no ConfigMap refs (backward compat)", func() {
		ns := createTestNamespace("em-396-004")
		defer deleteTestNamespace(ns)

		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "test-app", Namespace: ns},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To(int32(1)),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test-app"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test-app"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "app", Image: "nginx:1.25"}},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, dep)).To(Succeed())

		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{Name: "ea-396-004", Namespace: ns},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-396-004",
				RemediationRequestPhase: "Completed",
				SignalTarget:            eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: ns},
				RemediationTarget:       eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: ns},
				Config:                  eav1.EAConfig{StabilizationWindow: metav1.Duration{Duration: 1 * time.Second}},
			},
		}
		Expect(k8sClient.Create(ctx, ea)).To(Succeed())

		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ea.Name, Namespace: ea.Namespace}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.Components.HashComputed).To(BeTrue())
		Expect(fetchedEA.Status.Components.PostRemediationSpecHash).To(HavePrefix("sha256:"))
		Expect(fetchedEA.Status.Components.PostRemediationSpecHash).To(HaveLen(71),
			"hash format must still be sha256:<64-hex> for backward compat")
	})
})
