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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jordigilh/kubernaut/pkg/toolset/configmap"
)

var _ = Describe("ConfigMap Operations Integration", func() {
	var (
		builder       configmap.ConfigMapBuilder
		testCtx       context.Context
		testNamespace string
		testConfigMap string
	)

	BeforeEach(func() {
		testCtx = context.Background()
		testNamespace = "default"
		testConfigMap = "test-holmesgpt-toolset"

		// Builder takes name and namespace
		builder = configmap.NewConfigMapBuilder(testConfigMap, testNamespace)
	})

	AfterEach(func() {
		// Clean up test ConfigMap
		_ = k8sClient.CoreV1().ConfigMaps(testNamespace).Delete(testCtx, testConfigMap, metav1.DeleteOptions{})
	})

	Describe("ConfigMap Creation", func() {
		It("should create a ConfigMap with toolset JSON", func() {
			toolsetJSON := `{"tools":[{"name":"prometheus","url":"http://prometheus:9090"}]}`

			cm, err := builder.BuildConfigMap(testCtx, toolsetJSON)
			Expect(err).ToNot(HaveOccurred())
			Expect(cm).ToNot(BeNil())

			// Verify ConfigMap structure
			Expect(cm.Name).To(Equal(testConfigMap))
			Expect(cm.Namespace).To(Equal(testNamespace))
			Expect(cm.Data).To(HaveKey("toolset.json"))
			Expect(cm.Data["toolset.json"]).To(Equal(toolsetJSON))

			// Verify labels (based on actual implementation)
			Expect(cm.Labels).To(HaveKey("app.kubernetes.io/managed-by"))
			Expect(cm.Labels).To(HaveKey("app.kubernetes.io/component"))

			// Create in cluster
			created, err := k8sClient.CoreV1().ConfigMaps(testNamespace).Create(testCtx, cm, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(created.Name).To(Equal(testConfigMap))

			// Verify it exists
			retrieved, err := k8sClient.CoreV1().ConfigMaps(testNamespace).Get(testCtx, testConfigMap, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(retrieved.Data["toolset.json"]).To(Equal(toolsetJSON))
		})

		It("should reject invalid JSON", func() {
			invalidJSON := `{"tools": [invalid json}`

			_, err := builder.BuildConfigMap(testCtx, invalidJSON)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid"))
		})
	})

	Describe("ConfigMap Updates with Override Preservation", func() {
		It("should preserve manual overrides in labels", func() {
			// Create initial ConfigMap
			initialJSON := `{"tools":[{"name":"prometheus","url":"http://prometheus:9090"}]}`
			cm, err := builder.BuildConfigMap(testCtx, initialJSON)
			Expect(err).ToNot(HaveOccurred())

			created, err := k8sClient.CoreV1().ConfigMaps(testNamespace).Create(testCtx, cm, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Manually add custom labels
			created.Labels["custom-label"] = "custom-value"
			created.Labels["team"] = "platform"

			_, err = k8sClient.CoreV1().ConfigMaps(testNamespace).Update(testCtx, created, metav1.UpdateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Retrieve the manually updated ConfigMap
			existing, err := k8sClient.CoreV1().ConfigMaps(testNamespace).Get(testCtx, testConfigMap, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Build new ConfigMap with updated toolset
			updatedJSON := `{"tools":[{"name":"prometheus","url":"http://prometheus:9090"},{"name":"grafana","url":"http://grafana:3000"}]}`
			updated, err := builder.BuildConfigMapWithOverrides(testCtx, updatedJSON, existing)
			Expect(err).ToNot(HaveOccurred())

			// Verify custom labels are preserved
			Expect(updated.Labels).To(HaveKeyWithValue("custom-label", "custom-value"))
			Expect(updated.Labels).To(HaveKeyWithValue("team", "platform"))

			// Verify managed labels are updated
			Expect(updated.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "kubernaut"))

			// Verify toolset data is updated
			Expect(updated.Data["toolset.json"]).To(Equal(updatedJSON))
		})

		It("should preserve manual overrides in annotations", func() {
			// Create initial ConfigMap
			initialJSON := `{"tools":[{"name":"prometheus","url":"http://prometheus:9090"}]}`
			cm, err := builder.BuildConfigMap(testCtx, initialJSON)
			Expect(err).ToNot(HaveOccurred())

			created, err := k8sClient.CoreV1().ConfigMaps(testNamespace).Create(testCtx, cm, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Manually add custom annotations
			if created.Annotations == nil {
				created.Annotations = make(map[string]string)
			}
			created.Annotations["custom-annotation"] = "important-value"
			created.Annotations["contact"] = "platform-team@example.com"

			_, err = k8sClient.CoreV1().ConfigMaps(testNamespace).Update(testCtx, created, metav1.UpdateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Retrieve the manually updated ConfigMap
			existing, err := k8sClient.CoreV1().ConfigMaps(testNamespace).Get(testCtx, testConfigMap, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Build new ConfigMap with updated toolset
			updatedJSON := `{"tools":[{"name":"prometheus","url":"http://prometheus:9090"},{"name":"grafana","url":"http://grafana:3000"}]}`
			updated, err := builder.BuildConfigMapWithOverrides(testCtx, updatedJSON, existing)
			Expect(err).ToNot(HaveOccurred())

			// Verify custom annotations are preserved
			Expect(updated.Annotations).To(HaveKeyWithValue("custom-annotation", "important-value"))
			Expect(updated.Annotations).To(HaveKeyWithValue("contact", "platform-team@example.com"))
		})

		It("should preserve manual overrides in data fields", func() {
			// Create initial ConfigMap
			initialJSON := `{"tools":[{"name":"prometheus","url":"http://prometheus:9090"}]}`
			cm, err := builder.BuildConfigMap(testCtx, initialJSON)
			Expect(err).ToNot(HaveOccurred())

			created, err := k8sClient.CoreV1().ConfigMaps(testNamespace).Create(testCtx, cm, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Manually add custom data fields
			created.Data["custom-config"] = "custom value"
			created.Data["notes.txt"] = "This is manually added"

			_, err = k8sClient.CoreV1().ConfigMaps(testNamespace).Update(testCtx, created, metav1.UpdateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Retrieve the manually updated ConfigMap
			existing, err := k8sClient.CoreV1().ConfigMaps(testNamespace).Get(testCtx, testConfigMap, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Build new ConfigMap with updated toolset
			updatedJSON := `{"tools":[{"name":"prometheus","url":"http://prometheus:9090"},{"name":"grafana","url":"http://grafana:3000"}]}`
			updated, err := builder.BuildConfigMapWithOverrides(testCtx, updatedJSON, existing)
			Expect(err).ToNot(HaveOccurred())

			// Verify custom data fields are preserved
			Expect(updated.Data).To(HaveKeyWithValue("custom-config", "custom value"))
			Expect(updated.Data).To(HaveKeyWithValue("notes.txt", "This is manually added"))

			// Verify toolset data is updated
			Expect(updated.Data["toolset.json"]).To(Equal(updatedJSON))
		})
	})

	Describe("Drift Detection", func() {
		It("should detect drift when toolset JSON changes", func() {
			// Create ConfigMap with initial toolset
			initialJSON := `{"tools":[{"name":"prometheus","url":"http://prometheus:9090"}]}`
			cm, err := builder.BuildConfigMap(testCtx, initialJSON)
			Expect(err).ToNot(HaveOccurred())

			_, err = k8sClient.CoreV1().ConfigMaps(testNamespace).Create(testCtx, cm, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Retrieve existing ConfigMap
			existing, err := k8sClient.CoreV1().ConfigMaps(testNamespace).Get(testCtx, testConfigMap, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Check for drift with new toolset JSON
			newJSON := `{"tools":[{"name":"prometheus","url":"http://prometheus:9090"},{"name":"grafana","url":"http://grafana:3000"}]}`
			hasDrift := builder.DetectDrift(testCtx, existing, newJSON)
			Expect(err).ToNot(HaveOccurred())
			Expect(hasDrift).To(BeTrue())
		})

		It("should not detect drift when toolset JSON is identical", func() {
			// Create ConfigMap
			toolsetJSON := `{"tools":[{"name":"prometheus","url":"http://prometheus:9090"}]}`
			cm, err := builder.BuildConfigMap(testCtx, toolsetJSON)
			Expect(err).ToNot(HaveOccurred())

			_, err = k8sClient.CoreV1().ConfigMaps(testNamespace).Create(testCtx, cm, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Retrieve existing ConfigMap
			existing, err := k8sClient.CoreV1().ConfigMaps(testNamespace).Get(testCtx, testConfigMap, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Check for drift with same toolset JSON
			hasDrift := builder.DetectDrift(testCtx, existing, toolsetJSON)
			Expect(hasDrift).To(BeFalse())
		})

		It("should detect drift when toolset JSON has semantic differences", func() {
			// Create ConfigMap with formatted JSON
			initialJSON := `{
  "tools": [
    {
      "name": "prometheus",
      "url": "http://prometheus:9090"
    }
  ]
}`
			cm, err := builder.BuildConfigMap(testCtx, initialJSON)
			Expect(err).ToNot(HaveOccurred())

			_, err = k8sClient.CoreV1().ConfigMaps(testNamespace).Create(testCtx, cm, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Retrieve existing ConfigMap
			existing, err := k8sClient.CoreV1().ConfigMaps(testNamespace).Get(testCtx, testConfigMap, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Check for drift with different content (same formatting)
			newJSON := `{
  "tools": [
    {
      "name": "grafana",
      "url": "http://grafana:3000"
    }
  ]
}`
			hasDrift := builder.DetectDrift(testCtx, existing, newJSON)
			Expect(hasDrift).To(BeTrue())
		})
	})

	Describe("End-to-End ConfigMap Workflow", func() {
		It("should create, update, and detect drift in ConfigMap", func() {
			// Step 1: Create initial ConfigMap
			initialJSON := `{"tools":[{"name":"prometheus","url":"http://prometheus-server.monitoring.svc:9090"}]}`
			cm, err := builder.BuildConfigMap(testCtx, initialJSON)
			Expect(err).ToNot(HaveOccurred())

			_, err = k8sClient.CoreV1().ConfigMaps(testNamespace).Create(testCtx, cm, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Step 2: Verify creation
			retrieved, err := k8sClient.CoreV1().ConfigMaps(testNamespace).Get(testCtx, testConfigMap, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(retrieved.Data["toolset.json"]).To(Equal(initialJSON))

			// Step 3: Manually add override
			retrieved.Labels["environment"] = "production"
			_, err = k8sClient.CoreV1().ConfigMaps(testNamespace).Update(testCtx, retrieved, metav1.UpdateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Step 4: Update with new service
			updatedJSON := `{"tools":[{"name":"prometheus","url":"http://prometheus-server.monitoring.svc:9090"},{"name":"grafana","url":"http://grafana.monitoring.svc:3000"}]}`

			existing, err := k8sClient.CoreV1().ConfigMaps(testNamespace).Get(testCtx, testConfigMap, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Step 5: Detect drift
			hasDrift := builder.DetectDrift(testCtx, existing, updatedJSON)
			Expect(hasDrift).To(BeTrue())

			// Step 6: Update with overrides preserved
			updated, err := builder.BuildConfigMapWithOverrides(testCtx, updatedJSON, existing)
			Expect(err).ToNot(HaveOccurred())

			// Verify override is preserved
			Expect(updated.Labels).To(HaveKeyWithValue("environment", "production"))

			// Verify toolset is updated
			var updatedTools map[string]interface{}
			err = json.Unmarshal([]byte(updated.Data["toolset.json"]), &updatedTools)
			Expect(err).ToNot(HaveOccurred())

			tools := updatedTools["tools"].([]interface{})
			Expect(len(tools)).To(Equal(2))

			// Step 7: Apply update to cluster
			_, err = k8sClient.CoreV1().ConfigMaps(testNamespace).Update(testCtx, updated, metav1.UpdateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Step 8: Verify final state
			final, err := k8sClient.CoreV1().ConfigMaps(testNamespace).Get(testCtx, testConfigMap, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(final.Labels).To(HaveKeyWithValue("environment", "production"))
			Expect(final.Data["toolset.json"]).To(Equal(updatedJSON))
		})
	})

	Describe("ConfigMap Not Found Scenarios", func() {
		It("should handle ConfigMap not found gracefully", func() {
			// Try to get non-existent ConfigMap
			_, err := k8sClient.CoreV1().ConfigMaps(testNamespace).Get(testCtx, "nonexistent-cm", metav1.GetOptions{})
			Expect(err).To(HaveOccurred())
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("should create ConfigMap when it doesn't exist", func() {
			toolsetJSON := `{"tools":[{"name":"prometheus","url":"http://prometheus:9090"}]}`

			// Check if exists
			_, err := k8sClient.CoreV1().ConfigMaps(testNamespace).Get(testCtx, testConfigMap, metav1.GetOptions{})
			if err != nil && errors.IsNotFound(err) {
				// Create new
				cm, err := builder.BuildConfigMap(testCtx, toolsetJSON)
				Expect(err).ToNot(HaveOccurred())

				_, err = k8sClient.CoreV1().ConfigMaps(testNamespace).Create(testCtx, cm, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())
			}

			// Verify it now exists
			retrieved, err := k8sClient.CoreV1().ConfigMaps(testNamespace).Get(testCtx, testConfigMap, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(retrieved.Data["toolset.json"]).To(Equal(toolsetJSON))
		})
	})
})
