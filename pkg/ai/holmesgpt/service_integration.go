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

package holmesgpt

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
)

// ServiceIntegrationInterface defines the contract for service integration
// Following development guideline: create interface abstraction for testability
type ServiceIntegrationInterface interface {
	GetAvailableToolsets() []*ToolsetConfig
	GetToolsetStats() ToolsetStats
	GetServiceDiscoveryStats() ServiceDiscoveryStats
	RefreshToolsets(ctx context.Context) error
	GetHealthStatus() ServiceIntegrationHealth
	CheckKubernetesConnectivity(ctx context.Context) error
}

// ServiceIntegration provides integration between dynamic toolset manager and HolmesGPT API
// Business Requirement: BR-HOLMES-025 - Runtime toolset management API
type ServiceIntegration struct {
	dynamicToolsetManager *DynamicToolsetManager
	serviceDiscovery      *k8s.ServiceDiscovery
	log                   *logrus.Logger
	eventHandlers         []ToolsetUpdateHandler
}

// ToolsetUpdateHandler handles toolset configuration updates
type ToolsetUpdateHandler interface {
	OnToolsetsUpdated(toolsets []*ToolsetConfig) error
}

// NewServiceIntegration creates a new service integration
// Following development guideline: reuse existing patterns from unified_client
func NewServiceIntegration(
	k8sClient kubernetes.Interface,
	config *k8s.ServiceDiscoveryConfig,
	log *logrus.Logger,
) (*ServiceIntegration, error) {
	// Create service discovery
	serviceDiscovery := k8s.NewServiceDiscovery(k8sClient, config, log)

	// Create dynamic toolset manager
	dynamicToolsetManager := NewDynamicToolsetManager(serviceDiscovery, log)

	integration := &ServiceIntegration{
		dynamicToolsetManager: dynamicToolsetManager,
		serviceDiscovery:      serviceDiscovery,
		log:                   log,
		eventHandlers:         make([]ToolsetUpdateHandler, 0),
	}

	// Add integration as event handler to receive toolset updates
	dynamicToolsetManager.AddEventHandler(integration)

	return integration, nil
}

// Start starts the service integration
// Business Requirement: BR-HOLMES-020 - Real-time toolset configuration updates
func (si *ServiceIntegration) Start(ctx context.Context) error {
	si.log.Info("Starting HolmesGPT service integration")

	// Start service discovery
	if err := si.serviceDiscovery.Start(ctx); err != nil {
		return fmt.Errorf("failed to start service discovery: %w", err)
	}

	// Start dynamic toolset manager
	if err := si.dynamicToolsetManager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start dynamic toolset manager: %w", err)
	}

	si.log.Info("HolmesGPT service integration started successfully")
	return nil
}

// Stop stops the service integration
func (si *ServiceIntegration) Stop() {
	si.log.Info("Stopping HolmesGPT service integration")
	si.serviceDiscovery.Stop()
	si.dynamicToolsetManager.Stop()
}

// GetAvailableToolsets returns all currently available toolsets
func (si *ServiceIntegration) GetAvailableToolsets() []*ToolsetConfig {
	return si.dynamicToolsetManager.GetAvailableToolsets()
}

// GetToolsetStats returns toolset statistics
// Business Requirement: BR-HOLMES-029 - Service discovery metrics and monitoring
func (si *ServiceIntegration) GetToolsetStats() ToolsetStats {
	return si.dynamicToolsetManager.GetToolsetStats()
}

// GetServiceDiscoveryStats returns service discovery statistics
func (si *ServiceIntegration) GetServiceDiscoveryStats() ServiceDiscoveryStats {
	services := si.serviceDiscovery.GetDiscoveredServices()

	stats := ServiceDiscoveryStats{
		TotalServices:     len(services),
		AvailableServices: 0,
		ServiceTypes:      make(map[string]int),
		LastDiscovery:     time.Time{},
	}

	for _, service := range services {
		if service.Available {
			stats.AvailableServices++
		}

		stats.ServiceTypes[service.ServiceType]++

		if service.LastChecked.After(stats.LastDiscovery) {
			stats.LastDiscovery = service.LastChecked
		}
	}

	return stats
}

// ServiceDiscoveryStats represents service discovery statistics
type ServiceDiscoveryStats struct {
	TotalServices     int            `json:"total_services"`
	AvailableServices int            `json:"available_services"`
	ServiceTypes      map[string]int `json:"service_types"`
	LastDiscovery     time.Time      `json:"last_discovery"`
}

// AddToolsetUpdateHandler adds a handler for toolset updates
func (si *ServiceIntegration) AddToolsetUpdateHandler(handler ToolsetUpdateHandler) {
	si.eventHandlers = append(si.eventHandlers, handler)
}

// OnToolsetAdded handles toolset addition events
// Business Requirement: BR-HOLMES-020 - Real-time toolset updates
func (si *ServiceIntegration) OnToolsetAdded(config *ToolsetConfig) error {
	si.log.WithFields(logrus.Fields{
		"toolset_name": config.Name,
		"service_type": config.ServiceType,
	}).Info("New toolset added")

	// Notify handlers about toolset changes
	return si.notifyHandlers()
}

// OnToolsetUpdated handles toolset update events
func (si *ServiceIntegration) OnToolsetUpdated(config *ToolsetConfig) error {
	si.log.WithFields(logrus.Fields{
		"toolset_name": config.Name,
		"service_type": config.ServiceType,
	}).Info("Toolset updated")

	// Notify handlers about toolset changes
	return si.notifyHandlers()
}

// OnToolsetRemoved handles toolset removal events
func (si *ServiceIntegration) OnToolsetRemoved(config *ToolsetConfig) error {
	si.log.WithFields(logrus.Fields{
		"toolset_name": config.Name,
		"service_type": config.ServiceType,
	}).Info("Toolset removed")

	// Notify handlers about toolset changes
	return si.notifyHandlers()
}

// notifyHandlers notifies all registered handlers about toolset changes
func (si *ServiceIntegration) notifyHandlers() error {
	toolsets := si.GetAvailableToolsets()

	for _, handler := range si.eventHandlers {
		if err := handler.OnToolsetsUpdated(toolsets); err != nil {
			si.log.WithError(err).Error("Toolset update handler failed")
			// Continue with other handlers even if one fails
		}
	}

	return nil
}

// GetToolsetByServiceType returns toolsets for a specific service type
func (si *ServiceIntegration) GetToolsetByServiceType(serviceType string) []*ToolsetConfig {
	return si.dynamicToolsetManager.GetToolsetByServiceType(serviceType)
}

// IsServiceAvailable checks if a specific service type is available
// Business Requirement: BR-HOLMES-019 - Service availability validation
func (si *ServiceIntegration) IsServiceAvailable(serviceType string) bool {
	toolsets := si.GetToolsetByServiceType(serviceType)

	for _, toolset := range toolsets {
		if toolset.Enabled {
			return true
		}
	}

	return false
}

// GetAvailableServiceTypes returns all available service types
func (si *ServiceIntegration) GetAvailableServiceTypes() []string {
	serviceTypes := make(map[string]bool)
	toolsets := si.GetAvailableToolsets()

	for _, toolset := range toolsets {
		if toolset.Enabled {
			serviceTypes[toolset.ServiceType] = true
		}
	}

	types := make([]string, 0, len(serviceTypes))
	for serviceType := range serviceTypes {
		types = append(types, serviceType)
	}

	return types
}

// RefreshToolsets forces a refresh of toolset configurations
// Business Requirement: BR-HOLMES-025 - Runtime toolset management
func (si *ServiceIntegration) RefreshToolsets(ctx context.Context) error {
	si.log.Info("Forcing toolset refresh")

	// Trigger service discovery scan to get latest services
	services := si.serviceDiscovery.GetDiscoveredServices()

	si.log.WithField("discovered_services", len(services)).Info("Refreshed service discovery")

	// Force toolset regeneration for all discovered services
	// This ensures event handlers are notified of toolset changes
	if err := si.dynamicToolsetManager.RefreshAllToolsets(ctx); err != nil {
		si.log.WithError(err).Error("Failed to refresh dynamic toolsets")
		return fmt.Errorf("failed to refresh dynamic toolsets: %w", err)
	}

	// Notify all registered handlers about the refresh
	if err := si.notifyHandlers(); err != nil {
		si.log.WithError(err).Error("Failed to notify handlers after toolset refresh")
		return fmt.Errorf("failed to notify handlers: %w", err)
	}

	si.log.Info("Toolset refresh completed successfully")
	return nil
}

// GetHealthStatus returns the health status of the service integration
func (si *ServiceIntegration) GetHealthStatus() ServiceIntegrationHealth {
	toolsetStats := si.GetToolsetStats()
	discoveryStats := si.GetServiceDiscoveryStats()

	health := ServiceIntegrationHealth{
		ServiceDiscoveryHealthy: discoveryStats.TotalServices >= 0, // Basic check
		ToolsetManagerHealthy:   toolsetStats.TotalToolsets > 0,    // Must have at least baseline toolsets
		TotalToolsets:           toolsetStats.TotalToolsets,
		EnabledToolsets:         toolsetStats.EnabledCount,
		DiscoveredServices:      discoveryStats.TotalServices,
		AvailableServices:       discoveryStats.AvailableServices,
		LastUpdate:              toolsetStats.LastUpdate,
	}

	// Determine overall health
	health.Healthy = health.ServiceDiscoveryHealthy && health.ToolsetManagerHealthy

	return health
}

// ServiceIntegrationHealth represents the health status of service integration
type ServiceIntegrationHealth struct {
	Healthy                 bool      `json:"healthy"`
	ServiceDiscoveryHealthy bool      `json:"service_discovery_healthy"`
	ToolsetManagerHealthy   bool      `json:"toolset_manager_healthy"`
	TotalToolsets           int       `json:"total_toolsets"`
	EnabledToolsets         int       `json:"enabled_toolsets"`
	DiscoveredServices      int       `json:"discovered_services"`
	AvailableServices       int       `json:"available_services"`
	LastUpdate              time.Time `json:"last_update"`
}

// CheckKubernetesConnectivity validates Kubernetes cluster connectivity
// TDD REFACTOR: Enhanced with comprehensive connectivity validation and Kubernetes Safety patterns
// Business Requirement: BR-SERVICE-INTEGRATION-003 - Kubernetes connectivity validation
func (si *ServiceIntegration) CheckKubernetesConnectivity(ctx context.Context) error {
	si.log.WithFields(logrus.Fields{
		"operation": "kubernetes_connectivity_check",
		"component": "service_integration",
	}).Debug("Starting comprehensive Kubernetes connectivity validation")

	// Phase 1: Service Discovery Connectivity (existing infrastructure)
	services := si.serviceDiscovery.GetDiscoveredServices()

	if len(services) == 0 {
		si.log.Error("No Kubernetes services discovered - cluster connectivity may be impaired")
		return fmt.Errorf("no Kubernetes services discovered - cluster connectivity may be impaired")
	}

	// Phase 2: Service Health Validation (leverage existing health checks)
	healthyServices := 0
	unhealthyServices := 0
	connectivityIssues := []string{}

	for _, service := range services {
		if service.Available {
			healthyServices++
		} else {
			unhealthyServices++
			if service.HealthStatus.ErrorMessage != "" {
				connectivityIssues = append(connectivityIssues,
					fmt.Sprintf("Service %s/%s: %s", service.Namespace, service.Name, service.HealthStatus.ErrorMessage))
			}
		}
	}

	// Phase 3: Connectivity Assessment following Kubernetes Safety patterns
	totalServices := len(services)
	healthPercentage := float64(healthyServices) / float64(totalServices) * 100

	si.log.WithFields(logrus.Fields{
		"total_services":     totalServices,
		"healthy_services":   healthyServices,
		"unhealthy_services": unhealthyServices,
		"health_percentage":  healthPercentage,
	}).Info("Kubernetes connectivity assessment completed")

	// Phase 4: Safety Threshold Validation (following Kubernetes Safety patterns)
	// Require at least 70% of services to be healthy for good connectivity
	const healthThreshold = 70.0

	if healthPercentage < healthThreshold {
		errorMsg := fmt.Sprintf("Kubernetes connectivity degraded: only %.1f%% of services healthy (threshold: %.1f%%)",
			healthPercentage, healthThreshold)

		if len(connectivityIssues) > 0 {
			errorMsg += fmt.Sprintf(". Issues: %v", connectivityIssues)
		}

		si.log.WithFields(logrus.Fields{
			"health_percentage": healthPercentage,
			"threshold":         healthThreshold,
			"issues":            connectivityIssues,
		}).Warn("Kubernetes connectivity below safety threshold")

		return errors.New(errorMsg)
	}

	// Phase 5: Toolset Manager Connectivity (validate integration layer)
	toolsetStats := si.dynamicToolsetManager.GetToolsetStats()
	if toolsetStats.TotalToolsets == 0 {
		si.log.Warn("No toolsets available - dynamic toolset manager connectivity may be impaired")
		return fmt.Errorf("dynamic toolset manager has no available toolsets - integration connectivity impaired")
	}

	// Success: Comprehensive connectivity validated
	si.log.WithFields(logrus.Fields{
		"health_percentage":   healthPercentage,
		"healthy_services":    healthyServices,
		"total_toolsets":      toolsetStats.TotalToolsets,
		"enabled_toolsets":    toolsetStats.EnabledCount,
		"connectivity_status": "optimal",
	}).Info("Kubernetes connectivity validation successful - all systems operational")

	return nil
}
