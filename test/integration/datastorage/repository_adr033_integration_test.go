package datastorage

import (
	"context"
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
// - BR-STORAGE-031-02: Playbook success rate aggregation
// - BR-STORAGE-031-04: AI execution mode tracking
// - BR-STORAGE-031-05: Multi-dimensional success rate aggregation
//
// ========================================

var _ = Describe("ADR-033 Repository Integration Tests - Multi-Dimensional Success Tracking", func() {
	var (
		actionTraceRepo *repository.ActionTraceRepository
		testCtx         context.Context
	)

	BeforeEach(func() {
		testCtx = context.Background()
		actionTraceRepo = repository.NewActionTraceRepository(db, logger)

		// Clean up test data from resource_action_traces
		_, err := db.ExecContext(testCtx, "DELETE FROM resource_action_traces WHERE incident_type LIKE 'test-%'")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		// Clean up after each test
		_, err := db.ExecContext(testCtx, "DELETE FROM resource_action_traces WHERE incident_type LIKE 'test-%'")
		Expect(err).ToNot(HaveOccurred())
	})

	// Helper function to insert test action trace
	insertActionTrace := func(
		incidentType string,
		status string,
		playbookID string,
		playbookVersion string,
		aiSelectedPlaybook bool,
		aiChainedPlaybooks bool,
	) {
		query := `
			INSERT INTO resource_action_traces (
				action_id, action_type, action_timestamp, execution_status,
				resource_type, resource_name, resource_namespace,
				model_used, model_confidence,
				incident_type, playbook_id, playbook_version,
				ai_selected_playbook, ai_chained_playbooks
			) VALUES (
				gen_random_uuid()::text, 'increase_memory', NOW(), $1,
				'pod', 'test-pod', 'default',
				'gpt-4', 0.95,
				$2, $3, $4,
				$5, $6
			)
		`
		_, err := db.ExecContext(testCtx, query,
			status, incidentType, playbookID, playbookVersion,
			aiSelectedPlaybook, aiChainedPlaybooks,
		)
		Expect(err).ToNot(HaveOccurred())
	}

	Describe("GetSuccessRateByIncidentType - Integration", func() {
		Context("when incident type has sufficient data", func() {
			It("should calculate success rate correctly with exact counts (TC-ADR033-01)", func() {
				incidentType := "test-pod-oom-killer"

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

				// CORRECTNESS: Playbook breakdown data
				Expect(result.BreakdownByPlaybook).To(HaveLen(1))
				Expect(result.BreakdownByPlaybook[0].PlaybookID).To(Equal("pod-oom-recovery"))
				Expect(result.BreakdownByPlaybook[0].PlaybookVersion).To(Equal("v1.0"))
				Expect(result.BreakdownByPlaybook[0].Executions).To(Equal(10))
				Expect(result.BreakdownByPlaybook[0].SuccessRate).To(BeNumerically("~", 0.8, 0.01))

				// CORRECTNESS: AI execution mode stats
				Expect(result.AIExecutionMode).ToNot(BeNil())
				Expect(result.AIExecutionMode.CatalogSelected).To(Equal(10)) // All selected from catalog
				Expect(result.AIExecutionMode.Chained).To(Equal(0))          // None chained
			})

			It("should handle multiple playbooks for same incident type (TC-ADR033-02)", func() {
				incidentType := "test-node-pressure"

				// Setup: 2 different playbooks for same incident
				// Playbook 1: 60% success (6/10)
				for i := 0; i < 6; i++ {
					insertActionTrace(incidentType, "completed", "node-pressure-evict", "v1.0", true, false)
				}
				for i := 0; i < 4; i++ {
					insertActionTrace(incidentType, "failed", "node-pressure-evict", "v1.0", true, false)
				}

				// Playbook 2: 90% success (9/10)
				for i := 0; i < 9; i++ {
					insertActionTrace(incidentType, "completed", "node-pressure-scale", "v2.0", true, false)
				}
				for i := 0; i < 1; i++ {
					insertActionTrace(incidentType, "failed", "node-pressure-scale", "v2.0", true, false)
				}

				// Execute
				result, err := actionTraceRepo.GetSuccessRateByIncidentType(testCtx, incidentType, 7*24*time.Hour, 5)

				// BEHAVIOR: Aggregates across all playbooks for incident type
				Expect(err).ToNot(HaveOccurred())
				Expect(result.IncidentType).To(Equal(incidentType))

				// CORRECTNESS: Total counts across both playbooks (15 successes, 5 failures)
				Expect(result.TotalExecutions).To(Equal(20))
				Expect(result.SuccessfulExecutions).To(Equal(15))
				Expect(result.FailedExecutions).To(Equal(5))
				Expect(result.SuccessRate).To(BeNumerically("~", 75.0, 0.01)) // 15/20 = 75%

				// CORRECTNESS: Confidence level (20 executions = medium)
				Expect(result.Confidence).To(Equal("medium"))

				// CORRECTNESS: Playbook breakdown shows both playbooks
				Expect(result.BreakdownByPlaybook).To(HaveLen(2))

				// Verify playbook 1 breakdown
				playbook1 := findPlaybookBreakdown(result.BreakdownByPlaybook, "node-pressure-evict", "v1.0")
				Expect(playbook1).ToNot(BeNil())
				Expect(playbook1.Executions).To(Equal(10))
				Expect(playbook1.SuccessRate).To(BeNumerically("~", 0.6, 0.01)) // 6/10

				// Verify playbook 2 breakdown
				playbook2 := findPlaybookBreakdown(result.BreakdownByPlaybook, "node-pressure-scale", "v2.0")
				Expect(playbook2).ToNot(BeNil())
				Expect(playbook2.Executions).To(Equal(10))
				Expect(playbook2.SuccessRate).To(BeNumerically("~", 0.9, 0.01)) // 9/10
			})
		})

		Context("when incident type has insufficient data", func() {
			It("should return zero values with insufficient_data confidence (TC-ADR033-03)", func() {
				incidentType := "test-nonexistent-incident"

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
				Expect(result.BreakdownByPlaybook).To(BeEmpty())
			})
		})

		Context("when testing AI execution mode tracking", func() {
			It("should track AI execution mode distribution correctly (TC-ADR033-04)", func() {
				incidentType := "test-ai-execution-tracking"

				// Setup: 10 catalog-selected, 5 chained, 2 manual escalation
				for i := 0; i < 10; i++ {
					insertActionTrace(incidentType, "completed", "playbook-1", "v1.0", true, false)
				}
				for i := 0; i < 5; i++ {
					insertActionTrace(incidentType, "completed", "playbook-2", "v1.0", true, true)
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

	Describe("GetSuccessRateByPlaybook - Integration", func() {
		Context("when playbook has sufficient data", func() {
			It("should calculate playbook success rate correctly (TC-ADR033-05)", func() {
				playbookID := "test-memory-increase"
				playbookVersion := "v1.0"

				// Setup: 7 successes, 3 failures = 70% success rate
				for i := 0; i < 7; i++ {
					insertActionTrace("test-pod-oom", "completed", playbookID, playbookVersion, true, false)
				}
				for i := 0; i < 3; i++ {
					insertActionTrace("test-pod-oom", "failed", playbookID, playbookVersion, true, false)
				}

				// Execute
				result, err := actionTraceRepo.GetSuccessRateByPlaybook(testCtx, playbookID, playbookVersion, 7*24*time.Hour, 5)

				// BEHAVIOR: Repository returns playbook success rate response
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.PlaybookID).To(Equal(playbookID))
				Expect(result.PlaybookVersion).To(Equal(playbookVersion))

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

			It("should track playbook usage across multiple incident types (TC-ADR033-06)", func() {
				playbookID := "test-universal-recovery"
				playbookVersion := "v1.0"

				// Setup: Same playbook used for 3 different incident types
				// Incident 1: 5 executions
				for i := 0; i < 5; i++ {
					insertActionTrace("test-incident-a", "completed", playbookID, playbookVersion, true, false)
				}
				// Incident 2: 3 executions
				for i := 0; i < 3; i++ {
					insertActionTrace("test-incident-b", "completed", playbookID, playbookVersion, true, false)
				}
				// Incident 3: 2 executions
				for i := 0; i < 2; i++ {
					insertActionTrace("test-incident-c", "completed", playbookID, playbookVersion, true, false)
				}

				// Execute
				result, err := actionTraceRepo.GetSuccessRateByPlaybook(testCtx, playbookID, playbookVersion, 7*24*time.Hour, 5)

				// BEHAVIOR: Aggregates across all incident types
				Expect(err).ToNot(HaveOccurred())
				Expect(result.TotalExecutions).To(Equal(10))
				Expect(result.SuccessfulExecutions).To(Equal(10)) // All completed

				// CORRECTNESS: Incident type breakdown
				Expect(result.BreakdownByIncidentType).To(HaveLen(3))

				// Verify incident type breakdowns
				incident1 := findIncidentBreakdown(result.BreakdownByIncidentType, "test-incident-a")
				Expect(incident1).ToNot(BeNil())
				Expect(incident1.Executions).To(Equal(5))

				incident2 := findIncidentBreakdown(result.BreakdownByIncidentType, "test-incident-b")
				Expect(incident2).ToNot(BeNil())
				Expect(incident2.Executions).To(Equal(3))

				incident3 := findIncidentBreakdown(result.BreakdownByIncidentType, "test-incident-c")
				Expect(incident3).ToNot(BeNil())
				Expect(incident3.Executions).To(Equal(2))
			})
		})

		Context("when playbook has no data", func() {
			It("should return zero values with insufficient_data confidence (TC-ADR033-07)", func() {
				playbookID := "test-nonexistent-playbook"
				playbookVersion := "v999.0"

				// Execute without any test data
				result, err := actionTraceRepo.GetSuccessRateByPlaybook(testCtx, playbookID, playbookVersion, 7*24*time.Hour, 5)

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

// Helper function to find playbook breakdown by ID and version
func findPlaybookBreakdown(breakdown []models.PlaybookBreakdownItem, id, version string) *models.PlaybookBreakdownItem {
	for i := range breakdown {
		if breakdown[i].PlaybookID == id && breakdown[i].PlaybookVersion == version {
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
