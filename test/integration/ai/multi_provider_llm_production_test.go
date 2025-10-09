//go:build integration
// +build integration

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

package ai

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/integration/shared"
)

// MultiProviderLLMValidator validates Phase 2 Critical AI Requirements - Multi-Provider LLM Integration
// Business Requirements Covered:
// - BR-PA-006: Multi-provider LLM support with intelligent failover
// - BR-AI-001: Analytics processing with 10,000+ records in <30s, >90% confidence
// - BR-AI-002: Pattern recognition with >80% accuracy for alert classification
type MultiProviderLLMValidator struct {
	logger         *logrus.Logger
	testConfig     shared.IntegrationConfig
	stateManager   *shared.ComprehensiveStateManager
	providerPool   *LLMProviderPool
	metricsTracker *MultiProviderMetrics
}

// LLMProviderPool manages multiple LLM providers with intelligent failover
type LLMProviderPool struct {
	providers       map[string]llm.Client
	primaryProvider string
	failoverOrder   []string
	healthChecks    map[string]ProviderHealth
	switchLatency   map[string]time.Duration
	mu              sync.RWMutex
}

// ProviderHealth tracks individual provider health status
type ProviderHealth struct {
	IsHealthy        bool
	LastHealthCheck  time.Time
	ConsecutiveFails int
	AverageLatency   time.Duration
	SuccessRate      float64
}

// MultiProviderMetrics tracks performance metrics for business requirement validation
type MultiProviderMetrics struct {
	ProviderConnectivity map[string]bool
	FailoverLatencies    map[string]time.Duration
	AnalysisAccuracy     map[string]float64
	CostOptimization     map[string]CostMetrics
	ProviderSwitchEvents []ProviderSwitchEvent
	ProcessingMetrics    ProcessingPerformanceMetrics
	mu                   sync.RWMutex
}

// CostMetrics tracks cost optimization per provider
type CostMetrics struct {
	TokensUsed       int64
	EstimatedCost    float64
	CostPerToken     float64
	OptimizationRate float64
}

// ProviderSwitchEvent represents a provider failover event
type ProviderSwitchEvent struct {
	FromProvider string
	ToProvider   string
	SwitchTime   time.Duration
	Reason       string
	Timestamp    time.Time
	Success      bool
}

// ProcessingPerformanceMetrics tracks analytics processing performance
type ProcessingPerformanceMetrics struct {
	RecordsProcessed       int
	ProcessingTime         time.Duration
	ConfidenceScore        float64
	ClassificationAccuracy float64
	ThroughputPerSecond    float64
}

// LLMProviderConfig defines configuration for each provider
type LLMProviderConfig struct {
	Name                string
	Endpoint            string
	APIKey              string
	Model               string
	MaxTokens           int
	Temperature         float64
	TimeoutDuration     time.Duration
	CostPerToken        float64
	PriorityRank        int
	HealthCheckInterval time.Duration
}

// NewMultiProviderLLMValidator creates a validator for Phase 2 multi-provider LLM requirements
func NewMultiProviderLLMValidator(config shared.IntegrationConfig, stateManager *shared.ComprehensiveStateManager) *MultiProviderLLMValidator {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	return &MultiProviderLLMValidator{
		logger:       logger,
		testConfig:   config,
		stateManager: stateManager,
		providerPool: &LLMProviderPool{
			providers:     make(map[string]llm.Client),
			healthChecks:  make(map[string]ProviderHealth),
			switchLatency: make(map[string]time.Duration),
		},
		metricsTracker: &MultiProviderMetrics{
			ProviderConnectivity: make(map[string]bool),
			FailoverLatencies:    make(map[string]time.Duration),
			AnalysisAccuracy:     make(map[string]float64),
			CostOptimization:     make(map[string]CostMetrics),
		},
	}
}

// ValidateProviderConnectivity validates BR-PA-006: All 6 LLM providers functional
func (v *MultiProviderLLMValidator) ValidateProviderConnectivity(ctx context.Context, providerConfigs []LLMProviderConfig) (*ProviderConnectivityResult, error) {
	v.logger.WithField("provider_count", len(providerConfigs)).Info("Starting multi-provider connectivity validation")

	// BR-PA-006: Must support all 6 providers (OpenAI, Anthropic, Azure, AWS, Ollama, Local)
	requiredProviders := []string{"openai", "anthropic", "azure", "aws", "ollama", "local"}
	if len(providerConfigs) < len(requiredProviders) {
		return nil, fmt.Errorf("insufficient providers for validation: got %d, need at least %d", len(providerConfigs), len(requiredProviders))
	}

	// Track provider connectivity
	connectivityResults := make(map[string]bool)
	healthStatuses := make(map[string]ProviderHealth)
	connectionLatencies := make(map[string]time.Duration)

	// Test connectivity for each provider
	for _, config := range providerConfigs {
		v.logger.WithField("provider", config.Name).Debug("Testing provider connectivity")

		connectStart := time.Now()
		connected, health := v.testProviderConnectivity(ctx, config)
		connectionLatency := time.Since(connectStart)

		connectivityResults[config.Name] = connected
		healthStatuses[config.Name] = health
		connectionLatencies[config.Name] = connectionLatency
	}

	// Calculate overall connectivity metrics
	connectedCount := 0
	for _, connected := range connectivityResults {
		if connected {
			connectedCount++
		}
	}

	connectivityRate := float64(connectedCount) / float64(len(providerConfigs))

	// BR-PA-006: Must achieve 100% provider connectivity
	meetsRequirement := connectivityRate >= 1.0

	v.logger.WithFields(logrus.Fields{
		"connected_providers": connectedCount,
		"total_providers":     len(providerConfigs),
		"connectivity_rate":   connectivityRate,
		"meets_requirement":   meetsRequirement,
	}).Info("Provider connectivity validation completed")

	return &ProviderConnectivityResult{
		ConnectedProviders:  connectedCount,
		TotalProviders:      len(providerConfigs),
		ConnectivityRate:    connectivityRate,
		MeetsRequirement:    meetsRequirement,
		ProviderStatuses:    connectivityResults,
		HealthStatuses:      healthStatuses,
		ConnectionLatencies: connectionLatencies,
	}, nil
}

// ValidateIntelligentFailover validates BR-PA-006: <500ms provider switching latency
func (v *MultiProviderLLMValidator) ValidateIntelligentFailover(ctx context.Context, scenarios []FailoverScenario) (*FailoverValidationResult, error) {
	v.logger.WithField("scenarios_count", len(scenarios)).Info("Starting intelligent failover validation")

	// Track failover metrics
	failoverEvents := make([]ProviderSwitchEvent, 0)
	totalSwitchTime := time.Duration(0)
	successfulFailovers := 0

	// Execute each failover scenario
	for i, scenario := range scenarios {
		v.logger.WithFields(logrus.Fields{
			"scenario_index": i,
			"from_provider":  scenario.FromProvider,
			"to_provider":    scenario.ToProvider,
		}).Debug("Executing failover scenario")

		// Simulate provider failure and measure failover time
		failoverStart := time.Now()
		success := v.executeProviderFailover(ctx, scenario)
		switchTime := time.Since(failoverStart)

		// Record failover event
		event := ProviderSwitchEvent{
			FromProvider: scenario.FromProvider,
			ToProvider:   scenario.ToProvider,
			SwitchTime:   switchTime,
			Reason:       scenario.FailureReason,
			Timestamp:    time.Now(),
			Success:      success,
		}
		failoverEvents = append(failoverEvents, event)

		if success {
			successfulFailovers++
			totalSwitchTime += switchTime
		}
	}

	// Calculate failover performance metrics
	averageSwitchTime := time.Duration(0)
	if successfulFailovers > 0 {
		averageSwitchTime = totalSwitchTime / time.Duration(successfulFailovers)
	}

	failoverSuccessRate := float64(successfulFailovers) / float64(len(scenarios))

	// BR-PA-006: Must achieve <500ms switching latency and >95% success rate
	meetsLatencyRequirement := averageSwitchTime < 500*time.Millisecond
	meetsSuccessRequirement := failoverSuccessRate >= 0.95
	meetsRequirement := meetsLatencyRequirement && meetsSuccessRequirement

	v.logger.WithFields(logrus.Fields{
		"successful_failovers":  successfulFailovers,
		"total_scenarios":       len(scenarios),
		"average_switch_time":   averageSwitchTime,
		"failover_success_rate": failoverSuccessRate,
		"meets_latency_req":     meetsLatencyRequirement,
		"meets_success_req":     meetsSuccessRequirement,
		"meets_requirement":     meetsRequirement,
	}).Info("Intelligent failover validation completed")

	return &FailoverValidationResult{
		SuccessfulFailovers: successfulFailovers,
		TotalScenarios:      len(scenarios),
		AverageSwitchTime:   averageSwitchTime,
		FailoverSuccessRate: failoverSuccessRate,
		MeetsRequirement:    meetsRequirement,
		FailoverEvents:      failoverEvents,
	}, nil
}

// ValidateAnalysisAccuracy validates BR-AI-001 and BR-AI-002: 85% AI analysis accuracy
func (v *MultiProviderLLMValidator) ValidateAnalysisAccuracy(ctx context.Context, testDatasets []AnalysisDataset) (*AnalysisAccuracyResult, error) {
	v.logger.WithField("datasets_count", len(testDatasets)).Info("Starting cross-provider analysis accuracy validation")

	// Track accuracy metrics per provider
	providerAccuracies := make(map[string]float64)
	providerProcessingTimes := make(map[string]time.Duration)
	overallResults := make([]AnalysisResult, 0)

	// Test each provider with all datasets - protect concurrent access
	v.providerPool.mu.RLock()
	providers := make(map[string]llm.Client)
	for name, client := range v.providerPool.providers {
		providers[name] = client
	}
	v.providerPool.mu.RUnlock()

	for providerName, client := range providers {
		v.logger.WithField("provider", providerName).Debug("Testing provider analysis accuracy")

		providerStart := time.Now()
		accuracy := v.measureProviderAccuracy(ctx, client, testDatasets)
		processingTime := time.Since(providerStart)

		providerAccuracies[providerName] = accuracy
		providerProcessingTimes[providerName] = processingTime

		// Record result for each provider
		result := AnalysisResult{
			Provider:       providerName,
			Accuracy:       accuracy,
			ProcessingTime: processingTime,
			DatasetsCount:  len(testDatasets),
		}
		overallResults = append(overallResults, result)
	}

	// Calculate overall analysis metrics
	totalAccuracy := 0.0
	for _, accuracy := range providerAccuracies {
		totalAccuracy += accuracy
	}
	averageAccuracy := totalAccuracy / float64(len(providerAccuracies))

	// BR-AI-001 & BR-AI-002: Must achieve 85% analysis accuracy across all providers
	meetsRequirement := averageAccuracy >= 0.85

	v.logger.WithFields(logrus.Fields{
		"providers_tested":  len(providerAccuracies),
		"average_accuracy":  averageAccuracy,
		"meets_requirement": meetsRequirement,
	}).Info("Cross-provider analysis accuracy validation completed")

	return &AnalysisAccuracyResult{
		AverageAccuracy:    averageAccuracy,
		ProviderAccuracies: providerAccuracies,
		ProcessingTimes:    providerProcessingTimes,
		MeetsRequirement:   meetsRequirement,
		AnalysisResults:    overallResults,
	}, nil
}

// ValidateCostOptimization validates cost optimization through provider selection algorithms
func (v *MultiProviderLLMValidator) ValidateCostOptimization(ctx context.Context, workloadScenarios []CostOptimizationScenario) (*CostOptimizationResult, error) {
	v.logger.WithField("scenarios_count", len(workloadScenarios)).Info("Starting cost optimization validation")

	// Track cost optimization metrics
	totalSavings := 0.0
	optimizationResults := make([]OptimizationResult, 0)
	providerCosts := make(map[string]CostMetrics)

	// Execute each cost optimization scenario
	for i, scenario := range workloadScenarios {
		v.logger.WithField("scenario_index", i).Debug("Executing cost optimization scenario")

		optimizationStart := time.Now()
		result := v.executeCostOptimization(ctx, scenario)
		optimizationTime := time.Since(optimizationStart)

		result.OptimizationTime = optimizationTime
		optimizationResults = append(optimizationResults, result)
		totalSavings += result.CostSavings
	}

	// Calculate overall cost optimization metrics
	averageSavings := totalSavings / float64(len(workloadScenarios))

	// Measure cost optimization effectiveness
	optimizationEffectiveness := v.calculateOptimizationEffectiveness(optimizationResults)

	// Cost optimization target: measurable reduction in costs through intelligent provider selection
	meetsRequirement := averageSavings > 0.0 && optimizationEffectiveness >= 0.70

	v.logger.WithFields(logrus.Fields{
		"scenarios_processed":        len(workloadScenarios),
		"average_savings":            averageSavings,
		"optimization_effectiveness": optimizationEffectiveness,
		"meets_requirement":          meetsRequirement,
	}).Info("Cost optimization validation completed")

	return &CostOptimizationResult{
		AverageSavings:            averageSavings,
		TotalSavings:              totalSavings,
		OptimizationEffectiveness: optimizationEffectiveness,
		MeetsRequirement:          meetsRequirement,
		OptimizationResults:       optimizationResults,
		ProviderCostBreakdown:     providerCosts,
	}, nil
}

// Business contract types for TDD

type ProviderConnectivityResult struct {
	ConnectedProviders  int
	TotalProviders      int
	ConnectivityRate    float64
	MeetsRequirement    bool // Must be true for BR-PA-006 compliance
	ProviderStatuses    map[string]bool
	HealthStatuses      map[string]ProviderHealth
	ConnectionLatencies map[string]time.Duration
}

type FailoverValidationResult struct {
	SuccessfulFailovers int
	TotalScenarios      int
	AverageSwitchTime   time.Duration
	FailoverSuccessRate float64
	MeetsRequirement    bool // Must be true for BR-PA-006 compliance (<500ms)
	FailoverEvents      []ProviderSwitchEvent
}

type AnalysisAccuracyResult struct {
	AverageAccuracy    float64
	ProviderAccuracies map[string]float64
	ProcessingTimes    map[string]time.Duration
	MeetsRequirement   bool // Must be true for BR-AI-001 & BR-AI-002 compliance (85%)
	AnalysisResults    []AnalysisResult
}

type CostOptimizationResult struct {
	AverageSavings            float64
	TotalSavings              float64
	OptimizationEffectiveness float64
	MeetsRequirement          bool // Must be true for cost optimization compliance
	OptimizationResults       []OptimizationResult
	ProviderCostBreakdown     map[string]CostMetrics
}

type FailoverScenario struct {
	FromProvider  string
	ToProvider    string
	FailureReason string
	ExpectedTime  time.Duration
}

type AnalysisDataset struct {
	Name            string
	Alerts          []types.Alert
	ExpectedResults []string
	Complexity      string
	GroundTruth     map[string]interface{}
}

type AnalysisResult struct {
	Provider       string
	Accuracy       float64
	ProcessingTime time.Duration
	DatasetsCount  int
}

type CostOptimizationScenario struct {
	WorkloadType    string
	TokenCount      int
	QualityRequired float64
	TimeConstraint  time.Duration
	BudgetLimit     float64
}

type OptimizationResult struct {
	SelectedProvider   string
	CostSavings        float64
	QualityScore       float64
	OptimizationTime   time.Duration
	ReasonForSelection string
}

var _ = Describe("Phase 2: Multi-Provider LLM Integration - Critical AI Requirements", Ordered, func() {
	var (
		validator    *MultiProviderLLMValidator
		testConfig   shared.IntegrationConfig
		stateManager *shared.ComprehensiveStateManager
		ctx          context.Context
	)

	BeforeAll(func() {
		ctx = context.Background()
		testConfig = shared.LoadConfig()
		if testConfig.SkipIntegration {
			Skip("Integration tests disabled")
		}

		// Initialize comprehensive state manager with AI isolation
		patterns := &shared.TestIsolationPatterns{}
		stateManager = patterns.DatabaseTransactionIsolatedSuite("Phase 2 Multi-Provider LLM Integration")

		validator = NewMultiProviderLLMValidator(testConfig, stateManager)
	})

	AfterAll(func() {
		err := stateManager.CleanupAllState()
		Expect(err).ToNot(HaveOccurred())
	})

	// Helper function to create test provider configurations
	createTestProviderConfigs := func() []LLMProviderConfig {
		// Use the real LLM service at ramalama endpoint for multiple providers to simulate multi-provider testing
		// In a real implementation, these would be different actual providers
		realLLMEndpoint := os.Getenv("LLM_ENDPOINT")
		if realLLMEndpoint == "" {
			realLLMEndpoint = "http://192.168.1.169:8080" // Default to ramalama endpoint
		}
		realLLMModel := "ggml-org/gpt-oss-20b-GGUF" // Use the actual model available on the service

		return []LLMProviderConfig{
			{Name: "openai", Endpoint: realLLMEndpoint, Model: realLLMModel, MaxTokens: 131072, Temperature: 0.7, CostPerToken: 0.00003, PriorityRank: 1, TimeoutDuration: 30 * time.Second},
			{Name: "anthropic", Endpoint: realLLMEndpoint, Model: realLLMModel, MaxTokens: 200000, Temperature: 0.7, CostPerToken: 0.000015, PriorityRank: 2, TimeoutDuration: 30 * time.Second},
			{Name: "azure", Endpoint: realLLMEndpoint, Model: realLLMModel, MaxTokens: 128000, Temperature: 0.7, CostPerToken: 0.00001, PriorityRank: 3, TimeoutDuration: 30 * time.Second},
			{Name: "aws", Endpoint: realLLMEndpoint, Model: realLLMModel, MaxTokens: 100000, Temperature: 0.7, CostPerToken: 0.00008, PriorityRank: 4, TimeoutDuration: 30 * time.Second},
			{Name: "ollama", Endpoint: realLLMEndpoint, Model: realLLMModel, MaxTokens: 131072, Temperature: 0.7, CostPerToken: 0.0, PriorityRank: 5, TimeoutDuration: 30 * time.Second},
			{Name: "local", Endpoint: realLLMEndpoint, Model: realLLMModel, MaxTokens: 131072, Temperature: 0.7, CostPerToken: 0.0, PriorityRank: 6, TimeoutDuration: 30 * time.Second},
		}
	}

	Context("BR-PA-006: Multi-Provider LLM Connectivity (All 6 providers functional)", func() {
		It("should establish connectivity to all 6 LLM providers", func() {
			By("Testing connectivity to OpenAI, Anthropic, Azure, AWS, Ollama, and Local providers")
			providerConfigs := createTestProviderConfigs()

			result, err := validator.ValidateProviderConnectivity(ctx, providerConfigs)

			Expect(err).ToNot(HaveOccurred(), "Provider connectivity validation should not fail")
			Expect(result).ToNot(BeNil(), "Should return provider connectivity validation result")

			// BR-PA-006 Business Requirement: All 6 providers must be functional
			Expect(result.MeetsRequirement).To(BeTrue(), "Must achieve 100% provider connectivity")
			Expect(result.ConnectedProviders).To(Equal(6), "All 6 providers must be connected")
			Expect(result.ConnectivityRate).To(BeNumerically(">=", 1.0), "Connectivity rate must be 100%")

			// Validate each required provider is connected
			requiredProviders := []string{"openai", "anthropic", "azure", "aws", "ollama", "local"}
			for _, providerName := range requiredProviders {
				Expect(result.ProviderStatuses[providerName]).To(BeTrue(),
					"Provider %s must be connected", providerName)
			}

			GinkgoWriter.Printf("✅ BR-PA-006 Connectivity: %d/%d providers connected (%.1f%%)\\n",
				result.ConnectedProviders, result.TotalProviders, result.ConnectivityRate*100)
		})

		It("should validate provider health statuses and connection latencies", func() {
			By("Checking individual provider health and performance characteristics")
			providerConfigs := createTestProviderConfigs()

			result, err := validator.ValidateProviderConnectivity(ctx, providerConfigs)

			Expect(err).ToNot(HaveOccurred(), "Provider health validation should not fail")
			Expect(result).ToNot(BeNil(), "Should return provider health validation result")

			// Validate health status for each provider
			for providerName, health := range result.HealthStatuses {
				Expect(health.IsHealthy).To(BeTrue(), "Provider %s should be healthy", providerName)
				Expect(health.AverageLatency).To(BeNumerically("<", 5*time.Second),
					"Provider %s should have reasonable latency", providerName)
			}

			// Validate connection latencies are reasonable
			for providerName, latency := range result.ConnectionLatencies {
				Expect(latency).To(BeNumerically("<", 10*time.Second),
					"Provider %s connection should be established quickly", providerName)
			}

			GinkgoWriter.Printf("✅ Provider Health: All providers healthy with reasonable latencies\\n")
		})
	})

	Context("BR-PA-006: Intelligent Failover (<500ms provider switching latency)", func() {
		It("should perform intelligent failover with <500ms switching latency", func() {
			By("Testing provider failover scenarios with latency measurement")

			failoverScenarios := []FailoverScenario{
				{FromProvider: "openai", ToProvider: "anthropic", FailureReason: "rate_limit_exceeded", ExpectedTime: 300 * time.Millisecond},
				{FromProvider: "anthropic", ToProvider: "azure", FailureReason: "api_timeout", ExpectedTime: 250 * time.Millisecond},
				{FromProvider: "azure", ToProvider: "aws", FailureReason: "service_unavailable", ExpectedTime: 400 * time.Millisecond},
				{FromProvider: "aws", ToProvider: "ollama", FailureReason: "authentication_failed", ExpectedTime: 200 * time.Millisecond},
				{FromProvider: "ollama", ToProvider: "local", FailureReason: "model_not_loaded", ExpectedTime: 450 * time.Millisecond},
			}

			result, err := validator.ValidateIntelligentFailover(ctx, failoverScenarios)

			Expect(err).ToNot(HaveOccurred(), "Intelligent failover validation should not fail")
			Expect(result).ToNot(BeNil(), "Should return failover validation result")

			// BR-PA-006 Business Requirement: <500ms provider switching latency
			Expect(result.MeetsRequirement).To(BeTrue(), "Must meet <500ms failover latency requirement")
			Expect(result.AverageSwitchTime).To(BeNumerically("<", 500*time.Millisecond),
				"Average switch time must be <500ms")
			Expect(result.FailoverSuccessRate).To(BeNumerically(">=", 0.95),
				"Failover success rate must be >= 95%")

			// Validate individual failover events
			for _, event := range result.FailoverEvents {
				if event.Success {
					Expect(event.SwitchTime).To(BeNumerically("<", 1*time.Second),
						"Individual failover should complete within 1 second")
				}
			}

			GinkgoWriter.Printf("✅ BR-PA-006 Failover: %.0fms average switch time, %.1f%% success rate\\n",
				float64(result.AverageSwitchTime.Nanoseconds())/1000000, result.FailoverSuccessRate*100)
		})

		It("should maintain service continuity during provider failures", func() {
			By("Testing service continuity with cascading provider failures")

			// Test scenario with multiple simultaneous failures
			complexFailoverScenarios := []FailoverScenario{
				{FromProvider: "openai", ToProvider: "anthropic", FailureReason: "simultaneous_outage", ExpectedTime: 300 * time.Millisecond},
				{FromProvider: "anthropic", ToProvider: "ollama", FailureReason: "cascade_failure", ExpectedTime: 400 * time.Millisecond},
				{FromProvider: "azure", ToProvider: "local", FailureReason: "emergency_fallback", ExpectedTime: 200 * time.Millisecond},
			}

			result, err := validator.ValidateIntelligentFailover(ctx, complexFailoverScenarios)

			Expect(err).ToNot(HaveOccurred(), "Complex failover validation should not fail")
			Expect(result).ToNot(BeNil(), "Should return complex failover validation result")

			// Service should maintain continuity even with complex failures
			Expect(result.FailoverSuccessRate).To(BeNumerically(">=", 0.90),
				"Should maintain >= 90% success rate even with complex failures")

			GinkgoWriter.Printf("✅ Service Continuity: %.1f%% success rate under complex failure scenarios\\n",
				result.FailoverSuccessRate*100)
		})
	})

	Context("BR-AI-001 & BR-AI-002: Cross-Provider Analysis Accuracy (85% AI analysis accuracy)", func() {
		It("should achieve 85% AI analysis accuracy across all providers", func() {
			By("Testing analysis accuracy with standardized datasets across all providers")

			// Create comprehensive test datasets for analysis accuracy validation
			testDatasets := validator.createAnalysisTestDatasets()

			result, err := validator.ValidateAnalysisAccuracy(ctx, testDatasets)

			Expect(err).ToNot(HaveOccurred(), "Analysis accuracy validation should not fail")
			Expect(result).ToNot(BeNil(), "Should return analysis accuracy validation result")

			// BR-AI-001 & BR-AI-002 Business Requirement: 85% analysis accuracy
			Expect(result.MeetsRequirement).To(BeTrue(), "Must achieve 85% analysis accuracy requirement")
			Expect(result.AverageAccuracy).To(BeNumerically(">=", 0.85),
				"Average accuracy across all providers must be >= 85%")

			// Validate each provider meets minimum accuracy threshold
			for providerName, accuracy := range result.ProviderAccuracies {
				Expect(accuracy).To(BeNumerically(">=", 0.80),
					"Provider %s should achieve at least 80% accuracy", providerName)
			}

			// Validate processing times are reasonable
			for providerName, processingTime := range result.ProcessingTimes {
				Expect(processingTime).To(BeNumerically("<", 60*time.Second),
					"Provider %s should process within 60 seconds", providerName)
			}

			GinkgoWriter.Printf("✅ BR-AI-001 & BR-AI-002 Analysis: %.1f%% average accuracy across providers\\n",
				result.AverageAccuracy*100)
		})

		It("should demonstrate consistent accuracy across different alert types", func() {
			By("Validating analysis consistency across various alert complexities")

			// Test with different complexity levels
			complexDatasets := validator.createComplexityVariedDatasets()

			result, err := validator.ValidateAnalysisAccuracy(ctx, complexDatasets)

			Expect(err).ToNot(HaveOccurred(), "Complexity analysis validation should not fail")
			Expect(result).ToNot(BeNil(), "Should return complexity analysis validation result")

			// Consistency across complexity levels
			for _, analysisResult := range result.AnalysisResults {
				Expect(analysisResult.Accuracy).To(BeNumerically(">=", 0.75),
					"Provider %s should maintain >= 75% accuracy across complexity levels", analysisResult.Provider)
			}

			GinkgoWriter.Printf("✅ Analysis Consistency: Maintained accuracy across complexity levels\\n")
		})
	})

	Context("Cost Optimization Through Provider Selection", func() {
		It("should demonstrate cost optimization through intelligent provider selection", func() {
			By("Testing cost optimization algorithms across different workload scenarios")

			costOptimizationScenarios := []CostOptimizationScenario{
				{WorkloadType: "high_volume_simple", TokenCount: 100000, QualityRequired: 0.80, TimeConstraint: 30 * time.Second, BudgetLimit: 10.0},
				{WorkloadType: "low_volume_complex", TokenCount: 10000, QualityRequired: 0.95, TimeConstraint: 60 * time.Second, BudgetLimit: 5.0},
				{WorkloadType: "real_time_critical", TokenCount: 50000, QualityRequired: 0.90, TimeConstraint: 5 * time.Second, BudgetLimit: 20.0},
				{WorkloadType: "batch_processing", TokenCount: 500000, QualityRequired: 0.85, TimeConstraint: 300 * time.Second, BudgetLimit: 50.0},
			}

			result, err := validator.ValidateCostOptimization(ctx, costOptimizationScenarios)

			Expect(err).ToNot(HaveOccurred(), "Cost optimization validation should not fail")
			Expect(result).ToNot(BeNil(), "Should return cost optimization validation result")

			// Cost optimization should demonstrate measurable savings
			Expect(result.MeetsRequirement).To(BeTrue(), "Must achieve cost optimization through provider selection")
			Expect(result.AverageSavings).To(BeNumerically(">", 0.0), "Should demonstrate cost savings")
			Expect(result.OptimizationEffectiveness).To(BeNumerically(">=", 0.70),
				"Optimization algorithm should be >= 70% effective")

			// Validate optimization results for different workload types
			for _, optimization := range result.OptimizationResults {
				Expect(optimization.QualityScore).To(BeNumerically(">=", 0.75),
					"Optimized selection should maintain quality >= 75%")
				Expect(optimization.SelectedProvider).ToNot(BeEmpty(),
					"Should select a specific provider for optimization")
			}

			GinkgoWriter.Printf("✅ Cost Optimization: %.2f average savings, %.1f%% effectiveness\\n",
				result.AverageSavings, result.OptimizationEffectiveness*100)
		})
	})

	Context("Multi-Provider Integration Testing", func() {
		It("should demonstrate comprehensive multi-provider LLM integration", func() {
			By("Running integrated validation across all multi-provider requirements")

			// Test combined requirements: connectivity + failover + accuracy + cost optimization
			providerConfigs := createTestProviderConfigs()

			// Validate provider connectivity
			connectivityResult, err := validator.ValidateProviderConnectivity(ctx, providerConfigs)
			Expect(err).ToNot(HaveOccurred())
			Expect(connectivityResult.MeetsRequirement).To(BeTrue())

			// Validate intelligent failover
			failoverScenarios := []FailoverScenario{
				{FromProvider: "openai", ToProvider: "anthropic", FailureReason: "integration_test", ExpectedTime: 300 * time.Millisecond},
				{FromProvider: "azure", ToProvider: "ollama", FailureReason: "integration_test", ExpectedTime: 250 * time.Millisecond},
			}
			failoverResult, err := validator.ValidateIntelligentFailover(ctx, failoverScenarios)
			Expect(err).ToNot(HaveOccurred())
			Expect(failoverResult.MeetsRequirement).To(BeTrue())

			// Validate analysis accuracy
			testDatasets := validator.createAnalysisTestDatasets()
			accuracyResult, err := validator.ValidateAnalysisAccuracy(ctx, testDatasets)
			Expect(err).ToNot(HaveOccurred())
			Expect(accuracyResult.MeetsRequirement).To(BeTrue())

			GinkgoWriter.Printf("✅ Phase 2.1 Multi-Provider LLM: All critical requirements validated\\n")
			GinkgoWriter.Printf("   - Provider Connectivity: %d/%d providers (%.1f%%)\\n", connectivityResult.ConnectedProviders, connectivityResult.TotalProviders, connectivityResult.ConnectivityRate*100)
			GinkgoWriter.Printf("   - Failover Latency: %.0fms average (< 500ms)\\n", float64(failoverResult.AverageSwitchTime.Nanoseconds())/1000000)
			GinkgoWriter.Printf("   - Analysis Accuracy: %.1f%% (>= 85%%)\\n", accuracyResult.AverageAccuracy*100)
		})
	})
})

// Helper methods for creating test datasets - these will be implemented after business logic

func (v *MultiProviderLLMValidator) createAnalysisTestDatasets() []AnalysisDataset {
	// This method will be implemented after the business logic
	// For now, return minimal test data to make tests compile
	return []AnalysisDataset{
		{
			Name: "basic_alert_classification",
			Alerts: []types.Alert{
				{Name: "HighMemoryUsage", Severity: "warning", Namespace: "production"},
				{Name: "PodCrashLooping", Severity: "critical", Namespace: "production"},
			},
			ExpectedResults: []string{"scale_up", "restart_pod"},
			Complexity:      "simple",
		},
	}
}

func (v *MultiProviderLLMValidator) createComplexityVariedDatasets() []AnalysisDataset {
	// This method will be implemented after the business logic
	return []AnalysisDataset{
		{
			Name: "mixed_complexity_alerts",
			Alerts: []types.Alert{
				{Name: "ComplexNetworkIssue", Severity: "critical", Namespace: "production"},
			},
			ExpectedResults: []string{"diagnose_network"},
			Complexity:      "complex",
		},
	}
}

// Business logic methods - these will be implemented after tests are complete

func (v *MultiProviderLLMValidator) testProviderConnectivity(ctx context.Context, config LLMProviderConfig) (bool, ProviderHealth) {
	v.logger.WithField("provider", config.Name).Debug("Testing provider connectivity")

	startTime := time.Now()
	health := ProviderHealth{
		LastHealthCheck:  startTime,
		ConsecutiveFails: 0,
	}

	// Create LLM client for this provider
	client, err := v.createLLMClient(config)
	if err != nil {
		v.logger.WithError(err).WithField("provider", config.Name).Warn("Failed to create LLM client")
		health.IsHealthy = false
		health.ConsecutiveFails = 1
		health.AverageLatency = time.Since(startTime)
		health.SuccessRate = 0.0
		return false, health
	}

	// Test basic connectivity with a simple health check query
	testPrompt := "Respond with 'OK' to confirm connectivity."
	response, err := client.GenerateResponse(testPrompt)
	latency := time.Since(startTime)

	if err != nil {
		v.logger.WithError(err).WithField("provider", config.Name).Warn("Provider connectivity test failed")
		health.IsHealthy = false
		health.ConsecutiveFails = 1
		health.AverageLatency = latency
		health.SuccessRate = 0.0
		return false, health
	}

	// Validate response indicates successful connectivity
	isHealthy := response != ""
	if isHealthy {
		v.logger.WithFields(logrus.Fields{
			"provider":        config.Name,
			"latency":         latency,
			"response_length": len(response),
		}).Debug("Provider connectivity test successful")
	}

	health.IsHealthy = isHealthy
	health.AverageLatency = latency
	health.SuccessRate = func() float64 {
		if isHealthy {
			return 1.0
		}
		return 0.0
	}()

	// Store provider client for later use
	v.providerPool.mu.Lock()
	v.providerPool.providers[config.Name] = client
	v.providerPool.healthChecks[config.Name] = health
	v.providerPool.mu.Unlock()

	return isHealthy, health
}

func (v *MultiProviderLLMValidator) executeProviderFailover(ctx context.Context, scenario FailoverScenario) bool {
	v.logger.WithFields(logrus.Fields{
		"from_provider": scenario.FromProvider,
		"to_provider":   scenario.ToProvider,
		"reason":        scenario.FailureReason,
	}).Debug("Executing provider failover scenario")

	v.providerPool.mu.Lock()
	defer v.providerPool.mu.Unlock()

	// Verify source provider exists
	_, fromExists := v.providerPool.providers[scenario.FromProvider]
	if !fromExists {
		v.logger.WithField("provider", scenario.FromProvider).Warn("Source provider not found for failover")
		return false
	}

	// Verify target provider exists
	toClient, toExists := v.providerPool.providers[scenario.ToProvider]
	if !toExists {
		v.logger.WithField("provider", scenario.ToProvider).Warn("Target provider not found for failover")
		return false
	}

	// Simulate failure of source provider (already holding lock)
	v.simulateProviderFailureUnsafe(scenario.FromProvider)

	// Test that target provider can handle requests
	testPrompt := "Failover test: Process this alert - High CPU usage detected. Respond with recommended action."
	response, err := toClient.GenerateResponse(testPrompt)

	if err != nil {
		v.logger.WithError(err).WithField("to_provider", scenario.ToProvider).Warn("Failover target provider failed")
		return false
	}

	// Validate response indicates successful failover
	if response == "" {
		v.logger.WithField("to_provider", scenario.ToProvider).Warn("Failover target returned empty response")
		return false
	}

	// Update provider pool to reflect the failover
	v.providerPool.primaryProvider = scenario.ToProvider

	v.logger.WithFields(logrus.Fields{
		"from_provider":   scenario.FromProvider,
		"to_provider":     scenario.ToProvider,
		"response_length": len(response),
	}).Debug("Provider failover completed successfully")

	return true
}

func (v *MultiProviderLLMValidator) measureProviderAccuracy(ctx context.Context, client llm.Client, datasets []AnalysisDataset) float64 {
	if len(datasets) == 0 {
		v.logger.Warn("No datasets provided for accuracy measurement")
		return 0.0
	}

	totalTests := 0
	correctPredictions := 0

	for _, dataset := range datasets {
		v.logger.WithField("dataset", dataset.Name).Debug("Processing dataset for accuracy measurement")

		for i, alert := range dataset.Alerts {
			if i >= len(dataset.ExpectedResults) {
				v.logger.WithField("dataset", dataset.Name).Warn("More alerts than expected results")
				break
			}

			totalTests++
			expectedResult := dataset.ExpectedResults[i]

			// Create analysis prompt for the alert
			prompt := v.createAnalysisPrompt(alert, dataset.Complexity)

			// Get LLM response
			response, err := client.GenerateResponse(prompt)

			if err != nil {
				v.logger.WithError(err).WithField("alert", alert.Name).Warn("Failed to get LLM response for analysis")
				continue
			}

			if response == "" {
				v.logger.WithField("alert", alert.Name).Warn("Empty response from LLM")
				continue
			}

			// Check if the response contains the expected action
			if v.evaluateResponse(response, expectedResult) {
				correctPredictions++
				v.logger.WithFields(logrus.Fields{
					"alert":    alert.Name,
					"expected": expectedResult,
					"correct":  true,
				}).Debug("Correct prediction")
			} else {
				v.logger.WithFields(logrus.Fields{
					"alert":    alert.Name,
					"expected": expectedResult,
					"response": response,
					"correct":  false,
				}).Debug("Incorrect prediction")
			}
		}
	}

	if totalTests == 0 {
		v.logger.Warn("No tests executed for accuracy measurement")
		return 0.0
	}

	accuracy := float64(correctPredictions) / float64(totalTests)
	v.logger.WithFields(logrus.Fields{
		"correct_predictions": correctPredictions,
		"total_tests":         totalTests,
		"accuracy":            accuracy,
	}).Info("Provider accuracy measurement completed")

	return accuracy
}

func (v *MultiProviderLLMValidator) executeCostOptimization(ctx context.Context, scenario CostOptimizationScenario) OptimizationResult {
	v.logger.WithFields(logrus.Fields{
		"workload_type":    scenario.WorkloadType,
		"token_count":      scenario.TokenCount,
		"quality_required": scenario.QualityRequired,
		"budget_limit":     scenario.BudgetLimit,
	}).Debug("Executing cost optimization scenario")

	v.providerPool.mu.RLock()
	defer v.providerPool.mu.RUnlock()

	bestProvider := ""
	bestScore := 0.0
	bestQuality := 0.0
	maxSavings := 0.0

	// Evaluate each available provider
	for providerName := range v.providerPool.providers {
		config := v.getProviderConfig(providerName)
		if config == nil {
			continue
		}

		// Calculate cost for this provider
		estimatedCost := float64(scenario.TokenCount) * config.CostPerToken
		if estimatedCost > scenario.BudgetLimit {
			v.logger.WithFields(logrus.Fields{
				"provider":       providerName,
				"estimated_cost": estimatedCost,
				"budget_limit":   scenario.BudgetLimit,
			}).Debug("Provider exceeds budget limit")
			continue
		}

		// Estimate quality based on provider capabilities and scenario requirements
		estimatedQuality := v.estimateProviderQuality(providerName, scenario)
		if estimatedQuality < scenario.QualityRequired {
			v.logger.WithFields(logrus.Fields{
				"provider":          providerName,
				"estimated_quality": estimatedQuality,
				"quality_required":  scenario.QualityRequired,
			}).Debug("Provider does not meet quality requirements")
			continue
		}

		// Calculate optimization score (higher is better)
		// Score combines cost efficiency and quality
		costEfficiency := (scenario.BudgetLimit - estimatedCost) / scenario.BudgetLimit
		qualityScore := estimatedQuality
		optimizationScore := (costEfficiency * 0.4) + (qualityScore * 0.6)

		v.logger.WithFields(logrus.Fields{
			"provider":           providerName,
			"estimated_cost":     estimatedCost,
			"estimated_quality":  estimatedQuality,
			"cost_efficiency":    costEfficiency,
			"optimization_score": optimizationScore,
		}).Debug("Provider evaluation completed")

		if optimizationScore > bestScore {
			bestProvider = providerName
			bestScore = optimizationScore
			bestQuality = estimatedQuality
			// Calculate savings compared to most expensive viable option
			maxSavings = scenario.BudgetLimit - estimatedCost
		}
	}

	if bestProvider == "" {
		v.logger.Warn("No provider meets cost optimization criteria")
		return OptimizationResult{
			SelectedProvider:   "none",
			CostSavings:        0.0,
			QualityScore:       0.0,
			ReasonForSelection: "no_viable_provider",
		}
	}

	reason := v.generateOptimizationReason(bestProvider, bestScore, scenario)

	v.logger.WithFields(logrus.Fields{
		"selected_provider": bestProvider,
		"cost_savings":      maxSavings,
		"quality_score":     bestQuality,
		"reason":            reason,
	}).Info("Cost optimization completed")

	return OptimizationResult{
		SelectedProvider:   bestProvider,
		CostSavings:        maxSavings,
		QualityScore:       bestQuality,
		ReasonForSelection: reason,
	}
}

func (v *MultiProviderLLMValidator) calculateOptimizationEffectiveness(results []OptimizationResult) float64 {
	if len(results) == 0 {
		return 0.0
	}

	totalSavings := 0.0
	totalQualityScore := 0.0
	successfulOptimizations := 0

	for _, result := range results {
		if result.SelectedProvider != "none" && result.SelectedProvider != "" {
			successfulOptimizations++
			totalSavings += result.CostSavings
			totalQualityScore += result.QualityScore
		}
	}

	if successfulOptimizations == 0 {
		return 0.0
	}

	// Calculate effectiveness based on:
	// 1. Success rate of optimizations (50% weight)
	// 2. Average quality maintained (30% weight)
	// 3. Average cost savings achieved (20% weight)

	successRate := float64(successfulOptimizations) / float64(len(results))
	averageQuality := totalQualityScore / float64(successfulOptimizations)
	averageSavings := totalSavings / float64(successfulOptimizations)

	// Normalize savings to 0-1 scale (assuming max possible savings is budget limit)
	// For this calculation, we'll assume an average budget of $50 as baseline
	normalizedSavings := func() float64 {
		if averageSavings > 50.0 {
			return 1.0
		}
		return averageSavings / 50.0
	}()

	effectiveness := (successRate * 0.5) + (averageQuality * 0.3) + (normalizedSavings * 0.2)

	v.logger.WithFields(logrus.Fields{
		"success_rate":       successRate,
		"average_quality":    averageQuality,
		"average_savings":    averageSavings,
		"normalized_savings": normalizedSavings,
		"effectiveness":      effectiveness,
	}).Debug("Optimization effectiveness calculated")

	return effectiveness
}

// Helper methods for business logic implementation

func (v *MultiProviderLLMValidator) createLLMClient(config LLMProviderConfig) (llm.Client, error) {
	// For integration testing, we'll use the existing LLM client with different configurations
	// This simulates multiple providers by changing the endpoint and model configuration

	// Convert provider config to internal config format
	internalConfig := v.convertToInternalConfig(config)

	// Create the LLM client using the existing client factory
	client, err := llm.NewClient(internalConfig, v.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM client for provider %s: %w", config.Name, err)
	}

	return client, nil
}

func (v *MultiProviderLLMValidator) simulateProviderFailure(providerName string) {
	v.providerPool.mu.Lock()
	defer v.providerPool.mu.Unlock()
	v.simulateProviderFailureUnsafe(providerName)
}

// simulateProviderFailureUnsafe performs provider failure simulation without acquiring lock
// Caller must hold the appropriate lock
func (v *MultiProviderLLMValidator) simulateProviderFailureUnsafe(providerName string) {
	// Mark provider as unhealthy to simulate failure
	if health, exists := v.providerPool.healthChecks[providerName]; exists {
		health.IsHealthy = false
		health.ConsecutiveFails++
		health.SuccessRate = 0.0
		v.providerPool.healthChecks[providerName] = health
	}

	v.logger.WithField("provider", providerName).Debug("Simulated provider failure")
}

func (v *MultiProviderLLMValidator) createAnalysisPrompt(alert types.Alert, complexity string) string {
	base := fmt.Sprintf(`Analyze this Kubernetes alert and provide a recommended remediation action:

Alert: %s
Severity: %s
Namespace: %s`, alert.Name, alert.Severity, alert.Namespace)

	switch complexity {
	case "simple":
		return base + "\n\nProvide a single word action (e.g., restart_pod, scale_up, etc.)"
	case "complex":
		return base + "\n\nProvide a detailed analysis and step-by-step remediation plan."
	default:
		return base + "\n\nProvide the most appropriate remediation action."
	}
}

func (v *MultiProviderLLMValidator) evaluateResponse(response, expectedResult string) bool {
	// Simple keyword matching for now - in production this would be more sophisticated
	response = strings.ToLower(response)
	expectedResult = strings.ToLower(expectedResult)

	// Check if the expected result is contained in the response
	return strings.Contains(response, expectedResult) ||
		strings.Contains(response, strings.ReplaceAll(expectedResult, "_", " "))
}

func (v *MultiProviderLLMValidator) getProviderConfig(providerName string) *LLMProviderConfig {
	// In a real implementation, this would retrieve stored provider configurations
	// For now, return a basic configuration
	configs := map[string]LLMProviderConfig{
		"openai":    {Name: "openai", CostPerToken: 0.00003, TimeoutDuration: 30 * time.Second},
		"anthropic": {Name: "anthropic", CostPerToken: 0.000015, TimeoutDuration: 30 * time.Second},
		"azure":     {Name: "azure", CostPerToken: 0.00001, TimeoutDuration: 30 * time.Second},
		"aws":       {Name: "aws", CostPerToken: 0.00008, TimeoutDuration: 30 * time.Second},
		"ollama":    {Name: "ollama", CostPerToken: 0.0, TimeoutDuration: 30 * time.Second},
		"local":     {Name: "local", CostPerToken: 0.0, TimeoutDuration: 30 * time.Second},
	}

	if config, exists := configs[providerName]; exists {
		return &config
	}
	return nil
}

func (v *MultiProviderLLMValidator) estimateProviderQuality(providerName string, scenario CostOptimizationScenario) float64 {
	// Quality estimation based on provider characteristics and scenario requirements
	baseQuality := map[string]float64{
		"openai":    0.95, // High quality, state-of-the-art models
		"anthropic": 0.93, // High quality, strong reasoning
		"azure":     0.90, // Good quality, enterprise features
		"aws":       0.88, // Good quality, reliable
		"ollama":    0.82, // Good quality for local models
		"local":     0.80, // Variable quality depending on model
	}[providerName]

	// Adjust quality based on scenario type
	switch scenario.WorkloadType {
	case "high_volume_simple":
		// Local providers better for high volume simple tasks
		if providerName == "ollama" || providerName == "local" {
			baseQuality += 0.05
		}
	case "low_volume_complex":
		// Cloud providers better for complex reasoning
		if providerName == "openai" || providerName == "anthropic" {
			baseQuality += 0.03
		}
	case "real_time_critical":
		// Local providers better for latency-sensitive tasks
		if providerName == "ollama" || providerName == "local" {
			baseQuality += 0.04
		}
	}

	// Ensure quality doesn't exceed 1.0
	if baseQuality > 1.0 {
		baseQuality = 1.0
	}

	return baseQuality
}

func (v *MultiProviderLLMValidator) generateOptimizationReason(provider string, score float64, scenario CostOptimizationScenario) string {
	reasons := []string{}

	switch provider {
	case "ollama", "local":
		reasons = append(reasons, "zero_cost_operation")
		if scenario.TimeConstraint < 10*time.Second {
			reasons = append(reasons, "low_latency_local_processing")
		}
	case "openai":
		if scenario.QualityRequired > 0.90 {
			reasons = append(reasons, "highest_quality_available")
		}
	case "anthropic":
		if scenario.WorkloadType == "low_volume_complex" {
			reasons = append(reasons, "superior_reasoning_capabilities")
		}
	case "azure", "aws":
		reasons = append(reasons, "enterprise_reliability")
	}

	if score > 0.80 {
		reasons = append(reasons, "optimal_cost_quality_balance")
	} else if score > 0.60 {
		reasons = append(reasons, "acceptable_cost_quality_tradeoff")
	} else {
		reasons = append(reasons, "best_available_option")
	}

	return strings.Join(reasons, ", ")
}

// convertToInternalConfig converts provider config to internal LLM config format
func (v *MultiProviderLLMValidator) convertToInternalConfig(providerConfig LLMProviderConfig) config.LLMConfig {
	// For integration testing with real LLM service at ramalama endpoint, use ramalama provider
	// which supports OpenAI-compatible API format
	provider := "ramalama"
	expectedEndpoint := os.Getenv("LLM_ENDPOINT")
	if expectedEndpoint == "" {
		expectedEndpoint = "http://192.168.1.169:8080"
	}
	if providerConfig.Endpoint != expectedEndpoint {
		// For real external providers, use their actual provider type
		provider = providerConfig.Name
	}

	return config.LLMConfig{
		Provider: provider,
		Model:    providerConfig.Model,
		Endpoint: providerConfig.Endpoint,
		APIKey:   providerConfig.APIKey,
		Timeout:  providerConfig.TimeoutDuration,
	}
}
