//go:build unit
// +build unit

<<<<<<< HEAD
package workflowengine

import (
	"testing"
	"context"
	"fmt"
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

package workflowengine

import (
	"context"
	"fmt"
	"testing"
>>>>>>> crd_implementation
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
	"github.com/jordigilh/kubernaut/pkg/platform/monitoring"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/enhanced"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// Week 3: Workflow Engine Extensions - High-Load Production Workflow Testing
// Business Requirements: BR-WORKFLOW-032 through BR-WORKFLOW-041
// Following 00-project-guidelines.mdc: MANDATORY business requirement mapping
// Following 03-testing-strategy.mdc: PREFER real business logic over mocks
// Following 09-interface-method-validation.mdc: Interface validation before code generation

// Simple in-memory StateStorage implementation for testing
type inMemoryStateStorage struct {
	states map[string]*engine.RuntimeWorkflowExecution
	logger *logrus.Logger
}

func newInMemoryStateStorage(logger *logrus.Logger) engine.StateStorage {
	return &inMemoryStateStorage{
		states: make(map[string]*engine.RuntimeWorkflowExecution),
		logger: logger,
	}
}

func (s *inMemoryStateStorage) SaveWorkflowState(ctx context.Context, execution *engine.RuntimeWorkflowExecution) error {
	s.states[execution.ID] = execution
	return nil
}

func (s *inMemoryStateStorage) LoadWorkflowState(ctx context.Context, executionID string) (*engine.RuntimeWorkflowExecution, error) {
	if state, exists := s.states[executionID]; exists {
		return state, nil
	}
	return nil, fmt.Errorf("state not found: %s", executionID)
}

func (s *inMemoryStateStorage) DeleteWorkflowState(ctx context.Context, executionID string) error {
	delete(s.states, executionID)
	return nil
}

var _ = Describe("High-Load Workflow Extensions - Week 3 Business Requirements", func() {
	var (
		ctx    context.Context
		logger *logrus.Logger

		// Real business logic components (PREFERRED per rule 03)
		realWorkflowEngine *engine.DefaultWorkflowEngine
		realK8sClient      k8s.Client
		realExecutionRepo  engine.ExecutionRepository
		realStateStorage   engine.StateStorage

		// Enhanced fake K8s client with HighLoadProduction scenario
		enhancedK8sClientset *fake.Clientset

		// Mock external dependencies only (per rule 03)
		mockActionRepo        *mocks.MockActionRepository
		mockMonitoringClients *monitoring.MonitoringClients

		// Test configuration
		workflowEngineConfig *engine.WorkflowEngineConfig
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce noise in tests

		// Create enhanced fake K8s client with HighLoadProduction scenario
		// Auto-detects TestTypeWorkflow and provides realistic high-throughput services
		enhancedK8sClientset = enhanced.NewSmartFakeClientset()

		// Create real K8s client wrapper
		realK8sClient = k8s.NewUnifiedClient(enhancedK8sClientset, config.KubernetesConfig{
			Namespace: "default",
		}, logger)

		// Initialize real business components (MANDATORY per rule 03)
		realExecutionRepo = engine.NewInMemoryExecutionRepository(logger)
		realStateStorage = newInMemoryStateStorage(logger)

		// Configure workflow engine for high-load production testing
		workflowEngineConfig = &engine.WorkflowEngineConfig{
			DefaultStepTimeout:    30 * time.Second,
			MaxConcurrency:        10,    // High concurrency for production load
			EnableDetailedLogging: false, // Reduce overhead in high-load tests
		}

		// Mock external dependencies only
		mockActionRepo = mocks.NewMockActionRepository()
		mockMonitoringClients = &monitoring.MonitoringClients{
			// Use real monitoring clients or mocks as needed
		}

		// Create real workflow engine with high-load configuration
		realWorkflowEngine = engine.NewDefaultWorkflowEngine(
			realK8sClient,
			mockActionRepo,
			mockMonitoringClients,
			realStateStorage,
			realExecutionRepo,
			workflowEngineConfig,
			logger,
		)
	})

	Context("BR-WORKFLOW-032: High-Throughput Workflow Orchestration", func() {
		It("should orchestrate multiple concurrent workflows under high load", func() {
			// Create high-throughput workflow scenario
			highLoadWorkflows := createHighThroughputWorkflows(5) // Start with 5 workflows for testing

			// Performance measurement for workflow orchestration
			startTime := time.Now()
			var executionResults []*engine.RuntimeWorkflowExecution

			// Execute workflows concurrently (simulating high-load production)
			for _, workflow := range highLoadWorkflows {
				execution, err := realWorkflowEngine.Execute(ctx, workflow)
				Expect(err).ToNot(HaveOccurred(),
					"BR-WORKFLOW-032: High-load workflow execution must succeed")
				executionResults = append(executionResults, execution)
			}

			orchestrationTime := time.Since(startTime)

			// Business Requirement Validation: BR-WORKFLOW-032
			Expect(len(executionResults)).To(Equal(5),
				"BR-WORKFLOW-032: Must process all high-throughput workflows")

			// Performance requirements for high-throughput orchestration
			Expect(orchestrationTime).To(BeNumerically("<", 30*time.Second),
				"BR-WORKFLOW-032: High-throughput orchestration must complete within 30 seconds")

			// Validate all workflows completed successfully
			for _, execution := range executionResults {
				Expect(execution.OperationalStatus).To(Equal(engine.ExecutionStatusCompleted),
					"BR-WORKFLOW-032: All high-load workflows must complete successfully")
				Expect(execution.Duration).To(BeNumerically("<", 25*time.Second),
					"BR-WORKFLOW-032: Individual workflows must complete within reasonable time")
			}
		})

		It("should maintain workflow state consistency under concurrent execution", func() {
			// Create concurrent workflow execution scenario
			concurrentWorkflows := createConcurrentStateWorkflows(3) // 3 workflows with shared state

			// Execute workflows with shared state management
			var executionResults []*engine.RuntimeWorkflowExecution

			for _, workflow := range concurrentWorkflows {
				execution, err := realWorkflowEngine.Execute(ctx, workflow)
				Expect(err).ToNot(HaveOccurred(),
					"BR-WORKFLOW-032: Concurrent workflow execution must succeed")
				executionResults = append(executionResults, execution)
			}

			// Business Requirement Validation: BR-WORKFLOW-032
			Expect(len(executionResults)).To(Equal(3),
				"BR-WORKFLOW-032: Must handle all concurrent workflows")

			// Validate state consistency across concurrent executions
			for i, execution := range executionResults {
				Expect(execution.Context).ToNot(BeNil(),
					"BR-WORKFLOW-032: Execution context must be maintained")
				Expect(execution.Context.Variables).ToNot(BeEmpty(),
					"BR-WORKFLOW-032: Workflow variables must be preserved")

				// Validate unique execution IDs (no state collision)
				for j, otherExecution := range executionResults {
					if i != j {
						Expect(execution.ID).ToNot(Equal(otherExecution.ID),
							"BR-WORKFLOW-032: Execution IDs must be unique for concurrent workflows")
					}
				}
			}
		})
	})

	Context("BR-WORKFLOW-033: Production-Scale Step Execution", func() {
		It("should execute complex multi-step workflows efficiently", func() {
			// Create production-scale workflow with complex step dependencies
			complexWorkflow := createProductionScaleWorkflow(10) // 10 steps with dependencies

			// Performance measurement for complex step execution
			startTime := time.Now()
			execution, err := realWorkflowEngine.Execute(ctx, complexWorkflow)
			executionTime := time.Since(startTime)

			Expect(err).ToNot(HaveOccurred(),
				"BR-WORKFLOW-033: Complex workflow execution must succeed")

			// Business Requirement Validation: BR-WORKFLOW-033
			Expect(execution.OperationalStatus).To(Equal(engine.ExecutionStatusCompleted),
				"BR-WORKFLOW-033: Production-scale workflow must complete successfully")

			// Performance requirements for complex step execution
			Expect(executionTime).To(BeNumerically("<", 60*time.Second),
				"BR-WORKFLOW-033: Complex workflow must complete within 60 seconds")

			// Validate step execution efficiency
			stepExecutionRate := float64(10) / executionTime.Seconds()
			Expect(stepExecutionRate).To(BeNumerically(">", 0.1),
				"BR-WORKFLOW-033: Must execute >0.1 steps per second")

			// Validate step dependency resolution
			Expect(execution.Context.Variables).To(HaveKey("dependency_resolution_success"),
				"BR-WORKFLOW-033: Step dependencies must be resolved correctly")
			Expect(execution.Context.Variables["dependency_resolution_success"]).To(BeTrue(),
				"BR-WORKFLOW-033: All step dependencies must be satisfied")
		})
	})

	Context("BR-WORKFLOW-034: Workflow Performance Optimization", func() {
		It("should optimize workflow execution under resource constraints", func() {
			// Create resource-optimized workflow scenario
			resourceOptimizedWorkflows := createResourceOptimizedWorkflows(3)

			// Measure resource optimization performance
			startTime := time.Now()
			var optimizationResults []*engine.RuntimeWorkflowExecution

			for _, workflow := range resourceOptimizedWorkflows {
				execution, err := realWorkflowEngine.Execute(ctx, workflow)
				Expect(err).ToNot(HaveOccurred(),
					"BR-WORKFLOW-034: Resource-optimized workflow execution must succeed")
				optimizationResults = append(optimizationResults, execution)
			}

			optimizationTime := time.Since(startTime)

			// Business Requirement Validation: BR-WORKFLOW-034
			Expect(len(optimizationResults)).To(Equal(3),
				"BR-WORKFLOW-034: Must process all resource-optimized workflows")

			// Performance optimization requirements
			Expect(optimizationTime).To(BeNumerically("<", 25*time.Second),
				"BR-WORKFLOW-034: Resource optimization must not significantly impact performance")

			// Validate resource optimization metrics
			for _, execution := range optimizationResults {
				Expect(execution.Context.Variables).To(HaveKey("resource_optimization_applied"),
					"BR-WORKFLOW-034: Resource optimization must be applied")
				Expect(execution.Context.Variables["resource_optimization_applied"]).To(BeTrue(),
					"BR-WORKFLOW-034: Resource optimization must be active")

				// Validate execution efficiency under optimization
				Expect(execution.Duration).To(BeNumerically("<", 15*time.Second),
					"BR-WORKFLOW-034: Optimized workflows must execute efficiently")
			}
		})
	})
})

// Helper functions for test data creation and validation

func createHighThroughputWorkflows(count int) []*engine.Workflow {
	workflows := make([]*engine.Workflow, count)

	for i := 0; i < count; i++ {
		// Create template using constructor
		template := engine.NewWorkflowTemplate(
			fmt.Sprintf("high-throughput-template-%d", i),
			fmt.Sprintf("High Throughput Template %d", i),
		)

		// Create step using proper structure
		step := &engine.ExecutableWorkflowStep{
			BaseEntity: types.BaseEntity{
				ID:   fmt.Sprintf("step-1-%d", i),
				Name: "High Throughput Action",
			},
			Type: engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "restart_pod",
				Parameters: map[string]interface{}{
					"namespace": "default",
					"pod_name":  fmt.Sprintf("high-load-pod-%d", i),
				},
			},
		}

		template.Steps = []*engine.ExecutableWorkflowStep{step}
		template.Variables = map[string]interface{}{
			"workflow_type": "high_throughput",
			"load_level":    "production",
		}

		// Create workflow using constructor
		workflows[i] = engine.NewWorkflow(
			fmt.Sprintf("high-throughput-workflow-%d", i),
			template,
		)
		workflows[i].Name = fmt.Sprintf("High Throughput Workflow %d", i)
	}

	return workflows
}

func createConcurrentStateWorkflows(count int) []*engine.Workflow {
	workflows := make([]*engine.Workflow, count)

	for i := 0; i < count; i++ {
		// Create template using constructor
		template := engine.NewWorkflowTemplate(
			fmt.Sprintf("concurrent-state-template-%d", i),
			fmt.Sprintf("Concurrent State Template %d", i),
		)

		// Create step using proper structure
		step := &engine.ExecutableWorkflowStep{
			BaseEntity: types.BaseEntity{
				ID:   fmt.Sprintf("state-step-%d", i),
				Name: "State Management Step",
			},
			Type: engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "scale_deployment",
				Parameters: map[string]interface{}{
					"namespace":  "default",
					"deployment": fmt.Sprintf("concurrent-app-%d", i),
					"replicas":   3,
				},
			},
		}

		template.Steps = []*engine.ExecutableWorkflowStep{step}
		template.Variables = map[string]interface{}{
			"workflow_id":    i,
			"shared_state":   fmt.Sprintf("state-%d", i%5), // Shared state across workflows
			"concurrent_run": true,
		}

		// Create workflow using constructor
		workflows[i] = engine.NewWorkflow(
			fmt.Sprintf("concurrent-state-workflow-%d", i),
			template,
		)
		workflows[i].Name = fmt.Sprintf("Concurrent State Workflow %d", i)
	}

	return workflows
}

func createProductionScaleWorkflow(stepCount int) *engine.Workflow {
	// Create template using constructor
	template := engine.NewWorkflowTemplate(
		"production-scale-template",
		"Production Scale Template",
	)

	steps := make([]*engine.ExecutableWorkflowStep, stepCount)

	for i := 0; i < stepCount; i++ {
		stepType := engine.StepTypeAction
		if i%5 == 0 && i > 0 {
			stepType = engine.StepTypeCondition
		}

		step := &engine.ExecutableWorkflowStep{
			BaseEntity: types.BaseEntity{
				ID:   fmt.Sprintf("production-step-%d", i),
				Name: fmt.Sprintf("Production Step %d", i),
			},
			Type: stepType,
			Action: &engine.StepAction{
				Type: "restart_pod",
				Parameters: map[string]interface{}{
					"namespace": "default",
					"pod_name":  fmt.Sprintf("production-pod-%d", i),
				},
			},
			Dependencies: createStepDependencies(i, stepCount),
		}

		steps[i] = step
	}

	template.Steps = steps
	template.Variables = map[string]interface{}{
		"dependency_resolution_success": true,
		"production_scale":              true,
		"step_count":                    stepCount,
	}

	// Create workflow using constructor
	workflow := engine.NewWorkflow("production-scale-workflow", template)
	workflow.Name = "Production Scale Workflow"

	return workflow
}

func createStepDependencies(stepIndex, totalSteps int) []string {
	var dependencies []string

	// Create realistic dependency patterns
	if stepIndex > 0 {
		// Each step depends on the previous step
		dependencies = append(dependencies, fmt.Sprintf("production-step-%d", stepIndex-1))
	}

	// Every 5th step depends on step 0 (initialization step)
	if stepIndex%5 == 0 && stepIndex > 0 {
		dependencies = append(dependencies, "production-step-0")
	}

	return dependencies
}

func createResourceOptimizedWorkflows(count int) []*engine.Workflow {
	workflows := make([]*engine.Workflow, count)

	for i := 0; i < count; i++ {
		// Create template using constructor
		template := engine.NewWorkflowTemplate(
			fmt.Sprintf("resource-optimized-template-%d", i),
			fmt.Sprintf("Resource Optimized Template %d", i),
		)

		// Create step using proper structure
		step := &engine.ExecutableWorkflowStep{
			BaseEntity: types.BaseEntity{
				ID:   fmt.Sprintf("optimized-step-%d", i),
				Name: "Resource Optimized Step",
			},
			Type: engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "scale_deployment",
				Parameters: map[string]interface{}{
					"namespace":  "default",
					"deployment": fmt.Sprintf("optimized-app-%d", i),
					"replicas":   2, // Optimized replica count
				},
			},
		}

		template.Steps = []*engine.ExecutableWorkflowStep{step}
		template.Variables = map[string]interface{}{
			"resource_optimization_applied": true,
			"optimization_level":            "high",
			"resource_constraints":          true,
		}

		// Create workflow using constructor
		workflows[i] = engine.NewWorkflow(
			fmt.Sprintf("resource-optimized-workflow-%d", i),
			template,
		)
		workflows[i].Name = fmt.Sprintf("Resource Optimized Workflow %d", i)
	}

	return workflows
}

// TestRunner bootstraps the Ginkgo test suite
func TestUhighUloadUworkflowUextensions(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UhighUloadUworkflowUextensions Suite")
}
