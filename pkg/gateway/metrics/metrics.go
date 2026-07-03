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

	// MetricNameCircuitBreakerState tracks circuit breaker state (0=closed, 1=half-open, 2=open)
	MetricNameCircuitBreakerState = "gateway_circuit_breaker_state"

	// MetricNameSignalsRejectedTotal tracks signals rejected by scope filtering
	// BR-SCOPE-002: Gateway Signal Filtering
	MetricNameSignalsRejectedTotal = "gateway_signals_rejected_total"

	// MetricNameOwnerResolutionTotal tracks owner resolution attempts by outcome (#1029)
	MetricNameOwnerResolutionTotal = "gateway_owner_resolution_total"

	// MetricNameSignalsParseDroppedTotal tracks alerts dropped during batch parsing (#1032)
	MetricNameSignalsParseDroppedTotal = "gateway_signals_parse_dropped_total"

	// MetricNameDiscoveryRefreshErrorsTotal tracks API discovery refresh failures (#1029)
	MetricNameDiscoveryRefreshErrorsTotal = "gateway_discovery_refresh_errors_total"
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

	// Circuit Breaker Metrics (Resilience - Option B)
	CircuitBreakerState *prometheus.GaugeVec // gateway_circuit_breaker_state

	// Scope Filtering Metrics (BR-SCOPE-002: Gateway Signal Filtering)
	SignalsRejectedTotal *prometheus.CounterVec // gateway_signals_rejected_total{reason}

	// Owner Resolution Metrics (#1029)
	OwnerResolutionTotal *prometheus.CounterVec // gateway_owner_resolution_total{kind, outcome}

	// Batch Parse Drop Metrics (#1032)
	SignalsParseDroppedTotal *prometheus.CounterVec // gateway_signals_parse_dropped_total{reason}

	// Discovery Refresh Metrics (#1029)
	DiscoveryRefreshErrorsTotal prometheus.Counter // gateway_discovery_refresh_errors_total

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

	m := &Metrics{registry: resolveGatherer(registry)}
	registerIngestionAndCRDMetrics(m, factory)
	registerResilienceAndScopeMetrics(m, factory)
	return m
}

// resolveGatherer returns the registry as a Gatherer (for /metrics endpoint
// exposure) when possible, falling back to the process-wide default.
// Extracted from NewMetricsWithRegistry (funlen).
func resolveGatherer(registry prometheus.Registerer) prometheus.Gatherer {
	if reg, ok := registry.(prometheus.Gatherer); ok {
		return reg
	}
	return prometheus.DefaultGatherer
}

// registerIngestionAndCRDMetrics registers the signal-ingestion, CRD-creation,
// and HTTP performance metric families. Extracted from NewMetricsWithRegistry
// (funlen).
func registerIngestionAndCRDMetrics(m *Metrics, factory promauto.Factory) {
	// Signal Ingestion Metrics (BR-GATEWAY-066: Prometheus metrics endpoint)
	m.AlertsReceivedTotal = factory.NewCounterVec(
		prometheus.CounterOpts{
			Name: MetricNameSignalsReceivedTotal, // DD-005 V3.0: Pattern B,
			Help: "Total signals received by source type and severity (Prometheus alerts, K8s events, etc.)",
		},
		[]string{"source_type", "severity"},
	)
	m.AlertsDeduplicatedTotal = factory.NewCounterVec(
		prometheus.CounterOpts{
			Name: MetricNameSignalsDeduplicatedTotal, // DD-005 V3.0: Pattern B,
			Help: "Total signals deduplicated (duplicate fingerprint detected)",
		},
		[]string{"signal_name"},
	)

	// CRD Creation Metrics (BR-GATEWAY-068: CRD creation metrics)
	m.CRDsCreatedTotal = factory.NewCounterVec(
		prometheus.CounterOpts{
			Name: MetricNameCRDsCreatedTotal, // DD-005 V3.0: Pattern B,
			Help: "Total RemediationRequest CRDs created by source type and creation status",
		},
		[]string{"source_type", "status"},
	)
	m.CRDCreationErrors = factory.NewCounterVec(
		prometheus.CounterOpts{
			Name: MetricNameCRDCreationErrorsTotal, // DD-005 V3.0: Pattern B,
			Help: "Total CRD creation errors by error type",
		},
		[]string{"error_type"},
	)

	// Performance Metrics (BR-GATEWAY-067: HTTP metrics, BR-GATEWAY-079: Performance metrics)
	m.HTTPRequestDuration = factory.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    MetricNameHTTPRequestDuration, // DD-005 V3.0: Pattern B,
			Help:    "HTTP request duration in seconds (includes full pipeline)",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to ~1s for P50/P95/P99 SLO
		},
		[]string{"endpoint", "method", "status"},
	)
}

// registerResilienceAndScopeMetrics registers the circuit-breaker, scope
// filtering, owner resolution, batch parse drop, and discovery refresh metric
// families. Extracted from NewMetricsWithRegistry (funlen).
func registerResilienceAndScopeMetrics(m *Metrics, factory promauto.Factory) {
	// Circuit Breaker Metrics (Resilience - Option B)
	m.CircuitBreakerState = factory.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: MetricNameCircuitBreakerState,
			Help: "Circuit breaker state (0=closed, 1=half-open, 2=open)",
		},
		[]string{"name"},
	)

	// Scope Filtering Metrics (BR-SCOPE-002)
	m.SignalsRejectedTotal = factory.NewCounterVec(
		prometheus.CounterOpts{
			Name: MetricNameSignalsRejectedTotal,
			Help: "Total signals rejected by scope filtering by rejection reason",
		},
		[]string{"reason"},
	)

	// Owner Resolution Metrics (#1029)
	m.OwnerResolutionTotal = factory.NewCounterVec(
		prometheus.CounterOpts{
			Name: MetricNameOwnerResolutionTotal,
			Help: "Total owner resolution attempts by resource kind and outcome",
		},
		[]string{"kind", "outcome"},
	)

	// Batch Parse Drop Metrics (#1032)
	m.SignalsParseDroppedTotal = factory.NewCounterVec(
		prometheus.CounterOpts{
			Name: MetricNameSignalsParseDroppedTotal,
			Help: "Total alerts dropped during batch parsing by reason",
		},
		[]string{"reason"},
	)

	// Discovery Refresh Metrics (#1029)
	m.DiscoveryRefreshErrorsTotal = factory.NewCounter(
		prometheus.CounterOpts{
			Name: MetricNameDiscoveryRefreshErrorsTotal,
			Help: "Total API discovery refresh failures",
		},
	)
}

// Registry returns the Prometheus Gatherer for this metrics instance
// This is used by the /metrics endpoint to expose metrics from custom registries
func (m *Metrics) Registry() prometheus.Gatherer {
	if m.registry != nil {
		return m.registry
	}
	return prometheus.DefaultGatherer
}
