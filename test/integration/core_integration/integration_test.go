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
//go:build integration
// +build integration

package core_integration

import (
	"context"
	"os"
	"time"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/internal/config"

	"github.com/jordigilh/kubernaut/pkg/platform/executor"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/integration/shared"
	"github.com/jordigilh/kubernaut/test/integration/shared/testenv"
)

// SimpleMockRepository for integration testing
type SimpleMockRepository struct{}

func (m *SimpleMockRepository) EnsureResourceReference(ctx context.Context, ref actionhistory.ResourceReference) (int64, error) {
	// Check for context cancellation in integration test mock
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}
	return 1, nil
}

func (m *SimpleMockRepository) GetResourceReference(ctx context.Context, namespace, kind, name string) (*actionhistory.ResourceReference, error) {
	// Check for context cancellation in integration test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return nil, nil
}

func (m *SimpleMockRepository) EnsureActionHistory(ctx context.Context, resourceID int64) (*actionhistory.ActionHistory, error) {
	// Check for context cancellation in integration test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return nil, nil
}

func (m *SimpleMockRepository) GetActionHistory(ctx context.Context, resourceID int64) (*actionhistory.ActionHistory, error) {
	// Check for context cancellation in integration test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return nil, nil
}

func (m *SimpleMockRepository) UpdateActionHistory(ctx context.Context, history *actionhistory.ActionHistory) error {
	// Check for context cancellation in integration test mock
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	return nil
}

func (m *SimpleMockRepository) StoreAction(ctx context.Context, action *actionhistory.ActionRecord) (*actionhistory.ResourceActionTrace, error) {
	// Check for context cancellation in integration test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return nil, nil
}

func (m *SimpleMockRepository) GetActionTraces(ctx context.Context, query actionhistory.ActionQuery) ([]actionhistory.ResourceActionTrace, error) {
	// Check for context cancellation in integration test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return nil, nil
}

func (m *SimpleMockRepository) GetActionTrace(ctx context.Context, actionID string) (*actionhistory.ResourceActionTrace, error) {
	// Check for context cancellation in integration test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return nil, nil
}

func (m *SimpleMockRepository) UpdateActionTrace(ctx context.Context, trace *actionhistory.ResourceActionTrace) error {
	// Check for context cancellation in integration test mock
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	return nil
}

func (m *SimpleMockRepository) GetPendingEffectivenessAssessments(ctx context.Context) ([]*actionhistory.ResourceActionTrace, error) {
	// Check for context cancellation in integration test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return nil, nil
}

func (m *SimpleMockRepository) GetOscillationPatterns(ctx context.Context, patternType string) ([]actionhistory.OscillationPattern, error) {
	// Check for context cancellation in integration test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return nil, nil
}

func (m *SimpleMockRepository) StoreOscillationDetection(ctx context.Context, detection *actionhistory.OscillationDetection) error {
	// Check for context cancellation in integration test mock
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	return nil
}

func (m *SimpleMockRepository) GetOscillationDetections(ctx context.Context, resourceID int64, resolved *bool) ([]actionhistory.OscillationDetection, error) {
	// Check for context cancellation in integration test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return nil, nil
}

func (m *SimpleMockRepository) ApplyRetention(ctx context.Context, actionHistoryID int64) error {
	// Check for context cancellation in integration test mock
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	return nil
}

func (m *SimpleMockRepository) GetActionHistorySummaries(ctx context.Context, since time.Duration) ([]actionhistory.ActionHistorySummary, error) {
	// Check for context cancellation in integration test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return nil, nil
}

var _ = Describe("SLM Integration", func() {
	var (
		logger       *logrus.Logger
		errorMetrics *shared.MetricsCollector
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.DebugLevel)
		errorMetrics = shared.NewMetricsCollector(logger)
	})

	AfterEach(func() {
		if errorMetrics != nil {
			// Generate and log error analytics report
			report, err := errorMetrics.GenerateReport()
			if err == nil {
				logger.WithFields(logrus.Fields{
					"resilience_score":      report.SystemResilienceScore,
					"scenarios_executed":    report.TotalScenariosExecuted,
					"recovery_tests":        report.TotalRecoveryTests,
					"recommendations_count": len(report.Recommendations),
				}).Info("Error injection test analytics completed")
			}
		}
	})

	It("should successfully analyze alerts with SLM", func() {
		// Skip if Ollama is not available
		if os.Getenv("SKIP_INTEGRATION") != "" {
			Skip("Integration tests skipped")
		}

		// Configuration no longer needed for fake client

		// Create fake SLM client to eliminate external dependencies
		client := shared.NewSLMClient()

		// Fake client is always healthy
		Expect(client.IsHealthy()).To(BeTrue())

		// Create test alert
		testAlert := types.Alert{
			Name:        "HighMemoryUsage",
			Status:      "firing",
			Severity:    "warning",
			Description: "Pod is using 95% of memory limit",
			Namespace:   "production",
			Resource:    "web-app-pod-123",
			Labels: map[string]string{
				"alertname":  "HighMemoryUsage",
				"severity":   "warning",
				"namespace":  "production",
				"pod":        "web-app-pod-123",
				"deployment": "web-app",
			},
			Annotations: map[string]string{
				"description": "Pod is using 95% of memory limit",
				"summary":     "High memory usage detected",
			},
			StartsAt: time.Now().Add(-5 * time.Minute),
		}

		// Test alert analysis
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		recommendation, err := client.AnalyzeAlert(ctx, testAlert)
		Expect(err).ToNot(HaveOccurred())

		// Validate recommendation
		validActions := []string{"scale_deployment", "restart_pod", "increase_resources", "notify_only", "collect_diagnostics"}
		Expect(recommendation.Action).To(BeElementOf(validActions), "BR-WF-001-SUCCESS-RATE: Core integration must provide data for workflow success rate requirements")

		Expect(recommendation.Confidence).To(BeNumerically(">=", 0.0))
		Expect(recommendation.Confidence).To(BeNumerically("<=", 1.0))

		GinkgoWriter.Printf("Integration test successful - Action: %s, Confidence: %.2f\n",
			recommendation.Action, recommendation.Confidence)
	})
})

var _ = Describe("Kubernetes Integration", func() {
	var (
		testEnv *testenv.TestEnvironment
		logger  *logrus.Logger
	)

	BeforeEach(func() {
		// Skip if K8s integration is disabled
		if os.Getenv("SKIP_K8S_INTEGRATION") != "" {
			Skip("Kubernetes integration tests skipped")
		}

		// Create logger
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)

		// Setup fake Kubernetes environment
		var err error
		testEnv, err = testenv.SetupEnvironment()
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if testEnv != nil {
			err := testEnv.Cleanup()
			Expect(err).ToNot(HaveOccurred())
		}
	})

	It("should create and manage Kubernetes resources", func() {
		// Create a test namespace
		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-namespace",
			},
		}

		createdNS, err := testEnv.Client.CoreV1().Namespaces().Create(
			testEnv.Context, namespace, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(createdNS.Name).To(Equal("test-namespace"))

		// Create a test deployment
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deployment",
				Namespace: "test-namespace",
				Labels:    map[string]string{"app": "test"},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(3),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "test"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "test"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "test-container",
								Image: "nginx:latest",
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("100m"),
										corev1.ResourceMemory: resource.MustParse("128Mi"),
									},
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("500m"),
										corev1.ResourceMemory: resource.MustParse("512Mi"),
									},
								},
							},
						},
					},
				},
			},
		}

		createdDep, err := testEnv.Client.AppsV1().Deployments("test-namespace").Create(
			testEnv.Context, deployment, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(createdDep.Name).To(Equal("test-deployment"))
		Expect(*createdDep.Spec.Replicas).To(Equal(int32(3)))

		// Verify the deployment can be retrieved
		retrievedDep, err := testEnv.Client.AppsV1().Deployments("test-namespace").Get(
			testEnv.Context, "test-deployment", metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(retrievedDep.Name).To(Equal("test-deployment"))

		GinkgoWriter.Printf("Successfully created and verified deployment: %s\n", retrievedDep.Name)
	})

	It("should test K8s client interface", func() {
		// Create k8s client using testenv
		k8sClient := testEnv.CreateK8sClient(logger)
		Expect(k8sClient).ToNot(BeNil(), "BR-WF-001-SUCCESS-RATE: Core integration must return valid workflow components for success rate requirements")

		// Create a test namespace directly using the client
		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "client-test-ns",
			},
		}

		_, err := testEnv.Client.CoreV1().Namespaces().Create(
			testEnv.Context, namespace, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		// Verify namespace was created
		ns, err := testEnv.Client.CoreV1().Namespaces().Get(
			testEnv.Context, "client-test-ns", metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(ns.Name).To(Equal("client-test-ns"))

		GinkgoWriter.Printf("Successfully tested k8s client interface with namespace: %s\n", ns.Name)
	})
})

// Helper function for int32 pointer
func int32Ptr(i int32) *int32 {
	return &i
}

var _ = Describe("End-to-End Flow", func() {
	var (
		testEnv   *testenv.TestEnvironment
		logger    *logrus.Logger
		slmClient llm.Client
		exec      executor.Executor
		err       error
	)

	BeforeEach(func() {
		// Skip if components are not available
		if os.Getenv("SKIP_E2E") != "" {
			Skip("End-to-end tests skipped")
		}

		// Create logger
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)

		// Setup fake Kubernetes environment
		testEnv, err = testenv.SetupEnvironment()
		Expect(err).ToNot(HaveOccurred())

		// Setup SLM client (only if not skipping integration)
		if os.Getenv("SKIP_INTEGRATION") == "" {
			// Configuration no longer needed for fake client

			slmClient = shared.NewTestSLMClient()
			Expect(slmClient.IsHealthy()).To(BeTrue())
		}

		// Create executor with K8s client
		k8sClient := testEnv.CreateK8sClient(logger)
		mockRepo := &SimpleMockRepository{}
		exec, err = executor.NewExecutor(k8sClient, config.ActionsConfig{
			DryRun:         false,
			MaxConcurrent:  5,
			CooldownPeriod: 30 * time.Second,
		}, mockRepo, logger)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if testEnv != nil {
			err := testEnv.Cleanup()
			Expect(err).ToNot(HaveOccurred())
		}
	})

	It("should process complete alert and action flow", func() {
		// 1. Create a test deployment in K8s
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "web-app",
				Namespace: "default",
				Labels:    map[string]string{"app": "web-app"},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(2),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "web-app"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "web-app"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "web-container",
								Image: "nginx:latest",
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("100m"),
										corev1.ResourceMemory: resource.MustParse("128Mi"),
									},
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("200m"),
										corev1.ResourceMemory: resource.MustParse("256Mi"),
									},
								},
							},
						},
					},
				},
			},
		}

		_, err := testEnv.Client.AppsV1().Deployments("default").Create(
			testEnv.Context, deployment, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		// 2. Create test alert for high memory usage
		testAlert := types.Alert{
			Name:        "HighMemoryUsage",
			Status:      "firing",
			Severity:    "warning",
			Description: "Pod is using 90% of memory limit",
			Namespace:   "default",
			Resource:    "web-app",
			Labels: map[string]string{
				"alertname":  "HighMemoryUsage",
				"severity":   "warning",
				"namespace":  "default",
				"deployment": "web-app",
			},
			Annotations: map[string]string{
				"description": "Pod is using 90% of memory limit",
				"summary":     "High memory usage detected",
			},
			StartsAt: time.Now().Add(-5 * time.Minute),
		}

		// 3. Analyze alert with SLM (if available)
		var recommendation *types.ActionRecommendation
		if slmClient != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			// Guideline #1: Reuse existing type conversion helper
			analyzeResponse, err := slmClient.AnalyzeAlert(ctx, testAlert)
			Expect(err).ToNot(HaveOccurred())
			recommendation = shared.ConvertAnalyzeAlertResponse(analyzeResponse)
			Expect(recommendation).ToNot(BeNil(), "BR-WF-001-SUCCESS-RATE: Core integration must return valid workflow components for success rate requirements")
			validActions := []string{"scale_deployment", "restart_pod", "increase_resources", "notify_only", "collect_diagnostics"}
			Expect(recommendation.Action).To(BeElementOf(validActions), "BR-WF-001-SUCCESS-RATE: Core integration must provide data for workflow success rate requirements")

			GinkgoWriter.Printf("SLM recommended action: %s (confidence: %.2f)\n",
				recommendation.Action, recommendation.Confidence)
		} else {
			// Create a mock recommendation for testing executor
			recommendation = &types.ActionRecommendation{
				Action:     "scale_deployment",
				Confidence: 0.8,
				Reasoning: &types.ReasoningDetails{
					Summary: "Scale deployment due to high memory usage",
				},
				Parameters: map[string]interface{}{
					"replicas": 4,
				},
			}
		}

		// 4. Execute action via executor
		actionCtx, actionCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer actionCancel()

		err = exec.Execute(actionCtx, recommendation, testAlert, nil)
		Expect(err).ToNot(HaveOccurred())

		// 5. Verify the action was executed in K8s
		if recommendation.Action == "scale_deployment" {
			// Verify deployment was scaled
			updatedDep, err := testEnv.Client.AppsV1().Deployments("default").Get(
				testEnv.Context, "web-app", metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())

			expectedReplicas := int32(4)
			if replicas, ok := recommendation.Parameters["replicas"].(int); ok {
				expectedReplicas = int32(replicas)
			}
			Expect(*updatedDep.Spec.Replicas).To(Equal(expectedReplicas))

			GinkgoWriter.Printf("Successfully scaled deployment to %d replicas\n", *updatedDep.Spec.Replicas)
		}

		GinkgoWriter.Printf("End-to-end test completed successfully: %s\n", recommendation.Action)
	})

	Context("Error Injection and Resilience Testing", func() {
		var (
			localErrorMetrics *shared.MetricsCollector
		)

		BeforeEach(func() {
			localErrorMetrics = shared.NewMetricsCollector(logger)
		})

		AfterEach(func() {
			if localErrorMetrics != nil {
				// Generate and log error analytics report
				report, err := localErrorMetrics.GenerateReport()
				if err == nil && report.TotalScenariosExecuted > 0 {
					logger.WithFields(logrus.Fields{
						"resilience_score":      report.SystemResilienceScore,
						"scenarios_executed":    report.TotalScenariosExecuted,
						"recovery_tests":        report.TotalRecoveryTests,
						"recommendations_count": len(report.Recommendations),
					}).Info("Error injection test completed with analytics")
				}
			}
		})

		It("should handle SLM service failures gracefully", func() {
			// Skip if integration tests are disabled
			if os.Getenv("SKIP_INTEGRATION") != "" {
				Skip("Integration tests skipped")
			}

			// Create fake SLM client with error injection capabilities
			client := shared.NewTestSLMClient()

			// Enable error injection for testing resilience scenarios
			client.SetErrorInjectionEnabled(true)

			// Test alert for error injection scenarios
			testAlert := types.Alert{
				Name:        "HighMemoryUsage",
				Status:      "firing",
				Severity:    "warning",
				Description: "Container memory usage is above 80%",
				Namespace:   "default",
				Resource:    "test-app",
				Labels: map[string]string{
					"alertname": "HighMemoryUsage",
					"pod":       "test-app-pod",
				},
			}

			By("Testing normal operation baseline")
			baseline, err := client.AnalyzeAlert(context.Background(), testAlert)
			Expect(err).ToNot(HaveOccurred())
			Expect(baseline).ToNot(BeNil(), "BR-WF-001-SUCCESS-RATE: Core integration must return valid workflow components for success rate requirements")
			validActions := []string{"scale_deployment", "restart_pod", "increase_resources", "notify_only", "collect_diagnostics"}
			Expect(baseline.Action).To(BeElementOf(validActions), "BR-WF-001-SUCCESS-RATE: Core integration must provide data for workflow success rate requirements")

			logger.WithFields(logrus.Fields{
				"action":     baseline.Action,
				"confidence": baseline.Confidence,
			}).Info("Baseline SLM analysis successful")

			By("Injecting transient network failure scenario")
			networkFailureScenario := shared.PredefinedErrorScenarios["transient_network_failure"]

			// Record scenario execution for metrics
			scenarioExecution := &shared.ErrorScenarioExecution{
				Scenario:  networkFailureScenario,
				StartTime: time.Now(),
				Status:    shared.ScenarioStatusActive,
			}

			err = client.TriggerErrorScenario(networkFailureScenario)
			Expect(err).ToNot(HaveOccurred())

			// Test behavior under network failure
			logger.Info("Testing SLM behavior under network failure")

			failureResponse, err := client.AnalyzeAlert(context.Background(), testAlert)

			if err != nil {
				// Network failure should trigger error (expected behavior)
				logger.WithError(err).Info("Network failure correctly triggered error response")

				// Verify error is retryable network error
				Expect(shared.IsNetworkError(err)).To(BeTrue())

				// Record component operation for metrics
				localErrorMetrics.RecordComponentOperation("slm_client", "analyze_alert", false, 0, shared.NetworkError)
			} else {
				// If no error, should still get valid response (fallback behavior)
				logger.Info("SLM service provided fallback response during network failure")
				Expect(failureResponse).ToNot(BeNil(), "BR-WF-001-SUCCESS-RATE: Core integration must return valid workflow components for success rate requirements")

				// Record successful fallback
				localErrorMetrics.RecordComponentOperation("slm_client", "analyze_alert", true, time.Since(scenarioExecution.StartTime), shared.NetworkError)
			}

			By("Verifying automatic recovery after scenario duration")
			// Wait for scenario recovery (network failure scenario has 30s duration + 5s recovery)
			time.Sleep(6 * time.Second) // Give time for recovery

			recoveryResponse, err := client.AnalyzeAlert(context.Background(), testAlert)
			Expect(err).ToNot(HaveOccurred())
			Expect(recoveryResponse).ToNot(BeNil(), "BR-WF-001-SUCCESS-RATE: Core integration must return valid workflow components for success rate requirements")
			Expect(recoveryResponse.Action).To(BeElementOf(validActions), "BR-WF-001-SUCCESS-RATE: Core integration must provide data for workflow success rate requirements")

			logger.WithFields(logrus.Fields{
				"action":     recoveryResponse.Action,
				"confidence": recoveryResponse.Confidence,
			}).Info("SLM service successfully recovered from network failure")

			// Record scenario completion (metrics collector doesn't need scenario details)
			logger.WithFields(logrus.Fields{
				"scenario": networkFailureScenario.Name,
				"duration": time.Since(scenarioExecution.StartTime),
				"status":   "completed",
			}).Info("Network failure scenario completed successfully")
		})

		It("should handle circuit breaker activation", func() {
			// Skip if integration tests are disabled
			if os.Getenv("SKIP_INTEGRATION") != "" {
				Skip("Integration tests skipped")
			}

			client := shared.NewTestSLMClient()

			// Enable error injection for testing resilience scenarios
			client.SetErrorInjectionEnabled(true)

			testAlert := types.Alert{
				Name:      "ServiceDegraded",
				Status:    "firing",
				Severity:  "critical",
				Namespace: "production",
				Resource:  "api-service",
			}

			By("Configuring circuit breaker with low threshold for testing")
			circuitBreakerConfig := shared.CircuitBreakerConfig{
				Enabled:          true,
				FailureThreshold: 3,
				RecoveryTimeout:  5 * time.Second,
				SuccessThreshold: 2,
			}
			client.EnableCircuitBreaker(circuitBreakerConfig)

			By("Triggering circuit breaker activation scenario")
			circuitBreakerScenario := shared.PredefinedErrorScenarios["circuit_breaker_activation"]

			err := client.TriggerErrorScenario(circuitBreakerScenario)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying circuit breaker state transitions")
			// Initial state should be closed
			Expect(client.GetCircuitBreakerState()).To(Equal(shared.CircuitClosed))

			// Trigger multiple failures to open circuit breaker
			for i := 0; i < 4; i++ {
				_, err := client.AnalyzeAlert(context.Background(), testAlert)
				// Expect failures due to circuit breaker scenario
				if err != nil {
					logger.WithError(err).WithField("attempt", i+1).Debug("Circuit breaker failure triggered")
				}

				// Record circuit breaker transition for metrics
				if i == 2 { // Should open after 3rd failure (threshold = 3)
					localErrorMetrics.RecordCircuitBreakerTransition("slm_client", "closed", "open", "failure_threshold_exceeded", 3)
				}
			}

			// Circuit should now be open
			Eventually(func() shared.CircuitBreakerState {
				return client.GetCircuitBreakerState()
			}, "10s", "1s").Should(Equal(shared.CircuitOpen))

			logger.Info("Circuit breaker successfully opened after failure threshold")

			By("Testing circuit breaker recovery")
			// Wait for recovery timeout
			time.Sleep(6 * time.Second)

			// Trigger state check with a call - circuit breaker transitions during operations
			_, _ = client.AnalyzeAlert(context.Background(), testAlert)

			// Circuit should transition to half-open
			Expect(client.GetCircuitBreakerState()).To(Equal(shared.CircuitHalfOpen))
			logger.Info("Circuit breaker transitioned to half-open state")

			// Reset error state to allow successful calls
			client.ResetErrorState()

			// Make successful calls to close circuit
			for i := 0; i < 3; i++ {
				response, err := client.AnalyzeAlert(context.Background(), testAlert)
				Expect(err).ToNot(HaveOccurred())
				Expect(response).ToNot(BeNil(), "BR-WF-001-SUCCESS-RATE: Core integration must return valid workflow components for success rate requirements")
			}

			// Circuit should close after successful calls
			Eventually(func() shared.CircuitBreakerState {
				return client.GetCircuitBreakerState()
			}, "5s", "500ms").Should(Equal(shared.CircuitClosed))

			logger.Info("Circuit breaker successfully closed after recovery")

			// Record final transition
			localErrorMetrics.RecordCircuitBreakerTransition("slm_client", "half_open", "closed", "success_threshold_met", 0)
		})

		It("should provide comprehensive resilience analytics", func() {
			// This test verifies that error metrics collection works properly
			// Create a test metrics collector and add some sample data
			testMetrics := shared.NewMetricsCollector(logger)

			// Add some sample component operations for testing
			testMetrics.RecordComponentOperation("slm_client", "analyze_alert", true, 100*time.Millisecond, shared.NetworkError)
			testMetrics.RecordComponentOperation("k8s_client", "get_pod", false, 500*time.Millisecond, shared.K8sAPIError)
			testMetrics.RecordCircuitBreakerTransition("slm_client", "closed", "open", "threshold_exceeded", 3)

			report, err := testMetrics.GenerateReport()
			Expect(err).ToNot(HaveOccurred())
			Expect(report).ToNot(BeNil(), "BR-WF-001-SUCCESS-RATE: Core integration must return valid workflow components for success rate requirements")

			// Verify analytics capabilities
			Expect(report.SystemResilienceScore).To(BeNumerically(">=", 0))
			Expect(report.SystemResilienceScore).To(BeNumerically("<=", 1))

			logger.WithFields(logrus.Fields{
				"resilience_score":   report.SystemResilienceScore,
				"scenarios_executed": report.TotalScenariosExecuted,
				"recovery_tests":     report.TotalRecoveryTests,
				"component_count":    len(report.ComponentReliability),
				"recommendations":    len(report.Recommendations),
			}).Info("Comprehensive resilience analytics generated")

			// Export analytics to JSON for inspection
			analyticsJSON, err := testMetrics.ExportToJSON()
			Expect(err).ToNot(HaveOccurred())
			Expect(len(analyticsJSON)).To(BeNumerically(">", 100))

			// Verify recommendations are generated
			if len(report.Recommendations) > 0 {
				logger.WithField("recommendations", report.Recommendations).Info("System resilience recommendations generated")
			}

			// Verify component reliability metrics
			Expect(len(report.ComponentReliability)).To(BeNumerically(">", 0))
			for componentName, reliability := range report.ComponentReliability {
				logger.WithFields(logrus.Fields{
					"component":         componentName,
					"reliability_score": reliability.ReliabilityScore,
					"total_operations":  reliability.TotalOperations,
					"failed_operations": reliability.FailedOperations,
				}).Debug("Component reliability metrics")

				// Verify reliability score is valid
				Expect(reliability.ReliabilityScore).To(BeNumerically(">=", 0))
				Expect(reliability.ReliabilityScore).To(BeNumerically("<=", 1))
			}
		})
	})
})
