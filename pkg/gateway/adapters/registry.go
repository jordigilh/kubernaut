/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package adapters

import (
	"fmt"
	"sync"

	"github.com/go-logr/logr"
)

// AdapterRegistry manages signal adapter registration
//
// This registry provides:
// 1. Configuration-driven adapter registration (not hardcoded)
// 2. Thread-safe adapter lookup during HTTP request handling
// 3. Adapter discovery for HTTP route registration at server startup
//
// Design Decision: Simplified registry (no detection logic)
// - Removed from original Design A: CanHandle(), Priority(), DetectAndSelect()
// - Adapters are selected by HTTP route, not payload detection
// - ~60% less code than detection-based approach
//
// Usage:
//
//	registry := NewAdapterRegistry(logger)
//	registry.Register(NewPrometheusAdapter(nil, nil))
//	registry.Register(NewKubernetesEventAdapter())
//
//	// HTTP server iterates over adapters to register routes
//	for _, adapter := range registry.GetAllAdapters() {
//	    mux.HandleFunc(adapter.GetRoute(), makeHandler(adapter))
//	}
type AdapterRegistry struct {
	// adapters maps adapter name to adapter instance
	// Key: adapter.Name() (e.g., "prometheus", "kubernetes-event")
	// Value: RoutableAdapter implementation
	adapters map[string]RoutableAdapter

	// mu protects concurrent access to adapters map
	// Read-heavy workload: GetAdapter() called for every HTTP request
	// Write-once workload: Register() called only at server startup
	mu sync.RWMutex

	// log is used for adapter registration logging
	log logr.Logger
}

// NewAdapterRegistry creates an empty adapter registry
//
// Adapters must be registered explicitly using Register() before
// the HTTP server starts. This allows configuration-driven adapter
// enablement (enable/disable adapters via config files).
func NewAdapterRegistry(log logr.Logger) *AdapterRegistry {
	return &AdapterRegistry{
		adapters: make(map[string]RoutableAdapter),
		log:      log.WithName("adapter-registry"),
	}
}

// Register adds an adapter to the registry
//
// This method must be called at server startup (before HTTP server starts)
// to register all enabled adapters. Registration after startup is not supported.
//
// Registration is idempotent-safe: registering the same adapter twice returns an error.
//
// Returns:
// - error: If adapter with same name already registered (prevents accidental overwrites)
func (r *AdapterRegistry) Register(adapter RoutableAdapter) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := adapter.Name()

	// Check for duplicate registration
	if _, exists := r.adapters[name]; exists {
		return fmt.Errorf("adapter '%s' already registered", name)
	}

	// Add adapter to registry
	r.adapters[name] = adapter

	// Log registration for debugging
	metadata := adapter.GetMetadata()
	r.log.Info("Adapter registered",
		"adapter", name,
		"route", adapter.GetRoute(),
		"version", metadata.Version,
		"description", metadata.Description,
	)

	return nil
}

// GetAdapter retrieves an adapter by name
//
// This method is used for adapter lookup during HTTP request handling
// (read-heavy workload). Uses RLock for concurrent read access.
//
// Returns:
// - RoutableAdapter: The adapter instance
// - bool: true if found, false if not registered
func (r *AdapterRegistry) GetAdapter(name string) (RoutableAdapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	adapter, exists := r.adapters[name]
	return adapter, exists
}

// GetAllAdapters returns all registered adapters
//
// This method is used at server startup to:
// 1. Register HTTP routes for each adapter (adapter.GetRoute())
// 2. Log all enabled adapters
// 3. Generate API documentation
//
// Returns a copy of the adapter slice to prevent external modification.
//
// Returns:
// - []RoutableAdapter: Slice of all registered adapters
func (r *AdapterRegistry) GetAllAdapters() []RoutableAdapter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	adapters := make([]RoutableAdapter, 0, len(r.adapters))
	for _, adapter := range r.adapters {
		adapters = append(adapters, adapter)
	}

	return adapters
}

// Count returns the number of registered adapters
//
// This method is used for:
// - Startup validation (ensure at least 1 adapter registered)
// - Metrics (gauge for registered adapter count)
// - Testing
//
// Returns:
// - int: Number of registered adapters
func (r *AdapterRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.adapters)
}
