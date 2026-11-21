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

package adapters_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/shared/k8s/ownerchain"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func boolPtr(b bool) *bool { return &b }

// gvkRecordingReader is a client.Reader spy for the identity-path registry
// tests: it records the GVK stamped on each PartialObjectMetadata GET and
// serves a standalone (ownerless) object, so registry-derived GVKs and CRD
// stop verification are observable (#2659).
type gvkRecordingReader struct {
	gotGVKs []schema.GroupVersionKind
}

func (s *gvkRecordingReader) Get(_ context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	meta, ok := obj.(*metav1.PartialObjectMetadata)
	if !ok {
		return fmt.Errorf("unsupported object type %T", obj)
	}
	s.gotGVKs = append(s.gotGVKs, obj.GetObjectKind().GroupVersionKind())
	meta.Namespace, meta.Name = key.Namespace, key.Name
	return nil
}

func (s *gvkRecordingReader) List(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
	return errors.New("list not implemented in spy")
}

// UT-GW-2659-REG: real APIResourceRegistry integration on the identity path
// (BR-GATEWAY-004, BR-SCOPE-001). The registry derives the entry GVK when
// the caller supplies none, and its CRD classification stops traversal with
// a verified passthrough — never a kind-table fetch (#2659).
var _ = Describe("Owner Resolver with Registry on the identity path (#1029, #2659)", func() {
	const namespace = "test-ns"

	var (
		ctx      context.Context
		scheme   *runtime.Scheme
		registry *adapters.APIResourceRegistry
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())

		fd := newFakeDiscovery(standardResources(), ocpResources())
		var err error
		registry, err = adapters.NewAPIResourceRegistry(fd)
		Expect(err).ToNot(HaveOccurred())
	})

	// =========================================================================
	// Registry-Backed GVR Lookup (UT-GW-1029-024..025)
	// =========================================================================
	Context("Registry-Backed GVR Lookup", func() {
		It("UT-GW-1029-024: resolves Pod→ReplicaSet→Deployment using registry-derived GVKs", func() {
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "api-server", Namespace: namespace,
					UID: "deploy-uid-1",
				},
			}
			rs := &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "api-server-abc", Namespace: namespace,
					UID: "rs-uid-1",
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1", Kind: "Deployment",
						Name: "api-server", UID: "deploy-uid-1",
						Controller: boolPtr(true),
					}},
				},
			}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "api-server-abc-xyz", Namespace: namespace,
					UID: "pod-uid-1",
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1", Kind: "ReplicaSet",
						Name: "api-server-abc", UID: "rs-uid-1",
						Controller: boolPtr(true),
					}},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(deployment, rs, pod).
				Build()

			resolver := ownerchain.NewK8sOwnerResolver(fakeClient, logr.Discard(),
				ownerchain.WithRegistry(registry))

			resolved, err := resolver.ResolveTopLevelOwnerIdentity(ctx, sharedtypes.OwnerChainEntry{
				Namespace: namespace, Kind: "Pod", Name: "api-server-abc-xyz",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(resolved.Kind).To(Equal("Deployment"))
			Expect(resolved.Name).To(Equal("api-server"))
			Expect(resolved.APIVersion).To(Equal("apps/v1"),
				"the owner's authority must come from its ownerReference, not the registry preference")
		})

		It("UT-GW-1029-025: uses registry for GVK lookup instead of static kindToGroup", func() {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "standalone-pod", Namespace: namespace,
					UID: "pod-uid-2",
				},
			}
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(pod).
				Build()

			resolver := ownerchain.NewK8sOwnerResolver(fakeClient, logr.Discard(),
				ownerchain.WithRegistry(registry))

			resolved, err := resolver.ResolveTopLevelOwnerIdentity(ctx, sharedtypes.OwnerChainEntry{
				Namespace: namespace, Kind: "Pod", Name: "standalone-pod",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(resolved.Kind).To(Equal("Pod"))
			Expect(resolved.Name).To(Equal("standalone-pod"))
			Expect(resolved.APIVersion).To(Equal("v1"),
				"the registry must supply the GVK the static kind table would have guessed")
		})
	})

	// =========================================================================
	// CRD Traversal Control (UT-GW-1029-026..027)
	// =========================================================================
	Context("CRD Traversal Control", func() {
		It("UT-GW-1029-026: stops traversal at a CRD kind with a verified passthrough under the registry-derived GVK", func() {
			spy := &gvkRecordingReader{}
			resolver := ownerchain.NewK8sOwnerResolver(spy, logr.Discard(),
				ownerchain.WithRegistry(registry))

			// BuildConfig is an OpenShift CRD — not in core/apps/batch. The
			// registry derives its GVK; the identity path must verify the
			// passthrough with exactly one fetch under that GVK.
			resolved, err := resolver.ResolveTopLevelOwnerIdentity(ctx, sharedtypes.OwnerChainEntry{
				Namespace: namespace, Kind: "BuildConfig", Name: "frontend-build",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(resolved.Kind).To(Equal("BuildConfig"))
			Expect(resolved.Name).To(Equal("frontend-build"))
			Expect(resolved.APIVersion).To(Equal("build.openshift.io/v1"),
				"the registry-derived apiVersion must be preserved on the passthrough")
			Expect(spy.gotGVKs).To(HaveLen(1),
				"exactly one verification GET must be issued")
			Expect(spy.gotGVKs[0]).To(Equal(schema.FromAPIVersionAndKind("build.openshift.io/v1", "BuildConfig")),
				"the verification GET must use the registry-derived GVK, never the static kind table")
		})

		It("UT-GW-1029-027: traverses core/apps/batch kinds normally", func() {
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "web-app", Namespace: namespace,
					UID: "deploy-uid-3",
				},
			}
			rs := &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "web-app-rs", Namespace: namespace,
					UID: "rs-uid-3",
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1", Kind: "Deployment",
						Name: "web-app", UID: "deploy-uid-3",
						Controller: boolPtr(true),
					}},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(deployment, rs).
				Build()

			resolver := ownerchain.NewK8sOwnerResolver(fakeClient, logr.Discard(),
				ownerchain.WithRegistry(registry))

			resolved, err := resolver.ResolveTopLevelOwnerIdentity(ctx, sharedtypes.OwnerChainEntry{
				Namespace: namespace, Kind: "ReplicaSet", Name: "web-app-rs",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(resolved.Kind).To(Equal("Deployment"))
			Expect(resolved.Name).To(Equal("web-app"))
			Expect(resolved.APIVersion).To(Equal("apps/v1"))
		})
	})
})
