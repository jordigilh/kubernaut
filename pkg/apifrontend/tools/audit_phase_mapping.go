package tools

// phaseOrder defines the canonical lifecycle ordering for audit phases.
// Phases appear in this order in the aggregated result regardless of
// when events occurred. The order reflects the typical remediation
// lifecycle from signal receipt through effectiveness measurement.
var phaseOrder = []string{
	"Signal Processing",
	"Triage",
	"RR Creation",
	"Investigation",
	"Workflow Discovery",
	"Approval",
	"Execution",
	"Effectiveness",
	"Other",
}

// eventPhaseMap maps DS event types (exact match) and event type prefixes
// (ending with dot) to lifecycle phases. Exact matches are tried first,
// then progressively shorter dot-delimited prefixes. Unmapped types resolve
// to "Other".
//
// Source taxonomy: pkg/datastorage/ogen-client/oas_schemas_gen.go (discriminator).
var eventPhaseMap = map[string]string{
	"orchestrator.lifecycle.started":   "RR Creation",
	"orchestrator.lifecycle.created":   "RR Creation",
	"orchestrator.lifecycle.completed": "Execution",
	"orchestrator.lifecycle.failed":    "Execution",
	"apifrontend.user.decision":       "Approval",

	"gateway.signal.":              "Signal Processing",
	"signalprocessing.":            "Signal Processing",
	"apifrontend.triage.":          "Triage",
	"apifrontend.severity_triage.": "Triage",
	"gateway.crd.":                 "RR Creation",
	"apifrontend.rr.":              "RR Creation",
	"aiagent.session.":             "Investigation",
	"aiagent.rca.":                 "Investigation",
	"aiagent.llm.":                 "Investigation",
	"aiagent.response.":            "Investigation",
	"aianalysis.":                  "Investigation",
	"workflow.catalog.":            "Workflow Discovery",
	"workflowexecution.selection.": "Workflow Discovery",
	"aiagent.workflow.":            "Workflow Discovery",
	"orchestrator.approval.":       "Approval",
	"workflowexecution.workflow.":  "Execution",
	"workflowexecution.execution.": "Execution",
	"effectiveness.":               "Effectiveness",
}

// resolvePhase determines the lifecycle phase for a given DS event type.
// It tries exact match first, then progressively shorter dot-delimited
// prefixes. Returns "Other" if no match is found.
func resolvePhase(eventType string) string {
	if phase, ok := eventPhaseMap[eventType]; ok {
		return phase
	}
	for i := len(eventType) - 1; i >= 0; i-- {
		if eventType[i] == '.' {
			if phase, ok := eventPhaseMap[eventType[:i+1]]; ok {
				return phase
			}
		}
	}
	return "Other"
}
