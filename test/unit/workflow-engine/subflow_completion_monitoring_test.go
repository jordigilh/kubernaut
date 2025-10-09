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

package workflowengine

import (
	"testing"
	"context"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// Business Requirement: BR-WF-ADV-628 - Subflow Completion Monitoring Tests
// Following TDD methodology: Write failing tests first, then implement to pass
var _ = Describe("BR-WF-ADV-628: Subflow Completion Monitoring", func() {
	var (
		workflowEngine    *engine.DefaultWorkflowEngine
		mockExecutionRepo *SubflowMockExecutionRepository
		mockMetrics       *MockSubflowMetricsCollector
		logger            *logrus.Logger
		ctx               context.Context
		cancel            context.CancelFunc
		testExecutionID   string
		testExecution     *engine.RuntimeWorkflowExecution
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce test noise
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)

		testExecutionID = "test-subflow-execution-123"

		// Create mock execution repository
		mockExecutionRepo = NewSubflowMockExecutionRepository()

		// Create mock metrics collector
		mockMetrics = &MockSubflowMetricsCollector{}

		// Create workflow engine with mocked dependencies
		workflowEngine = createTestWorkflowEngineWithExecutionRepo(mockExecutionRepo, logger)
		workflowEngine.SetMetricsCollector(mockMetrics)

		// Create test execution
		testExecution = createTestRuntimeWorkflowExecution(testExecutionID)
	})

	AfterEach(func() {
		cancel()
	})

	Describe("Input Validation", func() {
		Context("when execution ID is empty", func() {
			It("should return validation error immediately", func() {
				// BR-WF-ADV-628: Input validation per project guidelines
				execution, err := workflowEngine.WaitForSubflowCompletion(ctx, "", 5*time.Minute)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("execution ID cannot be empty"))
				Expect(execution).To(BeNil())
			})
		})

		Context("when timeout is zero or negative", func() {
			It("should return validation error for zero timeout", func() {
				execution, err := workflowEngine.WaitForSubflowCompletion(ctx, testExecutionID, 0)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("timeout must be positive"))
				Expect(execution).To(BeNil())
			})

			It("should return validation error for negative timeout", func() {
				execution, err := workflowEngine.WaitForSubflowCompletion(ctx, testExecutionID, -5*time.Minute)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("timeout must be positive"))
				Expect(execution).To(BeNil())
			})
		})

		Context("when execution repository is nil", func() {
			It("should return repository unavailable error", func() {
				// Create engine without execution repository
				engineWithoutRepo := createTestWorkflowEngineWithoutExecutionRepo(logger)

				execution, err := engineWithoutRepo.WaitForSubflowCompletion(ctx, testExecutionID, 5*time.Minute)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("execution repository not available"))
				Expect(execution).To(BeNil())
			})
		})
	})

	Describe("Successful Completion Scenarios", func() {
		Context("when subflow completes immediately", func() {
			It("should return completed execution on first poll", func() {
				// Setup: execution is already completed
				testExecution.OperationalStatus = engine.ExecutionStatusCompleted
				mockExecutionRepo.SetGetExecutionResponse(testExecution, nil)

				startTime := time.Now()
				execution, err := workflowEngine.WaitForSubflowCompletion(ctx, testExecutionID, 5*time.Minute)
				duration := time.Since(startTime)

				Expect(err).ToNot(HaveOccurred())
				Expect(execution).To(Equal(testExecution))
				Expect(duration).To(BeNumerically("<", 2*time.Second)) // Should be very fast

				// Verify metrics were recorded
				Expect(mockMetrics.SubflowMonitoringCalls).To(HaveLen(1))
				Expect(mockMetrics.SubflowMonitoringCalls[0].Success).To(BeTrue())
			})
		})

		Context("when subflow completes after several polls", func() {
			It("should return completed execution after monitoring", func() {
				// Setup: execution starts running, then completes
				runningExecution := createTestRuntimeWorkflowExecution(testExecutionID)
				runningExecution.OperationalStatus = engine.ExecutionStatusRunning

				completedExecution := createTestRuntimeWorkflowExecution(testExecutionID)
				completedExecution.OperationalStatus = engine.ExecutionStatusCompleted

				// First few calls return running, then completed
				mockExecutionRepo.SetSequentialGetExecutionResponses([]GetExecutionResponse{
					{Execution: runningExecution, Error: nil},
					{Execution: runningExecution, Error: nil},
					{Execution: completedExecution, Error: nil},
				})

				startTime := time.Now()
				execution, err := workflowEngine.WaitForSubflowCompletion(ctx, testExecutionID, 5*time.Minute)
				duration := time.Since(startTime)

				Expect(err).ToNot(HaveOccurred())
				Expect(execution).To(Equal(completedExecution))
				Expect(duration).To(BeNumerically(">=", 1*time.Second)) // Should take some time

				// Verify metrics were recorded
				Expect(mockMetrics.SubflowMonitoringCalls).To(HaveLen(1))
				Expect(mockMetrics.SubflowMonitoringCalls[0].Success).To(BeTrue())
			})
		})

		Context("when subflow fails", func() {
			It("should return failed execution", func() {
				// Setup: execution fails
				testExecution.OperationalStatus = engine.ExecutionStatusFailed
				mockExecutionRepo.SetGetExecutionResponse(testExecution, nil)

				execution, err := workflowEngine.WaitForSubflowCompletion(ctx, testExecutionID, 5*time.Minute)

				Expect(err).ToNot(HaveOccurred())
				Expect(execution).To(Equal(testExecution))
				Expect(execution.OperationalStatus).To(Equal(engine.ExecutionStatusFailed))

				// Verify metrics recorded failure
				Expect(mockMetrics.SubflowMonitoringCalls).To(HaveLen(1))
				Expect(mockMetrics.SubflowMonitoringCalls[0].Success).To(BeFalse())
			})
		})
	})

	Describe("Timeout Scenarios", func() {
		Context("when timeout is exceeded", func() {
			It("should return timeout error", func() {
				// Setup: execution never completes
				runningExecution := createTestRuntimeWorkflowExecution(testExecutionID)
				runningExecution.OperationalStatus = engine.ExecutionStatusRunning
				mockExecutionRepo.SetGetExecutionResponse(runningExecution, nil)

				shortTimeout := 100 * time.Millisecond
				startTime := time.Now()
				execution, err := workflowEngine.WaitForSubflowCompletion(ctx, testExecutionID, shortTimeout)
				duration := time.Since(startTime)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("subflow completion timeout"))
				Expect(execution).To(BeNil())
				Expect(duration).To(BeNumerically(">=", shortTimeout))
			})
		})

		Context("when parent context is cancelled", func() {
			It("should return context cancellation error", func() {
				// Setup: execution never completes
				runningExecution := createTestRuntimeWorkflowExecution(testExecutionID)
				runningExecution.OperationalStatus = engine.ExecutionStatusRunning
				mockExecutionRepo.SetGetExecutionResponse(runningExecution, nil)

				// Cancel context after short delay
				go func() {
					time.Sleep(50 * time.Millisecond)
					cancel()
				}()

				execution, err := workflowEngine.WaitForSubflowCompletion(ctx, testExecutionID, 5*time.Minute)

				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(context.Canceled))
				Expect(execution).To(BeNil())
			})
		})
	})

	Describe("Circuit Breaker and Error Handling", func() {
		Context("when repository calls fail temporarily", func() {
			It("should continue polling and eventually succeed", func() {
				// Setup: first few calls fail, then succeed
				completedExecution := createTestRuntimeWorkflowExecution(testExecutionID)
				completedExecution.OperationalStatus = engine.ExecutionStatusCompleted

				mockExecutionRepo.SetSequentialGetExecutionResponses([]GetExecutionResponse{
					{Execution: nil, Error: errors.New("temporary error")},
					{Execution: nil, Error: errors.New("temporary error")},
					{Execution: completedExecution, Error: nil},
				})

				execution, err := workflowEngine.WaitForSubflowCompletion(ctx, testExecutionID, 5*time.Minute)

				Expect(err).ToNot(HaveOccurred())
				Expect(execution).To(Equal(completedExecution))

				// Verify metrics were recorded
				Expect(mockMetrics.SubflowMonitoringCalls).To(HaveLen(1))
				Expect(mockMetrics.SubflowMonitoringCalls[0].Success).To(BeTrue())
			})
		})

		Context("when repository calls fail repeatedly", func() {
			It("should activate circuit breaker and use exponential backoff", func() {
				// Setup: many consecutive failures, then success
				completedExecution := createTestRuntimeWorkflowExecution(testExecutionID)
				completedExecution.OperationalStatus = engine.ExecutionStatusCompleted

				// Create 7 failures (more than maxConsecutiveFailures = 5) then success
				responses := make([]GetExecutionResponse, 8)
				for i := 0; i < 7; i++ {
					responses[i] = GetExecutionResponse{Execution: nil, Error: errors.New("persistent error")}
				}
				responses[7] = GetExecutionResponse{Execution: completedExecution, Error: nil}

				mockExecutionRepo.SetSequentialGetExecutionResponses(responses)

				startTime := time.Now()
				execution, err := workflowEngine.WaitForSubflowCompletion(ctx, testExecutionID, 30*time.Second)
				duration := time.Since(startTime)

				Expect(err).ToNot(HaveOccurred())
				Expect(execution).To(Equal(completedExecution))

				// Should take longer due to circuit breaker backoff
				Expect(duration).To(BeNumerically(">=", 1*time.Second))

				// Verify circuit breaker metrics were recorded
				Expect(mockMetrics.CircuitBreakerCalls).To(HaveLen(1))
				Expect(mockMetrics.CircuitBreakerCalls[0].ConsecutiveFailures).To(BeNumerically(">=", 5))
			})
		})
	})

	Describe("Performance Requirements (BR-WF-ADV-628)", func() {
		Context("status update latency", func() {
			It("should detect completion within 1 second for real-time monitoring", func() {
				// Setup: execution completes immediately
				testExecution.OperationalStatus = engine.ExecutionStatusCompleted
				mockExecutionRepo.SetGetExecutionResponse(testExecution, nil)

				startTime := time.Now()
				execution, err := workflowEngine.WaitForSubflowCompletion(ctx, testExecutionID, 5*time.Minute)
				latency := time.Since(startTime)

				Expect(err).ToNot(HaveOccurred())
				Expect(execution).ToNot(BeNil())
				Expect(latency).To(BeNumerically("<", 1*time.Second)) // BR-WF-ADV-628 requirement
			})
		})

		Context("resource optimization", func() {
			It("should use efficient polling intervals", func() {
				// Setup: execution never completes, test polling behavior
				runningExecution := createTestRuntimeWorkflowExecution(testExecutionID)
				runningExecution.OperationalStatus = engine.ExecutionStatusRunning
				mockExecutionRepo.SetGetExecutionResponse(runningExecution, nil)

				// Use short timeout to test polling frequency
				shortTimeout := 2 * time.Second
				startTime := time.Now()
				_, err := workflowEngine.WaitForSubflowCompletion(ctx, testExecutionID, shortTimeout)
				duration := time.Since(startTime)

				Expect(err).To(HaveOccurred()) // Should timeout
				Expect(duration).To(BeNumerically(">=", shortTimeout))

				// Verify reasonable number of repository calls (not excessive)
				callCount := mockExecutionRepo.GetExecutionCallCount()
				expectedMaxCalls := int(shortTimeout/time.Millisecond/100) + 2 // Allow some variance
				Expect(callCount).To(BeNumerically("<=", expectedMaxCalls))
			})
		})
	})

	Describe("Progress Reporting", func() {
		Context("when monitoring long-running subflows", func() {
			It("should record progress metrics periodically", func() {
				// Setup: long-running execution with progress
				longRunningExecution := createTestRuntimeWorkflowExecutionWithSteps(testExecutionID, 10)
				longRunningExecution.OperationalStatus = engine.ExecutionStatusRunning
				longRunningExecution.StartTime = time.Now().Add(-2 * time.Minute) // Started 2 minutes ago

				// Set some steps as completed
				for i := 0; i < 5; i++ {
					longRunningExecution.Steps[i].Status = engine.ExecutionStatusCompleted
				}

				completedExecution := createTestRuntimeWorkflowExecution(testExecutionID)
				completedExecution.OperationalStatus = engine.ExecutionStatusCompleted

				// Return long-running for several calls, then completed
				mockExecutionRepo.SetSequentialGetExecutionResponses([]GetExecutionResponse{
					{Execution: longRunningExecution, Error: nil},
					{Execution: longRunningExecution, Error: nil},
					{Execution: longRunningExecution, Error: nil},
					{Execution: completedExecution, Error: nil},
				})

				execution, err := workflowEngine.WaitForSubflowCompletion(ctx, testExecutionID, 5*time.Minute)

				Expect(err).ToNot(HaveOccurred())
				Expect(execution).To(Equal(completedExecution))

				// Verify progress metrics were recorded
				Expect(mockMetrics.SubflowProgressCalls).To(HaveLen(1))
				progressCall := mockMetrics.SubflowProgressCalls[0]
				Expect(progressCall.ProgressPercent).To(BeNumerically("==", 50.0)) // 5/10 steps completed
			})
		})
	})
})

// Mock SubflowMetricsCollector for testing
type MockSubflowMetricsCollector struct {
	SubflowMonitoringCalls []SubflowMonitoringCall
	SubflowProgressCalls   []SubflowProgressCall
	CircuitBreakerCalls    []CircuitBreakerCall
}

type SubflowMonitoringCall struct {
	ExecutionID        string
	MonitoringDuration time.Duration
	ExecutionDuration  time.Duration
	Success            bool
}

type SubflowProgressCall struct {
	ExecutionID        string
	ProgressPercent    float64
	MonitoringDuration time.Duration
}

type CircuitBreakerCall struct {
	ExecutionID         string
	ConsecutiveFailures int
	BackoffDuration     time.Duration
}

func (m *MockSubflowMetricsCollector) RecordSubflowMonitoring(executionID string, monitoringDuration, executionDuration time.Duration, success bool) {
	m.SubflowMonitoringCalls = append(m.SubflowMonitoringCalls, SubflowMonitoringCall{
		ExecutionID:        executionID,
		MonitoringDuration: monitoringDuration,
		ExecutionDuration:  executionDuration,
		Success:            success,
	})
}

func (m *MockSubflowMetricsCollector) RecordSubflowProgress(executionID string, progressPercent float64, monitoringDuration time.Duration) {
	m.SubflowProgressCalls = append(m.SubflowProgressCalls, SubflowProgressCall{
		ExecutionID:        executionID,
		ProgressPercent:    progressPercent,
		MonitoringDuration: monitoringDuration,
	})
}

func (m *MockSubflowMetricsCollector) RecordCircuitBreakerActivation(executionID string, consecutiveFailures int, backoffDuration time.Duration) {
	m.CircuitBreakerCalls = append(m.CircuitBreakerCalls, CircuitBreakerCall{
		ExecutionID:         executionID,
		ConsecutiveFailures: consecutiveFailures,
		BackoffDuration:     backoffDuration,
	})
}

// Helper functions for test setup
func createTestWorkflowEngineWithExecutionRepo(executionRepo *SubflowMockExecutionRepository, logger *logrus.Logger) *engine.DefaultWorkflowEngine {
	config := &engine.WorkflowEngineConfig{
		DefaultStepTimeout:    10 * time.Minute,
		MaxRetryDelay:         5 * time.Minute,
		EnableStateRecovery:   true,
		EnableDetailedLogging: false,
		MaxConcurrency:        10,
	}

	mockK8sClient := &mocks.MockKubernetesClient{}
	mockActionRepo := &mocks.MockActionRepository{}
	mockStateStorage := &mocks.MockStateStorage{}

	return engine.NewDefaultWorkflowEngine(
		mockK8sClient,
		mockActionRepo,
		nil, // monitoring clients
		mockStateStorage,
		executionRepo,
		config,
		logger,
	)
}

func createTestWorkflowEngineWithoutExecutionRepo(logger *logrus.Logger) *engine.DefaultWorkflowEngine {
	return createTestWorkflowEngineWithExecutionRepo(nil, logger)
}

func createTestRuntimeWorkflowExecution(executionID string) *engine.RuntimeWorkflowExecution {
	execution := engine.NewRuntimeWorkflowExecution(executionID, "test-workflow")
	execution.OperationalStatus = engine.ExecutionStatusPending
	return execution
}

func createTestRuntimeWorkflowExecutionWithSteps(executionID string, stepCount int) *engine.RuntimeWorkflowExecution {
	execution := createTestRuntimeWorkflowExecution(executionID)

	for i := 0; i < stepCount; i++ {
		step := &engine.StepExecution{
			StepID:    fmt.Sprintf("step-%d", i),
			Status:    engine.ExecutionStatusPending,
			StartTime: time.Now(),
			Variables: make(map[string]interface{}),
			Metadata:  make(map[string]interface{}),
		}
		execution.Steps = append(execution.Steps, step)
	}

	return execution
}

// GetExecutionResponse represents a response from GetExecution call
type GetExecutionResponse struct {
	Execution *engine.RuntimeWorkflowExecution
	Error     error
}

// SubflowMockExecutionRepository provides a custom mock for testing subflow completion monitoring
type SubflowMockExecutionRepository struct {
	responses         []GetExecutionResponse
	currentIndex      int
	singleResponse    *GetExecutionResponse
	callCount         int
	useSingleResponse bool
}

func NewSubflowMockExecutionRepository() *SubflowMockExecutionRepository {
	return &SubflowMockExecutionRepository{
		responses:         make([]GetExecutionResponse, 0),
		currentIndex:      0,
		callCount:         0,
		useSingleResponse: false,
	}
}

func (m *SubflowMockExecutionRepository) SetGetExecutionResponse(execution *engine.RuntimeWorkflowExecution, err error) {
	m.singleResponse = &GetExecutionResponse{
		Execution: execution,
		Error:     err,
	}
	m.useSingleResponse = true
}

func (m *SubflowMockExecutionRepository) SetSequentialGetExecutionResponses(responses []GetExecutionResponse) {
	m.responses = responses
	m.currentIndex = 0
	m.useSingleResponse = false
}

func (m *SubflowMockExecutionRepository) GetExecutionCallCount() int {
	return m.callCount
}

// ExecutionRepository interface implementation
func (m *SubflowMockExecutionRepository) GetExecution(ctx context.Context, executionID string) (*engine.RuntimeWorkflowExecution, error) {
	m.callCount++

	if m.useSingleResponse && m.singleResponse != nil {
		return m.singleResponse.Execution, m.singleResponse.Error
	}

	if len(m.responses) == 0 {
		return nil, errors.New("no responses configured")
	}

	if m.currentIndex >= len(m.responses) {
		// Return the last response for subsequent calls
		lastResponse := m.responses[len(m.responses)-1]
		return lastResponse.Execution, lastResponse.Error
	}

	response := m.responses[m.currentIndex]
	m.currentIndex++
	return response.Execution, response.Error
}

func (m *SubflowMockExecutionRepository) StoreExecution(ctx context.Context, execution *engine.RuntimeWorkflowExecution) error {
	return nil // Not used in these tests
}

func (m *SubflowMockExecutionRepository) UpdateExecution(ctx context.Context, execution *engine.RuntimeWorkflowExecution) error {
	return nil // Not used in these tests
}

func (m *SubflowMockExecutionRepository) DeleteExecution(ctx context.Context, executionID string) error {
	return nil // Not used in these tests
}

func (m *SubflowMockExecutionRepository) GetExecutionsByWorkflowID(ctx context.Context, workflowID string) ([]*engine.RuntimeWorkflowExecution, error) {
	return nil, nil // Not used in these tests
}

func (m *SubflowMockExecutionRepository) GetExecutionsByPattern(ctx context.Context, pattern string) ([]*engine.RuntimeWorkflowExecution, error) {
	return nil, nil // Not used in these tests
}

func (m *SubflowMockExecutionRepository) GetExecutionsInTimeWindow(ctx context.Context, start, end time.Time) ([]*engine.RuntimeWorkflowExecution, error) {
	return nil, nil // Not used in these tests
}

// TestRunner bootstraps the Ginkgo test suite
func TestUsubflowUcompletionUmonitoring(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UsubflowUcompletionUmonitoring Suite")
}
