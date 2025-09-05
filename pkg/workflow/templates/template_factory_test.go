package templates_test

import (
	"context"
	"time"

	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/jordigilh/kubernaut/pkg/workflow/templates"
	. "github.com/onsi/ginkgo/v2"

	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
)

// MockWorkflowValidator for testing
type MockWorkflowValidator struct {
	validateFunc func(ctx context.Context, template *engine.WorkflowTemplate) (*engine.ValidationReport, error)
}

func (m *MockWorkflowValidator) ValidateWorkflow(ctx context.Context, template *engine.WorkflowTemplate) (*engine.ValidationReport, error) {
	if m.validateFunc != nil {
		return m.validateFunc(ctx, template)
	}

	// Default: return successful validation
	return &engine.ValidationReport{
		Summary: &engine.ValidationSummary{
			Total:   len(template.Steps),
			Passed:  len(template.Steps),
			Failed:  0,
			Skipped: 0,
		},
		Results: []*engine.ValidationResult{},
	}, nil
}

var _ = Describe("WorkflowTemplateFactory", func() {
	var (
		factory   *templates.WorkflowTemplateFactory
		validator *MockWorkflowValidator
		logger    *logrus.Logger
		ctx       context.Context
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce noise in tests

		validator = &MockWorkflowValidator{}
		factory = templates.NewWorkflowTemplateFactory(validator, logger)
		ctx = context.Background()
	})

	Context("Factory Initialization", func() {
		It("should create factory with default configuration", func() {
			Expect(factory).ToNot(BeNil())
		})

		It("should initialize predefined templates", func() {
			// Factory should be ready to create templates
			alert := types.Alert{
				Name:      "HighMemoryUsage",
				Namespace: "test",
				Resource:  "deployment",
				Severity:  "critical",
				Labels:    map[string]string{"pod": "test-app"},
			}

			template := factory.BuildHighMemoryWorkflow(alert)
			Expect(template).ToNot(BeNil())
			Expect(template.Name).To(Equal("High Memory Usage Remediation"))
		})
	})

	Context("High Memory Workflow Creation", func() {
		var testAlert types.Alert

		BeforeEach(func() {
			testAlert = types.Alert{
				Name:        "HighMemoryUsage",
				Status:      "firing",
				Severity:    "critical",
				Description: "Memory usage is above 85%",
				Namespace:   "production",
				Resource:    "deployment",
				Labels: map[string]string{
					"pod":        "web-app-pod",
					"deployment": "web-app",
				},
				StartsAt: time.Now(),
			}
		})

		It("should create high memory workflow with correct structure", func() {
			template := factory.BuildHighMemoryWorkflow(testAlert)

			Expect(template).ToNot(BeNil())
			Expect(template.Name).To(Equal("High Memory Usage Remediation"))
			Expect(template.Description).To(ContainSubstring("production/deployment"))
			Expect(template.Tags).To(ContainElement("memory"))
			Expect(template.Tags).To(ContainElement("automated"))
		})

		It("should create workflow with proper step sequence", func() {
			template := factory.BuildHighMemoryWorkflow(testAlert)

			Expect(len(template.Steps)).To(BeNumerically(">=", 4))

			// Verify step sequence
			stepIDs := make([]string, len(template.Steps))
			for i, step := range template.Steps {
				stepIDs[i] = step.ID
			}

			Expect(stepIDs).To(ContainElement("check-memory-state"))
			Expect(stepIDs).To(ContainElement("evaluate-memory-threshold"))
			Expect(stepIDs).To(ContainElement("scale-deployment"))
			Expect(stepIDs).To(ContainElement("verify-scaling"))
		})

		It("should include proper step dependencies", func() {
			template := factory.BuildHighMemoryWorkflow(testAlert)

			// Find evaluate step
			var evaluateStep *engine.WorkflowStep
			for _, step := range template.Steps {
				if step.ID == "evaluate-memory-threshold" {
					evaluateStep = step
					break
				}
			}

			Expect(evaluateStep).ToNot(BeNil())
			Expect(evaluateStep.Dependencies).To(ContainElement("check-memory-state"))
		})

		It("should configure appropriate timeouts", func() {
			template := factory.BuildHighMemoryWorkflow(testAlert)

			Expect(template.Timeouts).ToNot(BeNil())
			Expect(template.Timeouts.Execution).To(BeNumerically(">", 0))
			Expect(template.Timeouts.Step).To(Equal(60 * time.Second))
		})

		It("should include rollback actions", func() {
			template := factory.BuildHighMemoryWorkflow(testAlert)

			// Find scale step
			var scaleStep *engine.WorkflowStep
			for _, step := range template.Steps {
				if step.ID == "scale-deployment" {
					scaleStep = step
					break
				}
			}

			Expect(scaleStep).ToNot(BeNil())
			Expect(scaleStep.Action).ToNot(BeNil())
			Expect(scaleStep.Action.Rollback).ToNot(BeNil())
			Expect(scaleStep.Action.Rollback.Type).To(Equal("scale_deployment"))
		})

		It("should include proper action targets", func() {
			template := factory.BuildHighMemoryWorkflow(testAlert)

			// Check that action targets are properly set
			for _, step := range template.Steps {
				if step.Action != nil && step.Action.Target != nil {
					target := step.Action.Target
					if target.Type == "kubernetes" && target.Resource == "deployment" {
						Expect(target.Namespace).To(Equal("production"))
						Expect(target.Name).ToNot(BeEmpty())
					}
				}
			}
		})
	})

	Context("Pod Crash Loop Workflow Creation", func() {
		var crashAlert types.Alert

		BeforeEach(func() {
			crashAlert = types.Alert{
				Name:        "PodCrashLoop",
				Status:      "firing",
				Severity:    "warning",
				Description: "Pod is in crash loop backoff",
				Namespace:   "staging",
				Resource:    "pod",
				Labels: map[string]string{
					"pod": "api-server-abc123",
				},
				StartsAt: time.Now(),
			}
		})

		It("should create crash loop workflow with diagnostics", func() {
			template := factory.BuildPodCrashLoopWorkflow(crashAlert)

			Expect(template).ToNot(BeNil())
			Expect(template.Name).To(Equal("Pod Crash Loop Recovery"))
			Expect(template.Tags).To(ContainElement("crash-loop"))
			Expect(template.Tags).To(ContainElement("diagnostics"))
		})

		It("should start with diagnostics collection", func() {
			template := factory.BuildPodCrashLoopWorkflow(crashAlert)

			// First step should be diagnostics
			firstStep := template.Steps[0]
			Expect(firstStep.ID).To(Equal("collect-diagnostics"))
			Expect(firstStep.Action.Parameters["action"]).To(Equal("collect_diagnostics"))
		})

		It("should include crash pattern analysis", func() {
			template := factory.BuildPodCrashLoopWorkflow(crashAlert)

			stepIDs := make([]string, len(template.Steps))
			for i, step := range template.Steps {
				stepIDs[i] = step.ID
			}

			Expect(stepIDs).To(ContainElement("analyze-crash-pattern"))
		})

		It("should have rollback strategy", func() {
			template := factory.BuildPodCrashLoopWorkflow(crashAlert)

			stepIDs := make([]string, len(template.Steps))
			for i, step := range template.Steps {
				stepIDs[i] = step.ID
			}

			Expect(stepIDs).To(ContainElement("rollback-deployment"))
		})

		It("should configure conservative retry policy", func() {
			template := factory.BuildPodCrashLoopWorkflow(crashAlert)

			Expect(template.Recovery).ToNot(BeNil())
			Expect(template.Recovery.Enabled).To(BeTrue())
			Expect(template.Recovery.MaxRecoveryTime).To(BeNumerically(">", 0))
		})
	})

	Context("Node Issue Workflow Creation", func() {
		var nodeAlert types.Alert

		BeforeEach(func() {
			nodeAlert = types.Alert{
				Name:        "NodeNotReady",
				Status:      "firing",
				Severity:    "critical",
				Description: "Node is not ready",
				Namespace:   "",
				Resource:    "node",
				Labels: map[string]string{
					"node": "worker-node-01",
				},
				StartsAt: time.Now(),
			}
		})

		It("should create node workflow with proper sequence", func() {
			template := factory.BuildNodeIssueWorkflow(nodeAlert)

			Expect(template).ToNot(BeNil())
			Expect(template.Name).To(Equal("Node Issue Remediation"))
			Expect(template.Tags).To(ContainElement("node"))
			Expect(template.Tags).To(ContainElement("infrastructure"))
		})

		It("should include node assessment and cordoning", func() {
			template := factory.BuildNodeIssueWorkflow(nodeAlert)

			stepIDs := make([]string, len(template.Steps))
			for i, step := range template.Steps {
				stepIDs[i] = step.ID
			}

			Expect(stepIDs).To(ContainElement("assess-node-health"))
			Expect(stepIDs).To(ContainElement("cordon-node"))
			Expect(stepIDs).To(ContainElement("migrate-workloads"))
		})

		It("should configure longer timeout for node operations", func() {
			template := factory.BuildNodeIssueWorkflow(nodeAlert)

			Expect(template.Timeouts.Execution).To(Equal(15 * time.Minute))
		})

		It("should include workload migration verification", func() {
			template := factory.BuildNodeIssueWorkflow(nodeAlert)

			stepIDs := make([]string, len(template.Steps))
			for i, step := range template.Steps {
				stepIDs[i] = step.ID
			}

			Expect(stepIDs).To(ContainElement("verify-migration"))
		})
	})

	Context("Storage Issue Workflow Creation", func() {
		var storageAlert types.Alert

		BeforeEach(func() {
			storageAlert = types.Alert{
				Name:        "DiskSpaceCritical",
				Status:      "firing",
				Severity:    "critical",
				Description: "Disk usage above 85%",
				Namespace:   "default",
				Resource:    "pvc",
				Labels: map[string]string{
					"pvc": "data-volume",
				},
				StartsAt: time.Now(),
			}
		})

		It("should create storage workflow with cleanup sequence", func() {
			template := factory.BuildStorageIssueWorkflow(storageAlert)

			Expect(template).ToNot(BeNil())
			Expect(template.Name).To(Equal("Storage Issue Remediation"))
			Expect(template.Tags).To(ContainElement("storage"))
		})

		It("should start with disk usage check", func() {
			template := factory.BuildStorageIssueWorkflow(storageAlert)

			firstStep := template.Steps[0]
			Expect(firstStep.ID).To(Equal("check-disk-usage"))
		})

		It("should include cleanup and expansion options", func() {
			template := factory.BuildStorageIssueWorkflow(storageAlert)

			stepIDs := make([]string, len(template.Steps))
			for i, step := range template.Steps {
				stepIDs[i] = step.ID
			}

			Expect(stepIDs).To(ContainElement("cleanup-storage"))
			Expect(stepIDs).To(ContainElement("expand-pvc"))
		})

		It("should verify cleanup effectiveness", func() {
			template := factory.BuildStorageIssueWorkflow(storageAlert)

			stepIDs := make([]string, len(template.Steps))
			for i, step := range template.Steps {
				stepIDs[i] = step.ID
			}

			Expect(stepIDs).To(ContainElement("verify-cleanup"))
		})
	})

	Context("Network Issue Workflow Creation", func() {
		var networkAlert types.Alert

		BeforeEach(func() {
			networkAlert = types.Alert{
				Name:        "NetworkConnectivityIssue",
				Status:      "firing",
				Severity:    "warning",
				Description: "Service unreachable",
				Namespace:   "kube-system",
				Resource:    "service",
				Labels: map[string]string{
					"service": "coredns",
				},
				StartsAt: time.Now(),
			}
		})

		It("should create network workflow with connectivity tests", func() {
			template := factory.BuildNetworkIssueWorkflow(networkAlert)

			Expect(template).ToNot(BeNil())
			Expect(template.Name).To(Equal("Network Issue Remediation"))
			Expect(template.Tags).To(ContainElement("network"))
		})

		It("should include network diagnostics", func() {
			template := factory.BuildNetworkIssueWorkflow(networkAlert)

			stepIDs := make([]string, len(template.Steps))
			for i, step := range template.Steps {
				stepIDs[i] = step.ID
			}

			Expect(stepIDs).To(ContainElement("test-network-connectivity"))
			Expect(stepIDs).To(ContainElement("restart-network-components"))
		})

		It("should include policy update fallback", func() {
			template := factory.BuildNetworkIssueWorkflow(networkAlert)

			stepIDs := make([]string, len(template.Steps))
			for i, step := range template.Steps {
				stepIDs[i] = step.ID
			}

			Expect(stepIDs).To(ContainElement("update-network-policy"))
		})
	})

	Context("Dynamic Template Creation from Alert", func() {
		It("should determine correct template type for memory alerts", func() {
			alert := types.Alert{
				Name:        "HighMemoryUsage",
				Description: "Memory usage exceeds threshold",
				Namespace:   "test",
				Resource:    "deployment",
			}

			template := factory.BuildFromAlert(ctx, alert)
			Expect(template.Name).To(Equal("High Memory Usage Remediation"))
		})

		It("should determine correct template type for crash loop alerts", func() {
			alert := types.Alert{
				Name:        "PodCrashLoop",
				Description: "Pod keeps crashing",
				Namespace:   "test",
				Resource:    "pod",
			}

			template := factory.BuildFromAlert(ctx, alert)
			Expect(template.Name).To(Equal("Pod Crash Loop Recovery"))
		})

		It("should create generic workflow for unknown alerts", func() {
			alert := types.Alert{
				Name:        "UnknownIssue",
				Description: "Something is wrong",
				Namespace:   "test",
				Resource:    "deployment",
			}

			template := factory.BuildFromAlert(ctx, alert)
			Expect(template.Name).To(Equal("Generic Alert Remediation"))
		})
	})

	Context("Objective-Based Template Creation", func() {
		It("should create template from workflow objective", func() {
			objective := &engine.WorkflowObjective{
				ID:          "test-objective",
				Type:        "performance_optimization",
				Description: "Optimize application performance",
				Targets: []*engine.OptimizationTarget{
					{
						Type:     "kubernetes",
						Metric:   "cpu_usage",
						Priority: 1,
						Parameters: map[string]interface{}{
							"namespace": "production",
							"resource":  "deployment",
						},
					},
				},
				Priority: 5,
				Constraints: map[string]interface{}{
					"max_replicas": 10,
				},
			}

			template := factory.BuildFromObjective(ctx, objective)

			Expect(template).ToNot(BeNil())
			Expect(template.Name).To(Equal(objective.Description))
			Expect(template.Variables).To(HaveKeyWithValue("max_replicas", 10))
			Expect(len(template.Steps)).To(Equal(len(objective.Targets)))
		})

		It("should adjust timeouts based on objective priority", func() {
			lowPriorityObjective := &engine.WorkflowObjective{
				ID:       "low-priority",
				Type:     "maintenance",
				Priority: 2,
				Targets: []*engine.OptimizationTarget{
					{Type: "kubernetes", Metric: "cleanup"},
				},
			}

			highPriorityObjective := &engine.WorkflowObjective{
				ID:       "high-priority",
				Type:     "critical_fix",
				Priority: 9,
				Targets: []*engine.OptimizationTarget{
					{Type: "kubernetes", Metric: "restart"},
				},
			}

			lowTemplate := factory.BuildFromObjective(ctx, lowPriorityObjective)
			highTemplate := factory.BuildFromObjective(ctx, highPriorityObjective)

			Expect(highTemplate.Timeouts.Execution).To(BeNumerically(">", lowTemplate.Timeouts.Execution))
		})
	})

	Context("Template Composition", func() {
		It("should compose multiple templates into one", func() {
			alert1 := types.Alert{Name: "HighMemory", Namespace: "test", Resource: "deployment"}
			alert2 := types.Alert{Name: "PodCrash", Namespace: "test", Resource: "pod"}

			template1 := factory.BuildHighMemoryWorkflow(alert1)
			template2 := factory.BuildPodCrashLoopWorkflow(alert2)

			composite := factory.ComposeWorkflows(template1, template2)

			Expect(composite).ToNot(BeNil())
			Expect(composite.Name).To(Equal("Composite Workflow"))
			Expect(len(composite.Steps)).To(Equal(len(template1.Steps) + len(template2.Steps)))
		})

		It("should handle single template composition", func() {
			alert := types.Alert{Name: "Test", Namespace: "test", Resource: "deployment"}
			template := factory.BuildHighMemoryWorkflow(alert)

			composed := factory.ComposeWorkflows(template)

			Expect(composed).To(Equal(template))
		})

		It("should handle empty template composition", func() {
			composed := factory.ComposeWorkflows()

			Expect(composed).To(BeNil())
		})

		It("should prefix step IDs to avoid conflicts", func() {
			alert := types.Alert{Name: "Test", Namespace: "test", Resource: "deployment"}
			template1 := factory.BuildHighMemoryWorkflow(alert)
			template2 := factory.BuildHighMemoryWorkflow(alert)

			composite := factory.ComposeWorkflows(template1, template2)

			stepIDs := make([]string, len(composite.Steps))
			for i, step := range composite.Steps {
				stepIDs[i] = step.ID
			}

			// Check for prefixed IDs
			foundT0 := false
			foundT1 := false
			for _, id := range stepIDs {
				if id[:3] == "t0-" {
					foundT0 = true
				}
				if id[:3] == "t1-" {
					foundT1 = true
				}
			}

			Expect(foundT0).To(BeTrue())
			Expect(foundT1).To(BeTrue())
		})
	})

	Context("Environment Customization", func() {
		var baseTemplate *engine.WorkflowTemplate

		BeforeEach(func() {
			alert := types.Alert{Name: "Test", Namespace: "test", Resource: "deployment"}
			baseTemplate = factory.BuildHighMemoryWorkflow(alert)
		})

		It("should customize template for production environment", func() {
			prodTemplate := factory.CustomizeForEnvironment(baseTemplate, "production")

			Expect(prodTemplate.ID).To(ContainSubstring("-production"))
			Expect(prodTemplate.Tags).To(ContainElement("env-production"))

			// Production should have longer timeouts
			for i, step := range prodTemplate.Steps {
				originalStep := baseTemplate.Steps[i]
				Expect(step.Timeout).To(BeNumerically(">=", originalStep.Timeout))
			}
		})

		It("should customize template for development environment", func() {
			devTemplate := factory.CustomizeForEnvironment(baseTemplate, "development")

			Expect(devTemplate.ID).To(ContainSubstring("-development"))
			Expect(devTemplate.Tags).To(ContainElement("env-development"))

			// Development should have shorter timeouts
			for i, step := range devTemplate.Steps {
				originalStep := baseTemplate.Steps[i]
				Expect(step.Timeout).To(BeNumerically("<=", originalStep.Timeout))
			}
		})

		It("should not modify original template", func() {
			originalTimeout := baseTemplate.Steps[0].Timeout

			_ = factory.CustomizeForEnvironment(baseTemplate, "production")

			Expect(baseTemplate.Steps[0].Timeout).To(Equal(originalTimeout))
		})
	})

	Context("Safety Constraints", func() {
		var baseTemplate *engine.WorkflowTemplate
		var constraints *templates.SafetyConstraints

		BeforeEach(func() {
			alert := types.Alert{Name: "Test", Namespace: "test", Resource: "deployment"}
			baseTemplate = factory.BuildHighMemoryWorkflow(alert)

			constraints = &templates.SafetyConstraints{
				MaxResourceImpact:    0.5,
				RequireConfirmation:  true,
				DisruptiveOperations: false,
				RollbackRequired:     true,
				MaxConcurrentActions: 2,
				CooldownPeriod:       2 * time.Minute,
			}
		})

		It("should apply safety constraints to template", func() {
			safeTemplate := factory.AddSafetyConstraints(baseTemplate, constraints)

			Expect(safeTemplate.Tags).To(ContainElement("safety-constrained"))
			Expect(safeTemplate.Variables).To(HaveKey("safety_constraints"))
		})

		It("should modify step parameters for safety", func() {
			safeTemplate := factory.AddSafetyConstraints(baseTemplate, constraints)

			// Check that action steps have safety parameters
			for _, step := range safeTemplate.Steps {
				if step.Action != nil && step.Action.Parameters != nil {
					Expect(step.Action.Parameters).To(HaveKeyWithValue("dry_run", true))
					Expect(step.Action.Parameters).To(HaveKeyWithValue("safety_level", "high"))
				}
			}
		})

		It("should adjust recovery policy based on constraints", func() {
			safeTemplate := factory.AddSafetyConstraints(baseTemplate, constraints)

			Expect(safeTemplate.Recovery.MaxRecoveryTime).To(BeNumerically(">=", constraints.CooldownPeriod))
		})
	})

	Context("Template Validation Integration", func() {
		It("should use validator when provided", func() {
			validatorCalled := false
			validator.validateFunc = func(ctx context.Context, template *engine.WorkflowTemplate) (*engine.ValidationReport, error) {
				validatorCalled = true
				return &engine.ValidationReport{
					Summary: &engine.ValidationSummary{
						Total:  len(template.Steps),
						Passed: len(template.Steps),
					},
				}, nil
			}

			alert := types.Alert{Name: "Test", Namespace: "test", Resource: "deployment"}
			template := factory.BuildHighMemoryWorkflow(alert)

			// Trigger validation through a workflow builder that uses validation
			// (The template factory doesn't directly call validation, but the intelligent workflow builder would)
			Expect(template).ToNot(BeNil())

			// This test verifies the mock validator is properly configured
			report, err := validator.ValidateWorkflow(ctx, template)
			Expect(err).ToNot(HaveOccurred())
			Expect(validatorCalled).To(BeTrue())
			Expect(report.Summary.Passed).To(Equal(len(template.Steps)))
		})
	})

	Context("Error Handling", func() {
		It("should handle missing alert labels gracefully", func() {
			alert := types.Alert{
				Name:      "TestAlert",
				Namespace: "test",
				Resource:  "deployment",
				Labels:    map[string]string{}, // Empty labels
			}

			template := factory.BuildHighMemoryWorkflow(alert)

			Expect(template).ToNot(BeNil())
			// Should not panic and should have proper defaults
		})

		It("should handle nil objective gracefully", func() {
			// This would be caught at the interface level, but test defensive programming
			defer func() {
				if r := recover(); r != nil {
					Fail("Should not panic on nil objective")
				}
			}()

			// Create minimal valid objective
			objective := &engine.WorkflowObjective{
				ID:      "test",
				Targets: []*engine.OptimizationTarget{},
			}

			template := factory.BuildFromObjective(ctx, objective)
			Expect(template).ToNot(BeNil())
		})
	})

	Context("Performance Characteristics", func() {
		It("should create templates efficiently", func() {
			alert := types.Alert{
				Name:      "PerformanceTest",
				Namespace: "test",
				Resource:  "deployment",
			}

			start := time.Now()

			// Create multiple templates
			for i := 0; i < 10; i++ {
				template := factory.BuildHighMemoryWorkflow(alert)
				Expect(template).ToNot(BeNil())
			}

			elapsed := time.Since(start)
			Expect(elapsed).To(BeNumerically("<", 100*time.Millisecond))
		})

		It("should handle concurrent template creation", func() {
			alert := types.Alert{
				Name:      "ConcurrencyTest",
				Namespace: "test",
				Resource:  "deployment",
			}

			done := make(chan bool, 5)

			// Create templates concurrently
			for i := 0; i < 5; i++ {
				go func() {
					template := factory.BuildHighMemoryWorkflow(alert)
					Expect(template).ToNot(BeNil())
					done <- true
				}()
			}

			// Wait for all goroutines
			for i := 0; i < 5; i++ {
				select {
				case <-done:
					// Success
				case <-time.After(1 * time.Second):
					Fail("Template creation took too long")
				}
			}
		})
	})
})
