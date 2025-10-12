package discovery

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/jordigilh/kubernaut/pkg/toolset"
	"github.com/jordigilh/kubernaut/pkg/toolset/health"
)

// PrometheusDetector detects Prometheus services in the cluster
// Business Requirement: BR-TOOLSET-010 - Prometheus service detection
type PrometheusDetector struct {
	healthChecker *health.HTTPHealthChecker
}

// NewPrometheusDetector creates a new Prometheus detector with default health checker
func NewPrometheusDetector() *PrometheusDetector {
	return NewPrometheusDetectorWithHealthChecker(health.NewHTTPHealthChecker())
}

// NewPrometheusDetectorWithHealthChecker creates a new Prometheus detector with custom health checker
func NewPrometheusDetectorWithHealthChecker(checker *health.HTTPHealthChecker) *PrometheusDetector {
	return &PrometheusDetector{
		healthChecker: checker,
	}
}

// ServiceType returns "prometheus"
func (d *PrometheusDetector) ServiceType() string {
	return "prometheus"
}

// Detect examines a service and returns a DiscoveredService if it matches Prometheus patterns
// BR-TOOLSET-010: Detection strategies:
// 1. Label-based: app=prometheus or app.kubernetes.io/name=prometheus
// 2. Name-based: service name contains "prometheus"
// 3. Port-based: port named "web" on 9090
func (d *PrometheusDetector) Detect(ctx context.Context, service *corev1.Service) (*toolset.DiscoveredService, error) {
	// Check if service matches Prometheus patterns
	if !d.isPrometheusService(service) {
		return nil, nil // Not a Prometheus service (not an error)
	}

	// Service has no ports - can't be a valid Prometheus service
	if len(service.Spec.Ports) == 0 {
		return nil, nil
	}

	// Build endpoint URL (BR-TOOLSET-011)
	port := d.findPrometheusPort(service)
	endpoint := d.buildEndpoint(service.Name, service.Namespace, port)

	// Create discovered service
	discovered := &toolset.DiscoveredService{
		Name:         service.Name,
		Namespace:    service.Namespace,
		Type:         "prometheus",
		Endpoint:     endpoint,
		Labels:       service.Labels,
		Annotations:  service.Annotations,
		Metadata:     make(map[string]string),
		Healthy:      false, // Will be updated by health check
		DiscoveredAt: time.Now(),
	}

	return discovered, nil
}

// isPrometheusService checks if the service matches Prometheus detection patterns
// Uses shared detector utilities for consistent detection logic
func (d *PrometheusDetector) isPrometheusService(service *corev1.Service) bool {
	return DetectByAnyStrategy(service,
		CreateLabelStrategy("prometheus"), // Strategy 1: Labels
		CreateNameStrategy("prometheus"),  // Strategy 2: Name
		CreatePortStrategy("web", 9090),   // Strategy 3: Port
	)
}

// findPrometheusPort determines the port to use for the Prometheus endpoint
// Uses shared GetPortNumber utility for consistent port resolution
func (d *PrometheusDetector) findPrometheusPort(service *corev1.Service) int32 {
	return GetPortNumber(service, []string{"web"}, 9090, 9090)
}

// buildEndpoint constructs the service endpoint URL in cluster.local format
// BR-TOOLSET-011: Endpoint URL construction
// Uses shared BuildEndpoint utility
func (d *PrometheusDetector) buildEndpoint(name, namespace string, port int32) string {
	return BuildEndpoint(name, namespace, port)
}

// HealthCheck validates that Prometheus is actually operational
// BR-TOOLSET-012: Health validation using Prometheus /-/healthy endpoint
// Uses shared HTTPHealthChecker for consistent retry logic and timeout handling
// If health checker is nil (integration tests), returns success immediately
func (d *PrometheusDetector) HealthCheck(ctx context.Context, endpoint string) error {
	if d.healthChecker == nil {
		return nil // Skip health check (integration tests)
	}
	// Prometheus health check endpoint: /-/healthy
	return d.healthChecker.CheckSimple(ctx, endpoint, "/-/healthy")
}
