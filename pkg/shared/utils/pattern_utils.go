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

package utils

import (
	"strings"
)

// **REFACTOR PHASE**: Extracted common pattern analysis utilities for code quality improvement

// AlertPatternType represents different types of alert patterns
type AlertPatternType string

const (
	AlertPatternMemory  AlertPatternType = "memory_issue"
	AlertPatternCPU     AlertPatternType = "cpu_issue"
	AlertPatternStorage AlertPatternType = "storage_issue"
	AlertPatternNetwork AlertPatternType = "network_issue"
	AlertPatternGeneric AlertPatternType = "generic_issue"
)

// AlertPattern represents a standardized alert pattern
type AlertPattern struct {
	Type               AlertPatternType       `json:"type"`
	CommonCauses       []string               `json:"common_causes"`
	RecommendedActions []string               `json:"recommended_actions"`
	SeverityImpact     float64                `json:"severity_impact"`
	Context            map[string]interface{} `json:"context"`
}

// AnalyzeAlertPattern analyzes an alert name and returns pattern information
// **CODE QUALITY**: Extracted common pattern used across AI service integration
func AnalyzeAlertPattern(alertName, severity, namespace string) *AlertPattern {
	alertNameLower := strings.ToLower(alertName)

	var pattern *AlertPattern

	// Pattern analysis for common alert types using switch for better readability
	switch {
	case strings.Contains(alertNameLower, "memory") || strings.Contains(alertNameLower, "oom"):
		pattern = &AlertPattern{
			Type:               AlertPatternMemory,
			CommonCauses:       []string{"memory_leak", "insufficient_resources", "high_load"},
			RecommendedActions: []string{"check_memory_usage", "scale_resources", "investigate_memory_leaks"},
		}
	case strings.Contains(alertNameLower, "cpu") || strings.Contains(alertNameLower, "high_load"):
		pattern = &AlertPattern{
			Type:               AlertPatternCPU,
			CommonCauses:       []string{"high_cpu_usage", "inefficient_code", "resource_contention"},
			RecommendedActions: []string{"check_cpu_usage", "optimize_code", "scale_horizontally"},
		}
	case strings.Contains(alertNameLower, "disk") || strings.Contains(alertNameLower, "storage"):
		pattern = &AlertPattern{
			Type:               AlertPatternStorage,
			CommonCauses:       []string{"disk_full", "io_bottleneck", "storage_failure"},
			RecommendedActions: []string{"check_disk_usage", "cleanup_logs", "add_storage"},
		}
	case strings.Contains(alertNameLower, "network") || strings.Contains(alertNameLower, "connectivity"):
		pattern = &AlertPattern{
			Type:               AlertPatternNetwork,
			CommonCauses:       []string{"network_timeout", "connectivity_issues", "bandwidth_limit"},
			RecommendedActions: []string{"check_network_connectivity", "investigate_latency", "verify_dns"},
		}
	default:
		pattern = &AlertPattern{
			Type:    AlertPatternGeneric,
			Context: map[string]interface{}{"analysis": "general_alert_pattern"},
		}
	}

	// Add common context
	pattern.SeverityImpact = CalculateSeverityImpact(severity)
	if pattern.Context == nil {
		pattern.Context = make(map[string]interface{})
	}
	pattern.Context["namespace"] = namespace
	pattern.Context["severity"] = severity

	return pattern
}

// ExtractKeyPatterns extracts key patterns from text using domain-specific keywords
// **ARCHITECTURE IMPROVEMENT**: Extracted common pattern used across prompt builders
func ExtractKeyPatterns(text string, maxPatterns int) []string {
	var patterns []string
	textLower := strings.ToLower(text)

	// Domain-specific patterns
	domainPatterns := map[string][]string{
		"cpu":             {"cpu", "processor", "compute"},
		"memory":          {"memory", "ram", "heap", "oom"},
		"disk":            {"disk", "storage", "volume"},
		"network":         {"network", "connectivity", "bandwidth"},
		"kubernetes":      {"k8s", "kubernetes", "pod", "namespace", "deployment"},
		"scaling":         {"scale", "scaling", "resize", "capacity"},
		"monitoring":      {"monitor", "alert", "metric", "observe"},
		"troubleshooting": {"troubleshoot", "debug", "investigate", "diagnose"},
	}

	for pattern, keywords := range domainPatterns {
		for _, keyword := range keywords {
			if strings.Contains(textLower, keyword) {
				patterns = append(patterns, pattern)
				break // Don't add duplicate patterns
			}
		}
		if len(patterns) >= maxPatterns {
			break
		}
	}

	// If no domain patterns found, extract general patterns
	if len(patterns) == 0 {
		words := strings.Fields(textLower)
		for _, word := range words {
			if len(word) >= 4 { // Significant words only
				patterns = append(patterns, word)
				if len(patterns) >= maxPatterns {
					break
				}
			}
		}
	}

	return patterns
}

// CalculateSeverityImpact calculates business impact based on alert severity
// **BUSINESS LOGIC ENHANCEMENT**: Common severity impact calculation
func CalculateSeverityImpact(severity string) float64 {
	switch strings.ToLower(severity) {
	case "critical":
		return 1.0
	case "high":
		return 0.8
	case "warning", "medium":
		return 0.5
	case "info", "low":
		return 0.2
	default:
		return 0.3 // Default for unknown severity
	}
}

// CalculatePatternConfidence calculates confidence based on frequency
// **PERFORMANCE OPTIMIZATION**: Common confidence calculation pattern
func CalculatePatternConfidence(frequency float64, maxFrequency float64) float64 {
	if maxFrequency <= 0 {
		maxFrequency = 10.0 // Default max frequency
	}

	confidence := frequency / maxFrequency
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}
