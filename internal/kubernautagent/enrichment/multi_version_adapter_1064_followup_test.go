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
	fakedynamic "k8s.io/client-go/dynamic/fake"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
)

// failResourcesForAdapterMapper wraps a RESTMapper and forces ResourcesFor to
// return an error, simulating a stale discovery cache scenario where
// ResourcesFor cannot resolve the resource name to GVRs.
// All other methods delegate to the real mapper.
type failResourcesForAdapterMapper struct {
	delegate meta.RESTMapper
}

func (m *failResourcesForAdapterMapper) KindFor(resource schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	return m.delegate.KindFor(resource)
}
func (m *failResourcesForAdapterMapper) KindsFor(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	return m.delegate.KindsFor(resource)
}
func (m *failResourcesForAdapterMapper) ResourceFor(input schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	return m.delegate.ResourceFor(input)
}
func (m *failResourcesForAdapterMapper) ResourcesFor(input schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	return nil, fmt.Errorf("simulated stale cache: ResourcesFor failed for %s", input.Resource)
}
func (m *failResourcesForAdapterMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	return m.delegate.RESTMapping(gk, versions...)
}
func (m *failResourcesForAdapterMapper) RESTMappings(gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error) {
	return m.delegate.RESTMappings(gk, versions...)
}
func (m *failResourcesForAdapterMapper) ResourceSingularizer(resource string) (string, error) {
	return m.delegate.ResourceSingularizer(resource)
}

const (
	istioGroup   = "security.istio.io"
	istioV1Beta1 = "v1beta1"
	istioV1      = "v1"
	apKind       = "AuthorizationPolicy"
)

func buildMultiVersionAdapterMapper() *meta.DefaultRESTMapper {
	gvBeta := schema.GroupVersion{Group: istioGroup, Version: istioV1Beta1}
	gvStable := schema.GroupVersion{Group: istioGroup, Version: istioV1}
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{gvBeta, gvStable})
	mapper.Add(gvBeta.WithKind(apKind), meta.RESTScopeNamespace)
	mapper.Add(gvStable.WithKind(apKind), meta.RESTScopeNamespace)
	return mapper
}

func buildMultiVersionAdapterScheme() *runtime.Scheme {
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

func newAdapterAuthzPolicy(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": istioGroup + "/" + istioV1,
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

func captureAdapterLogger() (logr.Logger, *[]string) {
	var logs []string
	logger := funcr.New(func(prefix, args string) {
		logs = append(logs, prefix+" "+args)
	}, funcr.Options{Verbosity: 1})
	return logger, &logs
}

var _ = Describe("Issue #1064 follow-up: K8sAdapter multi-version fallback in resolveMappingsAll", func() {

	Describe("UT-EA-MVR-001: GetOwnerChain resolves via RESTMappings fallback when ResourcesFor fails", func() {
		It("should resolve AuthorizationPolicy in v1 via RESTMappings fallback", func() {
			scheme := buildMultiVersionAdapterScheme()
			ap := newAdapterAuthzPolicy("deny-all-traffic", "demo-mesh-failure")
			dynClient := fakedynamic.NewSimpleDynamicClient(scheme, ap)
			mapper := buildMultiVersionAdapterMapper()
			wrappedMapper := &failResourcesForAdapterMapper{delegate: mapper}

			adapter := enrichment.NewK8sAdapter(dynClient, wrappedMapper)
			adapter.SetKindIndex(map[string]schema.GroupKind{
				"authorizationpolicy": {Group: istioGroup, Kind: apKind},
			})

			chain, err := adapter.GetOwnerChain(context.Background(), apKind, "deny-all-traffic", "demo-mesh-failure", "")
			Expect(err).NotTo(HaveOccurred(),
				"resolveMappingsAll should fall back to RESTMappings when ResourcesFor fails")
			Expect(chain).To(BeEmpty(), "AuthorizationPolicy has no ownerReferences")
		})
	})

	Describe("UT-EA-MVR-002: GetSpecHash resolves via RESTMappings fallback when ResourcesFor fails", func() {
		It("should compute spec hash for AuthorizationPolicy resolved via fallback", func() {
			scheme := buildMultiVersionAdapterScheme()
			ap := newAdapterAuthzPolicy("deny-all-traffic", "demo-mesh-failure")
			dynClient := fakedynamic.NewSimpleDynamicClient(scheme, ap)
			mapper := buildMultiVersionAdapterMapper()
			wrappedMapper := &failResourcesForAdapterMapper{delegate: mapper}

			adapter := enrichment.NewK8sAdapter(dynClient, wrappedMapper)
			adapter.SetKindIndex(map[string]schema.GroupKind{
				"authorizationpolicy": {Group: istioGroup, Kind: apKind},
			})

			hash, err := adapter.GetSpecHash(context.Background(), apKind, "deny-all-traffic", "demo-mesh-failure", "")
			Expect(err).NotTo(HaveOccurred(),
				"GetSpecHash should succeed via RESTMappings fallback")
			Expect(hash).NotTo(BeEmpty(),
				"spec hash should be non-empty for a resource with .spec")
		})
	})

	Describe("UT-EA-MVR-003: Fallback uses kindIndex group hint for CRD kinds", func() {
		It("should use security.istio.io group from kindIndex when resolving AuthorizationPolicy", func() {
			scheme := buildMultiVersionAdapterScheme()
			ap := newAdapterAuthzPolicy("test-policy", "default")
			dynClient := fakedynamic.NewSimpleDynamicClient(scheme, ap)
			mapper := buildMultiVersionAdapterMapper()
			wrappedMapper := &failResourcesForAdapterMapper{delegate: mapper}

			adapter := enrichment.NewK8sAdapter(dynClient, wrappedMapper)
			adapter.SetKindIndex(map[string]schema.GroupKind{
				"authorizationpolicy": {Group: istioGroup, Kind: apKind},
			})

			chain, err := adapter.GetOwnerChain(context.Background(), apKind, "test-policy", "default", "")
			Expect(err).NotTo(HaveOccurred(),
				"kindIndex should provide the security.istio.io group hint for RESTMappings fallback")
			Expect(chain).To(BeEmpty())
		})
	})

	Describe("UT-EA-MVR-004: Fallback with empty kindIndex fails gracefully for CRD kinds", func() {
		It("should return an error when kindIndex is empty and ResourcesFor fails for CRD kind", func() {
			scheme := buildMultiVersionAdapterScheme()
			dynClient := fakedynamic.NewSimpleDynamicClient(scheme)
			mapper := buildMultiVersionAdapterMapper()
			wrappedMapper := &failResourcesForAdapterMapper{delegate: mapper}

			adapter := enrichment.NewK8sAdapter(dynClient, wrappedMapper)
			adapter.SetKindIndex(map[string]schema.GroupKind{})

			_, err := adapter.GetOwnerChain(context.Background(), apKind, "test", "test-ns", "")
			Expect(err).To(HaveOccurred(),
				"empty kindIndex with failed ResourcesFor should produce an error for CRD kinds")
		})
	})

	Describe("UT-EA-MVR-005: Fallback with nil kindIndex fails gracefully for CRD kinds", func() {
		It("should return an error when kindIndex is nil and ResourcesFor fails for CRD kind", func() {
			scheme := buildMultiVersionAdapterScheme()
			dynClient := fakedynamic.NewSimpleDynamicClient(scheme)
			mapper := buildMultiVersionAdapterMapper()
			wrappedMapper := &failResourcesForAdapterMapper{delegate: mapper}

			adapter := enrichment.NewK8sAdapter(dynClient, wrappedMapper)

			_, err := adapter.GetOwnerChain(context.Background(), apKind, "test", "test-ns", "")
			Expect(err).To(HaveOccurred(),
				"nil kindIndex (no SetKindIndex called) with failed ResourcesFor should produce an error for CRD kinds")
		})
	})

	Describe("UT-EA-MVR-006: Fallback log emitted at V(1) with version count", func() {
		It("should emit a structured log entry when fallback resolves multiple versions", func() {
			scheme := buildMultiVersionAdapterScheme()
			ap := newAdapterAuthzPolicy("deny-all", "ns")
			dynClient := fakedynamic.NewSimpleDynamicClient(scheme, ap)
			mapper := buildMultiVersionAdapterMapper()
			wrappedMapper := &failResourcesForAdapterMapper{delegate: mapper}
			logger, logs := captureAdapterLogger()

			adapter := enrichment.NewK8sAdapter(dynClient, wrappedMapper)
			adapter.SetLogger(logger)
			adapter.SetKindIndex(map[string]schema.GroupKind{
				"authorizationpolicy": {Group: istioGroup, Kind: apKind},
			})

			_, err := adapter.GetOwnerChain(context.Background(), apKind, "deny-all", "ns", "")
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

	Describe("UT-EA-MVR-007: Multi-group regression (Subscription still works via ResourcesFor path)", func() {
		It("should still resolve Subscription via ResourcesFor when mapper is not wrapped", func() {
			scheme := runtime.NewScheme()

			olmSub := &unstructured.Unstructured{}
			olmSub.SetGroupVersionKind(schema.GroupVersionKind{
				Group: "operators.coreos.com", Version: "v1alpha1", Kind: "Subscription",
			})
			olmSub.SetName("etcd")
			olmSub.SetNamespace("demo-operator")

			dynClient := fakedynamic.NewSimpleDynamicClient(scheme, olmSub)
			mapper := newAmbiguousKindMapper()

			adapter := enrichment.NewK8sAdapter(dynClient, mapper)
			chain, err := adapter.GetOwnerChain(context.Background(), "Subscription", "etcd", "demo-operator", "")
			Expect(err).NotTo(HaveOccurred(), "multi-group resolution via ResourcesFor should still work")
			Expect(chain).To(BeEmpty())
		})
	})
})
