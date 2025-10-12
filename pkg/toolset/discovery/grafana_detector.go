package discovery

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/jordigilh/kubernaut/pkg/toolset"
	"github.com/jordigilh/kubernaut/pkg/toolset/health"
)

// GrafanaDetector detects Grafana services in the cluster
// Business Requirement: BR-TOOLSET-013 - Grafana service detection
type GrafanaDetector struct {
	healthChecker *health.HTTPHealthChecker
}

// NewGrafanaDetector creates a new Grafana detector with default health checker
func NewGrafanaDetector() *GrafanaDetector {
	return NewGrafanaDetectorWithHealthChecker(health.NewHTTPHealthChecker())
}

// NewGrafanaDetectorWithHealthChecker creates a new Grafana detector with custom health checker
func NewGrafanaDetectorWithHealthChecker(checker *health.HTTPHealthChecker) *GrafanaDetector {
	return &GrafanaDetector{
		healthChecker: checker,
	}
}

// ServiceType returns "grafana"
func (d *GrafanaDetector) ServiceType() string {
	return "grafana"
}

// Detect examines a service and returns a DiscoveredService if it matches Grafana patterns
// BR-TOOLSET-013: Detection strategies:
// 1. Label-based: app=grafana or app.kubernetes.io/name=grafana
// 2. Name-based: service name contains "grafana"
// 3. Port-based: port named "service" on 3000
func (d *GrafanaDetector) Detect(ctx context.Context, service *corev1.Service) (*toolset.DiscoveredService, error) {
	// Check if service matches Grafana patterns
	if !d.isGrafanaService(service) {
		return nil, nil // Not a Grafana service (not an error)
	}

	// Service has no ports - can't be a valid Grafana service
	if len(service.Spec.Ports) == 0 {
		return nil, nil
	}

	// Build endpoint URL (BR-TOOLSET-014)
	port := d.findGrafanaPort(service)
	endpoint := d.buildEndpoint(service.Name, service.Namespace, port)

	// Create discovered service
	discovered := &toolset.DiscoveredService{
		Name:         service.Name,
		Namespace:    service.Namespace,
		Type:         "grafana",
		Endpoint:     endpoint,
		Labels:       service.Labels,
		Annotations:  service.Annotations,
		Metadata:     make(map[string]string),
		Healthy:      false, // Will be updated by health check
		DiscoveredAt: time.Now(),
	}

	return discovered, nil
}

// isGrafanaService checks if the service matches Grafana detection patterns
// Uses shared detector utilities for consistent detection logic
func (d *GrafanaDetector) isGrafanaService(service *corev1.Service) bool {
	return DetectByAnyStrategy(service,
		CreateLabelStrategy("grafana"),      // Strategy 1: Labels
		CreateNameStrategy("grafana"),       // Strategy 2: Name
		CreatePortStrategy("service", 3000), // Strategy 3: Port
	)
}

// findGrafanaPort determines the port to use for the Grafana endpoint
// Uses shared GetPortNumber utility for consistent port resolution
func (d *GrafanaDetector) findGrafanaPort(service *corev1.Service) int32 {
	return GetPortNumber(service, []string{"service", "http"}, 3000, 3000)
}

// buildEndpoint constructs the service endpoint URL in cluster.local format
// BR-TOOLSET-014: Endpoint URL construction
// Uses shared BuildEndpoint utility
func (d *GrafanaDetector) buildEndpoint(name, namespace string, port int32) string {
	return BuildEndpoint(name, namespace, port)
}

// HealthCheck validates that Grafana is actually operational
// BR-TOOLSET-015: Health validation using Grafana /api/health endpoint
// Uses shared HTTPHealthChecker for consistent retry logic and timeout handling
// If health checker is nil (integration tests), returns success immediately
func (d *GrafanaDetector) HealthCheck(ctx context.Context, endpoint string) error {
	if d.healthChecker == nil {
		return nil // Skip health check (integration tests)
	}
	// Grafana health check endpoint: /api/health
	return d.healthChecker.CheckSimple(ctx, endpoint, "/api/health")
}
