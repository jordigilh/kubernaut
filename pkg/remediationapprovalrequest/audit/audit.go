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

// Package audit provides audit event generation for RemediationApprovalRequest controller.
// BR-AUDIT-006: Approval decision audit trail (SOC 2 CC8.1 User Attribution)
// DD-AUDIT-002 V2.2: Uses shared pkg/audit library with zero unstructured data.
// DD-AUDIT-003: Implements service-specific audit event types.
package audit

import (
	"context"

	"github.com/go-logr/logr"

	remediationapprovalrequestv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
)

// Event categories
const (
	EventCategoryApproval = "approval"
)

// Event types
const (
	EventTypeApprovalDecision      = "approval.decision"       // P0 - SOC 2 critical
	EventTypeApprovalRequestCreated = "approval.request.created" // P1 - context
	EventTypeApprovalTimeout       = "approval.timeout"        // P1 - operational
)

// Event actions
const (
	EventActionDecisionMade   = "decision_made"
	EventActionRequestCreated = "request_created"
	EventActionTimeout        = "timeout"
)

// Actor types
const (
	ActorTypeService = "service"
	ActorTypeUser    = "user"
	ActorTypeSystem  = "system"
)

// Actor IDs
const (
	ActorIDController = "remediationapprovalrequest-controller"
)

// AuditClient handles audit event generation for RemediationApprovalRequest
type AuditClient struct {
	store audit.AuditStore
	log   logr.Logger
}

// NewAuditClient creates a new audit client
func NewAuditClient(store audit.AuditStore, log logr.Logger) *AuditClient {
	return &AuditClient{
		store: store,
		log:   log,
	}
}

// RecordApprovalDecision records approval decision event (P0 - SOC 2 critical)
// Captures WHO, WHEN, WHAT, WHY for compliance and forensic investigation
//
// TDD RED Phase: Stub implementation - tests will FAIL
func (c *AuditClient) RecordApprovalDecision(ctx context.Context, rar *remediationapprovalrequestv1alpha1.RemediationApprovalRequest) {
	// RED Phase: Empty implementation - tests will fail
	// Next: GREEN phase will implement this method
}
