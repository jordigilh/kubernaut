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
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jordigilh/kubernaut/pkg/toolset"
	"github.com/jordigilh/kubernaut/pkg/toolset/discovery"
	"github.com/jordigilh/kubernaut/pkg/toolset/generator"
)

// BR-027: Toolset generation from discovered services
// BR-028: HolmesGPT format validation
var _ = Describe("Generator Integration with Discovery", func() {
	var (
		discoverer discovery.ServiceDiscoverer
		gen        generator.ToolsetGenerator
		testCtx    context.Context
		genNs      string
	)

	BeforeEach(func() {
		testCtx = context.Background()
		genNs = getUniqueNamespace("gen-test")

		// Create test namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: genNs,
			},
		}
		_, err := k8sClient.CoreV1().Namespaces().Create(testCtx, ns, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		// Initialize components (nil health checker for integration tests)
		// Health check logic is fully covered by unit tests (80+ specs)
		// Integration tests focus on service discovery orchestration, not health validation
		discoverer = discovery.NewServiceDiscoverer(k8sClient)
		discoverer.RegisterDetector(discovery.NewPrometheusDetectorWithHealthChecker(nil))
		discoverer.RegisterDetector(discovery.NewGrafanaDetectorWithHealthChecker(nil))
		discoverer.RegisterDetector(discovery.NewJaegerDetectorWithHealthChecker(nil))
		discoverer.RegisterDetector(discovery.NewElasticsearchDetectorWithHealthChecker(nil))
		discoverer.RegisterDetector(discovery.NewCustomDetectorWithHealthChecker(nil))

		gen = generator.NewHolmesGPTGenerator()
	})

	AfterEach(func() {
		_ = k8sClient.CoreV1().Namespaces().Delete(testCtx, genNs, metav1.DeleteOptions{})
	})

	// BR-027/028: End-to-end discovery-to-JSON pipeline
	Describe("BR-027/028: Discovery-to-Generator Integration", func() {
		It("should generate valid HolmesGPT JSON from discovered services", func() {
			// Create test services
			services := []*corev1.Service{
				createServiceWithLabels(genNs, "prometheus-server", map[string]string{"app": "prometheus"}, 9090),
				createServiceWithLabels(genNs, "grafana", map[string]string{"app": "grafana"}, 3000),
			}

			for _, svc := range services {
				_, err := k8sClient.CoreV1().Services(genNs).Create(testCtx, svc, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())
			}

			time.Sleep(500 * time.Millisecond)

			// Execute discovery
			discovered, err := discoverer.DiscoverServices(testCtx)
			Expect(err).ToNot(HaveOccurred())

			// Filter for test namespace
			var nsServices []*toolset.DiscoveredService
			for i := range discovered {
				if discovered[i].Namespace == genNs {
					nsServices = append(nsServices, &discovered[i])
				}
			}

			Expect(len(nsServices)).To(BeNumerically(">=", 2),
				"Should discover at least 2 services")

			// Generate toolset
			toolsetJSON, err := gen.GenerateToolset(testCtx, nsServices)
			Expect(err).ToNot(HaveOccurred())
			Expect(toolsetJSON).ToNot(BeEmpty())

			// Verify JSON structure
			var toolset map[string]interface{}
			err = json.Unmarshal([]byte(toolsetJSON), &toolset)
			Expect(err).ToNot(HaveOccurred())

			// Verify tools array exists
			tools, ok := toolset["tools"].([]interface{})
			Expect(ok).To(BeTrue(), "Should have tools array")
			Expect(len(tools)).To(BeNumerically(">=", 2))

			// Verify service details preserved
			toolMap := make(map[string]interface{})
			for _, tool := range tools {
				t := tool.(map[string]interface{})
				toolType := t["type"].(string)
				toolMap[toolType] = t
			}

			// Verify Prometheus
			if promTool, exists := toolMap["prometheus"]; exists {
				prom := promTool.(map[string]interface{})
				endpoint := prom["endpoint"].(string)
				Expect(endpoint).To(ContainSubstring("prometheus-server"))
				Expect(endpoint).To(ContainSubstring(genNs))
				Expect(endpoint).To(ContainSubstring(":9090"))
			}

			// Verify Grafana
			if grafanaTool, exists := toolMap["grafana"]; exists {
				grafana := grafanaTool.(map[string]interface{})
				endpoint := grafana["endpoint"].(string)
				Expect(endpoint).To(ContainSubstring("grafana"))
				Expect(endpoint).To(ContainSubstring(genNs))
				Expect(endpoint).To(ContainSubstring(":3000"))
			}
		})

		It("should handle generator with empty discovery results", func() {
			// Use empty namespace
			emptyNs := getUniqueNamespace("empty-gen-test")
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

			// Filter for empty namespace
			var emptyNsServices []*toolset.DiscoveredService
			for i := range discovered {
				if discovered[i].Namespace == emptyNs {
					emptyNsServices = append(emptyNsServices, &discovered[i])
				}
			}

			// Generate toolset from empty results
			toolsetJSON, err := gen.GenerateToolset(testCtx, emptyNsServices)
			Expect(err).ToNot(HaveOccurred())

			// Verify valid JSON structure
			var toolset map[string]interface{}
			err = json.Unmarshal([]byte(toolsetJSON), &toolset)
			Expect(err).ToNot(HaveOccurred())

			// Verify empty tools array
			tools, ok := toolset["tools"].([]interface{})
			Expect(ok).To(BeTrue(), "Should have tools array")
			Expect(len(tools)).To(Equal(0), "Tools array should be empty")
		})

		It("should deduplicate services across multiple discoveries", func() {
			// Create service
			svc := createServiceWithLabels(genNs, "prometheus-dedup", map[string]string{"app": "prometheus"}, 9090)
			_, err := k8sClient.CoreV1().Services(genNs).Create(testCtx, svc, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(500 * time.Millisecond)

			// Run discovery twice
			discovered1, err := discoverer.DiscoverServices(testCtx)
			Expect(err).ToNot(HaveOccurred())

			discovered2, err := discoverer.DiscoverServices(testCtx)
			Expect(err).ToNot(HaveOccurred())

			// Combine results (simulating multiple discovery runs)
			var combined []*toolset.DiscoveredService
			for i := range discovered1 {
				if discovered1[i].Namespace == genNs {
					combined = append(combined, &discovered1[i])
				}
			}
			for i := range discovered2 {
				if discovered2[i].Namespace == genNs {
					combined = append(combined, &discovered2[i])
				}
			}

			// Generate toolset
			toolsetJSON, err := gen.GenerateToolset(testCtx, combined)
			Expect(err).ToNot(HaveOccurred())

			// Verify deduplication
			var toolset map[string]interface{}
			err = json.Unmarshal([]byte(toolsetJSON), &toolset)
			Expect(err).ToNot(HaveOccurred())

			tools := toolset["tools"].([]interface{})

			// Count occurrences of prometheus-dedup
			dedupCount := 0
			for _, tool := range tools {
				t := tool.(map[string]interface{})
				if name, ok := t["name"].(string); ok && name == "prometheus-dedup" {
					dedupCount++
				}
			}

			// Generator should deduplicate (ideally 1, but implementation-dependent)
			Expect(dedupCount).To(BeNumerically("<=", len(combined)/2),
				"Should deduplicate repeated services")
		})

		It("should preserve service metadata in generated JSON", func() {
			// Create service with annotations
			svc := createServiceWithLabels(genNs, "annotated-service", map[string]string{
				"app":                       "prometheus",
				"kubernaut.io/custom-label": "test-value",
			}, 9090)
			svc.Annotations = map[string]string{
				"kubernaut.io/toolset":      "enabled",
				"kubernaut.io/toolset-type": "prometheus",
			}

			_, err := k8sClient.CoreV1().Services(genNs).Create(testCtx, svc, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(500 * time.Millisecond)

			// Execute discovery
			discovered, err := discoverer.DiscoverServices(testCtx)
			Expect(err).ToNot(HaveOccurred())

			// Filter for test service
			var testServices []*toolset.DiscoveredService
			for i := range discovered {
				if discovered[i].Name == "annotated-service" && discovered[i].Namespace == genNs {
					testServices = append(testServices, &discovered[i])
				}
			}

			Expect(len(testServices)).To(BeNumerically(">=", 1))

			// Generate toolset
			toolsetJSON, err := gen.GenerateToolset(testCtx, testServices)
			Expect(err).ToNot(HaveOccurred())

			// Verify metadata preserved
			var toolset map[string]interface{}
			err = json.Unmarshal([]byte(toolsetJSON), &toolset)
			Expect(err).ToNot(HaveOccurred())

			tools := toolset["tools"].([]interface{})
			Expect(len(tools)).To(BeNumerically(">=", 1))

			// Verify service name preserved
			tool := tools[0].(map[string]interface{})
			Expect(tool["name"]).To(Equal("annotated-service"))
			Expect(tool["namespace"]).To(Equal(genNs))
		})

		It("should handle generator with mixed service types", func() {
			// Create mix of standard and custom annotated services
			services := []*corev1.Service{
				createServiceWithLabels(genNs, "prometheus-std", map[string]string{"app": "prometheus"}, 9090),
				createServiceWithLabels(genNs, "grafana-std", map[string]string{"app": "grafana"}, 3000),
			}

			// Add custom service with annotations
			customSvc := createServiceWithLabels(genNs, "custom-annotated", map[string]string{}, 8080)
			customSvc.Annotations = map[string]string{
				"kubernaut.io/toolset":      "enabled",
				"kubernaut.io/toolset-type": "custom-api",
			}
			services = append(services, customSvc)

			for _, svc := range services {
				_, err := k8sClient.CoreV1().Services(genNs).Create(testCtx, svc, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())
			}

			time.Sleep(500 * time.Millisecond)

			// Execute discovery
			discovered, err := discoverer.DiscoverServices(testCtx)
			Expect(err).ToNot(HaveOccurred())

			// Filter for test namespace
			var nsServices []*toolset.DiscoveredService
			for i := range discovered {
				if discovered[i].Namespace == genNs {
					nsServices = append(nsServices, &discovered[i])
				}
			}

			Expect(len(nsServices)).To(BeNumerically(">=", 3))

			// Generate toolset
			toolsetJSON, err := gen.GenerateToolset(testCtx, nsServices)
			Expect(err).ToNot(HaveOccurred())

			// Verify all types represented
			var toolset map[string]interface{}
			err = json.Unmarshal([]byte(toolsetJSON), &toolset)
			Expect(err).ToNot(HaveOccurred())

			tools := toolset["tools"].([]interface{})
			Expect(len(tools)).To(BeNumerically(">=", 3))

			// Collect types
			types := make(map[string]bool)
			for _, tool := range tools {
				t := tool.(map[string]interface{})
				if toolType, ok := t["type"].(string); ok {
					types[toolType] = true
				}
			}

			// Verify variety of types
			Expect(len(types)).To(BeNumerically(">=", 2),
				"Should have multiple service types represented")
		})

		It("should generate valid JSON that conforms to HolmesGPT schema", func() {
			// Create service
			svc := createServiceWithLabels(genNs, "prometheus-schema", map[string]string{"app": "prometheus"}, 9090)
			_, err := k8sClient.CoreV1().Services(genNs).Create(testCtx, svc, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(500 * time.Millisecond)

			// Execute discovery
			discovered, err := discoverer.DiscoverServices(testCtx)
			Expect(err).ToNot(HaveOccurred())

			// Filter for test service
			var testServices []*toolset.DiscoveredService
			for i := range discovered {
				if discovered[i].Namespace == genNs {
					testServices = append(testServices, &discovered[i])
				}
			}

			// Generate toolset
			toolsetJSON, err := gen.GenerateToolset(testCtx, testServices)
			Expect(err).ToNot(HaveOccurred())

			// Verify JSON structure conforms to expected schema
			var toolset map[string]interface{}
			err = json.Unmarshal([]byte(toolsetJSON), &toolset)
			Expect(err).ToNot(HaveOccurred())

			// Verify required fields
			Expect(toolset).To(HaveKey("tools"), "Should have tools field")

			tools := toolset["tools"].([]interface{})
			for _, tool := range tools {
				t := tool.(map[string]interface{})

				// Verify each tool has required fields
				Expect(t).To(HaveKey("name"), "Tool should have name")
				Expect(t).To(HaveKey("type"), "Tool should have type")
				Expect(t).To(HaveKey("endpoint"), "Tool should have endpoint")
				Expect(t).To(HaveKey("namespace"), "Tool should have namespace")

				// Verify field types
				Expect(t["name"]).To(BeAssignableToTypeOf("string"))
				Expect(t["type"]).To(BeAssignableToTypeOf("string"))
				Expect(t["endpoint"]).To(BeAssignableToTypeOf("string"))
				Expect(t["namespace"]).To(BeAssignableToTypeOf("string"))
			}
		})
	})
})
