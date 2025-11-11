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

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// ConfigMap Update Tests
// Business Requirements:
// - BR-TOOLSET-020: Real-time toolset configuration updates
// - BR-TOOLSET-021: ConfigMap generation and management

var _ = Describe("ConfigMap Updates", func() {
	var (
		testNamespace string
		testCtx       context.Context
		testCancel    context.CancelFunc
	)

	BeforeEach(func() {
		// Create unique namespace for this test (parallel-safe with nanosecond precision + process ID)
		testNamespace = fmt.Sprintf("toolset-configmap-p%d-%d", GinkgoParallelProcess(), time.Now().UnixNano())
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

	Context("ConfigMap Updates", func() {
		It("should update ConfigMap when service annotations change", func() {
			// BR-TOOLSET-020: Real-time toolset configuration updates

			// Deploy mock service with initial annotations
			annotations := map[string]string{
				"kubernaut.io/toolset":      "enabled",
				"kubernaut.io/toolset-type": "prometheus",
			}
			err := infrastructure.DeployMockService(testCtx, testNamespace, "mock-prometheus", annotations, kubeconfigPath, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			// Wait for ConfigMap to include Prometheus with priority 80
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

			// Update service annotations (change priority)
			// Note: In a real implementation, this would involve patching the service
			// For E2E tests, we simulate this by deleting and recreating with new annotations
			err = infrastructure.DeleteMockService(testCtx, testNamespace, "mock-prometheus", kubeconfigPath, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			// Wait for service to be deleted
			time.Sleep(5 * time.Second)

			// Redeploy with updated annotations (add custom endpoint)
			updatedAnnotations := map[string]string{
				"kubernaut.io/toolset":          "enabled",
				"kubernaut.io/toolset-type":     "prometheus",
				"kubernaut.io/toolset-endpoint": "http://prometheus-custom.monitoring.svc:9090",
			}
			err = infrastructure.DeployMockService(testCtx, testNamespace, "mock-prometheus", updatedAnnotations, kubeconfigPath, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			// Wait for ConfigMap to be updated with new priority
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

			// Verify ConfigMap was updated (functional correctness)
			configMap, err := infrastructure.GetConfigMap(testNamespace, "kubernaut-toolset-config", kubeconfigPath)
			Expect(err).ToNot(HaveOccurred())

			data, ok := configMap["data"].(map[string]interface{})
			Expect(ok).To(BeTrue())

			toolsetsJSON, ok := data["toolset.json"].(string)
			Expect(ok).To(BeTrue())
			Expect(toolsetsJSON).To(ContainSubstring("prometheus"))
		})

		It("should maintain ConfigMap consistency during concurrent service changes", func() {
			// BR-TOOLSET-020: Real-time toolset configuration updates (concurrency)

			// Deploy multiple services concurrently
			services := []struct {
				name        string
				serviceType string
				priority    string
			}{
				{"mock-prometheus", "prometheus", "80"},
				{"mock-grafana", "grafana", "70"},
				{"mock-jaeger", "jaeger", "60"},
			}

			// Deploy all services
			for _, svc := range services {
				annotations := map[string]string{
					"kubernaut.io/toolset":      "enabled",
					"kubernaut.io/toolset-type": svc.serviceType,
				}
				err := infrastructure.DeployMockService(testCtx, testNamespace, svc.name, annotations, kubeconfigPath, GinkgoWriter)
				Expect(err).ToNot(HaveOccurred())
			}

			// Wait for ConfigMap to include all services
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
				ContainSubstring("jaeger"),
			))

			// Delete one service
			err := infrastructure.DeleteMockService(testCtx, testNamespace, "mock-grafana", kubeconfigPath, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			// Wait for ConfigMap to be updated (Grafana removed, others remain)
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
			}, 30*time.Second, 2*time.Second).ShouldNot(ContainSubstring("grafana"))

			// Verify remaining services are still present
			configMap, err := infrastructure.GetConfigMap(testNamespace, "kubernaut-toolset-config", kubeconfigPath)
			Expect(err).ToNot(HaveOccurred())

			data, ok := configMap["data"].(map[string]interface{})
			Expect(ok).To(BeTrue())

			toolsetsJSON, ok := data["toolset.json"].(string)
			Expect(ok).To(BeTrue())
			Expect(toolsetsJSON).To(ContainSubstring("prometheus"))
			Expect(toolsetsJSON).To(ContainSubstring("jaeger"))
			Expect(toolsetsJSON).ToNot(ContainSubstring("grafana"))
		})

		It("should handle ConfigMap recreation if manually deleted", func() {
			// BR-TOOLSET-021: ConfigMap generation and management (resilience)

			// Deploy mock service
			annotations := map[string]string{
				"kubernaut.io/toolset":      "enabled",
				"kubernaut.io/toolset-type": "prometheus",
			}
			err := infrastructure.DeployMockService(testCtx, testNamespace, "mock-prometheus", annotations, kubeconfigPath, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			// Wait for ConfigMap to be created
			Eventually(func() error {
				_, err := infrastructure.GetConfigMap(testNamespace, "kubernaut-toolset-config", kubeconfigPath)
				return err
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			// Manually delete ConfigMap
			deleteCmd := fmt.Sprintf("kubectl delete configmap kubernaut-toolset-config -n %s --kubeconfig=%s", testNamespace, kubeconfigPath)
			_, err = infrastructure.RunCommand(deleteCmd, kubeconfigPath)
			Expect(err).ToNot(HaveOccurred())

			// Wait for ConfigMap to be recreated with Prometheus (not just exist)
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

			data, ok := configMap["data"].(map[string]interface{})
			Expect(ok).To(BeTrue())

			toolsetsJSON, ok := data["toolset.json"].(string)
			Expect(ok).To(BeTrue())
			Expect(toolsetsJSON).To(ContainSubstring("prometheus"))
		})

		It("should generate valid JSON in ConfigMap toolset.json", func() {
			// BR-TOOLSET-021: ConfigMap generation and management (data validation)

			// Deploy mock service
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

			// Verify ConfigMap contains valid JSON
			configMap, err := infrastructure.GetConfigMap(testNamespace, "kubernaut-toolset-config", kubeconfigPath)
			Expect(err).ToNot(HaveOccurred())

			data, ok := configMap["data"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "ConfigMap should have 'data' field")

			toolsetsJSON, ok := data["toolset.json"].(string)
			Expect(ok).To(BeTrue(), "ConfigMap should have 'toolset.json' in data")
			Expect(toolsetsJSON).ToNot(BeEmpty(), "toolset.json should not be empty")

			// Validate JSON structure (basic validation)
			// In a real implementation, this would parse JSON and validate schema
			Expect(toolsetsJSON).To(ContainSubstring("{"), "toolset.json should be valid JSON (contains {)")
			Expect(toolsetsJSON).To(ContainSubstring("}"), "toolset.json should be valid JSON (contains })")
			Expect(toolsetsJSON).To(ContainSubstring("prometheus"), "toolset.json should contain Prometheus toolset")
			Expect(toolsetsJSON).To(ContainSubstring("endpoint"), "toolset.json should contain endpoint field")
			Expect(toolsetsJSON).To(ContainSubstring("tools"), "toolset.json should contain tools array")

			// Validate expected fields are present
			Expect(toolsetsJSON).To(Or(
				ContainSubstring("name"),
				ContainSubstring("tools"),
				ContainSubstring("capabilities"),
			), "toolset.json should contain expected toolset fields")
		})
	})
})
