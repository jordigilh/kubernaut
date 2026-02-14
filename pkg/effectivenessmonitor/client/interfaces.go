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

// Package client defines interfaces for external dependencies of the Effectiveness Monitor.
// These interfaces enable dependency injection and testability across all tiers.
//
// Integration tests use httptest.NewServer mocks (per TESTING_GUIDELINES.md Section 4a).
// E2E tests use real Prometheus/AlertManager containers (per DD-TEST-001).
//
// Business Requirements:
// - BR-EM-001: Health check via K8s readiness/liveness
// - BR-EM-002: Alert resolution check via AlertManager
// - BR-EM-003: Metric comparison via Prometheus
// - BR-EM-004: Spec hash comparison via K8s API
package client

import (
	"context"
	"time"
)

// PrometheusQuerier abstracts Prometheus query operations.
// Used for metric comparison scoring (BR-EM-003).
//
// Integration tests: httptest.NewServer mock (test/infrastructure/prometheus_mock.go)
// E2E tests: real Prometheus container (test/infrastructure/prometheus_alertmanager_e2e.go)
type PrometheusQuerier interface {
	// Query executes an instant PromQL query and returns the result.
	Query(ctx context.Context, query string, ts time.Time) (*QueryResult, error)

	// QueryRange executes a range PromQL query and returns the result.
	QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) (*QueryResult, error)

	// Ready checks if Prometheus is ready to accept queries.
	Ready(ctx context.Context) error
}

// AlertManagerClient abstracts AlertManager API operations.
// Used for alert resolution scoring (BR-EM-002).
//
// Integration tests: httptest.NewServer mock (test/infrastructure/alertmanager_mock.go)
// E2E tests: real AlertManager container (test/infrastructure/prometheus_alertmanager_e2e.go)
type AlertManagerClient interface {
	// GetAlerts retrieves active alerts matching the given filters.
	GetAlerts(ctx context.Context, filters AlertFilters) ([]Alert, error)

	// Ready checks if AlertManager is ready to accept queries.
	Ready(ctx context.Context) error
}

// DataStorageQuerier abstracts queries to the DataStorage audit trail.
// Used by the EM to retrieve the pre-remediation spec hash from the
// remediation.workflow_created audit event (DD-EM-002).
type DataStorageQuerier interface {
	// QueryPreRemediationHash queries DS for the pre-remediation spec hash
	// associated with a given correlation ID. Returns empty string if not found.
	QueryPreRemediationHash(ctx context.Context, correlationID string) (string, error)
}

// ========================================
// Prometheus Types
// ========================================

// QueryResult represents the result of a Prometheus query.
type QueryResult struct {
	// Samples contains the data points returned by the query.
	Samples []Sample
}

// Sample represents a single data point from Prometheus.
type Sample struct {
	// Metric is the metric name and labels.
	Metric map[string]string
	// Value is the sample value.
	Value float64
	// Timestamp is the sample timestamp.
	Timestamp time.Time
}

// ========================================
// AlertManager Types
// ========================================

// AlertFilters defines criteria for querying alerts from AlertManager.
type AlertFilters struct {
	// Matchers are label matchers to filter alerts (e.g., alertname=~"HighLatency").
	Matchers []string
}

// Alert represents an alert from AlertManager.
type Alert struct {
	// Labels are the alert labels.
	Labels map[string]string
	// State is the alert state (active, suppressed, unprocessed).
	State string
	// StartsAt is when the alert started firing.
	StartsAt time.Time
	// EndsAt is when the alert stopped firing (zero if still active).
	EndsAt time.Time
}
