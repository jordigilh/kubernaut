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

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
	"github.com/jordigilh/kubernaut/pkg/api/server"
	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// TestDynamicToolsetEndToEndIntegration tests the complete integration flow
// Business Requirement: BR-HOLMES-025 - Runtime toolset management API
func TestDynamicToolsetEndToEndIntegration(t *testing.T) {
	// Setup test logger
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel) // Reduce noise

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Create fake Kubernetes client with test services
	fakeClient := fake.NewSimpleClientset()

	// Create Prometheus service
	prometheusService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "prometheus-server",
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

	_, err := fakeClient.CoreV1().Services("monitoring").Create(ctx, prometheusService, metav1.CreateOptions{})
	require.NoError(t, err, "Failed to create Prometheus service")

	// Create Grafana service
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
				{Name: "service", Port: 3000},
			},
		},
	}

	_, err = fakeClient.CoreV1().Services("monitoring").Create(ctx, grafanaService, metav1.CreateOptions{})
	require.NoError(t, err, "Failed to create Grafana service")

	// 2. Setup Service Discovery with test configuration
	serviceDiscoveryConfig := &k8s.ServiceDiscoveryConfig{
		DiscoveryInterval:   100 * time.Millisecond,
		CacheTTL:            5 * time.Minute,
		HealthCheckInterval: 1 * time.Second,
		Enabled:             true,
		Namespaces:          []string{"monitoring"},
		ServicePatterns:     k8s.GetDefaultServicePatterns(),
	}

	// 3. Create Service Integration
	serviceIntegration, err := holmesgpt.NewServiceIntegration(fakeClient, serviceDiscoveryConfig, log)
	require.NoError(t, err, "Failed to create service integration")

	// 4. Start Service Integration
	err = serviceIntegration.Start(ctx)
	require.NoError(t, err, "Failed to start service integration")
	defer serviceIntegration.Stop()

	// 5. Setup AI Service Integrator
	appConfig := &config.Config{}
	aiIntegrator := engine.NewAIServiceIntegrator(
		appConfig,
		nil, // llmClient - nil for test environment
		nil, // holmesClient - nil for test environment
		nil, // vectorDB - nil for test environment
		nil, // metricsClient - nil for test environment
		log,
	)

	// 6. Create Context API Server with Service Integration
	contextAPIConfig := server.ContextAPIConfig{
		Host:    "localhost",
		Port:    8091,
		Timeout: 10 * time.Second,
	}

	// Architecture: Context API serves data TO HolmesGPT (Python service), no direct client needed
	contextAPIServer := server.NewContextAPIServer(contextAPIConfig, aiIntegrator, serviceIntegration, log)

	// 7. Start Context API Server
	go func() {
		if err := contextAPIServer.Start(); err != nil && err != http.ErrServerClosed {
			t.Errorf("Context API Server failed: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(1 * time.Second)

	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := contextAPIServer.Stop(shutdownCtx); err != nil {
			fmt.Printf("Warning: Failed to stop context API server: %v\n", err)
		}
	}()

	// 8. Wait for initial toolset generation
	time.Sleep(2 * time.Second)

	// 9. Test Cases

	t.Run("Service Discovery Integration", func(t *testing.T) {
		// Test service discovery finds services
		discoveredServices := serviceIntegration.GetServiceDiscoveryStats()
		assert.GreaterOrEqual(t, discoveredServices.TotalServices, 2, "Should discover at least 2 services (Prometheus + Grafana)")

		// Test service types
		expectedTypes := []string{"prometheus", "grafana"}
		for _, expectedType := range expectedTypes {
			found := false
			for serviceType := range discoveredServices.ServiceTypes {
				if serviceType == expectedType {
					found = true
					break
				}
			}
			assert.True(t, found, "Should discover %s service type", expectedType)
		}
	})

	t.Run("Dynamic Toolset Generation", func(t *testing.T) {
		// Test toolset generation
		toolsets := serviceIntegration.GetAvailableToolsets()
		assert.GreaterOrEqual(t, len(toolsets), 4, "Should have at least 4 toolsets (kubernetes, internet, prometheus, grafana)")

		// Check for baseline toolsets
		hasKubernetes := false
		hasInternet := false
		hasPrometheus := false
		hasGrafana := false

		for _, toolset := range toolsets {
			switch toolset.ServiceType {
			case "kubernetes":
				hasKubernetes = true
			case "internet":
				hasInternet = true
			case "prometheus":
				hasPrometheus = true
			case "grafana":
				hasGrafana = true
			}
		}

		assert.True(t, hasKubernetes, "Should have kubernetes baseline toolset")
		assert.True(t, hasInternet, "Should have internet baseline toolset")
		assert.True(t, hasPrometheus, "Should have generated prometheus toolset")
		assert.True(t, hasGrafana, "Should have generated grafana toolset")
	})

	t.Run("API Endpoints Integration", func(t *testing.T) {
		// Test GET /api/v1/toolsets
		resp, err := http.Get("http://localhost:8091/api/v1/toolsets")
		require.NoError(t, err, "Failed to call toolsets API")
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Warning: Failed to close response body: %v", err)
			}
		}()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Toolsets API should return 200")

		var toolsetsResponse map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&toolsetsResponse)
		require.NoError(t, err, "Failed to decode toolsets response")

		toolsets, ok := toolsetsResponse["toolsets"].([]interface{})
		assert.True(t, ok, "Response should have toolsets array")
		assert.GreaterOrEqual(t, len(toolsets), 4, "Should return at least 4 toolsets")

		// Test GET /api/v1/toolsets/stats
		resp, err = http.Get("http://localhost:8091/api/v1/toolsets/stats")
		require.NoError(t, err, "Failed to call toolset stats API")
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Warning: Failed to close response body: %v", err)
			}
		}()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Toolset stats API should return 200")

		var statsResponse map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&statsResponse)
		require.NoError(t, err, "Failed to decode stats response")

		toolsetStats, ok := statsResponse["toolset_stats"].(map[string]interface{})
		assert.True(t, ok, "Response should have toolset_stats")

		totalToolsets, ok := toolsetStats["total_toolsets"].(float64)
		assert.True(t, ok, "Should have total_toolsets count")
		assert.GreaterOrEqual(t, int(totalToolsets), 4, "Should report at least 4 toolsets")

		// Test GET /api/v1/service-discovery
		resp, err = http.Get("http://localhost:8091/api/v1/service-discovery")
		require.NoError(t, err, "Failed to call service discovery API")
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Warning: Failed to close response body: %v", err)
			}
		}()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Service discovery API should return 200")

		var discoveryResponse map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&discoveryResponse)
		require.NoError(t, err, "Failed to decode discovery response")

		health, ok := discoveryResponse["health"].(map[string]interface{})
		assert.True(t, ok, "Response should have health status")

		healthy, ok := health["healthy"].(bool)
		assert.True(t, ok && healthy, "Service discovery should be healthy")
	})

	t.Run("Toolset Refresh Integration", func(t *testing.T) {
		// Test POST /api/v1/toolsets/refresh
		resp, err := http.Post("http://localhost:8091/api/v1/toolsets/refresh", "application/json", nil)
		require.NoError(t, err, "Failed to call refresh API")
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Warning: Failed to close response body: %v", err)
			}
		}()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Refresh API should return 200")

		var refreshResponse map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&refreshResponse)
		require.NoError(t, err, "Failed to decode refresh response")

		status, ok := refreshResponse["status"].(string)
		assert.True(t, ok, "Response should have status")
		assert.Equal(t, "success", status, "Refresh should succeed")

		totalToolsets, ok := refreshResponse["total_toolsets"].(float64)
		assert.True(t, ok, "Response should have total_toolsets count")
		assert.GreaterOrEqual(t, int(totalToolsets), 4, "Should maintain toolset count after refresh")
	})

	t.Run("Production Patterns Integration", func(t *testing.T) {
		// Verify we're using production patterns (not test patterns)
		patterns := k8s.GetDefaultServicePatterns()

		// Should have all 5 production patterns
		assert.Len(t, patterns, 5, "Should have 5 production service patterns")

		// Check that all expected patterns exist
		expectedPatterns := []string{"prometheus", "grafana", "jaeger", "elasticsearch", "custom"}
		for _, expected := range expectedPatterns {
			assert.Contains(t, patterns, expected, "Should include %s pattern", expected)
		}

		// Verify elasticsearch pattern has production capabilities
		elasticsearchPattern, exists := patterns["elasticsearch"]
		assert.True(t, exists, "Elasticsearch pattern should exist")
		assert.True(t, elasticsearchPattern.Enabled, "Elasticsearch pattern should be enabled")
		assert.Equal(t, 50, elasticsearchPattern.Priority, "Elasticsearch should have priority 50")
		assert.Contains(t, elasticsearchPattern.Capabilities, "search_logs", "Should have log search capability")
		assert.Contains(t, elasticsearchPattern.Capabilities, "full_text_search", "Should have full text search capability")

		// Verify service discovery is using production patterns
		toolsets := serviceIntegration.GetAvailableToolsets()

		// Should have baseline toolsets plus discovered services
		assert.GreaterOrEqual(t, len(toolsets), 4, "Should have baseline + discovered toolsets")

		// Verify baseline toolsets exist
		toolsetTypes := make(map[string]bool)
		for _, toolset := range toolsets {
			toolsetTypes[toolset.ServiceType] = true
		}

		assert.True(t, toolsetTypes["kubernetes"], "Should have kubernetes baseline toolset")
		assert.True(t, toolsetTypes["internet"], "Should have internet baseline toolset")
		assert.True(t, toolsetTypes["prometheus"], "Should have prometheus toolset for discovered service")
		assert.True(t, toolsetTypes["grafana"], "Should have grafana toolset for discovered service")

		// Verify API reflects production configuration
		resp, err := http.Get("http://localhost:8091/api/v1/toolsets")
		require.NoError(t, err, "Failed to call toolsets API")
		defer func() {
			if err := resp.Body.Close(); err != nil {
				t.Logf("Warning: Failed to close response body: %v", err)
			}
		}()

		var toolsetsResponse map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&toolsetsResponse)
		require.NoError(t, err, "Failed to decode toolsets response")

		toolsets_api, ok := toolsetsResponse["toolsets"].([]interface{})
		assert.True(t, ok, "Response should have toolsets array")
		assert.GreaterOrEqual(t, len(toolsets_api), 4, "API should return production toolsets")
	})
}
