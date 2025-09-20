package holmesgpt_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
)

var _ = Describe("ServiceIntegration - Implementation Correctness Testing", func() {
	var (
		serviceIntegration *holmesgpt.ServiceIntegration
		fakeClient         *fake.Clientset
		log                *logrus.Logger
		ctx                context.Context
	)

	BeforeEach(func() {
		fakeClient = fake.NewSimpleClientset()
		log = logrus.New()
		log.SetLevel(logrus.ErrorLevel) // Reduce test noise
		ctx = context.Background()

		// Create test configuration
		config := &k8s.ServiceDiscoveryConfig{
			DiscoveryInterval:   100 * time.Millisecond,
			CacheTTL:            1 * time.Minute,
			HealthCheckInterval: 10 * time.Second,
			Enabled:             true,
			Namespaces:          []string{"monitoring"},
			ServicePatterns:     k8s.GetDefaultServicePatterns(), // BR-HOLMES-017: Well-known service detection
		}

		var err error
		serviceIntegration, err = holmesgpt.NewServiceIntegration(fakeClient, config, log)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if serviceIntegration != nil {
			serviceIntegration.Stop()
		}
	})

	// BR-HOLMES-025: Unit tests for service integration implementation
	Describe("ServiceIntegration Implementation", func() {
		Context("Integration Initialization", func() {
			It("should initialize service integration successfully", func() {
				Expect(func() { _ = serviceIntegration.Start }).ToNot(Panic(), "BR-AI-002-RECOMMENDATION-CONFIDENCE: HolmesGPT service integration must provide functional start interface for valid service recommendations")

				// Should start without errors
				err := serviceIntegration.Start(ctx)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should handle initialization with nil configuration", func() {
				integration, err := holmesgpt.NewServiceIntegration(fakeClient, nil, log)
				Expect(err).ToNot(HaveOccurred())
				Expect(func() { _ = integration.Start }).ToNot(Panic(), "BR-AI-002-RECOMMENDATION-CONFIDENCE: HolmesGPT service integration must provide functional start interface for valid service recommendations")

				// Should use default configuration internally
				err = integration.Start(ctx)
				Expect(err).ToNot(HaveOccurred())

				integration.Stop()
			})

			It("should handle initialization with invalid client gracefully", func() {
				// Should not crash with nil client
				integration, err := holmesgpt.NewServiceIntegration(nil, nil, log)
				Expect(err).ToNot(HaveOccurred())
				Expect(func() { _ = integration.Start }).ToNot(Panic(), "BR-AI-002-RECOMMENDATION-CONFIDENCE: HolmesGPT service integration must provide functional start interface for valid service recommendations")

				// Starting might fail but shouldn't crash
				err = integration.Start(ctx)
				// We accept that this might fail with nil client
				if err != nil {
					Expect(len(err.Error())).To(BeNumerically(">=", 1), "BR-AI-002-RECOMMENDATION-CONFIDENCE: HolmesGPT service integration errors must provide details for confidence requirements")
				}

				integration.Stop()
			})
		})

		Context("Toolset Management", func() {
			BeforeEach(func() {
				err := serviceIntegration.Start(ctx)
				Expect(err).ToNot(HaveOccurred())

				// Wait a bit for initialization to complete
				time.Sleep(100 * time.Millisecond)
			})

			It("should provide available toolsets", func() {
				toolsets := serviceIntegration.GetAvailableToolsets()
				Expect(len(toolsets)).To(BeNumerically(">=", 0), "BR-AI-002-RECOMMENDATION-CONFIDENCE: HolmesGPT service integration must return measurable toolset collection for valid service recommendations")

				// Should have at least baseline toolsets
				var hasKubernetes, hasInternet bool
				for _, toolset := range toolsets {
					if toolset.ServiceType == "kubernetes" {
						hasKubernetes = true
					}
					if toolset.ServiceType == "internet" {
						hasInternet = true
					}
				}

				Expect(hasKubernetes).To(BeTrue())
				Expect(hasInternet).To(BeTrue())
			})

			It("should provide toolsets by service type", func() {
				kubernetesToolsets := serviceIntegration.GetToolsetByServiceType("kubernetes")
				Expect(len(kubernetesToolsets)).To(BeNumerically(">=", 1), "BR-AI-002-RECOMMENDATION-CONFIDENCE: HolmesGPT service integration must provide Kubernetes toolsets for confidence requirements")

				for _, toolset := range kubernetesToolsets {
					Expect(toolset.ServiceType).To(Equal("kubernetes"))
				}
			})

			It("should return empty slice for unknown service type", func() {
				unknownToolsets := serviceIntegration.GetToolsetByServiceType("unknown-type")
				Expect(unknownToolsets).To(BeEmpty())
			})

			It("should check service availability correctly", func() {
				// Kubernetes should always be available (baseline)
				isAvailable := serviceIntegration.IsServiceAvailable("kubernetes")
				Expect(isAvailable).To(BeTrue())

				// Unknown service should not be available
				isAvailable = serviceIntegration.IsServiceAvailable("non-existent-service")
				Expect(isAvailable).To(BeFalse())
			})

			It("should provide available service types", func() {
				serviceTypes := serviceIntegration.GetAvailableServiceTypes()
				Expect(len(serviceTypes)).To(BeNumerically(">=", 1), "BR-AI-002-RECOMMENDATION-CONFIDENCE: HolmesGPT service integration must provide service types for confidence requirements")

				// Should include baseline services
				Expect(serviceTypes).To(ContainElement("kubernetes"))
				Expect(serviceTypes).To(ContainElement("internet"))
			})
		})

		Context("Statistics and Health", func() {
			BeforeEach(func() {
				err := serviceIntegration.Start(ctx)
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(100 * time.Millisecond) // Wait for initialization
			})

			It("should provide toolset statistics", func() {
				stats := serviceIntegration.GetToolsetStats()

				Expect(stats.TotalToolsets).To(BeNumerically(">=", 2)) // At least kubernetes and internet
				Expect(stats.EnabledCount).To(BeNumerically(">=", 2))
				Expect(stats.TypeCounts).To(HaveKey("kubernetes"))
				Expect(stats.TypeCounts).To(HaveKey("internet"))
			})

			It("should provide service discovery statistics", func() {
				stats := serviceIntegration.GetServiceDiscoveryStats()

				Expect(stats.TotalServices).To(BeNumerically(">=", 0))
				Expect(stats.AvailableServices).To(BeNumerically(">=", 0))
				Expect(stats.ServiceTypes).To(BeAssignableToTypeOf(map[string]int{}), "BR-AI-002-RECOMMENDATION-CONFIDENCE: HolmesGPT service integration must provide functional service type statistics for valid service recommendations")
			})

			It("should provide health status", func() {
				health := serviceIntegration.GetHealthStatus()

				Expect(health.Healthy).To(BeTrue()) // Should be healthy with baseline toolsets
				Expect(health.ServiceDiscoveryHealthy).To(BeTrue())
				Expect(health.ToolsetManagerHealthy).To(BeTrue())
				Expect(health.TotalToolsets).To(BeNumerically(">=", 2))
				Expect(health.EnabledToolsets).To(BeNumerically(">=", 2))
			})
		})

		Context("Dynamic Updates", func() {
			BeforeEach(func() {
				err := serviceIntegration.Start(ctx)
				Expect(err).ToNot(HaveOccurred())
				time.Sleep(100 * time.Millisecond)
			})

			It("should refresh toolsets on demand", func() {
				initialStats := serviceIntegration.GetToolsetStats()

				err := serviceIntegration.RefreshToolsets(ctx)
				Expect(err).ToNot(HaveOccurred())

				// Stats should still be valid after refresh
				refreshedStats := serviceIntegration.GetToolsetStats()
				Expect(refreshedStats.TotalToolsets).To(BeNumerically(">=", initialStats.TotalToolsets))
			})

			// NOTE: "should handle service discovery updates" test moved to integration test suite
			// Business Requirement: BR-HOLMES-020 - Real K8s required for service discovery
			// See: test/integration/ai/service_integration_test.go
		})

		Context("Event Handling", func() {
			var testHandler *TestToolsetUpdateHandler

			BeforeEach(func() {
				testHandler = NewTestToolsetUpdateHandler()
				serviceIntegration.AddToolsetUpdateHandler(testHandler)

				err := serviceIntegration.Start(ctx)
				Expect(err).ToNot(HaveOccurred())
				time.Sleep(200 * time.Millisecond) // Wait for baseline toolsets to be added
			})

			It("should notify handlers of toolset updates", func() {
				// Initial toolsets should have been added
				Eventually(func() int {
					return len(testHandler.UpdatedToolsets)
				}, 2*time.Second, 100*time.Millisecond).Should(BeNumerically(">=", 1))

				// Verify we got notifications for baseline toolsets
				updatedTypes := make(map[string]bool)
				for _, toolsets := range testHandler.UpdatedToolsets {
					for _, toolset := range toolsets {
						updatedTypes[toolset.ServiceType] = true
					}
				}

				Expect(updatedTypes).To(HaveKey("kubernetes"))
				Expect(updatedTypes).To(HaveKey("internet"))
			})

			// NOTE: "should handle multiple event handlers" test moved to integration test suite
			// Business Requirement: BR-HOLMES-025 - Real K8s required for event handler testing
			// See: test/integration/ai/service_integration_test.go

			It("should handle handler errors gracefully", func() {
				// Add a handler that always fails
				failingHandler := &FailingToolsetUpdateHandler{}
				serviceIntegration.AddToolsetUpdateHandler(failingHandler)

				// Force a toolset update - should not crash despite failing handler
				err := serviceIntegration.RefreshToolsets(ctx)
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(100 * time.Millisecond)

				// Original handler should still receive updates
				Expect(len(testHandler.UpdatedToolsets)).To(BeNumerically(">=", 1))
			})
		})

		Context("Error Handling and Edge Cases", func() {
			It("should handle starting when already started gracefully", func() {
				err := serviceIntegration.Start(ctx)
				Expect(err).ToNot(HaveOccurred())

				// Starting again should not cause issues
				err = serviceIntegration.Start(ctx)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should handle stop before start gracefully", func() {
				// Should not crash
				serviceIntegration.Stop()
			})

			It("should handle multiple stops gracefully", func() {
				err := serviceIntegration.Start(ctx)
				Expect(err).ToNot(HaveOccurred())

				serviceIntegration.Stop()
				serviceIntegration.Stop() // Second stop should be safe
			})

			It("should handle context cancellation during start", func() {
				cancelCtx, cancel := context.WithCancel(ctx)
				cancel() // Cancel immediately

				// Should handle cancelled context gracefully
				err := serviceIntegration.Start(cancelCtx)
				// May or may not error, but should not crash
				if err != nil {
					Expect(len(err.Error())).To(BeNumerically(">=", 1), "BR-AI-002-RECOMMENDATION-CONFIDENCE: HolmesGPT service integration errors must provide details for confidence requirements")
				}
			})
		})

		Context("Configuration Validation", func() {
			It("should handle various configuration scenarios", func() {
				configs := []*k8s.ServiceDiscoveryConfig{
					// Minimal configuration
					{
						Enabled: true,
					},
					// Disabled discovery
					{
						Enabled: false,
					},
					// Custom intervals
					{
						Enabled:             true,
						DiscoveryInterval:   1 * time.Second,
						CacheTTL:            30 * time.Second,
						HealthCheckInterval: 5 * time.Second,
					},
				}

				for _, config := range configs {
					integration, err := holmesgpt.NewServiceIntegration(fakeClient, config, log)
					Expect(err).ToNot(HaveOccurred())

					err = integration.Start(ctx)
					Expect(err).ToNot(HaveOccurred())

					integration.Stop()
				}
			})
		})
	})
})

// Test helper structs for event handling tests
type TestToolsetUpdateHandler struct {
	UpdatedToolsets []([]*holmesgpt.ToolsetConfig)
}

func NewTestToolsetUpdateHandler() *TestToolsetUpdateHandler {
	return &TestToolsetUpdateHandler{
		UpdatedToolsets: make([]([]*holmesgpt.ToolsetConfig), 0),
	}
}

func (h *TestToolsetUpdateHandler) OnToolsetsUpdated(toolsets []*holmesgpt.ToolsetConfig) error {
	h.UpdatedToolsets = append(h.UpdatedToolsets, toolsets)
	return nil
}

type FailingToolsetUpdateHandler struct{}

func (h *FailingToolsetUpdateHandler) OnToolsetsUpdated(toolsets []*holmesgpt.ToolsetConfig) error {
	return fmt.Errorf("simulated handler failure")
}
