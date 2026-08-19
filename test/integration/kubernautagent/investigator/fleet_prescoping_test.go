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
	"errors"

	"github.com/go-logr/logr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// fleetOverlayResolverSpy is a test double for investigator.FleetOverlayResolver
// that records every clusterID it was asked to resolve, so IT-KA-FLEET-013 can
// assert Investigate() calls (or skips) pre-scoping at the right time, without
// a real MCP gateway.
type fleetOverlayResolverSpy struct {
	calls   []string
	overlay map[string]tools.Tool
	err     error
}

func (s *fleetOverlayResolverSpy) Overlay(_ context.Context, clusterID string) (map[string]tools.Tool, error) {
	s.calls = append(s.calls, clusterID)
	return s.overlay, s.err
}

// BR-INTEGRATION-1489, DD-FLEET-005: cluster-transparent tool exposure — wiring
// tier (IT). Proves the production entry point (Investigator.Investigate)
// actually calls the configured FleetOverlayResolver for fleet-target
// investigations, resolves generic tool names to the overlay's BridgeTool
// ahead of the local registry, and never exposes the removed LLM-facing
// discovery tools. Pure decision logic is covered separately by
// UT-KA-FLEET-014/018 (fleet_overlay_internal_test.go).
var _ = Describe("Fleet cluster-transparent tool pre-scoping (BR-INTEGRATION-1489, DD-FLEET-005)", Label("fleet", "integration"), func() {

	var (
		invLogger  logr.Logger
		auditStore *capturingAuditStore
	)

	BeforeEach(func() {
		invLogger = logr.Discard()
		auditStore = newCapturingAuditStore(suiteAuditStore)
	})

	Describe("IT-KA-FLEET-013 [AC-4/AC-6]: Investigate() pre-scopes via FleetOverlayResolver", func() {
		It("calls Overlay with the investigation's target ClusterID for a fleet-target investigation", func() {
			spy := &fleetOverlayResolverSpy{overlay: map[string]tools.Tool{}}
			mockClient := &mockLLMClient{responses: []llm.ChatResponse{
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
			}}
			enricher := enrichment.NewEnricher(&k8sFixtureClient{}, suiteDSAdapter, auditStore, invLogger)
			builder, _ := prompt.NewBuilder()
			rp := parser.NewResultParser()

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15,
				PhaseTools: investigator.DefaultPhaseToolMap(), Registry: registry.New(),
				FleetOverlayResolver: spy,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production", ClusterID: "remote-east",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(spy.calls).To(ConsistOf("remote-east"),
				"IT-KA-FLEET-013: Investigate() must resolve the fleet overlay exactly once, "+
					"for the investigation's own target cluster (AC-4: no LLM-driven cluster choice)")
		})

		It("never calls Overlay for a hub-local investigation (empty ClusterID)", func() {
			spy := &fleetOverlayResolverSpy{overlay: map[string]tools.Tool{}}
			mockClient := &mockLLMClient{responses: []llm.ChatResponse{
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
			}}
			enricher := enrichment.NewEnricher(&k8sFixtureClient{}, suiteDSAdapter, auditStore, invLogger)
			builder, _ := prompt.NewBuilder()
			rp := parser.NewResultParser()

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15,
				PhaseTools: investigator.DefaultPhaseToolMap(), Registry: registry.New(),
				FleetOverlayResolver: spy,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(spy.calls).To(BeEmpty(),
				"IT-KA-FLEET-013: a hub-local investigation (no ClusterID) must never invoke the fleet "+
					"resolver — zero behavior change for non-fleet deployments")
		})
	})

	Describe("IT-KA-FLEET-015 [AC-6]: generic tool names resolve to the overlay's tool ahead of the local registry", func() {
		It("routes a tool call to the overlay's BridgeTool stand-in, under the exact same name the local registry uses", func() {
			overlayTool := &fakeTool{name: "kubectl_get_by_name", result: `{"source":"remote-cluster-east"}`}
			localTool := &fakeTool{name: "kubectl_get_by_name", result: `{"source":"local-hub"}`}
			spy := &fleetOverlayResolverSpy{overlay: map[string]tools.Tool{"kubectl_get_by_name": overlayTool}}

			reg := registry.New()
			reg.Register(localTool)

			mockClient := &mockLLMClient{responses: []llm.ChatResponse{
				{
					Message:   llm.Message{Role: "assistant", Content: ""},
					ToolCalls: []llm.ToolCall{{ID: "tc_get", Name: "kubectl_get_by_name", Arguments: `{}`}},
				},
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
			}}
			enricher := enrichment.NewEnricher(&k8sFixtureClient{}, suiteDSAdapter, auditStore, invLogger)
			builder, _ := prompt.NewBuilder()
			rp := parser.NewResultParser()

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15,
				PhaseTools: investigator.DefaultPhaseToolMap(), Registry: reg,
				FleetOverlayResolver: spy,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production", ClusterID: "remote-east",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(allMessageContent(mockClient.calls[1].Messages)).To(ContainSubstring("remote-cluster-east"),
				"IT-KA-FLEET-015: a fleet-target investigation must route 'kubectl_get_by_name' to the "+
					"overlay's BridgeTool, not the local registry's tool of the same name")
			Expect(allMessageContent(mockClient.calls[1].Messages)).NotTo(ContainSubstring("local-hub"))
		})

		It("falls back to the local registry for the same generic name in a hub-local investigation", func() {
			localTool := &fakeTool{name: "kubectl_get_by_name", result: `{"source":"local-hub"}`}
			spy := &fleetOverlayResolverSpy{overlay: map[string]tools.Tool{}}

			reg := registry.New()
			reg.Register(localTool)

			mockClient := &mockLLMClient{responses: []llm.ChatResponse{
				{
					Message:   llm.Message{Role: "assistant", Content: ""},
					ToolCalls: []llm.ToolCall{{ID: "tc_get", Name: "kubectl_get_by_name", Arguments: `{}`}},
				},
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
			}}
			enricher := enrichment.NewEnricher(&k8sFixtureClient{}, suiteDSAdapter, auditStore, invLogger)
			builder, _ := prompt.NewBuilder()
			rp := parser.NewResultParser()

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15,
				PhaseTools: investigator.DefaultPhaseToolMap(), Registry: reg,
				FleetOverlayResolver: spy,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(allMessageContent(mockClient.calls[1].Messages)).To(ContainSubstring("local-hub"),
				"IT-KA-FLEET-015: with no overlay (hub-local), the same generic name must resolve to the "+
					"local registry unchanged (zero regression)")
		})
	})

	Describe("IT-KA-FLEET-020 [AU-3]: a failed overlay resolution fails open and is independently observable via audit", func() {
		It("proceeds with the investigation and records an EventTypeFleetOverlayFailed audit event carrying the cluster and correlation IDs", func() {
			resolveErr := errors.New("fleet gateway unreachable")
			spy := &fleetOverlayResolverSpy{err: resolveErr}
			mockClient := &mockLLMClient{responses: []llm.ChatResponse{
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
			}}
			enricher := enrichment.NewEnricher(&k8sFixtureClient{}, suiteDSAdapter, auditStore, invLogger)
			builder, _ := prompt.NewBuilder()
			rp := parser.NewResultParser()

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15,
				PhaseTools: investigator.DefaultPhaseToolMap(), Registry: registry.New(),
				FleetOverlayResolver: spy,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production", ClusterID: "remote-east",
				RemediationID: "rem-fleet-overlay-failed-001",
			})
			Expect(err).NotTo(HaveOccurred(),
				"IT-KA-FLEET-020: a degraded fleet dependency must fail open — the investigation "+
					"itself must still complete (GA Readiness Dimension 12: no silent failures, but "+
					"also no investigation-aborting cascading failures)")

			var failureEvents []*audit.AuditEvent
			for _, ev := range auditStore.events {
				if ev.EventType == audit.EventTypeFleetOverlayFailed {
					failureEvents = append(failureEvents, ev)
				}
			}
			Expect(failureEvents).To(HaveLen(1),
				"IT-KA-FLEET-020: exactly one EventTypeFleetOverlayFailed audit event must be recorded "+
					"for the one failed Overlay() resolution")
			Expect(failureEvents[0].EventAction).To(Equal(audit.ActionFleetOverlayFailed))
			Expect(failureEvents[0].EventOutcome).To(Equal(audit.OutcomeFailure))
			Expect(failureEvents[0].ClusterID).To(Equal("remote-east"),
				"AU-3: the audit event must carry the cluster ID the overlay resolution failed for")
			Expect(failureEvents[0].CorrelationID).To(Equal("rem-fleet-overlay-failed-001"),
				"CC8.1: the audit event must be queryable by the investigation's own correlation ID, "+
					"so a degraded fleet investigation's audit trail is reconstructable like any other")
			Expect(failureEvents[0].Data["error_message"]).To(Equal(resolveErr.Error()))
		})
	})

	Describe("IT-KA-FLEET-021 [AC-6]: a tool name absent from both the fleet overlay and the local registry surfaces cluster context in the error", func() {
		It("wraps the not-found error with the tool name and target cluster ID when an overlay is active", func() {
			spy := &fleetOverlayResolverSpy{overlay: map[string]tools.Tool{
				"kubectl_get_by_name": &fakeTool{name: "kubectl_get_by_name", result: `{"source":"remote-cluster-east"}`},
			}}

			mockClient := &mockLLMClient{responses: []llm.ChatResponse{
				{
					Message:   llm.Message{Role: "assistant", Content: ""},
					ToolCalls: []llm.ToolCall{{ID: "tc_miss", Name: "totally_unknown_tool", Arguments: `{}`}},
				},
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
			}}
			enricher := enrichment.NewEnricher(&k8sFixtureClient{}, suiteDSAdapter, auditStore, invLogger)
			builder, _ := prompt.NewBuilder()
			rp := parser.NewResultParser()

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15,
				PhaseTools: investigator.DefaultPhaseToolMap(), Registry: registry.New(),
				FleetOverlayResolver: spy,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production", ClusterID: "remote-east",
			})
			Expect(err).NotTo(HaveOccurred(),
				"IT-KA-FLEET-021: a tool-not-found result is returned to the LLM as a tool error "+
					"message, not surfaced as an Investigate() error")

			toolErrorContent := allMessageContent(mockClient.calls[1].Messages)
			Expect(toolErrorContent).To(ContainSubstring("totally_unknown_tool"))
			Expect(toolErrorContent).To(ContainSubstring("not found"))
			Expect(toolErrorContent).To(ContainSubstring("remote-east"),
				"IT-KA-FLEET-021: for a fleet-target investigation, a not-found error must name the "+
					"target cluster, so it's distinguishable from a tool name that was never valid at "+
					"all versus one simply not exposed for this cluster (AC-6)")
		})

		It("leaves the plain registry.ErrToolNotFound message unwrapped for a hub-local investigation (no overlay)", func() {
			spy := &fleetOverlayResolverSpy{overlay: map[string]tools.Tool{}}

			mockClient := &mockLLMClient{responses: []llm.ChatResponse{
				{
					Message:   llm.Message{Role: "assistant", Content: ""},
					ToolCalls: []llm.ToolCall{{ID: "tc_miss", Name: "totally_unknown_tool", Arguments: `{}`}},
				},
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
			}}
			enricher := enrichment.NewEnricher(&k8sFixtureClient{}, suiteDSAdapter, auditStore, invLogger)
			builder, _ := prompt.NewBuilder()
			rp := parser.NewResultParser()

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15,
				PhaseTools: investigator.DefaultPhaseToolMap(), Registry: registry.New(),
				FleetOverlayResolver: spy,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production",
			})
			Expect(err).NotTo(HaveOccurred())

			toolErrorContent := allMessageContent(mockClient.calls[1].Messages)
			Expect(toolErrorContent).To(ContainSubstring("tool not found: totally_unknown_tool"),
				"IT-KA-FLEET-021: a hub-local investigation has no overlay in context, so the plain "+
					"registry.ErrToolNotFound message must pass through unwrapped (zero regression)")
		})
	})

	// Issue #1729 (DD-FLEET-005 tool-transparency gap): toolDefinitionsForPhase
	// previously only ever *overrode* an existing local-registry tool entry
	// with the overlay's BridgeTool when both shared the exact same name (see
	// IT-KA-FLEET-015 above) -- it never added an overlay tool that had no
	// local-registry namesake at all. kube-mcp-server's own tool naming
	// convention (resources_get/resources_list/..., see
	// pkg/fleet/mcpclient/tool_names.go) never collides with KA's local
	// k8s-tool naming convention (kubectl_get_by_name/kubectl_list/...), so in
	// practice this meant fleet-only tools were silently never advertised to
	// the LLM at all -- present in the resolved overlay, invisible in the
	// schema, permanently unreachable regardless of Helm/gateway wiring.
	Describe("IT-KA-FLEET-024 [AC-6]: overlay tools with no local-registry namesake are appended to the RCA-phase tool schema", func() {
		It("includes a fleet-only overlay tool name in the RCA-phase schema for a fleet-target investigation", func() {
			spy := &fleetOverlayResolverSpy{overlay: map[string]tools.Tool{
				"resources_get": &fakeTool{name: "resources_get", result: `{"source":"remote-cluster-east"}`},
			}}
			mockClient := &mockLLMClient{responses: []llm.ChatResponse{
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
			}}
			enricher := enrichment.NewEnricher(&k8sFixtureClient{}, suiteDSAdapter, auditStore, invLogger)
			builder, _ := prompt.NewBuilder()
			rp := parser.NewResultParser()

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15,
				PhaseTools: investigator.DefaultPhaseToolMap(), Registry: registry.New(),
				FleetOverlayResolver: spy,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production", ClusterID: "remote-east",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(mockClient.calls).NotTo(BeEmpty())
			Expect(toolNamesFromCall(mockClient.calls[0])).To(ContainElement("resources_get"),
				"IT-KA-FLEET-024: a fleet-only overlay tool with no local-registry namesake must still be "+
					"advertised to the LLM for a fleet-target investigation -- otherwise it is discoverable "+
					"by executeResolved but permanently uncallable because the LLM never learns it exists")
		})

		It("never advertises the fleet-only tool name for a hub-local investigation (no overlay resolved)", func() {
			spy := &fleetOverlayResolverSpy{overlay: map[string]tools.Tool{
				"resources_get": &fakeTool{name: "resources_get", result: `{"source":"remote-cluster-east"}`},
			}}
			mockClient := &mockLLMClient{responses: []llm.ChatResponse{
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
			}}
			enricher := enrichment.NewEnricher(&k8sFixtureClient{}, suiteDSAdapter, auditStore, invLogger)
			builder, _ := prompt.NewBuilder()
			rp := parser.NewResultParser()

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15,
				PhaseTools: investigator.DefaultPhaseToolMap(), Registry: registry.New(),
				FleetOverlayResolver: spy,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(mockClient.calls).NotTo(BeEmpty())
			Expect(toolNamesFromCall(mockClient.calls[0])).NotTo(ContainElement("resources_get"),
				"IT-KA-FLEET-024: a hub-local investigation never resolves an overlay, so no fleet-only "+
					"tool name may leak into the schema -- zero behavior change for non-fleet deployments")
		})
	})

	Describe("IT-KA-FLEET-017 [AC-4/AC-6]: RCA phase never exposes the removed discovery tools", func() {
		It("never lists list_clusters or list_tools_for_cluster in the RCA phase tool schema, fleet-configured or not", func() {
			spy := &fleetOverlayResolverSpy{overlay: map[string]tools.Tool{"kubectl_get_by_name": &fakeTool{name: "kubectl_get_by_name", result: "{}"}}}
			mockClient := &mockLLMClient{responses: []llm.ChatResponse{
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
			}}
			enricher := enrichment.NewEnricher(&k8sFixtureClient{}, suiteDSAdapter, auditStore, invLogger)
			builder, _ := prompt.NewBuilder()
			rp := parser.NewResultParser()

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15,
				PhaseTools: investigator.DefaultPhaseToolMap(), Registry: registry.New(),
				FleetOverlayResolver: spy,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production", ClusterID: "remote-east",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(mockClient.calls).NotTo(BeEmpty())
			Expect(toolNamesFromCall(mockClient.calls[0])).NotTo(ContainElement("list_clusters"),
				"IT-KA-FLEET-017: list_clusters must never be exposed to the LLM (AC-4: the LLM never "+
					"chooses which cluster to look at)")
			Expect(toolNamesFromCall(mockClient.calls[0])).NotTo(ContainElement("list_tools_for_cluster"),
				"IT-KA-FLEET-017: list_tools_for_cluster must never be exposed to the LLM (AC-6: pre-scoping "+
					"replaces LLM-driven tool discovery entirely)")
		})
	})

	// Issue #1768 Gap D / Track 2: interactive AF<->KA sessions (RunInteractiveTurn)
	// never applied prescopeFleetOverlay, so a fleet-targeted interactive
	// investigation silently resolved tool calls against the HUB cluster instead
	// of the operator's actual target cluster. InvestigateTool.handleMessage
	// (internal/kubernautagent/mcp/tools/investigate_takeover.go) already resolves
	// SignalContext (including ClusterID) onto ctx via signalResolver on every
	// turn (#1374/F9) -- these specs prove RunInteractiveTurn now consumes that
	// ClusterID the same way Investigate() consumes signal.ClusterID.
	Describe("IT-KA-FLEET-022 [AC-4]: RunInteractiveTurn pre-scopes via FleetOverlayResolver from ctx SignalContext", func() {
		It("calls Overlay with the ClusterID carried on ctx for a fleet-target interactive turn", func() {
			spy := &fleetOverlayResolverSpy{overlay: map[string]tools.Tool{}}
			mockClient := &mockLLMClient{responses: []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: "no root cause identified yet"}},
			}}
			enricher := enrichment.NewEnricher(&k8sFixtureClient{}, suiteDSAdapter, auditStore, invLogger)
			builder, _ := prompt.NewBuilder()
			rp := parser.NewResultParser()

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15,
				PhaseTools: investigator.DefaultPhaseToolMap(), Registry: registry.New(),
				FleetOverlayResolver: spy,
			})

			ctx := katypes.WithSignalContext(context.Background(), katypes.SignalContext{
				ClusterID: "remote-east", RemediationID: "rem-interactive-fleet-001",
			})
			_, err := inv.RunInteractiveTurn(ctx, []llm.Message{
				{Role: "user", Content: "what is wrong with the deployment?"},
			}, "rem-interactive-fleet-001")
			Expect(err).NotTo(HaveOccurred())
			Expect(spy.calls).To(ConsistOf("remote-east"),
				"IT-KA-FLEET-022: an interactive turn for a fleet-target investigation must resolve "+
					"the fleet overlay for the ClusterID carried on ctx (AC-4: enforced against the "+
					"operator's actual target cluster, not silently defaulted to the hub)")
		})

		It("never calls Overlay for an interactive turn with no SignalContext on ctx (hub-local regression safety)", func() {
			spy := &fleetOverlayResolverSpy{overlay: map[string]tools.Tool{}}
			mockClient := &mockLLMClient{responses: []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: "no root cause identified yet"}},
			}}
			enricher := enrichment.NewEnricher(&k8sFixtureClient{}, suiteDSAdapter, auditStore, invLogger)
			builder, _ := prompt.NewBuilder()
			rp := parser.NewResultParser()

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15,
				PhaseTools: investigator.DefaultPhaseToolMap(), Registry: registry.New(),
				FleetOverlayResolver: spy,
			})

			_, err := inv.RunInteractiveTurn(context.Background(), []llm.Message{
				{Role: "user", Content: "what is wrong with the deployment?"},
			}, "rem-interactive-hub-001")
			Expect(err).NotTo(HaveOccurred())
			Expect(spy.calls).To(BeEmpty(),
				"IT-KA-FLEET-022: a hub-local interactive turn (no SignalContext on ctx) must never "+
					"invoke the fleet resolver -- zero behavior change for non-fleet deployments")
		})
	})

	Describe("IT-KA-FLEET-023 [AC-6]: interactive turn tool calls resolve via the overlay ahead of the local registry", func() {
		It("routes a tool call to the overlay's BridgeTool stand-in, under the exact same name the local registry uses", func() {
			overlayTool := &fakeTool{name: "kubectl_get_by_name", result: `{"source":"remote-cluster-east"}`}
			localTool := &fakeTool{name: "kubectl_get_by_name", result: `{"source":"local-hub"}`}
			spy := &fleetOverlayResolverSpy{overlay: map[string]tools.Tool{"kubectl_get_by_name": overlayTool}}

			reg := registry.New()
			reg.Register(localTool)

			mockClient := &mockLLMClient{responses: []llm.ChatResponse{
				{
					Message:   llm.Message{Role: "assistant", Content: ""},
					ToolCalls: []llm.ToolCall{{ID: "tc_get", Name: "kubectl_get_by_name", Arguments: `{}`}},
				},
				{Message: llm.Message{Role: "assistant", Content: "deployment looks healthy"}},
			}}
			enricher := enrichment.NewEnricher(&k8sFixtureClient{}, suiteDSAdapter, auditStore, invLogger)
			builder, _ := prompt.NewBuilder()
			rp := parser.NewResultParser()

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15,
				PhaseTools: investigator.DefaultPhaseToolMap(), Registry: reg,
				FleetOverlayResolver: spy,
			})

			ctx := katypes.WithSignalContext(context.Background(), katypes.SignalContext{
				ClusterID: "remote-east", RemediationID: "rem-interactive-fleet-002",
			})
			_, err := inv.RunInteractiveTurn(ctx, []llm.Message{
				{Role: "user", Content: "check the deployment status"},
			}, "rem-interactive-fleet-002")
			Expect(err).NotTo(HaveOccurred())

			Expect(allMessageContent(mockClient.calls[1].Messages)).To(ContainSubstring("remote-cluster-east"),
				"IT-KA-FLEET-023: a fleet-target interactive turn must route 'kubectl_get_by_name' to "+
					"the overlay's BridgeTool, not the local registry's tool of the same name")
			Expect(allMessageContent(mockClient.calls[1].Messages)).NotTo(ContainSubstring("local-hub"))
		})

		It("falls back to the local registry for the same generic name in a hub-local interactive turn", func() {
			localTool := &fakeTool{name: "kubectl_get_by_name", result: `{"source":"local-hub"}`}
			spy := &fleetOverlayResolverSpy{overlay: map[string]tools.Tool{}}

			reg := registry.New()
			reg.Register(localTool)

			mockClient := &mockLLMClient{responses: []llm.ChatResponse{
				{
					Message:   llm.Message{Role: "assistant", Content: ""},
					ToolCalls: []llm.ToolCall{{ID: "tc_get", Name: "kubectl_get_by_name", Arguments: `{}`}},
				},
				{Message: llm.Message{Role: "assistant", Content: "deployment looks healthy"}},
			}}
			enricher := enrichment.NewEnricher(&k8sFixtureClient{}, suiteDSAdapter, auditStore, invLogger)
			builder, _ := prompt.NewBuilder()
			rp := parser.NewResultParser()

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15,
				PhaseTools: investigator.DefaultPhaseToolMap(), Registry: reg,
				FleetOverlayResolver: spy,
			})

			_, err := inv.RunInteractiveTurn(context.Background(), []llm.Message{
				{Role: "user", Content: "check the deployment status"},
			}, "rem-interactive-hub-002")
			Expect(err).NotTo(HaveOccurred())
			Expect(allMessageContent(mockClient.calls[1].Messages)).To(ContainSubstring("local-hub"),
				"IT-KA-FLEET-023: with no overlay (hub-local), the same generic name must resolve to "+
					"the local registry unchanged (zero regression)")
		})
	})

	// QE readiness audit follow-up (PR #1799 Finding #2, tracked as #1834): UT-KA-FLEET-028
	// (fleet_overlay_internal_test.go) proves prescopeFleetOverlay's own
	// nil-resolver decision in isolation, calling it directly on a
	// hand-built *Investigator. These IT specs close the remaining wiring
	// gap: proving the SAME observability holds when a fleet-target
	// investigation/interactive turn reaches a KA instance with
	// FleetOverlayResolver simply never configured (the zero value,
	// investigator.Config{} without the field set) via the actual
	// production entry points (Investigate/RunInteractiveTurn), not a
	// hand-constructed *Investigator struct literal.
	Describe("IT-KA-FLEET-029 [AU-3, GA Readiness Dim. 12]: an unconfigured FleetOverlayResolver is observable through the production entry points", func() {
		It("Investigate() emits EventTypeFleetOverlayUnavailable for a fleet-target investigation when FleetOverlayResolver is unset", func() {
			mockClient := &mockLLMClient{responses: []llm.ChatResponse{
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
			}}
			enricher := enrichment.NewEnricher(&k8sFixtureClient{}, suiteDSAdapter, auditStore, invLogger)
			builder, _ := prompt.NewBuilder()
			rp := parser.NewResultParser()

			// FleetOverlayResolver deliberately omitted (nil) -- the exact
			// condition of a KA instance that never had fleet mode wired.
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15,
				PhaseTools: investigator.DefaultPhaseToolMap(), Registry: registry.New(),
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production", ClusterID: "remote-east",
				RemediationID: "rem-fleet-overlay-unavailable-001",
			})
			Expect(err).NotTo(HaveOccurred(),
				"IT-KA-FLEET-029: an unconfigured fleet resolver must fail open through the real "+
					"Investigate() entry point too, exactly like a resolver error does (IT-KA-FLEET-020)")

			var unavailableEvents []*audit.AuditEvent
			for _, ev := range auditStore.events {
				if ev.EventType == audit.EventTypeFleetOverlayUnavailable {
					unavailableEvents = append(unavailableEvents, ev)
				}
			}
			Expect(unavailableEvents).To(HaveLen(1),
				"IT-KA-FLEET-029: Investigate() must record exactly one EventTypeFleetOverlayUnavailable "+
					"audit event when it reaches an unconfigured FleetOverlayResolver, proving the wiring "+
					"(not just the unit-level decision function) is observable end-to-end")
			Expect(unavailableEvents[0].EventAction).To(Equal(audit.ActionFleetOverlayUnavailable))
			Expect(unavailableEvents[0].ClusterID).To(Equal("remote-east"))
			Expect(unavailableEvents[0].CorrelationID).To(Equal("rem-fleet-overlay-unavailable-001"),
				"CC8.1: reconstructable by the investigation's own correlation ID, like IT-KA-FLEET-020")
		})

		It("RunInteractiveTurn emits EventTypeFleetOverlayUnavailable for a fleet-target interactive turn when FleetOverlayResolver is unset", func() {
			mockClient := &mockLLMClient{responses: []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", Content: "no root cause identified yet"}},
			}}
			enricher := enrichment.NewEnricher(&k8sFixtureClient{}, suiteDSAdapter, auditStore, invLogger)
			builder, _ := prompt.NewBuilder()
			rp := parser.NewResultParser()

			// FleetOverlayResolver deliberately omitted (nil), mirroring the
			// Investigate() case above for the interactive entry point.
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15,
				PhaseTools: investigator.DefaultPhaseToolMap(), Registry: registry.New(),
			})

			ctx := katypes.WithSignalContext(context.Background(), katypes.SignalContext{
				ClusterID: "remote-east", RemediationID: "rem-interactive-fleet-unavailable-001",
			})
			_, err := inv.RunInteractiveTurn(ctx, []llm.Message{
				{Role: "user", Content: "what is wrong with the deployment?"},
			}, "rem-interactive-fleet-unavailable-001")
			Expect(err).NotTo(HaveOccurred())

			var unavailableEvents []*audit.AuditEvent
			for _, ev := range auditStore.events {
				if ev.EventType == audit.EventTypeFleetOverlayUnavailable {
					unavailableEvents = append(unavailableEvents, ev)
				}
			}
			Expect(unavailableEvents).To(HaveLen(1),
				"IT-KA-FLEET-029: RunInteractiveTurn must record exactly one "+
					"EventTypeFleetOverlayUnavailable audit event for a fleet-target interactive turn "+
					"reaching an unconfigured FleetOverlayResolver")
			Expect(unavailableEvents[0].ClusterID).To(Equal("remote-east"))
			Expect(unavailableEvents[0].CorrelationID).To(Equal("rem-interactive-fleet-unavailable-001"))
		})

		It("emits nothing for a hub-local investigation even when FleetOverlayResolver is unset (zero-regression no-op)", func() {
			mockClient := &mockLLMClient{responses: []llm.ChatResponse{
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
			}}
			enricher := enrichment.NewEnricher(&k8sFixtureClient{}, suiteDSAdapter, auditStore, invLogger)
			builder, _ := prompt.NewBuilder()
			rp := parser.NewResultParser()

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp, Enricher: enricher,
				AuditStore: auditStore, Logger: invLogger, MaxTurns: 15,
				PhaseTools: investigator.DefaultPhaseToolMap(), Registry: registry.New(),
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "api-server-abc", Namespace: "production",
			})
			Expect(err).NotTo(HaveOccurred())
			for _, ev := range auditStore.events {
				Expect(ev.EventType).NotTo(Equal(audit.EventTypeFleetOverlayUnavailable),
					"IT-KA-FLEET-029: a hub-local investigation (no target cluster) must stay silent "+
						"even with FleetOverlayResolver unset -- this is the expected, unchanged "+
						"zero-regression path for the overwhelming majority of (non-fleet) deployments")
			}
		})
	})
})
