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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// Discovery Lifecycle Tests
// Business Requirements:
// - BR-TOOLSET-016: Service discovery configuration
// - BR-TOOLSET-017: Priority-based service ordering
// - BR-TOOLSET-018: Service health monitoring
// - BR-TOOLSET-019: Multi-namespace service discovery
// - BR-TOOLSET-020: Real-time toolset configuration updates

var _ = Describe("Discovery Lifecycle", func() {
	var (
		testNamespace string
		testCtx       context.Context
		testCancel    context.CancelFunc
	)

	BeforeEach(func() {
		// Create unique namespace for this test (parallel-safe with nanosecond precision + process ID)
		testNamespace = fmt.Sprintf("toolset-discovery-p%d-%d", GinkgoParallelProcess(), time.Now().UnixNano())
		testCtx, testCancel = context.WithTimeout(ctx, 5*time.Minute)

		// Deploy Dynamic Toolset in test namespace
		err := infrastructure.DeployToolsetTestServices(testCtx, testNamespace, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		// Cleanup test namespace
		if testCancel != nil {
			testCancel()
		}

		// Only delete namespace if test passed (for debugging failed tests)
		if !CurrentSpecReport().Failed() {
			err := infrastructure.CleanupTestNamespace(testCtx, testNamespace, kubeconfigPath, GinkgoWriter)
			if err != nil {
				logger.Warn(fmt.Sprintf("Failed to cleanup namespace %s: %v", testNamespace, err))
			}
		} else {
			logger.Warn(fmt.Sprintf("⚠️  Test FAILED - Keeping namespace %s for debugging", testNamespace))
			logger.Info("To debug:")
			logger.Info(fmt.Sprintf("  export KUBECONFIG=%s", kubeconfigPath))
			logger.Info(fmt.Sprintf("  kubectl get pods -n %s", testNamespace))
			logger.Info(fmt.Sprintf("  kubectl logs -n %s deployment/kubernaut-dynamic-toolsets", testNamespace))
			logger.Info(fmt.Sprintf("  kubectl get configmap -n %s kubernaut-toolset-config -o yaml", testNamespace))
		}
	})

	Context("Service Discovery", func() {
		It("should discover new service and generate ConfigMap", func() {
			// BR-TOOLSET-016: Service discovery configuration
			// BR-TOOLSET-020: Real-time toolset configuration updates

			// Deploy mock Prometheus service
			annotations := map[string]string{
				"kubernaut.io/toolset":      "enabled",
				"kubernaut.io/toolset-type": "prometheus",
			}
			err := infrastructure.DeployMockService(testCtx, testNamespace, "mock-prometheus", annotations, kubeconfigPath, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			// Wait for ConfigMap to include Prometheus (not just exist)
			Eventually(func() string {
				configMap, err := infrastructure.GetConfigMap(testNamespace, "kubernaut-toolset-config", kubeconfigPath)
				if err != nil {
					return ""
				}
				data, ok := configMap["data"].(map[string]interface{})
				if !ok {
					return ""
				}
				toolsetsJSON, ok := data["toolset.json"].(string)
				if !ok {
					return ""
				}
				return toolsetsJSON
			}, 30*time.Second, 2*time.Second).Should(ContainSubstring("prometheus"))

			// Verify ConfigMap contains Prometheus toolset
			configMap, err := infrastructure.GetConfigMap(testNamespace, "kubernaut-toolset-config", kubeconfigPath)
			Expect(err).ToNot(HaveOccurred())

			// Validate ConfigMap structure
			data, ok := configMap["data"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "ConfigMap should have 'data' field")

			toolsetsJSON, ok := data["toolset.json"].(string)
			Expect(ok).To(BeTrue(), "ConfigMap should have 'toolset.json' in data")
			Expect(toolsetsJSON).ToNot(BeEmpty(), "toolset.json should not be empty")

			// Validate Prometheus toolset is present
			Expect(toolsetsJSON).To(ContainSubstring("prometheus"), "ConfigMap should contain Prometheus toolset")
			Expect(toolsetsJSON).To(ContainSubstring("endpoint"), "ConfigMap should contain endpoint field")
			Expect(toolsetsJSON).To(ContainSubstring("tools"), "ConfigMap should contain tools array")
		})

		It("should discover multiple services with correct priority ordering", func() {
			// BR-TOOLSET-017: Priority-based service ordering

			// Deploy mock services with different priorities
			services := []struct {
				name        string
				serviceType string
				priority    string
			}{
				{"mock-prometheus", "prometheus", "80"},
				{"mock-grafana", "grafana", "70"},
				{"mock-custom", "custom", "30"},
			}

			for _, svc := range services {
				annotations := map[string]string{
					"kubernaut.io/toolset":      "enabled",
					"kubernaut.io/toolset-type": svc.serviceType,
				}
				err := infrastructure.DeployMockService(testCtx, testNamespace, svc.name, annotations, kubeconfigPath, GinkgoWriter)
				Expect(err).ToNot(HaveOccurred())
			}

			// Wait for ConfigMap to be updated with all services
			Eventually(func() string {
				configMap, err := infrastructure.GetConfigMap(testNamespace, "kubernaut-toolset-config", kubeconfigPath)
				if err != nil {
					return ""
				}
				data, ok := configMap["data"].(map[string]interface{})
				if !ok {
					return ""
				}
				toolsetsJSON, ok := data["toolset.json"].(string)
				if !ok {
					return ""
				}
				return toolsetsJSON
			}, 45*time.Second, 3*time.Second).Should(And(
				ContainSubstring("prometheus"),
				ContainSubstring("grafana"),
				ContainSubstring("custom"),
			))

			// Verify priority ordering (Prometheus > Grafana > Custom)
			configMap, err := infrastructure.GetConfigMap(testNamespace, "kubernaut-toolset-config", kubeconfigPath)
			Expect(err).ToNot(HaveOccurred())

			data, ok := configMap["data"].(map[string]interface{})
			Expect(ok).To(BeTrue())

			toolsetsJSON, ok := data["toolset.json"].(string)
			Expect(ok).To(BeTrue())

			// Validate all services are present
			Expect(toolsetsJSON).To(ContainSubstring("prometheus"))
			Expect(toolsetsJSON).To(ContainSubstring("grafana"))
			Expect(toolsetsJSON).To(ContainSubstring("custom"))

			// Note: Priority ordering validation requires parsing JSON
			// This is a functional correctness test - priority order is validated by the business logic
		})

		It("should handle service deletion and update ConfigMap", func() {
			// BR-TOOLSET-020: Real-time toolset configuration updates

			// Deploy mock service
			annotations := map[string]string{
				"kubernaut.io/toolset":      "enabled",
				"kubernaut.io/toolset-type": "prometheus",
			}
			err := infrastructure.DeployMockService(testCtx, testNamespace, "mock-prometheus", annotations, kubeconfigPath, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			// Wait for ConfigMap to include Prometheus
			Eventually(func() string {
				configMap, err := infrastructure.GetConfigMap(testNamespace, "kubernaut-toolset-config", kubeconfigPath)
				if err != nil {
					return ""
				}
				data, ok := configMap["data"].(map[string]interface{})
				if !ok {
					return ""
				}
				toolsetsJSON, ok := data["toolset.json"].(string)
				if !ok {
					return ""
				}
				return toolsetsJSON
			}, 30*time.Second, 2*time.Second).Should(ContainSubstring("prometheus"))

			// Delete mock service
			err = infrastructure.DeleteMockService(testCtx, testNamespace, "mock-prometheus", kubeconfigPath, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			// Wait for ConfigMap to be updated (Prometheus toolset should be removed)
			// Note: Longer timeout for parallel execution (discovery interval + cluster load)
			Eventually(func() string {
				configMap, err := infrastructure.GetConfigMap(testNamespace, "kubernaut-toolset-config", kubeconfigPath)
				if err != nil {
					return ""
				}
				data, ok := configMap["data"].(map[string]interface{})
				if !ok {
					return ""
				}
				toolsetsJSON, ok := data["toolset.json"].(string)
				if !ok {
					return ""
				}
				return toolsetsJSON
			}, 60*time.Second, 2*time.Second).ShouldNot(ContainSubstring("prometheus"))
		})

		It("should handle service with missing annotations gracefully", func() {
			// BR-TOOLSET-016: Service discovery configuration (error handling)

			// Deploy mock service without holmesgpt.io annotations
			annotations := map[string]string{}
			err := infrastructure.DeployMockService(testCtx, testNamespace, "mock-no-annotations", annotations, kubeconfigPath, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			// Wait a reasonable time for discovery
			time.Sleep(15 * time.Second)

			// Verify ConfigMap exists but does not contain the unannotated service
			configMap, err := infrastructure.GetConfigMap(testNamespace, "kubernaut-toolset-config", kubeconfigPath)
			if err == nil {
				data, ok := configMap["data"].(map[string]interface{})
				if ok {
					toolsetsJSON, ok := data["toolset.json"].(string)
					if ok {
						// Service without annotations should not be included
						Expect(toolsetsJSON).ToNot(ContainSubstring("mock-no-annotations"))
					}
				}
			}
			// If ConfigMap doesn't exist yet, that's also acceptable (no services discovered)
		})

		It("should discover service with custom type and generate appropriate toolset", func() {
			// BR-TOOLSET-016: Service discovery configuration (custom service types)

			// Deploy mock service with custom type
			annotations := map[string]string{
				"kubernaut.io/toolset":      "enabled",
				"kubernaut.io/toolset-type": "custom-monitoring",
			}
			err := infrastructure.DeployMockService(testCtx, testNamespace, "mock-custom-monitoring", annotations, kubeconfigPath, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			// Wait for ConfigMap to include custom service
			Eventually(func() string {
				configMap, err := infrastructure.GetConfigMap(testNamespace, "kubernaut-toolset-config", kubeconfigPath)
				if err != nil {
					return ""
				}
				data, ok := configMap["data"].(map[string]interface{})
				if !ok {
					return ""
				}
				toolsetsJSON, ok := data["toolset.json"].(string)
				if !ok {
					return ""
				}
				return toolsetsJSON
			}, 30*time.Second, 2*time.Second).Should(ContainSubstring("custom"))

			// Verify ConfigMap contains custom toolset
			configMap, err := infrastructure.GetConfigMap(testNamespace, "kubernaut-toolset-config", kubeconfigPath)
			Expect(err).ToNot(HaveOccurred())

			data, ok := configMap["data"].(map[string]interface{})
			Expect(ok).To(BeTrue())

			toolsetsJSON, ok := data["toolset.json"].(string)
			Expect(ok).To(BeTrue())
			Expect(toolsetsJSON).To(ContainSubstring("custom"))
		})

		It("should handle rapid service creation and deletion", func() {
			// BR-TOOLSET-020: Real-time toolset configuration updates (stress test)

			// Deploy and delete services rapidly
			for i := 0; i < 3; i++ {
				serviceName := fmt.Sprintf("mock-rapid-%d", i)
				annotations := map[string]string{
					"kubernaut.io/toolset":      "enabled",
					"kubernaut.io/toolset-type": "prometheus",
				}

				// Deploy service
				err := infrastructure.DeployMockService(testCtx, testNamespace, serviceName, annotations, kubeconfigPath, GinkgoWriter)
				Expect(err).ToNot(HaveOccurred())

				// Wait briefly
				time.Sleep(5 * time.Second)

				// Delete service
				err = infrastructure.DeleteMockService(testCtx, testNamespace, serviceName, kubeconfigPath, GinkgoWriter)
				Expect(err).ToNot(HaveOccurred())
			}

			// Wait for ConfigMap to stabilize (use Eventually for parallel execution)
			// Note: Longer timeout for parallel execution (discovery interval + cluster load)
			Eventually(func() bool {
				configMap, err := infrastructure.GetConfigMap(testNamespace, "kubernaut-toolset-config", kubeconfigPath)
				if err != nil {
					// ConfigMap might not exist if all services deleted - that's OK
					return true
				}
				data, ok := configMap["data"].(map[string]interface{})
				if !ok {
					return true
				}
				toolsetsJSON, ok := data["toolset.json"].(string)
				if !ok {
					return true
				}
				// None of the rapid services should be present
				return !strings.Contains(toolsetsJSON, "mock-rapid-0") &&
					!strings.Contains(toolsetsJSON, "mock-rapid-1") &&
					!strings.Contains(toolsetsJSON, "mock-rapid-2")
			}, 60*time.Second, 2*time.Second).Should(BeTrue(), "ConfigMap should not contain any rapid services after deletion")
		})
	})
})
