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

// Package live holds the helios08-only ACM lane of the FMC inventory
// projection -- E2E-FMC-INV-2661-001 in docs/tests/2661/TEST_PLAN.md §8
// Tier 3. It is deliberately NOT part of any CI suite: the kuadrant
// (Makefile test-e2e-fleetmetadatacache-kuadrant) and EAIGW
// (test-e2e-fleetmetadatacache-eaigw) targets are non-recursive over this
// nested package, and no other target references it. Its proof is ONE live
// run against the recorded ACM 2.17.4 build
// (acm-search-v2-api-rhel9@sha256:85c53e64f719b91a91c1eaed06111e0c9eefe27d7f62e4114568bfe5b141a3ac)
// on helios08 (kubernaut492-sno-b) with the least-privilege hub principal;
// the run's evidence (output, FMC/Valkey versions, principal) is recorded
// verbatim in docs/tests/2661/ (plan §11 Live row or the linked spike
// record), per the plan's §13 boundary: no live claim is made from fixtures.
//
// Composition: every production construction path is the real one --
// fmc.NewACMHTTPSearch + fmc.NewACMSource + fmc.NewSyncer (the exact calls
// cmd/fleetmetadatacache/main.go's buildACMSource/buildFMCSyncer make for a
// sources.acm deployment), the real ACM Search endpoint and a real Valkey
// over env-provided addresses, the real FMC inventory reader and HTTP mux
// (fmc.NewInventoryReader + fmc.NewHandlerWithInventory), and the real AF
// Console dispatch (fleet.NewInventoryReader over fleet.fmc.endpoint +
// policyread.NewService + the Console handler). Only the in-process edges
// are loopback: the FMC API listener and the Console authorizer (the AC-6
// assessment for this lane is the Search principal's least privilege, not
// Console RBAC, which the CI lanes own).
//
// Control objectives assessed on this path (plan §8 Tier 3): AC-4 (the only
// information flow is Console -> FMC -> ACM Search; no fallback, no second
// federation path), AC-6 (the Search collector runs as the recorded
// least-privilege hub principal), SI-10 (malformed cursor -> 400,
// unregistered cluster -> 503 state-only, never interpreted), and ASVS
// V5/V8 (bearer handling and the sanitized metadata-only payload).
//
// Required environment (the suite fails closed when any is missing -- it is
// only ever invoked deliberately, via make test-e2e-fmc-acm-live):
//
//	LIVE_ACM_SEARCH_ENDPOINT  ACM Search GraphQL route (recorded build)
//	LIVE_ACM_TOKEN_PATH       token file of the least-privilege hub principal
//	LIVE_ACM_KUBECONFIG       kubeconfig for the hub (fixture creation only)
//	LIVE_VALKEY_ADDR          real Valkey address (e.g. helios08 native instance)
//	LIVE_ACM_CA_FILE          optional CA bundle for the Search route
//	LIVE_ACM_CLUSTER_ID       optional cluster ID (default "local-cluster")
package live

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck // Ginkgo DSL dot-import convention
	. "github.com/onsi/gomega"    //nolint:staticcheck // Ginkgo/Gomega DSL dot-import convention

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	afhandler "github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/policyread"
	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

func TestACMInventoryLiveE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "FMC ACM live inventory E2E Suite (Issue #2661, helios08)")
}

// liveEnv is the env-driven run configuration. Failing closed on a missing
// variable is the point: this suite never runs without a deliberate live
// environment behind it (plan §13: no live claim from fixtures).
type liveEnv struct {
	searchEndpoint string
	tokenPath      string
	caFile         string
	valkeyAddr     string
	clusterID      string
	kubeconfigPath string
}

func loadLiveEnv() liveEnv {
	required := func(name string) string {
		value := os.Getenv(name)
		ExpectWithOffset(1, value).NotTo(BeEmpty(),
			"%s must be set -- this suite only runs against the live helios08 environment (see the package comment)", name)
		return value
	}
	clusterID := os.Getenv("LIVE_ACM_CLUSTER_ID")
	if clusterID == "" {
		clusterID = "local-cluster"
	}
	return liveEnv{
		searchEndpoint: required("LIVE_ACM_SEARCH_ENDPOINT"),
		tokenPath:      required("LIVE_ACM_TOKEN_PATH"),
		caFile:         os.Getenv("LIVE_ACM_CA_FILE"),
		valkeyAddr:     required("LIVE_VALKEY_ADDR"),
		clusterID:      clusterID,
		kubeconfigPath: required("LIVE_ACM_KUBECONFIG"),
	}
}

// liveRegistry is the registration-fenced registry fake pinned to the live
// cluster identity (mirrors cmd/fleetmetadatacache/acm_source_it_test.go's
// acmITRegistry): membership is asserted, never invented (DD-FLEET-017
// D1.3a). The live run's registration UID is a stable synthetic value; the
// partition keyspace derives from it, and nothing on the hub depends on it.
type liveRegistry struct {
	cluster registry.ClusterInfo
}

func (r *liveRegistry) List() []registry.ClusterInfo { return []registry.ClusterInfo{r.cluster} }
func (r *liveRegistry) Get(clusterID string) (registry.ClusterInfo, bool) {
	if clusterID != r.cluster.ID {
		return registry.ClusterInfo{}, false
	}
	return r.cluster, true
}
func (r *liveRegistry) WatchClusters() <-chan registry.ClusterEvent {
	return make(chan registry.ClusterEvent)
}
func (r *liveRegistry) Ready() bool                 { return true }
func (r *liveRegistry) Start(context.Context) error { return nil }
func (r *liveRegistry) Stop()                       {}
func (r *liveRegistry) SnapshotAll() registry.RegistrySnapshot {
	return registry.RegistrySnapshot{
		Generation:    1,
		Registrations: []registry.RegistrationSnapshot{{Info: r.cluster, Revision: 1}},
	}
}
func (r *liveRegistry) Snapshot(clusterID string) (registry.RegistrationSnapshot, bool) {
	if clusterID != r.cluster.ID {
		return registry.RegistrationSnapshot{}, false
	}
	return registry.RegistrationSnapshot{Info: r.cluster, Revision: 1}, true
}
func (r *liveRegistry) WithCurrent(_ context.Context, clusterID, registrationUID string, expectedRevision uint64, callback func(registry.ClusterInfo) error) error {
	if clusterID != r.cluster.ID || registrationUID != r.cluster.RegistrationUID || expectedRevision != 1 {
		return fmt.Errorf("%w: cluster %q registration changed", registry.ErrRegistrationChanged, clusterID)
	}
	return callback(r.cluster)
}

// unavailableScopeChecker keeps the FMC mux's scope-check route honest
// without coupling this suite to a scope store: the inventory routes under
// test never consult it.
type unavailableScopeChecker struct{}

func (unavailableScopeChecker) Check(_ context.Context, _ scope.ResourceIdentity) (scope.ScopeResult, error) {
	return scope.ScopeResult{State: scope.ScopeUnavailable}, nil
}

// localInventoryProbe counts local fallback calls: the AC-4 half of the
// proof. Every non-local Console read must leave it untouched.
type localInventoryProbe struct {
	listCalls int
	getCalls  int
}

func (p *localInventoryProbe) List(context.Context, inventory.Filters) (inventory.Page, error) {
	p.listCalls++
	return inventory.StateOnly(inventory.StateUnavailable), inventory.ErrUnavailable
}

func (p *localInventoryProbe) Get(context.Context, string, string) (inventory.Resource, error) {
	p.getCalls++
	return inventory.Resource{}, inventory.ErrNotFound
}

// allowAllAuthorizer stands in for Console RBAC on the live lane: the AC-6
// assessment here is the Search principal, not the Console SAR surface
// (owned by IT-AF-2661-003 and the CI lanes).
type allowAllAuthorizer struct{}

func (allowAllAuthorizer) Check(context.Context, string, []string, string) (bool, error) {
	return true, nil
}

func (allowAllAuthorizer) CheckTarget(context.Context, string, []string, auth.TargetAccess) (bool, error) {
	return true, nil
}

var _ = Describe("E2E-FMC-INV-2661-001 [AC-4, AC-6, SI-10, ASVS V5/V8]: ACM lane -- FMC collects from live ACM Search, Console reads the projection, no fallback", Ordered, func() {
	var (
		env       liveEnv
		reg       *liveRegistry
		store     *scopecache.ValkeyStatusStore
		writer    *fmc.ValkeyWriter
		readable  *scopecache.ValkeyCacheReader
		readiness *scopecache.FMCReadinessSnapshot

		fmcServer *httptest.Server
		console   http.Handler
		local     *localInventoryProbe

		// syncCluster re-runs the real collection cycle (see BeforeAll).
		syncCluster func()

		fixtureNS   string
		fixtureName string
	)

	BeforeAll(func() {
		env = loadLiveEnv()
		reg = &liveRegistry{cluster: registry.ClusterInfo{
			ID:              env.clusterID,
			RegistrationUID: "dd2661-live-registration",
		}}

		By("creating the managed fixture on the hub (namespace label + group-ful kind, ADR-053)")
		fixtureNS = fmt.Sprintf("dd2661-live-inv-%d", time.Now().UnixNano())
		fixtureName = "dd2661-live-inv-deploy"
		restCfg, err := clientcmd.BuildConfigFromFlags("", env.kubeconfigPath)
		Expect(err).NotTo(HaveOccurred(), "LIVE_ACM_KUBECONFIG must be loadable")
		k8s, err := client.New(restCfg, client.Options{Scheme: runtime.NewScheme()})
		Expect(err).NotTo(HaveOccurred())
		Expect(clientgoschemeAddTo(k8s)).To(Succeed())
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name:   fixtureNS,
			Labels: map[string]string{"kubernaut.ai/managed": "true"},
		}}
		Expect(k8s.Create(context.Background(), ns)).To(Succeed())
		// replicas 0: the fixture exists for Search to index; it runs no pods.
		zero := int32(0)
		deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
			Name:      fixtureName,
			Namespace: fixtureNS,
			// No managed label on the resource: inclusion must come from the
			// namespace (the inheritance half, mirroring the GitOps lane's
			// #2311 shape).
		}, Spec: appsv1.DeploymentSpec{
			Replicas: &zero,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": fixtureName}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": fixtureName},
				},
				// A container is structurally required; with replicas 0 it is
				// never scheduled, so the placeholder image is never pulled.
				Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name:  "placeholder",
					Image: "registry.k8s.io/pause:3.10",
				}}},
			},
		}}
		Expect(k8s.Create(context.Background(), deploy)).To(Succeed())
		DeferCleanup(func() {
			_ = k8s.Delete(context.Background(), ns)
		})

		By("wiring the real Valkey and the production ACM collector composition")
		var storeErr error
		store, storeErr = scopecache.NewValkeyStatusStoreAt(env.valkeyAddr)
		Expect(storeErr).NotTo(HaveOccurred(), "LIVE_VALKEY_ADDR must be a reachable Valkey")
		Expect(store.LoadScript(context.Background())).To(Succeed())
		writer = fmc.NewValkeyWriter(env.valkeyAddr)
		readable = scopecache.NewValkeyCacheReader(env.valkeyAddr)
		var readyErr error
		readiness, readyErr = scopecache.NewFMCReadinessSnapshot(scopecache.LeasePolicyProcessWideRequired)
		Expect(readyErr).NotTo(HaveOccurred())
		DeferCleanup(func() {
			_ = store.Close()
			_ = writer.Close()
			_ = readable.Close()
		})

		// The exact production composition of buildACMSource + buildFMCSyncer
		// for a sources.acm deployment (cmd/fleetmetadatacache/main.go):
		// exported constructors, identical arguments.
		kinds, err := fmc.KindGVKs([]string{"Namespace", "Deployment"})
		Expect(err).NotTo(HaveOccurred())
		search, err := fmc.NewACMHTTPSearch(env.searchEndpoint, env.tokenPath, env.caFile, 0)
		Expect(err).NotTo(HaveOccurred(), "least-privilege principal token and CA must be readable")
		source, err := fmc.NewACMSource(fmc.ACMSourceConfig{Search: search, Kinds: kinds})
		Expect(err).NotTo(HaveOccurred())
		syncer := fmc.NewSyncer(fmc.Dependencies{
			Registry:    reg,
			Readers:     source,
			Writer:      writer,
			StatusStore: store,
			Lease:       scopecache.NewProcessWriterCoordinator(),
			Readiness:   readiness,
		}, fmc.Config{
			SyncInterval:  30 * time.Second,
			KeyTTL:        45 * time.Second,
			ResourceKinds: []string{"Namespace", "Deployment"},
		}, logr.Discard(), fmc.NewMetrics(prometheus.NewRegistry()))

		// syncCluster runs one real collection cycle -- exactly what the
		// production sync loop does every SyncInterval. The fixture is
		// created seconds before the first cycle, and live Search indexes
		// asynchronously (spike 2026-11-18: settle lag 0.4-6.5 s), so the
		// first walk may legitimately not see it yet: re-collecting until it
		// does is the production behavior, not outcome retry. The
		// state/freshness/sanitization assertions always run on the page the
		// final cycle served.
		syncCluster = func() {
			syncCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			ExpectWithOffset(1, syncer.SyncCluster(syncCtx, reg.cluster)).To(Succeed(),
				"the fenced repair must complete a clean commit from the real Search walk")
		}

		By("serving the real FMC inventory routes on a loopback listener")
		inventoryReader, err := fmc.NewInventoryReader(store, reg, readiness, fmc.InventoryReaderConfig{})
		Expect(err).NotTo(HaveOccurred())
		mux := http.NewServeMux()
		fmc.NewHandlerWithInventory(unavailableScopeChecker{}, reg, inventoryReader, logr.Discard()).RegisterRoutes(mux)
		fmcServer = httptest.NewServer(mux)
		DeferCleanup(fmcServer.Close)

		By("wiring the real AF Console dispatch from fleet.fmc.endpoint (wiring rows 4a/5)")
		// The production reader's transport is an auth token source: in a
		// real deployment it reads AF's projected ServiceAccount token. This
		// loopback run is not in a pod, so the default projected-volume path
		// does not exist and every request would fail with "authentication
		// token unavailable". A configured TokenPath (the production knob for
		// exactly this) points at a locally minted file; the FMC listener
		// under test applies no inbound auth of its own (that leg is owned by
		// the CI lanes' TokenReview/SAR middleware).
		loopbackToken := filepath.Join(GinkgoT().TempDir(), "af-token")
		Expect(os.WriteFile(loopbackToken, []byte("dd2661-live-loopback"), 0o600)).To(Succeed())
		afReader, err := fleet.NewInventoryReader(fleet.FleetConfig{
			Enabled:   true,
			FMC:       fleet.FleetFMCConfig{Endpoint: fmcServer.URL},
			TokenPath: loopbackToken,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(afReader).NotTo(BeNil(), "fleet.fmc.endpoint must select the FMC reader")
		local = &localInventoryProbe{}
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			LocalInventory:  local,
			RemoteInventory: afReader,
			Authorizer:      allowAllAuthorizer{},
		})
		Expect(err).NotTo(HaveOccurred())
		console = afhandler.NewConsolePolicyReadHandler(svc, logr.Discard())
	})

	consoleList := func(query string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, afConsoleInventoryPath+"?"+query, http.NoBody)
		req = req.WithContext(auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "dd2661-live"}))
		recorder := httptest.NewRecorder()
		console.ServeHTTP(recorder, req)
		return recorder
	}

	consoleGet := func(resourceID, query string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, afConsoleInventoryPath+"/"+resourceID+"?"+query, http.NoBody)
		req.SetPathValue("resourceID", resourceID)
		req = req.WithContext(auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "dd2661-live"}))
		recorder := httptest.NewRecorder()
		console.ServeHTTP(recorder, req)
		return recorder
	}

	It("Console lists the managed fixture with freshness=fresh from proof, sanitized, over the ACM-collected partition (BR-API-001, BR-SECURITY-017)", func() {
		// Bounded re-collection (see syncCluster): the assertions below run
		// once, on the page the final cycle served.
		var page inventory.Page
		Eventually(func(g Gomega) {
			syncCluster()
			recorder := consoleList(fmt.Sprintf("cluster_id=%s&namespace=%s&kind=Deployment", env.clusterID, fixtureNS))
			g.Expect(recorder.Code).To(Equal(http.StatusOK))
			g.Expect(json.Unmarshal(recorder.Body.Bytes(), &page)).To(Succeed())
			g.Expect(page.State).To(Equal(inventory.StateSuccess))
			g.Expect(page.Items).NotTo(BeEmpty())
		}, 2*time.Minute, 5*time.Second).Should(Succeed(),
			"the fixture must be indexed by live Search and committed through the fenced repair")

		Expect(page.Completeness).To(Equal(inventory.CompletenessComplete))
		Expect(page.Freshness.State).To(Equal(inventory.FreshnessFresh),
			"freshness must derive from the proof state of the clean commit, not from query time (R3)")
		Expect(page.Freshness.AsOf).NotTo(BeZero())

		Expect(page.Items).To(HaveLen(1), "the namespace+kind filtered read returns exactly the fixture")
		item := page.Items[0]
		Expect(item.ClusterID).To(Equal(env.clusterID))
		Expect(item.Group).To(Equal("apps"), "the group-ful kind proves the separate apigroup/apiversion wire shape end to end")
		Expect(item.Version).To(Equal("v1"))
		Expect(item.Kind).To(Equal("Deployment"))
		Expect(item.Namespace).To(Equal(fixtureNS))
		Expect(item.Name).To(Equal(fixtureName))
		Expect(item.Scope.Included).To(BeTrue())
		Expect(item.Scope.Source).To(Equal(inventory.ScopeSourceNamespaceLabel))
		Expect(item.ID).To(Equal(inventory.CanonicalResourceID(item.Identity())))

		Expect(local.listCalls).To(BeZero(), "AC-4: a non-local Console read must never fall back to the local backend")
	})

	It("Console pages the projection and reads one exact resource by its opaque ID (BR-API-001)", func() {
		syncCluster() // each read journey runs against a freshly proven partition
		whole := consoleList(fmt.Sprintf("cluster_id=%s&limit=1", env.clusterID))
		Expect(whole.Code).To(Equal(http.StatusOK))
		var first inventory.Page
		Expect(json.Unmarshal(whole.Body.Bytes(), &first)).To(Succeed())
		Expect(first.State).To(Equal(inventory.StateSuccess))
		Expect(first.Items).To(HaveLen(1))
		Expect(first.NextCursor).NotTo(BeEmpty(), "a limited read with more items must carry a cursor")

		next := consoleList(fmt.Sprintf("cluster_id=%s&limit=1&cursor=%s", env.clusterID, first.NextCursor))
		Expect(next.Code).To(Equal(http.StatusOK))
		var second inventory.Page
		Expect(json.Unmarshal(next.Body.Bytes(), &second)).To(Succeed())
		Expect(second.State).To(Equal(inventory.StateSuccess))
		Expect(second.Items).To(HaveLen(1))
		Expect(second.Items[0].ID).NotTo(Equal(first.Items[0].ID), "the cursor resumes after the last served item")

		fixtureID := inventory.CanonicalResourceID(scope.ResourceIdentity{
			ClusterID: env.clusterID, Group: "apps", Version: "v1",
			Kind: "Deployment", Namespace: fixtureNS, Name: fixtureName,
		})
		exact := consoleGet(fixtureID, "cluster_id="+env.clusterID)
		Expect(exact.Code).To(Equal(http.StatusOK))
		var resource inventory.Resource
		Expect(json.Unmarshal(exact.Body.Bytes(), &resource)).To(Succeed())
		Expect(resource.ID).To(Equal(fixtureID))
		Expect(resource.Name).To(Equal(fixtureName))
		Expect(resource.Scope.Included).To(BeTrue())

		Expect(local.listCalls).To(BeZero())
		Expect(local.getCalls).To(BeZero(), "AC-4: the exact read dispatches to FMC only")
	})

	It("rejects malformed input and unknown partitions state-only, never as inventory claims (SI-10, BR-SAFETY-001)", func() {
		By("malformed cursor -> 400")
		bad := consoleList(fmt.Sprintf("cluster_id=%s&cursor=../page", env.clusterID))
		Expect(bad.Code).To(Equal(http.StatusBadRequest))

		By("unregistered cluster -> 503 state-only unavailable, no rows, no fallback")
		unknown := consoleList("cluster_id=dd2661-live-unregistered")
		Expect(unknown.Code).To(Equal(http.StatusServiceUnavailable))
		Expect(unknown.Body.String()).To(ContainSubstring(`"state":"unavailable"`))
		Expect(unknown.Body.String()).NotTo(ContainSubstring(`"items"`))

		Expect(local.listCalls).To(BeZero(), "AC-4: a failed remote read must not fall back locally")
	})

	It("serves only the sanitized metadata-only projection on the wire (ASVS V8, BR-SECURITY-017)", func() {
		syncCluster()
		recorder := consoleList(fmt.Sprintf("cluster_id=%s&namespace=%s&kind=Deployment", env.clusterID, fixtureNS))
		Expect(recorder.Code).To(Equal(http.StatusOK))
		body := recorder.Body.String()
		for _, leaked := range []string{"labels", "annotations", "\"data\"", "spec", "status", "token", "secret"} {
			Expect(body).NotTo(ContainSubstring(leaked),
				"the sanitized inventory projection must not carry %q on the wire", leaked)
		}
	})
})

// afConsoleInventoryPath mirrors the handler package's unexported route
// constant (pkg/apifrontend/handler/console_policyread.go): a rename there
// breaks this literal and the spec fails loudly, never false-passes.
const afConsoleInventoryPath = "/api/v1/console/inventory/managed-resources"

// clientgoschemeAddTo registers the standard client-go types on the fixture
// client's scheme.
func clientgoschemeAddTo(c client.Client) error {
	return scheme.AddToScheme(c.Scheme())
}
