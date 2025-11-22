package datastorage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// ========================================
// ADR-033 REPOSITORY UNIT TESTS (TDD RED Phase)
// 📋 Authority: IMPLEMENTATION_PLAN_V5.3.md Day 13.1
// 📋 Testing Strategy: Behavior + Correctness
// ========================================
//
// This file tests ADR-033 multi-dimensional success tracking repository methods.
//
// TDD WORKFLOW:
// 1. RED: Write failing tests (this file)
// 2. GREEN: Implement repository methods to pass tests
// 3. REFACTOR: Optimize implementation
//
// MOCK STRATEGY (Unit Tests):
// - Mock: *sql.DB, *sql.Rows (external dependencies)
// - Real: Business logic, response builders, validators
//
// Business Requirements:
// - BR-STORAGE-031-01: Incident-type success rate aggregation
// - BR-STORAGE-031-02: Playbook success rate aggregation
// - BR-STORAGE-031-04: AI execution mode tracking
// - BR-STORAGE-031-05: Multi-dimensional success rate aggregation
//
// ========================================

var _ = Describe("ActionTraceRepository - ADR-033 Multi-Dimensional Success Tracking", func() {
	var (
		mockDB  *sql.DB
		sqlMock sqlmock.Sqlmock
		repo    *repository.ActionTraceRepository
		logger  *zap.Logger
		ctx     context.Context
	)

	BeforeEach(func() {
		var err error
		mockDB, sqlMock, err = sqlmock.New()
		Expect(err).ToNot(HaveOccurred())

		logger = zap.NewNop()
		ctx = context.Background()

		// Create repository with mocked database
		repo = repository.NewActionTraceRepository(mockDB, logger)
	})

	AfterEach(func() {
		mockDB.Close()
	})

	// ========================================
	// BR-STORAGE-031-01: Incident-Type Success Rate
	// ========================================

	Describe("GetSuccessRateByIncidentType", func() {
		Context("when incident type has sufficient data", func() {
			It("should calculate success rate correctly for pod-oom-killer incident", func() {
				// BEHAVIOR: Method returns success rate response
				// CORRECTNESS: Success rate calculation is mathematically accurate

				incidentType := "pod-oom-killer"
				duration := 7 * 24 * time.Hour
				minSamples := 5

				// Mock query expectations
				rows := sqlmock.NewRows([]string{
					"incident_type", "total_executions", "successful_executions", "failed_executions",
				}).AddRow(
					"pod-oom-killer", 100, 85, 15, // 85% success rate
				)

				sqlMock.ExpectQuery(`SELECT incident_type, COUNT\(\*\) as total_executions`).
					WithArgs(incidentType, sqlmock.AnyArg()).
					WillReturnRows(rows)

				// Mock playbook breakdown query
				playbookRows := sqlmock.NewRows([]string{
					"playbook_id", "playbook_version", "executions", "success_rate",
				}).AddRow(
					"pod-oom-recovery", "v1.0", 60, 0.90,
				).AddRow(
					"memory-increase", "v2.0", 40, 0.78,
				)

				sqlMock.ExpectQuery(`SELECT playbook_id, playbook_version`).
					WithArgs(incidentType, sqlmock.AnyArg()).
					WillReturnRows(playbookRows)

				// Mock AI execution mode query
				aiRows := sqlmock.NewRows([]string{
					"catalog_selected", "chained", "manual_escalation",
				}).AddRow(
					92, 7, 1, // 92% catalog, 7% chained, 1% manual
				)

				sqlMock.ExpectQuery(`SELECT COUNT\(CASE WHEN ai_selected_playbook`).
					WithArgs(incidentType, sqlmock.AnyArg()).
					WillReturnRows(aiRows)

				// Execute
				result, err := repo.GetSuccessRateByIncidentType(ctx, incidentType, duration, minSamples)

				// BEHAVIOR: No errors, response structure correct
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.IncidentType).To(Equal("pod-oom-killer"))

				// CORRECTNESS: Success rate calculation is exact
				Expect(result.TotalExecutions).To(Equal(100))
				Expect(result.SuccessfulExecutions).To(Equal(85))
				Expect(result.FailedExecutions).To(Equal(15))
				Expect(result.SuccessRate).To(BeNumerically("~", 85.0, 0.01)) // 85/100 = 85%

				// CORRECTNESS: Confidence level matches sample size
				Expect(result.MinSamplesMet).To(BeTrue())
				Expect(result.Confidence).To(Equal("high")) // >100 samples

				// CORRECTNESS: Playbook breakdown data is accurate
				Expect(result.BreakdownByWorkflow).To(HaveLen(2))
				Expect(result.BreakdownByWorkflow[0].PlaybookID).To(Equal("pod-oom-recovery"))
				Expect(result.BreakdownByWorkflow[0].Executions).To(Equal(60))
				Expect(result.BreakdownByWorkflow[0].SuccessRate).To(BeNumerically("~", 0.90, 0.01))

				// CORRECTNESS: AI execution mode stats are accurate
				Expect(result.AIExecutionMode).ToNot(BeNil())
				Expect(result.AIExecutionMode.CatalogSelected).To(Equal(92))
				Expect(result.AIExecutionMode.Chained).To(Equal(7))
				Expect(result.AIExecutionMode.ManualEscalation).To(Equal(1))

				// Verify all expectations met
				Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
			})

			It("should handle medium confidence level for 50 samples", func() {
				// BEHAVIOR: Confidence level adjusts based on sample size
				// CORRECTNESS: Threshold boundaries are exact

				incidentType := "high-cpu-usage"
				duration := 7 * 24 * time.Hour
				minSamples := 5

				rows := sqlmock.NewRows([]string{
					"incident_type", "total_executions", "successful_executions", "failed_executions",
				}).AddRow(
					"high-cpu-usage", 50, 40, 10, // 80% success rate, medium confidence
				)

				sqlMock.ExpectQuery(`SELECT incident_type, COUNT\(\*\) as total_executions`).
					WithArgs(incidentType, sqlmock.AnyArg()).
					WillReturnRows(rows)

				// Mock empty breakdown queries
				sqlMock.ExpectQuery(`SELECT playbook_id, playbook_version`).
					WillReturnRows(sqlmock.NewRows([]string{"playbook_id", "playbook_version", "executions", "success_rate"}))

				sqlMock.ExpectQuery(`SELECT COUNT\(CASE WHEN ai_selected_playbook`).
					WillReturnRows(sqlmock.NewRows([]string{"catalog_selected", "chained", "manual_escalation"}).AddRow(0, 0, 0))

				result, err := repo.GetSuccessRateByIncidentType(ctx, incidentType, duration, minSamples)

				Expect(err).ToNot(HaveOccurred())

				// CORRECTNESS: Confidence level thresholds
				// high: >100, medium: 20-100, low: 5-20, insufficient_data: <5
				Expect(result.TotalExecutions).To(Equal(50))
				Expect(result.Confidence).To(Equal("medium")) // 20-100 samples
				Expect(result.MinSamplesMet).To(BeTrue())     // 50 > 5

				Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
			})

			It("should handle low confidence level for 10 samples", func() {
				incidentType := "disk-pressure"
				duration := 7 * 24 * time.Hour
				minSamples := 5

				rows := sqlmock.NewRows([]string{
					"incident_type", "total_executions", "successful_executions", "failed_executions",
				}).AddRow(
					"disk-pressure", 10, 8, 2, // 80% success rate, low confidence
				)

				sqlMock.ExpectQuery(`SELECT incident_type, COUNT\(\*\) as total_executions`).
					WithArgs(incidentType, sqlmock.AnyArg()).
					WillReturnRows(rows)

				sqlMock.ExpectQuery(`SELECT playbook_id, playbook_version`).
					WillReturnRows(sqlmock.NewRows([]string{"playbook_id", "playbook_version", "executions", "success_rate"}))

				sqlMock.ExpectQuery(`SELECT COUNT\(CASE WHEN ai_selected_playbook`).
					WillReturnRows(sqlmock.NewRows([]string{"catalog_selected", "chained", "manual_escalation"}).AddRow(0, 0, 0))

				result, err := repo.GetSuccessRateByIncidentType(ctx, incidentType, duration, minSamples)

				Expect(err).ToNot(HaveOccurred())

				// CORRECTNESS: Low confidence for 5-20 samples
				Expect(result.TotalExecutions).To(Equal(10))
				Expect(result.Confidence).To(Equal("low")) // 5-20 samples
				Expect(result.MinSamplesMet).To(BeTrue())  // 10 >= 5

				Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
			})
		})

		Context("when incident type has insufficient data", func() {
			It("should return insufficient_data for samples below threshold", func() {
				// BEHAVIOR: Returns response even with insufficient data
				// CORRECTNESS: MinSamplesMet flag is accurate

				incidentType := "rare-incident"
				duration := 7 * 24 * time.Hour
				minSamples := 5

				rows := sqlmock.NewRows([]string{
					"incident_type", "total_executions", "successful_executions", "failed_executions",
				}).AddRow(
					"rare-incident", 3, 2, 1, // Only 3 samples, below threshold of 5
				)

				sqlMock.ExpectQuery(`SELECT incident_type, COUNT\(\*\) as total_executions`).
					WithArgs(incidentType, sqlmock.AnyArg()).
					WillReturnRows(rows)

				sqlMock.ExpectQuery(`SELECT playbook_id, playbook_version`).
					WillReturnRows(sqlmock.NewRows([]string{"playbook_id", "playbook_version", "executions", "success_rate"}))

				sqlMock.ExpectQuery(`SELECT COUNT\(CASE WHEN ai_selected_playbook`).
					WillReturnRows(sqlmock.NewRows([]string{"catalog_selected", "chained", "manual_escalation"}).AddRow(0, 0, 0))

				result, err := repo.GetSuccessRateByIncidentType(ctx, incidentType, duration, minSamples)

				Expect(err).ToNot(HaveOccurred())

				// CORRECTNESS: Insufficient data flags
				Expect(result.TotalExecutions).To(Equal(3))
				Expect(result.MinSamplesMet).To(BeFalse())               // 3 < 5
				Expect(result.Confidence).To(Equal("insufficient_data")) // <5 samples

				Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
			})

			It("should return zero success rate for incident with no data", func() {
				// BEHAVIOR: Handles empty result set gracefully
				// CORRECTNESS: Returns sensible defaults (0 executions, 0% success rate)

				incidentType := "nonexistent-incident"
				duration := 7 * 24 * time.Hour
				minSamples := 5

				rows := sqlmock.NewRows([]string{
					"incident_type", "total_executions", "successful_executions", "failed_executions",
				}) // Empty result set

				sqlMock.ExpectQuery(`SELECT incident_type, COUNT\(\*\) as total_executions`).
					WithArgs(incidentType, sqlmock.AnyArg()).
					WillReturnRows(rows)

				result, err := repo.GetSuccessRateByIncidentType(ctx, incidentType, duration, minSamples)

				Expect(err).ToNot(HaveOccurred())

				// CORRECTNESS: Zero values for empty result set
				Expect(result.TotalExecutions).To(Equal(0))
				Expect(result.SuccessfulExecutions).To(Equal(0))
				Expect(result.FailedExecutions).To(Equal(0))
				Expect(result.SuccessRate).To(BeNumerically("~", 0.0, 0.01))
				Expect(result.MinSamplesMet).To(BeFalse())
				Expect(result.Confidence).To(Equal("insufficient_data"))

				// No breakdown queries expected for empty result
				Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
			})
		})

		Context("when database errors occur", func() {
			It("should return error for database connection failure", func() {
				// BEHAVIOR: Propagates database errors properly

				incidentType := "any-incident"
				duration := 7 * 24 * time.Hour
				minSamples := 5

				expectedErr := fmt.Errorf("database connection lost")

				sqlMock.ExpectQuery(`SELECT incident_type, COUNT\(\*\) as total_executions`).
					WithArgs(incidentType, sqlmock.AnyArg()).
					WillReturnError(expectedErr)

				result, err := repo.GetSuccessRateByIncidentType(ctx, incidentType, duration, minSamples)

				// BEHAVIOR: Error is returned
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("database connection lost"))
				Expect(result).To(BeNil())

				Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
			})

			It("should return error for malformed SQL result", func() {
				// BEHAVIOR: Handles row scan errors properly

				incidentType := "any-incident"
				duration := 7 * 24 * time.Hour
				minSamples := 5

				// Invalid row data (wrong type for total_executions)
				rows := sqlmock.NewRows([]string{
					"incident_type", "total_executions", "successful_executions", "failed_executions",
				}).AddRow(
					"incident", "invalid", 50, 10, // "invalid" instead of int
				)

				sqlMock.ExpectQuery(`SELECT incident_type, COUNT\(\*\) as total_executions`).
					WithArgs(incidentType, sqlmock.AnyArg()).
					WillReturnRows(rows)

				result, err := repo.GetSuccessRateByIncidentType(ctx, incidentType, duration, minSamples)

				// BEHAVIOR: Scan error is returned
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())

				Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
			})
		})

		Context("when testing time range boundaries", func() {
			It("should handle 30-day time range correctly", func() {
				// CORRECTNESS: Time range calculation is exact

				incidentType := "test-incident"
				duration := 30 * 24 * time.Hour
				minSamples := 5

				rows := sqlmock.NewRows([]string{
					"incident_type", "total_executions", "successful_executions", "failed_executions",
				}).AddRow(
					"test-incident", 200, 180, 20,
				)

				sqlMock.ExpectQuery(`SELECT incident_type, COUNT\(\*\) as total_executions`).
					WithArgs(incidentType, sqlmock.AnyArg()).
					WillReturnRows(rows)

				sqlMock.ExpectQuery(`SELECT playbook_id, playbook_version`).
					WillReturnRows(sqlmock.NewRows([]string{"playbook_id", "playbook_version", "executions", "success_rate"}))

				sqlMock.ExpectQuery(`SELECT COUNT\(CASE WHEN ai_selected_playbook`).
					WillReturnRows(sqlmock.NewRows([]string{"catalog_selected", "chained", "manual_escalation"}).AddRow(0, 0, 0))

				result, err := repo.GetSuccessRateByIncidentType(ctx, incidentType, duration, minSamples)

				Expect(err).ToNot(HaveOccurred())

				// CORRECTNESS: TimeRange field reflects query parameter
				Expect(result.TimeRange).To(Equal("30d"))

				Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
			})
		})
	})

	// ========================================
	// BR-STORAGE-031-02: Playbook Success Rate
	// ========================================

	Describe("GetSuccessRateByWorkflow", func() {
		Context("when playbook has sufficient execution data", func() {
			It("should calculate success rate correctly for disk-cleanup playbook", func() {
				// BEHAVIOR: Method returns playbook success rate response
				// CORRECTNESS: Success rate and breakdown data are mathematically accurate

				playbookID := "disk-cleanup"
				playbookVersion := "v2.0"
				duration := 7 * 24 * time.Hour
				minSamples := 5

				// Mock main query
				rows := sqlmock.NewRows([]string{
					"playbook_id", "playbook_version", "total_executions", "successful_executions", "failed_executions",
				}).AddRow(
					"disk-cleanup", "v2.0", 120, 105, 15, // 87.5% success rate
				)

				sqlMock.ExpectQuery(`SELECT playbook_id, playbook_version, COUNT\(\*\) as total_executions`).
					WithArgs(playbookID, playbookVersion, sqlmock.AnyArg()).
					WillReturnRows(rows)

				// Mock incident type breakdown
				incidentRows := sqlmock.NewRows([]string{
					"incident_type", "executions", "success_rate",
				}).AddRow(
					"disk-pressure", 80, 0.90,
				).AddRow(
					"high-storage-usage", 40, 0.82,
				)

				sqlMock.ExpectQuery(`SELECT incident_type, COUNT\(\*\) as executions`).
					WithArgs(playbookID, playbookVersion, sqlmock.AnyArg()).
					WillReturnRows(incidentRows)

				// Mock AI execution mode
				aiRows := sqlmock.NewRows([]string{
					"catalog_selected", "chained", "manual_escalation",
				}).AddRow(
					110, 9, 1,
				)

				sqlMock.ExpectQuery(`SELECT COUNT\(CASE WHEN ai_selected_playbook`).
					WithArgs(playbookID, playbookVersion, sqlmock.AnyArg()).
					WillReturnRows(aiRows)

				result, err := repo.GetSuccessRateByWorkflow(ctx, playbookID, playbookVersion, duration, minSamples)

				// BEHAVIOR: No errors, response structure correct
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.PlaybookID).To(Equal("disk-cleanup"))
				Expect(result.PlaybookVersion).To(Equal("v2.0"))

				// CORRECTNESS: Success rate is exact
				Expect(result.TotalExecutions).To(Equal(120))
				Expect(result.SuccessfulExecutions).To(Equal(105))
				Expect(result.FailedExecutions).To(Equal(15))
				Expect(result.SuccessRate).To(BeNumerically("~", 87.5, 0.01)) // 105/120 = 87.5%

				// CORRECTNESS: Incident type breakdown
				Expect(result.BreakdownByIncidentType).To(HaveLen(2))
				Expect(result.BreakdownByIncidentType[0].IncidentType).To(Equal("disk-pressure"))
				Expect(result.BreakdownByIncidentType[0].Executions).To(Equal(80))
				Expect(result.BreakdownByIncidentType[0].SuccessRate).To(BeNumerically("~", 0.90, 0.01))

				// CORRECTNESS: AI execution mode
				Expect(result.AIExecutionMode).ToNot(BeNil())
				Expect(result.AIExecutionMode.CatalogSelected).To(Equal(110))
				Expect(result.AIExecutionMode.Chained).To(Equal(9))
				Expect(result.AIExecutionMode.ManualEscalation).To(Equal(1))

				Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
			})
		})

		Context("when playbook has insufficient data", func() {
			It("should handle playbook with only 2 executions", func() {
				// CORRECTNESS: MinSamplesMet flag is accurate

				playbookID := "experimental-playbook"
				playbookVersion := "v0.1"
				duration := 7 * 24 * time.Hour
				minSamples := 5

				rows := sqlmock.NewRows([]string{
					"playbook_id", "playbook_version", "total_executions", "successful_executions", "failed_executions",
				}).AddRow(
					"experimental-playbook", "v0.1", 2, 2, 0,
				)

				sqlMock.ExpectQuery(`SELECT playbook_id, playbook_version, COUNT\(\*\) as total_executions`).
					WithArgs(playbookID, playbookVersion, sqlmock.AnyArg()).
					WillReturnRows(rows)

				sqlMock.ExpectQuery(`SELECT incident_type, COUNT\(\*\) as executions`).
					WillReturnRows(sqlmock.NewRows([]string{"incident_type", "executions", "success_rate"}))

				sqlMock.ExpectQuery(`SELECT COUNT\(CASE WHEN ai_selected_playbook`).
					WillReturnRows(sqlmock.NewRows([]string{"catalog_selected", "chained", "manual_escalation"}).AddRow(0, 0, 0))

				result, err := repo.GetSuccessRateByWorkflow(ctx, playbookID, playbookVersion, duration, minSamples)

				Expect(err).ToNot(HaveOccurred())

				// CORRECTNESS: Insufficient data handling
				Expect(result.TotalExecutions).To(Equal(2))
				Expect(result.MinSamplesMet).To(BeFalse()) // 2 < 5
				Expect(result.Confidence).To(Equal("insufficient_data"))

				Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
			})
		})
	})

	// ========================================
	// BR-STORAGE-031-05: Multi-Dimensional Success Rate
	// TDD RED Phase: Write failing tests for GetSuccessRateMultiDimensional
	// ========================================
	Describe("GetSuccessRateMultiDimensional", func() {
		Context("with all three dimensions specified", func() {
			It("should query by incident_type + playbook + action_type", func() {
				// ARRANGE: Mock database response
				incidentType := "pod-oom-killer"
				playbookID := "pod-oom-recovery"
				playbookVersion := "v1.2"
				actionType := "increase_memory"
				minSamples := 5

				rows := sqlmock.NewRows([]string{
					"total_executions", "successful_executions", "failed_executions",
				}).AddRow(
					50, 45, 5,
				)

				sqlMock.ExpectQuery(`SELECT COUNT\(\*\) AS total_executions`).
					WithArgs(incidentType, playbookID, playbookVersion, actionType, sqlmock.AnyArg()).
					WillReturnRows(rows)

				// ACT: Call repository method
				result, err := repo.GetSuccessRateMultiDimensional(ctx, &models.MultiDimensionalQuery{
					IncidentType:    incidentType,
					PlaybookID:      playbookID,
					PlaybookVersion: playbookVersion,
					ActionType:      actionType,
					TimeRange:       "7d",
					MinSamples:      minSamples,
				})

				// ASSERT: No error
				Expect(err).ToNot(HaveOccurred())

				// BEHAVIOR: Returns multi-dimensional result
				Expect(result).ToNot(BeNil())
				Expect(result.Dimensions.IncidentType).To(Equal(incidentType))
				Expect(result.Dimensions.PlaybookID).To(Equal(playbookID))
				Expect(result.Dimensions.PlaybookVersion).To(Equal(playbookVersion))
				Expect(result.Dimensions.ActionType).To(Equal(actionType))

				// CORRECTNESS: Exact count validation
				Expect(result.TotalExecutions).To(Equal(50))
				Expect(result.SuccessfulExecutions).To(Equal(45))
				Expect(result.FailedExecutions).To(Equal(5))

				// CORRECTNESS: Mathematical accuracy (45/50 = 0.90 = 90%)
				Expect(result.SuccessRate).To(BeNumerically("~", 90.0, 0.1))

				// BEHAVIOR: Confidence level (50 samples = medium, 20-99 range)
				Expect(result.Confidence).To(Equal("medium"))
				Expect(result.MinSamplesMet).To(BeTrue())

				Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
			})
		})

		Context("with partial dimensions (incident_type + playbook only)", func() {
			It("should query by incident_type + playbook, aggregate across all action_types", func() {
				// ARRANGE: Mock database response (aggregated across actions)
				incidentType := "pod-oom-killer"
				playbookID := "pod-oom-recovery"
				playbookVersion := "v1.2"
				minSamples := 5

				rows := sqlmock.NewRows([]string{
					"total_executions", "successful_executions", "failed_executions",
				}).AddRow(
					100, 85, 15,
				)

				sqlMock.ExpectQuery(`SELECT COUNT\(\*\) AS total_executions`).
					WithArgs(incidentType, playbookID, playbookVersion, sqlmock.AnyArg()).
					WillReturnRows(rows)

				// ACT: Call repository method (no action_type specified)
				result, err := repo.GetSuccessRateMultiDimensional(ctx, &models.MultiDimensionalQuery{
					IncidentType:    incidentType,
					PlaybookID:      playbookID,
					PlaybookVersion: playbookVersion,
					TimeRange:       "7d",
					MinSamples:      minSamples,
				})

				// ASSERT: No error
				Expect(err).ToNot(HaveOccurred())

				// BEHAVIOR: Returns aggregated result across all actions
				Expect(result.TotalExecutions).To(Equal(100))
				Expect(result.SuccessfulExecutions).To(Equal(85))

				// CORRECTNESS: Success rate (85/100 = 0.85 = 85%)
				Expect(result.SuccessRate).To(BeNumerically("~", 85.0, 0.1))

				// BEHAVIOR: Dimensions reflect query
				Expect(result.Dimensions.IncidentType).To(Equal(incidentType))
				Expect(result.Dimensions.PlaybookID).To(Equal(playbookID))
				Expect(result.Dimensions.ActionType).To(BeEmpty())

				Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
			})
		})

		Context("with only incident_type dimension", func() {
			It("should query by incident_type only, aggregate across all playbooks and actions", func() {
				// ARRANGE: Mock database response
				incidentType := "pod-oom-killer"
				minSamples := 5

				rows := sqlmock.NewRows([]string{
					"total_executions", "successful_executions", "failed_executions",
				}).AddRow(
					150, 135, 15,
				)

				sqlMock.ExpectQuery(`SELECT COUNT\(\*\) AS total_executions`).
					WithArgs(incidentType, sqlmock.AnyArg()).
					WillReturnRows(rows)

				// ACT: Call repository method (only incident_type)
				result, err := repo.GetSuccessRateMultiDimensional(ctx, &models.MultiDimensionalQuery{
					IncidentType: incidentType,
					TimeRange:    "7d",
					MinSamples:   minSamples,
				})

				// ASSERT: No error
				Expect(err).ToNot(HaveOccurred())

				// BEHAVIOR: Returns incident-type aggregation
				Expect(result.Dimensions.IncidentType).To(Equal(incidentType))
				Expect(result.Dimensions.PlaybookID).To(BeEmpty())
				Expect(result.Dimensions.ActionType).To(BeEmpty())

				// CORRECTNESS: Aggregated across all playbooks and actions
				Expect(result.TotalExecutions).To(Equal(150))
				Expect(result.SuccessRate).To(BeNumerically("~", 90.0, 0.1))

				// BEHAVIOR: Confidence level (150 samples = high, >=100 range)
				Expect(result.Confidence).To(Equal("high"))

				Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
			})
		})

		Context("edge cases", func() {
			It("should return zero results for non-existent dimension combination", func() {
				// ARRANGE: Mock empty result
				incidentType := "non-existent-incident"
				playbookID := "non-existent-playbook"
				minSamples := 5

				rows := sqlmock.NewRows([]string{
					"total_executions", "successful_executions", "failed_executions",
				}).AddRow(
					0, 0, 0,
				)

				sqlMock.ExpectQuery(`SELECT COUNT\(\*\) AS total_executions`).
					WithArgs(incidentType, playbookID, sqlmock.AnyArg()).
					WillReturnRows(rows)

				// ACT: Call repository method
				result, err := repo.GetSuccessRateMultiDimensional(ctx, &models.MultiDimensionalQuery{
					IncidentType: incidentType,
					PlaybookID:   playbookID,
					TimeRange:    "7d",
					MinSamples:   minSamples,
				})

				// ASSERT: No error
				Expect(err).ToNot(HaveOccurred())

				// BEHAVIOR: Returns zero executions
				Expect(result.TotalExecutions).To(Equal(0))
				Expect(result.SuccessfulExecutions).To(Equal(0))
				Expect(result.FailedExecutions).To(Equal(0))

				// CORRECTNESS: Success rate is 0.0 (no data)
				Expect(result.SuccessRate).To(Equal(0.0))

				// BEHAVIOR: Confidence is insufficient_data
				Expect(result.Confidence).To(Equal("insufficient_data"))
				Expect(result.MinSamplesMet).To(BeFalse())

				Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
			})

			It("should handle playbook_version without playbook_id as validation error", func() {
				// ACT: Call repository method (invalid: version without ID)
				_, err := repo.GetSuccessRateMultiDimensional(ctx, &models.MultiDimensionalQuery{
					IncidentType:    "pod-oom-killer",
					PlaybookVersion: "v1.2", // Version without ID
					TimeRange:       "7d",
					MinSamples:      5,
				})

				// ASSERT: Returns validation error
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("playbook_version requires playbook_id"))
			})

			It("should handle invalid time_range format", func() {
				// ACT: Call repository method with invalid time_range
				_, err := repo.GetSuccessRateMultiDimensional(ctx, &models.MultiDimensionalQuery{
					IncidentType: "pod-oom-killer",
					TimeRange:    "invalid",
					MinSamples:   5,
				})

				// ASSERT: Returns validation error
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid time_range"))
			})

			It("should handle 100% success rate correctly", func() {
				// ARRANGE: Mock 100% success
				incidentType := "simple-restart"
				minSamples := 5

				rows := sqlmock.NewRows([]string{
					"total_executions", "successful_executions", "failed_executions",
				}).AddRow(
					25, 25, 0,
				)

				sqlMock.ExpectQuery(`SELECT COUNT\(\*\) AS total_executions`).
					WithArgs(incidentType, sqlmock.AnyArg()).
					WillReturnRows(rows)

				// ACT: Call repository method
				result, err := repo.GetSuccessRateMultiDimensional(ctx, &models.MultiDimensionalQuery{
					IncidentType: incidentType,
					TimeRange:    "7d",
					MinSamples:   minSamples,
				})

				// ASSERT: No error
				Expect(err).ToNot(HaveOccurred())

				// CORRECTNESS: 100% success rate
				Expect(result.SuccessRate).To(Equal(100.0))
				Expect(result.TotalExecutions).To(Equal(25))
				Expect(result.SuccessfulExecutions).To(Equal(25))
				Expect(result.FailedExecutions).To(Equal(0))

				Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
			})

			It("should handle 0% success rate correctly", func() {
				// ARRANGE: Mock 0% success
				incidentType := "failing-action"
				minSamples := 5

				rows := sqlmock.NewRows([]string{
					"total_executions", "successful_executions", "failed_executions",
				}).AddRow(
					10, 0, 10,
				)

				sqlMock.ExpectQuery(`SELECT COUNT\(\*\) AS total_executions`).
					WithArgs(incidentType, sqlmock.AnyArg()).
					WillReturnRows(rows)

				// ACT: Call repository method
				result, err := repo.GetSuccessRateMultiDimensional(ctx, &models.MultiDimensionalQuery{
					IncidentType: incidentType,
					TimeRange:    "7d",
					MinSamples:   minSamples,
				})

				// ASSERT: No error
				Expect(err).ToNot(HaveOccurred())

				// CORRECTNESS: 0% success rate
				Expect(result.SuccessRate).To(Equal(0.0))
				Expect(result.TotalExecutions).To(Equal(10))
				Expect(result.SuccessfulExecutions).To(Equal(0))
				Expect(result.FailedExecutions).To(Equal(10))

				Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
			})
		})

		Context("confidence level calculation", func() {
			It("should return 'high' confidence for >= 100 samples", func() {
				// ARRANGE: Mock 120 samples
				incidentType := "high-volume-incident"
				minSamples := 5

				rows := sqlmock.NewRows([]string{
					"total_executions", "successful_executions", "failed_executions",
				}).AddRow(
					120, 110, 10,
				)

				sqlMock.ExpectQuery(`SELECT COUNT\(\*\) AS total_executions`).
					WithArgs(incidentType, sqlmock.AnyArg()).
					WillReturnRows(rows)

				// ACT
				result, err := repo.GetSuccessRateMultiDimensional(ctx, &models.MultiDimensionalQuery{
					IncidentType: incidentType,
					TimeRange:    "7d",
					MinSamples:   minSamples,
				})

				// ASSERT
				Expect(err).ToNot(HaveOccurred())
				Expect(result.Confidence).To(Equal("high"))
				Expect(result.MinSamplesMet).To(BeTrue())

				Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
			})

			It("should return 'medium' confidence for 20-99 samples", func() {
				// ARRANGE: Mock 50 samples
				incidentType := "medium-volume-incident"
				minSamples := 5

				rows := sqlmock.NewRows([]string{
					"total_executions", "successful_executions", "failed_executions",
				}).AddRow(
					50, 45, 5,
				)

				sqlMock.ExpectQuery(`SELECT COUNT\(\*\) AS total_executions`).
					WithArgs(incidentType, sqlmock.AnyArg()).
					WillReturnRows(rows)

				// ACT
				result, err := repo.GetSuccessRateMultiDimensional(ctx, &models.MultiDimensionalQuery{
					IncidentType: incidentType,
					TimeRange:    "7d",
					MinSamples:   minSamples,
				})

				// ASSERT
				Expect(err).ToNot(HaveOccurred())
				Expect(result.Confidence).To(Equal("medium"))
				Expect(result.MinSamplesMet).To(BeTrue())

				Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
			})

			It("should return 'low' confidence for 5-19 samples", func() {
				// ARRANGE: Mock 10 samples
				incidentType := "low-volume-incident"
				minSamples := 5

				rows := sqlmock.NewRows([]string{
					"total_executions", "successful_executions", "failed_executions",
				}).AddRow(
					10, 9, 1,
				)

				sqlMock.ExpectQuery(`SELECT COUNT\(\*\) AS total_executions`).
					WithArgs(incidentType, sqlmock.AnyArg()).
					WillReturnRows(rows)

				// ACT
				result, err := repo.GetSuccessRateMultiDimensional(ctx, &models.MultiDimensionalQuery{
					IncidentType: incidentType,
					TimeRange:    "7d",
					MinSamples:   minSamples,
				})

				// ASSERT
				Expect(err).ToNot(HaveOccurred())
				Expect(result.Confidence).To(Equal("low"))
				Expect(result.MinSamplesMet).To(BeTrue())

				Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
			})

			It("should return 'insufficient_data' confidence for < 5 samples", func() {
				// ARRANGE: Mock 3 samples
				incidentType := "rare-incident"
				minSamples := 5

				rows := sqlmock.NewRows([]string{
					"total_executions", "successful_executions", "failed_executions",
				}).AddRow(
					3, 3, 0,
				)

				sqlMock.ExpectQuery(`SELECT COUNT\(\*\) AS total_executions`).
					WithArgs(incidentType, sqlmock.AnyArg()).
					WillReturnRows(rows)

				// ACT
				result, err := repo.GetSuccessRateMultiDimensional(ctx, &models.MultiDimensionalQuery{
					IncidentType: incidentType,
					TimeRange:    "7d",
					MinSamples:   minSamples,
				})

				// ASSERT
				Expect(err).ToNot(HaveOccurred())
				Expect(result.Confidence).To(Equal("insufficient_data"))
				Expect(result.MinSamplesMet).To(BeFalse())

				Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
			})
		})
	})
})
