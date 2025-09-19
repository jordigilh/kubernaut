package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// EnhancedHealthMetrics provides comprehensive health metrics following business requirements
// BR-METRICS-020 through BR-METRICS-039: Enhanced health monitoring metrics
type EnhancedHealthMetrics struct {
	registry *prometheus.Registry

	// BR-METRICS-020: llm_health_status gauge with component_type label
	healthStatusGauge prometheus.GaugeVec

	// BR-METRICS-021: llm_health_check_duration_seconds histogram
	healthCheckDurationHistogram prometheus.HistogramVec

	// BR-METRICS-022: llm_health_checks_total counter with status label
	healthChecksTotalCounter prometheus.CounterVec

	// BR-METRICS-023: llm_health_consecutive_failures_total gauge
	consecutiveFailuresGauge prometheus.GaugeVec

	// BR-METRICS-024: llm_health_uptime_percentage gauge
	uptimePercentageGauge prometheus.GaugeVec

	// BR-METRICS-025 & BR-METRICS-026: probe duration histograms
	probeDurationHistogram prometheus.HistogramVec

	// BR-METRICS-030: llm_dependency_status gauge
	dependencyStatusGauge prometheus.GaugeVec

	// BR-METRICS-031: llm_dependency_check_duration_seconds histogram
	dependencyCheckDurationHistogram prometheus.HistogramVec

	// BR-METRICS-032: llm_dependency_failures_total counter
	dependencyFailuresCounter prometheus.CounterVec

	// BR-METRICS-035: llm_monitoring_accuracy_percentage gauge for BR-REL-011 compliance
	monitoringAccuracyGauge prometheus.GaugeVec

	// BR-METRICS-036: llm_20b_model_parameter_count gauge for enterprise model validation
	modelParameterCountGauge prometheus.GaugeVec

	// BR-METRICS-037: llm_monitoring_sla_compliance gauge
	monitoringSLAComplianceGauge prometheus.GaugeVec
}

// NewEnhancedHealthMetrics creates enhanced health metrics following business requirements
// This follows the existing metrics pattern but adds comprehensive health monitoring
func NewEnhancedHealthMetrics(registry *prometheus.Registry) *EnhancedHealthMetrics {
	if registry == nil {
		registry = prometheus.DefaultRegisterer.(*prometheus.Registry)
	}

	metrics := &EnhancedHealthMetrics{
		registry: registry,

		// BR-METRICS-020: Health status gauge
		healthStatusGauge: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "llm_health_status",
			Help: "LLM component health status (0=unhealthy, 1=healthy)",
		}, []string{"component_type"}),

		// BR-METRICS-021: Health check duration histogram
		healthCheckDurationHistogram: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "llm_health_check_duration_seconds",
			Help:    "Duration of LLM health checks in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}, []string{"component_type"}),

		// BR-METRICS-022: Health checks total counter
		healthChecksTotalCounter: *prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "llm_health_checks_total",
			Help: "Total number of LLM health checks performed",
		}, []string{"component_type", "status"}),

		// BR-METRICS-023: Consecutive failures gauge
		consecutiveFailuresGauge: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "llm_health_consecutive_failures_total",
			Help: "Current number of consecutive LLM health check failures",
		}, []string{"component_type"}),

		// BR-METRICS-024: Uptime percentage gauge
		uptimePercentageGauge: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "llm_health_uptime_percentage",
			Help: "LLM component uptime percentage for availability tracking",
		}, []string{"component_type"}),

		// BR-METRICS-025 & BR-METRICS-026: Probe duration histogram
		probeDurationHistogram: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "llm_probe_duration_seconds",
			Help:    "Duration of LLM liveness and readiness probes in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}, []string{"probe_type", "component_id"}),

		// BR-METRICS-030: Dependency status gauge
		dependencyStatusGauge: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "llm_dependency_status",
			Help: "LLM external dependency health status (0=unavailable, 1=available)",
		}, []string{"dependency_name", "criticality"}),

		// BR-METRICS-031: Dependency check duration histogram
		dependencyCheckDurationHistogram: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "llm_dependency_check_duration_seconds",
			Help:    "Duration of LLM dependency health checks in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}, []string{"dependency_name"}),

		// BR-METRICS-032: Dependency failures counter
		dependencyFailuresCounter: *prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "llm_dependency_failures_total",
			Help: "Total number of LLM dependency failures",
		}, []string{"dependency_name"}),

		// BR-METRICS-035: Monitoring accuracy gauge
		monitoringAccuracyGauge: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "llm_monitoring_accuracy_percentage",
			Help: "LLM monitoring accuracy percentage for BR-REL-011 compliance (must be >99%)",
		}, []string{"monitor_component"}),

		// BR-METRICS-036: Model parameter count gauge
		modelParameterCountGauge: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "llm_20b_model_parameter_count",
			Help: "LLM model parameter count for enterprise model validation (must be >=20B)",
		}, []string{"model_name"}),

		// BR-METRICS-037: SLA compliance gauge
		monitoringSLAComplianceGauge: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "llm_monitoring_sla_compliance",
			Help: "LLM monitoring SLA compliance gauge for 99.95% uptime tracking",
		}, []string{"sla_type"}),
	}

	// Register all metrics with the registry
	registry.MustRegister(
		&metrics.healthStatusGauge,
		&metrics.healthCheckDurationHistogram,
		&metrics.healthChecksTotalCounter,
		&metrics.consecutiveFailuresGauge,
		&metrics.uptimePercentageGauge,
		&metrics.probeDurationHistogram,
		&metrics.dependencyStatusGauge,
		&metrics.dependencyCheckDurationHistogram,
		&metrics.dependencyFailuresCounter,
		&metrics.monitoringAccuracyGauge,
		&metrics.modelParameterCountGauge,
		&metrics.monitoringSLAComplianceGauge,
	)

	return metrics
}

// RecordHealthStatus records health status following BR-METRICS-020
func (m *EnhancedHealthMetrics) RecordHealthStatus(healthStatus *types.HealthStatus) {
	if healthStatus.IsHealthy {
		m.healthStatusGauge.WithLabelValues(healthStatus.ComponentType).Set(1)
	} else {
		m.healthStatusGauge.WithLabelValues(healthStatus.ComponentType).Set(0)
	}
}

// RecordHealthCheckDuration records health check duration following BR-METRICS-021
func (m *EnhancedHealthMetrics) RecordHealthCheckDuration(componentType string, duration time.Duration) {
	m.healthCheckDurationHistogram.WithLabelValues(componentType).Observe(duration.Seconds())
}

// RecordHealthCheck records health check results following BR-METRICS-022
func (m *EnhancedHealthMetrics) RecordHealthCheck(componentType, status string) {
	m.healthChecksTotalCounter.WithLabelValues(componentType, status).Inc()
}

// RecordConsecutiveFailures records consecutive failure count following BR-METRICS-023
func (m *EnhancedHealthMetrics) RecordConsecutiveFailures(componentType string, count int) {
	m.consecutiveFailuresGauge.WithLabelValues(componentType).Set(float64(count))
}

// RecordUptimePercentage records uptime percentage following BR-METRICS-024
func (m *EnhancedHealthMetrics) RecordUptimePercentage(componentType string, percentage float64) {
	m.uptimePercentageGauge.WithLabelValues(componentType).Set(percentage)
}

// RecordProbeDuration records probe duration following BR-METRICS-025 & BR-METRICS-026
func (m *EnhancedHealthMetrics) RecordProbeDuration(probeType, componentID string, duration time.Duration) {
	m.probeDurationHistogram.WithLabelValues(probeType, componentID).Observe(duration.Seconds())
}

// RecordDependencyStatus records dependency status following BR-METRICS-030
func (m *EnhancedHealthMetrics) RecordDependencyStatus(dependencyName, criticality string, isAvailable bool) {
	value := 0.0
	if isAvailable {
		value = 1.0
	}
	m.dependencyStatusGauge.WithLabelValues(dependencyName, criticality).Set(value)
}

// RecordDependencyCheckDuration records dependency check duration following BR-METRICS-031
func (m *EnhancedHealthMetrics) RecordDependencyCheckDuration(dependencyName string, duration time.Duration) {
	m.dependencyCheckDurationHistogram.WithLabelValues(dependencyName).Observe(duration.Seconds())
}

// RecordDependencyFailure records dependency failure following BR-METRICS-032
func (m *EnhancedHealthMetrics) RecordDependencyFailure(dependencyName string) {
	m.dependencyFailuresCounter.WithLabelValues(dependencyName).Inc()
}

// RecordMonitoringAccuracy records monitoring accuracy following BR-METRICS-035
func (m *EnhancedHealthMetrics) RecordMonitoringAccuracy(monitorComponent string, accuracyPercentage float64) {
	m.monitoringAccuracyGauge.WithLabelValues(monitorComponent).Set(accuracyPercentage)
}

// RecordModelParameterCount records model parameter count following BR-METRICS-036
func (m *EnhancedHealthMetrics) RecordModelParameterCount(modelName string, parameterCount float64) {
	m.modelParameterCountGauge.WithLabelValues(modelName).Set(parameterCount)
}

// RecordSLACompliance records SLA compliance following BR-METRICS-037
func (m *EnhancedHealthMetrics) RecordSLACompliance(slaType string, compliancePercentage float64) {
	m.monitoringSLAComplianceGauge.WithLabelValues(slaType).Set(compliancePercentage)
}

// Getter methods for testing - following existing patterns in codebase

func (m *EnhancedHealthMetrics) GetHealthStatusGauge() *prometheus.GaugeVec {
	return &m.healthStatusGauge
}

func (m *EnhancedHealthMetrics) GetHealthCheckDurationHistogram() *prometheus.HistogramVec {
	return &m.healthCheckDurationHistogram
}

func (m *EnhancedHealthMetrics) GetHealthChecksTotalCounter() *prometheus.CounterVec {
	return &m.healthChecksTotalCounter
}

func (m *EnhancedHealthMetrics) GetConsecutiveFailuresGauge() *prometheus.GaugeVec {
	return &m.consecutiveFailuresGauge
}

func (m *EnhancedHealthMetrics) GetUptimePercentageGauge() *prometheus.GaugeVec {
	return &m.uptimePercentageGauge
}

func (m *EnhancedHealthMetrics) GetProbeDurationHistogram() *prometheus.HistogramVec {
	return &m.probeDurationHistogram
}

func (m *EnhancedHealthMetrics) GetDependencyStatusGauge() *prometheus.GaugeVec {
	return &m.dependencyStatusGauge
}

func (m *EnhancedHealthMetrics) GetDependencyCheckDurationHistogram() *prometheus.HistogramVec {
	return &m.dependencyCheckDurationHistogram
}

func (m *EnhancedHealthMetrics) GetDependencyFailuresCounter() *prometheus.CounterVec {
	return &m.dependencyFailuresCounter
}

func (m *EnhancedHealthMetrics) GetMonitoringAccuracyGauge() *prometheus.GaugeVec {
	return &m.monitoringAccuracyGauge
}

func (m *EnhancedHealthMetrics) GetModelParameterCountGauge() *prometheus.GaugeVec {
	return &m.modelParameterCountGauge
}

func (m *EnhancedHealthMetrics) GetSLAComplianceGauge() *prometheus.GaugeVec {
	return &m.monitoringSLAComplianceGauge
}
