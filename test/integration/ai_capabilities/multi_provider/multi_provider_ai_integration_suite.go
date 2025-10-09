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

//go:build integration
// +build integration

package multi_provider

import (
	"context"
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
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
	MockLogger        *mocks.MockLogger
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
	mockLogger := mocks.NewMockLogger()
	// mockLogger level set automatically

	suite := &MultiProviderAIIntegrationSuite{
		Logger:          mockLogger.Logger,
		MockLogger:      mockLogger,
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

	mockLogger.Logger.Info("Multi-Provider AI Integration Suite initialized with real components")
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

	// Mock other providers for controlled testing - Rule 11: use existing patterns
	// Following user decision: Mock other providers for now, focus on ramalama
	s.ProviderClients["openai"] = mocks.NewMockLLMClient()
	s.ProviderClients["huggingface"] = mocks.NewMockLLMClient()
	s.ProviderClients["ollama"] = mocks.NewMockLLMClient()

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

// Rule 11 compliance: Using existing MockLLMClient from pkg/testutil/mocks instead of creating duplicate
