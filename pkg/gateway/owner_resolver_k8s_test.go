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

package gateway_test

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/types"
	"github.com/jordigilh/kubernaut/pkg/shared/k8s/ownerchain"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func boolPtr(b bool) *bool { return &b }

// UT-GW-2659-CHAR: characterization suite for the #2659 compatibility-path
// removal. Every scenario here runs through ResolveTopLevelOwnerIdentity —
// the only production entry point (BR-GATEWAY-004, BR-SCOPE-001) — so the
// behavioral coverage of the deleted kind-only entry (Service→Pod edge
// cases, #284 trust-but-verify) is preserved on the identity path. Chain,
// standalone, cross-group-substitution and depth scenarios are covered by
// owner_resolver_apiversion_test.go and owner_gvk_handoff_test.go and are
// not duplicated here.
var _ = Describe("K8sOwnerResolver - identity-path owner resolution (#2659)", func() {
	const namespace = "demo-memory-leak"

	var (
		ctx        context.Context
		scheme     *runtime.Scheme
		fakeClient client.Client
		resolver   *ownerchain.K8sOwnerResolver
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())
	})

	setup := func(objs ...client.Object) {
		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(objs...).
			Build()
		resolver = ownerchain.NewK8sOwnerResolver(fakeClient, logr.Discard(),
			ownerchain.WithFallbackReader(fakeClient))
	}

	Describe("Service -> workload traversal (#1029 Use Case 3)", func() {
		It("UT-GW-1029-041: Service → selector → Pods → StatefulSet resolves to StatefulSet", func() {
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: "etcd-client", Namespace: namespace,
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "etcd"},
				},
			}
			sts := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "etcd", Namespace: namespace,
				},
			}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "etcd-1", Namespace: namespace,
					Labels: map[string]string{"app": "etcd"},
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1", Kind: "StatefulSet", Name: "etcd",
						Controller: boolPtr(true),
					}},
				},
			}

			setup(svc, sts, pod)

			resolved, err := resolver.ResolveTopLevelOwnerIdentity(ctx, sharedtypes.OwnerChainEntry{
				Namespace: namespace, Kind: "Service", Name: "etcd-client", APIVersion: "v1",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(resolved.Kind).To(Equal("StatefulSet"))
			Expect(resolved.Name).To(Equal("etcd"))
			Expect(resolved.APIVersion).To(Equal("apps/v1"))
		})

		It("UT-GW-1029-042: Service → selector → Pods → ReplicaSet → Deployment resolves to Deployment", func() {
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: "api-svc", Namespace: namespace,
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "api"},
				},
			}
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "api-server", Namespace: namespace,
				},
			}
			rs := &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "api-server-abc123", Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1", Kind: "Deployment", Name: "api-server",
						Controller: boolPtr(true),
					}},
				},
			}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "api-server-abc123-xyz", Namespace: namespace,
					Labels: map[string]string{"app": "api"},
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "api-server-abc123",
						Controller: boolPtr(true),
					}},
				},
			}

			setup(svc, deploy, rs, pod)

			resolved, err := resolver.ResolveTopLevelOwnerIdentity(ctx, sharedtypes.OwnerChainEntry{
				Namespace: namespace, Kind: "Service", Name: "api-svc", APIVersion: "v1",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(resolved.Kind).To(Equal("Deployment"))
			Expect(resolved.Name).To(Equal("api-server"))
			Expect(resolved.APIVersion).To(Equal("apps/v1"))
		})

		It("UT-GW-1029-043: Service with no matching pods returns the verified Service identity", func() {
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: "orphan-svc", Namespace: namespace,
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "nonexistent"},
				},
			}

			setup(svc)

			resolved, err := resolver.ResolveTopLevelOwnerIdentity(ctx, sharedtypes.OwnerChainEntry{
				Namespace: namespace, Kind: "Service", Name: "orphan-svc", APIVersion: "v1",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(resolved.Kind).To(Equal("Service"))
			Expect(resolved.Name).To(Equal("orphan-svc"))
			Expect(resolved.APIVersion).To(Equal("v1"))
		})

		It("UT-GW-1029-044: Service without selector returns the verified Service identity", func() {
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: "external-svc", Namespace: namespace,
				},
				Spec: corev1.ServiceSpec{
					Type:         corev1.ServiceTypeExternalName,
					ExternalName: "external.example.com",
				},
			}

			setup(svc)

			resolved, err := resolver.ResolveTopLevelOwnerIdentity(ctx, sharedtypes.OwnerChainEntry{
				Namespace: namespace, Kind: "Service", Name: "external-svc", APIVersion: "v1",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(resolved.Kind).To(Equal("Service"))
			Expect(resolved.Name).To(Equal("external-svc"))
			Expect(resolved.APIVersion).To(Equal("v1"))
		})

		It("UT-GW-1029-045: Service → selector → standalone Pod (no owners) returns the Pod identity", func() {
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: "debug-svc", Namespace: namespace,
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "debug"},
				},
			}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "debug-pod", Namespace: namespace,
					Labels: map[string]string{"app": "debug"},
				},
			}

			setup(svc, pod)

			resolved, err := resolver.ResolveTopLevelOwnerIdentity(ctx, sharedtypes.OwnerChainEntry{
				Namespace: namespace, Kind: "Service", Name: "debug-svc", APIVersion: "v1",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(resolved.Kind).To(Equal("Pod"))
			Expect(resolved.Name).To(Equal("debug-pod"))
			Expect(resolved.APIVersion).To(Equal("v1"))
		})

		It("UT-GW-1029-046: without fallbackReader, Service returns the verified Service identity", func() {
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: "some-svc", Namespace: namespace,
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "some"},
				},
			}

			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(svc).
				Build()
			// No fallbackReader — the Service→Pod rewrite cannot run
			resolver = ownerchain.NewK8sOwnerResolver(fakeClient, logr.Discard())

			resolved, err := resolver.ResolveTopLevelOwnerIdentity(ctx, sharedtypes.OwnerChainEntry{
				Namespace: namespace, Kind: "Service", Name: "some-svc", APIVersion: "v1",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(resolved.Kind).To(Equal("Service"))
			Expect(resolved.Name).To(Equal("some-svc"))
			Expect(resolved.APIVersion).To(Equal("v1"))
		})

		It("UT-GW-1029-047: deterministic pod selection — multiple matching pods resolve to the same owner", func() {
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: "etcd-client", Namespace: namespace,
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "etcd"},
				},
			}
			sts := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "etcd", Namespace: namespace,
				},
			}
			pod0 := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "etcd-0", Namespace: namespace,
					Labels: map[string]string{"app": "etcd"},
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1", Kind: "StatefulSet", Name: "etcd",
						Controller: boolPtr(true),
					}},
				},
			}
			pod1 := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "etcd-1", Namespace: namespace,
					Labels: map[string]string{"app": "etcd"},
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1", Kind: "StatefulSet", Name: "etcd",
						Controller: boolPtr(true),
					}},
				},
			}

			setup(svc, sts, pod0, pod1)

			resolved, err := resolver.ResolveTopLevelOwnerIdentity(ctx, sharedtypes.OwnerChainEntry{
				Namespace: namespace, Kind: "Service", Name: "etcd-client", APIVersion: "v1",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(resolved.Kind).To(Equal("StatefulSet"))
			Expect(resolved.Name).To(Equal("etcd"))
			Expect(resolved.APIVersion).To(Equal("apps/v1"))
		})
	})

	Describe("Trust-but-verify fail-closed (#284, #2659)", func() {
		It("UT-GW-2659-284-001: returns an error when the cache has a Pod but the direct API cannot find it (deleted)", func() {
			// Cache has a stale entry for a deleted pod (no ownerReferences)
			stalePod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deleted-pod", Namespace: namespace,
				},
			}
			cacheClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(stalePod).
				Build()
			// Direct API has no such pod (it's been garbage-collected)
			apiClient := fake.NewClientBuilder().
				WithScheme(scheme).
				Build()

			resolver = ownerchain.NewK8sOwnerResolver(cacheClient, logr.Discard(),
				ownerchain.WithFallbackReader(apiClient))

			_, err := resolver.ResolveTopLevelOwnerIdentity(ctx, sharedtypes.OwnerChainEntry{
				Namespace: namespace, Kind: "Pod", Name: "deleted-pod", APIVersion: "v1",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found via direct API"))
		})

		It("UT-GW-2659-284-002: resolves via the direct API when the cache returns a Pod without ownerReferences (#284 advance branch)", func() {
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "leaky-app", Namespace: namespace,
				},
			}
			rs := &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "leaky-app-544b75986", Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1", Kind: "Deployment", Name: "leaky-app",
						Controller: boolPtr(true),
					}},
				},
			}

			// Cache has the Pod WITHOUT ownerReferences (stale metadata)
			stalePod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "leaky-app-544b75986-hndrn", Namespace: namespace,
				},
			}
			cacheClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(stalePod).
				Build()

			// Direct API has the Pod WITH ownerReferences (fresh metadata)
			freshPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "leaky-app-544b75986-hndrn", Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "leaky-app-544b75986",
						Controller: boolPtr(true),
					}},
				},
			}
			apiClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(deploy, rs, freshPod).
				Build()

			resolver = ownerchain.NewK8sOwnerResolver(cacheClient, logr.Discard(),
				ownerchain.WithFallbackReader(apiClient))

			resolved, err := resolver.ResolveTopLevelOwnerIdentity(ctx, sharedtypes.OwnerChainEntry{
				Namespace: namespace, Kind: "Pod", Name: "leaky-app-544b75986-hndrn", APIVersion: "v1",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(resolved.Kind).To(Equal("Deployment"))
			Expect(resolved.Name).To(Equal("leaky-app"))
			Expect(resolved.APIVersion).To(Equal("apps/v1"))
		})
	})

	Describe("Two pods from same Deployment produce same fingerprint (#270 bug scenario)", func() {
		It("should produce identical deployment-level fingerprints", func() {
			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "leaky-app", Namespace: namespace,
				},
			}
			rs := &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "leaky-app-544b75986", Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1", Kind: "Deployment", Name: "leaky-app",
						Controller: boolPtr(true),
					}},
				},
			}
			podA := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "leaky-app-544b75986-whccw", Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "leaky-app-544b75986",
						Controller: boolPtr(true),
					}},
				},
			}
			podB := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "leaky-app-544b75986-jxtck", Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "leaky-app-544b75986",
						Controller: boolPtr(true),
					}},
				},
			}

			setup(deploy, rs, podA, podB)

			resourceA := types.ResourceIdentifier{Namespace: namespace, Kind: "Pod", Name: podA.Name, APIVersion: corev1.SchemeGroupVersion.String()}
			resourceB := types.ResourceIdentifier{Namespace: namespace, Kind: "Pod", Name: podB.Name, APIVersion: corev1.SchemeGroupVersion.String()}

			fpA, _, err := types.ResolveFingerprint(ctx, resolver, resourceA, logr.Discard())
			Expect(err).ToNot(HaveOccurred())
			fpB, _, err := types.ResolveFingerprint(ctx, resolver, resourceB, logr.Discard())
			Expect(err).ToNot(HaveOccurred())

			Expect(fpA).To(Equal(fpB),
				"Two pods from the same Deployment must produce identical fingerprints")

			expectedFP := types.CalculateOwnerFingerprint(types.ResourceIdentifier{
				Namespace: namespace, Kind: "Deployment", Name: "leaky-app", APIVersion: "apps/v1",
			})
			Expect(fpA).To(Equal(expectedFP),
				"Fingerprint should be SHA256(apps:namespace:Deployment:leaky-app)")
		})
	})
})
