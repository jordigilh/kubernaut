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
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/k8s"
)

// failResourcesForMapper wraps a RESTMapper and forces ResourcesFor to return an error,
// simulating a stale discovery cache scenario. All other methods delegate to the real mapper.
// Validated in PoC 7 and PoC 10.
type failResourcesForMapper struct {
	delegate meta.RESTMapper
}

func (m *failResourcesForMapper) KindFor(resource schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	return m.delegate.KindFor(resource)
}
func (m *failResourcesForMapper) KindsFor(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	return m.delegate.KindsFor(resource)
}
func (m *failResourcesForMapper) ResourceFor(input schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	return m.delegate.ResourceFor(input)
}
func (m *failResourcesForMapper) ResourcesFor(input schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	return nil, fmt.Errorf("simulated stale cache: ResourcesFor failed for %s", input.Resource)
}
func (m *failResourcesForMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	return m.delegate.RESTMapping(gk, versions...)
}
func (m *failResourcesForMapper) RESTMappings(gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error) {
	return m.delegate.RESTMappings(gk, versions...)
}
func (m *failResourcesForMapper) ResourceSingularizer(resource string) (string, error) {
	return m.delegate.ResourceSingularizer(resource)
}

const (
	istioGroup   = "security.istio.io"
	istioV1Beta1 = "v1beta1"
	istioV1      = "v1"
	apKind       = "AuthorizationPolicy"
)

func buildMultiVersionMapper() *meta.DefaultRESTMapper {
	gvBeta := schema.GroupVersion{Group: istioGroup, Version: istioV1Beta1}
	gvStable := schema.GroupVersion{Group: istioGroup, Version: istioV1}
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{gvBeta, gvStable})
	mapper.Add(gvBeta.WithKind(apKind), meta.RESTScopeNamespace)
	mapper.Add(gvStable.WithKind(apKind), meta.RESTScopeNamespace)
	return mapper
}

func buildMultiVersionScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	for _, v := range []string{istioV1Beta1, istioV1} {
		scheme.AddKnownTypeWithName(
			schema.GroupVersionKind{Group: istioGroup, Version: v, Kind: apKind},
			&unstructured.Unstructured{},
		)
		scheme.AddKnownTypeWithName(
			schema.GroupVersionKind{Group: istioGroup, Version: v, Kind: apKind + "List"},
			&unstructured.UnstructuredList{},
		)
	}
	return scheme
}

func buildMultiVersionKindIndex() map[string]schema.GroupKind {
	return map[string]schema.GroupKind{
		"authorizationpolicy": {Group: istioGroup, Kind: apKind},
	}
}

func newAuthzPolicy(name, namespace, version string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": istioGroup + "/" + version,
			"kind":       apKind,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"action": "DENY",
			},
		},
	}
}

func captureLogger() (logr.Logger, *[]string) {
	var logs []string
	logger := funcr.New(func(prefix, args string) {
		logs = append(logs, prefix+" "+args)
	}, funcr.Options{Verbosity: 1})
	return logger, &logs
}

var _ = Describe("Issue #1064 follow-up: multi-version kind resolution fallback", func() {

	Describe("UT-MVR-001: resolveMappings fallback returns all versions when ResourcesFor fails", func() {
		It("should resolve both v1beta1 and v1 mappings via RESTMappings fallback", func() {
			scheme := buildMultiVersionScheme()
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme)
			mapper := buildMultiVersionMapper()
			kindIndex := buildMultiVersionKindIndex()
			wrappedMapper := &failResourcesForMapper{delegate: mapper}

			resolver := k8s.NewDynamicResolver(dynClient, wrappedMapper, kindIndex, logr.Discard())

			// List exercises resolveMappings; if only 1 mapping returned, we'd get
			// an empty v1beta1 list. With 2 mappings, we'd get "no results from any
			// API group" or an empty list from the last version tried.
			// The key assertion: the resolver does NOT return an "unsupported kind" error,
			// which would indicate RESTMapping(gk) failed with AmbiguousKindError.
			_, err := resolver.List(context.Background(), apKind, "test-ns")
			Expect(err).NotTo(HaveOccurred(),
				"resolveMappings fallback should return mappings via RESTMappings, not fail with unsupported kind")
		})
	})

	Describe("UT-MVR-002: List returns v1 items when v1beta1 is empty (fallback mapper)", func() {
		It("should find AuthorizationPolicy in v1 after v1beta1 returns empty list", func() {
			scheme := buildMultiVersionScheme()
			ap := newAuthzPolicy("deny-all-traffic", "demo-mesh-failure", istioV1)
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, ap)
			mapper := buildMultiVersionMapper()
			kindIndex := buildMultiVersionKindIndex()
			wrappedMapper := &failResourcesForMapper{delegate: mapper}

			resolver := k8s.NewDynamicResolver(dynClient, wrappedMapper, kindIndex, logr.Discard())
			result, err := resolver.List(context.Background(), apKind, "demo-mesh-failure")
			Expect(err).NotTo(HaveOccurred(), "List should succeed via multi-version fallback")
			data, _ := json.Marshal(result)
			Expect(string(data)).To(ContainSubstring("deny-all-traffic"),
				"should find AuthorizationPolicy deny-all-traffic from v1")
		})
	})

	Describe("UT-MVR-003: Get returns v1 resource when v1beta1 is NotFound (fallback mapper)", func() {
		It("should find AuthorizationPolicy in v1 after v1beta1 returns NotFound", func() {
			scheme := buildMultiVersionScheme()
			ap := newAuthzPolicy("deny-all-traffic", "demo-mesh-failure", istioV1)
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, ap)
			mapper := buildMultiVersionMapper()
			kindIndex := buildMultiVersionKindIndex()
			wrappedMapper := &failResourcesForMapper{delegate: mapper}

			resolver := k8s.NewDynamicResolver(dynClient, wrappedMapper, kindIndex, logr.Discard())
			result, err := resolver.Get(context.Background(), apKind, "deny-all-traffic", "demo-mesh-failure")
			Expect(err).NotTo(HaveOccurred(), "Get should succeed via multi-version fallback")
			data, _ := json.Marshal(result)
			Expect(string(data)).To(ContainSubstring("deny-all-traffic"),
				"should find AuthorizationPolicy deny-all-traffic from v1")
		})
	})

	Describe("UT-MVR-004: Fallback uses kindIndex group hint for CRD kinds", func() {
		It("should use security.istio.io group from kindIndex when resolving AuthorizationPolicy", func() {
			scheme := buildMultiVersionScheme()
			ap := newAuthzPolicy("test-policy", "default", istioV1)
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, ap)
			mapper := buildMultiVersionMapper()
			kindIndex := buildMultiVersionKindIndex()
			wrappedMapper := &failResourcesForMapper{delegate: mapper}

			resolver := k8s.NewDynamicResolver(dynClient, wrappedMapper, kindIndex, logr.Discard())
			result, err := resolver.Get(context.Background(), apKind, "test-policy", "default")
			Expect(err).NotTo(HaveOccurred(),
				"kindIndex should provide the security.istio.io group hint for RESTMappings")
			data, _ := json.Marshal(result)
			Expect(string(data)).To(ContainSubstring("test-policy"))
		})
	})

	Describe("UT-MVR-005: Fallback with empty group fails gracefully for CRD kinds", func() {
		It("should return an error when kindIndex is empty and ResourcesFor fails for CRD kind", func() {
			scheme := buildMultiVersionScheme()
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme)
			mapper := buildMultiVersionMapper()
			emptyKindIndex := map[string]schema.GroupKind{}
			wrappedMapper := &failResourcesForMapper{delegate: mapper}

			resolver := k8s.NewDynamicResolver(dynClient, wrappedMapper, emptyKindIndex, logr.Discard())
			_, err := resolver.List(context.Background(), apKind, "test-ns")
			Expect(err).To(HaveOccurred(),
				"empty kindIndex with failed ResourcesFor should produce an error for CRD kinds")
			Expect(err.Error()).To(ContainSubstring("unsupported kind"),
				"error should indicate the kind is unsupported")
		})
	})

	Describe("UT-MVR-006: Multi-group resolution regression (Subscription)", func() {
		It("should still resolve Subscription across operators.coreos.com and messaging.knative.dev", func() {
			scheme := newAmbiguousScheme()
			olmSub := newOLMSubscription("etcd", "demo-operator")
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, olmSub)
			mapper := buildAmbiguousKindMapper()
			kindIndex := buildAmbiguousKindIndex()

			resolver := k8s.NewDynamicResolver(dynClient, mapper, kindIndex, logr.Discard())
			result, err := resolver.Get(context.Background(), "Subscription", "etcd", "demo-operator")
			Expect(err).NotTo(HaveOccurred(), "multi-group resolution should still work")
			data, _ := json.Marshal(result)
			Expect(string(data)).To(ContainSubstring("etcd"),
				"should find the OLM Subscription named etcd")
		})
	})

	Describe("UT-MVR-007: Fallback log emitted at V(1) with version count", func() {
		It("should emit a structured log entry when fallback resolves multiple versions", func() {
			scheme := buildMultiVersionScheme()
			ap := newAuthzPolicy("deny-all", "ns", istioV1)
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, ap)
			mapper := buildMultiVersionMapper()
			kindIndex := buildMultiVersionKindIndex()
			wrappedMapper := &failResourcesForMapper{delegate: mapper}
			logger, logs := captureLogger()

			resolver := k8s.NewDynamicResolver(dynClient, wrappedMapper, kindIndex, logger)
			_, err := resolver.List(context.Background(), apKind, "ns")
			Expect(err).NotTo(HaveOccurred())

			hasLog := false
			for _, entry := range *logs {
				if strings.Contains(entry, "fallback") || strings.Contains(entry, "multi-version") {
					hasLog = true
					break
				}
			}
			Expect(hasLog).To(BeTrue(),
				"fallback resolution should emit a structured log entry; got logs: %v", *logs)
		})
	})

	Describe("UT-MVR-ADV-001: Fallback with single version in kindIndex (no multi-version)", func() {
		It("should return a single mapping and resolve normally for single-version kinds", func() {
			gv := schema.GroupVersion{Group: "apps", Version: "v1"}
			mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{gv})
			mapper.Add(gv.WithKind("Deployment"), meta.RESTScopeNamespace)

			scheme := runtime.NewScheme()
			scheme.AddKnownTypeWithName(gv.WithKind("Deployment"), &unstructured.Unstructured{})
			scheme.AddKnownTypeWithName(gv.WithKind("DeploymentList"), &unstructured.UnstructuredList{})

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
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme, dep)
			kindIndex := map[string]schema.GroupKind{
				"deployment": {Group: "apps", Kind: "Deployment"},
			}
			wrappedMapper := &failResourcesForMapper{delegate: mapper}

			resolver := k8s.NewDynamicResolver(dynClient, wrappedMapper, kindIndex, logr.Discard())
			result, err := resolver.Get(context.Background(), "Deployment", "api-server", "default")
			Expect(err).NotTo(HaveOccurred(),
				"single-version fallback should work normally via RESTMappings")
			data, _ := json.Marshal(result)
			Expect(string(data)).To(ContainSubstring("api-server"))
		})
	})

	Describe("UT-MVR-ADV-002: Fallback with nil kindIndex", func() {
		It("should return an error when kindIndex is nil and ResourcesFor fails for CRD kind", func() {
			scheme := buildMultiVersionScheme()
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme)
			mapper := buildMultiVersionMapper()
			wrappedMapper := &failResourcesForMapper{delegate: mapper}

			resolver := k8s.NewDynamicResolver(dynClient, wrappedMapper, nil, logr.Discard())
			_, err := resolver.List(context.Background(), apKind, "test-ns")
			Expect(err).To(HaveOccurred(),
				"nil kindIndex with failed ResourcesFor should produce an error for CRD kinds")
		})
	})
})
