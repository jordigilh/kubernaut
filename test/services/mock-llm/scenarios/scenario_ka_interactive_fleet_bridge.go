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
package scenarios

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/conversation"
)

// kaInteractiveFleetTargetName/Namespace are the dedicated marker Deployment
// E2E-FLEET-018 (test/e2e/fleet/18_af_ka_interactive_fleet_bridge_test.go,
// issue #1768 Track 2 Gap D) deploys on the REMOTE cluster only. Deliberately
// distinct from kaToolE2ETargetName (scenario_ka_fleet_investigation.go,
// E2E-FLEET-017) so both scenarios' fixtures never collide under parallel
// Ginkgo execution in the same "kubernaut-system" namespace.
const (
	kaInteractiveFleetTargetName      = "ka-interactive-fleet-target"
	kaInteractiveFleetTargetNamespace = "kubernaut-system"
)

// kaInteractiveFleetKeyword selects this scenario. Embedded verbatim in the
// kubernaut_message chat text sent by the E2E test (not a K8s resource name
// or alertname), since RunInteractiveTurn's conversation is built from the
// interactive message history, not from the RemediationRequest/Signal
// fields that back the autonomous RCA prompt.
const kaInteractiveFleetKeyword = "ka-interactive-fleet-e2e-test"

// kaInteractiveFleetConfidence mirrors kaToolE2EConfidence's precedent
// (scenario_ka_fleet_investigation.go): set above the generic YAML keyword
// scenarios (1.0) so this scenario isn't hijacked by a broader match on a
// shared substring in KA's own prompt.
const kaInteractiveFleetConfidence = 1.1

// kaInteractiveFleetEvidence is the memory-limit value the E2E test deploys
// kaInteractiveFleetTargetName with, on the remote cluster only (this
// scenario has no hub-local case: Gap D is specifically about the
// interactive bridge's fleet cluster-scoping, already covered for the
// hub-local/no-overlay path by UT-KA-FLEET-027).
const kaInteractiveFleetEvidence = "247Mi"

const (
	kaInteractiveFleetFleetToolName = "resources_get"
	kaInteractiveFleetLocalToolName = "kubectl_get_by_name"
)

// kaInteractiveFleetToolForAvailability mirrors
// kaToolCallForAvailability's strict tools-actually-offered selection (see
// that function's doc comment for the full rationale): resources_get is
// only ever advertised once RunInteractiveTurn's prescopeFleetOverlay has
// attached the fleet overlay for this specific interactive turn -- the
// exact mechanism #1768 Track 2 Gap D closed.
func kaInteractiveFleetToolForAvailability(available []string) (toolName string, args map[string]interface{}, ok bool) {
	switch {
	case slices.Contains(available, kaInteractiveFleetFleetToolName):
		return kaInteractiveFleetFleetToolName, map[string]interface{}{
			"kind":       "Deployment",
			"apiVersion": "apps/v1",
			"name":       kaInteractiveFleetTargetName,
			"namespace":  kaInteractiveFleetTargetNamespace,
		}, true
	case slices.Contains(available, kaInteractiveFleetLocalToolName):
		return kaInteractiveFleetLocalToolName, map[string]interface{}{
			"kind":      "Deployment",
			"name":      kaInteractiveFleetTargetName,
			"namespace": kaInteractiveFleetTargetNamespace,
		}, true
	default:
		return "", nil, false
	}
}

// kaInteractiveFleetBridgeScenario drives KA's real RunInteractiveTurn loop
// (triggered by AF's kubernaut_message tool, via a real MCP round trip) to
// call one real K8s read tool against the fleet-overlay-resolved cluster
// and echo proof of a genuine, correctly-targeted round trip back as plain
// text -- so it surfaces on the A2A artifact stream (see doc comment on the
// evidence-found branch of ConfigForContext for why plain text, not
// RootCause/submit_result, is required here).
//
// Turn 1 (no function results yet): emits the tool call
// kaInteractiveFleetToolForAvailability selected, as a single-entry
// MultiToolCalls batch (mirrors scenario_ka_fleet_investigation.go).
//
// Turn 2 (function results present): the tool has, by this point, already
// executed for real against this suite's fleet overlay -> MCP Gateway ->
// remote Kind cluster. ConfigForContext inspects ctx.AllText for the
// evidence value and only reports it found once genuinely present.
type kaInteractiveFleetBridgeScenario struct{}

func kaInteractiveFleetBridgeE2EScenario() *kaInteractiveFleetBridgeScenario {
	return &kaInteractiveFleetBridgeScenario{}
}

func (s *kaInteractiveFleetBridgeScenario) Name() string { return "ka_interactive_fleet_bridge_e2e" }

func (s *kaInteractiveFleetBridgeScenario) Metadata() ScenarioMetadata {
	return ScenarioMetadata{
		Name:        "ka_interactive_fleet_bridge_e2e",
		Description: "E2E-FLEET-018: kubernaut_message-triggered RunInteractiveTurn calls the fleet-scoped tool and echoes genuine remote-cluster evidence into the A2A artifact stream",
	}
}

func (s *kaInteractiveFleetBridgeScenario) DAG() *conversation.DAG { return nil }

// Match requires BOTH the keyword AND evidence that this specific LLM call
// is KA's own internal RCA loop, not AF's ADK agent loop -- CI RCA for run
// 30822413481 (job 91718433564, E2E-FLEET-018) proved the keyword alone is
// not a safe discriminator here, unlike kaToolE2EKeyword's precedent
// (scenario_ka_fleet_investigation.go): the message turn's AF-side YAML
// scenario (kaInteractiveFleetBridgeScenarioYAML, "..._message_1768")
// embeds this same literal keyword inside its kubernaut_message
// tool_call's "message" argument, which AF's own ADK agent echoes back
// into ITS OWN conversation history on every subsequent LLM call (observed
// as three "re-invoking agent after text-only turn end" cycles in the
// apifrontend log, immediately preceding the failure). At confidence 1.1
// this scenario out-scored AF's own completion scenarios on those AF-level
// calls too, answering them with a KA-flavored "neither tool offered"
// error -- ctx.AvailableTools on an AF-level call is AF's own ~27
// kubernaut_* tools (confirmed via the mock-llm log's per-match
// "tools=27", identical to the count logged for AF's other turns), never
// KA's own submit_result. Requiring submit_result -- unconditionally
// appended by toolDefinitionsForPhase for every non-workflow-discovery
// phase, and never defined anywhere in AF's own tool package -- confines
// this scenario to KA's own RCA-phase calls.
func (s *kaInteractiveFleetBridgeScenario) Match(ctx *DetectionContext) (bool, float64) {
	combined := strings.ToLower(ctx.Content + " " + ctx.AllText)
	if !strings.Contains(combined, kaInteractiveFleetKeyword) {
		return false, 0
	}
	if !slices.Contains(ctx.AvailableTools, "submit_result") {
		return false, 0
	}
	return true, kaInteractiveFleetConfidence
}

func (s *kaInteractiveFleetBridgeScenario) ConfigForContext(ctx *DetectionContext) MockScenarioConfig {
	cfg := MockScenarioConfig{
		ScenarioName: s.Name(),
		ForceText:    BoolPtr(false),
	}

	var available []string
	if ctx != nil {
		available = ctx.AvailableTools
	}
	toolName, args, ok := kaInteractiveFleetToolForAvailability(available)
	if !ok {
		cfg.ForceText = BoolPtr(true)
		cfg.ExactAnalysisText = fmt.Sprintf(
			"neither %q nor %q was offered in the tool schema -- fleet overlay wiring or local tool registration is broken",
			kaInteractiveFleetFleetToolName, kaInteractiveFleetLocalToolName)
		return cfg
	}

	// evidence only appears in ctx.AllText once the tool result (turn 2) has
	// been appended to the conversation with the correct cluster's live
	// object data.
	if ctx != nil && strings.Contains(ctx.AllText, kaInteractiveFleetEvidence) {
		// #1768 Track 2 Gap D spike finding: EventTypeToolResult has no
		// FormatEventForUser case (pkg/apifrontend/tools/ka_investigate_bridge.go)
		// and is silently dropped from the A2A artifact stream, so the raw
		// tool result can never be what the E2E test asserts on. RootCause
		// (scenario_ka_fleet_investigation.go's pattern) is also the wrong
		// sink here: it feeds submit_result -> the AIAnalysis CR, a
		// completely different consumer than the interactive
		// kubernaut_message A2A artifact stream this test reads via
		// afArtifactText. Only a genuine ForceText+ExactAnalysisText plain
		// response reaches resp.Message.Content, which
		// investigator_loop.go's EventTypeReasoningDelta truncates to 200
		// chars and forwards to launcher.EmitReasoningSafe -- so the
		// evidence value must appear within the first ~200 chars below.
		cfg.ForceText = BoolPtr(true)
		cfg.ExactAnalysisText = fmt.Sprintf(
			"Evidence %s confirmed via %s on the target deployment.", kaInteractiveFleetEvidence, toolName)
		return cfg
	}

	cfg.MultiToolCalls = []MultiToolCallEntry{{Name: toolName, Arguments: args}}
	return cfg
}
