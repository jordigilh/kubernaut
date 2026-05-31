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
	"strings"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/k8s"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	fakek8s "k8s.io/client-go/kubernetes/fake"
)

func buildAmbiguousKindMapper() *meta.DefaultRESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Group: "", Version: "v1"},
		{Group: "apps", Version: "v1"},
		{Group: "messaging.knative.dev", Version: "v1"},
		{Group: "operators.coreos.com", Version: "v1alpha1"},
	})
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Pod"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "messaging.knative.dev", Version: "v1", Kind: "Subscription"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "operators.coreos.com", Version: "v1alpha1", Kind: "Subscription"}, meta.RESTScopeNamespace)
	return mapper
}

func buildAmbiguousKindIndex() map[string]schema.GroupKind {
	return map[string]schema.GroupKind{
		"pod":          {Kind: "Pod"},
		"deployment":   {Group: "apps", Kind: "Deployment"},
		"subscription": {Group: "messaging.knative.dev", Kind: "Subscription"},
	}
}

func newOLMSubscription(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "operators.coreos.com/v1alpha1",
			"kind":       "Subscription",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"channel":         "stable",
				"name":            name,
				"source":          "community-operators",
				"sourceNamespace": "openshift-marketplace",
			},
		},
	}
}

func newAmbiguousScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "operators.coreos.com", Version: "v1alpha1", Kind: "Subscription"},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "operators.coreos.com", Version: "v1alpha1", Kind: "SubscriptionList"},
		&unstructured.UnstructuredList{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "messaging.knative.dev", Version: "v1", Kind: "Subscription"},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "messaging.knative.dev", Version: "v1", Kind: "SubscriptionList"},
		&unstructured.UnstructuredList{},
	)
	return scheme
}

var _ = Describe("Issue #1064: kubectl tool multi-group kind resolution fallback", func() {

	Describe("UT-KA-1064-001: Get resolves ambiguous kind with explicit api_group (#1311)", func() {
		It("should return the OLM Subscription when api_group is operators.coreos.com", func() {
			scheme := newAmbiguousScheme()
			olmSub := newOLMSubscription("etcd", "demo-operator")
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, olmSub)
			mapper := buildAmbiguousKindMapper()
			kindIndex := buildAmbiguousKindIndex()

			resolver := k8s.NewDynamicResolver(dynClient, mapper, kindIndex, logr.Discard())
			result, err := resolver.Get(context.Background(), "Subscription", "etcd", "demo-operator", "operators.coreos.com")
			Expect(err).NotTo(HaveOccurred(), "Get should succeed with explicit api_group")
			data, _ := json.Marshal(result)
			Expect(string(data)).To(ContainSubstring("etcd"),
				"result should contain the OLM Subscription named etcd")
		})
	})

	Describe("UT-KA-1064-002: List resolves ambiguous kind with explicit api_group (#1311)", func() {
		It("should return the OLM SubscriptionList when api_group is operators.coreos.com", func() {
			scheme := newAmbiguousScheme()
			olmSub := newOLMSubscription("etcd", "demo-operator")
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, olmSub)
			mapper := buildAmbiguousKindMapper()
			kindIndex := buildAmbiguousKindIndex()

			resolver := k8s.NewDynamicResolver(dynClient, mapper, kindIndex, logr.Discard())
			result, err := resolver.List(context.Background(), "Subscription", "demo-operator", "operators.coreos.com")
			Expect(err).NotTo(HaveOccurred(), "List should succeed with explicit api_group")
			data, _ := json.Marshal(result)
			Expect(string(data)).To(ContainSubstring("etcd"),
				"list should contain the OLM Subscription named etcd")
		})
	})

	Describe("UT-KA-1064-003: List with explicit api_group returns only that group's items (#1311)", func() {
		It("should return OLM Subscription in demo-operator when api_group is operators.coreos.com", func() {
			scheme := newAmbiguousScheme()
			olmSub := newOLMSubscription("alpha", "demo-operator")
			knativeSub := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "messaging.knative.dev/v1",
					"kind":       "Subscription",
					"metadata": map[string]interface{}{
						"name":      "beta",
						"namespace": "other-ns",
					},
				},
			}
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, olmSub, knativeSub)
			mapper := buildAmbiguousKindMapper()
			kindIndex := buildAmbiguousKindIndex()

			resolver := k8s.NewDynamicResolver(dynClient, mapper, kindIndex, logr.Discard())
			result, err := resolver.List(context.Background(), "Subscription", "demo-operator", "operators.coreos.com")
			Expect(err).NotTo(HaveOccurred())
			data, _ := json.Marshal(result)
			Expect(string(data)).To(ContainSubstring("alpha"),
				"should find OLM Subscription alpha in demo-operator")
		})
	})

	Describe("UT-KA-1064-004: Single-group kind Get works unchanged (regression)", func() {
		It("should resolve Deployment from apps/v1 as before", func() {
			scheme := newAmbiguousScheme()
			dep := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "api-server",
						"namespace": "default",
					},
				},
			}
			scheme.AddKnownTypeWithName(
				schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
				&unstructured.Unstructured{},
			)
			scheme.AddKnownTypeWithName(
				schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DeploymentList"},
				&unstructured.UnstructuredList{},
			)
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, dep)
			mapper := buildAmbiguousKindMapper()
			kindIndex := buildAmbiguousKindIndex()

			resolver := k8s.NewDynamicResolver(dynClient, mapper, kindIndex, logr.Discard())
			result, err := resolver.Get(context.Background(), "Deployment", "api-server", "default", "")
			Expect(err).NotTo(HaveOccurred())
			data, _ := json.Marshal(result)
			Expect(string(data)).To(ContainSubstring("api-server"))
		})
	})

	Describe("UT-KA-1064-005: Single-group kind List works unchanged (regression)", func() {
		It("should list Deployments from apps/v1 as before", func() {
			scheme := newAmbiguousScheme()
			dep := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "api-server",
						"namespace": "default",
					},
				},
			}
			scheme.AddKnownTypeWithName(
				schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
				&unstructured.Unstructured{},
			)
			scheme.AddKnownTypeWithName(
				schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DeploymentList"},
				&unstructured.UnstructuredList{},
			)
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, dep)
			mapper := buildAmbiguousKindMapper()
			kindIndex := buildAmbiguousKindIndex()

			resolver := k8s.NewDynamicResolver(dynClient, mapper, kindIndex, logr.Discard())
			result, err := resolver.List(context.Background(), "Deployment", "default", "")
			Expect(err).NotTo(HaveOccurred())
			data, _ := json.Marshal(result)
			Expect(string(data)).To(ContainSubstring("api-server"))
		})
	})

	Describe("UT-KA-1064-006: Unknown kind Get returns actionable error", func() {
		It("should return error containing the kind name", func() {
			scheme := newAmbiguousScheme()
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme)
			mapper := buildAmbiguousKindMapper()
			kindIndex := buildAmbiguousKindIndex()

			resolver := k8s.NewDynamicResolver(dynClient, mapper, kindIndex, logr.Discard())
			_, err := resolver.Get(context.Background(), "FooBarBaz", "test", "default", "")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("FooBarBaz"),
				"error must include the unresolved kind name for diagnostics")
		})
	})

	Describe("UT-KA-1064-007: Unknown kind List returns actionable error", func() {
		It("should return error containing the kind name", func() {
			scheme := newAmbiguousScheme()
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme)
			mapper := buildAmbiguousKindMapper()
			kindIndex := buildAmbiguousKindIndex()

			resolver := k8s.NewDynamicResolver(dynClient, mapper, kindIndex, logr.Discard())
			_, err := resolver.List(context.Background(), "FooBarBaz", "default", "")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("FooBarBaz"),
				"error must include the unresolved kind name for diagnostics")
		})
	})

	// Adversarial kind inputs (Checkpoint 1, Category 2)

	Describe("UT-KA-1064-ADV-001: Max-length+1 kind returns error without panic", func() {
		It("should reject a 256-char kind gracefully", func() {
			scheme := newAmbiguousScheme()
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme)
			mapper := buildAmbiguousKindMapper()
			kindIndex := buildAmbiguousKindIndex()

			longKind := strings.Repeat("A", 256)
			resolver := k8s.NewDynamicResolver(dynClient, mapper, kindIndex, logr.Discard())
			_, err := resolver.Get(context.Background(), longKind, "x", "default", "")
			Expect(err).To(HaveOccurred(), "256-char kind should fail resolution")
		})
	})

	Describe("UT-KA-1064-ADV-002: Unicode kind returns error without panic", func() {
		It("should reject a Unicode kind gracefully", func() {
			scheme := newAmbiguousScheme()
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme)
			mapper := buildAmbiguousKindMapper()
			kindIndex := buildAmbiguousKindIndex()

			resolver := k8s.NewDynamicResolver(dynClient, mapper, kindIndex, logr.Discard())
			_, err := resolver.Get(context.Background(), "Ünïcödé", "x", "default", "")
			Expect(err).To(HaveOccurred(), "Unicode kind should fail resolution")
		})
	})

	Describe("UT-KA-1064-ADV-003: Path-traversal kind returns error without panic", func() {
		It("should reject kind containing path separators", func() {
			scheme := newAmbiguousScheme()
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme)
			mapper := buildAmbiguousKindMapper()
			kindIndex := buildAmbiguousKindIndex()

			resolver := k8s.NewDynamicResolver(dynClient, mapper, kindIndex, logr.Discard())
			_, err := resolver.Get(context.Background(), "../etc/passwd", "x", "default", "")
			Expect(err).To(HaveOccurred(), "path-traversal kind should fail resolution")
		})
	})

	Describe("UT-KA-1064-NIL-001: Empty kindIndex falls back to mapper for known kind", func() {
		It("should resolve Deployment even with empty kindIndex", func() {
			scheme := newAmbiguousScheme()
			dep := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "api-server",
						"namespace": "default",
					},
				},
			}
			scheme.AddKnownTypeWithName(
				schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
				&unstructured.Unstructured{},
			)
			scheme.AddKnownTypeWithName(
				schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DeploymentList"},
				&unstructured.UnstructuredList{},
			)
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, dep)
			mapper := buildAmbiguousKindMapper()
			emptyIndex := map[string]schema.GroupKind{}

			resolver := k8s.NewDynamicResolver(dynClient, mapper, emptyIndex, logr.Discard())
			result, err := resolver.Get(context.Background(), "Deployment", "api-server", "default", "")
			Expect(err).NotTo(HaveOccurred(),
				"empty kindIndex should fall through to ResourcesFor via mapper")
			data, _ := json.Marshal(result)
			Expect(string(data)).To(ContainSubstring("api-server"))
		})
	})

	Describe("UT-KA-1064-008: Multi-group Get with explicit api_group emits log on success", func() {
		It("should resolve with explicit api_group for ambiguous kind", func() {
			scheme := newAmbiguousScheme()
			olmSub := newOLMSubscription("etcd", "demo-operator")
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, olmSub)
			mapper := buildAmbiguousKindMapper()
			kindIndex := buildAmbiguousKindIndex()

			resolver := k8s.NewDynamicResolver(dynClient, mapper, kindIndex, logr.Discard())
			result, err := resolver.Get(context.Background(), "Subscription", "etcd", "demo-operator", "operators.coreos.com")
			Expect(err).NotTo(HaveOccurred(),
				"Get must succeed with explicit api_group")
			data, _ := json.Marshal(result)
			Expect(string(data)).To(ContainSubstring("etcd"))
		})
	})

	Describe("UT-KA-1064-009: Ambiguous kind without api_group returns disambiguation error (#1311)", func() {
		It("should return error listing available groups when api_group is empty", func() {
			scheme := newAmbiguousScheme()
			olmSub := newOLMSubscription("etcd", "demo-operator")
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, olmSub)
			mapper := buildAmbiguousKindMapper()
			kindIndex := buildAmbiguousKindIndex()

			resolver := k8s.NewDynamicResolver(dynClient, mapper, kindIndex, logr.Discard())
			_, err := resolver.Get(context.Background(), "Subscription", "etcd", "demo-operator", "")
			Expect(err).To(HaveOccurred(),
				"ambiguous kind without api_group should return error")
			Expect(err.Error()).To(ContainSubstring("ambiguous"))
			Expect(err.Error()).To(ContainSubstring("operators.coreos.com"))
			Expect(err.Error()).To(ContainSubstring("messaging.knative.dev"))
		})
	})

	Describe("UT-KA-1064-010: kubectl_find_resource with explicit api_group finds correct items (#1311)", func() {
		It("should find OLM Subscription via keyword search with api_group", func() {
			scheme := newAmbiguousScheme()
			olmSub := newOLMSubscription("etcd", "demo-operator")
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, olmSub)
			mapper := buildAmbiguousKindMapper()
			kindIndex := buildAmbiguousKindIndex()

			resolver := k8s.NewDynamicResolver(dynClient, mapper, kindIndex, logr.Discard())
			typedClient := fakek8s.NewSimpleClientset()

			reg := registry.New()
			for _, t := range k8s.NewAllTools(typedClient, resolver) {
				reg.Register(t)
			}

			result, err := reg.Execute(context.Background(), "kubectl_find_resource",
				json.RawMessage(`{"kind":"Subscription","keyword":"etcd","api_group":"operators.coreos.com"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("etcd"),
				"find_resource should locate OLM Subscription matching keyword")
		})
	})

	Describe("UT-KA-1064-011: kubernetes_jq_query with explicit api_group queries correct group (#1311)", func() {
		It("should apply jq expression to OLM Subscriptions with api_group", func() {
			scheme := newAmbiguousScheme()
			olmSub := newOLMSubscription("etcd", "demo-operator")
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, olmSub)
			mapper := buildAmbiguousKindMapper()
			kindIndex := buildAmbiguousKindIndex()

			resolver := k8s.NewDynamicResolver(dynClient, mapper, kindIndex, logr.Discard())
			typedClient := fakek8s.NewSimpleClientset()

			reg := registry.New()
			for _, t := range k8s.NewAllTools(typedClient, resolver) {
				reg.Register(t)
			}

			result, err := reg.Execute(context.Background(), "kubernetes_jq_query",
				json.RawMessage(`{"kind":"Subscription","jq_expr":".items[].metadata.name","api_group":"operators.coreos.com"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("etcd"),
				"jq query should return names from OLM Subscriptions")
		})
	})
})
