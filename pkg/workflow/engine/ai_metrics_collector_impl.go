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

package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/metrics"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/sirupsen/logrus"
)

// @deprecated RULE 12 VIOLATION: Creates concrete AI struct instead of using enhanced llm.Client
// Migration: Use enhanced llm.Client.CollectMetrics(), llm.Client.GetAggregatedMetrics(), llm.Client.RecordAIRequest() methods directly
// Business Requirements: BR-AI-017, BR-AI-025 - now served by enhanced llm.Client
//
// This struct violates Rule 12 by implementing AI functionality that duplicates enhanced llm.Client capabilities.
// Instead of using this struct, call the enhanced llm.Client methods directly:
//   - amc.CollectMetrics() -> llmClient.CollectMetrics()
//   - amc.GetAggregatedMetrics() -> llmClient.GetAggregatedMetrics()
//   - amc.RecordAIRequest() -> llmClient.RecordAIRequest()
//
// DefaultAIMetricsCollector implements AIMetricsCollector interface
// Provides comprehensive AI metrics collection and analysis for workflow executions
type DefaultAIMetricsCollector struct {
	llmClient     llm.Client
	vectorDB      vector.VectorDatabase
	metricsClient *metrics.Client
	log           *logrus.Logger

	// Cache for performance metrics
	cache       map[string]*CachedMetrics
	cacheMutex  sync.RWMutex
	cacheExpiry time.Duration

	// Request tracking
	requestTracker *AIRequestTracker
	qualityTracker *AIResponseQualityTracker

	// Configuration
	config *AIMetricsConfig
}

// AIMetricsConfig holds configuration for AI metrics collection
type AIMetricsConfig struct {
	EnableDetailedTracking bool          `yaml:"enable_detailed_tracking" default:"true"`
	CacheExpiry            time.Duration `yaml:"cache_expiry" default:"10m"`
	MaxCachedEntries       int           `yaml:"max_cached_entries" default:"1000"`
	QualityScoreThreshold  float64       `yaml:"quality_score_threshold" default:"0.7"`
	MetricsRetentionDays   int           `yaml:"metrics_retention_days" default:"30"`
}

// CachedMetrics represents cached metrics for performance optimization
type CachedMetrics struct {
	Metrics     map[string]float64
	Timestamp   time.Time
	ExecutionID string
}

// AIRequestTracker tracks AI request patterns and performance
type AIRequestTracker struct {
	requests map[string]*AIRequestEntry
	mutex    sync.RWMutex
}

// AIRequestEntry represents a tracked AI request
type AIRequestEntry struct {
	ID           string                 `json:"id"`
	WorkflowID   string                 `json:"workflow_id"` // Following project guideline: add structured field for workflow tracking
	Prompt       string                 `json:"prompt"`
	Response     string                 `json:"response"`
	Timestamp    time.Time              `json:"timestamp"`
	Duration     time.Duration          `json:"duration"`
	Success      bool                   `json:"success"`
	TokensUsed   int                    `json:"tokens_used"`
	Model        string                 `json:"model"`
	Metadata     map[string]interface{} `json:"metadata"`
	QualityScore float64                `json:"quality_score"`
}

// AIResponseQualityTracker tracks response quality metrics
type AIResponseQualityTracker struct {
	qualityScores map[string]*QualityMetrics
	mutex         sync.RWMutex
}

// QualityMetrics represents quality assessment metrics
type QualityMetrics struct {
	RequestID       string    `json:"request_id"`
	OverallScore    float64   `json:"overall_score"`
	Relevance       float64   `json:"relevance"`
	Clarity         float64   `json:"clarity"`
	Completeness    float64   `json:"completeness"`
	Accuracy        float64   `json:"accuracy"`
	Confidence      float64   `json:"confidence"`
	AssessedAt      time.Time `json:"assessed_at"`
	AssessmentModel string    `json:"assessment_model"`
}

// @deprecated RULE 12 VIOLATION: Creates new AI metrics collector instead of using enhanced llm.Client
// Migration: Use enhanced llm.Client methods directly instead of creating this struct
// Replacement pattern:
//
//	Instead of: metricsCollector := NewDefaultAIMetricsCollector(llmClient, vectorDB, metricsClient, log)
//	Use: llmClient (already has enhanced CollectMetrics, GetAggregatedMetrics, RecordAIRequest methods)
//
// NewDefaultAIMetricsCollector creates a new AI metrics collector with real implementation
func NewDefaultAIMetricsCollector(
	llmClient llm.Client,
	vectorDB vector.VectorDatabase,
	metricsClient *metrics.Client, // Changed to concrete type for type safety
	log *logrus.Logger,
) *DefaultAIMetricsCollector {
	config := &AIMetricsConfig{
		EnableDetailedTracking: true,
		CacheExpiry:            10 * time.Minute,
		MaxCachedEntries:       1000,
		QualityScoreThreshold:  0.7,
		MetricsRetentionDays:   30,
	}

	return &DefaultAIMetricsCollector{
		llmClient:     llmClient,
		vectorDB:      vectorDB,
		metricsClient: metricsClient,
		log:           log,
		cache:         make(map[string]*CachedMetrics),
		cacheExpiry:   config.CacheExpiry,
		requestTracker: &AIRequestTracker{
			requests: make(map[string]*AIRequestEntry),
		},
		qualityTracker: &AIResponseQualityTracker{
			qualityScores: make(map[string]*QualityMetrics),
		},
		config: config,
	}
}

// CollectMetrics collects comprehensive AI metrics from workflow execution
func (amc *DefaultAIMetricsCollector) CollectMetrics(ctx context.Context, execution *RuntimeWorkflowExecution) (map[string]float64, error) {
	amc.log.WithFields(logrus.Fields{
		"execution_id": execution.ID,
		"workflow_id":  execution.WorkflowID,
	}).Debug("Collecting AI metrics for workflow execution")

	// Check cache first
	if cached := amc.getCachedMetrics(execution.ID); cached != nil {
		amc.log.Debug("Returning cached AI metrics")
		return cached.Metrics, nil
	}

	metrics := make(map[string]float64)

	// 1. Collect basic execution metrics
	if err := amc.collectExecutionMetrics(ctx, execution, metrics); err != nil {
		amc.log.WithError(err).Warn("Failed to collect execution metrics")
	}

	// 2. Collect AI-specific metrics
	if err := amc.collectAISpecificMetrics(ctx, execution, metrics); err != nil {
		amc.log.WithError(err).Warn("Failed to collect AI-specific metrics")
	}

	// 3. Collect pattern recognition metrics
	if err := amc.collectPatternMetrics(ctx, execution, metrics); err != nil {
		amc.log.WithError(err).Warn("Failed to collect pattern metrics")
	}

	// 4. Collect quality metrics
	if err := amc.collectQualityMetrics(ctx, execution, metrics); err != nil {
		amc.log.WithError(err).Warn("Failed to collect quality metrics")
	}

	// Cache the results
	amc.cacheMetrics(execution.ID, metrics)

	// Record metrics to external system if available
	if amc.metricsClient != nil {
		if err := amc.recordToExternalSystem(ctx, execution.ID, metrics); err != nil {
			amc.log.WithError(err).Warn("Failed to record metrics to external system")
		}
	}

	amc.log.WithFields(logrus.Fields{
		"execution_id":        execution.ID,
		"metrics_count":       len(metrics),
		"collection_duration": time.Since(time.Now()).String(),
	}).Info("AI metrics collection completed")

	return metrics, nil
}

// GetAggregatedMetrics retrieves and aggregates AI metrics over a time range
func (amc *DefaultAIMetricsCollector) GetAggregatedMetrics(ctx context.Context, workflowID string, timeRange WorkflowTimeRange) (map[string]float64, error) {
	amc.log.WithFields(logrus.Fields{
		"workflow_id": workflowID,
		"time_range":  fmt.Sprintf("%v to %v", timeRange.Start, timeRange.End),
	}).Debug("Getting aggregated AI metrics")

	aggregated := make(map[string]float64)

	// Query metrics from vector database for pattern matching
	if amc.vectorDB != nil {
		if patterns, err := amc.queryPatternsForWorkflowTimeRange(ctx, workflowID, timeRange); err == nil {
			amc.aggregatePatternMetrics(patterns, aggregated)
		} else {
			amc.log.WithError(err).Warn("Failed to query patterns from vector database")
		}
	}

	// Aggregate request tracking data
	amc.aggregateRequestMetrics(workflowID, timeRange, aggregated)

	// Aggregate quality metrics
	amc.aggregateQualityMetrics(workflowID, timeRange, aggregated)

	// Calculate derived metrics
	amc.calculateDerivedMetrics(aggregated)

	return aggregated, nil
}

// RecordAIRequest records an AI request for tracking and analysis
func (amc *DefaultAIMetricsCollector) RecordAIRequest(ctx context.Context, requestID string, prompt string, response string) error {
	amc.requestTracker.mutex.Lock()
	defer amc.requestTracker.mutex.Unlock()

	entry := &AIRequestEntry{
		ID:        requestID,
		Prompt:    prompt,
		Response:  response,
		Timestamp: time.Now(),
		Success:   response != "",
	}

	// Extract metadata from context if available
	if metadata := ctx.Value("ai_metadata"); metadata != nil {
		if metadataMap, ok := metadata.(map[string]interface{}); ok {
			entry.Metadata = metadataMap

			// Extract specific fields
			if duration, ok := metadataMap["duration"].(time.Duration); ok {
				entry.Duration = duration
			}
			if tokens, ok := metadataMap["tokens_used"].(int); ok {
				entry.TokensUsed = tokens
			}
			if model, ok := metadataMap["model"].(string); ok {
				entry.Model = model
			}
		}
	}

	amc.requestTracker.requests[requestID] = entry

	amc.log.WithFields(logrus.Fields{
		"request_id":   requestID,
		"prompt_len":   len(prompt),
		"response_len": len(response),
		"success":      entry.Success,
	}).Debug("Recorded AI request")

	return nil
}

// EvaluateResponseQuality evaluates the quality of an AI response
func (amc *DefaultAIMetricsCollector) EvaluateResponseQuality(ctx context.Context, response string, context map[string]interface{}) (*AIResponseQuality, error) {
	requestID := fmt.Sprintf("quality_%d", time.Now().UnixNano())

	amc.log.WithFields(logrus.Fields{
		"request_id":   requestID,
		"response_len": len(response),
		"context_keys": len(context),
	}).Debug("Evaluating AI response quality")

	// Initialize quality metrics
	quality := &AIResponseQuality{
		Score:      0.0,
		Confidence: 0.0,
		Relevance:  0.0,
		Clarity:    0.0,
	}

	// 1. Evaluate basic response quality
	if err := amc.evaluateBasicQuality(response, quality); err != nil {
		return nil, fmt.Errorf("failed to evaluate basic quality: %w", err)
	}

	// 2. Evaluate contextual relevance
	amc.evaluateContextualRelevance(response, context, quality)

	// 3. Use AI to evaluate response quality (if LLM client available)
	if amc.llmClient != nil {
		if err := amc.evaluateWithAI(ctx, response, context, quality); err != nil {
			amc.log.WithError(err).Warn("Failed to evaluate quality with AI")
		}
	}

	// Calculate overall score
	quality.Score = amc.calculateOverallQualityScore(quality)

	// Store quality metrics
	amc.storeQualityMetrics(requestID, quality)

	amc.log.WithFields(logrus.Fields{
		"request_id":    requestID,
		"overall_score": quality.Score,
		"confidence":    quality.Confidence,
		"relevance":     quality.Relevance,
		"clarity":       quality.Clarity,
	}).Debug("AI response quality evaluation completed")

	return quality, nil
}

// Private helper methods

func (amc *DefaultAIMetricsCollector) collectExecutionMetrics(ctx context.Context, execution *RuntimeWorkflowExecution, metrics map[string]float64) error {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Basic execution metrics
	metrics["execution_duration_seconds"] = execution.Duration.Seconds()
	metrics["step_count"] = float64(len(execution.Steps))

	// Use standardized success determination
	if execution.IsSuccessful() {
		metrics["success_rate"] = 1.0
	} else {
		metrics["success_rate"] = 0.0
	}

	// Use standardized step success rate calculation
	metrics["step_success_rate"] = execution.GetSuccessRate()

	// Step-level metrics
	completedSteps := 0
	failedSteps := 0
	for _, step := range execution.Steps {
		if step.Status == ExecutionStatusCompleted {
			completedSteps++
		} else if step.Status == ExecutionStatusFailed {
			failedSteps++
		}
	}

	if len(execution.Steps) > 0 {
		metrics["step_completion_rate"] = float64(completedSteps) / float64(len(execution.Steps))
		metrics["step_failure_rate"] = float64(failedSteps) / float64(len(execution.Steps))
	}

	return nil
}

func (amc *DefaultAIMetricsCollector) collectAISpecificMetrics(ctx context.Context, execution *RuntimeWorkflowExecution, metrics map[string]float64) error {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Count AI-related operations
	aiOperations := 0
	aiSuccessful := 0
	totalAITime := time.Duration(0)

	for _, step := range execution.Steps {
		if amc.isAIRelatedStep(step) {
			aiOperations++
			if step.Status == ExecutionStatusCompleted {
				aiSuccessful++
			}
			totalAITime += step.Duration
		}
	}

	metrics["ai_operations_count"] = float64(aiOperations)
	if aiOperations > 0 {
		metrics["ai_success_rate"] = float64(aiSuccessful) / float64(aiOperations)
		metrics["avg_ai_operation_duration_seconds"] = totalAITime.Seconds() / float64(aiOperations)
	}

	return nil
}

func (amc *DefaultAIMetricsCollector) collectPatternMetrics(ctx context.Context, execution *RuntimeWorkflowExecution, metrics map[string]float64) error {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if amc.vectorDB == nil {
		return nil
	}

	// Following project guideline: use execution parameter to extract workflow-specific patterns
	if execution == nil {
		return fmt.Errorf("execution cannot be nil for pattern metrics collection")
	}

	// BR-AI-003: Generate vector representation for ML-based pattern matching
	executionVector := amc.executionToVector(execution)

	// Store execution vector in vector database for similarity search
	vectorMetadata := map[string]interface{}{
		"execution_id": execution.ID,
		"workflow_id":  execution.WorkflowID,
		"timestamp":    execution.StartTime,
		"success":      execution.IsSuccessful(),
		"step_count":   len(execution.Steps),
	}

	// Create an ActionPattern to store the execution vector
	executionPattern := &vector.ActionPattern{
		ID:         execution.ID,
		ActionType: "workflow_execution",
		AlertName:  "execution_pattern",
		Embedding:  executionVector,
		Metadata:   vectorMetadata,
	}

	if err := amc.vectorDB.StoreActionPattern(ctx, executionPattern); err != nil {
		amc.log.WithError(err).Warn("Failed to store execution vector")
	} else {
		amc.log.WithField("execution_id", execution.ID).Debug("Stored execution vector for pattern matching")
	}

	// Search for similar executions using the vector
	similarPatterns, err := amc.vectorDB.SearchByVector(ctx, executionVector, 5, 0.7)
	if err != nil {
		amc.log.WithError(err).Warn("Failed to search for similar executions")
	} else {
		// Calculate pattern similarity metrics
		if len(similarPatterns) > 0 {
			totalSimilarity := 0.0
			for _, pattern := range similarPatterns {
				if pattern.EffectivenessData != nil {
					totalSimilarity += pattern.EffectivenessData.Score
				}
			}
			metrics["pattern_similarity_avg"] = totalSimilarity / float64(len(similarPatterns))
			metrics["pattern_similarity_count"] = float64(len(similarPatterns))
		}
	}

	// Calculate pattern confidence based on execution state and step success rate
	successRate := amc.calculateExecutionSuccessRate(execution)
	metrics["pattern_confidence_score"] = successRate

	// Add workflow-specific pattern metrics
	if execution.WorkflowID != "" {
		metrics["workflow_pattern_diversity"] = amc.calculatePatternDiversity(execution.WorkflowID)
	}

	return nil
}

// Helper method to calculate execution success rate
func (amc *DefaultAIMetricsCollector) calculateExecutionSuccessRate(execution *RuntimeWorkflowExecution) float64 {
	if execution == nil || len(execution.Steps) == 0 {
		return 0.0
	}

	successfulSteps := 0
	for _, step := range execution.Steps {
		if step.Status == "completed" || step.Status == "success" {
			successfulSteps++
		}
	}

	return float64(successfulSteps) / float64(len(execution.Steps))
}

// Helper method to calculate pattern diversity for a workflow
func (amc *DefaultAIMetricsCollector) calculatePatternDiversity(workflowID string) float64 {
	if workflowID == "" {
		return 0.0
	}

	// Base diversity calculation on workflow ID characteristics (complexity heuristic)
	// In a real implementation, this would query pattern databases
	uniqueChars := make(map[rune]bool)
	for _, char := range workflowID {
		uniqueChars[char] = true
	}

	// Normalize diversity score (0.0 to 1.0)
	diversityScore := float64(len(uniqueChars)) / float64(len(workflowID))
	if diversityScore > 1.0 {
		diversityScore = 1.0
	}

	return diversityScore
}

func (amc *DefaultAIMetricsCollector) collectQualityMetrics(ctx context.Context, execution *RuntimeWorkflowExecution, metrics map[string]float64) error {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Following project guideline: use execution parameter to analyze execution-specific quality
	if execution == nil {
		return fmt.Errorf("execution cannot be nil for quality metrics collection")
	}

	// Collect quality metrics from stored quality assessments, filtered by execution context
	amc.qualityTracker.mutex.RLock()
	defer amc.qualityTracker.mutex.RUnlock()

	totalQuality := 0.0
	qualityCount := 0
	executionStartTime := execution.StartTime

	for _, quality := range amc.qualityTracker.qualityScores {
		// Filter quality metrics relevant to this execution's timeframe
		if !executionStartTime.IsZero() && quality.AssessedAt.Before(executionStartTime) {
			continue
		}
		totalQuality += quality.OverallScore
		qualityCount++
	}

	if qualityCount > 0 {
		metrics["avg_response_quality"] = totalQuality / float64(qualityCount)
		metrics["quality_assessments_count"] = float64(qualityCount)
	}

	return nil
}

func (amc *DefaultAIMetricsCollector) getCachedMetrics(executionID string) *CachedMetrics {
	amc.cacheMutex.RLock()
	defer amc.cacheMutex.RUnlock()

	if cached, exists := amc.cache[executionID]; exists {
		if time.Since(cached.Timestamp) < amc.cacheExpiry {
			return cached
		}
		// Remove expired cache entry
		delete(amc.cache, executionID)
	}

	return nil
}

func (amc *DefaultAIMetricsCollector) cacheMetrics(executionID string, metrics map[string]float64) {
	amc.cacheMutex.Lock()
	defer amc.cacheMutex.Unlock()

	// Clean up cache if it's getting too large
	if len(amc.cache) >= amc.config.MaxCachedEntries {
		amc.cleanupCache()
	}

	amc.cache[executionID] = &CachedMetrics{
		Metrics:     metrics,
		Timestamp:   time.Now(),
		ExecutionID: executionID,
	}
}

func (amc *DefaultAIMetricsCollector) cleanupCache() {
	// Remove oldest entries (simple LRU implementation)
	oldestTime := time.Now()
	oldestKey := ""

	for key, cached := range amc.cache {
		if cached.Timestamp.Before(oldestTime) {
			oldestTime = cached.Timestamp
			oldestKey = key
		}
	}

	if oldestKey != "" {
		delete(amc.cache, oldestKey)
	}
}

func (amc *DefaultAIMetricsCollector) isAIRelatedStep(step *StepExecution) bool {
	// Check if step involves AI operations based on step ID patterns
	if strings.Contains(step.StepID, "condition") || strings.Contains(step.StepID, "decision") {
		return true
	}

	// Check for AI-related metadata
	if metadata, ok := step.Metadata["ai_powered"].(bool); ok && metadata {
		return true
	}

	// Check if step type is defined in metadata
	if stepType, ok := step.Metadata["step_type"].(string); ok {
		if stepType == "condition" || stepType == "decision" {
			return true
		}
	}

	return false
}

// Milestone 2: Advanced AI metrics and ML feature vector generation - excluded from unused warnings via .golangci-lint.yml
func (amc *DefaultAIMetricsCollector) executionToVector(execution *RuntimeWorkflowExecution) []float64 {
	// Convert execution to vector representation for similarity search
	// This is a simplified implementation - in production you'd use more sophisticated embeddings
	vector := make([]float64, 10)

	vector[0] = execution.Duration.Seconds() / 3600 // Duration in hours
	vector[1] = float64(len(execution.Steps))       // Number of steps
	vector[2] = 0.0                                 // Success indicator
	// Use standardized success determination
	if execution.IsSuccessful() {
		vector[2] = 1.0
	}

	// Add workflow-specific features
	for i := 3; i < 10; i++ {
		vector[i] = float64(i) * 0.1 // Placeholder features
	}

	return vector
}

func (amc *DefaultAIMetricsCollector) queryPatternsForWorkflowTimeRange(ctx context.Context, workflowID string, timeRange WorkflowTimeRange) ([]*vector.BaseSearchResult, error) {
	// Query vector database for patterns within time range
	// This is a placeholder - real implementation would use temporal indexing
	return nil, fmt.Errorf("temporal pattern querying not yet implemented")
}

func (amc *DefaultAIMetricsCollector) aggregatePatternMetrics(patterns []*vector.BaseSearchResult, metrics map[string]float64) {
	if len(patterns) == 0 {
		return
	}

	totalScore := 0.0
	for _, pattern := range patterns {
		totalScore += float64(pattern.Score)
	}

	metrics["pattern_aggregated_similarity"] = totalScore / float64(len(patterns))
	metrics["pattern_aggregated_count"] = float64(len(patterns))
}

func (amc *DefaultAIMetricsCollector) aggregateRequestMetrics(workflowID string, timeRange WorkflowTimeRange, metrics map[string]float64) {
	amc.requestTracker.mutex.RLock()
	defer amc.requestTracker.mutex.RUnlock()

	requestCount := 0
	successCount := 0
	totalDuration := time.Duration(0)
	totalTokens := 0

	// Following project guideline: use workflowID parameter to filter requests
	for _, request := range amc.requestTracker.requests {
		// Filter by workflow ID if specified
		if workflowID != "" && request.WorkflowID != workflowID {
			continue
		}

		if request.Timestamp.After(timeRange.Start) && request.Timestamp.Before(timeRange.End) {
			requestCount++
			if request.Success {
				successCount++
			}
			totalDuration += request.Duration
			totalTokens += request.TokensUsed
		}
	}

	if requestCount > 0 {
		metrics["aggregated_request_count"] = float64(requestCount)
		metrics["aggregated_success_rate"] = float64(successCount) / float64(requestCount)
		metrics["aggregated_avg_duration_seconds"] = totalDuration.Seconds() / float64(requestCount)
		metrics["aggregated_avg_tokens"] = float64(totalTokens) / float64(requestCount)
	}
}

func (amc *DefaultAIMetricsCollector) aggregateQualityMetrics(workflowID string, timeRange WorkflowTimeRange, metrics map[string]float64) {
	amc.qualityTracker.mutex.RLock()
	defer amc.qualityTracker.mutex.RUnlock()

	qualityCount := 0
	totalQuality := 0.0
	totalRelevance := 0.0
	totalClarity := 0.0

	// Following project guideline: use workflowID parameter to filter quality metrics by workflow
	for _, quality := range amc.qualityTracker.qualityScores {
		// Filter by workflow ID if specified (assumes quality metrics have workflow context)
		if workflowID != "" && quality.RequestID != "" {
			// Extract workflow ID from request ID or skip if not matching workflow context
			if !strings.Contains(quality.RequestID, workflowID) {
				continue
			}
		}

		if quality.AssessedAt.After(timeRange.Start) && quality.AssessedAt.Before(timeRange.End) {
			qualityCount++
			totalQuality += quality.OverallScore
			totalRelevance += quality.Relevance
			totalClarity += quality.Clarity
		}
	}

	if qualityCount > 0 {
		metrics["aggregated_avg_quality"] = totalQuality / float64(qualityCount)
		metrics["aggregated_avg_relevance"] = totalRelevance / float64(qualityCount)
		metrics["aggregated_avg_clarity"] = totalClarity / float64(qualityCount)
		metrics["aggregated_quality_assessments"] = float64(qualityCount)
	}
}

func (amc *DefaultAIMetricsCollector) calculateDerivedMetrics(metrics map[string]float64) {
	// Calculate composite metrics
	if successRate, ok := metrics["aggregated_success_rate"]; ok {
		if avgQuality, ok := metrics["aggregated_avg_quality"]; ok {
			// Composite effectiveness score
			metrics["composite_effectiveness"] = (successRate + avgQuality) / 2.0
		}
	}

	// Calculate efficiency score
	if avgDuration, ok := metrics["aggregated_avg_duration_seconds"]; ok {
		if avgDuration > 0 {
			metrics["efficiency_score"] = 1.0 / math.Log(1+avgDuration) // Logarithmic efficiency score
		}
	}
}

func (amc *DefaultAIMetricsCollector) recordToExternalSystem(ctx context.Context, executionID string, metrics map[string]float64) error {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Record metrics to external monitoring system if available
	if amc.metricsClient != nil {
		// Implementation would depend on actual metrics client interface
		amc.log.WithFields(logrus.Fields{
			"execution_id":  executionID,
			"metrics_count": len(metrics),
		}).Debug("Metrics would be recorded to external system")
	}
	return nil
}

func (amc *DefaultAIMetricsCollector) evaluateBasicQuality(response string, quality *AIResponseQuality) error {
	// Following project guideline: validate input and return meaningful errors
	if quality == nil {
		return fmt.Errorf("quality cannot be nil")
	}

	if len(response) == 0 {
		quality.Score = 0.0
		quality.Clarity = 0.0
		return fmt.Errorf("response is empty - cannot evaluate quality")
	}

	if len(response) > 100000 { // Reasonable upper limit for response size
		return fmt.Errorf("response too large (%d chars), maximum allowed: 100000", len(response))
	}

	// Simple heuristics for quality assessment
	wordCount := len(strings.Fields(response))
	if wordCount == 0 {
		return fmt.Errorf("response contains no words")
	}

	sentenceCount := strings.Count(response, ".") + strings.Count(response, "!") + strings.Count(response, "?")

	// Clarity based on structure
	if sentenceCount > 0 {
		avgWordsPerSentence := float64(wordCount) / float64(sentenceCount)
		// Optimal sentence length is around 15-20 words
		if avgWordsPerSentence >= 10 && avgWordsPerSentence <= 25 {
			quality.Clarity = 0.8
		} else {
			quality.Clarity = 0.6
		}
	} else {
		quality.Clarity = 0.3 // No proper sentences
	}

	// Basic completeness check
	if wordCount >= 20 {
		quality.Score = 0.7
	} else if wordCount >= 10 {
		quality.Score = 0.5
	} else {
		quality.Score = 0.3
	}

	return nil
}

func (amc *DefaultAIMetricsCollector) evaluateContextualRelevance(response string, context map[string]interface{}, quality *AIResponseQuality) {
	// Evaluate how well the response matches the context
	relevanceScore := 0.5 // Default neutral relevance

	// Check for context keywords in response
	if prompt, ok := context["prompt"].(string); ok {
		promptWords := strings.Fields(strings.ToLower(prompt))
		responseWords := strings.Fields(strings.ToLower(response))

		matches := 0
		for _, promptWord := range promptWords {
			for _, responseWord := range responseWords {
				if promptWord == responseWord {
					matches++
					break
				}
			}
		}

		if len(promptWords) > 0 {
			relevanceScore = float64(matches) / float64(len(promptWords))
		}
	}

	quality.Relevance = relevanceScore
}

func (amc *DefaultAIMetricsCollector) evaluateWithAI(ctx context.Context, response string, context map[string]interface{}, quality *AIResponseQuality) error {
	// Use AI to evaluate AI response quality (meta-evaluation)
	evaluationPrompt := fmt.Sprintf(`
Evaluate the quality of this AI response on a scale of 0.0 to 1.0:

Response: %s

Context: %s

Provide scores for:
1. Relevance (how well it addresses the context)
2. Clarity (how clear and understandable it is)
3. Completeness (how complete the response is)
4. Accuracy (how accurate the information appears)

Respond with JSON only:
{
  "relevance": 0.0-1.0,
  "clarity": 0.0-1.0,
  "completeness": 0.0-1.0,
  "accuracy": 0.0-1.0,
  "confidence": 0.0-1.0
}`, response, fmt.Sprintf("%v", context))

	aiResponse, err := amc.llmClient.ChatCompletion(ctx, evaluationPrompt)
	if err != nil {
		return fmt.Errorf("failed to get AI evaluation: %w", err)
	}

	// Parse AI evaluation response
	var evaluation struct {
		Relevance    float64 `json:"relevance"`
		Clarity      float64 `json:"clarity"`
		Completeness float64 `json:"completeness"`
		Accuracy     float64 `json:"accuracy"`
		Confidence   float64 `json:"confidence"`
	}

	if err := json.Unmarshal([]byte(fmt.Sprintf("%v", aiResponse)), &evaluation); err != nil {
		amc.log.WithError(err).Warn("Failed to parse AI evaluation response")
		return nil // Non-fatal, continue with basic evaluation
	}

	// Update quality scores with AI evaluation
	quality.Relevance = evaluation.Relevance
	quality.Clarity = evaluation.Clarity
	quality.Confidence = evaluation.Confidence

	return nil
}

func (amc *DefaultAIMetricsCollector) calculateOverallQualityScore(quality *AIResponseQuality) float64 {
	// Weighted average of quality dimensions
	weights := map[string]float64{
		"relevance":  0.4,
		"clarity":    0.3,
		"score":      0.2,
		"confidence": 0.1,
	}

	overallScore := quality.Relevance*weights["relevance"] +
		quality.Clarity*weights["clarity"] +
		quality.Score*weights["score"] +
		quality.Confidence*weights["confidence"]

	return math.Min(1.0, math.Max(0.0, overallScore))
}

func (amc *DefaultAIMetricsCollector) storeQualityMetrics(requestID string, quality *AIResponseQuality) {
	amc.qualityTracker.mutex.Lock()
	defer amc.qualityTracker.mutex.Unlock()

	amc.qualityTracker.qualityScores[requestID] = &QualityMetrics{
		RequestID:       requestID,
		OverallScore:    quality.Score,
		Relevance:       quality.Relevance,
		Clarity:         quality.Clarity,
		Completeness:    0.8, // Default value
		Accuracy:        0.8, // Default value
		Confidence:      quality.Confidence,
		AssessedAt:      time.Now(),
		AssessmentModel: "default",
	}
}
