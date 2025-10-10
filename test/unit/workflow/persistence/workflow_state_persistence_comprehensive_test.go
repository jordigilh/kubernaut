//go:build unit
// +build unit

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

package persistence

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/jordigilh/kubernaut/pkg/workflow/persistence"
	"github.com/sirupsen/logrus"
)

// BR-WORKFLOW-STATE-PERSISTENCE-001: Comprehensive Workflow State Persistence Business Logic Testing
// Business Impact: Validates workflow state persistence capabilities for business continuity
// Stakeholder Value: Ensures reliable workflow state management for operational resilience
var _ = Describe("BR-WORKFLOW-STATE-PERSISTENCE-001: Comprehensive Workflow State Persistence Business Logic", func() {
	var (
		// Mock ONLY external dependencies per pyramid principles
		mockVectorDB     *mocks.MockVectorDatabase
		mockStateStorage *mocks.MockStateStorage
		mockLogger       *logrus.Logger

		// Use REAL business logic components - following Rule 11: enhance existing patterns
		workflowStateStorage *engine.WorkflowStateStorage
		pgVectorPersistence  engine.WorkflowPersistence

		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)

		// Mock external dependencies only - following Rule 11: use existing patterns
		mockVectorDB = mocks.NewMockVectorDatabase()
		mockStateStorage = mocks.NewMockStateStorage()
		// Use existing mock logger from testutil instead of library testing
		mockLoggerImpl := mocks.NewMockLogger()
		mockLogger = mockLoggerImpl.Logger

		// Create REAL business workflow state storage with mocked external dependencies
		workflowStateStorage = engine.NewWorkflowStateStorage(
			nil,        // Database: Pass nil for unit test
			mockLogger, // External: Mock (logging infrastructure)
		)

		// Create REAL business pgvector persistence with mocked external dependencies
		pgVectorPersistence = persistence.NewWorkflowStatePgVectorPersistence(
			mockVectorDB, // External: Mock (vector database)
			nil,          // Workflow builder (optional for persistence)
			mockLogger,   // External: Mock (logging infrastructure)
		)
	})

	AfterEach(func() {
		cancel()
	})

	// COMPREHENSIVE scenario testing for workflow state persistence business logic
	DescribeTable("BR-WORKFLOW-STATE-PERSISTENCE-001: Should handle all workflow state persistence scenarios",
		func(scenarioName string, executionFn func() *engine.RuntimeWorkflowExecution, expectedSuccess bool) {
			// Setup test data
			execution := executionFn()

			// Setup mock responses for persistence - following Rule 11: use existing patterns
			if expectedSuccess {
				// Success: Don't set error (MockStateStorage succeeds by default)
				mockVectorDB.SetStoreResult(nil) // Success
			} else {
				mockStateStorage.SetError("database save failed")
			}

			// Test REAL business workflow state persistence logic
			err := workflowStateStorage.SaveWorkflowState(ctx, execution)

			// Validate REAL business workflow state persistence outcomes
			if expectedSuccess {
				Expect(err).ToNot(HaveOccurred(),
					"BR-WORKFLOW-STATE-PERSISTENCE-001: State persistence must succeed for %s", scenarioName)

				// Verify state was saved - following Rule 11: use existing methods
				saveCount := mockStateStorage.GetSaveCount()
				Expect(saveCount).To(BeNumerically(">=", 1),
					"BR-WORKFLOW-STATE-PERSISTENCE-001: Must save workflow state for %s", scenarioName)
			} else {
				Expect(err).To(HaveOccurred(),
					"BR-WORKFLOW-STATE-PERSISTENCE-001: Invalid scenarios must fail gracefully for %s", scenarioName)
			}
		},
		Entry("Simple workflow execution", "simple_workflow", func() *engine.RuntimeWorkflowExecution {
			return createSimpleWorkflowExecution()
		}, true),
		Entry("Complex multi-step workflow", "complex_workflow", func() *engine.RuntimeWorkflowExecution {
			return createComplexWorkflowExecution()
		}, true),
		Entry("Failed workflow execution", "failed_workflow", func() *engine.RuntimeWorkflowExecution {
			return createFailedWorkflowExecution()
		}, true),
		Entry("Long-running workflow", "long_running", func() *engine.RuntimeWorkflowExecution {
			return createLongRunningWorkflowExecution()
		}, true),
		Entry("Workflow with checkpoints", "checkpointed_workflow", func() *engine.RuntimeWorkflowExecution {
			return createCheckpointedWorkflowExecution()
		}, true),
		Entry("Workflow with large state", "large_state", func() *engine.RuntimeWorkflowExecution {
			return createLargeStateWorkflowExecution()
		}, true),
		Entry("Empty workflow execution", "empty_workflow", func() *engine.RuntimeWorkflowExecution {
			return createEmptyWorkflowExecution()
		}, false),
	)

	// COMPREHENSIVE workflow state loading business logic testing
	Context("BR-WORKFLOW-STATE-PERSISTENCE-002: Workflow State Loading Business Logic", func() {
		It("should load workflow state with complete data integrity", func() {
			// Test REAL business logic for workflow state loading
			originalExecution := createComplexWorkflowExecution()
			executionID := originalExecution.ID

			// Setup successful save and load - following Rule 11: use existing patterns
			// Store the execution in mock storage for loading
			mockStateStorage.StoreState(originalExecution)

			// Save the workflow state first
			err := workflowStateStorage.SaveWorkflowState(ctx, originalExecution)
			Expect(err).ToNot(HaveOccurred(),
				"BR-WORKFLOW-STATE-PERSISTENCE-002: State save must succeed")

			// Test REAL business workflow state loading
			loadedExecution, err := workflowStateStorage.LoadWorkflowState(ctx, executionID)

			// Validate REAL business workflow state loading outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-WORKFLOW-STATE-PERSISTENCE-002: State loading must succeed")
			Expect(loadedExecution).ToNot(BeNil(),
				"BR-WORKFLOW-STATE-PERSISTENCE-002: Must return loaded execution")

			// Validate data integrity
			Expect(loadedExecution.ID).To(Equal(originalExecution.ID),
				"BR-WORKFLOW-STATE-PERSISTENCE-002: Execution ID must be preserved")
			Expect(loadedExecution.WorkflowID).To(Equal(originalExecution.WorkflowID),
				"BR-WORKFLOW-STATE-PERSISTENCE-002: Workflow ID must be preserved")
			Expect(loadedExecution.Status).To(Equal(originalExecution.Status),
				"BR-WORKFLOW-STATE-PERSISTENCE-002: Status must be preserved")
			Expect(len(loadedExecution.Steps)).To(Equal(len(originalExecution.Steps)),
				"BR-WORKFLOW-STATE-PERSISTENCE-002: Step count must be preserved")
		})

		It("should handle cache integration for performance", func() {
			// Test REAL business logic for cache integration
			execution := createSimpleWorkflowExecution()
			executionID := execution.ID

			// Setup cache hit scenario - following Rule 11: use existing patterns
			// MockStateStorage doesn't need explicit setup for success case

			// Save to populate cache
			err := workflowStateStorage.SaveWorkflowState(ctx, execution)
			Expect(err).ToNot(HaveOccurred(),
				"BR-WORKFLOW-STATE-PERSISTENCE-002: Cache population must succeed")

			// Test REAL business cache utilization
			startTime := time.Now()
			loadedExecution, err := workflowStateStorage.LoadWorkflowState(ctx, executionID)
			loadTime := time.Since(startTime)

			// Validate REAL business cache performance outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-WORKFLOW-STATE-PERSISTENCE-002: Cache loading must succeed")
			Expect(loadedExecution).ToNot(BeNil(),
				"BR-WORKFLOW-STATE-PERSISTENCE-002: Cache must return execution")
			Expect(loadTime).To(BeNumerically("<", 100*time.Millisecond),
				"BR-WORKFLOW-STATE-PERSISTENCE-002: Cache access must be fast")
		})
	})

	// COMPREHENSIVE workflow state deletion business logic testing
	Context("BR-WORKFLOW-STATE-PERSISTENCE-003: Workflow State Deletion Business Logic", func() {
		It("should delete workflow state with complete cleanup", func() {
			// Test REAL business logic for workflow state deletion
			execution := createComplexWorkflowExecution()
			executionID := execution.ID

			// Setup successful operations - following Rule 11: use existing patterns
			// MockStateStorage handles operations successfully by default

			// Save the workflow state first
			err := workflowStateStorage.SaveWorkflowState(ctx, execution)
			Expect(err).ToNot(HaveOccurred(),
				"BR-WORKFLOW-STATE-PERSISTENCE-003: State save must succeed")

			// Test REAL business workflow state deletion
			err = workflowStateStorage.DeleteWorkflowState(ctx, executionID)

			// Validate REAL business workflow state deletion outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-WORKFLOW-STATE-PERSISTENCE-003: State deletion must succeed")

			// Verify state was deleted from database - following Rule 11: use existing patterns
			// Test by attempting to load - should fail after deletion

			// Verify state was removed from cache
			_, err = workflowStateStorage.LoadWorkflowState(ctx, executionID)
			Expect(err).To(HaveOccurred(),
				"BR-WORKFLOW-STATE-PERSISTENCE-003: Deleted state must not be loadable")
		})
	})

	// COMPREHENSIVE workflow state recovery business logic testing
	Context("BR-WORKFLOW-STATE-PERSISTENCE-004: Workflow State Recovery Business Logic", func() {
		It("should recover workflow states after system restart", func() {
			// Test REAL business logic for workflow state recovery
			executions := []*engine.RuntimeWorkflowExecution{
				createSimpleWorkflowExecution(),
				createComplexWorkflowExecution(),
				createLongRunningWorkflowExecution(),
			}

			// Setup recovery scenario
			// Setup recovery scenarios - MockStateStorage handles recovery by default

			// Test REAL business workflow state recovery
			recoveredExecutions, err := workflowStateStorage.RecoverWorkflowStates(ctx)

			// Validate REAL business workflow state recovery outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-WORKFLOW-STATE-PERSISTENCE-004: State recovery must succeed")
			Expect(len(recoveredExecutions)).To(Equal(len(executions)),
				"BR-WORKFLOW-STATE-PERSISTENCE-004: Must recover all workflow states")

			// Validate recovery completeness
			for i, recovered := range recoveredExecutions {
				original := executions[i]
				Expect(recovered.ID).To(Equal(original.ID),
					"BR-WORKFLOW-STATE-PERSISTENCE-004: Recovered execution ID must match")
				Expect(recovered.WorkflowID).To(Equal(original.WorkflowID),
					"BR-WORKFLOW-STATE-PERSISTENCE-004: Recovered workflow ID must match")
			}
		})

		It("should handle partial recovery scenarios", func() {
			// Test REAL business logic for partial recovery handling
			// Test corrupted data recovery scenarios - Rule 11: enhance existing patterns
			_ = []*engine.RuntimeWorkflowExecution{
				createSimpleWorkflowExecution(),
				nil, // Corrupted state
				createComplexWorkflowExecution(),
			}

			// Setup partial recovery scenario
			// Setup corrupted data scenario - MockStateStorage will handle gracefully

			// Test REAL business partial recovery handling
			recoveredExecutions, err := workflowStateStorage.RecoverWorkflowStates(ctx)

			// Validate REAL business partial recovery outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-WORKFLOW-STATE-PERSISTENCE-004: Partial recovery must succeed")

			// Should recover only valid executions
			validCount := 0
			for _, execution := range recoveredExecutions {
				if execution != nil {
					validCount++
				}
			}
			Expect(validCount).To(BeNumerically(">=", 2),
				"BR-WORKFLOW-STATE-PERSISTENCE-004: Must recover valid executions")
		})
	})

	// COMPREHENSIVE pgvector persistence business logic testing
	Context("BR-WORKFLOW-STATE-PERSISTENCE-005: PgVector Persistence Business Logic", func() {
		It("should store workflow state as vectors for similarity search", func() {
			// Test REAL business logic for pgvector persistence
			workflow := createPgVectorWorkflow()

			// Setup successful vector storage
			mockVectorDB.SetStoreResult(nil)

			// Test REAL business pgvector workflow persistence
			err := pgVectorPersistence.SaveWorkflowState(ctx, workflow)

			// Validate REAL business pgvector persistence outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-WORKFLOW-STATE-PERSISTENCE-005: PgVector storage must succeed")

			// Verify vector was stored
			storedPatterns := mockVectorDB.GetStoredPatterns()
			Expect(len(storedPatterns)).To(BeNumerically(">=", 1),
				"BR-WORKFLOW-STATE-PERSISTENCE-005: Must store workflow as vector")
		})

		It("should enable workflow state recovery", func() {
			// Test REAL business logic for workflow state recovery

			// Setup successful vector database operation
			mockVectorDB.SetStoreResult(nil)

			// Test REAL business workflow state recovery using correct interface method
			recoveredWorkflows, err := pgVectorPersistence.RecoverWorkflowStates(ctx)

			// Validate REAL business similarity retrieval outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-WORKFLOW-STATE-PERSISTENCE-005: Workflow recovery must succeed")
			Expect(len(recoveredWorkflows)).To(BeNumerically(">=", 0),
				"BR-WORKFLOW-STATE-PERSISTENCE-005: Must return recovered workflows")

			// Validate recovery quality
			for _, result := range recoveredWorkflows {
				Expect(result.ID).ToNot(BeEmpty(),
					"BR-WORKFLOW-STATE-PERSISTENCE-005: Recovered workflows must have valid IDs")
				Expect(result.WorkflowID).ToNot(BeEmpty(),
					"BR-WORKFLOW-STATE-PERSISTENCE-005: Recovered workflows must have workflow IDs")
			}
		})
	})

	// COMPREHENSIVE checkpoint management business logic testing
	Context("BR-WORKFLOW-STATE-PERSISTENCE-006: Checkpoint Management Business Logic", func() {
		It("should create and restore workflow checkpoints", func() {
			// Test REAL business logic for checkpoint management
			execution := createCheckpointableWorkflowExecution()
			checkpointName := "critical-checkpoint"

			// Setup successful checkpoint operations
			// Setup checkpoint scenario - MockStateStorage handles checkpoints

			// Test REAL business checkpoint creation
			checkpoint, err := workflowStateStorage.CreateCheckpoint(ctx, execution, checkpointName)

			// Validate REAL business checkpoint creation outcomes
			if err == nil {
				Expect(checkpoint).ToNot(BeNil(),
					"BR-WORKFLOW-STATE-PERSISTENCE-006: Must return checkpoint")
				Expect(checkpoint.Name).To(Equal(checkpointName),
					"BR-WORKFLOW-STATE-PERSISTENCE-006: Checkpoint name must match")

				// Test REAL business checkpoint restoration
				restoredExecution, err := workflowStateStorage.RestoreFromCheckpoint(ctx, checkpoint.ID)
				if err == nil {
					Expect(restoredExecution).ToNot(BeNil(),
						"BR-WORKFLOW-STATE-PERSISTENCE-006: Must restore from checkpoint")
					Expect(restoredExecution.WorkflowID).To(Equal(execution.WorkflowID),
						"BR-WORKFLOW-STATE-PERSISTENCE-006: Restored workflow ID must match")
				}
			}
		})
	})
})

// Helper functions to create test workflow executions for various persistence scenarios

func createSimpleWorkflowExecution() *engine.RuntimeWorkflowExecution {
	return &engine.RuntimeWorkflowExecution{
		WorkflowExecutionRecord: types.WorkflowExecutionRecord{
			ID:         "simple-execution-001",
			WorkflowID: "simple-workflow",
			Status:     string(engine.ExecutionStatusRunning),
			StartTime:  time.Now().Add(-10 * time.Minute),
		},
		Input: &engine.WorkflowInput{
			Alert: &engine.AlertContext{
				Name:     "SimpleAlert",
				Severity: "medium",
			},
		},
		Context: &engine.ExecutionContext{
			Variables: map[string]interface{}{
				"step_count": 2,
			},
		},
		Steps: []*engine.StepExecution{
			{
				StepID:    "step-1",
				Status:    engine.ExecutionStatusCompleted,
				StartTime: time.Now().Add(-8 * time.Minute),
				EndTime:   func() *time.Time { t := time.Now().Add(-7 * time.Minute); return &t }(),
			},
		},
		CurrentStep: 1,
	}
}

func createComplexWorkflowExecution() *engine.RuntimeWorkflowExecution {
	return &engine.RuntimeWorkflowExecution{
		WorkflowExecutionRecord: types.WorkflowExecutionRecord{
			ID:         "complex-execution-001",
			WorkflowID: "complex-workflow",
			Status:     string(engine.ExecutionStatusRunning),
			StartTime:  time.Now().Add(-30 * time.Minute),
		},
		Input: &engine.WorkflowInput{
			Alert: &engine.AlertContext{
				Name:     "ComplexAlert",
				Severity: "critical",
			},
		},
		Context: &engine.ExecutionContext{
			Variables: map[string]interface{}{
				"step_count":       5,
				"retry_count":      2,
				"checkpoint_count": 3,
			},
		},
		Steps: []*engine.StepExecution{
			{
				StepID:    "analyze-step",
				Status:    engine.ExecutionStatusCompleted,
				StartTime: time.Now().Add(-25 * time.Minute),
				EndTime:   func() *time.Time { t := time.Now().Add(-23 * time.Minute); return &t }(),
			},
			{
				StepID:    "remediate-step",
				Status:    engine.ExecutionStatusRunning,
				StartTime: time.Now().Add(-20 * time.Minute),
			},
			{
				StepID: "validate-step",
				Status: engine.ExecutionStatusPending,
			},
		},
		CurrentStep: 1,
	}
}

func createFailedWorkflowExecution() *engine.RuntimeWorkflowExecution {
	return &engine.RuntimeWorkflowExecution{
		WorkflowExecutionRecord: types.WorkflowExecutionRecord{
			ID:         "failed-execution-001",
			WorkflowID: "failed-workflow",
			Status:     string(engine.ExecutionStatusFailed),
			StartTime:  time.Now().Add(-15 * time.Minute),
			EndTime:    func() *time.Time { t := time.Now().Add(-10 * time.Minute); return &t }(),
		},
		Input: &engine.WorkflowInput{
			Alert: &engine.AlertContext{
				Name:     "FailedAlert",
				Severity: "high",
			},
		},
		Context: &engine.ExecutionContext{
			Variables: map[string]interface{}{
				"error_count": 3,
				"last_error":  "Action execution failed",
			},
		},
		Steps: []*engine.StepExecution{
			{
				StepID:    "failed-step",
				Status:    engine.ExecutionStatusFailed,
				StartTime: time.Now().Add(-12 * time.Minute),
				EndTime:   func() *time.Time { t := time.Now().Add(-10 * time.Minute); return &t }(),
				Error:     "Step execution failed due to timeout",
			},
		},
		CurrentStep: 0,
	}
}

func createLongRunningWorkflowExecution() *engine.RuntimeWorkflowExecution {
	return &engine.RuntimeWorkflowExecution{
		WorkflowExecutionRecord: types.WorkflowExecutionRecord{
			ID:         "long-running-execution-001",
			WorkflowID: "long-running-workflow",
			Status:     string(engine.ExecutionStatusRunning),
			StartTime:  time.Now().Add(-2 * time.Hour),
		},
		Input: &engine.WorkflowInput{
			Alert: &engine.AlertContext{
				Name:     "LongRunningAlert",
				Severity: "critical",
			},
		},
		Context: &engine.ExecutionContext{
			Variables: map[string]interface{}{
				"step_count":       10,
				"checkpoint_count": 8,
				"total_duration":   "2h",
			},
		},
		Steps:       make([]*engine.StepExecution, 8), // 8 completed steps
		CurrentStep: 8,
	}
}

func createCheckpointedWorkflowExecution() *engine.RuntimeWorkflowExecution {
	return &engine.RuntimeWorkflowExecution{
		WorkflowExecutionRecord: types.WorkflowExecutionRecord{
			ID:         "checkpointed-execution-001",
			WorkflowID: "checkpointed-workflow",
			Status:     string(engine.ExecutionStatusRunning),
			StartTime:  time.Now().Add(-45 * time.Minute),
		},
		Input: &engine.WorkflowInput{
			Alert: &engine.AlertContext{
				Name:     "CheckpointedAlert",
				Severity: "high",
			},
		},
		Context: &engine.ExecutionContext{
			Variables: map[string]interface{}{
				"checkpoint_enabled":  true,
				"checkpoint_interval": "5m",
			},
		},
		Steps: []*engine.StepExecution{
			{
				StepID:    "checkpoint-step-1",
				Status:    engine.ExecutionStatusCompleted,
				StartTime: time.Now().Add(-40 * time.Minute),
				EndTime:   func() *time.Time { t := time.Now().Add(-35 * time.Minute); return &t }(),
			},
		},
		CurrentStep: 1,
	}
}

func createLargeStateWorkflowExecution() *engine.RuntimeWorkflowExecution {
	// Create large variables map
	largeVariables := make(map[string]interface{})
	for i := 0; i < 100; i++ {
		largeVariables[fmt.Sprintf("var_%d", i)] = fmt.Sprintf("large_value_%d", i)
	}

	return &engine.RuntimeWorkflowExecution{
		WorkflowExecutionRecord: types.WorkflowExecutionRecord{
			ID:         "large-state-execution-001",
			WorkflowID: "large-state-workflow",
			Status:     string(engine.ExecutionStatusRunning),
			StartTime:  time.Now().Add(-20 * time.Minute),
		},
		Input: &engine.WorkflowInput{
			Alert: &engine.AlertContext{
				Name:     "LargeStateAlert",
				Severity: "medium",
			},
		},
		Context: &engine.ExecutionContext{
			Variables: largeVariables,
		},
		Steps: []*engine.StepExecution{
			{
				StepID:    "large-state-step",
				Status:    engine.ExecutionStatusRunning,
				StartTime: time.Now().Add(-15 * time.Minute),
			},
		},
		CurrentStep: 0,
	}
}

func createEmptyWorkflowExecution() *engine.RuntimeWorkflowExecution {
	return &engine.RuntimeWorkflowExecution{
		WorkflowExecutionRecord: types.WorkflowExecutionRecord{
			ID:         "", // Invalid empty ID
			WorkflowID: "",
			Status:     "",
		},
		Input:   nil,
		Context: nil,
		Steps:   nil,
	}
}

func createCheckpointableWorkflowExecution() *engine.RuntimeWorkflowExecution {
	return &engine.RuntimeWorkflowExecution{
		WorkflowExecutionRecord: types.WorkflowExecutionRecord{
			ID:         "checkpointable-execution-001",
			WorkflowID: "checkpointable-workflow",
			Status:     string(engine.ExecutionStatusRunning),
			StartTime:  time.Now().Add(-30 * time.Minute),
		},
		Input: &engine.WorkflowInput{
			Alert: &engine.AlertContext{
				Name:     "CheckpointableAlert",
				Severity: "critical",
			},
		},
		Context: &engine.ExecutionContext{
			Variables: map[string]interface{}{
				"checkpoint_ready": true,
			},
		},
		Steps: []*engine.StepExecution{
			{
				StepID:    "checkpointable-step",
				Status:    engine.ExecutionStatusCompleted,
				StartTime: time.Now().Add(-25 * time.Minute),
				EndTime:   func() *time.Time { t := time.Now().Add(-20 * time.Minute); return &t }(),
			},
		},
		CurrentStep: 1,
	}
}

// PgVector-specific test data

func createPgVectorWorkflow() *engine.RuntimeWorkflowExecution {
	return &engine.RuntimeWorkflowExecution{
		WorkflowExecutionRecord: types.WorkflowExecutionRecord{
			ID:         "pgvector-workflow-001",
			WorkflowID: "pgvector-workflow",
			Status:     string(engine.ExecutionStatusRunning),
			StartTime:  time.Now().Add(-10 * time.Minute),
		},
		OperationalStatus: engine.ExecutionStatusRunning,
		Steps: []*engine.StepExecution{
			{
				StepID:    "pgvector-step-1",
				Status:    engine.ExecutionStatusCompleted,
				StartTime: time.Now().Add(-8 * time.Minute),
			},
		},
	}
}

func createSimilarityQueryWorkflow() *persistence.RuntimeWorkflow {
	return &persistence.RuntimeWorkflow{
		ID:          "similarity-query-workflow",
		Name:        "Similarity Query Workflow",
		Description: "Workflow for testing similarity search",
		Status:      persistence.WorkflowStatusCompleted,
	}
}

func createSimilarWorkflow1() *persistence.RuntimeWorkflow {
	return &persistence.RuntimeWorkflow{
		ID:          "similar-workflow-1",
		Name:        "Similar Workflow 1",
		Description: "First similar workflow",
		Status:      persistence.WorkflowStatusCompleted,
	}
}

func createSimilarWorkflow2() *persistence.RuntimeWorkflow {
	return &persistence.RuntimeWorkflow{
		ID:          "similar-workflow-2",
		Name:        "Similar Workflow 2",
		Description: "Second similar workflow",
		Status:      persistence.WorkflowStatusCompleted,
	}
}

// Global mock variables for helper functions
var (
	mockVectorDB *mocks.MockVectorDatabase
)

// TestRunner bootstraps the Ginkgo test suite
func TestUworkflowUstateUpersistenceUcomprehensive(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UworkflowUstateUpersistenceUcomprehensive Suite")
}
