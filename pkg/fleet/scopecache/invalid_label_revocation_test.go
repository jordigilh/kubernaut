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

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// publishCleanGeneration republishes the fenced-read proof for a new committed
// generation on an existing readiness snapshot, mirroring what the FMC writer
// does after commit_swap.
func publishCleanGeneration(readiness *scopecache.FMCReadinessSnapshot, partition scopecache.PartitionStateKey, info registry.ClusterInfo, revision uint64, fence scopecache.WriterFence, generation uint64) {
	lease := scopecache.LeaseView{
		Policy: scopecache.LeasePolicyProcessWideRequired, Active: true, Valid: true,
		Epoch: fence.LeaseEpoch, SessionUID: fence.HolderSessionUID, WriterFence: fence,
		HeartbeatAt: time.Now().UTC(), HeartbeatValid: true,
	}
	ExpectWithOffset(1, readiness.BeginPartitionRepair(partition, info.RegistrationUID, revision, lease, generation)).To(Succeed())
	ExpectWithOffset(1, readiness.PublishPartitionClean(partition, scopecache.PartitionReadiness{
		Key:                  partition,
		RegistrationUID:      info.RegistrationUID,
		RegistrationRevision: revision,
		LeaseEpoch:           fence.LeaseEpoch,
		LeaseSessionUID:      fence.HolderSessionUID,
		WriterFence:          fence,
		LeaseHeartbeatAt:     time.Now().UTC(),
		LeaseHeartbeatValid:  true,
		Generation:           generation,
		State:                scopecache.PartitionClean,
		CleanCommit:          true,
		RecordValid:          true,
		IndexValid:           true,
		StatusValid:          true,
		RecordTTLValid:       true,
		IndexTTLValid:        true,
		StatusTTLValid:       true,
	})).To(Succeed())
}

// #2686: an invalid managed-label observation is a complete, decisive source
// observation. The write side must stage and commit it (DD-FLEET-016), and the
// already-shipped read-side mapping must turn the committed record into a
// fail-closed ScopeInvalid instead of retaining the last committed
// authorization until TTL. Local and fenced lanes share the rule; readiness
// and partition failures keep their own outcomes.
var _ = Describe("Invalid-label revocation on the fenced lane (BR-SCOPE-001, #2686)", func() {
	It("UT-FMC-2686-001 [BR-SCOPE-001, #2686]: observation contracts accept an invalid label state as a writable observation", func() {
		identity := scopecache.ResourceIdentity{
			ClusterID: "prod-east", Group: "apps", Version: "v1", Kind: "Deployment",
			Namespace: "payments", Name: "checkout",
		}
		Expect(scopecache.Observation{Identity: identity, LabelState: scope.LabelStateInvalid, TTL: 30 * time.Second}.Validate()).
			To(Succeed(), "the normal write cycle must be able to carry an invalid observation")
		Expect(scopecache.RepairObservation{
			Identity: identity, APIVersion: "apps/v1", LabelsPresent: true,
			LabelState: scope.LabelStateInvalid, TTL: 30 * time.Second,
		}.Validate()).To(Succeed(), "the staged repair boundary must be able to carry an invalid observation")
	})

	It("UT-FMC-2686-002 [BR-SCOPE-001, #2686]: a staged repair commits an invalid observation over a committed true record and fenced reads serve it", func() {
		f := newStagedRepairFixture()
		DeferCleanup(f.server.Close)
		DeferCleanup(f.store.Close)

		// Generation 1 commits a managed record.
		identity, managed := f.identity("checkout", "true")
		start1, err := f.store.BeginRepair(f.ctx, scopecache.BeginRepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: f.fence,
		})
		Expect(err).ToNot(HaveOccurred())
		f.commitGeneration(start1, []scopecache.RepairObservation{managed})

		// Generation 2 observes the same identity with an invalid label value.
		_, invalid := f.identity("checkout", "invalid")
		start2, err := f.store.BeginRepair(f.ctx, scopecache.BeginRepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: f.fence,
		})
		Expect(err).ToNot(HaveOccurred())
		f.commitGeneration(start2, []scopecache.RepairObservation{invalid})

		record, readErr := f.read(start2, identity)
		Expect(readErr).ToNot(HaveOccurred())
		Expect(record.State).To(Equal(scopecache.ReadVerified))
		Expect(record.Record.LabelState).To(Equal(scope.LabelStateInvalid),
			"the committed record must carry the invalid observation, not the last good state")
	})

	It("UT-FMC-2686-003 [BR-SCOPE-001, #2686]: a committed invalid record revokes a previously verified-managed target (AC-6, SI-10)", func() {
		f := newStagedRepairFixture()
		DeferCleanup(f.server.Close)
		DeferCleanup(f.store.Close)

		info := registry.ClusterInfo{ID: f.part.ClusterID, RegistrationUID: f.part.RegistrationUID}
		identity, managed := f.identity("checkout", "true")
		target := scope.ResourceIdentity{
			ClusterID: identity.ClusterID, Group: identity.Group, Version: identity.Version,
			Kind: identity.Kind, Namespace: identity.Namespace, Name: identity.Name,
		}

		start1, err := f.store.BeginRepair(f.ctx, scopecache.BeginRepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: f.fence,
		})
		Expect(err).ToNot(HaveOccurred())
		f.commitGeneration(start1, []scopecache.RepairObservation{managed})

		readiness := cleanReadiness(f.part, info, 1, f.fence, start1.Generation)
		client, err := scopecache.NewClientWithStatusStore(f.store, &staticRegistrationGuard{
			snapshot: registry.RegistrationSnapshot{Info: info, Revision: 1}, found: true,
		}, readiness)
		Expect(err).ToNot(HaveOccurred())

		authorized, err := client.Check(f.ctx, target)
		Expect(err).ToNot(HaveOccurred())
		Expect(authorized.State).To(Equal(scope.ScopeVerifiedManaged), "the committed true record still authorizes before the label breaks")

		_, invalid := f.identity("checkout", "invalid")
		start2, err := f.store.BeginRepair(f.ctx, scopecache.BeginRepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: f.fence,
		})
		Expect(err).ToNot(HaveOccurred())
		f.commitGeneration(start2, []scopecache.RepairObservation{invalid})
		publishCleanGeneration(readiness, f.part, info, 1, f.fence, start2.Generation)

		revoked, err := client.Check(f.ctx, target)
		Expect(err).ToNot(HaveOccurred())
		Expect(revoked.State).To(Equal(scope.ScopeInvalid), "an invalid managed label must revoke the authorization, not retain the last committed state")
		Expect(revoked.Provenance).To(Equal(scope.ScopeSourceUnavailable))
		Expect(revoked.Reason).To(Equal("invalid_resource_label"))
	})

	It("UT-FMC-2686-004 [BR-SCOPE-001, #2686]: a partition readiness failure is never flattened into the invalid outcome", func() {
		f := newStagedRepairFixture()
		DeferCleanup(f.server.Close)
		DeferCleanup(f.store.Close)

		info := registry.ClusterInfo{ID: f.part.ClusterID, RegistrationUID: f.part.RegistrationUID}
		identity, invalid := f.identity("checkout", "invalid")
		target := scope.ResourceIdentity{
			ClusterID: identity.ClusterID, Group: identity.Group, Version: identity.Version,
			Kind: identity.Kind, Namespace: identity.Namespace, Name: identity.Name,
		}
		start, err := f.store.BeginRepair(f.ctx, scopecache.BeginRepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: f.fence,
		})
		Expect(err).ToNot(HaveOccurred())
		f.commitGeneration(start, []scopecache.RepairObservation{invalid})

		// A readiness snapshot with no published partition: the committed
		// invalid record exists, but the reader must report the partition
		// outcome, not synthesize an authorization decision from it.
		readiness, err := scopecache.NewFMCReadinessSnapshot(scopecache.LeasePolicyProcessWideRequired)
		Expect(err).ToNot(HaveOccurred())
		Expect(readiness.PublishGlobal(scopecache.FMCGlobalReadiness{
			MCPInitialSync: true, RegistryInitialSync: true, RegistryWatchHealthy: true,
			ScriptReady: true, ValkeyReady: true, LeasePolicy: scopecache.LeasePolicyProcessWideRequired,
		})).To(Succeed())
		client, err := scopecache.NewClientWithStatusStore(f.store, &staticRegistrationGuard{
			snapshot: registry.RegistrationSnapshot{Info: info, Revision: 1}, found: true,
		}, readiness)
		Expect(err).ToNot(HaveOccurred())

		result, err := client.Check(f.ctx, target)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.State).To(Equal(scope.ScopeUnavailable))
		Expect(result.Reason).To(Equal("partition_unavailable"),
			"readiness and partition failures keep their own fail-closed outcome, never flattened into invalid")
	})
})
