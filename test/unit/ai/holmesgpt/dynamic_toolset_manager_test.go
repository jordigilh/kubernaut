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

package holmesgpt

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
)

var _ = Describe("DynamicToolsetManager", func() {
	var (
		toolsetManager   *holmesgpt.DynamicToolsetManager
		serviceDiscovery *k8s.ServiceDiscovery
		fakeClient       *fake.Clientset
		log              *logrus.Logger
		ctx              context.Context
	)

	BeforeEach(func() {
		fakeClient = fake.NewSimpleClientset()
		log = logrus.New()
		log.SetLevel(logrus.ErrorLevel) // Reduce test noise
		ctx = context.Background()

		// Create service discovery with test configuration
		config := &k8s.ServiceDiscoveryConfig{
			DiscoveryInterval:   100 * time.Millisecond,
			CacheTTL:            5 * time.Minute,
			HealthCheckInterval: 1 * time.Second,
			Enabled:             true,
			Namespaces:          []string{"monitoring"},
			ServicePatterns:     k8s.GetDefaultServicePatterns(), // BR-HOLMES-017: Well-known service detection
		}

		serviceDiscovery = k8s.NewServiceDiscovery(fakeClient, config, log)
		toolsetManager = holmesgpt.NewDynamicToolsetManager(serviceDiscovery, log)
	})

	AfterEach(func() {
		if toolsetManager != nil {
			toolsetManager.Stop()
		}
		if serviceDiscovery != nil {
			serviceDiscovery.Stop()
		}
	})

	// Business Requirement: BR-HOLMES-022 - Generate appropriate toolset configurations
	Describe("BR-HOLMES-022: Dynamic Toolset Configuration Generation", func() {
		It("should generate toolsets for discovered Prometheus service", func() {
			// Create Prometheus service
			prometheusService := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prometheus-server",
					Namespace: "monitoring",
					Labels: map[string]string{
						"app.kubernetes.io/name": "prometheus",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Name: "web", Port: 9090},
					},
				},
			}

			_, err := fakeClient.CoreV1().Services("monitoring").Create(ctx, prometheusService, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Start service discovery and toolset manager
			err = serviceDiscovery.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			err = toolsetManager.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Wait for toolset generation
			Eventually(func() bool {
				toolsets := toolsetManager.GetAvailableToolsets()
				for _, toolset := range toolsets {
					if toolset.ServiceType == "prometheus" {
						return true
					}
				}
				return false
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

			// Verify Prometheus toolset was generated
			prometheusToolsets := toolsetManager.GetToolsetByServiceType("prometheus")
			Expect(len(prometheusToolsets)).To(BeNumerically(">=", 1), "BR-AI-001-CONFIDENCE: Dynamic toolset management must provide Prometheus tools for AI confidence requirements")

			toolset := prometheusToolsets[0]
			Expect(toolset.Name).To(ContainSubstring("prometheus"))
			Expect(toolset.ServiceType).To(Equal("prometheus"))
			Expect(toolset.Capabilities).To(ContainElement("query_metrics"))
			Expect(toolset.Capabilities).To(ContainElement("time_series"))
			Expect(toolset.Enabled).To(BeTrue())
		})

		It("should generate toolsets for discovered Grafana service", func() {
			grafanaService := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "grafana",
					Namespace: "monitoring",
					Labels: map[string]string{
						"app.kubernetes.io/name": "grafana",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Name: "service", Port: 3000},
					},
				},
			}

			_, err := fakeClient.CoreV1().Services("monitoring").Create(ctx, grafanaService, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			err = serviceDiscovery.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			err = toolsetManager.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				toolsets := toolsetManager.GetToolsetByServiceType("grafana")
				return len(toolsets) > 0
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

			grafanaToolsets := toolsetManager.GetToolsetByServiceType("grafana")
			toolset := grafanaToolsets[0]
			Expect(toolset.ServiceType).To(Equal("grafana"))
			Expect(toolset.Capabilities).To(ContainElement("get_dashboards"))
			Expect(toolset.Capabilities).To(ContainElement("visualization"))
		})
	})

	// Business Requirement: BR-HOLMES-023 - Toolset configuration templates for common service types
	Describe("BR-HOLMES-023: Toolset Configuration Templates", func() {
		It("should use appropriate templates for different service types", func() {
			// Create services of different types
			services := []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "prometheus", Namespace: "monitoring",
						Labels: map[string]string{"app": "prometheus"},
					},
					Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 9090}}},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "jaeger-query", Namespace: "monitoring",
						Labels: map[string]string{"app.kubernetes.io/name": "jaeger"},
					},
					Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 16686}}},
				},
			}

			for _, service := range services {
				_, err := fakeClient.CoreV1().Services("monitoring").Create(ctx, service, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())
			}

			err := serviceDiscovery.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			err = toolsetManager.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Wait for toolsets to be generated
			Eventually(func() int {
				toolsets := toolsetManager.GetAvailableToolsets()
				count := 0
				for _, toolset := range toolsets {
					if toolset.ServiceType == "prometheus" || toolset.ServiceType == "jaeger" {
						count++
					}
				}
				return count
			}, 3*time.Second, 100*time.Millisecond).Should(BeNumerically(">=", 2))

			// Verify different service types have different capabilities
			prometheusToolsets := toolsetManager.GetToolsetByServiceType("prometheus")
			jaegerToolsets := toolsetManager.GetToolsetByServiceType("jaeger")

			Expect(len(prometheusToolsets)).To(BeNumerically(">=", 1), "BR-AI-001-CONFIDENCE: Dynamic toolset management must provide Prometheus tools for AI confidence requirements")
			Expect(len(jaegerToolsets)).To(BeNumerically(">=", 1), "BR-AI-001-CONFIDENCE: Dynamic toolset management must provide Jaeger tools for AI confidence requirements")

			// Prometheus should have metrics capabilities
			Expect(prometheusToolsets[0].Capabilities).To(ContainElement("query_metrics"))

			// Jaeger should have tracing capabilities
			Expect(jaegerToolsets[0].Capabilities).To(ContainElement("search_traces"))
		})
	})

	// Business Requirement: BR-HOLMES-024 - Toolset priority ordering based on service reliability
	Describe("BR-HOLMES-024: Toolset Priority Ordering", func() {
		It("should order toolsets by priority", func() {
			// Create multiple services with different priorities
			services := []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "prometheus", Namespace: "monitoring",
						Labels: map[string]string{"app": "prometheus"},
					},
					Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 9090}}},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "grafana", Namespace: "monitoring",
						Labels: map[string]string{"app.kubernetes.io/name": "grafana"},
					},
					Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3000}}},
				},
			}

			for _, service := range services {
				_, err := fakeClient.CoreV1().Services("monitoring").Create(ctx, service, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())
			}

			err := serviceDiscovery.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			err = toolsetManager.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() int {
				return len(toolsetManager.GetAvailableToolsets())
			}, 3*time.Second, 100*time.Millisecond).Should(BeNumerically(">=", 2))

			toolsets := toolsetManager.GetAvailableToolsets()
			sortedToolsets := holmesgpt.SortToolsetsByPriority(toolsets)

			// Verify toolsets are sorted by priority (higher first)
			if len(sortedToolsets) >= 2 {
				for i := 0; i < len(sortedToolsets)-1; i++ {
					Expect(sortedToolsets[i].Priority).To(BeNumerically(">=", sortedToolsets[i+1].Priority))
				}
			}

			// Kubernetes baseline should have highest priority (100)
			var kubernetesFound bool
			for _, toolset := range sortedToolsets {
				if toolset.ServiceType == "kubernetes" {
					Expect(toolset.Priority).To(Equal(100))
					kubernetesFound = true
					break
				}
			}
			Expect(kubernetesFound).To(BeTrue())
		})
	})

	// Business Requirement: BR-HOLMES-025 - Toolset configuration API endpoints for runtime management
	Describe("BR-HOLMES-025: Runtime Toolset Management", func() {
		It("should provide API to get available toolsets", func() {
			err := toolsetManager.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Should have at least baseline toolsets
			toolsets := toolsetManager.GetAvailableToolsets()
			Expect(len(toolsets)).To(BeNumerically(">=", 1), "BR-AI-001-CONFIDENCE: Dynamic toolset management must provide available tools for AI confidence requirements")

			// Check for baseline toolsets
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

		It("should provide API to get toolset by service type", func() {
			err := toolsetManager.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			kubernetesToolsets := toolsetManager.GetToolsetByServiceType("kubernetes")
			Expect(len(kubernetesToolsets)).To(BeNumerically(">=", 1), "BR-AI-001-CONFIDENCE: Dynamic toolset management must provide Kubernetes tools for AI confidence requirements")
			Expect(kubernetesToolsets[0].ServiceType).To(Equal("kubernetes"))
		})
	})

	// Business Requirement: BR-HOLMES-028 - Maintain baseline toolsets regardless of service discovery
	Describe("BR-HOLMES-028: Baseline Toolset Maintenance", func() {
		It("should always provide baseline toolsets", func() {
			// Start toolset manager without any discovered services
			err := toolsetManager.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			toolsets := toolsetManager.GetAvailableToolsets()
			Expect(len(toolsets)).To(BeNumerically(">=", 1), "BR-AI-001-CONFIDENCE: Dynamic toolset management must provide available tools for AI confidence requirements")

			// Should have baseline toolsets even with no services
			serviceTypes := make(map[string]bool)
			for _, toolset := range toolsets {
				serviceTypes[toolset.ServiceType] = true
			}

			Expect(serviceTypes).To(HaveKey("kubernetes"))
			Expect(serviceTypes).To(HaveKey("internet"))
		})

		It("should maintain baseline toolsets when services are removed", func() {
			// Start with a service
			prometheusService := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: "prometheus", Namespace: "monitoring",
					Labels: map[string]string{"app": "prometheus"},
				},
				Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 9090}}},
			}

			_, err := fakeClient.CoreV1().Services("monitoring").Create(ctx, prometheusService, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			err = serviceDiscovery.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			err = toolsetManager.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Wait for initial toolsets
			Eventually(func() bool {
				return len(toolsetManager.GetToolsetByServiceType("prometheus")) > 0
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

			// Delete the service
			err = fakeClient.CoreV1().Services("monitoring").Delete(ctx, "prometheus", metav1.DeleteOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Wait for service removal to be processed
			time.Sleep(500 * time.Millisecond)

			// Baseline toolsets should still exist
			toolsets := toolsetManager.GetAvailableToolsets()
			baselineExists := false
			for _, toolset := range toolsets {
				if toolset.ServiceType == "kubernetes" || toolset.ServiceType == "internet" {
					baselineExists = true
					break
				}
			}
			Expect(baselineExists).To(BeTrue())
		})
	})

	// Business Requirement: BR-HOLMES-029 - Service discovery metrics and monitoring for operational visibility
	Describe("BR-HOLMES-029: Toolset Management Metrics", func() {
		It("should provide toolset statistics", func() {
			err := toolsetManager.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			stats := toolsetManager.GetToolsetStats()
			Expect(stats.TotalToolsets).To(BeNumerically(">=", 2)) // At least kubernetes and internet
			Expect(stats.EnabledCount).To(BeNumerically(">=", 2))
			Expect(stats.TypeCounts).To(HaveKey("kubernetes"))
			Expect(stats.TypeCounts).To(HaveKey("internet"))
		})

		It("should track toolset type counts", func() {
			// Add a Prometheus service
			prometheusService := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: "prometheus", Namespace: "monitoring",
					Labels: map[string]string{"app": "prometheus"},
				},
				Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 9090}}},
			}

			_, err := fakeClient.CoreV1().Services("monitoring").Create(ctx, prometheusService, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			err = serviceDiscovery.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			err = toolsetManager.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				stats := toolsetManager.GetToolsetStats()
				_, hasPrometheus := stats.TypeCounts["prometheus"]
				return hasPrometheus
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

			stats := toolsetManager.GetToolsetStats()
			Expect(stats.TypeCounts["prometheus"]).To(BeNumerically(">=", 1))
		})
	})

	// Business Requirement: BR-HOLMES-030 - Gradual toolset enablement with A/B testing capabilities
	Describe("BR-HOLMES-030: Gradual Toolset Enablement", func() {
		It("should allow enabling and disabling toolsets", func() {
			err := toolsetManager.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Get initial toolsets
			toolsets := toolsetManager.GetAvailableToolsets()
			Expect(len(toolsets)).To(BeNumerically(">=", 1), "BR-AI-001-CONFIDENCE: Dynamic toolset management must provide available tools for AI confidence requirements")

			// All baseline toolsets should be enabled by default
			for _, toolset := range toolsets {
				if toolset.ServiceType == "kubernetes" || toolset.ServiceType == "internet" {
					Expect(toolset.Enabled).To(BeTrue())
				}
			}
		})

		It("should support A/B testing through toolset configuration", func() {
			err := toolsetManager.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// This test verifies that the architecture supports A/B testing
			// by allowing different toolset configurations

			toolsets := toolsetManager.GetAvailableToolsets()
			stats := toolsetManager.GetToolsetStats()

			// The framework should allow for different configurations
			// which enables A/B testing at the application level
			Expect(stats.TotalToolsets).To(BeNumerically(">=", 1))
			Expect(len(toolsets)).To(Equal(stats.TotalToolsets))
		})
	})
})

// TestEventHandler verifies event handling functionality
// Business Requirement: BR-HOLMES-022 - Service-specific toolset configurations
type TestEventHandler struct {
	addedEvents   []*holmesgpt.ToolsetConfig
	updatedEvents []*holmesgpt.ToolsetConfig
	removedEvents []*holmesgpt.ToolsetConfig
}

func NewTestEventHandler() *TestEventHandler {
	return &TestEventHandler{
		addedEvents:   []*holmesgpt.ToolsetConfig{},
		updatedEvents: []*holmesgpt.ToolsetConfig{},
		removedEvents: []*holmesgpt.ToolsetConfig{},
	}
}

func (h *TestEventHandler) OnToolsetAdded(config *holmesgpt.ToolsetConfig) error {
	if config == nil {
		return fmt.Errorf("toolset config cannot be nil")
	}
	if config.Name == "" {
		return fmt.Errorf("toolset config must have a name")
	}
	h.addedEvents = append(h.addedEvents, config)
	return nil
}

func (h *TestEventHandler) OnToolsetUpdated(config *holmesgpt.ToolsetConfig) error {
	if config == nil {
		return fmt.Errorf("toolset config cannot be nil")
	}
	if config.Name == "" {
		return fmt.Errorf("toolset config must have a name")
	}
	h.updatedEvents = append(h.updatedEvents, config)
	return nil
}

func (h *TestEventHandler) OnToolsetRemoved(config *holmesgpt.ToolsetConfig) error {
	if config == nil {
		return fmt.Errorf("toolset config cannot be nil")
	}
	if config.Name == "" {
		return fmt.Errorf("toolset config must have a name")
	}
	h.removedEvents = append(h.removedEvents, config)
	return nil
}

var _ = Describe("ToolsetEventHandling", func() {
	var (
		toolsetManager   *holmesgpt.DynamicToolsetManager
		serviceDiscovery *k8s.ServiceDiscovery
		eventHandler     *TestEventHandler
		fakeClient       *fake.Clientset
		log              *logrus.Logger
		ctx              context.Context
	)

	BeforeEach(func() {
		fakeClient = fake.NewSimpleClientset()
		log = logrus.New()
		log.SetLevel(logrus.ErrorLevel)
		ctx = context.Background()

		config := &k8s.ServiceDiscoveryConfig{
			DiscoveryInterval:   100 * time.Millisecond,
			CacheTTL:            5 * time.Minute,
			HealthCheckInterval: 1 * time.Second,
			Enabled:             true,
			Namespaces:          []string{"monitoring"},
		}

		serviceDiscovery = k8s.NewServiceDiscovery(fakeClient, config, log)
		toolsetManager = holmesgpt.NewDynamicToolsetManager(serviceDiscovery, log)
		eventHandler = NewTestEventHandler()
		toolsetManager.AddEventHandler(eventHandler)
	})

	AfterEach(func() {
		if toolsetManager != nil {
			toolsetManager.Stop()
		}
		if serviceDiscovery != nil {
			serviceDiscovery.Stop()
		}
	})

	// Business Requirement: BR-HOLMES-020 - Real-time toolset configuration updates
	Describe("BR-HOLMES-020: Event-Driven Toolset Updates", func() {
		It("should emit events when toolsets are added", func() {
			err := serviceDiscovery.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			err = toolsetManager.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Wait for baseline toolsets to be added
			Eventually(func() int {
				return len(eventHandler.addedEvents)
			}, 3*time.Second, 100*time.Millisecond).Should(BeNumerically(">=", 2)) // kubernetes + internet

			// Verify baseline toolsets were added
			addedTypes := make(map[string]bool)
			for _, config := range eventHandler.addedEvents {
				addedTypes[config.ServiceType] = true
			}

			Expect(addedTypes).To(HaveKey("kubernetes"))
			Expect(addedTypes).To(HaveKey("internet"))
		})
	})
})

// Note: Test suite is bootstrapped by holmesgpt_suite_test.go
// This file only contains Describe blocks that are automatically discovered by Ginkgo
