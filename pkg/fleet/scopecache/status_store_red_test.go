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
	"fmt"
	"time"

	"github.com/alicebob/miniredis/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

var _ = Describe("Valkey repair lifecycle (BR-SCOPE-001, BR-SECURITY-016)", func() {
	It("IT-FMC-CACHE-004: preserves repair token, full fence, generation, and TTL proof through clean read", func() {
		server, err := miniredis.Run()
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(server.Close)
		// Keep the strict server-time/PTTL proof deterministic under miniredis's
		// Go Lua interpreter, which otherwise advances the wall clock mid-script.
		server.SetTime(time.Unix(1_700_000_000, 0))

		store, err := scopecache.NewValkeyStatusStoreAt(server.Addr())
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(store.Close)
		ctx := context.Background()
		Expect(store.LoadScript(ctx)).To(Succeed())

		fence := scopecache.WriterFence{
			HolderPodUID:     "pod-uid-1",
			HolderSessionUID: "session-uid-1",
			LeaseEpoch:       9,
			Fence:            9,
		}
		partition := scopecache.PartitionStateKey{ClusterID: "prod-east", RegistrationUID: "registration-uid-1"}
		start, err := store.BeginRepair(ctx, scopecache.BeginRepairRequest{
			Partition:                partition,
			RegistrationUID:          partition.RegistrationUID,
			ExpectedRegistryRevision: 4,
			WriterFence:              fence,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(start.Token.Validate()).To(Succeed())

		identity := scopecache.ResourceIdentity{
			ClusterID: "prod-east",
			Group:     "apps",
			Version:   "v1",
			Kind:      "Deployment",
			Namespace: "payments",
			Name:      "checkout",
		}
		observations := scopecache.ValidatedObservationSet{
			Items: []scopecache.RepairObservation{{
				Identity:      identity,
				APIVersion:    "apps/v1",
				LabelsPresent: true,
				LabelState:    scope.LabelStateTrue,
				TTL:           30 * time.Second,
			}},
			Complete:  true,
			PageCount: 1,
			ItemCount: 1,
		}
		repair, err := store.Repair(ctx, scopecache.RepairRequest{
			Partition:                partition,
			RegistrationUID:          partition.RegistrationUID,
			ExpectedRegistryRevision: 4,
			WriterFence:              fence,
			Generation:               start.Generation,
			Token:                    start.Token,
			Observations:             observations,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(repair.CleanCommitEligible).To(BeTrue())
		Expect(repair.Token).To(Equal(start.Token))
		Expect(repair.ProvenWriterFence).To(Equal(fence))

		commit, err := store.CommitRepair(ctx, scopecache.RepairCommitRequest{
			Partition:                partition,
			RegistrationUID:          partition.RegistrationUID,
			ExpectedRegistryRevision: 4,
			WriterFence:              fence,
			Generation:               start.Generation,
			Token:                    start.Token,
			ExpectedCount:            1,
			Proof:                    repair,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(commit.Committed).To(BeTrue())
		Expect(commit.CleanCommit).To(BeTrue())
		Expect(commit.Token).To(Equal(start.Token))

		read, err := store.ReadVerified(ctx, scopecache.FencedReadRequest{
			Partition:                partition,
			RegistrationUID:          partition.RegistrationUID,
			ExpectedRegistryRevision: 4,
			WriterFence:              fence,
			Generation:               start.Generation,
			Identity:                 identity,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(read.State).To(Equal(scopecache.ReadVerified))
		Expect(read.Record.LabelState).To(Equal(scope.LabelStateTrue))
		Expect(read.RecordValid).To(BeTrue())
		Expect(read.RecordTTL.Valid).To(BeTrue())
		Expect(read.IndexTTL.Valid).To(BeTrue())
		Expect(read.StatusTTL.Valid).To(BeTrue())

		server.FastForward(31 * time.Second)
		read, err = store.ReadVerified(ctx, scopecache.FencedReadRequest{
			Partition:                partition,
			RegistrationUID:          partition.RegistrationUID,
			ExpectedRegistryRevision: 4,
			WriterFence:              fence,
			Generation:               start.Generation,
			Identity:                 identity,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(read.State).To(Equal(scopecache.ReadMissing))
	})

	It("UT-FMC-CACHE-014: rejects a reused repair token at the commit boundary", func() {
		server, err := miniredis.Run()
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(server.Close)
		store, err := scopecache.NewValkeyStatusStoreAt(server.Addr())
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(store.Close)
		ctx := context.Background()
		Expect(store.LoadScript(ctx)).To(Succeed())
		fence := scopecache.WriterFence{HolderPodUID: "pod-uid-1", HolderSessionUID: "session-uid-1", LeaseEpoch: 2, Fence: 2}
		partition := scopecache.PartitionStateKey{ClusterID: "prod-east", RegistrationUID: "registration-uid-1"}
		start, err := store.BeginRepair(ctx, scopecache.BeginRepairRequest{Partition: partition, RegistrationUID: partition.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: fence})
		Expect(err).ToNot(HaveOccurred())
		identity := scopecache.ResourceIdentity{
			ClusterID: "prod-east",
			Group:     "apps",
			Version:   "v1",
			Kind:      "Deployment",
			Namespace: "payments",
			Name:      "checkout",
		}
		_, err = store.Repair(ctx, scopecache.RepairRequest{
			Partition:                partition,
			RegistrationUID:          partition.RegistrationUID,
			ExpectedRegistryRevision: 1,
			WriterFence:              fence,
			Generation:               start.Generation,
			Token:                    scopecache.RepairToken{Value: "different-token"},
			Observations: scopecache.ValidatedObservationSet{
				Items: []scopecache.RepairObservation{{
					Identity:      identity,
					APIVersion:    "apps/v1",
					LabelsPresent: true,
					LabelState:    scope.LabelStateTrue,
					TTL:           30 * time.Second,
				}},
				Complete:  true,
				PageCount: 1,
				ItemCount: 1,
			},
		})
		Expect(err).To(HaveOccurred(), "a mismatched token must not repair")

		start, err = store.BeginRepair(ctx, scopecache.BeginRepairRequest{Partition: partition, RegistrationUID: partition.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: fence})
		Expect(err).ToNot(HaveOccurred())
		repair, err := store.Repair(ctx, scopecache.RepairRequest{
			Partition:                partition,
			RegistrationUID:          partition.RegistrationUID,
			ExpectedRegistryRevision: 1,
			WriterFence:              fence,
			Generation:               start.Generation,
			Token:                    start.Token,
			Observations: scopecache.ValidatedObservationSet{
				Items: []scopecache.RepairObservation{{
					Identity:      identity,
					APIVersion:    "apps/v1",
					LabelsPresent: true,
					LabelState:    scope.LabelStateTrue,
					TTL:           30 * time.Second,
				}},
				Complete:  true,
				PageCount: 1,
				ItemCount: 1,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		commitRequest := scopecache.RepairCommitRequest{
			Partition:                partition,
			RegistrationUID:          partition.RegistrationUID,
			ExpectedRegistryRevision: 1,
			WriterFence:              fence,
			Generation:               start.Generation,
			Token:                    start.Token,
			ExpectedCount:            1,
			Proof:                    repair,
		}
		commit, err := store.CommitRepair(ctx, commitRequest)
		Expect(err).ToNot(HaveOccurred())
		Expect(commit.Committed).To(BeTrue())

		_, err = store.CommitRepair(ctx, commitRequest)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("commit"))
	})

	It("IT-FMC-CACHE-015 [BR-SCOPE-001]: cleanup examines every stale member before deleting indexed entries", func() {
		server, err := miniredis.Run()
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(server.Close)
		server.SetTime(time.Unix(1_700_000_000, 0))
		store, err := scopecache.NewValkeyStatusStoreAt(server.Addr())
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(store.Close)
		ctx := context.Background()
		Expect(store.LoadScript(ctx)).To(Succeed())

		fence := scopecache.WriterFence{HolderPodUID: "pod-uid-1", HolderSessionUID: "session-uid-1", LeaseEpoch: 3, Fence: 3}
		partition := scopecache.PartitionStateKey{ClusterID: "prod-east", RegistrationUID: "registration-uid-1"}
		start, err := store.BeginRepair(ctx, scopecache.BeginRepairRequest{Partition: partition, RegistrationUID: partition.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: fence})
		Expect(err).ToNot(HaveOccurred())

		stale := make([]scopecache.RepairObservation, 0, 300)
		for i := 0; i < 300; i++ {
			stale = append(stale, scopecache.RepairObservation{
				Identity: scopecache.ResourceIdentity{
					ClusterID: "prod-east",
					Group:     "apps",
					Version:   "v1",
					Kind:      "Deployment",
					Namespace: "payments",
					Name:      fmt.Sprintf("stale-%03d", i),
				},
				APIVersion:    "apps/v1",
				LabelsPresent: true,
				LabelState:    scope.LabelStateTrue,
				TTL:           30 * time.Second,
			})
		}
		firstRepair, err := store.Repair(ctx, scopecache.RepairRequest{
			Partition:                partition,
			RegistrationUID:          partition.RegistrationUID,
			ExpectedRegistryRevision: 1,
			WriterFence:              fence,
			Generation:               start.Generation,
			Token:                    start.Token,
			Observations:             scopecache.ValidatedObservationSet{Items: stale, Complete: true, PageCount: 1, ItemCount: len(stale)},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(firstRepair.CleanCommitEligible).To(BeTrue())
		_, err = store.CommitRepair(ctx, scopecache.RepairCommitRequest{
			Partition:                partition,
			RegistrationUID:          partition.RegistrationUID,
			ExpectedRegistryRevision: 1,
			WriterFence:              fence,
			Generation:               start.Generation,
			Token:                    start.Token,
			ExpectedCount:            len(stale),
			Proof:                    firstRepair,
		})
		Expect(err).ToNot(HaveOccurred())

		start, err = store.BeginRepair(ctx, scopecache.BeginRepairRequest{Partition: partition, RegistrationUID: partition.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: fence})
		Expect(err).ToNot(HaveOccurred())
		current := scopecache.RepairObservation{
			Identity: scopecache.ResourceIdentity{
				ClusterID: "prod-east",
				Group:     "apps",
				Version:   "v1",
				Kind:      "Deployment",
				Namespace: "payments",
				Name:      "zz-observed",
			},
			APIVersion:    "apps/v1",
			LabelsPresent: true,
			LabelState:    scope.LabelStateTrue,
			TTL:           30 * time.Second,
		}
		secondRepair, err := store.Repair(ctx, scopecache.RepairRequest{
			Partition:                partition,
			RegistrationUID:          partition.RegistrationUID,
			ExpectedRegistryRevision: 1,
			WriterFence:              fence,
			Generation:               start.Generation,
			Token:                    start.Token,
			Observations:             scopecache.ValidatedObservationSet{Items: []scopecache.RepairObservation{current}, Complete: true, PageCount: 1, ItemCount: 1},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(secondRepair.CleanupCount).To(Equal(300))
	})
})
