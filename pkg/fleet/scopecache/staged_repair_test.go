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
	"errors"
	"strings"
	"time"

	"github.com/alicebob/miniredis/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"

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

	It("UT-FMC-STAGED-008 [BR-SCOPE-001]: replacing an abandoned staging clears its residual staged index and records", func() {
		f := newStagedRepairFixture()
		DeferCleanup(f.server.Close)
		DeferCleanup(f.store.Close)

		// First staging partially reconciles one member, then is abandoned.
		_, partialObservation := f.identity("abandoned", "true")
		start1, err := f.store.BeginRepair(f.ctx, scopecache.BeginRepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: f.fence,
		})
		Expect(err).ToNot(HaveOccurred())
		_, err = f.store.Repair(f.ctx, scopecache.RepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1,
			WriterFence: f.fence, Generation: start1.Generation, Token: start1.Token,
			Observations: scopecache.ValidatedObservationSet{Items: []scopecache.RepairObservation{partialObservation}, Complete: true, PageCount: 1, ItemCount: 1},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(f.store.MarkRepairUncertain(f.ctx, start1, scopecache.RepairFailure{Code: scopecache.RepairFailureSourceIncomplete, Message: "abandoned mid-reconcile"})).To(Succeed())

		// The replacement staging observes a DIFFERENT single member. Without
		// residual cleanup the abandoned member contaminates the staged index
		// and the commit count check rejects every subsequent cycle.
		identity, observation := f.identity("replacement", "true")
		start2, err := f.store.BeginRepair(f.ctx, scopecache.BeginRepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: f.fence,
		})
		Expect(err).ToNot(HaveOccurred())
		f.commitGeneration(start2, []scopecache.RepairObservation{observation})

		read, readErr := f.read(start2, identity)
		Expect(readErr).ToNot(HaveOccurred())
		Expect(read.State).To(Equal(scopecache.ReadVerified), "a replacement staging must not inherit contaminated members")
	})

	It("UT-FMC-STAGED-009 [BR-SCOPE-001]: the staged status TTL rolls forward on each reconcile so long repairs survive", func() {
		f := newStagedRepairFixture()
		DeferCleanup(f.server.Close)
		DeferCleanup(f.store.Close)

		identity, observation := f.identity("checkout", "true")
		start, err := f.store.BeginRepair(f.ctx, scopecache.BeginRepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: f.fence,
		})
		Expect(err).ToNot(HaveOccurred())

		// A load-slowed repair: 10s pass between begin and reconcile, and the
		// commit lands at t20 -- beyond the original fixed 15s staged window.
		// The reconcile must roll the staged TTL forward for the repair to
		// complete.
		f.server.FastForward(10 * time.Second)
		f.server.SetTime(time.Unix(1_700_000_000, 0).Add(10 * time.Second))
		repair, err := f.store.Repair(f.ctx, scopecache.RepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1,
			WriterFence: f.fence, Generation: start.Generation, Token: start.Token,
			Observations: scopecache.ValidatedObservationSet{Items: []scopecache.RepairObservation{observation}, Complete: true, PageCount: 1, ItemCount: 1},
		})
		Expect(err).ToNot(HaveOccurred())

		f.server.FastForward(10 * time.Second)
		f.server.SetTime(time.Unix(1_700_000_000, 0).Add(20 * time.Second))
		commit, err := f.store.CommitRepair(f.ctx, scopecache.RepairCommitRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1,
			WriterFence: f.fence, Generation: start.Generation, Token: start.Token,
			ExpectedCount: 1, Proof: repair,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(commit.Committed).To(BeTrue(), "a repair whose reconcile activity is recent must not hit a fixed staged TTL ceiling")

		read, readErr := f.read(start, identity)
		Expect(readErr).ToNot(HaveOccurred())
		Expect(read.State).To(Equal(scopecache.ReadVerified))
	})

	It("UT-FMC-STAGED-010 [BR-SCOPE-001]: the staged lifetime cap stops a reconcile-without-commit loop", func() {
		f := newStagedRepairFixture()
		DeferCleanup(f.server.Close)
		DeferCleanup(f.store.Close)

		_, observation := f.identity("checkout", "true")
		start, err := f.store.BeginRepair(f.ctx, scopecache.BeginRepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: f.fence,
		})
		Expect(err).ToNot(HaveOccurred())

		// A reconcile-without-commit loop: successful reconciles at t=10..50
		// each roll the staged TTL forward, so the staging is continuously
		// active and never idles out. Without the lifetime cap this loop
		// could continue indefinitely.
		advanced := time.Duration(0)
		advance := func(d time.Duration) {
			advanced += d
			f.server.FastForward(d)
			f.server.SetTime(time.Unix(1_700_000_000, 0).Add(advanced))
		}
		for step := 1; step <= 5; step++ {
			advance(10 * time.Second)
			_, err = f.store.Repair(f.ctx, scopecache.RepairRequest{
				Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1,
				WriterFence: f.fence, Generation: start.Generation, Token: start.Token,
				Observations: scopecache.ValidatedObservationSet{Items: []scopecache.RepairObservation{observation}, Complete: true, PageCount: 1, ItemCount: 1},
			})
			Expect(err).ToNot(HaveOccurred(), "reconcile at t=%ds within the lifetime cap must succeed", step*10)
		}

		// Reconcile at t=61 -- past the created_at + 60s total staged lifetime
		// cap. The roll cannot extend the staging beyond the cap, so the
		// staged status has expired and reconcile fails closed instead of
		// keeping a never-committing repair alive indefinitely.
		advance(11 * time.Second)
		_, err = f.store.Repair(f.ctx, scopecache.RepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1,
			WriterFence: f.fence, Generation: start.Generation, Token: start.Token,
			Observations: scopecache.ValidatedObservationSet{Items: []scopecache.RepairObservation{observation}, Complete: true, PageCount: 1, ItemCount: 1},
		})
		Expect(err).To(HaveOccurred(), "a staging past its total lifetime cap must not accept further reconciliation")

		// The expired staging no longer blocks the next repair cycle, and the
		// still-live staged records from the loop are cleared as residue: the
		// replacement generation commits and reads cleanly. Without the
		// absent-status residue cleanup, the leftover member contaminates the
		// staged index and the commit count check rejects the cycle.
		start2, err := f.store.BeginRepair(f.ctx, scopecache.BeginRepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: f.fence,
		})
		Expect(err).ToNot(HaveOccurred(), "an expired staging must not block the next repair")
		// The replacement stages a DISTINCT resource: reusing the loop's
		// resource would overwrite the residue member by identical public_id
		// and hide the contamination the cleanup must prevent.
		replacementIdentity, replacementObservation := f.identity("checkout-replacement", "false")
		f.commitGeneration(start2, []scopecache.RepairObservation{replacementObservation})
		read, readErr := f.read(start2, replacementIdentity)
		Expect(readErr).ToNot(HaveOccurred())
		Expect(read.State).To(Equal(scopecache.ReadVerified), "residue from an expired staging must not contaminate the replacement generation")
	})

	It("UT-FMC-STAGED-011 [BR-SCOPE-001, #2660 m2]: a reconcile past the lifetime cap is a terminal stale-fence expiry that the next begin reclaims", func() {
		f := newStagedRepairFixture()
		DeferCleanup(f.server.Close)
		DeferCleanup(f.store.Close)

		_, observation := f.identity("checkout", "true")
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

		// Past the created_at + 60s total staged lifetime cap the staging is
		// terminally expired. The rejection must happen BEFORE any staged
		// write: no NEW staged record or index keys may appear (miniredis
		// does not evict the keys written by the first, legitimate
		// reconcile, so the assertion is set-equality, not emptiness) and
		// the failure is a stale fence: the writer must begin a new repair.
		keysBefore := f.server.Keys()
		f.server.SetTime(time.Unix(1_700_000_000, 0).Add(61 * time.Second))
		_, err = f.store.Repair(f.ctx, scopecache.RepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1,
			WriterFence: f.fence, Generation: start.Generation, Token: start.Token,
			Observations: scopecache.ValidatedObservationSet{Items: []scopecache.RepairObservation{observation}, Complete: true, PageCount: 1, ItemCount: 1},
		})
		Expect(err).To(HaveOccurred(), "a staging past its total lifetime cap must not accept further reconciliation")
		var failure scopecache.RepairFailureError
		Expect(errors.As(err, &failure)).To(BeTrue(), "the failure must be a typed repair failure, got: %v", err)
		Expect(failure.Failure.Code).To(Equal(scopecache.RepairFailureStaleFence),
			"a terminally expired staging requires a new begin, exactly like a stale fence")
		Expect(f.server.Keys()).To(Equal(keysBefore),
			"a past-cap reconcile must not write any new staged record or index keys")

		// The guard also transitions the staging out of 'repairing' into a
		// TTL-bounded 'expired' marker, so begin's gate (state=='repairing')
		// no longer blocks and its residue cleanup reclaims the staging in
		// ONE cycle. This is the distinct case from a TTL-honoring server,
		// where the key is already gone before the guard runs.
		restart, err := f.store.BeginRepair(f.ctx, scopecache.BeginRepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: f.fence,
		})
		Expect(err).ToNot(HaveOccurred(), "an expired staging marker must not block the next begin")
		Expect(restart.Token.Value).ToNot(Equal(start.Token.Value), "the reclaimed staging must be a fresh repair attempt")
	})

	It("UT-FMC-STAGED-013 [BR-SCOPE-001, #2660 m2]: a commit against a gone staging keeps stale-fence fidelity", func() {
		f := newStagedRepairFixture()
		DeferCleanup(f.server.Close)
		DeferCleanup(f.store.Close)

		_, observation := f.identity("checkout", "true")
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

		// The staging expires (TTL bound or restitution) between the
		// reconcile and the commit. The commit must report a stale fence, not
		// a generic rejected/ambiguous write: the syncer keys its retry and
		// its MarkRepairUncertain code on this.
		stagedStatus := ""
		for _, key := range f.server.Keys() {
			if strings.HasSuffix(key, ":status:staged") {
				stagedStatus = key
			}
		}
		Expect(stagedStatus).ToNot(BeEmpty(), "the fixture must have a staged status key")
		f.server.Del(stagedStatus)

		result, err := f.store.CommitRepair(f.ctx, scopecache.RepairCommitRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1,
			WriterFence: f.fence, Generation: start.Generation, Token: start.Token,
			ExpectedCount: 1, Proof: repair,
		})
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, scopecache.ErrStaleFence)).To(BeTrue(),
			"a commit against a gone staging must unwrap to ErrStaleFence, got: %v", err)
		Expect(result.Failure.Code).To(Equal(scopecache.RepairFailureStaleFence),
			"the result must carry the typed stale-fence code, not commit_rejected")
	})

	It("UT-FMC-STAGED-014 [BR-SCOPE-001, #2660 review]: an empty-partition repair against a gone staging keeps stale-fence fidelity", func() {
		f := newStagedRepairFixture()
		DeferCleanup(f.server.Close)
		DeferCleanup(f.store.Close)

		// An empty partition never runs repair_reconcile; its only script
		// call is repair_finalize_empty. When the staging is gone by then,
		// the failure must still be classified stale_fence (the writer must
		// begin a new repair), not the generic commit_rejected the raw
		// STALE_FENCE reply currently falls through to.
		start, err := f.store.BeginRepair(f.ctx, scopecache.BeginRepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: f.fence,
		})
		Expect(err).ToNot(HaveOccurred())
		f.server.FastForward(16 * time.Second)

		_, err = f.store.Repair(f.ctx, scopecache.RepairRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1,
			WriterFence: f.fence, Generation: start.Generation, Token: start.Token,
			Observations: scopecache.ValidatedObservationSet{Items: nil, Complete: true, PageCount: 1, ItemCount: 0},
		})
		Expect(err).To(HaveOccurred(), "a repair against a gone staging must fail closed")
		var failure scopecache.RepairFailureError
		Expect(errors.As(err, &failure)).To(BeTrue(), "the failure must be a typed repair failure, got: %v", err)
		Expect(failure.Failure.Code).To(Equal(scopecache.RepairFailureStaleFence),
			"an empty-partition repair against a gone staging requires a new begin, exactly like a stale fence")
	})

	It("UT-FMC-STAGED-012 [BR-SCOPE-001, #2660 m1]: a non-eligible commit proof is still rejected while the reserved slots stay unenforced", func() {
		f := newStagedRepairFixture()
		DeferCleanup(f.server.Close)
		DeferCleanup(f.store.Close)

		_, observation := f.identity("checkout", "true")
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

		// Drive the production script directly so the contract under test is
		// the Lua itself, not the Go wrapper. The fixture's store already
		// loaded the script, so EVALSHA by digest must resolve.
		client := redis.NewClient(&redis.Options{Addr: f.server.Addr()})
		DeferCleanup(func() { _ = client.Close() })
		manager, err := scopecache.NewScriptManager(client)
		Expect(err).ToNot(HaveOccurred())
		Expect(manager.Digest()).ToNot(BeEmpty())

		var stagedStatus, stagedIndex, stagedRecord string
		for _, key := range f.server.Keys() {
			switch {
			case strings.HasSuffix(key, ":status:staged"):
				stagedStatus = key
			case strings.HasSuffix(key, ":index:staged"):
				stagedIndex = key
			case strings.Contains(key, ":record:staged:"):
				stagedRecord = key
			}
		}
		Expect(stagedStatus).ToNot(BeEmpty(), "the fixture must have a staged status key")
		Expect(stagedIndex).ToNot(BeEmpty(), "the fixture must have a staged index key")
		Expect(stagedRecord).ToNot(BeEmpty(), "the fixture must have a staged record key")

		liveStatus := strings.Replace(stagedStatus, ":staged", "", 1)
		liveIndex := strings.Replace(stagedIndex, ":staged", "", 1)
		recordBase, _, found := strings.Cut(stagedRecord, ":record:staged:")
		Expect(found).To(BeTrue(), "the staged record key must carry the staged record prefix")
		stagedRecordPrefix := recordBase + ":record:staged:"
		liveRecordPrefix := strings.Replace(stagedRecordPrefix, ":staged", "", 1)

		refreshTTL := repair.RecordTTL.ExpiresAtMillis - repair.RecordTTL.ServerNowMillis
		Expect(refreshTTL).To(BeNumerically(">", 0))
		argv := func(recordTTLFlag string) []interface{} {
			// [1]=op, [2..9]=identity, [10]=expected count, [11..12]=record
			// prefixes, [13..16]=reserved structural slots (kept for stable
			// numbering), [17..20]=proof flags, [21]=refresh TTL millis.
			return []interface{}{
				"commit", f.part.RegistrationUID, "1",
				f.fence.HolderPodUID, f.fence.HolderSessionUID,
				f.fence.LeaseEpoch, f.fence.Fence, start.Generation, start.Token.Value,
				1, liveRecordPrefix, stagedRecordPrefix,
				"1", "1", "1", "1",
				recordTTLFlag, "1", "1", "0",
				refreshTTL,
			}
		}
		evalCommit := func(recordTTLFlag string) error {
			commandArgs := []interface{}{"EVALSHA", manager.Digest(), 4, liveStatus, stagedStatus, liveIndex, stagedIndex}
			commandArgs = append(commandArgs, argv(recordTTLFlag)...)
			return client.Do(f.ctx, commandArgs...).Err()
		}

		// Rejection case FIRST: the control commit consumes the staging.
		Expect(evalCommit("0")).To(MatchError(ContainSubstring("COMMIT_REJECTED")),
			"a non-eligible commit proof must still be rejected inside the script")
		Expect(evalCommit("1")).ToNot(HaveOccurred(), "an all-valid proof must commit")
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
