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

package kind_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/testutil/kind"
)

// Example 1: Basic Integration Test Suite Setup
//
// This example shows the minimal setup needed for integration tests.
var _ = Describe("Example 1: Basic Suite Setup", func() {
	var suite *kind.IntegrationSuite

	BeforeEach(func() {
		// Setup creates test namespace and connects to Kind cluster
		suite = kind.Setup("example-test")
	})

	AfterEach(func() {
		// Cleanup deletes test namespaces and registered resources
		suite.Cleanup()
	})

	It("should connect to Kind cluster", func() {
		// Verify client is connected
		Expect(suite.Client).ToNot(BeNil())
		Expect(suite.Context).ToNot(BeNil())
	})
})

// Example 2: Service Discovery Testing
//
// This example shows how to test service discovery with real Kubernetes services.
var _ = Describe("Example 2: Service Discovery", func() {
	var suite *kind.IntegrationSuite

	BeforeEach(func() {
		suite = kind.Setup("service-discovery-test")
	})

	AfterEach(func() {
		suite.Cleanup()
	})

	It("should discover Prometheus service", func() {
		// Deploy Prometheus service to Kind cluster
		svc, err := suite.DeployPrometheusService("service-discovery-test")
		Expect(err).ToNot(HaveOccurred())
		Expect(svc.Name).To(Equal("prometheus"))

		// Get service endpoint (Kubernetes DNS format)
		endpoint := suite.GetServiceEndpoint("prometheus", "service-discovery-test", 9090)
		Expect(endpoint).To(Equal("http://prometheus.service-discovery-test.svc.cluster.local:9090"))
	})

	It("should discover multiple services", func() {
		// Deploy multiple services
		promSvc, err := suite.DeployPrometheusService("service-discovery-test")
		Expect(err).ToNot(HaveOccurred())

		grafanaSvc, err := suite.DeployGrafanaService("service-discovery-test")
		Expect(err).ToNot(HaveOccurred())

		// Verify both exist
		Expect(suite.ServiceExists("service-discovery-test", "prometheus")).To(BeTrue())
		Expect(suite.ServiceExists("service-discovery-test", "grafana")).To(BeTrue())

		// Verify labels
		Expect(promSvc.Labels["app"]).To(Equal("prometheus"))
		Expect(grafanaSvc.Labels["app"]).To(Equal("grafana"))
	})
})

// Example 3: ConfigMap Testing
//
// This example shows how to test ConfigMap operations.
var _ = Describe("Example 3: ConfigMap Operations", func() {
	var suite *kind.IntegrationSuite

	BeforeEach(func() {
		suite = kind.Setup("configmap-test", "kubernaut-system")
	})

	AfterEach(func() {
		suite.Cleanup()
	})

	It("should create and read ConfigMap", func() {
		// Create ConfigMap
		cm, err := suite.DeployConfigMap(kind.ConfigMapConfig{
			Name:      "test-config",
			Namespace: "configmap-test",
			Data: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(cm.Data).To(HaveKey("key1"))

		// Read ConfigMap
		retrievedCM, err := suite.GetConfigMap("configmap-test", "test-config")
		Expect(err).ToNot(HaveOccurred())
		Expect(retrievedCM.Data["key1"]).To(Equal("value1"))
	})

	It("should wait for ConfigMap creation", func() {
		// Deploy ConfigMap
		_, err := suite.DeployConfigMap(kind.ConfigMapConfig{
			Name:      "async-config",
			Namespace: "configmap-test",
			Data:      map[string]string{"test": "data"},
		})
		Expect(err).ToNot(HaveOccurred())

		// Wait for ConfigMap (useful for async operations)
		cm := suite.WaitForConfigMap("configmap-test", "async-config", 10*time.Second)
		Expect(cm.Data).To(HaveKey("test"))
	})

	It("should wait for ConfigMap key", func() {
		// Create ConfigMap without the key
		cm, err := suite.DeployConfigMap(kind.ConfigMapConfig{
			Name:      "partial-config",
			Namespace: "configmap-test",
			Data:      map[string]string{"initial": "data"},
		})
		Expect(err).ToNot(HaveOccurred())

		// Simulate async update (in real test, this would be controller)
		cm.Data["dynamic-key"] = "dynamic-value"
		_, err = suite.UpdateConfigMap(cm)
		Expect(err).ToNot(HaveOccurred())

		// Wait for specific key
		updatedCM := suite.WaitForConfigMapKey("configmap-test", "partial-config",
			"dynamic-key", 10*time.Second)
		Expect(updatedCM.Data["dynamic-key"]).To(Equal("dynamic-value"))
	})
})

// Example 4: Database Integration Testing
//
// This example shows how to test PostgreSQL integration.
var _ = Describe("Example 4: Database Integration", func() {
	var suite *kind.IntegrationSuite

	BeforeEach(func() {
		suite = kind.Setup("database-test")

		// Wait for PostgreSQL to be ready
		suite.WaitForPostgreSQLReady(60 * time.Second)
	})

	AfterEach(func() {
		suite.Cleanup()
	})

	It("should connect to PostgreSQL", func() {
		// Connect to PostgreSQL in Kind cluster
		db, err := suite.GetDefaultPostgreSQLConnection()
		Expect(err).ToNot(HaveOccurred())
		defer db.Close()

		// Execute simple query
		var result int
		err = db.QueryRow("SELECT 1").Scan(&result)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(1))
	})

	It("should create and query table", func() {
		db, err := suite.GetDefaultPostgreSQLConnection()
		Expect(err).ToNot(HaveOccurred())
		defer db.Close()

		// Create test table
		_, err = db.Exec(`
			CREATE TABLE IF NOT EXISTS test_table (
				id SERIAL PRIMARY KEY,
				name VARCHAR(100)
			)
		`)
		Expect(err).ToNot(HaveOccurred())

		// Insert data
		_, err = db.Exec("INSERT INTO test_table (name) VALUES ($1)", "test-name")
		Expect(err).ToNot(HaveOccurred())

		// Query data
		var name string
		err = db.QueryRow("SELECT name FROM test_table WHERE name = $1", "test-name").Scan(&name)
		Expect(err).ToNot(HaveOccurred())
		Expect(name).To(Equal("test-name"))

		// Cleanup
		_, err = db.Exec("DROP TABLE test_table")
		Expect(err).ToNot(HaveOccurred())
	})
})

// Example 5: Multi-Namespace Testing
//
// This example shows how to test across multiple namespaces.
var _ = Describe("Example 5: Multi-Namespace Testing", func() {
	var suite *kind.IntegrationSuite

	BeforeEach(func() {
		// Create multiple namespaces
		suite = kind.Setup("namespace-a", "namespace-b", "kubernaut-system")
	})

	AfterEach(func() {
		// All namespaces cleaned up automatically
		suite.Cleanup()
	})

	It("should deploy services to different namespaces", func() {
		// Deploy to namespace-a
		promSvc, err := suite.DeployPrometheusService("namespace-a")
		Expect(err).ToNot(HaveOccurred())
		Expect(promSvc.Namespace).To(Equal("namespace-a"))

		// Deploy to namespace-b
		grafanaSvc, err := suite.DeployGrafanaService("namespace-b")
		Expect(err).ToNot(HaveOccurred())
		Expect(grafanaSvc.Namespace).To(Equal("namespace-b"))

		// Verify isolation
		Expect(suite.ServiceExists("namespace-a", "prometheus")).To(BeTrue())
		Expect(suite.ServiceExists("namespace-a", "grafana")).To(BeFalse())

		Expect(suite.ServiceExists("namespace-b", "grafana")).To(BeTrue())
		Expect(suite.ServiceExists("namespace-b", "prometheus")).To(BeFalse())
	})
})

// Example 6: Custom Cleanup Registration
//
// This example shows how to register custom cleanup functions.
var _ = Describe("Example 6: Custom Cleanup", func() {
	var suite *kind.IntegrationSuite

	BeforeEach(func() {
		suite = kind.Setup("cleanup-test")
	})

	AfterEach(func() {
		suite.Cleanup() // Executes all registered cleanup functions
	})

	It("should execute custom cleanup", func() {
		cleanupExecuted := false

		// Register custom cleanup
		suite.RegisterCleanup(func() {
			cleanupExecuted = true
		})

		// Cleanup will execute the registered function
		suite.Cleanup()

		Expect(cleanupExecuted).To(BeTrue())
	})

	It("should execute cleanup in reverse order", func() {
		var executionOrder []string

		// Register multiple cleanup functions
		suite.RegisterCleanup(func() {
			executionOrder = append(executionOrder, "first")
		})
		suite.RegisterCleanup(func() {
			executionOrder = append(executionOrder, "second")
		})
		suite.RegisterCleanup(func() {
			executionOrder = append(executionOrder, "third")
		})

		suite.Cleanup()

		// Cleanup executes in LIFO order (reverse registration)
		Expect(executionOrder).To(Equal([]string{"third", "second", "first"}))
	})
})
