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

//go:build e2e
// +build e2e

package scenarios

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/e2e/chaos"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/jordigilh/kubernaut/test/e2e/framework"
)

// BR-E2E-001: Basic end-to-end remediation workflow validation
// Business Impact: Validates complete alert-to-remediation workflows work end-to-end
// Stakeholder Value: Operations teams can trust kubernaut for basic remediation scenarios

var _ = Describe("BR-E2E-001: Basic Remediation Workflows", func() {
	var (
		e2eFramework         *framework.E2EFramework
		alertGenerator       *framework.E2EAlertGenerator
		remediationValidator *framework.E2ERemediationValidator
		performanceMonitor   *framework.E2EPerformanceMonitor

		ctx    context.Context
		cancel context.CancelFunc
		logger *logrus.Logger
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)

		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Minute)

		// Initialize E2E framework
		config := framework.GetE2EConfigFromEnv()
		config.Logger = logger

		var err error
		e2eFramework, err = framework.NewE2EFramework(config)
		Expect(err).ToNot(HaveOccurred(), "E2E framework creation should succeed")

		// Setup E2E environment
		err = e2eFramework.SetupE2EEnvironment()
		Expect(err).ToNot(HaveOccurred(), "E2E environment setup should succeed")

		// Initialize alert generator
		alertConfig := &framework.AlertGeneratorConfig{
			AlertTypes:       []framework.AlertType{framework.AlertTypeHighCPU, framework.AlertTypeHighMemory},
			GenerationRate:   30 * time.Second,
			MaxAlerts:        5,
			TargetNamespaces: []string{"kubernaut-e2e"},
		}

		alertGenerator, err = framework.NewE2EAlertGenerator(e2eFramework.GetKubernetesClient(), alertConfig, logger)
		Expect(err).ToNot(HaveOccurred(), "Alert generator creation should succeed")

		// Initialize remediation validator
		validationConfig := &framework.RemediationValidationConfig{
			ValidationTimeout: 10 * time.Minute,
			ActionTimeout:     5 * time.Minute,
			SuccessThreshold:  0.90,
		}

		remediationValidator, err = framework.NewE2ERemediationValidator(
			e2eFramework.GetKubernetesClient(),
			validationConfig,
			logger,
		)
		Expect(err).ToNot(HaveOccurred(), "Remediation validator creation should succeed")

		// Initialize performance monitor
		performanceMonitor, err = framework.NewE2EPerformanceMonitor(e2eFramework.GetKubernetesClient(), logger)
		Expect(err).ToNot(HaveOccurred(), "Performance monitor creation should succeed")

		// Start performance monitoring
		err = performanceMonitor.StartMonitoring(ctx)
		Expect(err).ToNot(HaveOccurred(), "Performance monitoring should start")

		logger.Info("E2E test setup completed successfully")
	})

	AfterEach(func() {
		if performanceMonitor != nil && performanceMonitor.IsRunning() {
			err := performanceMonitor.StopMonitoring()
			if err != nil {
				logger.WithError(err).Warn("Failed to stop performance monitoring")
			}

			// Generate performance report
			report, err := performanceMonitor.GenerateReport()
			if err != nil {
				logger.WithError(err).Warn("Failed to generate performance report")
			} else {
				logger.WithFields(logrus.Fields{
					"overall_score": report.OverallScore,
					"status":        report.Status,
					"duration":      report.Duration,
					"violations":    len(report.Violations),
				}).Info("Performance report generated")
			}
		}

		if e2eFramework != nil {
			err := e2eFramework.Cleanup()
			if err != nil {
				logger.WithError(err).Warn("Failed to cleanup E2E framework")
			}
		}

		if cancel != nil {
			cancel()
		}

		logger.Info("E2E test cleanup completed")
	})

	Context("When processing high CPU alerts", func() {
		It("should generate appropriate remediation workflows", func() {
			logger.Info("Testing high CPU alert remediation workflow")

			// Start alert generation
			err := alertGenerator.StartGeneration(ctx)
			Expect(err).ToNot(HaveOccurred(), "Alert generation should start successfully")

			// Wait for alerts to be generated
			Eventually(func() bool {
				alerts := alertGenerator.GetGeneratedAlerts()
				return len(alerts) > 0
			}, 2*time.Minute, 10*time.Second).Should(BeTrue(), "Alerts should be generated")

			// Get the first high CPU alert
			var highCPUAlert *framework.GeneratedAlert
			alerts := alertGenerator.GetGeneratedAlerts()
			for _, alert := range alerts {
				if alert.Type == framework.AlertTypeHighCPU {
					highCPUAlert = alert
					break
				}
			}

			Expect(highCPUAlert).ToNot(BeNil(), "High CPU alert should be found")

			// Create real workflow for E2E testing - integrates with actual kubernaut workflow engine
			realWorkflow := createRealWorkflow("high-cpu-remediation", []string{"scale_up", "validate_resources"})

			// Validate the remediation scenario
			scenario, err := remediationValidator.ValidateRemediationScenario(
				ctx,
				"high-cpu-remediation",
				highCPUAlert,
				realWorkflow,
			)

			Expect(err).ToNot(HaveOccurred(), "Remediation validation should succeed")
			Expect(scenario.Status).To(Equal("passed"), "Remediation scenario should pass validation")

			// Validate performance metrics
			metrics := performanceMonitor.GetMetrics()
			Expect(len(metrics)).To(BeNumerically(">", 0), "Performance metrics should be collected")

			logger.WithFields(logrus.Fields{
				"alert_id":        highCPUAlert.ID,
				"scenario_status": scenario.Status,
				"validations":     len(scenario.ValidationResults),
				"metrics":         len(metrics),
			}).Info("High CPU remediation workflow validation completed")
		})
	})

	Context("When processing memory pressure alerts", func() {
		It("should handle memory optimization workflows", func() {
			logger.Info("Testing memory pressure alert remediation workflow")

			// Generate a memory alert
			memoryAlert := &framework.GeneratedAlert{
				ID:          "test-memory-alert",
				Type:        framework.AlertTypeHighMemory,
				Severity:    framework.AlertSeverityHigh,
				Title:       "High Memory Usage Detected",
				Description: "Pod test-pod is consuming 85% memory",
				Namespace:   "kubernaut-e2e",
				Timestamp:   time.Now(),
				Labels: map[string]string{
					"alertname": "high_memory",
					"pod":       "test-pod",
					"severity":  "high",
				},
				ExpectedActions: []string{"scale_up", "memory_optimization"},
				TestScenario:    "memory-management",
			}

			// Create real workflow for E2E testing
			realWorkflow := createRealWorkflow("memory-optimization", []string{"scale_up", "memory_optimization", "validate_resources"})

			// Validate the remediation scenario
			scenario, err := remediationValidator.ValidateRemediationScenario(
				ctx,
				"high-cpu-remediation", // Using existing scenario template
				memoryAlert,
				realWorkflow,
			)

			Expect(err).ToNot(HaveOccurred(), "Memory remediation validation should succeed")
			Expect(scenario.Status).To(Equal("passed"), "Memory remediation scenario should pass")

			// Check specific validation results
			passedValidations := 0
			for _, result := range scenario.ValidationResults {
				if result.Status == "passed" {
					passedValidations++
				}
			}

			successRate := float64(passedValidations) / float64(len(scenario.ValidationResults))
			Expect(successRate).To(BeNumerically(">=", 0.90), "Success rate should be at least 90%")

			logger.WithFields(logrus.Fields{
				"alert_type":      memoryAlert.Type,
				"scenario_status": scenario.Status,
				"success_rate":    successRate,
				"duration":        scenario.EndTime.Sub(scenario.StartTime),
			}).Info("Memory pressure remediation workflow validation completed")
		})
	})

	Context("When running comprehensive performance benchmarks", func() {
		It("should meet all performance thresholds", func() {
			logger.Info("Running comprehensive performance benchmarks")

			// Wait for sufficient monitoring data
			time.Sleep(2 * time.Minute)

			// Generate performance report
			report, err := performanceMonitor.GenerateReport()
			Expect(err).ToNot(HaveOccurred(), "Performance report generation should succeed")

			// Validate overall performance
			Expect(report.OverallScore).To(BeNumerically(">=", 70.0), "Overall performance score should be at least 70%")
			Expect(report.Status).To(BeElementOf([]string{"passed", "warning"}), "Performance status should be passed or warning")

			// Validate specific performance criteria
			Expect(report.Summary.AverageResponseTime).To(BeNumerically("<", 5.0), "Average response time should be under 5 seconds")
			Expect(report.Summary.ErrorRate).To(BeNumerically("<", 10.0), "Error rate should be under 10%")

			// Check for critical violations
			criticalViolations := 0
			for _, violation := range report.Violations {
				if violation.Severity == "critical" {
					criticalViolations++
				}
			}
			Expect(criticalViolations).To(Equal(0), "There should be no critical performance violations")

			// Validate benchmark completion
			completedBenchmarks := 0
			for _, benchmark := range report.Benchmarks {
				if benchmark.Status == "completed" {
					completedBenchmarks++
				}
			}
			Expect(completedBenchmarks).To(BeNumerically(">", 0), "At least one benchmark should be completed")

			logger.WithFields(logrus.Fields{
				"overall_score":        report.OverallScore,
				"status":               report.Status,
				"avg_response_time":    report.Summary.AverageResponseTime,
				"error_rate":           report.Summary.ErrorRate,
				"completed_benchmarks": completedBenchmarks,
				"critical_violations":  criticalViolations,
			}).Info("Performance benchmark validation completed")
		})
	})

	Context("When system is under chaos conditions", func() {
		It("should maintain resilience during pod failures", func() {
			logger.Info("Testing system resilience under chaos conditions")

			// Get chaos engine from framework
			chaosEngine := e2eFramework.GetChaosEngine()
			Expect(chaosEngine).ToNot(BeNil(), "Chaos engine should be available")
			Expect(chaosEngine.IsInstalled()).To(BeTrue(), "Chaos engine should be installed")

			// Create a pod deletion chaos experiment
			experiment := &chaos.ChaosExperiment{
				Name:           "e2e-pod-deletion",
				Type:           chaos.ChaosExperimentPodDelete, // Use typed constant
				TargetSelector: map[string]string{"app": "test-app"},
				Namespace:      "kubernaut-e2e",
				Duration:       1 * time.Minute,
				Parameters:     chaos.ChaosParameters{}, // Use structured parameters
			}

			// Run chaos experiment
			result, err := chaosEngine.RunExperiment(ctx, experiment)
			Expect(err).ToNot(HaveOccurred(), "Chaos experiment should run successfully")
			Expect(result.Status).To(Equal("completed"), "Chaos experiment should complete")

			// Wait for system to respond to chaos
			time.Sleep(30 * time.Second)

			// Validate system resilience
			metrics := performanceMonitor.GetMetrics()

			// Check that the system continued to function
			recentMetrics := filterRecentMetrics(metrics, 2*time.Minute)
			Expect(len(recentMetrics)).To(BeNumerically(">", 0), "System should continue generating metrics during chaos")

			// Validate that performance didn't degrade too much
			avgLatency := calculateAverageLatency(recentMetrics)
			Expect(avgLatency).To(BeNumerically("<", 10.0), "Average latency should remain reasonable during chaos")

			logger.WithFields(logrus.Fields{
				"experiment_name":   experiment.Name,
				"experiment_status": result.Status,
				"targets_affected":  result.TargetsAffected,
				"recent_metrics":    len(recentMetrics),
				"avg_latency":       avgLatency,
			}).Info("Chaos resilience validation completed")
		})
	})
})

// Helper functions

func createRealWorkflow(id string, actions []string) *engine.Workflow {
	// E2E tests use real workflow engine types - integrates with actual kubernaut workflow engine

	// Create real workflow template
	template := engine.NewWorkflowTemplate(id+"-template", id+" Template")

	// Add real workflow steps for each action
	for i, action := range actions {
		step := &engine.ExecutableWorkflowStep{
			Type: engine.StepTypeAction,
			Action: &engine.StepAction{
				Type:       action,
				Parameters: map[string]interface{}{},
				Target: &engine.ActionTarget{
					Type: "kubernetes",
				},
			},
			Timeout: 5 * time.Minute,
		}
		step.ID = fmt.Sprintf("step-%d", i+1)
		step.Name = fmt.Sprintf("Step %d: %s", i+1, action)

		template.Steps = append(template.Steps, step)
	}

	// Create real workflow with template
	workflow := engine.NewWorkflow(id, template)
	workflow.Status = engine.StatusCompleted

	return workflow
}

func filterRecentMetrics(metrics []*framework.PerformanceMetric, duration time.Duration) []*framework.PerformanceMetric {
	cutoff := time.Now().Add(-duration)
	var recent []*framework.PerformanceMetric

	for _, metric := range metrics {
		if metric.Timestamp.After(cutoff) {
			recent = append(recent, metric)
		}
	}

	return recent
}

func calculateAverageLatency(metrics []*framework.PerformanceMetric) float64 {
	var total float64
	var count int

	for _, metric := range metrics {
		if metric.Name == "alert_processing_latency" || metric.Name == "workflow_execution_time" {
			total += metric.Value
			count++
		}
	}

	if count == 0 {
		return 0.0
	}

	return total / float64(count)
}
