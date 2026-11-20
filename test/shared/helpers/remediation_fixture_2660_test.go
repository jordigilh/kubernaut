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

package helpers

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

func TestHelpers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Shared Helpers Suite")
}

var _ = Describe("Remediation target fixture (#2660 n4, BR-ORCH-025)", func() {
	var (
		ctx       context.Context
		k8sClient client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
	})

	It("UT-HELPERS-2660-001 [BR-ORCH-025]: fails loudly for a namespaced target kind the fixture cannot create", func() {
		err := ensureTargetFixture(ctx, k8sClient, remediationv1.ResourceIdentifier{
			Kind:      "StatefulSet",
			Name:      "orders",
			Namespace: "payments",
		})
		Expect(err).To(HaveOccurred(), "an unsupported kind must not silently skip the fixture")
		Expect(err.Error()).To(ContainSubstring("StatefulSet"))
		Expect(err.Error()).To(ContainSubstring("unsupported kind"))
	})

	It("UT-HELPERS-2660-002 [BR-ORCH-025]: creates Pod and Deployment fixtures", func() {
		Expect(ensureTargetFixture(ctx, k8sClient, remediationv1.ResourceIdentifier{
			Kind: "Pod", Name: "api-pod", Namespace: "production",
		})).To(Succeed())
		pod := &corev1.Pod{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "production", Name: "api-pod"}, pod)).To(Succeed())

		Expect(ensureTargetFixture(ctx, k8sClient, remediationv1.ResourceIdentifier{
			Kind: fixtureKindDeployment, Name: "api", Namespace: "production",
		})).To(Succeed())
		deploy := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "production", Name: "api"}, deploy)).To(Succeed())
	})

	It("UT-HELPERS-2660-003 [BR-ORCH-025]: leaves cluster-scoped targets to their own fixture", func() {
		Expect(ensureTargetFixture(ctx, k8sClient, remediationv1.ResourceIdentifier{
			Kind: "Node", Name: "worker-01",
		})).To(Succeed())
		node := &corev1.Node{}
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Name: "worker-01"}, node))).To(BeTrue(),
			"a cluster-scoped target is not created here")
	})
})
