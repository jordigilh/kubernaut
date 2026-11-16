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
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

// Connection/reader construction is NOT partition cleanliness. These tests use
// actual envtest registry, resilient session provider, MCP observation parser,
// coordinator, Valkey Lua store, readiness and Client.Check business components.
// Only the external MCP endpoint is controlled. DD-FLEET-013; AC-4/AC-6, SI-10.
var _ = Describe("FMC observed repair wiring (BR-SCOPE-001, BR-INTEGRATION-065)", Label("fmc", "broker-readiness"), func() {
	var (
		ctx         context.Context
		cancel      context.CancelFunc
		cluster     string
		namespace   string
		clusterReg  *registry.EAIGWRegistry
		store       *scopecache.ValkeyStatusStore
		readiness   *scopecache.FMCReadinessSnapshot
		coordinator *scopecache.ProcessWriterCoordinator
		syncer      *fmc.Syncer
		checker     *scopecache.Client
		available   atomic.Bool
		empty       atomic.Bool
		calls       atomic.Int32
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		DeferCleanup(cancel)
		available.Store(false)
		empty.Store(false)
		calls.Store(0)
		cluster = "broker-" + uuid.NewString()
		namespace = cluster
		kube, err := kubernetes.NewForConfig(restConfig)
		Expect(err).ToNot(HaveOccurred())
		_, err = kube.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() {
			Expect(kube.CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})).To(Succeed())
		})
		gw := mockgw.NewMockGateway(mockgw.WithMultiCluster(cluster), mockgw.WithListResourcesHandler(func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			calls.Add(1)
			if !available.Load() {
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "observation temporarily unavailable"}}}, nil
			}
			if empty.Load() {
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
		store, err = scopecache.NewValkeyStatusStoreAt(valkeyAddr)
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
			fmc.Config{SyncInterval: 30 * time.Millisecond, KeyTTL: 30 * time.Second, ResourceKinds: []string{"Pod"}, WaitForBrokerReady: true},
			logr.Discard(), fmc.NewMetrics(prometheus.NewRegistry()))
		checker, err = scopecache.NewClientWithStatusStore(store, clusterReg, readiness)
		Expect(err).ToNot(HaveOccurred())
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
		Eventually(func() error { return clusterReg.Probe(ctx) }, 5*time.Second).Should(Succeed())
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
		Expect(result.Managed).To(BeNil())
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
