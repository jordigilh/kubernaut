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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakedynamic "k8s.io/client-go/dynamic/fake"
	"k8s.io/utils/ptr"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
)

var _ = Describe("K8s Owner-Chain Adapter — TP-433-WIR Phase 1b", func() {

	Describe("UT-KA-433W-004: K8s adapter walks single-level ownerReference (Pod -> ReplicaSet)", func() {
		It("should return [{ReplicaSet, rs-abc, default}]", func() {
			scheme := runtime.NewScheme()

			pod := &unstructured.Unstructured{}
			pod.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"})
			pod.SetName("web-abc123")
			pod.SetNamespace("default")
			pod.SetOwnerReferences([]metav1.OwnerReference{
				{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "rs-abc", UID: "uid-rs", Controller: ptr.To(true)},
			})

			rs := &unstructured.Unstructured{}
			rs.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"})
			rs.SetName("rs-abc")
			rs.SetNamespace("default")

			dynClient := fakedynamic.NewSimpleDynamicClient(scheme, pod, rs)
			mapper := newSimpleRESTMapper()

			adapter := enrichment.NewK8sAdapter(dynClient, mapper)
			chain, err := adapter.GetOwnerChain(context.Background(), "Pod", "web-abc123", "default", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(chain).To(HaveLen(1))
			Expect(chain[0].Kind).To(Equal("ReplicaSet"))
			Expect(chain[0].Name).To(Equal("rs-abc"))
			Expect(chain[0].Namespace).To(Equal("default"))
		})
	})

	Describe("UT-KA-433W-005: K8s adapter walks multi-level chain (Pod -> RS -> Deployment)", func() {
		It("should return 2-entry chain in order", func() {
			scheme := runtime.NewScheme()

			pod := &unstructured.Unstructured{}
			pod.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"})
			pod.SetName("web-abc123")
			pod.SetNamespace("default")
			pod.SetOwnerReferences([]metav1.OwnerReference{
				{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "rs-abc", UID: "uid-rs", Controller: ptr.To(true)},
			})

			rs := &unstructured.Unstructured{}
			rs.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"})
			rs.SetName("rs-abc")
			rs.SetNamespace("default")
			rs.SetOwnerReferences([]metav1.OwnerReference{
				{APIVersion: "apps/v1", Kind: "Deployment", Name: "api-server", UID: "uid-deploy", Controller: ptr.To(true)},
			})

			deploy := &unstructured.Unstructured{}
			deploy.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"})
			deploy.SetName("api-server")
			deploy.SetNamespace("default")

			dynClient := fakedynamic.NewSimpleDynamicClient(scheme, pod, rs, deploy)
			mapper := newSimpleRESTMapper()

			adapter := enrichment.NewK8sAdapter(dynClient, mapper)
			chain, err := adapter.GetOwnerChain(context.Background(), "Pod", "web-abc123", "default", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(chain).To(HaveLen(2))
			Expect(chain[0].Kind).To(Equal("ReplicaSet"))
			Expect(chain[0].Name).To(Equal("rs-abc"))
			Expect(chain[1].Kind).To(Equal("Deployment"))
			Expect(chain[1].Name).To(Equal("api-server"))
		})
	})

	Describe("UT-KA-433W-006: K8s adapter returns empty chain for no ownerReference", func() {
		It("should return empty []OwnerChainEntry{}", func() {
			scheme := runtime.NewScheme()

			pod := &unstructured.Unstructured{}
			pod.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"})
			pod.SetName("standalone-pod")
			pod.SetNamespace("default")

			dynClient := fakedynamic.NewSimpleDynamicClient(scheme, pod)
			mapper := newSimpleRESTMapper()

			adapter := enrichment.NewK8sAdapter(dynClient, mapper)
			chain, err := adapter.GetOwnerChain(context.Background(), "Pod", "standalone-pod", "default", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(chain).NotTo(BeNil())
			Expect(chain).To(BeEmpty())
		})
	})

	Describe("UT-KA-433W-007: K8s adapter terminates at max depth (10)", func() {
		It("should not exceed maxOwnerChainDepth even with circular references", func() {
			scheme := runtime.NewScheme()

			var resources []runtime.Object
			for i := 0; i < 12; i++ {
				obj := &unstructured.Unstructured{}
				obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"})
				obj.SetName(fmt.Sprintf("rs-%d", i))
				obj.SetNamespace("default")
				if i < 11 {
					obj.SetOwnerReferences([]metav1.OwnerReference{
						{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: fmt.Sprintf("rs-%d", i+1), UID: "uid", Controller: ptr.To(true)},
					})
				}
				resources = append(resources, obj)
			}

			pod := &unstructured.Unstructured{}
			pod.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"})
			pod.SetName("deep-pod")
			pod.SetNamespace("default")
			pod.SetOwnerReferences([]metav1.OwnerReference{
				{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "rs-0", UID: "uid", Controller: ptr.To(true)},
			})
			resources = append(resources, pod)

			dynClient := fakedynamic.NewSimpleDynamicClient(scheme, resources...)
			mapper := newSimpleRESTMapper()

			adapter := enrichment.NewK8sAdapter(dynClient, mapper)
			chain, err := adapter.GetOwnerChain(context.Background(), "Pod", "deep-pod", "default", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(len(chain)).To(BeNumerically("<=", 10))
		})
	})

	Describe("UT-KA-433W-008: K8s adapter resolves 3-level Pod -> RS -> Deployment chain", func() {
		It("should resolve a 3-level chain against fake dynamic client with ownerRef fixtures", func() {
			scheme := runtime.NewScheme()

			pod := &unstructured.Unstructured{}
			pod.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"})
			pod.SetName("web-pod-1")
			pod.SetNamespace("production")
			pod.SetOwnerReferences([]metav1.OwnerReference{
				{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "web-rs-abc", UID: "rs-uid", Controller: ptr.To(true)},
			})

			rs := &unstructured.Unstructured{}
			rs.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"})
			rs.SetName("web-rs-abc")
			rs.SetNamespace("production")
			rs.SetOwnerReferences([]metav1.OwnerReference{
				{APIVersion: "apps/v1", Kind: "Deployment", Name: "web-deploy", UID: "deploy-uid", Controller: ptr.To(true)},
			})

			deploy := &unstructured.Unstructured{}
			deploy.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"})
			deploy.SetName("web-deploy")
			deploy.SetNamespace("production")

			dynClient := fakedynamic.NewSimpleDynamicClient(scheme, pod, rs, deploy)
			mapper := newSimpleRESTMapper()

			adapter := enrichment.NewK8sAdapter(dynClient, mapper)
			chain, err := adapter.GetOwnerChain(context.Background(), "Pod", "web-pod-1", "production", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(chain).To(HaveLen(2))
			Expect(chain[0].Kind).To(Equal("ReplicaSet"))
			Expect(chain[0].Name).To(Equal("web-rs-abc"))
			Expect(chain[0].Namespace).To(Equal("production"))
			Expect(chain[1].Kind).To(Equal("Deployment"))
			Expect(chain[1].Name).To(Equal("web-deploy"))
			Expect(chain[1].Namespace).To(Equal("production"))
		})
	})

	Describe("UT-KA-433W-009: K8s adapter resolves cluster-scoped resource (Node)", func() {
		It("should resolve Node with empty namespace", func() {
			scheme := runtime.NewScheme()

			node := &unstructured.Unstructured{}
			node.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Node"})
			node.SetName("worker-1")

			dynClient := fakedynamic.NewSimpleDynamicClient(scheme, node)
			mapper := newSimpleRESTMapper()

			adapter := enrichment.NewK8sAdapter(dynClient, mapper)
			chain, err := adapter.GetOwnerChain(context.Background(), "Node", "worker-1", "", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(chain).To(BeEmpty())
		})
	})
})

var _ = Describe("TP-693: Controller-ref owner chain selection", func() {

	Describe("UT-KA-693-008: Multiple ownerRefs — only controller:true is followed", func() {
		It("should follow the controller ownerRef, not the first one", func() {
			scheme := runtime.NewScheme()

			pod := &unstructured.Unstructured{}
			pod.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"})
			pod.SetName("test-pod")
			pod.SetNamespace("default")
			pod.SetOwnerReferences([]metav1.OwnerReference{
				{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "non-controller-rs", UID: "uid-1"},
				{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "controller-rs", UID: "uid-2", Controller: ptr.To(true)},
			})

			nonControllerRS := &unstructured.Unstructured{}
			nonControllerRS.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"})
			nonControllerRS.SetName("non-controller-rs")
			nonControllerRS.SetNamespace("default")

			controllerRS := &unstructured.Unstructured{}
			controllerRS.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"})
			controllerRS.SetName("controller-rs")
			controllerRS.SetNamespace("default")
			controllerRS.SetOwnerReferences([]metav1.OwnerReference{
				{APIVersion: "apps/v1", Kind: "Deployment", Name: "my-deploy", UID: "uid-deploy", Controller: ptr.To(true)},
			})

			deploy := &unstructured.Unstructured{}
			deploy.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"})
			deploy.SetName("my-deploy")
			deploy.SetNamespace("default")

			dynClient := fakedynamic.NewSimpleDynamicClient(scheme, pod, nonControllerRS, controllerRS, deploy)
			mapper := newSimpleRESTMapper()

			adapter := enrichment.NewK8sAdapter(dynClient, mapper)
			chain, err := adapter.GetOwnerChain(context.Background(), "Pod", "test-pod", "default", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(chain).To(HaveLen(2),
				"UT-KA-693-008: should follow controller RS → Deployment")
			Expect(chain[0].Name).To(Equal("controller-rs"),
				"UT-KA-693-008: first entry must be the controller RS, not non-controller-rs")
			Expect(chain[1].Name).To(Equal("my-deploy"))
		})
	})

	Describe("UT-KA-693-009: No controller:true ownerRef yields empty chain", func() {
		It("should return empty chain when no ownerRef has controller:true", func() {
			scheme := runtime.NewScheme()

			pod := &unstructured.Unstructured{}
			pod.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"})
			pod.SetName("orphan-pod")
			pod.SetNamespace("default")
			pod.SetOwnerReferences([]metav1.OwnerReference{
				{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "some-rs", UID: "uid-1"},
			})

			someRS := &unstructured.Unstructured{}
			someRS.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"})
			someRS.SetName("some-rs")
			someRS.SetNamespace("default")

			dynClient := fakedynamic.NewSimpleDynamicClient(scheme, pod, someRS)
			mapper := newSimpleRESTMapper()

			adapter := enrichment.NewK8sAdapter(dynClient, mapper)
			chain, err := adapter.GetOwnerChain(context.Background(), "Pod", "orphan-pod", "default", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(chain).To(BeEmpty(),
				"UT-KA-693-009: no controller:true → empty chain (aligned with SP/GW)")
		})
	})
})

var _ = Describe("UT-KA-704-MAPPER: RESTMapper refresh for CRDs installed after startup", func() {
	Describe("UT-KA-704-MAPPER-001: resolveGVR retries with Reset on resettable mapper", func() {
		It("should discover a CRD Kind after mapper reset", func() {
			scheme := runtime.NewScheme()

			cert := &unstructured.Unstructured{}
			cert.SetGroupVersionKind(schema.GroupVersionKind{
				Group: "cert-manager.io", Version: "v1", Kind: "Certificate",
			})
			cert.SetName("demo-app-cert")
			cert.SetNamespace("default")

			dynClient := fakedynamic.NewSimpleDynamicClient(scheme, cert)
			mapper := &lateRegistrationMapper{
				delegate: newSimpleRESTMapper(),
			}

			adapter := enrichment.NewK8sAdapter(dynClient, mapper)

			_, errBefore := adapter.GetOwnerChain(context.Background(), "Certificate", "demo-app-cert", "default", "")
			Expect(errBefore).To(HaveOccurred(),
				"should fail before CRD is registered")

			mapper.registerCertificate()

			chain, errAfter := adapter.GetOwnerChain(context.Background(), "Certificate", "demo-app-cert", "default", "")
			Expect(errAfter).NotTo(HaveOccurred(),
				"should succeed after mapper reset discovers the CRD")
			Expect(chain).To(BeEmpty(), "Certificate has no ownerReferences")
		})
	})
})

// lateRegistrationMapper simulates a DeferredDiscoveryRESTMapper that discovers
// a CRD after Reset(). The first lookup for "certificates" fails; after
// registerCertificate() + Reset() it succeeds.
type lateRegistrationMapper struct {
	delegate  *meta.DefaultRESTMapper
	certAdded bool
}

func (m *lateRegistrationMapper) Reset() {
	if m.certAdded {
		m.delegate.Add(schema.GroupVersionKind{
			Group: "cert-manager.io", Version: "v1", Kind: "Certificate",
		}, meta.RESTScopeNamespace)
	}
}

func (m *lateRegistrationMapper) registerCertificate() {
	m.certAdded = true
}

func (m *lateRegistrationMapper) ResourceFor(input schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	return m.delegate.ResourceFor(input)
}
func (m *lateRegistrationMapper) ResourcesFor(input schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	return m.delegate.ResourcesFor(input)
}
func (m *lateRegistrationMapper) KindFor(input schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	return m.delegate.KindFor(input)
}
func (m *lateRegistrationMapper) KindsFor(input schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	return m.delegate.KindsFor(input)
}
func (m *lateRegistrationMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	return m.delegate.RESTMapping(gk, versions...)
}
func (m *lateRegistrationMapper) RESTMappings(gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error) {
	return m.delegate.RESTMappings(gk, versions...)
}
func (m *lateRegistrationMapper) ResourceSingularizer(resource string) (string, error) {
	return m.delegate.ResourceSingularizer(resource)
}

// newSimpleRESTMapper creates a REST mapper that knows about common K8s types.
func newSimpleRESTMapper() *meta.DefaultRESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Group: "", Version: "v1"},
		{Group: "apps", Version: "v1"},
	})
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Node"}, meta.RESTScopeRoot)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}, meta.RESTScopeNamespace)
	return mapper
}

// newAmbiguousKindMapper creates a REST mapper where "Subscription" exists in
// two API groups (Knative first, OLM second by preference order), reproducing
// the ambiguous-kind scenario from Issue #1062.
func newAmbiguousKindMapper() *meta.DefaultRESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Group: "", Version: "v1"},
		{Group: "apps", Version: "v1"},
		{Group: "messaging.knative.dev", Version: "v1"},
		{Group: "operators.coreos.com", Version: "v1alpha1"},
	})
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "messaging.knative.dev", Version: "v1", Kind: "Subscription"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "operators.coreos.com", Version: "v1alpha1", Kind: "Subscription"}, meta.RESTScopeNamespace)
	return mapper
}

var _ = Describe("Issue #1062: Multi-group kind resolution fallback", func() {

	Describe("UT-KA-1062-001: Ambiguous kind, resource exists in second API group (OLM)", func() {
		It("should find Subscription in operators.coreos.com after messaging.knative.dev returns NotFound", func() {
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
			Expect(err).NotTo(HaveOccurred(),
				"BR-AI-1062: adapter must try alternate API groups when first returns NotFound")
			Expect(chain).To(BeEmpty(),
				"Subscription has no ownerReferences")
		})
	})

	Describe("UT-KA-1062-002: Ambiguous kind, resource exists in first API group (Knative)", func() {
		It("should find Subscription in messaging.knative.dev on first attempt", func() {
			scheme := runtime.NewScheme()

			knativeSub := &unstructured.Unstructured{}
			knativeSub.SetGroupVersionKind(schema.GroupVersionKind{
				Group: "messaging.knative.dev", Version: "v1", Kind: "Subscription",
			})
			knativeSub.SetName("my-sub")
			knativeSub.SetNamespace("default")

			dynClient := fakedynamic.NewSimpleDynamicClient(scheme, knativeSub)
			mapper := newAmbiguousKindMapper()

			adapter := enrichment.NewK8sAdapter(dynClient, mapper)
			chain, err := adapter.GetOwnerChain(context.Background(), "Subscription", "my-sub", "default", "")
			Expect(err).NotTo(HaveOccurred(),
				"BR-AI-1062: adapter must succeed when resource exists in first group")
			Expect(chain).To(BeEmpty())
		})
	})

	Describe("UT-KA-1062-003: Ambiguous kind, resource not found in any group", func() {
		It("should return error when Subscription is not found in any API group", func() {
			scheme := runtime.NewScheme()

			dynClient := fakedynamic.NewSimpleDynamicClient(scheme)
			mapper := newAmbiguousKindMapper()

			adapter := enrichment.NewK8sAdapter(dynClient, mapper)
			_, err := adapter.GetOwnerChain(context.Background(), "Subscription", "nonexistent", "demo-operator", "")
			Expect(err).To(HaveOccurred(),
				"BR-AI-1062: adapter must return error when resource not found in any group")
			Expect(enrichment.IsNotFoundError(err)).To(BeTrue(),
				"BR-AI-1062: error must preserve IsNotFoundError classification for TargetResourceDeleted")
		})
	})

	Describe("UT-KA-1062-004: Non-ambiguous kind preserves existing behavior", func() {
		It("should resolve Pod normally with single API group", func() {
			scheme := runtime.NewScheme()

			pod := &unstructured.Unstructured{}
			pod.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"})
			pod.SetName("test-pod")
			pod.SetNamespace("default")

			dynClient := fakedynamic.NewSimpleDynamicClient(scheme, pod)
			mapper := newAmbiguousKindMapper()

			adapter := enrichment.NewK8sAdapter(dynClient, mapper)
			chain, err := adapter.GetOwnerChain(context.Background(), "Pod", "test-pod", "default", "")
			Expect(err).NotTo(HaveOccurred(),
				"BR-AI-1062: non-ambiguous kinds must continue to work")
			Expect(chain).To(BeEmpty())
		})
	})

	Describe("UT-KA-1062-005: Explicit apiVersion bypasses fallback", func() {
		It("should use only the specified API group and not fall back", func() {
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

			chain, err := adapter.GetOwnerChain(context.Background(), "Subscription", "etcd", "demo-operator", "operators.coreos.com/v1alpha1")
			Expect(err).NotTo(HaveOccurred(),
				"BR-AI-1062: explicit apiVersion must resolve directly")
			Expect(chain).To(BeEmpty())

			_, err = adapter.GetOwnerChain(context.Background(), "Subscription", "etcd", "demo-operator", "messaging.knative.dev/v1")
			Expect(err).To(HaveOccurred(),
				"BR-AI-1062: explicit apiVersion pointing to wrong group must fail (no fallback)")
		})
	})

	Describe("UT-KA-1062-006: GetSpecHash fallback for ambiguous kind", func() {
		It("should compute spec hash via second API group when first returns NotFound", func() {
			scheme := runtime.NewScheme()

			olmSub := &unstructured.Unstructured{}
			olmSub.SetGroupVersionKind(schema.GroupVersionKind{
				Group: "operators.coreos.com", Version: "v1alpha1", Kind: "Subscription",
			})
			olmSub.SetName("etcd")
			olmSub.SetNamespace("demo-operator")
			olmSub.Object["spec"] = map[string]interface{}{
				"channel": "stable",
			}

			dynClient := fakedynamic.NewSimpleDynamicClient(scheme, olmSub)
			mapper := newAmbiguousKindMapper()

			adapter := enrichment.NewK8sAdapter(dynClient, mapper)
			hash, err := adapter.GetSpecHash(context.Background(), "Subscription", "etcd", "demo-operator", "")
			Expect(err).NotTo(HaveOccurred(),
				"BR-AI-1062: GetSpecHash must try alternate API groups")
			Expect(hash).NotTo(BeEmpty(),
				"BR-AI-1062: spec hash must be computed from the found resource")
		})
	})

	Describe("UT-KA-1062-007: All-groups-NotFound preserves IsNotFoundError for Enrich() classification", func() {
		It("should return wrapped error that IsNotFoundError recognizes", func() {
			scheme := runtime.NewScheme()

			dynClient := fakedynamic.NewSimpleDynamicClient(scheme)
			mapper := newAmbiguousKindMapper()

			adapter := enrichment.NewK8sAdapter(dynClient, mapper)
			_, err := adapter.GetSpecHash(context.Background(), "Subscription", "missing", "default", "")
			Expect(err).To(HaveOccurred())
			Expect(enrichment.IsNotFoundError(err)).To(BeTrue(),
				"BR-AI-1062: all-groups-NotFound error must be classifiable as NotFound "+
					"for TargetResourceDeleted handling in Enrich()")
		})
	})
})
