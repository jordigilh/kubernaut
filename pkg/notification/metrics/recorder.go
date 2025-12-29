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

package metrics

import "github.com/prometheus/client_golang/prometheus"

// ========================================
// PROMETHEUS RECORDER IMPLEMENTATION (DD-METRICS-001)
// 📋 Design Decision: DD-METRICS-001 | ✅ Dependency Injection Pattern
// See: docs/architecture/decisions/DD-METRICS-001-controller-metrics-wiring-pattern.md
// ========================================
//
// PrometheusRecorder implements the Recorder interface using Prometheus metrics.
// This wraps the Metrics struct that holds Prometheus collectors.
//
// DD-005 COMPLIANT:
// - All metrics follow {service}_{component}_{metric_name}_{unit} format
// - Registered with controller-runtime global registry
// - Accessible via /metrics endpoint
//
// DD-METRICS-001 COMPLIANT:
// - Dependency-injected into controller
// - Interface-based for testability
// - No global state in controller
// ========================================

// PrometheusRecorder is a Prometheus-based implementation of the Recorder interface
type PrometheusRecorder struct {
	metrics *Metrics
}

// NewPrometheusRecorder creates a new Prometheus metrics recorder.
// This should be called once in main.go and injected into the controller.
// Metrics are registered with controller-runtime on creation.
func NewPrometheusRecorder() *PrometheusRecorder {
	// Create metrics struct (registers with controller-runtime)
	// DD-METRICS-001: Metrics created and registered atomically
	metrics := NewMetrics()
	return &PrometheusRecorder{
		metrics: metrics,
	}
}

// NewPrometheusRecorderWithRegistry creates a new Prometheus metrics recorder
// with a custom registry. This is useful for testing to avoid registry conflicts.
func NewPrometheusRecorderWithRegistry(registry prometheus.Registerer) *PrometheusRecorder {
	// Create metrics struct with custom registry
	metrics := NewMetricsWithRegistry(registry)
	return &PrometheusRecorder{
		metrics: metrics,
	}
}

// RecordDeliveryAttempt records a delivery attempt (success or failure)
func (p *PrometheusRecorder) RecordDeliveryAttempt(namespace, channel, status string) {
	p.metrics.DeliveryAttemptsTotal.WithLabelValues(channel, status).Inc()
}

// RecordDeliveryDuration records the time taken for a delivery
func (p *PrometheusRecorder) RecordDeliveryDuration(namespace, channel string, durationSeconds float64) {
	p.metrics.DeliveryDuration.WithLabelValues(channel).Observe(durationSeconds)
}

// UpdateFailureRatio updates the failure ratio metric for a namespace (0-1 scale)
// Note: This metric is tracked per-notification, not per-namespace in current implementation
func (p *PrometheusRecorder) UpdateFailureRatio(namespace string, ratio float64) {
	// This metric is not currently exposed in the consolidated metrics
	// Keeping as no-op for interface compliance
}

// RecordStuckDuration records time spent in Delivering phase
// Note: This metric is not currently exposed in the consolidated metrics
func (p *PrometheusRecorder) RecordStuckDuration(namespace string, durationSeconds float64) {
	// Keeping as no-op for interface compliance
}

// UpdatePhaseCount updates the count of notifications in a specific phase
func (p *PrometheusRecorder) UpdatePhaseCount(namespace, phase string, count float64) {
	p.metrics.ReconcilerActive.WithLabelValues(phase).Set(count)
}

// RecordDeliveryRetries records the number of retries for a notification
func (p *PrometheusRecorder) RecordDeliveryRetries(namespace string, retries float64) {
	// This is tracked via DeliveryRetriesTotal counter
	// Convert to counter increment (simplified)
	for i := 0; i < int(retries); i++ {
		p.metrics.DeliveryRetriesTotal.WithLabelValues("slack", "retry").Inc()
	}
}

// RecordSlackRetry records a Slack API retry attempt
func (p *PrometheusRecorder) RecordSlackRetry(namespace, reason string) {
	p.metrics.DeliveryRetriesTotal.WithLabelValues("slack", reason).Inc()
}

// RecordSlackBackoff records the backoff duration for a Slack API retry
// Note: This metric is not currently exposed in the consolidated metrics
func (p *PrometheusRecorder) RecordSlackBackoff(namespace string, durationSeconds float64) {
	// Keeping as no-op for interface compliance
}

// Compile-time check that PrometheusRecorder implements Recorder
var _ Recorder = (*PrometheusRecorder)(nil)
