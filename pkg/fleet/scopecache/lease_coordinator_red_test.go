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

package scopecache_test

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	coordinationclient "k8s.io/client-go/kubernetes/typed/coordination/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
)

func expiredLeaseCoordinator() (*scopecache.KubernetesLeaseCoordinator, scopecache.LeaseSession) {
	scheme := runtime.NewScheme()
	Expect(coordinationv1.AddToScheme(scheme)).To(Succeed())
	clientset := &testCoordinationBridge{client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	coordinator, err := scopecache.NewKubernetesLeaseCoordinator(
		clientset, "kubernaut-system", "fmc-writer", types.UID("pod-a"), time.Second, logr.Discard())
	Expect(err).ToNot(HaveOccurred())
	session, err := coordinator.Acquire(context.Background())
	Expect(err).ToNot(HaveOccurred())
	lease, err := clientset.Leases("kubernaut-system").Get(context.Background(), "fmc-writer", metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred())
	expiredAt := metav1.NewMicroTime(time.Now().UTC().Add(-2 * time.Second))
	lease.Spec.RenewTime = &expiredAt
	_, err = clientset.Leases("kubernaut-system").Update(context.Background(), lease, metav1.UpdateOptions{})
	Expect(err).ToNot(HaveOccurred())
	return coordinator, session
}

// Test-only external-client bridge: use the ADR-004 controller-runtime fake,
// without changing the production typed Kubernetes client or singleton wiring.
// Embedded interfaces intentionally leave unused external API methods unsupported.
type testCoordinationBridge struct {
	coordinationclient.CoordinationV1Interface
	client client.Client
}

func (b *testCoordinationBridge) Leases(namespace string) coordinationclient.LeaseInterface {
	return &testLeaseBridge{client: b.client, namespace: namespace}
}

type testLeaseBridge struct {
	coordinationclient.LeaseInterface
	client    client.Client
	namespace string
}

func (b *testLeaseBridge) Get(ctx context.Context, name string, _ metav1.GetOptions) (*coordinationv1.Lease, error) {
	lease := &coordinationv1.Lease{}
	err := b.client.Get(ctx, client.ObjectKey{Namespace: b.namespace, Name: name}, lease)
	return lease, err
}

func (b *testLeaseBridge) Create(ctx context.Context, lease *coordinationv1.Lease, _ metav1.CreateOptions) (*coordinationv1.Lease, error) {
	cloned := lease.DeepCopy()
	cloned.Namespace = b.namespace
	err := b.client.Create(ctx, cloned)
	return cloned, err
}

func (b *testLeaseBridge) Update(ctx context.Context, lease *coordinationv1.Lease, _ metav1.UpdateOptions) (*coordinationv1.Lease, error) {
	cloned := lease.DeepCopy()
	err := b.client.Update(ctx, cloned)
	return cloned, err
}

var _ = Describe("FMC lease expiry (BR-SCOPE-001)", func() {
	It("UT-FMC-LEASE-001 [BR-SCOPE-001]: Verify rejects a lease after its deadline", func() {
		coordinator, session := expiredLeaseCoordinator()
		Expect(coordinator.Verify(context.Background(), session.Fence)).To(MatchError(ContainSubstring("stale writer fence")))
	})

	It("UT-FMC-LEASE-002 [BR-SCOPE-001]: Renew rejects a lease after its deadline", func() {
		coordinator, session := expiredLeaseCoordinator()
		Expect(coordinator.Renew(context.Background(), session.Fence)).To(MatchError(ContainSubstring("stale writer fence")))
	})
})
