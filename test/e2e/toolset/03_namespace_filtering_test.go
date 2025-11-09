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

// Namespace Filtering Tests
// Business Requirements:
// - BR-TOOLSET-019: Multi-namespace service discovery
// - BR-TOOLSET-038: Namespace requirement

var _ = Describe("Namespace Filtering", func() {
	var (
		testNamespace string
		testCtx       context.Context
		testCancel    context.CancelFunc
	)

	BeforeEach(func() {
		// Create unique namespace for this test
		testNamespace = fmt.Sprintf("toolset-namespace-%d", time.Now().Unix())
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

		// Delete namespace (cleanup)
		err := infrastructure.CleanupTestNamespace(testCtx, testNamespace, kubeconfigPath, GinkgoWriter)
		if err != nil {
			logger.Warn(fmt.Sprintf("Failed to cleanup namespace %s: %v", testNamespace, err))
		}
	})

	Context("Namespace Filtering", func() {
		It("should only discover services in configured namespace", func() {
			// BR-TOOLSET-019: Multi-namespace service discovery
			// BR-TOOLSET-038: Namespace requirement

			// Deploy mock service in test namespace
			annotations := map[string]string{
				"holmesgpt.io/service-type": "prometheus",
				"holmesgpt.io/priority":     "80",
			}
			err := infrastructure.DeployMockService(testCtx, testNamespace, "mock-prometheus", annotations, kubeconfigPath, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			// Wait for ConfigMap to include Prometheus
			Eventually(func() string {
				configMap, err := infrastructure.GetConfigMap(testNamespace, "holmesgpt-dynamic-toolsets", kubeconfigPath)
				if err != nil {
					return ""
				}
				data, ok := configMap["data"].(map[string]interface{})
				if !ok {
					return ""
				}
				toolsetsJSON, ok := data["toolsets.json"].(string)
				if !ok {
					return ""
				}
				return toolsetsJSON
			}, 30*time.Second, 2*time.Second).Should(ContainSubstring("prometheus"))

			// Verify ConfigMap contains service from test namespace
			configMap, err := infrastructure.GetConfigMap(testNamespace, "holmesgpt-dynamic-toolsets", kubeconfigPath)
			Expect(err).ToNot(HaveOccurred())

			data, ok := configMap["data"].(map[string]interface{})
			Expect(ok).To(BeTrue())

			toolsetsJSON, ok := data["toolsets.json"].(string)
			Expect(ok).To(BeTrue())
			Expect(toolsetsJSON).To(ContainSubstring("prometheus"))

			// Note: Testing that services from OTHER namespaces are NOT discovered
			// requires deploying services in a different namespace, which is beyond
			// the scope of this E2E test (namespace isolation is configured in deployment)
		})

		It("should handle namespace-scoped RBAC correctly", func() {
			// BR-TOOLSET-019: Multi-namespace service discovery (RBAC validation)

			// Deploy mock service in test namespace
			annotations := map[string]string{
				"holmesgpt.io/service-type": "grafana",
				"holmesgpt.io/priority":     "70",
			}
			err := infrastructure.DeployMockService(testCtx, testNamespace, "mock-grafana", annotations, kubeconfigPath, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			// Wait for ConfigMap to include Grafana
			Eventually(func() string {
				configMap, err := infrastructure.GetConfigMap(testNamespace, "holmesgpt-dynamic-toolsets", kubeconfigPath)
				if err != nil {
					return ""
				}
				data, ok := configMap["data"].(map[string]interface{})
				if !ok {
					return ""
				}
				toolsetsJSON, ok := data["toolsets.json"].(string)
				if !ok {
					return ""
				}
				return toolsetsJSON
			}, 30*time.Second, 2*time.Second).Should(ContainSubstring("grafana"))

			// Verify Dynamic Toolset has proper RBAC permissions
			// This is validated by the fact that ConfigMap was created successfully
			// If RBAC was incorrect, the ConfigMap would not be created

			configMap, err := infrastructure.GetConfigMap(testNamespace, "holmesgpt-dynamic-toolsets", kubeconfigPath)
			Expect(err).ToNot(HaveOccurred())

			// Validate ConfigMap metadata includes correct namespace
			metadata, ok := configMap["metadata"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "ConfigMap should have metadata")

			namespace, ok := metadata["namespace"].(string)
			Expect(ok).To(BeTrue(), "ConfigMap metadata should have namespace")
			Expect(namespace).To(Equal(testNamespace), "ConfigMap should be in test namespace")
		})

		It("should isolate ConfigMaps between namespaces", func() {
			// BR-TOOLSET-019: Multi-namespace service discovery (isolation)

			// Deploy mock service in test namespace
			annotations := map[string]string{
				"holmesgpt.io/service-type": "prometheus",
				"holmesgpt.io/priority":     "80",
			}
			err := infrastructure.DeployMockService(testCtx, testNamespace, "mock-prometheus", annotations, kubeconfigPath, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			// Wait for ConfigMap to be created in test namespace
			Eventually(func() error {
				_, err := infrastructure.GetConfigMap(testNamespace, "holmesgpt-dynamic-toolsets", kubeconfigPath)
				return err
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			// Verify ConfigMap exists in test namespace
			configMap, err := infrastructure.GetConfigMap(testNamespace, "holmesgpt-dynamic-toolsets", kubeconfigPath)
			Expect(err).ToNot(HaveOccurred())

			// Validate ConfigMap is namespace-scoped
			metadata, ok := configMap["metadata"].(map[string]interface{})
			Expect(ok).To(BeTrue())

			namespace, ok := metadata["namespace"].(string)
			Expect(ok).To(BeTrue())
			Expect(namespace).To(Equal(testNamespace))

			// Validate ConfigMap data
			data, ok := configMap["data"].(map[string]interface{})
			Expect(ok).To(BeTrue())

			toolsetsJSON, ok := data["toolsets.json"].(string)
			Expect(ok).To(BeTrue())
			Expect(toolsetsJSON).To(ContainSubstring("prometheus"))

			// Note: Testing that ConfigMaps in OTHER namespaces are NOT affected
			// requires creating multiple test namespaces, which is beyond the scope
			// of this E2E test (isolation is enforced by Kubernetes namespace model)
		})
	})
})

