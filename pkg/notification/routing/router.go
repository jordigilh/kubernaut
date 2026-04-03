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

package routing

import (
	"fmt"
	"strings"
	"sync"

	"github.com/go-logr/logr"
)

// Router provides thread-safe routing configuration management with hot-reload support.
// BR-NOT-067: Hot-reload routing configuration without service restart.
type Router struct {
	mu     sync.RWMutex
	config *Config
	logger logr.Logger
}

// NewRouter creates a new Router with the given logger.
func NewRouter(logger logr.Logger) *Router {
	return &Router{
		logger: logger,
		config: DefaultConfig(),
	}
}

// LoadConfig loads and validates routing configuration from YAML bytes.
// BR-NOT-067: Routing table updated without restart
//
// If the new configuration is invalid, the old configuration is preserved.
// This ensures in-flight notifications are not affected by bad config changes.
func (r *Router) LoadConfig(data []byte) error {
	// Parse and validate new config before acquiring lock
	newConfig, err := ParseConfig(data)
	if err != nil {
		r.logger.Error(err, "Failed to parse routing configuration, keeping existing config",
			"dataLength", len(data))
		return fmt.Errorf("failed to parse routing configuration: %w", err)
	}

	// Acquire write lock and swap config
	r.mu.Lock()
	oldConfig := r.config
	r.config = newConfig
	r.mu.Unlock()

	// Log config change with before/after summary
	// BR-NOT-067: Config reload logged with before/after diff
	oldSummary := summarizeConfig(oldConfig)
	newSummary := summarizeConfig(newConfig)

	r.logger.Info("Routing configuration reloaded",
		"oldConfig", oldSummary,
		"newConfig", newSummary,
		"receiversCount", len(newConfig.Receivers),
	)

	return nil
}

// GetConfig returns a copy of the current routing configuration.
// Thread-safe read access.
func (r *Router) GetConfig() *Config {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.config
}

// FindReceivers finds all matching receivers for the given routing attributes,
// respecting the Continue flag for multi-receiver fanout (BR-NOT-068).
// Returns resolved []*Receiver objects (not just names).
// Falls back to default console receiver when config is nil, no routes match,
// or resolved receiver names don't exist in the config.
// Thread-safe: acquires RLock for the duration of the call.
func (r *Router) FindReceivers(attrs map[string]string) []*Receiver {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.config == nil || r.config.Route == nil {
		r.logger.V(1).Info("No routing configuration, using default console fallback")
		return []*Receiver{defaultConsoleReceiver()}
	}

	names := r.config.Route.FindReceivers(attrs)
	if len(names) == 0 {
		r.logger.V(1).Info("No matching routes found, using default console fallback",
			"attributes", attrs)
		return []*Receiver{defaultConsoleReceiver()}
	}

	var receivers []*Receiver
	for _, name := range names {
		if recv := r.config.GetReceiver(name); recv != nil {
			receivers = append(receivers, recv)
		}
	}
	if len(receivers) == 0 {
		return []*Receiver{defaultConsoleReceiver()}
	}

	r.logger.V(1).Info("Resolved receivers from routing rules",
		"attributes", attrs,
		"receivers", names,
	)

	return receivers
}

// FindReceiver finds the matching receiver for the given routing attributes.
// BR-NOT-065: Attribute-based routing with ordered evaluation
// Thread-safe read access.
func (r *Router) FindReceiver(attrs map[string]string) *Receiver {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.config == nil || r.config.Route == nil {
		r.logger.V(1).Info("No routing configuration, using default console fallback")
		return defaultConsoleReceiver()
	}

	receiverName := r.config.Route.FindReceiver(attrs)
	if receiverName == "" {
		r.logger.V(1).Info("No matching route found, using default console fallback",
			"attributes", attrs)
		return defaultConsoleReceiver()
	}

	receiver := r.config.GetReceiver(receiverName)
	if receiver == nil {
		r.logger.Error(nil, "Receiver not found in config",
			"receiverName", receiverName,
			"attributes", attrs)
		return defaultConsoleReceiver()
	}

	r.logger.V(1).Info("Resolved receiver from routing rules",
		"attributes", attrs,
		"receiver", receiverName,
	)

	return receiver
}

// GetConfigSummary returns a human-readable summary of the current configuration.
// BR-NOT-067: Config reload logged with before/after diff
func (r *Router) GetConfigSummary() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return summarizeConfig(r.config)
}

// summarizeConfig creates a human-readable summary of a Config.
func summarizeConfig(config *Config) string {
	if config == nil {
		return "nil"
	}

	var receivers []string
	for _, r := range config.Receivers {
		receivers = append(receivers, r.Name)
	}

	routeCount := countRoutes(config.Route)
	defaultReceiver := ""
	if config.Route != nil {
		defaultReceiver = config.Route.Receiver
	}

	return fmt.Sprintf("default=%s, routes=%d, receivers=[%s]",
		defaultReceiver, routeCount, strings.Join(receivers, ", "))
}

// countRoutes recursively counts the number of routes in the routing tree.
func countRoutes(route *Route) int {
	if route == nil {
		return 0
	}
	count := 1
	for _, child := range route.Routes {
		count += countRoutes(child)
	}
	return count
}

// defaultConsoleReceiver returns a fallback console receiver.
func defaultConsoleReceiver() *Receiver {
	return &Receiver{
		Name: "default-console-fallback",
		ConsoleConfigs: []ConsoleConfig{
			{Enabled: true},
		},
	}
}

