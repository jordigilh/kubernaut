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

//go:build e2e
// +build e2e

package framework

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"

	"github.com/jordigilh/kubernaut/pkg/e2e/chaos"
	"github.com/jordigilh/kubernaut/pkg/e2e/cluster"
	"github.com/jordigilh/kubernaut/pkg/e2e/monitoring"
	"github.com/jordigilh/kubernaut/pkg/e2e/validation"
)

// BR-E2E-001: Complete alert-to-remediation workflow validation
// Business Impact: Ensures kubernaut can handle complete production scenarios end-to-end
// Stakeholder Value: Operations teams gain confidence in autonomous remediation capabilities

// E2EFramework provides comprehensive end-to-end testing infrastructure
// Integrates OCP clusters, chaos engineering, monitoring, and validation
type E2EFramework struct {
	logger          *logrus.Logger
	clusterManager  *cluster.E2EClusterManager
	chaosEngine     *chaos.LitmusChaosEngine
	monitoringStack *monitoring.E2EMonitoringStack
	validator       *validation.E2EValidator

	// Framework state
	ctx            context.Context
	cancel         context.CancelFunc
	setupCompleted bool
	config         *E2EConfig
}

// E2EConfig defines configuration for end-to-end testing framework
type E2EConfig struct {
	// Cluster configuration
	ClusterType    string        `yaml:"cluster_type" default:"ocp"`     // "ocp" or "kind"
	ClusterVersion string        `yaml:"cluster_version" default:"4.18"` // OCP version
	NodeCount      int           `yaml:"node_count" default:"6"`         // 3 master + 3 worker
	SetupTimeout   time.Duration `yaml:"setup_timeout" default:"600s"`   // 10 minutes

	// Chaos engineering configuration
	EnableChaosEngineering bool     `yaml:"enable_chaos" default:"true"`
	ChaosNamespace         string   `yaml:"chaos_namespace" default:"litmus"`
	ChaosExperiments       []string `yaml:"chaos_experiments"`

	// Monitoring configuration
	MonitoringNamespace string `yaml:"monitoring_namespace" default:"monitoring"`
	PrometheusEnabled   bool   `yaml:"prometheus_enabled" default:"true"`
	AlertManagerEnabled bool   `yaml:"alertmanager_enabled" default:"true"`
	GrafanaEnabled      bool   `yaml:"grafana_enabled" default:"true"`

	// AI/ML integration
	LLMEndpoint       string `yaml:"llm_endpoint" default:"http://192.168.1.169:8080"`
	HolmesGPTEndpoint string `yaml:"holmesgpt_endpoint" default:"http://localhost:3000"`
	UseMockLLM        bool   `yaml:"use_mock_llm" default:"false"`

	// Performance requirements
	MaxTestDuration      time.Duration `yaml:"max_test_duration" default:"1800s"`    // 30 minutes
	PerformanceThreshold float64       `yaml:"performance_threshold" default:"0.95"` // 95% success rate

	// Environment
	Environment string         `yaml:"environment" default:"e2e"`
	Logger      *logrus.Logger `yaml:"-"`
}

// NewE2EFramework creates a new end-to-end testing framework
// Business Requirement: BR-E2E-001 - Complete alert-to-remediation workflow validation
func NewE2EFramework(config *E2EConfig) (*E2EFramework, error) {
	if config == nil {
		config = &E2EConfig{}
		applyE2EDefaults(config)
	}

	if config.Logger == nil {
		config.Logger = logrus.New()
		config.Logger.SetLevel(logrus.InfoLevel)
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.MaxTestDuration)

	framework := &E2EFramework{
		logger: config.Logger,
		ctx:    ctx,
		cancel: cancel,
		config: config,
	}

	framework.logger.WithFields(logrus.Fields{
		"cluster_type":  config.ClusterType,
		"chaos_enabled": config.EnableChaosEngineering,
		"max_duration":  config.MaxTestDuration,
		"environment":   config.Environment,
	}).Info("Initializing E2E testing framework")

	return framework, nil
}

// SetupE2EEnvironment sets up the complete end-to-end testing environment
// Business Requirement: BR-E2E-001 - Infrastructure setup for comprehensive testing
func (e2e *E2EFramework) SetupE2EEnvironment() error {
	if e2e.setupCompleted {
		return fmt.Errorf("E2E environment already set up")
	}

	e2e.logger.Info("Setting up E2E testing environment...")

	// Step 1: Setup cluster management
	clusterManager, err := cluster.NewE2EClusterManager(e2e.config.ClusterType, e2e.logger)
	if err != nil {
		return fmt.Errorf("failed to create cluster manager: %w", err)
	}
	e2e.clusterManager = clusterManager

	// Step 2: Initialize cluster
	if err := e2e.clusterManager.InitializeCluster(e2e.ctx, e2e.config.ClusterVersion); err != nil {
		return fmt.Errorf("failed to initialize cluster: %w", err)
	}

	// Step 3: Setup monitoring stack
	if e2e.config.PrometheusEnabled {
		monitoringStack, err := monitoring.NewE2EMonitoringStack(e2e.clusterManager.GetKubernetesClient(), e2e.logger)
		if err != nil {
			return fmt.Errorf("failed to create monitoring stack: %w", err)
		}
		e2e.monitoringStack = monitoringStack

		if err := e2e.monitoringStack.Deploy(e2e.ctx, e2e.config.MonitoringNamespace); err != nil {
			return fmt.Errorf("failed to deploy monitoring stack: %w", err)
		}
	}

	// Step 4: Setup chaos engineering
	if e2e.config.EnableChaosEngineering {
		chaosEngine, err := chaos.NewLitmusChaosEngine(e2e.clusterManager.GetKubernetesClient(), e2e.logger)
		if err != nil {
			return fmt.Errorf("failed to create chaos engine: %w", err)
		}
		e2e.chaosEngine = chaosEngine

		if err := e2e.chaosEngine.Setup(e2e.ctx, e2e.config.ChaosNamespace); err != nil {
			return fmt.Errorf("failed to setup chaos engine: %w", err)
		}
	}

	// Step 5: Setup validation framework
	validator, err := validation.NewE2EValidator(e2e.clusterManager.GetKubernetesClient(), e2e.logger)
	if err != nil {
		return fmt.Errorf("failed to create validator: %w", err)
	}
	e2e.validator = validator

	e2e.setupCompleted = true
	e2e.logger.Info("E2E testing environment setup completed successfully")

	return nil
}

// GetClusterManager returns the cluster manager for test usage
func (e2e *E2EFramework) GetClusterManager() *cluster.E2EClusterManager {
	return e2e.clusterManager
}

// GetChaosEngine returns the chaos engine for chaos testing
func (e2e *E2EFramework) GetChaosEngine() *chaos.LitmusChaosEngine {
	return e2e.chaosEngine
}

// GetMonitoringStack returns the monitoring stack for alert generation
func (e2e *E2EFramework) GetMonitoringStack() *monitoring.E2EMonitoringStack {
	return e2e.monitoringStack
}

// GetValidator returns the validation framework for result verification
func (e2e *E2EFramework) GetValidator() *validation.E2EValidator {
	return e2e.validator
}

// GetKubernetesClient returns the Kubernetes client for direct cluster access
func (e2e *E2EFramework) GetKubernetesClient() kubernetes.Interface {
	if e2e.clusterManager == nil {
		return nil
	}
	return e2e.clusterManager.GetKubernetesClient()
}

// Cleanup cleans up all E2E testing resources
func (e2e *E2EFramework) Cleanup() error {
	e2e.logger.Info("Cleaning up E2E testing environment...")

	var cleanupErrors []error

	// Cleanup chaos engine
	if e2e.chaosEngine != nil {
		if err := e2e.chaosEngine.Cleanup(e2e.ctx); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("chaos cleanup failed: %w", err))
		}
	}

	// Cleanup monitoring stack
	if e2e.monitoringStack != nil {
		if err := e2e.monitoringStack.Cleanup(e2e.ctx); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("monitoring cleanup failed: %w", err))
		}
	}

	// Cleanup cluster
	if e2e.clusterManager != nil {
		if err := e2e.clusterManager.Cleanup(e2e.ctx); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("cluster cleanup failed: %w", err))
		}
	}

	// Cancel context
	if e2e.cancel != nil {
		e2e.cancel()
	}

	if len(cleanupErrors) > 0 {
		return fmt.Errorf("cleanup errors: %v", cleanupErrors)
	}

	e2e.logger.Info("E2E testing environment cleanup completed")
	return nil
}

// IsSetupCompleted returns whether the E2E environment setup is completed
func (e2e *E2EFramework) IsSetupCompleted() bool {
	return e2e.setupCompleted
}

// GetConfig returns the E2E configuration
func (e2e *E2EFramework) GetConfig() *E2EConfig {
	return e2e.config
}

// applyE2EDefaults applies default configuration values
func applyE2EDefaults(config *E2EConfig) {
	if config.ClusterType == "" {
		config.ClusterType = "ocp"
	}
	if config.ClusterVersion == "" {
		config.ClusterVersion = "4.18"
	}
	if config.NodeCount == 0 {
		config.NodeCount = 6
	}
	if config.SetupTimeout == 0 {
		config.SetupTimeout = 600 * time.Second
	}
	if config.ChaosNamespace == "" {
		config.ChaosNamespace = "litmus"
	}
	if config.MonitoringNamespace == "" {
		config.MonitoringNamespace = "monitoring"
	}
	if config.LLMEndpoint == "" {
		config.LLMEndpoint = "http://192.168.1.169:8080"
	}
	if config.HolmesGPTEndpoint == "" {
		config.HolmesGPTEndpoint = "http://localhost:3000"
	}
	if config.MaxTestDuration == 0 {
		config.MaxTestDuration = 1800 * time.Second // 30 minutes
	}
	if config.PerformanceThreshold == 0 {
		config.PerformanceThreshold = 0.95
	}
	if config.Environment == "" {
		config.Environment = "e2e"
	}

	// Enable features by default
	config.EnableChaosEngineering = true
	config.PrometheusEnabled = true
	config.AlertManagerEnabled = true
	config.GrafanaEnabled = true

	// Set chaos experiments if not specified
	if len(config.ChaosExperiments) == 0 {
		config.ChaosExperiments = []string{
			"pod-delete",
			"node-cpu-hog",
			"node-memory-hog",
			"network-partition",
			"resource-exhaustion",
		}
	}
}

// GetE2EConfigFromEnv creates E2E configuration from environment variables
func GetE2EConfigFromEnv() *E2EConfig {
	config := &E2EConfig{}
	applyE2EDefaults(config)

	// Override with environment variables
	if clusterType := os.Getenv("E2E_CLUSTER_TYPE"); clusterType != "" {
		config.ClusterType = clusterType
	}

	if llmEndpoint := os.Getenv("LLM_ENDPOINT"); llmEndpoint != "" {
		config.LLMEndpoint = llmEndpoint
	}

	if holmesEndpoint := os.Getenv("HOLMESGPT_ENDPOINT"); holmesEndpoint != "" {
		config.HolmesGPTEndpoint = holmesEndpoint
	}

	if os.Getenv("USE_MOCK_LLM") == "true" {
		config.UseMockLLM = true
	}

	if os.Getenv("DISABLE_CHAOS") == "true" {
		config.EnableChaosEngineering = false
	}

	return config
}
