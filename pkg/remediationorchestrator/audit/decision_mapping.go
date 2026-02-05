/*
Copyright 2025 Jordi Gil.

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

package audit

import (
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// REFACTOR-RO-AUD-001: Approval decision mapping type to eliminate triple switch statements
// Reference: ADR-034 v1.7 (Two-Event Audit Trail Pattern), BR-AUDIT-006

// ApprovalDecisionMapping encapsulates all mappings for an approval decision.
//
// This eliminates the need for three separate switch statements in BuildApprovalDecisionEvent:
// 1. decision → (action, outcome)
// 2. decision → payload EventType
// 3. decision → discriminator wrapper
//
// Pattern: Strategy pattern for decision-specific behavior
type ApprovalDecisionMapping struct {
	// Action is the event_action field (e.g., "approved", "rejected", "expired")
	Action string

	// Outcome is the event_outcome field (e.g., "success", "failure")
	Outcome api.AuditEventRequestEventOutcome

	// PayloadEventType is the EventType field in the payload
	PayloadEventType api.RemediationOrchestratorAuditPayloadEventType

	// DiscriminatorWrapper wraps the payload with the correct oneOf discriminator
	DiscriminatorWrapper func(api.RemediationOrchestratorAuditPayload) api.AuditEventRequestEventData
}

// approvalDecisionMappings defines all decision mappings in a single, maintainable location.
//
// Per ADR-034 v1.7: RemediationOrchestrator emits orchestration category events
// for RAR approvals, capturing WHAT/WHY (business context).
var approvalDecisionMappings = map[string]ApprovalDecisionMapping{
	"Approved": {
		Action:           ActionApproved,
		Outcome:          api.AuditEventRequestEventOutcomeSuccess,
		PayloadEventType: api.RemediationOrchestratorAuditPayloadEventTypeOrchestratorApprovalApproved,
		DiscriminatorWrapper: func(payload api.RemediationOrchestratorAuditPayload) api.AuditEventRequestEventData {
			return api.NewAuditEventRequestEventDataOrchestratorApprovalApprovedAuditEventRequestEventData(payload)
		},
	},
	"Rejected": {
		Action:           ActionRejected,
		Outcome:          api.AuditEventRequestEventOutcomeFailure,
		PayloadEventType: api.RemediationOrchestratorAuditPayloadEventTypeOrchestratorApprovalRejected,
		DiscriminatorWrapper: func(payload api.RemediationOrchestratorAuditPayload) api.AuditEventRequestEventData {
			return api.NewAuditEventRequestEventDataOrchestratorApprovalRejectedAuditEventRequestEventData(payload)
		},
	},
	"Expired": {
		Action:           ActionExpired,
		Outcome:          api.AuditEventRequestEventOutcomeFailure,
		PayloadEventType: api.RemediationOrchestratorAuditPayloadEventTypeOrchestratorApprovalRejected, // Expired uses Rejected discriminator
		DiscriminatorWrapper: func(payload api.RemediationOrchestratorAuditPayload) api.AuditEventRequestEventData {
			return api.NewAuditEventRequestEventDataOrchestratorApprovalRejectedAuditEventRequestEventData(payload)
		},
	},
}

// GetApprovalDecisionMapping returns the mapping for a given decision.
//
// Returns the mapping and true if found, or a default Rejected mapping and false if not found.
// This defensive approach ensures audit events are always emitted, even for unexpected decisions.
func GetApprovalDecisionMapping(decision string) (ApprovalDecisionMapping, bool) {
	mapping, ok := approvalDecisionMappings[decision]
	if !ok {
		// Default to Rejected mapping for unknown decisions (defensive programming)
		return approvalDecisionMappings["Rejected"], false
	}
	return mapping, true
}
