package holmesgpt

import (
	"context"
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
	"github.com/sirupsen/logrus"
)

// PrometheusToolsetGenerator generates toolsets for Prometheus services
// Business Requirement: BR-HOLMES-023 - Service-specific toolset generation
type PrometheusToolsetGenerator struct {
	log *logrus.Logger
}

// NewPrometheusToolsetGenerator creates a new Prometheus toolset generator
func NewPrometheusToolsetGenerator(log *logrus.Logger) *PrometheusToolsetGenerator {
	return &PrometheusToolsetGenerator{log: log}
}

// Generate generates a toolset configuration for a Prometheus service
func (ptg *PrometheusToolsetGenerator) Generate(ctx context.Context, service *k8s.DetectedService) (*ToolsetConfig, error) {
	if len(service.Endpoints) == 0 {
		return nil, fmt.Errorf("prometheus service has no endpoints")
	}

	endpoint := service.Endpoints[0] // Use first endpoint

	toolset := &ToolsetConfig{
		Name:        fmt.Sprintf("prometheus-%s-%s", service.Namespace, service.Name),
		ServiceType: "prometheus",
		Description: fmt.Sprintf("Prometheus metrics analysis tools for %s", endpoint.URL),
		Version:     "1.0.0",
		Endpoints: map[string]string{
			"query":       fmt.Sprintf("%s/api/v1/query", endpoint.URL),
			"query_range": fmt.Sprintf("%s/api/v1/query_range", endpoint.URL),
			"targets":     fmt.Sprintf("%s/api/v1/targets", endpoint.URL),
			"rules":       fmt.Sprintf("%s/api/v1/rules", endpoint.URL),
		},
		Capabilities: []string{
			"query_metrics",
			"alert_rules",
			"time_series",
			"resource_usage_analysis",
			"threshold_analysis",
		},
		Tools: []HolmesGPTTool{
			{
				Name:        "prometheus_query",
				Description: "Execute PromQL queries against Prometheus",
				Command:     fmt.Sprintf("curl -s '%s/api/v1/query?query=${query}'", endpoint.URL),
				Parameters: []ToolParameter{
					{Name: "query", Description: "PromQL query expression", Type: "string", Required: true},
				},
				Examples: []ToolExample{
					{
						Description: "Get CPU usage for all containers",
						Command:     "rate(container_cpu_usage_seconds_total[5m])",
						Expected:    "Returns CPU usage rate per container",
					},
					{
						Description: "Get memory usage for a specific pod",
						Command:     "container_memory_usage_bytes{pod=\"pod-name\"}",
						Expected:    "Returns memory usage for specified pod",
					},
				},
				Category: "metrics",
			},
			{
				Name:        "prometheus_range_query",
				Description: "Execute range queries for time series data",
				Command:     fmt.Sprintf("curl -s '%s/api/v1/query_range?query=${query}&start=${start}&end=${end}&step=${step}'", endpoint.URL),
				Parameters: []ToolParameter{
					{Name: "query", Description: "PromQL query expression", Type: "string", Required: true},
					{Name: "start", Description: "Start time (RFC3339 or Unix timestamp)", Type: "string", Required: true},
					{Name: "end", Description: "End time (RFC3339 or Unix timestamp)", Type: "string", Required: true},
					{Name: "step", Description: "Query resolution step", Type: "string", Default: "15s"},
				},
				Category: "metrics",
			},
			{
				Name:        "prometheus_targets",
				Description: "Get current target status from Prometheus",
				Command:     fmt.Sprintf("curl -s '%s/api/v1/targets'", endpoint.URL),
				Category:    "monitoring",
			},
			{
				Name:        "prometheus_rules",
				Description: "Get alerting and recording rules",
				Command:     fmt.Sprintf("curl -s '%s/api/v1/rules'", endpoint.URL),
				Category:    "alerting",
			},
		},
		HealthCheck: HealthCheckConfig{
			Endpoint: "/api/v1/status/buildinfo",
			Interval: 30 * time.Second,
			Timeout:  2 * time.Second,
			Retries:  3,
		},
		Priority: service.Priority,
		Enabled:  true,
		ServiceMeta: ServiceMetadata{
			Namespace:    service.Namespace,
			ServiceName:  service.Name,
			Labels:       service.Labels,
			Annotations:  service.Annotations,
			DiscoveredAt: time.Now(),
		},
	}

	return toolset, nil
}

// GetServiceType returns the service type handled by this generator
func (ptg *PrometheusToolsetGenerator) GetServiceType() string {
	return "prometheus"
}

// GetPriority returns the generator priority
func (ptg *PrometheusToolsetGenerator) GetPriority() int {
	return 80
}

// GrafanaToolsetGenerator generates toolsets for Grafana services
type GrafanaToolsetGenerator struct {
	log *logrus.Logger
}

// NewGrafanaToolsetGenerator creates a new Grafana toolset generator
func NewGrafanaToolsetGenerator(log *logrus.Logger) *GrafanaToolsetGenerator {
	return &GrafanaToolsetGenerator{log: log}
}

// Generate generates a toolset configuration for a Grafana service
func (gtg *GrafanaToolsetGenerator) Generate(ctx context.Context, service *k8s.DetectedService) (*ToolsetConfig, error) {
	if len(service.Endpoints) == 0 {
		return nil, fmt.Errorf("grafana service has no endpoints")
	}

	endpoint := service.Endpoints[0]

	toolset := &ToolsetConfig{
		Name:        fmt.Sprintf("grafana-%s-%s", service.Namespace, service.Name),
		ServiceType: "grafana",
		Description: fmt.Sprintf("Grafana dashboard and visualization tools for %s", endpoint.URL),
		Version:     "1.0.0",
		Endpoints: map[string]string{
			"api":         fmt.Sprintf("%s/api", endpoint.URL),
			"dashboards":  fmt.Sprintf("%s/api/search", endpoint.URL),
			"datasources": fmt.Sprintf("%s/api/datasources", endpoint.URL),
		},
		Capabilities: []string{
			"get_dashboards",
			"query_datasource",
			"get_alerts",
			"visualization",
			"dashboard_analysis",
		},
		Tools: []HolmesGPTTool{
			{
				Name:        "grafana_dashboards",
				Description: "List available Grafana dashboards",
				Command:     fmt.Sprintf("curl -s '%s/api/search?type=dash-db'", endpoint.URL),
				Category:    "visualization",
			},
			{
				Name:        "grafana_datasources",
				Description: "List configured data sources",
				Command:     fmt.Sprintf("curl -s '%s/api/datasources'", endpoint.URL),
				Category:    "configuration",
			},
			{
				Name:        "grafana_dashboard_info",
				Description: "Get detailed information about a dashboard",
				Command:     fmt.Sprintf("curl -s '%s/api/dashboards/uid/${dashboard_uid}'", endpoint.URL),
				Parameters: []ToolParameter{
					{Name: "dashboard_uid", Description: "Dashboard UID", Type: "string", Required: true},
				},
				Category: "visualization",
			},
		},
		HealthCheck: HealthCheckConfig{
			Endpoint: "/api/health",
			Interval: 30 * time.Second,
			Timeout:  2 * time.Second,
			Retries:  3,
		},
		Priority: service.Priority,
		Enabled:  true,
		ServiceMeta: ServiceMetadata{
			Namespace:    service.Namespace,
			ServiceName:  service.Name,
			Labels:       service.Labels,
			Annotations:  service.Annotations,
			DiscoveredAt: time.Now(),
		},
	}

	return toolset, nil
}

// GetServiceType returns the service type handled by this generator
func (gtg *GrafanaToolsetGenerator) GetServiceType() string {
	return "grafana"
}

// GetPriority returns the generator priority
func (gtg *GrafanaToolsetGenerator) GetPriority() int {
	return 70
}

// JaegerToolsetGenerator generates toolsets for Jaeger services
type JaegerToolsetGenerator struct {
	log *logrus.Logger
}

// NewJaegerToolsetGenerator creates a new Jaeger toolset generator
func NewJaegerToolsetGenerator(log *logrus.Logger) *JaegerToolsetGenerator {
	return &JaegerToolsetGenerator{log: log}
}

// Generate generates a toolset configuration for a Jaeger service
func (jtg *JaegerToolsetGenerator) Generate(ctx context.Context, service *k8s.DetectedService) (*ToolsetConfig, error) {
	if len(service.Endpoints) == 0 {
		return nil, fmt.Errorf("jaeger service has no endpoints")
	}

	endpoint := service.Endpoints[0]

	toolset := &ToolsetConfig{
		Name:        fmt.Sprintf("jaeger-%s-%s", service.Namespace, service.Name),
		ServiceType: "jaeger",
		Description: fmt.Sprintf("Jaeger distributed tracing analysis tools for %s", endpoint.URL),
		Version:     "1.0.0",
		Endpoints: map[string]string{
			"api":      fmt.Sprintf("%s/api", endpoint.URL),
			"traces":   fmt.Sprintf("%s/api/traces", endpoint.URL),
			"services": fmt.Sprintf("%s/api/services", endpoint.URL),
		},
		Capabilities: []string{
			"search_traces",
			"get_services",
			"analyze_latency",
			"distributed_tracing",
			"service_dependencies",
		},
		Tools: []HolmesGPTTool{
			{
				Name:        "jaeger_services",
				Description: "List all services tracked by Jaeger",
				Command:     fmt.Sprintf("curl -s '%s/api/services'", endpoint.URL),
				Category:    "tracing",
			},
			{
				Name:        "jaeger_traces",
				Description: "Search for traces by service and operation",
				Command:     fmt.Sprintf("curl -s '%s/api/traces?service=${service}&operation=${operation}&limit=${limit}'", endpoint.URL),
				Parameters: []ToolParameter{
					{Name: "service", Description: "Service name", Type: "string", Required: true},
					{Name: "operation", Description: "Operation name", Type: "string"},
					{Name: "limit", Description: "Maximum number of traces", Type: "int", Default: "20"},
				},
				Examples: []ToolExample{
					{
						Description: "Find traces for a specific service",
						Command:     "service=user-service&limit=10",
						Expected:    "Returns up to 10 traces for user-service",
					},
				},
				Category: "tracing",
			},
			{
				Name:        "jaeger_trace_detail",
				Description: "Get detailed information about a specific trace",
				Command:     fmt.Sprintf("curl -s '%s/api/traces/${trace_id}'", endpoint.URL),
				Parameters: []ToolParameter{
					{Name: "trace_id", Description: "Trace ID", Type: "string", Required: true},
				},
				Category: "tracing",
			},
		},
		HealthCheck: HealthCheckConfig{
			Endpoint: "/api/services",
			Interval: 30 * time.Second,
			Timeout:  2 * time.Second,
			Retries:  3,
		},
		Priority: service.Priority,
		Enabled:  true,
		ServiceMeta: ServiceMetadata{
			Namespace:    service.Namespace,
			ServiceName:  service.Name,
			Labels:       service.Labels,
			Annotations:  service.Annotations,
			DiscoveredAt: time.Now(),
		},
	}

	return toolset, nil
}

// GetServiceType returns the service type handled by this generator
func (jtg *JaegerToolsetGenerator) GetServiceType() string {
	return "jaeger"
}

// GetPriority returns the generator priority
func (jtg *JaegerToolsetGenerator) GetPriority() int {
	return 60
}

// ElasticsearchToolsetGenerator generates toolsets for Elasticsearch services
type ElasticsearchToolsetGenerator struct {
	log *logrus.Logger
}

// NewElasticsearchToolsetGenerator creates a new Elasticsearch toolset generator
func NewElasticsearchToolsetGenerator(log *logrus.Logger) *ElasticsearchToolsetGenerator {
	return &ElasticsearchToolsetGenerator{log: log}
}

// Generate generates a toolset configuration for an Elasticsearch service
func (etg *ElasticsearchToolsetGenerator) Generate(ctx context.Context, service *k8s.DetectedService) (*ToolsetConfig, error) {
	if len(service.Endpoints) == 0 {
		return nil, fmt.Errorf("elasticsearch service has no endpoints")
	}

	endpoint := service.Endpoints[0]

	toolset := &ToolsetConfig{
		Name:        fmt.Sprintf("elasticsearch-%s-%s", service.Namespace, service.Name),
		ServiceType: "elasticsearch",
		Description: fmt.Sprintf("Elasticsearch search and log analysis tools for %s", endpoint.URL),
		Version:     "1.0.0",
		Endpoints: map[string]string{
			"search":  fmt.Sprintf("%s/_search", endpoint.URL),
			"indices": fmt.Sprintf("%s/_cat/indices", endpoint.URL),
			"cluster": fmt.Sprintf("%s/_cluster/health", endpoint.URL),
		},
		Capabilities: []string{
			"search_logs",
			"analyze_patterns",
			"aggregation",
			"log_analysis",
			"full_text_search",
		},
		Tools: []HolmesGPTTool{
			{
				Name:        "elasticsearch_search",
				Description: "Search for logs and documents in Elasticsearch",
				Command:     fmt.Sprintf("curl -s -X POST '%s/_search' -H 'Content-Type: application/json' -d '${query}'", endpoint.URL),
				Parameters: []ToolParameter{
					{Name: "query", Description: "Elasticsearch query (JSON)", Type: "string", Required: true},
				},
				Examples: []ToolExample{
					{
						Description: "Search for error logs in the last hour",
						Command:     `{"query":{"bool":{"must":[{"match":{"level":"ERROR"}},{"range":{"@timestamp":{"gte":"now-1h"}}}]}}}`,
						Expected:    "Returns error logs from the last hour",
					},
				},
				Category: "search",
			},
			{
				Name:        "elasticsearch_indices",
				Description: "List all indices in Elasticsearch",
				Command:     fmt.Sprintf("curl -s '%s/_cat/indices?v'", endpoint.URL),
				Category:    "administration",
			},
			{
				Name:        "elasticsearch_cluster_health",
				Description: "Get cluster health status",
				Command:     fmt.Sprintf("curl -s '%s/_cluster/health'", endpoint.URL),
				Category:    "monitoring",
			},
		},
		HealthCheck: HealthCheckConfig{
			Endpoint: "/_cluster/health",
			Interval: 30 * time.Second,
			Timeout:  2 * time.Second,
			Retries:  3,
		},
		Priority: service.Priority,
		Enabled:  true,
		ServiceMeta: ServiceMetadata{
			Namespace:    service.Namespace,
			ServiceName:  service.Name,
			Labels:       service.Labels,
			Annotations:  service.Annotations,
			DiscoveredAt: time.Now(),
		},
	}

	return toolset, nil
}

// GetServiceType returns the service type handled by this generator
func (etg *ElasticsearchToolsetGenerator) GetServiceType() string {
	return "elasticsearch"
}

// GetPriority returns the generator priority
func (etg *ElasticsearchToolsetGenerator) GetPriority() int {
	return 50
}

// CustomToolsetGenerator generates toolsets for custom services with annotations
// Business Requirement: BR-HOLMES-018 - Custom service detection support
type CustomToolsetGenerator struct {
	log *logrus.Logger
}

// NewCustomToolsetGenerator creates a new custom toolset generator
func NewCustomToolsetGenerator(log *logrus.Logger) *CustomToolsetGenerator {
	return &CustomToolsetGenerator{log: log}
}

// Generate generates a toolset configuration for a custom service
func (ctg *CustomToolsetGenerator) Generate(ctx context.Context, service *k8s.DetectedService) (*ToolsetConfig, error) {
	if len(service.Endpoints) == 0 {
		return nil, fmt.Errorf("custom service has no endpoints")
	}

	// Get toolset type from annotation
	toolsetType := service.ServiceType
	if toolsetType == "custom" {
		// Use annotation value if available
		if service.Annotations != nil {
			if annotationType, exists := service.Annotations["kubernaut.io/toolset"]; exists {
				toolsetType = annotationType
			}
		}
	}

	endpoint := service.Endpoints[0]

	toolset := &ToolsetConfig{
		Name:         fmt.Sprintf("%s-%s-%s", toolsetType, service.Namespace, service.Name),
		ServiceType:  toolsetType,
		Description:  fmt.Sprintf("Custom %s tools for %s", toolsetType, endpoint.URL),
		Version:      "1.0.0",
		Endpoints:    ctg.buildEndpointsMap(service.Endpoints),
		Capabilities: service.Capabilities,
		Tools:        ctg.generateCustomTools(service),
		HealthCheck: HealthCheckConfig{
			Endpoint: "/health", // Default health check endpoint
			Interval: 30 * time.Second,
			Timeout:  2 * time.Second,
			Retries:  3,
		},
		Priority: service.Priority,
		Enabled:  true,
		ServiceMeta: ServiceMetadata{
			Namespace:    service.Namespace,
			ServiceName:  service.Name,
			Labels:       service.Labels,
			Annotations:  service.Annotations,
			DiscoveredAt: time.Now(),
		},
	}

	// Override health check if specified in annotations
	if healthEndpoint, exists := service.Annotations["kubernaut.io/health-endpoint"]; exists {
		toolset.HealthCheck.Endpoint = healthEndpoint
	}

	return toolset, nil
}

// buildEndpointsMap creates endpoints map from service endpoints
func (ctg *CustomToolsetGenerator) buildEndpointsMap(endpoints []k8s.ServiceEndpoint) map[string]string {
	endpointMap := make(map[string]string)
	for _, endpoint := range endpoints {
		endpointMap[endpoint.Name] = endpoint.URL
	}
	return endpointMap
}

// generateCustomTools generates tools based on service capabilities and endpoints
func (ctg *CustomToolsetGenerator) generateCustomTools(service *k8s.DetectedService) []HolmesGPTTool {
	var tools []HolmesGPTTool

	// Generate basic tools for each endpoint
	for _, endpoint := range service.Endpoints {
		tool := HolmesGPTTool{
			Name:        fmt.Sprintf("%s_api_call", endpoint.Name),
			Description: fmt.Sprintf("Make API call to %s endpoint", endpoint.Name),
			Command:     fmt.Sprintf("curl -s '%s/${path}'", endpoint.URL),
			Parameters: []ToolParameter{
				{Name: "path", Description: "API path", Type: "string", Default: ""},
			},
			Category: "api",
		}
		tools = append(tools, tool)
	}

	// Generate capability-specific tools
	for _, capability := range service.Capabilities {
		switch capability {
		case "query_logs":
			tools = append(tools, ctg.generateLogQueryTool(service))
		case "analyze_patterns":
			tools = append(tools, ctg.generatePatternAnalysisTool(service))
		case "custom_metric":
			tools = append(tools, ctg.generateMetricTool(service))
		}
	}

	return tools
}

// generateLogQueryTool generates a log query tool
func (ctg *CustomToolsetGenerator) generateLogQueryTool(service *k8s.DetectedService) HolmesGPTTool {
	endpoint := service.Endpoints[0]
	return HolmesGPTTool{
		Name:        "query_logs",
		Description: "Query logs from custom service",
		Command:     fmt.Sprintf("curl -s '%s/logs?query=${query}&limit=${limit}'", endpoint.URL),
		Parameters: []ToolParameter{
			{Name: "query", Description: "Log search query", Type: "string", Required: true},
			{Name: "limit", Description: "Maximum number of results", Type: "int", Default: "100"},
		},
		Category: "logs",
	}
}

// generatePatternAnalysisTool generates a pattern analysis tool
func (ctg *CustomToolsetGenerator) generatePatternAnalysisTool(service *k8s.DetectedService) HolmesGPTTool {
	endpoint := service.Endpoints[0]
	return HolmesGPTTool{
		Name:        "analyze_patterns",
		Description: "Analyze patterns in custom service data",
		Command:     fmt.Sprintf("curl -s '%s/patterns?timerange=${timerange}'", endpoint.URL),
		Parameters: []ToolParameter{
			{Name: "timerange", Description: "Time range for analysis", Type: "string", Default: "1h"},
		},
		Category: "analysis",
	}
}

// generateMetricTool generates a custom metric tool
func (ctg *CustomToolsetGenerator) generateMetricTool(service *k8s.DetectedService) HolmesGPTTool {
	endpoint := service.Endpoints[0]
	return HolmesGPTTool{
		Name:        "get_metrics",
		Description: "Get custom metrics from service",
		Command:     fmt.Sprintf("curl -s '%s/metrics'", endpoint.URL),
		Category:    "metrics",
	}
}

// GetServiceType returns the service type handled by this generator
func (ctg *CustomToolsetGenerator) GetServiceType() string {
	return "custom"
}

// GetPriority returns the generator priority
func (ctg *CustomToolsetGenerator) GetPriority() int {
	return 30 // Lower priority than well-known services
}
