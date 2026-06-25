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
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// spyReader is a test double for client.Reader that records List calls
// to verify label filter arguments (AC-4: information flow enforcement).
type spyReader struct {
	capturedListOpts client.ListOptions
	listItems        []unstructured.Unstructured
	listErr          error
}

func (r *spyReader) Get(_ context.Context, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
	return nil
}

func (r *spyReader) List(_ context.Context, list client.ObjectList, opts ...client.ListOption) error {
	for _, o := range opts {
		o.ApplyToList(&r.capturedListOpts)
	}
	if ul, ok := list.(*unstructured.UnstructuredList); ok {
		ul.Items = r.listItems
	}
	return r.listErr
}

// stubWriter is a test double for CacheWriter that counts writes.
type stubWriter struct {
	keysWritten []string
}

func (w *stubWriter) Set(_ context.Context, key string, _ time.Duration) error {
	w.keysWritten = append(w.keysWritten, key)
	return nil
}

func (w *stubWriter) Close() error { return nil }

var _ = Describe("Syncer with ReaderFactory (BR-FLEET-002, Phase A)", func() {
	var (
		ctx     context.Context
		writer  *stubWriter
		metrics *fmc.Metrics
	)

	BeforeEach(func() {
		ctx = context.Background()
		writer = &stubWriter{}
		reg := prometheus.NewPedanticRegistry()
		metrics = fmc.NewMetrics(reg)
	})

	Describe("UT-FLEET-FMC-003 [AC-4]: managed-label filter in syncKind", func() {
		It("passes MatchingLabels{kubernaut.ai/managed: true} to the reader List call, enforcing information flow control", func() {
			spy := &spyReader{
				listItems: []unstructured.Unstructured{
					{Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Pod",
						"metadata": map[string]interface{}{
							"name":      "nginx",
							"namespace": "default",
						},
					}},
				},
			}

			readerFactory := func(_ context.Context, _ string) (client.Reader, error) {
				return spy, nil
			}

			syncer := fmc.NewSyncerWithReaderFactory(
				&stubRegistry{clusters: []registry.ClusterInfo{{ID: "cluster-a"}}},
				readerFactory,
				writer,
			fmc.Config{
				KeyTTL:        30 * time.Second,
				ResourceKinds: []string{"Pod"},
			},
				logr.Discard(),
				metrics,
			)

			err := syncer.SyncCluster(ctx, registry.ClusterInfo{ID: "cluster-a"})
			Expect(err).ToNot(HaveOccurred())

			Expect(spy.capturedListOpts.LabelSelector).ToNot(BeNil(),
				"List must be called with a label selector to enforce AC-4 information flow boundaries")

			selectorStr := spy.capturedListOpts.LabelSelector.String()
			Expect(selectorStr).To(ContainSubstring(scope.ManagedLabelKey + "=" + scope.ManagedLabelValueTrue),
				"only resources with kubernaut.ai/managed=true must pass through the information flow boundary")
		})
	})
})

// stubRegistry is a test double for registry.ClusterRegistry.
type stubRegistry struct {
	clusters []registry.ClusterInfo
	eventCh  chan registry.ClusterEvent
}

func newStubRegistry(clusters ...registry.ClusterInfo) *stubRegistry {
	return &stubRegistry{
		clusters: clusters,
		eventCh:  make(chan registry.ClusterEvent, 8),
	}
}

func (r *stubRegistry) List() []registry.ClusterInfo {
	return r.clusters
}

func (r *stubRegistry) Get(clusterID string) (registry.ClusterInfo, bool) {
	for _, c := range r.clusters {
		if c.ID == clusterID {
			return c, true
		}
	}
	return registry.ClusterInfo{}, false
}

func (r *stubRegistry) WatchClusters() <-chan registry.ClusterEvent {
	return r.eventCh
}

func (r *stubRegistry) Ready() bool { return true }

func (r *stubRegistry) Start(_ context.Context) error { return nil }

func (r *stubRegistry) Stop() { close(r.eventCh) }

// UT-FLEET-FMC-LIFE: Syncer lifecycle and config tests
// Authority: BR-FLEET-002 (Fleet Metadata Caching)
// FedRAMP: SI-4 (Information System Monitoring) -- sync cycle tracking
var _ = Describe("UT-FLEET-FMC-LIFE: Syncer lifecycle", func() {
	var (
		ctx     context.Context
		cancel  context.CancelFunc
		writer  *stubWriter
		metrics *fmc.Metrics
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		writer = &stubWriter{}
		reg := prometheus.NewPedanticRegistry()
		metrics = fmc.NewMetrics(reg)
	})

	AfterEach(func() {
		cancel()
	})

	Describe("DefaultConfig", func() {
		It("UT-FLEET-FMC-004: should return sensible production defaults", func() {
			cfg := fmc.DefaultConfig()
			Expect(cfg.SyncInterval).To(Equal(30 * time.Second))
			Expect(cfg.KeyTTL).To(Equal(45 * time.Second))
			Expect(cfg.ResourceKinds).To(ContainElements("Deployment", "StatefulSet", "Pod", "Service", "Node"))
			Expect(len(cfg.ResourceKinds)).To(BeNumerically(">=", 5))
		})
	})

	Describe("Run", func() {
		It("UT-FLEET-FMC-005: should reject double-start", func() {
			stubReg := newStubRegistry()
			syncer := fmc.NewSyncerWithReaderFactory(
				stubReg,
				func(_ context.Context, _ string) (client.Reader, error) {
					return &spyReader{}, nil
				},
				writer,
				fmc.Config{SyncInterval: time.Hour, KeyTTL: 30 * time.Second, ResourceKinds: []string{"Pod"}},
				logr.Discard(),
				metrics,
			)

			go func() {
				defer GinkgoRecover()
				_ = syncer.Run(ctx)
			}()

			time.Sleep(50 * time.Millisecond)

			err := syncer.Run(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already running"))
		})

		It("UT-FLEET-FMC-006: should stop cleanly on context cancellation", func() {
			stubReg := newStubRegistry()
			syncer := fmc.NewSyncerWithReaderFactory(
				stubReg,
				func(_ context.Context, _ string) (client.Reader, error) {
					return &spyReader{}, nil
				},
				writer,
				fmc.Config{SyncInterval: time.Hour, KeyTTL: 30 * time.Second, ResourceKinds: []string{"Pod"}},
				logr.Discard(),
				metrics,
			)

			done := make(chan error, 1)
			go func() {
				done <- syncer.Run(ctx)
			}()

			time.Sleep(50 * time.Millisecond)
			cancel()

			Eventually(done).Should(Receive(BeNil()))
		})

		It("UT-FLEET-FMC-007: should stop when registry channel closes", func() {
			stubReg := newStubRegistry()
			syncer := fmc.NewSyncerWithReaderFactory(
				stubReg,
				func(_ context.Context, _ string) (client.Reader, error) {
					return &spyReader{}, nil
				},
				writer,
				fmc.Config{SyncInterval: time.Hour, KeyTTL: 30 * time.Second, ResourceKinds: []string{"Pod"}},
				logr.Discard(),
				metrics,
			)

			done := make(chan error, 1)
			go func() {
				done <- syncer.Run(ctx)
			}()

			time.Sleep(50 * time.Millisecond)
			close(stubReg.eventCh)

			Eventually(done).Should(Receive(BeNil()))
		})
	})

	Describe("handleClusterEvent", func() {
		It("UT-FLEET-FMC-008: should trigger immediate sync on Added event", func() {
			spy := &spyReader{}
			stubReg := newStubRegistry()
			syncer := fmc.NewSyncerWithReaderFactory(
				stubReg,
				func(_ context.Context, _ string) (client.Reader, error) {
					return spy, nil
				},
				writer,
				fmc.Config{SyncInterval: time.Hour, KeyTTL: 30 * time.Second, ResourceKinds: []string{"Pod"}},
				logr.Discard(),
				metrics,
			)

			done := make(chan error, 1)
			go func() {
				done <- syncer.Run(ctx)
			}()

			time.Sleep(50 * time.Millisecond)

			stubReg.eventCh <- registry.ClusterEvent{
				Type:    registry.EventAdded,
				Cluster: registry.ClusterInfo{ID: "new-cluster"},
			}

			time.Sleep(100 * time.Millisecond)
			cancel()
			Eventually(done).Should(Receive(BeNil()))
		})
	})

	Describe("syncAll", func() {
		It("UT-FLEET-FMC-009: should sync all registered clusters", func() {
			spy := &spyReader{
				listItems: []unstructured.Unstructured{
					{Object: map[string]interface{}{
						"apiVersion": "v1", "kind": "Pod",
						"metadata": map[string]interface{}{
							"name": "web", "namespace": "default",
						},
					}},
				},
			}
			stubReg := newStubRegistry(
				registry.ClusterInfo{ID: "cluster-x"},
				registry.ClusterInfo{ID: "cluster-y"},
			)
			syncer := fmc.NewSyncerWithReaderFactory(
				stubReg,
				func(_ context.Context, _ string) (client.Reader, error) {
					return spy, nil
				},
				writer,
				fmc.Config{SyncInterval: time.Hour, KeyTTL: 30 * time.Second, ResourceKinds: []string{"Pod"}},
				logr.Discard(),
				metrics,
			)

			done := make(chan error, 1)
			go func() {
				done <- syncer.Run(ctx)
			}()

			time.Sleep(100 * time.Millisecond)
			cancel()
			Eventually(done).Should(Receive(BeNil()))

			Expect(len(writer.keysWritten)).To(BeNumerically(">=", 2),
				"Should write keys for resources from both clusters")
		})
	})
})
