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

import "strings"

// Caller identifies the service that owns a mock-LLM request.
type Caller string

const (
	CallerUnknown Caller = ""
	CallerKA      Caller = "ka"
	CallerAF      Caller = "af"
)

// Phase identifies the current caller phase when it can be inferred safely.
type Phase string

const (
	PhaseUnknown           Phase = ""
	PhaseRCA               Phase = "rca"
	PhaseInvestigation     Phase = "investigation"
	PhaseWorkflowDiscovery Phase = "workflow_discovery"
	PhaseWorkflowSelection Phase = "workflow_selection"
	PhaseRemediation       Phase = "remediation"
)

// ScenarioScope contains optional request constraints for a scenario.
type ScenarioScope struct {
	Caller Caller
	Phase  Phase
}

// DetectionContext holds the input data used for scenario detection.
type DetectionContext struct {
	Content         string
	AllText         string
	SignalName      string
	IsProactive     bool
	LastUserContent string
	Caller          Caller
	Phase           Phase

	// AvailableTools lists the function-tool names the caller actually
	// advertised in this request (OpenAI req.Tools or Gemini function
	// declarations; empty for the Ollama handler, which has no tools field).
	// Lets a scenario pick which of several candidate tool calls to
	// script strictly based on what was really offered, mirroring how a
	// real LLM can only ever call a tool it was told about -- e.g.
	// distinguishing a hub-local investigation (only kubectl_* tools
	// offered) from a fleet one (overlay tools like resources_get also
	// offered) without the test needing to tell the scenario which
	// environment it's in (E2E-FLEET-017, issue #1729).
	AvailableTools []string
}

// Matches reports whether the metadata's optional request scope matches ctx.
// An empty metadata value is a wildcard for legacy scenarios. A scoped
// scenario never matches an unknown request scope, preventing it from
// shadowing a caller-specific scenario during incomplete request inference.
func (m ScenarioMetadata) Matches(ctx *DetectionContext) bool {
	if m.Caller != "" && (ctx == nil || ctx.Caller != m.Caller) {
		return false
	}
	if m.Phase != "" && (ctx == nil || ctx.Phase != m.Phase) {
		return false
	}
	return true
}

// InferRequestScope derives a conservative scope from declared tool names
// and the current user message. Unknown is intentional: legacy scenarios
// remain usable, while scoped scenarios require positive caller/phase evidence.
func InferRequestScope(content, allText, lastUserContent string, availableTools []string) (Caller, Phase) {
	caller := CallerUnknown
	for _, name := range availableTools {
		name = strings.ToLower(strings.TrimSpace(name))
		switch {
		case strings.HasPrefix(name, "kubernaut_"):
			caller = CallerAF
		case name == "submit_result", name == "submit_result_with_workflow", name == "submit_result_no_workflow",
			strings.HasPrefix(name, "kubectl_"), strings.HasPrefix(name, "prometheus_"), strings.HasPrefix(name, "resources_"):
			if caller == CallerUnknown {
				caller = CallerKA
			}
		}
	}

	combined := strings.ToLower(content + " " + allText)
	phaseText := strings.ToLower(strings.TrimSpace(lastUserContent))
	if phaseText == "" {
		phaseText = strings.ToLower(strings.TrimSpace(content))
	}
	phase := PhaseUnknown
	if caller != CallerAF {
		for _, name := range availableTools {
			name = strings.ToLower(strings.TrimSpace(name))
			switch name {
			case "submit_result":
				phase = PhaseRCA
			case "submit_result_with_workflow", "submit_result_no_workflow":
				phase = PhaseWorkflowDiscovery
			}
		}
	}
	if phase == PhaseUnknown {
		phase = phaseFromText(phaseText)
	}
	if phase == PhaseUnknown && caller == CallerAF {
		phase = uniqueAFPhase(availableTools)
	}
	if phase == PhaseUnknown {
		phase = phaseFromText(combined)
	}
	return caller, phase
}

func phaseFromText(text string) Phase {
	switch {
	case strings.Contains(text, "workflow selection"), strings.Contains(text, "select workflow"):
		return PhaseWorkflowSelection
	case strings.Contains(text, "workflow discovery"), strings.Contains(text, "discover workflows"):
		return PhaseWorkflowDiscovery
	case strings.Contains(text, "investigate"):
		return PhaseInvestigation
	case strings.Contains(text, "remediate"):
		return PhaseRemediation
	default:
		return PhaseUnknown
	}
}

func uniqueAFPhase(availableTools []string) Phase {
	phase := PhaseUnknown
	for _, name := range availableTools {
		var current Phase
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "kubernaut_investigate":
			current = PhaseInvestigation
		case "kubernaut_discover_workflows":
			current = PhaseWorkflowDiscovery
		case "kubernaut_select_workflow":
			current = PhaseWorkflowSelection
		case "kubernaut_remediate":
			current = PhaseRemediation
		default:
			continue
		}
		if phase != PhaseUnknown && phase != current {
			return PhaseUnknown
		}
		phase = current
	}
	return phase
}
