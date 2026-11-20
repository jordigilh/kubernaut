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

package severity_test

import (
	"context"
	"errors"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
)

// recordingDynamic counts Resource() calls so the single-group fast path can be
// proven to make no discovery/read calls at all.
type recordingDynamic struct {
	inner dynamic.Interface
	calls int
}

func (r *recordingDynamic) Resource(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	r.calls++
	return r.inner.Resource(resource)
}

func siblingTestMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper(nil)
	add := func(group, kind, resource string, scope meta.RESTScope) {
		gvk := schema.GroupVersionKind{Group: group, Version: "v1", Kind: kind}
		gvr := schema.GroupVersionResource{Group: group, Version: "v1", Resource: resource}
		mapper.AddSpecific(gvk, gvr, gvr, scope)
	}
	// Route is registered in two API groups (the canonical #2658 sibling case).
	add("route.openshift.io", "Route", "routes", meta.RESTScopeNamespace)
	add("serving.knative.dev", "Route", "routes", meta.RESTScopeNamespace)
	// Event is registered in the core group and events.k8s.io.
	add("", "Event", "events", meta.RESTScopeNamespace)
	add("events.k8s.io", "Event", "events", meta.RESTScopeNamespace)
	// Node is cluster-scoped (single group: the fast path).
	add("", "Node", "nodes", meta.RESTScopeRoot)
	// Deployment is registered in one API group only (fast path).
	add("apps", "Deployment", "deployments", meta.RESTScopeNamespace)
	return mapper
}

func routeObject(apiVersion, namespace, name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(apiVersion)
	obj.SetKind("Route")
	obj.SetNamespace(namespace)
	obj.SetName(name)
	return obj
}

func newSiblingChecker(mapper meta.RESTMapper, client dynamic.Interface) severity.CrossGroupSiblingChecker {
	return severity.NewRESTMapperCrossGroupSiblingChecker(mapper, client, logr.Discard())
}

func newSiblingDynClient(objects ...runtime.Object) dynamic.Interface {
	return dynfake.NewSimpleDynamicClient(runtime.NewScheme(), objects...)
}

var _ = Describe("RESTMapper-backed cross-group sibling checker (#2658) [BR-SEVERITY-001 AC-8]", func() {

	Describe("UT-AF-2658-001: a same-namespace sibling in another group is reported", func() {
		It("reports serving.knative.dev for a Route that exists in both groups", func() {
			client := newSiblingDynClient(
				routeObject("route.openshift.io/v1", "prod", "web"),
				routeObject("serving.knative.dev/v1", "prod", "web"),
			)
			siblings, err := newSiblingChecker(siblingTestMapper(), client).
				CheckCrossGroupSiblings(context.Background(), "prod", "Route", "web", "route.openshift.io")
			Expect(err).NotTo(HaveOccurred())
			Expect(siblings).To(Equal([]string{"serving.knative.dev"}))
		})

		It("reports the core group as a sibling group for a multi-group Kind", func() {
			evt := &unstructured.Unstructured{}
			evt.SetAPIVersion("v1")
			evt.SetKind("Event")
			evt.SetNamespace("prod")
			evt.SetName("web-abc")
			client := newSiblingDynClient(evt)

			siblings, err := newSiblingChecker(siblingTestMapper(), client).
				CheckCrossGroupSiblings(context.Background(), "prod", "Event", "web-abc", "events.k8s.io")
			Expect(err).NotTo(HaveOccurred())
			Expect(siblings).To(Equal([]string{""}), "the core group is a legitimate sibling group")
		})
	})

	Describe("UT-AF-2658-002: name/namespace mismatches are not siblings", func() {
		It("ignores a same-named resource in another namespace", func() {
			client := newSiblingDynClient(
				routeObject("route.openshift.io/v1", "prod", "web"),
				routeObject("serving.knative.dev/v1", "staging", "web"),
			)
			siblings, err := newSiblingChecker(siblingTestMapper(), client).
				CheckCrossGroupSiblings(context.Background(), "prod", "Route", "web", "route.openshift.io")
			Expect(err).NotTo(HaveOccurred())
			Expect(siblings).To(BeEmpty())
		})

		It("ignores a same-namespace resource with a different name", func() {
			client := newSiblingDynClient(
				routeObject("route.openshift.io/v1", "prod", "web"),
				routeObject("serving.knative.dev/v1", "prod", "api"),
			)
			siblings, err := newSiblingChecker(siblingTestMapper(), client).
				CheckCrossGroupSiblings(context.Background(), "prod", "Route", "web", "route.openshift.io")
			Expect(err).NotTo(HaveOccurred())
			Expect(siblings).To(BeEmpty())
		})
	})

	Describe("UT-AF-2658-003: single-group Kind fast path", func() {
		It("makes no K8s read calls for a Kind registered in one group", func() {
			recorder := &recordingDynamic{inner: newSiblingDynClient()}
			siblings, err := newSiblingChecker(siblingTestMapper(), recorder).
				CheckCrossGroupSiblings(context.Background(), "prod", "Deployment", "web", "apps")
			Expect(err).NotTo(HaveOccurred())
			Expect(siblings).To(BeEmpty())
			Expect(recorder.calls).To(BeZero(), "a single-group Kind cannot have a cross-group sibling")
		})
	})

	Describe("UT-AF-2658-007: cannot-enumerate and read failures fail closed", func() {
		It("errors when no RESTMapper is wired", func() {
			_, err := newSiblingChecker(nil, newSiblingDynClient()).
				CheckCrossGroupSiblings(context.Background(), "prod", "Route", "web", "route.openshift.io")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, severity.ErrGroupEnumerationUnavailable)).To(BeTrue())
		})

		It("errors when no K8s client is wired", func() {
			_, err := newSiblingChecker(siblingTestMapper(), nil).
				CheckCrossGroupSiblings(context.Background(), "prod", "Route", "web", "route.openshift.io")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, severity.ErrGroupEnumerationUnavailable)).To(BeTrue())
		})

		It("errors for an irregular plural that the resource guess cannot enumerate", func() {
			// "Gateway" pluralizes to "gateways"; the apimachinery guess
			// produces "gatewaies", so discovery cannot enumerate the Kind
			// and the check must fail closed rather than silently skip.
			_, err := newSiblingChecker(siblingTestMapper(), newSiblingDynClient()).
				CheckCrossGroupSiblings(context.Background(), "prod", "Gateway", "web", "gateway.networking.k8s.io")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, severity.ErrGroupEnumerationUnavailable)).To(BeTrue())
		})

		It("errors on a read failure other than NotFound", func() {
			client := newSiblingDynClient()
			client.(*dynfake.FakeDynamicClient).PrependReactor("get", "routes", func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "routes"}, "web", errors.New("rbac denied"))
			})
			_, err := newSiblingChecker(siblingTestMapper(), client).
				CheckCrossGroupSiblings(context.Background(), "prod", "Route", "web", "route.openshift.io")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("rbac denied"), "the RBAC failure must stay visible to the caller")
		})
	})

	Describe("TTL/reset self-heal", func() {
		It("retries enumeration once through Reset when the mapper is resettable", func() {
			client := newSiblingDynClient(routeObject("serving.knative.dev/v1", "prod", "web"))
			resettable := &resettableMapper{RESTMapper: siblingTestMapper(), failFirst: 1}
			siblings, err := newSiblingChecker(resettable, client).
				CheckCrossGroupSiblings(context.Background(), "prod", "Route", "web", "route.openshift.io")
			Expect(err).NotTo(HaveOccurred())
			Expect(resettable.resets).To(Equal(1))
			Expect(siblings).To(Equal([]string{"serving.knative.dev"}))
		})
	})
})

// resettableMapper fails the first KindsFor call (simulating a stale discovery
// cache) and only recovers after Reset, mirroring DD-K8S-001's self-heal.
type resettableMapper struct {
	meta.RESTMapper
	failFirst int
	resets    int
}

func (r *resettableMapper) KindsFor(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	if r.failFirst > 0 {
		r.failFirst--
		return nil, &meta.NoResourceMatchError{PartialResource: resource}
	}
	return r.RESTMapper.KindsFor(resource)
}

func (r *resettableMapper) Reset() {
	r.resets++
}

var _ = Describe("Confirmed target group is the escape hatch for an unavailable check (#2658)", func() {
	It("adjudicates when the user confirms the target's own group and the checker is unwired", func() {
		input := severity.TriageInput{
			Namespace: "prod", Kind: "Route", Name: "web",
			APIGroup: "route.openshift.io", APIGroupKnown: true,
			ConfirmedAPIGroup: "route.openshift.io",
			ClusterID:         hubClusterID,
			Labels:            map[string]string{"namespace": "prod", "kind": "Route", "name": "web"},
		}
		promClient := &mockPromClient{alerts: []prom.Alert{
			{Labels: map[string]string{"alertname": "RouteDegraded", "namespace": "prod", "kind": "Route", "name": "web", "severity": "critical"}, State: "firing"},
		}}
		triager := severity.NewTriager(promClient, &mockLLM{}, severity.DefaultConfig(), logr.Discard(),
			severity.WithCrossGroupSiblingChecker(&stubSiblingChecker{err: severity.ErrGroupEnumerationUnavailable}))
		result, err := triager.Triage(context.Background(), input)
		Expect(err).NotTo(HaveOccurred(), "an unavailable check must stay resolvable by the user's own-group confirmation")
		Expect(result.Severity).To(Equal("critical"))
	})

	It("still fails closed when the confirmation names a different group and the check is unavailable", func() {
		input := severity.TriageInput{
			Namespace: "prod", Kind: "Route", Name: "web",
			APIGroup: "route.openshift.io", APIGroupKnown: true,
			ConfirmedAPIGroup: "serving.knative.dev",
			ClusterID:         hubClusterID,
			Labels:            map[string]string{"namespace": "prod", "kind": "Route", "name": "web"},
		}
		promClient := &mockPromClient{alerts: []prom.Alert{
			{Labels: map[string]string{"alertname": "RouteDegraded", "namespace": "prod", "kind": "Route", "name": "web", "severity": "critical"}, State: "firing"},
		}}
		triager := severity.NewTriager(promClient, &mockLLM{}, severity.DefaultConfig(), logr.Discard(),
			severity.WithCrossGroupSiblingChecker(&stubSiblingChecker{err: severity.ErrGroupEnumerationUnavailable}))
		_, err := triager.Triage(context.Background(), input)
		var amb *severity.AmbiguousSeverityError
		Expect(errors.As(err, &amb)).To(BeTrue())
		Expect(amb.Candidate.AmbiguityReason).To(Equal(severity.AmbiguityReasonGroupUnscopable))
	})
})

// gvrStubDynamic answers existence GETs per exact GVR, independent of any
// plural guess.
type gvrStubDynamic struct {
	dynamic.Interface
	found map[schema.GroupVersionResource]bool
}

func (s *gvrStubDynamic) Resource(gvr schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return &gvrStubResource{gvr: gvr, exists: s.found[gvr]}
}

type gvrStubResource struct {
	dynamic.NamespaceableResourceInterface
	gvr    schema.GroupVersionResource
	exists bool
}

func (r *gvrStubResource) Namespace(string) dynamic.ResourceInterface { return r }

func (r *gvrStubResource) Get(_ context.Context, name string, _ metav1.GetOptions, _ ...string) (*unstructured.Unstructured, error) {
	if r.exists {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(r.gvr.GroupVersion().WithKind("Gateway"))
		obj.SetName(name)
		return obj, nil
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: r.gvr.Group, Resource: r.gvr.Resource}, name)
}

// fakeResourceLister serves the discovery surface the checker needs: exact
// Kind -> plural -> group -> scope data, no plural guessing (#2658).
type fakeResourceLister struct {
	lists []*metav1.APIResourceList
	err   error
	calls int
}

func (f *fakeResourceLister) ServerGroupsAndResources() ([]*metav1.APIGroup, []*metav1.APIResourceList, error) {
	f.calls++
	return nil, f.lists, f.err
}

func apiResourceList(groupVersion string, resources ...metav1.APIResource) *metav1.APIResourceList {
	return &metav1.APIResourceList{GroupVersion: groupVersion, APIResources: resources}
}

var _ = Describe("Discovery-backed cross-group sibling checker (#2658)", func() {
	It("UT-AF-2658-007 (irregular plural): resolves a Kind whose real plural the guess cannot derive", func() {
		lister := &fakeResourceLister{lists: []*metav1.APIResourceList{
			apiResourceList("gateway.networking.k8s.io/v1", metav1.APIResource{Name: "gateways", Kind: "Gateway", Namespaced: true}),
			apiResourceList("gateway.networking.x-k8s.io/v1alpha2", metav1.APIResource{Name: "gateways", Kind: "Gateway", Namespaced: true}),
		}}
		// A GVR-exact stub: the dynamic fake keys unstructured objects by the
		// same plural guess under test, which would mask the outcome.
		client := &gvrStubDynamic{found: map[schema.GroupVersionResource]bool{
			{Group: "gateway.networking.x-k8s.io", Version: "v1", Resource: "gateways"}:             false,
			{Group: "gateway.networking.x-k8s.io", Version: "v1", Resource: "gateways"}:             false,
			{Group: "gateway.networking.x-k8s.io", Version: "v1beta1", Resource: "gateways"}:        false,
			{Group: "gateway.networking.x-k8s.io", Version: "v1alpha2", Resource: "gateways"}:       true,
			{Group: "gateway.networking.x-k8s.io", Version: "v1alpha2", Resource: "gatewayclasses"}: false,
		}}

		siblings, err := severity.NewDiscoveryCrossGroupSiblingChecker(lister, client, logr.Discard()).
			CheckCrossGroupSiblings(context.Background(), "prod", "Gateway", "web", "gateway.networking.k8s.io")
		Expect(err).NotTo(HaveOccurred(), "exact discovery data must not fail closed on an irregular plural")
		Expect(siblings).To(Equal([]string{"gateway.networking.x-k8s.io"}))
	})

	It("UT-AF-2658-003 (discovery): single-group Kind stays a zero-client-call fast path", func() {
		lister := &fakeResourceLister{lists: []*metav1.APIResourceList{
			apiResourceList("apps/v1", metav1.APIResource{Name: "deployments", Kind: "Deployment", Namespaced: true}),
		}}
		recorder := &recordingDynamic{inner: newSiblingDynClient()}
		siblings, err := severity.NewDiscoveryCrossGroupSiblingChecker(lister, recorder, logr.Discard()).
			CheckCrossGroupSiblings(context.Background(), "prod", "Deployment", "web", "apps")
		Expect(err).NotTo(HaveOccurred())
		Expect(siblings).To(BeEmpty())
		Expect(recorder.calls).To(BeZero())
	})

	It("UT-AF-2658-001 (discovery): reports the core group and respects scope", func() {
		lister := &fakeResourceLister{lists: []*metav1.APIResourceList{
			apiResourceList("v1", metav1.APIResource{Name: "events", Kind: "Event", Namespaced: true}),
			apiResourceList("events.k8s.io/v1", metav1.APIResource{Name: "events", Kind: "Event", Namespaced: true}),
		}}
		evt := &unstructured.Unstructured{}
		evt.SetAPIVersion("v1")
		evt.SetKind("Event")
		evt.SetNamespace("prod")
		evt.SetName("web-abc")
		siblings, err := severity.NewDiscoveryCrossGroupSiblingChecker(lister, newSiblingDynClient(evt), logr.Discard()).
			CheckCrossGroupSiblings(context.Background(), "prod", "Event", "web-abc", "events.k8s.io")
		Expect(err).NotTo(HaveOccurred())
		Expect(siblings).To(Equal([]string{""}))
	})

	It("UT-AF-2658-007 (discovery): fails closed when discovery is unavailable", func() {
		lister := &fakeResourceLister{err: errors.New("discovery down")}
		_, err := severity.NewDiscoveryCrossGroupSiblingChecker(lister, newSiblingDynClient(), logr.Discard()).
			CheckCrossGroupSiblings(context.Background(), "prod", "Gateway", "web", "gateway.networking.k8s.io")
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, severity.ErrGroupEnumerationUnavailable)).To(BeTrue())
	})

	It("UT-AF-2658-007 (discovery): fails closed when the Kind is not served at all", func() {
		lister := &fakeResourceLister{lists: []*metav1.APIResourceList{
			apiResourceList("apps/v1", metav1.APIResource{Name: "deployments", Kind: "Deployment", Namespaced: true}),
		}}
		_, err := severity.NewDiscoveryCrossGroupSiblingChecker(lister, newSiblingDynClient(), logr.Discard()).
			CheckCrossGroupSiblings(context.Background(), "prod", "MysteryKind", "web", "example.com")
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, severity.ErrGroupEnumerationUnavailable)).To(BeTrue())
	})
})
