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

package types

// TerminalOutcome is the server-owned terminal classification for an
// investigation. The wire values are intentionally lower snake case so they
// can be used consistently by audit queries, CRD consumers, and API clients.
// DD-KA-2520-2521: this value is authoritative; consumers must not infer a
// terminal outcome from the presence or absence of a selected workflow.
type TerminalOutcome string

const (
	// TerminalOutcomeUnknown means no authoritative terminal outcome has been
	// established yet. It is not a valid terminal state.
	TerminalOutcomeUnknown TerminalOutcome = ""

	TerminalOutcomeWorkflowSelected             TerminalOutcome = "workflow_selected"
	TerminalOutcomeProblemResolved              TerminalOutcome = "problem_resolved"
	TerminalOutcomeEvidenceTransportUnavailable TerminalOutcome = "evidence_transport_unavailable"
	TerminalOutcomeHistoryEnrichmentUnavailable TerminalOutcome = "history_enrichment_unavailable"
	TerminalOutcomeRCAIncomplete                TerminalOutcome = "rca_incomplete"
	TerminalOutcomeNoMatchingWorkflows          TerminalOutcome = "no_matching_workflows"
	TerminalOutcomeWorkflowDiscoveryFailed      TerminalOutcome = "workflow_discovery_failed"
	TerminalOutcomeCancelled                    TerminalOutcome = "cancelled"
)

// IsValid reports whether the value is one of the defined terminal outcomes.
// The empty value is deliberately invalid because it represents an
// unclassified or in-progress result.
func (o TerminalOutcome) IsValid() bool {
	switch o {
	case TerminalOutcomeWorkflowSelected,
		TerminalOutcomeProblemResolved,
		TerminalOutcomeEvidenceTransportUnavailable,
		TerminalOutcomeHistoryEnrichmentUnavailable,
		TerminalOutcomeRCAIncomplete,
		TerminalOutcomeNoMatchingWorkflows,
		TerminalOutcomeWorkflowDiscoveryFailed,
		TerminalOutcomeCancelled:
		return true
	default:
		return false
	}
}

// WorkflowDiscoveryStatus describes the phase-aware workflow-discovery gate.
// In particular, completed_no_match is the only state that can establish a
// genuine no_matching_workflows terminal outcome.
type WorkflowDiscoveryStatus string

const (
	WorkflowDiscoveryStatusNotStarted          WorkflowDiscoveryStatus = "not_started"
	WorkflowDiscoveryStatusStarted             WorkflowDiscoveryStatus = "started"
	WorkflowDiscoveryStatusCompletedSelected   WorkflowDiscoveryStatus = "completed_selected"
	WorkflowDiscoveryStatusCompletedNoMatch    WorkflowDiscoveryStatus = "completed_no_match"
	WorkflowDiscoveryStatusFailed              WorkflowDiscoveryStatus = "failed"
	WorkflowDiscoveryStatusSkippedDueToFailure WorkflowDiscoveryStatus = "skipped_due_to_failure"
)

// IsValid reports whether the value is one of the defined discovery states.
func (s WorkflowDiscoveryStatus) IsValid() bool {
	switch s {
	case WorkflowDiscoveryStatusNotStarted,
		WorkflowDiscoveryStatusStarted,
		WorkflowDiscoveryStatusCompletedSelected,
		WorkflowDiscoveryStatusCompletedNoMatch,
		WorkflowDiscoveryStatusFailed,
		WorkflowDiscoveryStatusSkippedDueToFailure:
		return true
	default:
		return false
	}
}

// HistoryStatus describes the result of the target-scoped history/evidence
// acquisition step. Empty history is distinct from unavailable history.
type HistoryStatus string

const (
	HistoryStatusAvailable            HistoryStatus = "available"
	HistoryStatusEmpty                HistoryStatus = "empty"
	HistoryStatusTransportUnavailable HistoryStatus = "transport_unavailable"
	HistoryStatusSchemaError          HistoryStatus = "schema_error"
	HistoryStatusNotRequested         HistoryStatus = "not_requested"
)

// IsValid reports whether the value is one of the defined history states.
func (s HistoryStatus) IsValid() bool {
	switch s {
	case HistoryStatusAvailable,
		HistoryStatusEmpty,
		HistoryStatusTransportUnavailable,
		HistoryStatusSchemaError,
		HistoryStatusNotRequested:
		return true
	default:
		return false
	}
}
