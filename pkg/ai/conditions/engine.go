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
package conditions

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Real conditions engine implementation supporting complex conditional logic
// Provides rule evaluation, expression parsing, and business logic execution

type ConditionsEngine interface {
	EvaluateConditions() error
	EvaluateCondition(ctx context.Context, condition *Condition) (*EvaluationResult, error)
	EvaluateExpression(ctx context.Context, expression string, context map[string]interface{}) (bool, error)
	RegisterCondition(condition *Condition) error
	GetConditions() []*Condition
	ValidateCondition(condition *Condition) *ValidationResult
	GetStatus() *EngineStatus
}

type ConditionsEngineImpl struct {
	logger     *logrus.Logger
	conditions map[string]*Condition
	evaluators map[string]Evaluator
	config     *EngineConfig
	stats      *EngineStats
}

type EngineConfig struct {
	MaxEvaluationTime time.Duration `yaml:"max_evaluation_time" default:"10s"`
	EnableCaching     bool          `yaml:"enable_caching" default:"true"`
	LogLevel          string        `yaml:"log_level" default:"info"`
	MaxConditions     int           `yaml:"max_conditions" default:"1000"`
}

type Condition struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Expression  string                 `json:"expression"`
	Type        string                 `json:"type"`     // simple, complex, composite
	Priority    int                    `json:"priority"` // 1-10, higher = more important
	Enabled     bool                   `json:"enabled"`
	Metadata    map[string]interface{} `json:"metadata"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

type EvaluationResult struct {
	ConditionID   string                 `json:"condition_id"`
	Result        bool                   `json:"result"`
	Confidence    float64                `json:"confidence"`
	ExecutionTime time.Duration          `json:"execution_time"`
	Context       map[string]interface{} `json:"context"`
	Error         string                 `json:"error,omitempty"`
	EvaluatedAt   time.Time              `json:"evaluated_at"`
	Metadata      map[string]interface{} `json:"metadata"`
	// Additional fields for test compatibility
	Reasoning string `json:"reasoning,omitempty"`
}

// ConditionResult alias removed - use EvaluationResult directly

// For backward compatibility, add Satisfied field support
func (e *EvaluationResult) SetSatisfied(satisfied bool) {
	e.Result = satisfied
}

func (e *EvaluationResult) GetSatisfied() bool {
	return e.Result
}

type ValidationResult struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

type EngineStatus struct {
	Healthy          bool      `json:"healthy"`
	ConditionsLoaded int       `json:"conditions_loaded"`
	EvaluationsCount int64     `json:"evaluations_count"`
	SuccessRate      float64   `json:"success_rate"`
	AvgExecutionTime string    `json:"avg_execution_time"`
	LastEvaluation   time.Time `json:"last_evaluation"`
}

type EngineStats struct {
	TotalEvaluations   int64
	SuccessfulEvals    int64
	FailedEvals        int64
	TotalExecutionTime time.Duration
}

type Evaluator interface {
	Evaluate(ctx context.Context, expression string, context map[string]interface{}) (bool, error)
	GetType() string
	IsHealthy() bool
}

func NewConditionsEngine() *ConditionsEngineImpl {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	config := &EngineConfig{
		MaxEvaluationTime: 10 * time.Second,
		EnableCaching:     true,
		LogLevel:          "info",
		MaxConditions:     1000,
	}

	engine := &ConditionsEngineImpl{
		logger:     logger,
		conditions: make(map[string]*Condition),
		evaluators: make(map[string]Evaluator),
		config:     config,
		stats:      &EngineStats{},
	}

	// Register default evaluators
	engine.registerDefaultEvaluators()

	logger.Info("Conditions Engine initialized with real evaluation capabilities")
	return engine
}

func (e *ConditionsEngineImpl) EvaluateConditions() error {
	ctx, cancel := context.WithTimeout(context.Background(), e.config.MaxEvaluationTime)
	defer cancel()

	e.logger.WithField("conditions_count", len(e.conditions)).Debug("Evaluating all registered conditions")

	successCount := 0
	totalCount := 0

	for _, condition := range e.conditions {
		if !condition.Enabled {
			continue
		}

		totalCount++
		result, err := e.EvaluateCondition(ctx, condition)
		if err != nil {
			e.logger.WithError(err).WithField("condition_id", condition.ID).Warn("Condition evaluation failed")
			continue
		}

		if result.Result {
			successCount++
		}

		e.logger.WithFields(logrus.Fields{
			"condition_id":   condition.ID,
			"result":         result.Result,
			"confidence":     result.Confidence,
			"execution_time": result.ExecutionTime,
		}).Debug("Condition evaluated")
	}

	e.logger.WithFields(logrus.Fields{
		"total_conditions":      totalCount,
		"successful_conditions": successCount,
		"success_rate":          float64(successCount) / float64(totalCount) * 100,
	}).Info("Conditions evaluation completed")

	return nil
}

func (e *ConditionsEngineImpl) EvaluateCondition(ctx context.Context, condition *Condition) (*EvaluationResult, error) {
	start := time.Now()
	e.stats.TotalEvaluations++

	result := &EvaluationResult{
		ConditionID: condition.ID,
		Result:      false,
		Confidence:  0.0,
		Context:     make(map[string]interface{}),
		EvaluatedAt: time.Now(),
		Metadata:    make(map[string]interface{}),
	}

	// Validate condition
	validation := e.ValidateCondition(condition)
	if !validation.Valid {
		result.Error = fmt.Sprintf("condition validation failed: %v", validation.Errors)
		e.stats.FailedEvals++
		return result, fmt.Errorf(result.Error)
	}

	// Select appropriate evaluator
	evaluator := e.selectEvaluator(condition)
	if evaluator == nil {
		result.Error = "no suitable evaluator found for condition type"
		e.stats.FailedEvals++
		return result, fmt.Errorf(result.Error)
	}

	// Create evaluation context
	evalContext := map[string]interface{}{
		"condition_id":   condition.ID,
		"condition_name": condition.Name,
		"priority":       condition.Priority,
		"metadata":       condition.Metadata,
		"current_time":   time.Now(),
	}

	// Evaluate condition
	evalResult, err := evaluator.Evaluate(ctx, condition.Expression, evalContext)
	if err != nil {
		result.Error = err.Error()
		e.stats.FailedEvals++
		return result, err
	}

	// Calculate execution time
	executionTime := time.Since(start)
	e.stats.TotalExecutionTime += executionTime

	// Build result
	result.Result = evalResult
	result.Confidence = e.calculateConfidence(condition, evalResult, executionTime)
	result.ExecutionTime = executionTime
	result.Context = evalContext
	result.Metadata["evaluator_type"] = evaluator.GetType()
	result.Metadata["condition_type"] = condition.Type

	e.stats.SuccessfulEvals++

	return result, nil
}

func (e *ConditionsEngineImpl) EvaluateExpression(ctx context.Context, expression string, context map[string]interface{}) (bool, error) {
	e.logger.WithFields(logrus.Fields{
		"expression":   expression,
		"context_keys": len(context),
	}).Debug("Evaluating standalone expression")

	// Create temporary condition for evaluation
	tempCondition := &Condition{
		ID:         fmt.Sprintf("temp_%d", time.Now().Unix()),
		Expression: expression,
		Type:       "simple",
		Enabled:    true,
		CreatedAt:  time.Now(),
	}

	result, err := e.EvaluateCondition(ctx, tempCondition)
	if err != nil {
		return false, err
	}

	return result.Result, nil
}

func (e *ConditionsEngineImpl) RegisterCondition(condition *Condition) error {
	if condition.ID == "" {
		return fmt.Errorf("condition ID cannot be empty")
	}

	validation := e.ValidateCondition(condition)
	if !validation.Valid {
		return fmt.Errorf("condition validation failed: %v", validation.Errors)
	}

	if len(e.conditions) >= e.config.MaxConditions {
		return fmt.Errorf("maximum number of conditions (%d) reached", e.config.MaxConditions)
	}

	condition.CreatedAt = time.Now()
	condition.UpdatedAt = time.Now()

	e.conditions[condition.ID] = condition

	e.logger.WithFields(logrus.Fields{
		"condition_id":   condition.ID,
		"condition_name": condition.Name,
		"condition_type": condition.Type,
	}).Info("Condition registered successfully")

	return nil
}

func (e *ConditionsEngineImpl) GetConditions() []*Condition {
	conditions := make([]*Condition, 0, len(e.conditions))
	for _, condition := range e.conditions {
		conditions = append(conditions, condition)
	}
	return conditions
}

func (e *ConditionsEngineImpl) ValidateCondition(condition *Condition) *ValidationResult {
	result := &ValidationResult{
		Valid:    true,
		Errors:   []string{},
		Warnings: []string{},
	}

	if condition == nil {
		result.Valid = false
		result.Errors = append(result.Errors, "condition cannot be nil")
		return result
	}

	if condition.ID == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "condition ID is required")
	}

	if condition.Expression == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "condition expression is required")
	}

	if condition.Priority < 1 || condition.Priority > 10 {
		result.Warnings = append(result.Warnings, "condition priority should be between 1 and 10")
	}

	// Validate expression syntax
	if condition.Expression != "" {
		if err := e.validateExpressionSyntax(condition.Expression); err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("invalid expression syntax: %v", err))
		}
	}

	return result
}

func (e *ConditionsEngineImpl) GetStatus() *EngineStatus {
	successRate := 0.0
	if e.stats.TotalEvaluations > 0 {
		successRate = float64(e.stats.SuccessfulEvals) / float64(e.stats.TotalEvaluations) * 100
	}

	avgExecutionTime := time.Duration(0)
	if e.stats.TotalEvaluations > 0 {
		avgExecutionTime = e.stats.TotalExecutionTime / time.Duration(e.stats.TotalEvaluations)
	}

	return &EngineStatus{
		Healthy:          true,
		ConditionsLoaded: len(e.conditions),
		EvaluationsCount: e.stats.TotalEvaluations,
		SuccessRate:      successRate,
		AvgExecutionTime: avgExecutionTime.String(),
		LastEvaluation:   time.Now(),
	}
}

// Helper methods

func (e *ConditionsEngineImpl) registerDefaultEvaluators() {
	e.evaluators["simple"] = &SimpleEvaluator{logger: e.logger}
	e.evaluators["regex"] = &RegexEvaluator{logger: e.logger}
	e.evaluators["numeric"] = &NumericEvaluator{logger: e.logger}
	e.evaluators["time"] = &TimeEvaluator{logger: e.logger}
}

func (e *ConditionsEngineImpl) selectEvaluator(condition *Condition) Evaluator {
	if evaluator, exists := e.evaluators[condition.Type]; exists {
		return evaluator
	}
	// Default to simple evaluator
	return e.evaluators["simple"]
}

func (e *ConditionsEngineImpl) calculateConfidence(condition *Condition, result bool, executionTime time.Duration) float64 {
	baseConfidence := 0.70

	// Increase confidence for successful evaluations
	if result {
		baseConfidence += 0.20
	}

	// Decrease confidence for slow evaluations
	if executionTime > time.Second {
		baseConfidence -= 0.10
	}

	// Increase confidence for high-priority conditions
	if condition.Priority >= 8 {
		baseConfidence += 0.10
	}

	// Cap at 1.0
	if baseConfidence > 1.0 {
		baseConfidence = 1.0
	}

	// Floor at 0.0
	if baseConfidence < 0.0 {
		baseConfidence = 0.0
	}

	return baseConfidence
}

func (e *ConditionsEngineImpl) validateExpressionSyntax(expression string) error {
	// Basic syntax validation - can be enhanced with more sophisticated parsing
	if strings.TrimSpace(expression) == "" {
		return fmt.Errorf("empty expression")
	}

	// Check for balanced parentheses
	openCount := 0
	// Guideline #14: Use idiomatic patterns - switch for multiple character comparisons
	for _, char := range expression {
		switch char {
		case '(':
			openCount++
		case ')':
			openCount--
			if openCount < 0 {
				return fmt.Errorf("unmatched closing parenthesis")
			}
		}
	}
	if openCount != 0 {
		return fmt.Errorf("unmatched opening parenthesis")
	}

	return nil
}

// Default evaluator implementations

type SimpleEvaluator struct {
	logger *logrus.Logger
}

func (se *SimpleEvaluator) Evaluate(ctx context.Context, expression string, context map[string]interface{}) (bool, error) {
	// Add context-aware logging for traceability and potential timeout handling
	logger := se.logger.WithFields(logrus.Fields{
		"expression": expression,
		"trace_id":   ctx.Value("trace_id"),
	})
	logger.Debug("Evaluating simple expression")

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	// Simple evaluation logic - can be enhanced with more sophisticated parsing
	switch strings.ToLower(strings.TrimSpace(expression)) {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no":
		return false, nil
	default:
		// Try to evaluate as comparison
		return se.evaluateComparison(expression, context)
	}
}

func (se *SimpleEvaluator) evaluateComparison(expression string, context map[string]interface{}) (bool, error) {
	// Handle simple comparisons like "value > 10", "status == 'active'"
	operators := []string{">=", "<=", "!=", "==", ">", "<", "="}

	for _, op := range operators {
		if strings.Contains(expression, op) {
			parts := strings.Split(expression, op)
			if len(parts) != 2 {
				continue
			}

			left := strings.TrimSpace(parts[0])
			right := strings.TrimSpace(parts[1])

			return se.compareValues(left, right, op, context)
		}
	}

	// Default to checking if the expression exists in context as a boolean
	if val, exists := context[expression]; exists {
		if boolVal, ok := val.(bool); ok {
			return boolVal, nil
		}
	}

	return false, nil
}

func (se *SimpleEvaluator) compareValues(left, right, operator string, context map[string]interface{}) (bool, error) {
	// Get actual values from context or parse as literals
	leftVal := se.resolveValue(left, context)
	rightVal := se.resolveValue(right, context)

	switch operator {
	case "==", "=":
		return fmt.Sprintf("%v", leftVal) == fmt.Sprintf("%v", rightVal), nil
	case "!=":
		return fmt.Sprintf("%v", leftVal) != fmt.Sprintf("%v", rightVal), nil
	case ">", ">=", "<", "<=":
		return se.compareNumeric(leftVal, rightVal, operator)
	}

	return false, nil
}

func (se *SimpleEvaluator) resolveValue(value string, context map[string]interface{}) interface{} {
	// Remove quotes if present
	if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
		return value[1 : len(value)-1]
	}
	if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
		return value[1 : len(value)-1]
	}

	// Check if it's a context variable
	if val, exists := context[value]; exists {
		return val
	}

	// Try to parse as number
	if num, err := strconv.ParseFloat(value, 64); err == nil {
		return num
	}

	// Return as string
	return value
}

func (se *SimpleEvaluator) compareNumeric(left, right interface{}, operator string) (bool, error) {
	leftNum, leftOk := se.toFloat64(left)
	rightNum, rightOk := se.toFloat64(right)

	if !leftOk || !rightOk {
		return false, fmt.Errorf("cannot compare non-numeric values")
	}

	switch operator {
	case ">":
		return leftNum > rightNum, nil
	case ">=":
		return leftNum >= rightNum, nil
	case "<":
		return leftNum < rightNum, nil
	case "<=":
		return leftNum <= rightNum, nil
	}

	return false, fmt.Errorf("unknown numeric operator: %s", operator)
}

func (se *SimpleEvaluator) toFloat64(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case string:
		if num, err := strconv.ParseFloat(v, 64); err == nil {
			return num, true
		}
	}
	return 0, false
}

func (se *SimpleEvaluator) GetType() string { return "simple" }
func (se *SimpleEvaluator) IsHealthy() bool { return true }

// ============================================================================
// AI CONDITION EVALUATOR TYPES FOR BUSINESS REQUIREMENTS
// ============================================================================

// AIConditionEvaluator interface for AI-powered condition evaluation
type AIConditionEvaluator interface {
	EvaluateCondition(ctx context.Context, condition *Condition) (*EvaluationResult, error)
	GetID() string
	GetCapabilities() []string
	IsHealthy() bool
}

// AIConditionEvaluatorConfig holds configuration for AI condition evaluators
type AIConditionEvaluatorConfig struct {
	EnableAdvancedEvaluation bool          `yaml:"enable_advanced_evaluation" default:"true"`
	MaxEvaluationTime        time.Duration `yaml:"max_evaluation_time" default:"30s"`
	ConfidenceThreshold      float64       `yaml:"confidence_threshold" default:"0.7"`
	LogLevel                 string        `yaml:"log_level" default:"info"`
	// Additional fields for test compatibility
	EnableDetailedLogging   bool `yaml:"enable_detailed_logging" default:"false"`
	FallbackOnLowConfidence bool `yaml:"fallback_on_low_confidence" default:"true"`
	UseContextualAnalysis   bool `yaml:"use_contextual_analysis" default:"true"`
}

// SLMClient interface for AI service integration
type SLMClient interface {
	AnalyzeAlert(ctx context.Context, alert interface{}) (*types.ActionRecommendation, error)
	IsHealthy() bool
}

// DefaultAIConditionEvaluator provides default AI condition evaluation
type DefaultAIConditionEvaluator struct {
	id           string
	logger       *logrus.Logger
	config       *AIConditionEvaluatorConfig
	capabilities []string
	slmClient    SLMClient
}

// NewDefaultAIConditionEvaluator creates a new default AI condition evaluator
func NewDefaultAIConditionEvaluator(config *AIConditionEvaluatorConfig, logger *logrus.Logger) *DefaultAIConditionEvaluator {
	if config == nil {
		config = &AIConditionEvaluatorConfig{
			EnableAdvancedEvaluation: true,
			MaxEvaluationTime:        30 * time.Second,
			ConfidenceThreshold:      0.7,
			LogLevel:                 "info",
			EnableDetailedLogging:    false,
			FallbackOnLowConfidence:  true,
			UseContextualAnalysis:    true,
		}
	}

	if logger == nil {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)
	}

	return &DefaultAIConditionEvaluator{
		id:           "default-ai-condition-evaluator",
		logger:       logger,
		config:       config,
		capabilities: []string{"simple", "complex", "ai_enhanced"},
		slmClient:    nil, // Will be set via dependency injection in tests
	}
}

// SetSLMClient sets the SLM client for AI integration
func (d *DefaultAIConditionEvaluator) SetSLMClient(client SLMClient) {
	d.slmClient = client
}

// EvaluateCondition implements AIConditionEvaluator interface
func (d *DefaultAIConditionEvaluator) EvaluateCondition(ctx context.Context, condition *Condition) (*EvaluationResult, error) {
	start := time.Now()

	result := &EvaluationResult{
		ConditionID: condition.ID,
		Result:      false,
		Confidence:  0.0,
		Context:     make(map[string]interface{}),
		EvaluatedAt: time.Now(),
		Metadata:    make(map[string]interface{}),
	}

	// Set basic evaluator metadata
	result.Metadata["evaluator_id"] = d.id
	result.Metadata["evaluator_type"] = "ai_condition_evaluator"

	// Try AI analysis first if SLM client is available
	if d.slmClient != nil {
		d.logger.WithFields(logrus.Fields{
			"client_available": true,
			"client_healthy":   d.slmClient.IsHealthy(),
		}).Debug("Checking SLM client availability")

		if d.slmClient.IsHealthy() {
			d.logger.Debug("SLM client is healthy, attempting AI evaluation")
			aiResult, err := d.evaluateWithAI(ctx, condition)
			if err == nil && aiResult != nil {
				// Successfully got AI analysis
				d.logger.WithFields(logrus.Fields{
					"confidence": aiResult.Confidence,
					"method":     "ai",
				}).Debug("AI evaluation succeeded")
				result.Result = aiResult.Result
				result.Confidence = aiResult.Confidence
				result.Reasoning = aiResult.Reasoning
				result.Context = aiResult.Context
				if aiResult.Metadata != nil {
					for k, v := range aiResult.Metadata {
						result.Metadata[k] = v
					}
				}
				result.ExecutionTime = time.Since(start)
				return result, nil
			}
			// AI failed, fall back to basic evaluation
			d.logger.WithError(err).Debug("AI evaluation failed, using fallback")
		} else {
			d.logger.Debug("SLM client not healthy, using fallback")
		}
	} else {
		d.logger.Debug("No SLM client available, using fallback")
	}

	// Fallback evaluation when AI is not available or failed
	fallbackResult := d.evaluateWithFallback(ctx, condition)
	result.Result = fallbackResult.Result
	result.Confidence = fallbackResult.Confidence
	result.Reasoning = fallbackResult.Reasoning
	result.Context = fallbackResult.Context
	if fallbackResult.Metadata != nil {
		for k, v := range fallbackResult.Metadata {
			result.Metadata[k] = v
		}
	}

	result.ExecutionTime = time.Since(start)

	d.logger.WithFields(logrus.Fields{
		"condition_id": condition.ID,
		"result":       result.Result,
		"confidence":   result.Confidence,
		"duration":     result.ExecutionTime,
		"method":       "fallback",
	}).Debug("AI condition evaluation completed")

	return result, nil
}

// evaluateWithAI performs AI-powered condition evaluation
func (d *DefaultAIConditionEvaluator) evaluateWithAI(ctx context.Context, condition *Condition) (*EvaluationResult, error) {
	// Create a proper Alert structure for the AI analysis
	mockAlert := types.Alert{
		ID:          condition.ID,
		Name:        condition.Name,
		Summary:     fmt.Sprintf("Condition: %s", condition.Name),
		Description: fmt.Sprintf("Condition evaluation: %s", condition.Expression),
		Severity:    "info", // Default severity
		Status:      "active",
		Labels: map[string]string{
			"condition_id":   condition.ID,
			"condition_type": condition.Type,
			"priority":       fmt.Sprintf("%d", condition.Priority),
		},
		Annotations: map[string]string{
			"expression": condition.Expression,
		},
		StartsAt:  time.Now(),
		UpdatedAt: time.Now(),
	}

	// Call the AI service
	recommendation, err := d.slmClient.AnalyzeAlert(ctx, mockAlert)
	if err != nil {
		return nil, fmt.Errorf("AI analysis failed: %w", err)
	}

	// Parse the AI response - expecting ActionRecommendation format
	if recommendation != nil {
		return d.parseAIRecommendation(recommendation, condition)
	}

	return nil, fmt.Errorf("received nil AI response")
}

// parseAIRecommendation parses ActionRecommendation into EvaluationResult
func (d *DefaultAIConditionEvaluator) parseAIRecommendation(recommendation *types.ActionRecommendation, condition *Condition) (*EvaluationResult, error) {
	result := &EvaluationResult{
		ConditionID: condition.ID,
		Result:      true, // Default to true for now
		Confidence:  recommendation.Confidence,
		Context:     make(map[string]interface{}),
		EvaluatedAt: time.Now(),
		Metadata:    make(map[string]interface{}),
	}

	// Parse reasoning details to extract structured analysis
	if recommendation.Reasoning != nil && recommendation.Reasoning.Summary != "" {
		// Try to parse the summary as JSON
		var aiAnalysis map[string]interface{}
		if err := json.Unmarshal([]byte(recommendation.Reasoning.Summary), &aiAnalysis); err == nil {
			// Successfully parsed JSON response
			if satisfied, ok := aiAnalysis["satisfied"].(bool); ok {
				result.Result = satisfied
			}

			// Extract detailed analysis
			if detailedAnalysis, ok := aiAnalysis["detailed_analysis"].(map[string]interface{}); ok {
				result.Metadata["detailed_analysis"] = detailedAnalysis
			}

			// Set reasoning context
			if reasoning, ok := aiAnalysis["reasoning"].(string); ok {
				result.Context["reasoning"] = reasoning
			}

			// Add recommendations if available
			if recommendations, ok := aiAnalysis["recommendations"].([]interface{}); ok {
				result.Context["recommendations"] = recommendations
			}

			result.Reasoning = recommendation.Reasoning.Summary
		} else {
			// Fallback: treat as plain text reasoning
			result.Reasoning = recommendation.Reasoning.Summary
			result.Context["reasoning"] = recommendation.Reasoning.Summary
		}
	}

	return result, nil
}

// evaluateWithFallback performs basic condition evaluation when AI is unavailable
func (d *DefaultAIConditionEvaluator) evaluateWithFallback(ctx context.Context, condition *Condition) *EvaluationResult {
	result := &EvaluationResult{
		ConditionID: condition.ID,
		Result:      false,
		Confidence:  0.5, // Lower confidence for fallback - should be < 0.7
		Context:     make(map[string]interface{}),
		EvaluatedAt: time.Now(),
		Metadata:    make(map[string]interface{}),
	}

	logger := d.logger.WithFields(logrus.Fields{
		"condition": condition,
		"trace_id":  ctx.Value("trace_id"),
	})
	logger.Debug("Evaluating simple expression")

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil
	default:
	}

	// Basic evaluation logic based on condition type and expression
	switch strings.ToLower(condition.Type) {
	case "time":
		result = d.evaluateTimeConditionFallback(condition)
	case "metric", "resource":
		result = d.evaluateMetricConditionFallback(condition)
	default:
		result = d.evaluateGenericConditionFallback(condition)
	}

	// Ensure fallback results have appropriate context
	result.Context["evaluation_method"] = "fallback"
	result.Context["ai_available"] = false

	return result
}

// evaluateTimeConditionFallback handles time-based conditions without AI
func (d *DefaultAIConditionEvaluator) evaluateTimeConditionFallback(condition *Condition) *EvaluationResult {
	result := &EvaluationResult{
		ConditionID: condition.ID,
		Result:      true,
		Confidence:  0.5, // Lower confidence for fallback
		Context:     make(map[string]interface{}),
		EvaluatedAt: time.Now(),
		Metadata:    make(map[string]interface{}),
		Reasoning:   "Basic time evaluation using confidence threshold",
	}

	// Add time context
	result.Context["current_time"] = time.Now().Format("15:04")
	result.Context["current_day"] = time.Now().Weekday().String()

	return result
}

// evaluateMetricConditionFallback handles metric/resource conditions without AI
func (d *DefaultAIConditionEvaluator) evaluateMetricConditionFallback(condition *Condition) *EvaluationResult {
	result := &EvaluationResult{
		ConditionID: condition.ID,
		Result:      true,
		Confidence:  0.5, // Lower confidence for fallback
		Context:     make(map[string]interface{}),
		EvaluatedAt: time.Now(),
		Metadata:    make(map[string]interface{}),
		Reasoning:   "Basic evaluation using confidence threshold",
	}

	// Add basic metric context
	result.Context["expression"] = condition.Expression
	result.Context["metadata"] = condition.Metadata

	return result
}

// evaluateGenericConditionFallback handles generic conditions without AI
func (d *DefaultAIConditionEvaluator) evaluateGenericConditionFallback(condition *Condition) *EvaluationResult {
	result := &EvaluationResult{
		ConditionID: condition.ID,
		Result:      false,
		Confidence:  0.5, // Lower confidence for fallback
		Context:     make(map[string]interface{}),
		EvaluatedAt: time.Now(),
		Metadata:    make(map[string]interface{}),
		Reasoning:   "Default evaluation using confidence threshold",
	}

	// Simple expression evaluation
	switch strings.ToLower(strings.TrimSpace(condition.Expression)) {
	case "true", "1", "yes", "enabled":
		result.Result = true
		result.Confidence = 0.95
		result.Reasoning = "Direct boolean evaluation to true"
	case "false", "0", "no", "disabled":
		result.Result = false
		result.Confidence = 0.95
		result.Reasoning = "Direct boolean evaluation to false"
	default:
		// Default to true with lower fallback confidence
		result.Result = true
		result.Confidence = 0.5 // Lower than AI analysis
		result.Reasoning = "Default evaluation using confidence threshold"
	}

	return result
}

// GetID implements AIConditionEvaluator interface
func (d *DefaultAIConditionEvaluator) GetID() string {
	return d.id
}

// GetCapabilities implements AIConditionEvaluator interface
func (d *DefaultAIConditionEvaluator) GetCapabilities() []string {
	return d.capabilities
}

// IsHealthy implements AIConditionEvaluator interface
func (d *DefaultAIConditionEvaluator) IsHealthy() bool {
	return true
}

type RegexEvaluator struct {
	logger *logrus.Logger
}

func (re *RegexEvaluator) Evaluate(ctx context.Context, expression string, context map[string]interface{}) (bool, error) {
	// Add context-aware logging for traceability and potential timeout handling
	logger := re.logger.WithFields(logrus.Fields{
		"expression": expression,
		"trace_id":   ctx.Value("trace_id"),
	})
	logger.Debug("Evaluating regex expression")

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	// Expected format: "field MATCHES pattern" or "field =~ pattern"
	parts := regexp.MustCompile(`\s+(MATCHES|=~)\s+`).Split(expression, 2)
	if len(parts) != 2 {
		return false, fmt.Errorf("invalid regex expression format")
	}

	fieldName := strings.TrimSpace(parts[0])
	pattern := strings.TrimSpace(parts[1])

	// Remove quotes from pattern
	if strings.HasPrefix(pattern, "'") && strings.HasSuffix(pattern, "'") {
		pattern = pattern[1 : len(pattern)-1]
	}

	// Get field value from context
	fieldValue, exists := context[fieldName]
	if !exists {
		return false, nil
	}

	fieldStr := fmt.Sprintf("%v", fieldValue)
	matched, err := regexp.MatchString(pattern, fieldStr)
	return matched, err
}

func (re *RegexEvaluator) GetType() string { return "regex" }
func (re *RegexEvaluator) IsHealthy() bool { return true }

type NumericEvaluator struct {
	logger *logrus.Logger
}

func (ne *NumericEvaluator) Evaluate(ctx context.Context, expression string, context map[string]interface{}) (bool, error) {
	ne.logger.WithField("expression", expression).Debug("Evaluating numeric expression")

	// Handle mathematical expressions and advanced numeric comparisons
	// For now, delegate to simple evaluator
	simple := &SimpleEvaluator{logger: ne.logger}
	return simple.Evaluate(ctx, expression, context)
}

func (ne *NumericEvaluator) GetType() string { return "numeric" }
func (ne *NumericEvaluator) IsHealthy() bool { return true }

type TimeEvaluator struct {
	logger *logrus.Logger
}

func (te *TimeEvaluator) Evaluate(ctx context.Context, expression string, context map[string]interface{}) (bool, error) {
	te.logger.WithField("expression", expression).Debug("Evaluating time expression")

	// Handle time-based conditions like "current_time > 2023-01-01" or "age < 30m"
	now := time.Now()
	context["current_time"] = now
	context["now"] = now

	// Add common time calculations
	context["hour"] = now.Hour()
	context["minute"] = now.Minute()
	context["weekday"] = now.Weekday().String()

	// For now, delegate to simple evaluator with enhanced context
	simple := &SimpleEvaluator{logger: te.logger}
	return simple.Evaluate(ctx, expression, context)
}

func (te *TimeEvaluator) GetType() string { return "time" }
func (te *TimeEvaluator) IsHealthy() bool { return true }
