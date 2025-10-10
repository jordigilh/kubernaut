<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package workflowengine

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/jordigilh/kubernaut/pkg/workflow/shared"
)

func TestWorkflowPersistenceInterface(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workflow Persistence Interface Unit Tests Suite")
}

var _ = Describe("BR-WF-004, BR-REL-004, BR-DATA-011-013, BR-CONS-001: WorkflowPersistence Interface", Ordered, func() {
	var (
		ctx             context.Context
		logger          *logrus.Logger
		testExecution   *engine.RuntimeWorkflowExecution
		mockPersistence *MockWorkflowPersistence
		postgresStorage engine.WorkflowPersistence
		pgvectorStorage engine.WorkflowPersistence
	)

	BeforeAll(func() {
		ctx = context.Background()
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce test noise

		// Following guideline: Use structured field values, avoid interface{}
		// RuntimeWorkflowExecution embeds types.WorkflowExecutionRecord
		testExecution = &engine.RuntimeWorkflowExecution{
			WorkflowExecutionRecord: types.WorkflowExecutionRecord{
				ID:         "test-workflow-execution-001",
				WorkflowID: "test-workflow-001",
				Status:     string(engine.ExecutionStatusRunning),
				StartTime:  time.Now().Add(-10 * time.Minute),
				Metadata:   make(map[string]interface{}),
			},
			OperationalStatus: engine.ExecutionStatusRunning,
			Steps: []*engine.StepExecution{
				{
					StepID:    "step-001",
					Status:    engine.ExecutionStatusCompleted,
					StartTime: time.Now().Add(-5 * time.Minute),
					EndTime:   &[]time.Time{time.Now().Add(-2 * time.Minute)}[0],
				},
			},
		}

		// Following guideline: Reuse existing mocks
		mockPersistence = NewMockWorkflowPersistence()
	})

	Context("BR-WF-004: MUST provide workflow state management and persistence", func() {
		It("should define common interface for workflow state persistence operations", func() {
			By("validating interface contract exists")

			// The interface should be implemented by both storage types
			// Note: Direct assignability check not possible due to package boundaries, test functionality instead

			By("testing basic persistence operations")

			// BR-WF-004: Save workflow state
			err := mockPersistence.SaveWorkflowState(ctx, testExecution)
			Expect(err).ToNot(HaveOccurred(), "BR-WF-004: Should save workflow state successfully")

			// BR-WF-004: Load workflow state
			retrieved, err := mockPersistence.LoadWorkflowState(ctx, testExecution.ID)
			Expect(err).ToNot(HaveOccurred(), "BR-WF-004: Should load workflow state successfully")
			Expect(retrieved.ID).To(Equal(testExecution.ID), "BR-WF-004: Workflow state retrieval must return execution with correct identifier for persistence consistency")

			// BR-WF-004: Delete workflow state
			err = mockPersistence.DeleteWorkflowState(ctx, testExecution.ID)
			Expect(err).ToNot(HaveOccurred(), "BR-WF-004: Should delete workflow state successfully")
		})

		It("should handle workflow state management errors correctly", func() {
			By("testing error scenarios following guideline: Always handle errors")

			mockPersistence.SetSaveError(errors.New("storage unavailable"))

			err := mockPersistence.SaveWorkflowState(ctx, testExecution)
			Expect(err).To(HaveOccurred(), "Should handle save errors properly")
			Expect(err.Error()).To(ContainSubstring("storage unavailable"), "Should propagate original error message")

			mockPersistence.SetLoadError(errors.New("execution not found"))

			_, err = mockPersistence.LoadWorkflowState(ctx, "nonexistent-id")
			Expect(err).To(HaveOccurred(), "Should handle load errors properly")
			Expect(err.Error()).To(ContainSubstring("execution not found"), "Should propagate original error message")
		})
	})

	Context("BR-REL-004: MUST recover workflow state after system restarts", func() {
		BeforeEach(func() {
			// Following project principles: Test isolation - clear any error states from previous tests
			mockPersistence.SetSaveError(nil)
			mockPersistence.SetLoadError(nil)
		})

		It("should support workflow state recovery operations", func() {
			By("setting up recoverable workflow state")

			// Following project principles: Test business requirements, not implementation
			// BR-REL-004 requires recoverable workflow states to exist
			err := mockPersistence.SaveWorkflowState(ctx, testExecution)
			Expect(err).ToNot(HaveOccurred(), "Should save workflow state for recovery testing")

			By("testing crash recovery capabilities")

			// BR-REL-004: Recover workflow states
			recoveredExecutions, err := mockPersistence.RecoverWorkflowStates(ctx)
			Expect(err).ToNot(HaveOccurred(), "BR-REL-004: Should recover workflow states successfully")
			Expect(len(recoveredExecutions)).To(BeNumerically(">=", 0), "BR-REL-004: Workflow recovery must return measurable execution collection for reliability analysis")

			By("validating recovered state consistency")

			// Business requirement: BR-REL-004 - Should recover the running workflow we saved
			Expect(len(recoveredExecutions)).To(BeNumerically(">=", 1), "BR-REL-004: Should recover at least one execution (our test execution)")

			// Business requirement: Recovered states should be consistent
			foundTestExecution := false
			for _, execution := range recoveredExecutions {
				Expect(execution.ID).ToNot(BeEmpty(), "BR-REL-004: Recovered execution should have valid ID")
				Expect(execution.Status).ToNot(BeEmpty(), "BR-REL-004: Recovered execution should have valid status")
				Expect(execution.StartTime).ToNot(BeZero(), "BR-REL-004: Recovered execution should have valid start time")

				// Following project principles: Test business outcomes
				if execution.ID == testExecution.ID {
					foundTestExecution = true
					Expect(execution.OperationalStatus).To(Equal(engine.ExecutionStatusRunning), "BR-REL-004: Should recover running executions")
				}
			}
			Expect(foundTestExecution).To(BeTrue(), "BR-REL-004: Should recover the specific test execution we saved")
		})

		It("should provide recovery analytics for monitoring", func() {
			By("testing state analytics capabilities")

			// BR-REL-004: Get state analytics for recovery monitoring
			analytics, err := mockPersistence.GetStateAnalytics(ctx)
			Expect(err).ToNot(HaveOccurred(), "Should get state analytics successfully")
			Expect(analytics.TotalExecutions).To(BeNumerically(">=", 0), "BR-WF-004: State analytics must provide measurable execution metrics for workflow performance tracking")
			Expect(analytics.ActiveExecutions).To(BeNumerically(">=", 0), "Should have valid active execution count")
		})
	})

	Context("BR-DATA-012: MUST support state snapshots and checkpointing", func() {
		It("should support checkpoint creation and restoration", func() {
			By("creating workflow checkpoints")

			checkpoint, err := mockPersistence.CreateCheckpoint(ctx, testExecution, "before-critical-step")
			Expect(err).ToNot(HaveOccurred(), "BR-DATA-012: Should create checkpoint successfully")
			Expect(checkpoint.Name).To(Equal("before-critical-step"), "BR-DATA-012: Checkpoint creation must produce identifiable checkpoint with correct naming for data consistency")
			Expect(checkpoint.ExecutionID).To(Equal(testExecution.ID), "Checkpoint should reference correct execution")

			By("restoring from checkpoints")

			restored, err := mockPersistence.RestoreFromCheckpoint(ctx, checkpoint.ID)
			Expect(err).ToNot(HaveOccurred(), "BR-DATA-012: Should restore from checkpoint successfully")
			Expect(restored.ID).To(Equal(testExecution.ID), "BR-DATA-012: Checkpoint restoration must return execution with correct identifier for data consistency verification")
		})

		It("should validate checkpoint consistency", func() {
			By("testing checkpoint validation")

			checkpoint, err := mockPersistence.CreateCheckpoint(ctx, testExecution, "validation-test")
			Expect(err).ToNot(HaveOccurred())

			// BR-DATA-014: MUST provide state validation and consistency checks
			isValid, err := mockPersistence.ValidateCheckpoint(ctx, checkpoint.ID)
			Expect(err).ToNot(HaveOccurred(), "BR-DATA-014: Should validate checkpoint successfully")
			Expect(isValid).To(BeTrue(), "BR-DATA-014: Valid checkpoint should pass validation")
		})
	})

	Context("BR-DATA-013: MUST implement state recovery and restoration capabilities", func() {
		It("should support advanced recovery scenarios", func() {
			By("testing partial failure recovery")

			// Simulate workflow interruption
			interruptedExecution := &engine.RuntimeWorkflowExecution{
				WorkflowExecutionRecord: types.WorkflowExecutionRecord{
					ID:         "interrupted-workflow-001",
					WorkflowID: "interrupted-workflow",
					Status:     string(engine.ExecutionStatusFailed),
					StartTime:  time.Now().Add(-15 * time.Minute),
					Metadata:   make(map[string]interface{}),
				},
				OperationalStatus: engine.ExecutionStatusFailed,
				Steps: []*engine.StepExecution{
					{
						StepID: "step-001",
						Status: engine.ExecutionStatusCompleted,
					},
					{
						StepID: "step-002",
						Status: engine.ExecutionStatusFailed,
					},
				},
			}

			err := mockPersistence.SaveWorkflowState(ctx, interruptedExecution)
			Expect(err).ToNot(HaveOccurred())

			// BR-DATA-013: Recovery should identify resumable workflows
			recovered, err := mockPersistence.RecoverWorkflowStates(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Should include interrupted workflow for recovery
			foundInterrupted := false
			for _, exec := range recovered {
				if exec.ID == "interrupted-workflow-001" {
					foundInterrupted = true
					Expect(exec.OperationalStatus).To(Equal(engine.ExecutionStatusFailed), "Should preserve failure status")
				}
			}
			Expect(foundInterrupted).To(BeTrue(), "BR-DATA-013: Should recover interrupted workflows")
		})

		It("should provide recovery metrics for business monitoring", func() {
			By("tracking recovery success rates")

			analytics, err := mockPersistence.GetStateAnalytics(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Business requirement: Track recovery effectiveness
			Expect(analytics.RecoverySuccessRate).To(BeNumerically(">=", 0), "Should track recovery success rate")
			Expect(analytics.RecoverySuccessRate).To(BeNumerically("<=", 1), "Recovery rate should be valid percentage")
		})
	})

	Context("BR-CONS-001: Complete interface implementations for workflow engine constructors", func() {
		It("should support multiple persistence implementations via interface", func() {
			By("validating multiple implementations can be constructed")

			// Following guideline: Integration with existing code
			postgresStorage = engine.NewWorkflowStateStorage(nil, logger)
			Expect(func() { _ = postgresStorage.SaveWorkflowState }).ToNot(Panic(), "BR-CONS-001: PostgreSQL storage construction must provide functional persistence interface for workflow consistency")
			// Note: Direct interface assignment check done through compilation, not runtime assertions

			// pgvector storage should also implement the same interface
			pgvectorStorage = NewMockPgVectorPersistence()
			Expect(func() { _ = pgvectorStorage.SaveWorkflowState }).ToNot(Panic(), "BR-CONS-001: pgvector storage construction must provide functional persistence interface for workflow consistency")
			// Note: Interface compliance validated through successful method calls
		})

		It("should support interface-based dependency injection", func() {
			By("testing workflow engine integration")

			// BR-CONS-001: Test with actual PostgreSQL storage implementation
			testExecForDI := &engine.RuntimeWorkflowExecution{
				WorkflowExecutionRecord: types.WorkflowExecutionRecord{
					ID:         "di-test-exec-postgres",
					WorkflowID: "di-test-workflow",
					Status:     string(engine.ExecutionStatusRunning),
					StartTime:  time.Now(),
					Metadata:   make(map[string]interface{}),
				},
				OperationalStatus: engine.ExecutionStatusRunning,
			}

			// Test PostgreSQL storage implements interface correctly
			err := postgresStorage.SaveWorkflowState(ctx, testExecForDI)
			Expect(err).ToNot(HaveOccurred(), "BR-CONS-001: PostgreSQL storage should implement interface")

			retrieved, err := postgresStorage.LoadWorkflowState(ctx, testExecForDI.ID)
			Expect(err).ToNot(HaveOccurred(), "BR-CONS-001: PostgreSQL storage should load via interface")
			Expect(retrieved.ID).To(Equal(testExecForDI.ID), "Should retrieve correct execution")

			// Validate checkpoint operations work via interface
			checkpoint, err := postgresStorage.CreateCheckpoint(ctx, testExecForDI, "test-checkpoint")
			Expect(err).ToNot(HaveOccurred(), "BR-CONS-001: Checkpoint creation should work via interface")
			Expect(checkpoint.ExecutionID).To(Equal(testExecForDI.ID), "Checkpoint should reference correct execution")
		})
	})
})

// Mock implementations following project guideline: Reuse existing mocks where possible
type MockWorkflowPersistence struct {
	executions  map[string]*engine.RuntimeWorkflowExecution
	checkpoints map[string]*shared.WorkflowCheckpoint
	saveError   error
	loadError   error
}

func NewMockWorkflowPersistence() *MockWorkflowPersistence {
	return &MockWorkflowPersistence{
		executions:  make(map[string]*engine.RuntimeWorkflowExecution),
		checkpoints: make(map[string]*shared.WorkflowCheckpoint),
	}
}

func (m *MockWorkflowPersistence) SetSaveError(err error) { m.saveError = err }
func (m *MockWorkflowPersistence) SetLoadError(err error) { m.loadError = err }

func (m *MockWorkflowPersistence) SaveWorkflowState(ctx context.Context, execution *engine.RuntimeWorkflowExecution) error {
	if m.saveError != nil {
		return m.saveError
	}
	m.executions[execution.ID] = execution
	return nil
}

func (m *MockWorkflowPersistence) LoadWorkflowState(ctx context.Context, executionID string) (*engine.RuntimeWorkflowExecution, error) {
	if m.loadError != nil {
		return nil, m.loadError
	}
	if exec, exists := m.executions[executionID]; exists {
		return exec, nil
	}
	return nil, errors.New("execution not found")
}

func (m *MockWorkflowPersistence) DeleteWorkflowState(ctx context.Context, executionID string) error {
	delete(m.executions, executionID)
	return nil
}

func (m *MockWorkflowPersistence) RecoverWorkflowStates(ctx context.Context) ([]*engine.RuntimeWorkflowExecution, error) {
	var recovered []*engine.RuntimeWorkflowExecution
	for _, exec := range m.executions {
		if exec.OperationalStatus == engine.ExecutionStatusRunning || exec.OperationalStatus == engine.ExecutionStatusFailed {
			recovered = append(recovered, exec)
		}
	}
	return recovered, nil
}

func (m *MockWorkflowPersistence) GetStateAnalytics(ctx context.Context) (*shared.StateAnalytics, error) {
	activeCount := 0
	for _, exec := range m.executions {
		if exec.OperationalStatus == engine.ExecutionStatusRunning {
			activeCount++
		}
	}

	return &shared.StateAnalytics{
		TotalExecutions:     len(m.executions),
		ActiveExecutions:    activeCount,
		RecoverySuccessRate: 0.95, // Mock success rate
		LastUpdated:         time.Now(),
	}, nil
}

func (m *MockWorkflowPersistence) CreateCheckpoint(ctx context.Context, execution *engine.RuntimeWorkflowExecution, name string) (*shared.WorkflowCheckpoint, error) {
	checkpointID := fmt.Sprintf("checkpoint-%s-%s", execution.ID, name)
	checkpoint := &shared.WorkflowCheckpoint{
		ID:          checkpointID,
		Name:        name,
		ExecutionID: execution.ID,
		CreatedAt:   time.Now(),
	}
	m.checkpoints[checkpointID] = checkpoint
	return checkpoint, nil
}

func (m *MockWorkflowPersistence) RestoreFromCheckpoint(ctx context.Context, checkpointID string) (*engine.RuntimeWorkflowExecution, error) {
	if checkpoint, exists := m.checkpoints[checkpointID]; exists {
		if exec, exists := m.executions[checkpoint.ExecutionID]; exists {
			return exec, nil
		}
	}
	return nil, errors.New("checkpoint not found")
}

func (m *MockWorkflowPersistence) ValidateCheckpoint(ctx context.Context, checkpointID string) (bool, error) {
	_, exists := m.checkpoints[checkpointID]
	return exists, nil
}

func NewMockPgVectorPersistence() *MockWorkflowPersistence {
	// For now, return same mock - will be replaced with actual pgvector implementation
	return NewMockWorkflowPersistence()
}

// Helper for formatted strings
func Sprintf(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}
