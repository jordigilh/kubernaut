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

// Package conditions provides shared utilities for Kubernetes Conditions across all CRD controllers.
//
// Design Decision: DD-SHARED-001 (pending)
// Business Requirement: BR-SHARED-001 - Reduce code duplication across services
//
// Usage Pattern:
//
//	// In service-specific package (e.g., pkg/workflowexecution/conditions.go)
//	func SetCondition(wfe *workflowexecutionv1.WorkflowExecution, conditionType string, status metav1.ConditionStatus, reason, message string) {
//	    conditions.Set(&wfe.Status.Conditions, conditionType, status, reason, message)
//	}
//
//	func GetCondition(wfe *workflowexecutionv1.WorkflowExecution, conditionType string) *metav1.Condition {
//	    return conditions.Get(wfe.Status.Conditions, conditionType)
//	}
//
// Benefits:
// - Single source of truth for conditions logic
// - Consistent behavior across all services
// - Reduced duplication (~80 lines across 5 services)
// - Easier to maintain and test
package conditions

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Set sets or updates a condition in the conditions slice.
// This is a low-level function that works directly with condition slices.
//
// Parameters:
//   - conditions: Pointer to the conditions slice (e.g., &obj.Status.Conditions)
//   - conditionType: The condition type (e.g., "ValidationComplete")
//   - status: The condition status (True, False, or Unknown)
//   - reason: Machine-readable reason (e.g., "ValidationSucceeded")
//   - message: Human-readable message with context
//
// This function delegates to k8s.io/apimachinery/pkg/api/meta.SetStatusCondition
// to ensure compliance with Kubernetes API conventions.
func Set(conditions *[]metav1.Condition, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
	meta.SetStatusCondition(conditions, condition)
}

// SetWithGeneration sets or updates a condition with ObservedGeneration.
// Use this when the caller needs to track which generation of the resource
// the condition reflects (e.g., for CRD status subresources).
func SetWithGeneration(conditions *[]metav1.Condition, conditionType string, status metav1.ConditionStatus, reason, message string, observedGeneration int64) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
		ObservedGeneration: observedGeneration,
	}
	meta.SetStatusCondition(conditions, condition)
}

// Get returns the condition with the specified type, or nil if not found.
//
// Parameters:
//   - conditions: The conditions slice (e.g., obj.Status.Conditions)
//   - conditionType: The condition type to search for
//
// Returns:
//   - *metav1.Condition: The condition if found, nil otherwise
//
// This function delegates to k8s.io/apimachinery/pkg/api/meta.FindStatusCondition
// to ensure compliance with Kubernetes API conventions.
func Get(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	return meta.FindStatusCondition(conditions, conditionType)
}

// IsTrue returns true if the condition exists and has status True.
//
// Parameters:
//   - conditions: The conditions slice (e.g., obj.Status.Conditions)
//   - conditionType: The condition type to check
//
// Returns:
//   - bool: true if condition exists and Status == True, false otherwise
//
// This is a convenience function for the common pattern of checking
// if a condition is set to True.
func IsTrue(conditions []metav1.Condition, conditionType string) bool {
	condition := Get(conditions, conditionType)
	return condition != nil && condition.Status == metav1.ConditionTrue
}

// IsFalse returns true if the condition exists and has status False.
//
// Parameters:
//   - conditions: The conditions slice (e.g., obj.Status.Conditions)
//   - conditionType: The condition type to check
//
// Returns:
//   - bool: true if condition exists and Status == False, false otherwise
func IsFalse(conditions []metav1.Condition, conditionType string) bool {
	condition := Get(conditions, conditionType)
	return condition != nil && condition.Status == metav1.ConditionFalse
}

// IsUnknown returns true if the condition exists and has status Unknown.
//
// Parameters:
//   - conditions: The conditions slice (e.g., obj.Status.Conditions)
//   - conditionType: The condition type to check
//
// Returns:
//   - bool: true if condition exists and Status == Unknown, false otherwise
func IsUnknown(conditions []metav1.Condition, conditionType string) bool {
	condition := Get(conditions, conditionType)
	return condition != nil && condition.Status == metav1.ConditionUnknown
}
