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

package k8s_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/k8s"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakediscovery "k8s.io/client-go/discovery/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

type targetScopedListCall struct {
	apiVersion    string
	kind          string
	namespace     string
	labelSelector string
	fieldSelector string
}

// targetScopedResolver keeps the regular ResourceResolver contract while
// exposing the stronger exact-GVK list contract used by resources_list.
type targetScopedResolver struct {
	k8s.ResourceResolver
	result interface{}
	err    error
	calls  []targetScopedListCall
}

func (r *targetScopedResolver) ListForAPIVersion(_ context.Context, apiVersion, kind, namespace, labelSelector, fieldSelector string) (interface{}, error) {
	r.calls = append(r.calls, targetScopedListCall{
		apiVersion: apiVersion, kind: kind, namespace: namespace,
		labelSelector: labelSelector, fieldSelector: fieldSelector,
	})
	return r.result, r.err
}

func registerK8sTools(client kubernetes.Interface, resolver k8s.ResourceResolver) *registry.Registry {
	result := registry.New()
	for _, tool := range k8s.NewAllTools(client, resolver) {
		result.Register(tool)
	}
	return result
}

var _ = Describe("Target-scoped Kubernetes discovery tools (#2609)", func() {
	It("UT-KA-2609-003/005/006: rejects malformed or unavailable discovery inputs without dispatch", func() {
		reg := registerK8sTools(nil, nil)

		_, err := reg.Execute(context.Background(), "api_resources_list", json.RawMessage(`{invalid`))
		Expect(err).To(MatchError(ContainSubstring("parsing api resource discovery args")))
		_, err = reg.Execute(context.Background(), "api_resources_list", json.RawMessage(`{"kind":"  "}`))
		Expect(err).To(MatchError(ContainSubstring("kind must not be empty")))
		_, err = reg.Execute(context.Background(), "api_resources_list", json.RawMessage(`{"kind":"Deployment"}`))
		Expect(err).To(MatchError(ContainSubstring("discovery client not configured")))

		_, err = reg.Execute(context.Background(), "resources_list", json.RawMessage(`{invalid`))
		Expect(err).To(MatchError(ContainSubstring("parsing resources_list args")))
		_, err = reg.Execute(context.Background(), "resources_list", json.RawMessage(`{"apiVersion":"apps/v1"}`))
		Expect(err).To(MatchError(ContainSubstring("requires apiVersion and kind")))
		_, err = reg.Execute(context.Background(), "resources_list", json.RawMessage(`{"apiVersion":"apps/v1","kind":"Deployment"}`))
		Expect(err).To(MatchError(ContainSubstring("exact resource resolver not configured")))
	})

	It("UT-KA-2609-002/016 [SI-10]: returns only exact-kind preferred discovery records in stable order", func() {
		client := fakeclientset.NewSimpleClientset()
		discoveryClient := client.Discovery().(*fakediscovery.FakeDiscovery)
		discoveryClient.Resources = []*metav1.APIResourceList{
			{
				GroupVersion: "extensions/v1beta1",
				APIResources: []metav1.APIResource{{Name: "deployments", Kind: "Deployment", Namespaced: true}},
			},
			{
				GroupVersion: "v1",
				APIResources: []metav1.APIResource{{Name: "pods", Kind: "Pod", Namespaced: true}},
			},
			{
				GroupVersion: "apps/v1",
				APIResources: []metav1.APIResource{{Name: "deployments", Kind: "Deployment", Namespaced: true}},
			},
		}

		tool := findToolByName(registerK8sTools(client, nil), "api_resources_list")
		Expect(tool).NotTo(BeNil())
		Expect(tool.Description()).To(ContainSubstring("target cluster"))
		Expect(tool.Parameters()).To(MatchJSON(`{"type":"object","properties":{"kind":{"type":"string","minLength":1,"description":"Required case-insensitive exact Kubernetes Kind filter"}},"required":["kind"]}`))

		result, err := tool.Execute(context.Background(), json.RawMessage(`{"kind":" deployment "}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(MatchJSON(`{"resources":[{"apiVersion":"apps/v1","kind":"Deployment","name":"deployments","namespaced":true},{"apiVersion":"extensions/v1beta1","kind":"Deployment","name":"deployments","namespaced":true}]}`))
	})

	It("UT-KA-2609-005/006: preserves partial-discovery warnings instead of treating them as complete evidence", func() {
		client := fakeclientset.NewSimpleClientset()
		discoveryClient := client.Discovery().(*fakediscovery.FakeDiscovery)
		discoveryClient.Resources = []*metav1.APIResourceList{
			{
				GroupVersion: "apps/v1",
				APIResources: []metav1.APIResource{{Name: "deployments", Kind: "Deployment", Namespaced: true}},
			},
			{
				GroupVersion: "extensions/v1beta1",
				APIResources: []metav1.APIResource{{Name: "deployments", Kind: "Deployment", Namespaced: true}},
			},
		}
		var resourceRequests int32
		discoveryClient.PrependReactor("get", "resource", func(_ k8stesting.Action) (bool, runtime.Object, error) {
			if atomic.AddInt32(&resourceRequests, 1) == 1 {
				return true, nil, errors.New("simulated partial discovery failure")
			}
			return false, nil, nil
		})

		tool := findToolByName(registerK8sTools(client, nil), "api_resources_list")
		result, err := tool.Execute(context.Background(), json.RawMessage(`{"kind":"Deployment"}`))
		Expect(err).NotTo(HaveOccurred())
		var payload struct {
			Resources []map[string]interface{} `json:"resources"`
			Warning   string                   `json:"warning"`
		}
		Expect(json.Unmarshal([]byte(result), &payload)).To(Succeed())
		Expect(payload.Resources).To(HaveLen(1))
		Expect(payload.Warning).To(ContainSubstring("some API groups could not be queried and are omitted from this list"))
	})

	It("UT-KA-2609-009/016 [SI-10]: uses the exact GVK list resolver and propagates its error", func() {
		resolver := &targetScopedResolver{
			ResourceResolver: testResolverWithObjects(),
			result: &k8s.ResourceListSummary{
				APIVersion: "apps/v1",
				Kind:       "DeploymentList",
				Items: []k8s.ResourceMetadataSummary{{Metadata: k8s.ResourceObjectMetadata{
					Name: "api-server", Namespace: "prod",
				}}},
				ItemCount: 1,
				Truncated: false,
			},
		}
		reg := registerK8sTools(nil, resolver)
		tool := findToolByName(reg, "resources_list")
		Expect(tool).NotTo(BeNil())
		Expect(tool.Name()).To(Equal("resources_list"))
		Expect(tool.Description()).To(ContainSubstring("exact apiVersion and kind"))

		result, err := tool.Execute(context.Background(), json.RawMessage(`{"apiVersion":"apps/v1","kind":"Deployment","namespace":"prod","labelSelector":"app=api","fieldSelector":"metadata.name=api-server"}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(MatchJSON(`{"apiVersion":"apps/v1","kind":"DeploymentList","items":[{"metadata":{"name":"api-server","namespace":"prod"}}],"item_count":1,"truncated":false}`))
		Expect(resolver.calls).To(ConsistOf(targetScopedListCall{
			apiVersion: "apps/v1", kind: "Deployment", namespace: "prod",
			labelSelector: "app=api", fieldSelector: "metadata.name=api-server",
		}))

		resolver.err = errors.New("exact target list denied")
		_, err = tool.Execute(context.Background(), json.RawMessage(`{"apiVersion":"apps/v1","kind":"Deployment"}`))
		Expect(err).To(MatchError("exact target list denied"))
	})
})

var _ = Describe("Exact target-scoped metadata resolver (#2609)", func() {
	It("UT-KA-2609-009/016 [SI-10]: lists namespaced and cluster-scoped targets with bounded selectors", func() {
		metadataClient := newMetadataSummaryClient(
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "target-config", Namespace: "prod"},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "target-config", Namespace: "staging"},
			},
		)
		resolver := k8s.NewDynamicResolver(
			dynamicfake.NewSimpleDynamicClient(buildTestScheme()),
			buildTestMapper(), buildTestKindIndex(), logr.Discard(),
			k8s.WithMetadataClient(metadataClient),
		)
		exact, ok := resolver.(k8s.ExactResourceListResolver)
		Expect(ok).To(BeTrue())

		result, err := exact.ListForAPIVersion(context.Background(), "v1", "ConfigMap", "prod", "app=api", "metadata.name=target-config")
		Expect(err).NotTo(HaveOccurred())
		data, marshalErr := json.Marshal(result)
		Expect(marshalErr).NotTo(HaveOccurred())
		Expect(string(data)).To(MatchJSON(`{"apiVersion":"meta.k8s.io/v1","kind":"ConfigMapList","items":[{"metadata":{"name":"target-config","namespace":"prod"}}],"item_count":1,"total_count":1,"truncated":false}`))
		Expect(string(data)).NotTo(ContainSubstring(`"namespace":"staging"`))
		Expect(metadataClient.lastListOptions.Limit).To(Equal(k8s.DefaultResourceListLimit))
		Expect(metadataClient.lastListOptions.LabelSelector).To(Equal("app=api"))
		Expect(metadataClient.lastListOptions.FieldSelector).To(Equal("metadata.name=target-config"))

		clusterMetadataClient := newMetadataSummaryClient(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-1"}})
		clusterResolver := k8s.NewDynamicResolver(
			dynamicfake.NewSimpleDynamicClient(buildTestScheme()),
			buildTestMapper(), buildTestKindIndex(), logr.Discard(),
			k8s.WithMetadataClient(clusterMetadataClient),
		)
		clusterExact, ok := clusterResolver.(k8s.ExactResourceListResolver)
		Expect(ok).To(BeTrue())
		result, err = clusterExact.ListForAPIVersion(context.Background(), "v1", "Node", "ignored", "", "")
		Expect(err).NotTo(HaveOccurred())
		data, marshalErr = json.Marshal(result)
		Expect(marshalErr).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring(`"worker-1"`))
	})

	It("UT-KA-2609-005: fails closed when exact metadata discovery is unavailable or invalid", func() {
		withoutMetadata := k8s.NewDynamicResolver(
			dynamicfake.NewSimpleDynamicClient(buildTestScheme()),
			buildTestMapper(), buildTestKindIndex(), logr.Discard(),
		)
		exact, ok := withoutMetadata.(k8s.ExactResourceListResolver)
		Expect(ok).To(BeTrue())
		_, err := exact.ListForAPIVersion(context.Background(), "v1", "ConfigMap", "prod", "", "")
		Expect(err).To(MatchError(ContainSubstring("metadata client not configured")))

		withMetadata := k8s.NewDynamicResolver(
			dynamicfake.NewSimpleDynamicClient(buildTestScheme()),
			buildTestMapper(), buildTestKindIndex(), logr.Discard(),
			k8s.WithMetadataClient(newMetadataSummaryClient()),
		)
		exact, ok = withMetadata.(k8s.ExactResourceListResolver)
		Expect(ok).To(BeTrue())
		_, err = exact.ListForAPIVersion(context.Background(), "bad/version/extra", "ConfigMap", "prod", "", "")
		Expect(err).To(MatchError(ContainSubstring("invalid apiVersion")))
		_, err = exact.ListForAPIVersion(context.Background(), "v1", "", "prod", "", "")
		Expect(err).To(MatchError(ContainSubstring("kind must not be empty")))
		_, err = exact.ListForAPIVersion(context.Background(), "v1", "UnknownKind", "prod", "", "")
		Expect(err).To(MatchError(ContainSubstring("resolve exact GVK")))

		failing := k8s.NewDynamicResolver(
			dynamicfake.NewSimpleDynamicClient(buildTestScheme()),
			buildTestMapper(), buildTestKindIndex(), logr.Discard(),
			k8s.WithMetadataClient(newFailingMetadataSummaryClient(errors.New("metadata list denied"))),
		)
		exact, ok = failing.(k8s.ExactResourceListResolver)
		Expect(ok).To(BeTrue())
		_, err = exact.ListForAPIVersion(context.Background(), "v1", "ConfigMap", "prod", "", "")
		Expect(err).To(MatchError(ContainSubstring("list exact GVK v1/ConfigMap")))
	})
})
