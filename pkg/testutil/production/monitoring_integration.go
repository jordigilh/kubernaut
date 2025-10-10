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
package production

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"

	"github.com/jordigilh/kubernaut/pkg/platform/monitoring"
)

// Production Monitoring Integration
// Business Requirements: BR-PRODUCTION-007 - Production monitoring integration
// Following 00-project-guidelines.mdc: MANDATORY business requirement mapping
// Following 03-testing-strategy.mdc: Integration testing with monitoring systems
// Following 09-interface-method-validation.mdc: Interface validation before code generation

// ProductionMonitoringIntegrator manages monitoring integration for production clusters
type ProductionMonitoringIntegrator struct {
	client           kubernetes.Interface
	monitoringConfig *monitoring.MonitoringConfig
	factory          *monitoring.ClientFactory
	logger           *logrus.Logger
}

// MonitoringIntegrationConfig defines configuration for production monitoring integration
type MonitoringIntegrationConfig struct {
	EnableHealthChecks        bool               `yaml:"enable_health_checks"`
	EnablePerformanceMetrics  bool               `yaml:"enable_performance_metrics"`
	EnableResourceMonitoring  bool               `yaml:"enable_resource_monitoring"`
	MetricsCollectionInterval time.Duration      `yaml:"metrics_collection_interval"`
	HealthCheckInterval       time.Duration      `yaml:"health_check_interval"`
	AlertingEnabled           bool               `yaml:"alerting_enabled"`
	MonitoringTargets         *MonitoringTargets `yaml:"monitoring_targets"`
}

// MonitoringTargets defines monitoring performance targets
type MonitoringTargets struct {
	HealthCheckResponseTime time.Duration `yaml:"health_check_response_time"` // Target: <5s
	MetricsCollectionTime   time.Duration `yaml:"metrics_collection_time"`    // Target: <10s
	ResourceQueryTime       time.Duration `yaml:"resource_query_time"`        // Target: <3s
	AlertDeliveryTime       time.Duration `yaml:"alert_delivery_time"`        // Target: <30s
}

// ProductionMonitoringStatus represents the status of production monitoring
type ProductionMonitoringStatus struct {
	HealthChecksEnabled       bool                          `json:"health_checks_enabled"`
	PerformanceMetricsEnabled bool                          `json:"performance_metrics_enabled"`
	ResourceMonitoringEnabled bool                          `json:"resource_monitoring_enabled"`
	AlertingEnabled           bool                          `json:"alerting_enabled"`
	MonitoringClients         *MonitoringClientsStatus      `json:"monitoring_clients"`
	PerformanceMetrics        *MonitoringPerformanceMetrics `json:"performance_metrics"`
	LastUpdate                time.Time                     `json:"last_update"`
	ValidationResults         *MonitoringValidationResults  `json:"validation_results"`
}

// MonitoringClientsStatus represents the status of monitoring clients
type MonitoringClientsStatus struct {
	AlertClientAvailable        bool   `json:"alert_client_available"`
	MetricsClientAvailable      bool   `json:"metrics_client_available"`
	HealthMonitorAvailable      bool   `json:"health_monitor_available"`
	SideEffectDetectorAvailable bool   `json:"side_effect_detector_available"`
	ClientType                  string `json:"client_type"` // "production" or "stub"
}

// MonitoringPerformanceMetrics tracks monitoring system performance
type MonitoringPerformanceMetrics struct {
	HealthCheckResponseTime time.Duration `json:"health_check_response_time"`
	MetricsCollectionTime   time.Duration `json:"metrics_collection_time"`
	ResourceQueryTime       time.Duration `json:"resource_query_time"`
	AlertDeliveryTime       time.Duration `json:"alert_delivery_time"`
	SuccessRate             float64       `json:"success_rate"`
}

// MonitoringValidationResults contains monitoring validation results
type MonitoringValidationResults struct {
	HealthCheckValidation        bool     `json:"health_check_validation"`
	MetricsCollectionValidation  bool     `json:"metrics_collection_validation"`
	ResourceMonitoringValidation bool     `json:"resource_monitoring_validation"`
	AlertingValidation           bool     `json:"alerting_validation"`
	FailedValidations            []string `json:"failed_validations"`
}

// NewProductionMonitoringIntegrator creates a new production monitoring integrator
// Business Requirement: BR-PRODUCTION-007 - Production monitoring integration
func NewProductionMonitoringIntegrator(client kubernetes.Interface, config *MonitoringIntegrationConfig, logger *logrus.Logger) (*ProductionMonitoringIntegrator, error) {
	if logger == nil {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)
	}

	if config == nil {
		config = &MonitoringIntegrationConfig{
			EnableHealthChecks:        true,
			EnablePerformanceMetrics:  true,
			EnableResourceMonitoring:  true,
			MetricsCollectionInterval: 30 * time.Second,
			HealthCheckInterval:       10 * time.Second,
			AlertingEnabled:           false, // Minimal monitoring approach
			MonitoringTargets: &MonitoringTargets{
				HealthCheckResponseTime: 5 * time.Second,
				MetricsCollectionTime:   10 * time.Second,
				ResourceQueryTime:       3 * time.Second,
				AlertDeliveryTime:       30 * time.Second,
			},
		}
	}

	// Create monitoring configuration (minimal approach - no external Prometheus/AlertManager)
	monitoringConfig := &monitoring.MonitoringConfig{
		UseProductionClients: false, // Use stub clients for minimal monitoring
		AlertManagerConfig: monitoring.AlertManagerConfig{
			Enabled:  config.AlertingEnabled,
			Endpoint: "http://localhost:9093", // Placeholder
			Timeout:  30 * time.Second,
		},
		PrometheusConfig: monitoring.PrometheusConfig{
			Enabled:  false,                   // Minimal monitoring - no external Prometheus
			Endpoint: "http://localhost:9090", // Placeholder
			Timeout:  30 * time.Second,
		},
	}

	// Create monitoring client factory
	factory := monitoring.NewClientFactory(*monitoringConfig, nil, logger)

	integrator := &ProductionMonitoringIntegrator{
		client:           client,
		monitoringConfig: monitoringConfig,
		factory:          factory,
		logger:           logger,
	}

	logger.WithFields(logrus.Fields{
		"health_checks":       config.EnableHealthChecks,
		"performance_metrics": config.EnablePerformanceMetrics,
		"resource_monitoring": config.EnableResourceMonitoring,
		"alerting_enabled":    config.AlertingEnabled,
	}).Info("Production monitoring integrator initialized")

	return integrator, nil
}

// SetupMonitoring sets up monitoring for a production cluster environment
// Business Requirement: BR-PRODUCTION-007 - Comprehensive monitoring setup
func (pmi *ProductionMonitoringIntegrator) SetupMonitoring(ctx context.Context, clusterEnv *RealClusterEnvironment, config *MonitoringIntegrationConfig) (*ProductionMonitoringStatus, error) {
	pmi.logger.Info("Setting up production monitoring")

	status := &ProductionMonitoringStatus{
		HealthChecksEnabled:       config.EnableHealthChecks,
		PerformanceMetricsEnabled: config.EnablePerformanceMetrics,
		ResourceMonitoringEnabled: config.EnableResourceMonitoring,
		AlertingEnabled:           config.AlertingEnabled,
		MonitoringClients:         &MonitoringClientsStatus{},
		PerformanceMetrics:        &MonitoringPerformanceMetrics{},
		ValidationResults:         &MonitoringValidationResults{},
		LastUpdate:                time.Now(),
	}

	// Create monitoring clients
	monitoringClients := pmi.factory.CreateClients()
	if monitoringClients == nil {
		return status, fmt.Errorf("failed to create monitoring clients")
	}

	// Validate monitoring configuration
	if err := pmi.factory.ValidateConfig(); err != nil {
		return status, fmt.Errorf("monitoring configuration validation failed: %w", err)
	}

	// Update client status
	pmi.updateMonitoringClientsStatus(status, monitoringClients)

	// Perform health checks if enabled
	if config.EnableHealthChecks {
		if err := pmi.performHealthChecks(ctx, status, monitoringClients, clusterEnv); err != nil {
			pmi.logger.WithError(err).Warn("Health checks failed")
			status.ValidationResults.FailedValidations = append(status.ValidationResults.FailedValidations, "health_checks")
		} else {
			status.ValidationResults.HealthCheckValidation = true
		}
	}

	// Collect performance metrics if enabled
	if config.EnablePerformanceMetrics {
		if err := pmi.collectPerformanceMetrics(ctx, status, monitoringClients, clusterEnv); err != nil {
			pmi.logger.WithError(err).Warn("Performance metrics collection failed")
			status.ValidationResults.FailedValidations = append(status.ValidationResults.FailedValidations, "performance_metrics")
		} else {
			status.ValidationResults.MetricsCollectionValidation = true
		}
	}

	// Monitor resources if enabled
	if config.EnableResourceMonitoring {
		if err := pmi.monitorResources(ctx, status, monitoringClients, clusterEnv); err != nil {
			pmi.logger.WithError(err).Warn("Resource monitoring failed")
			status.ValidationResults.FailedValidations = append(status.ValidationResults.FailedValidations, "resource_monitoring")
		} else {
			status.ValidationResults.ResourceMonitoringValidation = true
		}
	}

	// Test alerting if enabled
	if config.AlertingEnabled {
		if err := pmi.testAlerting(ctx, status, monitoringClients); err != nil {
			pmi.logger.WithError(err).Warn("Alerting test failed")
			status.ValidationResults.FailedValidations = append(status.ValidationResults.FailedValidations, "alerting")
		} else {
			status.ValidationResults.AlertingValidation = true
		}
	}

	// Calculate overall success rate
	pmi.calculateSuccessRate(status)

	pmi.logger.WithFields(logrus.Fields{
		"health_checks":       status.ValidationResults.HealthCheckValidation,
		"performance_metrics": status.ValidationResults.MetricsCollectionValidation,
		"resource_monitoring": status.ValidationResults.ResourceMonitoringValidation,
		"alerting":            status.ValidationResults.AlertingValidation,
		"success_rate":        status.PerformanceMetrics.SuccessRate,
	}).Info("Production monitoring setup completed")

	return status, nil
}

// updateMonitoringClientsStatus updates the monitoring clients status
func (pmi *ProductionMonitoringIntegrator) updateMonitoringClientsStatus(status *ProductionMonitoringStatus, clients *monitoring.MonitoringClients) {
	status.MonitoringClients.AlertClientAvailable = clients.AlertClient != nil
	status.MonitoringClients.MetricsClientAvailable = clients.MetricsClient != nil
	status.MonitoringClients.HealthMonitorAvailable = clients.HealthMonitor != nil
	status.MonitoringClients.SideEffectDetectorAvailable = clients.SideEffectDetector != nil

	if pmi.monitoringConfig.UseProductionClients {
		status.MonitoringClients.ClientType = "production"
	} else {
		status.MonitoringClients.ClientType = "stub"
	}

	pmi.logger.WithFields(logrus.Fields{
		"alert_client":         status.MonitoringClients.AlertClientAvailable,
		"metrics_client":       status.MonitoringClients.MetricsClientAvailable,
		"health_monitor":       status.MonitoringClients.HealthMonitorAvailable,
		"side_effect_detector": status.MonitoringClients.SideEffectDetectorAvailable,
		"client_type":          status.MonitoringClients.ClientType,
	}).Debug("Monitoring clients status updated")
}

// performHealthChecks performs health checks on the cluster and monitoring systems
func (pmi *ProductionMonitoringIntegrator) performHealthChecks(ctx context.Context, status *ProductionMonitoringStatus, clients *monitoring.MonitoringClients, clusterEnv *RealClusterEnvironment) error {
	pmi.logger.Info("Performing health checks")

	healthCheckStart := time.Now()
	defer func() {
		status.PerformanceMetrics.HealthCheckResponseTime = time.Since(healthCheckStart)
	}()

	// Check cluster health
	clusterInfo, err := clusterEnv.GetClusterInfo(ctx)
	if err != nil {
		return fmt.Errorf("cluster health check failed: %w", err)
	}

	if clusterInfo.NodeCount == 0 {
		return fmt.Errorf("cluster has no nodes")
	}

	// Check monitoring clients health
	if err := pmi.factory.HealthCheck(clients); err != nil {
		return fmt.Errorf("monitoring clients health check failed: %w", err)
	}

	pmi.logger.WithFields(logrus.Fields{
		"cluster_nodes":     clusterInfo.NodeCount,
		"cluster_pods":      clusterInfo.PodCount,
		"health_check_time": status.PerformanceMetrics.HealthCheckResponseTime,
	}).Info("Health checks completed successfully")

	return nil
}

// collectPerformanceMetrics collects performance metrics from the cluster
func (pmi *ProductionMonitoringIntegrator) collectPerformanceMetrics(ctx context.Context, status *ProductionMonitoringStatus, clients *monitoring.MonitoringClients, clusterEnv *RealClusterEnvironment) error {
	pmi.logger.Info("Collecting performance metrics")

	metricsStart := time.Now()
	defer func() {
		status.PerformanceMetrics.MetricsCollectionTime = time.Since(metricsStart)
	}()

	// Get cluster information for metrics
	clusterInfo, err := clusterEnv.GetClusterInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cluster info for metrics: %w", err)
	}

	// Collect basic cluster metrics
	metrics := map[string]interface{}{
		"cluster_nodes":    clusterInfo.NodeCount,
		"cluster_pods":     clusterInfo.PodCount,
		"cluster_scenario": clusterInfo.Scenario,
		"setup_time":       clusterInfo.SetupTime,
		"resource_info":    clusterInfo.ResourceInfo,
		"collection_time":  time.Now(),
	}

	// Use metrics client if available (stub client will handle gracefully)
	if clients.MetricsClient != nil {
		// Metrics client operations would go here
		// For minimal monitoring, we just log the metrics
		pmi.logger.WithFields(logrus.Fields{
			"metrics": metrics,
		}).Debug("Performance metrics collected")
	}

	pmi.logger.WithFields(logrus.Fields{
		"metrics_count":   len(metrics),
		"collection_time": status.PerformanceMetrics.MetricsCollectionTime,
	}).Info("Performance metrics collection completed")

	return nil
}

// monitorResources monitors cluster resource usage
func (pmi *ProductionMonitoringIntegrator) monitorResources(ctx context.Context, status *ProductionMonitoringStatus, clients *monitoring.MonitoringClients, clusterEnv *RealClusterEnvironment) error {
	pmi.logger.Info("Monitoring cluster resources")

	resourceStart := time.Now()
	defer func() {
		status.PerformanceMetrics.ResourceQueryTime = time.Since(resourceStart)
	}()

	// Get cluster resource information
	clusterInfo, err := clusterEnv.GetClusterInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cluster info for resource monitoring: %w", err)
	}

	// Monitor basic resource usage
	resourceMetrics := map[string]interface{}{
		"total_nodes":     clusterInfo.NodeCount,
		"total_pods":      clusterInfo.PodCount,
		"resource_info":   clusterInfo.ResourceInfo,
		"monitoring_time": time.Now(),
	}

	// Use side effect detector if available for resource monitoring
	if clients.SideEffectDetector != nil {
		// Side effect detection operations would go here
		// For minimal monitoring, we just validate basic resource state
		pmi.logger.WithFields(logrus.Fields{
			"resource_metrics": resourceMetrics,
		}).Debug("Resource monitoring completed")
	}

	pmi.logger.WithFields(logrus.Fields{
		"resource_metrics_count": len(resourceMetrics),
		"query_time":             status.PerformanceMetrics.ResourceQueryTime,
	}).Info("Resource monitoring completed")

	return nil
}

// testAlerting tests alerting functionality
func (pmi *ProductionMonitoringIntegrator) testAlerting(ctx context.Context, status *ProductionMonitoringStatus, clients *monitoring.MonitoringClients) error {
	pmi.logger.Info("Testing alerting functionality")

	alertStart := time.Now()
	defer func() {
		status.PerformanceMetrics.AlertDeliveryTime = time.Since(alertStart)
	}()

	// Test alert client if available
	if clients.AlertClient != nil {
		// Alert client operations would go here
		// For minimal monitoring, we just validate the client is available
		pmi.logger.Debug("Alert client is available for testing")
	} else {
		return fmt.Errorf("alert client not available")
	}

	pmi.logger.WithFields(logrus.Fields{
		"alert_delivery_time": status.PerformanceMetrics.AlertDeliveryTime,
	}).Info("Alerting test completed")

	return nil
}

// calculateSuccessRate calculates the overall monitoring success rate
func (pmi *ProductionMonitoringIntegrator) calculateSuccessRate(status *ProductionMonitoringStatus) {
	successCount := 0
	totalChecks := 0

	if status.HealthChecksEnabled {
		totalChecks++
		if status.ValidationResults.HealthCheckValidation {
			successCount++
		}
	}

	if status.PerformanceMetricsEnabled {
		totalChecks++
		if status.ValidationResults.MetricsCollectionValidation {
			successCount++
		}
	}

	if status.ResourceMonitoringEnabled {
		totalChecks++
		if status.ValidationResults.ResourceMonitoringValidation {
			successCount++
		}
	}

	if status.AlertingEnabled {
		totalChecks++
		if status.ValidationResults.AlertingValidation {
			successCount++
		}
	}

	if totalChecks > 0 {
		status.PerformanceMetrics.SuccessRate = float64(successCount) / float64(totalChecks)
	} else {
		status.PerformanceMetrics.SuccessRate = 1.0 // No checks enabled, consider success
	}
}

// GetMonitoringMetrics returns current monitoring metrics
func (pmi *ProductionMonitoringIntegrator) GetMonitoringMetrics() map[string]interface{} {
	configSummary := pmi.factory.GetConfigSummary()

	return map[string]interface{}{
		"monitoring_config": configSummary,
		"integrator_info": map[string]interface{}{
			"client_type":          pmi.monitoringConfig.UseProductionClients,
			"alertmanager_enabled": pmi.monitoringConfig.AlertManagerConfig.Enabled,
			"prometheus_enabled":   pmi.monitoringConfig.PrometheusConfig.Enabled,
		},
	}
}

// ValidateMonitoringIntegration validates monitoring integration with cluster
func (pmi *ProductionMonitoringIntegrator) ValidateMonitoringIntegration(ctx context.Context, clusterEnv *RealClusterEnvironment) (*ProductionMonitoringStatus, error) {
	pmi.logger.Info("Validating monitoring integration")

	config := &MonitoringIntegrationConfig{
		EnableHealthChecks:        true,
		EnablePerformanceMetrics:  true,
		EnableResourceMonitoring:  true,
		MetricsCollectionInterval: 30 * time.Second,
		HealthCheckInterval:       10 * time.Second,
		AlertingEnabled:           false, // Minimal monitoring
		MonitoringTargets: &MonitoringTargets{
			HealthCheckResponseTime: 5 * time.Second,
			MetricsCollectionTime:   10 * time.Second,
			ResourceQueryTime:       3 * time.Second,
			AlertDeliveryTime:       30 * time.Second,
		},
	}

	return pmi.SetupMonitoring(ctx, clusterEnv, config)
}
