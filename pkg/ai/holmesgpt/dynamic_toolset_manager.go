<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package holmesgpt

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
	"github.com/sirupsen/logrus"
)

// DynamicToolsetManager manages dynamic toolset configuration based on discovered services
// Business Requirement: BR-HOLMES-022 - Generate appropriate toolset configurations
type DynamicToolsetManager struct {
	serviceDiscovery *k8s.ServiceDiscovery
	templateEngine   *ToolsetTemplateEngine
	configCache      *ToolsetConfigCache
	generators       map[string]ToolsetGenerator
	eventHandlers    []ToolsetEventHandler
	log              *logrus.Logger
	mu               sync.RWMutex
	stopChannel      chan struct{}
}

// ToolsetConfig represents a dynamic toolset configuration for HolmesGPT
// Business Requirement: BR-HOLMES-022 - Service-specific toolset configurations
type ToolsetConfig struct {
	Name         string            `json:"name"`
	ServiceType  string            `json:"service_type"`
	Description  string            `json:"description"`
	Version      string            `json:"version"`
	Endpoints    map[string]string `json:"endpoints"`
	Capabilities []string          `json:"capabilities"`
	Tools        []HolmesGPTTool   `json:"tools"`
	HealthCheck  HealthCheckConfig `json:"health_check"`
	Priority     int               `json:"priority"`
	Enabled      bool              `json:"enabled"`
	ServiceMeta  ServiceMetadata   `json:"service_meta"`
	LastUpdated  time.Time         `json:"last_updated"`
}

// HolmesGPTTool represents a tool definition for HolmesGPT
type HolmesGPTTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Command     string          `json:"command"`
	Parameters  []ToolParameter `json:"parameters,omitempty"`
	Examples    []ToolExample   `json:"examples,omitempty"`
	Category    string          `json:"category"`
}

// ToolParameter represents a tool parameter
type ToolParameter struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"` // "string", "int", "bool", "array"
	Required    bool   `json:"required"`
	Default     string `json:"default,omitempty"`
}

// ToolExample represents a tool usage example
type ToolExample struct {
	Description string `json:"description"`
	Command     string `json:"command"`
	Expected    string `json:"expected"`
}

// ServiceMetadata contains metadata about the discovered service
type ServiceMetadata struct {
	Namespace    string            `json:"namespace"`
	ServiceName  string            `json:"service_name"`
	Labels       map[string]string `json:"labels,omitempty"`
	Annotations  map[string]string `json:"annotations,omitempty"`
	DiscoveredAt time.Time         `json:"discovered_at"`
}

// HealthCheckConfig represents health check configuration for toolsets
type HealthCheckConfig struct {
	Endpoint string        `json:"endpoint"`
	Interval time.Duration `json:"interval"`
	Timeout  time.Duration `json:"timeout"`
	Retries  int           `json:"retries"`
}

// ToolsetGenerator interface for generating service-specific toolsets
type ToolsetGenerator interface {
	Generate(ctx context.Context, service *k8s.DetectedService) (*ToolsetConfig, error)
	GetServiceType() string
	GetPriority() int
}

// ToolsetEventHandler interface for handling toolset configuration changes
// Business Requirement: BR-HOLMES-020 - Real-time toolset configuration updates
type ToolsetEventHandler interface {
	OnToolsetAdded(config *ToolsetConfig) error
	OnToolsetUpdated(config *ToolsetConfig) error
	OnToolsetRemoved(config *ToolsetConfig) error
}

// NewDynamicToolsetManager creates a new dynamic toolset manager
// Following development guideline: reuse existing patterns
func NewDynamicToolsetManager(
	serviceDiscovery *k8s.ServiceDiscovery,
	log *logrus.Logger,
) *DynamicToolsetManager {
	dtm := &DynamicToolsetManager{
		serviceDiscovery: serviceDiscovery,
		templateEngine:   NewToolsetTemplateEngine(log),
		configCache:      NewToolsetConfigCache(10*time.Minute, log),
		generators:       make(map[string]ToolsetGenerator),
		eventHandlers:    make([]ToolsetEventHandler, 0),
		log:              log,
		stopChannel:      make(chan struct{}),
	}

	// Register default toolset generators
	// Business Requirement: BR-HOLMES-023 - Toolset configuration templates
	dtm.registerDefaultGenerators()

	return dtm
}

// Start starts the dynamic toolset manager
func (dtm *DynamicToolsetManager) Start(ctx context.Context) error {
	dtm.log.Info("Starting dynamic toolset manager")

	// Initial toolset generation from discovered services
	if err := dtm.generateInitialToolsets(ctx); err != nil {
		dtm.log.WithError(err).Error("Failed to generate initial toolsets")
		return fmt.Errorf("failed to generate initial toolsets: %w", err)
	}

	// Start listening for service discovery events
	go dtm.handleServiceEvents(ctx)

	dtm.log.Info("Dynamic toolset manager started successfully")
	return nil
}

// Stop stops the dynamic toolset manager
func (dtm *DynamicToolsetManager) Stop() {
	dtm.log.Info("Stopping dynamic toolset manager")

	// Safely close stop channel only once to prevent panic
	select {
	case <-dtm.stopChannel:
		// Channel already closed
		dtm.log.Debug("Dynamic toolset manager already stopped")
		return
	default:
		// Channel not closed, safe to close it
		close(dtm.stopChannel)
		dtm.log.Debug("Dynamic toolset manager stop channel closed")
	}
}

// GetAvailableToolsets returns all currently available toolsets
// Business Requirement: BR-HOLMES-025 - Toolset configuration API endpoints
func (dtm *DynamicToolsetManager) GetAvailableToolsets() []*ToolsetConfig {
	toolsets := dtm.configCache.GetAllToolsets()

	// If no toolsets are cached (e.g., manager not started), return baseline toolsets
	// This ensures the business logic works correctly in all states
	if len(toolsets) == 0 {
		dtm.log.Debug("No cached toolsets found, generating baseline toolsets")
		baselineToolsets := dtm.generateBaselineToolsets()

		// Cache the baseline toolsets for future calls
		for _, toolset := range baselineToolsets {
			if toolset != nil {
				dtm.configCache.SetToolset(toolset)
			}
		}

		return baselineToolsets
	}

	return toolsets
}

// GetToolsetByServiceType returns toolsets for a specific service type
func (dtm *DynamicToolsetManager) GetToolsetByServiceType(serviceType string) []*ToolsetConfig {
	return dtm.configCache.GetToolsetsByType(serviceType)
}

// GetToolsetConfig returns a specific toolset configuration
func (dtm *DynamicToolsetManager) GetToolsetConfig(name string) *ToolsetConfig {
	return dtm.configCache.GetToolset(name)
}

// RefreshAllToolsets forces regeneration of all toolsets from discovered services
// Business Requirement: BR-HOLMES-025 - Runtime toolset management
func (dtm *DynamicToolsetManager) RefreshAllToolsets(ctx context.Context) error {
	dtm.log.Info("Refreshing all toolsets from discovered services")

	// Get all discovered services
	discoveredServices := dtm.serviceDiscovery.GetDiscoveredServices()

	// Process each discovered service to regenerate its toolset
	for _, service := range discoveredServices {
		if err := dtm.handleServiceUpdated(ctx, service); err != nil {
			dtm.log.WithError(err).WithFields(logrus.Fields{
				"service":      service.Name,
				"service_type": service.ServiceType,
			}).Error("Failed to refresh toolset for service")
			// Continue with other services rather than failing completely
		}
	}

	dtm.log.WithField("services_processed", len(discoveredServices)).Info("Toolset refresh completed")
	return nil
}

// AddEventHandler adds a toolset event handler
func (dtm *DynamicToolsetManager) AddEventHandler(handler ToolsetEventHandler) {
	dtm.mu.Lock()
	defer dtm.mu.Unlock()
	dtm.eventHandlers = append(dtm.eventHandlers, handler)
}

// RegisterGenerator registers a toolset generator for a service type
func (dtm *DynamicToolsetManager) RegisterGenerator(generator ToolsetGenerator) {
	if generator == nil {
		return
	}

	dtm.mu.Lock()
	defer dtm.mu.Unlock()

	dtm.generators[generator.GetServiceType()] = generator
	dtm.log.WithField("service_type", generator.GetServiceType()).Debug("Registered toolset generator")
}

// generateInitialToolsets generates toolsets from currently discovered services
// Removed backwards compatibility - implements proper error handling following project guidelines
func (dtm *DynamicToolsetManager) generateInitialToolsets(ctx context.Context) error {
	discoveredServices := dtm.serviceDiscovery.GetDiscoveredServices()

	dtm.log.WithField("service_count", len(discoveredServices)).Info("Generating initial toolsets")

	// Always include baseline toolsets (Kubernetes, internet)
	// Business Requirement: BR-HOLMES-028 - Maintain baseline toolsets
	baselineToolsets := dtm.generateBaselineToolsets()
	if len(baselineToolsets) == 0 {
		return fmt.Errorf("failed to generate baseline toolsets")
	}

	for _, toolset := range baselineToolsets {
		if toolset == nil {
			return fmt.Errorf("baseline toolset generation returned nil toolset")
		}
		dtm.configCache.SetToolset(toolset)
		dtm.notifyEventHandlers("added", toolset)
	}

	// Generate toolsets for discovered services
	var generatedCount int
	var generationErrors []string

	for _, service := range discoveredServices {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if service.Available {
			toolset, err := dtm.generateToolsetForService(ctx, service)
			if err != nil {
				errorMsg := fmt.Sprintf("failed to generate toolset for service %s: %v", service.Name, err)
				generationErrors = append(generationErrors, errorMsg)
				dtm.log.WithError(err).WithField("service", service.Name).Error("Failed to generate toolset for service")
				continue
			}

			if toolset != nil {
				dtm.configCache.SetToolset(toolset)
				dtm.notifyEventHandlers("added", toolset)
				generatedCount++
			}
		}
	}

	dtm.log.WithFields(logrus.Fields{
		"generated_count": generatedCount,
		"error_count":     len(generationErrors),
		"baseline_count":  len(baselineToolsets),
	}).Info("Initial toolset generation completed")

	// Return error if we have baseline toolsets but failed to generate any service toolsets from available services
	availableServices := 0
	for _, service := range discoveredServices {
		if service.Available {
			availableServices++
		}
	}

	if availableServices > 0 && generatedCount == 0 && len(generationErrors) > 0 {
		return fmt.Errorf("failed to generate toolsets for any of %d available services: %v", availableServices, generationErrors)
	}

	// Log warnings for partial failures but don't fail the entire operation
	if len(generationErrors) > 0 {
		dtm.log.WithField("generation_errors", generationErrors).Warn("Some service toolset generations failed")
	}

	return nil
}

// generateToolsetForService generates a toolset for a specific service
func (dtm *DynamicToolsetManager) generateToolsetForService(ctx context.Context, service *k8s.DetectedService) (*ToolsetConfig, error) {
	dtm.mu.RLock()
	generator, exists := dtm.generators[service.ServiceType]
	dtm.mu.RUnlock()

	if !exists {
		dtm.log.WithField("service_type", service.ServiceType).Debug("No generator found for service type")
		return nil, nil
	}

	toolset, err := generator.Generate(ctx, service)
	if err != nil {
		return nil, fmt.Errorf("failed to generate toolset for service %s: %w", service.Name, err)
	}

	if toolset != nil {
		toolset.LastUpdated = time.Now()
		dtm.log.WithFields(logrus.Fields{
			"service":      service.Name,
			"service_type": service.ServiceType,
			"toolset_name": toolset.Name,
		}).Debug("Generated toolset for service")
	}

	return toolset, nil
}

// handleServiceEvents listens for service discovery events and updates toolsets
// Business Requirement: BR-HOLMES-020 - Real-time toolset configuration updates
func (dtm *DynamicToolsetManager) handleServiceEvents(ctx context.Context) {
	eventChannel := dtm.serviceDiscovery.GetEventChannel()

	for {
		select {
		case event := <-eventChannel:
			if err := dtm.processServiceEvent(ctx, event); err != nil {
				dtm.log.WithError(err).WithField("event_type", event.Type).Error("Failed to process service event")
			}

		case <-dtm.stopChannel:
			return
		case <-ctx.Done():
			return
		}
	}
}

// processServiceEvent processes a single service event
func (dtm *DynamicToolsetManager) processServiceEvent(ctx context.Context, event k8s.ServiceEvent) error {
	switch event.Type {
	case "created", "updated":
		return dtm.handleServiceUpdated(ctx, event.Service)
	case "deleted":
		return dtm.handleServiceDeleted(event.Service)
	default:
		dtm.log.WithField("event_type", event.Type).Debug("Unknown service event type")
		return nil
	}
}

// handleServiceUpdated handles service creation or update events
func (dtm *DynamicToolsetManager) handleServiceUpdated(ctx context.Context, service *k8s.DetectedService) error {
	if !service.Available {
		// If service is not available, remove its toolset
		return dtm.handleServiceDeleted(service)
	}

	// Generate or update toolset for the service
	toolset, err := dtm.generateToolsetForService(ctx, service)
	if err != nil {
		return fmt.Errorf("failed to generate toolset for updated service: %w", err)
	}

	if toolset == nil {
		return nil // No toolset generated for this service type
	}

	// Check if toolset already exists
	existingToolset := dtm.configCache.GetToolset(toolset.Name)

	dtm.configCache.SetToolset(toolset)

	if existingToolset == nil {
		// New toolset
		dtm.log.WithFields(logrus.Fields{
			"service":      service.Name,
			"toolset_name": toolset.Name,
		}).Info("Added new toolset for service")
		dtm.notifyEventHandlers("added", toolset)
	} else {
		// Updated toolset
		dtm.log.WithFields(logrus.Fields{
			"service":      service.Name,
			"toolset_name": toolset.Name,
		}).Info("Updated toolset for service")
		dtm.notifyEventHandlers("updated", toolset)
	}

	return nil
}

// handleServiceDeleted handles service deletion events
func (dtm *DynamicToolsetManager) handleServiceDeleted(service *k8s.DetectedService) error {
	// Find and remove toolsets for this service
	toolsetName := dtm.getToolsetNameForService(service)
	existingToolset := dtm.configCache.GetToolset(toolsetName)

	if existingToolset != nil {
		dtm.configCache.RemoveToolset(toolsetName)
		dtm.log.WithFields(logrus.Fields{
			"service":      service.Name,
			"toolset_name": toolsetName,
		}).Info("Removed toolset for deleted service")
		dtm.notifyEventHandlers("removed", existingToolset)
	}

	return nil
}

// getToolsetNameForService generates a toolset name for a service
func (dtm *DynamicToolsetManager) getToolsetNameForService(service *k8s.DetectedService) string {
	return fmt.Sprintf("%s-%s-%s", service.ServiceType, service.Namespace, service.Name)
}

// notifyEventHandlers notifies all registered event handlers
func (dtm *DynamicToolsetManager) notifyEventHandlers(eventType string, toolset *ToolsetConfig) {
	dtm.mu.RLock()
	handlers := dtm.eventHandlers
	dtm.mu.RUnlock()

	for _, handler := range handlers {
		var err error
		switch eventType {
		case "added":
			err = handler.OnToolsetAdded(toolset)
		case "updated":
			err = handler.OnToolsetUpdated(toolset)
		case "removed":
			err = handler.OnToolsetRemoved(toolset)
		}

		if err != nil {
			dtm.log.WithError(err).WithFields(logrus.Fields{
				"event_type":   eventType,
				"toolset_name": toolset.Name,
			}).Error("Event handler failed")
		}
	}
}

// generateBaselineToolsets generates baseline toolsets that are always available
// Business Requirement: BR-HOLMES-028 - Maintain baseline toolsets
func (dtm *DynamicToolsetManager) generateBaselineToolsets() []*ToolsetConfig {
	// Define all available baseline toolsets
	baselineTemplates := map[string]*ToolsetConfig{
		"kubernetes": {
			Name:        "kubernetes",
			ServiceType: "kubernetes",
			Description: "Kubernetes cluster investigation tools",
			Version:     "1.0.0",
			Capabilities: []string{
				"get_pods",
				"get_services",
				"get_deployments",
				"get_events",
				"describe_resources",
				"get_logs",
			},
			ServiceMeta: ServiceMetadata{
				Namespace:    "kube-system",
				ServiceName:  "kubernetes",
				Labels:       map[string]string{"component": "apiserver", "provider": "kubernetes"},
				Annotations:  map[string]string{"toolset.holmesgpt.io/baseline": "true"},
				DiscoveredAt: time.Now(),
			},
			Tools: []HolmesGPTTool{
				{
					Name:        "get_pods",
					Description: "Get pods in a namespace",
					Command:     "kubectl get pods -n ${namespace} -o json",
					Parameters: []ToolParameter{
						{Name: "namespace", Description: "Kubernetes namespace", Type: "string", Required: true},
					},
					Category: "kubernetes",
				},
				{
					Name:        "get_pod_logs",
					Description: "Get logs from a pod",
					Command:     "kubectl logs -n ${namespace} ${pod_name} --tail=${lines}",
					Parameters: []ToolParameter{
						{Name: "namespace", Description: "Kubernetes namespace", Type: "string", Required: true},
						{Name: "pod_name", Description: "Pod name", Type: "string", Required: true},
						{Name: "lines", Description: "Number of lines", Type: "int", Default: "100"},
					},
					Category: "kubernetes",
				},
				{
					Name:        "describe_pod",
					Description: "Describe a pod with detailed information",
					Command:     "kubectl describe pod -n ${namespace} ${pod_name}",
					Parameters: []ToolParameter{
						{Name: "namespace", Description: "Kubernetes namespace", Type: "string", Required: true},
						{Name: "pod_name", Description: "Pod name", Type: "string", Required: true},
					},
					Category: "kubernetes",
				},
			},
			Priority:    100,
			Enabled:     true,
			LastUpdated: time.Now(),
		},
		"internet": {
			Name:        "internet",
			ServiceType: "internet",
			Description: "Internet connectivity and external API tools",
			Version:     "1.0.0",
			Capabilities: []string{
				"web_search",
				"documentation_lookup",
				"api_status_check",
			},
			ServiceMeta: ServiceMetadata{
				Namespace:    "default",
				ServiceName:  "internet",
				Labels:       map[string]string{"component": "external", "provider": "internet"},
				Annotations:  map[string]string{"toolset.holmesgpt.io/baseline": "true"},
				DiscoveredAt: time.Now(),
			},
			Tools: []HolmesGPTTool{
				{
					Name:        "check_external_api",
					Description: "Check external API availability",
					Command:     "curl -s -o /dev/null -w '%{http_code}' ${url}",
					Parameters: []ToolParameter{
						{Name: "url", Description: "External URL to check", Type: "string", Required: true},
					},
					Category: "connectivity",
				},
			},
			Priority:    10,
			Enabled:     true,
			LastUpdated: time.Now(),
		},
		"prometheus": {
			Name:        "prometheus",
			ServiceType: "prometheus",
			Description: "Prometheus metrics analysis tools",
			Version:     "1.0.0",
			Capabilities: []string{
				"prometheus_query",
				"prometheus_range_query",
				"prometheus_targets",
			},
			Tools: []HolmesGPTTool{
				{
					Name:        "prometheus_query",
					Description: "Execute PromQL queries",
					Command:     "curl -s '${endpoint}/api/v1/query?query=${query}'",
					Parameters: []ToolParameter{
						{Name: "endpoint", Description: "Prometheus endpoint", Type: "string", Required: true},
						{Name: "query", Description: "PromQL query", Type: "string", Required: true},
					},
					Category: "monitoring",
				},
			},
			Priority:    50,
			Enabled:     true,
			LastUpdated: time.Now(),
		},
		"grafana": {
			Name:        "grafana",
			ServiceType: "grafana",
			Description: "Grafana dashboard and visualization tools",
			Version:     "1.0.0",
			Capabilities: []string{
				"grafana_dashboards",
				"grafana_datasources",
			},
			Tools: []HolmesGPTTool{
				{
					Name:        "grafana_dashboards",
					Description: "List available dashboards",
					Command:     "curl -s '${endpoint}/api/search?type=dash-db'",
					Parameters: []ToolParameter{
						{Name: "endpoint", Description: "Grafana endpoint", Type: "string", Required: true},
					},
					Category: "visualization",
				},
			},
			Priority:    40,
			Enabled:     true,
			LastUpdated: time.Now(),
		},
	}

	// Return baseline toolsets from configuration
	// Following project guidelines: follow configuration-driven baseline toolsets
	// Exclude services that have dynamic generators to avoid conflicts
	dynamicallyDiscoverableServices := map[string]bool{
		"grafana":       true, // Has GrafanaToolsetGenerator
		"prometheus":    true, // Has PrometheusToolsetGenerator
		"jaeger":        true, // Has JaegerToolsetGenerator
		"elasticsearch": true, // Has ElasticsearchToolsetGenerator
	}

	baselineNames := []string{"kubernetes", "internet", "prometheus", "grafana"}

	var baselineToolsets []*ToolsetConfig

	// Check if we have any discovered services for dynamically discoverable types
	discoveredServices := dtm.serviceDiscovery.GetDiscoveredServices()
	discoveredServiceTypes := make(map[string]bool)
	for _, service := range discoveredServices {
		if service.Available {
			discoveredServiceTypes[service.ServiceType] = true
		}
	}

	for _, name := range baselineNames {
		// Skip baseline for services that can be dynamically discovered AND are actually discovered
		// This prevents conflicts between baseline and service-discovered toolsets
		if dynamicallyDiscoverableServices[name] && discoveredServiceTypes[name] {
			dtm.log.WithField("service_type", name).Debug("Skipping baseline toolset - service dynamically discovered")
			continue
		}

		if template, exists := baselineTemplates[name]; exists {
			// Add ServiceMeta for dynamically discoverable services that weren't discovered
			if dynamicallyDiscoverableServices[name] && !discoveredServiceTypes[name] {
				template.ServiceMeta = ServiceMetadata{
					Namespace:    "monitoring",
					ServiceName:  name,
					Labels:       map[string]string{"component": name, "provider": "baseline"},
					Annotations:  map[string]string{"toolset.holmesgpt.io/baseline": "true", "toolset.holmesgpt.io/fallback": "true"},
					DiscoveredAt: time.Now(),
				}
			}
			baselineToolsets = append(baselineToolsets, template)
		} else {
			dtm.log.WithField("toolset_name", name).Warn("Baseline toolset template not found")
		}
	}

	dtm.log.WithField("baseline_count", len(baselineToolsets)).Info("Generated baseline toolsets")
	return baselineToolsets
}

// registerDefaultGenerators registers the default toolset generators
func (dtm *DynamicToolsetManager) registerDefaultGenerators() {
	dtm.RegisterGenerator(NewPrometheusToolsetGenerator(dtm.log))
	dtm.RegisterGenerator(NewGrafanaToolsetGenerator(dtm.log))
	dtm.RegisterGenerator(NewJaegerToolsetGenerator(dtm.log))
	dtm.RegisterGenerator(NewElasticsearchToolsetGenerator(dtm.log))
	dtm.RegisterGenerator(NewCustomToolsetGenerator(dtm.log))
}

// GetToolsetStats returns statistics about managed toolsets
// Business Requirement: BR-HOLMES-029 - Service discovery metrics and monitoring
func (dtm *DynamicToolsetManager) GetToolsetStats() ToolsetStats {
	allToolsets := dtm.configCache.GetAllToolsets()

	stats := ToolsetStats{
		TotalToolsets: len(allToolsets),
		EnabledCount:  0,
		TypeCounts:    make(map[string]int),
		LastUpdate:    time.Time{},
	}

	for _, toolset := range allToolsets {
		if toolset.Enabled {
			stats.EnabledCount++
		}

		stats.TypeCounts[toolset.ServiceType]++

		if toolset.LastUpdated.After(stats.LastUpdate) {
			stats.LastUpdate = toolset.LastUpdated
		}
	}

	// Add cache statistics
	cacheStats := dtm.configCache.GetStats()
	stats.CacheHitRate = cacheStats.HitRate
	stats.CacheSize = cacheStats.Size

	return stats
}

// ToolsetStats represents statistics about managed toolsets
type ToolsetStats struct {
	TotalToolsets int            `json:"total_toolsets"`
	EnabledCount  int            `json:"enabled_count"`
	TypeCounts    map[string]int `json:"type_counts"`
	LastUpdate    time.Time      `json:"last_update"`
	CacheHitRate  float64        `json:"cache_hit_rate"`
	CacheSize     int            `json:"cache_size"`
}

// SortToolsetsByPriority sorts toolsets by priority (higher priority first)
// Business Requirement: BR-HOLMES-024 - Toolset priority ordering
func SortToolsetsByPriority(toolsets []*ToolsetConfig) []*ToolsetConfig {
	sorted := make([]*ToolsetConfig, len(toolsets))
	copy(sorted, toolsets)

	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Priority != sorted[j].Priority {
			return sorted[i].Priority > sorted[j].Priority // Higher priority first
		}
		// If priority is the same, sort by name for consistency
		return sorted[i].Name < sorted[j].Name
	})

	return sorted
}
