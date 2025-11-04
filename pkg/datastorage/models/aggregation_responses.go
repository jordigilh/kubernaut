/*
Copyright 2025 Jordi Gil Heredia.

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

package models

// ========================================
// AGGREGATION API RESPONSE TYPES
// Business Requirement: BR-STORAGE-030, BR-STORAGE-032, BR-STORAGE-033, BR-STORAGE-034
// ========================================
//
// These structured types replace map[string]interface{} for aggregation API responses,
// providing compile-time type safety and clear API contracts.
//
// Anti-Pattern Addressed: Using map[string]interface{} eliminates type safety (IMPLEMENTATION_PLAN_V4.9 #21)
// ========================================

// SuccessRateAggregationResponse represents the success rate aggregation for a workflow
// BR-STORAGE-030: Workflow Success Rate Aggregation
type SuccessRateAggregationResponse struct {
	WorkflowID   string  `json:"workflow_id"`
	TotalCount   int     `json:"total_count"`
	SuccessCount int     `json:"success_count"`
	FailureCount int     `json:"failure_count"`
	SuccessRate  float64 `json:"success_rate"`
}

// AggregationItem represents a single item in a grouped aggregation (namespace, severity, etc.)
type AggregationItem struct {
	// Key is the grouping dimension (namespace, severity, etc.)
	// JSON field name varies by endpoint:
	// - "namespace" for /by-namespace
	// - "severity" for /by-severity
	Key   string `json:"-"` // Omit from JSON, populated dynamically
	Count int    `json:"count"`
}

// NamespaceAggregationItem represents a namespace aggregation item
// BR-STORAGE-032: Namespace Grouping Aggregation
type NamespaceAggregationItem struct {
	Namespace string `json:"namespace"`
	Count     int    `json:"count"`
}

// NamespaceAggregationResponse represents the namespace aggregation endpoint response
// BR-STORAGE-032: Namespace Grouping Aggregation
type NamespaceAggregationResponse struct {
	Aggregations []NamespaceAggregationItem `json:"aggregations"`
}

// SeverityAggregationItem represents a severity aggregation item
// BR-STORAGE-033: Severity Distribution Aggregation
type SeverityAggregationItem struct {
	Severity string `json:"severity"`
	Count    int    `json:"count"`
}

// SeverityAggregationResponse represents the severity aggregation endpoint response
// BR-STORAGE-033: Severity Distribution Aggregation
type SeverityAggregationResponse struct {
	Aggregations []SeverityAggregationItem `json:"aggregations"`
}

// TrendDataPoint represents a single point in a time-series trend
type TrendDataPoint struct {
	Date  string `json:"date"`  // Format: "YYYY-MM-DD"
	Count int    `json:"count"`
}

// TrendAggregationResponse represents the trend aggregation response
// BR-STORAGE-034: Incident Trend Aggregation
type TrendAggregationResponse struct {
	Period     string            `json:"period"` // e.g., "7d", "30d"
	DataPoints []TrendDataPoint `json:"data_points"`
}

