package discovery

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/jordigilh/kubernaut/pkg/toolset"
)

// serviceDiscoverer implements the ServiceDiscoverer interface
// BR-TOOLSET-025: Service discovery orchestration with multiple detectors
type serviceDiscoverer struct {
	client    kubernetes.Interface
	detectors []ServiceDetector
	callback  ServiceDiscoveryCallback // BR-TOOLSET-026: Callback for ConfigMap integration
	mu        sync.RWMutex

	// Discovery loop control
	stopChan chan struct{}
	interval time.Duration
	stopped  bool
	running  bool // Guard against multiple concurrent Start() calls
}

// NewServiceDiscoverer creates a new service discoverer with the given Kubernetes client
func NewServiceDiscoverer(client kubernetes.Interface, interval time.Duration) ServiceDiscoverer {
	// Use provided interval, fallback to 5 minutes if zero
	if interval == 0 {
		interval = 5 * time.Minute
	}
	return &serviceDiscoverer{
		client:    client,
		detectors: make([]ServiceDetector, 0),
		interval:  interval,
		stopChan:  make(chan struct{}),
	}
}

// RegisterDetector adds a new service detector to the discovery pipeline
// Detectors are executed in registration order
func (d *serviceDiscoverer) RegisterDetector(detector ServiceDetector) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.detectors = append(d.detectors, detector)
}

// SetCallback registers a callback to be invoked when services are discovered
// BR-TOOLSET-026: Service discovery with ConfigMap integration
func (d *serviceDiscoverer) SetCallback(callback ServiceDiscoveryCallback) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.callback = callback
}

// DiscoverServices finds all detectable services in the cluster
// BR-TOOLSET-026: Service discovery with health validation
func (d *serviceDiscoverer) DiscoverServices(ctx context.Context) ([]toolset.DiscoveredService, error) {
	d.mu.RLock()
	detectors := d.detectors
	d.mu.RUnlock()

	// List all services across all namespaces
	services, err := d.client.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	// Discover services using registered detectors
	var discovered []toolset.DiscoveredService

	for i := range services.Items {
		service := &services.Items[i]

		// Try each detector until one matches
		for _, detector := range detectors {
			discoveredService, err := detector.Detect(ctx, service)
			if err != nil {
				// Log error but continue with other detectors
				log.Printf("detector %s error for service %s/%s: %v",
					detector.ServiceType(), service.Namespace, service.Name, err)
				continue
			}

			if discoveredService != nil {
				// Run health check
				if err := detector.HealthCheck(ctx, discoveredService.Endpoint); err != nil {
					// Service is unhealthy, skip it
					log.Printf("health check failed for %s/%s: %v",
						service.Namespace, service.Name, err)
					continue
				}

				// Service is healthy, add to discovered list
				discovered = append(discovered, *discoveredService)
				break // Stop after first matching detector
			}
		}
	}

	return discovered, nil
}

// Start begins the discovery loop
// This method blocks until Stop() is called or context is canceled
// BR-TOOLSET-026: Periodic discovery loop
//
// NOTE: This is a blocking method and should be launched in a goroutine
// if you need to do other work concurrently. Returns error if already running.
func (d *serviceDiscoverer) Start(ctx context.Context) error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return fmt.Errorf("discovery loop already running")
	}
	d.running = true
	d.mu.Unlock()

	// Ensure running flag is cleared when we exit
	defer func() {
		d.mu.Lock()
		d.running = false
		d.mu.Unlock()
	}()

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	// Run initial discovery
	discovered, err := d.DiscoverServices(ctx)
	if err != nil {
		log.Printf("initial discovery error: %v", err)
	} else {
		// Invoke callback with discovered services
		d.mu.RLock()
		callback := d.callback
		d.mu.RUnlock()

		if callback != nil {
			if err := callback(ctx, discovered); err != nil {
				log.Printf("discovery callback error: %v", err)
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-d.stopChan:
			return nil
		case <-ticker.C:
			// Run discovery
			discovered, err := d.DiscoverServices(ctx)
			if err != nil {
				log.Printf("discovery error: %v", err)
				continue
			}

			// Invoke callback with discovered services
			d.mu.RLock()
			callback := d.callback
			d.mu.RUnlock()

			if callback != nil {
				if err := callback(ctx, discovered); err != nil {
					log.Printf("discovery callback error: %v", err)
				}
			}
		}
	}
}

// Stop gracefully shuts down the discovery loop
func (d *serviceDiscoverer) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.stopped {
		close(d.stopChan)
		d.stopped = true
	}
	return nil
}
