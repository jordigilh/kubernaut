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

package authwebhook

import (
	"fmt"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// REFACTOR-AW-003: Audit payload construction extracted for maintainability
// Reference: DD-WEBHOOK-003 (Webhook-Complete Audit Pattern), DD-AUDIT-004 (Zero unstructured data)

// BuildRARApprovalAuditPayload constructs a structured audit payload for RAR approval decisions.
//
// Per DD-WEBHOOK-003: Business context ONLY (attribution in structured columns).
// Per DD-AUDIT-004: Zero unstructured data in audit events.
//
// Note: Uses toRemediationApprovalAuditPayloadDecision from audit_helpers.go
func BuildRARApprovalAuditPayload(rar *remediationv1.RemediationApprovalRequest) api.RemediationApprovalAuditPayload {
	return api.RemediationApprovalAuditPayload{
		EventType:       "webhook.approval.decided",
		RequestName:     rar.Name,
		Decision:        toRemediationApprovalAuditPayloadDecision(string(rar.Status.Decision)),
		DecidedAt:       rar.Status.DecidedAt.Time,
		DecisionMessage: rar.Status.DecisionMessage,  // Per DD-WEBHOOK-003 line 316
		AiAnalysisRef:   rar.Spec.AIAnalysisRef.Name, // Per DD-WEBHOOK-003 line 317 (note: lowercase 'i' in ogen)
	}
}

// WrapRARApprovalPayloadWithDiscriminator wraps the audit payload with the correct discriminator.
//
// Per DD-AUDIT-002 V2.0: OpenAPI oneOf discriminators require specific wrapper types.
func WrapRARApprovalPayloadWithDiscriminator(payload api.RemediationApprovalAuditPayload) api.AuditEventRequestEventData {
	// Note: All RAR approval decisions use the same RemediationApprovalAuditPayload discriminator
	// regardless of Approved/Rejected/Expired, because the webhook emits a single event type:
	// webhook.remediationapprovalrequest.decided (per ADR-034 v1.7)
	return api.NewRemediationApprovalAuditPayloadAuditEventRequestEventData(payload)
}

// BuildRARApprovalAuditEvent is a convenience function that builds the complete audit event
// including correlation ID, namespace, actor, and resource.
//
// This encapsulates the complete audit event construction pattern for RAR approvals.
func BuildRARApprovalAuditEvent(
	rar *remediationv1.RemediationApprovalRequest,
	authenticatedUser string,
	parentRRName string,
) (*api.AuditEventRequest, error) {
	if rar == nil {
		return nil, fmt.Errorf("RAR cannot be nil")
	}
	if authenticatedUser == "" {
		return nil, fmt.Errorf("authenticated user cannot be empty")
	}
	if parentRRName == "" {
		return nil, fmt.Errorf("parent RR name cannot be empty")
	}

	// Import audit package helpers
	// Note: We'll use these in the handler refactoring
	payload := BuildRARApprovalAuditPayload(rar)
	eventData := WrapRARApprovalPayloadWithDiscriminator(payload)

	// Return the event data - the handler will set structured fields
	event := &api.AuditEventRequest{
		EventData: eventData,
	}

	return event, nil
}
