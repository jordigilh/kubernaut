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

// Package remediationapprovalrequest provides condition helpers for the RemediationApprovalRequest CRD.
// Per DD-CRD-002 v1.2, this package uses the canonical Kubernetes meta.SetStatusCondition
// and meta.FindStatusCondition functions for all condition operations.
//
// Reference: docs/architecture/decisions/DD-CRD-002-remediationapprovalrequest-conditions.md
package remediationapprovalrequest

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

// ========================================
// CONDITION TYPES (4 per DD-CRD-002-remediationapprovalrequest-conditions)
// ========================================

const (
	// ConditionApprovalPending indicates approval is awaiting human decision
	ConditionApprovalPending = "ApprovalPending"

	// ConditionApprovalDecided indicates a decision was made (approved/rejected)
	ConditionApprovalDecided = "ApprovalDecided"

	// ConditionApprovalExpired indicates the approval request timed out
	ConditionApprovalExpired = "ApprovalExpired"

	// ConditionAuditRecorded indicates audit event was written to DataStorage
	// Per BR-AUDIT-006: Track audit emission for idempotency
	// Pattern: WorkflowExecution AuditRecorded condition
	ConditionAuditRecorded = "AuditRecorded"
)

// ========================================
// CONDITION REASONS
// ========================================

// ApprovalPending reasons
const (
	ReasonAwaitingDecision = "AwaitingDecision"
	ReasonDecisionMade     = "DecisionMade"
)

// ApprovalDecided reasons
const (
	ReasonApproved        = "Approved"
	ReasonRejected        = "Rejected"
	ReasonPendingDecision = "PendingDecision"
)

// ApprovalExpired reasons
const (
	ReasonTimeout    = "Timeout"
	ReasonNotExpired = "NotExpired"
)

// AuditRecorded reasons
const (
	ReasonAuditSucceeded = "AuditSucceeded"
	ReasonAuditFailed    = "AuditFailed"
)

// ========================================
// GENERIC CONDITION FUNCTIONS
// DD-CRD-002 v1.2: MUST use meta.SetStatusCondition and meta.FindStatusCondition
// ========================================

// SetCondition sets or updates a condition on the RemediationApprovalRequest status.
// Uses the canonical Kubernetes meta.SetStatusCondition function per DD-CRD-002 v1.2.
// Records Prometheus metrics for condition status and transitions (BR-ORCH-043) if metrics is provided.
//
// Per DD-METRICS-001: metrics parameter is optional (can be nil) for backward compatibility.
// Controllers with metrics should pass their metrics instance; tests can pass nil.
func SetCondition(rar *remediationv1.RemediationApprovalRequest, conditionType string, status metav1.ConditionStatus, reason, message string, m *rometrics.Metrics) {
	// Get previous condition status for transition tracking
	previousCondition := meta.FindStatusCondition(rar.Status.Conditions, conditionType)
	previousStatus := ""
	if previousCondition != nil {
		previousStatus = string(previousCondition.Status)
	}

	// Set the condition using canonical K8s function
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
	meta.SetStatusCondition(&rar.Status.Conditions, condition)

	// Record metrics (BR-ORCH-043, DD-005) if metrics instance provided
	if m != nil {
		currentStatus := string(status)
		namespace := rar.Namespace
		if namespace == "" {
			namespace = "default" // Fallback for testing or unnamespaced objects
		}

		// Record current condition status (gauge)
		m.RecordConditionStatus("RemediationApprovalRequest", conditionType, currentStatus, namespace)

		// Record condition transition (counter) only if status changed
		if previousStatus != currentStatus {
			m.RecordConditionTransition("RemediationApprovalRequest", conditionType, previousStatus, currentStatus, namespace)
		}
	}
}

// GetCondition returns the condition with the specified type, or nil if not found.
// Uses the canonical Kubernetes meta.FindStatusCondition function per DD-CRD-002 v1.2.
func GetCondition(rar *remediationv1.RemediationApprovalRequest, conditionType string) *metav1.Condition {
	return meta.FindStatusCondition(rar.Status.Conditions, conditionType)
}

// ========================================
// TYPE-SPECIFIC SETTERS
// ========================================

// SetApprovalPending sets the ApprovalPending condition
func SetApprovalPending(rar *remediationv1.RemediationApprovalRequest, pending bool, message string, m *rometrics.Metrics) {
	status := metav1.ConditionTrue
	reason := ReasonAwaitingDecision
	if !pending {
		status = metav1.ConditionFalse
		reason = ReasonDecisionMade
	}
	SetCondition(rar, ConditionApprovalPending, status, reason, message, m)
}

// SetApprovalDecided sets the ApprovalDecided condition
func SetApprovalDecided(rar *remediationv1.RemediationApprovalRequest, decided bool, reason, message string, m *rometrics.Metrics) {
	status := metav1.ConditionTrue
	if !decided {
		status = metav1.ConditionFalse
	}
	SetCondition(rar, ConditionApprovalDecided, status, reason, message, m)
}

// SetApprovalExpired sets the ApprovalExpired condition
func SetApprovalExpired(rar *remediationv1.RemediationApprovalRequest, expired bool, message string, m *rometrics.Metrics) {
	status := metav1.ConditionTrue
	reason := ReasonTimeout
	if !expired {
		status = metav1.ConditionFalse
		reason = ReasonNotExpired
	}
	SetCondition(rar, ConditionApprovalExpired, status, reason, message, m)
}

// SetAuditRecorded sets the AuditRecorded condition
//
// Per BR-AUDIT-006: Track audit event emission for idempotency
// Pattern: WorkflowExecution SetAuditRecorded (pkg/workflowexecution/conditions.go)
//
// Usage:
//
//	// Audit succeeded
//	SetAuditRecorded(rar, true, ReasonAuditSucceeded,
//	    "Approval audit event recorded to DataStorage")
//
//	// Audit failed
//	SetAuditRecorded(rar, false, ReasonAuditFailed,
//	    "Failed to record audit event: DataStorage unavailable")
func SetAuditRecorded(rar *remediationv1.RemediationApprovalRequest, succeeded bool, reason, message string, m *rometrics.Metrics) {
	status := metav1.ConditionTrue
	if !succeeded {
		status = metav1.ConditionFalse
	}
	SetCondition(rar, ConditionAuditRecorded, status, reason, message, m)
}
