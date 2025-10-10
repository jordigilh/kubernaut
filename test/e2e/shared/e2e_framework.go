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
package shared

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// E2ETestFramework provides comprehensive E2E testing capabilities for business workflows
// Business Requirements: BR-E2E-CUSTOMER-SERVICE-001 - Complete customer service workflow testing
// Following project guidelines: reuse existing patterns from ChaosTestFactory
type E2ETestFramework struct {
	WorkflowEngine engine.WorkflowEngine
	K8sClient      k8s.Client
	LLMClient      llm.Client
	VectorDB       vector.VectorDatabase
	Logger         *logrus.Logger
	TestTimeout    time.Duration

	// Configuration
	config *config.Config
}

// NewE2ETestFramework creates a new E2E testing framework
// TDD RED: This will fail until real business components are properly integrated
func NewE2ETestFramework(ctx context.Context, logger *logrus.Logger) (*E2ETestFramework, error) {
	// TDD RED: Framework initialization that drives real component integration

	// TDD GREEN: Configure logging levels for E2E tests per testing strategy
	// E2E tests require Info level to capture model responses and business logic
	if logger.Level < logrus.InfoLevel {
		logger.SetLevel(logrus.InfoLevel)
		logger.Info("🔧 E2E: Set logging level to Info to capture model responses")
	}

	framework := &E2ETestFramework{
		Logger:      logger,
		TestTimeout: 10 * time.Minute,
	}

	// TDD GREEN: Use minimal configuration for E2E testing
	cfg := &config.Config{
		Kubernetes: config.KubernetesConfig{
			Namespace:     "default",
			ClientType:    "real",
			UseFakeClient: false,
		},
		AIServices: config.AIServicesConfig{
			HolmesGPT: config.HolmesGPTConfig{
				Enabled:    true,
				Mode:       "development",
				Endpoint:   "http://localhost:3000", // HolmesGPT service endpoint
				Timeout:    90 * time.Second,        // Extended timeout for E2E investigations
				RetryCount: 2,
				Toolsets:   []string{"kubernetes", "prometheus"},
				Priority:   100, // Higher priority - use HolmesGPT first
			},
			LLM: config.LLMConfig{
				Provider:    "ramalama",
				Endpoint:    "http://localhost:8010", // Fallback LLM endpoint via SSH tunnel
				Model:       "ggml-org/gpt-oss-20b-GGUF",
				Temperature: 0.1,
				MaxTokens:   8192,
				Timeout:     60 * time.Second, // Generous timeout for costly inference - E2E functionality over performance
			},
		},
		VectorDB: config.VectorDBConfig{
			Enabled: false, // Minimal E2E setup
			Backend: "memory",
			EmbeddingService: config.EmbeddingConfig{
				Service:   "local",
				Dimension: 384,
				Model:     "all-MiniLM-L6-v2",
			},
		},
	}
	framework.config = cfg

	// TDD GREEN: Initialize real business components
	if err := framework.initializeComponents(ctx); err != nil {
		// TDD GREEN: Return error if critical components fail
		return nil, fmt.Errorf("TDD GREEN: failed to initialize E2E components: %w", err)
	}

	logger.Info("✅ E2E Test Framework initialized successfully")
	return framework, nil
}

// TDD REFACTOR: Extract common logging configuration for E2E tests
// Following @03-testing-strategy.mdc - reduce duplication across test files
func NewE2ELogger() *logrus.Logger {
	logger := logrus.New()

	// E2E tests require Info level to capture:
	// - Model responses and AI inference details
	// - Business logic execution flow
	// - Integration validation steps
	logger.SetLevel(logrus.InfoLevel)

	// Enhanced formatting for E2E test visibility
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		ForceColors:   true,
	})

	logger.Info("🔧 E2E: Logger configured for model response capture")
	return logger
}

// TDD REFACTOR: Extract common logging configuration for unit/integration tests
// Following @03-testing-strategy.mdc - standardize logging across test types
func NewTestLogger() *logrus.Logger {
	logger := logrus.New()

	// Unit/Integration tests default to Error level to reduce noise
	// Only show critical issues unless explicitly configured otherwise
	logger.SetLevel(logrus.ErrorLevel)

	// Standard formatting for test output
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: false,
		ForceColors:   true,
	})

	return logger
}

// initializeComponents initializes all real business components for E2E testing
// TDD GREEN: Minimal implementation to make tests pass with real OCP cluster
func (f *E2ETestFramework) initializeComponents(ctx context.Context) error {
	// TDD GREEN: Initialize Kubernetes client with real OCP cluster
	if err := f.initializeK8sClient(); err != nil {
		// TDD GREEN: Log error but continue - framework can still function
		f.Logger.WithError(err).Warn("TDD GREEN: K8s client not available, E2E test may be limited")
		return fmt.Errorf("TDD GREEN: K8s client required for E2E tests: %w", err)
	}

	// E2E REQUIREMENT: LLM client MUST be available for real E2E testing
	if err := f.initializeLLMClient(); err != nil {
		f.Logger.WithError(err).Error("E2E FAILURE: Real LLM client required for E2E tests")
		return fmt.Errorf("E2E FAILURE: LLM client initialization required for E2E tests: %w", err)
	}

	// TDD GREEN: Initialize minimal Vector Database (can fail gracefully)
	if err := f.initializeVectorDB(); err != nil {
		f.Logger.WithError(err).Warn("TDD GREEN: VectorDB not available, using minimal implementation")
		f.VectorDB = nil // Will be handled gracefully in workflow engine
	}

	// TDD GREEN: Initialize minimal Workflow Engine with available components
	if err := f.initializeWorkflowEngine(); err != nil {
		f.Logger.WithError(err).Warn("TDD GREEN: Failed to initialize workflow engine")
		return fmt.Errorf("TDD GREEN: workflow engine initialization required for E2E tests: %w", err)
	}

	return nil
}

// GetWorkflowEngine returns the initialized workflow engine
// TDD RED: This will return nil until real workflow engine is integrated
func (f *E2ETestFramework) GetWorkflowEngine() engine.WorkflowEngine {
	// TDD RED: Will be nil until GREEN phase implementation
	return f.WorkflowEngine
}

// TDD RED: Component initialization methods that will drive implementation

func (f *E2ETestFramework) initializeK8sClient() error {
	// TDD GREEN: Create real K8s client for OCP cluster
	k8sConfig := config.KubernetesConfig{
		Namespace:     "default",
		ClientType:    "real", // Force real client for E2E testing
		UseFakeClient: false,  // Ensure we use real cluster
	}

	client, err := k8s.NewClient(k8sConfig, f.Logger)
	if err != nil {
		return fmt.Errorf("TDD GREEN: failed to create real K8s client: %w", err)
	}
	f.K8sClient = client
	f.Logger.Info("✅ TDD GREEN: Real Kubernetes client initialized for OCP cluster")
	return nil
}

func (f *E2ETestFramework) initializeLLMClient() error {
	// 🚨 E2E CRITICAL: Disable ALL fallback mechanisms via environment variable
	_ = os.Setenv("LLM_ENABLE_RULE_FALLBACK", "false")
	f.Logger.Info("🚨 E2E: Disabled LLM rule fallback mechanisms - real model required")

	// TDD RED: This will fail until real LLM client is available for E2E testing
	client, err := llm.NewClient(f.config.AIServices.LLM, f.Logger)
	if err != nil {
		return fmt.Errorf("E2E FAILURE: failed to create LLM client: %w", err)
	}

	// E2E REQUIREMENT: Validate LLM is actually reachable and working
	// Generous timeout for costly inference operations - functionality over performance
	// Max observed: 24s for complex scenarios + buffer for inference variability
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	// Test AI connectivity (HolmesGPT primary, LLM fallback) with a simple request
	testResponse, err := client.AnalyzeAlert(ctx, types.Alert{
		Name:        "E2E_AI_CONNECTIVITY_TEST",
		Severity:    "info",
		Description: "E2E test AI connectivity validation - HolmesGPT primary, LLM fallback",
		Labels:      map[string]string{"test": "e2e_ai_connectivity"},
	})
	if err != nil {
		return fmt.Errorf("E2E FAILURE: AI services not reachable (HolmesGPT: %s, LLM: %s): %w",
			f.config.AIServices.HolmesGPT.Endpoint, f.config.AIServices.LLM.Endpoint, err)
	}

	// Validate we got a real response (not fallback)
	if testResponse == nil || testResponse.Action == "" {
		return fmt.Errorf("E2E FAILURE: LLM returned empty response, may be using fallback mechanisms")
	}

	// E2E LOGGING: Capture and display the model's actual response
	f.Logger.WithFields(logrus.Fields{
		"action":     testResponse.Action,
		"confidence": testResponse.Confidence,
		"reasoning":  testResponse.Reasoning,
	}).Info("🤖 E2E: LLM Response Captured")

	// Print detailed response for test visibility
	fmt.Printf("\n=== LLM MODEL RESPONSE ===\n")
	fmt.Printf("Action: %s\n", testResponse.Action)
	fmt.Printf("Confidence: %.3f\n", testResponse.Confidence)
	if testResponse.Reasoning != nil {
		fmt.Printf("Reasoning Summary: %s\n", testResponse.Reasoning.Summary)
		if testResponse.Reasoning.PrimaryReason != "" {
			fmt.Printf("Primary Reason: %s\n", testResponse.Reasoning.PrimaryReason)
		}
		if len(testResponse.Reasoning.AlternativeActions) > 0 {
			fmt.Printf("Alternative Actions: %v\n", testResponse.Reasoning.AlternativeActions)
		}
	}
	if len(testResponse.Parameters) > 0 {
		fmt.Printf("Parameters: %v\n", testResponse.Parameters)
	}
	fmt.Printf("========================\n\n")

	// 🚨 E2E CRITICAL: Strict validation to prevent ANY fallback mechanisms
	if testResponse.Confidence < 0.7 {
		return fmt.Errorf("E2E FAILURE: LLM confidence too low (%.2f), likely using rule-based fallback instead of real model", testResponse.Confidence)
	}

	// 🚨 E2E CRITICAL: Validate reasoning structure indicates real LLM processing
	if testResponse.Reasoning == nil {
		return fmt.Errorf("E2E FAILURE: Missing reasoning structure, likely using fallback mechanisms")
	}

	// 🚨 E2E CRITICAL: Validate we have detailed reasoning from real LLM
	if testResponse.Reasoning.PrimaryReason == "" || len(testResponse.Reasoning.PrimaryReason) < 20 {
		return fmt.Errorf("E2E FAILURE: Missing or insufficient primary reasoning (got %d chars), likely using fallback mechanisms", len(testResponse.Reasoning.PrimaryReason))
	}

	// 🚨 E2E CRITICAL: Validate reasoning contains LLM-style analysis (not rule-based patterns)
	primaryReason := strings.ToLower(testResponse.Reasoning.PrimaryReason)
	llmPatterns := []string{
		"likely", "indicates", "suggests", "experiencing", "causing", "may", "can",
		"will", "should", "appears", "seems", "potential", "possible", "probable",
	}

	hasLLMPattern := false
	for _, pattern := range llmPatterns {
		if strings.Contains(primaryReason, pattern) {
			hasLLMPattern = true
			break
		}
	}

	if !hasLLMPattern {
		return fmt.Errorf("E2E FAILURE: Primary reasoning lacks LLM-style analysis patterns (got: '%s'), likely rule-based fallback", testResponse.Reasoning.PrimaryReason)
	}

	// 🚨 E2E CRITICAL: Validate we have alternative actions (LLM provides options)
	if len(testResponse.Reasoning.AlternativeActions) == 0 {
		return fmt.Errorf("E2E FAILURE: No alternative actions provided, likely using fallback mechanisms")
	}

	f.Logger.WithFields(logrus.Fields{
		"action":            testResponse.Action,
		"confidence":        testResponse.Confidence,
		"holmesgpt_enabled": f.config.IsHolmesGPTEnabled(),
		"ai_integration":    "holmesgpt_primary_llm_fallback",
	}).Info("✅ E2E: Real AI connectivity validated - HolmesGPT primary, LLM fallback")
	f.LLMClient = client
	return nil
}

func (f *E2ETestFramework) initializeVectorDB() error {
	// TDD GREEN: Skip VectorDB for minimal E2E implementation
	if !f.config.VectorDB.Enabled {
		f.Logger.Info("TDD GREEN: VectorDB disabled, skipping initialization")
		f.VectorDB = nil
		return nil
	}

	// TDD GREEN: VectorDB integration will be implemented in REFACTOR phase
	f.Logger.Warn("TDD GREEN: VectorDB initialization not yet implemented")
	f.VectorDB = nil
	return nil
}

func (f *E2ETestFramework) initializeWorkflowEngine() error {
	// 🚨 E2E CRITICAL: Use AI-integrated workflow engine for HolmesGPT + LLM fallback
	// Rule 09 validated: Use real implementations to fix nil pointer dereference

	// Create in-memory execution repository (validated interface implementation)
	executionRepo := engine.NewInMemoryExecutionRepository(f.Logger)

	// Create in-memory state storage (validated interface implementation)
	stateStorage := engine.NewWorkflowStateStorage(nil, f.Logger) // nil DB = memory-only mode

	// 🚨 E2E CRITICAL: Use AI-integrated workflow engine with HolmesGPT + LLM fallback
	workflowEngine, err := engine.NewDefaultWorkflowEngineWithAIIntegration(
		f.K8sClient,
		nil,           // actionRepo - minimal implementation allows nil
		nil,           // monitoringClients - minimal implementation allows nil
		stateStorage,  // stateStorage - now properly initialized
		executionRepo, // executionRepo - now properly initialized
		nil,           // config - will use defaults
		f.config,      // aiConfig - provides HolmesGPT + LLM configuration
		f.Logger,
	)
	if err != nil {
		return fmt.Errorf("E2E FAILURE: failed to create AI-integrated workflow engine: %w", err)
	}

	// E2E tests use real action executors - no mocks allowed
	// Workflow engine automatically registers real action executors:
	// - KubernetesActionExecutor for k8s operations (scale_up, restart_pod, etc.)
	// - MonitoringActionExecutor for alerts and monitoring
	// - CustomActionExecutor for generic operations
	// All executors run in dry-run mode for E2E safety

	f.WorkflowEngine = workflowEngine
	f.Logger.WithFields(logrus.Fields{
		"holmesgpt_enabled":  f.config.IsHolmesGPTEnabled(),
		"llm_enabled":        f.config.GetLLMConfig().Endpoint != "",
		"holmesgpt_endpoint": f.config.GetHolmesGPTConfig().Endpoint,
		"llm_endpoint":       f.config.GetLLMConfig().Endpoint,
	}).Info("✅ E2E: AI-integrated WorkflowEngine initialized - HolmesGPT primary, LLM fallback")
	return nil
}

// Cleanup cleans up E2E test resources
func (f *E2ETestFramework) Cleanup() {
	if f.Logger != nil {
		f.Logger.Info("🧹 Cleaning up E2E test framework resources")
	}

	// Cleanup components if needed
	// TDD: Implementation will be added during GREEN phase
}
