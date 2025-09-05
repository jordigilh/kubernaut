//go:build integration

package integration_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/jordigilh/kubernaut/pkg/workflow/templates"
)

// RealWorkflowValidator implements actual validation logic
type RealWorkflowValidator struct {
	log *logrus.Logger
}

func NewRealWorkflowValidator(log *logrus.Logger) *RealWorkflowValidator {
	return &RealWorkflowValidator{log: log}
}

func (v *RealWorkflowValidator) ValidateWorkflow(ctx context.Context, template *engine.WorkflowTemplate) (*engine.ValidationReport, error) {
	var results []*engine.ValidationResult
	passed := 0
	failed := 0

	// Validate each step
	for _, step := range template.Steps {
		stepPassed, stepResults := v.validateStep(step)
		results = append(results, stepResults...)

		if stepPassed {
			passed++
		} else {
			failed++
		}
	}

	// Validate overall template structure
	structurePassed, structureResults := v.validateTemplateStructure(template)
	results = append(results, structureResults...)

	if !structurePassed {
		failed++
	} else {
		passed++
	}

	report := &engine.ValidationReport{
		Summary: &engine.ValidationSummary{
			Total:   len(template.Steps) + 1, // +1 for structure validation
			Passed:  passed,
			Failed:  failed,
			Skipped: 0,
		},
		Results: results,
	}

	return report, nil
}

func (v *RealWorkflowValidator) validateStep(step *engine.WorkflowStep) (bool, []*engine.ValidationResult) {
	var results []*engine.ValidationResult
	allPassed := true

	// Check step ID
	if step.ID == "" {
		results = append(results, &engine.ValidationResult{
			RuleID:    "step-id-required",
			Type:      engine.ValidationTypeIntegrity,
			Passed:    false,
			Message:   "Step ID cannot be empty",
			Timestamp: time.Now(),
		})
		allPassed = false
	}

	// Check step name
	if step.Name == "" {
		results = append(results, &engine.ValidationResult{
			RuleID:    "step-name-recommended",
			Type:      engine.ValidationTypeIntegrity,
			Passed:    false,
			Message:   "Step name is empty",
			Timestamp: time.Now(),
		})
	}

	// Check timeout
	if step.Timeout <= 0 {
		results = append(results, &engine.ValidationResult{
			RuleID:    "step-timeout-recommended",
			Type:      engine.ValidationTypeIntegrity,
			Passed:    false,
			Message:   "Step timeout not specified",
			Timestamp: time.Now(),
		})
	}

	// Validate action steps
	if step.Type == engine.StepTypeAction && step.Action == nil {
		results = append(results, &engine.ValidationResult{
			RuleID:    "action-step-required",
			Type:      engine.ValidationTypeIntegrity,
			Passed:    false,
			Message:   "Action step must have action defined",
			Timestamp: time.Now(),
		})
		allPassed = false
	}

	// Validate action parameters
	if step.Action != nil {
		if step.Action.Type == "" {
			results = append(results, &engine.ValidationResult{
				RuleID:    "action-type-required",
				Type:      engine.ValidationTypeIntegrity,
				Passed:    false,
				Message:   "Action type cannot be empty",
				Timestamp: time.Now(),
			})
			allPassed = false
		}

		// Validate Kubernetes actions
		if step.Action.Type == "kubernetes" && step.Action.Target == nil {
			results = append(results, &engine.ValidationResult{
				RuleID:    "kubernetes-target-required",
				Type:      engine.ValidationTypeIntegrity,
				Passed:    false,
				Message:   "Kubernetes action must have target defined",
				Timestamp: time.Now(),
			})
			allPassed = false
		}
	}

	// Validate condition steps
	if step.Type == engine.StepTypeCondition && step.Condition == nil {
		results = append(results, &engine.ValidationResult{
			RuleID:    "condition-step-required",
			Type:      engine.ValidationTypeIntegrity,
			Passed:    false,
			Message:   "Condition step must have condition defined",
			Timestamp: time.Now(),
		})
		allPassed = false
	}

	return allPassed, results
}

func (v *RealWorkflowValidator) validateTemplateStructure(template *engine.WorkflowTemplate) (bool, []*engine.ValidationResult) {
	var results []*engine.ValidationResult
	allPassed := true

	// Check template ID
	if template.ID == "" {
		results = append(results, &engine.ValidationResult{
			RuleID:    "template-id-required",
			Type:      engine.ValidationTypeIntegrity,
			Passed:    false,
			Message:   "Template ID cannot be empty",
			Timestamp: time.Now(),
		})
		allPassed = false
	}

	// Check template name
	if template.Name == "" {
		results = append(results, &engine.ValidationResult{
			RuleID:    "template-name-recommended",
			Type:      engine.ValidationTypeIntegrity,
			Passed:    false,
			Message:   "Template name is empty",
			Timestamp: time.Now(),
		})
	}

	// Check steps exist
	if len(template.Steps) == 0 {
		results = append(results, &engine.ValidationResult{
			RuleID:    "template-steps-required",
			Type:      engine.ValidationTypeIntegrity,
			Passed:    false,
			Message:   "Template must have at least one step",
			Timestamp: time.Now(),
		})
		allPassed = false
	}

	// Validate step dependencies
	stepIDs := make(map[string]bool)
	for _, step := range template.Steps {
		stepIDs[step.ID] = true
	}

	for _, step := range template.Steps {
		for _, dep := range step.Dependencies {
			if !stepIDs[dep] {
				results = append(results, &engine.ValidationResult{
					RuleID:    "dependency-exists",
					Type:      engine.ValidationTypeIntegrity,
					Passed:    false,
					Message:   "Step dependency references non-existent step: " + dep,
					Timestamp: time.Now(),
				})
				allPassed = false
			}
		}
	}

	// Check for circular dependencies (basic check)
	if v.hasCircularDependencies(template.Steps) {
		results = append(results, &engine.ValidationResult{
			RuleID:    "no-circular-dependencies",
			Type:      engine.ValidationTypeIntegrity,
			Passed:    false,
			Message:   "Template contains circular dependencies",
			Timestamp: time.Now(),
		})
		allPassed = false
	}

	return allPassed, results
}

func (v *RealWorkflowValidator) hasCircularDependencies(steps []*engine.WorkflowStep) bool {
	// Simple cycle detection using DFS
	visited := make(map[string]bool)
	recursionStack := make(map[string]bool)

	// Build adjacency list
	graph := make(map[string][]string)
	for _, step := range steps {
		graph[step.ID] = step.Dependencies
	}

	var dfs func(string) bool
	dfs = func(stepID string) bool {
		visited[stepID] = true
		recursionStack[stepID] = true

		for _, dep := range graph[stepID] {
			if !visited[dep] {
				if dfs(dep) {
					return true
				}
			} else if recursionStack[dep] {
				return true
			}
		}

		recursionStack[stepID] = false
		return false
	}

	for _, step := range steps {
		if !visited[step.ID] {
			if dfs(step.ID) {
				return true
			}
		}
	}

	return false
}

var _ = Describe("Template Factory Integration Tests", func() {
	var (
		factory    *templates.WorkflowTemplateFactory
		validator  *RealWorkflowValidator
		logger     *logrus.Logger
		ctx        context.Context
		testReport *IntegrationTestReport
	)

	BeforeSuite(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)

		// Initialize test report
		testReport = &IntegrationTestReport{
			SuiteName:   "Template Factory Integration",
			StartTime:   time.Now(),
			TestResults: []TestResult{},
		}

		logger.Info("Starting Template Factory Integration Test Suite")
	})

	BeforeEach(func() {
		validator = NewRealWorkflowValidator(logger)
		factory = templates.NewWorkflowTemplateFactory(validator, logger)
		ctx = context.Background()

		testReport.TotalTests++
	})

	Context("Real Workflow Validation", func() {
		It("should validate high memory workflow with real validator", func() {
			By("Creating high memory workflow")
			alert := types.Alert{
				Name:        "HighMemoryUsage",
				Status:      "firing",
				Severity:    "critical",
				Description: "Memory usage exceeds 85%",
				Namespace:   "production",
				Resource:    "deployment",
				Labels: map[string]string{
					"deployment": "api-server",
					"pod":        "api-server-abc123",
				},
				StartsAt: time.Now(),
			}

			startTime := time.Now()
			template := factory.BuildHighMemoryWorkflow(alert)
			creationTime := time.Since(startTime)

			Expect(template).ToNot(BeNil())

			By("Validating workflow with real validator")
			validationStart := time.Now()
			report, err := validator.ValidateWorkflow(ctx, template)
			validationTime := time.Since(validationStart)

			Expect(err).ToNot(HaveOccurred())
			Expect(report).ToNot(BeNil())
			Expect(report.Summary.Failed).To(Equal(0),
				"Validation should pass for high memory workflow")

			By("Recording performance metrics")
			testReport.RecordTest(TestResult{
				Name:     "HighMemoryWorkflowValidation",
				Passed:   true,
				Duration: creationTime + validationTime,
				Details: map[string]interface{}{
					"creation_time":     creationTime.String(),
					"validation_time":   validationTime.String(),
					"steps_count":       len(template.Steps),
					"validation_passed": report.Summary.Passed,
					"validation_failed": report.Summary.Failed,
				},
			})

			logger.WithFields(logrus.Fields{
				"creation_time":     creationTime,
				"validation_time":   validationTime,
				"steps":             len(template.Steps),
				"validation_passed": report.Summary.Passed,
			}).Info("High memory workflow validation completed")
		})

		It("should validate crash loop workflow structure", func() {
			By("Creating crash loop workflow")
			alert := types.Alert{
				Name:        "PodCrashLoop",
				Status:      "firing",
				Severity:    "warning",
				Description: "Pod crash loop detected",
				Namespace:   "staging",
				Resource:    "pod",
				Labels: map[string]string{
					"pod": "web-app-xyz789",
				},
				StartsAt: time.Now(),
			}

			startTime := time.Now()
			template := factory.BuildPodCrashLoopWorkflow(alert)
			creationTime := time.Since(startTime)

			By("Validating crash loop workflow")
			report, err := validator.ValidateWorkflow(ctx, template)

			Expect(err).ToNot(HaveOccurred())
			Expect(report.Summary.Failed).To(Equal(0))

			By("Checking diagnostics step is first")
			Expect(template.Steps[0].ID).To(Equal("collect-diagnostics"))
			Expect(template.Steps[0].Action.Parameters["action"]).To(Equal("collect_diagnostics"))

			By("Verifying step dependencies are valid")
			stepMap := make(map[string]*engine.WorkflowStep)
			for _, step := range template.Steps {
				stepMap[step.ID] = step
			}

			for _, step := range template.Steps {
				for _, dep := range step.Dependencies {
					_, exists := stepMap[dep]
					Expect(exists).To(BeTrue(),
						"Dependency %s should exist for step %s", dep, step.ID)
				}
			}

			testReport.RecordTest(TestResult{
				Name:     "CrashLoopWorkflowValidation",
				Passed:   true,
				Duration: creationTime,
				Details: map[string]interface{}{
					"steps_count":     len(template.Steps),
					"has_diagnostics": true,
					"has_rollback":    containsStepID(template.Steps, "rollback-deployment"),
				},
			})
		})

		It("should validate node issue workflow with proper sequence", func() {
			By("Creating node issue workflow")
			alert := types.Alert{
				Name:        "NodeNotReady",
				Status:      "firing",
				Severity:    "critical",
				Description: "Node not ready",
				Resource:    "node",
				Labels: map[string]string{
					"node": "worker-01",
				},
				StartsAt: time.Now(),
			}

			template := factory.BuildNodeIssueWorkflow(alert)

			By("Validating node workflow structure")
			report, err := validator.ValidateWorkflow(ctx, template)

			Expect(err).ToNot(HaveOccurred())
			Expect(report.Summary.Failed).To(Equal(0))

			By("Verifying node workflow sequence")
			expectedSequence := []string{
				"assess-node-health",
				"evaluate-node-condition",
				"cordon-node",
				"migrate-workloads",
				"verify-migration",
			}

			stepIDs := make([]string, len(template.Steps))
			for i, step := range template.Steps {
				stepIDs[i] = step.ID
			}

			for _, expectedID := range expectedSequence {
				Expect(stepIDs).To(ContainElement(expectedID))
			}

			By("Checking migration step has proper timeout")
			migrateStep := findStepByID(template.Steps, "migrate-workloads")
			Expect(migrateStep).ToNot(BeNil())
			Expect(migrateStep.Timeout).To(Equal(5 * time.Minute))

			testReport.RecordTest(TestResult{
				Name:     "NodeIssueWorkflowValidation",
				Passed:   true,
				Duration: time.Since(time.Now()),
				Details: map[string]interface{}{
					"expected_sequence_found": true,
					"migration_timeout":       migrateStep.Timeout.String(),
					"execution_timeout":       template.Timeouts.Execution.String(),
				},
			})
		})
	})

	Context("Workflow Composition Integration", func() {
		It("should compose multiple workflows with validation", func() {
			By("Creating base workflows")
			memoryAlert := types.Alert{
				Name: "HighMemory", Namespace: "prod", Resource: "deployment",
				Labels: map[string]string{"deployment": "api"},
			}
			crashAlert := types.Alert{
				Name: "CrashLoop", Namespace: "prod", Resource: "pod",
				Labels: map[string]string{"pod": "worker"},
			}

			memoryTemplate := factory.BuildHighMemoryWorkflow(memoryAlert)
			crashTemplate := factory.BuildPodCrashLoopWorkflow(crashAlert)

			By("Composing workflows")
			startTime := time.Now()
			compositeTemplate := factory.ComposeWorkflows(memoryTemplate, crashTemplate)
			compositionTime := time.Since(startTime)

			Expect(compositeTemplate).ToNot(BeNil())

			By("Validating composite workflow")
			report, err := validator.ValidateWorkflow(ctx, compositeTemplate)

			Expect(err).ToNot(HaveOccurred())

			By("Checking step ID prefixing")
			prefixedSteps := 0
			for _, step := range compositeTemplate.Steps {
				if len(step.ID) > 3 && (step.ID[:3] == "t0-" || step.ID[:3] == "t1-") {
					prefixedSteps++
				}
			}

			Expect(prefixedSteps).To(BeNumerically(">", 0))

			By("Verifying no circular dependencies")
			Expect(report.Summary.Failed).To(Equal(0))

			testReport.RecordTest(TestResult{
				Name:     "WorkflowComposition",
				Passed:   true,
				Duration: compositionTime,
				Details: map[string]interface{}{
					"original_steps":    len(memoryTemplate.Steps) + len(crashTemplate.Steps),
					"composite_steps":   len(compositeTemplate.Steps),
					"prefixed_steps":    prefixedSteps,
					"validation_passed": report.Summary.Passed,
				},
			})
		})

		It("should handle complex workflow composition", func() {
			By("Creating multiple different workflows")
			alerts := []types.Alert{
				{Name: "MemoryAlert", Namespace: "ns1", Resource: "deployment"},
				{Name: "StorageAlert", Namespace: "ns2", Resource: "pvc"},
				{Name: "NetworkAlert", Namespace: "ns3", Resource: "service"},
			}

			var templates []*engine.WorkflowTemplate
			for _, alert := range alerts {
				template := factory.BuildFromAlert(ctx, alert)
				templates = append(templates, template)
			}

			By("Composing all workflows")
			compositeTemplate := factory.ComposeWorkflows(templates...)

			By("Validating complex composition")
			report, err := validator.ValidateWorkflow(ctx, compositeTemplate)

			Expect(err).ToNot(HaveOccurred())
			Expect(compositeTemplate).ToNot(BeNil())

			totalOriginalSteps := 0
			for _, template := range templates {
				totalOriginalSteps += len(template.Steps)
			}

			Expect(len(compositeTemplate.Steps)).To(Equal(totalOriginalSteps))

			testReport.RecordTest(TestResult{
				Name:     "ComplexWorkflowComposition",
				Passed:   true,
				Duration: time.Since(time.Now()),
				Details: map[string]interface{}{
					"source_workflows":  len(templates),
					"total_steps":       len(compositeTemplate.Steps),
					"validation_errors": report.Summary.Failed,
				},
			})
		})
	})

	Context("Environment Customization Integration", func() {
		It("should customize workflows for different environments", func() {
			By("Creating base workflow")
			alert := types.Alert{
				Name: "TestAlert", Namespace: "default", Resource: "deployment",
			}
			baseTemplate := factory.BuildHighMemoryWorkflow(alert)

			environments := []string{"production", "staging", "development"}
			results := make(map[string]*engine.WorkflowTemplate)

			By("Customizing for each environment")
			for _, env := range environments {
				customizedTemplate := factory.CustomizeForEnvironment(baseTemplate, env)
				results[env] = customizedTemplate

				// Validate each customized template
				report, err := validator.ValidateWorkflow(ctx, customizedTemplate)
				Expect(err).ToNot(HaveOccurred())
				Expect(report.Summary.Failed).To(Equal(0))
			}

			By("Verifying environment-specific customizations")

			// Production should have longer timeouts
			prodTimeouts := results["production"].Steps[0].Timeout
			devTimeouts := results["development"].Steps[0].Timeout
			Expect(prodTimeouts).To(BeNumerically(">", devTimeouts))

			// All should have environment tags
			for env, template := range results {
				Expect(template.Tags).To(ContainElement("env-" + env))
				Expect(template.ID).To(ContainSubstring("-" + env))
			}

			testReport.RecordTest(TestResult{
				Name:     "EnvironmentCustomization",
				Passed:   true,
				Duration: time.Since(time.Now()),
				Details: map[string]interface{}{
					"environments_tested":    len(environments),
					"customizations_applied": true,
					"timeout_differences": map[string]string{
						"production":  prodTimeouts.String(),
						"development": devTimeouts.String(),
					},
				},
			})
		})
	})

	Context("Safety Constraints Integration", func() {
		It("should apply and validate safety constraints", func() {
			By("Creating workflow with safety constraints")
			alert := types.Alert{
				Name: "SafetyTest", Namespace: "production", Resource: "deployment",
			}
			baseTemplate := factory.BuildHighMemoryWorkflow(alert)

			constraints := &templates.SafetyConstraints{
				MaxResourceImpact:    0.3,
				RequireConfirmation:  true,
				DisruptiveOperations: false,
				RollbackRequired:     true,
				MaxConcurrentActions: 1,
				CooldownPeriod:       5 * time.Minute,
			}

			By("Applying safety constraints")
			safeTemplate := factory.AddSafetyConstraints(baseTemplate, constraints)

			By("Validating safety-constrained workflow")
			report, err := validator.ValidateWorkflow(ctx, safeTemplate)

			Expect(err).ToNot(HaveOccurred())
			Expect(report.Summary.Failed).To(Equal(0))

			By("Verifying safety modifications")
			Expect(safeTemplate.Tags).To(ContainElement("safety-constrained"))
			Expect(safeTemplate.Variables).To(HaveKey("safety_constraints"))
			Expect(safeTemplate.Recovery.MaxRecoveryTime).To(Equal(constraints.CooldownPeriod))

			// Check safety parameters are applied to action steps
			safetyParametersFound := false
			for _, step := range safeTemplate.Steps {
				if step.Action != nil && step.Action.Parameters != nil {
					if dryRun, exists := step.Action.Parameters["dry_run"]; exists && dryRun == true {
						safetyParametersFound = true
						break
					}
				}
			}
			Expect(safetyParametersFound).To(BeTrue())

			testReport.RecordTest(TestResult{
				Name:     "SafetyConstraintsIntegration",
				Passed:   true,
				Duration: time.Since(time.Now()),
				Details: map[string]interface{}{
					"safety_parameters_applied": safetyParametersFound,
					"cooldown_period":           constraints.CooldownPeriod.String(),
					"dry_run_enabled":           true,
				},
			})
		})
	})

	Context("Performance and Load Testing", func() {
		It("should handle high-volume template creation", func() {
			By("Creating large number of templates")
			templateCount := 50
			startTime := time.Now()

			var createdTemplates []*engine.WorkflowTemplate
			for i := 0; i < templateCount; i++ {
				alert := types.Alert{
					Name:      "LoadTest",
					Namespace: "test",
					Resource:  "deployment",
					Labels:    map[string]string{"instance": string(rune(i))},
				}

				template := factory.BuildFromAlert(ctx, alert)
				createdTemplates = append(createdTemplates, template)
			}

			totalTime := time.Since(startTime)
			avgTime := totalTime / time.Duration(templateCount)

			By("Validating performance characteristics")
			Expect(len(createdTemplates)).To(Equal(templateCount))
			Expect(avgTime).To(BeNumerically("<", 10*time.Millisecond))

			By("Validating all created templates")
			validationStart := time.Now()
			validTemplates := 0

			for _, template := range createdTemplates {
				report, err := validator.ValidateWorkflow(ctx, template)
				if err == nil && report.Summary.Failed == 0 {
					validTemplates++
				}
			}

			validationTime := time.Since(validationStart)

			Expect(validTemplates).To(Equal(templateCount))

			testReport.RecordTest(TestResult{
				Name:     "HighVolumeTemplateCreation",
				Passed:   true,
				Duration: totalTime + validationTime,
				Details: map[string]interface{}{
					"templates_created": templateCount,
					"total_time":        totalTime.String(),
					"avg_creation_time": avgTime.String(),
					"validation_time":   validationTime.String(),
					"valid_templates":   validTemplates,
				},
			})
		})

		It("should handle concurrent template operations", func() {
			By("Running concurrent template creation and validation")
			concurrency := 10
			done := make(chan TestResult, concurrency)

			startTime := time.Now()

			for i := 0; i < concurrency; i++ {
				go func(id int) {
					operationStart := time.Now()

					alert := types.Alert{
						Name:      "ConcurrencyTest",
						Namespace: "test",
						Resource:  "deployment",
						Labels:    map[string]string{"worker": string(rune(id))},
					}

					// Create and validate template
					template := factory.BuildHighMemoryWorkflow(alert)
					report, err := validator.ValidateWorkflow(ctx, template)

					operationTime := time.Since(operationStart)
					success := err == nil && report.Summary.Failed == 0

					done <- TestResult{
						Name:     "ConcurrentOperation",
						Passed:   success,
						Duration: operationTime,
						Details: map[string]interface{}{
							"worker_id": id,
							"steps":     len(template.Steps),
						},
					}
				}(i)
			}

			By("Collecting results from concurrent operations")
			var results []TestResult
			for i := 0; i < concurrency; i++ {
				select {
				case result := <-done:
					results = append(results, result)
				case <-time.After(5 * time.Second):
					Fail("Concurrent operation timed out")
				}
			}

			totalConcurrentTime := time.Since(startTime)

			By("Verifying all operations succeeded")
			successCount := 0
			totalOperationTime := time.Duration(0)

			for _, result := range results {
				if result.Passed {
					successCount++
				}
				totalOperationTime += result.Duration
			}

			Expect(successCount).To(Equal(concurrency))
			avgOperationTime := totalOperationTime / time.Duration(concurrency)

			testReport.RecordTest(TestResult{
				Name:     "ConcurrentTemplateOperations",
				Passed:   true,
				Duration: totalConcurrentTime,
				Details: map[string]interface{}{
					"concurrency":           concurrency,
					"successful_operations": successCount,
					"total_concurrent_time": totalConcurrentTime.String(),
					"avg_operation_time":    avgOperationTime.String(),
				},
			})
		})
	})

	AfterSuite(func() {
		By("Generating integration test report")
		testReport.EndTime = time.Now()
		testReport.Duration = testReport.EndTime.Sub(testReport.StartTime)

		// Calculate success rate
		passedTests := 0
		for _, result := range testReport.TestResults {
			if result.Passed {
				passedTests++
			}
		}
		testReport.PassRate = float64(passedTests) / float64(testReport.TotalTests)

		logger.WithFields(logrus.Fields{
			"total_tests":  testReport.TotalTests,
			"passed_tests": passedTests,
			"pass_rate":    testReport.PassRate,
			"duration":     testReport.Duration.String(),
		}).Info("Template Factory Integration Tests Completed")

		// Print summary
		By("Template Factory Integration Test Summary")
		Expect(testReport.PassRate).To(BeNumerically(">=", 0.95)) // 95% success rate required
	})
})

// Helper functions for integration tests

func containsStepID(steps []*engine.WorkflowStep, stepID string) bool {
	for _, step := range steps {
		if step.ID == stepID {
			return true
		}
	}
	return false
}

func findStepByID(steps []*engine.WorkflowStep, stepID string) *engine.WorkflowStep {
	for _, step := range steps {
		if step.ID == stepID {
			return step
		}
	}
	return nil
}

// IntegrationTestReport tracks test execution metrics
type IntegrationTestReport struct {
	SuiteName   string
	StartTime   time.Time
	EndTime     time.Time
	Duration    time.Duration
	TotalTests  int
	PassRate    float64
	TestResults []TestResult
}

type TestResult struct {
	Name     string
	Passed   bool
	Duration time.Duration
	Details  map[string]interface{}
}

func (r *IntegrationTestReport) RecordTest(result TestResult) {
	r.TestResults = append(r.TestResults, result)
}
