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

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// HTTPLLMClient provides LLM analysis via AI Service HTTP API
// Business Requirement: BR-LLM-CENTRAL-001 - Centralized LLM access via AI Service
//
// This client implements the llm.Client interface by forwarding requests to a dedicated
// AI Service via HTTP, enabling microservices architecture with clear separation of concerns.
// The AI Service handles all LLM operations, providing fault isolation and independent scaling.
type HTTPLLMClient struct {
	endpoint   string
	httpClient *http.Client
	logger     *logrus.Logger
}

// NewHTTPLLMClient creates HTTP-based LLM client for microservices architecture
// Business Requirement: BR-LLM-CENTRAL-001 - Use AI Service for centralized LLM operations
func NewHTTPLLMClient(aiServiceEndpoint string) llm.Client {
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})

	return &HTTPLLMClient{
		endpoint:   aiServiceEndpoint,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		logger:     logger,
	}
}

// AnalyzeAlert implements llm.Client interface via HTTP call to AI Service
// Business Requirement: BR-LLM-CENTRAL-001 - Forward alert analysis to centralized AI Service
func (h *HTTPLLMClient) AnalyzeAlert(ctx context.Context, alert interface{}) (*llm.AnalyzeAlertResponse, error) {
	// Type validation with proper error context
	alertData, ok := alert.(types.Alert)
	if !ok {
		h.logger.WithField("alert_type", fmt.Sprintf("%T", alert)).Error("Invalid alert type provided to HTTP LLM client")
		return nil, fmt.Errorf("HTTP LLM client requires types.Alert, received %T: %w", alert, fmt.Errorf("type assertion failed"))
	}

	// Prepare request payload with proper error handling
	payload := map[string]interface{}{"alert": alertData}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		h.logger.WithError(err).Error("Failed to marshal alert payload for AI Service")
		return nil, fmt.Errorf("failed to marshal alert payload: %w", err)
	}

	// Create HTTP request with context
	endpoint := h.endpoint + "/api/v1/analyze-alert"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(jsonPayload))
	if err != nil {
		h.logger.WithError(err).WithField("endpoint", endpoint).Error("Failed to create HTTP request to AI Service")
		return nil, fmt.Errorf("failed to create HTTP request to AI Service: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "kubernaut-http-llm-client/1.0")

	// Execute HTTP request with structured logging
	h.logger.WithFields(logrus.Fields{
		"endpoint": endpoint,
		"method":   "POST",
		"alert_id": alertData.ID,
	}).Debug("Sending alert analysis request to AI Service")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		h.logger.WithError(err).WithField("endpoint", endpoint).Error("HTTP request to AI Service failed")
		return nil, fmt.Errorf("HTTP request to AI Service failed: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			h.logger.WithError(closeErr).Warn("Failed to close HTTP response body")
		}
	}()

	// Validate HTTP response status
	if resp.StatusCode != http.StatusOK {
		h.logger.WithFields(logrus.Fields{
			"status_code": resp.StatusCode,
			"endpoint":    endpoint,
		}).Error("AI Service returned non-OK status")
		return nil, fmt.Errorf("AI Service returned status %d from %s: %w", resp.StatusCode, endpoint, fmt.Errorf("HTTP error"))
	}

	// Parse response with proper error handling
	var response llm.AnalyzeAlertResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		h.logger.WithError(err).Error("Failed to decode AI Service response")
		return nil, fmt.Errorf("failed to decode AI Service response: %w", err)
	}

	h.logger.WithFields(logrus.Fields{
		"alert_id":   alertData.ID,
		"action":     response.Action,
		"confidence": response.Confidence,
	}).Info("Successfully received alert analysis from AI Service")

	return &response, nil
}

// Core LLM Interface Methods

// GenerateResponse forwards prompt to AI Service for response generation
// Business Requirement: BR-LLM-CENTRAL-001 - Centralized LLM operations
func (h *HTTPLLMClient) GenerateResponse(prompt string) (string, error) {
	h.logger.WithField("prompt_length", len(prompt)).Debug("GenerateResponse called on HTTP LLM client")
	// TODO: Implement HTTP forwarding to AI Service /api/v1/generate endpoint
	return "http response", fmt.Errorf("GenerateResponse not yet implemented for HTTP LLM client: %w", fmt.Errorf("method stub"))
}

// ChatCompletion forwards chat completion request to AI Service
// Business Requirement: BR-LLM-CENTRAL-001 - Centralized LLM operations
func (h *HTTPLLMClient) ChatCompletion(ctx context.Context, prompt string) (string, error) {
	h.logger.WithFields(logrus.Fields{
		"prompt_length": len(prompt),
		"context":       ctx != nil,
	}).Debug("ChatCompletion called on HTTP LLM client")
	// TODO: Implement HTTP forwarding to AI Service /api/v1/chat endpoint
	return "http completion", fmt.Errorf("ChatCompletion not yet implemented for HTTP LLM client: %w", fmt.Errorf("method stub"))
}

// GenerateWorkflow forwards workflow generation to AI Service
// Business Requirement: BR-LLM-CENTRAL-001 - Centralized workflow generation
func (h *HTTPLLMClient) GenerateWorkflow(ctx context.Context, objective *llm.WorkflowObjective) (*llm.WorkflowGenerationResult, error) {
	h.logger.WithField("objective", objective != nil).Debug("GenerateWorkflow called on HTTP LLM client")
	// TODO: Implement HTTP forwarding to AI Service /api/v1/generate-workflow endpoint
	return &llm.WorkflowGenerationResult{Success: true}, fmt.Errorf("GenerateWorkflow not yet implemented for HTTP LLM client: %w", fmt.Errorf("method stub"))
}

// Health Check Methods

// IsHealthy checks if the HTTP LLM client can reach the AI Service
func (h *HTTPLLMClient) IsHealthy() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := h.LivenessCheck(ctx)
	return err == nil
}

// LivenessCheck performs HTTP health check against AI Service
// Business Requirement: BR-HEALTH-002 - Liveness probes for Kubernetes
func (h *HTTPLLMClient) LivenessCheck(ctx context.Context) error {
	endpoint := h.endpoint + "/health/live"
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		h.logger.WithError(err).Error("Failed to create liveness check request")
		return fmt.Errorf("failed to create liveness check request: %w", err)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		h.logger.WithError(err).WithField("endpoint", endpoint).Error("Liveness check failed")
		return fmt.Errorf("liveness check failed for AI Service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		h.logger.WithField("status_code", resp.StatusCode).Error("AI Service liveness check returned non-OK status")
		return fmt.Errorf("AI Service liveness check returned status %d: %w", resp.StatusCode, fmt.Errorf("health check failed"))
	}

	return nil
}

// ReadinessCheck performs HTTP readiness check against AI Service
// Business Requirement: BR-HEALTH-002 - Readiness probes for Kubernetes
func (h *HTTPLLMClient) ReadinessCheck(ctx context.Context) error {
	endpoint := h.endpoint + "/health/ready"
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		h.logger.WithError(err).Error("Failed to create readiness check request")
		return fmt.Errorf("failed to create readiness check request: %w", err)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		h.logger.WithError(err).WithField("endpoint", endpoint).Error("Readiness check failed")
		return fmt.Errorf("readiness check failed for AI Service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		h.logger.WithField("status_code", resp.StatusCode).Error("AI Service readiness check returned non-OK status")
		return fmt.Errorf("AI Service readiness check returned status %d: %w", resp.StatusCode, fmt.Errorf("health check failed"))
	}

	return nil
}

// Configuration Methods

// GetEndpoint returns the AI Service endpoint URL
func (h *HTTPLLMClient) GetEndpoint() string {
	return h.endpoint
}

// GetModel returns the model identifier for HTTP LLM client
func (h *HTTPLLMClient) GetModel() string {
	return "http-llm-client-v1"
}

// GetMinParameterCount returns minimum parameter count for HTTP client
func (h *HTTPLLMClient) GetMinParameterCount() int64 {
	return 0 // HTTP client doesn't have parameter constraints
}

// AI Condition Evaluation Methods

// EvaluateCondition forwards condition evaluation to AI Service
// Business Requirement: BR-COND-001 - Intelligent condition evaluation with context awareness
func (h *HTTPLLMClient) EvaluateCondition(ctx context.Context, condition interface{}, contextData interface{}) (bool, error) {
	h.logger.WithFields(logrus.Fields{
		"condition_type": fmt.Sprintf("%T", condition),
		"context_type":   fmt.Sprintf("%T", contextData),
	}).Debug("EvaluateCondition called on HTTP LLM client")
	// TODO: Implement HTTP forwarding to AI Service /api/v1/evaluate-condition endpoint
	return true, fmt.Errorf("EvaluateCondition not yet implemented for HTTP LLM client: %w", fmt.Errorf("method stub"))
}

// ValidateCondition forwards condition validation to AI Service
// Business Requirement: BR-COND-001 - Condition validation via centralized AI
func (h *HTTPLLMClient) ValidateCondition(ctx context.Context, condition interface{}) error {
	h.logger.WithField("condition_type", fmt.Sprintf("%T", condition)).Debug("ValidateCondition called on HTTP LLM client")
	// TODO: Implement HTTP forwarding to AI Service /api/v1/validate-condition endpoint
	return fmt.Errorf("ValidateCondition not yet implemented for HTTP LLM client: %w", fmt.Errorf("method stub"))
}

// AI Metrics Collection Methods

// CollectMetrics forwards metrics collection to AI Service
// Business Requirement: BR-AI-017, BR-AI-025 - Comprehensive AI metrics collection
func (h *HTTPLLMClient) CollectMetrics(ctx context.Context, execution interface{}) (map[string]float64, error) {
	h.logger.WithField("execution_type", fmt.Sprintf("%T", execution)).Debug("CollectMetrics called on HTTP LLM client")
	// TODO: Implement HTTP forwarding to AI Service /api/v1/collect-metrics endpoint
	return map[string]float64{"http_client_stub": 1.0}, fmt.Errorf("CollectMetrics not yet implemented for HTTP LLM client: %w", fmt.Errorf("method stub"))
}

// GetAggregatedMetrics forwards aggregated metrics request to AI Service
// Business Requirement: BR-AI-017, BR-AI-025 - AI metrics analysis
func (h *HTTPLLMClient) GetAggregatedMetrics(ctx context.Context, workflowID string, timeRange interface{}) (map[string]float64, error) {
	h.logger.WithFields(logrus.Fields{
		"workflow_id":     workflowID,
		"time_range_type": fmt.Sprintf("%T", timeRange),
	}).Debug("GetAggregatedMetrics called on HTTP LLM client")
	// TODO: Implement HTTP forwarding to AI Service /api/v1/aggregated-metrics endpoint
	return map[string]float64{}, fmt.Errorf("GetAggregatedMetrics not yet implemented for HTTP LLM client: %w", fmt.Errorf("method stub"))
}

// RecordAIRequest forwards AI request recording to AI Service
// Business Requirement: BR-AI-017 - AI request tracking and analysis
func (h *HTTPLLMClient) RecordAIRequest(ctx context.Context, requestID string, prompt string, response string) error {
	h.logger.WithFields(logrus.Fields{
		"request_id":      requestID,
		"prompt_length":   len(prompt),
		"response_length": len(response),
	}).Debug("RecordAIRequest called on HTTP LLM client")
	// TODO: Implement HTTP forwarding to AI Service /api/v1/record-request endpoint
	return fmt.Errorf("RecordAIRequest not yet implemented for HTTP LLM client: %w", fmt.Errorf("method stub"))
}

// AI Prompt Optimization Methods

// RegisterPromptVersion forwards prompt version registration to AI Service
// Business Requirement: BR-AI-022 - Prompt optimization and A/B testing
func (h *HTTPLLMClient) RegisterPromptVersion(ctx context.Context, version interface{}) error {
	h.logger.WithField("version_type", fmt.Sprintf("%T", version)).Debug("RegisterPromptVersion called on HTTP LLM client")
	// TODO: Implement HTTP forwarding to AI Service /api/v1/register-prompt-version endpoint
	return fmt.Errorf("RegisterPromptVersion not yet implemented for HTTP LLM client: %w", fmt.Errorf("method stub"))
}

// GetOptimalPrompt forwards optimal prompt request to AI Service
// Business Requirement: BR-AI-022 - Prompt optimization via centralized AI
func (h *HTTPLLMClient) GetOptimalPrompt(ctx context.Context, objective interface{}) (interface{}, error) {
	h.logger.WithField("objective_type", fmt.Sprintf("%T", objective)).Debug("GetOptimalPrompt called on HTTP LLM client")
	// TODO: Implement HTTP forwarding to AI Service /api/v1/optimal-prompt endpoint
	return "http prompt", fmt.Errorf("GetOptimalPrompt not yet implemented for HTTP LLM client: %w", fmt.Errorf("method stub"))
}

// StartABTest forwards A/B test initiation to AI Service
// Business Requirement: BR-AI-022 - A/B testing for prompt optimization
func (h *HTTPLLMClient) StartABTest(ctx context.Context, experiment interface{}) error {
	h.logger.WithField("experiment_type", fmt.Sprintf("%T", experiment)).Debug("StartABTest called on HTTP LLM client")
	// TODO: Implement HTTP forwarding to AI Service /api/v1/start-ab-test endpoint
	return fmt.Errorf("StartABTest not yet implemented for HTTP LLM client: %w", fmt.Errorf("method stub"))
}

// Workflow Optimization Methods

// OptimizeWorkflow forwards workflow optimization to AI Service
// Business Requirement: BR-ORCH-003 - Workflow optimization and improvement suggestions
func (h *HTTPLLMClient) OptimizeWorkflow(ctx context.Context, workflow interface{}, executionHistory interface{}) (interface{}, error) {
	h.logger.WithFields(logrus.Fields{
		"workflow_type": fmt.Sprintf("%T", workflow),
		"history_type":  fmt.Sprintf("%T", executionHistory),
	}).Debug("OptimizeWorkflow called on HTTP LLM client")
	// TODO: Implement HTTP forwarding to AI Service /api/v1/optimize-workflow endpoint
	return "http optimization", fmt.Errorf("OptimizeWorkflow not yet implemented for HTTP LLM client: %w", fmt.Errorf("method stub"))
}

// SuggestOptimizations forwards optimization suggestions to AI Service
// Business Requirement: BR-ORCH-003 - AI-powered workflow improvement suggestions
func (h *HTTPLLMClient) SuggestOptimizations(ctx context.Context, workflow interface{}) (interface{}, error) {
	h.logger.WithField("workflow_type", fmt.Sprintf("%T", workflow)).Debug("SuggestOptimizations called on HTTP LLM client")
	// TODO: Implement HTTP forwarding to AI Service /api/v1/suggest-optimizations endpoint
	return "http suggestions", fmt.Errorf("SuggestOptimizations not yet implemented for HTTP LLM client: %w", fmt.Errorf("method stub"))
}

// Prompt Building Methods

// BuildPrompt forwards prompt building to AI Service
// Business Requirement: BR-PROMPT-001 - Dynamic prompt building and template optimization
func (h *HTTPLLMClient) BuildPrompt(ctx context.Context, template string, contextData map[string]interface{}) (string, error) {
	h.logger.WithFields(logrus.Fields{
		"template_length": len(template),
		"context_keys":    len(contextData),
	}).Debug("BuildPrompt called on HTTP LLM client")
	// TODO: Implement HTTP forwarding to AI Service /api/v1/build-prompt endpoint
	return "http built prompt", fmt.Errorf("BuildPrompt not yet implemented for HTTP LLM client: %w", fmt.Errorf("method stub"))
}

// LearnFromExecution forwards execution learning to AI Service
// Business Requirement: BR-PROMPT-001 - Learning from execution for prompt optimization
func (h *HTTPLLMClient) LearnFromExecution(ctx context.Context, execution interface{}) error {
	h.logger.WithField("execution_type", fmt.Sprintf("%T", execution)).Debug("LearnFromExecution called on HTTP LLM client")
	// TODO: Implement HTTP forwarding to AI Service /api/v1/learn-from-execution endpoint
	return fmt.Errorf("LearnFromExecution not yet implemented for HTTP LLM client: %w", fmt.Errorf("method stub"))
}

// GetOptimizedTemplate forwards template optimization request to AI Service
// Business Requirement: BR-PROMPT-001 - Template optimization via centralized AI
func (h *HTTPLLMClient) GetOptimizedTemplate(ctx context.Context, templateID string) (string, error) {
	h.logger.WithField("template_id", templateID).Debug("GetOptimizedTemplate called on HTTP LLM client")
	// TODO: Implement HTTP forwarding to AI Service /api/v1/optimized-template endpoint
	return "http optimized template", fmt.Errorf("GetOptimizedTemplate not yet implemented for HTTP LLM client: %w", fmt.Errorf("method stub"))
}

// Machine Learning Analytics Methods

// AnalyzePatterns forwards pattern analysis to AI Service
// Business Requirement: BR-ML-001 - Machine learning analytics for pattern discovery
func (h *HTTPLLMClient) AnalyzePatterns(ctx context.Context, executionData []interface{}) (interface{}, error) {
	h.logger.WithField("data_points", len(executionData)).Debug("AnalyzePatterns called on HTTP LLM client")
	// TODO: Implement HTTP forwarding to AI Service /api/v1/analyze-patterns endpoint
	return "http pattern analysis", fmt.Errorf("AnalyzePatterns not yet implemented for HTTP LLM client: %w", fmt.Errorf("method stub"))
}

// PredictEffectiveness forwards effectiveness prediction to AI Service
// Business Requirement: BR-ML-001 - ML-based workflow effectiveness prediction
func (h *HTTPLLMClient) PredictEffectiveness(ctx context.Context, workflow interface{}) (float64, error) {
	h.logger.WithField("workflow_type", fmt.Sprintf("%T", workflow)).Debug("PredictEffectiveness called on HTTP LLM client")
	// TODO: Implement HTTP forwarding to AI Service /api/v1/predict-effectiveness endpoint
	return 0.8, fmt.Errorf("PredictEffectiveness not yet implemented for HTTP LLM client: %w", fmt.Errorf("method stub"))
}

// ClusterWorkflows forwards workflow clustering to AI Service
// Business Requirement: BR-CLUSTER-001 - Workflow clustering and similarity analysis
func (h *HTTPLLMClient) ClusterWorkflows(ctx context.Context, executionData []interface{}, config map[string]interface{}) (interface{}, error) {
	h.logger.WithFields(logrus.Fields{
		"data_points":   len(executionData),
		"config_params": len(config),
	}).Debug("ClusterWorkflows called on HTTP LLM client")
	// TODO: Implement HTTP forwarding to AI Service /api/v1/cluster-workflows endpoint
	return "http workflow clusters", fmt.Errorf("ClusterWorkflows not yet implemented for HTTP LLM client: %w", fmt.Errorf("method stub"))
}

// Time Series Analysis Methods

// AnalyzeTrends forwards trend analysis to AI Service
// Business Requirement: BR-TIMESERIES-001 - Time series analysis capabilities
func (h *HTTPLLMClient) AnalyzeTrends(ctx context.Context, executionData []interface{}, timeRange interface{}) (interface{}, error) {
	h.logger.WithFields(logrus.Fields{
		"data_points":     len(executionData),
		"time_range_type": fmt.Sprintf("%T", timeRange),
	}).Debug("AnalyzeTrends called on HTTP LLM client")
	// TODO: Implement HTTP forwarding to AI Service /api/v1/analyze-trends endpoint
	return "http trend analysis", fmt.Errorf("AnalyzeTrends not yet implemented for HTTP LLM client: %w", fmt.Errorf("method stub"))
}

// DetectAnomalies forwards anomaly detection to AI Service
// Business Requirement: BR-TIMESERIES-001 - Anomaly detection in execution data
func (h *HTTPLLMClient) DetectAnomalies(ctx context.Context, executionData []interface{}) (interface{}, error) {
	h.logger.WithField("data_points", len(executionData)).Debug("DetectAnomalies called on HTTP LLM client")
	// TODO: Implement HTTP forwarding to AI Service /api/v1/detect-anomalies endpoint
	return "http anomaly detection", fmt.Errorf("DetectAnomalies not yet implemented for HTTP LLM client: %w", fmt.Errorf("method stub"))
}
