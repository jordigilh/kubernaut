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

package main

// DD-FLEET-017 D4 / Test Plan TP-2661-v2.0 §8 Tier 2, slices b2-b4: the
// integration tier of the ACM Search source adapter. Every spec goes through
// production construction (buildFMCSyncer / buildACMSource / NewACMHTTPSearch
// over a config loaded by fmcconfig.LoadFromFile), never a re-implemented
// composition. External dependencies are substituted at their edges only:
// ACM Search over httptest TLS, Valkey over miniredis (this package's suite
// has no real Valkey; scopecache's own ITs established the miniredis pattern
// with pinned server time), the MCP Gateway over a minimal JSON-RPC fake, and
// the cluster registry over a registration-fenced fake.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	fmcconfig "github.com/jordigilh/kubernaut/pkg/fleet/fmc/config"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/fleet/observation"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
)

// acmITCluster is the single registered cluster every fixture below uses.
var acmITCluster = registry.ClusterInfo{ID: "cluster-a", RegistrationUID: "registration-a"}

// acmITRegistry is a registration-fenced registry fake: it answers the
// CurrentRegistrationGuard and RegistrySnapshotter contracts the production
// syncer's repair path depends on, and nothing else.
type acmITRegistry struct {
	current  registry.ClusterInfo
	revision uint64
	events   chan registry.ClusterEvent
}

func newACMITRegistry() *acmITRegistry {
	return &acmITRegistry{
		current:  acmITCluster,
		revision: 7,
		events:   make(chan registry.ClusterEvent, 1),
	}
}

func (r *acmITRegistry) List() []registry.ClusterInfo { return []registry.ClusterInfo{r.current} }

func (r *acmITRegistry) Get(clusterID string) (registry.ClusterInfo, bool) {
	if clusterID != r.current.ID {
		return registry.ClusterInfo{}, false
	}
	return r.current, true
}

func (r *acmITRegistry) WatchClusters() <-chan registry.ClusterEvent { return r.events }

func (r *acmITRegistry) Ready() bool                            { return true }
func (r *acmITRegistry) Start(context.Context) error            { return nil }
func (r *acmITRegistry) Stop()                                  {}
func (r *acmITRegistry) SnapshotAll() registry.RegistrySnapshot { return r.snapshotAll() }

func (r *acmITRegistry) snapshotAll() registry.RegistrySnapshot {
	return registry.RegistrySnapshot{
		Generation:    r.revision,
		Registrations: []registry.RegistrationSnapshot{{Info: r.current, Revision: r.revision}},
	}
}

func (r *acmITRegistry) Snapshot(clusterID string) (registry.RegistrationSnapshot, bool) {
	if clusterID != r.current.ID {
		return registry.RegistrationSnapshot{}, false
	}
	return registry.RegistrationSnapshot{Info: r.current, Revision: r.revision}, true
}

func (r *acmITRegistry) WithCurrent(_ context.Context, clusterID, registrationUID string, expectedRevision uint64, callback func(registry.ClusterInfo) error) error {
	if clusterID != r.current.ID || registrationUID != r.current.RegistrationUID || expectedRevision != r.revision {
		return fmt.Errorf("%w: cluster %q registration changed", registry.ErrRegistrationChanged, clusterID)
	}
	return callback(r.current)
}

// acmSearchServer is an httptest TLS stand-in for one ACM Search endpoint.
// It serves the paginated GraphQL document the collector sends, honors
// offset/limit windows against a mutable per-kind catalog, requires a bearer
// token, and records every request so specs can prove the call surface.
type acmSearchServer struct {
	*httptest.Server

	mu             sync.Mutex
	clusterID      string
	pods           []map[string]any
	namespaces     []map[string]any
	podDelays      []time.Duration
	requestsByKind map[string]int
	requestCount   int
	authSeen       []string
	expectedToken  atomic.Value // string
}

func newACMSearchServer(clusterID, token string) *acmSearchServer {
	server := &acmSearchServer{
		clusterID:      clusterID,
		requestsByKind: make(map[string]int),
	}
	server.expectedToken.Store(token)
	server.Server = httptest.NewTLSServer(http.HandlerFunc(server.serve))
	return server
}

// setDefaultCatalog serves a small mixed fleet: a managed pod, an explicitly
// unmanaged pod, and one unlabeled namespace.
func (s *acmSearchServer) setDefaultCatalog() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pods = []map[string]any{
		acmSearchItem("uid-1", s.clusterID, "Pod", "payments", "api", "kubernaut.ai/managed=false"),
		acmSearchItem("uid-2", s.clusterID, "Pod", "payments", "web", "kubernaut.ai/managed=true"),
	}
	s.namespaces = []map[string]any{
		acmSearchItem("uid-0", s.clusterID, "Namespace", "", "payments", ""),
	}
	s.podDelays = nil
}

// setDelayedPodCatalog serves count pods split into pages of 50, delaying the
// first len(delays) pod requests by the given durations. It exists for the
// repair-deadline spec: each delay must stay inside the per-request cap while
// the summed delays exhaust the repair's collection budget.
func (s *acmSearchServer) setDelayedPodCatalog(count int, delays []time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pods = make([]map[string]any, 0, count)
	for index := 0; index < count; index++ {
		s.pods = append(s.pods, acmSearchItem(
			fmt.Sprintf("uid-%05d", index), s.clusterID, "Pod", "payments",
			fmt.Sprintf("p-%05d", index), "kubernaut.ai/managed=true"))
	}
	s.podDelays = delays
}

func (s *acmSearchServer) expectToken(token string) { s.expectedToken.Store(token) }

func (s *acmSearchServer) kindRequests(kind string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requestsByKind[kind]
}

func (s *acmSearchServer) totalRequests() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requestCount
}

func (s *acmSearchServer) lastAuth() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.authSeen) == 0 {
		return ""
	}
	return s.authSeen[len(s.authSeen)-1]
}

func (s *acmSearchServer) serve(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	expected := "Bearer " + s.expectedToken.Load().(string) //nolint:forcetypeassert // stored as string only
	s.mu.Lock()
	s.authSeen = append(s.authSeen, auth)
	s.requestCount++
	s.mu.Unlock()
	if auth != expected {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var request struct {
		Variables struct {
			Input []struct {
				Filters []struct {
					Property string   `json:"property"`
					Values   []string `json:"values"`
				} `json:"filters"`
				Limit  int `json:"limit"`
				Offset int `json:"offset"`
			} `json:"input"`
		} `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if len(request.Variables.Input) != 1 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	input := request.Variables.Input[0]
	kind := ""
	for _, filter := range input.Filters {
		if filter.Property == "kind" && len(filter.Values) == 1 {
			kind = filter.Values[0]
		}
	}
	s.mu.Lock()
	catalog := s.pods
	if kind == "Namespace" {
		catalog = s.namespaces
	}
	s.requestsByKind[kind]++
	var delay time.Duration
	if kind == "Pod" && len(s.podDelays) > 0 {
		delay = s.podDelays[0]
		s.podDelays = s.podDelays[1:]
	}
	s.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}
	var items []map[string]any
	if input.Offset >= 0 && input.Offset < len(catalog) {
		end := input.Offset + limit
		if end > len(catalog) {
			end = len(catalog)
		}
		items = catalog[input.Offset:end]
	}
	reply := map[string]any{
		"data": map[string]any{
			"searchResult": []map[string]any{{"count": len(catalog), "items": items}},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(reply)
}

// acmSearchItem builds one Search projection item in the spike-recorded shape
// (SPIKE_2026-11-18): lowercase keys, `_uid` for ordering, `label` as the
// semicolon projection. Every catalog kind in these fixtures is core/v1.
func acmSearchItem(uid, clusterID, kind, namespace, name, label string) map[string]any {
	item := map[string]any{
		"_uid":       uid,
		"cluster":    clusterID,
		"apiversion": "v1",
		"kind":       kind,
		"name":       name,
	}
	if namespace != "" {
		item["namespace"] = namespace
	}
	if label != "" {
		item["label"] = label
	}
	return item
}

// fakeMCPGateway is a minimal MCP-over-HTTP gateway: enough JSON-RPC for the
// production mcpclient.ResilientClient to connect and list one managed pod.
// It counts tools/call separately from handshake traffic so specs can tell
// "the MCP collector ran" from "a client was merely constructed".
type fakeMCPGateway struct {
	*httptest.Server

	mu          sync.Mutex
	toolCallCnt int
}

func newFakeMCPGateway() *fakeMCPGateway {
	gateway := &fakeMCPGateway{}
	gateway.Server = httptest.NewServer(http.HandlerFunc(gateway.serve))
	return gateway
}

func (g *fakeMCPGateway) toolCalls() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.toolCallCnt
}

func (g *fakeMCPGateway) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.Method == http.MethodDelete {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var request struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	switch request.Method {
	case "initialize":
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2025-11-25","capabilities":{},"serverInfo":{"name":"test","version":"1.0"}}}`, request.ID)
	case "notifications/initialized", "notifications/cancelled":
		w.WriteHeader(http.StatusAccepted)
	case "tools/call":
		g.mu.Lock()
		g.toolCallCnt++
		g.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"structuredContent":{"items":[`+
			`{"apiVersion":"v1","kind":"Pod","metadata":{"name":"web","namespace":"payments","labels":{"kubernaut.ai/managed":"true"}}}`+
			`],"continue":null}}}`, request.ID)
	default:
		w.WriteHeader(http.StatusNotImplemented)
	}
}

// writeITConfig renders a production-shaped config.yaml and returns its path.
// The file is always loaded through fmcconfig.LoadFromFile + Validate by the
// caller, so the strict `sources` decoding runs in every spec.
func writeITConfig(dir, mcpEndpoint, sourcesBlock string, kinds ...string) string {
	document := fmt.Sprintf(`server:
  apiAddr: 127.0.0.1:0
  healthAddr: 127.0.0.1:0
  metricsAddr: 127.0.0.1:0
mcpGateway:
  endpoint: %s
  gatewayType: eaigw
  namespace: kubernaut-system
valkey:
  addr: 127.0.0.1:1
sync:
  resourceKinds: [%s]
oauth2:
  tokenUrl: https://oauth.example/token
%s`, mcpEndpoint, strings.Join(kinds, ", "), sourcesBlock)
	path := filepath.Join(dir, "config.yaml")
	ExpectWithOffset(1, os.WriteFile(path, []byte(document), 0o600)).To(Succeed())
	return path
}

func acmSourcesBlock(endpoint, tokenPath, caPath string) string {
	return fmt.Sprintf("sources:\n  acm:\n    endpoint: %s\n    tokenPath: %s\n    caFile: %s\n", endpoint, tokenPath, caPath)
}

func loadValidatedITConfig(path string) *fmcconfig.ServiceConfig {
	cfg, err := fmcconfig.LoadFromFile(path)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "the fixture config must load through the production loader")
	ExpectWithOffset(1, cfg.Validate()).To(Succeed(), "the fixture config must pass production validation")
	return cfg
}

// writeServerCA persists an httptest TLS server's certificate as a caFile.
func writeServerCA(path string, server *httptest.Server) {
	cert := server.Certificate()
	ExpectWithOffset(1, cert).NotTo(BeNil())
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	ExpectWithOffset(1, os.WriteFile(path, pemBytes, 0o600)).To(Succeed())
}

// acmSyncITFixture assembles the production FMC writer dependency set around
// one miniredis instance and builds the syncer through buildFMCSyncer. Server
// time is pinned (the scopecache IT discipline) so TTL proofs stay
// deterministic under FastForward.
type acmSyncITFixture struct {
	server    *miniredis.Miniredis
	store     *scopecache.ValkeyStatusStore
	writer    *fmc.ValkeyWriter
	readiness *scopecache.FMCReadinessSnapshot
	reg       *acmITRegistry
	cfg       *fmcconfig.ServiceConfig
	deps      *fmcDeps
	syncer    *fmc.Syncer

	fakeTimeBase time.Time
	advanced     time.Duration
}

func newACMSyncITFixture(cfg *fmcconfig.ServiceConfig, mcpClient *mcpclient.ResilientClient) *acmSyncITFixture {
	server, err := miniredis.Run()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	base := time.Unix(1_700_000_000, 0)
	server.SetTime(base)
	store, err := scopecache.NewValkeyStatusStoreAt(server.Addr())
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, store.LoadScript(context.Background())).To(Succeed())
	readiness, err := scopecache.NewFMCReadinessSnapshot(scopecache.LeasePolicyProcessWideRequired)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	reg := newACMITRegistry()
	deps := &fmcDeps{
		metrics:           fmc.NewMetrics(prometheus.NewRegistry()),
		mcpClient:         mcpClient,
		writer:            fmc.NewValkeyWriter(server.Addr()),
		cacheReader:       scopecache.NewValkeyCacheReader(server.Addr()),
		statusStore:       store,
		lease:             buildFMCWriterCoordinator(),
		readiness:         readiness,
		clusterRegistry:   reg,
		registrationGuard: reg,
	}
	syncer, err := buildFMCSyncer(cfg, deps, logr.Discard())
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "production syncer construction must succeed for a validated config")
	fixture := &acmSyncITFixture{
		server:       server,
		store:        store,
		writer:       deps.writer,
		readiness:    readiness,
		reg:          reg,
		cfg:          cfg,
		deps:         deps,
		syncer:       syncer,
		fakeTimeBase: base,
	}
	DeferCleanup(func() {
		_ = store.Close()
		_ = deps.writer.Close()
		_ = deps.cacheReader.Close()
		server.Close()
	})
	return fixture
}

// advance moves the miniredis clock forward, re-pinning Lua TIME to the same
// instant so the strict TTL proof sees no skew (established discipline from
// pkg/fleet/scopecache/staged_repair_test.go).
func (f *acmSyncITFixture) advance(d time.Duration) {
	f.advanced += d
	f.server.FastForward(d)
	f.server.SetTime(f.fakeTimeBase.Add(f.advanced))
}

// partitionPrefix mirrors scopecache's hashed partition keyspace so the specs
// can assert on the committed/staged keys directly.
func (f *acmSyncITFixture) partitionPrefix() string {
	digest := sha256.Sum256([]byte(acmITCluster.ID + "\x00" + acmITCluster.RegistrationUID))
	return "kubernaut:fmc:v2:" + hex.EncodeToString(digest[:])
}

func (f *acmSyncITFixture) keyExists(suffix string) bool {
	prefix := f.partitionPrefix()
	for _, key := range f.server.Keys() {
		if key == prefix+suffix {
			return true
		}
	}
	return false
}

func (f *acmSyncITFixture) partitionKey() scopecache.PartitionStateKey {
	return scopecache.PartitionStateKey{ClusterID: acmITCluster.ID, RegistrationUID: acmITCluster.RegistrationUID}
}

// listInventory reads through the production FMC page reader over the same
// status store, registration guard and readiness snapshot the syncer used.
func (f *acmSyncITFixture) listInventory() (inventory.Page, error) {
	reader, err := fmc.NewInventoryReader(f.store, f.reg, f.readiness, fmc.InventoryReaderConfig{})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return reader.List(context.Background(), inventory.Filters{ClusterID: acmITCluster.ID, Limit: 50})
}

// newITMCPClient connects the production resilient client to a fake gateway.
func newITMCPClient(endpoint string) *mcpclient.ResilientClient {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := mcpclient.NewResilient(ctx, endpoint, mcpclient.DefaultResilienceConfig(), logr.Discard())
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "the fake MCP gateway must satisfy the production handshake")
	return client
}

var _ = Describe("FMC ACM source production sync (DD-FLEET-017 D4, wiring rows 3-4)", func() {
	It("IT-FMC-SRC-2661-001 [BR-INTEGRATION-065, BR-SAFETY-001, BR-SCOPE-001]: an ACM-configured deployment collects through ACM Search and publishes only through the fenced repair", func() {
		dir := GinkgoT().TempDir()
		search := newACMSearchServer(acmITCluster.ID, "token-from-file")
		search.setDefaultCatalog()
		DeferCleanup(search.Close)
		tokenPath := filepath.Join(dir, "token")
		Expect(os.WriteFile(tokenPath, []byte("token-from-file"), 0o600)).To(Succeed())
		caPath := filepath.Join(dir, "ca.crt")
		writeServerCA(caPath, search.Server)

		mcp := newFakeMCPGateway()
		DeferCleanup(mcp.Close)
		cfgPath := writeITConfig(dir, mcp.URL, acmSourcesBlock(search.URL, tokenPath, caPath), "Pod", "Namespace")
		fixture := newACMSyncITFixture(loadValidatedITConfig(cfgPath), newITMCPClient(mcp.URL))

		ctx := context.Background()
		Expect(fixture.syncer.SyncCluster(ctx, acmITCluster)).To(Succeed())

		By("ACM Search was the collection surface for every configured kind")
		Expect(search.kindRequests("Pod")).To(BeNumerically(">", 0),
			"a missing Search call for a configured kind must fail the spec")
		Expect(search.kindRequests("Namespace")).To(BeNumerically(">", 0),
			"a missing Search call for a configured kind must fail the spec")
		Expect(mcp.toolCalls()).To(BeZero(),
			"an ACM-configured deployment must never collect through MCP tools")

		By("publication reached BeginRepair -> Repair -> CommitRepair, not a direct Valkey write")
		// Only the fenced commit path can produce these keys: BeginRepair
		// stages them, Repair writes the staged records and both indexes,
		// CommitRepair atomically swaps them into the live keyspace and
		// deletes the staging. A direct SetObservation write would leave no
		// committed status and no proof-gated page.
		Expect(fixture.keyExists(":status")).To(BeTrue(), "CommitRepair must swap in the committed status")
		Expect(fixture.keyExists(":index")).To(BeTrue(), "CommitRepair must publish the data index")
		Expect(fixture.keyExists(":index:lex")).To(BeTrue(), "CommitRepair must publish the lexicographic index")
		Expect(fixture.keyExists(":status:staged")).To(BeFalse(), "a clean commit consumes the staging")
		Expect(fixture.keyExists(":index:staged")).To(BeFalse(), "a clean commit consumes the staged index")
		Expect(fixture.keyExists(":index:lex:staged")).To(BeFalse(), "a clean commit consumes the staged lex index")

		partition := fixture.partitionKey()
		view := fixture.readiness.View()
		Expect(view.Partitions).To(HaveKey(partition))
		clean := view.Partitions[partition]
		Expect(clean.State).To(Equal(scopecache.PartitionClean), "publishCleanPartition runs only after CommitRepair")
		Expect(clean.CleanCommit).To(BeTrue())
		Expect(clean.Generation).To(BeNumerically(">", 0))

		By("the proven page serves exactly the managed Search items")
		page, err := fixture.listInventory()
		Expect(err).NotTo(HaveOccurred())
		Expect(page.State).To(Equal(inventory.StateSuccess))
		Expect(page.Freshness.State).To(Equal(inventory.FreshnessFresh))
		Expect(page.Items).To(HaveLen(1), "the managed pod is included; the explicit-false pod and the unlabeled namespace are not")
		Expect(page.Items[0].Kind).To(Equal("Pod"))
		Expect(page.Items[0].Namespace).To(Equal("payments"))
		Expect(page.Items[0].Name).To(Equal("web"))
		Expect(page.Items[0].Scope.Included).To(BeTrue())
	})

	It("IT-FMC-SRC-2661-002 [BR-INTEGRATION-065]: absent sources selects the MCP collector, sources.acm selects the ACM collector, and an invalid sources block fails before syncer construction", func() {
		dir := GinkgoT().TempDir()
		search := newACMSearchServer(acmITCluster.ID, "token-from-file")
		search.setDefaultCatalog()
		DeferCleanup(search.Close)
		tokenPath := filepath.Join(dir, "token")
		Expect(os.WriteFile(tokenPath, []byte("token-from-file"), 0o600)).To(Succeed())
		caPath := filepath.Join(dir, "ca.crt")
		writeServerCA(caPath, search.Server)
		mcp := newFakeMCPGateway()
		DeferCleanup(mcp.Close)

		By("an absent sources block keeps the MCP collector (D4.1a rule 1)")
		mcpConfigPath := writeITConfig(dir, mcp.URL, "", "Pod")
		mcpFixture := newACMSyncITFixture(loadValidatedITConfig(mcpConfigPath), newITMCPClient(mcp.URL))
		Expect(mcpFixture.cfg.SourceKind()).To(Equal(fmcconfig.SourceMCP))
		Expect(mcpFixture.syncer.SyncCluster(context.Background(), acmITCluster)).To(Succeed())
		Expect(mcp.toolCalls()).To(BeNumerically(">", 0),
			"the MCP deployment must collect through the MCP gateway")
		Expect(search.totalRequests()).To(BeZero(),
			"an MCP-configured deployment must never contact ACM Search")
		Expect(mcpFixture.keyExists(":status")).To(BeTrue(),
			"the MCP lane publishes through the same fenced repair path")

		By("a sources.acm block selects the ACM collector (D4.1a rule 2)")
		acmRequestsBefore := search.totalRequests()
		toolCallsBefore := mcp.toolCalls()
		acmConfigPath := writeITConfig(dir, mcp.URL, acmSourcesBlock(search.URL, tokenPath, caPath), "Pod")
		acmFixture := newACMSyncITFixture(loadValidatedITConfig(acmConfigPath), newITMCPClient(mcp.URL))
		Expect(acmFixture.cfg.SourceKind()).To(Equal(fmcconfig.SourceACM))
		Expect(acmFixture.syncer.SyncCluster(context.Background(), acmITCluster)).To(Succeed())
		Expect(search.totalRequests()).To(BeNumerically(">", acmRequestsBefore),
			"the ACM deployment must collect through ACM Search")
		Expect(mcp.toolCalls()).To(Equal(toolCallsBefore),
			"the ACM deployment must never call MCP tools")

		By("an invalid sources block fails before syncer construction (D4.1a rule 3)")
		unknownKey := "sources:\n  acm:\n    endpoint: https://search.example:4010\n    tokenPath: " + tokenPath + "\n    endpointURL: https://typo.example:4010\n"
		_, err := fmcconfig.LoadFromFile(writeITConfig(dir, mcp.URL, unknownKey, "Pod"))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("endpointURL"),
			"an operator typo under sources.acm must fail the load, not silently disable the guard")

		emptyBlock := "sources: {}\n"
		_, err = fmcconfig.LoadFromFile(writeITConfig(dir, mcp.URL, emptyBlock, "Pod"))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("expected sources.acm"),
			"a present-but-empty sources block is a wiring defect, never a fallback to MCP")

		plainHTTP := acmSourcesBlock("http://search.example:4010", tokenPath, caPath)
		badConfig, err := fmcconfig.LoadFromFile(writeITConfig(dir, mcp.URL, plainHTTP, "Pod"))
		Expect(err).NotTo(HaveOccurred())
		Expect(badConfig.Validate()).To(HaveOccurred())
		Expect(badConfig.Validate().Error()).To(ContainSubstring("sources.acm.endpoint"),
			"fail-closed endpoint validation must name the key and run before any syncer is built")
	})

	It("IT-FMC-SRC-2661-004 [BR-SAFETY-001, DD-FLEET-017 D4.1a rule 7b]: two kinds share one repair deadline; the start floor refuses, no clean commit is published, and the previous proof ages to stale then unavailable", func() {
		dir := GinkgoT().TempDir()
		search := newACMSearchServer(acmITCluster.ID, "token-from-file")
		search.setDefaultCatalog()
		DeferCleanup(search.Close)
		tokenPath := filepath.Join(dir, "token")
		Expect(os.WriteFile(tokenPath, []byte("token-from-file"), 0o600)).To(Succeed())
		caPath := filepath.Join(dir, "ca.crt")
		writeServerCA(caPath, search.Server)
		mcp := newFakeMCPGateway()
		DeferCleanup(mcp.Close)
		cfgPath := writeITConfig(dir, mcp.URL, acmSourcesBlock(search.URL, tokenPath, caPath), "Pod", "Namespace")
		fixture := newACMSyncITFixture(loadValidatedITConfig(cfgPath), newITMCPClient(mcp.URL))

		By("a first sync commits a clean generation")
		ctx := context.Background()
		Expect(fixture.syncer.SyncCluster(ctx, acmITCluster)).To(Succeed())
		page, err := fixture.listInventory()
		Expect(err).NotTo(HaveOccurred())
		Expect(page.State).To(Equal(inventory.StateSuccess))
		Expect(page.Items).To(HaveLen(1))
		generation := fixture.readiness.View().Partitions[fixture.partitionKey()].Generation

		By("the second sync exhausts the repair-wide budget during kind one, so kind two never starts a request")
		// Rule 7b: exactly one deadline per repair, read from the context.
		// The repair path owns setting it in production (55 s from BeginRepair);
		// that value is deliberately not configurable, so this spec bounds the
		// same production context seam with a shorter staging lifetime: 20 s
		// minus the 5 s commit reserve leaves 15 s of collection budget, and
		// the pod pages below (5 s + 5 s + 4 s, each inside the 10 s per-request
		// cap) consume 14 s of it. The second kind then faces less than the
		// 2 s start floor and must be refused without a request.
		deadlineCtx := fmc.WithCollectionDeadline(ctx, time.Now().Add(20*time.Second))
		search.setDelayedPodCatalog(150, []time.Duration{5 * time.Second, 5 * time.Second, 4 * time.Second})
		namespaceRequestsBefore := search.kindRequests("Namespace")
		syncErr := fixture.syncer.SyncCluster(deadlineCtx, acmITCluster)
		Expect(syncErr).To(HaveOccurred())
		Expect(syncErr.Error()).To(ContainSubstring("start floor"),
			"the collector must abandon staging from the budget rule, not start a request it cannot finish")
		Expect(search.kindRequests("Namespace")).To(Equal(namespaceRequestsBefore),
			"the start floor must refuse the second kind's request outright")

		By("no clean commit was published and the previous proof stays readable")
		view := fixture.readiness.View().Partitions[fixture.partitionKey()]
		Expect(view.Generation).To(Equal(generation), "a failed collection must not publish a new generation")
		Expect(view.State).To(Equal(scopecache.PartitionClean))
		page, err = fixture.listInventory()
		Expect(err).NotTo(HaveOccurred())
		Expect(page.State).To(Equal(inventory.StateSuccess))
		Expect(page.Items).To(HaveLen(1), "the previous proven generation still serves its rows")

		By("the previous proof ages to stale below the staleness threshold")
		// Committed proof TTL is 60 s (status hash) with a 40 s staleness
		// threshold; records carry the 45 s key TTL.
		fixture.advance(25 * time.Second)
		page, err = fixture.listInventory()
		Expect(err).NotTo(HaveOccurred())
		Expect(page.State).To(Equal(inventory.StateSuccess))
		Expect(page.Items).To(HaveLen(1), "stale is 200 with rows, not a state-only failure")
		Expect(page.Freshness.State).To(Equal(inventory.FreshnessStale))

		By("then to unavailable once the proof expires")
		fixture.advance(36 * time.Second)
		page, err = fixture.listInventory()
		Expect(err).To(HaveOccurred(), "an expired proof must fail closed, never serve rows")
		Expect(page.State).To(Equal(inventory.StateUnavailable))
		Expect(page.Items).To(BeEmpty())
		Expect(page.NextCursor).To(BeEmpty())
	})
})

var _ = Describe("FMC ACM source transport credentials (DD-FLEET-017 D4.1a rule 6)", func() {
	It("IT-FMC-SRC-2661-003 [BR-SECURITY-016, SC-8]: production construction sends the bearer from tokenPath, validates TLS against caFile, and honours token and CA rotation on one client", func() {
		dir := GinkgoT().TempDir()
		search := newACMSearchServer(acmITCluster.ID, "token-one")
		search.setDefaultCatalog()
		DeferCleanup(search.Close)
		tokenPath := filepath.Join(dir, "token")
		Expect(os.WriteFile(tokenPath, []byte("token-one"), 0o600)).To(Succeed())
		caPath := filepath.Join(dir, "ca.crt")
		writeServerCA(caPath, search.Server)
		mcp := newFakeMCPGateway()
		DeferCleanup(mcp.Close)

		cfgPath := writeITConfig(dir, mcp.URL, acmSourcesBlock(search.URL, tokenPath, caPath), "Pod")
		cfg := loadValidatedITConfig(cfgPath)

		By("the production source construction authenticates from tokenPath over TLS validated against caFile")
		source, err := buildACMSource(cfg)
		Expect(err).NotTo(HaveOccurred())
		reader, err := source.ReaderFor(context.Background(), acmITCluster)
		Expect(err).NotTo(HaveOccurred())
		observed, err := reader.ListObserved(context.Background(), observation.ListRequest{
			APIVersion: "v1",
			Kind:       "Pod",
			Limit:      observation.ObservationPageLimit,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(observed.Complete).To(BeTrue())
		Expect(observed.Items).To(HaveLen(2))
		Expect(search.lastAuth()).To(Equal("Bearer token-one"),
			"the bearer must come from tokenPath through the production construction")
		// The request succeeded over TLS whose only trust root is caFile: the
		// handshake itself is the TLS-validation proof.

		By("a caFile that does not sign the endpoint fails closed")
		wrongCert := filepath.Join(dir, "wrong-ca.crt")
		wrongKey := filepath.Join(dir, "wrong-ca.key")
		generateSelfSignedCert(wrongCert, wrongKey)
		requestsBefore := search.totalRequests()
		wrongCfgPath := writeITConfig(dir, mcp.URL, acmSourcesBlock(search.URL, tokenPath, wrongCert), "Pod")
		wrongSource, err := buildACMSource(loadValidatedITConfig(wrongCfgPath))
		Expect(err).NotTo(HaveOccurred(), "construction loads the CA file; the failure is at request time")
		wrongReader, err := wrongSource.ReaderFor(context.Background(), acmITCluster)
		Expect(err).NotTo(HaveOccurred())
		_, err = wrongReader.ListObserved(context.Background(), observation.ListRequest{
			APIVersion: "v1",
			Kind:       "Pod",
			Limit:      observation.ObservationPageLimit,
		})
		Expect(err).To(HaveOccurred(), "TLS validation against the configured CA must reject an untrusted host")
		Expect(search.totalRequests()).To(Equal(requestsBefore),
			"the handshake must fail before any request reaches the handler")

		By("token rotation takes effect on the same client without rebuilding it")
		Expect(os.WriteFile(tokenPath, []byte("token-two"), 0o600)).To(Succeed())
		search.expectToken("token-two")
		_, err = reader.ListObserved(context.Background(), observation.ListRequest{
			APIVersion: "v1",
			Kind:       "Pod",
			Limit:      observation.ObservationPageLimit,
		})
		Expect(err).To(HaveOccurred(), "the cached token is rejected once; the 401 invalidates the cache")
		Expect(search.lastAuth()).To(Equal("Bearer token-one"),
			"the first request after rotation still carried the cached token")
		observed, err = reader.ListObserved(context.Background(), observation.ListRequest{
			APIVersion: "v1",
			Kind:       "Pod",
			Limit:      observation.ObservationPageLimit,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(observed.Complete).To(BeTrue())
		Expect(search.lastAuth()).To(Equal("Bearer token-two"),
			"the next request on the same client must carry the rotated token")

		By("CA rotation takes effect on the same client without rebuilding it")
		second := newACMSearchServer(acmITCluster.ID, "token-two")
		second.setDefaultCatalog()
		DeferCleanup(second.Close)
		secondCA := filepath.Join(dir, "second-ca.crt")
		writeServerCA(secondCA, second.Server)
		secondCfgPath := writeITConfig(dir, mcp.URL, acmSourcesBlock(second.URL, tokenPath, wrongCert), "Pod")
		secondCfg := loadValidatedITConfig(secondCfgPath)
		// The exact production constructor call buildACMSource makes, driven
		// by the loaded config values. ACMSource does not expose its transport
		// for ReloadCA, so the rotation proof uses the constructor directly.
		client, err := fmc.NewACMHTTPSearch(
			secondCfg.Sources.ACM.Endpoint,
			secondCfg.Sources.ACM.TokenPath,
			secondCfg.Sources.ACM.CAFile,
			0,
		)
		Expect(err).NotTo(HaveOccurred())
		options := fmc.ACMSearchOptions{ClusterID: acmITCluster.ID, Version: "v1", Kind: "Pod", OrderBy: "uid asc", Limit: 50}
		_, err = client.Search(context.Background(), options)
		Expect(err).To(HaveOccurred(), "a CA that does not sign the endpoint must fail closed")
		rotatedCAPEM, readErr := os.ReadFile(secondCA)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(client.ReloadCA(rotatedCAPEM)).To(Succeed())
		page, err := client.Search(context.Background(), options)
		Expect(err).NotTo(HaveOccurred())
		Expect(page.Count).To(Equal(2))
		Expect(second.lastAuth()).To(Equal("Bearer token-two"),
			"the rotated client still authenticates from tokenPath")
	})
})
