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

package enrichment_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func newTestMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Group: "", Version: "v1"},
		{Group: "apps", Version: "v1"},
		{Group: "batch", Version: "v1"},
		{Group: "autoscaling", Version: "v2"},
		{Group: "policy", Version: "v1"},
		{Group: "networking.k8s.io", Version: "v1"},
	})
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}, meta.RESTScopeRoot)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Node"}, meta.RESTScopeRoot)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "autoscaling", Version: "v2", Kind: "HorizontalPodAutoscaler"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "policy", Version: "v1", Kind: "PodDisruptionBudget"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ResourceQuota"}, meta.RESTScopeNamespace)
	return mapper
}

var _ = Describe("LabelDetector Non-Workload Root Owners — Issue #679", func() {

	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("UT-KA-679-001: ConfigMap root owner with Helm managed-by label", func() {
		It("should detect helmManaged=true with zero failedDetections", func() {
			scheme := newFullScheme()

			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "worker-config",
					Namespace: "demo-crashloop-helm",
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "Helm",
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, cm)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			labels, _, err := detector.DetectLabels(ctx, "ConfigMap", "worker-config", "demo-crashloop-helm", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.HelmManaged).To(BeTrue(), "ConfigMap with managed-by: Helm should be detected")
			Expect(labels.FailedDetections).To(BeEmpty(), "no detections should fail for ConfigMap")
		})
	})

	Describe("UT-KA-679-002: Secret root owner with Helm managed-by label", func() {
		It("should detect helmManaged=true with zero failedDetections", func() {
			scheme := newFullScheme()

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tls-cert",
					Namespace: "production",
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "Helm",
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, secret)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			labels, _, err := detector.DetectLabels(ctx, "Secret", "tls-cert", "production", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.HelmManaged).To(BeTrue(), "Secret with managed-by: Helm should be detected")
			Expect(labels.FailedDetections).To(BeEmpty(), "no detections should fail for Secret")
		})
	})

	Describe("UT-KA-679-003: Service root owner with no special labels", func() {
		It("should run all detections with zero failedDetections", func() {
			scheme := newFullScheme()

			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-gateway",
					Namespace: "default",
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, svc)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			labels, _, err := detector.DetectLabels(ctx, "Service", "api-gateway", "default", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.HelmManaged).To(BeFalse())
			Expect(labels.GitOpsManaged).To(BeFalse())
			Expect(labels.FailedDetections).To(BeEmpty(), "no detections should fail for Service")
		})
	})

	Describe("UT-KA-679-004: Node root owner (cluster-scoped)", func() {
		It("should detect serviceMesh annotation; namespace-scoped detections fail for cluster root (#762)", func() {
			scheme := newFullScheme()

			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-1",
					Annotations: map[string]string{
						"sidecar.istio.io/inject": "true",
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, node)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			labels, _, err := detector.DetectLabels(ctx, "Node", "worker-1", "", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.ServiceMesh).To(Equal("istio"), "Node with Istio inject annotation should be detected")
			Expect(labels.FailedDetections).To(ContainElements("hpaEnabled", "pdbProtected", "networkIsolated", "resourceQuotaConstrained"),
				"#762: namespace-scoped detections must be marked failed for cluster-scoped root with empty namespace")
		})
	})

	Describe("UT-KA-679-005: Deployment with only helm.sh/chart label", func() {
		It("should detect helmManaged=true via helm.sh/chart", func() {
			scheme := newFullScheme()

			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-app",
					Namespace: "default",
					Labels: map[string]string{
						"helm.sh/chart": "web-1.0.0",
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "web-app", Namespace: "default"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Deployment", "web-app", "default", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.HelmManaged).To(BeTrue(), "helm.sh/chart label should trigger helmManaged detection")
		})
	})

	Describe("UT-KA-679-006: Deployment with ArgoCD instance label", func() {
		It("should detect gitOpsManaged=true via argocd.argoproj.io/instance label", func() {
			scheme := newFullScheme()

			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-deploy",
					Namespace: "default",
					Labels: map[string]string{
						"argocd.argoproj.io/instance": "my-app",
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "web-deploy", Namespace: "default"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Deployment", "web-deploy", "default", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.GitOpsManaged).To(BeTrue(), "ArgoCD instance label should trigger gitOps detection")
			Expect(labels.GitOpsTool).To(Equal("argocd"))
		})
	})

	Describe("UT-KA-679-007: Deployment with Flux sync-gc-mark label", func() {
		It("should detect gitOpsManaged=true via fluxcd.io/sync-gc-mark label", func() {
			scheme := newFullScheme()

			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-deploy",
					Namespace: "default",
					Labels: map[string]string{
						"fluxcd.io/sync-gc-mark": "sha256:abc123",
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, deploy)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "web-deploy", Namespace: "default"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Deployment", "web-deploy", "default", ownerChain)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.GitOpsManaged).To(BeTrue(), "Flux sync-gc-mark label should trigger gitOps detection")
			Expect(labels.GitOpsTool).To(Equal("flux"))
		})
	})

	Describe("UT-KA-679-008: Full demo-crashloop-helm scenario (ConfigMap with all Helm labels)", func() {
		It("should detect helmManaged=true from realistic demo scenario labels", func() {
			scheme := newFullScheme()

			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "worker-config",
					Namespace: "demo-crashloop-helm",
					Labels: map[string]string{
						"app.kubernetes.io/name":       "demo-crashloop-helm",
						"app.kubernetes.io/instance":   "demo-crashloop-helm",
						"app.kubernetes.io/managed-by": "Helm",
						"helm.sh/chart":                "demo-crashloop-helm-0.1.0",
					},
				},
			}

			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, cm)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			labels, _, err := detector.DetectLabels(ctx, "ConfigMap", "worker-config", "demo-crashloop-helm", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).NotTo(BeNil())
			Expect(labels.HelmManaged).To(BeTrue(), "Full demo scenario ConfigMap should have helmManaged=true")
			Expect(labels.FailedDetections).To(BeEmpty(), "no detections should fail for ConfigMap with Helm labels")
		})
	})

	Describe("Edge: nil mapper produces graceful error", func() {
		It("should return error when mapper is nil", func() {
			scheme := newFullScheme()
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme)
			detector := enrichment.NewLabelDetector(dynClient, nil)

			labels, _, err := detector.DetectLabels(ctx, "ConfigMap", "some-cm", "default", nil)
			Expect(err).NotTo(HaveOccurred(), "DetectLabels should not error — it logs warnings")
			Expect(labels).NotTo(BeNil())
			Expect(labels.FailedDetections).NotTo(BeEmpty(), "all detections fail when mapper is nil")
		})
	})
})
