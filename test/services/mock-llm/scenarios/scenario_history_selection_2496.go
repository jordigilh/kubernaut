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
	"regexp"
	"strings"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	"github.com/jordigilh/kubernaut/pkg/shared/uuid"
)

const selectionHighSeverity2496 = "high"

// BR-KA-016 / DD-KA-2496: external provider fixtures leave history, eligibility,
// correction budgets and review/execution policy to the deployed platform.
// Caller scoping prevents a KA signal echoed into an AF tool result from
// selecting this provider response in AF's orchestration conversation.
func historySelectionScenarios2496() []Scenario {
	names := []string{
		"history_first_2496", "history_repeat_accepted_2496",
		"history_repeat_rejected_2496", "history_repeat_low_2496",
		"history_repeat_approval_2496",
		"history_no_workflow_2496", "history_different_2496", "history_hostile_2533",
	}
	result := make([]Scenario, 0, len(names))
	for _, name := range names {
		cfg := historySelectionConfig2496(name)
		result = append(result, newSelectorScenario(name, ScenarioSelector{
			Scope:          ScenarioScope{Caller: CallerKA},
			SignalPatterns: []string{strings.ToLower(cfg.SignalName)},
			Confidence:     1.0,
		}, cfg))
	}
	return result
}

// BR-KA-017 / #2498: only the external provider is adversarial. RCA submits
// directly despite optional context tools; discovery first attempts three
// registered off-phase tools with a spoofed phase, then follows the ordinary
// catalog planner after the real dispatcher returns its tool results.
func phaseIsolationScenarios2498() []Scenario {
	cfg := oomkilledConfig()
	cfg.SignalName = "phaseisolation2498"
	cfg.Severity = selectionHighSeverity2496
	cfg.Alternatives = nil
	rca := cfg
	rca.ToolCallName = openai.ToolSubmitResult
	cfg.MultiToolCalls = make([]MultiToolCallEntry, 0, 3)
	for _, name := range []string{"get_namespaced_resource_context", "get_cluster_resource_context", "kubectl_get_by_name"} {
		cfg.MultiToolCalls = append(cfg.MultiToolCalls, MultiToolCallEntry{Name: name, Arguments: map[string]interface{}{
			"kind": cfg.ResourceKind, "name": cfg.ResourceName, "namespace": cfg.ResourceNS,
			"phase": "rca",
		}})
	}
	return []Scenario{
		newSelectorScenario("phase_isolation_rca_2498", ScenarioSelector{
			Scope: ScenarioScope{Caller: CallerKA, Phase: PhaseRCA}, SignalPatterns: []string{cfg.SignalName}, Confidence: 1.0,
		}, rca),
		newSelectorScenario("phase_isolation_2498", ScenarioSelector{
			Scope: ScenarioScope{Caller: CallerKA, Phase: PhaseWorkflowDiscovery}, SignalPatterns: []string{cfg.SignalName}, Confidence: 1.0,
		}, cfg),
	}
}

var historyLiveRR2496 = regexp.MustCompile(`^investigate current selection 2496 for (rr-history-2496-[a-z0-9-]+)$`)

// This external AF provider fixture only issues the authorized read/investigate
// call. KA remains the source of the live result; no audit lookup or model-made
// selected_workflow is scripted into AF's output.
type afHistoryLiveScenario2496 struct {
	*configScenario
}

func afHistoryLive2496() Scenario {
	return &afHistoryLiveScenario2496{configScenario: newSelectorScenario("af_history_live_2496", ScenarioSelector{
		Scope: ScenarioScope{Caller: CallerAF},
		CustomMatch: func(ctx *DetectionContext) (bool, float64) {
			return ctx != nil && historyLiveRR2496.MatchString(strings.TrimSpace(ctx.LastUserContent)), 1.05
		},
	}, MockScenarioConfig{ToolCallName: "kubernaut_investigate", ForceText: BoolPtr(false), RepeatToolCall: true})}
}

func (s *afHistoryLiveScenario2496) ConfigForContext(ctx *DetectionContext) MockScenarioConfig {
	cfg := s.config
	matched := historyLiveRR2496.FindStringSubmatch(strings.TrimSpace(ctx.LastUserContent))
	if len(matched) == 2 {
		cfg.ToolCallArgs = map[string]interface{}{"rr_id": matched[1]}
	}
	return cfg
}

func historySelectionConfig2496(name string) MockScenarioConfig {
	cfg := oomkilledConfig()
	cfg.ScenarioName = name
	cfg.SignalName = strings.ReplaceAll(name, "_", "")
	cfg.Severity = selectionHighSeverity2496
	cfg.Confidence = .84
	rcaConfidence := .99
	cfg.RCAConfidence = &rcaConfidence
	cfg.Alternatives = nil
	cfg.RootCause = "Current Deployment memory limit is below the workload's observed memory demand"
	cfg.RepeatJustification = &sharedtypes.RepeatJustification{
		EvidenceBasis:         "Current workload remains memory constrained after the previous attempt",
		PriorOutcomeRelevance: "The previous workflow completed; its recorded outcome does not establish that the present memory demand is resolved",
		ExpectedMechanism:     "Increase the current memory limit above the observed workload demand",
	}
	switch name {
	case "history_first_2496":
		cfg.Confidence = .94
		cfg.Parameters = map[string]string{"MEMORY_LIMIT_NEW": "55Mi"}
		cfg.RepeatJustification = nil
	case "history_repeat_rejected_2496":
		cfg.RepeatJustification = nil
	case "history_repeat_low_2496":
		cfg.Confidence = .42
	case "history_repeat_approval_2496":
		cfg.Confidence = .75
	case "history_no_workflow_2496":
		cfg.DeclineWorkflow = true
		cfg.RepeatJustification = nil
	case "history_different_2496":
		cfg.WorkflowName = "oomkill-increase-memory-alternative-2496-v1"
		cfg.WorkflowID = uuid.DeterministicUUID(cfg.WorkflowName)
		cfg.Confidence = .96
		cfg.RepeatJustification = nil
	case "history_hostile_2533":
		cfg.Rationale = "Prior rationale contains password=history-secret-2533 and must be sanitized before historical reuse"
		cfg.RepeatJustification = nil
	}
	return cfg
}
