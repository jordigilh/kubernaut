//go:build unit
// +build unit

package platform

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/platform/executor"
	"github.com/jordigilh/kubernaut/pkg/platform/testutil"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	pkgtestutil "github.com/jordigilh/kubernaut/pkg/testutil"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

/*
 * Business Requirement Validation: Enterprise Platform & Execution
 *
 * This test suite validates business requirements for enterprise platform capabilities
 * following development guidelines:
 * - Reuses existing platform test patterns (Ginkgo/Gomega, testutil)
 * - Focuses on business outcomes: enterprise scale, cost optimization, compliance
 * - Uses meaningful assertions with business performance thresholds
 * - Integrates with existing executor and platform components
 * - Logs all errors and enterprise compliance metrics
 */

var _ = Describe("Business Requirement Validation: Enterprise Platform & Execution", func() {
	var (
		ctx                  context.Context
		cancel               context.CancelFunc
		logger               *logrus.Logger
		actionExecutor       executor.Executor
		crossClusterExecutor executor.CrossClusterExecutor
		costAnalyzer         executor.CostAnalyzer
		complianceAuditor    executor.ComplianceAuditor
		fakeK8sClient        *fake.Clientset
		mockActionHistory    *mocks.MockActionHistoryRepository
		testSuite            *testutil.PlatformTestSuiteComponents
		commonAssertions     *pkgtestutil.CommonAssertions
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 60*time.Second)

		// Reuse existing platform test suite setup from action_executor_test.go
		testSuite = testutil.ExecutorTestSuite("EnterprisePlatformTests")
		logger = testSuite.Logger
		logger.SetLevel(logrus.InfoLevel) // Enable info logging for business metrics
		fakeK8sClient = testSuite.FakeClientset
		commonAssertions = pkgtestutil.NewCommonAssertions()

		mockActionHistory = mocks.NewMockActionHistoryRepository()

		// Setup enterprise executor configurations
		executorConfig := config.ActionsConfig{
			DryRun:             false,
			MaxConcurrent:      20, // Business requirement: support enterprise scale
			CooldownPeriod:     2 * time.Minute,
			EnableCrossCluster: true, // Business requirement: cross-cluster operations
			EnableCostTracking: true, // Business requirement: cost analysis
			EnableCompliance:   true, // Business requirement: compliance auditing
		}

		actionExecutor = executor.NewActionExecutor(
			fakeK8sClient,
			mockActionHistory,
			executorConfig,
			logger,
		)

		crossClusterExecutor = executor.NewCrossClusterExecutor(logger)
		costAnalyzer = executor.NewCostAnalyzer(mockActionHistory, logger)
		complianceAuditor = executor.NewComplianceAuditor(logger)

		setupEnterpriseBusinessData(mockActionHistory)
	})

	AfterEach(func() {
		cancel()
	})

	/*
	 * Business Requirement: BR-EXEC-032
	 * Business Logic: MUST coordinate actions across multiple Kubernetes clusters
	 *
	 * Business Success Criteria:
	 *   - Multi-cluster action execution with 100% consistency across clusters
	 *   - Network partition handling with graceful degradation and recovery
	 *   - Cluster health assessment with automatic failover capabilities
	 *   - Cross-cluster resource dependency resolution with business continuity
	 *
	 * Test Focus: Cross-cluster coordination enabling enterprise-scale Kubernetes management
	 * Expected Business Value: Unified management of distributed Kubernetes environments
	 */
	Context("BR-EXEC-032: Cross-Cluster Action Coordination for Enterprise Scale", func() {
		It("should execute actions consistently across multiple clusters with business reliability", func() {
			By("Setting up multi-cluster business scenario")

			// Business Context: Enterprise deployment across multiple Kubernetes clusters
			enterpriseClusters := []EnterpriseCluster{
				{
					Name:        "production-east",
					Region:      "us-east-1",
					Environment: "production",
					Priority:    "critical",
					HealthScore: 0.95,
				},
				{
					Name:        "production-west",
					Region:      "us-west-1",
					Environment: "production",
					Priority:    "critical",
					HealthScore: 0.92,
				},
				{
					Name:        "staging-central",
					Region:      "us-central-1",
					Environment: "staging",
					Priority:    "high",
					HealthScore: 0.88,
				},
			}

			// Business scenario: Rolling update across all production clusters
			businessAction := types.ActionRecommendation{
				Action:     "rolling_update_deployment",
				Confidence: 0.90,
				Parameters: map[string]interface{}{
					"deployment": "web-service",
					"namespace":  "production",
					"image":      "app:v1.2.3",
					"strategy":   "rolling",
				},
			}

			By("Executing cross-cluster action with business consistency requirements")

			executionStart := time.Now()
			crossClusterResult, err := crossClusterExecutor.ExecuteAcrossClusters(ctx, businessAction, enterpriseClusters)
			executionDuration := time.Since(executionStart)

			// Business Requirement: Cross-cluster execution must succeed
			Expect(err).ToNot(HaveOccurred(), "Cross-cluster execution must succeed for business operations")
			Expect(crossClusterResult).ToNot(BeNil(), "Must provide execution results for business monitoring")

			By("Validating execution consistency across clusters")
			successfulClusters := 0
			for clusterName, result := range crossClusterResult.ClusterResults {
				if result.Success {
					successfulClusters++
				}

				// Business Requirement: All critical clusters must succeed
				cluster := findCluster(enterpriseClusters, clusterName)
				if cluster != nil && cluster.Priority == "critical" {
					Expect(result.Success).To(BeTrue(),
						"Critical cluster %s must succeed for business continuity", clusterName)
				}

				// Log cluster-specific results
				logger.WithFields(logrus.Fields{
					"cluster":     clusterName,
					"success":     result.Success,
					"duration_ms": result.ExecutionTime.Milliseconds(),
					"priority":    cluster.Priority,
				}).Info("Cross-cluster execution result")
			}

			// Business Requirement: 100% consistency across clusters
			consistencyRate := float64(successfulClusters) / float64(len(enterpriseClusters))
			Expect(consistencyRate).To(Equal(1.0),
				"Must achieve 100% execution consistency across all clusters for business reliability")

			By("Validating enterprise performance requirements")
			// Business Requirement: Reasonable execution time for enterprise scale
			Expect(executionDuration).To(BeNumerically("<", 5*time.Minute),
				"Cross-cluster execution must complete within 5 minutes for business SLA")

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement":    "BR-EXEC-032",
				"clusters_targeted":       len(enterpriseClusters),
				"successful_executions":   successfulClusters,
				"consistency_rate":        consistencyRate,
				"total_execution_minutes": executionDuration.Minutes(),
				"business_impact":         "Unified cross-cluster management enables enterprise scalability",
			}).Info("BR-EXEC-032: Cross-cluster coordination business validation completed")
		})

		It("should handle network partitions and cluster failures with business continuity", func() {
			By("Simulating network partition and cluster failure scenarios")

			// Business Context: Network partitions and cluster failures in enterprise environment
			failureScenarios := []ClusterFailureScenario{
				{
					ClusterName:     "production-east",
					FailureType:     "network_partition",
					FailureDuration: 30 * time.Second,
					BusinessImpact:  "high", // Production cluster failure
				},
				{
					ClusterName:     "staging-central",
					FailureType:     "cluster_unavailable",
					FailureDuration: 15 * time.Second,
					BusinessImpact:  "medium", // Staging cluster failure
				},
			}

			// Business action requiring cross-cluster coordination
			criticalBusinessAction := types.ActionRecommendation{
				Action: "emergency_scale_up",
				Parameters: map[string]interface{}{
					"deployment": "critical-service",
					"replicas":   10,
					"priority":   "critical",
				},
			}

			for _, scenario := range failureScenarios {
				By(fmt.Sprintf("Testing resilience for %s failure on cluster %s", scenario.FailureType, scenario.ClusterName))

				// Simulate cluster failure
				crossClusterExecutor.SimulateClusterFailure(scenario.ClusterName, scenario.FailureType, scenario.FailureDuration)

				failureHandlingStart := time.Now()
				result, err := crossClusterExecutor.ExecuteWithFailureHandling(ctx, criticalBusinessAction)
				failureHandlingDuration := time.Since(failureHandlingStart)

				// Business Requirement: Graceful degradation during failures
				Expect(err).ToNot(HaveOccurred(), "Cross-cluster execution must handle failures gracefully")

				// Business Validation: Must achieve partial success for business continuity
				Expect(result.PartialSuccessAchieved).To(BeTrue(),
					"Must achieve partial success during cluster failures for business continuity")

				// Business Requirement: Automatic failover capabilities
				if result.FailoverExecuted {
					Expect(result.FailoverTargetCluster).ToNot(BeEmpty(),
						"Failover must target alternative cluster for business continuity")
					Expect(result.FailoverTime).To(BeNumerically("<", 60*time.Second),
						"Failover must complete within 60 seconds for business RTO")
				}

				// Business Requirement: Recovery after partition/failure ends
				time.Sleep(scenario.FailureDuration + 5*time.Second) // Wait for recovery

				recoveryResult, err := crossClusterExecutor.ValidateClusterRecovery(ctx, scenario.ClusterName)
				Expect(err).ToNot(HaveOccurred(), "Cluster recovery validation must succeed")
				Expect(recoveryResult.ClusterHealthy).To(BeTrue(),
					"Cluster must recover to healthy state after partition/failure")

				// Log failure handling metrics
				logger.WithFields(logrus.Fields{
					"cluster":              scenario.ClusterName,
					"failure_type":         scenario.FailureType,
					"business_impact":      scenario.BusinessImpact,
					"partial_success":      result.PartialSuccessAchieved,
					"failover_executed":    result.FailoverExecuted,
					"recovery_successful":  recoveryResult.ClusterHealthy,
					"handling_duration_ms": failureHandlingDuration.Milliseconds(),
				}).Info("Cluster failure handling business scenario evaluated")
			}

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement": "BR-EXEC-032",
				"scenario":             "failure_resilience",
				"failure_scenarios":    len(failureScenarios),
				"business_impact":      "Failure resilience maintains business operations during infrastructure issues",
			}).Info("BR-EXEC-032: Failure resilience business validation completed")
		})
	})

	/*
	 * Business Requirement: BR-EXEC-044
	 * Business Logic: MUST analyze and optimize costs associated with action execution
	 *
	 * Business Success Criteria:
	 *   - Cost calculation accuracy for different action types with business budgeting
	 *   - Resource cost optimization with measurable savings targets >15%
	 *   - Cost-effectiveness analysis with business ROI quantification
	 *   - Budget threshold enforcement with business spending controls
	 *
	 * Test Focus: Cost optimization delivering measurable business financial benefits
	 * Expected Business Value: Reduced operational costs through intelligent resource management
	 */
	Context("BR-EXEC-044: Cost Analysis and Optimization for Business Financial Control", func() {
		It("should provide accurate cost analysis with measurable business ROI", func() {
			By("Setting up business cost analysis scenarios")

			// Business Context: Different action types with associated costs
			businessCostScenarios := []BusinessCostScenario{
				{
					ActionType:       "scale_deployment",
					ResourceType:     "compute",
					BaselineCost:     100.0, // $100/hour baseline
					OptimizedCost:    80.0,  // $80/hour optimized (20% savings)
					ExecutionCount:   50,    // Monthly execution volume
					BusinessPriority: "high",
				},
				{
					ActionType:       "storage_expansion",
					ResourceType:     "storage",
					BaselineCost:     200.0, // $200/month baseline
					OptimizedCost:    170.0, // $170/month optimized (15% savings)
					ExecutionCount:   10,    // Monthly execution volume
					BusinessPriority: "medium",
				},
				{
					ActionType:       "network_reconfiguration",
					ResourceType:     "network",
					BaselineCost:     50.0, // $50/change baseline
					OptimizedCost:    42.5, // $42.50/change optimized (15% savings)
					ExecutionCount:   25,   // Monthly execution volume
					BusinessPriority: "medium",
				},
			}

			totalMonthlySavings := 0.0

			for _, scenario := range businessCostScenarios {
				By(fmt.Sprintf("Analyzing costs for %s operations", scenario.ActionType))

				// Calculate baseline monthly cost
				baselineMonthlyCost := scenario.BaselineCost * float64(scenario.ExecutionCount)

				// Run cost optimization analysis
				optimizationResult, err := costAnalyzer.OptimizeActionCosts(ctx, scenario.ActionType, scenario)
				Expect(err).ToNot(HaveOccurred(), "Cost optimization analysis must succeed")

				// Business Requirement: Cost calculation accuracy
				Expect(optimizationResult.CalculatedBaseline).To(BeNumerically("~", baselineMonthlyCost, 5.0),
					"Cost calculation must be accurate within $5 for business budgeting")

				// Calculate actual cost savings
				actualSavings := optimizationResult.CalculatedBaseline - optimizationResult.OptimizedCost
				savingsPercentage := actualSavings / optimizationResult.CalculatedBaseline

				// Business Requirement: >15% cost savings
				Expect(savingsPercentage).To(BeNumerically(">=", 0.15),
					"Cost optimization must achieve >=15% savings for %s", scenario.ActionType)

				totalMonthlySavings += actualSavings

				// Business Requirement: ROI calculation
				implementationCost := 1000.0 // $1K implementation cost assumption
				monthsToROI := implementationCost / actualSavings

				Expect(monthsToROI).To(BeNumerically("<=", 12.0),
					"Cost optimization ROI must be achieved within 12 months for business justification")

				// Log cost analysis results
				logger.WithFields(logrus.Fields{
					"action_type":        scenario.ActionType,
					"baseline_monthly":   baselineMonthlyCost,
					"optimized_monthly":  optimizationResult.OptimizedCost,
					"monthly_savings":    actualSavings,
					"savings_percentage": savingsPercentage * 100,
					"months_to_roi":      monthsToROI,
				}).Info("Cost optimization business scenario evaluated")
			}

			By("Validating overall business financial impact")
			annualSavings := totalMonthlySavings * 12

			// Business Requirement: Significant total savings
			Expect(annualSavings).To(BeNumerically(">=", 5000),
				"Total annual cost savings must be >=5K for meaningful business impact")

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement":  "BR-EXEC-044",
				"total_monthly_savings": totalMonthlySavings,
				"total_annual_savings":  annualSavings,
				"scenarios_optimized":   len(businessCostScenarios),
				"business_impact":       "Cost optimization delivers measurable financial benefits",
			}).Info("BR-EXEC-044: Cost analysis business validation completed")
		})

		It("should enforce budget thresholds with business spending controls", func() {
			By("Setting up budget threshold enforcement scenarios")

			// Business Context: Budget controls for different operational scenarios
			budgetControlScenarios := []BudgetControlScenario{
				{
					BudgetPeriod:    "monthly",
					BudgetLimit:     5000.0, // $5K monthly limit
					CurrentSpending: 4800.0, // $4.8K current spending (96% utilization)
					ProposedAction: ActionCostProfile{
						ActionType:     "emergency_scaling",
						EstimatedCost:  300.0, // Would exceed budget
						BusinessReason: "production_outage",
					},
					ExpectedApproval: false, // Should require approval due to budget overflow
				},
				{
					BudgetPeriod:    "monthly",
					BudgetLimit:     5000.0, // $5K monthly limit
					CurrentSpending: 4200.0, // $4.2K current spending (84% utilization)
					ProposedAction: ActionCostProfile{
						ActionType:     "routine_maintenance",
						EstimatedCost:  600.0, // Within budget
						BusinessReason: "scheduled_maintenance",
					},
					ExpectedApproval: true, // Should be automatically approved
				},
			}

			for _, scenario := range budgetControlScenarios {
				By(fmt.Sprintf("Testing budget control for %s scenario", scenario.ProposedAction.BusinessReason))

				budgetUtilization := scenario.CurrentSpending / scenario.BudgetLimit
				projectedUtilization := (scenario.CurrentSpending + scenario.ProposedAction.EstimatedCost) / scenario.BudgetLimit

				// Test budget threshold enforcement
				approvalResult, err := costAnalyzer.EvaluateBudgetApproval(ctx, scenario)
				Expect(err).ToNot(HaveOccurred(), "Budget evaluation must succeed")

				// Business Requirement: Budget threshold enforcement
				if scenario.ExpectedApproval {
					Expect(approvalResult.AutoApproved).To(BeTrue(),
						"Action within budget should be auto-approved for business efficiency")
					Expect(projectedUtilization).To(BeNumerically("<=", 0.95),
						"Approved actions should keep utilization ≤95% for business safety margin")
				} else {
					Expect(approvalResult.RequiresApproval).To(BeTrue(),
						"Action exceeding budget should require approval for business control")

					// Business Requirement: Clear justification for budget overruns
					Expect(approvalResult.BusinessJustification).ToNot(BeEmpty(),
						"Budget overruns must include business justification")
				}

				// Log budget enforcement results
				logger.WithFields(logrus.Fields{
					"business_reason":       scenario.ProposedAction.BusinessReason,
					"current_utilization":   budgetUtilization,
					"projected_utilization": projectedUtilization,
					"auto_approved":         approvalResult.AutoApproved,
					"requires_approval":     approvalResult.RequiresApproval,
					"budget_limit":          scenario.BudgetLimit,
				}).Info("Budget control business scenario evaluated")
			}

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement": "BR-EXEC-044",
				"scenario":             "budget_enforcement",
				"control_scenarios":    len(budgetControlScenarios),
				"business_impact":      "Budget controls prevent overspending while enabling business operations",
			}).Info("BR-EXEC-044: Budget enforcement business validation completed")
		})
	})

	/*
	 * Business Requirement: BR-EXEC-054
	 * Business Logic: MUST provide comprehensive audit trails for regulatory compliance
	 *
	 * Business Success Criteria:
	 *   - Audit trail completeness with tamper-proof logging for regulatory compliance
	 *   - Compliance reporting with industry standard formats (SOX, SOC2, GDPR)
	 *   - Retention policy enforcement with legal requirement compliance
	 *   - Access control logging with complete administrative accountability
	 *
	 * Test Focus: Regulatory compliance capabilities meeting business legal requirements
	 * Expected Business Value: Enterprise deployment readiness with full regulatory compliance
	 */
	Context("BR-EXEC-054: Compliance and Audit for Regulatory Business Requirements", func() {
		It("should maintain comprehensive audit trails with regulatory compliance standards", func() {
			By("Setting up compliance audit scenarios for business regulatory requirements")

			// Business Context: Regulatory compliance scenarios
			complianceScenarios := []ComplianceScenario{
				{
					RegulationFramework: "SOX",
					RequiredElements:    []string{"user_identity", "action_details", "timestamp", "approval_chain", "business_justification"},
					RetentionPeriod:     7 * 365 * 24 * time.Hour, // 7 years for SOX
					AuditLevel:          "comprehensive",
				},
				{
					RegulationFramework: "SOC2",
					RequiredElements:    []string{"access_controls", "data_handling", "security_measures", "monitoring_evidence"},
					RetentionPeriod:     3 * 365 * 24 * time.Hour, // 3 years for SOC2
					AuditLevel:          "security_focused",
				},
				{
					RegulationFramework: "GDPR",
					RequiredElements:    []string{"data_processing", "consent_tracking", "data_subject_rights", "privacy_impact"},
					RetentionPeriod:     6 * 365 * 24 * time.Hour, // 6 years for GDPR
					AuditLevel:          "privacy_focused",
				},
			}

			// Business action requiring audit compliance
			auditedBusinessAction := types.ActionRecommendation{
				Action:     "user_data_processing",
				Confidence: 0.95,
				Parameters: map[string]interface{}{
					"data_type":        "customer_information",
					"processing_type":  "analytics",
					"business_purpose": "service_improvement",
				},
			}

			for _, scenario := range complianceScenarios {
				By(fmt.Sprintf("Validating %s compliance for business audit requirements", scenario.RegulationFramework))

				// Execute action with compliance auditing enabled
				auditStart := time.Now()
				auditResult, err := complianceAuditor.ExecuteWithCompliance(ctx, auditedBusinessAction, scenario)
				auditDuration := time.Since(auditStart)

				Expect(err).ToNot(HaveOccurred(), "Compliance-audited execution must succeed")
				Expect(auditResult).ToNot(BeNil(), "Must provide comprehensive audit results")

				// Business Requirement: Audit trail completeness
				for _, requiredElement := range scenario.RequiredElements {
					Expect(auditResult.AuditTrail).To(HaveKey(requiredElement),
						"Audit trail must include %s for %s compliance", requiredElement, scenario.RegulationFramework)
				}

				// Business Requirement: Tamper-proof logging
				Expect(auditResult.IntegrityHash).ToNot(BeEmpty(),
					"Audit trail must include integrity hash for tamper-proof logging")

				// Verify hash integrity
				recalculatedHash := complianceAuditor.CalculateAuditHash(auditResult.AuditTrail)
				Expect(recalculatedHash).To(Equal(auditResult.IntegrityHash),
					"Audit trail integrity must be verifiable for regulatory compliance")

				// Business Requirement: Retention policy enforcement
				Expect(auditResult.RetentionUntil.After(time.Now().Add(scenario.RetentionPeriod-24*time.Hour))).To(BeTrue(),
					"Retention period must meet regulatory requirements for %s", scenario.RegulationFramework)

				// Business Requirement: Complete administrative accountability
				Expect(auditResult.UserIdentity).ToNot(BeEmpty(),
					"Must track user identity for administrative accountability")
				Expect(auditResult.ApprovalChain).ToNot(BeEmpty(),
					"Must track approval chain for business accountability")

				// Performance requirement: Audit overhead must be minimal
				Expect(auditDuration).To(BeNumerically("<", 2*time.Second),
					"Audit logging must have <2s overhead for business operational efficiency")

				// Log compliance validation results
				logger.WithFields(logrus.Fields{
					"regulation":          scenario.RegulationFramework,
					"required_elements":   len(scenario.RequiredElements),
					"elements_captured":   len(auditResult.AuditTrail),
					"integrity_verified":  recalculatedHash == auditResult.IntegrityHash,
					"retention_compliant": auditResult.RetentionUntil.After(time.Now().Add(scenario.RetentionPeriod - 24*time.Hour)),
					"audit_overhead_ms":   auditDuration.Milliseconds(),
				}).Info("Compliance audit business scenario evaluated")
			}

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement":  "BR-EXEC-054",
				"compliance_frameworks": len(complianceScenarios),
				"business_impact":       "Comprehensive audit trails enable regulatory compliance for enterprise deployment",
			}).Info("BR-EXEC-054: Compliance audit business validation completed")
		})

		It("should generate compliance reports in industry standard formats", func() {
			By("Generating compliance reports for business regulatory requirements")

			// Business Context: Compliance report generation for different stakeholders
			reportingRequirements := []ComplianceReporting{
				{
					ReportType:          "SOX_quarterly",
					OutputFormat:        "pdf",
					BusinessStakeholder: "finance_audit_committee",
					RequiredSections:    []string{"executive_summary", "control_activities", "deficiency_analysis", "management_response"},
				},
				{
					ReportType:          "SOC2_annual",
					OutputFormat:        "xml",
					BusinessStakeholder: "security_compliance_officer",
					RequiredSections:    []string{"control_environment", "risk_assessment", "monitoring_activities", "information_communication"},
				},
				{
					ReportType:          "GDPR_data_processing",
					OutputFormat:        "json",
					BusinessStakeholder: "data_protection_officer",
					RequiredSections:    []string{"processing_activities", "legal_basis", "data_subject_rights", "breach_incidents"},
				},
			}

			for _, reporting := range reportingRequirements {
				By(fmt.Sprintf("Generating %s report for %s", reporting.ReportType, reporting.BusinessStakeholder))

				reportGenerationStart := time.Now()
				complianceReport, err := complianceAuditor.GenerateComplianceReport(ctx, reporting)
				reportGenerationTime := time.Since(reportGenerationStart)

				Expect(err).ToNot(HaveOccurred(), "Compliance report generation must succeed")
				Expect(complianceReport).ToNot(BeNil(), "Must generate compliance report")

				// Business Requirement: Industry standard format compliance
				Expect(complianceReport.Format).To(Equal(reporting.OutputFormat),
					"Report must be in requested format for business stakeholder consumption")

				// Business Requirement: Required sections completeness
				for _, section := range reporting.RequiredSections {
					Expect(complianceReport.Sections).To(HaveKey(section),
						"Report must include %s section for %s compliance", section, reporting.ReportType)
				}

				// Business Requirement: Report generation performance
				Expect(reportGenerationTime).To(BeNumerically("<", 30*time.Second),
					"Report generation must complete within 30 seconds for business efficiency")

				// Business Requirement: Report data accuracy and completeness
				Expect(complianceReport.DataAccuracy).To(BeNumerically(">=", 0.98),
					"Report data accuracy must be >=98% for regulatory reliability")

				// Log report generation metrics
				logger.WithFields(logrus.Fields{
					"report_type":          reporting.ReportType,
					"output_format":        reporting.OutputFormat,
					"business_stakeholder": reporting.BusinessStakeholder,
					"sections_included":    len(complianceReport.Sections),
					"data_accuracy":        complianceReport.DataAccuracy,
					"generation_time_ms":   reportGenerationTime.Milliseconds(),
				}).Info("Compliance report generation business scenario evaluated")
			}

			// Business Impact Logging
			logger.WithFields(logrus.Fields{
				"business_requirement": "BR-EXEC-054",
				"scenario":             "report_generation",
				"report_types":         len(reportingRequirements),
				"business_impact":      "Automated compliance reporting reduces manual effort and ensures regulatory readiness",
			}).Info("BR-EXEC-054: Compliance reporting business validation completed")
		})
	})
})

// Business type definitions for enterprise platform testing

type EnterpriseCluster struct {
	Name        string
	Region      string
	Environment string
	Priority    string
	HealthScore float64
}

type ClusterFailureScenario struct {
	ClusterName     string
	FailureType     string
	FailureDuration time.Duration
	BusinessImpact  string
}

type BusinessCostScenario struct {
	ActionType       string
	ResourceType     string
	BaselineCost     float64
	OptimizedCost    float64
	ExecutionCount   int
	BusinessPriority string
}

type BudgetControlScenario struct {
	BudgetPeriod     string
	BudgetLimit      float64
	CurrentSpending  float64
	ProposedAction   ActionCostProfile
	ExpectedApproval bool
}

type ActionCostProfile struct {
	ActionType     string
	EstimatedCost  float64
	BusinessReason string
}

type ComplianceScenario struct {
	RegulationFramework string
	RequiredElements    []string
	RetentionPeriod     time.Duration
	AuditLevel          string
}

type ComplianceReporting struct {
	ReportType          string
	OutputFormat        string
	BusinessStakeholder string
	RequiredSections    []string
}

// Helper functions for enterprise business scenarios

func setupEnterpriseBusinessData(mockActionHistory *mocks.MockActionHistoryRepository) {
	// Setup realistic enterprise action history for cost analysis
	enterpriseActions := []struct {
		actionType string
		cost       float64
		frequency  int // per month
	}{
		{"scale_deployment", 100.0, 50},
		{"storage_expansion", 200.0, 10},
		{"network_reconfiguration", 50.0, 25},
		{"security_update", 75.0, 20},
		{"backup_restore", 150.0, 5},
	}

	for _, action := range enterpriseActions {
		for i := 0; i < action.frequency; i++ {
			mockActionHistory.AddActionExecution(action.actionType, action.cost, time.Now().Add(-time.Duration(i)*time.Hour))
		}
	}
}

func findCluster(clusters []EnterpriseCluster, name string) *EnterpriseCluster {
	for i, cluster := range clusters {
		if cluster.Name == name {
			return &clusters[i]
		}
	}
	return nil
}
