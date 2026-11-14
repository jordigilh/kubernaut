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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
)

var _ = Describe("FMC readiness snapshot (BR-SCOPE-001, BR-SECURITY-016)", func() {
	const (
		clusterID       = "prod-east"
		registrationUID = "registration-uid-1"
		sessionUID      = "session-uid-1"
	)

	newFixture := func() (*scopecache.FMCReadinessSnapshot, scopecache.PartitionStateKey, scopecache.WriterFence, scopecache.LeaseView) {
		fence := scopecache.WriterFence{
			HolderPodUID:     "pod-uid-1",
			HolderSessionUID: sessionUID,
			LeaseEpoch:       7,
			Fence:            7,
		}
		lease := scopecache.LeaseView{
			Policy:         scopecache.LeasePolicyProcessWideRequired,
			Active:         true,
			Valid:          true,
			Epoch:          fence.LeaseEpoch,
			SessionUID:     sessionUID,
			WriterFence:    fence,
			HeartbeatAt:    time.Now().UTC(),
			HeartbeatValid: true,
		}
		partition := scopecache.PartitionStateKey{
			ClusterID:       clusterID,
			RegistrationUID: registrationUID,
		}
		snapshot, err := scopecache.NewFMCReadinessSnapshot(scopecache.LeasePolicyProcessWideRequired)
		Expect(err).ToNot(HaveOccurred())
		Expect(snapshot.PublishGlobal(scopecache.FMCGlobalReadiness{
			MCPInitialSync:       true,
			RegistryInitialSync:  true,
			RegistryWatchHealthy: true,
			ScriptReady:          true,
			ValkeyReady:          true,
			Lease:                lease,
			ActiveLeaseValid:     true,
			LeasePolicy:          scopecache.LeasePolicyProcessWideRequired,
		})).To(Succeed())
		return snapshot, partition, fence, lease
	}

	It("UT-FMC-CACHE-011: rejects a clean read after same-UID registration revision invalidation", func() {
		snapshot, partition, fence, lease := newFixture()
		Expect(snapshot.BeginPartitionRepair(partition, registrationUID, 3, lease, 11)).To(Succeed())
		Expect(snapshot.PublishPartitionClean(partition, scopecache.PartitionReadiness{
			Key:                  partition,
			RegistrationUID:      registrationUID,
			RegistrationRevision: 3,
			LeaseEpoch:           fence.LeaseEpoch,
			LeaseSessionUID:      fence.HolderSessionUID,
			WriterFence:          fence,
			LeaseHeartbeatAt:     lease.HeartbeatAt,
			LeaseHeartbeatValid:  true,
			Generation:           11,
			State:                scopecache.PartitionClean,
			CleanCommit:          true,
			RecordValid:          true,
			IndexValid:           true,
			StatusValid:          true,
			RecordTTLValid:       true,
			IndexTTLValid:        true,
			StatusTTLValid:       true,
		})).To(Succeed())

		proof, err := snapshot.PartitionReady(partition, registrationUID, 3, fence, 11)
		Expect(err).ToNot(HaveOccurred())
		Expect(proof.RecordValid).To(BeTrue())

		Expect(snapshot.InvalidatePartition(partition, "same-UID registration revision changed")).To(Succeed())
		_, err = snapshot.PartitionReady(partition, registrationUID, 3, fence, 11)
		Expect(err).To(MatchError(ContainSubstring("unavailable")))
	})

	It("UT-FMC-CACHE-014: rejects an expected revision that differs from the clean proof", func() {
		snapshot, partition, fence, lease := newFixture()
		Expect(snapshot.BeginPartitionRepair(partition, registrationUID, 4, lease, 12)).To(Succeed())
		Expect(snapshot.PublishPartitionClean(partition, scopecache.PartitionReadiness{
			Key:                  partition,
			RegistrationUID:      registrationUID,
			RegistrationRevision: 4,
			LeaseEpoch:           fence.LeaseEpoch,
			LeaseSessionUID:      fence.HolderSessionUID,
			WriterFence:          fence,
			LeaseHeartbeatAt:     lease.HeartbeatAt,
			LeaseHeartbeatValid:  true,
			Generation:           12,
			State:                scopecache.PartitionClean,
			CleanCommit:          true,
			RecordValid:          true,
			IndexValid:           true,
			StatusValid:          true,
			RecordTTLValid:       true,
			IndexTTLValid:        true,
			StatusTTLValid:       true,
		})).To(Succeed())

		_, err := snapshot.PartitionReady(partition, registrationUID, 5, fence, 12)
		Expect(err).To(MatchError(ContainSubstring("revision")))
	})

	It("UT-FMC-CACHE-016 [BR-SCOPE-001]: treats an empty registry as ready without a process-wide lease", func() {
		snapshot, err := scopecache.NewFMCReadinessSnapshot(scopecache.LeasePolicyProcessWideRequired)
		Expect(err).ToNot(HaveOccurred())
		Expect(snapshot.PublishGlobal(scopecache.FMCGlobalReadiness{
			MCPInitialSync:       true,
			RegistryInitialSync:  true,
			RegistryWatchHealthy: true,
			ScriptReady:          true,
			ValkeyReady:          true,
			Lease: scopecache.LeaseView{
				Policy: scopecache.LeasePolicyProcessWideRequired,
			},
			LeasePolicy: scopecache.LeasePolicyProcessWideRequired,
		})).To(Succeed())

		ready, err := snapshot.ServiceReady()
		Expect(err).ToNot(HaveOccurred())
		Expect(ready).To(BeTrue())
	})
})
