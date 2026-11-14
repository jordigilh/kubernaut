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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/observation"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// spyReader is a test double for client.Reader that records List calls
// to verify label filter arguments (AC-4: information flow enforcement).
type spyReader struct {
	capturedListOpts client.ListOptions
	capturedRequest  observation.ListRequest
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

func (r *spyReader) ListObserved(_ context.Context, request observation.ListRequest) (observation.ListObservation, error) {
	r.capturedRequest = request
	if r.listErr != nil {
		return observation.ListObservation{Failure: observation.FailureTransport}, r.listErr
	}
	items := make([]observation.ObservedItem, 0, len(r.listItems))
	for _, item := range r.listItems {
		items = append(items, observation.ObservedItem{
			Object:     item,
			LabelState: scope.LabelStateAbsent,
		})
	}
	return observation.ListObservation{
		Items:     items,
		Complete:  true,
		PageCount: 1,
		ItemCount: len(items),
	}, nil
}

// stubWriter is a test double for CacheWriter that counts writes.
type stubWriter struct {
	keysWritten    []string
	observedStates map[string]scope.LabelState
}

func (w *stubWriter) Set(_ context.Context, key string, _ time.Duration) error {
	w.keysWritten = append(w.keysWritten, key)
	return nil
}

func (w *stubWriter) SetObservation(_ context.Context, key string, labelState scope.LabelState, _ time.Duration) error {
	if w.observedStates == nil {
		w.observedStates = make(map[string]scope.LabelState)
	}
	w.observedStates[key] = labelState
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

	Describe("UT-FMC-PRES-005 [BR-SCOPE-001]: unfiltered source observation in syncKind", func() {
		It("does not use a positive-only label selector, so presence and explicit false can be observed", func() {
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

			readerFactory := fmc.ObservedReaderFactoryFunc(func(_ context.Context, _ registry.ClusterInfo) (fmc.ObservedReader, error) {
				return spy, nil
			})

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

			Expect(spy.capturedRequest.LabelSelector).To(BeNil(),
				"FMC must observe all identity-bearing items, not only managed=true matches")
		})
	})

	Describe("UT-FMC-PRES-006 [BR-SCOPE-001]: sync errors", func() {
		It("returns a failure when a source list fails instead of swallowing the error", func() {
			spy := &spyReader{listErr: fmt.Errorf("source unavailable")}
			readerFactory := fmc.ObservedReaderFactoryFunc(func(_ context.Context, _ registry.ClusterInfo) (fmc.ObservedReader, error) {
				return spy, nil
			})
			syncer := fmc.NewSyncerWithReaderFactory(
				&stubRegistry{clusters: []registry.ClusterInfo{{ID: "cluster-a"}}},
				readerFactory,
				writer,
				fmc.Config{KeyTTL: time.Second, ResourceKinds: []string{"Pod"}},
				logr.Discard(), metrics,
			)

			err := syncer.SyncCluster(ctx, registry.ClusterInfo{ID: "cluster-a"})
			Expect(err).To(HaveOccurred())
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

		// Issue #2311: without syncing Namespace objects too, a namespace
		// labeled kubernaut.ai/managed=true with unlabeled resources inside
		// it never populates a namespace-level cache entry for
		// scopecache.Client.IsManagedResource's fallback to consult, so the
		// fleet scope-check hierarchy silently degrades to "resource label
		// only" -- stricter than the documented (and local-cluster-honored)
		// 2-level hierarchy (ADR-053).
		It("UT-FLEET-FMC-014 [Issue #2311]: should include Namespace in default resource kinds", func() {
			cfg := fmc.DefaultConfig()
			Expect(cfg.ResourceKinds).To(ContainElement("Namespace"),
				"Namespace must be synced so scopecache.Client can fall back to namespace-level labels on fleet clusters, same as local")
		})
	})

	Describe("syncKind for Namespace (Issue #2311)", func() {
		It("UT-FLEET-FMC-015: syncs Namespace resources carrying the managed label into the cache", func() {
			spy := &spyReader{
				listItems: []unstructured.Unstructured{
					{Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Namespace",
						"metadata": map[string]interface{}{
							"name": "demo-checkout",
						},
					}},
				},
			}

			readerFactory := fmc.ObservedReaderFactoryFunc(func(_ context.Context, _ registry.ClusterInfo) (fmc.ObservedReader, error) {
				return spy, nil
			})

			syncer := fmc.NewSyncerWithReaderFactory(
				&stubRegistry{clusters: []registry.ClusterInfo{{ID: "remote-cluster"}}},
				readerFactory,
				writer,
				fmc.Config{
					KeyTTL:        30 * time.Second,
					ResourceKinds: []string{"Namespace"},
				},
				logr.Discard(),
				metrics,
			)

			err := syncer.SyncCluster(ctx, registry.ClusterInfo{ID: "remote-cluster"})
			Expect(err).ToNot(HaveOccurred())

			expectedKey, keyErr := scopecache.BuildKey("remote-cluster", "", "v1", "Namespace", "", "demo-checkout")
			Expect(keyErr).ToNot(HaveOccurred())
			Expect(writer.keysWritten).To(ContainElement(expectedKey),
				"a labeled Namespace must be written to the cache so IsManagedResource's namespace fallback can find it")
		})
	})

	Describe("Run", func() {
		It("UT-FLEET-FMC-005: should reject double-start", func() {
			stubReg := newStubRegistry()
			syncer := fmc.NewSyncerWithReaderFactory(
				stubReg,
				fmc.ObservedReaderFactoryFunc(func(_ context.Context, _ registry.ClusterInfo) (fmc.ObservedReader, error) {
					return &spyReader{}, nil
				}),
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
				fmc.ObservedReaderFactoryFunc(func(_ context.Context, _ registry.ClusterInfo) (fmc.ObservedReader, error) {
					return &spyReader{}, nil
				}),
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
				fmc.ObservedReaderFactoryFunc(func(_ context.Context, _ registry.ClusterInfo) (fmc.ObservedReader, error) {
					return &spyReader{}, nil
				}),
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
				fmc.ObservedReaderFactoryFunc(func(_ context.Context, _ registry.ClusterInfo) (fmc.ObservedReader, error) {
					return spy, nil
				}),
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

	Describe("broker readiness probe [SI-4, SC-5]", func() {
		It("UT-FLEET-FMC-010 [SI-4, SC-5]: syncer waits for broker readiness with exponential backoff before starting sync cycles, preventing startup storms against an unready MCP gateway (BR-FLEET-002)", func() {
			attempts := 0
			failingFactory := fmc.ObservedReaderFactoryFunc(func(_ context.Context, _ registry.ClusterInfo) (fmc.ObservedReader, error) {
				attempts++
				if attempts <= 3 {
					return nil, fmt.Errorf("broker not ready: tools not synced (attempt %d)", attempts)
				}
				return &spyReader{}, nil
			})

			stubReg := newStubRegistry(registry.ClusterInfo{ID: "cluster-a"})
			syncer := fmc.NewSyncerWithReaderFactory(
				stubReg,
				failingFactory,
				writer,
				fmc.Config{
					SyncInterval:               time.Hour,
					KeyTTL:                     30 * time.Second,
					ResourceKinds:              []string{"Pod"},
					WaitForBrokerReady:         true,
					BrokerProbeInitialInterval: 5 * time.Millisecond,
					BrokerProbeMaxInterval:     20 * time.Millisecond,
					BrokerProbeTimeout:         5 * time.Second,
				},
				logr.Discard(),
				metrics,
			)

			done := make(chan error, 1)
			go func() {
				done <- syncer.Run(ctx)
			}()

			time.Sleep(500 * time.Millisecond)
			cancel()
			Eventually(done, 5*time.Second).Should(Receive(BeNil()))

			Expect(attempts).To(BeNumerically(">=", 4),
				"Should retry ReaderFor at least 4 times (3 failures + 1 success)")
		})

		It("UT-FLEET-FMC-011 [SI-4]: syncer proceeds immediately to sync cycles when readiness probe is disabled, preserving backward compatibility (BR-FLEET-002)", func() {
			attempts := 0
			failingFactory := fmc.ObservedReaderFactoryFunc(func(_ context.Context, _ registry.ClusterInfo) (fmc.ObservedReader, error) {
				attempts++
				return nil, fmt.Errorf("broker not ready")
			})

			stubReg := newStubRegistry(registry.ClusterInfo{ID: "cluster-a"})
			syncer := fmc.NewSyncerWithReaderFactory(
				stubReg,
				failingFactory,
				writer,
				fmc.Config{
					SyncInterval:       time.Hour,
					KeyTTL:             30 * time.Second,
					ResourceKinds:      []string{"Pod"},
					WaitForBrokerReady: false,
				},
				logr.Discard(),
				metrics,
			)

			done := make(chan error, 1)
			go func() {
				done <- syncer.Run(ctx)
			}()

			time.Sleep(100 * time.Millisecond)
			cancel()
			Eventually(done, 5*time.Second).Should(Receive(BeNil()))

			// Without readiness probe, Run enters main loop immediately.
			// ReaderFor is only called during syncAll (1 call per kind).
			Expect(attempts).To(BeNumerically("<=", 6),
				"Without probe, only syncAll calls ReaderFor (1 per kind)")
		})

		It("UT-FLEET-FMC-012 [SC-5]: syncer skips readiness probe when no clusters are registered, avoiding unnecessary network probes against an empty registry (BR-FLEET-002)", func() {
			probeAttempts := 0
			factory := fmc.ObservedReaderFactoryFunc(func(_ context.Context, _ registry.ClusterInfo) (fmc.ObservedReader, error) {
				probeAttempts++
				return &spyReader{}, nil
			})

			emptyReg := newStubRegistry() // no clusters
			syncer := fmc.NewSyncerWithReaderFactory(
				emptyReg,
				factory,
				writer,
				fmc.Config{
					SyncInterval:       time.Hour,
					KeyTTL:             30 * time.Second,
					ResourceKinds:      []string{"Pod"},
					WaitForBrokerReady: true,
				},
				logr.Discard(),
				metrics,
			)

			done := make(chan error, 1)
			go func() {
				done <- syncer.Run(ctx)
			}()

			time.Sleep(100 * time.Millisecond)
			cancel()
			Eventually(done, 5*time.Second).Should(Receive(BeNil()))

			Expect(probeAttempts).To(Equal(0),
				"No clusters means no readiness probe, so ReaderFor should not be called")
		})

		It("UT-FLEET-FMC-013 [SI-4]: syncer shuts down cleanly when context is cancelled during readiness probe, ensuring graceful termination during deployment rollouts (BR-FLEET-002)", func() {
			probeCtx, probeCancel := context.WithCancel(context.Background())
			factory := fmc.ObservedReaderFactoryFunc(func(_ context.Context, _ registry.ClusterInfo) (fmc.ObservedReader, error) {
				return nil, fmt.Errorf("broker not ready")
			})

			stubReg := newStubRegistry(registry.ClusterInfo{ID: "cluster-a"})
			syncer := fmc.NewSyncerWithReaderFactory(
				stubReg,
				factory,
				writer,
				fmc.Config{
					SyncInterval:               time.Hour,
					KeyTTL:                     30 * time.Second,
					ResourceKinds:              []string{"Pod"},
					WaitForBrokerReady:         true,
					BrokerProbeInitialInterval: 5 * time.Millisecond,
					BrokerProbeMaxInterval:     20 * time.Millisecond,
					BrokerProbeTimeout:         5 * time.Second,
				},
				logr.Discard(),
				metrics,
			)

			done := make(chan error, 1)
			go func() {
				done <- syncer.Run(probeCtx)
			}()

			time.Sleep(50 * time.Millisecond)
			probeCancel()

			Eventually(done, 5*time.Second).Should(Receive(BeNil()))
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
				fmc.ObservedReaderFactoryFunc(func(_ context.Context, _ registry.ClusterInfo) (fmc.ObservedReader, error) {
					return spy, nil
				}),
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
