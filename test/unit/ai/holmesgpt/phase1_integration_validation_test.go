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

package holmesgpt

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Phase 1 TDD Integration Validation Tests
// Simple validation tests to ensure activated functions work correctly

func TestPhase1ActivationValidation(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	client, err := holmesgpt.NewClient("http://test-endpoint", "test-key", logger)
	require.NoError(t, err, "Should create test client")

	t.Run("AnalyzeCostImpactFactors_BasicFunctionality", func(t *testing.T) {
		alertContext := types.AlertContext{
			Name:        "TestAlert",
			Severity:    "critical",
			Description: "Test alert for cost analysis",
			Labels: map[string]string{
				"namespace":     "production",
				"resource_type": "memory",
				"cost_category": "compute",
			},
		}

		costFactors := client.AnalyzeCostImpactFactors(alertContext)

		// Validate basic structure
		assert.NotNil(t, costFactors, "Should return cost factors")
		assert.Contains(t, costFactors, "resource_cost_per_minute", "Should include resource cost")
		assert.Contains(t, costFactors, "business_impact_cost", "Should include business impact")
		assert.Contains(t, costFactors, "resolution_effort_cost", "Should include resolution effort")
		assert.Contains(t, costFactors, "optimization_potential", "Should include optimization potential")

		// Validate value types and ranges
		resourceCost, ok := costFactors["resource_cost_per_minute"].(float64)
		assert.True(t, ok, "Resource cost should be float64")
		assert.Greater(t, resourceCost, 0.0, "Resource cost should be positive")

		optimizationPotential, ok := costFactors["optimization_potential"].(float64)
		assert.True(t, ok, "Optimization potential should be float64")
		assert.GreaterOrEqual(t, optimizationPotential, 0.0, "Optimization potential should be non-negative")
		assert.LessOrEqual(t, optimizationPotential, 1.0, "Optimization potential should be <= 1.0")
	})

	t.Run("GetSuccessRateIndicators_BasicFunctionality", func(t *testing.T) {
		alertContext := types.AlertContext{
			Name:        "TestAlert",
			Severity:    "warning",
			Description: "Test alert for success rate analysis",
			Labels: map[string]string{
				"namespace":     "production",
				"resource_type": "deployment",
			},
		}

		successRates := client.GetSuccessRateIndicators(alertContext)

		// Validate basic structure
		assert.NotNil(t, successRates, "Should return success rates")
		assert.Greater(t, len(successRates), 0, "Should provide at least one strategy")

		// Validate BR-INS-007: Success rates should be >80% for recommended strategies
		for strategy, rate := range successRates {
			assert.GreaterOrEqual(t, rate, 0.8, "Strategy %s should meet >80%% success rate requirement", strategy)
			assert.LessOrEqual(t, rate, 1.0, "Strategy %s should have success rate <= 100%%", strategy)
		}

		// Should include key strategies
		assert.Contains(t, successRates, "rolling_deployment", "Should include rolling deployment strategy")
		assert.Contains(t, successRates, "horizontal_scaling", "Should include horizontal scaling strategy")
	})

	t.Run("Integration_CostAndSuccessRate", func(t *testing.T) {
		alertContext := types.AlertContext{
			Name:        "IntegrationTest",
			Severity:    "critical",
			Description: "Integration test for both functions",
			Labels: map[string]string{
				"namespace":           "production",
				"resource_type":       "memory",
				"cost_category":       "compute",
				"historical_tracking": "extensive",
			},
		}

		// Test both functions work together
		costFactors := client.AnalyzeCostImpactFactors(alertContext)
		successRates := client.GetSuccessRateIndicators(alertContext)

		// Both should return valid data
		assert.NotNil(t, costFactors, "Cost analysis should work")
		assert.NotNil(t, successRates, "Success rate analysis should work")

		// Validate integration compatibility
		assert.Greater(t, len(costFactors), 3, "Should provide comprehensive cost analysis")
		assert.Greater(t, len(successRates), 2, "Should provide multiple strategy options")

		// Cost category should be determined correctly
		costCategory, ok := costFactors["cost_category"].(string)
		assert.True(t, ok, "Cost category should be string")
		assert.Equal(t, "compute", costCategory, "Should correctly identify cost category")
	})
}

func TestPhase1EdgeCases(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	client, err := holmesgpt.NewClient("http://test-endpoint", "test-key", logger)
	require.NoError(t, err, "Should create test client")

	t.Run("EmptyAlertContext", func(t *testing.T) {
		emptyContext := types.AlertContext{}

		// Should handle empty context gracefully
		costFactors := client.AnalyzeCostImpactFactors(emptyContext)
		successRates := client.GetSuccessRateIndicators(emptyContext)

		assert.NotNil(t, costFactors, "Should handle empty context for cost analysis")
		assert.NotNil(t, successRates, "Should handle empty context for success rates")
		assert.Greater(t, len(successRates), 0, "Should provide fallback strategies")
	})

	t.Run("UnknownSeverity", func(t *testing.T) {
		unknownContext := types.AlertContext{
			Name:     "UnknownAlert",
			Severity: "unknown",
		}

		costFactors := client.AnalyzeCostImpactFactors(unknownContext)
		successRates := client.GetSuccessRateIndicators(unknownContext)

		assert.NotNil(t, costFactors, "Should handle unknown severity")
		assert.NotNil(t, successRates, "Should handle unknown severity")
	})
}
