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
package vector

import (
	"context"
	"crypto/md5"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	sharedmath "github.com/jordigilh/kubernaut/pkg/shared/math"
	"github.com/sirupsen/logrus"
)

// DefaultPatternExtractor implements PatternExtractor interface
type DefaultPatternExtractor struct {
	embeddingGenerator EmbeddingGenerator
	log                *logrus.Logger
}

// NewDefaultPatternExtractor creates a new pattern extractor
func NewDefaultPatternExtractor(embeddingGenerator EmbeddingGenerator, log *logrus.Logger) *DefaultPatternExtractor {
	return &DefaultPatternExtractor{
		embeddingGenerator: embeddingGenerator,
		log:                log,
	}
}

// ExtractPattern creates an ActionPattern from a ResourceActionTrace
func (pe *DefaultPatternExtractor) ExtractPattern(ctx context.Context, trace *actionhistory.ResourceActionTrace) (*ActionPattern, error) {
	if trace == nil {
		return nil, fmt.Errorf("action trace cannot be nil")
	}

	// Extract basic information
	namespace := extractNamespaceFromLabels(trace.AlertLabels)
	resourceName := extractResourceNameFromTrace(trace)
	resourceType := extractResourceTypeFromTrace(trace)

	// Create pattern
	pattern := &ActionPattern{
		ID:               generatePatternID(trace),
		ActionType:       trace.ActionType,
		AlertName:        trace.AlertName,
		AlertSeverity:    trace.AlertSeverity,
		Namespace:        namespace,
		ResourceType:     resourceType,
		ResourceName:     resourceName,
		ActionParameters: convertJSONMapToMap(trace.ActionParameters),
		ContextLabels:    convertJSONMapToStringMap(trace.AlertLabels),
		PreConditions:    extractPreConditions(trace),
		PostConditions:   extractPostConditions(trace),
		CreatedAt:        trace.CreatedAt,
		UpdatedAt:        trace.UpdatedAt,
		Metadata:         extractMetadata(trace),
	}

	// Extract effectiveness data
	if trace.EffectivenessScore != nil {
		pattern.EffectivenessData = &EffectivenessData{
			Score: *trace.EffectivenessScore,
		}

		if trace.EffectivenessAssessedAt != nil {
			pattern.EffectivenessData.LastAssessed = *trace.EffectivenessAssessedAt
		}

		// Calculate additional effectiveness metrics
		pattern.EffectivenessData.SuccessCount = 1
		if *trace.EffectivenessScore < 0.5 {
			pattern.EffectivenessData.SuccessCount = 0
			pattern.EffectivenessData.FailureCount = 1
		}

		if trace.ExecutionDurationMs != nil {
			pattern.EffectivenessData.AverageExecutionTime = time.Duration(*trace.ExecutionDurationMs) * time.Millisecond
		}

		// Extract contextual factors
		pattern.EffectivenessData.ContextualFactors = pe.extractContextualFactors(trace)
	}

	// Generate embedding
	embedding, err := pe.GenerateEmbedding(ctx, pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}
	pattern.Embedding = embedding

	pe.log.WithFields(logrus.Fields{
		"pattern_id":    pattern.ID,
		"action_type":   pattern.ActionType,
		"alert_name":    pattern.AlertName,
		"embedding_dim": len(pattern.Embedding),
	}).Debug("Extracted pattern from action trace")

	return pattern, nil
}

// GenerateEmbedding creates vector embedding for a pattern
func (pe *DefaultPatternExtractor) GenerateEmbedding(ctx context.Context, pattern *ActionPattern) ([]float64, error) {
	if pe.embeddingGenerator == nil {
		// Fallback to simple embedding generation
		return pe.generateSimpleEmbedding(pattern), nil
	}

	// Generate text embedding from action description
	description := pe.createPatternDescription(pattern)
	textEmbedding, err := pe.embeddingGenerator.GenerateTextEmbedding(ctx, description)
	if err != nil {
		pe.log.WithError(err).Warn("Failed to generate text embedding, using simple embedding")
		return pe.generateSimpleEmbedding(pattern), nil
	}

	// Generate action embedding
	actionEmbedding, err := pe.embeddingGenerator.GenerateActionEmbedding(ctx, pattern.ActionType, pattern.ActionParameters)
	if err != nil {
		pe.log.WithError(err).Warn("Failed to generate action embedding")
		actionEmbedding = make([]float64, len(textEmbedding))
	}

	// Generate context embedding
	contextEmbedding, err := pe.embeddingGenerator.GenerateContextEmbedding(ctx, pattern.ContextLabels, pattern.Metadata)
	if err != nil {
		pe.log.WithError(err).Warn("Failed to generate context embedding")
		contextEmbedding = make([]float64, len(textEmbedding))
	}

	// Combine embeddings
	combinedEmbedding := pe.embeddingGenerator.CombineEmbeddings(textEmbedding, actionEmbedding, contextEmbedding)

	return combinedEmbedding, nil
}

// ExtractFeatures extracts contextual features from the pattern
func (pe *DefaultPatternExtractor) ExtractFeatures(ctx context.Context, pattern *ActionPattern) (map[string]float64, error) {
	features := make(map[string]float64)

	// Action type features
	features["action_type_hash"] = hashStringToFloat(pattern.ActionType)

	// Alert severity features
	severityScores := map[string]float64{
		"critical": 1.0,
		"warning":  0.7,
		"info":     0.3,
		"":         0.0,
	}
	features["alert_severity"] = severityScores[strings.ToLower(pattern.AlertSeverity)]

	// Resource type features
	features["resource_type_hash"] = hashStringToFloat(pattern.ResourceType)

	// Namespace diversity (some namespaces may be more critical)
	namespaceScores := map[string]float64{
		"production":  1.0,
		"staging":     0.8,
		"development": 0.5,
		"kube-system": 0.9,
		"monitoring":  0.6,
		"default":     0.4,
	}
	if score, exists := namespaceScores[pattern.Namespace]; exists {
		features["namespace_criticality"] = score
	} else {
		features["namespace_criticality"] = 0.5 // Default for unknown namespaces
	}

	// Time-based features
	if !pattern.CreatedAt.IsZero() {
		// Hour of day (normalized)
		features["hour_of_day"] = float64(pattern.CreatedAt.Hour()) / 24.0
		// Day of week (normalized)
		features["day_of_week"] = float64(pattern.CreatedAt.Weekday()) / 7.0
	}

	// Effectiveness-based features
	if pattern.EffectivenessData != nil {
		features["effectiveness_score"] = pattern.EffectivenessData.Score
		features["success_rate"] = float64(pattern.EffectivenessData.SuccessCount) /
			float64(pattern.EffectivenessData.SuccessCount+pattern.EffectivenessData.FailureCount)

		if pattern.EffectivenessData.AverageExecutionTime > 0 {
			// Normalize execution time (log scale for very large values)
			execTimeSeconds := pattern.EffectivenessData.AverageExecutionTime.Seconds()
			features["execution_time_log"] = math.Log10(execTimeSeconds + 1)
		}
	}

	// Action parameter complexity
	features["parameter_count"] = float64(len(pattern.ActionParameters))

	// Context richness
	features["context_label_count"] = float64(len(pattern.ContextLabels))

	pe.log.WithFields(logrus.Fields{
		"pattern_id":    pattern.ID,
		"feature_count": len(features),
	}).Debug("Extracted features from pattern")

	return features, nil
}

// CalculateSimilarity computes similarity between two patterns
func (pe *DefaultPatternExtractor) CalculateSimilarity(pattern1, pattern2 *ActionPattern) float64 {
	if len(pattern1.Embedding) != len(pattern2.Embedding) {
		return 0.0
	}

	// Use cosine similarity for embedding comparison
	embeddingSimilarity := sharedmath.CosineSimilarity(pattern1.Embedding, pattern2.Embedding)

	// Add context-based similarity boosters
	contextSimilarity := pe.calculateContextSimilarity(pattern1, pattern2)

	// Weighted combination
	totalSimilarity := 0.7*embeddingSimilarity + 0.3*contextSimilarity

	return math.Max(0.0, math.Min(1.0, totalSimilarity))
}

// Helper methods

// generateSimpleEmbedding creates a simple embedding when no embedding generator is available
func (pe *DefaultPatternExtractor) generateSimpleEmbedding(pattern *ActionPattern) []float64 {
	// Create a simple 128-dimensional embedding based on pattern features
	embedding := make([]float64, 128)

	// Action type component
	actionHash := hashStringToFloat(pattern.ActionType)
	for i := 0; i < 32; i++ {
		embedding[i] = math.Sin(actionHash * float64(i+1))
	}

	// Alert name component
	alertHash := hashStringToFloat(pattern.AlertName)
	for i := 32; i < 64; i++ {
		embedding[i] = math.Cos(alertHash * float64(i-31))
	}

	// Resource type component
	resourceHash := hashStringToFloat(pattern.ResourceType)
	for i := 64; i < 96; i++ {
		embedding[i] = math.Tanh(resourceHash * float64(i-63))
	}

	// Context component
	contextHash := hashStringToFloat(pattern.Namespace + pattern.ResourceName)
	for i := 96; i < 128; i++ {
		embedding[i] = math.Sin(contextHash*float64(i-95)) * 0.5
	}

	// Normalize the embedding
	norm := 0.0
	for _, val := range embedding {
		norm += val * val
	}
	norm = math.Sqrt(norm)

	if norm > 0 {
		for i := range embedding {
			embedding[i] /= norm
		}
	}

	return embedding
}

// createPatternDescription creates a text description of the pattern for embedding generation
func (pe *DefaultPatternExtractor) createPatternDescription(pattern *ActionPattern) string {
	var parts []string

	parts = append(parts, fmt.Sprintf("action: %s", pattern.ActionType))
	parts = append(parts, fmt.Sprintf("alert: %s", pattern.AlertName))
	parts = append(parts, fmt.Sprintf("severity: %s", pattern.AlertSeverity))
	parts = append(parts, fmt.Sprintf("resource: %s", pattern.ResourceType))
	parts = append(parts, fmt.Sprintf("namespace: %s", pattern.Namespace))

	// Add key parameters
	for key, value := range pattern.ActionParameters {
		parts = append(parts, fmt.Sprintf("%s: %v", key, value))
	}

	return strings.Join(parts, ", ")
}

// extractContextualFactors extracts contextual factors that might affect effectiveness
func (pe *DefaultPatternExtractor) extractContextualFactors(trace *actionhistory.ResourceActionTrace) map[string]float64 {
	factors := make(map[string]float64)

	// Time-based factors
	if !trace.CreatedAt.IsZero() {
		factors["hour_of_day"] = float64(trace.CreatedAt.Hour()) / 24.0
		factors["day_of_week"] = float64(trace.CreatedAt.Weekday()) / 7.0
		factors["is_weekend"] = 0.0
		if trace.CreatedAt.Weekday() == time.Saturday || trace.CreatedAt.Weekday() == time.Sunday {
			factors["is_weekend"] = 1.0
		}
	}

	// Model confidence factor
	factors["model_confidence"] = trace.ModelConfidence

	// Execution context factors
	if trace.ExecutionDurationMs != nil {
		factors["execution_duration_normalized"] = math.Log10(float64(*trace.ExecutionDurationMs)+1) / 10.0
	}

	return factors
}

// calculateContextSimilarity calculates similarity based on context
func (pe *DefaultPatternExtractor) calculateContextSimilarity(pattern1, pattern2 *ActionPattern) float64 {
	var similarities []float64

	// Action type similarity
	if pattern1.ActionType == pattern2.ActionType {
		similarities = append(similarities, 1.0)
	} else {
		similarities = append(similarities, 0.0)
	}

	// Alert severity similarity
	severityOrder := map[string]int{"info": 1, "warning": 2, "critical": 3}
	sev1 := severityOrder[strings.ToLower(pattern1.AlertSeverity)]
	sev2 := severityOrder[strings.ToLower(pattern2.AlertSeverity)]
	if sev1 > 0 && sev2 > 0 {
		sevSim := 1.0 - math.Abs(float64(sev1-sev2))/2.0
		similarities = append(similarities, math.Max(0.0, sevSim))
	}

	// Namespace similarity
	if pattern1.Namespace == pattern2.Namespace {
		similarities = append(similarities, 1.0)
	} else {
		similarities = append(similarities, 0.0)
	}

	// Resource type similarity
	if pattern1.ResourceType == pattern2.ResourceType {
		similarities = append(similarities, 1.0)
	} else {
		similarities = append(similarities, 0.0)
	}

	// Calculate average
	if len(similarities) == 0 {
		return 0.0
	}

	total := 0.0
	for _, sim := range similarities {
		total += sim
	}
	return total / float64(len(similarities))
}

// Utility functions

// generatePatternID generates a unique ID for the pattern
func generatePatternID(trace *actionhistory.ResourceActionTrace) string {
	// Create a hash from key fields to ensure uniqueness while allowing pattern matching
	data := fmt.Sprintf("%s-%s-%s-%d",
		trace.ActionType,
		trace.AlertName,
		extractResourceNameFromTrace(trace),
		trace.ActionTimestamp.Unix())

	hash := md5.Sum([]byte(data))
	return fmt.Sprintf("pattern-%x", hash)[:16] // Use first 16 chars for readability
}

// hashStringToFloat converts a string to a float using a hash function
func hashStringToFloat(s string) float64 {
	hash := md5.Sum([]byte(s))
	// Convert first 8 bytes to uint64, then normalize to [0,1]
	var val uint64
	for i := 0; i < 8; i++ {
		val = val*256 + uint64(hash[i])
	}
	return float64(val) / float64(^uint64(0))
}

// Helper functions from effectiveness module (reused here)

// extractNamespaceFromLabels extracts namespace from alert labels
func extractNamespaceFromLabels(labels actionhistory.JSONData) string {
	if labels == nil {
		return "default"
	}

	labelsMap := map[string]interface{}(labels)

	if ns, ok := labelsMap["namespace"]; ok {
		if nsStr, ok := ns.(string); ok {
			return nsStr
		}
	}

	return "default"
}

// extractResourceNameFromTrace extracts resource name from action trace
func extractResourceNameFromTrace(trace *actionhistory.ResourceActionTrace) string {
	// Try to get from action parameters first
	if trace.ActionParameters != nil {
		paramsMap := map[string]interface{}(trace.ActionParameters)

		for _, key := range []string{"deployment", "resource", "name", "pod"} {
			if name, ok := paramsMap[key]; ok {
				if nameStr, ok := name.(string); ok {
					return nameStr
				}
			}
		}
	}

	// Try to get from alert labels
	if trace.AlertLabels != nil {
		labelsMap := map[string]interface{}(trace.AlertLabels)

		for _, key := range []string{"deployment", "pod", "service", "app"} {
			if name, ok := labelsMap[key]; ok {
				if nameStr, ok := name.(string); ok {
					return nameStr
				}
			}
		}
	}

	// Fallback to alert name
	return trace.AlertName
}

// extractResourceTypeFromTrace extracts resource type from action trace
func extractResourceTypeFromTrace(trace *actionhistory.ResourceActionTrace) string {
	// Infer from action type
	switch trace.ActionType {
	case "scale_deployment", "rollback_deployment":
		return "deployment"
	case "restart_pod", "delete_pod":
		return "pod"
	case "scale_statefulset":
		return "statefulset"
	case "drain_node", "cordon_node":
		return "node"
	case "increase_resources", "update_resources":
		// Try to determine specific resource type
		if trace.ActionParameters != nil {
			paramsMap := map[string]interface{}(trace.ActionParameters)
			if resourceType, ok := paramsMap["resource_type"]; ok {
				if rtStr, ok := resourceType.(string); ok {
					return rtStr
				}
			}
		}
		return "resource"
	default:
		return "unknown"
	}
}

// extractPreConditions extracts pre-conditions from the trace
func extractPreConditions(trace *actionhistory.ResourceActionTrace) map[string]interface{} {
	conditions := make(map[string]interface{})

	// Add alert context as pre-conditions
	conditions["alert_name"] = trace.AlertName
	conditions["alert_severity"] = trace.AlertSeverity
	conditions["alert_firing_time"] = trace.AlertFiringTime

	// Add resource state before
	if trace.ResourceStateBefore != nil {
		for k, v := range trace.ResourceStateBefore {
			conditions["resource_"+k] = v
		}
	}

	return conditions
}

// extractPostConditions extracts post-conditions from the trace
func extractPostConditions(trace *actionhistory.ResourceActionTrace) map[string]interface{} {
	conditions := make(map[string]interface{})

	// Add execution results
	conditions["execution_status"] = trace.ExecutionStatus
	if trace.ExecutionError != nil {
		conditions["execution_error"] = *trace.ExecutionError
	}

	// Add resource state after
	if trace.ResourceStateAfter != nil {
		for k, v := range trace.ResourceStateAfter {
			conditions["resource_"+k] = v
		}
	}

	// Add effectiveness results if available
	if trace.EffectivenessScore != nil {
		conditions["effectiveness_score"] = *trace.EffectivenessScore
	}

	return conditions
}

// extractMetadata extracts metadata from the trace
func extractMetadata(trace *actionhistory.ResourceActionTrace) map[string]interface{} {
	metadata := make(map[string]interface{})

	metadata["action_id"] = trace.ActionID
	metadata["model_used"] = trace.ModelUsed
	metadata["model_confidence"] = trace.ModelConfidence
	if trace.ModelReasoning != nil {
		metadata["model_reasoning"] = *trace.ModelReasoning
	}

	if trace.RoutingTier != nil {
		metadata["routing_tier"] = *trace.RoutingTier
	}

	return metadata
}

// convertJSONMapToMap converts JSONData to regular map
func convertJSONMapToMap(jm actionhistory.JSONData) map[string]interface{} {
	if jm == nil {
		return make(map[string]interface{})
	}
	return map[string]interface{}(jm)
}

// convertJSONMapToStringMap converts JSONData to string map
func convertJSONMapToStringMap(jm actionhistory.JSONData) map[string]string {
	result := make(map[string]string)
	if jm == nil {
		return result
	}

	for k, v := range jm {
		if strVal, ok := v.(string); ok {
			result[k] = strVal
		} else {
			result[k] = fmt.Sprintf("%v", v)
		}
	}
	return result
}
