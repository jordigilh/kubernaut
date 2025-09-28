//go:build unit
// +build unit

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/workflow"
)

func TestWorkflowService(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workflow Service Suite")
}

var _ = Describe("Workflow Orchestrator Service - TDD RED Phase", func() {
	var (
		server          *httptest.Server
		workflowService workflow.WorkflowService
		mockLLMClient   *MockLLMClient
		logger          *logrus.Logger
		cfg             *workflow.Config
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce noise in tests

		cfg = &workflow.Config{
			ServicePort:              8083,
			MaxConcurrentWorkflows:   10,
			WorkflowExecutionTimeout: 300 * time.Second,
		}

		mockLLMClient = &MockLLMClient{}
		workflowService = workflow.NewWorkflowService(mockLLMClient, cfg, logger)

		// Create test HTTP server
		handler := createWorkflowHTTPHandler(workflowService, logger)
		server = httptest.NewServer(handler)
	})

	AfterEach(func() {
		server.Close()
	})

	Context("BR-WORKFLOW-001: HTTP Service Configuration", func() {
		It("should respond to health check requests", func() {
			resp, err := http.Get(server.URL + "/health")
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var health map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&health)
			Expect(err).ToNot(HaveOccurred())
			Expect(health).To(HaveKey("status"))
		})

		It("should accept workflow execution requests on /api/v1/execute", func() {
			alert := types.Alert{
				Name:      "HighCPUUsage",
				Severity:  "critical",
				Namespace: "production",
				Resource:  "pod/web-server-123",
				Labels: map[string]string{
					"alertname": "HighCPUUsage",
					"severity":  "critical",
				},
			}

			alertPayload, _ := json.Marshal(alert)
			resp, err := http.Post(server.URL+"/api/v1/execute", "application/json", bytes.NewReader(alertPayload))
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			Expect(err).ToNot(HaveOccurred())
			Expect(response["status"]).To(Equal("success"))
			Expect(response["service"]).To(Equal("workflow-service"))
			Expect(response["result"]).ToNot(BeNil())
		})

		It("should reject non-POST requests to /api/v1/execute", func() {
			resp, err := http.Get(server.URL + "/api/v1/execute")
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusMethodNotAllowed))
		})

		It("should reject invalid JSON payloads", func() {
			resp, err := http.Post(server.URL+"/api/v1/execute", "application/json", bytes.NewReader([]byte("invalid json")))
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})
	})

	Context("BR-WORKFLOW-002: Service Coordination Flow", func() {
		It("should coordinate workflow execution from AI analysis to K8s execution", func() {
			// This test will FAIL initially - TDD RED phase
			// Expected flow: AI Service (8082) → Workflow Service (8083) → K8s Executor (8084)

			alert := types.Alert{
				Name:      "MemoryLeak",
				Severity:  "warning",
				Namespace: "staging",
				Resource:  "deployment/api-server",
				Labels: map[string]string{
					"alertname": "MemoryLeak",
					"pod":       "api-server-abc123",
				},
			}

			// Execute workflow - this should coordinate with downstream services
			result, err := workflowService.ProcessAlert(context.Background(), alert)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Success).To(BeTrue())
			Expect(result.WorkflowID).ToNot(BeEmpty())
			Expect(result.ActionsTotal).To(BeNumerically(">", 0))
			Expect(result.ActionsExecuted).To(Equal(result.ActionsTotal))

			// Verify service coordination occurred
			Expect(result.Status).To(Equal("completed"))
			Expect(result.ExecutionTime).To(BeNumerically(">", 0))
		})

		It("should handle AI service integration for workflow enhancement", func() {
			// This test will FAIL initially - TDD RED phase
			// Expected: Workflow service should use AI analysis to enhance workflows

			mockLLMClient.SetAnalysisResult(&llm.AnalysisResult{
				Confidence: 0.85,
				Actions: []llm.RecommendedAction{
					{
						Type:        "restart-pod",
						Confidence:  0.9,
						Parameters:  map[string]interface{}{"namespace": "staging", "pod": "api-server-abc123"},
						Reasoning:   "High confidence restart recommendation based on memory leak pattern",
						EstimatedDuration: 30 * time.Second,
					},
				},
			})

			alert := types.Alert{
				Name:      "MemoryLeak",
				Severity:  "warning",
				Namespace: "staging",
				Resource:  "deployment/api-server",
			}

			result, err := workflowService.ProcessAlert(context.Background(), alert)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Success).To(BeTrue())

			// Verify AI integration occurred
			Expect(mockLLMClient.AnalyzeCallCount()).To(BeNumerically(">", 0))
			Expect(result.ActionsTotal).To(BeNumerically(">", 0))
		})

		It("should coordinate with K8s executor service for action execution", func() {
			// This test will FAIL initially - TDD RED phase
			// Expected: Workflow service should send actions to K8s executor service

			alert := types.Alert{
				Name:      "DiskSpaceHigh",
				Severity:  "critical",
				Namespace: "production",
				Resource:  "pod/database-server-xyz789",
			}

			result, err := workflowService.ProcessAlert(context.Background(), alert)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Success).To(BeTrue())

			// Verify coordination with K8s executor occurred
			// This will fail until we implement service coordination
			Expect(result.ActionsExecuted).To(BeNumerically(">", 0))
			Expect(result.Status).To(Equal("completed"))
		})
	})

	Context("BR-WORKFLOW-003: Workflow State Management", func() {
		It("should manage workflow state throughout execution", func() {
			alert := types.Alert{
				Name:      "NetworkLatencyHigh",
				Severity:  "warning",
				Namespace: "production",
				Resource:  "service/frontend",
			}

			result, err := workflowService.ProcessAlert(context.Background(), alert)
			Expect(err).ToNot(HaveOccurred())

			// Verify state management
			Expect(result.WorkflowID).ToNot(BeEmpty())
			Expect(result.ExecutionID).ToNot(BeEmpty())
			Expect(result.Status).To(BeElementOf([]string{"completed", "failed", "partially_completed"}))
		})

		It("should handle workflow rollback on critical failures", func() {
			// This test will FAIL initially - need to implement rollback coordination
			alert := types.Alert{
				Name:      "CriticalSystemFailure",
				Severity:  "critical",
				Namespace: "production",
				Resource:  "deployment/core-service",
			}

			// Simulate critical failure scenario
			mockLLMClient.SetShouldFail(true)

			result, err := workflowService.ProcessAlert(context.Background(), alert)
			// Should not error, but should handle gracefully
			Expect(err).ToNot(HaveOccurred())

			// Verify rollback handling
			if !result.Success {
				Expect(result.Status).To(BeElementOf([]string{"failed", "rolled_back"}))
			}
		})
	})

	Context("BR-WORKFLOW-004: Execution Monitoring", func() {
		It("should monitor workflow execution progress", func() {
			alert := types.Alert{
				Name:      "ServiceUnavailable",
				Severity:  "critical",
				Namespace: "production",
				Resource:  "service/payment-api",
			}

			result, err := workflowService.ProcessAlert(context.Background(), alert)
			Expect(err).ToNot(HaveOccurred())

			// Verify monitoring data
			Expect(result.ExecutionTime).To(BeNumerically(">", 0))
			Expect(result.ActionsTotal).To(BeNumerically(">=", 0))
			Expect(result.ActionsExecuted).To(BeNumerically(">=", 0))
		})
	})

	Context("BR-WORKFLOW-005: Error Handling and Resilience", func() {
		It("should handle AI service unavailability gracefully", func() {
			mockLLMClient.SetShouldFail(true)

			alert := types.Alert{
				Name:      "TestAlert",
				Severity:  "warning",
				Namespace: "test",
				Resource:  "pod/test-pod",
			}

			result, err := workflowService.ProcessAlert(context.Background(), alert)
			Expect(err).ToNot(HaveOccurred()) // Should not error, should fallback

			// Should use fallback workflow generation
			Expect(result.WorkflowID).ToNot(BeEmpty())
		})

		It("should handle partial action execution failures", func() {
			// This test will help define resilient behavior
			alert := types.Alert{
				Name:      "PartialFailureScenario",
				Severity:  "warning",
				Namespace: "staging",
				Resource:  "deployment/test-service",
			}

			result, err := workflowService.ProcessAlert(context.Background(), alert)
			Expect(err).ToNot(HaveOccurred())

			// Should handle partial failures gracefully
			if result.ActionsExecuted < result.ActionsTotal {
				Expect(result.Status).To(BeElementOf([]string{"partially_completed", "completed"}))
			}
		})
	})
})

// MockLLMClient for testing AI integration
type MockLLMClient struct {
	analysisResult *llm.AnalysisResult
	shouldFail     bool
	callCount      int
}

func (m *MockLLMClient) SetAnalysisResult(result *llm.AnalysisResult) {
	m.analysisResult = result
}

func (m *MockLLMClient) SetShouldFail(fail bool) {
	m.shouldFail = fail
}

func (m *MockLLMClient) AnalyzeCallCount() int {
	return m.callCount
}

// Implement llm.Client interface methods
func (m *MockLLMClient) AnalyzeAlert(ctx context.Context, alert types.Alert) (*llm.AnalysisResult, error) {
	m.callCount++
	if m.shouldFail {
		return nil, fmt.Errorf("mock AI service failure")
	}
	if m.analysisResult != nil {
		return m.analysisResult, nil
	}
	// Default mock response
	return &llm.AnalysisResult{
		Confidence: 0.8,
		Actions: []llm.RecommendedAction{
			{
				Type:       "restart-pod",
				Confidence: 0.8,
				Parameters: map[string]interface{}{"namespace": alert.Namespace},
				Reasoning:  "Default mock action",
			},
		},
	}, nil
}

func (m *MockLLMClient) GenerateWorkflow(ctx context.Context, alert types.Alert) (*llm.WorkflowResult, error) {
	if m.shouldFail {
		return nil, fmt.Errorf("mock workflow generation failure")
	}
	return &llm.WorkflowResult{
		WorkflowID: "mock-workflow-123",
		Steps: []llm.WorkflowStep{
			{
				ID:          "step-1",
				Action:      "restart-pod",
				Parameters:  map[string]interface{}{"namespace": alert.Namespace},
				Description: "Mock workflow step",
			},
		},
	}, nil
}

func (m *MockLLMClient) OptimizeWorkflow(ctx context.Context, workflow *llm.WorkflowResult) (*llm.WorkflowResult, error) {
	if m.shouldFail {
		return nil, fmt.Errorf("mock optimization failure")
	}
	return workflow, nil
}
