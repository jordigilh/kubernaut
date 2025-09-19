package context

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// OptimizationService implements intelligent context optimization
// Business Requirements: BR-CONTEXT-016 to BR-CONTEXT-043
type OptimizationService struct {
	config               *config.ContextOptimizationConfig
	performanceMetrics   *PerformanceMetrics
	complexityClassifier *ComplexityClassifier
	adequacyValidator    *AdequacyValidator
	performanceMonitor   *PerformanceMonitor
	logger               *logrus.Logger
}

// NewOptimizationService creates a new context optimization service
func NewOptimizationService(cfg *config.ContextOptimizationConfig, logger *logrus.Logger) *OptimizationService {
	return &OptimizationService{
		config:               cfg,
		performanceMetrics:   NewPerformanceMetrics(),
		complexityClassifier: NewComplexityClassifier(cfg),
		adequacyValidator:    NewAdequacyValidator(cfg),
		performanceMonitor:   NewPerformanceMonitor(cfg, logger),
		logger:               logger,
	}
}

// ComplexityAssessment represents the result of alert complexity assessment
type ComplexityAssessment struct {
	Tier                 string                 `json:"tier"`
	ConfidenceScore      float64                `json:"confidence_score"`
	RecommendedReduction float64                `json:"recommended_reduction"`
	MinContextTypes      int                    `json:"min_context_types"`
	Characteristics      []string               `json:"characteristics"`
	EscalationRequired   bool                   `json:"escalation_required"`
	Metadata             map[string]interface{} `json:"metadata"`
}

// AdequacyAssessment represents the result of context adequacy validation
type AdequacyAssessment struct {
	IsAdequate          bool                   `json:"is_adequate"`
	AdequacyScore       float64                `json:"adequacy_score"`
	ConfidenceLevel     float64                `json:"confidence_level"`
	EnrichmentRequired  bool                   `json:"enrichment_required"`
	MissingContextTypes []string               `json:"missing_context_types"`
	SufficiencyAnalysis map[string]interface{} `json:"sufficiency_analysis"`
	Metadata            map[string]interface{} `json:"metadata"`
}

// PerformanceAssessment represents LLM performance monitoring results
type PerformanceAssessment struct {
	ResponseQuality     float64                `json:"response_quality"`
	ResponseTime        time.Duration          `json:"response_time"`
	TokenUsage          int                    `json:"token_usage"`
	BaselineDeviation   float64                `json:"baseline_deviation"`
	DegradationDetected bool                   `json:"degradation_detected"`
	AdjustmentTriggered bool                   `json:"adjustment_triggered"`
	NewReductionTarget  float64                `json:"new_reduction_target"`
	Metadata            map[string]interface{} `json:"metadata"`
}

// AssessComplexity implements BR-CONTEXT-016: Dynamic complexity assessment
func (s *OptimizationService) AssessComplexity(ctx context.Context, alert types.Alert) (*ComplexityAssessment, error) {
	s.logger.WithFields(logrus.Fields{
		"alert_name":     alert.Name,
		"alert_severity": alert.Severity,
		"namespace":      alert.Namespace,
	}).Debug("Assessing alert complexity")

	return s.complexityClassifier.Assess(ctx, alert)
}

// ValidateAdequacy implements BR-CONTEXT-021: Context adequacy validation
// Updated to use structured ContextData following project guidelines
func (s *OptimizationService) ValidateAdequacy(ctx context.Context, contextData *ContextData, investigationType string) (*AdequacyAssessment, error) {
	s.logger.WithFields(logrus.Fields{
		"investigation_type": investigationType,
		"context_types":      s.countContextTypes(contextData),
	}).Debug("Validating context adequacy")

	return s.adequacyValidator.Validate(ctx, contextData, investigationType)
}

// OptimizeContext implements BR-CONTEXT-031: Graduated context optimization
// Updated to use structured ContextData following project guidelines
func (s *OptimizationService) OptimizeContext(ctx context.Context, complexity *ComplexityAssessment, contextData *ContextData) (*ContextData, error) {
	s.logger.WithFields(logrus.Fields{
		"complexity_tier":       complexity.Tier,
		"recommended_reduction": complexity.RecommendedReduction,
		"original_size":         s.countContextTypes(contextData),
	}).Debug("Optimizing context based on complexity")

	// Apply graduated reduction based on complexity tier
	optimizedContext := &ContextData{}

	// Get tier configuration
	tier, exists := s.config.GraduatedReduction.Tiers[complexity.Tier]
	if !exists {
		return contextData, fmt.Errorf("unknown complexity tier: %s", complexity.Tier)
	}

	// Apply reduction strategy
	reductionTarget := tier.MaxReduction
	if complexity.RecommendedReduction > 0 {
		reductionTarget = complexity.RecommendedReduction
	}

	// Select most relevant context types based on priority
	contextPriorities := s.calculateContextPriorities(contextData, complexity)
	selectedTypes := s.selectHighPriorityContext(contextPriorities, tier.MinContextTypes, reductionTarget)

	// Copy selected context types to optimized context
	for _, contextType := range selectedTypes {
		s.copyContextType(contextData, optimizedContext, contextType)
	}

	originalTypes := s.countContextTypes(contextData)
	optimizedTypes := s.countContextTypes(optimizedContext)
	s.logger.WithFields(logrus.Fields{
		"original_types":  originalTypes,
		"optimized_types": optimizedTypes,
		"reduction_rate":  1.0 - float64(optimizedTypes)/float64(originalTypes),
	}).Info("Context optimization completed")

	return optimizedContext, nil
}

// MonitorPerformance implements BR-CONTEXT-039: Performance monitoring
func (s *OptimizationService) MonitorPerformance(ctx context.Context, responseQuality float64, responseTime time.Duration, tokenUsage int, contextSize int) (*PerformanceAssessment, error) {
	s.logger.WithFields(logrus.Fields{
		"response_quality": responseQuality,
		"response_time":    responseTime,
		"token_usage":      tokenUsage,
		"context_size":     contextSize,
	}).Debug("Monitoring LLM performance")

	return s.performanceMonitor.Monitor(ctx, responseQuality, responseTime, tokenUsage, contextSize)
}

// SelectOptimalLLMModel implements BR-CONTEXT-036: Dynamic model selection
func (s *OptimizationService) SelectOptimalLLMModel(ctx context.Context, contextSize int, complexity string) (string, error) {
	s.logger.WithFields(logrus.Fields{
		"context_size": contextSize,
		"complexity":   complexity,
	}).Debug("Selecting optimal LLM model")

	// Model selection logic based on context size and complexity
	switch {
	case contextSize <= 500 && (complexity == "simple" || complexity == "moderate"):
		return "gpt-3.5-turbo", nil
	case contextSize <= 1500 && complexity != "critical":
		return "gpt-4", nil
	case complexity == "critical" || contextSize > 1500:
		return "gpt-4", nil
	default:
		return "gpt-3.5-turbo", nil
	}
}

// AdjustReductionTargets implements BR-CONTEXT-038: Feedback loop adjustment
func (s *OptimizationService) AdjustReductionTargets(ctx context.Context, performance *PerformanceAssessment, currentTier string) error {
	if !performance.DegradationDetected {
		return nil
	}

	s.logger.WithFields(logrus.Fields{
		"current_tier":         currentTier,
		"baseline_deviation":   performance.BaselineDeviation,
		"new_reduction_target": performance.NewReductionTarget,
	}).Info("Adjusting context reduction targets due to performance degradation")

	// Update tier configuration with less aggressive reduction
	if tier, exists := s.config.GraduatedReduction.Tiers[currentTier]; exists {
		tier.MaxReduction = performance.NewReductionTarget
		s.config.GraduatedReduction.Tiers[currentTier] = tier
	}

	return nil
}

// Helper methods

func (s *OptimizationService) calculateContextPriorities(contextData *ContextData, complexity *ComplexityAssessment) map[string]float64 {
	priorities := make(map[string]float64)

	// Priority calculation based on context type and complexity
	contextTypes := s.getAvailableContextTypes(contextData)
	for _, contextType := range contextTypes {
		priority := 0.5 // Base priority

		// Adjust priority based on context type
		switch contextType {
		case "kubernetes":
			priority += 0.3
		case "metrics":
			priority += 0.2
		case "action-history":
			priority += 0.1
		case "logs":
			priority += 0.15
		case "events":
			priority += 0.08
		case "traces":
			priority += 0.12
		case "network-flows":
			priority += 0.06
		case "audit-logs":
			priority += 0.14
		}

		// Adjust priority based on complexity
		switch complexity.Tier {
		case "critical":
			priority += 0.2
		case "complex":
			priority += 0.1
		case "moderate":
			priority += 0.05
		}

		priorities[contextType] = priority
	}

	return priorities
}

func (s *OptimizationService) selectHighPriorityContext(priorities map[string]float64, minTypes int, reductionTarget float64) []string {
	type contextItem struct {
		name     string
		priority float64
	}

	// Sort by priority
	var items []contextItem
	for name, priority := range priorities {
		items = append(items, contextItem{name: name, priority: priority})
	}

	// Simple selection: keep top priority items
	maxTypes := len(items)
	if reductionTarget > 0 {
		maxTypes = int(float64(len(items)) * (1.0 - reductionTarget))
	}

	if maxTypes < minTypes {
		maxTypes = minTypes
	}

	if maxTypes > len(items) {
		maxTypes = len(items)
	}

	// Sort items by priority (highest first)
	for i := 0; i < len(items)-1; i++ {
		for j := i + 1; j < len(items); j++ {
			if items[i].priority < items[j].priority {
				items[i], items[j] = items[j], items[i]
			}
		}
	}

	var selected []string
	for i := 0; i < maxTypes && i < len(items); i++ {
		selected = append(selected, items[i].name)
	}

	return selected
}

// Helper methods for ContextData operations

// countContextTypes counts how many context types are present in ContextData
func (s *OptimizationService) countContextTypes(contextData *ContextData) int {
	count := 0
	if contextData.Kubernetes != nil {
		count++
	}
	if contextData.Metrics != nil {
		count++
	}
	if contextData.Logs != nil {
		count++
	}
	if contextData.ActionHistory != nil {
		count++
	}
	if contextData.Events != nil {
		count++
	}
	if contextData.Traces != nil {
		count++
	}
	if contextData.NetworkFlows != nil {
		count++
	}
	if contextData.AuditLogs != nil {
		count++
	}
	return count
}

// copyContextType copies a specific context type from source to destination
func (s *OptimizationService) copyContextType(source *ContextData, dest *ContextData, contextType string) {
	switch contextType {
	case "kubernetes":
		dest.Kubernetes = source.Kubernetes
	case "metrics":
		dest.Metrics = source.Metrics
	case "logs":
		dest.Logs = source.Logs
	case "action-history":
		dest.ActionHistory = source.ActionHistory
	case "events":
		dest.Events = source.Events
	case "traces":
		dest.Traces = source.Traces
	case "network-flows":
		dest.NetworkFlows = source.NetworkFlows
	case "audit-logs":
		dest.AuditLogs = source.AuditLogs
	}
}

// getAvailableContextTypes returns a list of context types that are present in ContextData
func (s *OptimizationService) getAvailableContextTypes(contextData *ContextData) []string {
	var types []string
	if contextData.Kubernetes != nil {
		types = append(types, "kubernetes")
	}
	if contextData.Metrics != nil {
		types = append(types, "metrics")
	}
	if contextData.Logs != nil {
		types = append(types, "logs")
	}
	if contextData.ActionHistory != nil {
		types = append(types, "action-history")
	}
	if contextData.Events != nil {
		types = append(types, "events")
	}
	if contextData.Traces != nil {
		types = append(types, "traces")
	}
	if contextData.NetworkFlows != nil {
		types = append(types, "network-flows")
	}
	if contextData.AuditLogs != nil {
		types = append(types, "audit-logs")
	}
	return types
}
