<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	sharedhttp "github.com/jordigilh/kubernaut/pkg/shared/http"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/sirupsen/logrus"
)

// PrometheusClient provides real Prometheus API integration
type PrometheusClient struct {
	endpoint   string
	httpClient *http.Client
	log        *logrus.Logger
}

// NewPrometheusClient creates a new Prometheus API client
func NewPrometheusClient(endpoint string, timeout time.Duration, log *logrus.Logger) *PrometheusClient {
	return &PrometheusClient{
		endpoint:   strings.TrimRight(endpoint, "/"),
		httpClient: sharedhttp.NewClient(sharedhttp.PrometheusClientConfig(timeout)),
		log:        log,
	}
}

// PrometheusQueryResult represents a single query result from Prometheus
// BR-TYPE-006: Define standard metric types instead of interface{}
type PrometheusQueryResult struct {
	Metric map[string]string `json:"metric"`
	Value  PrometheusValue   `json:"value"` // [timestamp, value]
}

// PrometheusValue represents a single metric value with timestamp
// Replaces []interface{} with proper typed structure that handles JSON marshaling
type PrometheusValue struct {
	Timestamp float64 `json:"-"` // Unix timestamp
	Value     string  `json:"-"` // Metric value as string (Prometheus format)
}

// UnmarshalJSON implements json.Unmarshaler to handle Prometheus [timestamp, value] format
// BR-TYPE-007: Implement data validation and schema enforcement
func (pv *PrometheusValue) UnmarshalJSON(data []byte) error {
	var raw []interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if len(raw) != 2 {
		return fmt.Errorf("prometheus value must have exactly 2 elements, got %d", len(raw))
	}

	// Parse timestamp
	if ts, ok := raw[0].(float64); ok {
		pv.Timestamp = ts
	} else {
		return fmt.Errorf("prometheus timestamp must be a number, got %T", raw[0])
	}

	// Parse value
	if val, ok := raw[1].(string); ok {
		pv.Value = val
	} else {
		return fmt.Errorf("prometheus value must be a string, got %T", raw[1])
	}

	return nil
}

// MarshalJSON implements json.Marshaler to output Prometheus [timestamp, value] format
func (pv PrometheusValue) MarshalJSON() ([]byte, error) {
	return json.Marshal([]interface{}{pv.Timestamp, pv.Value})
}

// PrometheusQueryResponse represents the response from Prometheus query API
type PrometheusQueryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string                  `json:"resultType"`
		Result     []PrometheusQueryResult `json:"result"`
	} `json:"data"`
}

// PrometheusRangeResult represents a range query result from Prometheus
// BR-TYPE-006: Define standard metric types instead of interface{}
type PrometheusRangeResult struct {
	Metric map[string]string `json:"metric"`
	Values []PrometheusValue `json:"values"` // Multiple [timestamp, value] pairs
}

// PrometheusRangeResponse represents the response from Prometheus range query API
type PrometheusRangeResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string                  `json:"resultType"`
		Result     []PrometheusRangeResult `json:"result"`
	} `json:"data"`
}

// CheckMetricsImprovement compares metrics before and after an action
func (c *PrometheusClient) CheckMetricsImprovement(ctx context.Context, alert types.Alert, actionTrace *actionhistory.ResourceActionTrace) (bool, error) {
	c.log.WithFields(logrus.Fields{
		"alert_name":  alert.Name,
		"action_type": actionTrace.ActionType,
		"action_id":   actionTrace.ActionID,
		"namespace":   alert.Namespace,
	}).Debug("Checking metrics improvement via Prometheus API")

	if actionTrace.ExecutionStartTime == nil || actionTrace.ExecutionEndTime == nil {
		return false, fmt.Errorf("action trace missing execution timestamps")
	}

	// Define metrics to check based on alert type and action
	metricsToCheck := c.getRelevantMetrics(alert, actionTrace)
	if len(metricsToCheck) == 0 {
		c.log.WithFields(logrus.Fields{
			"alert_name":  alert.Name,
			"action_type": actionTrace.ActionType,
		}).Debug("No relevant metrics found for improvement check")
		return false, nil
	}

	// Calculate time windows for before/after comparison
	executionStart := *actionTrace.ExecutionStartTime
	executionEnd := *actionTrace.ExecutionEndTime

	// Before window: 10 minutes before action execution
	beforeStart := executionStart.Add(-10 * time.Minute)
	beforeEnd := executionStart

	// After window: 5 minutes after action execution to allow stabilization
	afterStart := executionEnd.Add(5 * time.Minute)
	afterEnd := afterStart.Add(10 * time.Minute)

	improvementCount := 0
	totalMetrics := len(metricsToCheck)

	// Check each metric for improvement
	for _, metricQuery := range metricsToCheck {
		improved, err := c.checkSingleMetricImprovement(ctx, metricQuery, alert.Namespace, beforeStart, beforeEnd, afterStart, afterEnd)
		if err != nil {
			c.log.WithError(err).WithField("metric", metricQuery).Warn("Failed to check metric improvement")
			continue
		}
		if improved {
			improvementCount++
		}
	}

	// Consider improvement if majority of metrics improved
	threshold := float64(totalMetrics) * 0.6 // 60% threshold
	isImproved := float64(improvementCount) >= threshold

	c.log.WithFields(logrus.Fields{
		"alert_name":        alert.Name,
		"action_type":       actionTrace.ActionType,
		"improved_metrics":  improvementCount,
		"total_metrics":     totalMetrics,
		"improvement_ratio": float64(improvementCount) / float64(totalMetrics),
		"is_improved":       isImproved,
	}).Info("Metrics improvement analysis completed")

	return isImproved, nil
}

// GetResourceMetrics retrieves current metrics for a resource
func (c *PrometheusClient) GetResourceMetrics(ctx context.Context, namespace, resourceName string, metricNames []string) (map[string]float64, error) {
	c.log.WithFields(logrus.Fields{
		"namespace":     namespace,
		"resource_name": resourceName,
		"metric_names":  metricNames,
	}).Debug("Getting current resource metrics via Prometheus API")

	results := make(map[string]float64)
	var lastError error

	for _, metricName := range metricNames {
		// Build query for the specific metric
		query := c.buildResourceMetricQuery(metricName, namespace, resourceName)

		value, err := c.queryInstantMetric(ctx, query)
		if err != nil {
			// Distinguish between "no data" (acceptable) and real errors
			if strings.Contains(err.Error(), "no data returned for query") {
				c.log.WithError(err).WithField("metric", metricName).Warn("Failed to query metric")
				continue // No data is acceptable, just skip this metric
			} else {
				// Real errors (connection, parsing, etc.)
				c.log.WithError(err).WithField("metric", metricName).Warn("Failed to query metric")
				lastError = err
				continue
			}
		}

		results[metricName] = value
	}

	c.log.WithFields(logrus.Fields{
		"namespace":     namespace,
		"resource_name": resourceName,
		"result_count":  len(results),
	}).Debug("Retrieved current resource metrics")

	// If no metrics were successfully retrieved and we have real errors (not just "no data"), return the error
	if len(results) == 0 && lastError != nil {
		return results, lastError
	}

	return results, nil
}

// GetMetricsHistory retrieves historical metrics within a time range
func (c *PrometheusClient) GetMetricsHistory(ctx context.Context, namespace, resourceName string, metricNames []string, from, to time.Time) ([]MetricPoint, error) {
	c.log.WithFields(logrus.Fields{
		"namespace":     namespace,
		"resource_name": resourceName,
		"metric_names":  metricNames,
		"from":          from,
		"to":            to,
	}).Debug("Getting metrics history via Prometheus API")

	var allPoints []MetricPoint

	for _, metricName := range metricNames {
		// Build query for the specific metric
		query := c.buildResourceMetricQuery(metricName, namespace, resourceName)

		points, err := c.queryRangeMetric(ctx, query, from, to, time.Minute)
		if err != nil {
			c.log.WithError(err).WithField("metric", metricName).Warn("Failed to query metric history")
			continue
		}

		// Convert to MetricPoint format
		for _, point := range points {
			metricPoint := MetricPoint{
				MetricName: metricName,
				Value:      point.Value,
				Timestamp:  point.Timestamp,
				Labels: map[string]string{
					"namespace":     namespace,
					"resource_name": resourceName,
				},
			}
			allPoints = append(allPoints, metricPoint)
		}
	}

	c.log.WithFields(logrus.Fields{
		"namespace":     namespace,
		"resource_name": resourceName,
		"total_points":  len(allPoints),
	}).Debug("Retrieved metrics history")

	return allPoints, nil
}

// getRelevantMetrics returns metrics to check based on alert type and action
func (c *PrometheusClient) getRelevantMetrics(alert types.Alert, actionTrace *actionhistory.ResourceActionTrace) []string {
	var metrics []string

	// Base metrics for all resource types
	baseMetrics := []string{
		"container_cpu_usage_seconds_total",
		"container_memory_usage_bytes",
		"container_memory_working_set_bytes",
	}

	// Alert-specific metrics
	switch strings.ToLower(alert.Name) {
	case "highmemoryusage", "memorypressure":
		metrics = append(metrics,
			"container_memory_usage_bytes",
			"container_memory_working_set_bytes",
			"container_memory_cache",
		)
	case "highcpuusage", "cputhrottling":
		metrics = append(metrics,
			"container_cpu_usage_seconds_total",
			"container_cpu_cfs_throttled_seconds_total",
			"rate(container_cpu_usage_seconds_total[5m])",
		)
	case "diskspacefull", "diskpressure":
		metrics = append(metrics,
			"container_fs_usage_bytes",
			"container_fs_limit_bytes",
		)
	default:
		// Use base metrics for unknown alert types
		metrics = baseMetrics
	}

	// Action-specific additional metrics
	switch actionTrace.ActionType {
	case "scale_deployment":
		metrics = append(metrics, "kube_deployment_status_replicas")
	case "increase_resources":
		// Already covered by base metrics
	case "restart_pod":
		metrics = append(metrics, "kube_pod_container_status_restarts_total")
	}

	return metrics
}

// checkSingleMetricImprovement checks if a single metric improved between time windows
func (c *PrometheusClient) checkSingleMetricImprovement(ctx context.Context, query, _ string, beforeStart, beforeEnd, afterStart, afterEnd time.Time) (bool, error) {
	// Get average value before action
	beforeAvg, err := c.getAverageMetricValue(ctx, query, beforeStart, beforeEnd)
	if err != nil {
		return false, fmt.Errorf("failed to get before metric value: %w", err)
	}

	// Get average value after action
	afterAvg, err := c.getAverageMetricValue(ctx, query, afterStart, afterEnd)
	if err != nil {
		return false, fmt.Errorf("failed to get after metric value: %w", err)
	}

	// Define improvement based on metric type
	// For usage/consumption metrics (CPU, memory), lower is better
	// For availability/capacity metrics, higher is better
	isUsageMetric := strings.Contains(query, "usage") ||
		strings.Contains(query, "utilization") ||
		strings.Contains(query, "memory") ||
		strings.Contains(query, "cpu") ||
		strings.Contains(query, "throttled")

	var improved bool
	if isUsageMetric {
		// Lower is better for usage metrics
		improvementThreshold := 0.05 // 5% improvement threshold
		improved = (beforeAvg-afterAvg)/beforeAvg > improvementThreshold
	} else {
		// Higher is better for other metrics
		improvementThreshold := 0.05 // 5% improvement threshold
		improved = (afterAvg-beforeAvg)/beforeAvg > improvementThreshold
	}

	c.log.WithFields(logrus.Fields{
		"query":      query,
		"before_avg": beforeAvg,
		"after_avg":  afterAvg,
		"is_usage":   isUsageMetric,
		"improved":   improved,
	}).Debug("Single metric improvement analysis")

	return improved, nil
}

// getAverageMetricValue calculates average metric value over a time range
func (c *PrometheusClient) getAverageMetricValue(ctx context.Context, query string, start, end time.Time) (float64, error) {
	// Use avg_over_time to get average value in the time window
	avgQuery := fmt.Sprintf("avg_over_time((%s)[%s:])", query, c.formatDuration(end.Sub(start)))

	value, err := c.queryInstantMetricAtTime(ctx, avgQuery, end)
	if err != nil {
		return 0, err
	}

	return value, nil
}

// buildResourceMetricQuery builds a Prometheus query for a specific resource metric
func (c *PrometheusClient) buildResourceMetricQuery(metricName, namespace, resourceName string) string {
	// Build query with namespace and resource name filters
	query := fmt.Sprintf("%s{namespace=\"%s\"", metricName, namespace)

	// Add resource name filter based on metric type
	if strings.Contains(metricName, "container") {
		query += fmt.Sprintf(",pod=~\"%s.*\"", resourceName)
	} else if strings.Contains(metricName, "kube_deployment") {
		query += fmt.Sprintf(",deployment=\"%s\"", resourceName)
	} else if strings.Contains(metricName, "kube_pod") {
		query += fmt.Sprintf(",pod=~\"%s.*\"", resourceName)
	}

	query += "}"
	return query
}

// queryInstantMetric executes an instant query and returns the first result value
func (c *PrometheusClient) queryInstantMetric(ctx context.Context, query string) (float64, error) {
	return c.queryInstantMetricAtTime(ctx, query, time.Now())
}

// queryInstantMetricAtTime executes an instant query at a specific time
func (c *PrometheusClient) queryInstantMetricAtTime(ctx context.Context, query string, t time.Time) (float64, error) {
	queryURL := fmt.Sprintf("%s/api/v1/query", c.endpoint)

	params := url.Values{}
	params.Add("query", query)
	params.Add("time", strconv.FormatInt(t.Unix(), 10))

	req, err := http.NewRequestWithContext(ctx, "GET", queryURL+"?"+params.Encode(), nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.log.WithError(err).Error("Failed to close HTTP response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("prometheus API returned status %d", resp.StatusCode)
	}

	var response PrometheusQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return 0, fmt.Errorf("failed to decode response: %w", err)
	}

	if response.Status != "success" {
		return 0, fmt.Errorf("prometheus API returned status: %s", response.Status)
	}

	if len(response.Data.Result) == 0 {
		return 0, fmt.Errorf("no data returned for query: %s", query)
	}

	// Parse the value from the first result using typed PrometheusValue
	result := response.Data.Result[0]
	if result.Value.Value == "" {
		return 0, fmt.Errorf("invalid value format in result")
	}

	value, err := strconv.ParseFloat(result.Value.Value, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse value: %w", err)
	}

	return value, nil
}

// queryRangeMetric executes a range query and returns metric points
func (c *PrometheusClient) queryRangeMetric(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]MetricPoint, error) {
	queryURL := fmt.Sprintf("%s/api/v1/query_range", c.endpoint)

	params := url.Values{}
	params.Add("query", query)
	params.Add("start", strconv.FormatInt(start.Unix(), 10))
	params.Add("end", strconv.FormatInt(end.Unix(), 10))
	params.Add("step", c.formatDuration(step))

	req, err := http.NewRequestWithContext(ctx, "GET", queryURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.log.WithError(err).Error("Failed to close HTTP response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prometheus API returned status %d", resp.StatusCode)
	}

	var response PrometheusRangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if response.Status != "success" {
		return nil, fmt.Errorf("prometheus API returned status: %s", response.Status)
	}

	var points []MetricPoint
	for _, result := range response.Data.Result {
		for _, valuePoint := range result.Values {
			// Use typed PrometheusValue fields instead of array indexing
			if valuePoint.Value == "" {
				continue
			}

			value, err := strconv.ParseFloat(valuePoint.Value, 64)
			if err != nil {
				continue
			}

			point := MetricPoint{
				Value:     value,
				Timestamp: time.Unix(int64(valuePoint.Timestamp), 0),
				Labels:    result.Metric,
			}
			points = append(points, point)
		}
	}

	return points, nil
}

// formatDuration formats a duration for Prometheus query
func (c *PrometheusClient) formatDuration(d time.Duration) string {
	if d >= time.Hour {
		return fmt.Sprintf("%.0fh", d.Hours())
	}
	if d >= time.Minute {
		return fmt.Sprintf("%.0fm", d.Minutes())
	}
	return fmt.Sprintf("%.0fs", d.Seconds())
}

// HealthCheck verifies connectivity to Prometheus
func (c *PrometheusClient) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.endpoint+"/-/healthy", nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute health check: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.log.WithError(err).Error("Failed to close HTTP response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("prometheus health check failed with status %d", resp.StatusCode)
	}

	return nil
}
