package toolset

import "time"

// DiscoveredService represents a service discovered in the cluster
// Business Requirement: BR-TOOLSET-001 - Service discovery and cataloging
type DiscoveredService struct {
	// Name is the Kubernetes service name
	Name string `json:"name"`

	// Namespace is the Kubernetes namespace containing the service
	Namespace string `json:"namespace"`

	// Type is the service type (prometheus, grafana, jaeger, elasticsearch, custom)
	Type string `json:"type"`

	// Endpoint is the full service endpoint URL
	// Example: http://prometheus.monitoring.svc.cluster.local:9090
	Endpoint string `json:"endpoint"`

	// Labels are the Kubernetes service labels
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations are the Kubernetes service annotations
	Annotations map[string]string `json:"annotations,omitempty"`

	// Metadata contains additional service-specific information
	Metadata map[string]string `json:"metadata,omitempty"`

	// Healthy indicates if the service passed health checks
	Healthy bool `json:"healthy"`

	// LastCheck is the timestamp of the last health check
	LastCheck time.Time `json:"last_check"`

	// DiscoveredAt is the timestamp when the service was first discovered
	DiscoveredAt time.Time `json:"discovered_at"`
}

// ToolsetConfig represents a generated toolset configuration
// Business Requirement: BR-TOOLSET-002 - Toolset configuration generation
type ToolsetConfig struct {
	// Toolset is the toolset name (kubernetes, prometheus, grafana, etc.)
	Toolset string `yaml:"toolset" json:"toolset"`

	// Enabled indicates if the toolset is enabled
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Config contains toolset-specific configuration
	Config map[string]interface{} `yaml:"config" json:"config"`
}

// DiscoveryMetadata contains metadata about the discovery process
type DiscoveryMetadata struct {
	// LastDiscovery is the timestamp of the last discovery run
	LastDiscovery time.Time `json:"last_discovery"`

	// ServiceCount is the number of services discovered
	ServiceCount int `json:"service_count"`

	// Duration is how long the discovery took
	Duration time.Duration `json:"duration"`
}

// ToolsetResponse represents a discovered toolset in API responses
// BR-TOOLSET-039: Toolset API response format with filtering support
type ToolsetResponse struct {
	Name            string                 `json:"name"`
	Type            string                 `json:"type"`
	Enabled         bool                   `json:"enabled"`
	Healthy         bool                   `json:"healthy"`
	Config          map[string]interface{} `json:"config"`
	ServiceEndpoint string                 `json:"serviceEndpoint,omitempty"`
	DiscoveredAt    string                 `json:"discoveredAt"`
	LastHealthCheck string                 `json:"lastHealthCheck,omitempty"`
}

// ToolsetsListResponse represents the response for GET /api/v1/toolsets
// BR-TOOLSET-039: List toolsets with optional filtering
type ToolsetsListResponse struct {
	Toolsets         []ToolsetResponse `json:"toolsets"`
	Total            int               `json:"total"`
	ConfigMapVersion string            `json:"configMapVersion,omitempty"`
	LastDiscovery    string            `json:"lastDiscovery,omitempty"`
}

// ValidationError represents a single validation error
// BR-TOOLSET-042: Validation error details
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationResponse represents the response for POST /api/v1/toolsets/validate
// BR-TOOLSET-042: Toolset validation response
type ValidationResponse struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors,omitempty"`
}
