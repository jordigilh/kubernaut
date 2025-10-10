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
	"os"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/testutil/hybrid"
)

// Production AI Service Integration
// Business Requirements: BR-PRODUCTION-008 - Real AI Service Integration
// Following 00-project-guidelines.mdc: MANDATORY business requirement mapping
// Following 03-testing-strategy.mdc: Integration testing with real AI services
// Following 09-interface-method-validation.mdc: Interface validation before code generation

// ProductionAIIntegrator manages AI service integration with real Kubernetes clusters
type ProductionAIIntegrator struct {
	llmClient       llm.Client
	holmesGPTClient holmesgpt.Client
	logger          *logrus.Logger
	config          *AIIntegrationConfig
}

// AIIntegrationConfig defines configuration for AI service integration
type AIIntegrationConfig struct {
	LLMEndpoint        string                `yaml:"llm_endpoint"`
	HolmesGPTEndpoint  string                `yaml:"holmesgpt_endpoint"`
	HealthCheckTimeout time.Duration         `yaml:"health_check_timeout"`
	ValidationTimeout  time.Duration         `yaml:"validation_timeout"`
	EnableFallback     bool                  `yaml:"enable_fallback"`
	PerformanceTargets *AIPerformanceTargets `yaml:"performance_targets"`
}

// AIPerformanceTargets defines performance targets for AI services
type AIPerformanceTargets struct {
	LLMResponseTime       time.Duration `yaml:"llm_response_time"`       // Target: <30s
	HolmesGPTResponseTime time.Duration `yaml:"holmesgpt_response_time"` // Target: <45s
	HealthCheckTime       time.Duration `yaml:"health_check_time"`       // Target: <5s
	ValidationTime        time.Duration `yaml:"validation_time"`         // Target: <10s
}

// AIServiceStatus represents the status of AI services in production
type AIServiceStatus struct {
	LLMAvailable       bool                  `json:"llm_available"`
	HolmesGPTAvailable bool                  `json:"holmesgpt_available"`
	LLMEndpoint        string                `json:"llm_endpoint"`
	HolmesGPTEndpoint  string                `json:"holmesgpt_endpoint"`
	LastHealthCheck    time.Time             `json:"last_health_check"`
	PerformanceMetrics *AIPerformanceMetrics `json:"performance_metrics"`
	ValidationResults  *AIValidationResults  `json:"validation_results"`
	HealthCheckError   string                `json:"health_check_error,omitempty"`
}

// AIPerformanceMetrics tracks AI service performance
type AIPerformanceMetrics struct {
	LLMResponseTime       time.Duration `json:"llm_response_time"`
	HolmesGPTResponseTime time.Duration `json:"holmesgpt_response_time"`
	HealthCheckTime       time.Duration `json:"health_check_time"`
	ValidationTime        time.Duration `json:"validation_time"`
	SuccessRate           float64       `json:"success_rate"`
}

// AIValidationResults contains AI service validation results
type AIValidationResults struct {
	LLMValidation          bool   `json:"llm_validation"`
	HolmesGPTValidation    bool   `json:"holmesgpt_validation"`
	CrossServiceValidation bool   `json:"cross_service_validation"`
	ValidationDetails      string `json:"validation_details"`
}

// getEnvOrDefault returns environment variable value or default if not set
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// NewProductionAIIntegrator creates a new production AI integrator
// Business Requirement: BR-PRODUCTION-008 - Real AI service integration for production validation
func NewProductionAIIntegrator(config *AIIntegrationConfig, logger *logrus.Logger) (*ProductionAIIntegrator, error) {
	if logger == nil {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)
	}

	if config == nil {
		config = &AIIntegrationConfig{
			LLMEndpoint:        getEnvOrDefault("LLM_ENDPOINT", "http://192.168.1.169:8080"),
			HolmesGPTEndpoint:  getEnvOrDefault("HOLMESGPT_ENDPOINT", "http://localhost:3000"),
			HealthCheckTimeout: 5 * time.Second,
			ValidationTimeout:  10 * time.Second,
			EnableFallback:     true,
			PerformanceTargets: &AIPerformanceTargets{
				LLMResponseTime:       30 * time.Second,
				HolmesGPTResponseTime: 45 * time.Second,
				HealthCheckTime:       5 * time.Second,
				ValidationTime:        10 * time.Second,
			},
		}
	}

	// Create hybrid LLM client (real or mock based on environment)
	llmClient := hybrid.CreateLLMClient(logger)

	// Create HolmesGPT client
	holmesGPTClient, err := holmesgpt.NewClient(config.HolmesGPTEndpoint, "", logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create HolmesGPT client: %w", err)
	}

	integrator := &ProductionAIIntegrator{
		llmClient:       llmClient,
		holmesGPTClient: holmesGPTClient,
		logger:          logger,
		config:          config,
	}

	logger.WithFields(logrus.Fields{
		"llm_endpoint":       config.LLMEndpoint,
		"holmesgpt_endpoint": config.HolmesGPTEndpoint,
		"fallback_enabled":   config.EnableFallback,
	}).Info("Production AI integrator initialized")

	return integrator, nil
}

// ValidateAIServices validates AI services integration with real cluster
// Business Requirement: BR-PRODUCTION-008 - Comprehensive AI service validation
func (pai *ProductionAIIntegrator) ValidateAIServices(ctx context.Context, clusterEnv *RealClusterEnvironment) (*AIServiceStatus, error) {
	pai.logger.Info("Starting AI services validation with real cluster")

	status := &AIServiceStatus{
		LLMEndpoint:        pai.config.LLMEndpoint,
		HolmesGPTEndpoint:  pai.config.HolmesGPTEndpoint,
		LastHealthCheck:    time.Now(),
		PerformanceMetrics: &AIPerformanceMetrics{},
		ValidationResults:  &AIValidationResults{},
	}

	// Validate LLM service
	if err := pai.validateLLMService(ctx, status); err != nil {
		pai.logger.WithError(err).Warn("LLM service validation failed")
		if !pai.config.EnableFallback {
			return status, fmt.Errorf("LLM validation failed: %w", err)
		}
	}

	// Validate HolmesGPT service
	if err := pai.validateHolmesGPTService(ctx, status); err != nil {
		pai.logger.WithError(err).Warn("HolmesGPT service validation failed")
		if !pai.config.EnableFallback {
			return status, fmt.Errorf("HolmesGPT validation failed: %w", err)
		}
	}

	// Validate cross-service integration
	if err := pai.validateCrossServiceIntegration(ctx, status, clusterEnv); err != nil {
		pai.logger.WithError(err).Warn("Cross-service validation failed")
		status.ValidationResults.CrossServiceValidation = false
	} else {
		status.ValidationResults.CrossServiceValidation = true
	}

	// Calculate overall success rate
	successCount := 0
	totalChecks := 3
	if status.LLMAvailable {
		successCount++
	}
	if status.HolmesGPTAvailable {
		successCount++
	}
	if status.ValidationResults.CrossServiceValidation {
		successCount++
	}
	status.PerformanceMetrics.SuccessRate = float64(successCount) / float64(totalChecks)

	pai.logger.WithFields(logrus.Fields{
		"llm_available":       status.LLMAvailable,
		"holmesgpt_available": status.HolmesGPTAvailable,
		"cross_service_valid": status.ValidationResults.CrossServiceValidation,
		"success_rate":        status.PerformanceMetrics.SuccessRate,
	}).Info("AI services validation completed")

	return status, nil
}

// validateLLMService validates LLM service connectivity and performance
func (pai *ProductionAIIntegrator) validateLLMService(ctx context.Context, status *AIServiceStatus) error {
	pai.logger.Info("Validating LLM service")

	// Health check with timeout
	healthStart := time.Now()
	ctx, cancel := context.WithTimeout(ctx, pai.config.HealthCheckTimeout)
	defer cancel()

	isHealthy := pai.llmClient.IsHealthy()
	status.PerformanceMetrics.HealthCheckTime = time.Since(healthStart)

	if !isHealthy {
		status.LLMAvailable = false
		status.ValidationResults.LLMValidation = false
		status.HealthCheckError = "LLM service health check failed"
		return fmt.Errorf("LLM service is not healthy")
	}

	// Performance validation with simple prompt
	responseStart := time.Now()
	testPrompt := "Validate AI service integration with Kubernetes cluster"

	response, err := pai.llmClient.ChatCompletion(ctx, testPrompt)
	status.PerformanceMetrics.LLMResponseTime = time.Since(responseStart)

	if err != nil {
		status.LLMAvailable = false
		status.ValidationResults.LLMValidation = false
		return fmt.Errorf("LLM response validation failed: %w", err)
	}

	if len(response) == 0 {
		status.LLMAvailable = false
		status.ValidationResults.LLMValidation = false
		return fmt.Errorf("LLM returned empty response")
	}

	// Validate performance targets
	if status.PerformanceMetrics.LLMResponseTime > pai.config.PerformanceTargets.LLMResponseTime {
		pai.logger.WithFields(logrus.Fields{
			"actual_time": status.PerformanceMetrics.LLMResponseTime,
			"target_time": pai.config.PerformanceTargets.LLMResponseTime,
		}).Warn("LLM response time exceeds target")
	}

	status.LLMAvailable = true
	status.ValidationResults.LLMValidation = true

	pai.logger.WithFields(logrus.Fields{
		"response_time": status.PerformanceMetrics.LLMResponseTime,
		"health_time":   status.PerformanceMetrics.HealthCheckTime,
		"response_len":  len(response),
	}).Info("LLM service validation successful")

	return nil
}

// validateHolmesGPTService validates HolmesGPT service connectivity and performance
func (pai *ProductionAIIntegrator) validateHolmesGPTService(ctx context.Context, status *AIServiceStatus) error {
	pai.logger.Info("Validating HolmesGPT service")

	// Health check with timeout
	healthStart := time.Now()
	ctx, cancel := context.WithTimeout(ctx, pai.config.HealthCheckTimeout)
	defer cancel()

	err := pai.holmesGPTClient.GetHealth(ctx)
	healthTime := time.Since(healthStart)

	if status.PerformanceMetrics.HealthCheckTime < healthTime {
		status.PerformanceMetrics.HealthCheckTime = healthTime
	}

	if err != nil {
		status.HolmesGPTAvailable = false
		status.ValidationResults.HolmesGPTValidation = false
		status.HealthCheckError = fmt.Sprintf("HolmesGPT health check failed: %v", err)
		return fmt.Errorf("HolmesGPT service health check failed: %w", err)
	}

	// Performance validation with investigation request
	responseStart := time.Now()

	// Test investigation capability for comprehensive validation
	investigateReq := &holmesgpt.InvestigateRequest{
		AlertName:       "validation-test-alert",
		Namespace:       "default",
		Labels:          map[string]string{"test": "validation"},
		Annotations:     map[string]string{"purpose": "service-validation"},
		Priority:        "low",
		AsyncProcessing: false,
		IncludeContext:  true,
	}

	investigateResp, err := pai.holmesGPTClient.Investigate(ctx, investigateReq)
	holmesResponseTime := time.Since(responseStart)
	status.PerformanceMetrics.HolmesGPTResponseTime = holmesResponseTime

	if err != nil {
		pai.logger.WithError(err).Warn("HolmesGPT investigation test failed, but service is healthy")
		status.ValidationResults.HolmesGPTValidation = false
		// Don't fail validation if investigation fails but health check passes - graceful degradation
	} else {
		// Validate investigation response quality
		if investigateResp == nil {
			pai.logger.Warn("HolmesGPT investigation returned nil response")
			status.ValidationResults.HolmesGPTValidation = false
		} else if len(investigateResp.Summary) == 0 {
			pai.logger.Warn("HolmesGPT investigation returned empty summary")
			status.ValidationResults.HolmesGPTValidation = false
		} else if len(investigateResp.Recommendations) == 0 {
			pai.logger.Warn("HolmesGPT investigation returned no recommendations")
			status.ValidationResults.HolmesGPTValidation = false
		} else {
			pai.logger.WithFields(logrus.Fields{
				"investigation_id":      investigateResp.InvestigationID,
				"summary_length":        len(investigateResp.Summary),
				"recommendations_count": len(investigateResp.Recommendations),
			}).Info("HolmesGPT investigation validation successful")
			status.ValidationResults.HolmesGPTValidation = true
		}
	}

	// Validate performance targets
	if status.PerformanceMetrics.HolmesGPTResponseTime > pai.config.PerformanceTargets.HolmesGPTResponseTime {
		pai.logger.WithFields(logrus.Fields{
			"actual_time": status.PerformanceMetrics.HolmesGPTResponseTime,
			"target_time": pai.config.PerformanceTargets.HolmesGPTResponseTime,
		}).Warn("HolmesGPT response time exceeds target")
	}

	status.HolmesGPTAvailable = true
	// Note: HolmesGPTValidation is set based on investigation results above

	pai.logger.WithFields(logrus.Fields{
		"response_time": status.PerformanceMetrics.HolmesGPTResponseTime,
		"health_time":   healthTime,
	}).Info("HolmesGPT service validation successful")

	return nil
}

// validateCrossServiceIntegration validates AI services work together with real cluster
func (pai *ProductionAIIntegrator) validateCrossServiceIntegration(ctx context.Context, status *AIServiceStatus, clusterEnv *RealClusterEnvironment) error {
	pai.logger.Info("Validating cross-service AI integration with real cluster")

	validationStart := time.Now()
	defer func() {
		status.PerformanceMetrics.ValidationTime = time.Since(validationStart)
	}()

	// Only validate if both services are available
	if !status.LLMAvailable || !status.HolmesGPTAvailable {
		status.ValidationResults.ValidationDetails = "Cross-service validation skipped: not all services available"
		return fmt.Errorf("cannot validate cross-service integration: services unavailable")
	}

	// Get cluster information for context
	clusterInfo, err := clusterEnv.GetClusterInfo(ctx)
	if err != nil {
		status.ValidationResults.ValidationDetails = fmt.Sprintf("Failed to get cluster info: %v", err)
		return fmt.Errorf("failed to get cluster info for validation: %w", err)
	}

	// Create a realistic scenario for cross-service validation
	validationPrompt := fmt.Sprintf(
		"Analyze Kubernetes cluster with %d nodes and %d pods in scenario %s. Provide recommendations for optimization.",
		clusterInfo.NodeCount,
		clusterInfo.PodCount,
		clusterInfo.Scenario,
	)

	// Test LLM analysis
	llmResponse, err := pai.llmClient.ChatCompletion(ctx, validationPrompt)
	if err != nil {
		status.ValidationResults.ValidationDetails = fmt.Sprintf("LLM analysis failed: %v", err)
		return fmt.Errorf("LLM analysis failed in cross-service validation: %w", err)
	}

	// Validate response quality
	if len(llmResponse) < 50 {
		status.ValidationResults.ValidationDetails = "LLM response too short for meaningful analysis"
		return fmt.Errorf("LLM response quality insufficient")
	}

	// Test HolmesGPT integration with cross-service validation
	// Create investigation request for cross-service validation
	investigateReq := &holmesgpt.InvestigateRequest{
		AlertName:       fmt.Sprintf("cluster-performance-validation-%d", time.Now().Unix()),
		Namespace:       "kube-system",
		Labels:          map[string]string{"nodes": fmt.Sprintf("%d", clusterInfo.NodeCount), "pods": fmt.Sprintf("%d", clusterInfo.PodCount)},
		Annotations:     map[string]string{"scenario": clusterInfo.Scenario, "validation": "cross-service"},
		Priority:        "medium",
		AsyncProcessing: false,
		IncludeContext:  true,
	}

	holmesResp, holmesErr := pai.holmesGPTClient.Investigate(ctx, investigateReq)
	if holmesErr != nil {
		pai.logger.WithError(holmesErr).Debug("HolmesGPT investigation in cross-service validation failed")
		status.ValidationResults.ValidationDetails = fmt.Sprintf("Cross-service validation partial: LLM successful (%d chars), HolmesGPT investigation failed", len(llmResponse))
		// Don't fail cross-service validation if investigation fails but other validations pass
	} else {
		// Validate cross-service investigation results
		if holmesResp == nil || len(holmesResp.Summary) == 0 {
			pai.logger.Warn("HolmesGPT cross-service investigation returned insufficient results")
			status.ValidationResults.ValidationDetails = fmt.Sprintf("Cross-service validation partial: LLM successful (%d chars), HolmesGPT investigation insufficient", len(llmResponse))
		} else {
			pai.logger.WithFields(logrus.Fields{
				"investigation_id": holmesResp.InvestigationID,
				"summary_length":   len(holmesResp.Summary),
				"recommendations":  len(holmesResp.Recommendations),
			}).Info("Cross-service HolmesGPT investigation successful")
		}
	}

	// Set final validation details if not already set by investigation results
	if status.ValidationResults.ValidationDetails == "" {
		status.ValidationResults.ValidationDetails = fmt.Sprintf(
			"Cross-service validation successful: LLM response %d chars, cluster %d nodes/%d pods",
			len(llmResponse),
			clusterInfo.NodeCount,
			clusterInfo.PodCount,
		)
	}

	pai.logger.WithFields(logrus.Fields{
		"llm_response_len": len(llmResponse),
		"cluster_nodes":    clusterInfo.NodeCount,
		"cluster_pods":     clusterInfo.PodCount,
		"validation_time":  status.PerformanceMetrics.ValidationTime,
	}).Info("Cross-service AI integration validation successful")

	return nil
}

// GetAIServiceMetrics returns current AI service performance metrics
func (pai *ProductionAIIntegrator) GetAIServiceMetrics() map[string]interface{} {
	return map[string]interface{}{
		"llm_endpoint":       pai.config.LLMEndpoint,
		"holmesgpt_endpoint": pai.config.HolmesGPTEndpoint,
		"fallback_enabled":   pai.config.EnableFallback,
		"performance_targets": map[string]interface{}{
			"llm_response_time":       pai.config.PerformanceTargets.LLMResponseTime.String(),
			"holmesgpt_response_time": pai.config.PerformanceTargets.HolmesGPTResponseTime.String(),
			"health_check_time":       pai.config.PerformanceTargets.HealthCheckTime.String(),
			"validation_time":         pai.config.PerformanceTargets.ValidationTime.String(),
		},
	}
}
