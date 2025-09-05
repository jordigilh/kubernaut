package conditions

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// Response parsing methods

func (ace *DefaultAIConditionEvaluator) parseConditionResponse(recommendation *types.ActionRecommendation, condition *engine.WorkflowCondition) (*ConditionResult, error) {
	// Extract JSON from reasoning
	reasoningText := ""
	if recommendation.Reasoning != nil {
		reasoningText = recommendation.Reasoning.Summary
	}

	jsonData := ace.extractJSONFromResponse(reasoningText)
	if jsonData == "" {
		return nil, fmt.Errorf("no valid JSON found in AI response")
	}

	var aiResponse ConditionAnalysisResponse
	if err := json.Unmarshal([]byte(jsonData), &aiResponse); err != nil {
		return nil, fmt.Errorf("failed to parse AI condition response: %w", err)
	}

	// Validate confidence threshold
	if aiResponse.Confidence < ace.config.ConfidenceThreshold {
		return nil, fmt.Errorf("AI confidence %.3f below threshold %.3f", aiResponse.Confidence, ace.config.ConfidenceThreshold)
	}

	// Convert to ConditionResult
	result := &ConditionResult{
		Satisfied:   aiResponse.Satisfied,
		Confidence:  aiResponse.Confidence,
		Reasoning:   aiResponse.Reasoning,
		Metadata:    aiResponse.Metadata,
		NextActions: aiResponse.Recommendations,
		Warnings:    aiResponse.Warnings,
		EvaluatedAt: time.Now(),
	}

	// Add detailed analysis to metadata if available
	if result.Metadata == nil {
		result.Metadata = make(map[string]interface{})
	}
	if aiResponse.DetailedAnalysis != nil {
		result.Metadata["detailed_analysis"] = aiResponse.DetailedAnalysis
	}
	if len(aiResponse.AlternativeActions) > 0 {
		result.Metadata["alternative_actions"] = aiResponse.AlternativeActions
	}

	return result, nil
}

func (ace *DefaultAIConditionEvaluator) extractJSONFromResponse(text string) string {
	// Find JSON blocks in the response
	start := strings.Index(text, "{")
	if start == -1 {
		return ""
	}

	// Find the matching closing bracket
	depth := 0
	for i, char := range text[start:] {
		if char == '{' {
			depth++
		} else if char == '}' {
			depth--
			if depth == 0 {
				return text[start : start+i+1]
			}
		}
	}

	return ""
}

// Fallback evaluation methods for when AI is unavailable

func (ace *DefaultAIConditionEvaluator) fallbackMetricEvaluation(condition *engine.WorkflowCondition, stepContext *engine.StepContext) *ConditionResult {
	// Basic metric condition evaluation without AI
	satisfied := true // Conservative default
	confidence := 0.6 // Lower confidence for fallback
	reasoning := "Basic metric evaluation (AI unavailable): "

	// Simple heuristics based on expression content
	expr := strings.ToLower(condition.Expression)
	if strings.Contains(expr, "error") || strings.Contains(expr, "fail") {
		satisfied = false
		reasoning += "Expression contains error/failure indicators"
	} else if strings.Contains(expr, "success") || strings.Contains(expr, "healthy") {
		satisfied = true
		reasoning += "Expression contains success/health indicators"
	} else if strings.Contains(expr, ">") || strings.Contains(expr, "<") {
		// For comparison expressions, default to satisfied with warning
		satisfied = true
		reasoning += "Comparison expression evaluated with default logic"
	} else {
		reasoning += "Generic condition evaluation"
	}

	return &ConditionResult{
		Satisfied:   satisfied,
		Confidence:  confidence,
		Reasoning:   reasoning,
		Metadata:    map[string]interface{}{"fallback": true, "method": "basic_metric", "execution_id": stepContext.ExecutionID, "step_id": stepContext.StepID},
		Warnings:    []string{"AI evaluation unavailable - using basic fallback logic"},
		EvaluatedAt: time.Now(),
	}
}

func (ace *DefaultAIConditionEvaluator) fallbackResourceEvaluation(condition *engine.WorkflowCondition, stepContext *engine.StepContext) *ConditionResult {
	// Basic resource condition evaluation without AI
	satisfied := true
	confidence := 0.5
	reasoning := "Basic resource evaluation (AI unavailable): "

	expr := strings.ToLower(condition.Expression)

	// Check if we can get basic resource information
	if ace.k8sClient != nil {
		ctx := context.Background()

		// Try to get basic cluster health
		nodes, err := ace.k8sClient.ListNodes(ctx)
		if err != nil {
			satisfied = false
			reasoning += "Unable to access cluster nodes"
		} else if nodes != nil {
			reasoning += fmt.Sprintf("Cluster accessible with %d nodes", len(nodes.Items))
		}
	} else {
		satisfied = false
		reasoning += "No Kubernetes client available"
	}

	// Simple expression analysis
	if strings.Contains(expr, "ready") || strings.Contains(expr, "available") {
		if satisfied {
			reasoning += ", condition likely satisfied based on cluster accessibility"
		}
	} else if strings.Contains(expr, "failed") || strings.Contains(expr, "error") {
		satisfied = !satisfied // Invert for failure conditions
		reasoning += ", inverted logic for failure condition"
	}

	return &ConditionResult{
		Satisfied:   satisfied,
		Confidence:  confidence,
		Reasoning:   reasoning,
		Metadata:    map[string]interface{}{"fallback": true, "method": "basic_resource"},
		Warnings:    []string{"AI evaluation unavailable - using basic resource check"},
		EvaluatedAt: time.Now(),
	}
}

func (ace *DefaultAIConditionEvaluator) fallbackTimeEvaluation(condition *engine.WorkflowCondition, stepContext *engine.StepContext) *ConditionResult {
	// Basic time condition evaluation without AI
	satisfied := true
	confidence := 0.8 // Time evaluation can be more deterministic
	reasoning := "Basic time evaluation (AI unavailable): "

	now := time.Now()
	expr := strings.ToLower(condition.Expression)

	// Handle common time-based patterns
	if strings.Contains(expr, "business_hours") {
		hour := now.Hour()
		satisfied = hour >= 9 && hour <= 17 // 9 AM to 5 PM
		reasoning += fmt.Sprintf("Business hours check: current hour %d", hour)
	} else if strings.Contains(expr, "weekend") {
		weekday := now.Weekday()
		satisfied = weekday == time.Saturday || weekday == time.Sunday
		reasoning += fmt.Sprintf("Weekend check: current day %s", weekday.String())
	} else if strings.Contains(expr, "timeout") {
		// Check if step has exceeded timeout
		if condition.Timeout > 0 {
			// Without StartTime tracking, we can't check actual elapsed time
			satisfied = true // Default to not timed out
			reasoning += fmt.Sprintf("Timeout check: timeout configured as %s, assuming not exceeded", condition.Timeout)
		} else {
			reasoning += "Timeout check: no timeout configured"
		}
	} else if strings.Contains(expr, "after") || strings.Contains(expr, "before") {
		// Simple before/after logic would require more parsing
		reasoning += "Time comparison evaluation (simplified)"
	} else {
		reasoning += "Generic time condition evaluation"
	}

	return &ConditionResult{
		Satisfied:   satisfied,
		Confidence:  confidence,
		Reasoning:   reasoning,
		Metadata:    map[string]interface{}{"fallback": true, "method": "basic_time", "evaluation_time": now.Format(time.RFC3339)},
		Warnings:    []string{"AI evaluation unavailable - using basic time logic"},
		EvaluatedAt: time.Now(),
	}
}

func (ace *DefaultAIConditionEvaluator) fallbackExpressionEvaluation(condition *engine.WorkflowCondition, stepContext *engine.StepContext) *ConditionResult {
	// Basic expression evaluation without AI
	satisfied := false
	confidence := 0.4 // Low confidence for expression parsing
	reasoning := "Basic expression evaluation (AI unavailable): "

	expr := condition.Expression
	if expr == "" {
		satisfied = false
		reasoning += "Empty expression"
	} else {
		// Very basic expression evaluation
		exprLower := strings.ToLower(expr)

		// Simple boolean checks
		if exprLower == "true" || strings.Contains(exprLower, "true") {
			satisfied = true
			reasoning += "Expression contains 'true'"
		} else if exprLower == "false" || strings.Contains(exprLower, "false") {
			satisfied = false
			reasoning += "Expression contains 'false'"
		} else if strings.Contains(exprLower, "&&") || strings.Contains(exprLower, "and") {
			// For AND expressions, be conservative
			satisfied = false
			reasoning += "AND expression evaluated conservatively"
		} else if strings.Contains(exprLower, "||") || strings.Contains(exprLower, "or") {
			// For OR expressions, be optimistic
			satisfied = true
			reasoning += "OR expression evaluated optimistically"
		} else {
			// Default behavior for unknown expressions
			satisfied = true
			confidence = 0.3
			reasoning += "Unknown expression format, defaulting to satisfied"
		}
	}

	return &ConditionResult{
		Satisfied:   satisfied,
		Confidence:  confidence,
		Reasoning:   reasoning,
		Metadata:    map[string]interface{}{"fallback": true, "method": "basic_expression", "original_expression": expr},
		Warnings:    []string{"AI evaluation unavailable - using basic expression parsing", "Expression evaluation may be inaccurate"},
		EvaluatedAt: time.Now(),
	}
}

func (ace *DefaultAIConditionEvaluator) fallbackCustomEvaluation(condition *engine.WorkflowCondition, stepContext *engine.StepContext) *ConditionResult {
	// Basic custom condition evaluation without AI
	var satisfied bool
	var confidence float64
	reasoning := "Basic custom evaluation (AI unavailable): "

	// Combine insights from other fallback methods
	metricResult := ace.fallbackMetricEvaluation(condition, stepContext)
	resourceResult := ace.fallbackResourceEvaluation(condition, stepContext)
	timeResult := ace.fallbackTimeEvaluation(condition, stepContext)

	// Simple consensus logic
	satisfiedCount := 0
	totalChecks := 3

	if metricResult.Satisfied {
		satisfiedCount++
	}
	if resourceResult.Satisfied {
		satisfiedCount++
	}
	if timeResult.Satisfied {
		satisfiedCount++
	}

	satisfied = satisfiedCount >= 2 // Majority rule
	confidence = float64(satisfiedCount) / float64(totalChecks)

	reasoning += fmt.Sprintf("Consensus evaluation: %d/%d checks satisfied", satisfiedCount, totalChecks)

	warnings := []string{
		"AI evaluation unavailable - using fallback consensus method",
		"Custom condition evaluation may be less accurate",
	}

	return &ConditionResult{
		Satisfied:  satisfied,
		Confidence: confidence,
		Reasoning:  reasoning,
		Metadata: map[string]interface{}{
			"fallback":        true,
			"method":          "consensus_custom",
			"metric_result":   metricResult.Satisfied,
			"resource_result": resourceResult.Satisfied,
			"time_result":     timeResult.Satisfied,
		},
		Warnings:    warnings,
		EvaluatedAt: time.Now(),
	}
}
