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

package adapters

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func boolPtr(b bool) *bool { return &b }

var _ = Describe("Owner Resolver with Registry (#1029)", func() {
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
		It("UT-GW-1029-024: resolves Pod→ReplicaSet→Deployment using registry GVR", func() {
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

			resolver := adapters.NewK8sOwnerResolver(fakeClient, logr.Discard(),
				adapters.WithRegistry(registry))

			ownerKind, ownerName, err := resolver.ResolveTopLevelOwner(ctx, namespace, "Pod", "api-server-abc-xyz")
			Expect(err).ToNot(HaveOccurred())
			Expect(ownerKind).To(Equal("Deployment"))
			Expect(ownerName).To(Equal("api-server"))
		})

		It("UT-GW-1029-025: uses registry for GVR lookup instead of static kindToGroup", func() {
			// Verify the resolver works with the registry for a kind that
			// exists in the registry (Pod → standalone, no owner)
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

			resolver := adapters.NewK8sOwnerResolver(fakeClient, logr.Discard(),
				adapters.WithRegistry(registry))

			ownerKind, ownerName, err := resolver.ResolveTopLevelOwner(ctx, namespace, "Pod", "standalone-pod")
			Expect(err).ToNot(HaveOccurred())
			Expect(ownerKind).To(Equal("Pod"))
			Expect(ownerName).To(Equal("standalone-pod"))
		})
	})

	// =========================================================================
	// CRD Traversal Control (UT-GW-1029-026..027)
	// =========================================================================
	Context("CRD Traversal Control", func() {
		It("UT-GW-1029-026: stops traversal at CRD kind (BuildConfig not in core/apps/batch)", func() {
			// When the resolver encounters a CRD kind (e.g., BuildConfig),
			// it should stop traversal because owner chains for CRDs are
			// unpredictable and cluster-specific.
			resolver := adapters.NewK8sOwnerResolver(
				fake.NewClientBuilder().WithScheme(scheme).Build(),
				logr.Discard(),
				adapters.WithRegistry(registry),
			)

			// BuildConfig is an OpenShift CRD — not in core/apps/batch
			ownerKind, ownerName, err := resolver.ResolveTopLevelOwner(ctx, namespace, "BuildConfig", "frontend-build")
			Expect(err).ToNot(HaveOccurred())
			Expect(ownerKind).To(Equal("BuildConfig"))
			Expect(ownerName).To(Equal("frontend-build"))
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

			resolver := adapters.NewK8sOwnerResolver(fakeClient, logr.Discard(),
				adapters.WithRegistry(registry))

			ownerKind, ownerName, err := resolver.ResolveTopLevelOwner(ctx, namespace, "ReplicaSet", "web-app-rs")
			Expect(err).ToNot(HaveOccurred())
			Expect(ownerKind).To(Equal("Deployment"))
			Expect(ownerName).To(Equal("web-app"))
		})
	})
})
