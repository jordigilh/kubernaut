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

// Package signalprocessing provides condition helpers for SignalProcessing CRDs.
//
// Design Decisions:
//   - DD-SP-002: Kubernetes Conditions Specification
//   - DD-CRD-002: Cross-Service Conditions Standard
//
// Business Requirements:
//   - BR-SP-110: Kubernetes Conditions for Operator Visibility
package signalprocessing

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	spv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// ========================================
// CONDITION TYPES (DD-SP-002)
// ========================================

// Condition types for SignalProcessing
const (
	// ConditionReady indicates the SignalProcessing is ready (or not)
	ConditionReady = "Ready"

	// ConditionEnrichmentComplete indicates K8s context enrichment phase finished
	ConditionEnrichmentComplete = "EnrichmentComplete"

	// ConditionClassificationComplete indicates environment/priority classification finished
	ConditionClassificationComplete = "ClassificationComplete"

	// ConditionCategorizationComplete indicates business categorization finished
	ConditionCategorizationComplete = "CategorizationComplete"

	// ConditionProcessingComplete indicates signal processing completed (terminal state)
	ConditionProcessingComplete = "ProcessingComplete"
)

// ========================================
// CONDITION REASONS - ENRICHMENT (BR-SP-001)
// ========================================

// Condition reasons for EnrichmentComplete
const (
	// ReasonEnrichmentSucceeded - K8s context enriched successfully
	ReasonEnrichmentSucceeded = "EnrichmentSucceeded"

	// ReasonEnrichmentFailed - K8s context enrichment failed
	ReasonEnrichmentFailed = "EnrichmentFailed"

	// ReasonK8sAPITimeout - K8s API call timed out
	ReasonK8sAPITimeout = "K8sAPITimeout"

	// ReasonResourceNotFound - Target resource not found in cluster
	ReasonResourceNotFound = "ResourceNotFound"

	// ReasonRBACDenied - Controller lacks RBAC permissions
	ReasonRBACDenied = "RBACDenied"

	// ReasonDegradedMode - Enrichment completed with partial context (K8s API unavailable)
	ReasonDegradedMode = "DegradedMode"
)

// ========================================
// CONDITION REASONS - CLASSIFICATION (BR-SP-051, BR-SP-070)
// ========================================

// Condition reasons for ClassificationComplete
const (
	// ReasonClassificationSucceeded - environment and priority classified successfully
	ReasonClassificationSucceeded = "ClassificationSucceeded"

	// ReasonClassificationFailed - classification failed
	ReasonClassificationFailed = "ClassificationFailed"

	// ReasonRegoEvaluationError - Rego policy evaluation failed (runtime error)
	ReasonRegoEvaluationError = "RegoEvaluationError"

	// ReasonPolicyNotFound - Rego policy file not found (using fallback)
	ReasonPolicyNotFound = "PolicyNotFound"

	// ReasonInvalidNamespaceLabels - namespace labels missing or invalid
	ReasonInvalidNamespaceLabels = "InvalidNamespaceLabels"

	// ReasonSeverityFallback - priority assigned via severity fallback (BR-SP-071)
	ReasonSeverityFallback = "SeverityFallback"
)

// ========================================
// CONDITION REASONS - CATEGORIZATION (BR-SP-002, BR-SP-080)
// ========================================

// Condition reasons for CategorizationComplete
const (
	// ReasonCategorizationSucceeded - business categorization completed
	ReasonCategorizationSucceeded = "CategorizationSucceeded"

	// ReasonCategorizationFailed - business categorization failed
	ReasonCategorizationFailed = "CategorizationFailed"

	// ReasonInvalidBusinessUnit - business unit not recognized
	ReasonInvalidBusinessUnit = "InvalidBusinessUnit"

	// ReasonInvalidSLATier - SLA tier not valid
	ReasonInvalidSLATier = "InvalidSLATier"
)

// ========================================
// CONDITION REASONS - PROCESSING COMPLETE
// ========================================

// Condition reasons for ProcessingComplete
const (
	// ReasonProcessingSucceeded - signal processing completed successfully
	ReasonProcessingSucceeded = "ProcessingSucceeded"

	// ReasonProcessingFailed - signal processing failed
	ReasonProcessingFailed = "ProcessingFailed"

	// ReasonAuditWriteFailed - audit event write failed (logged, not blocking)
	ReasonAuditWriteFailed = "AuditWriteFailed"

	// ReasonValidationFailed - CRD validation failed
	ReasonValidationFailed = "ValidationFailed"
)

// Ready condition reasons
const (
	ReasonReady    = "Ready"
	ReasonNotReady = "NotReady"
)

// ========================================
// GENERIC CONDITION HELPERS
// ========================================

// SetCondition sets or updates a condition on the SignalProcessing status.
// Uses k8s.io/apimachinery/pkg/api/meta.SetStatusCondition for proper handling.
func SetCondition(sp *spv1.SignalProcessing, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
		ObservedGeneration: sp.Generation,
	}
	meta.SetStatusCondition(&sp.Status.Conditions, condition)
}

// SetReady sets the Ready condition on the SignalProcessing.
func SetReady(sp *spv1.SignalProcessing, ready bool, reason, message string) {
	status := metav1.ConditionTrue
	if !ready {
		status = metav1.ConditionFalse
	}
	SetCondition(sp, ConditionReady, status, reason, message)
}

// GetCondition returns the condition with the specified type, or nil if not found.
func GetCondition(sp *spv1.SignalProcessing, conditionType string) *metav1.Condition {
	return meta.FindStatusCondition(sp.Status.Conditions, conditionType)
}

// IsConditionTrue returns true if the condition exists and has status True.
func IsConditionTrue(sp *spv1.SignalProcessing, conditionType string) bool {
	condition := GetCondition(sp, conditionType)
	return condition != nil && condition.Status == metav1.ConditionTrue
}

// ========================================
// PHASE-SPECIFIC CONDITION HELPERS
// ========================================

// SetEnrichmentComplete sets the EnrichmentComplete condition.
// Per BR-SP-001: K8s context enrichment.
func SetEnrichmentComplete(sp *spv1.SignalProcessing, succeeded bool, reason, message string) {
	status := metav1.ConditionTrue
	if !succeeded {
		status = metav1.ConditionFalse
	}
	if reason == "" {
		reason = ReasonEnrichmentSucceeded
		if !succeeded {
			reason = ReasonEnrichmentFailed
		}
	}
	SetCondition(sp, ConditionEnrichmentComplete, status, reason, message)
}

// SetClassificationComplete sets the ClassificationComplete condition.
// Per BR-SP-051, BR-SP-070: Environment and priority classification.
func SetClassificationComplete(sp *spv1.SignalProcessing, succeeded bool, reason, message string) {
	status := metav1.ConditionTrue
	if !succeeded {
		status = metav1.ConditionFalse
	}
	if reason == "" {
		reason = ReasonClassificationSucceeded
		if !succeeded {
			reason = ReasonClassificationFailed
		}
	}
	SetCondition(sp, ConditionClassificationComplete, status, reason, message)
}

// SetCategorizationComplete sets the CategorizationComplete condition.
// Per BR-SP-002, BR-SP-080: Business categorization.
func SetCategorizationComplete(sp *spv1.SignalProcessing, succeeded bool, reason, message string) {
	status := metav1.ConditionTrue
	if !succeeded {
		status = metav1.ConditionFalse
	}
	if reason == "" {
		reason = ReasonCategorizationSucceeded
		if !succeeded {
			reason = ReasonCategorizationFailed
		}
	}
	SetCondition(sp, ConditionCategorizationComplete, status, reason, message)
}

// SetProcessingComplete sets the ProcessingComplete condition.
// This is the terminal condition indicating overall processing result.
func SetProcessingComplete(sp *spv1.SignalProcessing, succeeded bool, reason, message string) {
	status := metav1.ConditionTrue
	if !succeeded {
		status = metav1.ConditionFalse
	}
	if reason == "" {
		reason = ReasonProcessingSucceeded
		if !succeeded {
			reason = ReasonProcessingFailed
		}
	}
	SetCondition(sp, ConditionProcessingComplete, status, reason, message)
}
