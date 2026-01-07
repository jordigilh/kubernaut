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

// Package audit provides shared audit types for standardized error details.
//
// This package implements BR-AUDIT-005 v2.0 Gap #7: Standardized error details
// across all Kubernaut services for SOC2 compliance and RR reconstruction.
//
// Error Details Structure:
// This structure is for audit events only (not HTTP responses).
// HTTP responses use RFC7807 per DD-004.
//
// Business Requirement: BR-AUDIT-005 v2.0 (SOC2 Type II + RR Reconstruction)
// Design Decision: DD-ERROR-001 (Error Details Standardization) - TBD
//
// Authority Documents:
// - DD-004: RFC7807 error response standard (HTTP responses only)
// - DD-AUDIT-003 v1.4: Service audit trace requirements
// - ADR-034: Unified audit table design
// - SOC2_AUDIT_IMPLEMENTATION_PLAN.md: Day 4 - Error Details Standardization
package audit

import (
	"fmt"
	"runtime"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// ErrorDetails provides standardized error information for audit events.
//
// Used for SOC2 compliance and RemediationRequest reconstruction (Gap #7).
// This structure is for audit events only (not HTTP responses).
// HTTP responses use RFC7807 per DD-004.
//
// Field Descriptions:
// - Message: Human-readable error description
// - Code: Machine-readable error classification (ERR_[CATEGORY]_[SPECIFIC])
// - Component: Service emitting the error (gateway, aianalysis, workflowexecution, remediationorchestrator)
// - RetryPossible: Indicates if operation can be retried (true=transient, false=permanent)
// - StackTrace: Top N stack frames for debugging (optional, 5-10 frames max)
//
// Error Code Taxonomy:
// - ERR_INVALID_*: Input validation errors (retry=false)
// - ERR_K8S_*: Kubernetes API errors (retry=false usually)
// - ERR_UPSTREAM_*: External service errors (retry=true)
// - ERR_INTERNAL_*: Internal logic errors (retry=depends)
// - ERR_LIMIT_*: Resource limit errors (retry=false usually)
// - ERR_TIMEOUT_*: Timeout errors (retry=true)
type ErrorDetails struct {
	// Message is the human-readable error description.
	// For K8s errors: Use error message from apierrors.
	// For external errors: Use service-specific error message.
	Message string `json:"message"`

	// Code is the machine-readable error classification.
	// Format: ERR_[CATEGORY]_[SPECIFIC]
	// Examples: ERR_INVALID_YAML, ERR_K8S_NOT_FOUND, ERR_UPSTREAM_TIMEOUT
	Code string `json:"code"`

	// Component identifies the service emitting the error.
	// Values: "gateway", "aianalysis", "workflowexecution", "remediationorchestrator"
	Component string `json:"component"`

	// RetryPossible indicates if the operation can be retried.
	// true: Transient error (network timeout, service unavailable)
	// false: Permanent error (invalid input, resource not found)
	RetryPossible bool `json:"retry_possible"`

	// StackTrace contains top N stack frames for debugging (optional).
	// Only populated for internal errors, not external service errors.
	// Limit: 5-10 frames to avoid excessive data.
	StackTrace []string `json:"stack_trace,omitempty"`
}

// NewErrorDetails creates a standardized ErrorDetails structure.
//
// Parameters:
// - component: Service name (gateway, aianalysis, workflowexecution, remediationorchestrator)
// - code: Error code (ERR_[CATEGORY]_[SPECIFIC])
// - message: Human-readable error description
// - retryPossible: Whether the operation can be retried
//
// Example:
//
//	errorDetails := audit.NewErrorDetails(
//	    "gateway",
//	    "ERR_UPSTREAM_TIMEOUT",
//	    "Holmes API request timeout after 30s",
//	    true, // Timeout is transient, can retry
//	)
func NewErrorDetails(component, code, message string, retryPossible bool) *ErrorDetails {
	return &ErrorDetails{
		Message:       message,
		Code:          code,
		Component:     component,
		RetryPossible: retryPossible,
	}
}

// NewErrorDetailsFromK8sError creates ErrorDetails from a Kubernetes API error.
//
// This helper translates K8s errors to standardized ErrorDetails format.
// It automatically determines the appropriate error code and retry guidance.
//
// Parameters:
// - component: Service name emitting the error
// - err: Kubernetes API error (from k8s.io/apimachinery/pkg/api/errors)
//
// Example:
//
//	err := r.Client.Get(ctx, key, wfe)
//	if err != nil {
//	    errorDetails := audit.NewErrorDetailsFromK8sError("workflowexecution", err)
//	    // Use errorDetails in audit event
//	}
func NewErrorDetailsFromK8sError(component string, err error) *ErrorDetails {
	// Default values
	code := "ERR_K8S_UNKNOWN"
	retryPossible := false
	message := err.Error()

	// Translate K8s error types to error codes
	switch {
	case apierrors.IsNotFound(err):
		code = "ERR_K8S_NOT_FOUND"
		retryPossible = false // Not found is permanent until created
	case apierrors.IsAlreadyExists(err):
		code = "ERR_K8S_ALREADY_EXISTS"
		retryPossible = false // Already exists is permanent
	case apierrors.IsConflict(err):
		code = "ERR_K8S_CONFLICT"
		retryPossible = true // Conflicts are often transient (version mismatch)
	case apierrors.IsForbidden(err):
		code = "ERR_K8S_FORBIDDEN"
		retryPossible = false // RBAC issues are permanent
	case apierrors.IsUnauthorized(err):
		code = "ERR_K8S_UNAUTHORIZED"
		retryPossible = false // Auth issues are permanent
	case apierrors.IsInvalid(err):
		code = "ERR_K8S_INVALID"
		retryPossible = false // Invalid input is permanent
	case apierrors.IsTimeout(err):
		code = "ERR_K8S_TIMEOUT"
		retryPossible = true // Timeouts are transient
	case apierrors.IsServerTimeout(err):
		code = "ERR_K8S_SERVER_TIMEOUT"
		retryPossible = true // Server timeouts are transient
	case apierrors.IsServiceUnavailable(err):
		code = "ERR_K8S_SERVICE_UNAVAILABLE"
		retryPossible = true // Service unavailable is transient
	case apierrors.IsInternalError(err):
		code = "ERR_K8S_INTERNAL"
		retryPossible = true // Internal K8s errors may be transient
	}

	return &ErrorDetails{
		Message:       message,
		Code:          code,
		Component:     component,
		RetryPossible: retryPossible,
	}
}

// NewErrorDetailsWithStackTrace creates ErrorDetails with stack trace.
//
// This helper is for internal errors where stack trace debugging is valuable.
// External service errors (Holmes API, Tekton) should NOT include stack traces.
//
// Parameters:
// - component: Service name
// - code: Error code
// - message: Human-readable error description
// - retryPossible: Whether the operation can be retried
// - depth: Number of stack frames to capture (recommended: 5-10, max: 10)
//
// Example:
//
//	errorDetails := audit.NewErrorDetailsWithStackTrace(
//	    "workflowexecution",
//	    "ERR_INTERNAL_STATE",
//	    "Invalid state transition",
//	    false,
//	    10, // Capture 10 stack frames
//	)
func NewErrorDetailsWithStackTrace(component, code, message string, retryPossible bool, depth int) *ErrorDetails {
	// Limit stack trace depth to avoid excessive data
	if depth > 10 {
		depth = 10
	}
	if depth < 0 {
		depth = 0
	}

	stackTrace := captureStackTrace(depth)

	return &ErrorDetails{
		Message:       message,
		Code:          code,
		Component:     component,
		RetryPossible: retryPossible,
		StackTrace:    stackTrace,
	}
}

// captureStackTrace captures the current stack trace.
//
// Parameters:
// - depth: Number of stack frames to capture
//
// Returns:
// - Stack trace as array of strings (format: "file:line function")
func captureStackTrace(depth int) []string {
	if depth == 0 {
		return nil
	}

	stackTrace := make([]string, 0, depth)

	// Skip 2 frames: runtime.Callers + captureStackTrace
	pcs := make([]uintptr, depth+2)
	n := runtime.Callers(2, pcs)
	if n == 0 {
		return nil
	}

	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		stackTrace = append(stackTrace, fmt.Sprintf("%s:%d %s", frame.File, frame.Line, frame.Function))
		if !more || len(stackTrace) >= depth {
			break
		}
	}

	return stackTrace
}


