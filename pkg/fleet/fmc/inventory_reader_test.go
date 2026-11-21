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

package fmc_test

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

const (
	inventoryClusterID     = "prod-east"
	inventoryRegistration  = "registration-1"
	inventoryRevision      = uint64(3)
	inventoryGeneration    = uint64(7)
	inventoryCacheTTL      = 45 * time.Second
	inventorySyncInterval  = 30 * time.Second
	inventoryStaleAfter    = 40 * time.Second
	inventoryProofRemained = 60 * time.Second

	reasonNoLexIndex = "no_lex_index"
)

type fakeInventoryRecord struct {
	member   string
	publicID string
	record   scopecache.InventoryRecord
}

// fakeInventoryStore mimics the atomic script's page contract: a committed
// generation ordered by lexicographic identity, an exclusive cursor bound, a
// range prefix, and proof-gated records. The script itself is proven against
// real Valkey semantics in pkg/fleet/scopecache/inventory_page_test.go; this
// fake lets the page reader's projection be specified without a Valkey.
type fakeInventoryStore struct {
	partition    scopecache.PartitionStateKey
	records      []fakeInventoryRecord
	namespaces   map[string]scopecache.InventoryNamespaceLabel
	remaining    time.Duration
	state        scopecache.InventoryReadState
	reason       string
	headerErr    error
	recordsErr   error
	exactRecord  *scopecache.InventoryResourceRecord
	unproven     bool
	staleOnce    int
	headerCalls  int
	recordsCalls int
	limits       []int
}

func newFakeInventoryStore() *fakeInventoryStore {
	return &fakeInventoryStore{
		partition:  scopecache.PartitionStateKey{ClusterID: inventoryClusterID, RegistrationUID: inventoryRegistration},
		namespaces: make(map[string]scopecache.InventoryNamespaceLabel),
		remaining:  inventoryProofRemained,
	}
}

func (f *fakeInventoryStore) put(namespace, kind, group, version, name, label string) string {
	identity := scopecache.ResourceIdentity{
		ClusterID: inventoryClusterID, Group: group, Version: version, Kind: kind, Namespace: namespace, Name: name,
	}
	publicID := inventory.CanonicalResourceID(scope.ResourceIdentity{
		ClusterID: inventoryClusterID, Group: group, Version: version, Kind: kind, Namespace: namespace, Name: name,
	})
	f.records = append(f.records, fakeInventoryRecord{
		member:   scopecache.LexicalMember(identity, publicID),
		publicID: publicID,
		record: scopecache.InventoryRecord{
			Identity: identity, LabelState: scope.LabelState(label), Generation: inventoryGeneration,
		},
	})
	sort.Slice(f.records, func(i, j int) bool { return f.records[i].member < f.records[j].member })
	return publicID
}

func (f *fakeInventoryStore) setNamespace(name, label string) {
	f.namespaces[name] = scopecache.InventoryNamespaceLabel{Present: true, LabelState: scope.LabelState(label)}
}

// rangePrefix mirrors the script's encodable range: only a leading run of
// separator-terminated fields is a range, and everything else stays a
// read-time filter.
func rangePrefix(prefix scopecache.InventoryPrefix) string {
	if prefix.Namespace == "" {
		return ""
	}
	value := prefix.Namespace
	if prefix.Kind == "" {
		return value + "\x00"
	}
	value += "\x00" + prefix.Kind
	if prefix.Group != "" && prefix.Version != "" {
		value += "\x00" + prefix.Group + "\x00" + prefix.Version
		if prefix.Name != "" {
			value += "\x00" + prefix.Name
		}
	}
	return value + "\x00"
}

func (f *fakeInventoryStore) ReadInventoryPageHeader(_ context.Context, request scopecache.InventoryPageRequest) (scopecache.InventoryPageHeader, error) {
	f.headerCalls++
	f.limits = append(f.limits, request.Limit)
	if f.staleOnce > 0 {
		f.staleOnce--
		return scopecache.InventoryPageHeader{State: scopecache.InventoryReadMissing, Reason: "stale_proof"}, nil
	}
	if f.headerErr != nil {
		return scopecache.InventoryPageHeader{}, f.headerErr
	}
	switch f.state {
	case scopecache.InventoryReadEmpty:
		return scopecache.InventoryPageHeader{State: scopecache.InventoryReadEmpty, Generation: request.Generation, RemainingProofTTL: f.remaining}, nil
	case scopecache.InventoryReadMissing:
		return scopecache.InventoryPageHeader{State: scopecache.InventoryReadMissing, Reason: f.reason}, nil
	}
	handler := rangePrefix(request.Prefix)
	start := 0
	if request.Cursor != "" {
		found := false
		for index, record := range f.records {
			if record.publicID == request.Cursor {
				start, found = index+1, true
				break
			}
		}
		if !found {
			return scopecache.InventoryPageHeader{State: scopecache.InventoryReadDrift, Reason: "cursor_missing"}, nil
		}
	}
	members := make([]scopecache.InventoryMember, 0, request.Limit)
	for _, record := range f.records[start:] {
		if len(members) == request.Limit {
			break
		}
		if handler != "" && !strings.HasPrefix(record.member, handler) {
			continue
		}
		members = append(members, scopecache.InventoryMember{
			Member: record.member, PublicID: record.publicID, Identity: record.record.Identity, Generation: request.Generation,
		})
	}
	return scopecache.InventoryPageHeader{
		State: scopecache.InventoryReadOK, Members: members, Generation: request.Generation, RemainingProofTTL: f.remaining,
	}, nil
}

func (f *fakeInventoryStore) ReadInventoryPageRecords(_ context.Context, request scopecache.InventoryPageRecordsRequest) (scopecache.InventoryPageRecords, error) {
	f.recordsCalls++
	if f.recordsErr != nil {
		return scopecache.InventoryPageRecords{}, f.recordsErr
	}
	if f.state == scopecache.InventoryReadMissing {
		return scopecache.InventoryPageRecords{State: scopecache.InventoryReadMissing, Reason: f.reason}, nil
	}
	result := scopecache.InventoryPageRecords{
		State:             scopecache.InventoryReadOK,
		Generation:        request.Generation,
		RemainingProofTTL: f.remaining,
		NamespaceLabels:   make(map[string]scopecache.InventoryNamespaceLabel, len(request.Namespaces)),
	}
	for _, member := range request.Members {
		found := false
		for _, record := range f.records {
			if record.member != member {
				continue
			}
			result.Records = append(result.Records, record.record)
			found = true
			break
		}
		if !found {
			return scopecache.InventoryPageRecords{State: scopecache.InventoryReadMissing, Reason: "record_missing"}, nil
		}
	}
	for _, namespace := range request.Namespaces {
		result.NamespaceLabels[namespace] = f.namespaces[namespace]
	}
	return result, nil
}

func (f *fakeInventoryStore) ReadInventoryResource(_ context.Context, request scopecache.InventoryResourceRequest) (scopecache.InventoryResourceRecord, error) {
	if f.unproven {
		return scopecache.InventoryResourceRecord{State: scopecache.InventoryReadUnproven, Reason: "stale_proof"}, nil
	}
	if f.exactRecord != nil {
		return *f.exactRecord, nil
	}
	for _, record := range f.records {
		if record.publicID == request.PublicID {
			return scopecache.InventoryResourceRecord{
				State: scopecache.InventoryReadOK, Record: record.record, Generation: request.Generation, RemainingProofTTL: f.remaining,
			}, nil
		}
	}
	// An absent identity is still answered under the valid proof, so it carries
	// that proof's remaining life exactly like the atomic script does.
	return scopecache.InventoryResourceRecord{State: scopecache.InventoryReadMissing, RemainingProofTTL: f.remaining}, nil
}

// provenReadiness builds a clean partition proof at inventoryGeneration.
func provenReadiness() (*scopecache.FMCReadinessSnapshot, scopecache.WriterFence) {
	fence := scopecache.WriterFence{HolderPodUID: "pod-1", HolderSessionUID: "session-1", LeaseEpoch: 5, Fence: 5}
	lease := scopecache.LeaseView{
		Policy:         scopecache.LeasePolicyProcessWideRequired,
		Active:         true,
		Valid:          true,
		Epoch:          fence.LeaseEpoch,
		SessionUID:     fence.HolderSessionUID,
		WriterFence:    fence,
		HeartbeatAt:    time.Now().UTC(),
		HeartbeatValid: true,
	}
	readiness, err := scopecache.NewFMCReadinessSnapshot(scopecache.LeasePolicyProcessWideRequired)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	ExpectWithOffset(1, readiness.PublishGlobal(scopecache.FMCGlobalReadiness{
		MCPInitialSync:       true,
		RegistryInitialSync:  true,
		RegistryWatchHealthy: true,
		ScriptReady:          true,
		ValkeyReady:          true,
		Lease:                lease,
		ActiveLeaseValid:     true,
		LeasePolicy:          scopecache.LeasePolicyProcessWideRequired,
	})).To(Succeed())
	ExpectWithOffset(1, readiness.PublishWriterLease(scopecache.WriterFence{}, lease)).To(Succeed())
	ExpectWithOffset(1, readiness.ReconcileRegistry(registry.RegistrySnapshot{
		Generation: inventoryRevision,
		Registrations: []registry.RegistrationSnapshot{{
			Info:     registry.ClusterInfo{ID: inventoryClusterID, RegistrationUID: inventoryRegistration},
			Revision: inventoryRevision,
		}},
	})).To(Succeed())
	partition := scopecache.PartitionStateKey{ClusterID: inventoryClusterID, RegistrationUID: inventoryRegistration}
	ExpectWithOffset(1, readiness.BeginPartitionRepair(partition, inventoryRegistration, inventoryRevision, lease, inventoryGeneration)).To(Succeed())
	ExpectWithOffset(1, readiness.PublishPartitionClean(partition, scopecache.PartitionReadiness{
		Key:                  partition,
		RegistrationUID:      inventoryRegistration,
		RegistrationRevision: inventoryRevision,
		LeaseEpoch:           fence.LeaseEpoch,
		LeaseSessionUID:      fence.HolderSessionUID,
		WriterFence:          fence,
		LeaseHeartbeatAt:     lease.HeartbeatAt,
		LeaseHeartbeatValid:  true,
		Generation:           inventoryGeneration,
		State:                scopecache.PartitionClean,
		CleanCommit:          true,
		RecordValid:          true,
		IndexValid:           true,
		StatusValid:          true,
		RecordTTLValid:       true,
		IndexTTLValid:        true,
		StatusTTLValid:       true,
	})).To(Succeed())
	return readiness, fence
}

func newInventoryReaderFixture(store *fakeInventoryStore, cfg fmc.InventoryReaderConfig) (*fmc.InventoryReader, *fencedRegistry) {
	readiness, _ := provenReadiness()
	registration := newFencedRegistry(registry.ClusterInfo{ID: inventoryClusterID, RegistrationUID: inventoryRegistration}, inventoryRevision)
	reader, err := fmc.NewInventoryReader(store, registration, readiness, cfg)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	return reader, registration
}

func inventoryFilters() inventory.Filters {
	return inventory.Filters{ClusterID: inventoryClusterID}
}

func namesOfResources(resources []inventory.Resource) []string {
	names := make([]string, 0, len(resources))
	for _, resource := range resources {
		names = append(names, resource.Namespace+"/"+resource.Name)
	}
	return names
}

var _ = Describe("UT-FMC-INV-2661-001 [BR-API-001]: proven page assembly", func() {
	var (
		ctx    context.Context
		store  *fakeInventoryStore
		reader *fmc.InventoryReader
	)

	BeforeEach(func() {
		ctx = context.Background()
		store = newFakeInventoryStore()
		store.setNamespace("payments", "absent")
		store.setNamespace("checkout", "true")
		store.put("payments", "Deployment", "apps", "v1", "api", "true")
		store.put("payments", "Deployment", "apps", "v1", "web", "true")
		store.put("checkout", "Deployment", "apps", "v1", "cart", "absent")
		reader, _ = newInventoryReaderFixture(store, fmc.InventoryReaderConfig{})
	})

	It("serves a fresh success page with a cursor while more remain", func() {
		filters := inventoryFilters()
		filters.Limit = 2
		page, err := reader.List(ctx, filters)
		Expect(err).ToNot(HaveOccurred())
		Expect(page.Validate()).To(Succeed())
		Expect(page.State).To(Equal(inventory.StateSuccess))
		Expect(page.Freshness.State).To(Equal(inventory.FreshnessFresh))
		Expect(page.Freshness.AsOf).ToNot(BeZero(), "as_of is the projection completion time")
		Expect(page.ObservedAt).To(Equal(page.Freshness.AsOf))
		Expect(page.Items).To(HaveLen(2))
		Expect(namesOfResources(page.Items)).To(Equal([]string{"checkout/cart", "payments/api"}))
		Expect(page.NextCursor).To(Equal(page.Items[1].ID))

		next := inventoryFilters()
		next.Limit = 2
		next.Cursor = page.NextCursor
		second, err := reader.List(ctx, next)
		Expect(err).ToNot(HaveOccurred())
		Expect(namesOfResources(second.Items)).To(Equal([]string{"payments/web"}))
		Expect(second.NextCursor).To(BeEmpty())
	})

	It("never asks the cache for more than one item beyond the page", func() {
		filters := inventoryFilters()
		filters.Limit = 2
		_, err := reader.List(ctx, filters)
		Expect(err).ToNot(HaveOccurred())
		Expect(store.limits[0]).To(Equal(3), "one extra identity is how the reader knows more remain")
	})
})

var _ = Describe("UT-FMC-INV-2661-002 [BR-SAFETY-001]: empty requires the partition proof", func() {
	It("serves a proven-empty partition as empty and fresh", func() {
		store := newFakeInventoryStore()
		store.state = scopecache.InventoryReadEmpty
		reader, _ := newInventoryReaderFixture(store, fmc.InventoryReaderConfig{})
		page, err := reader.List(context.Background(), inventoryFilters())
		Expect(err).ToNot(HaveOccurred())
		Expect(page.Validate()).To(Succeed())
		Expect(page.State).To(Equal(inventory.StateEmpty))
		Expect(page.Freshness.State).To(Equal(inventory.FreshnessFresh))
		Expect(page.Items).To(BeEmpty())
		Expect(page.NextCursor).To(BeEmpty())
	})

	It("fails closed when the lexicographic index is missing without an empty proof", func() {
		store := newFakeInventoryStore()
		store.state = scopecache.InventoryReadMissing
		store.reason = reasonNoLexIndex
		reader, _ := newInventoryReaderFixture(store, fmc.InventoryReaderConfig{})
		page, err := reader.List(context.Background(), inventoryFilters())
		Expect(err).To(MatchError(ContainSubstring("inventory unavailable")))
		Expect(page.State).To(Equal(inventory.StateUnavailable), "a missing index is never evidence of an empty fleet")
		Expect(page.Items).To(BeEmpty())
		Expect(page.NextCursor).To(BeEmpty())
		Expect(page.Validate()).To(Succeed())
	})
})

var _ = Describe("UT-FMC-INV-2661-003 [BR-SAFETY-001]: freshness follows remaining proof life", func() {
	It("keeps the threshold strictly between the sync interval and the cache TTL", func() {
		// DD-FLEET-017 D3.2a / §9.4: outside this window `stale` is unreachable
		// and a single missed cycle jumps from fresh to unavailable.
		Expect(fmc.DefaultInventoryStaleAfter).To(BeNumerically(">", inventorySyncInterval))
		Expect(fmc.DefaultInventoryStaleAfter).To(BeNumerically("<", inventoryCacheTTL))
	})

	It("serves fresh, then stale, then unavailable as the proof ages", func() {
		store := newFakeInventoryStore()
		store.put("payments", "Deployment", "apps", "v1", "api", "true")
		store.setNamespace("payments", "absent")
		reader, _ := newInventoryReaderFixture(store, fmc.InventoryReaderConfig{StaleAfter: inventoryStaleAfter})

		store.remaining = inventoryStaleAfter
		page, err := reader.List(context.Background(), inventoryFilters())
		Expect(err).ToNot(HaveOccurred())
		Expect(page.State).To(Equal(inventory.StateSuccess))
		Expect(page.Freshness.State).To(Equal(inventory.FreshnessFresh))

		store.remaining = inventoryStaleAfter - time.Second
		page, err = reader.List(context.Background(), inventoryFilters())
		Expect(err).ToNot(HaveOccurred())
		Expect(page.Validate()).To(Succeed())
		Expect(page.State).To(Equal(inventory.StateSuccess), "a valid proof still carries rows while it ages")
		Expect(page.Freshness.State).To(Equal(inventory.FreshnessStale))
		Expect(page.Items).To(HaveLen(1))

		store.state = scopecache.InventoryReadMissing
		store.reason = "stale_proof"
		page, err = reader.List(context.Background(), inventoryFilters())
		Expect(err).To(HaveOccurred())
		Expect(page.State).To(Equal(inventory.StateUnavailable))
		Expect(page.State).ToNot(Equal(inventory.StatePartial), "per-kind state does not exist, so a failed kind is never partial")
		Expect(page.Items).To(BeEmpty())
		Expect(page.NextCursor).To(BeEmpty())
	})

	It("honours a test override of the threshold", func() {
		store := newFakeInventoryStore()
		store.put("payments", "Deployment", "apps", "v1", "api", "true")
		store.setNamespace("payments", "absent")
		store.remaining = 20 * time.Second
		reader, _ := newInventoryReaderFixture(store, fmc.InventoryReaderConfig{StaleAfter: 10 * time.Second})
		page, err := reader.List(context.Background(), inventoryFilters())
		Expect(err).ToNot(HaveOccurred())
		Expect(page.Freshness.State).To(Equal(inventory.FreshnessFresh))
	})
})

var _ = Describe("UT-FMC-INV-2661-004 [BR-SAFETY-001]: unproven partitions are unavailable", func() {
	It("fails closed for an unknown cluster, an unknown partition and an unproven proof", func() {
		store := newFakeInventoryStore()
		store.put("payments", "Deployment", "apps", "v1", "api", "true")
		reader, registration := newInventoryReaderFixture(store, fmc.InventoryReaderConfig{})

		unknownCluster := inventoryFilters()
		unknownCluster.ClusterID = "prod-west"
		page, err := reader.List(context.Background(), unknownCluster)
		Expect(err).To(MatchError(ContainSubstring("inventory unavailable")))
		Expect(page.State).To(Equal(inventory.StateUnavailable))
		Expect(page.Items).To(BeEmpty())

		registration.current.RegistrationUID = "replacement-registration"
		page, err = reader.List(context.Background(), inventoryFilters())
		Expect(err).To(MatchError(ContainSubstring("inventory unavailable")))
		Expect(page.State).To(Equal(inventory.StateUnavailable),
			"a registration replacement must never reuse the old partition proof")
	})

	It("fails closed when the store reports a missing committed proof", func() {
		for _, reason := range []string{"no_status", "not_clean", "stale_proof", "status_ttl", "index_ttl"} {
			store := newFakeInventoryStore()
			store.state = scopecache.InventoryReadMissing
			store.reason = reason
			reader, _ := newInventoryReaderFixture(store, fmc.InventoryReaderConfig{})
			page, err := reader.List(context.Background(), inventoryFilters())
			Expect(err).To(HaveOccurred(), reason)
			Expect(page.State).To(Equal(inventory.StateUnavailable), reason)
			Expect(page.Items).To(BeEmpty(), reason)
			Expect(page.NextCursor).To(BeEmpty(), reason)
		}
	})

	It("propagates a cache transport failure without inventing a page", func() {
		store := newFakeInventoryStore()
		store.headerErr = errors.New("valkey unavailable")
		reader, _ := newInventoryReaderFixture(store, fmc.InventoryReaderConfig{})
		page, err := reader.List(context.Background(), inventoryFilters())
		Expect(err).To(HaveOccurred())
		Expect(page.State).To(Equal(inventory.StateUnavailable))
		Expect(page.Items).To(BeEmpty())
	})
})

var _ = Describe("UT-FMC-INV-2661-005 [BR-API-001]: cursor resumption and drift", func() {
	var (
		store  *fakeInventoryStore
		reader *fmc.InventoryReader
	)

	BeforeEach(func() {
		store = newFakeInventoryStore()
		store.setNamespace("payments", "absent")
		store.put("payments", "Deployment", "apps", "v1", "alpha", "true")
		store.put("payments", "Deployment", "apps", "v1", "bravo", "true")
		store.put("payments", "Deployment", "apps", "v1", "charlie", "true")
		reader, _ = newInventoryReaderFixture(store, fmc.InventoryReaderConfig{})
	})

	It("resumes strictly after the cursor", func() {
		filters := inventoryFilters()
		filters.Limit = 1
		first, err := reader.List(context.Background(), filters)
		Expect(err).ToNot(HaveOccurred())
		Expect(first.Items).To(HaveLen(1))
		Expect(first.Items[0].Name).To(Equal("alpha"))

		next := inventoryFilters()
		next.Limit = 1
		next.Cursor = first.NextCursor
		second, err := reader.List(context.Background(), next)
		Expect(err).ToNot(HaveOccurred())
		Expect(second.Items).To(HaveLen(1))
		Expect(second.Items[0].Name).To(Equal("bravo"))
	})

	It("re-runs the projection from the start when the cursor's identity is gone", func() {
		filters := inventoryFilters()
		filters.Limit = 1
		filters.Cursor = "r-" + strings.Repeat("d", 64)
		page, err := reader.List(context.Background(), filters)
		Expect(err).ToNot(HaveOccurred(), "cache drift is ordinary churn, never caller error")
		Expect(page.Validate()).To(Succeed())
		Expect(page.State).To(Equal(inventory.StateSuccess))
		Expect(page.Items).To(HaveLen(1))
		Expect(page.Items[0].Name).To(Equal("alpha"), "the projection restarts from the first member")
	})
})

var _ = Describe("UT-FMC-INV-2661-006 [BR-API-001]: malformed input is the only caller error", func() {
	It("rejects a malformed cursor and a widening limit as an invalid filter", func() {
		store := newFakeInventoryStore()
		reader, _ := newInventoryReaderFixture(store, fmc.InventoryReaderConfig{})

		badCursor := inventoryFilters()
		badCursor.Cursor = "../page"
		_, err := reader.List(context.Background(), badCursor)
		Expect(errors.Is(err, inventory.ErrInvalidFilter)).To(BeTrue())

		wide := inventoryFilters()
		wide.Limit = inventory.MaxLimit + 1
		_, err = reader.List(context.Background(), wide)
		Expect(errors.Is(err, inventory.ErrInvalidFilter)).To(BeTrue())

		missingCluster := inventory.Filters{}
		_, err = reader.List(context.Background(), missingCluster)
		Expect(errors.Is(err, inventory.ErrInvalidFilter)).To(BeTrue())
		Expect(store.headerCalls).To(Equal(0), "a rejected filter must never reach the cache")
	})
})

var _ = Describe("UT-FMC-INV-2661-008 [BR-API-001]: exact reads", func() {
	var (
		store      *fakeInventoryStore
		reader     *fmc.InventoryReader
		resourceID string
	)

	BeforeEach(func() {
		store = newFakeInventoryStore()
		store.setNamespace("payments", "true")
		resourceID = store.put("payments", "Deployment", "apps", "v1", "api", "absent")
		reader, _ = newInventoryReaderFixture(store, fmc.InventoryReaderConfig{})
	})

	It("returns the canonical resource with its inherited scope", func() {
		resource, err := reader.Get(context.Background(), inventoryClusterID, resourceID)
		Expect(err).ToNot(HaveOccurred())
		Expect(resource.Validate()).To(Succeed())
		Expect(resource.ID).To(Equal(resourceID))
		Expect(resource.Scope.Included).To(BeTrue())
		Expect(resource.Scope.Source).To(Equal(inventory.ScopeSourceNamespaceLabel))
	})

	It("reports an absent or cross-cluster identity as not found", func() {
		_, err := reader.Get(context.Background(), inventoryClusterID, "r-"+strings.Repeat("e", 64))
		Expect(errors.Is(err, inventory.ErrNotFound)).To(BeTrue())

		otherClusterID := inventory.CanonicalResourceID(scope.ResourceIdentity{
			ClusterID: "prod-west", Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "payments", Name: "api",
		})
		_, err = reader.Get(context.Background(), inventoryClusterID, otherClusterID)
		Expect(errors.Is(err, inventory.ErrNotFound)).To(BeTrue(), "a cross-cluster ID is absent from this partition, not an outage")
	})

	It("reports an identity mismatch as unavailable", func() {
		mismatched := scopecache.InventoryResourceRecord{
			State: scopecache.InventoryReadOK,
			Record: scopecache.InventoryRecord{
				Identity: scopecache.ResourceIdentity{
					ClusterID: inventoryClusterID, Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "payments", Name: "other",
				},
				LabelState: scope.LabelStateTrue, Generation: inventoryGeneration,
			},
			Generation: inventoryGeneration,
		}
		store.exactRecord = &mismatched
		_, err := reader.Get(context.Background(), inventoryClusterID, resourceID)
		Expect(err).To(MatchError(ContainSubstring("inventory unavailable")))
	})

	It("treats a resource outside the managed projection as not found", func() {
		unmanagedID := store.put("payments", "Deployment", "apps", "v1", "ignored", "false")
		_, err := reader.Get(context.Background(), inventoryClusterID, unmanagedID)
		Expect(errors.Is(err, inventory.ErrNotFound)).To(BeTrue())
	})

	It("rejects a malformed resource ID without reading the cache", func() {
		_, err := reader.Get(context.Background(), inventoryClusterID, "opaque-id")
		Expect(errors.Is(err, inventory.ErrInvalidFilter)).To(BeTrue())
		Expect(store.recordsCalls).To(Equal(0))
	})
})

var _ = Describe("UT-FMC-INV-2661-008B [BR-API-001]: exact reads derive freshness from the proof", func() {
	var (
		store      *fakeInventoryStore
		reader     *fmc.InventoryReader
		resourceID string
	)

	BeforeEach(func() {
		store = newFakeInventoryStore()
		store.setNamespace("payments", "true")
		resourceID = store.put("payments", "Deployment", "apps", "v1", "api", "absent")
		reader, _ = newInventoryReaderFixture(store, fmc.InventoryReaderConfig{StaleAfter: inventoryStaleAfter})
	})

	exactPage := func() inventory.Page {
		filters := inventory.Filters{ClusterID: inventoryClusterID, ResourceID: resourceID}
		page, err := reader.List(context.Background(), filters)
		ExpectWithOffset(1, err).ToNot(HaveOccurred())
		ExpectWithOffset(1, page.Validate()).To(Succeed())
		return page
	}

	It("claims fresh only while the proof is inside the budget", func() {
		store.remaining = inventoryProofRemained
		page := exactPage()
		Expect(page.State).To(Equal(inventory.StateSuccess))
		Expect(page.Items).To(HaveLen(1))
		Expect(page.Freshness.State).To(Equal(inventory.FreshnessFresh))
		Expect(page.Freshness.AsOf).ToNot(BeZero())
	})

	It("reports stale, not fresh, on a nearly-expired proof", func() {
		store.remaining = inventoryStaleAfter - time.Second
		page := exactPage()
		Expect(page.State).To(Equal(inventory.StateSuccess), "a valid proof still carries the row while it ages")
		Expect(page.Items).To(HaveLen(1))
		Expect(page.Freshness.State).To(Equal(inventory.FreshnessStale))
		Expect(page.Freshness.State).ToNot(Equal(inventory.FreshnessFresh),
			"an exact read must not claim fresher than a list read of the same partition")
	})

	It("reports an absent identity under the same ageing proof as empty and stale", func() {
		store.remaining = inventoryStaleAfter - time.Second
		missingID := "r-" + strings.Repeat("9", 64)
		page, err := reader.List(context.Background(), inventory.Filters{ClusterID: inventoryClusterID, ResourceID: missingID})
		Expect(err).ToNot(HaveOccurred())
		Expect(page.Validate()).To(Succeed())
		Expect(page.State).To(Equal(inventory.StateEmpty))
		Expect(page.Freshness.State).To(Equal(inventory.FreshnessStale))
	})

	It("reports an unproven partition as unavailable instead of not found", func() {
		store.unproven = true
		page, err := reader.List(context.Background(), inventory.Filters{ClusterID: inventoryClusterID, ResourceID: resourceID})
		Expect(err).To(MatchError(ContainSubstring("inventory unavailable")))
		Expect(page.State).To(Equal(inventory.StateUnavailable))
		Expect(page.Items).To(BeEmpty())
		Expect(page.NextCursor).To(BeEmpty())

		_, err = reader.Get(context.Background(), inventoryClusterID, resourceID)
		Expect(errors.Is(err, inventory.ErrUnavailable)).To(BeTrue(),
			"a missing proof is an outage, never an assertion that the resource does not exist")
		Expect(errors.Is(err, inventory.ErrNotFound)).To(BeFalse())
	})
})

var _ = Describe("UT-FMC-INV-2661-009 [BR-API-001]: bounds and truncation", func() {
	It("reports a projection that cannot be completed within its scan budget as partial", func() {
		store := newFakeInventoryStore()
		for index := 0; index < 12; index++ {
			store.put("payments", "Deployment", "apps", "v1", "unmanaged-"+string(rune('a'+index)), "false")
		}
		store.put("payments", "Deployment", "apps", "v1", "managed", "true")
		store.setNamespace("payments", "absent")
		reader, _ := newInventoryReaderFixture(store, fmc.InventoryReaderConfig{ScanBudget: 2})

		filters := inventoryFilters()
		filters.Limit = 1
		page, err := reader.List(context.Background(), filters)
		Expect(err).To(HaveOccurred())
		Expect(page.State).To(Equal(inventory.StatePartial))
		Expect(page.Items).To(BeEmpty(), "a truncated projection carries no rows")
		Expect(page.NextCursor).To(BeEmpty())
		Expect(page.Validate()).To(Succeed())
	})

	It("keeps a partition above the proof ceiling unavailable", func() {
		// The commit path bounds the observation set (5000 items / 100 pages) and
		// the staged index (10 000 members), so an oversized cluster can never
		// publish a clean generation and must read unavailable.
		store := newFakeInventoryStore()
		store.state = scopecache.InventoryReadMissing
		store.reason = "no_status"
		reader, _ := newInventoryReaderFixture(store, fmc.InventoryReaderConfig{})
		page, err := reader.List(context.Background(), inventoryFilters())
		Expect(err).To(HaveOccurred())
		Expect(page.State).To(Equal(inventory.StateUnavailable))
	})
})

var _ = Describe("UT-FMC-INV-2661-010 [BR-SECURITY-017]: metadata-only payload", func() {
	It("serializes only the sanitized contract", func() {
		store := newFakeInventoryStore()
		store.setNamespace("payments", "true")
		store.put("payments", "Deployment", "apps", "v1", "api", "absent")
		reader, _ := newInventoryReaderFixture(store, fmc.InventoryReaderConfig{})
		page, err := reader.List(context.Background(), inventory.Filters{ClusterID: inventoryClusterID})
		Expect(err).ToNot(HaveOccurred())

		encoded, err := json.Marshal(page)
		Expect(err).ToNot(HaveOccurred())
		payload := string(encoded)
		for _, forbidden := range []string{"kubernaut.ai", "annotations", "labels\"", "_uid", "data\"", "secret", "token"} {
			Expect(payload).ToNot(ContainSubstring(forbidden), forbidden)
		}

		var decoded map[string]json.RawMessage
		Expect(json.Unmarshal(encoded, &decoded)).To(Succeed())
		Expect(decoded).To(HaveKey("state"))
		Expect(decoded).To(HaveKey("freshness"))
		Expect(decoded).To(HaveKey("completeness"))
		Expect(decoded).To(HaveKey("observed_at"))

		var item map[string]json.RawMessage
		Expect(json.Unmarshal(encoded, &struct {
			Items []json.RawMessage `json:"items"`
		}{})).To(Succeed())
		Expect(page.Items).To(HaveLen(1))
		itemBytes, err := json.Marshal(page.Items[0])
		Expect(err).ToNot(HaveOccurred())
		Expect(json.Unmarshal(itemBytes, &item)).To(Succeed())
		Expect(item).To(HaveLen(8), "the resource contract is exactly id, cluster, group, version, kind, namespace, name, scope")
	})
})

var _ = Describe("UT-FMC-INV-2661-011 [BR-SCOPE-001]: managed projection", func() {
	It("returns exactly the Included set resolved by the shared precedence", func() {
		store := newFakeInventoryStore()
		store.setNamespace("inherits", "true")
		store.setNamespace("overridden", "true")
		store.setNamespace("plain", "absent")

		inherited := store.put("inherits", "Deployment", "apps", "v1", "web", "absent")
		overridden := store.put("overridden", "Deployment", "apps", "v1", "web", "false")
		absent := store.put("plain", "Deployment", "apps", "v1", "web", "absent")
		explicit := store.put("plain", "Deployment", "apps", "v1", "api", "true")
		invalid := store.put("plain", "Deployment", "apps", "v1", "broken", "invalid")

		reader, _ := newInventoryReaderFixture(store, fmc.InventoryReaderConfig{})
		page, err := reader.List(context.Background(), inventoryFilters())
		Expect(err).ToNot(HaveOccurred())
		Expect(page.Validate()).To(Succeed())

		ids := make([]string, 0, len(page.Items))
		for _, item := range page.Items {
			ids = append(ids, item.ID)
		}
		Expect(ids).To(ConsistOf(inherited, explicit))
		Expect(ids).ToNot(ContainElement(overridden), "an explicit false overrides a managed namespace")
		Expect(ids).ToNot(ContainElement(absent), "an absent resource in an absent namespace is not managed")
		Expect(ids).ToNot(ContainElement(invalid), "an invalid label is invalid, never managed")
	})

	It("serves an explicitly labelled resource without needing its namespace record", func() {
		// ADR-053: an explicit resource label settles the decision, so an
		// absent namespace record cannot make the page unavailable.
		store := newFakeInventoryStore()
		explicit := store.put("orphan", "Deployment", "apps", "v1", "web", "true")
		refused := store.put("orphan", "Deployment", "apps", "v1", "api", "false")
		reader, _ := newInventoryReaderFixture(store, fmc.InventoryReaderConfig{})
		page, err := reader.List(context.Background(), inventoryFilters())
		Expect(err).ToNot(HaveOccurred())
		Expect(page.Validate()).To(Succeed())
		Expect(namesOfResources(page.Items)).To(Equal([]string{"orphan/web"}))
		Expect(page.Items[0].ID).To(Equal(explicit))
		Expect(page.Items[0].Scope.Source).To(Equal(inventory.ScopeSourceResourceLabel))
		for _, item := range page.Items {
			Expect(item.ID).ToNot(Equal(refused))
		}
	})

	It("fails closed when the namespace record needed for inheritance is not observable", func() {
		store := newFakeInventoryStore()
		store.put("orphan", "Deployment", "apps", "v1", "web", "absent")
		reader, _ := newInventoryReaderFixture(store, fmc.InventoryReaderConfig{})
		page, err := reader.List(context.Background(), inventoryFilters())
		Expect(err).To(MatchError(ContainSubstring("inventory unavailable")))
		Expect(page.State).To(Equal(inventory.StateUnavailable),
			"an unproven inherited label must never be published as unmanaged")
		Expect(page.Items).To(BeEmpty())
	})

	It("never inherits for cluster-scoped kinds: an absent label stays excluded without a namespace lookup", func() {
		// ADR-053 / DD-FLEET-017 §3.7: cluster-scoped objects never inherit.
		// The reader must skip the namespace record entirely — an unlabeled
		// cluster-scoped item is simply excluded, not resolved through (or
		// failed by) a namespace it does not have.
		store := newFakeInventoryStore()
		unlabeled := store.put("", "Node", "", "v1", "worker", "absent")
		labelled := store.put("", "Node", "", "v1", "control", "true")
		reader, _ := newInventoryReaderFixture(store, fmc.InventoryReaderConfig{})
		page, err := reader.List(context.Background(), inventoryFilters())
		Expect(err).ToNot(HaveOccurred(),
			"the cluster-scoped path must not consult a namespace record; consulting one would fail closed on the missing lookup")
		Expect(page.Validate()).To(Succeed())

		ids := make([]string, 0, len(page.Items))
		for _, item := range page.Items {
			ids = append(ids, item.ID)
		}
		Expect(ids).To(ConsistOf(labelled), "only the explicitly labelled cluster-scoped item is managed")
		Expect(ids).ToNot(ContainElement(unlabeled), "cluster-scoped never inherits: absent stays excluded")
		Expect(page.Items[0].Scope.ClusterScoped).To(BeTrue())
	})
})

var _ = Describe("UT-FMC-INV-2661-012/013 [BR-API-001]: lexicographic ordering and prefix filters", func() {
	It("orders namespace-first and deterministically, not like SortResources", func() {
		store := newFakeInventoryStore()
		store.setNamespace("ns-b", "true")
		store.setNamespace("ns-a", "true")
		store.put("ns-b", "ConfigMap", "", "v1", "aaa", "true")
		store.put("ns-a", "StatefulSet", "apps", "v1", "zzz", "true")
		store.put("ns-a", "Deployment", "apps", "v1", "mmm", "true")
		store.put("ns-b", "Deployment", "apps", "v1", "bbb", "true")

		reader, _ := newInventoryReaderFixture(store, fmc.InventoryReaderConfig{})
		page, err := reader.List(context.Background(), inventoryFilters())
		Expect(err).ToNot(HaveOccurred())
		names := make([]string, 0, len(page.Items))
		for _, item := range page.Items {
			names = append(names, item.Namespace+"/"+item.Kind+"/"+item.Name)
		}
		Expect(names).To(Equal([]string{
			"ns-a/Deployment/mmm", "ns-a/StatefulSet/zzz", "ns-b/ConfigMap/aaa", "ns-b/Deployment/bbb",
		}), "namespace, then kind, then name: determinism is the contract")

		again, err := reader.List(context.Background(), inventoryFilters())
		Expect(err).ToNot(HaveOccurred())
		Expect(namesOfResources(again.Items)).To(Equal(namesOfResources(page.Items)),
			"the same generation must page identically")
	})

	It("serves namespace, namespace+kind and namespace+kind+name ranges", func() {
		store := newFakeInventoryStore()
		for _, namespace := range []string{"ns-a", "ns-b"} {
			store.setNamespace(namespace, "true")
			store.put(namespace, "Deployment", "apps", "v1", "api", "true")
			store.put(namespace, "Deployment", "apps", "v1", "web", "true")
			store.put(namespace, "StatefulSet", "apps", "v1beta1", "db", "true")
		}
		reader, _ := newInventoryReaderFixture(store, fmc.InventoryReaderConfig{})

		namesIn := func(filters inventory.Filters) []string {
			filters.ClusterID = inventoryClusterID
			page, err := reader.List(context.Background(), filters)
			ExpectWithOffset(1, err).ToNot(HaveOccurred())
			names := make([]string, 0, len(page.Items))
			for _, item := range page.Items {
				names = append(names, item.Namespace+"/"+item.Name)
			}
			return names
		}

		Expect(namesIn(inventory.Filters{Namespace: "ns-a"})).To(Equal([]string{"ns-a/api", "ns-a/web", "ns-a/db"}))
		Expect(namesIn(inventory.Filters{Namespace: "ns-a", Kind: "Deployment"})).To(Equal([]string{"ns-a/api", "ns-a/web"}))
		Expect(namesIn(inventory.Filters{Namespace: "ns-a", Kind: "Deployment", Group: "apps", Version: "v1", Name: "web"})).
			To(Equal([]string{"ns-a/web"}))

		// A kind-only query has no namespace to anchor the range, so the reader
		// scans the partition and filters (DD-FLEET-017 D2.5a).
		Expect(namesIn(inventory.Filters{Kind: "StatefulSet"})).To(Equal([]string{"ns-a/db", "ns-b/db"}))
		Expect(namesIn(inventory.Filters{Name: "api", Limit: 1})).To(Equal([]string{"ns-a/api"}))
	})
})

var _ = Describe("UT-FMC-INV-2661-014 [BR-SAFETY-001]: a missing lexicographic index is recoverable", func() {
	It("reads unavailable, then serves the page after the next clean commit", func() {
		store := newFakeInventoryStore()
		store.setNamespace("payments", "absent")
		store.put("payments", "Deployment", "apps", "v1", "api", "true")
		reader, _ := newInventoryReaderFixture(store, fmc.InventoryReaderConfig{})

		page, err := reader.List(context.Background(), inventoryFilters())
		Expect(err).ToNot(HaveOccurred())
		Expect(page.Items).To(HaveLen(1))

		store.state = scopecache.InventoryReadMissing
		store.reason = reasonNoLexIndex
		page, err = reader.List(context.Background(), inventoryFilters())
		Expect(err).To(HaveOccurred())
		Expect(page.State).To(Equal(inventory.StateUnavailable))
		Expect(page.State).ToNot(Equal(inventory.StateEmpty))

		store.state = scopecache.InventoryReadOK
		store.reason = ""
		page, err = reader.List(context.Background(), inventoryFilters())
		Expect(err).ToNot(HaveOccurred())
		Expect(page.Items).To(HaveLen(1))
	})
})

// UT-FMC-INV-2661-015 [BR-SAFETY-001]: DD-FLEET-017 D2.5b.
// A page is read under one proof across two calls (header, then records). A
// commit landing between them supersedes the proof; the reader re-resolves once
// so an ordinary rotation is invisible to the caller, and still fails closed if
// the second attempt cannot prove the page.
var _ = Describe("UT-FMC-INV-2661-015: bounded proof re-resolve", func() {
	It("retries once when a commit supersedes the proof mid-read", func() {
		store := newFakeInventoryStore()
		store.put("ns", "Deployment", "apps", "v1", "api", "true")
		store.staleOnce = 1
		reader, _ := newInventoryReaderFixture(store, fmc.InventoryReaderConfig{})

		page, err := reader.List(context.Background(), inventoryFilters())

		Expect(err).ToNot(HaveOccurred())
		Expect(page.State).To(Equal(inventory.StateSuccess))
		Expect(page.Validate()).To(Succeed())
		Expect(store.headerCalls).To(Equal(2), "one read plus exactly one retry")
	})

	It("fails closed once the single retry is spent", func() {
		store := newFakeInventoryStore()
		store.put("ns", "Deployment", "apps", "v1", "api", "true")
		store.staleOnce = 5
		reader, _ := newInventoryReaderFixture(store, fmc.InventoryReaderConfig{})

		page, err := reader.List(context.Background(), inventoryFilters())

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("inventory unavailable")))
		Expect(page.State).To(Equal(inventory.StateUnavailable))
		Expect(page.Items).To(BeEmpty())
		Expect(store.headerCalls).To(Equal(2), "the retry is bounded, never a loop")
	})
})
