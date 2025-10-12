package discovery

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/jordigilh/kubernaut/pkg/toolset"
	"github.com/jordigilh/kubernaut/pkg/toolset/health"
)

// ElasticsearchDetector detects Elasticsearch services in the cluster
// Business Requirement: BR-TOOLSET-019 - Elasticsearch service detection
type ElasticsearchDetector struct {
	healthChecker *health.HTTPHealthChecker
}

// NewElasticsearchDetector creates a new Elasticsearch detector with default health checker
func NewElasticsearchDetector() *ElasticsearchDetector {
	return NewElasticsearchDetectorWithHealthChecker(health.NewHTTPHealthChecker())
}

// NewElasticsearchDetectorWithHealthChecker creates a new Elasticsearch detector with custom health checker
func NewElasticsearchDetectorWithHealthChecker(checker *health.HTTPHealthChecker) *ElasticsearchDetector {
	return &ElasticsearchDetector{
		healthChecker: checker,
	}
}

// ServiceType returns "elasticsearch"
func (d *ElasticsearchDetector) ServiceType() string {
	return "elasticsearch"
}

// Detect examines a service and returns a DiscoveredService if it matches Elasticsearch patterns
// BR-TOOLSET-019: Detection strategies:
// 1. Label-based: app=elasticsearch or app.kubernetes.io/name=elasticsearch
// 2. Name-based: service name contains "elasticsearch"
// 3. Port-based: port 9200 (HTTP API)
func (d *ElasticsearchDetector) Detect(ctx context.Context, service *corev1.Service) (*toolset.DiscoveredService, error) {
	// Check if service matches Elasticsearch patterns
	if !d.isElasticsearchService(service) {
		return nil, nil // Not an Elasticsearch service (not an error)
	}

	// Service has no ports - can't be a valid Elasticsearch service
	if len(service.Spec.Ports) == 0 {
		return nil, nil
	}

	// Build endpoint URL (BR-TOOLSET-020)
	port := d.findElasticsearchPort(service)
	endpoint := d.buildEndpoint(service.Name, service.Namespace, port)

	// Create discovered service
	discovered := &toolset.DiscoveredService{
		Name:         service.Name,
		Namespace:    service.Namespace,
		Type:         "elasticsearch",
		Endpoint:     endpoint,
		Labels:       service.Labels,
		Annotations:  service.Annotations,
		Metadata:     make(map[string]string),
		Healthy:      false, // Will be updated by health check
		DiscoveredAt: time.Now(),
	}

	return discovered, nil
}

// isElasticsearchService checks if the service matches Elasticsearch detection patterns
// Uses shared detector utilities for consistent detection logic
func (d *ElasticsearchDetector) isElasticsearchService(service *corev1.Service) bool {
	return DetectByAnyStrategy(service,
		CreateLabelStrategy("elasticsearch"), // Strategy 1: Labels
		CreateNameStrategy("elasticsearch"),  // Strategy 2: Name
		CreatePortStrategy("", 9200),         // Strategy 3: Port (no specific name, just 9200)
	)
}

// findElasticsearchPort determines the port to use for the Elasticsearch endpoint
// Uses shared GetPortNumber utility for consistent port resolution
func (d *ElasticsearchDetector) findElasticsearchPort(service *corev1.Service) int32 {
	return GetPortNumber(service, []string{"http", "api"}, 9200, 9200)
}

// buildEndpoint constructs the service endpoint URL in cluster.local format
// BR-TOOLSET-020: Endpoint URL construction
// Uses shared BuildEndpoint utility
func (d *ElasticsearchDetector) buildEndpoint(name, namespace string, port int32) string {
	return BuildEndpoint(name, namespace, port)
}

// HealthCheck validates that Elasticsearch is actually operational
// BR-TOOLSET-021: Health validation using Elasticsearch /_cluster/health endpoint
// Uses shared HTTPHealthChecker for consistent retry logic and timeout handling
// If health checker is nil (integration tests), returns success immediately
func (d *ElasticsearchDetector) HealthCheck(ctx context.Context, endpoint string) error {
	if d.healthChecker == nil {
		return nil // Skip health check (integration tests)
	}
	// Elasticsearch health check endpoint: /_cluster/health
	return d.healthChecker.CheckSimple(ctx, endpoint, "/_cluster/health")
}
