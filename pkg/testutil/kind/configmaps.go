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

package kind

import (
	"fmt"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConfigMapConfig defines a test ConfigMap to deploy to Kind cluster.
type ConfigMapConfig struct {
	Name      string
	Namespace string
	Labels    map[string]string
	Data      map[string]string
}

// DeployConfigMap creates a ConfigMap in the Kind cluster.
// Returns the created ConfigMap or fails the test if deployment fails.
//
// Example:
//
//	cm, err := suite.DeployConfigMap(kind.ConfigMapConfig{
//	    Name:      "my-config",
//	    Namespace: "test-ns",
//	    Data:      map[string]string{"key": "value"},
//	})
func (s *IntegrationSuite) DeployConfigMap(config ConfigMapConfig) (*corev1.ConfigMap, error) {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.Name,
			Namespace: config.Namespace,
			Labels:    config.Labels,
		},
		Data: config.Data,
	}

	createdCM, err := s.Client.CoreV1().ConfigMaps(config.Namespace).Create(
		s.Context, configMap, metav1.CreateOptions{})

	if err != nil {
		return nil, err
	}

	// Register cleanup
	s.RegisterCleanup(func() {
		_ = s.Client.CoreV1().ConfigMaps(config.Namespace).Delete(
			s.Context, config.Name, metav1.DeleteOptions{})
	})

	return createdCM, nil
}

// GetConfigMap retrieves a ConfigMap from the Kind cluster.
//
// Example:
//
//	cm, err := suite.GetConfigMap("kubernaut-system", "toolset-config")
//	Expect(err).ToNot(HaveOccurred())
func (s *IntegrationSuite) GetConfigMap(namespace, name string) (*corev1.ConfigMap, error) {
	return s.Client.CoreV1().ConfigMaps(namespace).Get(
		s.Context, name, metav1.GetOptions{})
}

// DeleteConfigMap deletes a ConfigMap from the Kind cluster.
// If the ConfigMap doesn't exist, it's treated as success (idempotent).
func (s *IntegrationSuite) DeleteConfigMap(namespace, name string) error {
	err := s.Client.CoreV1().ConfigMaps(namespace).Delete(
		s.Context, name, metav1.DeleteOptions{})

	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	return nil
}

// UpdateConfigMap updates an existing ConfigMap in the Kind cluster.
//
// Example:
//
//	cm, _ := suite.GetConfigMap("test-ns", "my-config")
//	cm.Data["new-key"] = "new-value"
//	updatedCM, err := suite.UpdateConfigMap(cm)
func (s *IntegrationSuite) UpdateConfigMap(configMap *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	return s.Client.CoreV1().ConfigMaps(configMap.Namespace).Update(
		s.Context, configMap, metav1.UpdateOptions{})
}

// ConfigMapExists checks if a ConfigMap exists in the Kind cluster.
func (s *IntegrationSuite) ConfigMapExists(namespace, name string) bool {
	_, err := s.GetConfigMap(namespace, name)
	return err == nil
}

// WaitForConfigMap waits for a ConfigMap to be created in the Kind cluster.
// Uses Gomega's Eventually() for asynchronous assertions with configurable timeout.
//
// Example:
//
//	cm := suite.WaitForConfigMap("kubernaut-system", "toolset-config", 30*time.Second)
//	Expect(cm.Data).To(HaveKey("prometheus-toolset.yaml"))
func (s *IntegrationSuite) WaitForConfigMap(namespace, name string, timeout time.Duration) *corev1.ConfigMap {
	var cm *corev1.ConfigMap
	Eventually(func() error {
		var err error
		cm, err = s.GetConfigMap(namespace, name)
		return err
	}, timeout, 500*time.Millisecond).Should(Succeed(),
		fmt.Sprintf("ConfigMap %s/%s should exist within %v", namespace, name, timeout))

	return cm
}

// WaitForConfigMapKey waits for a specific key to exist in a ConfigMap.
// Uses Gomega's Eventually() for asynchronous assertions.
//
// Example:
//
//	cm := suite.WaitForConfigMapKey("kubernaut-system", "toolset-config",
//	    "prometheus-toolset.yaml", 30*time.Second)
func (s *IntegrationSuite) WaitForConfigMapKey(namespace, configMapName, key string, timeout time.Duration) *corev1.ConfigMap {
	var cm *corev1.ConfigMap
	Eventually(func() bool {
		var err error
		cm, err = s.GetConfigMap(namespace, configMapName)
		if err != nil {
			return false
		}
		_, exists := cm.Data[key]
		return exists
	}, timeout, 500*time.Millisecond).Should(BeTrue(),
		fmt.Sprintf("ConfigMap %s/%s should have key %s within %v", namespace, configMapName, key, timeout))

	return cm
}

// WaitForConfigMapUpdate waits for a ConfigMap to be updated (resourceVersion changes).
// This is useful for testing reconciliation loops that update ConfigMaps.
//
// Example:
//
//	initialCM, _ := suite.GetConfigMap("test-ns", "my-config")
//	// Trigger reconciliation...
//	updatedCM := suite.WaitForConfigMapUpdate("test-ns", "my-config",
//	    initialCM.ResourceVersion, 30*time.Second)
func (s *IntegrationSuite) WaitForConfigMapUpdate(namespace, name, initialResourceVersion string, timeout time.Duration) *corev1.ConfigMap {
	var cm *corev1.ConfigMap
	Eventually(func() bool {
		var err error
		cm, err = s.GetConfigMap(namespace, name)
		if err != nil {
			return false
		}
		// ResourceVersion changes when ConfigMap is updated
		return cm.ResourceVersion != initialResourceVersion
	}, timeout, 500*time.Millisecond).Should(BeTrue(),
		fmt.Sprintf("ConfigMap %s/%s should be updated within %v", namespace, name, timeout))

	return cm
}

// ConfigMapHasData checks if a ConfigMap has the expected data.
// This is a convenience method for common assertion pattern.
//
// Example:
//
//	suite.ConfigMapHasData("test-ns", "my-config", map[string]string{
//	    "key1": "value1",
//	    "key2": "value2",
//	})
func (s *IntegrationSuite) ConfigMapHasData(namespace, name string, expectedData map[string]string) {
	cm, err := s.GetConfigMap(namespace, name)
	Expect(err).ToNot(HaveOccurred(), "ConfigMap should exist")

	for key, expectedValue := range expectedData {
		actualValue, exists := cm.Data[key]
		Expect(exists).To(BeTrue(), fmt.Sprintf("ConfigMap should have key: %s", key))
		Expect(actualValue).To(Equal(expectedValue),
			fmt.Sprintf("ConfigMap key %s should have value %s", key, expectedValue))
	}
}
