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

package investigator_test

import (
	"context"

	"github.com/go-logr/logr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/tools/custom"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// Issue #2343: Investigate()'s automatic pre-fetch enrichment step
// (resolveEnrichmentCached -> Enricher.Enrich) always resolved owner-chain/
// spec-hash against the hub K8sClient, even for a fleet-target investigation
// -- unlike get_namespaced_resource_context/get_cluster_resource_context
// (issue #2306, proven by fleet_resource_context_it_test.go), which already
// resolve per-call from ctx via custom.ResolveK8sClient. Every existing fleet
// IT in this package built its Enricher with a bare k8sFixtureClient and
// never installed a K8sResolver, so the exact call path that was broken
// (Enricher.k8s inside the pre-fetch, not the LLM-callable tools) was never
// exercised with a fixture that could reveal "wrong cluster" -- these two
// specs close that coverage gap by wiring the Enricher exactly as
// cmd/kubernautagent/datastorage.go's buildEnricher does in production.
var _ = Describe("Fleet-aware enrichment pre-fetch (BR-INTEGRATION-1489, Issue #2343)", Label("fleet", "integration"), func() {

	var (
		invLogger  logr.Logger
		auditStore *capturingAuditStore
	)

	BeforeEach(func() {
		invLogger = logr.Discard()
		auditStore = newCapturingAuditStore(suiteAuditStore)
	})

	// minimalInvestigationResponses is the shortest response sequence that
	// completes Investigate() without exercising any tool calls that would
	// themselves route through the fleet overlay -- isolating this spec to
	// the automatic pre-fetch enrichment step alone (mirrors
	// fleet_prescoping_test.go's IT-KA-FLEET-013 minimal sequence).
	minimalInvestigationResponses := func() []llm.ChatResponse {
		return []llm.ChatResponse{
			{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled","confidence":0.9}`}},
			{
				Message:   llm.Message{Role: "assistant", Content: ""},
				ToolCalls: []llm.ToolCall{{ID: "tc_wf1", Name: "list_available_actions", Arguments: `{}`}},
			},
			{
				Message: llm.Message{Role: "assistant", Content: ""},
				ToolCalls: []llm.ToolCall{
					{ID: "tc_submit", Name: "submit_result_no_workflow", Arguments: `{"root_cause_analysis":{"summary":"OOMKilled"},"reasoning":"none"}`},
				},
			},
		}
	}

	Describe("IT-KA-FLEET-036 [AC-6]: pre-fetch enrichment resolves against the fleet overlay, never the hub K8sClient", func() {
		It("emits an enrichment.completed audit event whose root owner is the overlay's resource, not the hub's", func() {
			// Hub-bound K8sClient: fixed, unrelated owner chain. If the
			// pre-fetch enrichment step ever used this for a fleet-target
			// investigation, its sentinel name would leak into the audit
			// event asserted on below.
			hubK8s := &k8sFixtureClient{ownerChain: []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "hub-should-not-appear", Namespace: "production"},
			}}

			overlayGetTool := &fakeTool{name: "resources_get", result: `{"apiVersion":"apps/v1","kind":"Deployment",` +
				`"metadata":{"name":"remote-target","namespace":"remote-ns"}}`}
			spy := &fleetOverlayResolverSpy{overlay: map[string]tools.Tool{"resources_get": overlayGetTool}}

			mockClient := &mockLLMClient{responses: minimalInvestigationResponses()}

			// WithK8sResolver installed exactly as buildEnricher wires it in
			// production (cmd/kubernautagent/datastorage.go): per-call
			// routing through custom.ResolveK8sClient, not a fixed client.
			enricher := enrichment.NewEnricher(hubK8s, suiteDSAdapter, auditStore, invLogger).
				WithK8sResolver(func(ctx context.Context) enrichment.K8sClient {
					return custom.ResolveK8sClient(ctx, hubK8s, invLogger)
				})
			builder, berr := prompt.NewBuilder()
			Expect(berr).ToNot(HaveOccurred())
			rp := parser.NewResultParser()

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15,
				PhaseTools: investigator.DefaultPhaseToolMap(), Registry: registry.New(),
				FleetOverlayResolver: spy,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "remote-target", Namespace: "remote-ns", ResourceKind: "Deployment", ResourceName: "remote-target",
				ClusterID: "remote-east", RemediationID: "rem-2343-036",
			})
			Expect(err).NotTo(HaveOccurred())

			completedEvents := filterEvents(auditStore.events, audit.EventTypeEnrichmentCompleted)
			Expect(completedEvents).NotTo(BeEmpty(), "IT-KA-FLEET-036: an enrichment.completed audit event must be emitted")
			ev := completedEvents[0]
			Expect(ev.Data["root_owner_name"]).To(Equal("remote-target"),
				"IT-KA-FLEET-036: pre-fetch enrichment must resolve the owner chain against the target "+
					"cluster's own resources_get tool, not fall through to the hub")
			Expect(ev.Data["root_owner_name"]).NotTo(Equal("hub-should-not-appear"),
				"IT-KA-FLEET-036: the hub-bound K8sClient must never be consulted for a fleet-target "+
					"investigation's pre-fetch enrichment (AC-6)")
		})

		It("still resolves against the hub K8sClient for a hub-local investigation (zero regression)", func() {
			hubK8s := &k8sFixtureClient{ownerChain: []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "hub-local-target", Namespace: "production"},
			}}
			mockClient := &mockLLMClient{responses: minimalInvestigationResponses()}

			enricher := enrichment.NewEnricher(hubK8s, suiteDSAdapter, auditStore, invLogger).
				WithK8sResolver(func(ctx context.Context) enrichment.K8sClient {
					return custom.ResolveK8sClient(ctx, hubK8s, invLogger)
				})
			builder, berr := prompt.NewBuilder()
			Expect(berr).ToNot(HaveOccurred())
			rp := parser.NewResultParser()

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15,
				PhaseTools: investigator.DefaultPhaseToolMap(), Registry: registry.New(),
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "hub-local-target", Namespace: "production", ResourceKind: "Deployment", ResourceName: "hub-local-target",
				RemediationID: "rem-2343-036b",
			})
			Expect(err).NotTo(HaveOccurred())

			completedEvents := filterEvents(auditStore.events, audit.EventTypeEnrichmentCompleted)
			Expect(completedEvents).NotTo(BeEmpty())
			ev := completedEvents[0]
			Expect(ev.Data["root_owner_name"]).To(Equal("hub-local-target"),
				"a hub-local investigation (no ClusterID, no overlay) must still resolve via the hub K8sClient")
		})
	})
})
