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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
)

var _ = Describe("FMC process-local writer coordinator", func() {
	It("UT-FMC-SINGLETON-001 [BR-SCOPE-001]: provides a valid singleton writer fence without Kubernetes identity", func() {
		coordinator := scopecache.NewProcessWriterCoordinator()

		first, err := coordinator.Acquire(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Fence.Validate()).To(Succeed())
		Expect(first.Loss).NotTo(BeNil())
		Expect(coordinator.Verify(context.Background(), first.Fence)).To(Succeed())
		Expect(coordinator.Renew(context.Background(), first.Fence)).To(Succeed())

		_, err = coordinator.Acquire(context.Background())
		Expect(err).To(MatchError(scopecache.ErrLeaseHeld))

		Expect(coordinator.Release(context.Background(), first.Fence)).To(Succeed())

		second, err := coordinator.Acquire(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(second.Fence.Validate()).To(Succeed())
		Expect(second.Fence.LeaseEpoch).To(BeNumerically(">", first.Fence.LeaseEpoch))
		Expect(second.Fence.HolderSessionUID).NotTo(Equal(first.Fence.HolderSessionUID))
		Expect(coordinator.Verify(context.Background(), first.Fence)).To(MatchError(scopecache.ErrStaleFence))
	})
})
