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
		}
		Expect(store.events[0].Data["terminal_outcome"]).To(Equal(string(sharedtypes.TerminalOutcomeUnknown)))
		Expect(store.events[0].Data["workflow_discovery_status"]).To(Equal(string(sharedtypes.WorkflowDiscoveryStatusNotStarted)))
		Expect(store.events[1].Data["workflow_discovery_status"]).To(Equal(string(sharedtypes.WorkflowDiscoveryStatusStarted)))
		Expect(store.events[2].Data["terminal_outcome"]).To(Equal(string(sharedtypes.TerminalOutcomeNoMatchingWorkflows)))
		Expect(store.events[2].EventOutcome).To(Equal(audit.OutcomeSuccess))
	})
})
