package insights

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	sharedmath "github.com/jordigilh/kubernaut/pkg/shared/math"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/sirupsen/logrus"
)

// SimilarityBasedModel uses vector similarity to predict effectiveness
type SimilarityBasedModel struct {
	vectorDB  vector.VectorDatabase
	log       *logrus.Logger
	trained   bool
	modelInfo *ModelInfo
}

// NewSimilarityBasedModel creates a new similarity-based prediction model
func NewSimilarityBasedModel(vectorDB vector.VectorDatabase, log *logrus.Logger) *SimilarityBasedModel {
	return &SimilarityBasedModel{
		vectorDB: vectorDB,
		log:      log,
		modelInfo: &ModelInfo{
			Name:      "Similarity-Based Predictor",
			Version:   "1.0.0",
			Algorithm: "Vector Similarity with Weighted Features",
		},
	}
}

// Predict effectiveness using similarity to historical patterns
func (m *SimilarityBasedModel) Predict(ctx context.Context, pattern *vector.ActionPattern) (*EffectivenessPrediction, error) {
	// Find similar patterns
	similarPatterns, err := m.vectorDB.FindSimilarPatterns(ctx, pattern, 10, 0.3)
	if err != nil {
		return nil, fmt.Errorf("failed to find similar patterns: %w", err)
	}

	if len(similarPatterns) == 0 {
		// No similar patterns found, use default prediction
		return &EffectivenessPrediction{
			PredictedScore:      0.5, // Conservative default
			Confidence:          0.1, // Low confidence
			FactorContributions: make(map[string]float64),
			SimilarPatterns:     []*vector.SimilarPattern{},
			RiskFactors:         []string{"No historical data available"},
			Recommendations:     []string{"Monitor closely as this is a new pattern"},
			ModelUsed:           m.modelInfo.Name,
			PredictionTime:      time.Now(),
		}, nil
	}

	// Calculate weighted prediction based on similarity
	prediction := m.calculateWeightedPrediction(similarPatterns, pattern)

	// Analyze risk factors
	riskFactors := m.analyzeRiskFactors(pattern, similarPatterns)

	// Generate recommendations
	recommendations := m.generateRecommendations(pattern, similarPatterns, prediction.PredictedScore)

	prediction.SimilarPatterns = similarPatterns
	prediction.RiskFactors = riskFactors
	prediction.Recommendations = recommendations
	prediction.ModelUsed = m.modelInfo.Name
	prediction.PredictionTime = time.Now()

	m.log.WithFields(logrus.Fields{
		"action_type":      pattern.ActionType,
		"alert_name":       pattern.AlertName,
		"similar_patterns": len(similarPatterns),
		"predicted_score":  prediction.PredictedScore,
		"confidence":       prediction.Confidence,
	}).Debug("Generated similarity-based prediction")

	return prediction, nil
}

// Train the model (for similarity-based model, this mainly validates data availability)
func (m *SimilarityBasedModel) Train(ctx context.Context, patterns []*vector.ActionPattern) error {
	if len(patterns) == 0 {
		return fmt.Errorf("no training data available")
	}

	// Calculate model accuracy based on existing patterns
	accuracy := m.calculateModelAccuracy(patterns)

	m.modelInfo.TrainedAt = time.Now()
	m.modelInfo.TrainingSize = len(patterns)
	m.modelInfo.Accuracy = accuracy
	m.trained = true

	m.log.WithFields(logrus.Fields{
		"training_size": len(patterns),
		"accuracy":      accuracy,
	}).Info("Trained similarity-based model")

	return nil
}

// GetModelInfo returns information about the model
func (m *SimilarityBasedModel) GetModelInfo() *ModelInfo {
	return m.modelInfo
}

// IsReady returns whether the model is ready for predictions
func (m *SimilarityBasedModel) IsReady() bool {
	return m.vectorDB != nil
}

// Private methods for SimilarityBasedModel

func (m *SimilarityBasedModel) calculateWeightedPrediction(similarPatterns []*vector.SimilarPattern, targetPattern *vector.ActionPattern) *EffectivenessPrediction {
	if len(similarPatterns) == 0 {
		return &EffectivenessPrediction{
			PredictedScore:      0.5,
			Confidence:          0.1,
			FactorContributions: make(map[string]float64),
		}
	}

	var weightedScore float64
	var totalWeight float64
	factorContributions := make(map[string]float64)

	for _, simPattern := range similarPatterns {
		if simPattern.Pattern.EffectivenessData == nil {
			continue
		}

		weight := simPattern.Similarity
		score := simPattern.Pattern.EffectivenessData.Score

		weightedScore += score * weight
		totalWeight += weight

		// Track factor contributions
		if simPattern.Pattern.ActionType == targetPattern.ActionType {
			factorContributions["action_type_match"] += weight * 0.3
		}
		if simPattern.Pattern.AlertSeverity == targetPattern.AlertSeverity {
			factorContributions["severity_match"] += weight * 0.2
		}
		if simPattern.Pattern.Namespace == targetPattern.Namespace {
			factorContributions["namespace_match"] += weight * 0.1
		}
		if simPattern.Pattern.ResourceType == targetPattern.ResourceType {
			factorContributions["resource_type_match"] += weight * 0.2
		}

		factorContributions["similarity_score"] += weight * simPattern.Similarity * 0.2
	}

	predictedScore := 0.5 // Default
	if totalWeight > 0 {
		predictedScore = weightedScore / totalWeight
	}

	// Calculate confidence based on similarity scores and pattern count
	confidence := m.calculateConfidence(similarPatterns)

	return &EffectivenessPrediction{
		PredictedScore:      predictedScore,
		Confidence:          confidence,
		FactorContributions: factorContributions,
	}
}

func (m *SimilarityBasedModel) calculateConfidence(similarPatterns []*vector.SimilarPattern) float64 {
	if len(similarPatterns) == 0 {
		return 0.1
	}

	// Base confidence on number of similar patterns and their similarity scores
	avgSimilarity := 0.0
	for _, pattern := range similarPatterns {
		avgSimilarity += pattern.Similarity
	}
	avgSimilarity /= float64(len(similarPatterns))

	// More patterns with higher similarity = higher confidence
	patternCountFactor := math.Min(float64(len(similarPatterns))/10.0, 1.0)
	similarityFactor := avgSimilarity

	confidence := 0.3 + (0.7 * patternCountFactor * similarityFactor)
	return math.Max(0.1, math.Min(0.95, confidence))
}

func (m *SimilarityBasedModel) analyzeRiskFactors(pattern *vector.ActionPattern, similarPatterns []*vector.SimilarPattern) []string {
	var riskFactors []string

	// Check if any similar patterns had low effectiveness
	lowEffectivenessCount := 0
	for _, simPattern := range similarPatterns {
		if simPattern.Pattern.EffectivenessData != nil && simPattern.Pattern.EffectivenessData.Score < 0.5 {
			lowEffectivenessCount++
		}
	}

	if lowEffectivenessCount > len(similarPatterns)/2 {
		riskFactors = append(riskFactors, "Similar patterns have shown low effectiveness")
	}

	// Check for high-risk action types
	highRiskActions := []string{"drain_node", "rollback_deployment", "delete_pod"}
	for _, riskAction := range highRiskActions {
		if pattern.ActionType == riskAction {
			riskFactors = append(riskFactors, fmt.Sprintf("Action type '%s' is considered high-risk", riskAction))
			break
		}
	}

	// Check for critical namespace
	if pattern.Namespace == "kube-system" || pattern.Namespace == "production" {
		riskFactors = append(riskFactors, fmt.Sprintf("Operating in critical namespace '%s'", pattern.Namespace))
	}

	// Check for critical alert severity
	if pattern.AlertSeverity == "critical" {
		riskFactors = append(riskFactors, "Responding to critical severity alert")
	}

	return riskFactors
}

func (m *SimilarityBasedModel) generateRecommendations(pattern *vector.ActionPattern, similarPatterns []*vector.SimilarPattern, predictedScore float64) []string {
	var recommendations []string

	// General recommendations based on predicted score
	if predictedScore < 0.3 {
		recommendations = append(recommendations, "Consider alternative actions due to low predicted effectiveness")
		recommendations = append(recommendations, "Implement additional monitoring before and after action execution")
	} else if predictedScore < 0.6 {
		recommendations = append(recommendations, "Proceed with caution and monitor closely")
	} else {
		recommendations = append(recommendations, "Action shows good predicted effectiveness")
	}

	// Recommendations based on similar patterns
	if len(similarPatterns) > 0 {
		// Find the most successful similar pattern
		var bestPattern *vector.SimilarPattern
		bestScore := 0.0
		for _, simPattern := range similarPatterns {
			if simPattern.Pattern.EffectivenessData != nil && simPattern.Pattern.EffectivenessData.Score > bestScore {
				bestScore = simPattern.Pattern.EffectivenessData.Score
				bestPattern = simPattern
			}
		}

		if bestPattern != nil && bestScore > 0.8 {
			recommendations = append(recommendations,
				fmt.Sprintf("Consider using parameters similar to high-performing pattern %s", bestPattern.Pattern.ID))
		}
	}

	// Action-specific recommendations
	switch pattern.ActionType {
	case "scale_deployment":
		recommendations = append(recommendations, "Ensure resource quotas allow for scaling")
		recommendations = append(recommendations, "Monitor deployment readiness after scaling")
	case "restart_pod":
		recommendations = append(recommendations, "Verify pod restart policy before executing")
		recommendations = append(recommendations, "Check for persistent volume dependencies")
	case "increase_resources":
		recommendations = append(recommendations, "Validate cluster resource availability")
		recommendations = append(recommendations, "Consider impact on other workloads")
	}

	return recommendations
}

func (m *SimilarityBasedModel) calculateModelAccuracy(patterns []*vector.ActionPattern) float64 {
	// Simple cross-validation approach
	if len(patterns) < 5 {
		return 0.5 // Not enough data for meaningful accuracy calculation
	}

	correct := 0
	total := 0

	// Use leave-one-out validation for small datasets
	for i, testPattern := range patterns {
		if testPattern.EffectivenessData == nil || len(testPattern.Embedding) == 0 {
			continue
		}

		// Create a temporary pattern without effectiveness data
		tempPattern := *testPattern
		tempPattern.EffectivenessData = nil

		// Find similar patterns (excluding the test pattern)
		var trainPatterns []*vector.SimilarPattern
		for j, trainPattern := range patterns {
			if i == j || trainPattern.EffectivenessData == nil || len(trainPattern.Embedding) == 0 {
				continue
			}

			// Calculate actual similarity using cosine similarity
			similarity := m.calculateCosineSimilarity(testPattern.Embedding, trainPattern.Embedding)

			// Apply additional context-based similarity factors
			contextSimilarity := m.calculateContextualSimilarity(testPattern, trainPattern)
			totalSimilarity := 0.7*similarity + 0.3*contextSimilarity

			if totalSimilarity > 0.3 {
				trainPatterns = append(trainPatterns, &vector.SimilarPattern{
					Pattern:    trainPattern,
					Similarity: totalSimilarity,
				})
			}
		}

		if len(trainPatterns) == 0 {
			continue
		}

		// Sort by similarity and limit to top 5 for better accuracy
		sort.Slice(trainPatterns, func(i, j int) bool {
			return trainPatterns[i].Similarity > trainPatterns[j].Similarity
		})
		if len(trainPatterns) > 5 {
			trainPatterns = trainPatterns[:5]
		}

		// Make prediction
		prediction := m.calculateWeightedPrediction(trainPatterns, &tempPattern)

		// Compare with actual
		actualScore := testPattern.EffectivenessData.Score
		predictedScore := prediction.PredictedScore

		// Consider prediction correct if within 20% of actual
		if math.Abs(actualScore-predictedScore) <= 0.2 {
			correct++
		}
		total++
	}

	if total == 0 {
		return 0.5
	}

	accuracy := float64(correct) / float64(total)
	return accuracy
}

// calculateCosineSimilarity computes cosine similarity between two embedding vectors
func (m *SimilarityBasedModel) calculateCosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}

	var dotProduct, normA, normB float64

	for i := 0; i < len(a); i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0.0 || normB == 0.0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// calculateContextualSimilarity computes similarity based on context features
func (m *SimilarityBasedModel) calculateContextualSimilarity(pattern1, pattern2 *vector.ActionPattern) float64 {
	var similarity float64
	var factors int

	// Action type match
	if pattern1.ActionType == pattern2.ActionType {
		similarity += 0.4
	}
	factors++

	// Alert severity match
	if pattern1.AlertSeverity == pattern2.AlertSeverity {
		similarity += 0.3
	}
	factors++

	// Namespace match
	if pattern1.Namespace == pattern2.Namespace {
		similarity += 0.2
	}
	factors++

	// Resource type match
	if pattern1.ResourceType == pattern2.ResourceType {
		similarity += 0.1
	}
	factors++

	// Normalize by number of factors
	if factors > 0 {
		similarity = similarity / float64(factors)
	}

	return math.Max(0.0, math.Min(1.0, similarity))
}

// StatisticalModel uses statistical analysis for effectiveness prediction
type StatisticalModel struct {
	log             *logrus.Logger
	trained         bool
	modelInfo       *ModelInfo
	actionTypeStats map[string]*ActionStatistics
	globalStats     *GlobalStatistics
}

// ActionStatistics holds statistical data for specific action types
type ActionStatistics struct {
	Count               int
	MeanEffectiveness   float64
	StdDevEffectiveness float64
	SuccessRate         float64
	ParameterStats      map[string]*ParameterStatistics
}

// ParameterStatistics holds statistics for action parameters
type ParameterStatistics struct {
	CommonValues    map[string]int
	EffectiveValues map[string]float64
}

// GlobalStatistics holds overall statistical data
type GlobalStatistics struct {
	TotalPatterns   int
	OverallMean     float64
	OverallStdDev   float64
	SeasonalFactors map[string]float64
}

// NewStatisticalModel creates a new statistical prediction model
func NewStatisticalModel(log *logrus.Logger) *StatisticalModel {
	return &StatisticalModel{
		log: log,
		modelInfo: &ModelInfo{
			Name:      "Statistical Predictor",
			Version:   "1.0.0",
			Algorithm: "Bayesian Statistical Analysis",
		},
		actionTypeStats: make(map[string]*ActionStatistics),
	}
}

// Predict effectiveness using statistical analysis
func (m *StatisticalModel) Predict(ctx context.Context, pattern *vector.ActionPattern) (*EffectivenessPrediction, error) {
	if !m.trained {
		return nil, fmt.Errorf("model has not been trained yet")
	}

	// Get statistics for this action type
	stats, exists := m.actionTypeStats[pattern.ActionType]
	if !exists {
		// Use global statistics
		stats = &ActionStatistics{
			MeanEffectiveness:   m.globalStats.OverallMean,
			StdDevEffectiveness: m.globalStats.OverallStdDev,
			SuccessRate:         0.7, // Conservative default
		}
	}

	// Base prediction on action type statistics
	predictedScore := stats.MeanEffectiveness
	confidence := 0.6

	// Adjust prediction based on pattern features
	factorContributions := make(map[string]float64)

	// Severity adjustment
	severityAdjustment := m.getSeverityAdjustment(pattern.AlertSeverity)
	predictedScore += severityAdjustment
	factorContributions["severity_adjustment"] = severityAdjustment

	// Namespace adjustment
	namespaceAdjustment := m.getNamespaceAdjustment(pattern.Namespace)
	predictedScore += namespaceAdjustment
	factorContributions["namespace_adjustment"] = namespaceAdjustment

	// Time-based adjustment
	timeAdjustment := m.getTimeAdjustment(pattern.CreatedAt)
	predictedScore += timeAdjustment
	factorContributions["time_adjustment"] = timeAdjustment

	// Clamp prediction to valid range
	predictedScore = math.Max(0.0, math.Min(1.0, predictedScore))

	// Adjust confidence based on available data
	if stats.Count > 100 {
		confidence += 0.2
	} else if stats.Count < 10 {
		confidence -= 0.2
	}
	confidence = math.Max(0.1, math.Min(0.9, confidence))

	return &EffectivenessPrediction{
		PredictedScore:      predictedScore,
		Confidence:          confidence,
		FactorContributions: factorContributions,
		RiskFactors:         m.analyzeStatisticalRiskFactors(pattern, stats),
		Recommendations:     m.generateStatisticalRecommendations(pattern, stats, predictedScore),
		ModelUsed:           m.modelInfo.Name,
		PredictionTime:      time.Now(),
	}, nil
}

// Train the statistical model
func (m *StatisticalModel) Train(ctx context.Context, patterns []*vector.ActionPattern) error {
	if len(patterns) == 0 {
		return fmt.Errorf("no training data available")
	}

	// Reset statistics
	m.actionTypeStats = make(map[string]*ActionStatistics)

	// Calculate statistics for each action type
	actionGroups := make(map[string][]*vector.ActionPattern)
	var allScores []float64

	for _, pattern := range patterns {
		if pattern.EffectivenessData == nil {
			continue
		}

		actionGroups[pattern.ActionType] = append(actionGroups[pattern.ActionType], pattern)
		allScores = append(allScores, pattern.EffectivenessData.Score)
	}

	// Calculate global statistics
	m.globalStats = &GlobalStatistics{
		TotalPatterns:   len(allScores),
		OverallMean:     sharedmath.Mean(allScores),
		OverallStdDev:   sharedmath.StandardDeviation(allScores),
		SeasonalFactors: make(map[string]float64),
	}

	// Calculate action type statistics
	for actionType, actionPatterns := range actionGroups {
		var scores []float64
		successCount := 0

		for _, pattern := range actionPatterns {
			score := pattern.EffectivenessData.Score
			scores = append(scores, score)
			if score >= 0.6 {
				successCount++
			}
		}

		m.actionTypeStats[actionType] = &ActionStatistics{
			Count:               len(actionPatterns),
			MeanEffectiveness:   sharedmath.Mean(scores),
			StdDevEffectiveness: sharedmath.StandardDeviation(scores),
			SuccessRate:         float64(successCount) / float64(len(actionPatterns)),
			ParameterStats:      make(map[string]*ParameterStatistics),
		}
	}

	m.modelInfo.TrainedAt = time.Now()
	m.modelInfo.TrainingSize = len(patterns)
	m.modelInfo.Accuracy = m.calculateStatisticalAccuracy(patterns)
	m.trained = true

	m.log.WithFields(logrus.Fields{
		"training_size": len(patterns),
		"action_types":  len(m.actionTypeStats),
		"accuracy":      m.modelInfo.Accuracy,
	}).Info("Trained statistical model")

	return nil
}

// GetModelInfo returns information about the model
func (m *StatisticalModel) GetModelInfo() *ModelInfo {
	return m.modelInfo
}

// IsReady returns whether the model is ready for predictions
func (m *StatisticalModel) IsReady() bool {
	return m.trained
}

// Private methods for StatisticalModel

func (m *StatisticalModel) getSeverityAdjustment(severity string) float64 {
	adjustments := map[string]float64{
		"critical": -0.1, // Critical alerts might be harder to resolve effectively
		"warning":  0.0,  // Baseline
		"info":     0.1,  // Info alerts might be easier to handle
	}

	if adj, exists := adjustments[strings.ToLower(severity)]; exists {
		return adj
	}
	return 0.0
}

func (m *StatisticalModel) getNamespaceAdjustment(namespace string) float64 {
	adjustments := map[string]float64{
		"production":  -0.05, // More careful in production
		"kube-system": -0.1,  // System namespaces are critical
		"monitoring":  0.0,   // Baseline
		"development": 0.05,  // Less critical environment
		"staging":     0.02,  // Slightly less critical
	}

	if adj, exists := adjustments[strings.ToLower(namespace)]; exists {
		return adj
	}
	return 0.0
}

func (m *StatisticalModel) getTimeAdjustment(createdAt time.Time) float64 {
	if createdAt.IsZero() {
		return 0.0
	}

	// Weekend might have different effectiveness
	if createdAt.Weekday() == time.Saturday || createdAt.Weekday() == time.Sunday {
		return -0.02 // Slightly lower effectiveness on weekends
	}

	// Night time operations might be less effective
	hour := createdAt.Hour()
	if hour < 6 || hour > 22 {
		return -0.03 // Lower effectiveness during night hours
	}

	return 0.0
}

func (m *StatisticalModel) analyzeStatisticalRiskFactors(pattern *vector.ActionPattern, stats *ActionStatistics) []string {
	var riskFactors []string

	// Low success rate for this action type
	if stats.SuccessRate < 0.5 {
		riskFactors = append(riskFactors,
			fmt.Sprintf("Action type '%s' has low historical success rate (%.1f%%)",
				pattern.ActionType, stats.SuccessRate*100))
	}

	// High variability in effectiveness
	if stats.StdDevEffectiveness > 0.3 {
		riskFactors = append(riskFactors, "High variability in historical effectiveness for this action type")
	}

	// Limited historical data
	if stats.Count < 10 {
		riskFactors = append(riskFactors, "Limited historical data for this action type")
	}

	return riskFactors
}

func (m *StatisticalModel) generateStatisticalRecommendations(_ *vector.ActionPattern, stats *ActionStatistics, predictedScore float64) []string {
	var recommendations []string

	if predictedScore < 0.4 {
		recommendations = append(recommendations, "Statistical analysis suggests low effectiveness - consider alternative approaches")
	} else if predictedScore > 0.8 {
		recommendations = append(recommendations, "Statistical analysis indicates high likelihood of success")
	}

	if stats.StdDevEffectiveness > 0.2 {
		recommendations = append(recommendations, "High variability detected - ensure monitoring is in place")
	}

	if stats.SuccessRate > 0.8 {
		recommendations = append(recommendations, "This action type has high historical success rate")
	}

	return recommendations
}

func (m *StatisticalModel) calculateStatisticalAccuracy(patterns []*vector.ActionPattern) float64 {
	if len(patterns) < 5 {
		return 0.6 // Conservative estimate for small datasets
	}

	correct := 0
	total := 0

	// Use cross-validation approach similar to similarity model
	for i, testPattern := range patterns {
		if testPattern.EffectivenessData == nil {
			continue
		}

		// Create training set excluding test pattern
		var trainPatterns []*vector.ActionPattern
		for j, trainPattern := range patterns {
			if i != j && trainPattern.EffectivenessData != nil {
				trainPatterns = append(trainPatterns, trainPattern)
			}
		}

		if len(trainPatterns) == 0 {
			continue
		}

		// Calculate statistics for action type from training set
		actionTypePatterns := make([]*vector.ActionPattern, 0)
		for _, pattern := range trainPatterns {
			if pattern.ActionType == testPattern.ActionType {
				actionTypePatterns = append(actionTypePatterns, pattern)
			}
		}

		var predictedScore float64
		if len(actionTypePatterns) > 0 {
			// Use action-specific statistics
			var scores []float64
			for _, pattern := range actionTypePatterns {
				scores = append(scores, pattern.EffectivenessData.Score)
			}
			predictedScore = sharedmath.Mean(scores)
		} else {
			// Fallback to global statistics
			var allScores []float64
			for _, pattern := range trainPatterns {
				allScores = append(allScores, pattern.EffectivenessData.Score)
			}
			predictedScore = sharedmath.Mean(allScores)
		}

		// Apply contextual adjustments
		predictedScore += m.getSeverityAdjustment(testPattern.AlertSeverity)
		predictedScore += m.getNamespaceAdjustment(testPattern.Namespace)
		predictedScore += m.getTimeAdjustment(testPattern.CreatedAt)

		// Clamp to valid range
		predictedScore = math.Max(0.0, math.Min(1.0, predictedScore))

		// Compare with actual
		actualScore := testPattern.EffectivenessData.Score

		// Consider correct if within 25% (statistical models are generally less precise)
		if math.Abs(actualScore-predictedScore) <= 0.25 {
			correct++
		}
		total++
	}

	if total == 0 {
		return 0.6
	}

	accuracy := float64(correct) / float64(total)

	// Statistical models typically have lower accuracy than similarity-based models
	// Add a small penalty to reflect this
	return math.Max(0.3, accuracy*0.9)
}

// Utility functions
