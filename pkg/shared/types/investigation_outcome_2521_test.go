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

package types_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("Investigation outcome wire contract — #2521", func() {
	It("UT-KA-2521-001: round-trips every typed value using lower snake case", func() {
		terminalValues := map[sharedtypes.TerminalOutcome]string{
			sharedtypes.TerminalOutcomeWorkflowSelected:             "workflow_selected",
			sharedtypes.TerminalOutcomeProblemResolved:              "problem_resolved",
			sharedtypes.TerminalOutcomeEvidenceTransportUnavailable: "evidence_transport_unavailable",
			sharedtypes.TerminalOutcomeHistoryEnrichmentUnavailable: "history_enrichment_unavailable",
			sharedtypes.TerminalOutcomeRCAIncomplete:                "rca_incomplete",
			sharedtypes.TerminalOutcomeNoMatchingWorkflows:          "no_matching_workflows",
			sharedtypes.TerminalOutcomeWorkflowDiscoveryFailed:      "workflow_discovery_failed",
			sharedtypes.TerminalOutcomeCancelled:                    "cancelled",
		}
		for value, wire := range terminalValues {
			raw, err := json.Marshal(value)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(raw)).To(Equal(`"` + wire + `"`))
			Expect(value.IsValid()).To(BeTrue())

			var restored sharedtypes.TerminalOutcome
			Expect(json.Unmarshal(raw, &restored)).To(Succeed())
			Expect(restored).To(Equal(value))
		}

		statusValues := []struct {
			value sharedtypes.WorkflowDiscoveryStatus
			wire  string
		}{
			{sharedtypes.WorkflowDiscoveryStatusNotStarted, "not_started"},
			{sharedtypes.WorkflowDiscoveryStatusStarted, "started"},
			{sharedtypes.WorkflowDiscoveryStatusCompletedSelected, "completed_selected"},
			{sharedtypes.WorkflowDiscoveryStatusCompletedNoMatch, "completed_no_match"},
			{sharedtypes.WorkflowDiscoveryStatusFailed, "failed"},
			{sharedtypes.WorkflowDiscoveryStatusSkippedDueToFailure, "skipped_due_to_failure"},
		}
		for _, entry := range statusValues {
			raw, err := json.Marshal(entry.value)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(raw)).To(Equal(`"` + entry.wire + `"`))
			Expect(entry.value.IsValid()).To(BeTrue())
		}

		historyValues := []struct {
			value sharedtypes.HistoryStatus
			wire  string
		}{
			{sharedtypes.HistoryStatusAvailable, "available"},
			{sharedtypes.HistoryStatusEmpty, "empty"},
			{sharedtypes.HistoryStatusTransportUnavailable, "transport_unavailable"},
			{sharedtypes.HistoryStatusSchemaError, "schema_error"},
			{sharedtypes.HistoryStatusNotRequested, "not_requested"},
		}
		for _, entry := range historyValues {
			raw, err := json.Marshal(entry.value)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(raw)).To(Equal(`"` + entry.wire + `"`))
			Expect(entry.value.IsValid()).To(BeTrue())
		}
	})

	It("UT-KA-2521-001: rejects unknown wire values instead of accepting them as valid state", func() {
		Expect(sharedtypes.TerminalOutcome("made_up").IsValid()).To(BeFalse())
		Expect(sharedtypes.WorkflowDiscoveryStatus("made_up").IsValid()).To(BeFalse())
		Expect(sharedtypes.HistoryStatus("made_up").IsValid()).To(BeFalse())
		Expect(sharedtypes.TerminalOutcomeUnknown.IsValid()).To(BeFalse())
	})
})
