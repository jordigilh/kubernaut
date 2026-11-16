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

package fleetmetadatacache_test

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

// IT-REG-GW-005 proves BR-INTEGRATION-065's runtime health contract against
// the real envtest API: the registry must reconcile its authoritative state
// after startup, not remain permanently healthy based only on its first sync.
// FedRAMP SI-4 is satisfied by detecting API-backed registry health changes.
var _ = Describe("IT-REG-GW-005 [SI-4]: authoritative registry health refresh", Label("fmc", "integration"), func() {
	DescribeTable("IT-REG-REVIEW-GUARD-001 [BR-INTEGRATION-065]: production registries separate per-registration proofs from membership generation", func(kuadrant bool) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		east := fmt.Sprintf("it-reg-review-east-%t-%d", kuadrant, GinkgoParallelProcess())
		west := fmt.Sprintf("it-reg-review-west-%t-%d", kuadrant, GinkgoParallelProcess())
		var reg registry.ClusterRegistry
		var gvr schema.GroupVersionResource
		if kuadrant {
			reg = registry.NewKuadrantRegistry(dynClient, registry.EAIGWRegistryConfig{Namespace: "default"}, nil, logr.Discard())
			gvr = registry.MCPServerRegistrationGVR
			createMCPServerRegistration(ctx, east)
			createMCPServerRegistration(ctx, west)
			DeferCleanup(deleteMCPServerRegistration, context.Background(), east)
			DeferCleanup(deleteMCPServerRegistration, context.Background(), west)
		} else {
			reg = registry.NewEAIGWRegistry(dynClient, registry.EAIGWRegistryConfig{Namespace: "default"}, nil, logr.Discard())
			gvr = registry.BackendGVR
			createBackend(ctx, east)
			createBackend(ctx, west)
			DeferCleanup(deleteBackend, context.Background(), east)
			DeferCleanup(deleteBackend, context.Background(), west)
		}
		Expect(reg.Start(ctx)).To(Succeed())
		DeferCleanup(reg.Stop)
		guard, ok := reg.(registry.CurrentRegistrationGuard)
		Expect(ok).To(BeTrue())
		snapshotter, ok := reg.(registry.RegistrySnapshotter)
		Expect(ok).To(BeTrue())
		eastProof, found := guard.Snapshot(east)
		Expect(found).To(BeTrue())
		westProof, found := guard.Snapshot(west)
		Expect(found).To(BeTrue())
		generation := snapshotter.SnapshotAll().Generation

		By("changing only the unrelated registration's extracted metadata")
		obj, err := dynClient.Resource(gvr).Namespace("default").Get(ctx, west, metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		labels := obj.GetLabels()
		labels["review-version"] = "changed"
		obj.SetLabels(labels)
		_, err = dynClient.Resource(gvr).Namespace("default").Update(ctx, obj, metav1.UpdateOptions{})
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() uint64 { proof, _ := guard.Snapshot(west); return proof.Revision }, 5*time.Second).ShouldNot(Equal(westProof.Revision))
		Expect(snapshotter.SnapshotAll().Generation).To(BeNumerically(">", generation))
		Expect(guard.WithCurrent(ctx, east, eastProof.Info.RegistrationUID, eastProof.Revision, func(registry.ClusterInfo) error { return nil })).To(Succeed())

		By("changing the proof's own registration metadata")
		obj, err = dynClient.Resource(gvr).Namespace("default").Get(ctx, east, metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		labels = obj.GetLabels()
		labels["review-version"] = "changed"
		obj.SetLabels(labels)
		_, err = dynClient.Resource(gvr).Namespace("default").Update(ctx, obj, metav1.UpdateOptions{})
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() uint64 { proof, _ := guard.Snapshot(east); return proof.Revision }, 5*time.Second).ShouldNot(Equal(eastProof.Revision))
		Expect(guard.WithCurrent(ctx, east, eastProof.Info.RegistrationUID, eastProof.Revision, func(registry.ClusterInfo) error { return nil })).NotTo(Succeed())
	}, Entry("EAIGW", false), Entry("Kuadrant", true))

	It("refreshes EAIGW registry membership after Kubernetes API changes", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		name := fmt.Sprintf("it-reg-gw-005-eaigw-%d", GinkgoParallelProcess())

		reg := registry.NewEAIGWRegistry(dynClient, registry.EAIGWRegistryConfig{}, nil, logr.Discard())
		defer reg.Stop()
		Expect(reg.Start(ctx)).To(Succeed())
		_, found := reg.Get(name)
		Expect(found).To(BeFalse())

		By("creating a managed Backend after the initial informer sync")
		createBackend(ctx, name)
		Expect(reg.Probe(ctx)).To(Succeed())
		_, found = reg.Get(name)
		Expect(found).To(BeTrue(), "SI-4: an authoritative refresh must discover new managed clusters")

		By("deleting the Backend and refreshing again")
		deleteBackend(ctx, name)
		Expect(reg.Probe(ctx)).To(Succeed())
		_, found = reg.Get(name)
		Expect(found).To(BeFalse(), "SI-4: an authoritative refresh must remove deleted managed clusters")
	})

	It("refreshes Kuadrant registry membership after Kubernetes API changes", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		name := fmt.Sprintf("it-reg-gw-005-kuadrant-%d", GinkgoParallelProcess())

		reg := registry.NewKuadrantRegistry(dynClient, registry.EAIGWRegistryConfig{}, nil, logr.Discard())
		defer reg.Stop()
		Expect(reg.Start(ctx)).To(Succeed())
		_, found := reg.Get(name)
		Expect(found).To(BeFalse())

		By("creating a managed MCPServerRegistration after the initial informer sync")
		createMCPServerRegistration(ctx, name)
		Expect(reg.Probe(ctx)).To(Succeed())
		_, found = reg.Get(name)
		Expect(found).To(BeTrue(), "SI-4: an authoritative refresh must discover new managed clusters")

		By("deleting the MCPServerRegistration and refreshing again")
		deleteMCPServerRegistration(ctx, name)
		Expect(reg.Probe(ctx)).To(Succeed())
		_, found = reg.Get(name)
		Expect(found).To(BeFalse(), "SI-4: an authoritative refresh must remove deleted managed clusters")
	})
})
