package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/sirupsen/logrus"
)

// **REFACTOR PHASE**: Enhanced ensemble algorithms for superior decision-making
// Business Requirements: BR-ENSEMBLE-001 through BR-ENSEMBLE-004

// ModelResponse represents a structured response from an individual model
type ModelResponse struct {
	ModelID      string                 `json:"model_id"`
	Action       string                 `json:"action"`
	Confidence   float64                `json:"confidence"`
	Reasoning    string                 `json:"reasoning"`
	Parameters   map[string]interface{} `json:"parameters"`
	ResponseTime time.Duration          `json:"response_time"`
	Cost         float64                `json:"cost"`
}

// ConsensusAlgorithm defines different consensus strategies
type ConsensusAlgorithm string

const (
	WeightedVoting     ConsensusAlgorithm = "weighted_voting"
	MajorityVoting     ConsensusAlgorithm = "majority_voting"
	ConfidenceWeighted ConsensusAlgorithm = "confidence_weighted"
	CostOptimized      ConsensusAlgorithm = "cost_optimized"
	PerformanceBased   ConsensusAlgorithm = "performance_based"
)

// EnhancedConsensusEngine provides sophisticated ensemble decision-making
type EnhancedConsensusEngine struct {
	logger             *logrus.Logger
	performanceTracker *PerformanceTracker
	costOptimizer      *CostOptimizer
	healthMonitor      *HealthMonitor
}

// NewEnhancedConsensusEngine creates a new consensus engine
func NewEnhancedConsensusEngine(logger *logrus.Logger) *EnhancedConsensusEngine {
	return &EnhancedConsensusEngine{
		logger:             logger,
		performanceTracker: NewPerformanceTracker(logger),
		costOptimizer:      NewCostOptimizer(logger),
		healthMonitor:      NewHealthMonitor(logger),
	}
}

// ExecuteConsensus performs sophisticated ensemble decision-making
// BR-ENSEMBLE-001: Multi-model consensus with intelligent algorithms
func (e *EnhancedConsensusEngine) ExecuteConsensus(ctx context.Context, models []llm.Client, prompt string, algorithm ConsensusAlgorithm, options ConsensusOptions) (*EnhancedConsensusDecision, error) {
	startTime := time.Now()

	// Collect responses from all healthy models
	responses, err := e.collectModelResponses(ctx, models, prompt, options)
	if err != nil {
		return nil, fmt.Errorf("failed to collect model responses: %w", err)
	}

	// Apply consensus algorithm
	var decision *EnhancedConsensusDecision
	switch algorithm {
	case WeightedVoting:
		decision = e.executeWeightedVoting(responses, options)
	case MajorityVoting:
		decision = e.executeMajorityVoting(responses, options)
	case ConfidenceWeighted:
		decision = e.executeConfidenceWeighted(responses, options)
	case CostOptimized:
		decision = e.executeCostOptimized(responses, options)
	case PerformanceBased:
		decision = e.executePerformanceBased(responses, options)
	default:
		decision = e.executeWeightedVoting(responses, options) // Default fallback
	}

	// Enhance decision with metadata
	decision.ProcessingTime = time.Since(startTime)
	decision.Algorithm = string(algorithm)
	// Convert to interface slice
	decision.ModelResponses = make([]interface{}, len(responses))
	for i, response := range responses {
		decision.ModelResponses[i] = response
	}
	decision.QualityScore = e.calculateQualityScore(decision, responses)

	e.logger.WithFields(logrus.Fields{
		"algorithm":       algorithm,
		"participating":   len(responses),
		"confidence":      decision.Confidence,
		"processing_time": decision.ProcessingTime,
		"quality_score":   decision.QualityScore,
	}).Info("BR-ENSEMBLE-001: Enhanced consensus decision completed")

	return decision, nil
}

// collectModelResponses gathers responses from all available models
func (e *EnhancedConsensusEngine) collectModelResponses(ctx context.Context, models []llm.Client, prompt string, options ConsensusOptions) ([]*ModelResponse, error) {
	responses := make([]*ModelResponse, 0, len(models))

	for i, model := range models {
		modelID := fmt.Sprintf("model-%d", i+1)

		// Skip unhealthy models
		if !e.healthMonitor.IsModelHealthy(modelID) {
			e.logger.Debugf("Skipping unhealthy model %s", modelID)
			continue
		}

		// Skip models in maintenance
		if e.healthMonitor.IsModelInMaintenance(modelID) {
			e.logger.Debugf("Skipping model %s in maintenance", modelID)
			continue
		}

		startTime := time.Now()
		rawResponse, err := model.ChatCompletion(ctx, prompt)
		responseTime := time.Since(startTime)

		if err != nil {
			e.logger.WithError(err).Warnf("Model %s failed to respond", modelID)
			e.healthMonitor.RecordFailure(modelID)
			continue
		}

		// Parse structured response
		response := e.parseModelResponse(modelID, rawResponse, responseTime)
		if response != nil {
			responses = append(responses, response)
			e.performanceTracker.RecordResponse(modelID, response)
		}
	}

	if len(responses) == 0 {
		return nil, fmt.Errorf("no models provided valid responses")
	}

	return responses, nil
}

// parseModelResponse parses and validates model response
func (e *EnhancedConsensusEngine) parseModelResponse(modelID, rawResponse string, responseTime time.Duration) *ModelResponse {
	// Try to parse as JSON first
	var jsonResp map[string]interface{}
	if err := json.Unmarshal([]byte(rawResponse), &jsonResp); err == nil {
		return &ModelResponse{
			ModelID:      modelID,
			Action:       e.extractString(jsonResp, "action", "restart_pod"),
			Confidence:   e.extractFloat(jsonResp, "confidence", 0.5),
			Reasoning:    e.extractString(jsonResp, "reasoning", "Model analysis"),
			Parameters:   e.extractParameters(jsonResp),
			ResponseTime: responseTime,
			Cost:         e.costOptimizer.GetModelCost(modelID),
		}
	}

	// Fallback to text parsing
	return &ModelResponse{
		ModelID:      modelID,
		Action:       e.extractActionFromText(rawResponse),
		Confidence:   e.extractConfidenceFromText(rawResponse),
		Reasoning:    rawResponse,
		Parameters:   make(map[string]interface{}),
		ResponseTime: responseTime,
		Cost:         e.costOptimizer.GetModelCost(modelID),
	}
}

// executeWeightedVoting implements weighted voting based on model performance
// BR-ENSEMBLE-001: Weighted consensus for improved accuracy
func (e *EnhancedConsensusEngine) executeWeightedVoting(responses []*ModelResponse, options ConsensusOptions) *EnhancedConsensusDecision {
	actionWeights := make(map[string]float64)
	totalWeight := 0.0

	for _, response := range responses {
		weight := e.performanceTracker.GetModelWeight(response.ModelID)
		actionWeights[response.Action] += weight * response.Confidence
		totalWeight += weight
	}

	// Find action with highest weighted score
	var bestAction string
	var bestScore float64
	for action, score := range actionWeights {
		if score > bestScore {
			bestAction = action
			bestScore = score
		}
	}

	// Calculate consensus confidence
	consensusConfidence := bestScore / totalWeight
	if consensusConfidence > 1.0 {
		consensusConfidence = 1.0
	}

	// Calculate disagreement metrics
	disagreementScore := e.calculateDisagreementScore(responses)

	return &EnhancedConsensusDecision{
		Action:                 bestAction,
		Confidence:             consensusConfidence,
		ParticipatingModels:    len(responses),
		DisagreementScore:      disagreementScore,
		DisagreementResolution: "weighted_voting_by_performance",
		ConflictScore:          disagreementScore,
	}
}

// executeMajorityVoting implements simple majority voting
func (e *EnhancedConsensusEngine) executeMajorityVoting(responses []*ModelResponse, options ConsensusOptions) *EnhancedConsensusDecision {
	actionCounts := make(map[string]int)
	totalConfidence := 0.0

	for _, response := range responses {
		actionCounts[response.Action]++
		totalConfidence += response.Confidence
	}

	// Find majority action
	var bestAction string
	var bestCount int
	for action, count := range actionCounts {
		if count > bestCount {
			bestAction = action
			bestCount = count
		}
	}

	consensusConfidence := totalConfidence / float64(len(responses))
	disagreementScore := e.calculateDisagreementScore(responses)

	return &EnhancedConsensusDecision{
		Action:                 bestAction,
		Confidence:             consensusConfidence,
		ParticipatingModels:    len(responses),
		DisagreementScore:      disagreementScore,
		DisagreementResolution: "majority_voting",
		ConflictScore:          disagreementScore,
	}
}

// executeConfidenceWeighted implements confidence-weighted voting
func (e *EnhancedConsensusEngine) executeConfidenceWeighted(responses []*ModelResponse, options ConsensusOptions) *EnhancedConsensusDecision {
	actionScores := make(map[string]float64)
	totalConfidence := 0.0

	for _, response := range responses {
		actionScores[response.Action] += response.Confidence
		totalConfidence += response.Confidence
	}

	// Find action with highest confidence-weighted score
	var bestAction string
	var bestScore float64
	for action, score := range actionScores {
		if score > bestScore {
			bestAction = action
			bestScore = score
		}
	}

	consensusConfidence := bestScore / totalConfidence
	disagreementScore := e.calculateDisagreementScore(responses)

	return &EnhancedConsensusDecision{
		Action:                 bestAction,
		Confidence:             consensusConfidence,
		ParticipatingModels:    len(responses),
		DisagreementScore:      disagreementScore,
		DisagreementResolution: "confidence_weighted_voting",
		ConflictScore:          disagreementScore,
	}
}

// executeCostOptimized implements cost-aware consensus
// BR-ENSEMBLE-003: Cost-aware model selection
func (e *EnhancedConsensusEngine) executeCostOptimized(responses []*ModelResponse, options ConsensusOptions) *EnhancedConsensusDecision {
	// Sort responses by cost-effectiveness (confidence/cost ratio)
	sort.Slice(responses, func(i, j int) bool {
		ratioI := responses[i].Confidence / math.Max(responses[i].Cost, 0.001)
		ratioJ := responses[j].Confidence / math.Max(responses[j].Cost, 0.001)
		return ratioI > ratioJ
	})

	// Use most cost-effective responses within budget
	selectedResponses := []*ModelResponse{}
	totalCost := 0.0

	for _, response := range responses {
		if totalCost+response.Cost <= options.Budget.MaxCostPerRequest {
			selectedResponses = append(selectedResponses, response)
			totalCost += response.Cost
		}
	}

	if len(selectedResponses) == 0 {
		// Use cheapest option if budget is too low
		selectedResponses = []*ModelResponse{responses[len(responses)-1]}
		totalCost = selectedResponses[0].Cost
	}

	// Apply weighted voting to selected responses
	decision := e.executeWeightedVoting(selectedResponses, options)
	decision.TotalCost = totalCost
	decision.CostSavings = e.calculateCostSavings(responses, selectedResponses)
	decision.DegradationApplied = len(selectedResponses) < len(responses)

	return decision
}

// executePerformanceBased implements performance-based consensus
// BR-ENSEMBLE-002: Performance-based optimization
func (e *EnhancedConsensusEngine) executePerformanceBased(responses []*ModelResponse, options ConsensusOptions) *EnhancedConsensusDecision {
	// Filter responses by performance threshold
	highPerformingResponses := []*ModelResponse{}

	for _, response := range responses {
		performance := e.performanceTracker.GetModelPerformance(response.ModelID)
		if performance.AccuracyRate >= options.MinAccuracyThreshold {
			highPerformingResponses = append(highPerformingResponses, response)
		}
	}

	if len(highPerformingResponses) == 0 {
		// Fallback to all responses if none meet threshold
		highPerformingResponses = responses
	}

	// Apply weighted voting to high-performing models
	decision := e.executeWeightedVoting(highPerformingResponses, options)
	decision.ExcludedModels = e.getExcludedModelIDs(responses, highPerformingResponses)

	return decision
}

// Helper methods for enhanced algorithms

func (e *EnhancedConsensusEngine) calculateDisagreementScore(responses []*ModelResponse) float64 {
	if len(responses) <= 1 {
		return 0.0
	}

	actionCounts := make(map[string]int)
	for _, response := range responses {
		actionCounts[response.Action]++
	}

	// Calculate entropy as disagreement measure
	entropy := 0.0
	total := float64(len(responses))

	for _, count := range actionCounts {
		if count > 0 {
			p := float64(count) / total
			entropy -= p * math.Log2(p)
		}
	}

	// Normalize entropy to 0-1 scale
	maxEntropy := math.Log2(total)
	if maxEntropy > 0 {
		return entropy / maxEntropy
	}
	return 0.0
}

func (e *EnhancedConsensusEngine) calculateQualityScore(decision *EnhancedConsensusDecision, responses []*ModelResponse) float64 {
	// Combine confidence, agreement, and performance metrics
	confidenceScore := decision.Confidence
	agreementScore := 1.0 - decision.DisagreementScore

	// Calculate average model performance
	totalPerformance := 0.0
	for _, response := range responses {
		performance := e.performanceTracker.GetModelPerformance(response.ModelID)
		totalPerformance += performance.AccuracyRate
	}
	performanceScore := totalPerformance / float64(len(responses))

	// Weighted combination
	qualityScore := (confidenceScore*0.4 + agreementScore*0.3 + performanceScore*0.3)
	return math.Min(qualityScore, 1.0)
}

func (e *EnhancedConsensusEngine) calculateCostSavings(allResponses, selectedResponses []*ModelResponse) float64 {
	totalCost := 0.0
	selectedCost := 0.0

	for _, response := range allResponses {
		totalCost += response.Cost
	}
	for _, response := range selectedResponses {
		selectedCost += response.Cost
	}

	if totalCost > 0 {
		return (totalCost - selectedCost) / totalCost
	}
	return 0.0
}

func (e *EnhancedConsensusEngine) getExcludedModelIDs(allResponses, selectedResponses []*ModelResponse) []string {
	selectedIDs := make(map[string]bool)
	for _, response := range selectedResponses {
		selectedIDs[response.ModelID] = true
	}

	excluded := []string{}
	for _, response := range allResponses {
		if !selectedIDs[response.ModelID] {
			excluded = append(excluded, response.ModelID)
		}
	}

	return excluded
}

// Text parsing helpers for non-JSON responses

func (e *EnhancedConsensusEngine) extractActionFromText(text string) string {
	text = strings.ToLower(text)

	actions := []string{"restart_pod", "scale_deployment", "notify_only", "investigate_logs", "emergency_cleanup"}
	for _, action := range actions {
		if strings.Contains(text, action) {
			return action
		}
	}

	// Fallback action detection
	if strings.Contains(text, "restart") || strings.Contains(text, "reboot") {
		return "restart_pod"
	}
	if strings.Contains(text, "scale") || strings.Contains(text, "increase") {
		return "scale_deployment"
	}

	return "investigate_logs" // Conservative default
}

func (e *EnhancedConsensusEngine) extractConfidenceFromText(text string) float64 {
	// Look for confidence patterns in text
	confidencePatterns := []string{"confidence", "certain", "sure", "likely"}

	for _, pattern := range confidencePatterns {
		if strings.Contains(strings.ToLower(text), pattern) {
			return 0.8 // High confidence if mentioned
		}
	}

	return 0.6 // Default moderate confidence
}

// JSON parsing helpers

func (e *EnhancedConsensusEngine) extractString(data map[string]interface{}, key, defaultValue string) string {
	if val, ok := data[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return defaultValue
}

func (e *EnhancedConsensusEngine) extractFloat(data map[string]interface{}, key string, defaultValue float64) float64 {
	if val, ok := data[key]; ok {
		if f, ok := val.(float64); ok {
			return f
		}
		if i, ok := val.(int); ok {
			return float64(i)
		}
	}
	return defaultValue
}

func (e *EnhancedConsensusEngine) extractParameters(data map[string]interface{}) map[string]interface{} {
	if params, ok := data["parameters"]; ok {
		if paramMap, ok := params.(map[string]interface{}); ok {
			return paramMap
		}
	}
	return make(map[string]interface{})
}
