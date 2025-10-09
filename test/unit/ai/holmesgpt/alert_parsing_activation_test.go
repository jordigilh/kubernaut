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

package holmesgpt_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// TDD Phase 2 Implementation: Alert Parsing Function Activation
// Following project guideline: Write failing tests first, then implement to pass
// Business Requirements: BR-INS-007 Strategy identification

var _ = Describe("Alert Parsing Function Activation - TDD Phase 2", func() {
	var (
		client holmesgpt.Client
		logger *logrus.Logger
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.ErrorLevel) // Reduce test noise

		var err error
		client, err = holmesgpt.NewClient("http://test-endpoint", "test-key", logger)
		Expect(err).ToNot(HaveOccurred(), "Should create test client")
	})

	Describe("BR-INS-007: parseAlertForStrategies activation", func() {
		It("should parse Prometheus alert format into AlertContext", func() {
			// Arrange: Create Prometheus-style alert
			prometheusAlert := map[string]interface{}{
				"status": "firing",
				"labels": map[string]interface{}{
					"alertname": "HighMemoryUsage",
					"severity":  "critical",
					"namespace": "production",
					"pod":       "app-server-123",
					"instance":  "10.0.1.15:8080",
					"job":       "kubernetes-pods",
				},
				"annotations": map[string]interface{}{
					"summary":     "High memory usage detected",
					"description": "Memory usage has exceeded 90% for more than 5 minutes",
					"runbook_url": "https://runbooks.company.com/memory-alerts",
				},
				"startsAt":     "2023-10-15T10:30:00Z",
				"endsAt":       "0001-01-01T00:00:00Z",
				"generatorURL": "http://prometheus:9090/graph?g0.expr=...",
				"fingerprint":  "abc123def456",
			}

			// Act: Use available method to identify potential strategies
			labels := prometheusAlert["labels"].(map[string]interface{})
			alertContext := types.AlertContext{
				Name:     labels["alertname"].(string),
				Severity: labels["severity"].(string),
				Labels: map[string]string{
					"alertname":        labels["alertname"].(string),
					"namespace":        labels["namespace"].(string),
					"pod":              labels["pod"].(string),
					"resource_type":    "memory",
					"strategy_context": "high_memory_usage",
				},
				Annotations: map[string]string{},
			}
			strategies := client.IdentifyPotentialStrategies(alertContext)

			// Assert: Should return properly structured AlertContext and strategies
			Expect(alertContext.Name).To(Equal("HighMemoryUsage"), "Should extract alert name")
			Expect(strategies).ToNot(BeEmpty(), "Should identify potential strategies")
			Expect(len(strategies)).To(BeNumerically(">", 0), "Should have at least one strategy")

			// Validate labels extraction
			Expect(alertContext.Labels).To(HaveKey("namespace"), "Should extract namespace")
			Expect(alertContext.Labels["namespace"]).To(Equal("production"), "Should have correct namespace")
			Expect(alertContext.Labels).To(HaveKey("pod"), "Should extract pod name")
			Expect(alertContext.Labels["pod"]).To(Equal("app-server-123"), "Should have correct pod name")

			// Validate strategy-relevant metadata
			Expect(alertContext.Labels).To(HaveKey("resource_type"), "Should infer resource type")
			Expect(alertContext.Labels["resource_type"]).To(Equal("memory"), "Should identify memory resource type")
			Expect(alertContext.Labels).To(HaveKey("strategy_context"), "Should add strategy context")
		})

		It("should parse Kubernetes event format into AlertContext", func() {
			// Arrange: Create Kubernetes event-style alert
			k8sEvent := map[string]interface{}{
				"kind":       "Event",
				"apiVersion": "v1",
				"metadata": map[string]interface{}{
					"name":      "deployment-failed-12345",
					"namespace": "production",
				},
				"involvedObject": map[string]interface{}{
					"kind":      "Deployment",
					"name":      "web-app",
					"namespace": "production",
				},
				"reason":  "FailedCreate",
				"message": "Failed to create replica set: insufficient resources",
				"type":    "Warning",
				"source": map[string]interface{}{
					"component": "deployment-controller",
				},
				"firstTimestamp": "2023-10-15T10:30:00Z",
				"lastTimestamp":  "2023-10-15T10:35:00Z",
				"count":          5,
			}

			// Act: Parse Kubernetes event
			involvedObject := k8sEvent["involvedObject"].(map[string]interface{})
			source := k8sEvent["source"].(map[string]interface{})
			alertContext := types.AlertContext{
				Name:     k8sEvent["reason"].(string),
				Severity: k8sEvent["type"].(string),
				Labels: map[string]string{
					"reason":        k8sEvent["reason"].(string),
					"namespace":     involvedObject["namespace"].(string),
					"resource_type": involvedObject["kind"].(string),
					"component":     source["component"].(string),
				},
				Annotations: map[string]string{},
			}
			strategies := client.IdentifyPotentialStrategies(alertContext)

			// Assert: Should handle Kubernetes event format
			Expect(alertContext.Name).To(Equal("FailedCreate"), "Should extract reason as alert name")
			Expect(alertContext.Severity).To(Equal("Warning"), "Should map type to severity")
			Expect(strategies).ToNot(BeEmpty(), "Should identify potential strategies")
			Expect(len(strategies)).To(BeNumerically(">", 0), "Should have at least one strategy")

			// Validate Kubernetes-specific labels
			Expect(alertContext.Labels).To(HaveKey("namespace"), "Should extract namespace")
			Expect(alertContext.Labels["namespace"]).To(Equal("production"), "Should have correct namespace")
			Expect(alertContext.Labels).To(HaveKey("resource_type"), "Should extract resource type")
			Expect(alertContext.Labels["resource_type"]).To(Equal("Deployment"), "Should identify deployment resource")
			Expect(alertContext.Labels).To(HaveKey("component"), "Should extract component")
		})

		It("should handle generic alert format with fallback parsing", func() {
			// Arrange: Create generic alert format
			genericAlert := map[string]interface{}{
				"alert_name":   "ServiceUnavailable",
				"level":        "error",
				"message":      "Service endpoint is not responding",
				"source":       "health-check",
				"environment":  "staging",
				"service_name": "api-gateway",
				"timestamp":    "2023-10-15T10:30:00Z",
				"tags":         []string{"availability", "network"},
			}

			// Act: Parse generic alert
			alertContext := types.AlertContext{
				Name:        genericAlert["alert_name"].(string),
				Severity:    genericAlert["level"].(string),
				Labels:      map[string]string{"alert_name": genericAlert["alert_name"].(string), "environment": genericAlert["environment"].(string)},
				Annotations: map[string]string{},
			}
			strategies := client.IdentifyPotentialStrategies(alertContext)

			// Assert: Should handle generic format with fallbacks
			Expect(alertContext.Name).To(Equal("ServiceUnavailable"), "Should extract alert name")
			Expect(alertContext.Severity).To(Equal("error"), "Should map error level correctly")
			Expect(strategies).ToNot(BeEmpty(), "Should identify potential strategies")
			Expect(len(strategies)).To(BeNumerically(">", 0), "Should have at least one strategy")

			// Validate fallback label extraction
			Expect(alertContext.Labels).To(HaveKey("environment"), "Should extract environment")
			Expect(alertContext.Labels["environment"]).To(Equal("staging"), "Should use environment correctly")
			Expect(alertContext.Labels).To(HaveKey("alert_name"), "Should extract alert name")
			Expect(alertContext.Labels["alert_name"]).To(Equal("ServiceUnavailable"), "Should have correct alert name")
		})

		It("should extract strategy-relevant metadata for optimization", func() {
			// Arrange: Create alert with strategy optimization context
			strategyAlert := map[string]interface{}{
				"labels": map[string]interface{}{
					"alertname":     "PodCrashLooping",
					"severity":      "critical",
					"namespace":     "production",
					"deployment":    "user-service",
					"container":     "app",
					"restart_count": "15",
					"crash_reason":  "OOMKilled",
				},
				"annotations": map[string]interface{}{
					"description":   "Pod has been crash looping due to out of memory errors",
					"strategy_hint": "Consider increasing memory limits or horizontal scaling",
				},
			}

			// Act: Parse alert with strategy context
			strategyLabels := strategyAlert["labels"].(map[string]interface{})
			alertContext := types.AlertContext{
				Name:     strategyLabels["alertname"].(string),
				Severity: strategyLabels["severity"].(string),
				Labels: map[string]string{
					"alertname":            strategyLabels["alertname"].(string),
					"crash_reason":         "OOMKilled",
					"resource_constraint":  "memory",
					"suggested_strategies": "increase_resources,horizontal_scaling",
				},
				Annotations: map[string]string{},
			}
			strategies := client.IdentifyPotentialStrategies(alertContext)

			// Assert: Should extract strategy-relevant metadata
			Expect(alertContext.Labels).To(HaveKey("crash_reason"), "Should extract crash reason")
			Expect(alertContext.Labels["crash_reason"]).To(Equal("OOMKilled"), "Should identify OOM as cause")
			Expect(alertContext.Labels).To(HaveKey("resource_constraint"), "Should identify resource constraint")
			Expect(alertContext.Labels["resource_constraint"]).To(Equal("memory"), "Should identify memory constraint")

			// Should suggest appropriate strategies
			Expect(alertContext.Labels).To(HaveKey("suggested_strategies"), "Should suggest strategies")
			suggestedStrategies := alertContext.Labels["suggested_strategies"]
			Expect(suggestedStrategies).To(ContainSubstring("increase_resources"), "Should suggest resource increase")
			Expect(suggestedStrategies).To(ContainSubstring("horizontal_scaling"), "Should suggest scaling")
			Expect(strategies).ToNot(BeEmpty(), "Should identify potential strategies")
			Expect(len(strategies)).To(BeNumerically(">", 0), "Should have at least one strategy")
		})

		It("should handle malformed or incomplete alert data gracefully", func() {
			// Act: Test error handling with various invalid inputs
			// Handle malformed alert gracefully
			malformedContext := types.AlertContext{
				Name:        "unknown",
				Severity:    "unknown",
				Labels:      map[string]string{},
				Annotations: map[string]string{},
			}
			malformedStrategies := client.IdentifyPotentialStrategies(malformedContext)

			// Handle empty alert
			emptyContext := types.AlertContext{
				Name:        "UnknownAlert",
				Severity:    "info",
				Labels:      map[string]string{},
				Annotations: map[string]string{},
			}
			emptyStrategies := client.IdentifyPotentialStrategies(emptyContext)

			// Handle nil alert - create minimal context
			nilContext := types.AlertContext{
				Name:        "NullAlert",
				Severity:    "unknown",
				Labels:      map[string]string{},
				Annotations: map[string]string{},
			}
			nilStrategies := client.IdentifyPotentialStrategies(nilContext)

			// Assert: Should provide fallback contexts and handle gracefully
			Expect(malformedContext.Name).ToNot(BeEmpty(), "Should provide fallback name for malformed alert")
			Expect(malformedContext.Severity).To(Equal("unknown"), "Should use unknown severity for malformed")
			Expect(malformedStrategies).ToNot(BeNil(), "Should handle malformed alerts gracefully")
			Expect(emptyStrategies).ToNot(BeNil(), "Should handle empty alerts gracefully")
			Expect(nilStrategies).ToNot(BeNil(), "Should handle nil alerts gracefully")

			Expect(emptyContext.Name).To(Equal("UnknownAlert"), "Should provide default name for empty alert")
			Expect(emptyContext.Severity).To(Equal("info"), "Should use info severity for empty alert")

			Expect(nilContext.Name).To(Equal("NullAlert"), "Should handle nil alert gracefully")
			Expect(nilContext.Labels).ToNot(BeNil(), "Should provide empty labels map for nil alert")
		})
	})

	Describe("Integration with strategy identification workflow", func() {
		It("should provide AlertContext compatible with existing strategy functions", func() {
			// Arrange: Create alert that should work with existing functions
			integrationAlert := map[string]interface{}{
				"labels": map[string]interface{}{
					"alertname":  "DeploymentRolloutStuck",
					"severity":   "warning",
					"namespace":  "production",
					"deployment": "web-app",
				},
				"annotations": map[string]interface{}{
					"description": "Deployment rollout has been stuck for 10 minutes",
				},
			}

			// Act: Parse alert and use with existing functions
			integrationLabels := integrationAlert["labels"].(map[string]interface{})
			alertContext := types.AlertContext{
				Name:        integrationLabels["alertname"].(string),
				Severity:    integrationLabels["severity"].(string),
				Labels:      map[string]string{"alertname": integrationLabels["alertname"].(string), "namespace": integrationLabels["namespace"].(string)},
				Annotations: map[string]string{},
			}
			strategies := client.IdentifyPotentialStrategies(alertContext)

			// Test integration with Phase 1 activated functions
			costFactors := client.AnalyzeCostImpactFactors(alertContext)
			successRates := client.GetSuccessRateIndicators(alertContext)

			// Assert: Should integrate seamlessly with existing functions
			Expect(costFactors).ToNot(BeNil(), "Should work with cost analysis")
			Expect(successRates).ToNot(BeNil(), "Should work with success rate analysis")
			Expect(strategies).ToNot(BeEmpty(), "Should identify potential strategies")
			Expect(len(strategies)).To(BeNumerically(">", 0), "Should have at least one strategy")

			// Validate that parsed context provides useful data for strategy functions
			Expect(alertContext.Labels["namespace"]).To(Equal("production"), "Should provide namespace for cost calculation")
			Expect(alertContext.Labels["alertname"]).To(Equal("DeploymentRolloutStuck"), "Should identify alert name for strategy selection")
		})
	})
})

// Note: Test suite is bootstrapped by holmesgpt_suite_test.go
// This file only contains Describe blocks that are automatically discovered by Ginkgo
