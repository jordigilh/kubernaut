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
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// setupConfidencePatterns configures realistic confidence patterns for different alert types
func (f *TestSLMClient) setupConfidencePatterns() {
	f.confidencePatterns = map[string]float64{
		"OOMKilled":               0.9,  // High confidence for memory issues
		"HighMemoryUsage":         0.85, // High confidence for known patterns
		"HighCPUUsage":            0.8,  // Good confidence for CPU issues
		"DiskSpaceLow":            0.95, // Very high confidence for storage
		"PodCrashLooping":         0.87, // High confidence for restart loops
		"ServiceDown":             0.9,  // High confidence for service failures
		"HighNetworkLatency":      0.7,  // Medium confidence for network issues
		"UnauthorizedAccess":      0.9,  // High confidence for security alerts (updated for test scenarios)
		"DatabaseConnectionsHigh": 0.8,
		"LoadBalancerError":       0.75,
		"SlowResponseTime":        0.65,
		"HighErrorRate":           0.82,
		// Test scenario specific patterns
		"obvious_storage_expansion":  0.9, // High confidence for obvious storage scenarios
		"obvious_security_threat":    0.9, // High confidence for security threats
		"ActiveSecurityThreat":       0.9, // High confidence for security threats
		"PVCNearFull":                0.9, // High confidence for storage alerts
		"storage":                    0.9, // Pattern matching for storage-related alerts
		"security":                   0.9, // Pattern matching for security-related alerts
		"malware":                    0.9, // Pattern matching for malware alerts
		"SystemInstability":          0.7, // High confidence for system instability
		"instability":                0.7, // Pattern matching for instability alerts
		"ResourceExhaustion":         0.8, // High confidence for resource exhaustion
		"resource":                   0.8, // Pattern matching for resource-related alerts
		"exhaustion":                 0.8, // Pattern matching for exhaustion scenarios
		"obvious_deployment_failure": 0.8, // High confidence for deployment failures
		"deployment":                 0.8, // Pattern matching for deployment alerts
		"failure":                    0.8, // Pattern matching for failure scenarios
		// Resource exhaustion patterns
		"MemoryExhaustion":         0.8,  // High confidence for memory exhaustion
		"DiskSpaceExhaustion":      0.8,  // High confidence for disk exhaustion
		"FileDescriptorExhaustion": 0.75, // High confidence for FD exhaustion
		"Exhaustion":               0.8,  // Pattern matching for exhaustion scenarios
		"SecurityIncident":         0.85, // High confidence for security incidents
		"default":                  0.5,  // Default for unknown alert types
	}
}

// generateAlertFingerprint creates a unique fingerprint for an alert
func (f *TestSLMClient) generateAlertFingerprint(alert types.Alert) string {
	fingerprint := fmt.Sprintf("%s-%s", alert.Name, alert.Namespace)
	// Add labels for uniqueness
	for k, v := range alert.Labels {
		fingerprint += fmt.Sprintf("-%s=%s", k, v)
	}
	hash := sha256.Sum256([]byte(fingerprint))
	return fmt.Sprintf("%x", hash)[:16] // Use first 16 chars of hash
}

// calculateComplexityDelay determines delay based on alert complexity
func (f *TestSLMClient) calculateComplexityDelay(alert types.Alert) time.Duration {
	if !f.complexityDelay {
		return 0
	}

	complexity := 1.0

	// More labels = more complexity
	complexity += float64(len(alert.Labels)) * 0.1

	// Certain alert types are inherently more complex
	complexAlertTypes := map[string]float64{
		"UnauthorizedAccess":      2.0,
		"HighNetworkLatency":      1.5,
		"DatabaseConnectionsHigh": 1.8,
		"SlowResponseTime":        1.3,
	}

	if multiplier, exists := complexAlertTypes[alert.Name]; exists {
		complexity *= multiplier
	}

	// Cap complexity multiplier
	if complexity > 3.0 {
		complexity = 3.0
	}

	return time.Duration(float64(f.latency) * complexity)
}

// simulateNetworkConditions applies network simulation effects
func (f *TestSLMClient) simulateNetworkConditions() error {
	if !f.networkSimulation.Enabled {
		return nil
	}

	// Simulate request failure
	if rand.Float64() < f.networkSimulation.FailureRate {
		return errors.New("network error: connection refused")
	}

	// Simulate timeout
	if rand.Float64() < f.networkSimulation.TimeoutRate {
		return errors.New("network error: request timeout")
	}

	// Add network latency
	if f.networkSimulation.LatencyMax > f.networkSimulation.LatencyMin {
		latencyRange := f.networkSimulation.LatencyMax - f.networkSimulation.LatencyMin
		additionalLatency := time.Duration(rand.Float64() * float64(latencyRange))
		time.Sleep(f.networkSimulation.LatencyMin + additionalLatency)
	}

	return nil
}

// checkRateLimit verifies if the request should be rate limited
func (f *TestSLMClient) checkRateLimit() error {
	now := time.Now()
	windowStart := now.Truncate(f.rateLimitWindow)

	// Clean old entries
	for timestamp := range f.rateLimitCounts {
		if timestamp.Before(windowStart) {
			delete(f.rateLimitCounts, timestamp)
		}
	}

	// Check current window count
	currentCount := f.rateLimitCounts[windowStart]
	if currentCount >= f.maxCalls {
		return fmt.Errorf("rate limit exceeded: %d calls in %v", currentCount, f.rateLimitWindow)
	}

	// Increment counter
	f.rateLimitCounts[windowStart] = currentCount + 1
	return nil
}

// getRealisticConfidence calculates a realistic confidence value
func (f *TestSLMClient) getRealisticConfidence(alert types.Alert, baseConfidence float64) float64 {
	confidence := baseConfidence

	// Apply variation
	if f.responseVariation > 0 {
		variation := (rand.Float64() - 0.5) * f.responseVariation
		confidence += variation
	}

	// Check if this is a repeated alert (memory effect)
	if f.memoryEnabled {
		fingerprint := f.generateAlertFingerprint(alert)
		if lastSeen, exists := f.previousAlerts[fingerprint]; exists {
			timeSinceLast := time.Since(lastSeen)

			// Recent alerts get higher confidence (learned behavior)
			if timeSinceLast < time.Hour {
				confidence += 0.1 // 10% boost for recent similar alerts
			} else if timeSinceLast < 24*time.Hour {
				confidence += 0.05 // 5% boost for alerts seen today
			}
		}

		// Remember this alert
		f.previousAlerts[fingerprint] = time.Now()
	}

	// Ensure confidence stays within bounds
	if confidence < 0.1 {
		confidence = 0.1
	} else if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// selectAvailableModel chooses an available model for the request
func (f *TestSLMClient) selectAvailableModel() (string, error) {
	availableModels := make([]string, 0)
	for model, available := range f.modelAvailability {
		if available {
			availableModels = append(availableModels, model)
		}
	}

	if len(availableModels) == 0 {
		return "", errors.New("no models currently available")
	}

	// Select a random available model
	return availableModels[rand.Intn(len(availableModels))], nil
}

// recordCall logs a call in the history with detailed information
func (f *TestSLMClient) recordCall(method string, alert *types.Alert, prompt string, duration time.Duration, success bool, errorType string, confidence float64, modelUsed string) {
	call := TestSLMCall{
		Method:     method,
		Alert:      alert,
		Prompt:     prompt,
		Timestamp:  time.Now(),
		Duration:   duration,
		Success:    success,
		ErrorType:  errorType,
		Confidence: confidence,
		ModelUsed:  modelUsed,
	}

	f.callHistory = append(f.callHistory, call)
	f.callCount++
}

// EnhancedAnalyzeAlert provides the enhanced implementation with realistic behavior
func (f *TestSLMClient) EnhancedAnalyzeAlert(ctx context.Context, alert types.Alert) (*types.ActionRecommendation, error) {
	startTime := time.Now()
	var confidence float64
	var modelUsed string

	// Check rate limiting
	if err := f.checkRateLimit(); err != nil {
		f.recordCall("AnalyzeAlert", &alert, "", time.Since(startTime), false, "rate_limit", 0, "")
		return nil, err
	}

	// Simulate network conditions
	if err := f.simulateNetworkConditions(); err != nil {
		f.recordCall("AnalyzeAlert", &alert, "", time.Since(startTime), false, "network", 0, "")
		return nil, err
	}

	// Check for forced errors
	if f.simulateError {
		f.recordCall("AnalyzeAlert", &alert, "", time.Since(startTime), false, "forced", 0, "")
		return nil, errors.New(f.errorMessage)
	}

	// Select an available model
	var err error
	modelUsed, err = f.selectAvailableModel()
	if err != nil {
		f.recordCall("AnalyzeAlert", &alert, "", time.Since(startTime), false, "no_model", 0, "")
		return nil, err
	}

	// Calculate complexity-based delay
	complexityDelay := f.calculateComplexityDelay(alert)
	time.Sleep(f.latency + complexityDelay)

	// Check if we have a pre-configured response
	alertKey := alert.Name // Use alert name as key for simplicity
	if response, exists := f.responses[alertKey]; exists {
		// Apply realistic confidence calculation
		baseConfidence := response.Confidence
		if patternConfidence, patternExists := f.confidencePatterns[alert.Name]; patternExists {
			baseConfidence = patternConfidence
		}

		confidence = f.getRealisticConfidence(alert, baseConfidence)

		// Create enhanced response
		enhancedResponse := &types.ActionRecommendation{
			Action:     response.Action,
			Confidence: confidence,
			Reasoning:  &types.ReasoningDetails{Summary: fmt.Sprintf("Enhanced realistic analysis for %s (model: %s)", alert.Name, modelUsed)},
		}

		f.recordCall("AnalyzeAlert", &alert, "", time.Since(startTime), true, "", confidence, modelUsed)
		return enhancedResponse, nil
	}

	// Generate default realistic response
	confidence = f.getRealisticConfidence(alert, f.confidencePatterns["default"])
	action := f.generateRealisticAction(alert)

	response := &types.ActionRecommendation{
		Action:     action,
		Confidence: confidence,
		Reasoning:  &types.ReasoningDetails{Summary: fmt.Sprintf("Generated realistic response for %s (model: %s)", alert.Name, modelUsed)},
	}

	f.recordCall("AnalyzeAlert", &alert, "", time.Since(startTime), true, "", confidence, modelUsed)
	return response, nil
}

// generateRealisticAction creates a realistic action based on alert characteristics
func (f *TestSLMClient) generateRealisticAction(alert types.Alert) string {
	// Prioritize actions based on alert type and severity
	highConfidenceActions := map[string]string{
		"OOMKilled":               "increase_resources",
		"HighMemoryUsage":         "increase_resources",
		"DiskSpaceLow":            "increase_resources",
		"PodCrashLooping":         "restart_pod",
		"ServiceDown":             "restart_pod",
		"DatabaseConnectionsHigh": "scale_deployment",
		"DeploymentFailed":        "rollback_deployment", // BR-2: Consistent action mapping
	}

	mediumConfidenceActions := map[string]string{
		"HighCPUUsage":       "scale_deployment",
		"HighNetworkLatency": "collect_diagnostics",
		"SlowResponseTime":   "collect_diagnostics",
		"HighErrorRate":      "restart_pod",
		"LoadBalancerError":  "collect_diagnostics",
	}

	// Check high confidence actions first
	if action, exists := highConfidenceActions[alert.Name]; exists {
		return action
	}

	// Check medium confidence actions
	if action, exists := mediumConfidenceActions[alert.Name]; exists {
		return action
	}

	// For security alerts or unknown types, be more conservative
	if strings.Contains(strings.ToLower(alert.Name), "security") ||
		strings.Contains(strings.ToLower(alert.Name), "unauthorized") {
		return "notify_only"
	}

	// Default fallback with some randomness
	defaultActions := []string{"collect_diagnostics", "notify_only", "restart_pod"}
	return defaultActions[rand.Intn(len(defaultActions))]
}

// GetCallStats returns statistics about the client usage
func (f *TestSLMClient) GetCallStats() map[string]interface{} {
	totalCalls := len(f.callHistory)
	successfulCalls := 0
	var totalLatency time.Duration
	errorTypes := make(map[string]int)
	confidenceSum := 0.0

	for _, call := range f.callHistory {
		if call.Success {
			successfulCalls++
			confidenceSum += call.Confidence
		} else {
			errorTypes[call.ErrorType]++
		}
		totalLatency += call.Duration
	}

	stats := map[string]interface{}{
		"total_calls":        totalCalls,
		"successful_calls":   successfulCalls,
		"success_rate":       0.0,
		"average_latency":    0.0,
		"average_confidence": 0.0,
		"error_breakdown":    errorTypes,
	}

	if totalCalls > 0 {
		stats["success_rate"] = float64(successfulCalls) / float64(totalCalls)
		stats["average_latency"] = float64(totalLatency) / float64(totalCalls) / float64(time.Millisecond)
	}

	if successfulCalls > 0 {
		stats["average_confidence"] = confidenceSum / float64(successfulCalls)
	}

	return stats
}

// ClearHistory resets the call history
func (f *TestSLMClient) ClearHistory() {
	f.callHistory = make([]TestSLMCall, 0)
	f.callCount = 0
	f.rateLimitCounts = make(map[time.Time]int)
	if f.memoryEnabled {
		f.previousAlerts = make(map[string]time.Time)
	}
}
