//go:build unit
// +build unit

package workflow_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/pkg/integration/processor"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/sirupsen/logrus"
)

// TDD RED PHASE: Workflow Service Business Requirements Tests
// Following TDD methodology: Write failing tests FIRST that define business contract
// Business Requirements: BR-WORKFLOW-001 to BR-WORKFLOW-005 (Workflow Service)

var _ = Describe("TDD RED: Workflow Service Business Requirements", func() {
	var (
		// Mock ONLY external dependencies (Rule 03 compliance)
		mockLLMClient *mocks.MockLLMClient
		mockExecutor  *mockWorkflowExecutor
		mockLogger    *logrus.Logger

		// Use REAL business logic components
		workflowService *processor.EnhancedService
		workflowConfig  *processor.Config

		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 60*time.Second)

		// Mock ONLY external dependencies
		mockLLMClient = mocks.NewMockLLMClient()
		mockExecutor = &mockWorkflowExecutor{}
		mockLogger = logrus.New()
		mockLogger.SetLevel(logrus.ErrorLevel)

		// Configure AI client for testing
		mockLLMClient.SetHealthy(true)
		mockLLMClient.SetEndpoint("mock://ai-service")

		// Create REAL business configuration for workflow service
		workflowConfig = &processor.Config{
			ProcessorPort:           8083,
			AIServiceTimeout:        60 * time.Second,
			MaxConcurrentProcessing: 50,
			ProcessingTimeout:       600 * time.Second,
			AI: processor.AIConfig{
				Provider:            "holmesgpt",
				Endpoint:            "mock://ai-service",
				Model:               "hf://ggml-org/gpt-oss-20b-GGUF",
				Timeout:             60 * time.Second,
				MaxRetries:          3,
				ConfidenceThreshold: 0.7,
			},
		}

		// Create workflow service with REAL business logic
		// This MUST fail initially (TDD RED phase requirement)
		workflowService = processor.NewEnhancedService(
			mockLLMClient,  // External: AI service
			mockExecutor,   // External: K8s operations
			workflowConfig, // Real: Business configuration
		)
	})

	AfterEach(func() {
		cancel()
	})

	// BR-WORKFLOW-001: Workflow Creation and Management
	Context("BR-WORKFLOW-001: Workflow Creation and Management", func() {
		It("should create workflows from processed alerts", func() {
			// TDD RED: This test MUST fail - CreateWorkflow() method doesn't exist yet
			alert := types.Alert{
				Name:      "DatabaseAlert",
				Severity:  "critical",
				Status:    "firing",
				Namespace: "production",
				Labels: map[string]string{
					"component": "database",
				},
			}

			workflow := workflowService.CreateWorkflow(ctx, alert)
			Expect(workflow).To(HaveKey("workflow_id"))
			Expect(workflow).To(HaveKey("steps"))
			Expect(workflow).To(HaveKey("status"))
			Expect(workflow["status"]).To(Equal("created"))
		})

		It("should manage workflow lifecycle", func() {
			// TDD RED: This test MUST fail - workflow lifecycle methods don't exist yet
			workflowID := "test-workflow-123"

			// Start workflow
			startResult := workflowService.StartWorkflow(ctx, workflowID)
			Expect(startResult["status"]).To(Equal("running"))

			// Get workflow status
			status := workflowService.GetWorkflowStatus(workflowID)
			Expect(status).To(HaveKey("workflow_id"))
			Expect(status).To(HaveKey("current_step"))
			Expect(status).To(HaveKey("progress"))
		})
	})

	// BR-WORKFLOW-002: Action Execution Coordination
	Context("BR-WORKFLOW-002: Action Execution Coordination", func() {
		It("should coordinate multiple action executions", func() {
			// TDD RED: This test MUST fail - CoordinateActions() method doesn't exist yet
			alert := types.Alert{
				Name:      "MultiActionAlert",
				Severity:  "critical",
				Status:    "firing",
				Namespace: "production",
			}

			// Configure mock AI to recommend multiple actions
			mockLLMClient.SetAnalysisResponse(&mocks.AnalysisResponse{
				RecommendedAction: "restart-pod,scale-deployment",
				Confidence:        0.85,
				Reasoning:         "Pod failure with high load",
				ProcessingTime:    50 * time.Millisecond,
			})

			coordinationResult := workflowService.CoordinateActions(ctx, alert)
			Expect(coordinationResult).To(HaveKey("actions_scheduled"))
			Expect(coordinationResult).To(HaveKey("execution_order"))
			Expect(coordinationResult["actions_scheduled"]).To(BeNumerically(">=", 2))
		})

		It("should handle action execution failures", func() {
			// TDD RED: This test MUST fail - failure handling doesn't exist yet
			alert := types.Alert{
				Name:     "FailureAlert",
				Severity: "critical",
				Status:   "firing",
			}

			// Configure executor to fail
			mockExecutor.SetShouldFail(true)

			result, err := workflowService.ProcessAlert(ctx, alert)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKey("execution_failed"))
			Expect(result).To(HaveKey("rollback_initiated"))
		})
	})

	// BR-WORKFLOW-003: Workflow State Management
	Context("BR-WORKFLOW-003: Workflow State Management", func() {
		It("should persist workflow state", func() {
			// TDD RED: This test MUST fail - PersistWorkflowState() method doesn't exist yet
			workflowID := "state-test-workflow"
			state := map[string]interface{}{
				"current_step":      2,
				"completed_actions": []string{"restart-pod"},
				"pending_actions":   []string{"scale-deployment"},
			}

			persistResult := workflowService.PersistWorkflowState(workflowID, state)
			Expect(persistResult["persisted"]).To(BeTrue())
		})

		It("should restore workflow state after service restart", func() {
			// TDD RED: This test MUST fail - RestoreWorkflowState() method doesn't exist yet
			workflowID := "restore-test-workflow"

			restoredState := workflowService.RestoreWorkflowState(workflowID)
			Expect(restoredState).To(HaveKey("current_step"))
			Expect(restoredState).To(HaveKey("completed_actions"))
		})
	})

	// BR-WORKFLOW-004: Execution Monitoring
	Context("BR-WORKFLOW-004: Execution Monitoring", func() {
		It("should monitor workflow execution progress", func() {
			// TDD RED: This test MUST fail - MonitorExecution() method doesn't exist yet
			workflowID := "monitor-test-workflow"

			monitoring := workflowService.MonitorExecution(workflowID)
			Expect(monitoring).To(HaveKey("progress_percentage"))
			Expect(monitoring).To(HaveKey("current_action"))
			Expect(monitoring).To(HaveKey("estimated_completion"))
		})

		It("should provide execution metrics", func() {
			// TDD RED: This test MUST fail - GetExecutionMetrics() method doesn't exist yet
			metrics := workflowService.GetExecutionMetrics()
			Expect(metrics).To(HaveKey("active_workflows"))
			Expect(metrics).To(HaveKey("completed_workflows"))
			Expect(metrics).To(HaveKey("failed_workflows"))
			Expect(metrics).To(HaveKey("average_execution_time"))
		})
	})

	// BR-WORKFLOW-005: Rollback Capabilities
	Context("BR-WORKFLOW-005: Rollback Capabilities", func() {
		It("should rollback failed workflows", func() {
			// TDD RED: This test MUST fail - RollbackWorkflow() method doesn't exist yet
			workflowID := "rollback-test-workflow"

			rollbackResult := workflowService.RollbackWorkflow(ctx, workflowID)
			Expect(rollbackResult).To(HaveKey("rollback_initiated"))
			Expect(rollbackResult).To(HaveKey("rollback_steps"))
			Expect(rollbackResult["rollback_initiated"]).To(BeTrue())
		})

		It("should track rollback operations", func() {
			// TDD RED: This test MUST fail - GetRollbackHistory() method doesn't exist yet
			history := workflowService.GetRollbackHistory("production", 24*time.Hour)
			Expect(history).To(HaveKey("rollbacks"))
			Expect(history).To(HaveKey("success_rate"))
		})
	})

	// BR-WORKFLOW-006: Service Integration
	Context("BR-WORKFLOW-006: Workflow Service Integration", func() {
		It("should integrate with alert service for workflow triggers", func() {
			// TDD RED: This test MUST fail - alert service integration doesn't exist yet
			alert := types.Alert{
				Name:      "IntegrationAlert",
				Severity:  "critical",
				Status:    "firing",
				Namespace: "production",
			}

			integrationResult := workflowService.ProcessAlertFromService(ctx, alert, "alert-service")
			Expect(integrationResult).To(HaveKey("workflow_created"))
			Expect(integrationResult).To(HaveKey("source_service"))
			Expect(integrationResult["source_service"]).To(Equal("alert-service"))
		})
	})
})

// mockWorkflowExecutor implements executor.Executor for workflow service testing
type mockWorkflowExecutor struct {
	shouldFail bool
	executions []string
}

func (m *mockWorkflowExecutor) SetShouldFail(fail bool) {
	m.shouldFail = fail
}

func (m *mockWorkflowExecutor) Execute(ctx context.Context, action *types.ActionRecommendation, alert types.Alert, trace *actionhistory.ResourceActionTrace) error {
	if m.shouldFail {
		return fmt.Errorf("mock execution failure")
	}
	m.executions = append(m.executions, action.Action)
	return nil
}

func (m *mockWorkflowExecutor) IsHealthy() bool {
	return !m.shouldFail
}

func (m *mockWorkflowExecutor) GetActionRegistry() interface{} {
	return nil
}

func (m *mockWorkflowExecutor) GetExecutions() []string {
	return m.executions
}

func TestWorkflowServiceTDD(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workflow Service TDD Suite")
}
