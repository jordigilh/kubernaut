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

// kaToolE2ETargetName is the name (and app label) of the dedicated
// memory-eater-style Deployment E2E-FLEET-017
// (test/e2e/fleet/17_ka_real_fleet_investigation_test.go) deploys, in the
// fixed "kubernaut-system" namespace, on whichever single cluster (primary
// or remote) each test case is exercising.
const kaToolE2ETargetName = "ka-tool-e2e-target"

// kaToolE2ETargetNamespace matches the fleet E2E suite's fixed `namespace`
// constant (test/e2e/fleet/suite_test.go), duplicated here as a literal
// since mock-llm scenarios don't import test/e2e packages (same convention
// as afFleetKubectlRemoteClusterID in scenario_af_fleet_kubectl.go).
const kaToolE2ETargetNamespace = "kubernaut-system"

// kaToolE2EKeyword selects this scenario. A single keyword serves BOTH the
// hub-local and fleet test cases (issue #1729, E2E-FLEET-017): the test
// tells the pipeline nothing about which tool KA should call -- it only
// varies the alert's cluster_id and which cluster the target resource lives
// on -- so the scenario itself must be environment-agnostic too.
const kaToolE2EKeyword = "ka-tool-e2e-test"

// kaToolE2EFleetToolName/LocalToolName are the two candidate tool names this
// scenario may call, matching pkg/fleet/mcpclient's kube-mcp-server naming
// (fleet, reached via the real MCP overlay once DD-FLEET-004's
// tool-transparency fix appends it to the schema) and
// pkg/kubernautagent/tools/k8s's local registry naming (hub-local,
// unaffected by fleet wiring) respectively.
const (
	kaToolE2EFleetToolName = "resources_get"
	kaToolE2ELocalToolName = "kubectl_get_by_name"
)

// kaToolE2ELocalEvidence/RemoteEvidence are the memory limit values
// E2E-FLEET-017 deploys ka-tool-e2e-target with on each cluster (a
// deliberately different value per cluster, NOT the resource name/kind/
// namespace, which are identical everywhere the fixture is deployed and
// therefore would not distinguish "reached the right cluster" from "reached
// the wrong one"). A resource-limit value is invisible to SignalProcessing's
// own enrichment (sharedtypes.KubernetesContext.Workload only carries
// kind/name/labels/annotations, pkg/shared/types/enrichment.go) and to the
// alert payload itself (buildPrometheusAlertWithCluster carries no
// resource-limit fields), so its presence in the final analysis can only
// come from a genuine kubectl_get_by_name/resources_get round trip against
// the correct cluster's live object.
const (
	kaToolE2ELocalEvidence  = "111Mi"
	kaToolE2ERemoteEvidence = "222Mi"
)

// kaToolCallForAvailability picks which single tool call to script based
// strictly on what the caller actually advertised in this request
// (ctx.AvailableTools, sourced from req.Tools) -- never on which test case
// is "supposed" to be running. This is what makes the scenario a genuine,
// strict proof rather than a guess: a real LLM can only ever call a tool it
// was told about, and toolDefinitionsForPhase
// (internal/kubernautagent/investigator/investigator_tools.go) only ever
// advertises resources_get when ctx carries a populated fleet overlay AND
// the tool-transparency fix has appended it to the schema. If neither
// candidate is offered, ok is false and the caller should surface a
// diagnostic RootCause rather than silently guessing -- a misconfigured
// environment must fail loudly, not pass by accident.
//
// resources_get is checked first: a fleet-tagged investigation advertises
// BOTH kubectl_get_by_name (local registry, always present) and
// resources_get (overlay, fleet-only) once the transparency fix ships, and
// the overlay's cluster-scoped tool is the correct one to call for that
// investigation.
func kaToolCallForAvailability(available []string) (toolName string, args map[string]interface{}, expectedEvidence string, ok bool) {
	switch {
	case slices.Contains(available, kaToolE2EFleetToolName):
		return kaToolE2EFleetToolName, map[string]interface{}{
			"kind":       "Deployment",
			"apiVersion": "apps/v1",
			"name":       kaToolE2ETargetName,
			"namespace":  kaToolE2ETargetNamespace,
		}, kaToolE2ERemoteEvidence, true
	case slices.Contains(available, kaToolE2ELocalToolName):
		return kaToolE2ELocalToolName, map[string]interface{}{
			"kind":      "Deployment",
			"name":      kaToolE2ETargetName,
			"namespace": kaToolE2ETargetNamespace,
		}, kaToolE2ELocalEvidence, true
	default:
		return "", nil, "", false
	}
}

// kaToolCallE2EScenario drives KA's real investigation loop (via real
// AIAnalysis reconciliation, not a mocked shortcut) to call one real K8s
// read tool and echo back proof of a genuine, correctly-targeted round
// trip. One scenario, one keyword, for BOTH the hub-local and fleet test
// cases (issue #1729 close-out, E2E-FLEET-017) -- see
// kaToolCallForAvailability for how it stays strictly environment-agnostic.
//
// Turn 1 (no function results yet): emits the tool call
// kaToolCallForAvailability selected as a single-entry MultiToolCalls batch
// (kept as MultiToolCalls rather than ToolCallName for parity with other
// dynamic scenarios, e.g. afFleetKubectlScenario).
//
// Turn 2 (function results present): KA's investigator loop has, by this
// point, already executed the tool for real -- against the local registry
// (hub-local case) or against this suite's real fleet overlay -> MCP
// Gateway -> remote Kind cluster (fleet case) -- and appended the tool
// result to the conversation. ConfigForContext inspects ctx.AllText for the
// environment-specific expectedEvidence value and only reports it found in
// RootCause if present, so the E2E test can assert on genuine,
// correctly-targeted data rather than a canned string that would pass even
// if the tool call silently no-op'd or reached the wrong cluster.
type kaToolCallE2EScenario struct{}

func kaToolCallE2EInvestigationScenario() *kaToolCallE2EScenario {
	return &kaToolCallE2EScenario{}
}

func (s *kaToolCallE2EScenario) Name() string { return "ka_tool_call_e2e" }

func (s *kaToolCallE2EScenario) Metadata() ScenarioMetadata {
	return ScenarioMetadata{
		Name:        "ka_tool_call_e2e",
		Description: "E2E-FLEET-017: real KA investigation calls the correct (local or fleet) read tool and returns genuine, correctly-targeted data",
	}
}

func (s *kaToolCallE2EScenario) DAG() *conversation.DAG { return nil }

func (s *kaToolCallE2EScenario) Match(ctx *DetectionContext) (bool, float64) {
	combined := strings.ToLower(ctx.Content + " " + ctx.AllText)
	if strings.Contains(combined, kaToolE2EKeyword) {
		return true, 0.95
	}
	return false, 0
}

func (s *kaToolCallE2EScenario) ConfigForContext(ctx *DetectionContext) MockScenarioConfig {
	cfg := MockScenarioConfig{
		ScenarioName:         s.Name(),
		ForceText:            BoolPtr(false),
		Severity:             "high",
		InvestigationOutcome: "problem_resolved",
		IsActionable:         BoolPtr(false),
	}

	var available []string
	if ctx != nil {
		available = ctx.AvailableTools
	}
	toolName, args, expectedEvidence, ok := kaToolCallForAvailability(available)
	if !ok {
		cfg.RootCause = fmt.Sprintf(
			"neither %q nor %q was offered in the tool schema -- fleet overlay wiring or local tool registration is broken",
			kaToolE2EFleetToolName, kaToolE2ELocalToolName)
		return cfg
	}
	cfg.MultiToolCalls = []MultiToolCallEntry{{Name: toolName, Arguments: args}}

	// expectedEvidence only appears in ctx.AllText once the tool result
	// (turn 2) has been appended to the conversation with the correct
	// cluster's live object data -- neither the turn-1 alert-derived prompt
	// nor SP's own KubernetesContext enrichment carries a resource-limit
	// value (see kaToolE2ELocal/RemoteEvidence doc comment above).
	if ctx != nil && strings.Contains(ctx.AllText, expectedEvidence) {
		cfg.RootCause = fmt.Sprintf(
			"Verified %s via %s: found expected evidence %q from a genuine, correctly-targeted cluster round trip",
			kaToolE2ETargetName, toolName, expectedEvidence)
	} else {
		cfg.RootCause = fmt.Sprintf(
			"%s call for %s did not return the expected evidence %q -- tool call missing, failed, or reached the wrong cluster",
			toolName, kaToolE2ETargetName, expectedEvidence)
	}
	return cfg
}
