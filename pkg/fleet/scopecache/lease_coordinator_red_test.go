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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"

	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
)

func expiredLeaseCoordinator() (*scopecache.KubernetesLeaseCoordinator, scopecache.LeaseSession) {
	clientset := kubernetesfake.NewSimpleClientset()
	coordinator, err := scopecache.NewKubernetesLeaseCoordinator(
		clientset.CoordinationV1(), "kubernaut-system", "fmc-writer", types.UID("pod-a"), time.Second, logr.Discard())
	Expect(err).ToNot(HaveOccurred())
	session, err := coordinator.Acquire(context.Background())
	Expect(err).ToNot(HaveOccurred())
	lease, err := clientset.CoordinationV1().Leases("kubernaut-system").Get(context.Background(), "fmc-writer", metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred())
	expiredAt := metav1.NewMicroTime(time.Now().UTC().Add(-2 * time.Second))
	lease.Spec.RenewTime = &expiredAt
	_, err = clientset.CoordinationV1().Leases("kubernaut-system").Update(context.Background(), lease, metav1.UpdateOptions{})
	Expect(err).ToNot(HaveOccurred())
	return coordinator, session
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
