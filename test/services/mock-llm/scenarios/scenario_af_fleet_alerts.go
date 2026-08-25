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

// afFleetAlertsE2EKeyword is the distinctive phrase the E2E test's A2A
// message text must contain to select this scenario (mirrors
// afFleetKubectlE2EKeyword's pattern in scenario_af_fleet_kubectl.go).
const afFleetAlertsE2EKeyword = "fleet-alerts-e2e-test"

// afFleetAlertsRemoteClusterID matches the "remote-cluster" identity DD-TEST-013
// registers in the Fleet E2E suite.
const afFleetAlertsRemoteClusterID = "remote-cluster"

// afFleetAlertsE2ENamespace/E2EAlertName are fixed labels the E2E test
// injects its collision alerts under (test/e2e/fleet/19_af_alerts_fleet_scoped_test.go,
// E2E-FLEET-020) -- hardcoded here the same way scenario_af_fleet_kubectl.go
// hardcodes "coredns"/"kube-system", since this scenario's tool-call
// arguments must be known at compile time.
const (
	afFleetAlertsE2ENamespace = "fleet-alerts-e2e-ns"
)

// afFleetAlertsScenario drives AF's real ADK agent (via a real A2A call) to
// call its own list_alerts(cluster_id=remote-cluster, namespace=...) tool,
// closing E2E-FLEET-020 (Issue #2274): no existing test proves AF's
// list_alerts/get_alert_details tools are cluster-scoped against a real
// fleet AlertManager with a genuine cross-cluster alertname collision.
//
// Turn 1 (no function results yet): emits list_alerts as a single-entry
// MultiToolCalls batch, scoped to both cluster_id and namespace so
// concurrently-running fleet E2E specs' own alerts (shared AlertManager
// instance, test/infrastructure/prometheus_alertmanager_e2e.go) can't dilute
// or truncate (trimResultToFit) the two alerts this scenario's own E2E test
// injects.
//
// Turn 2 (function results present): mirrors afFleetKubectlScenario --
// AF's ADK loop has, by this point, already executed list_alerts for real
// against the Fleet E2E suite's real AlertManager and appended its
// FunctionResponse payload (including each alert's redacted
// labels/annotations) to the conversation. Echoing ctx.AllText back
// verbatim lets the E2E test assert on the genuine returned alert data
// (a marker string unique to the matching cluster's alert annotation)
// rather than a canned string that would pass even if cluster_id
// filtering silently no-op'd.
type afFleetAlertsScenario struct{}

func afFleetAlertsE2EScenario() *afFleetAlertsScenario {
	return &afFleetAlertsScenario{}
}

func (s *afFleetAlertsScenario) Name() string { return "af_fleet_alerts_e2e" }

func (s *afFleetAlertsScenario) Metadata() ScenarioMetadata {
	return ScenarioMetadata{
		Name:        "af_fleet_alerts_e2e",
		Description: "E2E-FLEET-020: real AF binary calls list_alerts(cluster_id) via real A2A (Issue #2274)",
	}
}

func (s *afFleetAlertsScenario) DAG() *conversation.DAG { return nil }

func (s *afFleetAlertsScenario) Match(ctx *DetectionContext) (bool, float64) {
	combined := strings.ToLower(ctx.Content + " " + ctx.AllText)
	if strings.Contains(combined, afFleetAlertsE2EKeyword) {
		return true, 0.95
	}
	return false, 0
}

func (s *afFleetAlertsScenario) ConfigForContext(ctx *DetectionContext) MockScenarioConfig {
	cfg := MockScenarioConfig{
		ScenarioName: s.Name(),
		ForceText:    BoolPtr(false),
		MultiToolCalls: []MultiToolCallEntry{
			{Name: "list_alerts", Arguments: map[string]interface{}{
				"namespace":  afFleetAlertsE2ENamespace,
				"cluster_id": afFleetAlertsRemoteClusterID,
			}},
		},
	}
	// The injected marker annotation text only appears in ctx.AllText once
	// the tool results (turn 2) have been appended to the conversation.
	// ctx.AllText is already lowercased by the mock-llm request handler
	// (test/services/mock-llm/handlers/{gemini,openai}.go build
	// DetectionContext.AllText via strings.ToLower), so the marker constant
	// checked here -- and injected by the E2E test as an alert annotation --
	// must itself be lowercase, or this Contains check silently never
	// matches (CI RCA, PR #2286: an uppercase "MARKER-" check against an
	// always-lowercased AllText fell through to the default fallback
	// analysis text on every run).
	if ctx != nil && strings.Contains(ctx.AllText, "marker-") {
		cfg.ExactAnalysisText = ctx.AllText
	}
	return cfg
}
