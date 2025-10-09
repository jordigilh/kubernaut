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
	"math/rand"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// CONSOLIDATED: ChaosExperiment definition moved to pkg/e2e/chaos/chaos_orchestration.go
// Following project guidelines: REUSE existing code and AVOID duplication
// Use: chaos.ChaosExperiment for all chaos engineering experiments

// BR-E2E-003: Realistic alert generation system for E2E testing
// Business Impact: Provides authentic alert scenarios for comprehensive workflow validation
// Stakeholder Value: Operations teams can validate complete alert-to-remediation workflows

// AlertGeneratorConfig defines configuration for alert generation
type AlertGeneratorConfig struct {
	AlertTypes        []AlertType     `yaml:"alert_types"`
	GenerationRate    time.Duration   `yaml:"generation_rate" default:"30s"`
	AlertSeverities   []AlertSeverity `yaml:"alert_severities"`
	TargetNamespaces  []string        `yaml:"target_namespaces"`
	EnableChaosAlerts bool            `yaml:"enable_chaos_alerts" default:"true"`
	MaxAlerts         int             `yaml:"max_alerts" default:"10"`

	// Production-like patterns
	BusinessHours  bool `yaml:"business_hours" default:"false"`
	WeekendPattern bool `yaml:"weekend_pattern" default:"false"`
	IncidentBursts bool `yaml:"incident_bursts" default:"true"`
}

// AlertType defines types of alerts to generate
type AlertType string

const (
	// Resource-related alerts
	AlertTypeHighCPU      AlertType = "high_cpu"
	AlertTypeHighMemory   AlertType = "high_memory"
	AlertTypeHighDisk     AlertType = "high_disk"
	AlertTypePodCrashLoop AlertType = "pod_crash_loop"
	AlertTypePodPending   AlertType = "pod_pending"

	// Application alerts
	AlertTypeHighLatency   AlertType = "high_latency"
	AlertTypeHighErrorRate AlertType = "high_error_rate"
	AlertTypeServiceDown   AlertType = "service_down"
	AlertTypeDatabaseSlow  AlertType = "database_slow"

	// Infrastructure alerts
	AlertTypeNodeDown      AlertType = "node_down"
	AlertTypeNetworkIssues AlertType = "network_issues"
	AlertTypeStorageIssues AlertType = "storage_issues"

	// Security alerts
	AlertTypeUnauthorizedAccess AlertType = "unauthorized_access"
	AlertTypeSuspiciousActivity AlertType = "suspicious_activity"

	// Chaos-induced alerts
	AlertTypeChaosExperiment AlertType = "chaos_experiment"
)

// AlertSeverity defines alert severity levels
type AlertSeverity string

const (
	AlertSeverityCritical AlertSeverity = "critical"
	AlertSeverityHigh     AlertSeverity = "high"
	AlertSeverityMedium   AlertSeverity = "medium"
	AlertSeverityLow      AlertSeverity = "low"
	AlertSeverityInfo     AlertSeverity = "info"
)

// GeneratedAlert represents a generated alert for E2E testing
type GeneratedAlert struct {
	ID          string            `json:"id"`
	Type        AlertType         `json:"type"`
	Severity    AlertSeverity     `json:"severity"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	Timestamp   time.Time         `json:"timestamp"`
	Namespace   string            `json:"namespace"`
	Source      string            `json:"source"`

	// E2E-specific fields
	ExpectedActions []string               `json:"expected_actions"`
	TestScenario    string                 `json:"test_scenario"`
	ValidationRules map[string]interface{} `json:"validation_rules"`
}

// E2EAlertGenerator generates realistic alerts for end-to-end testing
type E2EAlertGenerator struct {
	client kubernetes.Interface
	logger *logrus.Logger
	config *AlertGeneratorConfig

	// Generation state
	running         bool
	generatedAlerts map[string]*GeneratedAlert
	alertChannel    chan *GeneratedAlert

	// Pattern generators
	severityDistribution map[AlertSeverity]float64
	typeDistribution     map[AlertType]float64
}

// NewE2EAlertGenerator creates a new alert generator for E2E testing
// Business Requirement: BR-E2E-003 - Realistic alert generation for workflow validation
func NewE2EAlertGenerator(client kubernetes.Interface, config *AlertGeneratorConfig, logger *logrus.Logger) (*E2EAlertGenerator, error) {
	if client == nil {
		return nil, fmt.Errorf("Kubernetes client is required")
	}

	if config == nil {
		config = getDefaultAlertGeneratorConfig()
	}

	if logger == nil {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)
	}

	generator := &E2EAlertGenerator{
		client:          client,
		logger:          logger,
		config:          config,
		generatedAlerts: make(map[string]*GeneratedAlert),
		alertChannel:    make(chan *GeneratedAlert, 100),
	}

	// Initialize distribution patterns
	generator.initializeDistributions()

	logger.WithFields(logrus.Fields{
		"alert_types":     len(config.AlertTypes),
		"generation_rate": config.GenerationRate,
		"max_alerts":      config.MaxAlerts,
	}).Info("E2E alert generator created")

	return generator, nil
}

// initializeDistributions sets up realistic alert distribution patterns
func (gen *E2EAlertGenerator) initializeDistributions() {
	// Severity distribution (realistic production patterns)
	gen.severityDistribution = map[AlertSeverity]float64{
		AlertSeverityCritical: 0.05, // 5% critical
		AlertSeverityHigh:     0.15, // 15% high
		AlertSeverityMedium:   0.30, // 30% medium
		AlertSeverityLow:      0.35, // 35% low
		AlertSeverityInfo:     0.15, // 15% info
	}

	// Alert type distribution (based on kubernaut operational patterns)
	gen.typeDistribution = map[AlertType]float64{
		AlertTypeHighCPU:       0.20, // Most common
		AlertTypeHighMemory:    0.18,
		AlertTypePodCrashLoop:  0.15,
		AlertTypeHighLatency:   0.12,
		AlertTypeHighErrorRate: 0.10,
		AlertTypePodPending:    0.08,
		AlertTypeServiceDown:   0.05,
		AlertTypeNodeDown:      0.03,
		AlertTypeNetworkIssues: 0.04,
		AlertTypeStorageIssues: 0.03,
		AlertTypeDatabaseSlow:  0.02,
	}
}

// StartGeneration starts the alert generation process
// Business Requirement: BR-E2E-003 - Continuous alert generation for testing
func (gen *E2EAlertGenerator) StartGeneration(ctx context.Context) error {
	if gen.running {
		return fmt.Errorf("alert generation already running")
	}

	gen.running = true
	gen.logger.Info("Starting E2E alert generation")

	go gen.generationLoop(ctx)

	return nil
}

// generationLoop runs the continuous alert generation
func (gen *E2EAlertGenerator) generationLoop(ctx context.Context) {
	ticker := time.NewTicker(gen.config.GenerationRate)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			gen.logger.Info("Alert generation stopped due to context cancellation")
			gen.running = false
			return
		case <-ticker.C:
			if len(gen.generatedAlerts) >= gen.config.MaxAlerts {
				gen.logger.Debug("Maximum alerts reached, skipping generation")
				continue
			}

			// Generate new alert
			alert, err := gen.generateAlert()
			if err != nil {
				gen.logger.WithError(err).Warn("Failed to generate alert")
				continue
			}

			// Store and send alert
			gen.generatedAlerts[alert.ID] = alert

			select {
			case gen.alertChannel <- alert:
				gen.logger.WithFields(logrus.Fields{
					"alert_id":   alert.ID,
					"alert_type": alert.Type,
					"severity":   alert.Severity,
				}).Info("Alert generated")
			default:
				gen.logger.Warn("Alert channel full, dropping alert")
			}
		}
	}
}

// generateAlert generates a single realistic alert
func (gen *E2EAlertGenerator) generateAlert() (*GeneratedAlert, error) {
	alertType := gen.selectAlertType()
	severity := gen.selectSeverity()
	namespace := gen.selectNamespace()

	alert := &GeneratedAlert{
		ID:              generateAlertID(),
		Type:            alertType,
		Severity:        severity,
		Timestamp:       time.Now(),
		Namespace:       namespace,
		Source:          "e2e-generator",
		Labels:          make(map[string]string),
		Annotations:     make(map[string]string),
		ValidationRules: make(map[string]interface{}),
	}

	// Set basic labels
	alert.Labels["alertname"] = string(alertType)
	alert.Labels["severity"] = string(severity)
	alert.Labels["namespace"] = namespace
	alert.Labels["kubernaut.io/e2e"] = "true"
	alert.Labels["kubernaut.io/test-scenario"] = "alert-generation"

	// Generate type-specific alert details
	switch alertType {
	case AlertTypeHighCPU:
		gen.generateHighCPUAlert(alert)
	case AlertTypeHighMemory:
		gen.generateHighMemoryAlert(alert)
	case AlertTypePodCrashLoop:
		gen.generatePodCrashLoopAlert(alert)
	case AlertTypeHighLatency:
		gen.generateHighLatencyAlert(alert)
	case AlertTypeHighErrorRate:
		gen.generateHighErrorRateAlert(alert)
	case AlertTypePodPending:
		gen.generatePodPendingAlert(alert)
	case AlertTypeServiceDown:
		gen.generateServiceDownAlert(alert)
	case AlertTypeNodeDown:
		gen.generateNodeDownAlert(alert)
	default:
		return nil, fmt.Errorf("unsupported alert type: %s", alertType)
	}

	return alert, nil
}

// generateHighCPUAlert generates a high CPU alert
func (gen *E2EAlertGenerator) generateHighCPUAlert(alert *GeneratedAlert) {
	cpuUsage := 75.0 + rand.Float64()*20.0 // 75-95%
	podName := fmt.Sprintf("high-cpu-pod-%d", rand.Intn(100))

	alert.Title = fmt.Sprintf("High CPU usage detected: %.1f%%", cpuUsage)
	alert.Description = fmt.Sprintf("Pod %s in namespace %s is consuming %.1f%% CPU", podName, alert.Namespace, cpuUsage)

	alert.Labels["pod"] = podName
	alert.Labels["container"] = "main"
	alert.Annotations["cpu_usage"] = fmt.Sprintf("%.1f", cpuUsage)
	alert.Annotations["threshold"] = "75.0"

	// Expected actions for this alert type
	alert.ExpectedActions = []string{"scale_up", "investigate_cpu_usage", "optimize_resources"}
	alert.TestScenario = "resource-optimization"

	// Validation rules
	alert.ValidationRules["action_timeout"] = 300 // 5 minutes
	alert.ValidationRules["expected_resolution"] = "cpu_usage_reduced"
	alert.ValidationRules["success_threshold"] = 70.0
}

// generateHighMemoryAlert generates a high memory alert
func (gen *E2EAlertGenerator) generateHighMemoryAlert(alert *GeneratedAlert) {
	memoryUsage := 80.0 + rand.Float64()*15.0 // 80-95%
	podName := fmt.Sprintf("high-memory-pod-%d", rand.Intn(100))

	alert.Title = fmt.Sprintf("High memory usage detected: %.1f%%", memoryUsage)
	alert.Description = fmt.Sprintf("Pod %s in namespace %s is consuming %.1f%% memory", podName, alert.Namespace, memoryUsage)

	alert.Labels["pod"] = podName
	alert.Labels["container"] = "main"
	alert.Annotations["memory_usage"] = fmt.Sprintf("%.1f", memoryUsage)
	alert.Annotations["threshold"] = "80.0"

	alert.ExpectedActions = []string{"scale_up", "memory_optimization", "investigate_memory_leak"}
	alert.TestScenario = "memory-management"

	alert.ValidationRules["action_timeout"] = 240
	alert.ValidationRules["expected_resolution"] = "memory_usage_reduced"
	alert.ValidationRules["success_threshold"] = 75.0
}

// generatePodCrashLoopAlert generates a pod crash loop alert
func (gen *E2EAlertGenerator) generatePodCrashLoopAlert(alert *GeneratedAlert) {
	podName := fmt.Sprintf("crashloop-pod-%d", rand.Intn(100))
	restartCount := rand.Intn(10) + 5 // 5-15 restarts

	alert.Title = fmt.Sprintf("Pod crash loop detected: %s", podName)
	alert.Description = fmt.Sprintf("Pod %s in namespace %s has restarted %d times in the last 10 minutes", podName, alert.Namespace, restartCount)

	alert.Labels["pod"] = podName
	alert.Labels["reason"] = "CrashLoopBackOff"
	alert.Annotations["restart_count"] = fmt.Sprintf("%d", restartCount)
	alert.Annotations["crash_reason"] = "exit_code_1"

	alert.ExpectedActions = []string{"investigate_logs", "check_configuration", "rollback_deployment"}
	alert.TestScenario = "pod-stability"

	alert.ValidationRules["action_timeout"] = 600 // 10 minutes
	alert.ValidationRules["expected_resolution"] = "pod_stable"
	alert.ValidationRules["max_restarts_after_action"] = 2
}

// generateHighLatencyAlert generates a high latency alert
func (gen *E2EAlertGenerator) generateHighLatencyAlert(alert *GeneratedAlert) {
	latency := 500.0 + rand.Float64()*1000.0 // 500-1500ms
	serviceName := fmt.Sprintf("slow-service-%d", rand.Intn(20))

	alert.Title = fmt.Sprintf("High latency detected: %.1fms", latency)
	alert.Description = fmt.Sprintf("Service %s in namespace %s has average latency of %.1fms", serviceName, alert.Namespace, latency)

	alert.Labels["service"] = serviceName
	alert.Labels["endpoint"] = "/api/v1/data"
	alert.Annotations["avg_latency"] = fmt.Sprintf("%.1f", latency)
	alert.Annotations["threshold"] = "500.0"
	alert.Annotations["percentile_95"] = fmt.Sprintf("%.1f", latency*1.5)

	alert.ExpectedActions = []string{"investigate_performance", "scale_service", "optimize_database"}
	alert.TestScenario = "performance-optimization"

	alert.ValidationRules["action_timeout"] = 300
	alert.ValidationRules["expected_resolution"] = "latency_improved"
	alert.ValidationRules["success_threshold"] = 400.0
}

// generateHighErrorRateAlert generates a high error rate alert
func (gen *E2EAlertGenerator) generateHighErrorRateAlert(alert *GeneratedAlert) {
	errorRate := 5.0 + rand.Float64()*10.0 // 5-15%
	serviceName := fmt.Sprintf("error-service-%d", rand.Intn(20))

	alert.Title = fmt.Sprintf("High error rate detected: %.1f%%", errorRate)
	alert.Description = fmt.Sprintf("Service %s in namespace %s has error rate of %.1f%%", serviceName, alert.Namespace, errorRate)

	alert.Labels["service"] = serviceName
	alert.Labels["error_type"] = "5xx"
	alert.Annotations["error_rate"] = fmt.Sprintf("%.1f", errorRate)
	alert.Annotations["threshold"] = "5.0"
	alert.Annotations["total_requests"] = fmt.Sprintf("%d", rand.Intn(1000)+100)

	alert.ExpectedActions = []string{"investigate_errors", "check_dependencies", "review_logs"}
	alert.TestScenario = "error-handling"

	alert.ValidationRules["action_timeout"] = 240
	alert.ValidationRules["expected_resolution"] = "error_rate_reduced"
	alert.ValidationRules["success_threshold"] = 3.0
}

// generatePodPendingAlert generates a pod pending alert
func (gen *E2EAlertGenerator) generatePodPendingAlert(alert *GeneratedAlert) {
	podName := fmt.Sprintf("pending-pod-%d", rand.Intn(100))
	reason := []string{"Insufficient cpu", "Insufficient memory", "No nodes available"}[rand.Intn(3)]

	alert.Title = fmt.Sprintf("Pod pending: %s", podName)
	alert.Description = fmt.Sprintf("Pod %s in namespace %s is pending: %s", podName, alert.Namespace, reason)

	alert.Labels["pod"] = podName
	alert.Labels["reason"] = "Pending"
	alert.Annotations["pending_reason"] = reason
	alert.Annotations["pending_duration"] = "300s"

	alert.ExpectedActions = []string{"add_nodes", "adjust_resources", "investigate_scheduling"}
	alert.TestScenario = "resource-scheduling"

	alert.ValidationRules["action_timeout"] = 600
	alert.ValidationRules["expected_resolution"] = "pod_running"
}

// generateServiceDownAlert generates a service down alert
func (gen *E2EAlertGenerator) generateServiceDownAlert(alert *GeneratedAlert) {
	serviceName := fmt.Sprintf("down-service-%d", rand.Intn(20))

	alert.Title = fmt.Sprintf("Service down: %s", serviceName)
	alert.Description = fmt.Sprintf("Service %s in namespace %s is not responding", serviceName, alert.Namespace)

	alert.Labels["service"] = serviceName
	alert.Labels["reason"] = "ServiceUnavailable"
	alert.Annotations["last_seen"] = time.Now().Add(-5 * time.Minute).Format(time.RFC3339)
	alert.Annotations["health_check"] = "failed"

	alert.ExpectedActions = []string{"restart_service", "check_pods", "investigate_dependencies"}
	alert.TestScenario = "service-recovery"

	alert.ValidationRules["action_timeout"] = 300
	alert.ValidationRules["expected_resolution"] = "service_healthy"
}

// generateNodeDownAlert generates a node down alert
func (gen *E2EAlertGenerator) generateNodeDownAlert(alert *GeneratedAlert) {
	nodeName := fmt.Sprintf("node-%d", rand.Intn(10))

	alert.Title = fmt.Sprintf("Node down: %s", nodeName)
	alert.Description = fmt.Sprintf("Node %s is not ready", nodeName)

	alert.Labels["node"] = nodeName
	alert.Labels["reason"] = "NodeNotReady"
	alert.Annotations["last_heartbeat"] = time.Now().Add(-10 * time.Minute).Format(time.RFC3339)
	alert.Annotations["node_condition"] = "NotReady"

	alert.ExpectedActions = []string{"investigate_node", "cordon_node", "drain_workloads"}
	alert.TestScenario = "node-failure"

	alert.ValidationRules["action_timeout"] = 900 // 15 minutes
	alert.ValidationRules["expected_resolution"] = "workloads_rescheduled"
}

// selectAlertType selects an alert type based on distribution
func (gen *E2EAlertGenerator) selectAlertType() AlertType {
	r := rand.Float64()
	cumulative := 0.0

	for alertType, probability := range gen.typeDistribution {
		cumulative += probability
		if r <= cumulative {
			return alertType
		}
	}

	// Fallback to high CPU if distribution doesn't sum to 1
	return AlertTypeHighCPU
}

// selectSeverity selects an alert severity based on distribution
func (gen *E2EAlertGenerator) selectSeverity() AlertSeverity {
	r := rand.Float64()
	cumulative := 0.0

	for severity, probability := range gen.severityDistribution {
		cumulative += probability
		if r <= cumulative {
			return severity
		}
	}

	// Fallback to medium severity
	return AlertSeverityMedium
}

// selectNamespace selects a random namespace from the configured list
func (gen *E2EAlertGenerator) selectNamespace() string {
	if len(gen.config.TargetNamespaces) == 0 {
		return "default"
	}

	return gen.config.TargetNamespaces[rand.Intn(len(gen.config.TargetNamespaces))]
}

// GetAlertChannel returns the channel for receiving generated alerts
func (gen *E2EAlertGenerator) GetAlertChannel() <-chan *GeneratedAlert {
	return gen.alertChannel
}

// GetGeneratedAlerts returns all generated alerts
func (gen *E2EAlertGenerator) GetGeneratedAlerts() map[string]*GeneratedAlert {
	return gen.generatedAlerts
}

// StopGeneration stops the alert generation
func (gen *E2EAlertGenerator) StopGeneration() {
	gen.running = false
	gen.logger.Info("Alert generation stopped")
}

// IsRunning returns whether alert generation is running
func (gen *E2EAlertGenerator) IsRunning() bool {
	return gen.running
}

// ConvertToKubernautAlert converts a generated alert to kubernaut alert format
func (gen *E2EAlertGenerator) ConvertToKubernautAlert(alert *GeneratedAlert) types.Alert {
	return types.Alert{
		ID:          alert.ID,
		Name:        alert.Title,
		Summary:     alert.Title,
		Description: alert.Description,
		Severity:    string(alert.Severity),
		Status:      "firing",
		Labels:      alert.Labels,
		Annotations: alert.Annotations,
		StartsAt:    alert.Timestamp,
	}
}

// Helper functions

func generateAlertID() string {
	return fmt.Sprintf("e2e-alert-%d-%d", time.Now().Unix(), rand.Intn(10000))
}

func getDefaultAlertGeneratorConfig() *AlertGeneratorConfig {
	return &AlertGeneratorConfig{
		AlertTypes: []AlertType{
			AlertTypeHighCPU,
			AlertTypeHighMemory,
			AlertTypePodCrashLoop,
			AlertTypeHighLatency,
			AlertTypeHighErrorRate,
			AlertTypePodPending,
			AlertTypeServiceDown,
		},
		GenerationRate:    30 * time.Second,
		AlertSeverities:   []AlertSeverity{AlertSeverityCritical, AlertSeverityHigh, AlertSeverityMedium, AlertSeverityLow},
		TargetNamespaces:  []string{"default", "kubernaut-e2e", "monitoring"},
		EnableChaosAlerts: true,
		MaxAlerts:         10,
		BusinessHours:     false,
		WeekendPattern:    false,
		IncidentBursts:    true,
	}
}
