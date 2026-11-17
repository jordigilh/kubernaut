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

	"github.com/alicebob/miniredis/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// DD-FLEET-016 A2: staged repair with atomic commit swap. Repairs must never
// make the committed clean proof unavailable to fenced reads.
type stagedRepairFixture struct {
	ctx    context.Context
	server *miniredis.Miniredis
	store  *scopecache.ValkeyStatusStore
	fence  scopecache.WriterFence
	part   scopecache.PartitionStateKey
}

func newStagedRepairFixture() *stagedRepairFixture {
	server, err := miniredis.Run()
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	server.SetTime(time.Unix(1_700_000_000, 0))
	store, err := scopecache.NewValkeyStatusStoreAt(server.Addr())
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	ExpectWithOffset(1, store.LoadScript(context.Background())).To(Succeed())
	return &stagedRepairFixture{
		ctx:    context.Background(),
		server: server,
		store:  store,
		fence:  scopecache.WriterFence{HolderPodUID: "pod-1", HolderSessionUID: "session-1", LeaseEpoch: 5, Fence: 5},
		part:   scopecache.PartitionStateKey{ClusterID: "prod-east", RegistrationUID: "registration-1"},
	}
}

func (f *stagedRepairFixture) identity(name, label string) (scopecache.ResourceIdentity, scopecache.RepairObservation) {
	identity := scopecache.ResourceIdentity{
		ClusterID: f.part.ClusterID, Group: "apps", Version: "v1", Kind: "Deployment",
		Namespace: "payments", Name: name,
	}
	return identity, scopecache.RepairObservation{
		Identity: identity, APIVersion: "apps/v1", LabelsPresent: true,
		LabelState: scope.LabelState(label), TTL: 30 * time.Second,
	}
}

func (f *stagedRepairFixture) commitGeneration(start scopecache.RepairStartResult, observations []scopecache.RepairObservation) scopecache.RepairCommitResult {
	repair, err := f.store.Repair(f.ctx, scopecache.RepairRequest{
		Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1,
		WriterFence: f.fence, Generation: start.Generation, Token: start.Token,
		Observations: scopecache.ValidatedObservationSet{Items: observations, Complete: true, PageCount: 1, ItemCount: len(observations)},
	})
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	commit, err := f.store.CommitRepair(f.ctx, scopecache.RepairCommitRequest{
		Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1,
		WriterFence: f.fence, Generation: start.Generation, Token: start.Token,
		ExpectedCount: len(observations), Proof: repair,
	})
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	return commit
}

func (f *stagedRepairFixture) read(start scopecache.RepairStartResult, identity scopecache.ResourceIdentity) (scopecache.FencedReadResult, error) {
	return f.store.ReadVerified(f.ctx, scopecache.FencedReadRequest{
		Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1,
		WriterFence: f.fence, Generation: start.Generation, Identity: identity,
	})
}

var _ = Describe("DD-FLEET-016 A2: staged repair availability (BR-SCOPE-001)", func() {
	It("UT-FMC-STAGED-001 [BR-SCOPE-001]: fenced reads serve the committed proof while a staged repair is in flight", func() {
		f := newStagedRepairFixture()
		DeferCleanup(f.server.Close)
		DeferCleanup(f.store.Close)

		// Commit generation 1 with one managed record.
		oldIdentity, oldObservation := f.identity("checkout", "true")
		start1, err := f.store.BeginRepair(f.ctx, scopecache.BeginRepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: f.fence,
		})
		Expect(err).ToNot(HaveOccurred())
		f.commitGeneration(start1, []scopecache.RepairObservation{oldObservation})

		// Begin a staged repair that will change the record's label state.
		_, newObservation := f.identity("checkout", "false")
		start2, err := f.store.BeginRepair(f.ctx, scopecache.BeginRepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: f.fence,
		})
		Expect(err).ToNot(HaveOccurred())
		_, err = f.store.Repair(f.ctx, scopecache.RepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1,
			WriterFence: f.fence, Generation: start2.Generation, Token: start2.Token,
			Observations: scopecache.ValidatedObservationSet{Items: []scopecache.RepairObservation{newObservation}, Complete: true, PageCount: 1, ItemCount: 1},
		})
		Expect(err).ToNot(HaveOccurred())

		// THE A2 CONTRACT: while the repair is staged but not committed, the
		// committed generation-1 proof must remain readable.
		read, readErr := f.read(start1, oldIdentity)
		Expect(readErr).ToNot(HaveOccurred())
		Expect(read.State).To(Equal(scopecache.ReadVerified), "a staged repair must not make the committed proof unavailable")
		Expect(read.Record.LabelState).To(Equal(scope.LabelStateTrue))
	})

	It("UT-FMC-STAGED-002 [BR-SCOPE-001]: commit swap atomically installs the new proof", func() {
		f := newStagedRepairFixture()
		DeferCleanup(f.server.Close)
		DeferCleanup(f.store.Close)

		oldIdentity, oldObservation := f.identity("checkout", "true")
		start1, err := f.store.BeginRepair(f.ctx, scopecache.BeginRepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: f.fence,
		})
		Expect(err).ToNot(HaveOccurred())
		f.commitGeneration(start1, []scopecache.RepairObservation{oldObservation})

		newIdentity, newObservation := f.identity("checkout", "false")
		_ = newIdentity
		start2, err := f.store.BeginRepair(f.ctx, scopecache.BeginRepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: f.fence,
		})
		Expect(err).ToNot(HaveOccurred())
		f.commitGeneration(start2, []scopecache.RepairObservation{newObservation})

		read, readErr := f.read(start2, newIdentity)
		Expect(readErr).ToNot(HaveOccurred())
		Expect(read.State).To(Equal(scopecache.ReadVerified))
		Expect(read.Record.LabelState).To(Equal(scope.LabelStateFalse))

		// The previous generation's proof must no longer be served.
		oldRead, _ := f.read(start1, oldIdentity)
		Expect(oldRead.State).To(Equal(scopecache.ReadMissing))
	})

	It("UT-FMC-STAGED-003 [BR-SCOPE-001, BR-SECURITY-016]: without a committed proof, a staged repair still refuses reads", func() {
		f := newStagedRepairFixture()
		DeferCleanup(f.server.Close)
		DeferCleanup(f.store.Close)

		identity, observation := f.identity("checkout", "true")
		start, err := f.store.BeginRepair(f.ctx, scopecache.BeginRepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: f.fence,
		})
		Expect(err).ToNot(HaveOccurred())
		_, err = f.store.Repair(f.ctx, scopecache.RepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1,
			WriterFence: f.fence, Generation: start.Generation, Token: start.Token,
			Observations: scopecache.ValidatedObservationSet{Items: []scopecache.RepairObservation{observation}, Complete: true, PageCount: 1, ItemCount: 1},
		})
		Expect(err).ToNot(HaveOccurred())

		// IT-AF-FMC-2593-001's narrowed contract: staged-but-uncommitted state
		// with NO prior committed proof must remain unavailable.
		read, readErr := f.read(start, identity)
		Expect(readErr).ToNot(HaveOccurred())
		Expect(read.State).ToNot(Equal(scopecache.ReadVerified))
	})

	It("UT-FMC-STAGED-004 [BR-SCOPE-001]: an abandoned staged repair self-cleans and the committed proof survives", func() {
		f := newStagedRepairFixture()
		DeferCleanup(f.server.Close)
		DeferCleanup(f.store.Close)

		oldIdentity, oldObservation := f.identity("checkout", "true")
		start1, err := f.store.BeginRepair(f.ctx, scopecache.BeginRepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: f.fence,
		})
		Expect(err).ToNot(HaveOccurred())
		f.commitGeneration(start1, []scopecache.RepairObservation{oldObservation})

		// Begin a second repair and abandon it WITHOUT committing. The
		// committed proof must remain readable within its data TTL.
		start2, err := f.store.BeginRepair(f.ctx, scopecache.BeginRepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: f.fence,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(start2.Generation).To(BeNumerically(">", start1.Generation))

		// miniredis: FastForward advances expirations but the Lua TIME
		// bridge needs SetTime re-pinned to the same instant, or the strict
		// TTL proof sees a clock skew and correctly rejects it.
		advanced := time.Duration(0)
		advance := func(d time.Duration) {
			advanced += d
			f.server.FastForward(d)
			f.server.SetTime(time.Unix(1_700_000_000, 0).Add(advanced))
		}
		advance(10 * time.Second)
		read, readErr := f.read(start1, oldIdentity)
		Expect(readErr).ToNot(HaveOccurred())
		Expect(read.State).To(Equal(scopecache.ReadVerified), "the committed proof must survive an abandoned staged repair within its TTL")
		Expect(read.Record.LabelState).To(Equal(scope.LabelStateTrue))

		// The syncer's failure path marks the staging uncertain, so the next
		// repair cycle can begin immediately without waiting for the TTL.
		Expect(f.store.MarkRepairUncertain(f.ctx, start2, scopecache.RepairFailure{Code: scopecache.RepairFailureSourceIncomplete, Message: "abandoned"})).To(Succeed())
		_, err = f.store.BeginRepair(f.ctx, scopecache.BeginRepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: f.fence,
		})
		Expect(err).ToNot(HaveOccurred(), "an explicitly abandoned staging must not block the next repair")

		// A lost-reply stranding (no explicit mark) self-clears via the staged
		// TTL, well inside the 30s data TTL. begin3's replacement staging
		// (created at t10 with a 15s TTL) must expire: advance to t26.
		advance(16 * time.Second)
		read, readErr = f.read(start1, oldIdentity)
		Expect(readErr).ToNot(HaveOccurred())
		Expect(read.State).To(Equal(scopecache.ReadVerified), "reads must stay available across a lost-reply stranding window")
		_, err = f.store.BeginRepair(f.ctx, scopecache.BeginRepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: f.fence,
		})
		Expect(err).ToNot(HaveOccurred(), "an expired staged repair must not block future repairs")
	})

	It("UT-FMC-STAGED-005 [BR-SCOPE-001]: staged empty partitions finalize and swap without fabricating record TTLs", func() {
		f := newStagedRepairFixture()
		DeferCleanup(f.server.Close)
		DeferCleanup(f.store.Close)

		// First generation commits one record.
		oldIdentity, oldObservation := f.identity("stale", "true")
		start1, err := f.store.BeginRepair(f.ctx, scopecache.BeginRepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: f.fence,
		})
		Expect(err).ToNot(HaveOccurred())
		f.commitGeneration(start1, []scopecache.RepairObservation{oldObservation})

		// Second generation observes nothing: staged empty finalization + swap.
		start2, err := f.store.BeginRepair(f.ctx, scopecache.BeginRepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: f.fence,
		})
		Expect(err).ToNot(HaveOccurred())
		f.commitGeneration(start2, nil)

		read, readErr := f.read(start2, oldIdentity)
		Expect(readErr).ToNot(HaveOccurred())
		Expect(read.State).To(Equal(scopecache.ReadMissing), "a swapped empty partition must not serve removed records")
	})

	It("UT-FMC-STAGED-007 [BR-SCOPE-001]: the commit refreshes record TTLs from commit time, not reconcile time", func() {
		f := newStagedRepairFixture()
		DeferCleanup(f.server.Close)
		DeferCleanup(f.store.Close)

		identity, observation := f.identity("checkout", "true")
		start, err := f.store.BeginRepair(f.ctx, scopecache.BeginRepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: f.fence,
		})
		Expect(err).ToNot(HaveOccurred())
		repair, err := f.store.Repair(f.ctx, scopecache.RepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1,
			WriterFence: f.fence, Generation: start.Generation, Token: start.Token,
			Observations: scopecache.ValidatedObservationSet{Items: []scopecache.RepairObservation{observation}, Complete: true, PageCount: 1, ItemCount: 1},
		})
		Expect(err).ToNot(HaveOccurred())

		// A slow repair: the staged observation was reconciled at t0 with a
		// 30s TTL, but the commit only lands at t5 (e.g. load-slowed cycle).
		advanced := time.Duration(0)
		advance := func(d time.Duration) {
			advanced += d
			f.server.FastForward(d)
			f.server.SetTime(time.Unix(1_700_000_000, 0).Add(advanced))
		}
		advance(5 * time.Second)
		_, err = f.store.CommitRepair(f.ctx, scopecache.RepairCommitRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1,
			WriterFence: f.fence, Generation: start.Generation, Token: start.Token,
			ExpectedCount: 1, Proof: repair,
		})
		Expect(err).ToNot(HaveOccurred())

		// At t32 the reconcile-time TTL (t0+30) would have lapsed; the
		// commit-time refresh (t5+30) must keep the proof readable.
		advance(27 * time.Second)
		read, readErr := f.read(start, identity)
		Expect(readErr).ToNot(HaveOccurred())
		Expect(read.State).To(Equal(scopecache.ReadVerified), "committed records must carry a full TTL from commit time")
	})

	It("UT-FMC-STAGED-006 [BR-SECURITY-016]: a fenced-read proof mismatch never serves staged state", func() {
		f := newStagedRepairFixture()
		DeferCleanup(f.server.Close)
		DeferCleanup(f.store.Close)

		oldIdentity, _ := f.identity("checkout", "true")
		// No committed state at all: any read must fail closed regardless of
		// staged activity.
		start, err := f.store.BeginRepair(f.ctx, scopecache.BeginRepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: f.fence,
		})
		Expect(err).ToNot(HaveOccurred())
		read, readErr := f.read(start, oldIdentity)
		Expect(readErr).ToNot(HaveOccurred())
		Expect(read.State).ToNot(Equal(scopecache.ReadVerified))
	})
})
