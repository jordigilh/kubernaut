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

// Integration Test Coverage Strategy:
//
// This test suite focuses on:
//   - Service discovery (labels, annotations, ports, namespaces)
//   - ConfigMap operations (CRUD, watch, OwnerReferences)
//   - Discovery orchestration (multi-detector coordination)
//
// Health Check Validation:
//   - Covered by unit tests (80+ specs): test/unit/toolset/*_detector_test.go
//   - BRs: BR-TOOLSET-012 (Prometheus), BR-TOOLSET-015 (Grafana),
//          BR-TOOLSET-018 (Jaeger), BR-TOOLSET-021 (Elasticsearch),
//          BR-TOOLSET-024 (Custom health paths)
//   - Integration tests require in-cluster deployment (planned V2)
//
// Server Execution Model:
//   - V1: Server runs in test process (local) - can't test health checks
//   - V2: Server deployed in cluster - will add health check integration tests
//
// See: docs/services/stateless/dynamic-toolset/implementation/HEALTH_CHECK_INTEGRATION_TEST_DECISION.md

package toolset

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/toolset/discovery"
)

var _ = Describe("Service Discovery Integration", func() {
	var (
		discoverer discovery.ServiceDiscoverer
		testCtx    context.Context
	)

	BeforeEach(func() {
		testCtx = context.Background()
		discoverer = discovery.NewServiceDiscoverer(k8sClient)
	})

	Describe("Detector Registration", func() {
		It("should register multiple detectors", func() {
			// Register detectors with nil health checker (integration tests skip health checks)
			discoverer.RegisterDetector(discovery.NewPrometheusDetectorWithHealthChecker(nil))
			discoverer.RegisterDetector(discovery.NewGrafanaDetectorWithHealthChecker(nil))
			discoverer.RegisterDetector(discovery.NewJaegerDetectorWithHealthChecker(nil))
			discoverer.RegisterDetector(discovery.NewElasticsearchDetectorWithHealthChecker(nil))
			discoverer.RegisterDetector(discovery.NewCustomDetectorWithHealthChecker(nil))

			// Discovery should work with all detectors registered
			services, err := discoverer.DiscoverServices(testCtx)
			Expect(err).ToNot(HaveOccurred())
			Expect(services).ToNot(BeEmpty())
		})
	})

	Describe("Service Discovery with Real Kubernetes Services", func() {
		BeforeEach(func() {
			// Create standard test services for discovery
			createStandardTestServices(testCtx, k8sClient)

			// Wait for services to be available in Kubernetes
			time.Sleep(1 * time.Second)

			// Register detectors with nil health checker (integration tests skip health checks)
			// Health check logic is fully covered by unit tests (80+ specs)
			// Integration tests focus on service discovery orchestration, not health validation
			// See: docs/services/stateless/dynamic-toolset/implementation/PROPER_FIX_TEST_TRIAGE.md
			discoverer.RegisterDetector(discovery.NewPrometheusDetectorWithHealthChecker(nil))
			discoverer.RegisterDetector(discovery.NewGrafanaDetectorWithHealthChecker(nil))
			discoverer.RegisterDetector(discovery.NewJaegerDetectorWithHealthChecker(nil))
			discoverer.RegisterDetector(discovery.NewElasticsearchDetectorWithHealthChecker(nil))
			discoverer.RegisterDetector(discovery.NewCustomDetectorWithHealthChecker(nil))
		})

		AfterEach(func() {
			// Cleanup standard test services
			cleanupStandardTestServices(testCtx, k8sClient)
		})

		It("should discover Prometheus service", func() {
			services, err := discoverer.DiscoverServices(testCtx)
			Expect(err).ToNot(HaveOccurred())

			// Filter for services in our test namespaces only
			prometheusFound := false
			for _, svc := range services {
				if svc.Type == "prometheus" && svc.Name == "prometheus-server" && svc.Namespace == "monitoring" {
					prometheusFound = true
					Expect(svc.Endpoint).To(ContainSubstring("prometheus-server"))
					Expect(svc.Endpoint).To(ContainSubstring(":9090"))
					break
				}
			}

			Expect(prometheusFound).To(BeTrue(), "Prometheus service should be discovered in monitoring namespace")
		})

		It("should discover Grafana service", func() {
			services, err := discoverer.DiscoverServices(testCtx)
			Expect(err).ToNot(HaveOccurred())

			// Filter for services in our test namespaces only
			grafanaFound := false
			for _, svc := range services {
				if svc.Type == "grafana" && svc.Name == "grafana" && svc.Namespace == "monitoring" {
					grafanaFound = true
					Expect(svc.Endpoint).To(ContainSubstring("grafana"))
					Expect(svc.Endpoint).To(ContainSubstring(":3000"))
					break
				}
			}

			Expect(grafanaFound).To(BeTrue(), "Grafana service should be discovered in monitoring namespace")
		})

		It("should discover Jaeger service", func() {
			services, err := discoverer.DiscoverServices(testCtx)
			Expect(err).ToNot(HaveOccurred())

			// Filter for services in our test namespaces only
			jaegerFound := false
			for _, svc := range services {
				if svc.Type == "jaeger" && svc.Name == "jaeger-query" && svc.Namespace == "observability" {
					jaegerFound = true
					Expect(svc.Endpoint).To(ContainSubstring("jaeger-query"))
					Expect(svc.Endpoint).To(ContainSubstring(":16686"))
					break
				}
			}

			Expect(jaegerFound).To(BeTrue(), "Jaeger service should be discovered in observability namespace")
		})

		It("should discover Elasticsearch service", func() {
			services, err := discoverer.DiscoverServices(testCtx)
			Expect(err).ToNot(HaveOccurred())

			// Filter for services in our test namespaces only
			elasticFound := false
			for _, svc := range services {
				if svc.Type == "elasticsearch" && svc.Name == "elasticsearch" && svc.Namespace == "observability" {
					elasticFound = true
					Expect(svc.Endpoint).To(ContainSubstring("elasticsearch"))
					Expect(svc.Endpoint).To(ContainSubstring(":9200"))
					break
				}
			}

			Expect(elasticFound).To(BeTrue(), "Elasticsearch service should be discovered in observability namespace")
		})

		It("should discover custom annotated service", func() {
			services, err := discoverer.DiscoverServices(testCtx)
			Expect(err).ToNot(HaveOccurred())

			// Filter for services in our test namespaces only
			customFound := false
			for _, svc := range services {
				if svc.Type == "custom-api" && svc.Name == "custom-toolset-service" && svc.Namespace == "default" {
					customFound = true
					// Accept either DNS format (both are valid)
					Expect(svc.Endpoint).To(Or(
						Equal("http://custom-toolset-service.default.svc:8080"),
						Equal("http://custom-toolset-service.default.svc.cluster.local:8080"),
					))
					// Health path would be in Metadata if needed
					break
				}
			}

			Expect(customFound).To(BeTrue(), "Custom annotated service should be discovered in default namespace")
		})

		It("should discover all test services", func() {
			services, err := discoverer.DiscoverServices(testCtx)
			Expect(err).ToNot(HaveOccurred())

			// Filter for services in our test namespaces only
			testNamespaces := map[string]bool{
				"monitoring":    true,
				"observability": true,
				"default":       true,
			}

			var testServices []string
			types := make(map[string]bool)
			for _, svc := range services {
				if testNamespaces[svc.Namespace] {
					testServices = append(testServices, svc.Name)
					types[svc.Type] = true
				}
			}

			// We created 5 test services in our test namespaces
			Expect(len(testServices)).To(BeNumerically(">=", 5))

			// Verify we have all expected types
			Expect(types["prometheus"]).To(BeTrue())
			Expect(types["grafana"]).To(BeTrue())
			Expect(types["jaeger"]).To(BeTrue())
			Expect(types["elasticsearch"]).To(BeTrue())
			Expect(types["custom-api"]).To(BeTrue())
		})

		It("should discover services with complete metadata", func() {
			services, err := discoverer.DiscoverServices(testCtx)
			Expect(err).ToNot(HaveOccurred())

			// All services should be detected with complete metadata
			// Note: Health validation covered by unit tests (80+ specs)
			// See: test/unit/toolset/*_detector_test.go
			for _, svc := range services {
				Expect(svc.Name).ToNot(BeEmpty())
				Expect(svc.Namespace).ToNot(BeEmpty())
				Expect(svc.Type).ToNot(BeEmpty())
				Expect(svc.Endpoint).ToNot(BeEmpty())
			}
		})
	})

	Describe("Discovery Loop Lifecycle", func() {
		BeforeEach(func() {
			discoverer.RegisterDetector(discovery.NewPrometheusDetectorWithHealthChecker(nil))
		})

		It("should start and stop discovery loop", func() {
			loopCtx, loopCancel := context.WithCancel(context.Background())
			defer loopCancel()

			// Launch discovery loop in background (Start() is blocking)
			done := make(chan error, 1)
			go func() {
				done <- discoverer.Start(loopCtx)
			}()

			// Let it run for a bit
			time.Sleep(1 * time.Second)

			// Stop the loop
			err := discoverer.Stop()
			Expect(err).ToNot(HaveOccurred())

			// Wait for Start() to return
			Eventually(done, 2*time.Second).Should(Receive(BeNil()))

			// Stopping again should be idempotent
			err = discoverer.Stop()
			Expect(err).ToNot(HaveOccurred())
		})

		It("should run periodic discoveries", func() {
			loopCtx, loopCancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer loopCancel()

			// Launch discovery loop in background (Start() is blocking)
			done := make(chan error, 1)
			go func() {
				done <- discoverer.Start(loopCtx)
			}()

			// Wait for initial discovery
			time.Sleep(500 * time.Millisecond)

			// Stop the loop
			err := discoverer.Stop()
			Expect(err).ToNot(HaveOccurred())

			// Wait for Start() to return
			Eventually(done, 2*time.Second).Should(Receive(BeNil()))

			// Discovery should have run at least once
			// We can't easily verify this without internal state exposure,
			// but we can verify it doesn't crash
		})

		It("should respect context cancellation", func() {
			loopCtx, loopCancel := context.WithCancel(context.Background())

			// Launch discovery loop in background (Start() is blocking)
			done := make(chan error, 1)
			go func() {
				done <- discoverer.Start(loopCtx)
			}()

			// Wait a bit for loop to start
			time.Sleep(100 * time.Millisecond)

			// Cancel context
			loopCancel()

			// Wait for Start() to return with context.Canceled error
			Eventually(done, 2*time.Second).Should(Receive(Equal(context.Canceled)))

			// Stop should work even after context cancellation (idempotent)
			err := discoverer.Stop()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("Cross-Namespace Discovery", func() {
		BeforeEach(func() {
			// Create standard test services for discovery
			createStandardTestServices(testCtx, k8sClient)
			time.Sleep(1 * time.Second)

			discoverer.RegisterDetector(discovery.NewPrometheusDetectorWithHealthChecker(nil))
			discoverer.RegisterDetector(discovery.NewGrafanaDetectorWithHealthChecker(nil))
			discoverer.RegisterDetector(discovery.NewJaegerDetectorWithHealthChecker(nil))
			discoverer.RegisterDetector(discovery.NewElasticsearchDetectorWithHealthChecker(nil))
			discoverer.RegisterDetector(discovery.NewCustomDetectorWithHealthChecker(nil))
		})

		AfterEach(func() {
			cleanupStandardTestServices(testCtx, k8sClient)
		})

		It("should discover services across multiple namespaces", func() {
			services, err := discoverer.DiscoverServices(testCtx)
			Expect(err).ToNot(HaveOccurred())

			// Group services by namespace
			namespaces := make(map[string]int)
			for _, svc := range services {
				namespaces[svc.Namespace]++
			}

			// Verify we discovered services in multiple namespaces
			Expect(len(namespaces)).To(BeNumerically(">=", 3))
			Expect(namespaces["monitoring"]).To(BeNumerically(">=", 2))    // Prometheus + Grafana
			Expect(namespaces["observability"]).To(BeNumerically(">=", 2)) // Jaeger + Elasticsearch
			Expect(namespaces["default"]).To(BeNumerically(">=", 1))       // Custom service
		})
	})

	Describe("Detector Priority and Matching", func() {
		BeforeEach(func() {
			// Create standard test services for discovery
			createStandardTestServices(testCtx, k8sClient)
			time.Sleep(1 * time.Second)
		})

		AfterEach(func() {
			cleanupStandardTestServices(testCtx, k8sClient)
		})

		It("should only match each service once", func() {
			discoverer.RegisterDetector(discovery.NewPrometheusDetectorWithHealthChecker(nil))
			discoverer.RegisterDetector(discovery.NewGrafanaDetectorWithHealthChecker(nil))
			discoverer.RegisterDetector(discovery.NewJaegerDetectorWithHealthChecker(nil))
			discoverer.RegisterDetector(discovery.NewElasticsearchDetectorWithHealthChecker(nil))
			discoverer.RegisterDetector(discovery.NewCustomDetectorWithHealthChecker(nil))

			services, err := discoverer.DiscoverServices(testCtx)
			Expect(err).ToNot(HaveOccurred())

			// Check for duplicates (same name + namespace)
			seen := make(map[string]bool)
			for _, svc := range services {
				key := svc.Namespace + "/" + svc.Name
				Expect(seen[key]).To(BeFalse(), "Service %s should only be discovered once", key)
				seen[key] = true
			}
		})
	})
})
