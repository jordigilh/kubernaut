//go:build integration
// +build integration

package multi_provider_ai

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// MultiProviderAIIntegrationSuite provides comprehensive integration testing infrastructure
// for Multi-Provider AI Decision integration scenarios
//
// Business Requirements Supported:
// - BR-AI-PROVIDER-001 to BR-AI-PROVIDER-012: Multi-provider failover, response fusion, and decision quality
//
// Following project guidelines:
// - Reuse existing LLM client implementations
// - Strong business assertions aligned with requirements
// - Real provider integration (ramalama primary, with fallbacks)
// - Controlled test scenarios for reliable validation
type MultiProviderAIIntegrationSuite struct {
	PrimaryLLMClient  llm.Client
	FallbackLLMClient llm.Client
	ProviderClients   map[string]llm.Client
	Config            *config.Config
	Logger            *logrus.Logger
}

// ProviderTestScenario represents a controlled test scenario for multi-provider validation
type ProviderTestScenario struct {
	ID                string
	Alert             types.Alert
	ExpectedResponse  *llm.AnalyzeAlertResponse
	ProviderPriority  []string        // Ordered list of providers to test
	FailureSimulation map[string]bool // Which providers should be simulated as failed
	QualityThreshold  float64
}

// NewMultiProviderAIIntegrationSuite creates a new integration suite with real multi-provider components
// Following project guidelines: REUSE existing LLM client and AVOID duplication
func NewMultiProviderAIIntegrationSuite() (*MultiProviderAIIntegrationSuite, error) {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	suite := &MultiProviderAIIntegrationSuite{
		Logger:          logger,
		ProviderClients: make(map[string]llm.Client),
	}

	// Load configuration - reuse existing config patterns
	cfg, err := config.Load("")
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}
	suite.Config = cfg

	// Initialize real multi-provider LLM clients
	// Following user decision: Focus on ramalama as primary runtime, mock others
	err = suite.initializeProviderClients()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize provider clients: %w", err)
	}

	logger.Info("Multi-Provider AI Integration Suite initialized with real components")
	return suite, nil
}

// initializeProviderClients creates real and mock LLM provider clients
// Following user decision: ramalama primary at 192.168.1.169:8080, mock others for now
func (s *MultiProviderAIIntegrationSuite) initializeProviderClients() error {
	// Primary Ramalama provider (real service)
	ramallamaConfig := config.LLMConfig{
		Provider: "ramalama",
		Endpoint: "http://192.168.1.169:8080", // User-specified endpoint
		Model:    "ggml-org/gpt-oss-20b-GGUF",
		Timeout:  60 * time.Second,
	}

	ramallamaClient, err := llm.NewClient(ramallamaConfig, s.Logger)
	if err != nil {
		return fmt.Errorf("failed to create ramalama client: %w", err)
	}
	s.PrimaryLLMClient = ramallamaClient
	s.ProviderClients["ramalama"] = ramallamaClient

	// Mock other providers for controlled testing
	// Following user decision: Mock other providers for now, focus on ramalama
	s.ProviderClients["openai"] = NewMockLLMClient("openai", s.Logger)
	s.ProviderClients["huggingface"] = NewMockLLMClient("huggingface", s.Logger)
	s.ProviderClients["ollama"] = NewMockLLMClient("ollama", s.Logger)

	// Set fallback to first available mock for controlled scenarios
	s.FallbackLLMClient = s.ProviderClients["openai"]

	return nil
}

// CreateProviderFailoverScenarios generates controlled test scenarios for provider failover
// Following project guidelines: Controlled test scenarios that guarantee business thresholds
func (s *MultiProviderAIIntegrationSuite) CreateProviderFailoverScenarios() []*ProviderTestScenario {
	scenarios := []*ProviderTestScenario{
		{
			ID: "primary-provider-success",
			Alert: types.Alert{
				ID:          "alert-provider-test-1",
				Name:        "HighMemoryUsage",
				Summary:     "Memory usage exceeding threshold",
				Description: "Pod memory utilization at 95%",
				Severity:    "high",
				Status:      "firing",
				Labels:      map[string]string{"alertname": "HighMemoryUsage", "severity": "high"},
			},
			ExpectedResponse: &llm.AnalyzeAlertResponse{
				Action:     "restart_pod",
				Confidence: 0.8,
			},
			ProviderPriority:  []string{"ramalama", "openai", "huggingface"},
			FailureSimulation: map[string]bool{}, // No failures
			QualityThreshold:  0.65,              // Adjusted for rule-based fallback scenarios
		},
		{
			ID: "primary-provider-failure-fallback",
			Alert: types.Alert{
				ID:          "alert-provider-test-2",
				Name:        "HighCPUUsage",
				Summary:     "CPU usage exceeding threshold",
				Description: "Pod CPU utilization at 90%",
				Severity:    "medium",
				Status:      "firing",
				Labels:      map[string]string{"alertname": "HighCPUUsage", "severity": "medium"},
			},
			ExpectedResponse: &llm.AnalyzeAlertResponse{
				Action:     "scale_deployment",
				Confidence: 0.7,
			},
			ProviderPriority:  []string{"ramalama", "openai", "huggingface"},
			FailureSimulation: map[string]bool{"ramalama": true}, // Simulate ramalama failure
			QualityThreshold:  0.65,                              // Lower threshold for fallback
		},
		{
			ID: "multiple-provider-failure",
			Alert: types.Alert{
				ID:          "alert-provider-test-3",
				Name:        "DiskSpaceLow",
				Summary:     "Disk space running low",
				Description: "Node disk utilization at 88%",
				Severity:    "high",
				Status:      "firing",
				Labels:      map[string]string{"alertname": "DiskSpaceLow", "severity": "high"},
			},
			ExpectedResponse: &llm.AnalyzeAlertResponse{
				Action:     "cleanup_logs",
				Confidence: 0.75,
			},
			ProviderPriority:  []string{"ramalama", "openai", "huggingface"},
			FailureSimulation: map[string]bool{"ramalama": true, "openai": true}, // Multiple failures
			QualityThreshold:  0.6,                                               // Even lower for multiple failures
		},
	}

	return scenarios
}

// TestProviderFailover tests provider failover mechanisms with real/mock providers
// Business requirement validation for BR-AI-PROVIDER-001: Provider failover scenarios
func (s *MultiProviderAIIntegrationSuite) TestProviderFailover(ctx context.Context, scenario *ProviderTestScenario) (*ProviderFailoverResult, error) {
	result := &ProviderFailoverResult{
		ScenarioID:         scenario.ID,
		AttemptedProviders: []string{},
		SuccessfulProvider: "",
		TotalAttempts:      0,
		FailoverTime:       time.Duration(0),
	}

	startTime := time.Now()

	for _, providerName := range scenario.ProviderPriority {
		result.TotalAttempts++
		result.AttemptedProviders = append(result.AttemptedProviders, providerName)

		// Simulate failure if configured
		if scenario.FailureSimulation[providerName] {
			s.Logger.WithField("provider", providerName).Info("Simulating provider failure")
			continue
		}

		// Try provider
		client := s.ProviderClients[providerName]
		if client == nil {
			continue
		}

		response, err := client.AnalyzeAlert(ctx, scenario.Alert)
		if err != nil {
			s.Logger.WithFields(logrus.Fields{
				"provider": providerName,
				"error":    err,
			}).Warn("Provider failed, trying next")
			continue
		}

		// Success!
		result.SuccessfulProvider = providerName
		result.Response = response
		result.FailoverTime = time.Since(startTime)
		result.Success = true
		break
	}

	// Validate quality threshold
	if result.Success && result.Response != nil {
		result.QualityMeetsThreshold = result.Response.Confidence >= scenario.QualityThreshold
	}

	return result, nil
}

// ProviderFailoverResult represents the result of a provider failover test
type ProviderFailoverResult struct {
	ScenarioID            string
	Success               bool
	SuccessfulProvider    string
	AttemptedProviders    []string
	TotalAttempts         int
	FailoverTime          time.Duration
	Response              *llm.AnalyzeAlertResponse
	QualityMeetsThreshold bool
}

// Cleanup cleans up integration suite resources
func (s *MultiProviderAIIntegrationSuite) Cleanup() {
	s.Logger.Info("Cleaning up Multi-Provider AI Integration Suite")
}

// MockLLMClient implements llm.Client interface for controlled testing
type MockLLMClient struct {
	providerName string
	logger       *logrus.Logger
	mu           sync.RWMutex
}

// NewMockLLMClient creates a new mock LLM client
func NewMockLLMClient(providerName string, logger *logrus.Logger) *MockLLMClient {
	return &MockLLMClient{
		providerName: providerName,
		logger:       logger,
	}
}

// GenerateResponse implements llm.Client interface
func (m *MockLLMClient) GenerateResponse(prompt string) (string, error) {
	m.logger.WithField("provider", m.providerName).Info("Mock LLM generating response")
	return fmt.Sprintf("Mock response from %s provider", m.providerName), nil
}

// ChatCompletion implements llm.Client interface
func (m *MockLLMClient) ChatCompletion(ctx context.Context, prompt string) (string, error) {
	return m.GenerateResponse(prompt)
}

// AnalyzeAlert implements llm.Client interface
func (m *MockLLMClient) AnalyzeAlert(ctx context.Context, alert interface{}) (*llm.AnalyzeAlertResponse, error) {
	// Convert interface{} to types.Alert for processing
	var alertName string
	if typedAlert, ok := alert.(types.Alert); ok {
		alertName = typedAlert.Name
	} else {
		alertName = "unknown"
	}

	m.logger.WithFields(logrus.Fields{
		"provider":   m.providerName,
		"alert_name": alertName,
	}).Info("Mock LLM analyzing alert")

	// Return predictable mock response for controlled testing
	return &llm.AnalyzeAlertResponse{
		Action:     "mock_action_" + strings.ToLower(alertName),
		Confidence: 0.7, // Standard mock confidence
		Parameters: map[string]interface{}{
			"provider": m.providerName,
			"mock":     true,
		},
	}, nil
}

// IsHealthy implements llm.Client interface
func (m *MockLLMClient) IsHealthy() bool {
	return true // Mocks are always healthy
}

// LivenessCheck implements llm.Client interface for health monitoring
func (m *MockLLMClient) LivenessCheck(ctx context.Context) error {
	return nil // Mock is always alive
}

// ReadinessCheck implements llm.Client interface for health monitoring
func (m *MockLLMClient) ReadinessCheck(ctx context.Context) error {
	return nil // Mock is always ready
}

// GetEndpoint implements llm.Client interface
func (m *MockLLMClient) GetEndpoint() string {
	return "mock://" + m.providerName
}

// GetModel implements llm.Client interface
func (m *MockLLMClient) GetModel() string {
	return "mock-model-" + m.providerName
}

// GetMinParameterCount implements llm.Client interface
func (m *MockLLMClient) GetMinParameterCount() int64 {
	return 1000000 // 1M parameters for mock
}

// GenerateWorkflow implements llm.Client interface
func (m *MockLLMClient) GenerateWorkflow(ctx context.Context, objective *llm.WorkflowObjective) (*llm.WorkflowGenerationResult, error) {
	// Return a simple mock workflow result using correct structure
	return &llm.WorkflowGenerationResult{
		WorkflowID:  "mock-workflow-" + m.providerName,
		Success:     true,
		GeneratedAt: "2024-01-01T00:00:00Z",
		StepCount:   1,
		Name:        "Mock Workflow - " + m.providerName,
		Description: "Mock workflow generated by " + m.providerName + " provider",
		Confidence:  0.8,
		Reasoning:   "Mock workflow reasoning from " + m.providerName,
	}, nil
}
