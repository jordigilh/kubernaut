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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
	"github.com/jordigilh/kubernaut/test/testutil/redisfault"
)

// Connection/reader construction is NOT partition cleanliness. These tests use
// actual envtest registry, resilient session provider, MCP observation parser,
// coordinator, Valkey Lua store, readiness and Client.Check business components.
// Only the external MCP endpoint is controlled. DD-FLEET-013; AC-4/AC-6, SI-10.
var _ = Describe("FMC observed repair wiring (BR-SCOPE-001, BR-INTEGRATION-065)", Label("fmc", "broker-readiness"), func() {
	var (
		ctx               context.Context
		cancel            context.CancelFunc
		cluster           string
		namespace         string
		clusterReg        *registry.EAIGWRegistry
		store             *scopecache.ValkeyStatusStore
		readiness         *scopecache.FMCReadinessSnapshot
		coordinator       *scopecache.ProcessWriterCoordinator
		syncer            *fmc.Syncer
		checker           *scopecache.Client
		available         atomic.Bool
		empty             atomic.Bool
		deferred          atomic.Bool
		nodeUnlabeled     atomic.Bool
		namespaceFalse    atomic.Bool
		calls             atomic.Int32
		proxy             *redisfault.Proxy
		holdCollection    atomic.Bool
		collectionEntered chan struct{}
		releaseCollection chan struct{}
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		DeferCleanup(cancel)
		available.Store(false)
		empty.Store(false)
		deferred.Store(false)
		nodeUnlabeled.Store(false)
		namespaceFalse.Store(false)
		calls.Store(0)
		holdCollection.Store(false)
		collectionEntered = make(chan struct{}, 1)
		releaseCollection = make(chan struct{})
		cluster = "broker-" + uuid.NewString()
		namespace = cluster
		kube, err := kubernetes.NewForConfig(restConfig)
		Expect(err).ToNot(HaveOccurred())
		_, err = kube.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() {
			Expect(kube.CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})).To(Succeed())
		})
		gw := mockgw.NewMockGateway(mockgw.WithMultiCluster(cluster), mockgw.WithListResourcesHandler(func(callCtx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			calls.Add(1)
			if holdCollection.Load() {
				select {
				case collectionEntered <- struct{}{}:
				default:
				}
				select {
				case <-releaseCollection:
				case <-callCtx.Done():
					return nil, callCtx.Err()
				}
			}
			if !available.Load() {
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "observation temporarily unavailable"}}}, nil
			}
			if empty.Load() {
				return &mcp.CallToolResult{StructuredContent: map[string]interface{}{"items": []map[string]interface{}{}}}, nil
			}
			var args struct {
				Kind string `json:"kind"`
			}
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return nil, fmt.Errorf("decode list fixture arguments: %w", err)
			}
			if deferred.Load() {
				metadata := map[string]interface{}{"name": "observed", "namespace": "default"}
				if args.Kind != "Pod" {
					metadata = map[string]interface{}{"name": "observed", "labels": map[string]interface{}{"kubernaut.ai/managed": "true"}}
					if args.Kind == "Node" && nodeUnlabeled.Load() {
						metadata = map[string]interface{}{"name": "observed"}
					}
					if args.Kind == "Namespace" {
						metadata["name"] = "default"
					}
					if args.Kind == "Namespace" && namespaceFalse.Load() {
						metadata["labels"] = map[string]interface{}{"kubernaut.ai/managed": "false"}
					}
				}
				return &mcp.CallToolResult{StructuredContent: map[string]interface{}{"items": []map[string]interface{}{{"apiVersion": "v1", "kind": args.Kind, "metadata": metadata}}}}, nil
			}
			if args.Kind != "Pod" {
				return &mcp.CallToolResult{StructuredContent: map[string]interface{}{"items": []map[string]interface{}{}}}, nil
			}
			return &mcp.CallToolResult{StructuredContent: map[string]interface{}{"items": []map[string]interface{}{{
				"apiVersion": "v1", "kind": "Pod",
				"metadata": map[string]interface{}{"name": "observed", "namespace": "default", "labels": map[string]interface{}{"kubernaut.ai/managed": "true"}},
			}}}}, nil
		}))
		DeferCleanup(gw.Close)
		config := mcpclient.DefaultResilienceConfig()
		config.MaxElapsedTime = 3 * time.Second
		rc, err := mcpclient.NewResilient(ctx, gw.URL(), config, logr.Discard())
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() { Expect(rc.Close()).To(Succeed()) })
		createBackendInNamespace(ctx, namespace, cluster)
		DeferCleanup(func() {
			err := dynClient.Resource(registry.BackendGVR).Namespace(namespace).Delete(context.Background(), cluster, metav1.DeleteOptions{})
			Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue(), "delete fixture Backend: %v", err)
		})
		clusterReg = registry.NewEAIGWRegistry(dynClient, registry.EAIGWRegistryConfig{Namespace: namespace, ChannelSize: 1}, nil, logr.Discard())
		Expect(clusterReg.Start(ctx)).To(Succeed())
		DeferCleanup(clusterReg.Stop)
		proxy, err = redisfault.New(ctx, valkeyAddr)
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(proxy.Close)
		store, err = scopecache.NewValkeyStatusStoreAt(proxy.Addr())
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() { Expect(store.Close()).To(Succeed()) })
		Expect(store.LoadScript(ctx)).To(Succeed())
		readiness, err = scopecache.NewFMCReadinessSnapshot(scopecache.LeasePolicyProcessWideRequired)
		Expect(err).ToNot(HaveOccurred())
		coordinator = scopecache.NewProcessWriterCoordinator()
		readers := fmc.ObservedReaderFactoryFunc(func(_ context.Context, info registry.ClusterInfo) (fmc.ObservedReader, error) {
			return mcpclient.NewFromSessionProvider(rc.SessionProvider(), info.ID,
				mcpclient.WithToolPrefix(info.ToolPrefix), mcpclient.WithReconnect(rc.Reconnect)), nil
		})
		syncer = fmc.NewSyncer(fmc.Dependencies{Registry: clusterReg, Readers: readers, StatusStore: store, Lease: coordinator, Readiness: readiness},
			fmc.Config{SyncInterval: 30 * time.Millisecond, KeyTTL: 30 * time.Second, ResourceKinds: []string{"Pod", "Node", "Namespace"}, WaitForBrokerReady: true},
			logr.Discard(), fmc.NewMetrics(prometheus.NewRegistry()))
		checker, err = scopecache.NewClientWithStatusStore(store, clusterReg, readiness)
		Expect(err).ToNot(HaveOccurred())
	})

	It("IT-AF-FMC-2593-001 [BR-SCOPE-001, BR-INTEGRATION-065]: repair-window refusal is unavailable, never unmanaged or an admitted investigation", func() {
		available.Store(true)
		info, found := clusterReg.Get(cluster)
		Expect(found).To(BeTrue())
		key := scopecache.PartitionStateKey{ClusterID: cluster, RegistrationUID: info.RegistrationUID}

		// Phase 1 (refusal contract, no committed proof): hold the FIRST
		// repair at the external observation boundary. There is no committed
		// clean proof yet, so the repair window must refuse fail-closed.
		holdCollection.Store(true)
		firstDone := make(chan error, 1)
		go func() { firstDone <- syncer.SyncCluster(ctx, info) }()
		releaseFirst := releaseCollection
		joinedFirst := false
		DeferCleanup(func() {
			if !joinedFirst {
				close(releaseFirst)
				cancel()
				Eventually(firstDone, 5*time.Second).Should(Receive())
			}
		})
		Eventually(collectionEntered, 5*time.Second).Should(Receive())
		duringFirst := readiness.View().Partitions[key]
		Expect(duringFirst.State).To(Equal(scopecache.PartitionRepairing))
		Expect(duringFirst.CleanCommit).To(BeFalse())
		Expect(syncer.Ready()).To(BeFalse())

		mux := http.NewServeMux()
		fmc.NewHandler(checker, clusterReg, logr.Discard()).RegisterRoutes(mux)
		server := httptest.NewServer(mux)
		DeferCleanup(server.Close)
		response, err := http.Get(server.URL + fmc.ScopeCheckPath + "?cluster=" + cluster + "&version=v1&kind=Pod&namespace=default&name=observed")
		Expect(err).NotTo(HaveOccurred())
		body, err := io.ReadAll(response.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.Body.Close()).To(Succeed())
		Expect(response.StatusCode).To(Equal(http.StatusServiceUnavailable))
		Expect(string(body)).NotTo(ContainSubstring(`"managed"`))

		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		rrClient, err := crclient.New(restConfig, crclient.Options{Scheme: scheme})
		Expect(err).ToNot(HaveOccurred())
		var starts atomic.Int32
		kaClient := &ka.MockMCPClient{StartInvestigationFn: func(context.Context, ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
			starts.Add(1)
			return nil, fmt.Errorf("unexpected KA start during repair")
		}}
		actionCtx := auth.WithUserIdentity(ctx, &auth.UserIdentity{Username: "repair-window-sre", Groups: []string{"sre"}})
		result, refusal := tools.HandleInvestigationMCPWithRegistry(actionCtx, &tools.InvestigateConfig{
			MCPClient: kaClient, Client: rrClient, Namespace: namespace,
			ScopeChecker: fmc.NewHTTPClient(server.URL),
		}, tools.InvestigateMCPArgs{APIVersion: "v1", Kind: "Pod", Namespace: "default", Name: "observed", ClusterID: cluster}, true, "repair-window-sre")
		Expect(refusal).To(HaveOccurred())
		Expect(result.RRID).To(BeEmpty())
		Expect(result.SessionID).To(BeEmpty())
		Expect(result.RCA).To(BeNil(), "a refusal must not fabricate investigation findings")
		Expect(result.Summary).To(BeEmpty())
		Expect(starts.Load()).To(BeZero())
		var requests remediationv1.RemediationRequestList
		Expect(rrClient.List(ctx, &requests, crclient.InNamespace(namespace))).To(Succeed())
		Expect(requests.Items).To(BeEmpty(), "repair-window refusal must persist no RR")
		// DD-FLEET-015: repair refusal is unavailable, never opt-in advice.
		Expect(refusal.Error()).To(ContainSubstring("unavailable"))
		Expect(refusal.Error()).NotTo(ContainSubstring("Add label"))
		Expect(refusal.Error()).NotTo(ContainSubstring("not managed"))

		close(releaseFirst)
		Eventually(firstDone, 5*time.Second).Should(Receive(BeNil()))
		joinedFirst = true
		committed := readiness.View().Partitions[key]
		Expect(committed.State).To(Equal(scopecache.PartitionClean))
		Expect(committed.CleanCommit).To(BeTrue())
		Expect(syncer.Ready()).To(BeTrue())
		verified, err := checker.Check(ctx, scope.ResourceIdentity{ClusterID: cluster, Version: "v1", Kind: "Pod", Namespace: "default", Name: "observed"})
		Expect(err).NotTo(HaveOccurred())
		Expect(verified.State).To(Equal(scope.ScopeVerifiedManaged))

		// Phase 2 (DD-FLEET-016 A2 availability contract): hold the SECOND
		// repair. The committed clean proof must remain readable for the
		// entire staged repair window.
		releaseCollection = make(chan struct{})
		holdCollection.Store(true)
		select {
		case <-collectionEntered:
		default:
		}
		secondDone := make(chan error, 1)
		go func() { secondDone <- syncer.SyncCluster(ctx, info) }()
		joinedSecond := false
		DeferCleanup(func() {
			if !joinedSecond {
				select {
				case <-releaseCollection:
				default:
					close(releaseCollection)
				}
				cancel()
				Eventually(secondDone, 5*time.Second).Should(Receive())
			}
		})
		Eventually(collectionEntered, 5*time.Second).Should(Receive())
		duringSecond := readiness.View().Partitions[key]
		Expect(duringSecond.State).To(Equal(scopecache.PartitionClean), "a staged repair must keep the committed proof readable")
		Expect(duringSecond.CleanCommit).To(BeTrue())
		Expect(syncer.Ready()).To(BeTrue(), "readiness must survive a staged repair")
		httpStill, err := http.Get(server.URL + fmc.ScopeCheckPath + "?cluster=" + cluster + "&version=v1&kind=Pod&namespace=default&name=observed")
		Expect(err).NotTo(HaveOccurred())
		stillBody, err := io.ReadAll(httpStill.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(httpStill.Body.Close()).To(Succeed())
		Expect(httpStill.StatusCode).To(Equal(http.StatusOK), "the staged repair window must serve the committed proof, body: %s", stillBody)
		stillVerified, err := checker.Check(ctx, scope.ResourceIdentity{ClusterID: cluster, Version: "v1", Kind: "Pod", Namespace: "default", Name: "observed"})
		Expect(err).NotTo(HaveOccurred())
		Expect(stillVerified.State).To(Equal(scope.ScopeVerifiedManaged))

		close(releaseCollection)
		Eventually(secondDone, 5*time.Second).Should(Receive(BeNil()))
		joinedSecond = true
		after := readiness.View().Partitions[key]
		Expect(after.State).To(Equal(scopecache.PartitionClean))
		Expect(after.CleanCommit).To(BeTrue())
		Expect(after.Generation).To(BeNumerically(">", committed.Generation))
		Expect(syncer.Ready()).To(BeTrue())
	})

	DescribeTable("IT-FMC-REPLAY-001 [BR-SCOPE-001]: lost replies require fresh repair through production wiring", func(verb string) {
		available.Store(true)
		info, found := clusterReg.Get(cluster)
		Expect(found).To(BeTrue())
		Expect(syncer.SyncCluster(ctx, info)).To(Succeed())
		Expect(syncer.Ready()).To(BeTrue())
		before := proxy.Calls(verb)
		proxy.Arm(verb)
		err := syncer.SyncCluster(ctx, info)
		Expect(proxy.Calls(verb)-before).To(Equal(1), "must not replay the executed mutation")
		Expect(err).To(MatchError(scopecache.ErrAmbiguousWrite))
		// DD-FLEET-016 A2: a lost reply strands only the staged state; the
		// committed clean proof stays readable and readiness stays up.
		Expect(syncer.Ready()).To(BeTrue(), "a lost reply must not invalidate the committed proof")
		result, _ := checker.Check(ctx, scope.ResourceIdentity{ClusterID: cluster, Version: "v1", Kind: "Pod", Namespace: "default", Name: "observed"})
		if verb == "begin" {
			// The begin never executed, so the committed proof serves reads.
			Expect(result.State).To(Equal(scope.ScopeVerifiedManaged))
		} else {
			// An ambiguous commit may have executed server-side: the server
			// then holds the NEW generation while readiness still publishes
			// the previous proof. Both outcomes are fail-closed (verified or
			// unavailable, never unmanaged); the next cycle republishes.
			Expect(result.State).To(BeElementOf(scope.ScopeVerifiedManaged, scope.ScopeUnavailable), "an ambiguous commit must never authorize from uncertainty")
		}
		// The stranded staging is abandoned fence-scoped (begin) or marked
		// uncertain (commit), so the next cycle repairs without waiting for
		// the staged TTL.
		Expect(syncer.SyncCluster(ctx, info)).To(Succeed(), "the next repair cycle must recover immediately")
		Expect(syncer.Ready()).To(BeTrue())
		result, err = checker.Check(ctx, scope.ResourceIdentity{ClusterID: cluster, Version: "v1", Kind: "Pod", Namespace: "default", Name: "observed"})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.State).To(Equal(scope.ScopeVerifiedManaged))
	}, Entry("begin", "begin"), Entry("commit", "commit"))

	It("IT-FMC-DEFERRED-001 [BR-SCOPE-001, AC-4, AC-6]: observed managed Nodes authorize targets and Namespace observations still support inheritance", func() {
		available.Store(true)
		deferred.Store(true)
		info, found := clusterReg.Get(cluster)
		Expect(found).To(BeTrue())
		Expect(syncer.SyncCluster(ctx, info)).To(Succeed())
		Expect(syncer.Ready()).To(BeTrue())
		node := scope.ResourceIdentity{ClusterID: cluster, Version: "v1", Kind: "Node", Name: "observed"}
		result, err := checker.Check(ctx, node)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.State).To(Equal(scope.ScopeVerifiedManaged), "#2670: a labeled Node must authorize remediation")
		Expect(result.Provenance).To(Equal(scope.ScopeSourceResourceLabel))
		mux := http.NewServeMux()
		fmc.NewHandler(checker, clusterReg, logr.Discard()).RegisterRoutes(mux)
		server := httptest.NewServer(mux)
		DeferCleanup(server.Close)
		response, err := http.Get(server.URL + fmc.ScopeCheckPath + "?cluster=" + cluster + "&version=v1&kind=Node&name=observed")
		Expect(err).ToNot(HaveOccurred())
		body, err := io.ReadAll(response.Body)
		Expect(err).ToNot(HaveOccurred())
		Expect(response.Body.Close()).To(Succeed())
		Expect(response.StatusCode).To(Equal(http.StatusOK), "a verified cluster-scoped decision is a 200, not a 422: %s", body)
		Expect(string(body)).To(ContainSubstring("verified_managed"))
		remote := fmc.NewHTTPClient(server.URL)
		managed, err := remote.Check(ctx, node)
		Expect(err).NotTo(HaveOccurred())
		Expect(managed.State).To(Equal(scope.ScopeVerifiedManaged))

		// A present Node with no managed label is decisive unmanaged: no
		// namespace exists to inherit, and a coincidentally matching Namespace
		// observation must not authorize it (AC-6).
		nodeUnlabeled.Store(true)
		Expect(syncer.SyncCluster(ctx, info)).To(Succeed())
		managed, err = remote.Check(ctx, node)
		Expect(err).NotTo(HaveOccurred())
		Expect(managed.State).To(Equal(scope.ScopeVerifiedUnmanaged))
		Expect(managed.Provenance).To(Equal(scope.ScopeSourceNone))
		response, err = http.Get(server.URL + fmc.ScopeCheckPath + "?cluster=" + cluster + "&version=v1&kind=Node&name=observed")
		Expect(err).ToNot(HaveOccurred())
		body, err = io.ReadAll(response.Body)
		Expect(err).ToNot(HaveOccurred())
		Expect(response.Body.Close()).To(Succeed())
		Expect(response.StatusCode).To(Equal(http.StatusOK), "verified_unmanaged is a 200 decision: %s", body)

		managed, err = remote.Check(ctx, scope.ResourceIdentity{ClusterID: cluster, Version: "v1", Kind: "Pod", Namespace: "default", Name: "observed"})
		Expect(err).ToNot(HaveOccurred())
		Expect(managed.State).To(Equal(scope.ScopeVerifiedManaged), "internal Namespace observations must remain available for inheritance")
		namespaceFalse.Store(true)
		Expect(syncer.SyncCluster(ctx, info)).To(Succeed())
		managed, err = remote.Check(ctx, scope.ResourceIdentity{ClusterID: cluster, Version: "v1", Kind: "Pod", Namespace: "default", Name: "observed"})
		Expect(err).NotTo(HaveOccurred())
		Expect(managed.State).To(Equal(scope.ScopeVerifiedUnmanaged))
		Expect(managed.Provenance).To(Equal(scope.ScopeSourceNamespaceLabel), "S03: fenced namespace false is a determining label observation, not absent opt-in")
	})

	It("IT-FMC-BROKER-001 [BR-SCOPE-001, BR-INTEGRATION-065]: successful connection cannot authorize a resource until real observations commit", func() {
		done := make(chan error, 1)
		go func() { done <- syncer.Run(ctx) }()
		DeferCleanup(func() { cancel(); Eventually(done, 5*time.Second).Should(Receive(BeNil())) })
		Eventually(calls.Load, 5*time.Second).Should(BeNumerically(">", 0))
		Expect(syncer.Ready()).To(BeFalse())
		result, err := checker.Check(ctx, scope.ResourceIdentity{ClusterID: cluster, Version: "v1", Kind: "Pod", Namespace: "default", Name: "observed"})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.State).To(Equal(scope.ScopeUnavailable))
		available.Store(true)
		Eventually(syncer.Ready, 5*time.Second).Should(BeTrue())
		Eventually(func(g Gomega) {
			result, err := checker.Check(ctx, scope.ResourceIdentity{ClusterID: cluster, Version: "v1", Kind: "Pod", Namespace: "default", Name: "observed"})
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(result.State).To(Equal(scope.ScopeVerifiedManaged))
		}, 5*time.Second).Should(Succeed())
	})

	It("IT-FMC-ISOLATION-001 [BR-SCOPE-001]: another fixture's registration cannot enter this writer's membership", func() {
		foreign := "foreign-" + uuid.NewString()
		createBackend(ctx, foreign)
		DeferCleanup(deleteBackend, context.Background(), foreign)
		// Observe a later in-namespace WATCH update as a progress barrier. A
		// direct Get immediately after the foreign Create would be vacuous.
		generation := clusterReg.SnapshotAll().Generation
		own, err := dynClient.Resource(registry.BackendGVR).Namespace(namespace).Get(ctx, cluster, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		own.SetAnnotations(map[string]string{"watch-barrier": uuid.NewString()})
		_, err = dynClient.Resource(registry.BackendGVR).Namespace(namespace).Update(ctx, own, metav1.UpdateOptions{})
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() uint64 { return clusterReg.SnapshotAll().Generation }, 5*time.Second).Should(BeNumerically(">", generation))
		_, found := clusterReg.Get(foreign)
		Expect(found).To(BeFalse(), "a parallel fixture's writer must not discover or repair this fixture's partitions")
	})

	It("IT-FMC-REVIEW-LIFECYCLE-001 [BR-SECURITY-016]: reacquisition fails closed through Syncer, readiness and Client.Check", func() {
		available.Store(true)
		info, found := clusterReg.Get(cluster)
		Expect(found).To(BeTrue())
		Expect(syncer.SyncCluster(ctx, info)).To(Succeed())
		old := readiness.View().Global.Lease.WriterFence
		Expect(coordinator.Release(ctx, old)).To(Succeed())
		available.Store(false)
		Expect(syncer.SyncCluster(ctx, info)).NotTo(Succeed())
		current := readiness.View().Global.Lease.WriterFence
		Expect(current).NotTo(Equal(old))
		Expect(readiness.InvalidateLease(old, "delayed old loss")).To(Succeed())
		Expect(readiness.View().Global.ActiveLeaseValid).To(BeTrue())
		result, err := checker.Check(ctx, scope.ResourceIdentity{ClusterID: cluster, Version: "v1", Kind: "Pod", Namespace: "default", Name: "observed"})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.State).To(Equal(scope.ScopeUnavailable))
		available.Store(true)
		Expect(syncer.SyncCluster(ctx, info)).To(Succeed())
		result, err = checker.Check(ctx, scope.ResourceIdentity{ClusterID: cluster, Version: "v1", Kind: "Pod", Namespace: "default", Name: "observed"})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.State).To(Equal(scope.ScopeVerifiedManaged))
	})

	It("IT-FMC-REVIEW-EMPTY-004 [BR-SCOPE-001]: complete empty repair is ready but missing resources cannot be authorized", func() {
		available.Store(true)
		info, found := clusterReg.Get(cluster)
		Expect(found).To(BeTrue())
		Expect(syncer.SyncCluster(ctx, info)).To(Succeed())
		empty.Store(true)
		Expect(syncer.SyncCluster(ctx, info)).To(Succeed())
		Expect(syncer.Ready()).To(BeTrue())
		view := readiness.View()
		partition := view.Partitions[scopecache.PartitionStateKey{ClusterID: cluster, RegistrationUID: info.RegistrationUID}]
		Expect(partition.EmptyPartitionVerified).To(BeTrue())
		Expect(partition.RecordTTLValid).To(BeFalse())
		Expect(partition.IndexTTLValid).To(BeFalse())
		result, err := checker.Check(ctx, scope.ResourceIdentity{ClusterID: cluster, Version: "v1", Kind: "Pod", Namespace: "default", Name: "observed"})
		Expect(err).To(MatchError("scopecache: fenced-read proof changed"))
		Expect(result.State).To(Equal(scope.ScopeUnavailable))
		Expect(result.Provenance).To(Equal(scope.ScopeSourceUnavailable))
	})

	It("IT-FMC-STAGED-001 [BR-SCOPE-001, #2660 m2]: a stranded staging expires on the real server and the next cycle reclaims it", func() {
		available.Store(true)
		info, found := clusterReg.Get(cluster)
		Expect(found).To(BeTrue())
		// A clean committed cycle first so the post-recovery read has a
		// committed proof to return to.
		Expect(syncer.SyncCluster(ctx, info)).To(Succeed())

		// Strand a staging: begin a repair with the writer fence the syncer
		// already holds (readiness publishes it) that never reconciles or
		// commits. The fence stays valid for the follow-up production cycle.
		fence := readiness.View().Global.Lease.WriterFence
		snapshot, found := clusterReg.Snapshot(cluster)
		Expect(found).To(BeTrue())
		partition := scopecache.PartitionStateKey{ClusterID: cluster, RegistrationUID: info.RegistrationUID}
		start, err := store.BeginRepair(ctx, scopecache.BeginRepairRequest{
			Partition:                partition,
			RegistrationUID:          info.RegistrationUID,
			ExpectedRegistryRevision: snapshot.Revision,
			WriterFence:              fence,
		})
		Expect(err).ToNot(HaveOccurred())

		// On a real TTL-honoring server the staged status expires after its
		// 15s TTL; nothing may keep it alive (no reconcile runs). This is the
		// real-server counterpart of UT-FMC-STAGED-011, which can only reach
		// the past-cap guard on a non-evicting miniredis clock.
		stagedDigest := sha256.Sum256([]byte(cluster + "\x00" + info.RegistrationUID))
		stagedKey := "kubernaut:fmc:v2:" + hex.EncodeToString(stagedDigest[:]) + ":status:staged"
		poller := redis.NewClient(&redis.Options{Addr: proxy.Addr()})
		DeferCleanup(func() { _ = poller.Close() })
		Eventually(func() bool {
			exists, err := poller.Exists(ctx, stagedKey).Result()
			return err == nil && exists == 0
		}, 25*time.Second, 500*time.Millisecond).Should(BeTrue(),
			"the stranded staging must expire on the real server")

		// A compliant server reports the expired staging as a stale fence
		// (the fence check reads the gone key before any cap logic), and the
		// failure is typed — the writer must begin a new repair.
		identity := scopecache.ResourceIdentity{
			ClusterID: cluster, Version: "v1", Kind: "Pod", Namespace: "default", Name: "observed",
		}
		_, err = store.Repair(ctx, scopecache.RepairRequest{
			Partition:                partition,
			RegistrationUID:          info.RegistrationUID,
			ExpectedRegistryRevision: snapshot.Revision,
			WriterFence:              fence,
			Generation:               start.Generation,
			Token:                    start.Token,
			Observations: scopecache.ValidatedObservationSet{
				Items: []scopecache.RepairObservation{{
					Identity: identity, APIVersion: "v1", LabelsPresent: true,
					LabelState: "true", TTL: 30 * time.Second,
				}},
				Complete: true, PageCount: 1, ItemCount: 1,
			},
		})
		Expect(err).To(HaveOccurred(), "a repair against the expired staging must fail closed")
		var failure scopecache.RepairFailureError
		Expect(errors.As(err, &failure)).To(BeTrue(), "the failure must be typed, got: %v", err)
		Expect(failure.Failure.Code).To(Equal(scopecache.RepairFailureStaleFence),
			"a compliant server reports the expired staging as a stale fence")

		// Self-heal: the next production cycle reclaims the expired staging
		// (a fresh begin) and publishes a clean proof readers can use.
		Expect(syncer.SyncCluster(ctx, info)).To(Succeed())
		result, err := checker.Check(ctx, scope.ResourceIdentity{ClusterID: cluster, Version: "v1", Kind: "Pod", Namespace: "default", Name: "observed"})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.State).To(Equal(scope.ScopeVerifiedManaged))
	})

	It("IT-FMC-REVIEW-RETIRE-001 [BR-SCOPE-001]: Run reconciles a failed retired partition without a delivered deletion event", func() {
		info, found := clusterReg.Get(cluster)
		Expect(found).To(BeTrue())
		Expect(syncer.SyncCluster(ctx, info)).NotTo(Succeed())
		Expect(syncer.Ready()).To(BeFalse())
		Expect(dynClient.Resource(registry.BackendGVR).Namespace(namespace).Delete(ctx, cluster, metav1.DeleteOptions{})).To(Succeed())
		Eventually(func() bool { _, found := clusterReg.Get(cluster); return found }, 5*time.Second).Should(BeFalse())
		// Remove all queued events before Run, so the production syncAll snapshot
		// path, not an event-specific removal, must restore empty-registry readiness.
		for len(clusterReg.WatchClusters()) > 0 {
			<-clusterReg.WatchClusters()
		}
		done := make(chan error, 1)
		go func() { done <- syncer.Run(ctx) }()
		DeferCleanup(func() { cancel(); Eventually(done, 5*time.Second).Should(Receive(BeNil())) })
		Eventually(syncer.Ready, 5*time.Second).Should(BeTrue())
		Expect(readiness.View().Partitions).To(BeEmpty())
		Expect(syncer.SyncCluster(ctx, info)).NotTo(Succeed())
		Expect(readiness.View().Partitions).To(BeEmpty())
	})
})
