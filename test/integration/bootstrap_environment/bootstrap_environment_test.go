//go:build integration
// +build integration

package bootstrap_environment

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/bootstrap/validator"
	testshared "github.com/jordigilh/kubernaut/test/integration/shared"
)

var _ = Describe("BR-BOOTSTRAP-ENVIRONMENT-001: Complete Bootstrap Environment Validation", Ordered, func() {
	var (
		hooks              *testshared.TestLifecycleHooks
		ctx                context.Context
		suite              *testshared.StandardTestSuite
		bootstrapValidator *validator.BootstrapEnvironmentValidator
		logger             *logrus.Logger
	)

	BeforeAll(func() {
		// Following guideline: Reuse existing test infrastructure
		hooks = testshared.SetupAIIntegrationTest("Bootstrap Environment Validation",
			testshared.WithRealDatabase(),
			testshared.WithRealVectorDB(),
			testshared.WithMockLLM(), // Use mock LLM for consistent local testing
		)
		hooks.Setup()

		suite = hooks.GetSuite()
		logger = suite.Logger
	})

	AfterAll(func() {
		if hooks != nil {
			hooks.Cleanup()
		}
	})

	BeforeEach(func() {
		ctx = context.Background()

		// Create bootstrap environment validator for integration testing
		bootstrapValidator = validator.NewBootstrapEnvironmentValidator(suite, logger)
		Expect(bootstrapValidator).ToNot(BeNil(), "Bootstrap environment validator should be created successfully")
	})

	Context("when validating complete system workflow with bootstrap environment", func() {
		It("should validate full stack integration with the bootstrap development environment", func() {
			By("validating core database connectivity (PostgreSQL and pgvector)")
			dbStatus, err := bootstrapValidator.ValidateDatabaseConnectivity(ctx)
			Expect(err).ToNot(HaveOccurred(), "Database connectivity validation should succeed")
			Expect(dbStatus.PostgreSQLConnected).To(BeTrue(), "BR-BOOTSTRAP-ENVIRONMENT-002: PostgreSQL should be connected")
			Expect(dbStatus.PgVectorConnected).To(BeTrue(), "BR-BOOTSTRAP-ENVIRONMENT-003: pgvector should be connected")
			Expect(dbStatus.PgVectorDimension).To(BeNumerically(">", 0), "BR-BOOTSTRAP-ENVIRONMENT-004: pgvector dimension should be configured")

			By("validating AI & pgvector integration support")
			aiPgVectorSupport, err := bootstrapValidator.ValidateAIPgVectorSupport(ctx)
			Expect(err).ToNot(HaveOccurred(), "AI & pgvector support validation should succeed")
			Expect(aiPgVectorSupport.EmbeddingPipelineSupported).To(BeTrue(), "Should support embedding pipeline scenarios")
			Expect(aiPgVectorSupport.PerformanceIntegrationSupported).To(BeTrue(), "Should support performance integration scenarios")
			Expect(aiPgVectorSupport.AnalyticsIntegrationSupported).To(BeTrue(), "Should support analytics integration scenarios")

			By("validating Platform Multi-cluster integration support")
			multiClusterSupport, err := bootstrapValidator.ValidateMultiClusterSupport(ctx)
			Expect(err).ToNot(HaveOccurred(), "Platform Multi-cluster support validation should succeed")
			Expect(multiClusterSupport.VectorSyncSupported).To(BeTrue(), "Should support vector data synchronization")
			Expect(multiClusterSupport.ResourceDiscoverySupported).To(BeTrue(), "Should support resource discovery with vector correlation")
			Expect(multiClusterSupport.ResourceOptimizationSupported).To(BeTrue(), "Should support resource allocation optimization")

			By("validating Workflow Engine pgvector integration support")
			workflowPgVectorSupport, err := bootstrapValidator.ValidateWorkflowPgVectorSupport(ctx)
			Expect(err).ToNot(HaveOccurred(), "Workflow Engine pgvector support validation should succeed")
			Expect(workflowPgVectorSupport.StatePersistenceSupported).To(BeTrue(), "Should support state persistence scenarios")
			Expect(workflowPgVectorSupport.VectorDecisionMakingSupported).To(BeTrue(), "Should support vector decision making scenarios")
			Expect(workflowPgVectorSupport.ResourceOptimizationSupported).To(BeTrue(), "Should support resource optimization scenarios")

			By("measuring overall integration test environment readiness")
			// BR-BOOTSTRAP-ENVIRONMENT-007: Integration test readiness validation
			readinessSum := aiPgVectorSupport.OverallReadinessScore + multiClusterSupport.OverallReadinessScore + workflowPgVectorSupport.OverallReadinessScore
			overallReadiness := readinessSum / 3.0

			Expect(overallReadiness).To(BeNumerically(">=", 0.9), "BR-BOOTSTRAP-ENVIRONMENT-007: Bootstrap environment should be highly ready for integration tests")
		})

		It("should validate complete bootstrap environment workflow", func() {
			By("executing complete bootstrap environment validation")
			validationStartTime := time.Now()
			environmentSummary, err := bootstrapValidator.ValidateBootstrapEnvironment(ctx)
			validationTime := time.Since(validationStartTime)

			Expect(err).ToNot(HaveOccurred(), "Complete bootstrap environment validation should succeed")
			Expect(environmentSummary).ToNot(BeNil(), "Should provide environment summary")

			// BR-BOOTSTRAP-ENVIRONMENT-001: Performance validation
			Expect(validationTime).To(BeNumerically("<", 30*time.Second), "BR-BOOTSTRAP-ENVIRONMENT-001: Complete validation should be efficient")

			// BR-BOOTSTRAP-ENVIRONMENT-008: Overall readiness validation
			Expect(environmentSummary.ReadinessScore).To(BeNumerically(">=", 0.9), "BR-BOOTSTRAP-ENVIRONMENT-008: Overall readiness should be high")

			// Validate individual components
			Expect(environmentSummary.DatabaseStatus.PostgreSQLConnected).To(BeTrue(), "Database should be connected")
			Expect(environmentSummary.DatabaseStatus.PgVectorConnected).To(BeTrue(), "pgvector should be connected")
			Expect(environmentSummary.AIPgVectorSupport.OverallReadinessScore).To(BeNumerically(">=", 0.8), "AI support should be ready")
			Expect(environmentSummary.MultiClusterSupport.OverallReadinessScore).To(BeNumerically(">=", 0.8), "Multi-cluster support should be ready")
			Expect(environmentSummary.WorkflowPgVectorSupport.OverallReadinessScore).To(BeNumerically(">=", 0.8), "Workflow support should be ready")

			By("validating system health status")
			systemHealth, err := bootstrapValidator.ValidateSystemHealth(ctx)
			Expect(err).ToNot(HaveOccurred(), "System health validation should succeed")
			Expect(systemHealth.DatabaseHealthy).To(BeTrue(), "Database should be healthy")
			Expect(systemHealth.VectorDBHealthy).To(BeTrue(), "Vector database should be healthy")
			Expect(systemHealth.OverallHealthScore).To(BeNumerically(">=", 0.9), "Overall system health should be high")
		})
	})

	Context("when testing bootstrap environment business requirements", func() {
		It("should validate all bootstrap environment business requirements", func() {
			By("testing BR-BOOTSTRAP-ENVIRONMENT-002: Database connectivity requirements")
			dbStatus, err := bootstrapValidator.ValidateDatabaseConnectivity(ctx)
			Expect(err).ToNot(HaveOccurred(), "Database connectivity should meet requirements")

			// Business requirement validations
			Expect(dbStatus.PostgreSQLConnected).To(BeTrue(), "BR-BOOTSTRAP-ENVIRONMENT-002: PostgreSQL connectivity in bootstrap environment")
			Expect(dbStatus.PgVectorConnected).To(BeTrue(), "BR-BOOTSTRAP-ENVIRONMENT-003: pgvector connectivity in bootstrap environment")
			Expect(dbStatus.PgVectorDimension).To(BeNumerically(">", 0), "BR-BOOTSTRAP-ENVIRONMENT-004: pgvector dimension should be configured")

			By("testing BR-BOOTSTRAP-ENVIRONMENT-005: Multi-component integration requirements")
			aiSupport, err := bootstrapValidator.ValidateAIPgVectorSupport(ctx)
			Expect(err).ToNot(HaveOccurred(), "AI support validation should meet requirements")

			Expect(aiSupport.OverallReadinessScore).To(BeNumerically(">=", 0.8), "BR-BOOTSTRAP-ENVIRONMENT-005: AI & pgvector integration readiness should be high")

			multiClusterSupport, err := bootstrapValidator.ValidateMultiClusterSupport(ctx)
			Expect(err).ToNot(HaveOccurred(), "Multi-cluster support validation should meet requirements")

			Expect(multiClusterSupport.OverallReadinessScore).To(BeNumerically(">=", 0.8), "BR-BOOTSTRAP-ENVIRONMENT-006: Multi-cluster integration readiness should be high")

			workflowSupport, err := bootstrapValidator.ValidateWorkflowPgVectorSupport(ctx)
			Expect(err).ToNot(HaveOccurred(), "Workflow support validation should meet requirements")

			Expect(workflowSupport.OverallReadinessScore).To(BeNumerically(">=", 0.8), "BR-BOOTSTRAP-ENVIRONMENT-007: Workflow pgvector integration readiness should be high")

			By("testing BR-BOOTSTRAP-ENVIRONMENT-008: System health requirements")
			systemHealth, err := bootstrapValidator.ValidateSystemHealth(ctx)
			Expect(err).ToNot(HaveOccurred(), "System health validation should meet requirements")

			Expect(systemHealth.OverallHealthScore).To(BeNumerically(">=", 0.9), "BR-BOOTSTRAP-ENVIRONMENT-008: Overall system health score should be high")

			By("testing BR-BOOTSTRAP-ENVIRONMENT-009: Complete environment validation requirements")
			environmentSummary, err := bootstrapValidator.ValidateBootstrapEnvironment(ctx)
			Expect(err).ToNot(HaveOccurred(), "Complete environment validation should meet requirements")

			Expect(environmentSummary.ReadinessScore).To(BeNumerically(">=", 0.9), "BR-BOOTSTRAP-ENVIRONMENT-009: Complete bootstrap environment readiness should be high")

			// BR-BOOTSTRAP-ENVIRONMENT-010: Validation time requirements (current milestone: accuracy over speed)
			Expect(environmentSummary.ValidationTime).To(BeNumerically("<", 60*time.Second), "BR-BOOTSTRAP-ENVIRONMENT-010: Validation should complete within reasonable time")
		})
	})
})
