/*
Copyright 2026 Jordi Gil.

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

package registry

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	MetricNameReconcileTotal   = "fleet_registry_reconcile_total"
	MetricNameReconcileErrors  = "fleet_registry_reconcile_errors_total"
	MetricNameClustersActive   = "fleet_registry_clusters_active"
	MetricNameEventDropTotal   = "fleet_registry_event_drop_total"
)

// Metrics holds Prometheus metrics for the BackendInformerRegistry.
// DD-005 Compliant: {service}_{metric_name}_{unit} naming.
// DD-METRICS-001 Compliant: Dependency-injected, nil-safe.
type Metrics struct {
	ReconcileTotal  prometheus.Counter
	ReconcileErrors prometheus.Counter
	ClustersActive  prometheus.Gauge
	EventDropTotal  prometheus.Counter
}

var (
	defaultMetrics     *Metrics
	defaultMetricsOnce sync.Once
)

// NewMetrics creates Metrics registered with the default Prometheus registerer.
func NewMetrics() *Metrics {
	defaultMetricsOnce.Do(func() {
		defaultMetrics = NewMetricsWithRegistry(prometheus.DefaultRegisterer)
	})
	return defaultMetrics
}

// NewMetricsWithRegistry creates Metrics with a custom registerer (for testing).
func NewMetricsWithRegistry(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		ReconcileTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: MetricNameReconcileTotal,
			Help: "Total number of reconcile cycles for cluster registry",
		}),
		ReconcileErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: MetricNameReconcileErrors,
			Help: "Total number of failed reconcile cycles for cluster registry",
		}),
		ClustersActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: MetricNameClustersActive,
			Help: "Number of currently active managed clusters",
		}),
		EventDropTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: MetricNameEventDropTotal,
			Help: "Total number of events dropped due to full subscriber channel",
		}),
	}
	reg.MustRegister(m.ReconcileTotal, m.ReconcileErrors, m.ClustersActive, m.EventDropTotal)
	return m
}

// NilSafeIncReconcile increments reconcile total (nil-safe for testing).
func (m *Metrics) NilSafeIncReconcile() {
	if m != nil && m.ReconcileTotal != nil {
		m.ReconcileTotal.Inc()
	}
}

// NilSafeIncReconcileError increments reconcile error total (nil-safe).
func (m *Metrics) NilSafeIncReconcileError() {
	if m != nil && m.ReconcileErrors != nil {
		m.ReconcileErrors.Inc()
	}
}

// NilSafeSetClusters sets the active cluster gauge (nil-safe).
func (m *Metrics) NilSafeSetClusters(n float64) {
	if m != nil && m.ClustersActive != nil {
		m.ClustersActive.Set(n)
	}
}

// NilSafeIncEventDrop increments event drop counter (nil-safe).
func (m *Metrics) NilSafeIncEventDrop() {
	if m != nil && m.EventDropTotal != nil {
		m.EventDropTotal.Inc()
	}
}
