package discovery

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/jordigilh/kubernaut/pkg/toolset"
	"github.com/jordigilh/kubernaut/pkg/toolset/health"
)

// JaegerDetector detects Jaeger services in the cluster
// Business Requirement: BR-TOOLSET-016 - Jaeger service detection
type JaegerDetector struct {
	healthChecker *health.HTTPHealthChecker
}

// NewJaegerDetector creates a new Jaeger detector with default health checker
func NewJaegerDetector() *JaegerDetector {
	return NewJaegerDetectorWithHealthChecker(health.NewHTTPHealthChecker())
}

// NewJaegerDetectorWithHealthChecker creates a new Jaeger detector with custom health checker
func NewJaegerDetectorWithHealthChecker(checker *health.HTTPHealthChecker) *JaegerDetector {
	return &JaegerDetector{
		healthChecker: checker,
	}
}

// ServiceType returns "jaeger"
func (d *JaegerDetector) ServiceType() string {
	return "jaeger"
}

// Detect examines a service and returns a DiscoveredService if it matches Jaeger patterns
// BR-TOOLSET-016: Detection strategies:
// 1. Label-based: app=jaeger or app.kubernetes.io/name=jaeger
// 2. Name-based: service name contains "jaeger"
// 3. Port-based: port named "query" on 16686
func (d *JaegerDetector) Detect(ctx context.Context, service *corev1.Service) (*toolset.DiscoveredService, error) {
	// Check if service matches Jaeger patterns
	if !d.isJaegerService(service) {
		return nil, nil // Not a Jaeger service (not an error)
	}

	// Service has no ports - can't be a valid Jaeger service
	if len(service.Spec.Ports) == 0 {
		return nil, nil
	}

	// Build endpoint URL (BR-TOOLSET-017)
	port := d.findJaegerPort(service)
	endpoint := d.buildEndpoint(service.Name, service.Namespace, port)

	// Create discovered service
	discovered := &toolset.DiscoveredService{
		Name:         service.Name,
		Namespace:    service.Namespace,
		Type:         "jaeger",
		Endpoint:     endpoint,
		Labels:       service.Labels,
		Annotations:  service.Annotations,
		Metadata:     make(map[string]string),
		Healthy:      false, // Will be updated by health check
		DiscoveredAt: time.Now(),
	}

	return discovered, nil
}

// isJaegerService checks if the service matches Jaeger detection patterns
// Uses shared detector utilities for consistent detection logic
func (d *JaegerDetector) isJaegerService(service *corev1.Service) bool {
	return DetectByAnyStrategy(service,
		CreateLabelStrategy("jaeger"),      // Strategy 1: Labels
		CreateNameStrategy("jaeger"),       // Strategy 2: Name
		CreatePortStrategy("query", 16686), // Strategy 3: Port
	)
}

// findJaegerPort determines the port to use for the Jaeger endpoint
// Uses shared GetPortNumber utility for consistent port resolution
func (d *JaegerDetector) findJaegerPort(service *corev1.Service) int32 {
	return GetPortNumber(service, []string{"query", "ui"}, 16686, 16686)
}

// buildEndpoint constructs the service endpoint URL in cluster.local format
// BR-TOOLSET-017: Endpoint URL construction
// Uses shared BuildEndpoint utility
func (d *JaegerDetector) buildEndpoint(name, namespace string, port int32) string {
	return BuildEndpoint(name, namespace, port)
}

// HealthCheck validates that Jaeger is actually operational
// BR-TOOLSET-018: Health validation using Jaeger / endpoint
// Uses shared HTTPHealthChecker for consistent retry logic and timeout handling
// If health checker is nil (integration tests), returns success immediately
func (d *JaegerDetector) HealthCheck(ctx context.Context, endpoint string) error {
	if d.healthChecker == nil {
		return nil // Skip health check (integration tests)
	}
	// Jaeger health check endpoint: / (UI root endpoint)
	return d.healthChecker.CheckSimple(ctx, endpoint, "/")
}
