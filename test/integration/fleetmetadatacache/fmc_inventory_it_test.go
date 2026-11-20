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

package fleetmetadatacache_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// DD-FLEET-017 §8 / TEST_PLAN.md §10 wiring row 2: the cache-backed page and
// cursor over the proof gate, served through the production FMC route against a
// real Valkey. Kubernetes-derived state (registration + readiness) is seeded
// through the production types; the cache, the atomic script, the page reader
// and the HTTP surface are all real.
//
// Proves: proven paging with a cursor and no overlap; a staged repair serves the
// previous proven generation with no staging value on the wire; an unprovable
// partition is unavailable state-only; a proven-empty partition is empty+complete
// while a missing lexicographic index without that proof is unavailable.

// itInventoryRegistration is a registration guard over a fixed cluster set. The
// registry CRDs are exercised by the EAIGW registry ITs; this IT isolates the
// cache-read wiring (DD-FLEET-017 wiring row 2).
type itInventoryRegistration struct {
	clusters map[string]registry.ClusterInfo
	revision uint64
}

var _ registry.CurrentRegistrationGuard = (*itInventoryRegistration)(nil)

func (g *itInventoryRegistration) Snapshot(clusterID string) (registry.RegistrationSnapshot, bool) {
	info, found := g.clusters[clusterID]
	if !found {
		return registry.RegistrationSnapshot{}, false
	}
	return registry.RegistrationSnapshot{Info: info, Revision: g.revision}, true
}

func (g *itInventoryRegistration) WithCurrent(_ context.Context, clusterID, registrationUID string, expectedRevision uint64, callback func(registry.ClusterInfo) error) error {
	info, found := g.clusters[clusterID]
	if !found || info.RegistrationUID != registrationUID || expectedRevision != g.revision {
		return context.Canceled
	}
	return callback(info)
}

type itInventoryFixture struct {
	ctx          context.Context
	store        *scopecache.ValkeyStatusStore
	admin        *redis.Client
	readiness    *scopecache.FMCReadinessSnapshot
	registration *itInventoryRegistration
	reader       *fmc.InventoryReader
	client       *fmc.HTTPClient
	server       *httptest.Server
	fence        scopecache.WriterFence
	lease        scopecache.LeaseView
}

const itInventoryRevision = uint64(1)

// itInventoryVersion is the apiVersion every fixture observation is stored
// under. The projection tests exercise ordering and freshness, not version
// variation, so the version stays fixed.
const itInventoryVersion = "v1"

func newITInventoryFixture(ctx context.Context) *itInventoryFixture {
	store, err := scopecache.NewValkeyStatusStoreAt(valkeyAddr)
	Expect(err).ToNot(HaveOccurred())
	Expect(store.LoadScript(ctx)).To(Succeed())

	admin := redis.NewClient(&redis.Options{Addr: valkeyAddr})
	fence := scopecache.WriterFence{HolderPodUID: "it-inventory-pod", HolderSessionUID: "it-inventory-session", LeaseEpoch: 3, Fence: 3}
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
	Expect(err).ToNot(HaveOccurred())
	Expect(readiness.PublishGlobal(scopecache.FMCGlobalReadiness{
		MCPInitialSync:       true,
		RegistryInitialSync:  true,
		RegistryWatchHealthy: true,
		ScriptReady:          true,
		ValkeyReady:          true,
		Lease:                lease,
		ActiveLeaseValid:     true,
		LeasePolicy:          scopecache.LeasePolicyProcessWideRequired,
	})).To(Succeed())
	Expect(readiness.PublishWriterLease(scopecache.WriterFence{}, lease)).To(Succeed())

	registration := &itInventoryRegistration{clusters: map[string]registry.ClusterInfo{}, revision: itInventoryRevision}
	fixture := &itInventoryFixture{
		ctx: ctx, store: store, admin: admin, readiness: readiness,
		registration: registration, fence: fence, lease: lease,
	}
	reader, err := fmc.NewInventoryReader(store, registration, readiness, fmc.InventoryReaderConfig{})
	Expect(err).ToNot(HaveOccurred())
	fixture.reader = reader

	mux := http.NewServeMux()
	fmc.NewHandlerWithInventory(&localAlwaysFalse{}, &fakeEmptyRegistry{}, reader, logr.Discard()).RegisterRoutes(mux)
	fixture.server = httptest.NewServer(mux)
	fixture.client = fmc.NewHTTPClient(fixture.server.URL)
	return fixture
}

func (f *itInventoryFixture) close() {
	if f.server != nil {
		f.server.Close()
	}
	if f.admin != nil {
		_ = f.admin.Close()
	}
	if f.store != nil {
		_ = f.store.Close()
	}
}

// partition registers one cluster and publishes its membership revision so the
// reader can resolve a partition for it.
func (f *itInventoryFixture) partition(clusterID string) scopecache.PartitionStateKey {
	registrationUID := clusterID + "-registration"
	f.registration.clusters[clusterID] = registry.ClusterInfo{ID: clusterID, RegistrationUID: registrationUID}
	registrations := make([]registry.RegistrationSnapshot, 0, len(f.registration.clusters))
	for _, info := range f.registration.clusters {
		registrations = append(registrations, registry.RegistrationSnapshot{Info: info, Revision: itInventoryRevision})
	}
	Expect(f.readiness.ReconcileRegistry(registry.RegistrySnapshot{
		Generation: itInventoryRevision, Registrations: registrations,
	})).To(Succeed())
	return scopecache.PartitionStateKey{ClusterID: clusterID, RegistrationUID: registrationUID}
}

func itObservation(partition scopecache.PartitionStateKey, namespace, kind, group, name, label string) scopecache.RepairObservation {
	apiVersion := itInventoryVersion
	if group != "" {
		apiVersion = group + "/" + itInventoryVersion
	}
	return scopecache.RepairObservation{
		Identity: scopecache.ResourceIdentity{
			ClusterID: partition.ClusterID, Group: group, Version: itInventoryVersion, Kind: kind, Namespace: namespace, Name: name,
		},
		APIVersion:    apiVersion,
		LabelsPresent: true,
		LabelState:    scope.LabelState(label),
		TTL:           30 * time.Second,
	}
}

// begin starts a staged generation and publishes the repair transition exactly
// as the production syncer does (store begin, then readiness begin).
func (f *itInventoryFixture) begin(partition scopecache.PartitionStateKey) scopecache.RepairStartResult {
	start, err := f.store.BeginRepair(f.ctx, scopecache.BeginRepairRequest{
		Partition: partition, RegistrationUID: partition.RegistrationUID,
		ExpectedRegistryRevision: itInventoryRevision, WriterFence: f.fence,
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(f.readiness.BeginPartitionRepair(partition, partition.RegistrationUID, itInventoryRevision, f.lease, start.Generation)).To(Succeed())
	return start
}

func (f *itInventoryFixture) repair(start scopecache.RepairStartResult, observations []scopecache.RepairObservation) scopecache.RepairResult {
	repair, err := f.store.Repair(f.ctx, scopecache.RepairRequest{
		Partition: start.Partition, RegistrationUID: start.RegistrationUID,
		ExpectedRegistryRevision: itInventoryRevision, WriterFence: f.fence,
		Generation: start.Generation, Token: start.Token,
		Observations: scopecache.ValidatedObservationSet{Items: observations, Complete: true, PageCount: 1, ItemCount: len(observations)},
	})
	Expect(err).ToNot(HaveOccurred())
	return repair
}

func (f *itInventoryFixture) commit(start scopecache.RepairStartResult, observations []scopecache.RepairObservation) scopecache.RepairCommitResult {
	repair := f.repair(start, observations)
	commit, err := f.store.CommitRepair(f.ctx, scopecache.RepairCommitRequest{
		Partition: start.Partition, RegistrationUID: start.RegistrationUID,
		ExpectedRegistryRevision: itInventoryRevision, WriterFence: f.fence,
		Generation: start.Generation, Token: start.Token, ExpectedCount: repair.ExpectedCount, Proof: repair,
	})
	Expect(err).ToNot(HaveOccurred())
	partition := start.Partition
	Expect(f.readiness.PublishPartitionClean(partition, scopecache.PartitionReadiness{
		Key:                  partition,
		RegistrationUID:      partition.RegistrationUID,
		RegistrationRevision: itInventoryRevision,
		LeaseEpoch:           f.fence.LeaseEpoch,
		LeaseSessionUID:      f.fence.HolderSessionUID,
		WriterFence:          f.fence,
		LeaseHeartbeatAt:     f.lease.HeartbeatAt,
		LeaseHeartbeatValid:  true,
		Generation:           start.Generation,
		State:                scopecache.PartitionClean,
		CleanCommit:          true,
		RecordValid:          true,
		IndexValid:           true,
		StatusValid:          true,
		RecordTTLValid:       true,
		IndexTTLValid:        true,
		StatusTTLValid:       true,
	})).To(Succeed())
	return commit
}

// getPage performs the real HTTP read and returns the raw body alongside the
// decoded page so a spec can assert nothing leaked onto the wire.
func (f *itInventoryFixture) getPage(clusterID, cursor string, limit int) (inventory.Page, string) {
	query := "?cluster_id=" + clusterID
	if cursor != "" {
		query += "&cursor=" + cursor
	}
	if limit > 0 {
		query += "&limit=" + itoa(limit)
	}
	response, err := f.server.Client().Get(f.server.URL + fmc.InventoryPath + query)
	Expect(err).ToNot(HaveOccurred())
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	Expect(err).ToNot(HaveOccurred())
	Expect(response.StatusCode).To(Equal(http.StatusOK), string(body))
	var page inventory.Page
	Expect(json.Unmarshal(body, &page)).To(Succeed())
	Expect(page.Validate()).To(Succeed(), string(body))
	return page, string(body)
}

func (f *itInventoryFixture) getState(clusterID string) (inventory.Page, string, int) {
	response, err := f.server.Client().Get(f.server.URL + fmc.InventoryPath + "?cluster_id=" + clusterID)
	Expect(err).ToNot(HaveOccurred())
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	Expect(err).ToNot(HaveOccurred())
	var page inventory.Page
	Expect(json.Unmarshal(body, &page)).To(Succeed())
	return page, string(body), response.StatusCode
}

func (f *itInventoryFixture) liveKeys(pattern string) []string {
	keys, err := f.admin.Keys(f.ctx, pattern).Result()
	Expect(err).ToNot(HaveOccurred())
	return keys
}

// keysAdded returns the keys that appeared under pattern since `before`. The IT
// shares one Valkey with the other FMC specs, so partition keyspace is observed
// by difference rather than by a hardcoded partition hash.
func (f *itInventoryFixture) keysAdded(pattern string, before []string) []string {
	existing := make(map[string]struct{}, len(before))
	for _, key := range before {
		existing[key] = struct{}{}
	}
	added := make([]string, 0, 1)
	for _, key := range f.liveKeys(pattern) {
		if _, known := existing[key]; !known {
			added = append(added, key)
		}
	}
	return added
}

func itoa(value int) string {
	digits := []byte{}
	if value == 0 {
		return "0"
	}
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}

var _ = Describe("IT-FMC-INV-2661-002 [BR-API-001, BR-SAFETY-001]: cache-backed inventory page over the proof gate", Ordered, Label("fmc", "integration"), func() {
	var (
		ctx     context.Context
		cancel  context.CancelFunc
		fixture *itInventoryFixture
	)

	BeforeAll(func() {
		ctx, cancel = context.WithCancel(context.Background())
		fixture = newITInventoryFixture(ctx)
	})

	AfterAll(func() {
		if fixture != nil {
			fixture.close()
		}
		cancel()
	})

	It("pages a proven partition with a cursor and never repeats an identity", func() {
		partition := fixture.partition("it-inv-paging")
		start := fixture.begin(partition)
		fixture.commit(start, []scopecache.RepairObservation{
			itObservation(partition, "", "Namespace", "", "payments", "true"),
			itObservation(partition, "payments", "Deployment", "apps", "api", "true"),
			itObservation(partition, "payments", "Deployment", "apps", "web", "absent"),
		})

		first, body := fixture.getPage("it-inv-paging", "", 1)
		Expect(first.State).To(Equal(inventory.StateSuccess))
		Expect(first.Freshness.State).To(Equal(inventory.FreshnessFresh), "a fresh commit is inside the freshness budget")
		Expect(first.Completeness).To(Equal(inventory.CompletenessComplete))
		Expect(first.Items).To(HaveLen(1))
		Expect(first.NextCursor).To(Equal(first.Items[0].ID))
		Expect(first.Items[0].Scope.Included).To(BeTrue())
		Expect(body).ToNot(ContainSubstring("kubernaut.ai/managed"), "the payload carries no raw labels")

		second, _ := fixture.getPage("it-inv-paging", first.NextCursor, 1)
		Expect(second.State).To(Equal(inventory.StateSuccess))
		Expect(second.Items).To(HaveLen(1))
		Expect(second.Items[0].ID).ToNot(Equal(first.Items[0].ID), "a cursor page never repeats an identity")

		seen := []string{first.Items[0].ID, second.Items[0].ID}
		cursor := second.NextCursor
		for cursor != "" {
			next, _ := fixture.getPage("it-inv-paging", cursor, 1)
			Expect(next.Items).To(HaveLen(1))
			Expect(seen).ToNot(ContainElement(next.Items[0].ID), "paging must never overlap")
			seen = append(seen, next.Items[0].ID)
			cursor = next.NextCursor
		}
		// The Namespace carries the managed label, so it is itself a managed
		// resource (cluster-scoped, no inheritance); the unlabeled Deployment
		// inherits that label from its namespace. All three are included.
		Expect(seen).To(HaveLen(3))
	})

	It("serves the previous proven generation mid-repair and never leaks staged state", func() {
		partition := fixture.partition("it-inv-staged")
		start := fixture.begin(partition)
		fixture.commit(start, []scopecache.RepairObservation{
			itObservation(partition, "payments", "Deployment", "apps", "committed", "true"),
		})

		// A repair is in flight: the staging holds a distinctive identity that
		// must never reach a response.
		before := fixture.liveKeys("kubernaut:fmc:v2:*:status:staged")
		staged := fixture.begin(partition)
		fixture.repair(staged, []scopecache.RepairObservation{
			itObservation(partition, "payments", "Deployment", "apps", "staged-only-identity", "true"),
			itObservation(partition, "payments", "Deployment", "apps", "second-staged-identity", "true"),
		})
		Expect(fixture.keysAdded("kubernaut:fmc:v2:*:status:staged", before)).To(HaveLen(1),
			"the staging must really be present")

		page, body := fixture.getPage("it-inv-staged", "", 10)
		Expect(page.State).To(Equal(inventory.StateSuccess), "the committed generation stays readable during a staged repair")
		Expect(page.Items).To(HaveLen(1))
		Expect(page.Items[0].Name).To(Equal("committed"))

		Expect(body).ToNot(ContainSubstring("staged-only-identity"), "no staging value may ever appear on the wire")
		Expect(body).ToNot(ContainSubstring("second-staged-identity"))
		for _, item := range page.Items {
			Expect(item.Name).ToNot(HavePrefix("staged"))
		}

		_ = fixture.store.AbandonStaging(ctx, partition, partition.RegistrationUID, itInventoryRevision, fixture.fence)
	})

	It("is unavailable state-only when the committed proof is gone or invalid", func() {
		before := fixture.liveKeys("kubernaut:fmc:v2:*:status")
		partition := fixture.partition("it-inv-broken")
		start := fixture.begin(partition)
		fixture.commit(start, []scopecache.RepairObservation{
			itObservation(partition, "payments", "Deployment", "apps", "api", "true"),
		})

		// An expired proof leaves no committed status behind.
		statusKeys := fixture.keysAdded("kubernaut:fmc:v2:*:status", before)
		Expect(statusKeys).To(HaveLen(1))
		Expect(fixture.admin.Del(ctx, statusKeys[0]).Err()).ToNot(HaveOccurred())

		page, body, code := fixture.getState("it-inv-broken")
		Expect(code).To(Equal(http.StatusServiceUnavailable), "a gone proof is a 503, never a 200 empty page")
		Expect(page.State).To(Equal(inventory.StateUnavailable))
		Expect(page.Items).To(BeEmpty())
		Expect(page.NextCursor).To(BeEmpty())
		Expect(page.Completeness).To(Equal(inventory.CompletenessPartial))
		Expect(body).ToNot(ContainSubstring(`"items"`), "a state-only failure carries no records")
		Expect(body).ToNot(ContainSubstring(`"next_cursor"`))
		Expect(body).ToNot(ContainSubstring("payments"), "no identity of the unprovable generation may leak")
	})

	It("is empty and complete only for a proven-empty partition", func() {
		partition := fixture.partition("it-inv-empty")
		before := fixture.liveKeys("kubernaut:fmc:v2:*:index")
		start := fixture.begin(partition)
		fixture.commit(start, nil)

		Expect(fixture.keysAdded("kubernaut:fmc:v2:*:index", before)).To(BeEmpty(),
			"a proven-empty partition carries no indexes at all")

		page, _, code := fixture.getState("it-inv-empty")
		Expect(code).To(Equal(http.StatusOK))
		Expect(page.State).To(Equal(inventory.StateEmpty))
		Expect(page.Completeness).To(Equal(inventory.CompletenessComplete))
		Expect(page.Items).To(BeEmpty())
		Expect(page.NextCursor).To(BeEmpty())
		Expect(page.Freshness.State).To(Equal(inventory.FreshnessFresh))
	})

	It("is unavailable, never empty, when a populated partition loses its lexicographic index", func() {
		before := fixture.liveKeys("kubernaut:fmc:v2:*:index:lex")
		partition := fixture.partition("it-inv-nolex")
		start := fixture.begin(partition)
		fixture.commit(start, []scopecache.RepairObservation{
			itObservation(partition, "payments", "Deployment", "apps", "api", "true"),
		})

		lexKeys := fixture.keysAdded("kubernaut:fmc:v2:*:index:lex", before)
		Expect(lexKeys).To(HaveLen(1))
		Expect(fixture.admin.Del(ctx, lexKeys[0]).Err()).ToNot(HaveOccurred())

		page, body, code := fixture.getState("it-inv-nolex")
		Expect(code).To(Equal(http.StatusServiceUnavailable))
		Expect(page.State).To(Equal(inventory.StateUnavailable),
			"a missing index without the empty proof is never 'nothing managed'")
		Expect(page.Items).To(BeEmpty())
		Expect(page.NextCursor).To(BeEmpty())
		Expect(strings.Contains(body, "payments")).To(BeFalse(), "no identity may be served from an unpaginable generation")

		// The next clean commit restores the index and the page.
		next := fixture.begin(partition)
		fixture.commit(next, []scopecache.RepairObservation{
			itObservation(partition, "payments", "Deployment", "apps", "api", "true"),
		})
		recovered, _ := fixture.getPage("it-inv-nolex", "", 10)
		Expect(recovered.State).To(Equal(inventory.StateSuccess))
		Expect(recovered.Items).To(HaveLen(1))
	})
})
