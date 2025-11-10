package discovery

import (
	"context"

	"github.com/jordigilh/kubernaut/pkg/toolset"
)

// ServiceDiscoveryCallback is invoked when services are discovered
// This callback pattern enables integration with ConfigMap generation and other consumers
// BR-TOOLSET-026: Service discovery with ConfigMap integration
type ServiceDiscoveryCallback func(ctx context.Context, services []toolset.DiscoveredService) error

// ServiceDiscoverer discovers available Kubernetes services and generates toolset configurations
// Business Requirement: BR-TOOLSET-004 - Service discovery orchestration
//
// Design Note: Start/Stop methods are structured to enable trivial leader election addition
// in the future (estimated 1-2 day effort). This allows wrapping in leader election callbacks
// without changing the discovery logic.
type ServiceDiscoverer interface {
	// DiscoverServices finds all detectable services in the cluster
	// Returns a list of discovered services with health check results
	DiscoverServices(ctx context.Context) ([]toolset.DiscoveredService, error)

	// RegisterDetector adds a new service detector to the discovery pipeline
	// Detectors are executed in registration order
	RegisterDetector(detector ServiceDetector)

	// SetCallback registers a callback to be invoked when services are discovered
	// This enables integration with ConfigMap generation and other consumers
	// BR-TOOLSET-026: Service discovery with ConfigMap integration
	SetCallback(callback ServiceDiscoveryCallback)

	// Start begins the discovery loop (every 5 minutes by default)
	// This method blocks until Stop() is called or context is canceled
	// Design: Structured for future leader election wrapping
	Start(ctx context.Context) error

	// Stop gracefully shuts down the discovery loop
	// Design: Paired with Start() for clean lifecycle management
	Stop() error
}
