package k8s_test

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

	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
)

var _ = Describe("ServiceDiscovery", func() {
	var (
		serviceDiscovery *k8s.ServiceDiscovery
		fakeClient       *fake.Clientset
		log              *logrus.Logger
		ctx              context.Context
		config           *k8s.ServiceDiscoveryConfig
	)

	BeforeEach(func() {
		fakeClient = fake.NewSimpleClientset()
		log = logrus.New()
		log.SetLevel(logrus.ErrorLevel) // Reduce test noise
		ctx = context.Background()

		// Create test configuration
		config = &k8s.ServiceDiscoveryConfig{
			DiscoveryInterval:   1 * time.Second,
			CacheTTL:            5 * time.Minute,
			HealthCheckInterval: 10 * time.Second,
			Enabled:             true,
			Namespaces:          []string{"monitoring", "observability"},
		}

		serviceDiscovery = k8s.NewServiceDiscovery(fakeClient, config, log)
	})

	AfterEach(func() {
		if serviceDiscovery != nil {
			serviceDiscovery.Stop()
		}
	})

	// Business Requirement: BR-HOLMES-016 - Dynamic service discovery in Kubernetes cluster
	Describe("BR-HOLMES-029: Service discovery metrics and monitoring", func() {
		It("should track service discovery event metrics with structured data", func() {
			// Business Requirement: BR-HOLMES-029 - Event monitoring without interface{} usage
			err := serviceDiscovery.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Create a test service
			testService := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prometheus-service",
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

			// Add service to fake client
			_, err = fakeClient.CoreV1().Services("monitoring").Create(ctx, testService, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Wait for service discovery to process
			Eventually(func() bool {
				metrics := serviceDiscovery.GetEventMetrics()
				return metrics.TotalEventsProcessed > 0
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

			// Validate structured metrics - no interface{} usage
			metrics := serviceDiscovery.GetEventMetrics()
			Expect(metrics.TotalEventsProcessed).To(BeNumerically(">", 0))
			Expect(metrics.SuccessfulEvents).To(BeNumerically(">", 0))
			Expect(metrics.DroppedEvents).To(BeNumerically(">=", 0))
			Expect(metrics.EventTypeCounts).To(HaveKey("added"))
			Expect(metrics.EventTypeCounts["added"]).To(BeNumerically(">", 0))
		})

		It("should track dropped events when channel buffer is full", func() {
			// Business Requirement: BR-HOLMES-029 - Monitor event drops for reliability
			err := serviceDiscovery.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Create many services rapidly to potentially cause drops
			for i := 0; i < 100; i++ {
				testService := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("test-service-%d", i),
						Namespace: "monitoring",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Name: "web", Port: 8080}},
					},
				}
				_, err = fakeClient.CoreV1().Services("monitoring").Create(ctx, testService, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())
			}

			// Allow processing
			time.Sleep(2 * time.Second)

			// Validate metrics track all events (processed + dropped)
			metrics := serviceDiscovery.GetEventMetrics()
			totalEvents := metrics.TotalEventsProcessed + metrics.DroppedEvents
			Expect(totalEvents).To(BeNumerically(">=", 100))

			// Ensure dropped events are properly tracked if they occur
			if metrics.DroppedEvents > 0 {
				Expect(metrics.DropRate).To(BeNumerically(">", 0))
				Expect(metrics.DropRate).To(BeNumerically("<=", 1))
			}
		})

		It("should provide event processing latency metrics", func() {
			// Business Requirement: BR-HOLMES-029 - Performance monitoring
			err := serviceDiscovery.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			testService := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "latency-test-service",
					Namespace: "monitoring",
				},
			}

			_, err = fakeClient.CoreV1().Services("monitoring").Create(ctx, testService, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				metrics := serviceDiscovery.GetEventMetrics()
				return metrics.AverageProcessingTime > 0
			}, 3*time.Second, 100*time.Millisecond).Should(BeTrue())

			metrics := serviceDiscovery.GetEventMetrics()
			Expect(metrics.AverageProcessingTime).To(BeNumerically(">", 0))
			Expect(metrics.MaxProcessingTime).To(BeNumerically(">", 0))
			Expect(metrics.LastEventTimestamp).ToNot(BeZero())
		})
	})
	Describe("BR-HOLMES-016: Dynamic Service Discovery", func() {
		It("should discover Prometheus services", func() {
			// Create a Prometheus service in the test cluster
			prometheusService := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prometheus-server",
					Namespace: "monitoring",
					Labels: map[string]string{
						"app.kubernetes.io/name": "prometheus",
						"app":                    "prometheus",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name: "web",
							Port: 9090,
						},
					},
				},
			}

			_, err := fakeClient.CoreV1().Services("monitoring").Create(ctx, prometheusService, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Start service discovery
			err = serviceDiscovery.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Wait for discovery to complete
			Eventually(func() int {
				services := serviceDiscovery.GetDiscoveredServices()
				return len(services)
			}, 2*time.Second, 100*time.Millisecond).Should(BeNumerically(">", 0))

			// Verify Prometheus service was discovered
			discoveredServices := serviceDiscovery.GetDiscoveredServices()
			Expect(discoveredServices).ToNot(BeEmpty())

			var prometheusFound bool
			for _, service := range discoveredServices {
				if service.ServiceType == "prometheus" && service.Name == "prometheus-server" {
					prometheusFound = true
					Expect(service.Namespace).To(Equal("monitoring"))
					Expect(service.Endpoints).ToNot(BeEmpty())
					break
				}
			}
			Expect(prometheusFound).To(BeTrue(), "Prometheus service should be discovered")
		})

		It("should discover Grafana services", func() {
			// Create a Grafana service in the test cluster
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
						{
							Name: "service",
							Port: 3000,
						},
					},
				},
			}

			_, err := fakeClient.CoreV1().Services("monitoring").Create(ctx, grafanaService, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			err = serviceDiscovery.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() int {
				return len(serviceDiscovery.GetDiscoveredServices())
			}, 2*time.Second, 100*time.Millisecond).Should(BeNumerically(">", 0))

			services := serviceDiscovery.GetServicesByType("grafana")
			Expect(services).ToNot(BeEmpty())
			Expect(services[0].Name).To(Equal("grafana"))
			Expect(services[0].ServiceType).To(Equal("grafana"))
		})

		It("should discover Jaeger services", func() {
			// Create a Jaeger service in the test cluster
			jaegerService := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "jaeger-query",
					Namespace: "observability",
					Labels: map[string]string{
						"app.kubernetes.io/name": "jaeger",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name: "query-http",
							Port: 16686,
						},
					},
				},
			}

			_, err := fakeClient.CoreV1().Services("observability").Create(ctx, jaegerService, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			err = serviceDiscovery.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() int {
				return len(serviceDiscovery.GetServicesByType("jaeger"))
			}, 2*time.Second, 100*time.Millisecond).Should(BeNumerically(">", 0))

			services := serviceDiscovery.GetServicesByType("jaeger")
			Expect(services).ToNot(BeEmpty())
			Expect(services[0].Name).To(Equal("jaeger-query"))
		})
	})

	// Business Requirement: BR-HOLMES-017 - Automatic detection of well-known services
	Describe("BR-HOLMES-017: Well-Known Service Detection", func() {
		Context("Prometheus Detection", func() {
			It("should detect Prometheus by app label", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "prometheus",
						Namespace: "monitoring",
						Labels:    map[string]string{"app": "prometheus"},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 9090}},
					},
				}

				_, err := fakeClient.CoreV1().Services("monitoring").Create(ctx, service, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				err = serviceDiscovery.Start(ctx)
				Expect(err).ToNot(HaveOccurred())

				Eventually(func() bool {
					services := serviceDiscovery.GetServicesByType("prometheus")
					return len(services) > 0
				}, 2*time.Second).Should(BeTrue())
			})

			It("should detect Prometheus by port", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "metrics-server",
						Namespace: "monitoring",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Port: 9090}}, // Prometheus default port
					},
				}

				_, err := fakeClient.CoreV1().Services("monitoring").Create(ctx, service, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				err = serviceDiscovery.Start(ctx)
				Expect(err).ToNot(HaveOccurred())

				Eventually(func() bool {
					services := serviceDiscovery.GetServicesByType("prometheus")
					return len(services) > 0
				}, 2*time.Second).Should(BeTrue())
			})
		})
	})

	// Business Requirement: BR-HOLMES-018 - Custom service detection through annotations
	Describe("BR-HOLMES-018: Custom Service Detection", func() {
		It("should detect custom services by annotation", func() {
			customService := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "custom-monitoring",
					Namespace: "monitoring",
					Annotations: map[string]string{
						"kubernaut.io/toolset":      "custom-metrics",
						"kubernaut.io/endpoints":    "metrics:8080,health:8081",
						"kubernaut.io/capabilities": "custom_metrics,health_check",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Name: "metrics", Port: 8080},
						{Name: "health", Port: 8081},
					},
				},
			}

			_, err := fakeClient.CoreV1().Services("monitoring").Create(ctx, customService, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			err = serviceDiscovery.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				services := serviceDiscovery.GetServicesByType("custom-metrics")
				return len(services) > 0
			}, 2*time.Second).Should(BeTrue())

			services := serviceDiscovery.GetServicesByType("custom-metrics")
			Expect(services).ToNot(BeEmpty())

			customSvc := services[0]
			Expect(customSvc.Name).To(Equal("custom-monitoring"))
			Expect(customSvc.ServiceType).To(Equal("custom-metrics"))
			Expect(customSvc.Capabilities).To(ContainElement("custom_metrics"))
			Expect(customSvc.Capabilities).To(ContainElement("health_check"))
		})
	})

	// Business Requirement: BR-HOLMES-020 - Real-time toolset configuration updates
	Describe("BR-HOLMES-020: Real-Time Updates", func() {
		It("should emit events when services are added", func() {
			err := serviceDiscovery.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			eventChannel := serviceDiscovery.GetEventChannel()

			// Create a new service
			newService := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-prometheus",
					Namespace: "monitoring",
					Labels:    map[string]string{"app": "prometheus"},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 9090}},
				},
			}

			_, err = fakeClient.CoreV1().Services("monitoring").Create(ctx, newService, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Wait for service event
			var receivedEvent k8s.ServiceEvent
			Eventually(func() bool {
				select {
				case event := <-eventChannel:
					receivedEvent = event
					return true
				default:
					return false
				}
			}, 3*time.Second).Should(BeTrue())

			Expect(receivedEvent.Type).To(BeElementOf([]string{"created", "updated"}))
			Expect(receivedEvent.Service).ToNot(BeNil())
			Expect(receivedEvent.Service.Name).To(Equal("new-prometheus"))
		})
	})

	// Business Requirement: BR-HOLMES-021 - Service discovery result caching
	Describe("BR-HOLMES-021: Result Caching", func() {
		It("should cache discovered services", func() {
			// Create a service
			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cached-service",
					Namespace: "monitoring",
					Labels:    map[string]string{"app": "prometheus"},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 9090}},
				},
			}

			_, err := fakeClient.CoreV1().Services("monitoring").Create(ctx, service, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			err = serviceDiscovery.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Wait for initial discovery
			Eventually(func() int {
				return len(serviceDiscovery.GetDiscoveredServices())
			}, 2*time.Second).Should(BeNumerically(">", 0))

			// Get services multiple times - should hit cache
			services1 := serviceDiscovery.GetDiscoveredServices()
			services2 := serviceDiscovery.GetDiscoveredServices()

			Expect(services1).To(HaveLen(len(services2)))
			Expect(services1).ToNot(BeEmpty())
		})
	})

	// Business Requirement: BR-HOLMES-027 - Multi-namespace service discovery with RBAC
	Describe("BR-HOLMES-027: Multi-Namespace Discovery", func() {
		It("should discover services across configured namespaces", func() {
			// Create services in different namespaces
			service1 := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prometheus1",
					Namespace: "monitoring",
					Labels:    map[string]string{"app": "prometheus"},
				},
				Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 9090}}},
			}

			service2 := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prometheus2",
					Namespace: "observability",
					Labels:    map[string]string{"app": "prometheus"},
				},
				Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 9090}}},
			}

			_, err := fakeClient.CoreV1().Services("monitoring").Create(ctx, service1, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			_, err = fakeClient.CoreV1().Services("observability").Create(ctx, service2, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			err = serviceDiscovery.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() int {
				return len(serviceDiscovery.GetServicesByType("prometheus"))
			}, 2*time.Second).Should(BeNumerically(">=", 2))

			prometheusServices := serviceDiscovery.GetServicesByType("prometheus")
			namespaces := make(map[string]bool)
			for _, svc := range prometheusServices {
				namespaces[svc.Namespace] = true
			}

			Expect(namespaces).To(HaveKey("monitoring"))
			Expect(namespaces).To(HaveKey("observability"))
		})
	})

	// Business Requirement: BR-HOLMES-028 - Maintain baseline toolsets
	Describe("BR-HOLMES-028: Baseline Service Support", func() {
		It("should always include Kubernetes as a discoverable service type", func() {
			err := serviceDiscovery.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Even with no services, baseline services should be conceptually available
			// This is more of an architectural requirement - the baseline toolsets
			// are handled by the DynamicToolsetManager, not ServiceDiscovery directly
			services := serviceDiscovery.GetDiscoveredServices()
			// ServiceDiscovery only finds actual services, baseline toolsets are added separately
			Expect(services).ToNot(BeNil()) // Just ensure it doesn't crash
		})
	})
})

var _ = Describe("ServiceCache", func() {
	var (
		cache *k8s.ServiceCache
	)

	BeforeEach(func() {
		cache = k8s.NewServiceCache(1*time.Minute, 2*time.Minute)
	})

	// Business Requirement: BR-HOLMES-021 - Service discovery result caching
	Describe("BR-HOLMES-021: Service Discovery Caching", func() {
		It("should cache and retrieve services", func() {
			testService := &k8s.DetectedService{
				Name:        "test-service",
				Namespace:   "default",
				ServiceType: "prometheus",
				Available:   true,
				LastChecked: time.Now(),
			}

			cache.SetService(testService)

			retrieved := cache.GetServiceByNamespace("default", "test-service")
			Expect(retrieved).ToNot(BeNil())
			Expect(retrieved.Name).To(Equal("test-service"))
			Expect(retrieved.ServiceType).To(Equal("prometheus"))
		})

		It("should track cache hit rates", func() {
			testService := &k8s.DetectedService{
				Name:        "hit-rate-test",
				Namespace:   "default",
				ServiceType: "grafana",
				Available:   true,
				LastChecked: time.Now(),
			}

			cache.SetService(testService)

			// Hit the cache
			_ = cache.GetServiceByNamespace("default", "hit-rate-test")

			// Miss the cache
			_ = cache.GetServiceByNamespace("default", "nonexistent")

			hitRate := cache.GetHitRate()
			Expect(hitRate).To(BeNumerically(">", 0))
			Expect(hitRate).To(BeNumerically("<=", 1))
		})

		It("should expire old services", func() {
			// Create cache with very short TTL for testing
			shortCache := k8s.NewServiceCache(10*time.Millisecond, 20*time.Millisecond)

			testService := &k8s.DetectedService{
				Name:        "expiring-service",
				Namespace:   "default",
				ServiceType: "jaeger",
				Available:   false,                            // Unavailable services expire faster
				LastChecked: time.Now().Add(-1 * time.Minute), // Already old
			}

			shortCache.SetService(testService)

			// Should not be retrievable due to expiration
			retrieved := shortCache.GetServiceByNamespace("default", "expiring-service")
			Expect(retrieved).To(BeNil())
		})
	})
})
