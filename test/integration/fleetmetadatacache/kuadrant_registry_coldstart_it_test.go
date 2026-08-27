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
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

// IT-REG-GW-004 [AC-3, SI-4]: cold-start discovery of pre-existing managed
// resources must be complete synchronously the instant Start() returns --
// no Eventually, no retry, zero grace period.
//
// Guards the gap identified in #2299/#2298: cache.WaitForCacheSync only
// guarantees the reflector's initial List has populated the informer's
// internal indexer; it does NOT guarantee the sharedProcessor has finished
// dispatching AddFunc (onAdd) for every pre-existing object -- that dispatch
// runs on independent processorListener goroutines with no ordering
// guarantee relative to WaitForCacheSync returning. Both ClusterRegistry
// implementations (Kuadrant and EAIGW) share this gap, so both are covered
// here against the real envtest API server.
//
// RED-phase outcome (documented, #2299): against envtest's fast in-process
// API server with only 10 objects and no handler-side delay, dispatch
// latency is low enough that this test passes today more often than not --
// matching the earlier throwaway spike's finding (0/30 trials showed a gap
// with zero artificial delay, vs. 30/30 with a 5ms handler delay). The gap
// is real and structural (confirmed by that spike and by direct client-go
// source review) but not reliably reproducible in a fast, low-object-count
// integration test without injecting delay into production code. This
// test guards the contract going forward (GREEN's seed-from-indexer fix
// makes it correct by construction, not by luck) -- the primary
// regression-proving evidence for the race itself is the UT run under
// go test -race -count=50 in pkg/fleet/registry/kuadrant_registry_test.go.
//
// Assertion note: this suite's specs share one real envtest API server
// cluster-wide (each Registry here watches all namespaces), and other
// specs in this package (e.g. fmc_e2e_test.go's Ordered "e2e-cluster"
// Backend) hold their own long-lived managed objects for their lifetime.
// Under parallel (--procs) execution those specs can run concurrently
// with this one, so asserting an exact List() length is a real flake --
// confirmed by a `make test-integration-fleetmetadatacache` run: passes
// every time in isolation (--focus, single proc) but failed the
// EAIGWRegistry variant once under the default parallel Makefile target.
// Asserting containment of every name this test created (not exact
// count) proves the same completeness contract -- zero grace period,
// no Eventually -- without depending on being the only spec with
// managed objects live in the shared cluster at that instant.
var _ = Describe("IT-REG-GW-004 [AC-3, SI-4]: cold-start discovery of pre-existing objects (BR-INTEGRATION-065)", Label("fmc", "integration"), func() {
	const numObjects = 10

	Describe("KuadrantRegistry", func() {
		var names []string

		BeforeEach(func() {
			names = make([]string, numObjects)
			for i := 0; i < numObjects; i++ {
				name := fmt.Sprintf("it-reg-gw-004-kuadrant-%d", i)
				names[i] = name
				createMCPServerRegistration(context.Background(), name)
			}
		})

		AfterEach(func() {
			for _, name := range names {
				deleteMCPServerRegistration(context.Background(), name)
			}
		})

		It("discovers all pre-existing MCPServerRegistrations synchronously when Start() returns", func() {
			reg := registry.NewKuadrantRegistry(dynClient, registry.EAIGWRegistryConfig{}, nil, logr.Discard())
			defer reg.Stop()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			Expect(reg.Start(ctx)).To(Succeed(), "KuadrantRegistry should start against envtest")

			clusters := reg.List()
			Expect(clusterIDs(clusters)).To(ContainElements(names),
				"IT-REG-GW-004: cold-start discovery must be complete the instant Start() returns, "+
					"not eventually -- expected all %d pre-existing clusters with zero grace period, got %v",
				numObjects, clusterIDs(clusters))
		})
	})

	Describe("EAIGWRegistry", func() {
		var names []string

		BeforeEach(func() {
			names = make([]string, numObjects)
			for i := 0; i < numObjects; i++ {
				name := fmt.Sprintf("it-reg-gw-004-eaigw-%d", i)
				names[i] = name
				createBackend(context.Background(), name)
			}
		})

		AfterEach(func() {
			for _, name := range names {
				deleteBackend(context.Background(), name)
			}
		})

		It("discovers all pre-existing Backends synchronously when Start() returns", func() {
			reg := registry.NewEAIGWRegistry(dynClient, registry.EAIGWRegistryConfig{}, nil, logr.Discard())
			defer reg.Stop()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			Expect(reg.Start(ctx)).To(Succeed(), "EAIGWRegistry should start against envtest")

			clusters := reg.List()
			Expect(clusterIDs(clusters)).To(ContainElements(names),
				"IT-REG-GW-004: cold-start discovery must be complete the instant Start() returns, "+
					"not eventually -- expected all %d pre-existing clusters with zero grace period, got %v",
				numObjects, clusterIDs(clusters))
		})
	})
})

// clusterIDs extracts the ID field from a slice of ClusterInfo for use with
// ContainElements, since exact-count assertions are unsafe here (see
// "Assertion note" above this Describe block).
func clusterIDs(clusters []registry.ClusterInfo) []string {
	ids := make([]string, 0, len(clusters))
	for _, c := range clusters {
		ids = append(ids, c.ID)
	}
	return ids
}
