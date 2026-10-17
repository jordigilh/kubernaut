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

package investigator

import (
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

const (
	humanReviewReasonLLMParsingError           = "llm_parsing_error"
	humanReviewReasonInvestigationInconclusive = "investigation_inconclusive"
)

// OutcomeClassificationInput contains the server-owned state used to classify
// an investigation terminal result. The workflow-discovery gate is explicit so
// a missing SelectedWorkflow can never, by itself, establish no-match.
// DD-KA-2520-2521.
type OutcomeClassificationInput struct {
	Result                  *katypes.InvestigationResult
	HistoryStatus           sharedtypes.HistoryStatus
	WorkflowDiscoveryStatus sharedtypes.WorkflowDiscoveryStatus

	// EvidenceTransportUnavailable is set by the typed #2518 exhaustion result.
	// The retry/reconnect loop itself remains owned by #2518.
	EvidenceTransportUnavailable bool
	// HistoryEnrichmentUnavailable identifies a required target-scoped history
	// acquisition failure separately from generic evidence transport failure.
	HistoryEnrichmentUnavailable bool
}

// ClassifyTerminalOutcome returns the authoritative terminal classification for
// an investigation. It is intentionally conservative: unknown or partially
// populated results do not become no_matching_workflows.
func ClassifyTerminalOutcome(in OutcomeClassificationInput) sharedtypes.TerminalOutcome {
	result := in.Result
	if result == nil {
		return sharedtypes.TerminalOutcomeUnknown
	}

	historyStatus := in.HistoryStatus
	if historyStatus == "" {
		historyStatus = result.HistoryStatus
	}
	discoveryStatus := in.WorkflowDiscoveryStatus
	if discoveryStatus == "" {
		discoveryStatus = result.WorkflowDiscoveryStatus
	}

	if result.Cancelled {
		return sharedtypes.TerminalOutcomeCancelled
	}
	if outcome := classifyUnavailableOutcome(in, historyStatus); outcome != sharedtypes.TerminalOutcomeUnknown {
		return outcome
	}
	if result.TerminalOutcome.IsValid() && terminalOutcomeAllowed(result.TerminalOutcome, discoveryStatus) {
		return result.TerminalOutcome
	}

	if outcome := classifyDiscoveryOutcome(discoveryStatus); outcome != sharedtypes.TerminalOutcomeUnknown {
		return outcome
	}
	return classifyLegacyOutcome(result, discoveryStatus)
}

func classifyUnavailableOutcome(in OutcomeClassificationInput, historyStatus sharedtypes.HistoryStatus) sharedtypes.TerminalOutcome {
	// Evidence/history acquisition is evaluated before a provisional terminal
	// value. An RCA loop can exhaust before the enrichment boundary reports its
	// final #2518 status; once transport/schema failure is known, it must not be
	// mislabeled as rca_incomplete.
	if in.HistoryEnrichmentUnavailable {
		return sharedtypes.TerminalOutcomeHistoryEnrichmentUnavailable
	}
	if in.EvidenceTransportUnavailable || historyStatus == sharedtypes.HistoryStatusTransportUnavailable || historyStatus == sharedtypes.HistoryStatusSchemaError {
		return sharedtypes.TerminalOutcomeEvidenceTransportUnavailable
	}
	return sharedtypes.TerminalOutcomeUnknown
}

func classifyDiscoveryOutcome(status sharedtypes.WorkflowDiscoveryStatus) sharedtypes.TerminalOutcome {
	// Only the completed-no-match gate can establish a genuine no-workflow
	// result. This check must stay before the legacy HumanReviewReason fallback.
	switch status {
	case sharedtypes.WorkflowDiscoveryStatusCompletedNoMatch:
		return sharedtypes.TerminalOutcomeNoMatchingWorkflows
	case sharedtypes.WorkflowDiscoveryStatusFailed:
		return sharedtypes.TerminalOutcomeWorkflowDiscoveryFailed
	default:
		return sharedtypes.TerminalOutcomeUnknown
	}
}

func classifyLegacyOutcome(result *katypes.InvestigationResult, discoveryStatus sharedtypes.WorkflowDiscoveryStatus) sharedtypes.TerminalOutcome {
	if result.InvestigationOutcome == katypes.InvestigationOutcomeProblemResolved {
		return sharedtypes.TerminalOutcomeProblemResolved
	}
	if result.WorkflowID != "" || discoveryStatus == sharedtypes.WorkflowDiscoveryStatusCompletedSelected {
		return sharedtypes.TerminalOutcomeWorkflowSelected
	}
	if result.HumanReviewReason == katypes.HumanReviewReasonRCAIncomplete {
		return sharedtypes.TerminalOutcomeRCAIncomplete
	}
	if result.HumanReviewReason == humanReviewReasonLLMParsingError && discoveryStatus != sharedtypes.WorkflowDiscoveryStatusStarted {
		return sharedtypes.TerminalOutcomeRCAIncomplete
	}
	if result.HumanReviewNeeded && discoveryStatus != sharedtypes.WorkflowDiscoveryStatusStarted &&
		discoveryStatus != sharedtypes.WorkflowDiscoveryStatusCompletedSelected &&
		discoveryStatus != sharedtypes.WorkflowDiscoveryStatusCompletedNoMatch {
		return sharedtypes.TerminalOutcomeRCAIncomplete
	}

	return sharedtypes.TerminalOutcomeUnknown
}

func terminalOutcomeAllowed(outcome sharedtypes.TerminalOutcome, discoveryStatus sharedtypes.WorkflowDiscoveryStatus) bool {
	if outcome == sharedtypes.TerminalOutcomeNoMatchingWorkflows {
		return discoveryStatus == sharedtypes.WorkflowDiscoveryStatusCompletedNoMatch
	}
	if outcome == sharedtypes.TerminalOutcomeWorkflowSelected {
		return discoveryStatus == sharedtypes.WorkflowDiscoveryStatusCompletedSelected || discoveryStatus == sharedtypes.WorkflowDiscoveryStatusStarted || discoveryStatus == ""
	}
	return true
}

// applyEnrichmentHistoryStatus copies the bounded history acquisition state
// from the enrichment result into the investigation result. The legacy bool is
// retained as a fallback for fixtures and older callers that predate the typed
// status.
func applyEnrichmentHistoryStatus(result *katypes.InvestigationResult, data *enrichment.EnrichmentResult) {
	if result == nil {
		return
	}
	if data == nil {
		if result.HistoryStatus == "" {
			result.HistoryStatus = sharedtypes.HistoryStatusNotRequested
		}
		return
	}
	// A later target enrichment may fail after an actionable RCA has already
	// been established. Preserve that authoritative unavailable state when the
	// caller deliberately retains the earlier successful enrichment snapshot;
	// an available/empty fallback must not erase a transport/schema failure.
	if isHistoryUnavailableStatus(result.HistoryStatus) &&
		data.HistoryStatus != sharedtypes.HistoryStatusTransportUnavailable &&
		data.HistoryStatus != sharedtypes.HistoryStatusSchemaError &&
		!data.HistoryUnavailable {
		return
	}
	if data.HistoryStatus.IsValid() {
		result.HistoryStatus = data.HistoryStatus
		return
	}
	if data.HistoryUnavailable {
		result.HistoryStatus = sharedtypes.HistoryStatusTransportUnavailable
		return
	}
	if result.HistoryStatus == "" {
		result.HistoryStatus = sharedtypes.HistoryStatusNotRequested
	}
}

func isHistoryUnavailableStatus(status sharedtypes.HistoryStatus) bool {
	return status == sharedtypes.HistoryStatusTransportUnavailable || status == sharedtypes.HistoryStatusSchemaError
}

// markHistoryEnrichmentUnavailable preserves the already accepted RCA while
// recording that the required target-scoped history could not be acquired.
// The compatibility reason remains rca_incomplete; the typed terminal outcome
// carries the more precise source classification (DD-KA-2520-2521).
func markHistoryEnrichmentUnavailable(result *katypes.InvestigationResult, data *enrichment.EnrichmentResult) {
	if result == nil {
		return
	}
	applyEnrichmentHistoryStatus(result, data)
	result.HumanReviewNeeded = true
	result.HumanReviewReason = katypes.HumanReviewReasonRCAIncomplete
	markDiscoverySkippedDueToFailure(result, result.HistoryStatus, true)
}

// setTypedOutcome updates the authoritative state after a phase transition.
// The compatibility human-review/reason fields remain untouched.
func setTypedOutcome(result *katypes.InvestigationResult, historyStatus sharedtypes.HistoryStatus, discoveryStatus sharedtypes.WorkflowDiscoveryStatus, historyUnavailable bool) {
	if result == nil {
		return
	}
	if historyStatus != "" {
		result.HistoryStatus = historyStatus
	}
	if discoveryStatus != "" {
		result.WorkflowDiscoveryStatus = discoveryStatus
	}
	result.TerminalOutcome = ClassifyTerminalOutcome(OutcomeClassificationInput{
		Result:                       result,
		HistoryStatus:                result.HistoryStatus,
		WorkflowDiscoveryStatus:      result.WorkflowDiscoveryStatus,
		HistoryEnrichmentUnavailable: historyUnavailable,
	})
}

func markWorkflowDiscoveryStarted(result *katypes.InvestigationResult) {
	if result == nil {
		return
	}
	result.WorkflowDiscoveryStatus = sharedtypes.WorkflowDiscoveryStatusStarted
	if result.TerminalOutcome == sharedtypes.TerminalOutcomeNoMatchingWorkflows || result.TerminalOutcome == sharedtypes.TerminalOutcomeWorkflowDiscoveryFailed {
		result.TerminalOutcome = sharedtypes.TerminalOutcomeUnknown
	}
}

func markWorkflowDiscoverySelected(result *katypes.InvestigationResult) {
	if result == nil {
		return
	}
	result.WorkflowDiscoveryStatus = sharedtypes.WorkflowDiscoveryStatusCompletedSelected
	result.TerminalOutcome = sharedtypes.TerminalOutcomeWorkflowSelected
}

func markWorkflowDiscoveryNoMatch(result *katypes.InvestigationResult) {
	if result == nil {
		return
	}
	result.WorkflowDiscoveryStatus = sharedtypes.WorkflowDiscoveryStatusCompletedNoMatch
	result.TerminalOutcome = sharedtypes.TerminalOutcomeNoMatchingWorkflows
}

func markWorkflowDiscoveryFailed(result *katypes.InvestigationResult) {
	if result == nil {
		return
	}
	result.WorkflowDiscoveryStatus = sharedtypes.WorkflowDiscoveryStatusFailed
	result.TerminalOutcome = sharedtypes.TerminalOutcomeWorkflowDiscoveryFailed
}

func classifyWorkflowDiscoveryNoWorkflow(result *katypes.InvestigationResult) (sharedtypes.WorkflowDiscoveryStatus, sharedtypes.TerminalOutcome, bool) {
	if result == nil || result.HumanReviewNeeded {
		return "", "", false
	}
	switch {
	case result.InvestigationOutcome == katypes.InvestigationOutcomeProblemResolved:
		return sharedtypes.WorkflowDiscoveryStatusCompletedNoMatch, sharedtypes.TerminalOutcomeProblemResolved, true
	case result.IsActionable != nil && !*result.IsActionable:
		return sharedtypes.WorkflowDiscoveryStatusCompletedNoMatch, "", true
	default:
		return "", "", false
	}
}

// markRCAIncomplete records a terminal RCA failure without pretending that
// workflow discovery ran. Enrichment status is deliberately left for the
// caller to apply first so a later #2518 transport result can take precedence.
func markRCAIncomplete(result *katypes.InvestigationResult) {
	if result == nil {
		return
	}
	result.WorkflowDiscoveryStatus = sharedtypes.WorkflowDiscoveryStatusNotStarted
	result.TerminalOutcome = sharedtypes.TerminalOutcomeRCAIncomplete
}

func markDiscoverySkippedDueToFailure(result *katypes.InvestigationResult, historyStatus sharedtypes.HistoryStatus, historyUnavailable bool) {
	setTypedOutcome(result, historyStatus, sharedtypes.WorkflowDiscoveryStatusSkippedDueToFailure, historyUnavailable)
}
