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

package datastorage

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
)

// ========================================
// DD-WE-006: Dependency Validator Unit Tests
// Tests cover DS registration-time validation of Secret/ConfigMap existence
// ========================================

var _ = Describe("K8sDependencyValidator (DD-WE-006)", func() {
	var (
		ctx       context.Context
		scheme    *runtime.Scheme
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		namespace = "kubernaut-workflows"
	})

	It("UT-DS-006-030: should pass when all declared secrets exist with data", func() {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "gitea-repo-creds", Namespace: namespace},
			Data:       map[string][]byte{"username": []byte("kubernaut"), "password": []byte("pass")},
		}
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
		validator := validation.NewK8sDependencyValidator(k8sClient)

		deps := &models.WorkflowDependencies{
			Secrets: []models.ResourceDependency{{Name: "gitea-repo-creds"}},
		}
		Expect(validator.ValidateDependencies(ctx, namespace, deps)).To(Succeed())
	})

	It("UT-DS-006-031: should fail when a declared secret does not exist", func() {
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		validator := validation.NewK8sDependencyValidator(k8sClient)

		deps := &models.WorkflowDependencies{
			Secrets: []models.ResourceDependency{{Name: "missing-secret"}},
		}
		err := validator.ValidateDependencies(ctx, namespace, deps)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing-secret"))
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("UT-DS-006-032: should fail when a declared secret has empty data", func() {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "empty-secret", Namespace: namespace},
			Data:       map[string][]byte{},
		}
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
		validator := validation.NewK8sDependencyValidator(k8sClient)

		deps := &models.WorkflowDependencies{
			Secrets: []models.ResourceDependency{{Name: "empty-secret"}},
		}
		err := validator.ValidateDependencies(ctx, namespace, deps)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("empty data"))
	})

	It("UT-DS-006-033: should pass when all declared configMaps exist with data", func() {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "remediation-config", Namespace: namespace},
			Data:       map[string]string{"threshold": "0.8"},
		}
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
		validator := validation.NewK8sDependencyValidator(k8sClient)

		deps := &models.WorkflowDependencies{
			ConfigMaps: []models.ResourceDependency{{Name: "remediation-config"}},
		}
		Expect(validator.ValidateDependencies(ctx, namespace, deps)).To(Succeed())
	})

	It("UT-DS-006-034: should fail when a declared configMap does not exist", func() {
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		validator := validation.NewK8sDependencyValidator(k8sClient)

		deps := &models.WorkflowDependencies{
			ConfigMaps: []models.ResourceDependency{{Name: "missing-cm"}},
		}
		err := validator.ValidateDependencies(ctx, namespace, deps)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing-cm"))
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("UT-DS-006-035: should fail when a declared configMap has empty data", func() {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "empty-cm", Namespace: namespace},
			Data:       map[string]string{},
		}
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
		validator := validation.NewK8sDependencyValidator(k8sClient)

		deps := &models.WorkflowDependencies{
			ConfigMaps: []models.ResourceDependency{{Name: "empty-cm"}},
		}
		err := validator.ValidateDependencies(ctx, namespace, deps)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("empty data"))
	})

	It("UT-DS-006-036: should pass with nil dependencies", func() {
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		validator := validation.NewK8sDependencyValidator(k8sClient)

		Expect(validator.ValidateDependencies(ctx, namespace, nil)).To(Succeed())
	})

	It("UT-DS-006-037: should validate both secrets and configMaps together", func() {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "gitea-repo-creds", Namespace: namespace},
			Data:       map[string][]byte{"username": []byte("kubernaut")},
		}
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "remediation-config", Namespace: namespace},
			Data:       map[string]string{"threshold": "0.8"},
		}
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret, cm).Build()
		validator := validation.NewK8sDependencyValidator(k8sClient)

		deps := &models.WorkflowDependencies{
			Secrets:    []models.ResourceDependency{{Name: "gitea-repo-creds"}},
			ConfigMaps: []models.ResourceDependency{{Name: "remediation-config"}},
		}
		Expect(validator.ValidateDependencies(ctx, namespace, deps)).To(Succeed())
	})

	It("UT-DS-006-038: should accept configMap with binary data only", func() {
		cm := &corev1.ConfigMap{
			ObjectMeta:  metav1.ObjectMeta{Name: "binary-config", Namespace: namespace},
			BinaryData:  map[string][]byte{"cert.pem": []byte("binary-cert-data")},
		}
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
		validator := validation.NewK8sDependencyValidator(k8sClient)

		deps := &models.WorkflowDependencies{
			ConfigMaps: []models.ResourceDependency{{Name: "binary-config"}},
		}
		Expect(validator.ValidateDependencies(ctx, namespace, deps)).To(Succeed())
	})

	It("UT-DS-006-039: should report the specific failing resource when one passes and one fails", func() {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "gitea-repo-creds", Namespace: namespace},
			Data:       map[string][]byte{"username": []byte("kubernaut")},
		}
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
		validator := validation.NewK8sDependencyValidator(k8sClient)

		deps := &models.WorkflowDependencies{
			Secrets:    []models.ResourceDependency{{Name: "gitea-repo-creds"}},
			ConfigMaps: []models.ResourceDependency{{Name: "missing-config"}},
		}
		err := validator.ValidateDependencies(ctx, namespace, deps)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing-config"),
			"error should name the specific failing resource")
		Expect(err.Error()).ToNot(ContainSubstring("gitea-repo-creds"),
			"passing resources should not appear in the error")
	})
})
