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

// Package metrics provides Prometheus metrics for the SignalProcessing controller.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds Prometheus metrics for the SignalProcessing controller.
type Metrics struct {
	ProcessingTotal    *prometheus.CounterVec
	ProcessingDuration *prometheus.HistogramVec
	EnrichmentErrors   *prometheus.CounterVec
}

// NewMetrics creates a new Metrics instance with the provided registry.
func NewMetrics(registry *prometheus.Registry) *Metrics {
	m := &Metrics{
		ProcessingTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "signalprocessing_processing_total",
				Help: "Total number of signal processing operations",
			},
			[]string{"phase", "result"},
		),
		ProcessingDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "signalprocessing_processing_duration_seconds",
				Help:    "Duration of signal processing operations",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"phase"},
		),
		EnrichmentErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "signalprocessing_enrichment_errors_total",
				Help: "Total number of enrichment errors",
			},
			[]string{"error_type"},
		),
	}

	registry.MustRegister(m.ProcessingTotal)
	registry.MustRegister(m.ProcessingDuration)
	registry.MustRegister(m.EnrichmentErrors)

	return m
}
