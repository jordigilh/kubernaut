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
	"fmt"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

// IT-FMC-BROKER: Broker readiness probe integration tests.
// Authority: BR-FLEET-002 (Fleet Metadata Caching)
// FedRAMP: SI-4 (Information System Monitoring) -- startup readiness tracking
//          SC-5 (Denial of Service Protection) -- backoff prevents startup storms
//
// Pyramid Invariant: IT proves wiring.
// These tests mirror the Syncer construction in cmd/fleetmetadatacache/main.go
// to verify that the WaitForBrokerReady config field is correctly wired through
// the production dispatch path:
//
//   fmcconfig.ServiceConfig.Sync.WaitForBrokerReady (YAML config)
//     -> fmc.Config.WaitForBrokerReady (syncer config)
//       -> syncer.Run() calls waitForBrokerReady() before sync loop
//
// Wiring Manifest:
//
//	waitForBrokerReady()      -> syncer.Run() startup  -> IT-FMC-BROKER-001
//	Config.WaitForBrokerReady -> cmd/fleetmetadatacache -> IT-FMC-BROKER-001
var _ = Describe("IT-FMC-BROKER: Broker readiness probe wiring (BR-FLEET-002)", Label("fmc", "broker-readiness"), func() {
	var (
		ctx     context.Context
		cancel  context.CancelFunc
		writer  *fmc.ValkeyWriter
		metrics *fmc.Metrics
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		writer = fmc.NewValkeyWriter(valkeyAddr)
		reg := prometheus.NewPedanticRegistry()
		metrics = fmc.NewMetrics(reg)
	})

	AfterEach(func() {
		cancel()
		if writer != nil {
			_ = writer.Close()
		}
	})

	It("IT-FMC-BROKER-001 [SI-4, SC-5]: syncer constructed with WaitForBrokerReady=true waits for broker before starting sync cycles, mirroring cmd/fleetmetadatacache/main.go wiring", func() {
		var probeAttempts atomic.Int32
		var syncAllCalled atomic.Bool

		By("Creating a ReaderFactory that simulates an unready broker (mirrors cmd/fleetmetadatacache/main.go line 147-153)")
		readerFactory := fleet.ReaderFactoryFunc(func(_ context.Context, _ string) (client.Reader, error) {
			attempt := probeAttempts.Add(1)
			if attempt <= 3 {
				return nil, fmt.Errorf("broker not ready: tools not synced (attempt %d)", attempt)
			}
			syncAllCalled.Store(true)
			return &itSpyReader{}, nil
		})

		By("Creating Syncer with production-equivalent config (mirrors cmd/fleetmetadatacache/main.go line 140-154)")
		clusterReg := &itStubRegistry{
			clusters: []registry.ClusterInfo{{ID: "it-cluster"}},
			eventCh:  make(chan registry.ClusterEvent, 8),
		}

		syncerConfig := fmc.Config{
			SyncInterval:               time.Hour,
			KeyTTL:                      30 * time.Second,
			ResourceKinds:               []string{"Pod"},
			WaitForBrokerReady:          true,
			BrokerProbeInitialInterval:  5 * time.Millisecond,
			BrokerProbeMaxInterval:      20 * time.Millisecond,
			BrokerProbeTimeout:          5 * time.Second,
		}

		syncer := fmc.NewSyncerWithReaderFactory(clusterReg, readerFactory, writer, syncerConfig, logr.Discard(), metrics)

		By("Running Syncer through the same path as cmd/fleetmetadatacache/main.go line 200")
		done := make(chan error, 1)
		go func() {
			done <- syncer.Run(ctx)
		}()

		By("Verifying readiness probe completed before sync cycles started")
		Eventually(func() int32 {
			return probeAttempts.Load()
		}, 5*time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 4),
			"SI-4: readiness probe must retry with backoff until broker responds, providing operational visibility into startup state transitions")

		cancel()
		Eventually(done, 5*time.Second).Should(Receive(BeNil()))

		Expect(syncAllCalled.Load()).To(BeTrue(),
			"SC-5: sync cycles must only begin after readiness probe succeeds, preventing denial-of-service storms against an unready MCP gateway")
	})
})

// itSpyReader is a minimal client.Reader for IT tests.
type itSpyReader struct{}

func (r *itSpyReader) Get(_ context.Context, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
	return nil
}

func (r *itSpyReader) List(_ context.Context, list client.ObjectList, _ ...client.ListOption) error {
	if ul, ok := list.(*unstructured.UnstructuredList); ok {
		ul.Items = []unstructured.Unstructured{
			{Object: map[string]interface{}{
				"apiVersion": "v1", "kind": "Pod",
				"metadata": map[string]interface{}{
					"name": "it-pod", "namespace": "default",
				},
			}},
		}
	}
	return nil
}

// itStubRegistry is a minimal registry for IT tests.
type itStubRegistry struct {
	clusters []registry.ClusterInfo
	eventCh  chan registry.ClusterEvent
}

func (r *itStubRegistry) List() []registry.ClusterInfo       { return r.clusters }
func (r *itStubRegistry) WatchClusters() <-chan registry.ClusterEvent { return r.eventCh }
func (r *itStubRegistry) Ready() bool                        { return true }
func (r *itStubRegistry) Start(_ context.Context) error      { return nil }
func (r *itStubRegistry) Stop()                              { close(r.eventCh) }

func (r *itStubRegistry) Get(clusterID string) (registry.ClusterInfo, bool) {
	for _, c := range r.clusters {
		if c.ID == clusterID {
			return c, true
		}
	}
	return registry.ClusterInfo{}, false
}
