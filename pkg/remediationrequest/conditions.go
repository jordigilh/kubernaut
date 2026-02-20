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

// Package remediationrequest provides condition helpers for the RemediationRequest CRD.
// Per DD-CRD-002 v1.2, this package uses the canonical Kubernetes meta.SetStatusCondition
// and meta.FindStatusCondition functions for all condition operations.
//
// Reference: docs/architecture/decisions/DD-CRD-002-remediationrequest-conditions.md
// Business Requirement: BR-ORCH-043
package remediationrequest

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

// ========================================
// CONDITION TYPES (7 per BR-ORCH-043)
// ========================================

const (
	// ConditionReady indicates the RemediationRequest is ready (aggregate condition)
	ConditionReady = "Ready"

	// ConditionSignalProcessingReady indicates SP CRD was created successfully
	ConditionSignalProcessingReady = "SignalProcessingReady"

	// ConditionSignalProcessingComplete indicates SP completed/failed
	ConditionSignalProcessingComplete = "SignalProcessingComplete"

	// ConditionAIAnalysisReady indicates AI CRD was created successfully
	ConditionAIAnalysisReady = "AIAnalysisReady"

	// ConditionAIAnalysisComplete indicates AI completed/failed
	ConditionAIAnalysisComplete = "AIAnalysisComplete"

	// ConditionWorkflowExecutionReady indicates WE CRD was created successfully
	ConditionWorkflowExecutionReady = "WorkflowExecutionReady"

	// ConditionWorkflowExecutionComplete indicates WE completed/failed
	ConditionWorkflowExecutionComplete = "WorkflowExecutionComplete"

	// ConditionRecoveryComplete indicates terminal phase reached
	ConditionRecoveryComplete = "RecoveryComplete"

	// ConditionNotificationDelivered indicates notification delivery outcome
	ConditionNotificationDelivered = "NotificationDelivered"
)

// ========================================
// CONDITION REASONS
// ========================================

// Ready reasons
const (
	ReasonReady    = "Ready"
	ReasonNotReady = "NotReady"
)

// SignalProcessing reasons
const (
	ReasonSignalProcessingCreated        = "SignalProcessingCreated"
	ReasonSignalProcessingCreationFailed = "SignalProcessingCreationFailed"
	ReasonSignalProcessingSucceeded      = "SignalProcessingSucceeded"
	ReasonSignalProcessingFailed         = "SignalProcessingFailed"
	ReasonSignalProcessingTimeout        = "SignalProcessingTimeout"
)

// AIAnalysis reasons
const (
	ReasonAIAnalysisCreated        = "AIAnalysisCreated"
	ReasonAIAnalysisCreationFailed = "AIAnalysisCreationFailed"
	ReasonAIAnalysisSucceeded      = "AIAnalysisSucceeded"
	ReasonAIAnalysisFailed         = "AIAnalysisFailed"
	ReasonAIAnalysisTimeout        = "AIAnalysisTimeout"
	ReasonNoWorkflowSelected       = "NoWorkflowSelected"
)

// WorkflowExecution reasons
const (
	ReasonWorkflowExecutionCreated        = "WorkflowExecutionCreated"
	ReasonWorkflowExecutionCreationFailed = "WorkflowExecutionCreationFailed"
	ReasonWorkflowSucceeded               = "WorkflowSucceeded"
	ReasonWorkflowFailed                  = "WorkflowFailed"
	ReasonWorkflowTimeout                 = "WorkflowTimeout"
	ReasonApprovalPending                 = "ApprovalPending"
)

// Recovery reasons
const (
	ReasonRecoverySucceeded            = "RecoverySucceeded"
	ReasonRecoveryFailed               = "RecoveryFailed"
	ReasonMaxAttemptsReached           = "MaxAttemptsReached"
	ReasonBlockedByConsecutiveFailures = "BlockedByConsecutiveFailures"
	ReasonInProgress                   = "InProgress"
)

// Notification delivery reasons
const (
	ReasonDeliverySucceeded = "DeliverySucceeded"
	ReasonDeliveryFailed   = "DeliveryFailed"
	ReasonUserCancelled    = "UserCancelled"
)

// ========================================
// GENERIC CONDITION FUNCTIONS
// DD-CRD-002 v1.2: MUST use meta.SetStatusCondition and meta.FindStatusCondition
// ========================================

// SetCondition sets or updates a condition on the RemediationRequest status.
// Uses the canonical Kubernetes meta.SetStatusCondition function per DD-CRD-002 v1.2.
// Records Prometheus metrics for condition status and transitions (BR-ORCH-043) if metrics is provided.
//
// Per DD-METRICS-001: metrics parameter is optional (can be nil) for backward compatibility.
// Controllers with metrics should pass their metrics instance; tests can pass nil.
func SetCondition(rr *remediationv1.RemediationRequest, conditionType string, status metav1.ConditionStatus, reason, message string, m *rometrics.Metrics) {
	// Get previous condition status for transition tracking
	previousCondition := meta.FindStatusCondition(rr.Status.Conditions, conditionType)
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
		ObservedGeneration: rr.Generation,
	}
	meta.SetStatusCondition(&rr.Status.Conditions, condition)

	// Record metrics (BR-ORCH-043, DD-005) if metrics instance provided
	if m != nil {
		currentStatus := string(status)
		namespace := rr.Namespace
		if namespace == "" {
			namespace = "default" // Fallback for testing or unnamespaced objects
		}

		// Record current condition status (gauge)
		m.RecordConditionStatus("RemediationRequest", conditionType, currentStatus, namespace)

		// Record condition transition (counter) only if status changed
		if previousStatus != currentStatus {
			m.RecordConditionTransition("RemediationRequest", conditionType, previousStatus, currentStatus, namespace)
		}
	}
}

// GetCondition returns the condition with the specified type, or nil if not found.
// Uses the canonical Kubernetes meta.FindStatusCondition function per DD-CRD-002 v1.2.
func GetCondition(rr *remediationv1.RemediationRequest, conditionType string) *metav1.Condition {
	return meta.FindStatusCondition(rr.Status.Conditions, conditionType)
}

// ========================================
// TYPE-SPECIFIC SETTERS
// ========================================

// SetReady sets the Ready condition on the RemediationRequest
func SetReady(rr *remediationv1.RemediationRequest, ready bool, reason, message string, m *rometrics.Metrics) {
	status := metav1.ConditionTrue
	if !ready {
		status = metav1.ConditionFalse
	}
	SetCondition(rr, ConditionReady, status, reason, message, m)
}

// SetSignalProcessingReady sets the SignalProcessingReady condition
func SetSignalProcessingReady(rr *remediationv1.RemediationRequest, ready bool, message string, m *rometrics.Metrics) {
	status := metav1.ConditionTrue
	reason := ReasonSignalProcessingCreated
	if !ready {
		status = metav1.ConditionFalse
		reason = ReasonSignalProcessingCreationFailed
	}
	SetCondition(rr, ConditionSignalProcessingReady, status, reason, message, m)
}

// SetSignalProcessingComplete sets the SignalProcessingComplete condition
func SetSignalProcessingComplete(rr *remediationv1.RemediationRequest, succeeded bool, reason, message string, m *rometrics.Metrics) {
	status := metav1.ConditionTrue
	if !succeeded {
		status = metav1.ConditionFalse
	}
	SetCondition(rr, ConditionSignalProcessingComplete, status, reason, message, m)
}

// SetAIAnalysisReady sets the AIAnalysisReady condition
func SetAIAnalysisReady(rr *remediationv1.RemediationRequest, ready bool, message string, m *rometrics.Metrics) {
	status := metav1.ConditionTrue
	reason := ReasonAIAnalysisCreated
	if !ready {
		status = metav1.ConditionFalse
		reason = ReasonAIAnalysisCreationFailed
	}
	SetCondition(rr, ConditionAIAnalysisReady, status, reason, message, m)
}

// SetAIAnalysisComplete sets the AIAnalysisComplete condition
func SetAIAnalysisComplete(rr *remediationv1.RemediationRequest, succeeded bool, reason, message string, m *rometrics.Metrics) {
	status := metav1.ConditionTrue
	if !succeeded {
		status = metav1.ConditionFalse
	}
	SetCondition(rr, ConditionAIAnalysisComplete, status, reason, message, m)
}

// SetWorkflowExecutionReady sets the WorkflowExecutionReady condition
func SetWorkflowExecutionReady(rr *remediationv1.RemediationRequest, ready bool, message string, m *rometrics.Metrics) {
	status := metav1.ConditionTrue
	reason := ReasonWorkflowExecutionCreated
	if !ready {
		status = metav1.ConditionFalse
		reason = ReasonWorkflowExecutionCreationFailed
	}
	SetCondition(rr, ConditionWorkflowExecutionReady, status, reason, message, m)
}

// SetWorkflowExecutionComplete sets the WorkflowExecutionComplete condition
func SetWorkflowExecutionComplete(rr *remediationv1.RemediationRequest, succeeded bool, reason, message string, m *rometrics.Metrics) {
	status := metav1.ConditionTrue
	if !succeeded {
		status = metav1.ConditionFalse
	}
	SetCondition(rr, ConditionWorkflowExecutionComplete, status, reason, message, m)
}

// SetRecoveryComplete sets the RecoveryComplete condition
func SetRecoveryComplete(rr *remediationv1.RemediationRequest, succeeded bool, reason, message string, m *rometrics.Metrics) {
	status := metav1.ConditionTrue
	if !succeeded {
		status = metav1.ConditionFalse
	}
	SetCondition(rr, ConditionRecoveryComplete, status, reason, message, m)
}

// SetNotificationDelivered sets the NotificationDelivered condition
func SetNotificationDelivered(rr *remediationv1.RemediationRequest, succeeded bool, reason, message string, m *rometrics.Metrics) {
	status := metav1.ConditionTrue
	if !succeeded {
		status = metav1.ConditionFalse
	}
	SetCondition(rr, ConditionNotificationDelivered, status, reason, message, m)
}
