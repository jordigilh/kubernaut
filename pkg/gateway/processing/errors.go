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

package processing

import (
	"fmt"
	"time"
)

// OperationError provides rich context for processing errors with timing, correlation, and retry information.
// GAP-10: Enhanced Error Wrapping
//
// This structured error type helps operators quickly diagnose issues by providing:
// - Operation: Operation name (e.g., "create_remediation_request")
// - Phase: Processing phase (e.g., "deduplication", "crd_creation")
// - Fingerprint: Signal fingerprint (serves as correlation ID)
// - Namespace: Target namespace
// - Attempts: Number of retry attempts
// - Duration: Total operation duration
// - StartTime: Operation start time
// - CorrelationID: Request correlation ID (typically RR name)
// - Underlying: Wrapped underlying error
//
// Example:
//
//	err := NewOperationError(OperationErrorParams{
//	    Operation:     "create_remediation_request",
//	    Phase:         "crd_creation",
//	    Fingerprint:   "abc123",
//	    Namespace:     "default",
//	    CorrelationID: "rr-pod-crash-abc123",
//	    Attempts:      3,
//	    StartTime:     startTime,
//	    Err:           kubernetesErr,
//	})
type OperationError struct {
	Operation     string        // Operation name (e.g., "create_remediation_request")
	Phase         string        // Processing phase (e.g., "deduplication", "crd_creation")
	Fingerprint   string        // Signal fingerprint (correlation ID)
	Namespace     string        // Target namespace
	Attempts      int           // Number of retry attempts
	Duration      time.Duration // Total operation duration
	StartTime     time.Time     // Operation start time
	CorrelationID string        // Request correlation ID (RR name)
	Underlying    error         // Wrapped underlying error
}

// Error implements the error interface with rich, actionable error messages.
// Format:
//
//	{operation} failed: phase={phase}, fingerprint={fingerprint}, namespace={namespace},
//	attempts={attempts}, duration={duration}, correlation={correlationID}: {underlying}
func (e *OperationError) Error() string {
	return fmt.Sprintf(
		"%s failed: phase=%s, fingerprint=%s, namespace=%s, attempts=%d, duration=%s, correlation=%s: %v",
		e.Operation, e.Phase, e.Fingerprint, e.Namespace,
		e.Attempts, e.Duration, e.CorrelationID, e.Underlying,
	)
}

// Unwrap returns the underlying error for error chain unwrapping.
// This enables errors.Is() and errors.As() to work with OperationError.
func (e *OperationError) Unwrap() error {
	return e.Underlying
}

// OperationErrorParams groups the operation/phase/timing/correlation fields
// shared by NewOperationError and its specializations (NewCRDCreationError,
// NewDeduplicationError). Extracted per AGENTS.md's 8+-param Options-pattern
// rule.
type OperationErrorParams struct {
	Operation     string
	Phase         string
	Fingerprint   string
	Namespace     string
	CorrelationID string
	Attempts      int
	StartTime     time.Time
	Err           error
}

// NewOperationError creates a new operation error with automatic duration calculation.
// Duration is calculated as time.Since(p.StartTime).
func NewOperationError(p OperationErrorParams) *OperationError {
	return &OperationError{
		Operation:     p.Operation,
		Phase:         p.Phase,
		Fingerprint:   p.Fingerprint,
		Namespace:     p.Namespace,
		Attempts:      p.Attempts,
		Duration:      time.Since(p.StartTime),
		StartTime:     p.StartTime,
		CorrelationID: p.CorrelationID,
		Underlying:    p.Err,
	}
}

// CRDCreationError is a specialized error for CRD creation failures.
// Extends OperationError with CRD-specific fields.
type CRDCreationError struct {
	*OperationError
	CRDName    string // RemediationRequest name
	SignalType string // Signal type (alert/event)
	SignalName string // Alert name (if applicable)
}

// CRDCreationErrorParams groups the context needed to build a CRDCreationError.
// Extracted per AGENTS.md's 8+-param Options-pattern rule.
type CRDCreationErrorParams struct {
	Fingerprint string
	Namespace   string
	CRDName     string
	SignalType  string
	SignalName  string
	Attempts    int
	StartTime   time.Time
	Err         error
}

// NewCRDCreationError creates a CRD creation error with full context.
// Automatically sets operation to "create_remediation_request" and phase to "crd_creation".
func NewCRDCreationError(p CRDCreationErrorParams) *CRDCreationError {
	return &CRDCreationError{
		OperationError: NewOperationError(OperationErrorParams{
			Operation:     "create_remediation_request",
			Phase:         "crd_creation",
			Fingerprint:   p.Fingerprint,
			Namespace:     p.Namespace,
			CorrelationID: p.CRDName, // Use CRD name as correlation ID
			Attempts:      p.Attempts,
			StartTime:     p.StartTime,
			Err:           p.Err,
		}),
		CRDName:    p.CRDName,
		SignalType: p.SignalType,
		SignalName: p.SignalName,
	}
}

// Error extends OperationError.Error() with CRD-specific fields.
func (e *CRDCreationError) Error() string {
	baseErr := e.OperationError.Error()
	return fmt.Sprintf("%s, crd_name=%s, signal_type=%s, signal_name=%s",
		baseErr, e.CRDName, e.SignalType, e.SignalName)
}

// DeduplicationError is a specialized error for deduplication failures.
// Extends OperationError with deduplication-specific fields.
type DeduplicationError struct {
	*OperationError
	DedupeStatus string // Deduplication status (new/duplicate/unknown)
}

// NewDeduplicationError creates a deduplication error with full context.
// Automatically sets operation to "check_deduplication" and phase to "deduplication".
func NewDeduplicationError(fingerprint, namespace, dedupeStatus string, attempts int, startTime time.Time, err error) *DeduplicationError {
	return &DeduplicationError{
		OperationError: NewOperationError(OperationErrorParams{
			Operation:     "check_deduplication",
			Phase:         "deduplication",
			Fingerprint:   fingerprint,
			Namespace:     namespace,
			CorrelationID: fingerprint, // Use fingerprint as correlation ID
			Attempts:      attempts,
			StartTime:     startTime,
			Err:           err,
		}),
		DedupeStatus: dedupeStatus,
	}
}

// Error extends OperationError.Error() with deduplication-specific fields.
func (e *DeduplicationError) Error() string {
	baseErr := e.OperationError.Error()
	return fmt.Sprintf("%s, dedupe_status=%s", baseErr, e.DedupeStatus)
}

// RetryError wraps errors from retry operations with retry context.
// This is used by createCRDWithRetry to provide rich context about retry attempts.
type RetryError struct {
	Attempt     int    // Final attempt number (1-based)
	MaxAttempts int    // Maximum retry attempts configured
	OriginalErr error  // Original underlying error
	ErrorType   string // Error type classification (e.g., "transient", "timeout")
	IsRetryable bool   // Whether error was retryable
}

// Error implements the error interface for RetryError.
func (e *RetryError) Error() string {
	return fmt.Sprintf("operation failed after %d/%d attempts (error_type=%s, retryable=%t): %v",
		e.Attempt, e.MaxAttempts, e.ErrorType, e.IsRetryable, e.OriginalErr)
}

// Unwrap returns the underlying error for error chain unwrapping.
func (e *RetryError) Unwrap() error {
	return e.OriginalErr
}
