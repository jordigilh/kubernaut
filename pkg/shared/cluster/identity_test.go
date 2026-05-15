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

package cluster_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apitypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/jordigilh/kubernaut/pkg/shared/cluster"
)

var _ = Describe("Issue #615: Cluster Identity Discovery", func() {
	var (
		scheme *runtime.Scheme
		ctx    context.Context
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		ctx = context.Background()
	})

	It("UT-SHARED-615-001: should discover UUID from kube-system namespace UID", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kube-system",
				UID:  apitypes.UID("test-uuid-001"),
			},
		}
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()

		identity, err := cluster.DiscoverIdentity(ctx, cl)
		Expect(err).ToNot(HaveOccurred())
		Expect(identity.UUID).To(Equal("test-uuid-001"))
	})

	It("UT-SHARED-615-002: should discover name from OCP infrastructure resource", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kube-system",
				UID:  apitypes.UID("ocp-uuid"),
			},
		}

		infraGVK := schema.GroupVersionKind{
			Group: "config.openshift.io", Version: "v1", Kind: "Infrastructure",
		}
		scheme.AddKnownTypeWithName(infraGVK, &unstructured.Unstructured{})
		infraListGVK := schema.GroupVersionKind{
			Group: "config.openshift.io", Version: "v1", Kind: "InfrastructureList",
		}
		scheme.AddKnownTypeWithName(infraListGVK, &unstructured.UnstructuredList{})

		infra := &unstructured.Unstructured{}
		infra.SetGroupVersionKind(infraGVK)
		infra.SetName("cluster")
		_ = unstructured.SetNestedField(infra.Object, "ocp-prod", "status", "infrastructureName")

		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, infra).Build()

		identity, err := cluster.DiscoverIdentity(ctx, cl)
		Expect(err).ToNot(HaveOccurred())
		Expect(identity.Name).To(Equal("ocp-prod"))
		Expect(identity.UUID).To(Equal("ocp-uuid"))
	})

	It("UT-SHARED-615-003: should discover name from Kind node label", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kube-system",
				UID:  apitypes.UID("kind-uuid"),
			},
		}
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kind-control-plane",
				Labels: map[string]string{
					"io.x-k8s.kind.cluster": "kind-demo",
				},
			},
		}
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, node).Build()

		identity, err := cluster.DiscoverIdentity(ctx, cl)
		Expect(err).ToNot(HaveOccurred())
		Expect(identity.Name).To(Equal("kind-demo"))
		Expect(identity.UUID).To(Equal("kind-uuid"))
	})

	It("UT-SHARED-615-004: should return empty name when neither OCP nor Kind is detected", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kube-system",
				UID:  apitypes.UID("generic-uuid"),
			},
		}
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "worker-01",
				Labels: map[string]string{},
			},
		}
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, node).Build()

		identity, err := cluster.DiscoverIdentity(ctx, cl)
		Expect(err).ToNot(HaveOccurred())
		Expect(identity.Name).To(BeEmpty())
		Expect(identity.UUID).To(Equal("generic-uuid"))
	})

	It("UT-SHARED-615-005: should return error when kube-system namespace is missing", func() {
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()

		identity, err := cluster.DiscoverIdentity(ctx, cl)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("kube-system"))
		Expect(identity.UUID).To(BeEmpty())
	})
})
