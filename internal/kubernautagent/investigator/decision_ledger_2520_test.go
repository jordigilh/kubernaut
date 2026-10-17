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
	"context"
	"sync"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

type decisionLedgerAuditStore struct {
	mu     sync.Mutex
	events []*audit.AuditEvent
}

func (s *decisionLedgerAuditStore) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

var _ audit.AuditStore = (*decisionLedgerAuditStore)(nil)

var _ = Describe("Investigation decision ledger transitions — #2520", func() {
	It("UT-KA-2520-003: records ordered typed transitions under one logical correlation", func() {
		store := &decisionLedgerAuditStore{}
		inv := &Investigator{auditStore: store, logger: logr.Discard()}
		ctx := audit.WithCorrelationID(context.Background(), "rr-2520-003")

		inv.emitInvestigationDecision(ctx, investigationDecisionParams{
			CorrelationID:      "rr-2520-003",
			Phase:              katypes.PhaseRCA,
			TurnKind:           investigationDecisionTurnState,
			TurnID:             "rca:complete",
			Result:             &katypes.InvestigationResult{HistoryStatus: sharedtypes.HistoryStatusAvailable},
			ContinuationReason: "rca_complete",
		})
		inv.emitInvestigationDecision(ctx, investigationDecisionParams{
			CorrelationID: "rr-2520-003",
			Phase:         katypes.PhaseWorkflowDiscovery,
			TurnKind:      investigationDecisionTurnState,
			TurnID:        "workflow_discovery:started",
			Result: &katypes.InvestigationResult{
				HistoryStatus:           sharedtypes.HistoryStatusAvailable,
				WorkflowDiscoveryStatus: sharedtypes.WorkflowDiscoveryStatusStarted,
			},
			ContinuationReason: "workflow_discovery_started",
		})
		inv.emitInvestigationDecision(ctx, investigationDecisionParams{
			CorrelationID: "rr-2520-003",
			Phase:         katypes.PhaseWorkflowDiscovery,
			TurnKind:      investigationDecisionTurnTerminal,
			TurnID:        "investigation:complete",
			Result: &katypes.InvestigationResult{
				HistoryStatus:           sharedtypes.HistoryStatusEmpty,
				WorkflowDiscoveryStatus: sharedtypes.WorkflowDiscoveryStatusCompletedNoMatch,
				TerminalOutcome:         sharedtypes.TerminalOutcomeNoMatchingWorkflows,
			},
			ContinuationReason: "investigation_complete",
		})

		Expect(store.events).To(HaveLen(3))
		for _, event := range store.events {
			Expect(event.CorrelationID).To(Equal("rr-2520-003"))
			Expect(event.Data["event_id"]).NotTo(BeEmpty())
			Expect(event.EventID.String()).To(Equal(event.Data["event_id"]))
		}
		Expect(store.events[0].Data["terminal_outcome"]).To(Equal(string(sharedtypes.TerminalOutcomeUnknown)))
		Expect(store.events[0].Data["workflow_discovery_status"]).To(Equal(string(sharedtypes.WorkflowDiscoveryStatusNotStarted)))
		Expect(store.events[1].Data["workflow_discovery_status"]).To(Equal(string(sharedtypes.WorkflowDiscoveryStatusStarted)))
		Expect(store.events[2].Data["terminal_outcome"]).To(Equal(string(sharedtypes.TerminalOutcomeNoMatchingWorkflows)))
		Expect(store.events[2].EventOutcome).To(Equal(audit.OutcomeSuccess))
	})

	It("UT-KA-2520-004: keeps skipped discovery terminal events in the RCA phase", func() {
		Expect(terminalDecisionPhase(&katypes.InvestigationResult{
			WorkflowDiscoveryStatus: sharedtypes.WorkflowDiscoveryStatusSkippedDueToFailure,
		})).To(Equal(katypes.PhaseRCA))
	})

	It("UT-KA-2520-005: preserves problem-resolved and not-actionable discovery outcomes", func() {
		resolved := &katypes.InvestigationResult{
			InvestigationOutcome: katypes.InvestigationOutcomeProblemResolved,
		}
		applyWorkflowDiscoveryTypedStatus(resolved, nil, nil)
		Expect(resolved.WorkflowDiscoveryStatus).To(Equal(sharedtypes.WorkflowDiscoveryStatusCompletedNoMatch))
		Expect(resolved.TerminalOutcome).To(Equal(sharedtypes.TerminalOutcomeProblemResolved))

		actionable := false
		notActionable := &katypes.InvestigationResult{IsActionable: &actionable}
		applyWorkflowDiscoveryTypedStatus(notActionable, nil, nil)
		Expect(notActionable.WorkflowDiscoveryStatus).To(Equal(sharedtypes.WorkflowDiscoveryStatusCompletedNoMatch))
		Expect(notActionable.TerminalOutcome).To(BeEmpty())
	})

	It("UT-KA-2520-006: normalizes the human-readable tool budget reason for the ledger wire value", func() {
		Expect(decisionFailureClass(ReasonToolBudgetExhausted, sharedtypes.TerminalOutcomeUnknown)).To(Equal("tool_budget_exhausted"))
	})

	It("UT-KA-2520-007: separates workflow-loop attempt namespaces in turn IDs", func() {
		Expect(decisionLoopPrefix(katypes.PhaseWorkflowDiscovery, "workflow_selection")).To(Equal("workflow_discovery:workflow_selection"))
		Expect(decisionLoopPrefix(katypes.PhaseWorkflowDiscovery, "workflow_self_correction:1")).NotTo(Equal(
			decisionLoopPrefix(katypes.PhaseWorkflowDiscovery, "workflow_self_correction:2")))
	})

	It("UT-KA-2520-008: records cancelled workflow parse retries as discovery-started", func() {
		store := &decisionLedgerAuditStore{}
		inv := &Investigator{auditStore: store, logger: logr.Discard()}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		inv.emitParseRetryDecision(ctx, parseRetryDecisionParams{
			Phase:         katypes.PhaseWorkflowDiscovery,
			CorrelationID: "rr-2520-008",
			Attempt:       1,
			Cancelled:     true,
		})

		Expect(store.events).To(HaveLen(1))
		Expect(store.events[0].Data["workflow_discovery_status"]).To(Equal(string(sharedtypes.WorkflowDiscoveryStatusStarted)))
	})

	It("UT-KA-2520-009: namespaces deterministic decision IDs by session", func() {
		firstStore := &decisionLedgerAuditStore{}
		inv := &Investigator{auditStore: firstStore, logger: logr.Discard()}
		params := investigationDecisionParams{
			CorrelationID: "rr-2520-009",
			Phase:         katypes.PhaseRCA,
			TurnKind:      investigationDecisionTurnState,
			TurnID:        "rca:complete",
			Result:        &katypes.InvestigationResult{HistoryStatus: sharedtypes.HistoryStatusAvailable},
		}
		inv.emitInvestigationDecision(context.Background(), params)
		params.SessionID = "session-b"
		inv.emitInvestigationDecision(context.Background(), params)

		Expect(firstStore.events).To(HaveLen(2))
		Expect(firstStore.events[0].EventID).NotTo(Equal(firstStore.events[1].EventID))

		secondStore := &decisionLedgerAuditStore{}
		inv.auditStore = secondStore
		params.SessionID = ""
		inv.emitInvestigationDecision(context.Background(), params)
		inv.emitInvestigationDecision(context.Background(), params)
		Expect(secondStore.events).To(HaveLen(2))
		Expect(secondStore.events[0].EventID).To(Equal(secondStore.events[1].EventID))
	})
})
