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

import (
	"context"
	"sync"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
)

// countingDiscoverer implements mcpclient.GatewayDiscoverer, counting
// ToolsForCluster calls per clusterID and gating them behind a channel so a
// test can force N concurrent callers to overlap in-flight before any of
// them is allowed to return.
type countingDiscoverer struct {
	mu      sync.Mutex
	calls   map[string]int
	release chan struct{} // closed by the test once all callers have entered
	entered chan struct{} // one send per call that has entered, before it blocks on release
}

func newCountingDiscoverer() *countingDiscoverer {
	return &countingDiscoverer{
		calls:   make(map[string]int),
		release: make(chan struct{}),
		entered: make(chan struct{}, 16),
	}
}

func (d *countingDiscoverer) ListClusters(_ context.Context, _ string) ([]mcpclient.ClusterInfo, error) {
	return nil, nil
}

func (d *countingDiscoverer) ToolsForCluster(_ context.Context, clusterID string) ([]mcpclient.ToolDefinition, error) {
	d.mu.Lock()
	d.calls[clusterID]++
	d.mu.Unlock()

	d.entered <- struct{}{}
	<-d.release // block until the test has fired every concurrent caller

	return []mcpclient.ToolDefinition{{Name: clusterID + "__resources_get"}}, nil
}

func (d *countingDiscoverer) callCount(clusterID string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls[clusterID]
}

// UT-KA-FLEET-022 [SC-5]: gatewayOverlayResolver.Overlay deduplicates
// concurrent ToolsForCluster calls for the same clusterID via
// singleflight, exactly as the deleted ListToolsForClusterTool did before
// DD-FLEET-004 removed the LLM-facing discovery tools. This protects the
// MCP Gateway from a redundant discover_tools/select_tools round trip per
// concurrent investigation when many investigations target the same busy
// cluster at once.
var _ = Describe("gatewayOverlayResolver.Overlay singleflight dedup (DD-FLEET-004, BR-INTEGRATION-1489)", func() {
	It("UT-KA-FLEET-022 [SC-5]: N concurrent Overlay calls for the same cluster trigger exactly one ToolsForCluster call", func() {
		disc := newCountingDiscoverer()
		resolver := &gatewayOverlayResolver{discoverer: disc}

		const concurrency = 5
		var wg sync.WaitGroup
		var successes atomic.Int32
		wg.Add(concurrency)
		for i := 0; i < concurrency; i++ {
			go func() {
				defer wg.Done()
				overlay, err := resolver.Overlay(context.Background(), "prod-east")
				if err == nil && overlay != nil {
					successes.Add(1)
				}
			}()
		}

		// Exactly one of the concurrency goroutines is expected to actually
		// enter ToolsForCluster -- the rest block inside singleflight.Do
		// itself, never reaching countingDiscoverer at all. Waiting for this
		// single signal (rather than a fixed sleep) proves the goroutines
		// were genuinely racing to call Overlay concurrently before any of
		// them could have returned.
		<-disc.entered
		close(disc.release)
		wg.Wait()

		Expect(disc.callCount("prod-east")).To(Equal(1),
			"SC-5: singleflight must collapse concurrent Overlay calls for the same cluster into one ToolsForCluster call")
		Expect(successes.Load()).To(Equal(int32(concurrency)),
			"every concurrent caller must still receive a valid overlay, not just the one that triggered the real call")
	})

	It("UT-KA-FLEET-023: Overlay calls for different clusters are never deduplicated against each other", func() {
		disc := newCountingDiscoverer()
		close(disc.release) // no gating needed; calls run sequentially here
		resolver := &gatewayOverlayResolver{discoverer: disc}

		_, errA := resolver.Overlay(context.Background(), "prod-east")
		_, errB := resolver.Overlay(context.Background(), "prod-west")
		Expect(errA).NotTo(HaveOccurred())
		Expect(errB).NotTo(HaveOccurred())

		Expect(disc.callCount("prod-east")).To(Equal(1))
		Expect(disc.callCount("prod-west")).To(Equal(1))
	})
})
