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

package datastorage

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// ========================================
// ADR-033 REPOSITORY INTEGRATION TESTS
// 📋 Authority: IMPLEMENTATION_PLAN_V5.3.md Day 15
// 📋 Testing Strategy: Behavior + Correctness with REAL PostgreSQL
// ========================================
//
// This file tests ADR-033 multi-dimensional success tracking repository methods
// against a REAL PostgreSQL database with ADR-033 schema (migration 012).
//
// INTEGRATION TEST STRATEGY:
// - Use REAL PostgreSQL (not mocks)
// - Insert test data directly into resource_action_traces table
// - Execute repository aggregation methods
// - Verify exact counts and calculations
// - Clean up test data after each test
//
// Business Requirements:
// - BR-STORAGE-031-01: Incident-type success rate aggregation
// - BR-STORAGE-031-02: Workflow success rate aggregation
// - BR-STORAGE-031-04: AI execution mode tracking
// - BR-STORAGE-031-05: Multi-dimensional success rate aggregation
//
// ========================================

var _ = Describe("ADR-033 Repository Integration Tests - Multi-Dimensional Success Tracking", func() {
	var (
		actionTraceRepo *repository.ActionTraceRepository
		testCtx         context.Context
		testID          string
	)

	BeforeEach(func() {
		testCtx = context.Background()
		testID = generateTestID()                                            // Unique ID for parallel execution isolation
		actionTraceRepo = repository.NewActionTraceRepository(db.DB, logger) // Use db.DB to get *sql.DB from sqlx

		// DS-FLAKY-002 FIX: Scope cleanup to this test's testID only
		// Clean up test data (cascade delete will handle resource_action_traces)
		resourcePattern := fmt.Sprintf("test-pod-%s-%%", testID)
		_, err := db.ExecContext(testCtx, "DELETE FROM action_histories WHERE resource_id IN (SELECT id FROM resource_references WHERE name LIKE $1)", resourcePattern)
		Expect(err).ToNot(HaveOccurred())
		_, err = db.ExecContext(testCtx, "DELETE FROM resource_references WHERE name LIKE $1", resourcePattern)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		// DS-FLAKY-002 FIX: Scope cleanup to this test's testID only
		// Clean up after each test (cascade delete will handle resource_action_traces)
		resourcePattern := fmt.Sprintf("test-pod-%s-%%", testID)
		_, err := db.ExecContext(testCtx, "DELETE FROM action_histories WHERE resource_id IN (SELECT id FROM resource_references WHERE name LIKE $1)", resourcePattern)
		Expect(err).ToNot(HaveOccurred())
		_, err = db.ExecContext(testCtx, "DELETE FROM resource_references WHERE name LIKE $1", resourcePattern)
		Expect(err).ToNot(HaveOccurred())
	})

	// Helper function to insert test action trace
	// Ensure parent records exist for foreign key constraints
	// Note: Each test creates its own parent records to avoid conflicts
	// DS-FLAKY-002 FIX: Include testID in resource name for proper cleanup scoping
	ensureParentRecords := func() int64 {
		// Create resource_reference with unique UUID scoped to testID
		var resourceID int64
		err := db.QueryRowContext(testCtx, `
			INSERT INTO resource_references (
				resource_uid, api_version, kind, name, namespace
			) VALUES (
				gen_random_uuid()::text, 'v1', 'Pod', 'test-pod-' || $1 || '-' || gen_random_uuid()::text, 'default'
			)
			RETURNING id
		`, testID).Scan(&resourceID)
		Expect(err).ToNot(HaveOccurred())

		// Create action_history
		var historyID int64
		err = db.QueryRowContext(testCtx, `
			INSERT INTO action_histories (
				resource_id, total_actions, last_action_at
			) VALUES (
				$1, 0, NOW()
			)
			RETURNING id
		`, resourceID).Scan(&historyID)
		Expect(err).ToNot(HaveOccurred())

		return historyID
	}

	insertActionTrace := func(
		incidentType string,
		status string,
		workflowID string,
		workflowVersion string,
		aiSelectedWorkflow bool,
		aiChainedWorkflows bool,
	) {
		historyID := ensureParentRecords()

		query := `
			INSERT INTO resource_action_traces (
				action_history_id, action_id, action_type, action_timestamp, execution_status,
				signal_name, signal_severity,
				model_used, model_confidence,
				incident_type, workflow_id, workflow_version,
				ai_selected_workflow, ai_chained_workflows
			) VALUES (
				$1, gen_random_uuid()::text, 'increase_memory', NOW(), $2,
				'test-signal', 'critical',
				'gpt-4', 0.95,
				$3, $4, $5,
				$6, $7
			)
		`
		_, err := db.ExecContext(testCtx, query,
			historyID, status, incidentType, workflowID, workflowVersion,
			aiSelectedWorkflow, aiChainedWorkflows,
		)
		Expect(err).ToNot(HaveOccurred())
	}

	Describe("GetSuccessRateByIncidentType - Integration", func() {
		Context("when incident type has sufficient data", func() {
			It("should calculate success rate correctly with exact counts (TC-ADR033-01)", func() {
				incidentType := fmt.Sprintf("test-pod-oom-killer-%s", testID)

				// BEHAVIOR: Repository calculates incident-type success rate from real database

				// Setup: 8 successes, 2 failures = 80% success rate
				for i := 0; i < 8; i++ {
					insertActionTrace(incidentType, "completed", "pod-oom-recovery", "v1.0", true, false)
				}
				for i := 0; i < 2; i++ {
					insertActionTrace(incidentType, "failed", "pod-oom-recovery", "v1.0", true, false)
				}

				// Execute
				duration := 7 * 24 * time.Hour
				minSamples := 5
				result, err := actionTraceRepo.GetSuccessRateByIncidentType(testCtx, incidentType, duration, minSamples)

				// BEHAVIOR: Repository returns success rate response
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.IncidentType).To(Equal(incidentType))
				Expect(result.TimeRange).To(Equal("7d"))

				// CORRECTNESS: Exact count validation (8 successes + 2 failures = 10 total)
				Expect(result.TotalExecutions).To(Equal(10))
				Expect(result.SuccessfulExecutions).To(Equal(8))
				Expect(result.FailedExecutions).To(Equal(2))

				// CORRECTNESS: Mathematical accuracy (8/10 = 80%)
				Expect(result.SuccessRate).To(BeNumerically("~", 80.0, 0.01))

				// CORRECTNESS: Confidence level calculation
				Expect(result.MinSamplesMet).To(BeTrue())  // 10 >= 5
				Expect(result.Confidence).To(Equal("low")) // 10 < 20 = low

				// CORRECTNESS: Workflow breakdown data
				Expect(result.BreakdownByWorkflow).To(HaveLen(1))
				Expect(result.BreakdownByWorkflow[0].WorkflowID).To(Equal("pod-oom-recovery"))
				Expect(result.BreakdownByWorkflow[0].WorkflowVersion).To(Equal("v1.0"))
				Expect(result.BreakdownByWorkflow[0].Executions).To(Equal(10))
				Expect(result.BreakdownByWorkflow[0].SuccessRate).To(BeNumerically("~", 0.8, 0.01))

				// CORRECTNESS: AI execution mode stats
				Expect(result.AIExecutionMode).ToNot(BeNil())
				Expect(result.AIExecutionMode.CatalogSelected).To(Equal(10)) // All selected from catalog
				Expect(result.AIExecutionMode.Chained).To(Equal(0))          // None chained
			})

			It("should handle multiple workflows for same incident type (TC-ADR033-02)", func() {
				incidentType := fmt.Sprintf("test-node-pressure-%s", testID)

				// Setup: 2 different workflows for same incident
				// Workflow 1: 60% success (6/10)
				for i := 0; i < 6; i++ {
					insertActionTrace(incidentType, "completed", "node-pressure-evict", "v1.0", true, false)
				}
				for i := 0; i < 4; i++ {
					insertActionTrace(incidentType, "failed", "node-pressure-evict", "v1.0", true, false)
				}

				// Workflow 2: 90% success (9/10)
				for i := 0; i < 9; i++ {
					insertActionTrace(incidentType, "completed", "node-pressure-scale", "v2.0", true, false)
				}
				for i := 0; i < 1; i++ {
					insertActionTrace(incidentType, "failed", "node-pressure-scale", "v2.0", true, false)
				}

				// Execute
				result, err := actionTraceRepo.GetSuccessRateByIncidentType(testCtx, incidentType, 7*24*time.Hour, 5)

				// BEHAVIOR: Aggregates across all workflows for incident type
				Expect(err).ToNot(HaveOccurred())
				Expect(result.IncidentType).To(Equal(incidentType))

				// CORRECTNESS: Total counts across both workflows (15 successes, 5 failures)
				Expect(result.TotalExecutions).To(Equal(20))
				Expect(result.SuccessfulExecutions).To(Equal(15))
				Expect(result.FailedExecutions).To(Equal(5))
				Expect(result.SuccessRate).To(BeNumerically("~", 75.0, 0.01)) // 15/20 = 75%

				// CORRECTNESS: Confidence level (20 executions = medium)
				Expect(result.Confidence).To(Equal("medium"))

				// CORRECTNESS: Workflow breakdown shows both workflows
				Expect(result.BreakdownByWorkflow).To(HaveLen(2))

				// Verify workflow 1 breakdown
				workflow1 := findWorkflowBreakdown(result.BreakdownByWorkflow, "node-pressure-evict", "v1.0")
				Expect(workflow1).ToNot(BeNil())
				Expect(workflow1.Executions).To(Equal(10))
				Expect(workflow1.SuccessRate).To(BeNumerically("~", 0.6, 0.01)) // 6/10

				// Verify workflow 2 breakdown
				workflow2 := findWorkflowBreakdown(result.BreakdownByWorkflow, "node-pressure-scale", "v2.0")
				Expect(workflow2).ToNot(BeNil())
				Expect(workflow2.Executions).To(Equal(10))
				Expect(workflow2.SuccessRate).To(BeNumerically("~", 0.9, 0.01)) // 9/10
			})
		})

		Context("when incident type has insufficient data", func() {
			It("should return zero values with insufficient_data confidence (TC-ADR033-03)", func() {
				incidentType := fmt.Sprintf("test-nonexistent-incident-%s", testID)

				// Execute without any test data
				result, err := actionTraceRepo.GetSuccessRateByIncidentType(testCtx, incidentType, 7*24*time.Hour, 5)

				// BEHAVIOR: Returns response even with no data
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.IncidentType).To(Equal(incidentType))

				// CORRECTNESS: Zero values
				Expect(result.TotalExecutions).To(Equal(0))
				Expect(result.SuccessfulExecutions).To(Equal(0))
				Expect(result.FailedExecutions).To(Equal(0))
				Expect(result.SuccessRate).To(BeNumerically("==", 0.0))

				// CORRECTNESS: Insufficient data indicators
				Expect(result.Confidence).To(Equal("insufficient_data"))
				Expect(result.MinSamplesMet).To(BeFalse())
				Expect(result.BreakdownByWorkflow).To(BeEmpty())
			})
		})

		Context("when testing AI execution mode tracking", func() {
			It("should track AI execution mode distribution correctly (TC-ADR033-04)", func() {
				incidentType := fmt.Sprintf("test-ai-execution-tracking-%s", testID)

				// Setup: 10 catalog-selected, 5 chained, 2 manual escalation
				for i := 0; i < 10; i++ {
					insertActionTrace(incidentType, "completed", fmt.Sprintf("workflow-1-%s", testID), "v1.0", true, false)
				}
				for i := 0; i < 5; i++ {
					insertActionTrace(incidentType, "completed", fmt.Sprintf("workflow-2-%s", testID), "v1.0", true, true)
				}
				// Manual escalation: no AI selection or chaining
				for i := 0; i < 2; i++ {
					insertActionTrace(incidentType, "completed", "", "", false, false)
				}

				// Execute
				result, err := actionTraceRepo.GetSuccessRateByIncidentType(testCtx, incidentType, 7*24*time.Hour, 5)

				// BEHAVIOR: Returns AI execution mode stats
				Expect(err).ToNot(HaveOccurred())
				Expect(result.AIExecutionMode).ToNot(BeNil())

				// CORRECTNESS: AI execution mode counts
				Expect(result.AIExecutionMode.CatalogSelected).To(Equal(15)) // 10 + 5 (both catalog and chained)
				Expect(result.AIExecutionMode.Chained).To(Equal(5))
				Expect(result.AIExecutionMode.ManualEscalation).To(Equal(0)) // Not tracked via ai_manual_escalation field
			})
		})
	})

	Describe("GetSuccessRateByWorkflow - Integration", func() {
		Context("when workflow has sufficient data", func() {
			It("should calculate workflow success rate correctly (TC-ADR033-05)", func() {
				// Scope workflowID and incidentType to testID for proper test isolation.
				// Previously hardcoded values ("test-memory-increase", "test-pod-oom") caused
				// cross-contamination from parallel tests or incomplete cleanup of prior runs.
				// NOTE: Prefixes kept short so scoped ID fits in workflow_id VARCHAR(64)
				workflowID := fmt.Sprintf("wf05-%s", testID)
				workflowVersion := "v1.0"
				incidentType := fmt.Sprintf("it05-%s", testID)

				// Setup: 7 successes, 3 failures = 70% success rate
				for i := 0; i < 7; i++ {
					insertActionTrace(incidentType, "completed", workflowID, workflowVersion, true, false)
				}
				for i := 0; i < 3; i++ {
					insertActionTrace(incidentType, "failed", workflowID, workflowVersion, true, false)
				}

				// Execute
				result, err := actionTraceRepo.GetSuccessRateByWorkflow(testCtx, workflowID, workflowVersion, 7*24*time.Hour, 5)

				// BEHAVIOR: Repository returns workflow success rate response
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.WorkflowID).To(Equal(workflowID))
				Expect(result.WorkflowVersion).To(Equal(workflowVersion))

				// CORRECTNESS: Exact count validation
				Expect(result.TotalExecutions).To(Equal(10))
				Expect(result.SuccessfulExecutions).To(Equal(7))
				Expect(result.FailedExecutions).To(Equal(3))

				// CORRECTNESS: Mathematical accuracy (7/10 = 70%)
				Expect(result.SuccessRate).To(BeNumerically("~", 70.0, 0.01))

				// CORRECTNESS: Confidence level
				Expect(result.MinSamplesMet).To(BeTrue())
				Expect(result.Confidence).To(Equal("low")) // 10 < 20 = low
			})

			It("should track workflow usage across multiple incident types (TC-ADR033-06)", func() {
				// Scope workflowID to testID for proper test isolation.
				// Previously hardcoded "test-universal-recovery" caused cross-contamination.
				workflowID := fmt.Sprintf("test-universal-recovery-%s", testID)
				workflowVersion := "v1.0"

				// Setup: Same workflow used for 3 different incident types
				// Incident 1: 5 executions
				for i := 0; i < 5; i++ {
					insertActionTrace(fmt.Sprintf("test-incident-a-%s", testID), "completed", workflowID, workflowVersion, true, false)
				}
				// Incident 2: 3 executions
				for i := 0; i < 3; i++ {
					insertActionTrace(fmt.Sprintf("test-incident-b-%s", testID), "completed", workflowID, workflowVersion, true, false)
				}
				// Incident 3: 2 executions
				for i := 0; i < 2; i++ {
					insertActionTrace(fmt.Sprintf("test-incident-c-%s", testID), "completed", workflowID, workflowVersion, true, false)
				}

				// Execute
				result, err := actionTraceRepo.GetSuccessRateByWorkflow(testCtx, workflowID, workflowVersion, 7*24*time.Hour, 5)

				// BEHAVIOR: Aggregates across all incident types
				Expect(err).ToNot(HaveOccurred())
				Expect(result.TotalExecutions).To(Equal(10))
				Expect(result.SuccessfulExecutions).To(Equal(10)) // All completed

				// CORRECTNESS: Incident type breakdown
				Expect(result.BreakdownByIncidentType).To(HaveLen(3))

				// Verify incident type breakdowns
				incident1 := findIncidentBreakdown(result.BreakdownByIncidentType, fmt.Sprintf("test-incident-a-%s", testID))
				Expect(incident1).ToNot(BeNil())
				Expect(incident1.Executions).To(Equal(5))

				incident2 := findIncidentBreakdown(result.BreakdownByIncidentType, fmt.Sprintf("test-incident-b-%s", testID))
				Expect(incident2).ToNot(BeNil())
				Expect(incident2.Executions).To(Equal(3))

				incident3 := findIncidentBreakdown(result.BreakdownByIncidentType, fmt.Sprintf("test-incident-c-%s", testID))
				Expect(incident3).ToNot(BeNil())
				Expect(incident3.Executions).To(Equal(2))
			})
		})

		Context("when workflow has no data", func() {
			It("should return zero values with insufficient_data confidence (TC-ADR033-07)", func() {
				workflowID := "test-nonexistent-workflow"
				workflowVersion := "v999.0"

				// Execute without any test data
				result, err := actionTraceRepo.GetSuccessRateByWorkflow(testCtx, workflowID, workflowVersion, 7*24*time.Hour, 5)

				// BEHAVIOR: Returns response even with no data
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())

				// CORRECTNESS: Zero values
				Expect(result.TotalExecutions).To(Equal(0))
				Expect(result.SuccessRate).To(BeNumerically("==", 0.0))
				Expect(result.Confidence).To(Equal("insufficient_data"))
				Expect(result.MinSamplesMet).To(BeFalse())
				Expect(result.BreakdownByIncidentType).To(BeEmpty())
			})
		})
	})

	Describe("Confidence Level Thresholds - Integration", func() {
		It("should assign correct confidence levels based on sample size (TC-ADR033-08)", func() {
			// Test confidence levels: low (5-19), medium (20-99), high (>=100)

			// Low: 10 samples
			incidentLow := "test-confidence-low"
			for i := 0; i < 10; i++ {
				insertActionTrace(incidentLow, "completed", "test", "v1.0", true, false)
			}
			resultLow, err := actionTraceRepo.GetSuccessRateByIncidentType(testCtx, incidentLow, 7*24*time.Hour, 5)
			Expect(err).ToNot(HaveOccurred())
			Expect(resultLow.Confidence).To(Equal("low"))
			Expect(resultLow.MinSamplesMet).To(BeTrue())

			// Medium: 50 samples
			incidentMedium := "test-confidence-medium"
			for i := 0; i < 50; i++ {
				insertActionTrace(incidentMedium, "completed", "test", "v1.0", true, false)
			}
			resultMedium, err := actionTraceRepo.GetSuccessRateByIncidentType(testCtx, incidentMedium, 7*24*time.Hour, 5)
			Expect(err).ToNot(HaveOccurred())
			Expect(resultMedium.Confidence).To(Equal("medium"))

			// High: 100 samples (boundary test)
			incidentHigh := "test-confidence-high"
			for i := 0; i < 100; i++ {
				insertActionTrace(incidentHigh, "completed", "test", "v1.0", true, false)
			}
			resultHigh, err := actionTraceRepo.GetSuccessRateByIncidentType(testCtx, incidentHigh, 7*24*time.Hour, 5)
			Expect(err).ToNot(HaveOccurred())
			Expect(resultHigh.Confidence).To(Equal("high"))
		})
	})
})

// Helper function to find workflow breakdown by ID and version
func findWorkflowBreakdown(breakdown []models.WorkflowBreakdownItem, id, version string) *models.WorkflowBreakdownItem {
	for i := range breakdown {
		if breakdown[i].WorkflowID == id && breakdown[i].WorkflowVersion == version {
			return &breakdown[i]
		}
	}
	return nil
}

// Helper function to find incident type breakdown
func findIncidentBreakdown(breakdown []models.IncidentTypeBreakdownItem, incidentType string) *models.IncidentTypeBreakdownItem {
	for i := range breakdown {
		if breakdown[i].IncidentType == incidentType {
			return &breakdown[i]
		}
	}
	return nil
}
