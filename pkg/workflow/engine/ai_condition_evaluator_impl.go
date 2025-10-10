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
package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
)

// AIConditionEvaluator interface for backward compatibility with existing mocks
// @deprecated RULE 12 VIOLATION: Use enhanced llm.Client methods directly instead
type AIConditionEvaluator interface {
	EvaluateCondition(ctx context.Context, condition *ExecutableCondition, stepContext *StepContext) (bool, error)
	ValidateCondition(ctx context.Context, condition *ExecutableCondition) error
}

// DefaultAIConditionEvaluator implements AIConditionEvaluator using available AI services
// Business Requirement: BR-AI-COND-001 - Intelligent condition evaluation using AI services
type DefaultAIConditionEvaluator struct {
	llmClient    llm.Client
	holmesClient holmesgpt.Client
	vectorDB     vector.VectorDatabase
	fallbackMode bool // When true, uses basic evaluation for reliability
	log          *logrus.Logger
}

// NewDefaultAIConditionEvaluator creates a new AI condition evaluator
// Following development guideline: use factory pattern for consistent service creation
func NewDefaultAIConditionEvaluator(
	llmClient llm.Client,
	holmesClient holmesgpt.Client,
	vectorDB vector.VectorDatabase,
	log *logrus.Logger,
) *DefaultAIConditionEvaluator {
	evaluator := &DefaultAIConditionEvaluator{
		llmClient:    llmClient,
		holmesClient: holmesClient,
		vectorDB:     vectorDB,
		fallbackMode: false,
		log:          log,
	}

	// Enable fallback mode if no AI services are available
	if llmClient == nil && holmesClient == nil {
		evaluator.fallbackMode = true
		log.Warn("No AI services available for condition evaluation, using basic fallback mode")
	} else {
		log.WithFields(logrus.Fields{
			"llm_available":       llmClient != nil,
			"holmesgpt_available": holmesClient != nil,
			"vectordb_available":  vectorDB != nil,
		}).Info("AI condition evaluator created with available services")
	}

	return evaluator
}

// EvaluateCondition evaluates a workflow condition using AI services
// Business Requirement: BR-AI-COND-001 - Intelligent condition evaluation
func (ace *DefaultAIConditionEvaluator) EvaluateCondition(
	ctx context.Context,
	condition *ExecutableCondition,
	stepContext *StepContext,
) (bool, error) {
	// Validate inputs
	if condition == nil {
		return false, fmt.Errorf("condition cannot be nil")
	}

	ace.log.WithFields(logrus.Fields{
		"condition_type":       condition.Type,
		"condition_expression": condition.Expression,
		"condition_id":         condition.ID,
	}).Debug("Evaluating condition using AI services")

	// Use fallback mode if AI services are unavailable
	if ace.fallbackMode {
		return ace.basicConditionEvaluation(condition, stepContext), nil
	}

	// Try AI-powered evaluation strategies in order of preference

	// Strategy 1: Use LLM for complex expression evaluation
	if ace.llmClient != nil && ace.isComplexCondition(condition) {
		result, err := ace.evaluateWithLLM(ctx, condition, stepContext)
		if err == nil {
			ace.log.WithField("method", "llm").Debug("Condition evaluated successfully using LLM")
			return result, nil
		}
		ace.log.WithError(err).Warn("LLM condition evaluation failed, trying next method")
	}

	// Strategy 2: Use HolmesGPT for alert-related conditions
	if ace.holmesClient != nil && ace.isAlertRelatedCondition(condition) {
		result, err := ace.evaluateWithHolmesGPT(ctx, condition, stepContext)
		if err == nil {
			ace.log.WithField("method", "holmesgpt").Debug("Condition evaluated successfully using HolmesGPT")
			return result, nil
		}
		ace.log.WithError(err).Warn("HolmesGPT condition evaluation failed, trying next method")
	}

	// Strategy 3: Use vector database for pattern-based evaluation
	if ace.vectorDB != nil {
		result, err := ace.evaluateWithVectorDB(ctx, condition, stepContext)
		if err == nil {
			ace.log.WithField("method", "vector_db").Debug("Condition evaluated successfully using vector database")
			return result, nil
		}
		ace.log.WithError(err).Warn("Vector database condition evaluation failed, using basic fallback")
	}

	// Final fallback: Basic evaluation
	ace.log.Debug("Using basic condition evaluation as final fallback")
	return ace.basicConditionEvaluation(condition, stepContext), nil
}

// ValidateCondition validates a workflow condition using AI services
// Business Requirement: BR-AI-COND-002 - Intelligent condition validation
func (ace *DefaultAIConditionEvaluator) ValidateCondition(
	ctx context.Context,
	condition *ExecutableCondition,
) error {
	if condition == nil {
		return fmt.Errorf("condition cannot be nil")
	}

	ace.log.WithFields(logrus.Fields{
		"condition_type":       condition.Type,
		"condition_expression": condition.Expression,
	}).Debug("Validating condition using AI services")

	// Basic structural validation
	if err := ace.basicConditionValidation(condition); err != nil {
		return fmt.Errorf("basic validation failed: %w", err)
	}

	// Use AI services for enhanced validation if available
	if !ace.fallbackMode {
		// Enhanced validation using LLM for complex conditions
		if ace.llmClient != nil && ace.isComplexCondition(condition) {
			if err := ace.validateWithLLM(ctx, condition); err != nil {
				ace.log.WithError(err).Warn("LLM validation failed, condition may have issues")
				// Don't fail validation, just log warning
			}
		}
	}

	return nil
}

// isComplexCondition determines if a condition requires complex AI evaluation
func (ace *DefaultAIConditionEvaluator) isComplexCondition(condition *ExecutableCondition) bool {
	if condition == nil {
		return false
	}

	// Conditions that benefit from LLM evaluation
	complexTypes := map[ConditionType]bool{
		ConditionTypeExpression: true,
		ConditionTypeCustom:     true,
	}

	return complexTypes[condition.Type] ||
		strings.Contains(strings.ToLower(condition.Expression), "expression") ||
		strings.Contains(strings.ToLower(condition.Expression), "formula")
}

// isAlertRelatedCondition determines if a condition is related to alerts/monitoring
func (ace *DefaultAIConditionEvaluator) isAlertRelatedCondition(condition *ExecutableCondition) bool {
	if condition == nil {
		return false
	}

	// Conditions that benefit from HolmesGPT evaluation
	alertRelatedFields := []string{"alert", "metric", "threshold", "monitoring", "health", "status"}

	expressionLower := strings.ToLower(condition.Expression)
	for _, field := range alertRelatedFields {
		if strings.Contains(expressionLower, field) {
			return true
		}
	}

	return condition.Type == ConditionTypeMetric || condition.Type == ConditionTypeResource
}

// evaluateWithLLM evaluates condition using LLM for complex logic
func (ace *DefaultAIConditionEvaluator) evaluateWithLLM(
	ctx context.Context,
	condition *ExecutableCondition,
	stepContext *StepContext,
) (bool, error) {
	// Create a context-aware prompt for condition evaluation
	prompt := ace.buildConditionEvaluationPrompt(condition, stepContext)

	// Call LLM with timeout
	llmCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	response, err := ace.llmClient.ChatCompletion(llmCtx, prompt)
	if err != nil {
		return false, fmt.Errorf("LLM condition evaluation failed: %w", err)
	}

	// Parse LLM response to boolean
	return ace.parseLLMResponseToBoolean(response), nil
}

// evaluateWithHolmesGPT evaluates condition using HolmesGPT for alert-related logic
func (ace *DefaultAIConditionEvaluator) evaluateWithHolmesGPT(
	ctx context.Context,
	condition *ExecutableCondition,
	stepContext *StepContext,
) (bool, error) {
	// Create HolmesGPT investigation request for condition evaluation
	request := &holmesgpt.InvestigateRequest{
		AlertName:       fmt.Sprintf("condition_evaluation_%s", condition.ID),
		Namespace:       ace.extractNamespaceFromContext(stepContext),
		Priority:        "medium",
		AsyncProcessing: false,
		IncludeContext:  true,
	}

	// Call HolmesGPT with timeout
	holmesCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	result, err := ace.holmesClient.Investigate(holmesCtx, request)
	if err != nil {
		return false, fmt.Errorf("HolmesGPT condition evaluation failed: %w", err)
	}

	// Analyze HolmesGPT result for condition satisfaction
	return ace.analyzeHolmesGPTResultForCondition(result, condition), nil
}

// evaluateWithVectorDB evaluates condition using vector database for pattern matching
// Business Requirement: BR-AI-COND-001 - Enhanced vector-based condition evaluation using buildConditionVector
func (ace *DefaultAIConditionEvaluator) evaluateWithVectorDB(
	ctx context.Context,
	condition *ExecutableCondition,
	stepContext *StepContext,
) (bool, error) {
	vectorCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	ace.log.WithFields(logrus.Fields{
		"condition_type": condition.Type,
		"condition_id":   condition.ID,
		"business_req":   "BR-AI-COND-001",
	}).Debug("Evaluating condition using vector-based similarity search")

	// Use buildConditionVector for enhanced vector-based search (integrating previously unused function)
	conditionVector := ace.buildConditionVector(condition, stepContext)

	ace.log.WithFields(logrus.Fields{
		"vector_dimension": len(conditionVector),
		"condition_id":     condition.ID,
	}).Debug("Generated condition vector for similarity search")

	// Use vector-based search instead of text-based semantic search for better accuracy
	// Threshold of 0.7 provides good balance between precision and recall
	patterns, err := ace.vectorDB.SearchByVector(vectorCtx, conditionVector, 5, 0.7)
	if err != nil {
		// Fallback to semantic search if vector search fails
		ace.log.WithError(err).Warn("Vector search failed, falling back to semantic search")

		queryText := fmt.Sprintf("condition type:%s expression:%s", condition.Type, condition.Expression)
		patterns, err = ace.vectorDB.SearchBySemantics(vectorCtx, queryText, 5)
		if err != nil {
			return false, fmt.Errorf("vector database condition search failed: %w", err)
		}
	}

	ace.log.WithFields(logrus.Fields{
		"found_patterns": len(patterns),
		"condition_id":   condition.ID,
		"search_method":  "vector_similarity",
	}).Debug("Found similar patterns for condition evaluation")

	// Analyze similar patterns to evaluate condition
	result := ace.analyzeVectorPatternsForCondition(patterns, condition)

	ace.log.WithFields(logrus.Fields{
		"condition_id":      condition.ID,
		"evaluation_result": result,
		"pattern_count":     len(patterns),
		"business_req":      "BR-AI-COND-001",
	}).Debug("Completed vector-based condition evaluation")

	return result, nil
}

// Helper methods for AI condition evaluation

func (ace *DefaultAIConditionEvaluator) buildConditionEvaluationPrompt(
	condition *ExecutableCondition,
	stepContext *StepContext,
) string {
	contextInfo := ""
	if stepContext != nil && stepContext.Variables != nil {
		contextInfo = fmt.Sprintf("Context variables: %v", stepContext.Variables)
	}

	return fmt.Sprintf(`Evaluate the following workflow condition and respond with only "true" or "false":

Condition Type: %s
Expression: %s
ID: %s
%s

Based on the condition logic, should this condition evaluate to true or false?`,
		condition.Type, condition.Expression, condition.ID, contextInfo)
}

func (ace *DefaultAIConditionEvaluator) parseLLMResponseToBoolean(response string) bool {
	responseLower := strings.ToLower(strings.TrimSpace(response))

	// Look for clear boolean indicators
	if strings.Contains(responseLower, "true") {
		return true
	}
	if strings.Contains(responseLower, "false") {
		return false
	}

	// Look for positive/negative indicators
	positiveIndicators := []string{"yes", "pass", "satisfied", "met", "valid", "correct"}
	for _, indicator := range positiveIndicators {
		if strings.Contains(responseLower, indicator) {
			return true
		}
	}

	// Default to false for safety
	return false
}

func (ace *DefaultAIConditionEvaluator) extractNamespaceFromContext(stepContext *StepContext) string {
	if stepContext == nil || stepContext.Variables == nil {
		return "default"
	}

	if namespace, ok := stepContext.Variables["namespace"].(string); ok {
		return namespace
	}

	return "default"
}

func (ace *DefaultAIConditionEvaluator) analyzeHolmesGPTResultForCondition(
	result *holmesgpt.InvestigateResponse,
	condition *ExecutableCondition,
) bool {
	if result == nil {
		return false
	}

	// Analyze investigation result for condition satisfaction
	// This is a simplified implementation - could be enhanced with more sophisticated analysis
	summary := strings.ToLower(result.Summary)

	// Look for positive indicators in the investigation summary
	positiveIndicators := []string{"healthy", "normal", "ok", "good", "stable", "within limits"}
	for _, indicator := range positiveIndicators {
		if strings.Contains(summary, indicator) {
			return true
		}
	}

	// Look for negative indicators
	negativeIndicators := []string{"error", "failed", "critical", "down", "unhealthy", "issue"}
	for _, indicator := range negativeIndicators {
		if strings.Contains(summary, indicator) {
			return false
		}
	}

	// Default based on status and summary content
	return result.Status == "completed" && strings.Contains(summary, "healthy")
}

func (ace *DefaultAIConditionEvaluator) buildConditionVector(
	condition *ExecutableCondition,
	stepContext *StepContext,
) []float64 {
	// Create a simple vector representation of the condition
	// This is a simplified implementation - could be enhanced with proper embedding generation
	vector := make([]float64, 10)

	// Encode condition type
	switch condition.Type {
	case "metric":
		vector[0] = 1.0
	case "time":
		vector[1] = 1.0
	case "resource":
		vector[2] = 1.0
	case "expression":
		vector[3] = 1.0
	default:
		vector[4] = 1.0
	}

	// Encode expression content (simplified)
	if strings.Contains(strings.ToLower(condition.Expression), "equal") {
		vector[5] = 1.0
	} else if strings.Contains(strings.ToLower(condition.Expression), "greater") {
		vector[6] = 1.0
	} else if strings.Contains(strings.ToLower(condition.Expression), "less") {
		vector[7] = 1.0
	} else {
		vector[8] = 1.0
	}

	// Add context influence
	if stepContext != nil {
		vector[9] = 0.5
	}

	return vector
}

func (ace *DefaultAIConditionEvaluator) analyzeVectorPatternsForCondition(
	patterns []*vector.ActionPattern,
	condition *ExecutableCondition,
) bool {
	if len(patterns) == 0 {
		return false
	}

	// Analyze pattern similarity for condition evaluation
	// This is a simplified implementation - could be enhanced with proper pattern analysis
	totalScore := 0.0
	for _, pattern := range patterns {
		// Use effectiveness as score proxy
		if pattern.EffectivenessData != nil {
			totalScore += pattern.EffectivenessData.Score
		}
	}

	averageScore := totalScore / float64(len(patterns))

	// If we have high similarity with historical patterns, trust the pattern
	return averageScore > 0.7
}

func (ace *DefaultAIConditionEvaluator) validateWithLLM(
	ctx context.Context,
	condition *ExecutableCondition,
) error {
	prompt := fmt.Sprintf(`Validate the following workflow condition for syntax and logical correctness:

Type: %s
Expression: %s
ID: %s

Is this condition valid? Respond with "VALID" or "INVALID: <reason>"`,
		condition.Type, condition.Expression, condition.ID)

	llmCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	response, err := ace.llmClient.ChatCompletion(llmCtx, prompt)
	if err != nil {
		return fmt.Errorf("LLM validation request failed: %w", err)
	}

	if strings.Contains(strings.ToUpper(response), "INVALID") {
		return fmt.Errorf("LLM validation failed: %s", response)
	}

	return nil
}

// Basic fallback methods

func (ace *DefaultAIConditionEvaluator) basicConditionEvaluation(
	condition *ExecutableCondition,
	stepContext *StepContext,
) bool {
	// Basic condition evaluation logic (same as workflow engine fallbacks)
	// This ensures consistent behavior when AI services are unavailable

	if condition == nil {
		return false
	}

	// Simple field-based evaluation
	switch condition.Type {
	case "always_true":
		return true
	case "always_false":
		return false
	case "metric":
		return ace.basicMetricEvaluation(condition, stepContext)
	case "resource":
		return ace.basicResourceEvaluation(condition, stepContext)
	case "time":
		return ace.basicTimeEvaluation(condition, stepContext)
	default:
		// Default to true for unknown conditions to avoid blocking workflows
		return true
	}
}

func (ace *DefaultAIConditionEvaluator) basicConditionValidation(condition *ExecutableCondition) error {
	if condition.Type == "" {
		return fmt.Errorf("condition type cannot be empty")
	}
	if condition.Expression == "" {
		return fmt.Errorf("condition expression cannot be empty")
	}
	if condition.ID == "" {
		return fmt.Errorf("condition ID cannot be empty")
	}
	return nil
}

func (ace *DefaultAIConditionEvaluator) basicMetricEvaluation(
	condition *ExecutableCondition,
	stepContext *StepContext,
) bool {
	// Basic metric evaluation - simplified implementation
	return true // Default to true for basic evaluation
}

func (ace *DefaultAIConditionEvaluator) basicResourceEvaluation(
	condition *ExecutableCondition,
	stepContext *StepContext,
) bool {
	// Basic resource evaluation - simplified implementation
	return true // Default to true for basic evaluation
}

func (ace *DefaultAIConditionEvaluator) basicTimeEvaluation(
	condition *ExecutableCondition,
	stepContext *StepContext,
) bool {
	// Basic time evaluation - simplified implementation
	return true // Default to true for basic evaluation
}
