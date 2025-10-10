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
	"time"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
	"github.com/sirupsen/logrus"
)

// MonitoringConfig holds configuration for monitoring clients
type MonitoringConfig struct {
	// Client type configuration
	UseProductionClients bool `yaml:"use_production_clients" env:"USE_PRODUCTION_MONITORING_CLIENTS" default:"false"`

	// AlertManager configuration
	AlertManagerConfig AlertManagerConfig `yaml:"alertmanager"`

	// Prometheus configuration
	PrometheusConfig PrometheusConfig `yaml:"prometheus"`
}

// AlertManagerConfig holds AlertManager client configuration
type AlertManagerConfig struct {
	Enabled  bool          `yaml:"enabled" env:"ALERTMANAGER_ENABLED" default:"false"`
	Endpoint string        `yaml:"endpoint" env:"ALERTMANAGER_ENDPOINT" default:"http://localhost:9093"`
	Timeout  time.Duration `yaml:"timeout" env:"ALERTMANAGER_TIMEOUT" default:"30s"`
}

// PrometheusConfig holds Prometheus client configuration
type PrometheusConfig struct {
	Enabled  bool          `yaml:"enabled" env:"PROMETHEUS_ENABLED" default:"false"`
	Endpoint string        `yaml:"endpoint" env:"PROMETHEUS_ENDPOINT" default:"http://localhost:9090"`
	Timeout  time.Duration `yaml:"timeout" env:"PROMETHEUS_TIMEOUT" default:"30s"`
}

// MonitoringClients holds all monitoring client implementations
type MonitoringClients struct {
	AlertClient        AlertClient
	MetricsClient      MetricsClient
	SideEffectDetector SideEffectDetector
	HealthMonitor      HealthMonitor
}

// ClientFactory creates monitoring clients based on configuration
type ClientFactory struct {
	config MonitoringConfig
	k8s    k8s.Client
	logger *logrus.Logger
}

// NewClientFactory creates a new monitoring client factory
func NewClientFactory(config MonitoringConfig, k8sClient k8s.Client, logger *logrus.Logger) *ClientFactory {
	return &ClientFactory{
		config: config,
		k8s:    k8sClient,
		logger: logger,
	}
}

// CreateClients creates monitoring clients based on configuration
func (f *ClientFactory) CreateClients() *MonitoringClients {
	if f.config.UseProductionClients {
		return f.createProductionClients()
	}
	return f.createStubClients()
}

// createProductionClients creates real monitoring clients
func (f *ClientFactory) createProductionClients() *MonitoringClients {
	f.logger.Info("Creating production monitoring clients")

	clients := &MonitoringClients{}

	// Create AlertManager client
	if f.config.AlertManagerConfig.Enabled {
		f.logger.WithFields(logrus.Fields{
			"endpoint": f.config.AlertManagerConfig.Endpoint,
			"timeout":  f.config.AlertManagerConfig.Timeout,
		}).Info("Creating production AlertManager client")

		clients.AlertClient = NewAlertManagerClient(
			f.config.AlertManagerConfig.Endpoint,
			f.config.AlertManagerConfig.Timeout,
			f.logger,
		)
	} else {
		f.logger.Info("AlertManager client disabled, using stub implementation")
		clients.AlertClient = NewStubAlertClient(f.logger)
	}

	// Create Prometheus client
	if f.config.PrometheusConfig.Enabled {
		f.logger.WithFields(logrus.Fields{
			"endpoint": f.config.PrometheusConfig.Endpoint,
			"timeout":  f.config.PrometheusConfig.Timeout,
		}).Info("Creating production Prometheus client")

		clients.MetricsClient = NewPrometheusClient(
			f.config.PrometheusConfig.Endpoint,
			f.config.PrometheusConfig.Timeout,
			f.logger,
		)
	} else {
		f.logger.Info("Prometheus client disabled, using stub implementation")
		clients.MetricsClient = NewStubMetricsClient(f.logger)
	}

	// Create enhanced side effect detector
	if f.config.AlertManagerConfig.Enabled || f.config.PrometheusConfig.Enabled {
		f.logger.Info("Creating enhanced side effect detector")
		clients.SideEffectDetector = NewEnhancedSideEffectDetector(
			f.k8s,
			clients.AlertClient,
			f.logger,
		)
	} else {
		f.logger.Info("Enhanced side effect detection disabled, using stub implementation")
		clients.SideEffectDetector = NewStubSideEffectDetector(f.logger)
	}

	// Note: LLM Health Monitor requires LLM client dependency - will be injected at service level
	// This follows the dependency injection pattern established in the codebase
	f.logger.Info("LLM Health Monitor requires LLM client injection at service level")

	return clients
}

// createStubClients creates stub monitoring clients for development/testing
func (f *ClientFactory) createStubClients() *MonitoringClients {
	f.logger.Info("Creating stub monitoring clients")

	return &MonitoringClients{
		AlertClient:        NewStubAlertClient(f.logger),
		MetricsClient:      NewStubMetricsClient(f.logger),
		SideEffectDetector: NewStubSideEffectDetector(f.logger),
		HealthMonitor:      nil, // Will be injected with LLM client at service level
	}
}

// ValidateConfig validates the monitoring configuration
func (f *ClientFactory) ValidateConfig() error {
	if !f.config.UseProductionClients {
		f.logger.Info("Using stub monitoring clients, skipping validation")
		return nil
	}

	f.logger.Info("Validating production monitoring configuration")

	// Validate AlertManager configuration
	if f.config.AlertManagerConfig.Enabled {
		if f.config.AlertManagerConfig.Endpoint == "" {
			return fmt.Errorf("AlertManager endpoint is required when enabled")
		}
		if f.config.AlertManagerConfig.Timeout <= 0 {
			return fmt.Errorf("AlertManager timeout must be positive")
		}
	}

	// Validate Prometheus configuration
	if f.config.PrometheusConfig.Enabled {
		if f.config.PrometheusConfig.Endpoint == "" {
			return fmt.Errorf("prometheus endpoint is required when enabled")
		}
		if f.config.PrometheusConfig.Timeout <= 0 {
			return fmt.Errorf("prometheus timeout must be positive")
		}
	}

	f.logger.Info("Monitoring configuration validation passed")
	return nil
}

// InjectLLMHealthMonitor injects LLM health monitor after LLM client is available
// This follows dependency injection pattern and ensures proper integration
// BR-HEALTH-001: MUST implement comprehensive health checks for all components
func (f *ClientFactory) InjectLLMHealthMonitor(clients *MonitoringClients, llmClient llm.Client) {
	if clients == nil {
		f.logger.Error("Cannot inject LLM health monitor: monitoring clients is nil")
		return
	}

	if llmClient == nil {
		f.logger.Error("Cannot inject LLM health monitor: LLM client is nil")
		return
	}

	f.logger.Info("Injecting LLM health monitor with enterprise 20B+ model support")
	clients.HealthMonitor = NewLLMHealthMonitor(llmClient, f.logger)
}

// HealthCheck performs health checks on enabled monitoring clients
func (f *ClientFactory) HealthCheck(clients *MonitoringClients) error {
	if !f.config.UseProductionClients {
		f.logger.Debug("Skipping health checks for stub clients")
		return nil
	}

	f.logger.Info("Performing monitoring clients health check")

	// Check AlertManager health
	if f.config.AlertManagerConfig.Enabled {
		if alertClient, ok := clients.AlertClient.(*AlertManagerClient); ok {
			if err := alertClient.HealthCheck(context.Background()); err != nil {
				return fmt.Errorf("AlertManager health check failed: %w", err)
			}
			f.logger.Info("AlertManager health check passed")
		}
	}

	// Check Prometheus health
	if f.config.PrometheusConfig.Enabled {
		if promClient, ok := clients.MetricsClient.(*PrometheusClient); ok {
			if err := promClient.HealthCheck(context.Background()); err != nil {
				return fmt.Errorf("prometheus health check failed: %w", err)
			}
			f.logger.Info("Prometheus health check passed")
		}
	}

	f.logger.Info("All monitoring clients health checks passed")
	return nil
}

// GetConfigSummary returns a summary of the current configuration
func (f *ClientFactory) GetConfigSummary() map[string]interface{} {
	summary := map[string]interface{}{
		"use_production_clients": f.config.UseProductionClients,
		"alertmanager": map[string]interface{}{
			"enabled":  f.config.AlertManagerConfig.Enabled,
			"endpoint": f.config.AlertManagerConfig.Endpoint,
			"timeout":  f.config.AlertManagerConfig.Timeout.String(),
		},
		"prometheus": map[string]interface{}{
			"enabled":  f.config.PrometheusConfig.Enabled,
			"endpoint": f.config.PrometheusConfig.Endpoint,
			"timeout":  f.config.PrometheusConfig.Timeout.String(),
		},
	}

	return summary
}
