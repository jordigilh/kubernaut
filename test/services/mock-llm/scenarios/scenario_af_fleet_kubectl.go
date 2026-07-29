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
	"strings"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/conversation"
)

// afFleetKubectlE2EKeyword is the distinctive phrase the E2E test's A2A
// message text must contain to select this scenario. Chosen to be unique
// enough to never collide with real SRE-facing prompt text.
const afFleetKubectlE2EKeyword = "fleet-kubectl-e2e-test"

// afFleetKubectlRemoteClusterID matches the "remote-cluster" identity DD-TEST-013
// registers in the Fleet E2E suite (test/e2e/fleet/13_cluster_scoped_workflow_targeting_test.go).
const afFleetKubectlRemoteClusterID = "remote-cluster"

// afFleetKubectlScenario drives AF's real ADK agent (via a real A2A call, no
// mocked ResourceReaderFactory) to call its own list_clusters and
// kubectl_get(cluster_id=remote-cluster) tools, closing E2E-FLEET-016
// (issue #1768, Gaps A+C): no existing test drives a real AF binary against
// the Fleet E2E suite's real second cluster + gateway.
//
// Turn 1 (no function results yet): emits list_clusters + kubectl_get in a
// single MultiToolCalls batch — the LLM "decides" to check the fleet roster
// and then read the remote cluster's coredns Deployment.
//
// Turn 2 (function results present): AF's ADK loop has, by this point,
// already executed both tools for real (against the Fleet E2E suite's real
// FleetReaderFactory -> kube-mcp-server -> Kuadrant gateway -> remote Kind
// cluster) and appended their FunctionResponse payloads to the conversation.
// ConfigForContext echoes ctx.AllText (which response.ExtractTextFromContents
// includes FunctionResponse.Response JSON in, per gemini.go) back verbatim as
// the final answer text, so the E2E test can assert on genuine remote-cluster
// data (e.g. the coredns object) surfacing through AF's real A2A response,
// rather than a canned string that would pass even if the tool call silently
// no-op'd.
type afFleetKubectlScenario struct{}

func afFleetKubectlE2EScenario() *afFleetKubectlScenario {
	return &afFleetKubectlScenario{}
}

func (s *afFleetKubectlScenario) Name() string { return "af_fleet_kubectl_e2e" }

func (s *afFleetKubectlScenario) Metadata() ScenarioMetadata {
	return ScenarioMetadata{
		Name:        "af_fleet_kubectl_e2e",
		Description: "E2E-FLEET-016: real AF binary calls list_clusters + kubectl_get(cluster_id) via real A2A",
	}
}

func (s *afFleetKubectlScenario) DAG() *conversation.DAG { return nil }

func (s *afFleetKubectlScenario) Match(ctx *DetectionContext) (bool, float64) {
	combined := strings.ToLower(ctx.Content + " " + ctx.AllText)
	if strings.Contains(combined, afFleetKubectlE2EKeyword) {
		return true, 0.95
	}
	return false, 0
}

func (s *afFleetKubectlScenario) ConfigForContext(ctx *DetectionContext) MockScenarioConfig {
	cfg := MockScenarioConfig{
		ScenarioName: s.Name(),
		ForceText:    BoolPtr(false),
		MultiToolCalls: []MultiToolCallEntry{
			{Name: "list_clusters"},
			{Name: "kubectl_get", Arguments: map[string]interface{}{
				"kind":       "Deployment",
				"name":       "coredns",
				"namespace":  "kube-system",
				"cluster_id": afFleetKubectlRemoteClusterID,
			}},
		},
	}
	// "coredns" only appears in ctx.AllText once the tool results (turn 2)
	// have been appended to the conversation — the turn-1 user prompt itself
	// never mentions it. Echoing AllText verbatim lets the E2E test assert on
	// the real returned object rather than a scenario-authored string.
	if ctx != nil && strings.Contains(ctx.AllText, "coredns") {
		cfg.ExactAnalysisText = ctx.AllText
	}
	return cfg
}
