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

package enrichment_test

import (
	"context"

	"github.com/go-logr/logr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
)

// Issue #2343: Investigator.Investigate()'s pre-fetch enrichment step
// (resolveSpecHash/resolveOwnerChainWithRetry) always used the K8sClient
// passed to NewEnricher, ignoring any fleet-target routing carried on ctx --
// unlike the get_namespaced_resource_context/get_cluster_resource_context
// tools (issue #2306), which already resolve per-call from ctx via
// resolveK8sClient. WithK8sResolver closes that gap by letting a caller
// install a per-call override; production wiring (buildEnricher in
// cmd/kubernautagent/datastorage.go) supplies a closure backed by
// custom.ResolveK8sClient -- proven end to end by the IT in
// test/integration/kubernautagent/investigator/fleet_enrichment_prefetch_it_test.go.
var _ = Describe("Enricher.WithK8sResolver — #2343", func() {

	var (
		logger     = logr.Discard()
		auditStore *recordingAuditStore
		hub        *fakeK8sClient
		overlay    *fakeK8sClient
	)

	BeforeEach(func() {
		auditStore = &recordingAuditStore{}
		hub = &fakeK8sClient{
			ownerChain: []enrichment.OwnerChainEntry{{Kind: "Deployment", Name: "hub-should-not-appear", Namespace: "production"}},
			specHash:   "sha256:hub-hash",
		}
		overlay = &fakeK8sClient{
			ownerChain: []enrichment.OwnerChainEntry{{Kind: "Deployment", Name: "remote-target", Namespace: "demo-checkout"}},
			specHash:   "sha256:overlay-hash",
		}
	})

	Describe("UT-KA-2343-001: a resolved override client is used for owner-chain and spec-hash instead of the hub client", func() {
		It("routes GetOwnerChain/GetSpecHash through the resolver's returned client, never the hub client passed to NewEnricher", func() {
			ds := &fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{}}
			e := enrichment.NewEnricher(hub, ds, auditStore, logger).WithK8sResolver(func(_ context.Context) enrichment.K8sClient {
				return overlay
			})

			result, err := e.Enrich(context.Background(), enrichment.EnrichRequest{
				Kind: "Deployment", Name: "remote-target", Namespace: "demo-checkout",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.OwnerChain).To(HaveLen(1))
			Expect(result.OwnerChain[0].Name).To(Equal("remote-target"),
				"UT-KA-2343-001: owner chain must come from the resolver's overlay client, not the hub client")
			Expect(ds.capturedSpecHash).To(Equal("sha256:overlay-hash"),
				"UT-KA-2343-001: specHash must be auto-computed via the resolver's overlay client, not the hub client")
		})
	})

	Describe("UT-KA-2343-002: no resolver installed falls back to the hub client (zero regression)", func() {
		It("uses the hub client passed to NewEnricher when WithK8sResolver is never called", func() {
			ds := &fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{}}
			e := enrichment.NewEnricher(hub, ds, auditStore, logger)

			result, err := e.Enrich(context.Background(), enrichment.EnrichRequest{
				Kind: "Deployment", Name: "hub-should-not-appear", Namespace: "production",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.OwnerChain).To(HaveLen(1))
			Expect(result.OwnerChain[0].Name).To(Equal("hub-should-not-appear"))
			Expect(ds.capturedSpecHash).To(Equal("sha256:hub-hash"))
		})
	})
})
