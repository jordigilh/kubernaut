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

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakedynamic "k8s.io/client-go/dynamic/fake"
	"k8s.io/utils/ptr"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
)

var _ = Describe("TP-762: Scope-Aware K8sAdapter (#762)", func() {

	Describe("UT-KA-762-001: GetOwnerChain uses cluster-scoped client for Node", func() {
		It("should succeed for a Node even when a non-empty namespace is passed", func() {
			scheme := runtime.NewScheme()

			node := &unstructured.Unstructured{}
			node.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Node"})
			node.SetName("worker-1")

			dynClient := fakedynamic.NewSimpleDynamicClient(scheme, node)
			mapper := newSimpleRESTMapper()

			adapter := enrichment.NewK8sAdapter(dynClient, mapper)
			chain, err := adapter.GetOwnerChain(context.Background(), "Node", "worker-1", "kube-system")
			Expect(err).NotTo(HaveOccurred(),
				"UT-KA-762-001: GetOwnerChain must use cluster-scoped client for Node, ignoring namespace")
			Expect(chain).To(BeEmpty())
		})
	})

	Describe("UT-KA-762-002: GetOwnerChain uses namespaced client for Deployment", func() {
		It("should use namespaced client and resolve owner chain correctly", func() {
			scheme := runtime.NewScheme()

			pod := &unstructured.Unstructured{}
			pod.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"})
			pod.SetName("web-abc123")
			pod.SetNamespace("production")
			pod.SetOwnerReferences([]metav1.OwnerReference{
				{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "web-rs", UID: "uid-rs", Controller: ptr.To(true)},
			})

			rs := &unstructured.Unstructured{}
			rs.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"})
			rs.SetName("web-rs")
			rs.SetNamespace("production")
			rs.SetOwnerReferences([]metav1.OwnerReference{
				{APIVersion: "apps/v1", Kind: "Deployment", Name: "web", UID: "uid-d", Controller: ptr.To(true)},
			})

			deploy := &unstructured.Unstructured{}
			deploy.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"})
			deploy.SetName("web")
			deploy.SetNamespace("production")

			dynClient := fakedynamic.NewSimpleDynamicClient(scheme, pod, rs, deploy)
			mapper := newSimpleRESTMapper()

			adapter := enrichment.NewK8sAdapter(dynClient, mapper)
			chain, err := adapter.GetOwnerChain(context.Background(), "Pod", "web-abc123", "production")
			Expect(err).NotTo(HaveOccurred())
			Expect(chain).To(HaveLen(2),
				"UT-KA-762-002: namespaced chain should resolve normally")
			Expect(chain[1].Kind).To(Equal("Deployment"))
		})
	})

	Describe("UT-KA-762-003: GetSpecHash uses cluster-scoped client for Node", func() {
		It("should compute spec hash for a cluster-scoped Node even with non-empty namespace", func() {
			scheme := runtime.NewScheme()

			node := &unstructured.Unstructured{}
			node.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Node"})
			node.SetName("worker-1")
			node.Object["spec"] = map[string]interface{}{
				"providerID": "aws:///us-east-1a/i-1234567890",
			}

			dynClient := fakedynamic.NewSimpleDynamicClient(scheme, node)
			mapper := newSimpleRESTMapper()

			adapter := enrichment.NewK8sAdapter(dynClient, mapper)
			hash, err := adapter.GetSpecHash(context.Background(), "Node", "worker-1", "kube-system")
			Expect(err).NotTo(HaveOccurred(),
				"UT-KA-762-003: GetSpecHash must use cluster-scoped client for Node, ignoring namespace")
			Expect(hash).NotTo(BeEmpty(), "Node has a .spec, so hash should be non-empty")
		})
	})

	Describe("UT-KA-762-004: GetSpecHash uses namespaced client for Deployment", func() {
		It("should compute spec hash for a namespaced Deployment", func() {
			scheme := runtime.NewScheme()

			deploy := &unstructured.Unstructured{}
			deploy.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"})
			deploy.SetName("api-server")
			deploy.SetNamespace("default")
			deploy.Object["spec"] = map[string]interface{}{
				"replicas": int64(3),
			}

			dynClient := fakedynamic.NewSimpleDynamicClient(scheme, deploy)
			mapper := newSimpleRESTMapper()

			adapter := enrichment.NewK8sAdapter(dynClient, mapper)
			hash, err := adapter.GetSpecHash(context.Background(), "Deployment", "api-server", "default")
			Expect(err).NotTo(HaveOccurred(),
				"UT-KA-762-004: namespaced GetSpecHash should work for Deployment")
			Expect(hash).NotTo(BeEmpty())
		})
	})
})

var _ = Describe("TP-762: Scope-Aware LabelDetector (#762)", func() {

	Describe("UT-KA-762-005: LabelDetector uses scope-aware client for Node", func() {
		It("should fetch cluster-scoped Node via LabelDetector.DetectLabels without error", func() {
			scheme := runtime.NewScheme()

			node := &unstructured.Unstructured{}
			node.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Node"})
			node.SetName("worker-1")
			node.SetAnnotations(map[string]string{
				"node.kubernetes.io/instance-type": "m5.xlarge",
			})

			dynClient := fakedynamic.NewSimpleDynamicClientWithCustomListKinds(scheme,
				map[schema.GroupVersionResource]string{
					{Group: "", Version: "v1", Resource: "nodes"}:                                          "NodeList",
					{Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"}:             "HorizontalPodAutoscalerList",
					{Group: "policy", Version: "v1", Resource: "poddisruptionbudgets"}:                     "PodDisruptionBudgetList",
					{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}:                "NetworkPolicyList",
					{Group: "", Version: "v1", Resource: "resourcequotas"}:                                  "ResourceQuotaList",
				}, node)
			mapper := newSimpleRESTMapper()
			addLabelDetectorKinds(mapper)

			ld := enrichment.NewLabelDetector(dynClient, mapper)
			labels, _, err := ld.DetectLabels(context.Background(), "Node", "worker-1", "kube-system", nil)
			Expect(err).NotTo(HaveOccurred(),
				"UT-KA-762-005: LabelDetector must fetch Node using cluster-scoped client")
			Expect(labels).NotTo(BeNil())
		})
	})

	Describe("UT-KA-762-006: LabelDetector skips namespace-scoped lists for cluster-scoped root", func() {
		It("should mark HPA/PDB/NP/RQ as failed detections when rootNS is empty", func() {
			scheme := runtime.NewScheme()

			node := &unstructured.Unstructured{}
			node.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Node"})
			node.SetName("worker-1")

			dynClient := fakedynamic.NewSimpleDynamicClient(scheme, node)
			mapper := newSimpleRESTMapper()
			addLabelDetectorKinds(mapper)

			ld := enrichment.NewLabelDetector(dynClient, mapper)
			labels, _, err := ld.DetectLabels(context.Background(), "Node", "worker-1", "", nil)
			Expect(err).NotTo(HaveOccurred(),
				"UT-KA-762-006: should not error when namespace is empty")
			Expect(labels).NotTo(BeNil())
			Expect(labels.FailedDetections).To(ContainElements("hpaEnabled", "pdbProtected", "networkIsolated", "resourceQuotaConstrained"),
				"UT-KA-762-006: namespace-scoped detections should be marked failed for cluster-scoped root")
		})
	})
})

var _ = Describe("TP-762: DS Adapter 400 Error Surfacing (#762)", func() {

	Describe("UT-KA-762-008: DS adapter returns error on 400 Bad Request", func() {
		It("should return an error instead of silently swallowing 400 responses", func() {
			client := &stubDSClient{
				response: &ogenclient.GetRemediationHistoryContextBadRequest{},
			}

			adapter := enrichment.NewDSAdapter(client)
			result, err := adapter.GetRemediationHistory(context.Background(), "Deployment", "api-server", "default", "")
			Expect(err).To(HaveOccurred(),
				"UT-KA-762-008: DS adapter must return error on 400, not silently swallow")
			Expect(err.Error()).To(ContainSubstring("bad request"))
			Expect(result).To(BeNil())
		})
	})
})

func addLabelDetectorKinds(mapper *meta.DefaultRESTMapper) {
	mapper.Add(schema.GroupVersionKind{Group: "autoscaling", Version: "v2", Kind: "HorizontalPodAutoscaler"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "policy", Version: "v1", Kind: "PodDisruptionBudget"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ResourceQuota"}, meta.RESTScopeNamespace)
}
