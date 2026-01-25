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

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	dto "github.com/prometheus/client_model/go"
)

// Gateway Service Prometheus Metrics
//
// This package defines all metrics for the Gateway service, following the
// specifications in docs/services/stateless/gateway-service/metrics-slos.md
//
// Metrics aligned with business requirements:
// 1. Signal ingestion (received, deduplicated) - BR-GATEWAY-066, BR-GATEWAY-069
// 2. CRD creation (success, failures) - BR-GATEWAY-068
// 3. Performance (HTTP request duration) - BR-GATEWAY-067, BR-GATEWAY-079
// 4. Deduplication (cache hits, deduplication rate) - BR-GATEWAY-069
//
// DD-005 V3.0 Compliance: All metric names defined as exported constants
// for type safety and test/production parity.

// Metric name constants (DD-005 V3.0 Section 1.1 - MANDATORY)
// These constants ensure tests use correct metric names and prevent typos.
const (
	// MetricNameSignalsReceivedTotal tracks total signals received by source and severity
	MetricNameSignalsReceivedTotal = "gateway_signals_received_total"

	// MetricNameSignalsDeduplicatedTotal tracks signals deduplicated by fingerprint
	MetricNameSignalsDeduplicatedTotal = "gateway_signals_deduplicated_total"

	// MetricNameCRDsCreatedTotal tracks successful CRD creations
	MetricNameCRDsCreatedTotal = "gateway_crds_created_total"

	// MetricNameCRDCreationErrorsTotal tracks CRD creation failures by error type
	MetricNameCRDCreationErrorsTotal = "gateway_crd_creation_errors_total"

	// MetricNameHTTPRequestDuration tracks HTTP request latency (P50/P95/P99 SLO)
	MetricNameHTTPRequestDuration = "gateway_http_request_duration_seconds"

	// MetricNameDeduplicationCacheHitsTotal tracks deduplication cache hits
	MetricNameDeduplicationCacheHitsTotal = "gateway_deduplication_cache_hits_total"

	// MetricNameDeduplicationRate tracks current deduplication percentage
	MetricNameDeduplicationRate = "gateway_deduplication_rate"

	// MetricNameConflictRetriesTotal tracks K8s optimistic concurrency retry attempts
	MetricNameConflictRetriesTotal = "gateway_conflict_retries_total"

	// MetricNameConflictResolutionDuration tracks latency for conflict resolution with retries
	MetricNameConflictResolutionDuration = "gateway_conflict_resolution_duration_seconds"

	// MetricNameFieldIndexQueryDuration tracks field index query performance
	MetricNameFieldIndexQueryDuration = "gateway_field_index_query_duration_seconds"

	// MetricNameCircuitBreakerState tracks circuit breaker state (0=closed, 1=half-open, 2=open)
	MetricNameCircuitBreakerState = "gateway_circuit_breaker_state"

	// MetricNameCircuitBreakerOperationsTotal tracks operations through circuit breaker
	MetricNameCircuitBreakerOperationsTotal = "gateway_circuit_breaker_operations_total"
)

// Metrics holds all Gateway service Prometheus metrics
// Specification-driven design: Only metrics defined in metrics-slos.md are implemented
type Metrics struct {
	// Signal Ingestion Metrics (BR-GATEWAY-066: Metrics endpoint)
	// Note: Field names kept as "Alerts*" for backward compatibility, but metric names use "signals_"
	AlertsReceivedTotal     *prometheus.CounterVec // gateway_signals_received_total
	AlertsDeduplicatedTotal *prometheus.CounterVec // gateway_signals_deduplicated_total

	// CRD Creation Metrics (BR-GATEWAY-068: CRD creation metrics)
	CRDsCreatedTotal  *prometheus.CounterVec // gateway_crds_created_total
	CRDCreationErrors *prometheus.CounterVec // gateway_crd_creation_errors_total

	// Performance Metrics (BR-GATEWAY-067: HTTP request metrics, BR-GATEWAY-079: Performance metrics)
	HTTPRequestDuration *prometheus.HistogramVec // gateway_http_request_duration_seconds

	// Deduplication Metrics (BR-GATEWAY-069: Deduplication metrics)
	DeduplicationCacheHitsTotal prometheus.Counter // gateway_deduplication_cache_hits_total
	// Note: DeduplicationRate is NOT stored here - it's calculated on-the-fly by DeduplicationRateCollector

	// Conflict Resolution Metrics (Performance Observability - Option A)
	// Track optimistic concurrency control performance for status updates
	ConflictRetriesTotal      *prometheus.CounterVec   // gateway_conflict_retries_total
	ConflictResolutionLatency *prometheus.HistogramVec // gateway_conflict_resolution_duration_seconds

	// Field Index Performance Metrics (Performance Observability - Option A)
	// Track O(1) field index query performance for deduplication lookups
	FieldIndexQueryDuration *prometheus.HistogramVec // gateway_field_index_query_duration_seconds

	// Circuit Breaker Metrics (Resilience - Option B)
	// Track K8s API circuit breaker state and operation results
	CircuitBreakerState      *prometheus.GaugeVec   // gateway_circuit_breaker_state
	CircuitBreakerOperations *prometheus.CounterVec // gateway_circuit_breaker_operations_total

	// Internal: Registry for custom metrics exposure (test isolation)
	registry prometheus.Gatherer // Used by /metrics endpoint to expose custom registry metrics
}

// NewMetrics creates a new Metrics instance with the default global Prometheus registry
func NewMetrics() *Metrics {
	return NewMetricsWithRegistry(prometheus.DefaultRegisterer)
}

// NewMetricsWithRegistry creates a new Metrics instance with a custom Prometheus registry
//
// This allows integration tests to use isolated registries, preventing metric collisions
// when running multiple Gateway instances in parallel tests.
func NewMetricsWithRegistry(registry prometheus.Registerer) *Metrics {
	factory := promauto.With(registry)

	// Store registry as Gatherer for /metrics endpoint exposure
	var gatherer prometheus.Gatherer
	if reg, ok := registry.(prometheus.Gatherer); ok {
		gatherer = reg
	} else {
		gatherer = prometheus.DefaultGatherer
	}

	m := &Metrics{
		// Store registry for /metrics endpoint
		registry: gatherer,

		// Signal Ingestion Metrics (BR-GATEWAY-066: Prometheus metrics endpoint)
		AlertsReceivedTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameSignalsReceivedTotal, // DD-005 V3.0: Pattern B,
				Help: "Total signals received by source type and severity (Prometheus alerts, K8s events, etc.)",
			},
			[]string{"source_type", "severity"},
		),
		AlertsDeduplicatedTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameSignalsDeduplicatedTotal, // DD-005 V3.0: Pattern B,
				Help: "Total signals deduplicated (duplicate fingerprint detected)",
			},
			[]string{"signal_name"},
		),

		// CRD Creation Metrics (BR-GATEWAY-068: CRD creation metrics)
		CRDsCreatedTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameCRDsCreatedTotal, // DD-005 V3.0: Pattern B,
				Help: "Total RemediationRequest CRDs created by source type and creation status",
			},
			[]string{"source_type", "status"},
		),
		CRDCreationErrors: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameCRDCreationErrorsTotal, // DD-005 V3.0: Pattern B,
				Help: "Total CRD creation errors by error type",
			},
			[]string{"error_type"},
		),

		// Performance Metrics (BR-GATEWAY-067: HTTP metrics, BR-GATEWAY-079: Performance metrics)
		HTTPRequestDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    MetricNameHTTPRequestDuration, // DD-005 V3.0: Pattern B,
				Help:    "HTTP request duration in seconds (includes full pipeline)",
				Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to ~1s for P50/P95/P99 SLO
			},
			[]string{"endpoint", "method", "status"},
		),

		// Deduplication Metrics (BR-GATEWAY-069: Deduplication metrics)
		DeduplicationCacheHitsTotal: factory.NewCounter(
			prometheus.CounterOpts{
				Name: MetricNameDeduplicationCacheHitsTotal, // DD-005 V3.0: Pattern B,
				Help: "Total deduplication cache hits (duplicate fingerprint found)",
			},
		),
		// DeduplicationRate: Removed - now calculated by custom collector (see below)

		// Conflict Resolution Metrics (Performance Observability - Option A)
		// Track optimistic concurrency control performance for status updates
		ConflictRetriesTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameConflictRetriesTotal,
				Help: "Total K8s optimistic concurrency retry attempts by operation and error type",
			},
			[]string{"operation", "error_type"},
		),
		ConflictResolutionLatency: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    MetricNameConflictResolutionDuration,
				Help:    "Latency for K8s conflict resolution including retries (seconds)",
				Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to ~1s
			},
			[]string{"operation"},
		),

		// Field Index Performance Metrics (Performance Observability - Option A)
		// Track O(1) field index query performance for deduplication lookups
		FieldIndexQueryDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    MetricNameFieldIndexQueryDuration,
				Help:    "Field index query duration for deduplication lookups (seconds)",
				Buckets: prometheus.ExponentialBuckets(0.0001, 2, 10), // 0.1ms to ~100ms
			},
			[]string{"index_name"},
		),

		// Circuit Breaker Metrics (Resilience - Option B)
		// Track K8s API circuit breaker state and operation results
		CircuitBreakerState: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: MetricNameCircuitBreakerState,
				Help: "Circuit breaker state (0=closed, 1=half-open, 2=open)",
			},
			[]string{"name"},
		),
		CircuitBreakerOperations: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: MetricNameCircuitBreakerOperationsTotal,
				Help: "Total operations through circuit breaker by name and result",
			},
			[]string{"name", "result"},
		),
	}

	// BR-GATEWAY-069: Register custom collector for deduplication rate
	// This calculates the rate on-the-fly when /metrics is scraped
	dedupRateCollector := NewDeduplicationRateCollector(m)
	registry.MustRegister(dedupRateCollector)

	return m
}

// Registry returns the Prometheus Gatherer for this metrics instance
// This is used by the /metrics endpoint to expose metrics from custom registries
func (m *Metrics) Registry() prometheus.Gatherer {
	if m.registry != nil {
		return m.registry
	}
	return prometheus.DefaultGatherer
}

// DeduplicationRateCollector is a custom Prometheus collector that calculates
// gateway_deduplication_rate gauge on-the-fly when /metrics is scraped
// BR-GATEWAY-069: Derived metric implementation
//
// Design Pattern: Custom Collector for Derived Metrics
// - Calculates rate from existing counters (DeduplicationCacheHitsTotal / sum(AlertsReceivedTotal))
// - No state duplication - reads from Prometheus counters
// - Metric always fresh when scraped
// - Standard Prometheus pattern for derived metrics
type DeduplicationRateCollector struct {
	metrics *Metrics
	desc    *prometheus.Desc
}

// NewDeduplicationRateCollector creates a custom collector for the deduplication rate gauge
func NewDeduplicationRateCollector(m *Metrics) *DeduplicationRateCollector {
	return &DeduplicationRateCollector{
		metrics: m,
		desc: prometheus.NewDesc(
			MetricNameDeduplicationRate,
			"Current deduplication rate (percentage of signals deduplicated, calculated on-the-fly)",
			nil, // No labels
			nil, // No constant labels
		),
	}
}

// Describe implements prometheus.Collector
func (c *DeduplicationRateCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

// Collect implements prometheus.Collector
// Calculates deduplication rate from existing counters when /metrics is scraped
func (c *DeduplicationRateCollector) Collect(ch chan<- prometheus.Metric) {
	// Get total deduplications (simple counter, no labels)
	var totalDeduplicated float64
	metricCh := make(chan prometheus.Metric, 1)
	go func() {
		c.metrics.DeduplicationCacheHitsTotal.Collect(metricCh)
		close(metricCh)
	}()
	for m := range metricCh {
		var dto dto.Metric
		if err := m.Write(&dto); err == nil && dto.Counter != nil {
			totalDeduplicated = dto.Counter.GetValue()
		}
	}

	// Get total signals received (sum across all source_type and severity labels)
	var totalReceived float64
	receivedCh := make(chan prometheus.Metric, 100)
	go func() {
		c.metrics.AlertsReceivedTotal.Collect(receivedCh)
		close(receivedCh)
	}()
	for m := range receivedCh {
		var dto dto.Metric
		if err := m.Write(&dto); err == nil && dto.Counter != nil {
			totalReceived += dto.Counter.GetValue()
		}
	}

	// Calculate rate (0 if no signals received yet)
	var dedupRate float64
	if totalReceived > 0 {
		dedupRate = totalDeduplicated / totalReceived
	}

	// Emit gauge metric
	ch <- prometheus.MustNewConstMetric(
		c.desc,
		prometheus.GaugeValue,
		dedupRate,
	)
}
