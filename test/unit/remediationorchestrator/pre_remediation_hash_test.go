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

package remediationorchestrator

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	controller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
)

// ========================================
// CapturePreRemediationHash Tests (DD-EM-002)
//
// Contract: CapturePreRemediationHash resolves the target resource Kind,
// fetches it via apiReader, extracts .spec, and computes the canonical hash.
// Non-fatal on missing resources (returns empty string).
// ========================================
var _ = Describe("CapturePreRemediationHash (DD-EM-002)", func() {

	var (
		ctx        context.Context
		scheme     *runtime.Scheme
		restMapper meta.RESTMapper
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		// Build a simple REST mapper that maps Deployment Kind to apps/v1
		restMapper = meta.NewDefaultRESTMapper([]schema.GroupVersion{
			{Group: "apps", Version: "v1"},
			{Group: "", Version: "v1"},
		})
		restMapper.(*meta.DefaultRESTMapper).Add(
			schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			meta.RESTScopeNamespace,
		)
	})

	It("should return canonical hash when target resource exists", func() {
		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nginx",
				Namespace: "default",
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(3),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "nginx"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "nginx"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "nginx", Image: "nginx:1.21"},
						},
					},
				},
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(deploy).
			Build()

		hash, degradedReason, err := controller.CapturePreRemediationHash(
			ctx, fakeClient, restMapper, "Deployment", "nginx", "default",
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(degradedReason).To(BeEmpty(), "Success should not set degradedReason")
		Expect(hash).To(HavePrefix("sha256:"))
		Expect(hash).To(HaveLen(71))
	})

	It("should return empty string when target resource not found (non-fatal)", func() {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		hash, degradedReason, err := controller.CapturePreRemediationHash(
			ctx, fakeClient, restMapper, "Deployment", "nonexistent", "default",
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(degradedReason).To(BeEmpty(), "NotFound is legitimate, not degraded")
		Expect(hash).To(BeEmpty(), "Missing resource should return empty hash, not an error")
	})

	It("should return empty string when resource has no .spec", func() {
		// ConfigMap has no .spec field
		restMapper.(*meta.DefaultRESTMapper).Add(
			schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"},
			meta.RESTScopeNamespace,
		)

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-config",
				Namespace: "default",
			},
			Data: map[string]string{"key": "value"},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cm).
			Build()

		hash, degradedReason, err := controller.CapturePreRemediationHash(
			ctx, fakeClient, restMapper, "ConfigMap", "test-config", "default",
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(degradedReason).To(BeEmpty(), "No .spec is legitimate, not degraded")
		Expect(hash).NotTo(BeEmpty(), "#765: ConfigMap functional state (.data) produces valid fingerprint")
		Expect(hash).To(HavePrefix("sha256:"))
	})

	// UT-RO-545-001: Forbidden error yields empty hash (degraded soft-fail)
	It("UT-RO-545-001: should return degraded soft-fail when RBAC denies access (Forbidden)", func() {
		restMapper.(*meta.DefaultRESTMapper).Add(
			schema.GroupVersionKind{Group: "cert-manager.io", Version: "v1", Kind: "Certificate"},
			meta.RESTScopeNamespace,
		)

		forbiddenClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithInterceptorFuncs(interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					return apierrors.NewForbidden(
						schema.GroupResource{Group: "cert-manager.io", Resource: "certificates"},
						key.Name,
						fmt.Errorf("user system:serviceaccount:kubernaut:remediationorchestrator-controller is not allowed to get certificates"),
					)
				},
			}).
			Build()

		hash, degradedReason, err := controller.CapturePreRemediationHash(
			ctx, forbiddenClient, restMapper, "Certificate", "demo-app-cert", "demo-cert-failure",
		)
		Expect(err).ToNot(HaveOccurred(), "Forbidden should soft-fail, not return error")
		Expect(hash).To(BeEmpty(), "Hash should be empty on Forbidden")
		Expect(degradedReason).ToNot(BeEmpty(), "degradedReason should describe the access denial")
		Expect(degradedReason).To(ContainSubstring("forbidden"), "degradedReason should mention forbidden")
	})

	// UT-RO-545-002: K8s InternalError yields empty hash (degraded soft-fail)
	It("UT-RO-545-002: should return degraded soft-fail when K8s API returns InternalError", func() {
		internalErrClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithInterceptorFuncs(interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					return apierrors.NewInternalError(fmt.Errorf("etcd timeout"))
				},
			}).
			Build()

		hash, degradedReason, err := controller.CapturePreRemediationHash(
			ctx, internalErrClient, restMapper, "Deployment", "failing-deploy", "default",
		)
		Expect(err).ToNot(HaveOccurred(), "InternalError should soft-fail, not return error")
		Expect(hash).To(BeEmpty(), "Hash should be empty on InternalError")
		Expect(degradedReason).ToNot(BeEmpty(), "degradedReason should describe the fetch failure")
	})

	// UT-RO-545-004: Unknown Kind returns empty hash
	It("UT-RO-545-004: should return empty hash when GVK cannot be resolved (unknown Kind)", func() {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		hash, degradedReason, err := controller.CapturePreRemediationHash(
			ctx, fakeClient, restMapper, "UnknownCRD", "something", "default",
		)
		Expect(err).ToNot(HaveOccurred(), "Unknown Kind should soft-fail")
		Expect(degradedReason).To(BeEmpty(), "Unknown GVK is legitimate, not degraded")
		Expect(hash).To(BeEmpty(), "Unknown Kind should return empty hash")
	})
})

// ========================================
// CapturePreRemediationHash with ConfigMaps (#396, BR-EM-004)
//
// Tests that CapturePreRemediationHash includes ConfigMap content in the
// composite hash via resolveConfigMapHashes + CompositeResourceFingerprint.
// ========================================
var _ = Describe("CapturePreRemediationHash with ConfigMaps (#396)", func() {

	var (
		ctx        context.Context
		scheme     *runtime.Scheme
		restMapper *meta.DefaultRESTMapper
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(batchv1.AddToScheme(scheme)).To(Succeed())

		restMapper = meta.NewDefaultRESTMapper([]schema.GroupVersion{
			{Group: "apps", Version: "v1"},
			{Group: "batch", Version: "v1"},
			{Group: "", Version: "v1"},
		})
		restMapper.Add(
			schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			meta.RESTScopeNamespace,
		)
		restMapper.Add(
			schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"},
			meta.RESTScopeNamespace,
		)
	})

	It("UT-RO-396-001: should produce composite hash when Deployment has ConfigMap volume", func() {
		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(1),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "app", Image: "nginx:1.25"}},
						Volumes: []corev1.Volume{{
							Name:         "config-vol",
							VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "app-config"}}},
						}},
					},
				},
			},
		}
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "default"},
			Data:       map[string]string{"config.yaml": "server:\n  port: 8080"},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deploy, cm).Build()

		compositeHash, _, err := controller.CapturePreRemediationHash(ctx, fakeClient, restMapper, "Deployment", "app", "default")
		Expect(err).ToNot(HaveOccurred())
		Expect(compositeHash).To(HavePrefix("sha256:"))
		Expect(compositeHash).To(HaveLen(71))

		// Same deployment but without ConfigMap available -> sentinel-based composite
		fakeClientNoCM := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deploy).Build()
		sentinelHash, _, err := controller.CapturePreRemediationHash(ctx, fakeClientNoCM, restMapper, "Deployment", "app", "default")
		Expect(err).ToNot(HaveOccurred())
		Expect(compositeHash).ToNot(Equal(sentinelHash),
			"composite hash with real ConfigMap data must differ from sentinel-based hash")
	})

	It("UT-RO-396-002: should use absent sentinel when referenced ConfigMap not found (404)", func() {
		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "missing-cm", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(1),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "app", Image: "nginx:1.25"}},
						Volumes: []corev1.Volume{{
							Name:         "missing-vol",
							VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "nonexistent-config"}}},
						}},
					},
				},
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deploy).Build()

		hash, _, err := controller.CapturePreRemediationHash(ctx, fakeClient, restMapper, "Deployment", "missing-cm", "default")
		Expect(err).ToNot(HaveOccurred())
		Expect(hash).To(HavePrefix("sha256:"))
		Expect(hash).To(HaveLen(71))

		// Sentinel hash is deterministic: same absent ConfigMap -> same hash
		hash2, _, err := controller.CapturePreRemediationHash(ctx, fakeClient, restMapper, "Deployment", "missing-cm", "default")
		Expect(err).ToNot(HaveOccurred())
		Expect(hash).To(Equal(hash2), "sentinel hash must be deterministic for same absent ConfigMap")
	})

	It("UT-RO-396-003: should return spec-only hash when no ConfigMap refs", func() {
		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "simple", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(2),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "simple"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "simple"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "app", Image: "nginx:1.25"}},
					},
				},
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deploy).Build()

		hash, _, err := controller.CapturePreRemediationHash(ctx, fakeClient, restMapper, "Deployment", "simple", "default")
		Expect(err).ToNot(HaveOccurred())
		Expect(hash).To(HavePrefix("sha256:"))
		Expect(hash).To(HaveLen(71))
	})

	It("UT-RO-396-004: should treat 403 Forbidden on ConfigMap fetch as absent (sentinel + warning)", func() {
		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "forbidden-cm", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(1),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "app", Image: "nginx:1.25"}},
						Volumes: []corev1.Volume{{
							Name:         "config-vol",
							VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "restricted-config"}}},
						}},
					},
				},
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deploy).
			WithInterceptorFuncs(interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if _, ok := obj.(*corev1.ConfigMap); ok {
						return apierrors.NewForbidden(
							schema.GroupResource{Group: "", Resource: "configmaps"},
							key.Name,
							nil,
						)
					}
					return c.Get(ctx, key, obj, opts...)
				},
			}).Build()

		hash, _, err := controller.CapturePreRemediationHash(ctx, fakeClient, restMapper, "Deployment", "forbidden-cm", "default")
		Expect(err).ToNot(HaveOccurred())
		Expect(hash).To(HavePrefix("sha256:"))
		Expect(hash).To(HaveLen(71),
			"403 Forbidden must be treated as absent, producing a sentinel-based composite hash")

		// Verify it matches the 404 sentinel for the same ConfigMap name
		deployNotFound := deploy.DeepCopy()
		fakeClientNotFound := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deployNotFound).Build()
		hashNotFound, _, err := controller.CapturePreRemediationHash(ctx, fakeClientNotFound, restMapper, "Deployment", "forbidden-cm", "default")
		Expect(err).ToNot(HaveOccurred())
		Expect(hash).To(Equal(hashNotFound),
			"403 Forbidden and 404 NotFound must produce identical sentinel hashes")
	})

	It("UT-RO-396-005: should resolve ConfigMap refs from CronJob nested pod template", func() {
		cronJob := &batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{Name: "batch-job", Namespace: "default"},
			Spec: batchv1.CronJobSpec{
				Schedule: "*/5 * * * *",
				JobTemplate: batchv1.JobTemplateSpec{
					Spec: batchv1.JobSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								RestartPolicy: corev1.RestartPolicyOnFailure,
								Containers:    []corev1.Container{{Name: "worker", Image: "worker:1.0"}},
								Volumes: []corev1.Volume{{
									Name:         "cron-config-vol",
									VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "cron-config"}}},
								}},
							},
						},
					},
				},
			},
		}
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "cron-config", Namespace: "default"},
			Data:       map[string]string{"schedule.yaml": "interval: 5m"},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cronJob, cm).Build()

		compositeHash, _, err := controller.CapturePreRemediationHash(ctx, fakeClient, restMapper, "CronJob", "batch-job", "default")
		Expect(err).ToNot(HaveOccurred())
		Expect(compositeHash).To(HavePrefix("sha256:"))
		Expect(compositeHash).To(HaveLen(71))

		// Without the ConfigMap, sentinel hash must differ
		fakeClientNoCM := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cronJob).Build()
		sentinelHash, _, err := controller.CapturePreRemediationHash(ctx, fakeClientNoCM, restMapper, "CronJob", "batch-job", "default")
		Expect(err).ToNot(HaveOccurred())
		Expect(compositeHash).ToNot(Equal(sentinelHash),
			"CronJob composite hash with real ConfigMap must differ from sentinel-based hash")
	})
})

func int32Ptr(i int32) *int32 {
	return &i
}
