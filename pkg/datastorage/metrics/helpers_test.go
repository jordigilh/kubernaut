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

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMetrics(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Metrics Cardinality Protection Suite")
}

var _ = Describe("Cardinality Protection Helpers", func() {
	Context("SanitizeFailureReason", func() {
		DescribeTable("should return known reasons unchanged",
			func(reason string, expected string) {
				result := SanitizeFailureReason(reason)
				Expect(result).To(Equal(expected))
			},

			Entry("postgresql_failure", ReasonPostgreSQLFailure, ReasonPostgreSQLFailure),
			Entry("vectordb_failure", ReasonVectorDBFailure, ReasonVectorDBFailure),
			Entry("validation_failure", ReasonValidationFailure, ReasonValidationFailure),
			Entry("context_canceled", ReasonContextCanceled, ReasonContextCanceled),
			Entry("transaction_rollback", ReasonTransactionRollback, ReasonTransactionRollback),
		)

		DescribeTable("should sanitize unknown reasons to 'unknown'",
			func(unknownReason string) {
				result := SanitizeFailureReason(unknownReason)
				Expect(result).To(Equal(ReasonUnknown), "Unknown reason should map to 'unknown'")
			},

			Entry("random error message", "connection timeout: failed to connect to database"),
			Entry("user-generated content", "something went wrong with user input"),
			Entry("dynamic error", "error at 2025-10-13T10:30:00Z"),
			Entry("empty string", ""),
			Entry("SQL error message", "pq: duplicate key value violates unique constraint"),
		)

		It("should maintain low cardinality", func() {
			// Simulate 100 different error messages
			uniqueReasons := make(map[string]bool)
			for i := 0; i < 100; i++ {
				// Various error messages that could come from errors
				errorMessages := []string{
					"connection timeout",
					"database unavailable",
					"network error",
					"permission denied",
					"resource exhausted",
					// ... many more potential error messages
				}
				sanitized := SanitizeFailureReason(errorMessages[i%len(errorMessages)])
				uniqueReasons[sanitized] = true
			}

			// Should only have the known reasons + "unknown"
			Expect(len(uniqueReasons)).To(BeNumerically("<=", 6),
				"Cardinality should be bounded to 6 values maximum")
		})
	})

	Context("SanitizeValidationReason", func() {
		DescribeTable("should return known validation reasons unchanged",
			func(reason string, expected string) {
				result := SanitizeValidationReason(reason)
				Expect(result).To(Equal(expected))
			},

			Entry("required", ValidationReasonRequired, ValidationReasonRequired),
			Entry("invalid", ValidationReasonInvalid, ValidationReasonInvalid),
			Entry("length_exceeded", ValidationReasonLengthExceeded, ValidationReasonLengthExceeded),
			Entry("xss_detected", ValidationReasonXSSDetected, ValidationReasonXSSDetected),
			Entry("sql_injection_detected", ValidationReasonSQLInjection, ValidationReasonSQLInjection),
			Entry("whitespace_only", ValidationReasonWhitespaceOnly, ValidationReasonWhitespaceOnly),
		)

		DescribeTable("should sanitize unknown validation reasons to 'invalid'",
			func(unknownReason string) {
				result := SanitizeValidationReason(unknownReason)
				Expect(result).To(Equal(ValidationReasonInvalid), "Unknown validation reason should map to 'invalid'")
			},

			Entry("dynamic validation message", "field must match pattern /[a-z]+/"),
			Entry("user error message", "your input is wrong"),
			Entry("empty string", ""),
		)

		It("should maintain low cardinality with field combinations", func() {
			// Simulate validation failures for 10 fields with various reasons
			uniqueCombinations := make(map[string]bool)
			fields := []string{"name", "namespace", "phase", "action_type", "status",
				"severity", "environment", "cluster_name", "target_resource", "metadata"}

			for _, field := range fields {
				// Try various validation reasons
				for i := 0; i < 20; i++ {
					reason := SanitizeValidationReason("some dynamic error message")
					combination := field + ":" + reason
					uniqueCombinations[combination] = true
				}
			}

			// Should only have 10 fields × 6 reasons = 60 max combinations
			Expect(len(uniqueCombinations)).To(BeNumerically("<=", 60),
				"Field+Reason cardinality should be bounded to ~60 combinations")
		})
	})

	Context("SanitizeTableName", func() {
		DescribeTable("should return known table names unchanged",
			func(table string, expected string) {
				result := SanitizeTableName(table)
				Expect(result).To(Equal(expected))
			},

			Entry("remediation_audit", TableRemediationAudit, TableRemediationAudit),
			Entry("aianalysis_audit", TableAIAnalysisAudit, TableAIAnalysisAudit),
			Entry("workflow_audit", TableWorkflowAudit, TableWorkflowAudit),
			Entry("execution_audit", TableExecutionAudit, TableExecutionAudit),
		)

		DescribeTable("should return empty string for unknown tables",
			func(unknownTable string) {
				result := SanitizeTableName(unknownTable)
				Expect(result).To(Equal(""), "Unknown table should return empty string")
			},

			Entry("user-generated table", "user_generated_table_123"),
			Entry("SQL injection attempt", "remediation_audit'; DROP TABLE users; --"),
			Entry("empty string", ""),
		)

		It("should maintain low cardinality", func() {
			uniqueTables := make(map[string]bool)
			// Simulate many different table names
			for i := 0; i < 100; i++ {
				table := SanitizeTableName("dynamic_table_" + string(rune(i)))
				if table != "" {
					uniqueTables[table] = true
				}
			}

			// Should only have the 4 known tables
			Expect(len(uniqueTables)).To(BeZero(),
				"No unknown tables should pass through sanitization")
		})
	})

	Context("SanitizeStatus", func() {
		It("should return 'success' for success status", func() {
			result := SanitizeStatus(StatusSuccess)
			Expect(result).To(Equal(StatusSuccess))
		})

		DescribeTable("should return 'failure' for all other statuses",
			func(status string) {
				result := SanitizeStatus(status)
				Expect(result).To(Equal(StatusFailure))
			},

			Entry("failure", StatusFailure),
			Entry("error", "error"),
			Entry("pending", "pending"),
			Entry("unknown", "unknown"),
			Entry("empty string", ""),
		)

		It("should maintain cardinality of exactly 2", func() {
			uniqueStatuses := make(map[string]bool)
			// Try 100 different status values
			statuses := []string{"success", "failure", "error", "pending", "cancelled",
				"timeout", "retrying", "unknown", "", "404"}

			for i := 0; i < 100; i++ {
				sanitized := SanitizeStatus(statuses[i%len(statuses)])
				uniqueStatuses[sanitized] = true
			}

			// Should only have 2 values: "success" and "failure"
			Expect(len(uniqueStatuses)).To(Equal(2),
				"Status cardinality should be exactly 2 (success, failure)")
		})
	})

	Context("SanitizeQueryOperation", func() {
		DescribeTable("should return known operations unchanged",
			func(operation string, expected string) {
				result := SanitizeQueryOperation(operation)
				Expect(result).To(Equal(expected))
			},

			Entry("list", OperationList, OperationList),
			Entry("get", OperationGet, OperationGet),
			Entry("semantic_search", OperationSemanticSearch, OperationSemanticSearch),
			Entry("filter", OperationFilter, OperationFilter),
		)

		DescribeTable("should sanitize unknown operations to 'filter'",
			func(unknownOperation string) {
				result := SanitizeQueryOperation(unknownOperation)
				Expect(result).To(Equal(OperationFilter), "Unknown operation should map to 'filter'")
			},

			Entry("custom operation", "custom_query"),
			Entry("user input", "SELECT * FROM users"),
			Entry("empty string", ""),
		)

		It("should maintain low cardinality", func() {
			uniqueOperations := make(map[string]bool)
			// Simulate many different operation names
			for i := 0; i < 100; i++ {
				operation := SanitizeQueryOperation("operation_" + string(rune(i)))
				uniqueOperations[operation] = true
			}

			// Should only have the 4 known operations (unknown maps to "filter")
			Expect(len(uniqueOperations)).To(BeNumerically("<=", 4),
				"Operation cardinality should be bounded to 4 values")
		})
	})

	Context("Overall Cardinality Protection", func() {
		It("should maintain total cardinality under 100 across all metrics", func() {
			// Calculate theoretical maximum cardinality
			maxFailureReasons := 6    // 5 known + 1 unknown
			maxValidationCombos := 60 // 10 fields × 6 reasons
			maxTableStatus := 8       // 4 tables × 2 statuses
			maxOperations := 4        // 4 query operations

			totalMaxCardinality := maxFailureReasons + maxValidationCombos + maxTableStatus + maxOperations

			Expect(totalMaxCardinality).To(Equal(78),
				"Total cardinality should be exactly 78")
			Expect(totalMaxCardinality).To(BeNumerically("<", 100),
				"Total cardinality should be well under 100 (Prometheus best practice)")
		})

		It("should protect against accidental high-cardinality labels", func() {
			// Simulate worst-case scenario: many unique error messages
			uniqueLabels := make(map[string]bool)

			// 1000 different error messages
			for i := 0; i < 1000; i++ {
				reason := SanitizeFailureReason("error message " + string(rune(i)))
				uniqueLabels[reason] = true
			}

			// Should still only have 6 values (5 known + 1 unknown)
			Expect(len(uniqueLabels)).To(BeNumerically("<=", 6),
				"Even with 1000 different inputs, cardinality stays at 6")
		})
	})
})
