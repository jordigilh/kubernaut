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
	return make(chan registry.ClusterEvent)
}

func (r *stubRegistry) Ready() bool { return true }

func (r *stubRegistry) Start(_ context.Context) error { return nil }

func (r *stubRegistry) Stop() {}
