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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

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

		hash, err := controller.CapturePreRemediationHash(
			ctx, fakeClient, restMapper, "Deployment", "nginx", "default",
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(hash).To(HavePrefix("sha256:"))
		Expect(hash).To(HaveLen(71))
	})

	It("should return empty string when target resource not found (non-fatal)", func() {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		hash, err := controller.CapturePreRemediationHash(
			ctx, fakeClient, restMapper, "Deployment", "nonexistent", "default",
		)
		Expect(err).ToNot(HaveOccurred())
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

		hash, err := controller.CapturePreRemediationHash(
			ctx, fakeClient, restMapper, "ConfigMap", "test-config", "default",
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(hash).To(BeEmpty(), "Resource without .spec should return empty hash")
	})
})

func int32Ptr(i int32) *int32 {
	return &i
}
