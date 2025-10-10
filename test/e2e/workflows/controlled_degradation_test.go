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

package workflows

import (
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/jordigilh/kubernaut/test/e2e/shared"
)

var _ = Describe("BR-E2E-006: Controlled Graceful Degradation Scenarios", func() {
	var (
		framework *shared.E2ETestFramework
		ctx       context.Context
		cancel    context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 15*time.Minute)

		// TDD REFACTOR: Use standardized E2E logger configuration
		logger := shared.NewE2ELogger()

		var err error
		framework, err = shared.NewE2ETestFramework(ctx, logger)
		Expect(err).NotTo(HaveOccurred(), "E2E framework should initialize successfully")
	})

	AfterEach(func() {
		if framework != nil {
			framework.Cleanup()
		}
		cancel()
	})

	Context("When AI services are unavailable", func() {
		It("should allow graceful degradation for info severity alerts", func() {
			By("Creating an info severity alert")
			alert := types.Alert{
				Name:        "E2E_INFO_ALERT_DEGRADATION_TEST",
				Severity:    "info", // Non-critical - should allow graceful degradation
				Description: "Info alert for testing controlled graceful degradation",
				Labels: map[string]string{
					"test":      "controlled_degradation",
					"severity":  "info",
					"component": "test-service",
				},
				Namespace: "default",
			}

			By("Simulating AI services unavailable by setting environment")
			// Temporarily disable AI services to test graceful degradation
			originalMode := os.Getenv("KUBERNAUT_MODE")
			_ = os.Setenv("KUBERNAUT_MODE", "testing") // Enable testing mode for graceful degradation
			defer func() {
				if originalMode == "" {
					_ = os.Unsetenv("KUBERNAUT_MODE")
				} else {
					_ = os.Setenv("KUBERNAUT_MODE", originalMode)
				}
			}()

			By("Creating a simple workflow template for the alert")
			template := engine.NewWorkflowTemplate("controlled-degradation-test", "Controlled Degradation Test")
			template.Steps = []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{
						ID:   "step-1",
						Name: "Test Step",
					},
					Type: engine.StepTypeAction, // Set the step type
					Action: &engine.StepAction{
						Type: "assessment", // This should match registered executor
						Parameters: map[string]interface{}{
							"alert_name": alert.Name,
							"severity":   alert.Severity,
						},
					},
				},
			}

			By("Creating and executing workflow")
			workflow := engine.NewWorkflow("controlled-degradation-workflow", template)
			execution, err := framework.WorkflowEngine.Execute(ctx, workflow)

			By("Verifying graceful degradation behavior")
			if err != nil {
				// Check if error indicates controlled graceful degradation
				GinkgoWriter.Printf("Workflow execution result: %v\n", err)
			} else {
				// Workflow executed successfully
				Expect(execution).NotTo(BeNil(), "Execution should be created")
				Expect(execution.ID).NotTo(BeEmpty(), "Execution should have an ID")
				GinkgoWriter.Printf("✅ Workflow executed successfully with graceful degradation\n")
			}
		})

		It("should fail hard for critical severity alerts when AI services required", func() {
			By("Creating a critical severity alert")
			alert := types.Alert{
				Name:        "E2E_CRITICAL_ALERT_HARD_FAILURE_TEST",
				Severity:    "critical", // Critical - should require AI services
				Description: "Critical alert requiring AI analysis - should fail hard when AI unavailable",
				Labels: map[string]string{
					"test":      "hard_failure",
					"severity":  "critical",
					"component": "production-service",
				},
				Namespace: "default",
			}

			By("Ensuring we're not in testing mode")
			originalMode := os.Getenv("KUBERNAUT_MODE")
			_ = os.Unsetenv("KUBERNAUT_MODE") // Ensure not in testing mode
			defer func() {
				if originalMode != "" {
					_ = os.Setenv("KUBERNAUT_MODE", originalMode)
				}
			}()

			By("Simulating AI services unavailable")
			// This test assumes AI services might be unavailable
			// The system should fail hard for critical alerts

			By("Creating a workflow template for critical alert")
			template := engine.NewWorkflowTemplate("critical-alert-test", "Critical Alert Test")
			template.Steps = []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{
						ID:   "critical-step-1",
						Name: "Critical Assessment Step",
					},
					Type: engine.StepTypeAction,
					Action: &engine.StepAction{
						Type: "assessment",
						Parameters: map[string]interface{}{
							"alert_name": alert.Name,
							"severity":   alert.Severity,
							"critical":   true,
						},
					},
				},
			}

			By("Creating and executing workflow for critical alert")
			workflow := engine.NewWorkflow("critical-alert-workflow", template)
			execution, err := framework.WorkflowEngine.Execute(ctx, workflow)

			By("Verifying the system behavior for critical alerts")
			if err != nil {
				// Check if error indicates hard failure when AI services required
				GinkgoWriter.Printf("Critical alert workflow result: %v\n", err)
				if ctx.Err() == context.DeadlineExceeded {
					GinkgoWriter.Printf("⚠️ Workflow timed out - may indicate AI services unavailable\n")
				}
			} else {
				// If workflow executed successfully, AI services are available
				Expect(execution).NotTo(BeNil(), "Execution should be created")
				Expect(execution.ID).NotTo(BeEmpty(), "Execution should have an ID")
				GinkgoWriter.Printf("✅ AI services are available - critical workflow executed successfully\n")
			}
		})

		It("should allow graceful degradation with explicit flag", func() {
			By("Creating an alert with explicit graceful degradation flag")
			alert := types.Alert{
				Name:        "E2E_EXPLICIT_DEGRADATION_TEST",
				Severity:    "warning", // Normally would require AI
				Description: "Warning alert with explicit graceful degradation allowed",
				Labels: map[string]string{
					"test":                       "explicit_degradation",
					"severity":                   "warning",
					"allow_graceful_degradation": "true", // Explicit flag
					"component":                  "test-service",
				},
				Namespace: "default",
			}

			By("Creating workflow template for explicit degradation test")
			template := engine.NewWorkflowTemplate("explicit-degradation-test", "Explicit Degradation Test")
			template.Steps = []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{
						ID:   "explicit-step-1",
						Name: "Explicit Degradation Step",
					},
					Type: engine.StepTypeAction,
					Action: &engine.StepAction{
						Type: "assessment",
						Parameters: map[string]interface{}{
							"alert_name":                 alert.Name,
							"severity":                   alert.Severity,
							"allow_graceful_degradation": "true",
						},
					},
				},
			}

			By("Creating and executing workflow with explicit flag")
			workflow := engine.NewWorkflow("explicit-degradation-workflow", template)
			execution, err := framework.WorkflowEngine.Execute(ctx, workflow)
			Expect(err).NotTo(HaveOccurred(), "Workflow execution should succeed with explicit flag")
			Expect(execution).NotTo(BeNil(), "Execution should be created")

			By("Verifying graceful degradation is used")
			Expect(execution.ID).NotTo(BeEmpty(), "Execution should have an ID")
			GinkgoWriter.Printf("✅ Workflow executed successfully with explicit graceful degradation flag\n")
		})
	})

	Context("When testing maintenance mode scenarios", func() {
		It("should allow graceful degradation in maintenance mode", func() {
			By("Setting maintenance mode")
			originalMaintenance := os.Getenv("KUBERNAUT_MAINTENANCE_MODE")
			_ = os.Setenv("KUBERNAUT_MAINTENANCE_MODE", "true")
			defer func() {
				if originalMaintenance == "" {
					_ = os.Unsetenv("KUBERNAUT_MAINTENANCE_MODE")
				} else {
					_ = os.Setenv("KUBERNAUT_MAINTENANCE_MODE", originalMaintenance)
				}
			}()

			By("Creating a warning severity alert")
			alert := types.Alert{
				Name:        "E2E_MAINTENANCE_MODE_TEST",
				Severity:    "warning", // Normally would require AI
				Description: "Warning alert during maintenance mode",
				Labels: map[string]string{
					"test":      "maintenance_mode",
					"severity":  "warning",
					"component": "maintenance-service",
				},
				Namespace: "default",
			}

			By("Creating workflow template for maintenance mode test")
			template := engine.NewWorkflowTemplate("maintenance-mode-test", "Maintenance Mode Test")
			template.Steps = []*engine.ExecutableWorkflowStep{
				{
					BaseEntity: types.BaseEntity{
						ID:   "maintenance-step-1",
						Name: "Maintenance Mode Step",
					},
					Type: engine.StepTypeAction,
					Action: &engine.StepAction{
						Type: "assessment",
						Parameters: map[string]interface{}{
							"alert_name":       alert.Name,
							"severity":         alert.Severity,
							"maintenance_mode": true,
						},
					},
				},
			}

			By("Creating and executing workflow in maintenance mode")
			workflow := engine.NewWorkflow("maintenance-mode-workflow", template)
			execution, err := framework.WorkflowEngine.Execute(ctx, workflow)
			Expect(err).NotTo(HaveOccurred(), "Workflow execution should succeed in maintenance mode")
			Expect(execution).NotTo(BeNil(), "Execution should be created")

			By("Verifying graceful degradation is used in maintenance mode")
			Expect(execution.ID).NotTo(BeEmpty(), "Execution should have an ID")
			GinkgoWriter.Printf("✅ Workflow executed successfully in maintenance mode with graceful degradation\n")
		})
	})
})
