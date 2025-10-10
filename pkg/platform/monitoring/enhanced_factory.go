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

package monitoring

import (
	"context"
	"fmt"

	"github.com/jordigilh/kubernaut/pkg/infrastructure/metrics"
	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
	"github.com/sirupsen/logrus"
)

// EnhancedMonitoringFactory provides type-safe client creation with proper dependency injection
type EnhancedMonitoringFactory struct {
	baseFactory *ClientFactory
	config      MonitoringConfig
	k8sClient   k8s.Client
	logger      *logrus.Logger
}

// NewEnhancedMonitoringFactory creates a new enhanced monitoring factory
func NewEnhancedMonitoringFactory(config MonitoringConfig, k8sClient k8s.Client, logger *logrus.Logger) *EnhancedMonitoringFactory {
	return &EnhancedMonitoringFactory{
		baseFactory: NewClientFactory(config, k8sClient, logger),
		config:      config,
		k8sClient:   k8sClient,
		logger:      logger,
	}
}

// MonitoringClientSet provides strongly-typed access to all monitoring clients
type MonitoringClientSet struct {
	AlertClient        AlertClient        // Interface for alert management
	MetricsClient      MetricsClient      // Interface for metrics collection
	SideEffectDetector SideEffectDetector // Interface for side effect detection

	// Type-safe accessors for concrete implementations
	alertManagerClient    *AlertManagerClient // Concrete AlertManager client
	prometheusClient      *PrometheusClient   // Concrete Prometheus client
	infrastructureMetrics *metrics.Client     // Infrastructure metrics client
}

// CreateEnhancedClients creates a comprehensive set of monitoring clients with proper type safety
func (emf *EnhancedMonitoringFactory) CreateEnhancedClients() (*MonitoringClientSet, error) {
	// Validate configuration first
	if err := emf.baseFactory.ValidateConfig(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	clientSet := &MonitoringClientSet{}

	if emf.config.UseProductionClients {
		if err := emf.createProductionClientSet(clientSet); err != nil {
			return nil, fmt.Errorf("failed to create production clients: %w", err)
		}
	} else {
		emf.createStubClientSet(clientSet)
	}

	return clientSet, nil
}

// createProductionClientSet creates production clients with proper type safety
func (emf *EnhancedMonitoringFactory) createProductionClientSet(clientSet *MonitoringClientSet) error {
	emf.logger.Info("Creating enhanced production monitoring client set")

	// Create AlertManager client with type safety
	if emf.config.AlertManagerConfig.Enabled {
		alertClient := NewAlertManagerClient(
			emf.config.AlertManagerConfig.Endpoint,
			emf.config.AlertManagerConfig.Timeout,
			emf.logger,
		)

		clientSet.AlertClient = alertClient
		clientSet.alertManagerClient = alertClient

		emf.logger.WithFields(logrus.Fields{
			"endpoint": emf.config.AlertManagerConfig.Endpoint,
			"timeout":  emf.config.AlertManagerConfig.Timeout,
		}).Info("Created production AlertManager client")
	} else {
		clientSet.AlertClient = NewStubAlertClient(emf.logger)
		emf.logger.Info("Created stub AlertManager client")
	}

	// Create Prometheus client with type safety
	if emf.config.PrometheusConfig.Enabled {
		promClient := NewPrometheusClient(
			emf.config.PrometheusConfig.Endpoint,
			emf.config.PrometheusConfig.Timeout,
			emf.logger,
		)

		clientSet.MetricsClient = promClient
		clientSet.prometheusClient = promClient

		emf.logger.WithFields(logrus.Fields{
			"endpoint": emf.config.PrometheusConfig.Endpoint,
			"timeout":  emf.config.PrometheusConfig.Timeout,
		}).Info("Created production Prometheus client")
	} else {
		clientSet.MetricsClient = NewStubMetricsClient(emf.logger)
		emf.logger.Info("Created stub Prometheus client")
	}

	// Create infrastructure metrics client
	infraClient := &metrics.Client{}
	clientSet.infrastructureMetrics = infraClient
	emf.logger.Info("Created infrastructure metrics client")

	// Create enhanced side effect detector
	clientSet.SideEffectDetector = NewEnhancedSideEffectDetector(
		emf.k8sClient,
		clientSet.AlertClient,
		emf.logger,
	)
	emf.logger.Info("Created enhanced side effect detector")

	return nil
}

// createStubClientSet creates stub clients for development and testing
func (emf *EnhancedMonitoringFactory) createStubClientSet(clientSet *MonitoringClientSet) {
	emf.logger.Info("Creating enhanced stub monitoring client set")

	clientSet.AlertClient = NewStubAlertClient(emf.logger)
	clientSet.MetricsClient = NewStubMetricsClient(emf.logger)
	clientSet.SideEffectDetector = NewStubSideEffectDetector(emf.logger)
}

// GetAlertManagerClient returns the concrete AlertManager client with type safety
func (mcs *MonitoringClientSet) GetAlertManagerClient() (*AlertManagerClient, bool) {
	if mcs.alertManagerClient != nil {
		return mcs.alertManagerClient, true
	}
	return nil, false
}

// GetPrometheusClient returns the concrete Prometheus client with type safety
func (mcs *MonitoringClientSet) GetPrometheusClient() (*PrometheusClient, bool) {
	if mcs.prometheusClient != nil {
		return mcs.prometheusClient, true
	}
	return nil, false
}

// GetInfrastructureMetricsClient returns the infrastructure metrics client with type safety
func (mcs *MonitoringClientSet) GetInfrastructureMetricsClient() (*metrics.Client, bool) {
	if mcs.infrastructureMetrics != nil {
		return mcs.infrastructureMetrics, true
	}
	return nil, false
}

// HealthCheckAll performs comprehensive health checks on all enabled clients
func (mcs *MonitoringClientSet) HealthCheckAll(ctx context.Context) error {
	var healthErrors []error

	// Check AlertManager client
	if alertClient, ok := mcs.GetAlertManagerClient(); ok {
		if err := alertClient.HealthCheck(ctx); err != nil {
			healthErrors = append(healthErrors, fmt.Errorf("AlertManager health check failed: %w", err))
		}
	}

	// Check Prometheus client
	if promClient, ok := mcs.GetPrometheusClient(); ok {
		if err := promClient.HealthCheck(ctx); err != nil {
			healthErrors = append(healthErrors, fmt.Errorf("prometheus health check failed: %w", err))
		}
	}

	// Infrastructure metrics client doesn't have health check method
	// This is a simple client that records metrics to Prometheus
	if infraClient, ok := mcs.GetInfrastructureMetricsClient(); ok && infraClient != nil {
		// Client exists and is ready - no additional health check needed
		// Note: Infrastructure client availability confirmed
		_ = infraClient // Acknowledge we're checking the client
	}

	if len(healthErrors) > 0 {
		return fmt.Errorf("health check failures: %v", healthErrors)
	}

	return nil
}

// ToLegacyMonitoringClients converts to the legacy MonitoringClients struct for backward compatibility
func (mcs *MonitoringClientSet) ToLegacyMonitoringClients() *MonitoringClients {
	return &MonitoringClients{
		AlertClient:        mcs.AlertClient,
		MetricsClient:      mcs.MetricsClient,
		SideEffectDetector: mcs.SideEffectDetector,
	}
}

// ClientBuilder provides a fluent interface for building monitoring clients
type ClientBuilder struct {
	factory *EnhancedMonitoringFactory
	config  MonitoringConfig
}

// NewClientBuilder creates a new client builder
func NewClientBuilder() *ClientBuilder {
	return &ClientBuilder{
		config: MonitoringConfig{}, // Start with empty config
	}
}

// WithConfig sets the monitoring configuration
func (cb *ClientBuilder) WithConfig(config MonitoringConfig) *ClientBuilder {
	cb.config = config
	return cb
}

// WithProduction enables production clients
func (cb *ClientBuilder) WithProduction() *ClientBuilder {
	cb.config.UseProductionClients = true
	return cb
}

// WithStubs enables stub clients
func (cb *ClientBuilder) WithStubs() *ClientBuilder {
	cb.config.UseProductionClients = false
	return cb
}

// WithAlertManager configures AlertManager
func (cb *ClientBuilder) WithAlertManager(endpoint string, enabled bool) *ClientBuilder {
	cb.config.AlertManagerConfig = AlertManagerConfig{
		Enabled:  enabled,
		Endpoint: endpoint,
		Timeout:  30, // Default timeout
	}
	return cb
}

// WithPrometheus configures Prometheus
func (cb *ClientBuilder) WithPrometheus(endpoint string, enabled bool) *ClientBuilder {
	cb.config.PrometheusConfig = PrometheusConfig{
		Enabled:  enabled,
		Endpoint: endpoint,
		Timeout:  30, // Default timeout
	}
	return cb
}

// Build creates the monitoring client set
func (cb *ClientBuilder) Build(k8sClient k8s.Client, logger *logrus.Logger) (*MonitoringClientSet, error) {
	cb.factory = NewEnhancedMonitoringFactory(cb.config, k8sClient, logger)
	return cb.factory.CreateEnhancedClients()
}
