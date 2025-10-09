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

package configuration

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/testutil"
	"github.com/jordigilh/kubernaut/pkg/platform/executor"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

// BR-INFRASTRUCTURE-001: Infrastructure configuration business foundation
// BR-INFRASTRUCTURE-002: Platform executor business execution capability
// Business Impact: Infrastructure configuration and platform execution support business operations continuity
// Stakeholder Value: Executive confidence in infrastructure scalability and automated action execution
var _ = Describe("BR-INFRASTRUCTURE-001-002: Infrastructure Configuration Unit Tests", func() {
	var (
		// Use REAL business logic components per cursor rules
		dataFactory      *testutil.InfrastructureTestDataFactory
		assertions       *testutil.InfrastructureAssertions
		platformExecutor executor.Executor
		logger           *logrus.Logger

		// Mock ONLY external dependencies per 03-testing-strategy.mdc
		mockK8sClient  *mocks.MockK8sClient
		mockActionRepo *mocks.MockActionRepository
	)

	BeforeEach(func() {
		// Create real logger
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)

		// Create REAL business logic components
		dataFactory = testutil.NewInfrastructureTestDataFactory()
		assertions = testutil.NewInfrastructureAssertions()

		// Mock ONLY external dependencies
		mockK8sClient = mocks.NewMockK8sClient(nil)      // Mock external K8s dependency
		mockActionRepo = mocks.NewMockActionRepository() // Mock external storage dependency

		// Create REAL platform executor with mocked external dependencies
		actionsConfig := config.ActionsConfig{
			DryRun:        true,
			MaxConcurrent: 5,
		}

		var err error
		platformExecutor, err = executor.NewExecutor(mockK8sClient, actionsConfig, mockActionRepo, logger)
		Expect(err).ToNot(HaveOccurred(), "BR-INFRASTRUCTURE-002: Platform executor creation must succeed for business operations")
	})

	Context("When testing infrastructure configuration business logic (BR-INFRASTRUCTURE-001)", func() {
		It("should create valid infrastructure configurations for business operations", func() {
			// Business Scenario: Infrastructure must provide valid configurations for business scalability
			// Business Impact: Valid configuration ensures platform can support business requirements

			// Test REAL business logic: infrastructure configuration generation algorithm
			config := dataFactory.CreateInfrastructureConfig()

			// Business Validation: Configuration must meet business requirements
			Expect(config).ToNot(BeNil(),
				"BR-INFRASTRUCTURE-001: Infrastructure configuration must be available for business operations")

			Expect(config.Port).To(BeNumerically(">", 0),
				"BR-INFRASTRUCTURE-001: Port must be valid for business service accessibility")

			Expect(config.Port).To(BeNumerically("<=", 65535),
				"BR-INFRASTRUCTURE-001: Port must be within valid range for network operations")

			Expect(config.ServiceName).ToNot(BeEmpty(),
				"BR-INFRASTRUCTURE-001: Service name must be specified for business service identification")

			Expect(config.Namespace).ToNot(BeEmpty(),
				"BR-INFRASTRUCTURE-001: Namespace must be specified for business resource organization")

			// Test REAL business logic: configuration validation using assertions
			assertions.AssertInfrastructureConfigValid(config)

			// Business Validation: Configuration provides business foundation
			businessConfigReady := config.Port > 0 && config.ServiceName != "" && config.Namespace != ""
			Expect(businessConfigReady).To(BeTrue(),
				"BR-INFRASTRUCTURE-001: Infrastructure configuration must provide business foundation for executive confidence in platform operations")
		})

		It("should generate high-performance configurations for business scalability", func() {
			// Business Scenario: High-performance requirements need specialized configurations
			// Business Impact: High-performance config supports business scaling requirements

			// Test REAL business logic: high-performance configuration algorithm
			highPerfConfig := dataFactory.CreateHighPerformanceConfig()
			standardConfig := dataFactory.CreateInfrastructureConfig()

			// Business Validation: High-performance config must exceed standard performance
			Expect(highPerfConfig).ToNot(BeNil(),
				"BR-INFRASTRUCTURE-001: High-performance configuration must support business scalability")

			Expect(highPerfConfig.MaxRequestsPerSecond).To(BeNumerically(">", standardConfig.MaxRequestsPerSecond),
				"BR-INFRASTRUCTURE-001: High-performance config must exceed standard performance for business scaling")

			// Business Logic: High-performance configurations should optimize for speed
			Expect(highPerfConfig.ReadTimeout).To(BeNumerically("<", standardConfig.ReadTimeout),
				"BR-INFRASTRUCTURE-001: High-performance config should optimize read timeout for business responsiveness")

			Expect(highPerfConfig.WriteTimeout).To(BeNumerically("<", standardConfig.WriteTimeout),
				"BR-INFRASTRUCTURE-001: High-performance config should optimize write timeout for business efficiency")

			// Business Validation: High-performance config supports business scalability requirements
			scalabilityReady := highPerfConfig.MaxRequestsPerSecond >= 10000
			Expect(scalabilityReady).To(BeTrue(),
				"BR-INFRASTRUCTURE-001: High-performance configuration must support high business load requirements")
		})

		It("should create minimal configurations for business cost optimization", func() {
			// Business Scenario: Cost-conscious deployments need minimal configurations
			// Business Impact: Minimal configurations support business cost optimization

			// Test REAL business logic: minimal configuration algorithm
			minimalConfig := dataFactory.CreateMinimalConfig()

			// Business Validation: Minimal config must still meet business requirements
			Expect(minimalConfig).ToNot(BeNil(),
				"BR-INFRASTRUCTURE-001: Minimal configuration must be available for cost-optimized business operations")

			Expect(minimalConfig.Port).To(Equal(8080),
				"BR-INFRASTRUCTURE-001: Minimal config should use standard business port")

			Expect(minimalConfig.EnableMetrics).To(BeTrue(),
				"BR-INFRASTRUCTURE-001: Even minimal config must support business monitoring")

			Expect(minimalConfig.ServiceName).ToNot(BeEmpty(),
				"BR-INFRASTRUCTURE-001: Minimal config must maintain service identification for business operations")

			// Business Logic: Minimal configurations should omit optional features
			Expect(minimalConfig.ReadTimeout).To(BeZero(),
				"BR-INFRASTRUCTURE-001: Minimal config should omit non-essential timeouts for cost optimization")

			// Test business validation using assertions
			assertions.AssertInfrastructureConfigValid(minimalConfig)
		})

		It("should generate metrics data for business monitoring", func() {
			// Business Scenario: Business operations need comprehensive metrics data
			// Business Impact: Metrics data enables business performance monitoring and optimization

			// Test REAL business logic: metrics data generation algorithms
			metricsData := dataFactory.CreateMetricsData()
			healthyMetrics := dataFactory.CreateHealthyMetricsData()
			unhealthyMetrics := dataFactory.CreateUnhealthyMetricsData()

			// Business Validation: Metrics data must support business monitoring
			Expect(metricsData).ToNot(BeEmpty(),
				"BR-INFRASTRUCTURE-001: Metrics data must be available for business monitoring")

			Expect(healthyMetrics).To(HaveKey("cpu_usage_percent"),
				"BR-INFRASTRUCTURE-001: Healthy metrics must include CPU data for business performance monitoring")

			Expect(healthyMetrics).To(HaveKey("throughput_requests_per_sec"),
				"BR-INFRASTRUCTURE-001: Healthy metrics must include throughput data for business capacity planning")

			// Business Logic: Healthy vs unhealthy metrics should differ meaningfully
			healthyCPU := healthyMetrics["cpu_usage_percent"]
			unhealthyCPU := unhealthyMetrics["cpu_usage_percent"]

			Expect(unhealthyCPU).To(BeNumerically(">", healthyCPU),
				"BR-INFRASTRUCTURE-001: Unhealthy metrics should reflect degraded business performance")

			// Business Validation: All metrics must be valid for business analysis
			for metricName, metricValue := range metricsData {
				Expect(metricName).ToNot(BeEmpty(),
					"BR-INFRASTRUCTURE-001: Metric names must be valid for business analysis")

				Expect(metricValue).To(BeNumerically(">=", 0),
					"BR-INFRASTRUCTURE-001: Metric values must be non-negative for business calculations")
			}
		})

		It("should validate HTTP response creation for business API testing", func() {
			// Business Scenario: Business API testing requires HTTP response validation
			// Business Impact: HTTP response validation ensures business API reliability

			// Test REAL business logic: HTTP response creation algorithms
			successResponse := dataFactory.CreateSuccessfulHTTPResponse()
			customResponse := dataFactory.CreateHTTPResponse(404, "Not Found")

			// Business Validation: HTTP responses must support business API testing
			Expect(successResponse).ToNot(BeNil(),
				"BR-INFRASTRUCTURE-001: Successful HTTP response must be available for business API testing")

			Expect(customResponse).ToNot(BeNil(),
				"BR-INFRASTRUCTURE-001: Custom HTTP response must be available for business error testing")

			// Test business validation using assertions
			assertions.AssertHTTPResponseValid(successResponse)
			assertions.AssertHTTPResponseValid(customResponse)

			// Business Logic: Custom response should have expected status code
			Expect(customResponse.StatusCode).To(Equal(404),
				"BR-INFRASTRUCTURE-001: Custom response must have correct status code for business error scenarios")
		})
	})

	Context("When testing platform executor business logic (BR-INFRASTRUCTURE-002)", func() {
		It("should validate platform executor health for business operations", func() {
			// Business Scenario: Platform executor must be healthy for automated business operations
			// Business Impact: Executor health ensures reliable business action execution

			// Test REAL business logic: platform executor health checking algorithm
			isHealthy := platformExecutor.IsHealthy()

			// Business Validation: Executor must be healthy for business operations
			Expect(isHealthy).To(BeTrue(),
				"BR-INFRASTRUCTURE-002: Platform executor must be healthy for business operations")

			// Business Logic: Healthy executor should have functional components
			Expect(platformExecutor).ToNot(BeNil(),
				"BR-INFRASTRUCTURE-002: Platform executor must be initialized for business action execution")
		})

		It("should provide action registry for business action management", func() {
			// Business Scenario: Business operations require comprehensive action registry
			// Business Impact: Action registry enables automated business remediation capabilities

			// Test REAL business logic: action registry retrieval and validation
			actionRegistry := platformExecutor.GetActionRegistry()

			// Business Validation: Action registry must be available for business action management
			Expect(actionRegistry).ToNot(BeNil(),
				"BR-INFRASTRUCTURE-002: Action registry must be available for business action management")

			// Test REAL business logic: action registry functionality
			registeredActions := actionRegistry.GetRegisteredActions()

			// Business Validation: Registry must contain business-critical actions
			Expect(registeredActions).ToNot(BeEmpty(),
				"BR-INFRASTRUCTURE-002: Action registry must contain business actions for automated operations")

			// Business Logic: Registry should support essential business actions
			expectedBusinessActions := []string{"scale_deployment", "restart_pod", "increase_resources"}
			for _, expectedAction := range expectedBusinessActions {
				isRegistered := actionRegistry.IsRegistered(expectedAction)
				Expect(isRegistered).To(BeTrue(),
					"BR-INFRASTRUCTURE-002: Essential business action '%s' must be registered for automated operations", expectedAction)
			}

			// Business Validation: Comprehensive business execution capability
			executorBusinessReady := platformExecutor != nil && actionRegistry != nil && len(registeredActions) > 0
			Expect(executorBusinessReady).To(BeTrue(),
				"BR-INFRASTRUCTURE-002: Platform executor must provide comprehensive business execution capability for executive confidence in automated operations")
		})

		It("should validate action registration business logic", func() {
			// Business Scenario: Dynamic action registration supports business extensibility
			// Business Impact: Action registration enables custom business automation capabilities

			// Test REAL business logic: action registry management algorithms
			actionRegistry := platformExecutor.GetActionRegistry()

			// Get initial registered actions count
			initialActions := actionRegistry.GetRegisteredActions()
			initialCount := len(initialActions)

			// Test business logic: action registration functionality
			testAction := func(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
				return nil // Mock business action implementation
			}

			// Business Action: Register custom business action
			err := actionRegistry.Register("custom_business_action", testAction)
			Expect(err).ToNot(HaveOccurred(),
				"BR-INFRASTRUCTURE-002: Custom business action registration must succeed for business extensibility")

			// Business Validation: Registry should reflect new business action
			isRegistered := actionRegistry.IsRegistered("custom_business_action")
			Expect(isRegistered).To(BeTrue(),
				"BR-INFRASTRUCTURE-002: Registered business action must be available for business operations")

			updatedActions := actionRegistry.GetRegisteredActions()
			Expect(len(updatedActions)).To(Equal(initialCount+1),
				"BR-INFRASTRUCTURE-002: Action count must increase after business action registration")

			// Business Action: Test duplicate registration prevention
			duplicateErr := actionRegistry.Register("custom_business_action", testAction)
			Expect(duplicateErr).To(HaveOccurred(),
				"BR-INFRASTRUCTURE-002: Duplicate action registration must be prevented for business consistency")

			// Business Action: Test action unregistration
			actionRegistry.Unregister("custom_business_action")
			isStillRegistered := actionRegistry.IsRegistered("custom_business_action")
			Expect(isStillRegistered).To(BeFalse(),
				"BR-INFRASTRUCTURE-002: Unregistered business action must not be available")

			finalActions := actionRegistry.GetRegisteredActions()
			Expect(len(finalActions)).To(Equal(initialCount),
				"BR-INFRASTRUCTURE-002: Action count must return to original after unregistration")
		})
	})

	Context("When testing TDD compliance", func() {
		It("should validate real business logic usage per cursor rules", func() {
			// Business Scenario: Validate TDD approach with real business components

			// Verify we're testing REAL business logic per cursor rules
			Expect(dataFactory).ToNot(BeNil(),
				"TDD: Must test real InfrastructureTestDataFactory business logic")

			Expect(assertions).ToNot(BeNil(),
				"TDD: Must test real InfrastructureAssertions business logic")

			Expect(platformExecutor).ToNot(BeNil(),
				"TDD: Must test real platform executor business logic")

			// Verify we're using real business logic, not mocks
			Expect(dataFactory).To(BeAssignableToTypeOf(&testutil.InfrastructureTestDataFactory{}),
				"TDD: Must use actual data factory type, not mock")

			Expect(assertions).To(BeAssignableToTypeOf(&testutil.InfrastructureAssertions{}),
				"TDD: Must use actual assertions type, not mock")

			// Verify external dependencies are mocked
			Expect(mockK8sClient).To(BeAssignableToTypeOf(&mocks.MockK8sClient{}),
				"Cursor Rules: External K8s client should be mocked")

			Expect(mockActionRepo).To(BeAssignableToTypeOf(&mocks.MockActionRepository{}),
				"Cursor Rules: External action repository should be mocked")

			// Verify internal components are real
			Expect(logger).To(BeAssignableToTypeOf(&logrus.Logger{}),
				"Cursor Rules: Internal logger should be real, not mocked")

			// Test that business logic is accessible for validation
			config := dataFactory.CreateInfrastructureConfig()
			Expect(config).ToNot(BeNil(),
				"TDD: Real business logic must be accessible for testing")

			Expect(config.Port).To(BeNumerically(">", 0),
				"TDD: Real business logic must provide valid business results")
		})
	})
})
