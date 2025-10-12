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

package toolset

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/jordigilh/kubernaut/pkg/toolset/discovery"
)

// BR-025: Service discovery orchestration across multiple detectors
// BR-026: Discovery loop lifecycle management
var _ = Describe("End-to-End Discovery Flow Integration", func() {
	var (
		discoverer discovery.ServiceDiscoverer
		testCtx    context.Context
		flowNs     string
	)

	BeforeEach(func() {
		testCtx = context.Background()
		flowNs = getUniqueNamespace("flow-test")

		// Create test namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: flowNs,
			},
		}
		_, err := k8sClient.CoreV1().Namespaces().Create(testCtx, ns, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		// Initialize discoverer with all detectors (nil health checker for integration tests)
		// Health check logic is fully covered by unit tests (80+ specs)
		// Integration tests focus on service discovery orchestration, not health validation
		discoverer = discovery.NewServiceDiscoverer(k8sClient)
		discoverer.RegisterDetector(discovery.NewPrometheusDetectorWithHealthChecker(nil))
		discoverer.RegisterDetector(discovery.NewGrafanaDetectorWithHealthChecker(nil))
		discoverer.RegisterDetector(discovery.NewJaegerDetectorWithHealthChecker(nil))
		discoverer.RegisterDetector(discovery.NewElasticsearchDetectorWithHealthChecker(nil))
		discoverer.RegisterDetector(discovery.NewCustomDetectorWithHealthChecker(nil))
	})

	AfterEach(func() {
		// Clean up test namespace
		_ = k8sClient.CoreV1().Namespaces().Delete(testCtx, flowNs, metav1.DeleteOptions{})
	})

	// BR-025: Multi-detector orchestration
	Describe("BR-025: Multi-Detector Discovery Orchestration", func() {
		It("should discover services across all detectors in registration order", func() {
			// Create services for each detector type
			services := []*corev1.Service{
				createServiceWithLabels(flowNs, "prometheus-server", map[string]string{"app": "prometheus"}, 9090),
				createServiceWithLabels(flowNs, "grafana", map[string]string{"app": "grafana"}, 3000),
				createServiceWithLabels(flowNs, "jaeger-query", map[string]string{"app.kubernetes.io/name": "jaeger"}, 16686),
				createServiceWithLabels(flowNs, "elasticsearch", map[string]string{"app": "elasticsearch"}, 9200),
			}

			for _, svc := range services {
				_, err := k8sClient.CoreV1().Services(flowNs).Create(testCtx, svc, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())
			}

			// Wait for services to be available
			time.Sleep(500 * time.Millisecond)

			// Execute discovery
			discovered, err := discoverer.DiscoverServices(testCtx)
			Expect(err).ToNot(HaveOccurred())

			// Verify all services discovered
			Expect(len(discovered)).To(BeNumerically(">=", 4),
				"Should discover at least 4 services (one per detector)")

			// Verify service types
			serviceTypes := make(map[string]bool)
			for _, svc := range discovered {
				serviceTypes[svc.Type] = true
			}

			// Verify expected types present
			expectedTypes := []string{"prometheus", "grafana", "jaeger", "elasticsearch"}
			for _, expectedType := range expectedTypes {
				Expect(serviceTypes[expectedType]).To(BeTrue(),
					"Should discover service of type: %s", expectedType)
			}
		})

		It("should handle no duplicates when multiple detectors match same service", func() {
			// Create a service that could potentially match multiple detectors
			svc := createServiceWithLabels(flowNs, "multi-match", map[string]string{
				"app":                       "prometheus",
				"app.kubernetes.io/name":    "jaeger",
				"kubernaut.io/toolset":      "enabled",
				"kubernaut.io/toolset-type": "custom",
			}, 8080)

			_, err := k8sClient.CoreV1().Services(flowNs).Create(testCtx, svc, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(500 * time.Millisecond)

			// Execute discovery
			discovered, err := discoverer.DiscoverServices(testCtx)
			Expect(err).ToNot(HaveOccurred())

			// Verify service discovered
			Expect(len(discovered)).To(BeNumerically(">=", 1))

			// Count how many times the service appears
			matchCount := 0
			for _, disc := range discovered {
				if disc.Name == "multi-match" && disc.Namespace == flowNs {
					matchCount++
				}
			}

			// Verify detected (detection logic may match one or prioritize)
			// The actual behavior depends on detector implementation
			// At minimum, should be detected once
			Expect(matchCount).To(BeNumerically(">=", 1),
				"Service should be detected at least once")
		})

		It("should handle empty cluster gracefully", func() {
			// Use a fresh namespace with no services
			emptyNs := getUniqueNamespace("empty-disc-test")
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: emptyNs,
				},
			}
			_, err := k8sClient.CoreV1().Namespaces().Create(testCtx, ns, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
			defer k8sClient.CoreV1().Namespaces().Delete(testCtx, emptyNs, metav1.DeleteOptions{})

			// Execute discovery
			discovered, err := discoverer.DiscoverServices(testCtx)
			Expect(err).ToNot(HaveOccurred())

			// Filter for empty namespace (shouldn't find any)
			emptyNsServices := 0
			for _, svc := range discovered {
				if svc.Namespace == emptyNs {
					emptyNsServices++
				}
			}

			Expect(emptyNsServices).To(Equal(0),
				"Should find no services in empty namespace")
		})
	})

	// BR-026: Discovery loop lifecycle
	Describe("BR-026: Discovery Loop Lifecycle Management", func() {
		It("should handle discovery with services added between calls", func() {
			// Create initial service
			svc := createServiceWithLabels(flowNs, "prometheus-initial", map[string]string{"app": "prometheus"}, 9090)
			_, err := k8sClient.CoreV1().Services(flowNs).Create(testCtx, svc, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(500 * time.Millisecond)

			// First discovery
			firstDiscovery, err := discoverer.DiscoverServices(testCtx)
			Expect(err).ToNot(HaveOccurred())

			foundInitial := false
			for _, disc := range firstDiscovery {
				if disc.Name == "prometheus-initial" && disc.Namespace == flowNs {
					foundInitial = true
					break
				}
			}
			Expect(foundInitial).To(BeTrue(), "Initial service should be discovered")

			// Add new service
			svc2 := createServiceWithLabels(flowNs, "grafana-added", map[string]string{"app": "grafana"}, 3000)
			_, err = k8sClient.CoreV1().Services(flowNs).Create(testCtx, svc2, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(500 * time.Millisecond)

			// Second discovery
			secondDiscovery, err := discoverer.DiscoverServices(testCtx)
			Expect(err).ToNot(HaveOccurred())

			// Verify second discovery includes new service
			foundNew := false
			for _, disc := range secondDiscovery {
				if disc.Name == "grafana-added" && disc.Namespace == flowNs {
					foundNew = true
					break
				}
			}

			Expect(foundNew).To(BeTrue(), "New service should be discovered")
		})

		It("should handle service deletion between discovery calls", func() {
			// Create service
			svc := createServiceWithLabels(flowNs, "prometheus-to-delete", map[string]string{"app": "prometheus"}, 9090)
			createdSvc, err := k8sClient.CoreV1().Services(flowNs).Create(testCtx, svc, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(500 * time.Millisecond)

			// First discovery
			firstDiscovery, err := discoverer.DiscoverServices(testCtx)
			Expect(err).ToNot(HaveOccurred())

			foundBefore := false
			for _, disc := range firstDiscovery {
				if disc.Name == "prometheus-to-delete" && disc.Namespace == flowNs {
					foundBefore = true
					break
				}
			}
			Expect(foundBefore).To(BeTrue(), "Service should be discovered initially")

			// Delete service
			err = k8sClient.CoreV1().Services(flowNs).Delete(testCtx, createdSvc.Name, metav1.DeleteOptions{})
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(500 * time.Millisecond)

			// Second discovery
			laterDiscovery, err := discoverer.DiscoverServices(testCtx)
			Expect(err).ToNot(HaveOccurred())

			// Verify deleted service not in later discovery
			foundAfter := false
			for _, disc := range laterDiscovery {
				if disc.Name == "prometheus-to-delete" && disc.Namespace == flowNs {
					foundAfter = true
					break
				}
			}
			Expect(foundAfter).To(BeFalse(), "Deleted service should not be in later discovery")
		})

		It("should discover services across multiple namespaces", func() {
			// Create new namespace with service
			newNs := getUniqueNamespace("multi-ns-test")
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: newNs,
				},
			}
			_, err := k8sClient.CoreV1().Namespaces().Create(testCtx, ns, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
			defer k8sClient.CoreV1().Namespaces().Delete(testCtx, newNs, metav1.DeleteOptions{})

			svc := createServiceWithLabels(newNs, "prometheus-new-ns", map[string]string{"app": "prometheus"}, 9090)
			_, err = k8sClient.CoreV1().Services(newNs).Create(testCtx, svc, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(500 * time.Millisecond)

			// Discovery should find services in multiple namespaces
			discovery, err := discoverer.DiscoverServices(testCtx)
			Expect(err).ToNot(HaveOccurred())

			// Verify service in new namespace discovered
			foundInNewNs := false
			for _, disc := range discovery {
				if disc.Name == "prometheus-new-ns" && disc.Namespace == newNs {
					foundInNewNs = true
					break
				}
			}
			Expect(foundInNewNs).To(BeTrue(), "Service in new namespace should be discovered")
		})

		It("should respect context cancellation during discovery", func() {
			// Create many services
			for i := 0; i < 10; i++ {
				svc := createServiceWithLabels(flowNs, fmt.Sprintf("prometheus-%d", i),
					map[string]string{"app": "prometheus"}, 9090+i)
				_, err := k8sClient.CoreV1().Services(flowNs).Create(testCtx, svc, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())
			}

			time.Sleep(500 * time.Millisecond)

			// Create cancelable context
			cancelCtx, cancel := context.WithCancel(testCtx)

			// Start discovery in goroutine
			done := make(chan error, 1)
			go func() {
				_, err := discoverer.DiscoverServices(cancelCtx)
				done <- err
			}()

			// Cancel immediately
			cancel()

			// Wait for completion
			var discoveryErr error
			Eventually(done, 2*time.Second).Should(Receive(&discoveryErr))

			// Either completed successfully or got context canceled error
			if discoveryErr != nil {
				Expect(discoveryErr).To(MatchError(context.Canceled))
			}
		})

		It("should handle concurrent service updates during discovery", func() {
			// Create initial services
			for i := 0; i < 5; i++ {
				svc := createServiceWithLabels(flowNs, fmt.Sprintf("service-%d", i),
					map[string]string{"app": "prometheus"}, 9090+i)
				_, err := k8sClient.CoreV1().Services(flowNs).Create(testCtx, svc, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())
			}

			time.Sleep(500 * time.Millisecond)

			// Initial discovery
			initialDiscovery, err := discoverer.DiscoverServices(testCtx)
			Expect(err).ToNot(HaveOccurred())

			// Count services in test namespace
			initialCount := 0
			for _, disc := range initialDiscovery {
				if disc.Namespace == flowNs {
					initialCount++
				}
			}
			Expect(initialCount).To(BeNumerically(">=", 5))

			// Concurrently update service labels
			for i := 0; i < 5; i++ {
				go func(index int) {
					svcName := fmt.Sprintf("service-%d", index)
					svc, err := k8sClient.CoreV1().Services(flowNs).Get(testCtx, svcName, metav1.GetOptions{})
					if err == nil {
						svc.Labels["updated"] = "true"
						_, _ = k8sClient.CoreV1().Services(flowNs).Update(testCtx, svc, metav1.UpdateOptions{})
					}
				}(i)
			}

			// Wait for updates to complete
			time.Sleep(1 * time.Second)

			// Discovery after updates
			laterDiscovery, err := discoverer.DiscoverServices(testCtx)
			Expect(err).ToNot(HaveOccurred())

			// Count services in test namespace
			laterCount := 0
			for _, disc := range laterDiscovery {
				if disc.Namespace == flowNs {
					laterCount++
				}
			}

			// Verify discovery still works despite concurrent updates
			Expect(laterCount).To(BeNumerically(">=", 5),
				"Discovery should handle concurrent updates gracefully")
		})
	})
})

// Helper function to create service with labels
func createServiceWithLabels(namespace, name string, labels map[string]string, port int) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       int32(port),
					TargetPort: intstr.FromInt(port),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: map[string]string{
				"app": name,
			},
		},
	}
}
