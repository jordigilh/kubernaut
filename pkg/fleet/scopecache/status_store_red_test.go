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
	"strings"
	"time"

	"github.com/alicebob/miniredis/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"

	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
	"github.com/jordigilh/kubernaut/test/testutil/redisfault"
)

var _ = Describe("Valkey repair lifecycle (BR-SCOPE-001, BR-SECURITY-016)", func() {
	DescribeTable("lost begin replies are never replayed (BR-SCOPE-001)", func(injected, flush bool) {
		server, err := miniredis.Run()
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(server.Close)
		proxy, err := redisfault.New(context.Background(), server.Addr())
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(proxy.Close)
		var store *scopecache.ValkeyStatusStore
		if injected {
			store, err = scopecache.NewValkeyStatusStore(redis.NewClient(&redis.Options{Addr: proxy.Addr()}))
		} else {
			store, err = scopecache.NewValkeyStatusStoreAt(proxy.Addr())
		}
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(store.Close)
		ctx := context.Background()
		Expect(store.LoadScript(ctx)).To(Succeed())
		if flush {
			admin := redis.NewClient(&redis.Options{Addr: server.Addr()})
			DeferCleanup(admin.Close)
			Expect(admin.ScriptFlush(ctx).Err()).To(Succeed())
		}
		proxy.Arm("begin")
		start, err := store.BeginRepair(ctx, scopecache.BeginRepairRequest{
			Partition:       scopecache.PartitionStateKey{ClusterID: "lost", RegistrationUID: "uid"},
			RegistrationUID: "uid", ExpectedRegistryRevision: 1,
			WriterFence: scopecache.WriterFence{HolderPodUID: "pod", HolderSessionUID: "session", LeaseEpoch: 1, Fence: 1},
		})
		expected := 1
		if flush {
			expected++
		} // Only the definitive NOSCRIPT invocation may be replayed.
		Expect(proxy.Calls("begin")).To(Equal(expected), "an executed mutation must not be replayed after EOF")
		Expect(err).To(MatchError(scopecache.ErrAmbiguousWrite))
		Expect(start.Generation).To(BeZero())
	},
		Entry("UT-FMC-REPLAY-001 standalone client", false, false),
		Entry("UT-FMC-REPLAY-002 injected retry-enabled client", true, false),
		Entry("UT-FMC-REPLAY-003 lost reply after explicit NOSCRIPT recovery", true, true),
	)
	It("UT-FMC-REPLAY-004 [BR-SCOPE-001]: definitive NOSCRIPT still permits one explicit recovery", func() {
		server, err := miniredis.Run()
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(server.Close)
		proxy, err := redisfault.New(context.Background(), server.Addr())
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(proxy.Close)
		store, err := scopecache.NewValkeyStatusStoreAt(proxy.Addr())
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(store.Close)
		ctx := context.Background()
		Expect(store.LoadScript(ctx)).To(Succeed())
		admin := redis.NewClient(&redis.Options{Addr: server.Addr()})
		DeferCleanup(admin.Close)
		Expect(admin.ScriptFlush(ctx).Err()).To(Succeed())
		start, err := store.BeginRepair(ctx, scopecache.BeginRepairRequest{
			Partition:       scopecache.PartitionStateKey{ClusterID: "recover", RegistrationUID: "uid"},
			RegistrationUID: "uid", ExpectedRegistryRevision: 1,
			WriterFence: scopecache.WriterFence{HolderPodUID: "pod", HolderSessionUID: "session", LeaseEpoch: 1, Fence: 1},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(start.Generation).To(Equal(uint64(1)))
		Expect(proxy.Calls("begin")).To(Equal(2))
	})
	It("IT-FMC-REVIEW-EMPTY-001 [BR-SCOPE-001]: commits a complete empty partition without fabricating TTL validity", func() {
		server, err := miniredis.Run()
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(server.Close)
		server.SetTime(time.Unix(1_700_000_000, 0))
		store, err := scopecache.NewValkeyStatusStoreAt(server.Addr())
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(store.Close)
		ctx := context.Background()
		Expect(store.LoadScript(ctx)).To(Succeed())
		fence := scopecache.WriterFence{HolderPodUID: "pod", HolderSessionUID: "session", LeaseEpoch: 1, Fence: 1}
		partition := scopecache.PartitionStateKey{ClusterID: "empty", RegistrationUID: "uid"}
		start, err := store.BeginRepair(ctx, scopecache.BeginRepairRequest{Partition: partition, RegistrationUID: "uid", ExpectedRegistryRevision: 1, WriterFence: fence})
		Expect(err).ToNot(HaveOccurred())
		repair, err := store.Repair(ctx, scopecache.RepairRequest{
			Partition: partition, RegistrationUID: "uid", ExpectedRegistryRevision: 1, WriterFence: fence,
			Generation: start.Generation, Token: start.Token,
			Observations: scopecache.ValidatedObservationSet{Complete: true, PageCount: 1},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(repair.CleanCommitEligible).To(BeTrue())
		Expect(repair.RecordTTL.Valid).To(BeFalse())
		Expect(repair.IndexTTL.Valid).To(BeFalse())
		Expect(repair.StatusTTL.Valid).To(BeTrue())
		commit, err := store.CommitRepair(ctx, scopecache.RepairCommitRequest{
			Partition: partition, RegistrationUID: "uid", ExpectedRegistryRevision: 1, WriterFence: fence,
			Generation: start.Generation, Token: start.Token, Proof: repair,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(commit.Committed).To(BeTrue())
		read, err := store.ReadVerified(ctx, scopecache.FencedReadRequest{
			Partition: partition, RegistrationUID: "uid", ExpectedRegistryRevision: 1, WriterFence: fence,
			Generation: start.Generation,
			Identity:   scopecache.ResourceIdentity{ClusterID: "empty", Version: "v1", Kind: "Pod", Namespace: "ns", Name: "absent"},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(read.State).To(Equal(scopecache.ReadMissing))
	})
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
		// Keep the strict server-time/PTTL proof deterministic under miniredis's
		// Go Lua interpreter, which otherwise advances the wall clock mid-script.
		server.SetTime(time.Unix(1_700_000_000, 0))
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

		By("repairing the formerly populated partition to explicitly verified empty")
		start, err = store.BeginRepair(ctx, scopecache.BeginRepairRequest{Partition: partition, RegistrationUID: partition.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: fence})
		Expect(err).ToNot(HaveOccurred())
		empty, err := store.Repair(ctx, scopecache.RepairRequest{
			Partition: partition, RegistrationUID: partition.RegistrationUID, ExpectedRegistryRevision: 1,
			WriterFence: fence, Generation: start.Generation, Token: start.Token,
			Observations: scopecache.ValidatedObservationSet{Complete: true, PageCount: 1},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(empty.CleanupCount).To(Equal(1))
		Expect(empty.EmptyPartitionVerified).To(BeTrue())
		Expect(empty.RecordTTL.Valid).To(BeFalse())
		Expect(empty.IndexTTL.Valid).To(BeFalse())
		commit, err := store.CommitRepair(ctx, scopecache.RepairCommitRequest{
			Partition: partition, RegistrationUID: partition.RegistrationUID, ExpectedRegistryRevision: 1,
			WriterFence: fence, Generation: start.Generation, Token: start.Token, Proof: empty,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(commit.Committed).To(BeTrue())
	})

	DescribeTable("IT-FMC-REVIEW-EMPTY-002 [BR-SCOPE-001, BR-SECURITY-016]: empty repair rejects invalid status and interrupted cleanup", func(fault string) {
		server, err := miniredis.Run()
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(server.Close)
		server.SetTime(time.Unix(1_700_000_000, 0))
		store, err := scopecache.NewValkeyStatusStoreAt(server.Addr())
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(store.Close)
		ctx := context.Background()
		Expect(store.LoadScript(ctx)).To(Succeed())
		partition := scopecache.PartitionStateKey{ClusterID: "fault", RegistrationUID: "uid"}
		fence := scopecache.WriterFence{HolderPodUID: "pod", HolderSessionUID: "session", LeaseEpoch: 1, Fence: 1}
		start, err := store.BeginRepair(ctx, scopecache.BeginRepairRequest{Partition: partition, RegistrationUID: "uid", ExpectedRegistryRevision: 1, WriterFence: fence})
		Expect(err).ToNot(HaveOccurred())
		if fault == "interrupted" {
			_, err := store.Repair(ctx, scopecache.RepairRequest{
				Partition: partition, RegistrationUID: "uid", ExpectedRegistryRevision: 1, WriterFence: fence, Generation: start.Generation, Token: start.Token,
				Observations: scopecache.ValidatedObservationSet{Complete: true, PageCount: 1, ItemCount: 1, Items: []scopecache.RepairObservation{{
					Identity:   scopecache.ResourceIdentity{ClusterID: "fault", Version: "v1", Kind: "Pod", Namespace: "default", Name: "old"},
					APIVersion: "v1", LabelsPresent: true, LabelState: scope.LabelStateTrue, TTL: 30 * time.Second,
				}}},
			})
			Expect(err).ToNot(HaveOccurred())
		}
		for _, key := range server.Keys() {
			switch {
			case strings.HasSuffix(key, ":status") && fault == "wrongtype":
				server.Del(key)
				Expect(server.Set(key, "corrupted")).To(Succeed())
			case strings.HasSuffix(key, ":status") && fault == "invalid-ttl":
				server.SetTTL(key, 0)
			case strings.Contains(key, ":record:") && fault == "interrupted":
				server.Del(key)
				Expect(server.Set(key, "corrupted")).To(Succeed())
			}
		}
		if fault == "expired" {
			server.FastForward(61 * time.Second)
		}
		result, err := store.Repair(ctx, scopecache.RepairRequest{
			Partition: partition, RegistrationUID: "uid", ExpectedRegistryRevision: 1, WriterFence: fence,
			Generation: start.Generation, Token: start.Token, Observations: scopecache.ValidatedObservationSet{Complete: true, PageCount: 1},
		})
		Expect(err).To(HaveOccurred())
		Expect(result.CleanCommitEligible).To(BeFalse())
		Expect(result.EmptyPartitionVerified).To(BeFalse())
	}, Entry("wrong-type status", "wrongtype"), Entry("expired status", "expired"), Entry("status without expiry", "invalid-ttl"), Entry("interrupted cleanup", "interrupted"))

	DescribeTable("IT-FMC-REVIEW-EMPTY-003 [BR-SECURITY-016]: empty commit rechecks server predicates after finalization", func(fault string) {
		server, err := miniredis.Run()
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(server.Close)
		server.SetTime(time.Unix(1_700_000_000, 0))
		store, err := scopecache.NewValkeyStatusStoreAt(server.Addr())
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(store.Close)
		ctx := context.Background()
		Expect(store.LoadScript(ctx)).To(Succeed())
		partition := scopecache.PartitionStateKey{ClusterID: "empty-commit", RegistrationUID: "uid"}
		fence := scopecache.WriterFence{HolderPodUID: "pod", HolderSessionUID: "session", LeaseEpoch: 1, Fence: 1}
		start, err := store.BeginRepair(ctx, scopecache.BeginRepairRequest{Partition: partition, RegistrationUID: "uid", ExpectedRegistryRevision: 1, WriterFence: fence})
		Expect(err).ToNot(HaveOccurred())
		proof, err := store.Repair(ctx, scopecache.RepairRequest{
			Partition: partition, RegistrationUID: "uid", ExpectedRegistryRevision: 1, WriterFence: fence,
			Generation: start.Generation, Token: start.Token, Observations: scopecache.ValidatedObservationSet{Complete: true, PageCount: 1},
		})
		Expect(err).ToNot(HaveOccurred())
		for _, key := range server.Keys() {
			if !strings.HasSuffix(key, ":status") {
				continue
			}
			switch fault {
			case "residual":
				_, err := server.ZAdd(strings.TrimSuffix(key, ":status")+":index", 1, "unexpected")
				Expect(err).ToNot(HaveOccurred())
			case "wrongtype-index":
				Expect(server.Set(strings.TrimSuffix(key, ":status")+":index", "corrupted")).To(Succeed())
			default:
				server.FastForward(61 * time.Second)
			}
		}
		commit, err := store.CommitRepair(ctx, scopecache.RepairCommitRequest{
			Partition: partition, RegistrationUID: "uid", ExpectedRegistryRevision: 1, WriterFence: fence,
			Generation: start.Generation, Token: start.Token, Proof: proof,
		})
		Expect(err).To(HaveOccurred())
		Expect(commit.Committed).To(BeFalse())
	}, Entry("residual index member", "residual"), Entry("wrong-type index", "wrongtype-index"), Entry("expired status", "expired"))
})
