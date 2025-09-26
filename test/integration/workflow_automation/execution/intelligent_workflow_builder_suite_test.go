//go:build integration

package execution

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	testshared "github.com/jordigilh/kubernaut/test/integration/shared"
)

func TestIntelligentWorkflowBuilder(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Intelligent Workflow Builder Integration Tests Suite")
}

// BR-WORKFLOW-INTEGRATION-001: Intelligent Workflow Builder Business Operations
// Business Impact: Ensures AI-powered workflow generation for executive operational efficiency
// Stakeholder Value: Provides end-to-end workflow automation for business continuity

var _ = Describe("Intelligent Workflow Builder Integration Tests", func() {
	var (
		workflowBuilder engine.IntelligentWorkflowBuilder
		mockLLMClient   llm.Client
		mockVectorDB    vector.VectorDatabase
		ctx             context.Context
		mockLogger      *mocks.MockLogger
		logger          *logrus.Logger // Business requirement: BR-WORKFLOW-LOG-001 business metrics logging
		testConfig      testshared.IntegrationConfig
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockLogger = mocks.NewMockLogger()
		logger = mockLogger.Logger // Business requirement: Use mockLogger for business metrics

		// Load real integration test configuration
		testConfig = testshared.LoadConfig()

		// Create real LLM client for integration testing
		var err error
		llmConfig := config.LLMConfig{
			Provider:    testConfig.LLMProvider,
			Model:       testConfig.LLMModel,
			Endpoint:    testConfig.LLMEndpoint,
			Timeout:     testConfig.TestTimeout,
			Temperature: 0.7,
			MaxTokens:   1000,
		}
		mockLLMClient, err = llm.NewClient(llmConfig, mockLogger.Logger)
		Expect(err).ToNot(HaveOccurred(), "BR-WORKFLOW-INTEGRATION-001: LLM client creation must succeed for workflow intelligence")

		// Create vector database for integration testing
		mockVectorDB = vector.NewMemoryVectorDatabase(mockLogger.Logger)

		// Create real intelligent workflow builder using config pattern
		config := &engine.IntelligentWorkflowBuilderConfig{
			LLMClient:       mockLLMClient, // External: Mock
			VectorDB:        mockVectorDB,  // External: Mock
			AnalyticsEngine: nil,           // External: Mock not needed for this test
			PatternStore:    nil,           // External: Mock not needed for this test
			ExecutionRepo:   nil,           // External: Mock not needed for this test
			Logger:          mockLogger.Logger,
		}

		workflowBuilder, err = engine.NewIntelligentWorkflowBuilder(config)
		Expect(err).ToNot(HaveOccurred(), "Workflow builder creation should not fail")
	})

	Context("BR-WORKFLOW-INTEGRATION-001: Workflow Generation Operations", func() {
		It("should generate intelligent workflows for business automation", func() {
			// Business Requirement: Intelligent workflow builder must generate workflows from business objectives
			// Business Impact: Enables automated workflow creation for executive operational efficiency

			// Skip if LLM tests are disabled in CI/CD
			if testConfig.UseMockLLM {
				Skip("LLM tests disabled for CI/CD environment")
			}

			businessObjective := &engine.WorkflowObjective{
				Description: "Automate incident response for production critical alerts",
				Priority:    5, // High priority (1=low, 5=high)
				Constraints: map[string]interface{}{
					"max_duration":     "15m",
					"business_impact":  "critical",
					"automation_level": "high",
					"cost_threshold":   1000.0,
				},
			}

			// Test intelligent workflow generation - core business capability
			startTime := time.Now()
			workflow, err := workflowBuilder.GenerateWorkflow(ctx, businessObjective)
			duration := time.Since(startTime)

			// Business requirement validation
			Expect(err).ToNot(HaveOccurred(), "BR-WORKFLOW-INTEGRATION-001: Workflow generation must succeed for business automation")
			Expect(workflow).ToNot(BeNil(), "BR-WORKFLOW-INTEGRATION-001: Generated workflow must provide business value")

			// Business outcome validation: Workflow contains actionable business logic
			if workflow != nil {
				Expect(workflow.ID).ToNot(BeEmpty(), "BR-WORKFLOW-INTEGRATION-001: Generated workflow must have business identifier")
				Expect(workflow.Steps).ToNot(BeNil(), "BR-WORKFLOW-INTEGRATION-001: Workflow must contain executable business steps")
			}

			// Performance requirement: Workflow generation must complete within business SLA
			Expect(duration).To(BeNumerically("<=", testConfig.TestTimeout), "BR-WORKFLOW-INTEGRATION-001: Workflow generation must meet business SLA requirements")

			workflowID := "nil"
			if workflow != nil {
				workflowID = workflow.ID
			}

			logger.WithFields(logrus.Fields{
				"workflow_id":       workflowID,
				"generation_time":   duration,
				"business_priority": businessObjective.Priority,
			}).Info("BR-WORKFLOW-INTEGRATION-001: Intelligent workflow generation completed successfully")
		})

		It("should validate workflows for business compliance", func() {
			// Business Requirement: Workflow builder must validate workflows for business compliance
			// Business Impact: Ensures workflows meet business standards and operational requirements

			// Create a test workflow template for validation
			testTemplate := engine.NewWorkflowTemplate("compliance-test", "Business Compliance Validation Template")

			// Create an executable workflow step for business compliance
			validationStep := &engine.ExecutableWorkflowStep{
				BaseEntity: types.BaseEntity{
					ID:        "validate-business-rules",
					Name:      "Validate Business Rules",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
					Metadata:  map[string]interface{}{"compliance_level": "high"},
				},
				Type: engine.StepTypeAction,
				Action: &engine.StepAction{
					Type:       "validation",
					Parameters: map[string]interface{}{"compliance_level": "high"},
				},
				Timeout:   30 * time.Second,
				Variables: map[string]interface{}{"required": true},
			}

			testTemplate.Steps = []*engine.ExecutableWorkflowStep{validationStep}

			// Test workflow validation - business compliance capability
			startTime := time.Now()
			validationReport := workflowBuilder.ValidateWorkflow(ctx, testTemplate)
			duration := time.Since(startTime)

			// Business validation requirements
			Expect(validationReport).ToNot(BeNil(), "BR-WORKFLOW-INTEGRATION-001: Workflow validation must provide business compliance feedback")

			// Performance requirement: Validation must complete quickly for business efficiency
			Expect(duration).To(BeNumerically("<=", 5*time.Second), "BR-WORKFLOW-INTEGRATION-001: Workflow validation must complete within business efficiency requirements")

			logger.WithFields(logrus.Fields{
				"template_id":      testTemplate.ID,
				"validation_time":  duration,
				"compliance_check": true,
			}).Info("BR-WORKFLOW-INTEGRATION-001: Workflow validation completed successfully")
		})
	})

	Context("BR-WORKFLOW-INTEGRATION-002: End-to-End Business Operations", func() {
		It("should execute complete workflow lifecycle for business continuity", func() {
			// Business Requirement: Complete workflow lifecycle execution for business operations
			// Business Impact: Validates end-to-end business automation capabilities

			// Skip if slow tests are disabled
			if testConfig.SkipSlowTests {
				Skip("Slow integration tests disabled")
			}

			// Create business-critical alert scenario
			alertContext := types.AlertContext{
				Name:        "ProductionDatabaseConnection",
				Severity:    "critical",
				Description: "Production database connection failure requiring immediate business response",
				Labels: map[string]string{
					"business_impact": "critical",
					"service_tier":    "production",
					"escalation":      "immediate",
				},
			}

			// Test complete business workflow lifecycle
			startTime := time.Now()

			// Step 1: Generate workflow from business alert
			objective := &engine.WorkflowObjective{
				Description: "Resolve database connectivity issues to restore business operations",
				Priority:    5, // Critical business priority
				Constraints: map[string]interface{}{
					"business_sla": "5m",
					"cost_limit":   5000.0,
					"automation":   "full",
					"impact_level": "critical",
				},
			}

			workflow, err := workflowBuilder.GenerateWorkflow(ctx, objective)
			Expect(err).ToNot(HaveOccurred(), "BR-WORKFLOW-INTEGRATION-002: Business workflow generation must succeed for operational continuity")
			Expect(workflow).ToNot(BeNil(), "BR-WORKFLOW-INTEGRATION-002: Business workflow must be generated for incident response")

			// Step 2: Validate workflow for business compliance
			if workflow != nil {
				validationReport := workflowBuilder.ValidateWorkflow(ctx, workflow)
				Expect(validationReport).ToNot(BeNil(), "BR-WORKFLOW-INTEGRATION-002: Business workflow validation must ensure operational compliance")
			}

			duration := time.Since(startTime)

			// Business outcome validation: Complete lifecycle supports business operations
			Expect(duration).To(BeNumerically("<=", testConfig.TestTimeout), "BR-WORKFLOW-INTEGRATION-002: Complete workflow lifecycle must meet business SLA requirements")

			workflowIDForLog := "nil"
			if workflow != nil {
				workflowIDForLog = workflow.ID
			}

			logger.WithFields(logrus.Fields{
				"alert_severity":     alertContext.Severity,
				"workflow_id":        workflowIDForLog,
				"lifecycle_duration": duration,
				"business_impact":    "critical",
			}).Info("BR-WORKFLOW-INTEGRATION-002: Complete workflow lifecycle validation successful")
		})
	})
})
