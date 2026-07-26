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

// BR-INTEGRATION-1489, DD-FLEET-004: cluster-transparent tool exposure — wiring
// tier (IT). Proves the production entry point (Investigator.Investigate)
// actually calls the configured FleetOverlayResolver for fleet-target
// investigations, resolves generic tool names to the overlay's BridgeTool
// ahead of the local registry, and never exposes the removed LLM-facing
// discovery tools. Pure decision logic is covered separately by
// UT-KA-FLEET-014/018 (fleet_overlay_internal_test.go).
var _ = Describe("Fleet cluster-transparent tool pre-scoping (BR-INTEGRATION-1489, DD-FLEET-004)", Label("fleet", "integration"), func() {

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
})
