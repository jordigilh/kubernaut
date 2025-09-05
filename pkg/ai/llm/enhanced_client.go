package llm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
	"github.com/sirupsen/logrus"
)

// EnhancedClient extends the basic SLM client with AI-powered response processing
type EnhancedClient interface {
	Client // Embed the basic client interface

	// AnalyzeAlertWithEnhancement provides AI-enhanced alert analysis with comprehensive insights
	AnalyzeAlertWithEnhancement(ctx context.Context, alert types.Alert) (*EnhancedActionRecommendation, error)

	// ValidateRecommendation validates an action recommendation with AI analysis
	ValidateRecommendation(ctx context.Context, recommendation *types.ActionRecommendation, alert types.Alert) (*ValidationResult, error)

	// SetResponseProcessor sets the AI response processor for the client
	SetResponseProcessor(processor AIResponseProcessor)

	// GetResponseProcessor returns the current AI response processor
	GetResponseProcessor() AIResponseProcessor
}

// enhancedClient implements EnhancedClient with AI-powered response processing
type enhancedClient struct {
	Client            // Embed the basic client
	responseProcessor AIResponseProcessor
	knowledgeBase     KnowledgeBase
	config            config.LLMConfig
	log               *logrus.Logger
}

// NewEnhancedClient creates a new AI-enhanced SLM client
func NewEnhancedClient(cfg config.LLMConfig, knowledgeBase KnowledgeBase, log *logrus.Logger) (EnhancedClient, error) {
	// Create basic client first
	basicClient, err := NewClient(cfg, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create basic SLM client: %w", err)
	}

	// Create AI response processor configuration
	processorConfig := &AIResponseProcessorConfig{
		EnableAdvancedValidation:    true,
		EnableReasoningAnalysis:     true,
		EnableConfidenceCalibration: true,
		EnableContextualEnhancement: true,
		ConfidenceThreshold:         0.75,
		ValidationTimeout:           10 * time.Second,
		MaxProcessingTime:           30 * time.Second,
		EnableDetailedLogging:       false,
	}

	// Create AI response processor using the basic client for AI analysis
	responseProcessor := NewDefaultAIResponseProcessor(basicClient, knowledgeBase, processorConfig)

	enhanced := &enhancedClient{
		Client:            basicClient,
		responseProcessor: responseProcessor,
		knowledgeBase:     knowledgeBase,
		config:            cfg,
		log:               log,
	}

	log.WithFields(logrus.Fields{
		"provider": cfg.Provider,
		"endpoint": cfg.Endpoint,
		"model":    cfg.Model,
		"enhanced": true,
	}).Info("Enhanced SLM client initialized with AI response processing")

	return enhanced, nil
}

// NewEnhancedClientWithProcessor creates a new enhanced client with a custom response processor
func NewEnhancedClientWithProcessor(cfg config.LLMConfig, processor AIResponseProcessor, log *logrus.Logger) (EnhancedClient, error) {
	// Create basic client first
	basicClient, err := NewClient(cfg, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create basic SLM client: %w", err)
	}

	enhanced := &enhancedClient{
		Client:            basicClient,
		responseProcessor: processor,
		config:            cfg,
		log:               log,
	}

	log.WithFields(logrus.Fields{
		"provider":         "LocalAI",
		"endpoint":         cfg.Endpoint,
		"model":            cfg.Model,
		"enhanced":         true,
		"custom_processor": true,
	}).Info("Enhanced SLM client initialized with custom AI response processor")

	return enhanced, nil
}

// NewEnhancedClientForTesting creates an enhanced client with injected dependencies for testing
func NewEnhancedClientForTesting(basicClient Client, processor AIResponseProcessor, knowledgeBase KnowledgeBase, cfg config.LLMConfig, log *logrus.Logger) EnhancedClient {
	enhanced := &enhancedClient{
		Client:            basicClient,
		responseProcessor: processor,
		knowledgeBase:     knowledgeBase,
		config:            cfg,
		log:               log,
	}

	return enhanced
}

// AnalyzeAlertWithEnhancement provides AI-enhanced alert analysis with comprehensive insights
func (ec *enhancedClient) AnalyzeAlertWithEnhancement(ctx context.Context, alert types.Alert) (*EnhancedActionRecommendation, error) {
	startTime := time.Now()

	ec.log.WithFields(logrus.Fields{
		"alert_name": alert.Name,
		"severity":   alert.Severity,
		"namespace":  alert.Namespace,
		"resource":   alert.Resource,
	}).Debug("Starting AI-enhanced alert analysis")

	// First get the basic recommendation using the standard client
	basicRecommendation, err := ec.Client.AnalyzeAlert(ctx, alert)
	if err != nil {
		return nil, fmt.Errorf("failed to get basic recommendation: %w", err)
	}

	// If we don't have a response processor, return a basic enhanced recommendation
	if ec.responseProcessor == nil || !ec.responseProcessor.IsHealthy() {
		ec.log.Warn("AI response processor unavailable, returning basic recommendation")
		return &EnhancedActionRecommendation{
			ActionRecommendation: basicRecommendation,
			ProcessingMetadata: &ProcessingMetadata{
				ProcessingTime:      time.Since(startTime),
				AIModelUsed:         "basic_client_only",
				ProcessingSteps:     []string{"basic_analysis"},
				ConfidenceThreshold: 0.75,
				ProcessingErrors:    []string{"AI response processor unavailable"},
			},
		}, nil
	}

	// Use AI response processor for enhancement
	// We'll reconstruct the raw response for processing (this is a simplified approach)
	rawResponse := ec.reconstructRawResponse(basicRecommendation)

	enhanced, err := ec.responseProcessor.ProcessResponse(ctx, rawResponse, alert)
	if err != nil {
		ec.log.WithError(err).Warn("AI response processing failed, returning basic recommendation")
		return &EnhancedActionRecommendation{
			ActionRecommendation: basicRecommendation,
			ProcessingMetadata: &ProcessingMetadata{
				ProcessingTime:   time.Since(startTime),
				AIModelUsed:      "basic_client_fallback",
				ProcessingSteps:  []string{"basic_analysis", "ai_processing_failed"},
				ProcessingErrors: []string{fmt.Sprintf("AI processing error: %v", err)},
			},
		}, nil
	}

	// Log successful enhancement
	ec.log.WithFields(logrus.Fields{
		"alert_name":         alert.Name,
		"processing_time":    enhanced.ProcessingMetadata.ProcessingTime,
		"enhancements":       len(enhanced.ProcessingMetadata.EnhancementsApplied),
		"validations_passed": enhanced.ProcessingMetadata.ValidationsPassed,
		"validations_failed": enhanced.ProcessingMetadata.ValidationsFailed,
	}).Debug("AI-enhanced alert analysis completed")

	return enhanced, nil
}

// ValidateRecommendation validates an action recommendation with AI analysis
func (ec *enhancedClient) ValidateRecommendation(ctx context.Context, recommendation *types.ActionRecommendation, alert types.Alert) (*ValidationResult, error) {
	if ec.responseProcessor == nil || !ec.responseProcessor.IsHealthy() {
		return nil, fmt.Errorf("AI response processor unavailable for validation")
	}

	return ec.responseProcessor.ValidateRecommendation(ctx, recommendation, alert)
}

// SetResponseProcessor sets the AI response processor for the client
func (ec *enhancedClient) SetResponseProcessor(processor AIResponseProcessor) {
	ec.responseProcessor = processor
	ec.log.Info("AI response processor updated for enhanced SLM client")
}

// GetResponseProcessor returns the current AI response processor
func (ec *enhancedClient) GetResponseProcessor() AIResponseProcessor {
	return ec.responseProcessor
}

// Helper methods

func (ec *enhancedClient) reconstructRawResponse(recommendation *types.ActionRecommendation) string {
	// This is a simplified reconstruction of what the raw LLM response might have looked like
	// In a real implementation, we might want to store the original raw response

	reasoning := ""
	if recommendation.Reasoning != nil {
		reasoning = recommendation.Reasoning.Summary
	}

	// Construct a JSON-like response similar to what the LLM would return
	response := fmt.Sprintf(`{
  "action": "%s",
  "parameters": %s,
  "confidence": %.3f,
  "reasoning": "%s"
}`,
		recommendation.Action,
		ec.formatParameters(recommendation.Parameters),
		recommendation.Confidence,
		reasoning,
	)

	return response
}

func (ec *enhancedClient) formatParameters(params map[string]interface{}) string {
	if len(params) == 0 {
		return "{}"
	}

	// Simple JSON formatting for parameters
	result := "{"
	first := true
	for key, value := range params {
		if !first {
			result += ", "
		}
		result += fmt.Sprintf(`"%s": "%v"`, key, value)
		first = false
	}
	result += "}"

	return result
}

// BasicKnowledgeBase provides a simple implementation of KnowledgeBase for testing/demos
type BasicKnowledgeBase struct {
	actionRisks     map[string]*RiskAssessment
	validationRules []ValidationRule
}

// NewBasicKnowledgeBase creates a new basic knowledge base with default data
func NewBasicKnowledgeBase() *BasicKnowledgeBase {
	kb := &BasicKnowledgeBase{
		actionRisks:     make(map[string]*RiskAssessment),
		validationRules: loadDefaultValidationRules(),
	}

	// Populate with default risk assessments
	kb.actionRisks["restart_pod"] = &RiskAssessment{
		RiskLevel:          "low",
		BlastRadius:        "pod",
		ReversibilityScore: 0.9,
		ImpactAnalysis:     map[string]interface{}{"impact": "minimal", "recovery_time": "fast"},
		SafetyChecks:       []string{"pod_readiness", "health_check"},
		PreconditionsMet:   true,
	}

	kb.actionRisks["scale_deployment"] = &RiskAssessment{
		RiskLevel:          "medium",
		BlastRadius:        "deployment",
		ReversibilityScore: 0.8,
		ImpactAnalysis:     map[string]interface{}{"impact": "moderate", "recovery_time": "medium"},
		SafetyChecks:       []string{"resource_availability", "cluster_capacity"},
		PreconditionsMet:   true,
	}

	kb.actionRisks["drain_node"] = &RiskAssessment{
		RiskLevel:          "high",
		BlastRadius:        "cluster",
		ReversibilityScore: 0.5,
		ImpactAnalysis:     map[string]interface{}{"impact": "significant", "recovery_time": "slow"},
		SafetyChecks:       []string{"workload_migration", "cluster_stability"},
		PreconditionsMet:   false,
	}

	return kb
}

// GetActionRisks returns risk assessment for a specific action
func (kb *BasicKnowledgeBase) GetActionRisks(action string) *RiskAssessment {
	if risk, exists := kb.actionRisks[action]; exists {
		return risk
	}

	// Return default risk assessment for unknown actions
	return &RiskAssessment{
		RiskLevel:          "medium",
		BlastRadius:        "deployment",
		ReversibilityScore: 0.7,
		ImpactAnalysis:     map[string]interface{}{"impact": "unknown", "method": "default"},
		SafetyChecks:       []string{"basic_validation"},
		PreconditionsMet:   true,
	}
}

// GetHistoricalPatterns returns historical patterns for an alert (simplified implementation)
func (kb *BasicKnowledgeBase) GetHistoricalPatterns(alert types.Alert) []HistoricalPattern {
	// In a real implementation, this would query a database or data store
	patterns := []HistoricalPattern{}

	// Add some example patterns based on alert characteristics
	if strings.Contains(strings.ToLower(alert.Description), "memory") {
		patterns = append(patterns, HistoricalPattern{
			Pattern:       "memory_pressure_pattern",
			Frequency:     5,
			LastSeen:      time.Now().Add(-24 * time.Hour),
			Effectiveness: 0.8,
			Context:       "Memory alerts often resolve with resource increases",
		})
	}

	if strings.Contains(strings.ToLower(alert.Description), "cpu") {
		patterns = append(patterns, HistoricalPattern{
			Pattern:       "cpu_spike_pattern",
			Frequency:     3,
			LastSeen:      time.Now().Add(-12 * time.Hour),
			Effectiveness: 0.7,
			Context:       "CPU spikes may indicate scaling needs",
		})
	}

	return patterns
}

// GetValidationRules returns validation rules
func (kb *BasicKnowledgeBase) GetValidationRules() []ValidationRule {
	return kb.validationRules
}

// GetSystemState returns current system state (simplified implementation)
func (kb *BasicKnowledgeBase) GetSystemState(ctx context.Context) (*SystemStateAnalysis, error) {
	// In a real implementation, this would gather actual system metrics
	return &SystemStateAnalysis{
		HealthScore:    0.8,
		StabilityScore: 0.75,
		CapacityUtilization: map[string]float64{
			"cpu":     0.65,
			"memory":  0.70,
			"storage": 0.45,
		},
		BottleneckAnalysis: []Bottleneck{
			{
				Resource:    "memory",
				Utilization: 0.85,
				Impact:      "moderate",
				Resolution:  "consider scaling or optimization",
			},
		},
		Dependencies: []string{"database", "cache", "external_api"},
		CriticalPath: []string{"load_balancer", "app_tier", "database"},
	}, nil
}
