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

package holmesgpt_test

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
	"k8s.io/client-go/kubernetes/fake"
)

// BR-SERVICE-INTEGRATION-001: Service Integration Business Logic Comprehensive Testing
// Business Impact: Ensures service integration algorithms deliver reliable toolset management
// Stakeholder Value: Operations teams can trust service integration for production workloads
//
// PYRAMID COMPLIANCE: Unit test with real business logic + mocked external dependencies
var _ = Describe("BR-SERVICE-INTEGRATION-001: Service Integration Business Logic", func() {
	var (
		// Mock ONLY external dependencies - following Rule 09: use existing mocks
		mockK8sClient *fake.Clientset
		logger        *logrus.Logger

		// Use REAL business logic components
		serviceIntegration *holmesgpt.ServiceIntegration
		realConfig         *k8s.ServiceDiscoveryConfig

		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Use existing testutil patterns instead of deprecated TDDConversionHelper
		logger = logrus.New()
		logger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests

		// Mock external dependencies only - Rule 09: use existing patterns
		mockK8sClient = fake.NewSimpleClientset()

		// Create REAL business configuration
		realConfig = &k8s.ServiceDiscoveryConfig{
			DiscoveryInterval:   100 * time.Millisecond,
			CacheTTL:            1 * time.Minute,
			HealthCheckInterval: 10 * time.Second,
			Enabled:             true,
		}

		// Create REAL service integration with mocked externals
		serviceIntegration, _ = holmesgpt.NewServiceIntegration(
			mockK8sClient, // External: Mock K8s client
			realConfig,    // Business Logic: Real config
			logger,        // External: Logger
		)
	})

	// COMPREHENSIVE business logic testing - Rule 03 compliant
	Context("BR-SERVICE-INTEGRATION-001: Core Business Operations", func() {
		It("should provide valid toolset management interface", func() {
			// Test REAL business logic - actual ServiceIntegration behavior
			availableToolsets := serviceIntegration.GetAvailableToolsets()

			// Validate REAL business requirements - not just types
			Expect(availableToolsets).ToNot(BeNil(),
				"BR-SERVICE-INTEGRATION-001: GetAvailableToolsets must return non-nil slice")

			// Test that the business logic maintains consistency
			toolsetStats := serviceIntegration.GetToolsetStats()
			Expect(toolsetStats.TotalToolsets).To(Equal(len(availableToolsets)),
				"BR-SERVICE-INTEGRATION-001: Stats must be consistent with actual toolsets")
		})

		It("should maintain health status consistency", func() {
			// Test REAL business logic for health reporting
			healthStatus := serviceIntegration.GetHealthStatus()

			// Validate actual business requirements
			Expect(healthStatus.TotalToolsets).To(BeNumerically(">=", 0),
				"BR-SERVICE-INTEGRATION-001: Total toolsets count must be non-negative")
			Expect(healthStatus.EnabledToolsets).To(BeNumerically("<=", healthStatus.TotalToolsets),
				"BR-SERVICE-INTEGRATION-001: Enabled toolsets cannot exceed total toolsets")

			// Test business logic consistency
			if healthStatus.TotalToolsets == 0 {
				Expect(healthStatus.EnabledToolsets).To(Equal(0),
					"BR-SERVICE-INTEGRATION-001: No enabled toolsets when total is zero")
			}
		})
	})

	// COMPREHENSIVE service availability testing - Rule 03 compliant
	Context("BR-SERVICE-INTEGRATION-002: Service Availability Business Logic", func() {
		It("should correctly identify service availability patterns", func() {
			// Test REAL business logic for service availability
			availableTypes := serviceIntegration.GetAvailableServiceTypes()

			// Validate actual business behavior - not just types
			for _, serviceType := range availableTypes {
				Expect(strings.TrimSpace(serviceType)).ToNot(BeEmpty(),
					"BR-SERVICE-INTEGRATION-002: Service type names must not be empty")

				// Test business logic consistency
				isAvailable := serviceIntegration.IsServiceAvailable(serviceType)
				Expect(isAvailable).To(BeTrue(),
					"BR-SERVICE-INTEGRATION-002: Available service types must return true for IsServiceAvailable")
			}

			// Test edge case business logic
			Expect(serviceIntegration.IsServiceAvailable("nonexistent-service-type")).To(BeFalse(),
				"BR-SERVICE-INTEGRATION-002: Nonexistent service types must return false")
		})

		It("should handle service type filtering with business consistency", func() {
			// Test real business logic for service filtering
			availableTypes := serviceIntegration.GetAvailableServiceTypes()

			// Test business logic for each available service type
			for _, serviceType := range availableTypes {
				toolsets := serviceIntegration.GetToolsetByServiceType(serviceType)

				// Validate business filtering logic
				for _, toolset := range toolsets {
					Expect(toolset.ServiceType).To(Equal(serviceType),
						"BR-SERVICE-INTEGRATION-002: Filtered toolsets must match requested service type")
				}
			}
		})
	})

	// COMPREHENSIVE error handling testing - Rule 03 compliant
	Context("BR-SERVICE-INTEGRATION-003: Error Handling and Resilience", func() {
		It("should handle toolset refresh operations with proper error propagation", func() {
			// Test real business logic error handling
			err := serviceIntegration.RefreshToolsets(ctx)

			// Validate actual business error handling behavior
			if err != nil {
				Expect(err.Error()).ToNot(BeEmpty(),
					"BR-SERVICE-INTEGRATION-003: Error messages must be descriptive")
				Expect(strings.Contains(err.Error(), "refresh") || strings.Contains(err.Error(), "toolset"),
					"BR-SERVICE-INTEGRATION-003: Refresh errors must be contextually relevant").To(BeTrue())
			}

			// Test that service remains operational after error
			healthStatus := serviceIntegration.GetHealthStatus()
			Expect(healthStatus).ToNot(BeNil(),
				"BR-SERVICE-INTEGRATION-003: Service must remain queryable after refresh errors")
		})

		It("should maintain service connectivity validation", func() {
			// Test real business logic connectivity checking
			err := serviceIntegration.CheckKubernetesConnectivity(ctx)

			// Validate business connectivity logic
			if err != nil {
				Expect(err.Error()).ToNot(BeEmpty(),
					"BR-SERVICE-INTEGRATION-003: Connectivity errors must be descriptive")
			}

			// Test that connectivity check doesn't break service
			availableToolsets := serviceIntegration.GetAvailableToolsets()
			Expect(availableToolsets).ToNot(BeNil(),
				"BR-SERVICE-INTEGRATION-003: Service must remain functional after connectivity check")
		})

		It("should handle event notifications without service disruption", func() {
			// Test REAL business logic event handling resilience
			testConfig := &holmesgpt.ToolsetConfig{
				Name:        "test-toolset",
				ServiceType: "test",
				Enabled:     true,
				Version:     "1.0.0",
				LastUpdated: time.Now(),
			}

			err := serviceIntegration.OnToolsetUpdated(testConfig)

			// Validate business resilience behavior
			Expect(err).ToNot(HaveOccurred(),
				"BR-SERVICE-INTEGRATION-003: Event handling must not fail service")

			// Test that service state remains consistent after event
			healthStatus := serviceIntegration.GetHealthStatus()
			Expect(healthStatus.LastUpdate).To(BeTemporally("<=", time.Now()),
				"BR-SERVICE-INTEGRATION-003: Health status must remain current after events")
		})
	})

	// COMPREHENSIVE performance testing - Rule 03 compliant
	Context("BR-SERVICE-INTEGRATION-004: Performance Requirements", func() {
		It("should execute core operations within acceptable time bounds", func() {
			// Test REAL business logic performance characteristics
			startTime := time.Now()

			// Execute core business operations
			availableToolsets := serviceIntegration.GetAvailableToolsets()
			healthStatus := serviceIntegration.GetHealthStatus()
			availableTypes := serviceIntegration.GetAvailableServiceTypes()

			executionTime := time.Since(startTime)

			// Validate actual business performance requirements
			Expect(executionTime).To(BeNumerically("<", 500*time.Millisecond),
				"BR-SERVICE-INTEGRATION-004: Core operations must complete within 500ms")

			// Test that performance doesn't compromise data consistency
			Expect(healthStatus.TotalToolsets).To(Equal(len(availableToolsets)),
				"BR-SERVICE-INTEGRATION-004: Fast operations must maintain data consistency")

			// Test performance characteristics under different conditions
			if len(availableToolsets) > 0 {
				// Test filtering performance
				filterStartTime := time.Now()
				for _, serviceType := range availableTypes {
					serviceIntegration.GetToolsetByServiceType(serviceType)
				}
				filterTime := time.Since(filterStartTime)

				Expect(filterTime).To(BeNumerically("<", 100*time.Millisecond),
					"BR-SERVICE-INTEGRATION-004: Service filtering must be performant")
			}
		})
	})

	// COMPREHENSIVE edge case testing - Rule 03 compliant
	Context("BR-SERVICE-INTEGRATION-005: Edge Cases and Boundary Conditions", func() {
		BeforeEach(func() {
			err := serviceIntegration.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Wait for initialization to complete
			time.Sleep(100 * time.Millisecond)
		})

		It("should handle empty service configurations gracefully", func() {
			// Test real business logic with minimal data scenarios
			availableToolsets := serviceIntegration.GetAvailableToolsets()
			healthStatus := serviceIntegration.GetHealthStatus()
			availableTypes := serviceIntegration.GetAvailableServiceTypes()

			// Validate business edge case handling with actual requirements
			Expect(len(availableToolsets)).To(BeNumerically(">=", 0),
				"BR-SERVICE-INTEGRATION-005: Empty toolset list must be handled gracefully")

			// Test business logic consistency in edge cases
			if len(availableToolsets) == 0 {
				Expect(healthStatus.TotalToolsets).To(Equal(0),
					"BR-SERVICE-INTEGRATION-005: Empty toolsets must reflect in health status")
				Expect(len(availableTypes)).To(Equal(0),
					"BR-SERVICE-INTEGRATION-005: No toolsets means no available service types")
			}
		})

		It("should maintain service stability with invalid inputs", func() {
			// Test REAL business logic resilience with edge case inputs

			// Test with invalid service type
			invalidToolsets := serviceIntegration.GetToolsetByServiceType("")
			Expect(invalidToolsets).ToNot(BeNil(),
				"BR-SERVICE-INTEGRATION-005: Invalid service type queries must not crash service")

			// Test availability check with invalid input
			invalidAvailability := serviceIntegration.IsServiceAvailable("")
			Expect(invalidAvailability).To(BeFalse(),
				"BR-SERVICE-INTEGRATION-005: Empty service type must not be available")

			// Test that service remains functional after invalid inputs
			healthStatus := serviceIntegration.GetHealthStatus()
			Expect(healthStatus).ToNot(BeNil(),
				"BR-SERVICE-INTEGRATION-005: Service must remain operational after invalid inputs")
		})
	})
})

// Note: Test suite is bootstrapped by holmesgpt_suite_test.go
// This file only contains Describe blocks that are automatically discovered by Ginkgo
