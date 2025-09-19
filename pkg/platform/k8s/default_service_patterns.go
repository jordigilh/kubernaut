package k8s

import "time"

// GetDefaultServicePatterns returns default service detection patterns
// Business Requirement: BR-HOLMES-017 - Well-known service detection patterns
// Following development guideline: reuse existing code and avoid duplication
func GetDefaultServicePatterns() map[string]ServicePattern {
	return map[string]ServicePattern{
		"prometheus": {
			Enabled: true,
			Selectors: []map[string]string{
				{"app.kubernetes.io/name": "prometheus"},
				{"app": "prometheus"},
			},
			ServiceNames:  []string{"prometheus", "prometheus-server"},
			RequiredPorts: []int32{9090},
			HealthCheck: HealthCheckConfig{
				Endpoint: "/api/v1/status/buildinfo",
				Timeout:  2 * time.Second,
				Retries:  3,
				Method:   "GET",
			},
			Priority: 80,
			Capabilities: []string{
				"query_metrics",
				"alert_rules",
				"time_series",
				"resource_usage_analysis",
			},
		},
		"grafana": {
			Enabled: true,
			Selectors: []map[string]string{
				{"app.kubernetes.io/name": "grafana"},
			},
			ServiceNames:  []string{"grafana"},
			RequiredPorts: []int32{3000},
			HealthCheck: HealthCheckConfig{
				Endpoint: "/api/health",
				Timeout:  2 * time.Second,
				Retries:  3,
				Method:   "GET",
			},
			Priority: 70,
			Capabilities: []string{
				"get_dashboards",
				"query_datasource",
				"get_alerts",
				"visualization",
			},
		},
		"jaeger": {
			Enabled: true,
			Selectors: []map[string]string{
				{"app.kubernetes.io/name": "jaeger"},
			},
			ServiceNames:  []string{"jaeger-query"},
			RequiredPorts: []int32{16686},
			HealthCheck: HealthCheckConfig{
				Endpoint: "/api/services",
				Timeout:  2 * time.Second,
				Retries:  3,
				Method:   "GET",
			},
			Priority: 60,
			Capabilities: []string{
				"search_traces",
				"get_services",
				"analyze_latency",
				"distributed_tracing",
			},
		},
		"elasticsearch": {
			Enabled: true,
			Selectors: []map[string]string{
				{"app.kubernetes.io/name": "elasticsearch"},
			},
			ServiceNames:  []string{"elasticsearch", "elasticsearch-master"},
			RequiredPorts: []int32{9200},
			HealthCheck: HealthCheckConfig{
				Endpoint: "/_cluster/health",
				Timeout:  2 * time.Second,
				Retries:  3,
				Method:   "GET",
			},
			Priority: 50,
			Capabilities: []string{
				"search_logs",
				"analyze_patterns",
				"aggregation",
				"log_analysis",
				"full_text_search",
			},
		},
		"custom": {
			Enabled:  true,
			Priority: 30,
		},
	}
}
