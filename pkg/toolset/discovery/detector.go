package discovery

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	"github.com/jordigilh/kubernaut/pkg/toolset"
)

// ServiceDetector detects a specific type of service (Prometheus, Grafana, etc.)
// Business Requirement: BR-TOOLSET-003 - Pluggable service detection
type ServiceDetector interface {
	// Detect examines a single service and returns a DiscoveredService if it matches this detector's criteria
	// Returns nil if the service does not match (not an error condition)
	// Returns error only for actual failures during detection
	Detect(ctx context.Context, service *corev1.Service) (*toolset.DiscoveredService, error)

	// ServiceType returns the type identifier (e.g., "prometheus", "grafana")
	ServiceType() string

	// HealthCheck validates that the service is actually operational
	// Takes the service endpoint URL and returns an error if unhealthy
	HealthCheck(ctx context.Context, endpoint string) error
}
