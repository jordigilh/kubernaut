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
	"github.com/redis/go-redis/v9"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// DD-FLEET-017 D2.5a / D2.4: the lexicographic identity index and the
// proof-gated page read, exercised through the real atomic script on Valkey
// semantics (miniredis). These specs prove the *store* half of the contract;
// the page reader's projection lives in pkg/fleet/fmc.
type inventoryCacheFixture struct {
	ctx        context.Context
	server     *miniredis.Miniredis
	store      *scopecache.ValkeyStatusStore
	admin      *redis.Client
	fence      scopecache.WriterFence
	part       scopecache.PartitionStateKey
	generation uint64
}

func newInventoryCacheFixture() *inventoryCacheFixture {
	server, err := miniredis.Run()
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	server.SetTime(time.Unix(1_700_000_000, 0))
	store, err := scopecache.NewValkeyStatusStoreAt(server.Addr())
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	ExpectWithOffset(1, store.LoadScript(context.Background())).To(Succeed())
	admin := redis.NewClient(&redis.Options{Addr: server.Addr()})
	return &inventoryCacheFixture{
		ctx:    context.Background(),
		server: server,
		store:  store,
		admin:  admin,
		fence:  scopecache.WriterFence{HolderPodUID: "pod-1", HolderSessionUID: "session-1", LeaseEpoch: 7, Fence: 7},
		part:   scopecache.PartitionStateKey{ClusterID: "prod-east", RegistrationUID: "registration-1"},
	}
}

func (f *inventoryCacheFixture) close() {
	if f.admin != nil {
		_ = f.admin.Close()
	}
	if f.store != nil {
		_ = f.store.Close()
	}
	if f.server != nil {
		f.server.Close()
	}
}

func (f *inventoryCacheFixture) observation(namespace, kind, group, version, name, label string) scopecache.RepairObservation {
	apiVersion := version
	if group != "" {
		apiVersion = group + "/" + version
	}
	return scopecache.RepairObservation{
		Identity: scopecache.ResourceIdentity{
			ClusterID: f.part.ClusterID,
			Group:     group,
			Version:   version,
			Kind:      kind,
			Namespace: namespace,
			Name:      name,
		},
		APIVersion:    apiVersion,
		LabelsPresent: true,
		LabelState:    scope.LabelState(label),
		TTL:           30 * time.Second,
	}
}

func (f *inventoryCacheFixture) begin() scopecache.RepairStartResult {
	start, err := f.store.BeginRepair(f.ctx, scopecache.BeginRepairRequest{
		Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1, WriterFence: f.fence,
	})
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	return start
}

func (f *inventoryCacheFixture) repair(start scopecache.RepairStartResult, observations []scopecache.RepairObservation) scopecache.RepairResult {
	repair, err := f.store.Repair(f.ctx, scopecache.RepairRequest{
		Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1,
		WriterFence: f.fence, Generation: start.Generation, Token: start.Token,
		Observations: scopecache.ValidatedObservationSet{Items: observations, Complete: true, PageCount: 1, ItemCount: len(observations)},
	})
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	return repair
}

func (f *inventoryCacheFixture) commit(start scopecache.RepairStartResult, observations []scopecache.RepairObservation) scopecache.RepairCommitResult {
	repair := f.repair(start, observations)
	return f.commitProof(start, repair)
}

func (f *inventoryCacheFixture) commitProof(start scopecache.RepairStartResult, repair scopecache.RepairResult) scopecache.RepairCommitResult {
	commit, err := f.store.CommitRepair(f.ctx, scopecache.RepairCommitRequest{
		Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1,
		WriterFence: f.fence, Generation: start.Generation, Token: start.Token,
		ExpectedCount: repair.ExpectedCount, Proof: repair,
	})
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	return commit
}

func (f *inventoryCacheFixture) pageRequest(prefix scopecache.InventoryPrefix, cursor string, limit int) scopecache.InventoryPageRequest {
	return scopecache.InventoryPageRequest{
		Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1,
		WriterFence: f.fence, Generation: f.generation, Prefix: prefix, Cursor: cursor, Limit: limit,
	}
}

func (f *inventoryCacheFixture) header(prefix scopecache.InventoryPrefix, cursor string, limit int) scopecache.InventoryPageHeader {
	header, err := f.store.ReadInventoryPageHeader(f.ctx, f.pageRequest(prefix, cursor, limit))
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	if header.Generation != 0 {
		ExpectWithOffset(1, header.Generation).To(Equal(f.generation))
	}
	return header
}

func (f *inventoryCacheFixture) keys(pattern string) []string {
	keys, err := f.admin.Keys(f.ctx, pattern).Result()
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	return keys
}

func (f *inventoryCacheFixture) liveLexKeys() []string   { return f.keys("kubernaut:fmc:v2:*:index:lex") }
func (f *inventoryCacheFixture) liveIndexKeys() []string { return f.keys("kubernaut:fmc:v2:*:index") }
func (f *inventoryCacheFixture) stagedLexKeys() []string {
	return f.keys("kubernaut:fmc:v2:*:index:lex:staged")
}

func namesOf(members []scopecache.InventoryMember) []string {
	names := make([]string, 0, len(members))
	for _, member := range members {
		names = append(names, member.Identity.Namespace+"/"+member.Identity.Name)
	}
	return names
}

var _ = Describe("UT-FMC-SRC-2661-020 [BR-SCOPE-001]: lexicographic identity index in the commit path", func() {
	It("writes both indexes from one staged generation and serves the lexicographic page", func() {
		f := newInventoryCacheFixture()
		DeferCleanup(f.close)

		start := f.begin()
		f.generation = start.Generation
		commit := f.commit(start, []scopecache.RepairObservation{
			f.observation("ns-b", "Deployment", "apps", "v1", "checkout", "true"),
			f.observation("ns-a", "ConfigMap", "", "v1", "settings", "absent"),
			f.observation("ns-a", "Deployment", "apps", "v1", "checkout", "true"),
			f.observation("ns-b", "ConfigMap", "", "v1", "settings", "false"),
		})
		Expect(commit.Committed).To(BeTrue())
		Expect(commit.CleanCommit).To(BeTrue())

		Expect(f.liveLexKeys()).To(HaveLen(1), "one commit must publish the lexicographic index")
		Expect(f.liveIndexKeys()).To(HaveLen(1), "one commit must publish the data index")
		Expect(f.admin.ZCard(f.ctx, f.liveLexKeys()[0]).Val()).To(Equal(int64(4)))
		Expect(f.admin.ZCard(f.ctx, f.liveIndexKeys()[0]).Val()).To(Equal(int64(4)))
		Expect(f.stagedLexKeys()).To(BeEmpty(), "the swap must consume the staged lexicographic index")

		header := f.header(scopecache.InventoryPrefix{}, "", 10)
		Expect(header.State).To(Equal(scopecache.InventoryReadOK))
		// namespace-first, then kind, group, version, name: the deliberate
		// divergence from inventory.SortResources (DD-FLEET-017 D2.5a).
		Expect(namesOf(header.Members)).To(Equal([]string{
			"ns-a/settings", "ns-a/checkout", "ns-b/settings", "ns-b/checkout",
		}))
	})

	It("rejects a commit whose indexes disagree with LEX_COUNT_MISMATCH", func() {
		f := newInventoryCacheFixture()
		DeferCleanup(f.close)

		start := f.begin()
		f.generation = start.Generation
		repair := f.repair(start, []scopecache.RepairObservation{
			f.observation("ns-a", "Deployment", "apps", "v1", "one", "true"),
			f.observation("ns-a", "Deployment", "apps", "v1", "two", "true"),
		})
		stagedLex := f.stagedLexKeys()
		Expect(stagedLex).To(HaveLen(1))
		removed, err := f.admin.ZRem(f.ctx, stagedLex[0], f.admin.ZRange(f.ctx, stagedLex[0], 0, 0).Val()[0]).Result()
		Expect(err).ToNot(HaveOccurred())
		Expect(removed).To(Equal(int64(1)))

		_, err = f.store.CommitRepair(f.ctx, scopecache.RepairCommitRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1,
			WriterFence: f.fence, Generation: start.Generation, Token: start.Token,
			ExpectedCount: 2, Proof: repair,
		})
		Expect(err).To(HaveOccurred())
		Expect(strings.ToLower(err.Error())).To(ContainSubstring("lex_count_mismatch"),
			"a writer defect must be rejected, not published as an unpaginable page")
		Expect(f.liveLexKeys()).To(BeEmpty(), "a rejected commit must publish nothing")
	})

	It("clears an abandoned staging's lexicographic index on the next begin", func() {
		f := newInventoryCacheFixture()
		DeferCleanup(f.close)

		start := f.begin()
		f.generation = start.Generation
		f.commit(start, []scopecache.RepairObservation{
			f.observation("ns-a", "Deployment", "apps", "v1", "committed", "true"),
		})

		abandoned := f.begin()
		f.repair(abandoned, []scopecache.RepairObservation{
			f.observation("ns-residue", "Deployment", "apps", "v1", "ghost", "true"),
		})
		Expect(f.stagedLexKeys()).To(HaveLen(1))
		Expect(f.store.MarkRepairUncertain(f.ctx, abandoned, scopecache.RepairFailure{
			Code: scopecache.RepairFailureSourceIncomplete, Message: "abandoned",
		})).To(Succeed())

		replacement := f.begin()
		f.generation = replacement.Generation
		Expect(f.stagedLexKeys()).To(BeEmpty(), "begin must clear an abandoned staging's lexicographic index")

		f.commit(replacement, []scopecache.RepairObservation{
			f.observation("ns-a", "Deployment", "apps", "v1", "replacement", "true"),
		})
		header := f.header(scopecache.InventoryPrefix{}, "", 10)
		Expect(header.State).To(Equal(scopecache.InventoryReadOK))
		Expect(namesOf(header.Members)).To(Equal([]string{"ns-a/replacement"}),
			"no member of the abandoned generation may survive into the committed page")
		Expect(f.admin.ZCard(f.ctx, f.liveLexKeys()[0]).Val()).To(Equal(int64(1)))
	})

	It("removes both indexes for an empty commit and reports the proof-derived empty page", func() {
		f := newInventoryCacheFixture()
		DeferCleanup(f.close)

		start := f.begin()
		f.generation = start.Generation
		f.commit(start, []scopecache.RepairObservation{
			f.observation("ns-a", "Deployment", "apps", "v1", "gone", "true"),
		})
		Expect(f.liveLexKeys()).To(HaveLen(1))

		empty := f.begin()
		f.generation = empty.Generation
		f.commit(empty, nil)
		Expect(f.liveLexKeys()).To(BeEmpty(), "a proven-empty partition carries no lexicographic index")
		Expect(f.liveIndexKeys()).To(BeEmpty(), "a proven-empty partition carries no data index")

		header := f.header(scopecache.InventoryPrefix{}, "", 10)
		Expect(header.State).To(Equal(scopecache.InventoryReadEmpty))
		Expect(header.Members).To(BeEmpty())
		Expect(header.RemainingProofTTL).To(BeNumerically(">", 0))
	})

	It("enforces the per-index member bound on both indexes", func() {
		f := newInventoryCacheFixture()
		DeferCleanup(f.close)

		start := f.begin()
		f.generation = start.Generation
		repair := f.repair(start, []scopecache.RepairObservation{
			f.observation("ns-a", "Deployment", "apps", "v1", "one", "true"),
		})
		inflate := func(key string, count int) {
			values := make([]redis.Z, 0, count)
			for index := 0; index < count; index++ {
				values = append(values, redis.Z{Score: 0, Member: fmt.Sprintf("filler-%05d", index)})
			}
			ExpectWithOffset(1, f.admin.ZAdd(f.ctx, key, values...).Err()).ToNot(HaveOccurred())
		}
		stagedIndex := f.keys("kubernaut:fmc:v2:*:index:staged")
		stagedLex := f.stagedLexKeys()
		Expect(stagedIndex).To(HaveLen(1))
		Expect(stagedLex).To(HaveLen(1))

		inflate(stagedIndex[0], 10_000)
		_, err := f.store.CommitRepair(f.ctx, scopecache.RepairCommitRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1,
			WriterFence: f.fence, Generation: start.Generation, Token: start.Token,
			ExpectedCount: repair.ExpectedCount, Proof: repair,
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("INDEX_BOUNDS"))

		Expect(f.admin.ZRemRangeByRank(f.ctx, stagedIndex[0], 1, -1).Err()).ToNot(HaveOccurred())
		inflate(stagedLex[0], 10_000)
		_, err = f.store.CommitRepair(f.ctx, scopecache.RepairCommitRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1,
			WriterFence: f.fence, Generation: start.Generation, Token: start.Token,
			ExpectedCount: repair.ExpectedCount, Proof: repair,
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("INDEX_BOUNDS"))
	})
})

var _ = Describe("UT-FMC-INV-2661-002S/014S [BR-SAFETY-001]: proof-gated page reads fail closed", func() {
	It("derives empty from the proof, never from index presence", func() {
		f := newInventoryCacheFixture()
		DeferCleanup(f.close)

		start := f.begin()
		f.generation = start.Generation
		f.commit(start, nil)
		header := f.header(scopecache.InventoryPrefix{}, "", 10)
		Expect(header.State).To(Equal(scopecache.InventoryReadEmpty))
		Expect(f.liveLexKeys()).To(BeEmpty())
		Expect(f.liveIndexKeys()).To(BeEmpty())
	})

	It("fails closed when a populated partition has no lexicographic index", func() {
		f := newInventoryCacheFixture()
		DeferCleanup(f.close)

		start := f.begin()
		f.generation = start.Generation
		f.commit(start, []scopecache.RepairObservation{
			f.observation("ns-a", "Deployment", "apps", "v1", "one", "true"),
		})
		Expect(f.admin.Del(f.ctx, f.liveLexKeys()[0]).Err()).ToNot(HaveOccurred())

		header := f.header(scopecache.InventoryPrefix{}, "", 10)
		Expect(header.State).To(Equal(scopecache.InventoryReadMissing),
			"a missing index is never evidence of an empty partition")
		Expect(header.Reason).To(Equal("no_lex_index"))
		Expect(header.Members).To(BeEmpty())
	})

	It("becomes readable again after the next clean commit", func() {
		f := newInventoryCacheFixture()
		DeferCleanup(f.close)

		start := f.begin()
		f.generation = start.Generation
		f.commit(start, []scopecache.RepairObservation{
			f.observation("ns-a", "Deployment", "apps", "v1", "one", "true"),
		})
		Expect(f.admin.Del(f.ctx, f.liveLexKeys()[0]).Err()).ToNot(HaveOccurred())
		Expect(f.header(scopecache.InventoryPrefix{}, "", 10).State).To(Equal(scopecache.InventoryReadMissing))

		next := f.begin()
		f.generation = next.Generation
		f.commit(next, []scopecache.RepairObservation{
			f.observation("ns-a", "Deployment", "apps", "v1", "one", "true"),
		})
		header := f.header(scopecache.InventoryPrefix{}, "", 10)
		Expect(header.State).To(Equal(scopecache.InventoryReadOK))
		Expect(namesOf(header.Members)).To(Equal([]string{"ns-a/one"}))
	})

	It("serves only the committed generation while a repair is staged", func() {
		f := newInventoryCacheFixture()
		DeferCleanup(f.close)

		start := f.begin()
		f.generation = start.Generation
		f.commit(start, []scopecache.RepairObservation{
			f.observation("ns-a", "Deployment", "apps", "v1", "committed", "true"),
		})

		staged := f.begin()
		f.repair(staged, []scopecache.RepairObservation{
			f.observation("ns-a", "Deployment", "apps", "v1", "staged-only", "true"),
		})
		header := f.header(scopecache.InventoryPrefix{}, "", 10)
		Expect(header.State).To(Equal(scopecache.InventoryReadOK))
		Expect(namesOf(header.Members)).To(Equal([]string{"ns-a/committed"}),
			"staged state must stay invisible to the committed page")
	})

	It("reports a missing committed proof for a staging-only partition", func() {
		f := newInventoryCacheFixture()
		DeferCleanup(f.close)

		staged := f.begin()
		f.generation = staged.Generation
		f.repair(staged, []scopecache.RepairObservation{
			f.observation("ns-a", "Deployment", "apps", "v1", "staged-only", "true"),
		})
		header := f.header(scopecache.InventoryPrefix{}, "", 10)
		Expect(header.State).To(Equal(scopecache.InventoryReadMissing))
		Expect(header.Reason).To(Equal("no_status"))
	})
})

var _ = Describe("UT-FMC-INV-2661-012S/013S [BR-API-001]: lexicographic range reads", func() {
	It("pages with an exclusive member bound and prefix ranges", func() {
		f := newInventoryCacheFixture()
		DeferCleanup(f.close)

		start := f.begin()
		f.generation = start.Generation
		f.commit(start, []scopecache.RepairObservation{
			f.observation("ns-a", "ConfigMap", "", "v1", "settings", "true"),
			f.observation("ns-a", "Deployment", "apps", "v1", "api", "true"),
			f.observation("ns-a", "Deployment", "apps", "v1", "web", "true"),
			f.observation("ns-b", "Deployment", "apps", "v1", "api", "true"),
			f.observation("ns-c", "StatefulSet", "apps", "v1beta1", "db", "true"),
		})

		first := f.header(scopecache.InventoryPrefix{}, "", 2)
		Expect(first.State).To(Equal(scopecache.InventoryReadOK))
		// namespace-first, then kind, then group/version, then name.
		Expect(namesOf(first.Members)).To(Equal([]string{"ns-a/settings", "ns-a/api"}))

		second := f.header(scopecache.InventoryPrefix{}, first.Members[1].PublicID, 2)
		Expect(namesOf(second.Members)).To(Equal([]string{"ns-a/web", "ns-b/api"}),
			"the cursor is an exclusive lower bound, so paging never repeats a member")

		third := f.header(scopecache.InventoryPrefix{}, second.Members[1].PublicID, 2)
		Expect(namesOf(third.Members)).To(Equal([]string{"ns-c/db"}))

		namespace := f.header(scopecache.InventoryPrefix{Namespace: "ns-a"}, "", 10)
		Expect(namesOf(namespace.Members)).To(Equal([]string{"ns-a/settings", "ns-a/api", "ns-a/web"}))

		namespaceKind := f.header(scopecache.InventoryPrefix{Namespace: "ns-a", Kind: "Deployment"}, "", 10)
		Expect(namesOf(namespaceKind.Members)).To(Equal([]string{"ns-a/api", "ns-a/web"}))

		exact := f.header(scopecache.InventoryPrefix{Namespace: "ns-a", Kind: "Deployment", Group: "apps", Version: "v1", Name: "web"}, "", 10)
		Expect(namesOf(exact.Members)).To(Equal([]string{"ns-a/web"}))

		// A name cannot be encoded without a group/version pair, so that query
		// scans the namespace+kind range and the page reader applies the
		// residual name filter (DD-FLEET-017 D2.5a: documented, not hidden).
		partial := f.header(scopecache.InventoryPrefix{Namespace: "ns-a", Kind: "Deployment", Name: "web"}, "", 10)
		Expect(namesOf(partial.Members)).To(Equal([]string{"ns-a/api", "ns-a/web"}))

		Expect(f.header(scopecache.InventoryPrefix{Namespace: "ns-zzz"}, "", 10).State).To(Equal(scopecache.InventoryReadOK))
		Expect(f.header(scopecache.InventoryPrefix{Namespace: "ns-zzz"}, "", 10).Members).To(BeEmpty())
	})

	It("reports cursor drift instead of guessing an identity", func() {
		f := newInventoryCacheFixture()
		DeferCleanup(f.close)

		start := f.begin()
		f.generation = start.Generation
		f.commit(start, []scopecache.RepairObservation{
			f.observation("ns-a", "Deployment", "apps", "v1", "one", "true"),
		})
		goneID := "r-" + strings.Repeat("a", 64)
		header := f.header(scopecache.InventoryPrefix{}, goneID, 10)
		Expect(header.State).To(Equal(scopecache.InventoryReadDrift),
			"a cursor whose identity left the generation is cache drift, never caller error")
		Expect(header.Reason).To(Equal("cursor_missing"))
	})
})

var _ = Describe("UT-FMC-INV-2661-002 [BR-SAFETY-001]: page records and inheritance payload", func() {
	It("returns the page's records plus the namespace records needed for inheritance", func() {
		f := newInventoryCacheFixture()
		DeferCleanup(f.close)

		start := f.begin()
		f.generation = start.Generation
		f.commit(start, []scopecache.RepairObservation{
			f.observation("", "Namespace", "", "v1", "ns-a", "true"),
			f.observation("ns-a", "Deployment", "apps", "v1", "web", "absent"),
			f.observation("ns-b", "Deployment", "apps", "v1", "api", "false"),
		})

		header := f.header(scopecache.InventoryPrefix{}, "", 10)
		Expect(header.State).To(Equal(scopecache.InventoryReadOK))
		records, err := f.store.ReadInventoryPageRecords(f.ctx, scopecache.InventoryPageRecordsRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1,
			WriterFence: f.fence, Generation: f.generation,
			Members:    []string{header.Members[1].Member, header.Members[2].Member},
			Namespaces: []string{"ns-a", "ns-b"},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(records.State).To(Equal(scopecache.InventoryReadOK))
		Expect(records.Records).To(HaveLen(2))
		Expect(records.Records[0].Identity.Namespace).To(Equal("ns-a"))
		Expect(records.Records[0].LabelState).To(Equal(scope.LabelStateAbsent))
		Expect(records.NamespaceLabels).To(HaveLen(2))
		Expect(records.NamespaceLabels["ns-a"]).To(Equal(scopecache.InventoryNamespaceLabel{Present: true, LabelState: scope.LabelStateTrue}))
		// No Namespace record was observed for ns-b, so its inherited label is
		// unproven: the label is absent, not "managed=false".
		Expect(records.NamespaceLabels["ns-b"].Present).To(BeFalse())
		Expect(records.RemainingProofTTL).To(BeNumerically(">", 0))
	})

	It("fails the whole page closed when one record is gone", func() {
		f := newInventoryCacheFixture()
		DeferCleanup(f.close)

		start := f.begin()
		f.generation = start.Generation
		f.commit(start, []scopecache.RepairObservation{
			f.observation("ns-a", "Deployment", "apps", "v1", "web", "true"),
		})
		header := f.header(scopecache.InventoryPrefix{}, "", 10)
		Expect(header.State).To(Equal(scopecache.InventoryReadOK))
		recordKeys := f.keys("kubernaut:fmc:v2:*:record:*")
		Expect(recordKeys).To(HaveLen(1))
		Expect(f.admin.Del(f.ctx, recordKeys[0]).Err()).ToNot(HaveOccurred())

		records, err := f.store.ReadInventoryPageRecords(f.ctx, scopecache.InventoryPageRecordsRequest{
			Partition: f.part, RegistrationUID: f.part.RegistrationUID, ExpectedRegistryRevision: 1,
			WriterFence: f.fence, Generation: f.generation,
			Members: []string{header.Members[0].Member},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(records.State).To(Equal(scopecache.InventoryReadMissing),
			"a single unverifiable record fails the page, never a partial set")
	})
})
