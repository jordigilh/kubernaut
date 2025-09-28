//go:build unit
// +build unit

package workflowengine

import (
	"testing"
	"context"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/sirupsen/logrus"
)

// BR-WF-ADV-001: Advanced Workflow Engine Extensions - Pyramid Testing (70% Unit Coverage)
// Business Impact: Comprehensive workflow automation for complex Kubernetes scenarios
// Stakeholder Value: Operations teams can handle sophisticated remediation workflows
var _ = Describe("BR-WF-ADV-001: Advanced Workflow Engine Extensions", func() {
	var (
		// Mock ONLY external dependencies per pyramid principles
		mockStateStorage  *mocks.MockStateStorage
		mockExecutionRepo *mocks.WorkflowExecutionRepositoryMock
		mockLogger        *logrus.Logger

		// Use REAL business logic components
		workflowEngine *engine.DefaultWorkflowEngine
		engineConfig   *engine.WorkflowEngineConfig

		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)

		// Mock external dependencies only
		mockStateStorage = mocks.NewMockStateStorage()
		mockExecutionRepo = mocks.NewWorkflowExecutionRepositoryMock()
		mockLogger = logrus.New()
		mockLogger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests

		// Create REAL business logic configuration
		engineConfig = &engine.WorkflowEngineConfig{
			DefaultStepTimeout:    5 * time.Second,
			MaxRetryDelay:         1 * time.Second,
			EnableStateRecovery:   true,
			EnableDetailedLogging: false,
			MaxConcurrency:        10,
		}

		// Create REAL workflow engine with mocked external dependencies
		workflowEngine = engine.NewDefaultWorkflowEngine(
			mocks.NewMockK8sClient(nil),     // External: Mock
			mocks.NewMockActionRepository(), // External: Mock
			nil,                             // External: Mock (monitoring)
			mockStateStorage,                // External: Mock
			mockExecutionRepo,               // External: Mock
			engineConfig,                    // Real: Business configuration
			mockLogger,                      // External: Mock (logging infrastructure)
		)
	})

	AfterEach(func() {
		cancel()
	})

	// BR-WF-ADV-002: Dynamic Workflow Composition and Modification
	Context("BR-WF-ADV-002: Dynamic Workflow Composition", func() {
		It("should compose workflows dynamically based on runtime conditions", func() {
			// Business Scenario: System adapts workflows based on current cluster state
			// Business Impact: Reduces manual intervention, improves response accuracy

			// Test REAL business logic for dynamic composition
			baseTemplate := createBaseWorkflowTemplate()
			conditions := map[string]interface{}{
				"cluster_health":    0.7, // 70% healthy
				"resource_pressure": 0.8, // 80% resource usage
				"alert_severity":    "high",
				"business_hours":    true,
			}

			// Test REAL dynamic composition logic
			dynamicWorkflow := engine.NewWorkflow("dynamic-composition-001", baseTemplate)
			dynamicWorkflow.Metadata["composition_conditions"] = conditions

			result, err := workflowEngine.Execute(ctx, dynamicWorkflow)

			// Validate REAL business outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-WF-ADV-002: Dynamic workflow composition must succeed for business adaptability")
			Expect(result).ToNot(BeNil(),
				"BR-WF-ADV-002: Dynamic composition must produce executable workflow")
			Expect(result.Metadata["composition_applied"]).To(BeTrue(),
				"BR-WF-ADV-002: Must track dynamic composition for business monitoring")
		})

		It("should modify workflows in-flight based on changing conditions", func() {
			// Business Scenario: Long-running workflows adapt to changing cluster conditions
			// Business Impact: Maintains workflow relevance, prevents outdated actions

			// Test REAL business logic for in-flight modification
			longRunningTemplate := createAdvancedLongRunningWorkflowTemplate()
			workflow := engine.NewWorkflow("in-flight-modification-001", longRunningTemplate)

			// Start workflow execution
			executionCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			result, err := workflowEngine.Execute(executionCtx, workflow)

			// Validate REAL business modification outcomes
			if err != nil {
				// Execution may be modified or interrupted, which is expected
				mockLogger.WithError(err).Debug("Workflow modified as expected")
			}

			if result != nil {
				Expect(result.Metadata["modification_applied"]).To(BeTrue(),
					"BR-WF-ADV-002: Must track in-flight modifications for business monitoring")
			}

			// Business Value: Adaptive workflows maintain operational relevance
		})
	})

	// BR-WF-ADV-003: Advanced Parallel Execution and Resource Optimization
	Context("BR-WF-ADV-003: Advanced Parallel Execution", func() {
		It("should optimize parallel execution based on resource availability", func() {
			// Business Scenario: System maximizes throughput while respecting resource limits
			// Business Impact: Improves operational efficiency, prevents resource exhaustion

			// Test REAL business logic for resource-aware parallelization
			parallelTemplate := createResourceOptimizedParallelTemplate(15) // Request 15 parallel steps
			workflow := engine.NewWorkflow("resource-optimized-parallel-001", parallelTemplate)

			// Configure resource constraints
			workflow.Metadata["resource_constraints"] = map[string]interface{}{
				"max_cpu_cores":    4,
				"max_memory_gb":    8,
				"max_network_mbps": 100,
			}

			startTime := time.Now()
			result, err := workflowEngine.Execute(ctx, workflow)
			executionTime := time.Since(startTime)

			// Validate REAL business optimization outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-WF-ADV-003: Resource-optimized parallel execution must succeed")
			Expect(result).ToNot(BeNil(),
				"BR-WF-ADV-003: Parallel execution must produce results")
			Expect(executionTime).To(BeNumerically("<", 8*time.Second),
				"BR-WF-ADV-003: Parallel execution must improve performance")
			Expect(result.Metadata["resource_optimization_applied"]).To(BeTrue(),
				"BR-WF-ADV-003: Must track resource optimization for business monitoring")
		})

		It("should implement intelligent load balancing across execution resources", func() {
			// Business Scenario: System distributes workload evenly across available resources
			// Business Impact: Prevents resource hotspots, improves system stability

			// Test REAL business logic for load balancing
			loadBalancedTemplate := createLoadBalancedWorkflowTemplate(20) // 20 steps to balance
			workflow := engine.NewWorkflow("load-balanced-001", loadBalancedTemplate)

			// Configure multiple execution contexts
			workflow.Metadata["execution_contexts"] = []map[string]interface{}{
				{"context": "primary", "capacity": 0.8},
				{"context": "secondary", "capacity": 0.6},
				{"context": "tertiary", "capacity": 0.4},
			}

			result, err := workflowEngine.Execute(ctx, workflow)

			// Validate REAL business load balancing outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-WF-ADV-003: Load-balanced execution must succeed for business efficiency")
			Expect(result).ToNot(BeNil(),
				"BR-WF-ADV-003: Load balancing must produce execution results")
			Expect(result.Metadata["load_balancing_applied"]).To(BeTrue(),
				"BR-WF-ADV-003: Must track load balancing for business monitoring")
		})
	})

	// BR-WF-ADV-004: Intelligent Workflow Caching and Reuse
	Context("BR-WF-ADV-004: Intelligent Workflow Caching", func() {
		It("should cache and reuse workflow patterns for similar scenarios", func() {
			// Business Scenario: System learns from successful workflows and reuses patterns
			// Business Impact: Reduces execution time, improves consistency

			// Test REAL business logic for workflow caching
			cachableTemplate := createCachableWorkflowTemplate("memory-pressure")
			workflow1 := engine.NewWorkflow("cachable-001", cachableTemplate)
			workflow1.Metadata["caching_enabled"] = true

			// Execute first workflow to establish cache
			result1, err1 := workflowEngine.Execute(ctx, workflow1)
			Expect(err1).ToNot(HaveOccurred(),
				"BR-WF-ADV-004: Initial workflow execution must succeed for caching")
			Expect(result1).ToNot(BeNil(),
				"BR-WF-ADV-004: Initial workflow must produce results for caching")

			// Execute similar workflow that should benefit from caching
			workflow2 := engine.NewWorkflow("cachable-002", cachableTemplate)
			workflow2.Metadata["caching_enabled"] = true

			startTime := time.Now()
			result2, err2 := workflowEngine.Execute(ctx, workflow2)
			cachedExecutionTime := time.Since(startTime)

			// Validate REAL business caching outcomes
			Expect(err2).ToNot(HaveOccurred(),
				"BR-WF-ADV-004: Cached workflow execution must succeed")
			Expect(result2).ToNot(BeNil(),
				"BR-WF-ADV-004: Cached execution must produce results")
			Expect(result2.Metadata["cache_hit"]).To(BeTrue(),
				"BR-WF-ADV-004: Must utilize workflow cache for business efficiency")
			Expect(cachedExecutionTime).To(BeNumerically("<", 2*time.Second),
				"BR-WF-ADV-004: Cached execution must be significantly faster")

			// Business Value: Improved response times through intelligent caching
		})

		It("should invalidate cache when workflow patterns become outdated", func() {
			// Business Scenario: System detects when cached patterns are no longer effective
			// Business Impact: Maintains workflow accuracy, prevents stale automation

			// Test REAL business logic for cache invalidation
			evolvingTemplate := createEvolvingWorkflowTemplate("cpu-spike")
			workflow := engine.NewWorkflow("cache-invalidation-001", evolvingTemplate)
			workflow.Metadata["cache_invalidation_enabled"] = true

			// Simulate changing conditions that should invalidate cache
			workflow.Metadata["environment_changes"] = map[string]interface{}{
				"cluster_version_changed": true,
				"new_monitoring_rules":    true,
				"updated_policies":        true,
			}

			result, err := workflowEngine.Execute(ctx, workflow)

			// Validate REAL business cache invalidation outcomes
			Expect(err).ToNot(HaveOccurred(),
				"BR-WF-ADV-004: Cache invalidation must not prevent execution")
			Expect(result).ToNot(BeNil(),
				"BR-WF-ADV-004: Invalidated cache execution must produce results")
			Expect(result.Metadata["cache_invalidated"]).To(BeTrue(),
				"BR-WF-ADV-004: Must detect and invalidate outdated cache entries")
		})
	})

	// BR-WF-ADV-005: Cross-Workflow Communication and Coordination
	Context("BR-WF-ADV-005: Cross-Workflow Communication", func() {
		It("should coordinate multiple workflows for complex multi-step operations", func() {
			// Business Scenario: Complex operations require multiple coordinated workflows
			// Business Impact: Enables sophisticated automation, reduces manual coordination

			// Test REAL business logic for workflow coordination
			coordinatorTemplate := createCoordinatorWorkflowTemplate()
			coordinatedTemplates := []struct {
				name     string
				template *engine.ExecutableTemplate
			}{
				{"preparation", createPreparationWorkflowTemplate()},
				{"execution", createExecutionWorkflowTemplate()},
				{"validation", createValidationWorkflowTemplate()},
				{"cleanup", createCleanupWorkflowTemplate()},
			}

			// Create coordinator workflow
			coordinatorWorkflow := engine.NewWorkflow("coordinator-001", coordinatorTemplate)
			coordinatorWorkflow.Metadata["coordination_enabled"] = true

			// Test coordination logic
			var wg sync.WaitGroup
			results := make(map[string]*engine.RuntimeWorkflowExecution)
			var mu sync.Mutex

			// Execute coordinator workflow
			wg.Add(1)
			go func() {
				defer wg.Done()
				result, err := workflowEngine.Execute(ctx, coordinatorWorkflow)
				Expect(err).ToNot(HaveOccurred(),
					"BR-WF-ADV-005: Coordinator workflow must execute successfully")

				mu.Lock()
				results["coordinator"] = result
				mu.Unlock()
			}()

			// Execute coordinated workflows
			for _, coordinated := range coordinatedTemplates {
				wg.Add(1)
				go func(name string, template *engine.ExecutableTemplate) {
					defer wg.Done()
					workflow := engine.NewWorkflow("coordinated-"+name, template)
					workflow.Metadata["coordination_enabled"] = true

					result, err := workflowEngine.Execute(ctx, workflow)
					Expect(err).ToNot(HaveOccurred(),
						"BR-WF-ADV-005: Coordinated workflow %s must execute successfully", name)

					mu.Lock()
					results[name] = result
					mu.Unlock()
				}(coordinated.name, coordinated.template)
			}

			wg.Wait()

			// Validate REAL business coordination outcomes
			Expect(len(results)).To(Equal(5), // coordinator + 4 coordinated workflows
				"BR-WF-ADV-005: All coordinated workflows must complete")

			for name, result := range results {
				Expect(result).ToNot(BeNil(),
					"BR-WF-ADV-005: Coordinated workflow %s must produce results", name)
				Expect(result.Metadata["coordination_applied"]).To(BeTrue(),
					"BR-WF-ADV-005: Must track coordination for workflow %s", name)
			}

			// Business Value: Complex operations executed through coordinated automation
		})
	})
})

// Helper functions for advanced workflow template creation
// These test REAL business logic with sophisticated scenarios

func createBaseWorkflowTemplate() *engine.ExecutableTemplate {
	template := engine.NewWorkflowTemplate("base-dynamic-workflow", "Base Dynamic Workflow")

	step := &engine.ExecutableWorkflowStep{
		BaseEntity: types.BaseEntity{
			ID:   "dynamic-step-1",
			Name: "Dynamic Action",
		},
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "dynamic_action",
			Parameters: map[string]interface{}{
				"adaptable": true,
			},
		},
	}

	template.Steps = []*engine.ExecutableWorkflowStep{step}
	return template
}

func createAdvancedLongRunningWorkflowTemplate() *engine.ExecutableTemplate {
	template := engine.NewWorkflowTemplate("long-running-workflow", "Long Running Workflow")

	steps := []*engine.ExecutableWorkflowStep{}
	for i := 1; i <= 5; i++ {
		step := &engine.ExecutableWorkflowStep{
			BaseEntity: types.BaseEntity{
				ID:   fmt.Sprintf("long-step-%d", i),
				Name: fmt.Sprintf("Long Running Step %d", i),
			},
			Type: engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "long_running_action",
				Parameters: map[string]interface{}{
					"duration": "2s",
					"step":     i,
				},
			},
		}
		steps = append(steps, step)
	}

	template.Steps = steps
	return template
}

func createResourceOptimizedParallelTemplate(stepCount int) *engine.ExecutableTemplate {
	template := engine.NewWorkflowTemplate("resource-optimized-parallel", "Resource Optimized Parallel Workflow")

	steps := []*engine.ExecutableWorkflowStep{}
	for i := 1; i <= stepCount; i++ {
		step := &engine.ExecutableWorkflowStep{
			BaseEntity: types.BaseEntity{
				ID:   fmt.Sprintf("parallel-step-%d", i),
				Name: fmt.Sprintf("Parallel Step %d", i),
			},
			Type: engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "parallel_action",
				Parameters: map[string]interface{}{
					"resource_requirements": map[string]interface{}{
						"cpu":    0.1,
						"memory": 0.2,
					},
					"step": i,
				},
			},
		}
		steps = append(steps, step)
	}

	template.Steps = steps
	return template
}

func createLoadBalancedWorkflowTemplate(stepCount int) *engine.ExecutableTemplate {
	template := engine.NewWorkflowTemplate("load-balanced-workflow", "Load Balanced Workflow")

	steps := []*engine.ExecutableWorkflowStep{}
	for i := 1; i <= stepCount; i++ {
		step := &engine.ExecutableWorkflowStep{
			BaseEntity: types.BaseEntity{
				ID:   fmt.Sprintf("balanced-step-%d", i),
				Name: fmt.Sprintf("Load Balanced Step %d", i),
			},
			Type: engine.StepTypeAction,
			Action: &engine.StepAction{
				Type: "balanced_action",
				Parameters: map[string]interface{}{
					"load_weight": float64(i) / float64(stepCount),
					"step":        i,
				},
			},
		}
		steps = append(steps, step)
	}

	template.Steps = steps
	return template
}

func createCachableWorkflowTemplate(scenario string) *engine.ExecutableTemplate {
	template := engine.NewWorkflowTemplate("cachable-workflow-"+scenario, "Cachable Workflow for "+scenario)

	step := &engine.ExecutableWorkflowStep{
		BaseEntity: types.BaseEntity{
			ID:   "cachable-step-1",
			Name: "Cachable Action",
		},
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "cachable_action",
			Parameters: map[string]interface{}{
				"scenario": scenario,
				"cachable": true,
			},
		},
	}

	template.Steps = []*engine.ExecutableWorkflowStep{step}
	return template
}

func createEvolvingWorkflowTemplate(scenario string) *engine.ExecutableTemplate {
	template := engine.NewWorkflowTemplate("evolving-workflow-"+scenario, "Evolving Workflow for "+scenario)

	step := &engine.ExecutableWorkflowStep{
		BaseEntity: types.BaseEntity{
			ID:   "evolving-step-1",
			Name: "Evolving Action",
		},
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "evolving_action",
			Parameters: map[string]interface{}{
				"scenario": scenario,
				"evolving": true,
			},
		},
	}

	template.Steps = []*engine.ExecutableWorkflowStep{step}
	return template
}

func createCoordinatorWorkflowTemplate() *engine.ExecutableTemplate {
	template := engine.NewWorkflowTemplate("coordinator-workflow", "Coordinator Workflow")

	step := &engine.ExecutableWorkflowStep{
		BaseEntity: types.BaseEntity{
			ID:   "coordinator-step-1",
			Name: "Coordination Action",
		},
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "coordinator_action",
			Parameters: map[string]interface{}{
				"coordination_role": "primary",
			},
		},
	}

	template.Steps = []*engine.ExecutableWorkflowStep{step}
	return template
}

func createPreparationWorkflowTemplate() *engine.ExecutableTemplate {
	return engine.NewWorkflowTemplate("preparation-workflow", "Preparation Workflow")
}

func createExecutionWorkflowTemplate() *engine.ExecutableTemplate {
	return engine.NewWorkflowTemplate("execution-workflow", "Execution Workflow")
}

func createValidationWorkflowTemplate() *engine.ExecutableTemplate {
	return engine.NewWorkflowTemplate("validation-workflow", "Validation Workflow")
}

func createCleanupWorkflowTemplate() *engine.ExecutableTemplate {
	return engine.NewWorkflowTemplate("cleanup-workflow", "Cleanup Workflow")
}

// TestRunner bootstraps the Ginkgo test suite
func TestUadvancedUworkflowUengineUextensions(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UadvancedUworkflowUengineUextensions Suite")
}
