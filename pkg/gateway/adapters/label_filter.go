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

package adapters

import (
	"strings"

	"github.com/go-logr/logr"
)

// LabelFilter determines whether a Prometheus alert label is monitoring
// infrastructure metadata (scrape config artifact) rather than an actual
// workload target. Used by extractTargetResource to skip misleading labels.
//
// Issue #191 / BR-GATEWAY-184: Prometheus scrape configuration injects labels
// like "service" and "job" that refer to the scraper, not the monitored resource.
type LabelFilter interface {
	// IsMonitoringMetadata returns true if the label should be skipped during
	// target resource extraction because it refers to monitoring infrastructure.
	IsMonitoringMetadata(labelKey, labelValue string) bool
}

// monitoringMetadataFilter implements LabelFilter using naming pattern matching.
//
// Only filters the "service" label key. All other label keys pass through.
// Matches known monitoring infrastructure service names (kube-prometheus-stack,
// Thanos, VictoriaMetrics, Grafana, Loki, Jaeger, etc.) via substring and
// prefix matching.
//
// SME-approved (#191): Covers 90%+ of cases. LLM's affectedResource field
// provides a safety net for edge cases.
type monitoringMetadataFilter struct {
	logger logr.Logger
}

// NewMonitoringMetadataFilter creates a filter that detects Prometheus scrape
// metadata labels by matching against known monitoring infrastructure naming patterns.
func NewMonitoringMetadataFilter(logger logr.Logger) LabelFilter {
	return &monitoringMetadataFilter{
		logger: logger.WithName("label-filter"),
	}
}

// IsMonitoringMetadata returns true if the label is a monitoring infrastructure
// artifact that should be skipped during target resource extraction.
//
// Only acts on the "service" label key. For all other keys, returns false.
//
// Naming patterns (SME-approved, #191):
//   - Substrings: prometheus, kube-state-metrics, alertmanager, grafana, thanos, exporter
//   - Prefixes: victoria, loki, jaeger
//   - Suffixes: -operator
func (f *monitoringMetadataFilter) IsMonitoringMetadata(labelKey, labelValue string) bool {
	if labelKey != "service" {
		return false
	}

	lower := strings.ToLower(labelValue)

	if isKnownMonitoringServiceName(lower) {
		f.logger.V(1).Info("Filtered monitoring metadata label",
			"labelKey", labelKey,
			"labelValue", labelValue,
			"reason", "naming_pattern",
		)
		return true
	}

	return false
}

// isKnownMonitoringServiceName checks if a lowercased service name matches
// known monitoring infrastructure naming patterns.
//
// Patterns are derived from common kube-prometheus-stack, VictoriaMetrics,
// Grafana LGTM stack, and Jaeger deployments. See SME review #191 for
// the full rationale and approved pattern list.
func isKnownMonitoringServiceName(lower string) bool {
	substrings := []string{
		"prometheus",
		"kube-state-metrics",
		"alertmanager",
		"grafana",
		"thanos",
		"exporter",
	}
	for _, s := range substrings {
		if strings.Contains(lower, s) {
			return true
		}
	}

	prefixes := []string{
		"victoria",
		"loki",
		"jaeger",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}

	if strings.HasSuffix(lower, "-operator") {
		return true
	}

	return false
}
