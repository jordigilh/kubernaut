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

// Package conditions provides condition helpers for the EffectivenessAssessment CRD.
// Per DD-CRD-002 v1.2, this package uses the canonical Kubernetes meta.SetStatusCondition
// and meta.FindStatusCondition functions for all condition operations.
//
// Reference: docs/architecture/decisions/DD-CRD-002-effectivenessassessment-conditions.md
// Business Requirements: BR-EM-001, BR-EM-002, BR-EM-003, BR-EM-004
package conditions

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
)

// ========================================
// CONDITION TYPES (2 per DD-CRD-002-EA)
// ========================================

const (
	// ConditionAssessmentComplete indicates the assessment reached a terminal state.
	ConditionAssessmentComplete = "AssessmentComplete"

	// ConditionSpecIntegrity indicates whether the post-remediation spec hash is still valid.
	// Set on every reconcile after HashComputed=true (DD-EM-002 v1.1).
	ConditionSpecIntegrity = "SpecIntegrity"
)

// ========================================
// CONDITION REASONS: AssessmentComplete
// ========================================

const (
	// ReasonAssessmentFull indicates all enabled components were assessed successfully.
	ReasonAssessmentFull = "AssessmentFull"

	// ReasonAssessmentPartial indicates some components were assessed but not all.
	ReasonAssessmentPartial = "AssessmentPartial"

	// ReasonAssessmentExpired indicates the validity window expired before completion.
	ReasonAssessmentExpired = "AssessmentExpired"

	// ReasonSpecDrift indicates the target resource spec was modified during assessment.
	// Assessment is invalidated — effectiveness score = 0.0 (DD-EM-002 v1.1).
	ReasonSpecDrift = "SpecDrift"

	// ReasonMetricsTimedOut indicates metrics were not available before validity expired.
	ReasonMetricsTimedOut = "MetricsTimedOut"

	// ReasonNoExecution indicates no workflow execution was found for this remediation.
	ReasonNoExecution = "NoExecution"
)

// ========================================
// CONDITION REASONS: SpecIntegrity
// ========================================

const (
	// ReasonSpecUnchanged indicates the target resource spec hash matches the post-remediation hash.
	ReasonSpecUnchanged = "SpecUnchanged"

	// ReasonSpecDrifted indicates the target resource spec hash has changed since post-remediation.
	ReasonSpecDrifted = "SpecDrifted"
)

// ========================================
// GENERIC CONDITION FUNCTIONS
// DD-CRD-002 v1.2: MUST use meta.SetStatusCondition and meta.FindStatusCondition
// ========================================

// SetCondition sets or updates a condition on the EffectivenessAssessment status.
// Uses the canonical Kubernetes meta.SetStatusCondition function per DD-CRD-002 v1.2.
func SetCondition(ea *eav1.EffectivenessAssessment, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
	meta.SetStatusCondition(&ea.Status.Conditions, condition)
}

// GetCondition returns the condition with the specified type, or nil if not found.
// Uses the canonical Kubernetes meta.FindStatusCondition function per DD-CRD-002 v1.2.
func GetCondition(ea *eav1.EffectivenessAssessment, conditionType string) *metav1.Condition {
	return meta.FindStatusCondition(ea.Status.Conditions, conditionType)
}

// IsConditionTrue returns true if the condition exists and has status True.
func IsConditionTrue(ea *eav1.EffectivenessAssessment, conditionType string) bool {
	condition := GetCondition(ea, conditionType)
	return condition != nil && condition.Status == metav1.ConditionTrue
}
