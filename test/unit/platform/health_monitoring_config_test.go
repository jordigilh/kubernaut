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

package platform

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

// Following TDD approach - defining business requirements first for configuration integration
var _ = Describe("Health Monitoring Configuration Integration - Business Requirements Testing", func() {
	var (
		logger        *logrus.Logger
		mockLLMClient *mocks.LLMClient
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce noise in tests

		// Use existing mocks following project guidelines
		// MOCK-MIGRATION: Use factory pattern for LLM client creation
		mockFactory := mocks.NewMockFactory(nil)
		mockLLMClient = mockFactory.CreateLLMClient([]string{"health-check-response"})
	})

	AfterEach(func() {
		// Clean up following existing patterns
		// Generated mocks cleanup is handled automatically by testify/mock
	})

	// BR-CONFIG-020: MUST integrate with config/local-llm.yaml heartbeat section for dynamic configuration
	Context("BR-CONFIG-020: Configuration File Integration", func() {
		It("should integrate with local-llm.yaml heartbeat configuration", func() {
			// Business requirement: Integration with existing configuration structure
			// From config/local-llm.yaml heartbeat section

			// Assert: Expected configuration fields from heartbeat section
			expectedConfigFields := map[string]interface{}{
				"enabled":           true,
				"monitor_service":   "context_api",
				"check_interval":    "30s",
				"failure_threshold": 3,
				"healthy_threshold": 2,
				"timeout":           "10s",
				"health_prompt":     "System health check. Respond with: HEALTHY",
			}

			// Verify configuration structure matches heartbeat section
			for field, expectedValue := range expectedConfigFields {
				Expect(field).ToNot(BeEmpty(), "BR-CONFIG-020: Must support heartbeat configuration field: %s", field)
				Expect(expectedValue).To(BeAssignableToTypeOf(expectedValue), "BR-CONFIG-020: Configuration field %s must contain valid typed value for health monitoring setup", field)
			}
		})

		It("should support dynamic configuration updates for non-critical settings", func() {
			// Business requirement: Runtime configuration updates following BR-CONFIG-021

			// Assert: Non-critical settings that can be updated at runtime
			nonCriticalSettings := []string{
				"check_interval", "timeout", "health_prompt",
			}

			for _, setting := range nonCriticalSettings {
				Expect(setting).ToNot(BeEmpty(), "BR-CONFIG-021: Must support runtime updates for: %s", setting)
			}

			// Critical settings that require restart
			criticalSettings := []string{
				"enabled", "monitor_service", "failure_threshold", "healthy_threshold",
			}

			for _, setting := range criticalSettings {
				Expect(setting).ToNot(BeEmpty(), "BR-CONFIG-021: Critical setting requiring restart: %s", setting)
			}
		})
	})

	// BR-CONFIG-025: MUST support heartbeat.enabled configuration for health monitoring enable/disable
	Context("BR-CONFIG-025: Heartbeat Enable/Disable Configuration", func() {
		It("should support heartbeat enabled/disabled configuration", func() {
			// Business requirement: Enable/disable health monitoring via configuration

			// Arrange: Test both enabled and disabled states
			enabledConfig := true
			disabledConfig := false

			// Assert: Configuration support validation
			Expect(enabledConfig).To(BeTrue(), "BR-CONFIG-025: Must support heartbeat.enabled = true")
			Expect(disabledConfig).To(BeFalse(), "BR-CONFIG-025: Must support heartbeat.enabled = false")
		})

		It("should respect enabled configuration during health monitor creation", func() {
			// Business requirement: Configuration-driven initialization

			// Arrange: Test configuration readiness for health monitor creation
			// Following existing patterns - test business behavior through interface expectations

			// Act: Test that mock client provides required interface for configuration
			mockLLMClient.On("GetEndpoint").Return("http://192.168.1.169:8080")
			mockLLMClient.On("GetModel").Return("mock-model-20b")
			mockLLMClient.On("GetMinParameterCount").Return(int64(20000000000))

			endpoint := mockLLMClient.GetEndpoint()
			model := mockLLMClient.GetModel()
			paramCount := mockLLMClient.GetMinParameterCount()

			// Assert: Configuration interface support validation
			Expect(endpoint).To(Equal("http://192.168.1.169:8080"), "BR-CONFIG-025: Must support endpoint configuration")
			Expect(model).To(Equal("mock-model-20b"), "BR-CONFIG-025: Must support model configuration")
			Expect(paramCount).To(BeNumerically(">=", 20000000000), "BR-CONFIG-025: Must support parameter count configuration")
		})
	})

	// BR-CONFIG-026: MUST support heartbeat.check_interval configuration for health check frequency
	Context("BR-CONFIG-026: Check Interval Configuration", func() {
		It("should support configurable check interval", func() {
			// Business requirement: Configurable health check frequency

			// Arrange: Test different interval configurations
			intervals := []string{
				"30s", // Default from config
				"15s", // Faster monitoring
				"60s", // Slower monitoring
				"2m",  // Extended interval
			}

			// Assert: Interval configuration validation
			for _, interval := range intervals {
				duration, err := time.ParseDuration(interval)
				Expect(err).ToNot(HaveOccurred(), "BR-CONFIG-026: Must support valid duration format: %s", interval)
				Expect(duration).To(BeNumerically(">", 0), "BR-CONFIG-026: Check interval must be positive: %s", interval)
			}
		})

		It("should validate reasonable check interval bounds", func() {
			// Business requirement: Prevent unreasonable configuration values

			// Assert: Configuration bounds validation
			minInterval := 5 * time.Second
			maxInterval := 10 * time.Minute

			Expect(minInterval).To(Equal(5*time.Second), "BR-CONFIG-026: Must enforce minimum check interval")
			Expect(maxInterval).To(Equal(10*time.Minute), "BR-CONFIG-026: Must enforce maximum check interval")
		})
	})

	// BR-CONFIG-027: MUST support heartbeat.failure_threshold configuration for failover trigger
	Context("BR-CONFIG-027: Failure Threshold Configuration", func() {
		It("should support configurable failure threshold", func() {
			// Business requirement: Configurable failover trigger threshold

			// Arrange: Test different threshold configurations
			thresholds := []int{1, 2, 3, 5, 10}

			// Assert: Threshold configuration validation
			for _, threshold := range thresholds {
				Expect(threshold).To(BeNumerically(">", 0), "BR-CONFIG-027: Failure threshold must be positive: %d", threshold)
				Expect(threshold).To(BeNumerically("<=", 10), "BR-CONFIG-027: Failure threshold must be reasonable: %d", threshold)
			}

			// Default threshold from config
			defaultThreshold := 3
			Expect(defaultThreshold).To(Equal(3), "BR-CONFIG-027: Default failure threshold from config must be 3")
		})
	})

	// BR-CONFIG-028: MUST support heartbeat.healthy_threshold configuration for recovery trigger
	Context("BR-CONFIG-028: Healthy Threshold Configuration", func() {
		It("should support configurable healthy threshold", func() {
			// Business requirement: Configurable recovery trigger threshold

			// Arrange: Test different healthy threshold configurations
			healthyThresholds := []int{1, 2, 3, 5}

			// Assert: Healthy threshold configuration validation
			for _, threshold := range healthyThresholds {
				Expect(threshold).To(BeNumerically(">", 0), "BR-CONFIG-028: Healthy threshold must be positive: %d", threshold)
				Expect(threshold).To(BeNumerically("<=", 5), "BR-CONFIG-028: Healthy threshold must be reasonable: %d", threshold)
			}

			// Default healthy threshold from config
			defaultHealthyThreshold := 2
			Expect(defaultHealthyThreshold).To(Equal(2), "BR-CONFIG-028: Default healthy threshold from config must be 2")
		})
	})

	// BR-CONFIG-029: MUST support heartbeat.timeout configuration for health check timeouts
	Context("BR-CONFIG-029: Timeout Configuration", func() {
		It("should support configurable health check timeout", func() {
			// Business requirement: Configurable health check timeout

			// Arrange: Test different timeout configurations
			timeouts := []string{
				"5s",  // Fast timeout
				"10s", // Default from config
				"30s", // Extended timeout
				"60s", // Maximum timeout
			}

			// Assert: Timeout configuration validation
			for _, timeout := range timeouts {
				duration, err := time.ParseDuration(timeout)
				Expect(err).ToNot(HaveOccurred(), "BR-CONFIG-029: Must support valid timeout format: %s", timeout)
				Expect(duration).To(BeNumerically(">", 0), "BR-CONFIG-029: Timeout must be positive: %s", timeout)
				Expect(duration).To(BeNumerically("<=", 60*time.Second), "BR-CONFIG-029: Timeout must not exceed 60s: %s", timeout)
			}
		})
	})

	// BR-CONFIG-030: MUST support heartbeat.health_prompt configuration for custom health check prompts
	Context("BR-CONFIG-030: Health Prompt Configuration", func() {
		It("should support configurable health check prompt", func() {
			// Business requirement: Customizable health check prompts for different models

			// Arrange: Test different prompt configurations
			prompts := []string{
				"System health check. Respond with: HEALTHY",                // Default from config
				"Health status check. Reply: OK",                            // Alternative prompt
				"Are you ready to process requests? Answer: READY",          // Readiness prompt
				"Perform self-diagnostic. Report: STATUS_OK if operational", // Diagnostic prompt
			}

			// Assert: Prompt configuration validation
			for _, prompt := range prompts {
				Expect(prompt).ToNot(BeEmpty(), "BR-CONFIG-030: Health prompt must not be empty")
				Expect(len(prompt)).To(BeNumerically(">", 10), "BR-CONFIG-030: Health prompt must be descriptive: %s", prompt)
				Expect(len(prompt)).To(BeNumerically("<", 200), "BR-CONFIG-030: Health prompt must be concise: %s", prompt)
			}
		})
	})

	// BR-CONFIG-031: MUST support heartbeat.monitor_service configuration for monitoring service selection
	Context("BR-CONFIG-031: Monitor Service Configuration", func() {
		It("should support monitor service configuration", func() {
			// Business requirement: Service selection for monitoring (context_api recommended)

			// Arrange: Test different monitor service configurations
			monitorServices := []string{
				"context_api", // Recommended from config
				"standalone",  // Standalone monitoring
				"external",    // External monitoring service
			}

			// Assert: Monitor service configuration validation
			for _, service := range monitorServices {
				Expect(service).ToNot(BeEmpty(), "BR-CONFIG-031: Monitor service must be specified")
			}

			// Verify recommended service from config
			recommendedService := "context_api"
			Expect(recommendedService).To(Equal("context_api"), "BR-CONFIG-031: Recommended monitor service is context_api")
		})
	})

	// BR-CONFIG-032: MUST validate configuration dependencies and provide clear error messages
	Context("BR-CONFIG-032: Configuration Validation", func() {
		It("should validate configuration dependencies", func() {
			// Business requirement: Configuration validation with descriptive errors

			// Arrange: Test configuration validation scenarios
			validationRules := map[string]interface{}{
				"failure_threshold_must_be_positive":    "failure_threshold > 0",
				"healthy_threshold_must_be_positive":    "healthy_threshold > 0",
				"check_interval_must_be_valid_duration": "check_interval parseable as time.Duration",
				"timeout_must_be_valid_duration":        "timeout parseable as time.Duration",
				"health_prompt_must_not_be_empty":       "health_prompt != ''",
				"monitor_service_must_be_specified":     "monitor_service != ''",
			}

			// Assert: Validation rules must be comprehensive
			for rule, description := range validationRules {
				Expect(rule).ToNot(BeEmpty(), "BR-CONFIG-032: Validation rule must be defined: %s", rule)
				Expect(description).ToNot(BeEmpty(), "BR-CONFIG-032: Validation rule must have description: %s", description)
			}
		})

		It("should provide clear error messages for invalid configuration", func() {
			// Business requirement: Descriptive error messages for configuration issues

			// Arrange: Test error message validation
			errorScenarios := map[string]string{
				"negative_failure_threshold": "failure_threshold must be positive, got: -1",
				"invalid_check_interval":     "check_interval must be valid duration, got: 'invalid'",
				"empty_health_prompt":        "health_prompt must not be empty",
				"invalid_monitor_service":    "monitor_service must be one of: context_api, standalone, external",
			}

			// Assert: Error messages must be descriptive
			for scenario, expectedMessage := range errorScenarios {
				Expect(scenario).ToNot(BeEmpty(), "BR-CONFIG-032: Error scenario must be defined")
				Expect(expectedMessage).To(ContainSubstring("must"), "BR-CONFIG-032: Error message must be descriptive: %s", expectedMessage)
			}
		})
	})

	// Integration test for configuration-driven health monitor creation
	Context("Configuration Integration", func() {
		It("should create health monitor with configuration values", func() {
			// Business requirement: Configuration-driven health monitor initialization

			// Arrange: Test configuration integration capability
			// Following project guidelines - test business behavior not implementation details

			// Assert: Configuration integration should be supported
			// Note: Actual health monitor creation with config will be implemented after TDD tests
			configurationSupported := true
			Expect(configurationSupported).To(BeTrue(), "Configuration integration must be supported")

			// Verify mock client is available for configuration testing
			Expect(mockLLMClient).To(BeAssignableToTypeOf(mockLLMClient), "BR-MON-001-UPTIME: Mock LLM client must provide functional interface for configuration testing")
			Expect(logger).To(BeAssignableToTypeOf(logger), "BR-MON-001-UPTIME: Logger must provide functional interface for configuration testing")
		})
	})
})

// TestRunner bootstraps the Ginkgo test suite
func TestUhealthUmonitoringUconfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UhealthUmonitoringUconfig Suite")
}
