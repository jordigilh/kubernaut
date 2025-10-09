//go:build integration
// +build integration

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

package shared

import (
	"math/rand"
	"time"
)

// PredefinedScenarios contains realistic testing scenarios
var PredefinedScenarios = map[string]RealisticScenario{
	"high_confidence_ops": {
		Name:              "High Confidence Operations",
		ConfidenceRange:   [2]float64{0.85, 0.95},
		LatencyMultiplier: 1.0,
		ErrorRate:         0.01, // 1% error rate
		ConsistencyLevel:  0.95, // Very consistent
	},
	"uncertain_scenarios": {
		Name:              "Uncertain Edge Cases",
		ConfidenceRange:   [2]float64{0.3, 0.6},
		LatencyMultiplier: 1.5,  // Takes longer to analyze
		ErrorRate:         0.05, // 5% error rate
		ConsistencyLevel:  0.7,  // Less consistent
	},
	"learning_phase": {
		Name:              "System Learning Phase",
		ConfidenceRange:   [2]float64{0.4, 0.8},
		LatencyMultiplier: 1.2,
		ErrorRate:         0.03,
		ConsistencyLevel:  0.6, // Improving over time
	},
	"production_stable": {
		Name:              "Production Stable",
		ConfidenceRange:   [2]float64{0.7, 0.9},
		LatencyMultiplier: 0.8,   // Well-optimized responses
		ErrorRate:         0.005, // Very low error rate
		ConsistencyLevel:  0.95,  // Very consistent
	},
	"network_issues": {
		Name:              "Network Instability",
		ConfidenceRange:   [2]float64{0.6, 0.8},
		LatencyMultiplier: 2.0,  // Network delays
		ErrorRate:         0.15, // High error rate due to network
		ConsistencyLevel:  0.5,  // Inconsistent due to retries
	},
}

// ConfigureForScenario applies a realistic testing scenario to the fake client
func (f *TestSLMClient) ConfigureForScenario(scenario RealisticScenario) {
	// Set confidence patterns based on scenario
	f.responseVariation = 1.0 - scenario.ConsistencyLevel

	// Adjust latency
	baseLatency := 50 * time.Millisecond
	f.latency = time.Duration(float64(baseLatency) * scenario.LatencyMultiplier)

	// Configure network simulation for realistic scenarios
	if scenario.ErrorRate > 0.1 {
		f.networkSimulation.Enabled = true
		f.networkSimulation.FailureRate = scenario.ErrorRate * 0.5 // Split between failures and timeouts
		f.networkSimulation.TimeoutRate = scenario.ErrorRate * 0.5
		f.networkSimulation.LatencyMin = f.latency
		f.networkSimulation.LatencyMax = time.Duration(float64(f.latency) * 2.0)
	}

	// Store scenario-specific confidence range
	for alertType := range f.confidencePatterns {
		min := scenario.ConfidenceRange[0]
		max := scenario.ConfidenceRange[1]
		f.confidencePatterns[alertType] = min + rand.Float64()*(max-min)
	}
}

// EnableNetworkSimulation turns on network condition simulation
func (f *TestSLMClient) EnableNetworkSimulation(latencyMin, latencyMax time.Duration, failureRate, timeoutRate float64) {
	f.networkSimulation = NetworkSimulation{
		Enabled:     true,
		LatencyMin:  latencyMin,
		LatencyMax:  latencyMax,
		FailureRate: failureRate,
		TimeoutRate: timeoutRate,
		RetryAfter:  5 * time.Second,
	}
}

// DisableNetworkSimulation turns off network simulation for fast testing
func (f *TestSLMClient) DisableNetworkSimulation() {
	f.networkSimulation.Enabled = false
}

// SetModelAvailability configures which models are available for testing
func (f *TestSLMClient) SetModelAvailability(modelName string, available bool) {
	if f.modelAvailability == nil {
		f.modelAvailability = make(map[string]bool)
	}
	f.modelAvailability[modelName] = available
}

// EnableMemory turns on/off the client's memory of previous interactions
func (f *TestSLMClient) EnableMemory(enabled bool) {
	f.memoryEnabled = enabled
	if !enabled {
		f.previousAlerts = make(map[string]time.Time)
	}
}

// SetResponseVariation sets how much responses should vary (0.0 = consistent, 1.0 = highly variable)
func (f *TestSLMClient) SetResponseVariation(variation float64) {
	if variation < 0.0 {
		variation = 0.0
	}
	if variation > 1.0 {
		variation = 1.0
	}
	f.responseVariation = variation
}

// GetScenarioNames returns available predefined scenarios
func GetScenarioNames() []string {
	names := make([]string, 0, len(PredefinedScenarios))
	for name := range PredefinedScenarios {
		names = append(names, name)
	}
	return names
}

// GetScenario returns a predefined scenario by name
func GetScenario(name string) (RealisticScenario, bool) {
	scenario, exists := PredefinedScenarios[name]
	return scenario, exists
}
