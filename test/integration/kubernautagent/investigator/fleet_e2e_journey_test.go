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
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	fleetclient "github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	kaopenai "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/openai"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/conversation"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

// fleetE2EMarker is a distinctive, unlikely-to-collide string embedded in the
// mock gateway's tool result. The mock-LLM scenario below inspects
// ctx.AllText for this marker to detect whether the remote tool has already
// executed (i.e. which conversation turn it's building a response for),
// without needing access to raw message internals — mirroring the technique
// used by isPermanentError/mockKeywordScenario's own keyword-in-content
// matching elsewhere in this test double.
const fleetE2EMarker = "kubernaut-fleet-e2e-remote-marker-1732"

// fleetE2EOverlayResolver re-derives cmd/kubernautagent's unexported
// gatewayOverlayResolver.Overlay() recipe (DD-FLEET-004) from exported
// fleetclient primitives: discover the target cluster's tools, then re-key
// each one under its generic (unprefixed) name so the LLM sees the exact
// same tool identity it would for a hub-local investigation. This is the
// same re-derivation already used by IT-KA-FLEET-010/011/012
// (test/integration/kubernautagent/fleet/fleet_wiring_test.go); duplicated
// here because the production type is unexported and this package cannot
// import cmd/kubernautagent (a main package).
type fleetE2EOverlayResolver struct {
	discoverer fleetclient.GatewayDiscoverer
	session    fleetclient.Session
}

func (r *fleetE2EOverlayResolver) Overlay(ctx context.Context, clusterID string) (map[string]tools.Tool, error) {
	defs, err := r.discoverer.ToolsForCluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	overlay := make(map[string]tools.Tool, len(defs))
	for _, def := range defs {
		generic := strings.TrimPrefix(def.Name, clusterID+"__")
		bridge := fleetclient.NewBridgeTool(def, clusterID, r.session)
		overlay[generic] = &fleetE2EGenericNameTool{inner: bridge, name: generic}
	}
	return overlay, nil
}

// fleetE2EGenericNameTool locally mirrors cmd/kubernautagent's unexported
// genericNameTool decorator: it exposes a *fleetclient.BridgeTool to the
// investigator under a generic name while Execute still delegates to the
// inner BridgeTool, which dispatches using the tool's original wire name.
type fleetE2EGenericNameTool struct {
	inner *fleetclient.BridgeTool
	name  string
}

func (g *fleetE2EGenericNameTool) Name() string                { return g.name }
func (g *fleetE2EGenericNameTool) Description() string         { return g.inner.Description() }
func (g *fleetE2EGenericNameTool) Parameters() json.RawMessage { return g.inner.Parameters() }
func (g *fleetE2EGenericNameTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return g.inner.Execute(ctx, args)
}

// fleetE2EScenario is a hand-rolled mock-LLM scenario (rather than a
// KeywordScenarioOverride) because it needs turn-dependent responses: a
// tool call on turn 1, then a full RCA-complete text response on turn 2 that
// bypasses the DAG engine entirely (ForceText) so no other scenario
// machinery (submit_result/split-tool detection) needs to be modeled.
// ConfigForContext (ScenarioWithContextConfig) inspects ctx.AllText for
// fleetE2EMarker -- present only once the remote tool result has flowed
// back into the conversation -- to pick which turn's config to return.
type fleetE2EScenario struct{}

func (fleetE2EScenario) Name() string { return "fleet_e2e_journey_1732" }
func (fleetE2EScenario) Metadata() scenarios.ScenarioMetadata {
	return scenarios.ScenarioMetadata{Name: "fleet_e2e_journey_1732", Description: "E2E-KA-FLEET-001"}
}
func (fleetE2EScenario) DAG() *conversation.DAG { return nil }
func (fleetE2EScenario) Match(ctx *scenarios.DetectionContext) (bool, float64) {
	if strings.Contains(ctx.Content, "fleettransparencyprobe") {
		return true, 1.0
	}
	return false, 0
}
func (fleetE2EScenario) ConfigForContext(ctx *scenarios.DetectionContext) scenarios.MockScenarioConfig {
	if strings.Contains(ctx.AllText, fleetE2EMarker) {
		actionable := true
		return scenarios.MockScenarioConfig{
			ScenarioName:         "fleet_e2e_journey_1732",
			SignalName:           "FleetTransparencyProbe",
			Severity:             "critical",
			RootCause:            "remote pod api-server-abc confirmed OOMKilled via a fleet-transparent tool call routed to cluster remote-east",
			InvestigationOutcome: "actionable",
			IsActionable:         &actionable,
			Confidence:           0.9,
			ForceText:            scenarios.BoolPtr(true),
		}
	}
	return scenarios.MockScenarioConfig{
		ScenarioName: "fleet_e2e_journey_1732",
		ToolCallName: "kubectl_get_by_name",
		ToolCallArgs: map[string]interface{}{
			"kind": "Pod", "name": "api-server-abc", "namespace": "production",
		},
		ForceText: scenarios.BoolPtr(false),
	}
}

var _ = Describe("Fleet cluster-transparent tool exposure — full journey (BR-INTEGRATION-1489, DD-FLEET-004)", Label("fleet", "integration"), func() {

	Describe("E2E-KA-FLEET-001: a real mock-LLM issues a tool call under a generic name during a fleet-target investigation, and it reaches the remote cluster, never the hub-local tool", func() {
		It("routes kubectl_get_by_name to the remote-east mock gateway via the fleet overlay, not to the local registry", func() {
			// --- Remote cluster side: mock MCP gateway exposing exactly one
			// wire-prefixed tool for cluster "remote-east". ---
			inputSchema := json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"},"name":{"type":"string"},"namespace":{"type":"string"}},"required":["kind","name"]}`)
			gw := mockgw.NewMockGateway(mockgw.WithTool(
				"remote-east__kubectl_get_by_name",
				"Get a Kubernetes resource by name from the remote cluster",
				inputSchema,
				func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					text := fmt.Sprintf(`{"marker":%q,"cluster":"remote-east","kind":"Pod","name":"api-server-abc","namespace":"production","status":"OOMKilled"}`, fleetE2EMarker)
					return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil
				},
			))
			defer gw.Close()

			mcpC, err := fleetclient.New(context.Background(), gw.URL())
			Expect(err).NotTo(HaveOccurred())
			defer mcpC.Close()
			session := mcpC.Session()

			disc, err := fleetclient.NewDiscoverer("eaigw", session)
			Expect(err).NotTo(HaveOccurred())

			// --- Mock LLM side: a real HTTP server serving the hand-rolled
			// turn-aware scenario above. ---
			reg2 := scenarios.NewRegistry()
			reg2.Register(fleetE2EScenario{})
			llmServer := httptest.NewServer(handlers.NewRouter(reg2, false, ""))
			defer llmServer.Close()

			llmClient := kaopenai.New("test-model", llmServer.URL, "test-key")
			sw, err := llm.NewSwappableClient(llmClient, "test-model")
			Expect(err).NotTo(HaveOccurred())

			builder, err := prompt.NewBuilder()
			Expect(err).NotTo(HaveOccurred())

			// --- Hub-local side: a local registry with a fakeTool under the
			// SAME generic name the remote overlay will also use. Production
			// only exposes a phase tool to the LLM when it's registered
			// locally (toolDefinitionsForPhase iterates the local phase-tool
			// list and only *overrides* entries also present in the fleet
			// overlay) -- the overlay never adds brand-new tool names. This
			// fakeTool's content ("local-hub") must never appear in the
			// final result: DD-FLEET-004 requires cluster-transparent
			// execution to resolve via the overlay first. ---
			reg := registry.New()
			reg.Register(&fakeTool{name: "kubectl_get_by_name", result: `{"source":"local-hub","warning":"must never be reached for a fleet-target investigation"}`})

			auditStore := newCapturingAuditStore(suiteAuditStore)

			inv := investigator.New(investigator.Config{
				PhaseResolver:        investigator.NewDefaultPhaseResolver(sw, nil),
				Builder:              builder,
				ResultParser:         parser.NewResultParser(),
				AuditStore:           auditStore,
				Logger:               logr.Discard(),
				MaxTurns:             5,
				PhaseTools:           investigator.DefaultPhaseToolMap(),
				Registry:             reg,
				FleetOverlayResolver: &fleetE2EOverlayResolver{discoverer: disc, session: session},
			})

			signal := katypes.SignalContext{
				Name:          "FleetTransparencyProbe",
				Namespace:     "production",
				Severity:      "critical",
				Message:       "OOMKilled",
				ClusterID:     "remote-east",
				ResourceKind:  "Pod",
				ResourceName:  "api-server-abc",
				RemediationID: "rem-e2e-fleet-1732",
				// Interactive: RCA-only short-circuit. This test proves tool
				// routing transparency, not workflow selection, so it stops
				// right after RCA rather than adding a 3rd LLM turn.
				Interactive: true,
			}

			result, err := inv.Investigate(context.Background(), signal)
			Expect(err).NotTo(HaveOccurred(),
				"E2E-KA-FLEET-001: a fleet-target investigation through a real mock-LLM + real mock MCP gateway must complete without error")
			Expect(result).NotTo(BeNil())

			calls := gw.CallLog()
			Expect(calls).To(HaveLen(1),
				"the remote mock gateway must receive exactly one real MCP tool call")
			Expect(calls[0].ToolName).To(Equal("remote-east__kubectl_get_by_name"),
				"DD-FLEET-004: the LLM only ever named the generic tool 'kubectl_get_by_name', "+
					"yet the wire call must reach cluster remote-east's own prefixed tool -- proving "+
					"cluster-transparent resolution end to end through a real MCP client/session, not a mock")

			Expect(result.RCASummary).NotTo(ContainSubstring("local-hub"),
				"the hub-local fakeTool registered under the same generic name must never execute "+
					"for a fleet-target investigation")
		})
	})
})
