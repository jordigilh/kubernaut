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

package metrics

// Cardinality Protection Helpers
//
// This file provides runtime sanitization to ensure metric labels remain low-cardinality.
// All label values are whitelisted to prevent accidental high-cardinality explosions.
//
// Cardinality Risk Mitigation:
// - Bounded failure reasons (5 values)
// - Bounded validation reasons (6 values)
// - Schema-defined field names (10-12 values)
// - No user-generated content in labels
//
// Target Cardinality: < 100 unique label combinations per metric
// Current Cardinality: ~5-60 combinations (SAFE)

// Failure reasons - bounded set for cardinality protection
const (
	// ReasonPostgreSQLFailure indicates PostgreSQL transaction failure
	ReasonPostgreSQLFailure = "postgresql_failure"

	// ReasonValidationFailure indicates input validation failure
	ReasonValidationFailure = "validation_failure"

	// ReasonContextCanceled indicates context cancellation (BR-STORAGE-016)
	ReasonContextCanceled = "context_canceled"

	// ReasonTransactionRollback indicates transaction rollback
	ReasonTransactionRollback = "transaction_rollback"

	// ReasonUnknown is a catch-all for unexpected failures (protects cardinality)
	ReasonUnknown = "unknown"
)

// Validation failure reasons - bounded set for cardinality protection
const (
	// ValidationReasonRequired indicates a required field is missing
	ValidationReasonRequired = "required"

	// ValidationReasonInvalid indicates an invalid field value
	ValidationReasonInvalid = "invalid"

	// ValidationReasonLengthExceeded indicates field length limit exceeded
	ValidationReasonLengthExceeded = "length_exceeded"

	// ValidationReasonXSSDetected indicates XSS pattern detected (BR-STORAGE-011)
	ValidationReasonXSSDetected = "xss_detected"

	// ValidationReasonSQLInjection indicates SQL injection pattern detected (BR-STORAGE-011)
	ValidationReasonSQLInjection = "sql_injection_detected"

	// ValidationReasonWhitespaceOnly indicates field contains only whitespace
	ValidationReasonWhitespaceOnly = "whitespace_only"
)

// Table names - bounded set for cardinality protection
const (
	TableRemediationAudit = "remediation_audit"
	TableAIAnalysisAudit  = "aianalysis_audit"
	TableWorkflowAudit    = "workflow_audit"
	TableExecutionAudit   = "execution_audit"
)

// Operation status - bounded set for cardinality protection
const (
	StatusSuccess = "success"
	StatusFailure = "failure"
)

// Audit write statuses - bounded set for cardinality protection (GAP-10)
const (
	AuditStatusSuccess     = "success"
	AuditStatusFailure     = "failure"
	AuditStatusDLQFallback = "dlq_fallback" // DD-009: Dead Letter Queue fallback
)

// Service names - bounded set for cardinality protection (GAP-10)
const (
	ServiceNotification      = "notification"
	ServiceSignalProcessing  = "signal-processing"
	ServiceOrchestration     = "orchestration"
	ServiceAIAnalysis        = "ai-analysis"
	ServiceWorkflowExecution = "workflow-execution"
	ServiceEffectiveness     = "effectiveness"
)

// Query operations - bounded set for cardinality protection
const (
	OperationList   = "list"
	OperationGet    = "get"
	OperationFilter = "filter"
)

// SanitizeFailureReason ensures the failure reason is from a known bounded set.
// This prevents accidental high-cardinality labels from error messages or user input.
//
// Usage:
//
//	reason := metrics.SanitizeFailureReason("postgresql_failure")
//	// Use sanitized reason in metric labels
//
// Returns:
//   - Original reason if it's in the known set
//   - ReasonUnknown if it's not recognized (prevents cardinality explosion)
func SanitizeFailureReason(reason string) string {
	knownReasons := map[string]bool{
		ReasonPostgreSQLFailure:   true,
		ReasonValidationFailure:   true,
		ReasonContextCanceled:     true,
		ReasonTransactionRollback: true,
	}

	if knownReasons[reason] {
		return reason
	}

	// Unknown reason - use catch-all to protect cardinality
	return ReasonUnknown
}

// SanitizeValidationReason ensures the validation reason is from a known bounded set.
//
// Usage:
//
//	metrics.ValidationFailures.WithLabelValues(
//	    "name",
//	    metrics.SanitizeValidationReason("required"),
//	).Inc()
//
// Returns:
//   - Original reason if it's in the known set
//   - ValidationReasonInvalid if it's not recognized (catch-all)
func SanitizeValidationReason(reason string) string {
	knownReasons := map[string]bool{
		ValidationReasonRequired:       true,
		ValidationReasonInvalid:        true,
		ValidationReasonLengthExceeded: true,
		ValidationReasonXSSDetected:    true,
		ValidationReasonSQLInjection:   true,
		ValidationReasonWhitespaceOnly: true,
	}

	if knownReasons[reason] {
		return reason
	}

	// Unknown reason - use catch-all to protect cardinality
	return ValidationReasonInvalid
}

// SanitizeTableName ensures the table name is from a known bounded set.
//
// Usage:
//
//	metrics.WriteTotal.WithLabelValues(
//	    metrics.SanitizeTableName("remediation_audit"),
//	    metrics.StatusSuccess,
//	).Inc()
//
// Returns:
//   - Original table name if it's in the known set
//   - Empty string if not recognized (caller should handle)
func SanitizeTableName(table string) string {
	knownTables := map[string]bool{
		TableRemediationAudit: true,
		TableAIAnalysisAudit:  true,
		TableWorkflowAudit:    true,
		TableExecutionAudit:   true,
	}

	if knownTables[table] {
		return table
	}

	// Unknown table - return empty string (caller decides how to handle)
	return ""
}

// SanitizeStatus ensures the status is either "success" or "failure".
//
// Usage:
//
//	metrics.WriteTotal.WithLabelValues(
//	    metrics.TableRemediationAudit,
//	    metrics.SanitizeStatus("success"),
//	).Inc()
//
// Returns:
//   - StatusSuccess if status indicates success
//   - StatusFailure otherwise (default to failure for safety)
func SanitizeStatus(status string) string {
	if status == StatusSuccess {
		return StatusSuccess
	}
	return StatusFailure
}

// SanitizeQueryOperation ensures the query operation is from a known bounded set.
//
// Usage:
//
//	metrics.QueryDuration.WithLabelValues(
//	    metrics.SanitizeQueryOperation("list"),
//	).Observe(duration.Seconds())
//
// Returns:
//   - Original operation if it's in the known set
//   - OperationFilter if not recognized (catch-all)
func SanitizeQueryOperation(operation string) string {
	knownOperations := map[string]bool{
		OperationList:   true,
		OperationGet:    true,
		OperationFilter: true,
	}

	if knownOperations[operation] {
		return operation
	}

	// Unknown operation - use catch-all to protect cardinality
	return OperationFilter
}

// Cardinality Summary:
//
// Metric: datastorage_validation_failures_total{field, reason}
//   - Maximum cardinality: ~60 combinations (10 fields × 6 reasons)
//   - Protected by: SanitizeValidationReason() + schema-defined field names
//
// Metric: datastorage_write_total{table, status}
//   - Maximum cardinality: 8 combinations (4 tables × 2 statuses)
//   - Protected by: SanitizeTableName() + SanitizeStatus()
//
// Metric: datastorage_query_duration_seconds{operation}
//   - Maximum cardinality: 3 values
//   - Protected by: SanitizeQueryOperation()
//
// Total Maximum Cardinality: < 100 (SAFE for Prometheus)
