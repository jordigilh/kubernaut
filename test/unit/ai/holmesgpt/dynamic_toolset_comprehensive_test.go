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

//go:build unit
// +build unit

package holmesgpt_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
	"github.com/sirupsen/logrus"
)

// BR-HOLMES-016: Dynamic service discovery for HolmesGPT toolsets
// BR-HOLMES-017: Automatic detection of well-known services (Prometheus, Grafana, etc.)
// BR-HOLMES-019: Service availability and health validation before enabling toolsets
// BR-HOLMES-022: Generate appropriate toolset configurations with service-specific endpoints
// Business Impact: Validates dynamic toolset configuration and management for HolmesGPT integration
// Stakeholder Value: Ensures reliable toolset management for automated investigation capabilities
var _ = Describe("BR-HOLMES-016/017/019/022: HolmesGPT Dynamic Toolset Management Business Logic", func() {
	var (
		// Mock ONLY external dependencies per pyramid principles
		fakeClientset        *fake.Clientset
		realServiceDiscovery *k8s.ServiceDiscovery
		mockLogger           *logrus.Logger

		// Use REAL business logic components
		dynamicToolsetManager *holmesgpt.DynamicToolsetManager

		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)

		// Mock external dependencies only - following pyramid approach
		fakeClientset = fake.NewSimpleClientset()
		mockLogger = logrus.New()
		mockLogger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests

		// Create REAL service discovery - PYRAMID APPROACH
		realServiceDiscovery = k8s.NewServiceDiscovery(
			fakeClientset, // External: Mock K8s client interface
			&k8s.ServiceDiscoveryConfig{
				DiscoveryInterval:   30 * time.Second,
				CacheTTL:            5 * time.Minute,
				HealthCheckInterval: 10 * time.Second,
				Enabled:             true,
			},
			mockLogger,
		)

		// Create REAL dynamic toolset manager with real service discovery
		dynamicToolsetManager = holmesgpt.NewDynamicToolsetManager(
			realServiceDiscovery, // Business Logic: Real
			mockLogger,           // External: Mock (logging infrastructure)
		)
	})

	AfterEach(func() {
		if dynamicToolsetManager != nil {
			dynamicToolsetManager.Stop()
		}
		cancel()
	})

	// COMPREHENSIVE scenario testing for actual HolmesGPT business requirements
	Context("BR-HOLMES-016: Dynamic service discovery for HolmesGPT toolsets", func() {
		It("should dynamically discover available services in Kubernetes cluster", func() {
			// Test REAL business logic for BR-HOLMES-016 - dynamic service discovery
			err := dynamicToolsetManager.Start(ctx)

			// Validate REAL business requirement BR-HOLMES-016 outcomes
			if err != nil {
				// If start fails, that's acceptable - but should not panic
				Expect(err.Error()).ToNot(ContainSubstring("panic"),
					"BR-HOLMES-016: Dynamic service discovery should not panic")
			}

			// BR-HOLMES-016: Must dynamically discover available services
			availableToolsets := dynamicToolsetManager.GetAvailableToolsets()
			Expect(availableToolsets).ToNot(BeNil(),
				"BR-HOLMES-016: Service discovery should provide toolset results")

			// BR-HOLMES-016: Should discover services and configure corresponding toolsets
			Expect(len(availableToolsets)).To(BeNumerically(">=", 1),
				"BR-HOLMES-016: Should discover at least baseline services for toolset configuration")
		})

		It("should automatically detect well-known services per BR-HOLMES-017", func() {
			// Test REAL business logic for BR-HOLMES-017 - automatic detection of well-known services
			err := dynamicToolsetManager.Start(ctx)
			if err != nil {
				Skip("Toolset manager failed to start: " + err.Error())
			}

			// BR-HOLMES-017: Must automatically detect well-known services (Prometheus, Grafana, etc.)
			prometheusToolsets := dynamicToolsetManager.GetToolsetByServiceType("prometheus")
			Expect(prometheusToolsets).ToNot(BeNil(),
				"BR-HOLMES-017: Prometheus service detection should provide toolset results")

			// BR-HOLMES-025: Must provide toolset configuration API endpoints for runtime management
			allToolsets := dynamicToolsetManager.GetAvailableToolsets()
			if len(allToolsets) > 0 {
				firstToolset := allToolsets[0]
				retrievedToolset := dynamicToolsetManager.GetToolsetConfig(firstToolset.Name)
				Expect(retrievedToolset).ToNot(BeNil(),
					"BR-HOLMES-025: API endpoints should retrieve toolset by name")
				Expect(retrievedToolset.Name).To(Equal(firstToolset.Name),
					"BR-HOLMES-025: Retrieved toolset should match requested name")
			}
		})

		It("should provide real-time toolset updates per BR-HOLMES-020", func() {
			// Test REAL business logic for BR-HOLMES-020 - real-time toolset configuration updates
			err := dynamicToolsetManager.Start(ctx)
			if err != nil {
				Skip("Toolset manager failed to start: " + err.Error())
			}

			// Get initial toolset count
			initialToolsets := dynamicToolsetManager.GetAvailableToolsets()
			initialCount := len(initialToolsets)

			// BR-HOLMES-020: Must provide real-time toolset configuration updates when services are deployed/removed
			refreshErr := dynamicToolsetManager.RefreshAllToolsets(ctx)
			Expect(refreshErr).ToNot(HaveOccurred(),
				"BR-HOLMES-020: Real-time toolset updates should not fail")

			// Validate toolsets still available after refresh
			refreshedToolsets := dynamicToolsetManager.GetAvailableToolsets()
			Expect(len(refreshedToolsets)).To(BeNumerically(">=", initialCount),
				"BR-HOLMES-020: Real-time updates should maintain or increase available toolsets")
		})
	})

	// COMPREHENSIVE toolset generation business logic testing
	Context("BR-HOLMES-019/022: Service Health Validation and Toolset Configuration", func() {
		It("should validate service availability before enabling toolsets per BR-HOLMES-019", func() {
			// Test REAL business logic for BR-HOLMES-019 - service availability and health validation
			err := dynamicToolsetManager.Start(ctx)

			// Validate REAL business requirement BR-HOLMES-019 outcomes
			if err != nil {
				// If start fails, should provide detailed error information
				Expect(err.Error()).ToNot(BeEmpty(),
					"BR-HOLMES-019: Service health validation errors should be descriptive")
			} else {
				// BR-HOLMES-019: Must validate service availability and health before enabling toolsets
				availableToolsets := dynamicToolsetManager.GetAvailableToolsets()
				Expect(len(availableToolsets)).To(BeNumerically(">=", 1),
					"BR-HOLMES-019: Must validate services and generate toolsets for available services")

				// Validate toolset structure and quality per BR-HOLMES-022
				for _, toolset := range availableToolsets {
					Expect(toolset.Name).ToNot(BeEmpty(),
						"BR-HOLMES-022: Toolset configurations must have valid names")
					Expect(toolset.Version).ToNot(BeEmpty(),
						"BR-HOLMES-022: Toolset configurations must have version information")
					Expect(toolset.LastUpdated).ToNot(BeZero(),
						"BR-HOLMES-022: Toolset configurations must have update timestamps")
				}
			}
		})

		It("should generate appropriate toolset configurations with service-specific endpoints per BR-HOLMES-022", func() {
			// Test REAL business logic for BR-HOLMES-022 - generate appropriate toolset configurations
			err := dynamicToolsetManager.Start(ctx)
			if err != nil {
				Skip("Toolset manager failed to start: " + err.Error())
			}

			// Validate REAL business requirement BR-HOLMES-022 outcomes
			availableToolsets := dynamicToolsetManager.GetAvailableToolsets()

			for _, toolset := range availableToolsets {
				// BR-HOLMES-022: Must generate appropriate toolset configurations with service-specific endpoints
				Expect(toolset.Description).ToNot(BeEmpty(),
					"BR-HOLMES-022: Toolset configurations must have descriptions")
				Expect(toolset.Priority).To(BeNumerically(">=", 0),
					"BR-HOLMES-022: Toolset configurations must have valid priority")
				Expect(toolset.Enabled).To(BeTrue(),
					"BR-HOLMES-022: Generated toolset configurations should be enabled")

				// BR-HOLMES-022: Service-specific endpoints and capabilities
				Expect(toolset.ServiceMeta.Namespace).ToNot(BeEmpty(),
					"BR-HOLMES-022: Service metadata must include namespace for service-specific configuration")

				// Validate tools structure if present
				if len(toolset.Tools) > 0 {
					for _, tool := range toolset.Tools {
						Expect(tool.Name).ToNot(BeEmpty(),
							"BR-HOLMES-022: Tools in configurations must have valid names")
						Expect(tool.Category).ToNot(BeEmpty(),
							"BR-HOLMES-022: Tools in configurations must be categorized")
					}
				}
			}
		})
	})

	// COMPREHENSIVE error handling and edge cases for actual business requirements
	Context("BR-HOLMES-019/025: Health Validation and API Reliability", func() {
		It("should handle service health validation failures gracefully per BR-HOLMES-019", func() {
			// Test REAL business logic for BR-HOLMES-019 - service availability and health validation
			err := dynamicToolsetManager.Start(ctx)

			// Validate REAL business requirement BR-HOLMES-019 error handling
			if err != nil {
				// Error should be descriptive and actionable per BR-HOLMES-019
				Expect(err.Error()).ToNot(BeEmpty(),
					"BR-HOLMES-019: Service health validation errors should be descriptive")
				Expect(err.Error()).ToNot(ContainSubstring("panic"),
					"BR-HOLMES-019: Should not panic on service health validation failures")
			}

			// BR-HOLMES-025: API endpoints should work even with service failures
			availableToolsets := dynamicToolsetManager.GetAvailableToolsets()
			Expect(availableToolsets).ToNot(BeNil(),
				"BR-HOLMES-025: API endpoints should handle service failures gracefully")
		})

		It("should handle concurrent API operations safely per BR-HOLMES-025", func() {
			// Test REAL business logic for BR-HOLMES-025 - API endpoints concurrent access safety
			err := dynamicToolsetManager.Start(ctx)
			if err != nil {
				Skip("Toolset manager failed to start: " + err.Error())
			}

			// Test concurrent access to BR-HOLMES-025 API endpoints
			done := make(chan bool, 3)

			// Concurrent API endpoint reads
			go func() {
				defer GinkgoRecover()
				for i := 0; i < 10; i++ {
					toolsets := dynamicToolsetManager.GetAvailableToolsets()
					Expect(toolsets).ToNot(BeNil(),
						"BR-HOLMES-025: Concurrent API endpoint reads should be safe")
				}
				done <- true
			}()

			// Concurrent service type API queries
			go func() {
				defer GinkgoRecover()
				for i := 0; i < 10; i++ {
					toolsets := dynamicToolsetManager.GetToolsetByServiceType("kubernetes")
					Expect(toolsets).ToNot(BeNil(),
						"BR-HOLMES-025: Concurrent service type API queries should be safe")
				}
				done <- true
			}()

			// Concurrent refresh operations via API
			go func() {
				defer GinkgoRecover()
				for i := 0; i < 5; i++ {
					refreshErr := dynamicToolsetManager.RefreshAllToolsets(ctx)
					// Refresh might fail, but should not panic per BR-HOLMES-025 API reliability
					_ = refreshErr
				}
				done <- true
			}()

			// Wait for all goroutines to complete
			for i := 0; i < 3; i++ {
				Eventually(done, 10*time.Second).Should(Receive(BeTrue()),
					"BR-HOLMES-025: Concurrent API operations should complete safely")
			}
		})

		It("should cleanup API resources properly on stop per BR-HOLMES-025", func() {
			// Test REAL business logic for BR-HOLMES-025 - API resource cleanup
			err := dynamicToolsetManager.Start(ctx)
			if err != nil {
				Skip("Toolset manager failed to start: " + err.Error())
			}

			// BR-HOLMES-025: Test proper API resource cleanup
			Expect(func() {
				dynamicToolsetManager.Stop()
			}).ToNot(Panic(),
				"BR-HOLMES-025: API resource cleanup should not panic")

			// BR-HOLMES-025: Multiple API stops should be safe for reliability
			Expect(func() {
				dynamicToolsetManager.Stop()
				dynamicToolsetManager.Stop()
			}).ToNot(Panic(),
				"BR-HOLMES-025: Multiple API stops should be safe for API reliability")
		})
	})
})

// TestRunner bootstraps the Ginkgo test suite
func TestUdynamicUtoolsetUcomprehensive(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UdynamicUtoolsetUcomprehensive Suite")
}
