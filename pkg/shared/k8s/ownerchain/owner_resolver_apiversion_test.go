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

package ownerchain_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/shared/k8s/ownerchain"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// gvkCapturingReader is a client.Reader spy that records the GVK stamped on
// each object at Get time and either returns a canned error, a standalone
// (ownerless) object, or an object controlled by a fixed ownerReference (so
// traversal advances once). Tests can observe exactly which GVK the resolver
// used for each fetch (#2649).
type gvkCapturingReader struct {
	err      error
	ownerRef *metav1.OwnerReference
	// getErrs supplies per-call results for multi-fetch scenarios (e.g.
	// level-0 fetch OK followed by a NotFound passthrough verification). Call
	// i returns getErrs[i]; once exhausted, the last entry repeats. A nil
	// entry means the call succeeds. Takes precedence over err.
	getErrs []error

	gotGVKs []schema.GroupVersionKind
	calls   int
}

func (s *gvkCapturingReader) Get(_ context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	s.calls++
	s.gotGVKs = append(s.gotGVKs, obj.GetObjectKind().GroupVersionKind())
	if s.getErrs != nil {
		idx := s.calls - 1
		if idx >= len(s.getErrs) {
			idx = len(s.getErrs) - 1
		}
		if e := s.getErrs[idx]; e != nil {
			return e
		}
	} else if s.err != nil {
		return s.err
	}
	obj.SetNamespace(key.Namespace)
	obj.SetName(key.Name)
	if s.ownerRef != nil {
		obj.SetOwnerReferences([]metav1.OwnerReference{*s.ownerRef})
	}
	return nil
}

func (s *gvkCapturingReader) List(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
	return errors.New("list not implemented in spy")
}

// serviceBackingReader fakes the fallback reader for the Service→Pod rewrite:
// it serves a selector-bearing core Service, a selector-matched Pod list, and
// standalone PartialObjectMetadata for any later direct verification GETs.
type serviceBackingReader struct {
	selector    map[string]string
	serviceGets int
	podLists    int
	// podOwnerRef is stamped on PartialObjectMetadata GETs so a level-0 Pod
	// fetch can advance to the workload owner.
	podOwnerRef *metav1.OwnerReference
	metaGVKs    []schema.GroupVersionKind
}

func (s *serviceBackingReader) Get(_ context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	switch o := obj.(type) {
	case *corev1.Service:
		s.serviceGets++
		o.Namespace, o.Name = key.Namespace, key.Name
		o.Spec.Selector = s.selector
		return nil
	case *metav1.PartialObjectMetadata:
		s.metaGVKs = append(s.metaGVKs, obj.GetObjectKind().GroupVersionKind())
		o.Namespace, o.Name = key.Namespace, key.Name
		if s.podOwnerRef != nil {
			o.SetOwnerReferences([]metav1.OwnerReference{*s.podOwnerRef})
		}
		return nil
	}
	return fmt.Errorf("unsupported object type %T", obj)
}

func (s *serviceBackingReader) List(_ context.Context, list client.ObjectList, _ ...client.ListOption) error {
	pl, ok := list.(*corev1.PodList)
	if !ok {
		return fmt.Errorf("unsupported list type %T", list)
	}
	s.podLists++
	pl.Items = []corev1.Pod{{ObjectMeta: metav1.ObjectMeta{
		Name: "payment-api-abc", Namespace: "production",
	}}}
	return nil
}

// fakeKindResolver is a local ownerchain.KindResolver double mapping kinds to
// fixed GVRs and marking every mapped kind as core/apps/batch so traversal
// reaches the level-0 fetch.
type fakeKindResolver struct {
	gvrs map[string]schema.GroupVersionResource
	// nonCore marks kinds the registry would classify as CRD (not
	// core/apps/batch), exercising the traversal's CRD gate.
	nonCore map[string]bool
}

func (f *fakeKindResolver) KindToGVR(kind string) (schema.GroupVersionResource, bool) {
	gvr, ok := f.gvrs[kind]
	return gvr, ok
}

func (f *fakeKindResolver) IsCoreBatchAppsKind(kind string) bool { return !f.nonCore[kind] }

// entry builds the sharedtypes.OwnerChainEntry the identity-carrying
// ResolveTopLevelOwnerIdentity consumes (#2649/#2593).
func entry(namespace, kind, name, apiVersion string) sharedtypes.OwnerChainEntry {
	return sharedtypes.OwnerChainEntry{
		Namespace:  namespace,
		Kind:       kind,
		Name:       name,
		APIVersion: apiVersion,
	}
}

// notFound builds a typed NotFound for the given group/resource.
func notFound(group, resource, name string) error {
	return apierrors.NewNotFound(schema.GroupResource{Group: group, Resource: resource}, name)
}

var _ = Describe("ResolveTopLevelOwnerIdentity verified passthrough (#2649)", func() {
	ctx := context.Background()

	Describe("UT-GW-2649-RES-001 [BR-GATEWAY-004, BR-SCOPE-001, SI-10]: the caller-supplied apiVersion threads into the level-0 fetch and the returned identity", func() {
		It("fetches under the entry GVK and returns it unchanged for a standalone resource", func() {
			spy := &gvkCapturingReader{}
			resolver := ownerchain.NewK8sOwnerResolver(spy, logr.Discard())

			resolved, err := resolver.ResolveTopLevelOwnerIdentity(ctx, entry("production", "Deployment", "payment-api", "apps/v2"))

			Expect(err).NotTo(HaveOccurred())
			Expect(resolved).To(Equal(entry("production", "Deployment", "payment-api", "apps/v2")),
				"the identity path must never re-map the caller-supplied GVK through the static kind table")
			Expect(spy.calls).To(Equal(1))
			Expect(spy.gotGVKs[0]).To(Equal(schema.GroupVersionKind{
				Group: "apps", Version: "v2", Kind: "Deployment",
			}), "the level-0 GET must be issued with the entry GVK")
		})
	})

	Describe("UT-GW-2649-RES-002 [BR-SCOPE-001, AC-4, SI-10]: empty entry apiVersion handling", func() {
		It("fails closed without a registry (no derivable GVK)", func() {
			spy := &gvkCapturingReader{}
			resolver := ownerchain.NewK8sOwnerResolver(spy, logr.Discard())

			_, err := resolver.ResolveTopLevelOwnerIdentity(ctx, entry("production", "Deployment", "payment-api", ""))

			Expect(err).To(HaveOccurred(),
				"kind-only identity is never inferred on the identity path (#2593/#2592 fail-closed contract)")
			Expect(err.Error()).To(ContainSubstring("missing or invalid GVK"))
			Expect(spy.calls).To(Equal(0))
		})

		It("derives the apiVersion from the registry when one is configured", func() {
			registry := &fakeKindResolver{gvrs: map[string]schema.GroupVersionResource{
				"Deployment": {Group: "apps", Version: "v1", Resource: "deployments"},
			}}
			spy := &gvkCapturingReader{}
			resolver := ownerchain.NewK8sOwnerResolver(spy, logr.Discard(), ownerchain.WithRegistry(registry))

			resolved, err := resolver.ResolveTopLevelOwnerIdentity(ctx, entry("production", "Deployment", "payment-api", ""))

			Expect(err).NotTo(HaveOccurred())
			Expect(resolved.APIVersion).To(Equal("apps/v1"))
			Expect(spy.gotGVKs[0]).To(Equal(schema.GroupVersionKind{
				Group: "apps", Version: "v1", Kind: "Deployment",
			}), "the registry-derived GVK must drive the level-0 fetch")
		})
	})

	Describe("UT-GW-2649-RES-003 [BR-SCOPE-001, SI-10]: malformed entry apiVersion fails closed", func() {
		It("rejects a version-less group", func() {
			resolver := ownerchain.NewK8sOwnerResolver(&gvkCapturingReader{}, logr.Discard())

			_, err := resolver.ResolveTopLevelOwnerIdentity(ctx, entry("production", "Deployment", "payment-api", "apps/"))

			Expect(err).To(HaveOccurred(),
				"malformed entry identity must fail closed, never be silently ignored")
			Expect(err.Error()).To(ContainSubstring("missing or invalid GVK"))
		})
	})

	Describe("UT-GW-2649-RES-005 [BR-GATEWAY-004, AC-4]: a NotFound entry GVK fails closed with no cross-group substitution", func() {
		It("wraps the underlying NotFound instead of resolving via the static kind map", func() {
			spy := &gvkCapturingReader{err: notFound("kubevirt.io", "virtualmachines", "vm-1")}
			resolver := ownerchain.NewK8sOwnerResolver(spy, logr.Discard())

			_, err := resolver.ResolveTopLevelOwnerIdentity(ctx, entry("default", "VirtualMachine", "vm-1", "kubevirt.io/v1"))

			Expect(err).To(HaveOccurred(),
				"a well-formed but stale entry GVK must fail closed instead of being re-resolved via the kind map")
			Expect(apierrors.IsNotFound(err)).To(BeTrue(),
				"the underlying NotFound must remain detectable through the wrap")
			Expect(spy.calls).To(Equal(1),
				"no second lookup under a different API group may be attempted")
		})
	})

	Describe("UT-GW-2649-RES-008 [BR-GATEWAY-004, AC-4]: the #284 trust-but-verify re-fetch uses the entry GVK", func() {
		It("re-verifies a standalone resource with the entry GVK, not a kind-map one", func() {
			primary := &gvkCapturingReader{}
			fallback := &gvkCapturingReader{}
			resolver := ownerchain.NewK8sOwnerResolver(primary, logr.Discard(),
				ownerchain.WithFallbackReader(fallback))

			resolved, err := resolver.ResolveTopLevelOwnerIdentity(ctx, entry("production", "Deployment", "payment-api", "apps/v2"))

			Expect(err).NotTo(HaveOccurred())
			Expect(resolved.APIVersion).To(Equal("apps/v2"),
				"a verified standalone resource passes through with its entry apiVersion")
			Expect(primary.calls).To(Equal(1))
			Expect(fallback.calls).To(Equal(1))
			Expect(fallback.gotGVKs[0]).To(Equal(schema.GroupVersionKind{
				Group: "apps", Version: "v2", Kind: "Deployment",
			}), "the trust-but-verify re-fetch must use the entry GVK — using the mapped GVK would query a different API group's resource")
		})
	})

	// #2649 seed-verify increment: the two zero-fetch passthrough paths
	// (Service early-return and the CRD-gate stop) are verified when the
	// identity was caller-supplied or registry-derived. A fabricated
	// passthrough is only acceptable when the resolver has nothing to verify
	// it against.
	Describe("UT-GW-2649-RES-009 [BR-GATEWAY-004, BR-SCOPE-001, AC-4, SI-10]: a custom-group Service entry is verified, never rewritten", func() {
		It("verifies the seeded identity even when a same-named core Service exists", func() {
			primary := &gvkCapturingReader{}
			// If the rewrite (wrongly) ran, this reader would serve a
			// selector-bearing core Service and matching Pods.
			fallback := &serviceBackingReader{selector: map[string]string{"app": "payment-api"}}
			resolver := ownerchain.NewK8sOwnerResolver(primary, logr.Discard(),
				ownerchain.WithFallbackReader(fallback))

			resolved, err := resolver.ResolveTopLevelOwnerIdentity(ctx, entry("production", "Service", "payment-api", "custom.example/v1"))

			Expect(err).NotTo(HaveOccurred())
			Expect(resolved).To(Equal(entry("production", "Service", "payment-api", "custom.example/v1")),
				"a custom-group Service signal must keep its identity")
			Expect(fallback.serviceGets).To(Equal(0),
				"the Service-to-Pod rewrite must not run for a non-native Service identity")
			Expect(primary.calls).To(Equal(1))
			Expect(primary.gotGVKs[0]).To(Equal(schema.GroupVersionKind{
				Group: "custom.example", Version: "v1", Kind: "Service",
			}), "the verification GET must use the entry GVK")
		})
	})

	Describe("UT-GW-2649-RES-010 [BR-GATEWAY-004, BR-SCOPE-001, AC-4]: a custom-group Service NotFound fails closed without cross-group substitution", func() {
		It("returns the seeded lookup failure instead of a same-named native Service", func() {
			spy := &gvkCapturingReader{getErrs: []error{
				notFound("custom.example", "services", "payment-api"),
			}}
			resolver := ownerchain.NewK8sOwnerResolver(spy, logr.Discard())

			_, err := resolver.ResolveTopLevelOwnerIdentity(ctx, entry("production", "Service", "payment-api", "custom.example/v1"))

			Expect(err).To(HaveOccurred(),
				"a stale custom-group Service must fail closed — it must never be substituted by a different API group's same-named resource")
			Expect(err.Error()).To(ContainSubstring("custom.example/v1"),
				"the error must name the attempted GVK")
			Expect(apierrors.IsNotFound(err)).To(BeTrue(),
				"the underlying NotFound must remain detectable through the wrap")
			Expect(spy.calls).To(Equal(1),
				"no second lookup under a different API group may be attempted")
		})
	})

	Describe("UT-GW-2649-RES-011 [BR-GATEWAY-004, AC-4]: passthrough verification retries a primary NotFound through the fallback reader", func() {
		It("verifies the identity via the direct reader after a cache miss", func() {
			primary := &gvkCapturingReader{getErrs: []error{
				notFound("custom.example", "services", "payment-api"),
			}}
			fallback := &gvkCapturingReader{}
			resolver := ownerchain.NewK8sOwnerResolver(primary, logr.Discard(),
				ownerchain.WithFallbackReader(fallback))

			resolved, err := resolver.ResolveTopLevelOwnerIdentity(ctx, entry("production", "Service", "payment-api", "custom.example/v1"))

			Expect(err).NotTo(HaveOccurred())
			Expect(resolved).To(Equal(entry("production", "Service", "payment-api", "custom.example/v1")),
				"the identity is verified through the direct reader after the cached reader missed")
			Expect(primary.calls).To(Equal(1))
			Expect(fallback.calls).To(Equal(1))
			Expect(fallback.gotGVKs[0]).To(Equal(schema.GroupVersionKind{
				Group: "custom.example", Version: "v1", Kind: "Service",
			}), "the retry must reuse the entry GVK")
		})
	})

	Describe("UT-GW-2649-RES-012 [BR-GATEWAY-004, BR-SCOPE-001, AC-4, SI-10]: a CRD-kind stop is verified with one fetch", func() {
		It("fetches the entry GVK once and returns a verified passthrough", func() {
			spy := &gvkCapturingReader{}
			resolver := ownerchain.NewK8sOwnerResolver(spy, logr.Discard())

			resolved, err := resolver.ResolveTopLevelOwnerIdentity(ctx, entry("default", "VirtualMachine", "vm-1", "kubevirt.io/v1"))

			Expect(err).NotTo(HaveOccurred())
			Expect(resolved).To(Equal(entry("default", "VirtualMachine", "vm-1", "kubevirt.io/v1")),
				"CRD kinds pass through with their GVK only after it is verified by a fetch (fleet standalone-CRD path)")
			Expect(spy.calls).To(Equal(1),
				"exactly one verification GET must be issued for the entry GVK")
			Expect(spy.gotGVKs[0]).To(Equal(schema.GroupVersionKind{
				Group: "kubevirt.io", Version: "v1", Kind: "VirtualMachine",
			}), "the verification GET must use the entry GVK")
		})
	})

	Describe("UT-GW-2649-RES-013 [BR-GATEWAY-004, BR-SCOPE-001, AC-4]: a CRD-kind stop fails closed when the GET misses", func() {
		It("wraps the underlying GET error verbatim instead of synthesizing a not-found claim", func() {
			spy := &gvkCapturingReader{err: notFound("kubevirt.io", "virtualmachines", "vm-1")}
			resolver := ownerchain.NewK8sOwnerResolver(spy, logr.Discard())

			_, err := resolver.ResolveTopLevelOwnerIdentity(ctx, entry("default", "VirtualMachine", "vm-1", "kubevirt.io/v1"))

			Expect(err).To(HaveOccurred(),
				"an unverified CRD-kind identity must fail closed")
			Expect(apierrors.IsNotFound(err)).To(BeTrue(),
				"the underlying NotFound must be preserved through the wrap")
			Expect(err.Error()).To(ContainSubstring("not found"),
				"the underlying error text must survive verbatim (a remote REST-mapping error must stay diagnosable)")
		})
	})

	// RES-014 mirrors the gateway-fleet configuration exactly: a resolver
	// built as PrometheusAdapter.resolverForCluster builds its remote
	// resolvers (ownerchain.NewK8sOwnerResolver(reader, logger) — no
	// registry, no fallbackReader).
	Describe("UT-GW-2649-RES-014 [BR-GATEWAY-004, BR-SCOPE-001, AC-4, SI-10]: the gateway-fleet remote configuration resolves verified", func() {
		It("returns the entry identity through a resolver configured like resolverForCluster", func() {
			spy := &gvkCapturingReader{}
			resolver := ownerchain.NewK8sOwnerResolver(spy, logr.Discard())

			resolved, err := resolver.ResolveTopLevelOwnerIdentity(ctx, entry("fleet-ns", "Service", "edge-svc", "custom.example/v1"))

			Expect(err).NotTo(HaveOccurred())
			Expect(resolved).To(Equal(entry("fleet-ns", "Service", "edge-svc", "custom.example/v1")),
				"the fleet resolver has no fallbackReader, so the Service rewrite cannot run; the passthrough must be verified via the primary reader")
			Expect(spy.calls).To(Equal(1))
			Expect(spy.gotGVKs[0]).To(Equal(schema.GroupVersionKind{
				Group: "custom.example", Version: "v1", Kind: "Service",
			}))
		})
	})

	Describe("UT-GW-2649-RES-016 [BR-GATEWAY-004, BR-SCOPE-001, AC-4]: a core v1 Service entry keeps the Service-to-Pod rewrite", func() {
		It("rewrites to the backing Pod and resolves its workload owner under the core group", func() {
			controller := true
			primary := &gvkCapturingReader{ownerRef: &metav1.OwnerReference{
				APIVersion: "apps/v1", Kind: "Deployment", Name: "payment-api", Controller: &controller,
			}}
			fallback := &serviceBackingReader{selector: map[string]string{"app": "payment-api"}}
			resolver := ownerchain.NewK8sOwnerResolver(primary, logr.Discard(),
				ownerchain.WithFallbackReader(fallback))

			resolved, err := resolver.ResolveTopLevelOwnerIdentity(ctx, entry("production", "Service", "payment-api", "v1"))

			Expect(err).NotTo(HaveOccurred())
			Expect(resolved.Kind).To(Equal("Deployment"),
				"the native Service rewrite must still find the backing workload")
			Expect(resolved.APIVersion).To(Equal("apps/v1"))
			Expect(fallback.serviceGets).To(Equal(1),
				"the rewrite runs for a native core Service identity")
			Expect(fallback.podLists).To(Equal(1))
			Expect(primary.gotGVKs[0]).To(Equal(schema.GroupVersionKind{
				Version: "v1", Kind: "Pod",
			}), "the rewritten Pod must be fetched under the core group, not the Service's input identity")
		})
	})

	Describe("UT-GW-2649-RES-017 [BR-GATEWAY-004, BR-SCOPE-001, AC-4]: a non-v1 core Service entry skips the rewrite and is verified under its version", func() {
		It("verifies the entry version instead of rewriting through a v1 Service", func() {
			primary := &gvkCapturingReader{}
			// If the rewrite (wrongly) ran for a v2 entry, this reader would
			// serve a v1 Service and matching Pods.
			fallback := &serviceBackingReader{selector: map[string]string{"app": "payment-api"}}
			resolver := ownerchain.NewK8sOwnerResolver(primary, logr.Discard(),
				ownerchain.WithFallbackReader(fallback))

			resolved, err := resolver.ResolveTopLevelOwnerIdentity(ctx, entry("production", "Service", "payment-api", "v2"))

			Expect(err).NotTo(HaveOccurred())
			Expect(resolved).To(Equal(entry("production", "Service", "payment-api", "v2")),
				"a v2 Service keeps its identity — the rewrite reads v1 objects and must not run")
			Expect(fallback.serviceGets).To(Equal(0),
				"the v1-based Service-to-Pod rewrite must not run for a non-v1 core identity")
			Expect(primary.calls).To(Equal(1))
			Expect(primary.gotGVKs[0]).To(Equal(schema.GroupVersionKind{
				Version: "v2", Kind: "Service",
			}), "the verification GET must use the entry version")
		})
	})

	Describe("UT-GW-2649-RES-018 [BR-GATEWAY-004, BR-SCOPE-001, AC-4]: the rewritten Pod keeps its core v1 identity against a colliding registry mapping", func() {
		It("resolves the workload owner even when the registry maps Pod to a custom group", func() {
			// The identity path threads the GVK directly (never consulting the
			// registry for a fetch GVK), so a colliding Pod mapping can
			// neither redirect the level-0 Pod fetch nor stop traversal at
			// the CRD gate.
			registry := &fakeKindResolver{
				gvrs: map[string]schema.GroupVersionResource{
					"Pod":        {Group: "custom.example", Version: "v1", Resource: "pods"},
					"Deployment": {Group: "apps", Version: "v1", Resource: "deployments"},
				},
				nonCore: map[string]bool{"Pod": true},
			}
			controller := true
			primary := &gvkCapturingReader{}
			fallback := &serviceBackingReader{
				selector: map[string]string{"app": "payment-api"},
				podOwnerRef: &metav1.OwnerReference{
					APIVersion: "apps/v1", Kind: "Deployment", Name: "payment-api", Controller: &controller,
				},
			}
			resolver := ownerchain.NewK8sOwnerResolver(primary, logr.Discard(),
				ownerchain.WithRegistry(registry),
				ownerchain.WithFallbackReader(fallback))

			resolved, err := resolver.ResolveTopLevelOwnerIdentity(ctx, entry("production", "Service", "payment-api", "v1"))

			Expect(err).NotTo(HaveOccurred())
			Expect(resolved).To(Equal(entry("production", "Deployment", "payment-api", "apps/v1")),
				"the rewritten Pod is a known core v1 object; a colliding registry mapping must not stop traversal or redirect its fetch")
			Expect(fallback.serviceGets).To(Equal(1))
			Expect(primary.gotGVKs[0]).To(Equal(schema.GroupVersionKind{
				Version: "v1", Kind: "Pod",
			}), "the level-0 Pod fetch must use the forced core v1 identity, not the registry's custom group")
			Expect(fallback.metaGVKs[0]).To(Equal(schema.GroupVersionKind{
				Version: "v1", Kind: "Pod",
			}), "the #284 re-verification of the rewritten Pod must use the core v1 identity, not the registry's custom group")
			Expect(primary.gotGVKs).To(ContainElement(schema.GroupVersionKind{
				Group: "apps", Version: "v1", Kind: "Deployment",
			}), "the traversal must proceed to the workload owner under the apps group")
		})
	})

	// #2649: the CRD-gate stop at level > 0 fires after an ownerReference
	// advanced the identity but before the owner itself was fetched — the
	// returned owner passthrough must be verified like any other.
	Describe("UT-GW-2649-RES-019 [BR-GATEWAY-004, BR-SCOPE-001, AC-4]: a CRD owner reached via an ownerReference is verified before being returned", func() {
		It("fetches the owner under its ownerReference GVK and returns the verified identity", func() {
			controller := true
			spy := &gvkCapturingReader{ownerRef: &metav1.OwnerReference{
				APIVersion: "kubevirt.io/v1", Kind: "VirtualMachine", Name: "vm-1", Controller: &controller,
			}}
			resolver := ownerchain.NewK8sOwnerResolver(spy, logr.Discard())

			resolved, err := resolver.ResolveTopLevelOwnerIdentity(ctx, entry("default", "Pod", "pod-1", "v1"))

			Expect(err).NotTo(HaveOccurred())
			Expect(resolved).To(Equal(entry("default", "VirtualMachine", "vm-1", "kubevirt.io/v1")),
				"a CRD owner reached via ownerReference is returned only after its existence is verified")
			Expect(spy.calls).To(Equal(2),
				"level-0 Pod fetch plus exactly one owner verification fetch")
			Expect(spy.gotGVKs[1]).To(Equal(schema.GroupVersionKind{
				Group: "kubevirt.io", Version: "v1", Kind: "VirtualMachine",
			}), "the owner verification fetch must use the ownerReference's GVK")
		})
	})

	Describe("UT-GW-2649-RES-020 [BR-GATEWAY-004, BR-SCOPE-001, AC-4]: a ghost CRD owner reached via an ownerReference fails closed", func() {
		It("wraps the verification failure instead of returning the unverified owner", func() {
			controller := true
			spy := &gvkCapturingReader{
				ownerRef: &metav1.OwnerReference{
					APIVersion: "kubevirt.io/v1", Kind: "VirtualMachine", Name: "vm-1", Controller: &controller,
				},
				getErrs: []error{nil, notFound("kubevirt.io", "virtualmachines", "vm-1")},
			}
			resolver := ownerchain.NewK8sOwnerResolver(spy, logr.Discard())

			_, err := resolver.ResolveTopLevelOwnerIdentity(ctx, entry("default", "Pod", "pod-1", "v1"))

			Expect(err).To(HaveOccurred(),
				"a deleted CRD owner must fail closed instead of being returned as a resolved identity")
			Expect(apierrors.IsNotFound(err)).To(BeTrue(),
				"the underlying NotFound must be preserved through the wrap")
			Expect(err.Error()).To(ContainSubstring("VirtualMachine"),
				"the error must name the owner whose verification failed")
		})
	})
})
