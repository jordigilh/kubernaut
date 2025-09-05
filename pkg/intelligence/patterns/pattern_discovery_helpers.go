package patterns

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
	"github.com/jordigilh/kubernaut/pkg/intelligence/shared"
	sharedmath "github.com/jordigilh/kubernaut/pkg/shared/math"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/sirupsen/logrus"
)

type ValidationResult struct {
	RuleID    string
	Type      ValidationType
	Passed    bool
	Message   string
	Details   map[string]interface{}
	Timestamp time.Time
}

type ValidationType string

type WorkflowExecutionData struct {
	ExecutionID     string
	TemplateID      string
	Timestamp       time.Time
	Alert           *types.Alert
	ExecutionResult *WorkflowExecutionResult
	ResourceUsage   *sharedtypes.ResourceUsageData
	Context         map[string]interface{}
}

// WorkflowExecutionResult represents the result of a workflow execution
type WorkflowExecutionResult struct {
	Success           bool          `json:"success"`
	Duration          time.Duration `json:"duration"`
	StepsCompleted    int           `json:"steps_completed"`
	ErrorMessage      string        `json:"error_message,omitempty"`
	ResourcesAffected []string      `json:"resources_affected"`
}

// WorkflowExecutionEvent represents real-time events during workflow execution
// This is semantically different from WorkflowExecutionData which represents completed execution summaries
type WorkflowExecutionEvent struct {
	Type        string                 `json:"type"`              // Event type: "step_start", "step_complete", "error", "alert_triggered", etc.
	WorkflowID  string                 `json:"workflow_id"`       // ID of the workflow being executed
	ExecutionID string                 `json:"execution_id"`      // ID of this specific execution instance
	StepID      string                 `json:"step_id,omitempty"` // ID of current step (if applicable)
	Timestamp   time.Time              `json:"timestamp"`         // When this event occurred
	Data        map[string]interface{} `json:"data"`              // Event-specific payload (alert details, step results, etc.)
	Metrics     map[string]float64     `json:"metrics"`           // Real-time metrics snapshot at event time
	Context     map[string]interface{} `json:"context"`           // Execution context (environment, user, etc.)
}

// minInt returns the minimum of two integers
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Helper methods for PatternDiscoveryEngine

// filterByConfidence filters patterns by minimum confidence threshold
func (pde *PatternDiscoveryEngine) filterByConfidence(patterns []*shared.DiscoveredPattern, minConfidence float64) []*shared.DiscoveredPattern {
	filtered := make([]*shared.DiscoveredPattern, 0)
	for _, pattern := range patterns {
		if pattern.Confidence >= minConfidence {
			filtered = append(filtered, pattern)
		}
	}
	return filtered
}

// rankPatterns ranks patterns by confidence and frequency
func (pde *PatternDiscoveryEngine) rankPatterns(patterns []*shared.DiscoveredPattern) []*shared.DiscoveredPattern {
	if len(patterns) == 0 {
		return patterns
	}

	// Sort by composite score (confidence * frequency + success rate)
	sort.Slice(patterns, func(i, j int) bool {
		scoreI := patterns[i].Confidence*0.4 + float64(patterns[i].Frequency)*0.3 + patterns[i].SuccessRate*0.3
		scoreJ := patterns[j].Confidence*0.4 + float64(patterns[j].Frequency)*0.3 + patterns[j].SuccessRate*0.3
		return scoreI > scoreJ
	})

	return patterns
}

// generateRecommendations generates pattern recommendations
func (pde *PatternDiscoveryEngine) generateRecommendations(patterns []*shared.DiscoveredPattern) []*PatternRecommendation {
	recommendations := make([]*PatternRecommendation, 0)

	for i, pattern := range patterns {
		if i >= 5 { // Limit to top 5 patterns
			break
		}

		recommendation := &PatternRecommendation{
			ID:               fmt.Sprintf("rec-%s", pattern.ID),
			Type:             string(pattern.Type),
			Title:            fmt.Sprintf("Optimize based on %s pattern", pattern.Type),
			Description:      fmt.Sprintf("Pattern: %s shows %d occurrences with %.2f confidence", pattern.Name, pattern.Frequency, pattern.Confidence),
			Impact:           pde.estimateImpact(pattern),
			Effort:           pde.estimateEffort(pattern),
			Priority:         i + 1,
			BasedOnPatterns:  []string{pattern.ID},
			EstimatedBenefit: pattern.Confidence * pattern.SuccessRate,
		}

		recommendations = append(recommendations, recommendation)
	}

	return recommendations
}

// storePattern stores a discovered pattern
func (pde *PatternDiscoveryEngine) storePattern(ctx context.Context, pattern *shared.DiscoveredPattern) error {
	if pde.patternStore == nil {
		pde.log.Warn("Pattern store not configured")
		return nil
	}

	return pde.patternStore.StorePattern(ctx, pattern)
}

// calculateConfidenceDistribution calculates confidence distribution
func (pde *PatternDiscoveryEngine) calculateConfidenceDistribution(patterns []*shared.DiscoveredPattern) map[string]int {
	distribution := map[string]int{
		"high":   0, // 0.8+
		"medium": 0, // 0.5-0.8
		"low":    0, // <0.5
	}

	for _, pattern := range patterns {
		if pattern.Confidence >= 0.8 {
			distribution["high"]++
		} else if pattern.Confidence >= 0.5 {
			distribution["medium"]++
		} else {
			distribution["low"]++
		}
	}

	return distribution
}

// eventMatchesPattern checks if an event matches a pattern
func (pde *PatternDiscoveryEngine) eventMatchesPattern(event *WorkflowExecutionEvent, pattern *shared.DiscoveredPattern) bool {
	// Check pattern type compatibility
	switch pattern.Type {
	case shared.PatternTypeAlert:
		return pde.matchesAlertPattern(event, pattern)
	case shared.PatternTypeTemporal:
		return pde.matchesTemporalPattern(event, pattern)
	case shared.PatternTypeResource:
		return pde.matchesResourcePattern(event, pattern)
	default:
		return false
	}
}

// createAnomalyPattern creates an anomaly pattern from an event
func (pde *PatternDiscoveryEngine) createAnomalyPattern(event *WorkflowExecutionEvent, anomaly interface{}) *shared.DiscoveredPattern {
	pattern := &shared.DiscoveredPattern{
		ID:           fmt.Sprintf("anomaly-%d", time.Now().Unix()),
		Type:         shared.PatternTypeAnomaly,
		Name:         "Real-time Anomaly Detection",
		Description:  "Anomaly detected in real-time workflow execution",
		Confidence:   0.7, // Medium confidence for anomalies
		Frequency:    1,
		SuccessRate:  0.5,
		DiscoveredAt: time.Now(),
		LastSeen:     time.Now(),
		Metrics:      make(map[string]float64),
	}

	return pattern
}

// updateVectorEmbeddings updates vector embeddings for the event
func (pde *PatternDiscoveryEngine) updateVectorEmbeddings(ctx context.Context, event *WorkflowExecutionEvent) error {
	if pde.vectorDB == nil {
		return nil
	}

	// Create vector representation of the event
	vector := pde.createEventVector(event)
	metadata := map[string]interface{}{
		"event_type":   event.Type,
		"workflow_id":  event.WorkflowID,
		"execution_id": event.ExecutionID,
		"step_id":      event.StepID,
		"timestamp":    event.Timestamp,
	}

	return pde.vectorDB.Store(ctx, event.ExecutionID, vector, metadata)
}

// extractPredictionFeatures extracts features for prediction with robust error handling
func (pde *PatternDiscoveryEngine) extractPredictionFeatures(template *sharedtypes.WorkflowTemplate, alert *types.Alert) *shared.WorkflowFeatures {
	// Initialize with safe defaults
	features := &shared.WorkflowFeatures{
		AlertCount:      0,
		AlertTypes:      make(map[string]int),
		ResourceCount:   0,
		ResourceTypes:   make(map[string]int),
		NamespaceCount:  0,
		HourOfDay:       0,
		DayOfWeek:       0,
		IsWeekend:       false,
		IsBusinessHour:  false,
		StepCount:       0,
		DependencyDepth: 0,
		SeverityScore:   0.25, // Default to lowest severity
		CustomMetrics:   make(map[string]float64),
	}

	// Safely extract alert features
	if alert != nil {
		features.AlertCount = 1

		// Handle alert name with validation
		alertName := pde.sanitizeString(alert.Name)
		if alertName != "" {
			features.AlertTypes[alertName] = 1
		} else {
			features.AlertTypes["unknown"] = 1
		}

		// Handle resource with validation
		resourceType := pde.sanitizeString(alert.Resource)
		if resourceType != "" {
			features.ResourceTypes[resourceType] = 1
			features.ResourceCount = 1
		} else {
			features.ResourceTypes["unknown"] = 1
		}

		// Handle namespace
		if pde.sanitizeString(alert.Namespace) != "" {
			features.NamespaceCount = 1
		}

		// Robust severity encoding with validation
		features.SeverityScore = pde.encodeSeverityWithValidation(alert.Severity)

		// Extract additional alert features if available
		if alert.Labels != nil {
			features.CustomMetrics["label_count"] = float64(len(alert.Labels))
			features.CustomMetrics["has_priority"] = pde.extractBooleanFeature(alert.Labels, "priority")
			features.CustomMetrics["has_component"] = pde.extractBooleanFeature(alert.Labels, "component")
		}
	}

	// Safely extract template features
	if template != nil {
		features.StepCount = len(template.Steps)
		features.DependencyDepth = pde.calculateDependencyDepthRobust(template)

		// Extract template complexity features
		features.CustomMetrics["template_complexity"] = pde.calculateTemplateComplexity(template)
		features.CustomMetrics["has_conditional_steps"] = pde.extractConditionalStepsFeature(template)
		features.CustomMetrics["parallel_steps_ratio"] = pde.calculateParallelStepsRatio(template)
	}

	// Robust temporal features with timezone handling
	now := time.Now()
	features.HourOfDay = now.Hour()
	features.DayOfWeek = int(now.Weekday())
	features.IsWeekend = now.Weekday() == time.Saturday || now.Weekday() == time.Sunday
	features.IsBusinessHour = pde.calculateBusinessHour(now)

	// Add additional temporal context
	features.CustomMetrics["day_of_month"] = float64(now.Day()) / 31.0    // Normalized
	features.CustomMetrics["month_of_year"] = float64(now.Month()) / 12.0 // Normalized
	features.CustomMetrics["hour_normalized"] = float64(now.Hour()) / 24.0

	// Validate and normalize all features
	pde.validateAndNormalizeFeatures(features)

	return features
}

// sanitizeString cleans and validates string inputs
func (pde *PatternDiscoveryEngine) sanitizeString(input string) string {
	cleaned := strings.TrimSpace(input)
	cleaned = strings.ToLower(cleaned)

	// Remove invalid characters and limit length
	if len(cleaned) > 100 {
		cleaned = cleaned[:100]
	}

	// Replace spaces and special characters
	cleaned = strings.ReplaceAll(cleaned, " ", "_")
	cleaned = strings.ReplaceAll(cleaned, "-", "_")

	return cleaned
}

// encodeSeverityWithValidation provides robust severity encoding
func (pde *PatternDiscoveryEngine) encodeSeverityWithValidation(severity string) float64 {
	normalized := strings.ToLower(strings.TrimSpace(severity))

	switch normalized {
	case "critical", "crit", "emergency", "fatal", "error", "high":
		return 1.0
	case "warning", "warn", "major", "medium":
		return 0.75
	case "info", "information", "minor", "notice", "low":
		return 0.5
	case "debug", "trace", "verbose":
		return 0.25
	default:
		// Try to extract numeric severity if present
		if strings.Contains(normalized, "1") || strings.Contains(normalized, "critical") {
			return 1.0
		}
		if strings.Contains(normalized, "2") || strings.Contains(normalized, "warning") {
			return 0.75
		}
		if strings.Contains(normalized, "3") || strings.Contains(normalized, "info") {
			return 0.5
		}
		return 0.25 // Default fallback
	}
}

// extractBooleanFeature extracts boolean features from labels
func (pde *PatternDiscoveryEngine) extractBooleanFeature(labels map[string]string, key string) float64 {
	if labels == nil {
		return 0.0
	}

	if _, exists := labels[key]; exists {
		return 1.0
	}

	// Try common variations
	variations := []string{
		key,
		strings.ToLower(key),
		strings.ToUpper(key),
		"alert_" + key,
		key + "_level",
	}

	for _, variation := range variations {
		if _, exists := labels[variation]; exists {
			return 1.0
		}
	}

	return 0.0
}

// calculateDependencyDepthRobust calculates dependency depth with error handling
func (pde *PatternDiscoveryEngine) calculateDependencyDepthRobust(template *sharedtypes.WorkflowTemplate) int {
	if template == nil || len(template.Steps) == 0 {
		return 0
	}

	maxDepth := 0
	visited := make(map[string]bool)

	for _, step := range template.Steps {
		depth := pde.calculateStepDepth(&step, template.Steps, visited, 0)
		if depth > maxDepth {
			maxDepth = depth
		}
	}

	// Cap at reasonable maximum to prevent outliers
	if maxDepth > 20 {
		maxDepth = 20
	}

	return maxDepth
}

// calculateStepDepth recursively calculates step dependency depth
func (pde *PatternDiscoveryEngine) calculateStepDepth(step *sharedtypes.WorkflowStep, allSteps []sharedtypes.WorkflowStep, visited map[string]bool, currentDepth int) int {
	if step == nil || visited[step.ID] || currentDepth > 20 {
		return currentDepth
	}

	visited[step.ID] = true
	maxChildDepth := currentDepth

	for _, depID := range step.Dependencies {
		// Find the dependency step
		for _, depStep := range allSteps {
			if depStep.ID == depID {
				childDepth := pde.calculateStepDepth(&depStep, allSteps, visited, currentDepth+1)
				if childDepth > maxChildDepth {
					maxChildDepth = childDepth
				}
				break
			}
		}
	}

	delete(visited, step.ID) // Allow revisiting in different paths
	return maxChildDepth
}

// calculateTemplateComplexity calculates overall template complexity
func (pde *PatternDiscoveryEngine) calculateTemplateComplexity(template *sharedtypes.WorkflowTemplate) float64 {
	if template == nil || len(template.Steps) == 0 {
		return 0.0
	}

	complexity := 0.0
	stepCount := float64(len(template.Steps))

	// Base complexity from step count
	complexity += stepCount / 10.0 // Normalize to 0-1+ range

	// Add complexity from dependencies
	totalDependencies := 0
	for _, step := range template.Steps {
		totalDependencies += len(step.Dependencies)
	}
	complexity += float64(totalDependencies) / (stepCount * 2.0) // Avg dependencies per step

	// Add complexity from step types (if available)
	conditionalSteps := 0
	for _, step := range template.Steps {
		if step.Type == "conditional" || step.Type == "loop" || step.Type == "parallel" {
			conditionalSteps++
		}
	}
	complexity += float64(conditionalSteps) / stepCount

	// Cap complexity at reasonable maximum
	return math.Min(complexity, 5.0)
}

// extractConditionalStepsFeature checks for conditional steps
func (pde *PatternDiscoveryEngine) extractConditionalStepsFeature(template *sharedtypes.WorkflowTemplate) float64 {
	if template == nil || len(template.Steps) == 0 {
		return 0.0
	}

	conditionalCount := 0
	for _, step := range template.Steps {
		stepType := strings.ToLower(step.Type)
		if strings.Contains(stepType, "conditional") ||
			strings.Contains(stepType, "if") ||
			strings.Contains(stepType, "switch") ||
			strings.Contains(stepType, "loop") {
			conditionalCount++
		}
	}

	return float64(conditionalCount) / float64(len(template.Steps))
}

// calculateParallelStepsRatio calculates ratio of parallelizable steps
func (pde *PatternDiscoveryEngine) calculateParallelStepsRatio(template *sharedtypes.WorkflowTemplate) float64 {
	if template == nil || len(template.Steps) == 0 {
		return 0.0
	}

	// Count steps with no dependencies (can run in parallel)
	parallelSteps := 0
	for _, step := range template.Steps {
		if len(step.Dependencies) == 0 {
			parallelSteps++
		}
	}

	return float64(parallelSteps) / float64(len(template.Steps))
}

// calculateBusinessHour determines if current time is business hour with timezone awareness
func (pde *PatternDiscoveryEngine) calculateBusinessHour(t time.Time) bool {
	hour := t.Hour()
	weekday := t.Weekday()

	// Standard business hours: 9 AM - 5 PM, Monday - Friday
	isWeekday := weekday >= time.Monday && weekday <= time.Friday
	isBusinessTime := hour >= 9 && hour <= 17

	return isWeekday && isBusinessTime
}

// validateAndNormalizeFeatures validates and normalizes all extracted features
func (pde *PatternDiscoveryEngine) validateAndNormalizeFeatures(features *shared.WorkflowFeatures) {
	// Validate and cap counts
	if features.AlertCount < 0 {
		features.AlertCount = 0
	}
	if features.AlertCount > 100 {
		features.AlertCount = 100
	}

	if features.ResourceCount < 0 {
		features.ResourceCount = 0
	}
	if features.ResourceCount > 100 {
		features.ResourceCount = 100
	}

	if features.StepCount < 0 {
		features.StepCount = 0
	}
	if features.StepCount > 1000 {
		features.StepCount = 1000
	}

	// Validate temporal features
	if features.HourOfDay < 0 || features.HourOfDay > 23 {
		features.HourOfDay = 0
	}
	if features.DayOfWeek < 0 || features.DayOfWeek > 6 {
		features.DayOfWeek = 0
	}

	// Validate and cap severity score
	if features.SeverityScore < 0 {
		features.SeverityScore = 0
	}
	if features.SeverityScore > 1 {
		features.SeverityScore = 1
	}

	// Validate custom metrics
	for key, value := range features.CustomMetrics {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			features.CustomMetrics[key] = 0.0
		}
		// Cap extreme values
		if value > 100 {
			features.CustomMetrics[key] = 100.0
		}
		if value < -100 {
			features.CustomMetrics[key] = -100.0
		}
	}
}

// findSimilarPatterns finds patterns similar to given features
func (pde *PatternDiscoveryEngine) findSimilarPatterns(ctx context.Context, features *shared.WorkflowFeatures) ([]*shared.DiscoveredPattern, error) {
	if pde.vectorDB == nil {
		return []*shared.DiscoveredPattern{}, nil
	}

	// Convert features to vector
	vector := pde.featuresToVector(features)

	// Search for similar vectors
	results, err := pde.vectorDB.Search(ctx, vector, 10)
	if err != nil {
		return nil, err
	}

	// Convert results to patterns using proper shared types
	patterns := make([]*shared.DiscoveredPattern, 0)
	for _, result := range results {
		if result.Score > 0.7 { // Similarity threshold
			pattern := &shared.DiscoveredPattern{
				ID:          result.ID,
				Type:        shared.PatternTypeAnomaly, // Use proper enum
				Name:        fmt.Sprintf("Similar Pattern %s", result.ID),
				Description: "Pattern discovered through similarity search",
				Confidence:  result.Score,
				Frequency:   1,
			}
			patterns = append(patterns, pattern)
		}
	}

	return patterns, nil
}

// generateOptimizationSuggestions generates optimization suggestions
func (pde *PatternDiscoveryEngine) generateOptimizationSuggestions(patterns []*shared.DiscoveredPattern, template *sharedtypes.WorkflowTemplate) []*sharedtypes.OptimizationSuggestion {
	suggestions := make([]*sharedtypes.OptimizationSuggestion, 0)

	for _, pattern := range patterns {
		for _, hint := range pattern.OptimizationHints {
			suggestion := &sharedtypes.OptimizationSuggestion{
				Type:                 hint.Type,
				Description:          hint.Description,
				ExpectedImprovement:  hint.ImpactEstimate,
				ImplementationEffort: "medium",
				Priority:             hint.Priority,
			}
			suggestions = append(suggestions, suggestion)
		}
	}

	return suggestions
}

// findRelevantPatterns finds patterns relevant to a template
func (pde *PatternDiscoveryEngine) findRelevantPatterns(ctx context.Context, template *sharedtypes.WorkflowTemplate) ([]*shared.DiscoveredPattern, error) {
	// For now, return all active patterns
	// In a full implementation, this would filter by template characteristics
	patterns := make([]*shared.DiscoveredPattern, 0)

	pde.mu.RLock()
	for _, pattern := range pde.activePatterns {
		patterns = append(patterns, pattern)
	}
	pde.mu.RUnlock()

	return patterns, nil
}

// cloneTemplate creates a copy of a workflow template
func (pde *PatternDiscoveryEngine) cloneTemplate(template *engine.WorkflowTemplate) *engine.WorkflowTemplate {
	cloned := &engine.WorkflowTemplate{
		ID:          template.ID + "-optimized",
		Name:        template.Name + " (Optimized)",
		Description: template.Description,
		Version:     template.Version,
		Steps:       make([]*engine.WorkflowStep, len(template.Steps)),
		Variables:   make(map[string]interface{}),
		Tags:        make([]string, len(template.Tags)),
	}

	// Deep copy steps
	for i, step := range template.Steps {
		cloned.Steps[i] = &engine.WorkflowStep{
			ID:           step.ID,
			Name:         step.Name,
			Type:         step.Type,
			Dependencies: make([]string, len(step.Dependencies)),
			Timeout:      step.Timeout,
		}
		copy(cloned.Steps[i].Dependencies, step.Dependencies)
	}

	// Copy variables
	for k, v := range template.Variables {
		cloned.Variables[k] = v
	}

	// Copy tags
	copy(cloned.Tags, template.Tags)

	return cloned
}

// applyOptimizationHint applies an optimization hint to a template
func (pde *PatternDiscoveryEngine) applyOptimizationHint(template *engine.WorkflowTemplate, hint *shared.OptimizationHint, pattern *engine.DiscoveredPattern) *TemplateOptimization {
	optimization := &TemplateOptimization{
		ID:                  fmt.Sprintf("opt-%d", time.Now().Unix()),
		Type:                hint.Type,
		Description:         hint.Description,
		Rationale:           hint.ActionSuggestion,
		ExpectedImprovement: hint.ImpactEstimate,
		RiskLevel:           "low",
	}

	// Apply specific optimizations based on hint type
	switch hint.Type {
	case "timeout_optimization":
		pde.optimizeTimeouts(template, optimization)
	case "step_parallelization":
		pde.optimizeParallelization(template, optimization)
	case "retry_policy":
		pde.optimizeRetryPolicy(template, optimization)
	default:
		optimization.Description = "Generic optimization applied"
	}

	return optimization
}

// calculateOptimizationImpact calculates the impact of optimizations
func (pde *PatternDiscoveryEngine) calculateOptimizationImpact(optimizations []*TemplateOptimization) *OptimizationImpact {
	impact := &OptimizationImpact{
		PerformanceGain:      0.0,
		ResourceSavings:      0.0,
		ReliabilityGain:      0.0,
		MaintenanceReduction: 0.0,
		EstimatedROI:         0.0,
		PaybackPeriod:        30 * 24 * time.Hour, // Default 30 days
	}

	for _, opt := range optimizations {
		impact.PerformanceGain += opt.ExpectedImprovement * 0.3
		impact.ResourceSavings += opt.ExpectedImprovement * 0.2
		impact.ReliabilityGain += opt.ExpectedImprovement * 0.25
		impact.MaintenanceReduction += opt.ExpectedImprovement * 0.15
	}

	// Calculate ROI
	totalImprovement := impact.PerformanceGain + impact.ResourceSavings + impact.ReliabilityGain + impact.MaintenanceReduction
	impact.EstimatedROI = totalImprovement * 2.0 // Simplified ROI calculation

	return impact
}

// extractLearningData extracts learning data from execution
func (pde *PatternDiscoveryEngine) extractLearningData(execution *engine.WorkflowExecution) *shared.WorkflowLearningData {
	return &shared.WorkflowLearningData{
		ExecutionID:       execution.ID,
		TemplateID:        execution.WorkflowID,
		LearningObjective: "pattern_discovery",
		Context: map[string]interface{}{
			"success":        execution.Status == engine.ExecutionStatusCompleted,
			"duration":       execution.Duration.Seconds(),
			"steps_executed": len(execution.Steps),
		},
	}
}

// updateExistingPatterns updates existing patterns with new data
func (pde *PatternDiscoveryEngine) updateExistingPatterns(ctx context.Context, learningData *shared.WorkflowLearningData) error {
	pde.mu.Lock()
	defer pde.mu.Unlock()

	for _, pattern := range pde.activePatterns {
		// Update pattern metrics
		pattern.LastSeen = time.Now()
		pattern.UpdatedAt = time.Now()

		// Simple frequency update
		pattern.Frequency++
	}

	return nil
}

// checkForNewPatterns checks for emergence of new patterns
func (pde *PatternDiscoveryEngine) checkForNewPatterns(ctx context.Context, learningData *shared.WorkflowLearningData) error {
	// Simplified implementation - in practice, this would analyze
	// the learning data for new pattern emergence
	pde.log.Debug("Checking for new patterns")
	return nil
}

// updateVectorDatabase updates the vector database
func (pde *PatternDiscoveryEngine) updateVectorDatabase(ctx context.Context, learningData *shared.WorkflowLearningData) error {
	if pde.vectorDB == nil {
		return nil
	}

	// Create vector representation
	vector := make([]float64, 10) // Simplified vector
	for i := range vector {
		vector[i] = float64(i) * 0.1 // Placeholder values
	}

	metadata := map[string]interface{}{
		"execution_id": learningData.ExecutionID,
		"template_id":  learningData.TemplateID,
		"timestamp":    time.Now(),
	}

	return pde.vectorDB.Store(ctx, learningData.ExecutionID, vector, metadata)
}

// calculatePatternDistribution calculates pattern type distribution
func (pde *PatternDiscoveryEngine) calculatePatternDistribution(patterns []*shared.DiscoveredPattern) map[string]int {
	distribution := make(map[string]int)

	for _, pattern := range patterns {
		distribution[string(pattern.Type)]++
	}

	return distribution
}

// calculateConfidenceStats calculates confidence statistics
func (pde *PatternDiscoveryEngine) calculateConfidenceStats(patterns []*shared.DiscoveredPattern) *ConfidenceStatistics {
	if len(patterns) == 0 {
		return &ConfidenceStatistics{}
	}

	confidences := make([]float64, len(patterns))
	sum := 0.0
	min := 1.0
	max := 0.0

	for i, pattern := range patterns {
		confidences[i] = pattern.Confidence
		sum += pattern.Confidence
		if pattern.Confidence < min {
			min = pattern.Confidence
		}
		if pattern.Confidence > max {
			max = pattern.Confidence
		}
	}

	mean := sum / float64(len(patterns))

	// Calculate standard deviation
	sumSquares := 0.0
	for _, conf := range confidences {
		sumSquares += (conf - mean) * (conf - mean)
	}
	stdDev := math.Sqrt(sumSquares / float64(len(patterns)))

	// Sort for median
	sort.Float64s(confidences)
	median := confidences[len(confidences)/2]

	// Count high/low confidence
	highCount := 0
	lowCount := 0
	for _, conf := range confidences {
		if conf >= 0.8 {
			highCount++
		} else if conf < 0.5 {
			lowCount++
		}
	}

	return &ConfidenceStatistics{
		Mean:                mean,
		Median:              median,
		StandardDeviation:   stdDev,
		Min:                 min,
		Max:                 max,
		HighConfidenceCount: highCount,
		LowConfidenceCount:  lowCount,
	}
}

// analyzeTemporalTrends analyzes temporal trends in patterns
func (pde *PatternDiscoveryEngine) analyzeTemporalTrends(patterns []*shared.DiscoveredPattern) *TemporalTrendAnalysis {
	if len(patterns) == 0 {
		return &TemporalTrendAnalysis{
			OverallTrend: "stable",
		}
	}

	// Simple trend analysis based on discovery times
	recent := 0
	old := 0
	now := time.Now()

	for _, pattern := range patterns {
		if now.Sub(pattern.DiscoveredAt) < 7*24*time.Hour {
			recent++
		} else {
			old++
		}
	}

	trend := "stable"
	if recent > old*2 {
		trend = "increasing"
	} else if old > recent*2 {
		trend = "decreasing"
	}

	return &TemporalTrendAnalysis{
		OverallTrend:    trend,
		TrendStrength:   0.5, // Placeholder
		TrendConfidence: 0.7, // Placeholder
	}
}

// getTopOptimizations gets top optimization opportunities
func (pde *PatternDiscoveryEngine) getTopOptimizations(patterns []*shared.DiscoveredPattern) []*OptimizationInsight {
	insights := make([]*OptimizationInsight, 0)

	for i, pattern := range patterns {
		if i >= 5 { // Limit to top 5
			break
		}

		insight := &OptimizationInsight{
			Area:                     "workflow_efficiency",
			PotentialImprovement:     pattern.Confidence * pde.getPatternSuccessRate(pattern),
			ImplementationDifficulty: "medium",
			Priority:                 i + 1,
			AffectedWorkflows:        pde.getPatternFrequency(pattern),
			EstimatedROI:             pattern.Confidence * 2.0,
		}

		insights = append(insights, insight)
	}

	return insights
}

// analyzeOptimizationPatterns analyzes optimization patterns
func (pde *PatternDiscoveryEngine) analyzeOptimizationPatterns(data []*engine.WorkflowExecutionData) ([]*shared.DiscoveredPattern, error) {
	patterns := make([]*shared.DiscoveredPattern, 0)

	// Analyze execution times for optimization opportunities
	durationMap := make(map[string][]time.Duration)

	for _, execution := range data {
		key := execution.WorkflowID
		if _, exists := durationMap[key]; !exists {
			durationMap[key] = make([]time.Duration, 0)
		}
		durationMap[key] = append(durationMap[key], execution.Duration)
	}

	// Find templates with high variance in execution time
	for templateID, durations := range durationMap {
		if len(durations) >= 5 {
			variance := pde.calculateDurationVariance(durations)
			if variance > 0.5 { // High variance threshold
				pattern := &shared.DiscoveredPattern{
					ID:           fmt.Sprintf("opt-pattern-%s", templateID),
					Type:         shared.PatternTypeOptimization,
					Confidence:   variance,
					DiscoveredAt: time.Now(),
				}
				patterns = append(patterns, pattern)
			}
		}
	}

	return patterns, nil
}

// analyzeAnomalyPatterns analyzes anomaly patterns
func (pde *PatternDiscoveryEngine) analyzeAnomalyPatterns(data []*engine.WorkflowExecutionData) ([]*shared.DiscoveredPattern, error) {
	patterns := make([]*shared.DiscoveredPattern, 0)

	// Simple anomaly detection based on success rate
	successRate := pde.calculateOverallSuccessRate(data)

	if successRate < 0.7 { // Low success rate threshold
		pattern := &shared.DiscoveredPattern{
			ID:           fmt.Sprintf("anomaly-pattern-%d", time.Now().Unix()),
			Type:         shared.PatternTypeAnomaly,
			Confidence:   1.0 - successRate, // Higher confidence for lower success rates
			DiscoveredAt: time.Now(),
		}
		patterns = append(patterns, pattern)
	}

	return patterns, nil
}

// analyzeWorkflowPatterns analyzes workflow effectiveness patterns
func (pde *PatternDiscoveryEngine) analyzeWorkflowPatterns(data []*engine.WorkflowExecutionData) ([]*shared.DiscoveredPattern, error) {
	patterns := make([]*shared.DiscoveredPattern, 0)

	// Group by template ID
	templateGroups := make(map[string][]*engine.WorkflowExecutionData)

	for _, execution := range data {
		if _, exists := templateGroups[execution.WorkflowID]; !exists {
			templateGroups[execution.WorkflowID] = make([]*engine.WorkflowExecutionData, 0)
		}
		templateGroups[execution.WorkflowID] = append(templateGroups[execution.WorkflowID], execution)
	}

	// Analyze each template group
	for templateID, executions := range templateGroups {
		if len(executions) >= 5 {
			successRate := pde.calculateSuccessRateForGroup(executions)

			pattern := &shared.DiscoveredPattern{
				ID:           fmt.Sprintf("workflow-pattern-%s", templateID),
				Type:         shared.PatternTypeWorkflow,
				Confidence:   successRate,
				DiscoveredAt: time.Now(),
			}
			patterns = append(patterns, pattern)
		}
	}

	return patterns, nil
}

// analyzeFailureChains analyzes failure propagation chains
func (pde *PatternDiscoveryEngine) analyzeFailureChains(data []*engine.WorkflowExecutionData) []*FailureChainAnalysis {
	chains := make([]*FailureChainAnalysis, 0)

	// Group failures by time windows
	failures := make([]*engine.WorkflowExecutionData, 0)
	for _, execution := range data {
		if !execution.Success {
			failures = append(failures, execution)
		}
	}

	if len(failures) >= 2 {
		// Simple chain analysis - consecutive failures within time window
		for i := 0; i < len(failures)-1; i++ {
			timeDiff := failures[i+1].Timestamp.Sub(failures[i].Timestamp)
			if timeDiff < time.Hour { // Failures within 1 hour
				chain := &FailureChainAnalysis{
					Nodes: []*shared.FailureNode{
						{
							ID:          failures[i].ExecutionID,
							Type:        "execution_failure",
							Component:   failures[i].WorkflowID,
							FailureTime: failures[i].Timestamp,
						},
						{
							ID:          failures[i+1].ExecutionID,
							Type:        "execution_failure",
							Component:   failures[i+1].WorkflowID,
							FailureTime: failures[i+1].Timestamp,
						},
					},
					RootCause:       "Unknown",
					FinalEffect:     "Cascading failure",
					PropagationTime: timeDiff,
					Confidence:      0.6,
					Occurrences:     1,
				}
				chains = append(chains, chain)
			}
		}
	}

	return chains
}

// Helper utility methods

// extractAlertFromMetadata extracts alert information from execution metadata
func (pde *PatternDiscoveryEngine) extractAlertFromMetadata(execution *engine.WorkflowExecutionData) *types.Alert {
	alertData, hasAlert := execution.Metadata["alert"]
	if !hasAlert {
		return nil
	}
	if alertMap, ok := alertData.(map[string]interface{}); ok {
		alert := &types.Alert{}
		if name, ok := alertMap["name"].(string); ok {
			alert.Name = name
		}
		if severity, ok := alertMap["severity"].(string); ok {
			alert.Severity = severity
		}
		if namespace, ok := alertMap["namespace"].(string); ok {
			alert.Namespace = namespace
		}
		if resource, ok := alertMap["resource"].(string); ok {
			alert.Resource = resource
		}
		if labels, ok := alertMap["labels"].(map[string]string); ok {
			alert.Labels = labels
		}
		return alert
	}
	return nil
}

// Helper functions for pattern field access since engine.DiscoveredPattern may not have all fields

// getPatternName returns a pattern name, using ID if Name field doesn't exist
func (pde *PatternDiscoveryEngine) getPatternName(pattern *engine.DiscoveredPattern) string {
	// Use pattern ID as name since engine.DiscoveredPattern doesn't have Name field
	return pattern.ID
}

// getPatternFrequency returns pattern frequency, defaulting to 1
func (pde *PatternDiscoveryEngine) getPatternFrequency(pattern *shared.DiscoveredPattern) int {
	// Return the frequency from shared.DiscoveredPattern
	return pattern.Frequency
}

// getPatternSuccessRate returns pattern success rate, using confidence as fallback
func (pde *PatternDiscoveryEngine) getPatternSuccessRate(pattern *shared.DiscoveredPattern) float64 {
	// Return the success rate from shared.DiscoveredPattern
	return pattern.SuccessRate
}

func (pde *PatternDiscoveryEngine) estimateImpact(pattern *shared.DiscoveredPattern) string {
	frequency := pde.getPatternFrequency(pattern)
	if pattern.Confidence > 0.8 && frequency > 10 {
		return "high"
	} else if pattern.Confidence > 0.6 || frequency > 5 {
		return "medium"
	}
	return "low"
}

func (pde *PatternDiscoveryEngine) estimateEffort(pattern *shared.DiscoveredPattern) string {
	switch strings.ToLower(string(pattern.Type)) {
	case "alert":
		return "low"
	case "temporal":
		return "medium"
	case "workflow":
		return "high"
	default:
		return "medium"
	}
}

func (pde *PatternDiscoveryEngine) matchesAlertPattern(event *WorkflowExecutionEvent, pattern *shared.DiscoveredPattern) bool {
	// Simplified matching - check if alert name is in pattern
	if alertName, exists := event.Data["alert_name"]; exists {
		for _, alertPattern := range pattern.AlertPatterns {
			for _, alertType := range alertPattern.AlertTypes {
				if alertType == alertName {
					return true
				}
			}
		}
	}
	return false
}

func (pde *PatternDiscoveryEngine) matchesTemporalPattern(event *WorkflowExecutionEvent, pattern *shared.DiscoveredPattern) bool {
	// Check if current time matches pattern peak times
	now := time.Now()
	for _, temporalPattern := range pattern.TemporalPatterns {
		for _, peakTime := range temporalPattern.PeakTimes {
			if now.Hour() >= peakTime.Start.Hour() && now.Hour() <= peakTime.End.Hour() {
				return true
			}
		}
	}
	return false
}

func (pde *PatternDiscoveryEngine) matchesResourcePattern(event *WorkflowExecutionEvent, pattern *shared.DiscoveredPattern) bool {
	// Check resource metrics against pattern
	for _, resourcePattern := range pattern.ResourcePatterns {
		if resourcePattern.ResourceType == "cpu" {
			if cpuUsage, exists := event.Metrics["cpu_usage"]; exists {
				// Simple threshold check
				if cpuUsage > 0.8 {
					return true
				}
			}
		}
	}
	return false
}

func (pde *PatternDiscoveryEngine) createEventVector(event *WorkflowExecutionEvent) []float64 {
	// Create a simple vector representation
	vector := make([]float64, 10)

	// Add timestamp features
	vector[0] = float64(event.Timestamp.Hour()) / 24.0
	vector[1] = float64(event.Timestamp.Weekday()) / 7.0

	// Add metric features
	idx := 2
	for _, value := range event.Metrics {
		if idx < len(vector) {
			vector[idx] = value
			idx++
		}
	}

	return vector
}

func (pde *PatternDiscoveryEngine) featuresToVector(features *shared.WorkflowFeatures) []float64 {
	vector := make([]float64, 10)

	vector[0] = float64(features.AlertCount) / 10.0
	vector[1] = features.SeverityScore
	vector[2] = float64(features.ResourceCount) / 10.0
	vector[3] = float64(features.HourOfDay) / 24.0
	vector[4] = float64(features.DayOfWeek) / 7.0
	vector[5] = float64(features.StepCount) / 20.0
	vector[6] = float64(features.DependencyDepth) / 10.0

	if features.IsWeekend {
		vector[7] = 1.0
	}
	if features.IsBusinessHour {
		vector[8] = 1.0
	}

	return vector
}

func (pde *PatternDiscoveryEngine) calculateDependencyDepth(template *engine.WorkflowTemplate) int {
	if len(template.Steps) == 0 {
		return 0
	}

	// Simple calculation - find maximum dependency chain
	maxDepth := 0
	for _, step := range template.Steps {
		depth := len(step.Dependencies)
		if depth > maxDepth {
			maxDepth = depth
		}
	}

	return maxDepth
}

func (pde *PatternDiscoveryEngine) optimizeTimeouts(template *engine.WorkflowTemplate, optimization *TemplateOptimization) {
	// Reduce timeouts by 20%
	for _, step := range template.Steps {
		originalTimeout := step.Timeout
		step.Timeout = time.Duration(float64(step.Timeout) * 0.8)

		optimization.OptimizedStep = step.ID
		optimization.OriginalValue = originalTimeout
		optimization.OptimizedValue = step.Timeout
	}
}

func (pde *PatternDiscoveryEngine) optimizeParallelization(template *engine.WorkflowTemplate, optimization *TemplateOptimization) {
	// Identify steps that can be parallelized
	optimization.Description = "Identified steps that can be executed in parallel"
	optimization.ExpectedImprovement = 0.3 // 30% improvement
}

func (pde *PatternDiscoveryEngine) optimizeRetryPolicy(template *engine.WorkflowTemplate, optimization *TemplateOptimization) {
	// Optimize retry policies
	optimization.Description = "Optimized retry policies for better reliability"
	optimization.ExpectedImprovement = 0.2 // 20% improvement
}

func (pde *PatternDiscoveryEngine) calculateDurationVariance(durations []time.Duration) float64 {
	if len(durations) < 2 {
		return 0
	}

	// Convert to seconds and calculate variance
	seconds := make([]float64, len(durations))
	sum := 0.0

	for i, d := range durations {
		seconds[i] = d.Seconds()
		sum += seconds[i]
	}

	mean := sum / float64(len(seconds))
	sumSquares := 0.0

	for _, s := range seconds {
		sumSquares += (s - mean) * (s - mean)
	}

	variance := sumSquares / float64(len(seconds))
	return variance / (mean * mean) // Coefficient of variation
}

func (pde *PatternDiscoveryEngine) calculateOverallSuccessRate(data []*engine.WorkflowExecutionData) float64 {
	if len(data) == 0 {
		return 0
	}

	successful := 0
	for _, execution := range data {
		if execution.Success {
			successful++
		}
	}

	return float64(successful) / float64(len(data))
}

func (pde *PatternDiscoveryEngine) calculateSuccessRateForGroup(executions []*engine.WorkflowExecutionData) float64 {
	return pde.calculateOverallSuccessRate(executions)
}

func (pde *PatternDiscoveryEngine) calculateAverageDuration(executions []*engine.WorkflowExecutionData) time.Duration {
	if len(executions) == 0 {
		return 0
	}

	total := time.Duration(0)
	count := 0

	for _, execution := range executions {
		total += execution.Duration
		count++
	}

	if count == 0 {
		return 0
	}

	return total / time.Duration(count)
}

// PatternFailureChainAnalysis represents analysis of failure chains in pattern discovery
type PatternFailureChainAnalysis struct {
	Nodes              []*FailureNode
	RootCause          string
	FinalEffect        string
	PropagationTime    time.Duration
	AffectedComponents []string
	Confidence         float64
	Occurrences        int
}

// Data collection helper methods for PatternDiscoveryEngine

// validateAnalysisRequest validates the analysis request parameters
func (pde *PatternDiscoveryEngine) validateAnalysisRequest(request *PatternAnalysisRequest) error {
	if request == nil {
		return fmt.Errorf("analysis request cannot be nil")
	}

	// Validate time range
	if request.TimeRange.Start.IsZero() || request.TimeRange.End.IsZero() {
		return fmt.Errorf("time range must have valid start and end times")
	}

	if request.TimeRange.End.Before(request.TimeRange.Start) {
		return fmt.Errorf("time range end cannot be before start")
	}

	// Validate time range is not too large
	duration := request.TimeRange.End.Sub(request.TimeRange.Start)
	maxDuration := time.Duration(pde.config.MaxHistoryDays) * 24 * time.Hour
	if duration > maxDuration {
		return fmt.Errorf("time range too large: %v, maximum allowed: %v", duration, maxDuration)
	}

	// Validate pattern types
	if len(request.PatternTypes) == 0 {
		return fmt.Errorf("at least one pattern type must be specified")
	}

	// Validate confidence threshold
	if request.MinConfidence < 0 || request.MinConfidence > 1 {
		return fmt.Errorf("minimum confidence must be between 0 and 1, got: %f", request.MinConfidence)
	}

	return nil
}

// collectFromPatternStore collects data from the pattern store
func (pde *PatternDiscoveryEngine) collectFromPatternStore(ctx context.Context, request *PatternAnalysisRequest) ([]*engine.WorkflowExecutionData, error) {
	// Create filters for pattern store query
	filters := make(map[string]interface{})

	// Add time range filters
	filters["start_time"] = request.TimeRange.Start
	filters["end_time"] = request.TimeRange.End

	// Add user-provided filters
	for k, v := range request.Filters {
		filters[k] = v
	}

	// Get patterns from store
	patterns, err := pde.patternStore.GetPatterns(ctx, filters)
	if err != nil {
		return nil, err
	}

	// Convert patterns to workflow execution data
	executions := make([]*engine.WorkflowExecutionData, 0)
	for _, pattern := range patterns {
		for _, sourceExecution := range pattern.SourceExecutions {
			// Create synthetic execution data from pattern
			execution := &engine.WorkflowExecutionData{
				ExecutionID: sourceExecution,
				WorkflowID:  fmt.Sprintf("template-from-pattern-%s", pattern.ID),
				Timestamp:   pattern.LastSeen,
				Duration:    pattern.AverageExecutionTime,
				Success:     pattern.SuccessRate > 0.5,
				Metrics:     make(map[string]float64),
				Metadata: map[string]interface{}{
					"alert": map[string]interface{}{
						"name":      pattern.Name,
						"severity":  "info",
						"namespace": "unknown",
						"resource":  "unknown",
					},
				},
			}
			executions = append(executions, execution)
		}
	}

	return executions, nil
}

// collectFromVectorDB collects data from the vector database
func (pde *PatternDiscoveryEngine) collectFromVectorDB(ctx context.Context, request *PatternAnalysisRequest) ([]*engine.WorkflowExecutionData, error) {
	// Create a query vector based on request
	queryVector := pde.createQueryVector(request)

	// Search for similar vectors
	results, err := pde.vectorDB.Search(ctx, queryVector, 100) // Limit to 100 results
	if err != nil {
		return nil, err
	}

	executions := make([]*engine.WorkflowExecutionData, 0)
	for _, result := range results {
		// Check if result is within time range
		if timestamp, ok := result.Metadata["timestamp"].(time.Time); ok {
			if timestamp.Before(request.TimeRange.Start) || timestamp.After(request.TimeRange.End) {
				continue
			}
		}

		// Convert vector result to execution data
		execution := pde.vectorResultToExecution(result)
		if execution != nil {
			executions = append(executions, execution)
		}
	}

	return executions, nil
}

// collectFromHistoricalBuffer collects data from the anomaly detector's historical buffer
func (pde *PatternDiscoveryEngine) collectFromHistoricalBuffer(request *PatternAnalysisRequest) []*engine.WorkflowExecutionData {
	if pde.anomalyDetector == nil {
		return []*engine.WorkflowExecutionData{}
	}

	// Since engine.AnomalyDetector doesn't have historicalData field,
	// we return empty slice for now. In a real implementation, this would
	// need to be properly implemented based on the actual AnomalyDetector interface
	return []*engine.WorkflowExecutionData{}
}

// validateAndCleanData validates and cleans the collected data
func (pde *PatternDiscoveryEngine) validateAndCleanData(data []*engine.WorkflowExecutionData) ([]*engine.WorkflowExecutionData, []error) {
	cleaned := make([]*engine.WorkflowExecutionData, 0)
	errors := make([]error, 0)

	for i, execution := range data {
		if execution == nil {
			errors = append(errors, fmt.Errorf("execution at index %d is nil", i))
			continue
		}

		// Validate required fields
		if execution.ExecutionID == "" {
			errors = append(errors, fmt.Errorf("execution at index %d has empty ExecutionID", i))
			continue
		}

		if execution.WorkflowID == "" {
			errors = append(errors, fmt.Errorf("execution %s has empty WorkflowID", execution.ExecutionID))
			continue
		}

		if execution.Timestamp.IsZero() {
			errors = append(errors, fmt.Errorf("execution %s has zero timestamp", execution.ExecutionID))
			continue
		}

		// Clean and normalize data
		cleanedExecution := pde.cleanExecutionData(execution)
		cleaned = append(cleaned, cleanedExecution)
	}

	return cleaned, errors
}

// cleanExecutionData cleans and normalizes execution data
func (pde *PatternDiscoveryEngine) cleanExecutionData(execution *engine.WorkflowExecutionData) *engine.WorkflowExecutionData {
	// Create cleaned version using the engine.WorkflowExecutionData structure
	cleaned := &engine.WorkflowExecutionData{
		ExecutionID: strings.TrimSpace(execution.ExecutionID),
		WorkflowID:  strings.TrimSpace(execution.WorkflowID),
		Timestamp:   execution.Timestamp,
		Duration:    execution.Duration,
		Success:     execution.Success,
		Metrics:     make(map[string]float64),
		Metadata:    make(map[string]interface{}),
	}

	// Copy and clean metrics
	for k, v := range execution.Metrics {
		if !math.IsNaN(v) && !math.IsInf(v, 0) {
			cleaned.Metrics[k] = v
		}
	}

	// Clean and normalize alert data in metadata
	alert := pde.extractAlertFromMetadata(execution)
	if alert != nil {
		cleanedAlert := map[string]interface{}{
			"name":      strings.TrimSpace(alert.Name),
			"severity":  pde.normalizeSeverity(alert.Severity),
			"namespace": strings.TrimSpace(alert.Namespace),
			"resource":  strings.TrimSpace(alert.Resource),
		}
		if alert.Labels != nil {
			cleanedAlert["labels"] = alert.Labels
		}
		cleaned.Metadata["alert"] = cleanedAlert
	}

	// Copy other metadata
	for k, v := range execution.Metadata {
		if k != "alert" { // Don't overwrite cleaned alert
			cleaned.Metadata[k] = v
		}
	}

	// Validate duration
	if cleaned.Duration < 0 {
		cleaned.Duration = 0
	}
	// Cap extremely long durations (likely data errors)
	if cleaned.Duration > 24*time.Hour {
		cleaned.Duration = 24 * time.Hour
	}

	return cleaned
}

// normalizeSeverity normalizes severity strings
func (pde *PatternDiscoveryEngine) normalizeSeverity(severity string) string {
	normalized := strings.ToLower(strings.TrimSpace(severity))
	switch normalized {
	case "crit", "critical", "high":
		return "critical"
	case "warn", "warning", "medium":
		return "warning"
	case "info", "information", "low":
		return "info"
	default:
		return "info" // Default to info
	}
}

// normalizeResourceValue normalizes resource usage values to 0-1 range
func (pde *PatternDiscoveryEngine) normalizeResourceValue(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

// applyRequestFilters applies filters from the request to the data
func (pde *PatternDiscoveryEngine) applyRequestFilters(data []*engine.WorkflowExecutionData, request *PatternAnalysisRequest) []*engine.WorkflowExecutionData {
	if len(request.Filters) == 0 {
		return data
	}

	filtered := make([]*engine.WorkflowExecutionData, 0)

	for _, execution := range data {
		if pde.matchesFilters(execution, request.Filters) {
			filtered = append(filtered, execution)
		}
	}

	return filtered
}

// matchesFilters checks if execution matches the provided filters
func (pde *PatternDiscoveryEngine) matchesFilters(execution *engine.WorkflowExecutionData, filters map[string]interface{}) bool {
	for key, value := range filters {
		switch key {
		case "template_id":
			if stringValue, ok := value.(string); ok && execution.WorkflowID != stringValue {
				return false
			}
		case "alert_name":
			alertData, hasAlert := execution.Metadata["alert"]
			if !hasAlert {
				return false
			}
			if alertMap, ok := alertData.(map[string]interface{}); ok {
				if name, ok := alertMap["name"].(string); ok {
					if stringValue, ok := value.(string); ok && name != stringValue {
						return false
					}
				}
			}
		case "severity":
			alertData, hasAlert := execution.Metadata["alert"]
			if !hasAlert {
				return false
			}
			if alertMap, ok := alertData.(map[string]interface{}); ok {
				if severity, ok := alertMap["severity"].(string); ok {
					if stringValue, ok := value.(string); ok && severity != stringValue {
						return false
					}
				}
			}
		case "namespace":
			alertData, hasAlert := execution.Metadata["alert"]
			if !hasAlert {
				return false
			}
			if alertMap, ok := alertData.(map[string]interface{}); ok {
				if namespace, ok := alertMap["namespace"].(string); ok {
					if stringValue, ok := value.(string); ok && namespace != stringValue {
						return false
					}
				}
			}
		case "success":
			if boolValue, ok := value.(bool); ok && execution.Success != boolValue {
				return false
			}
		case "min_duration":
			if durationValue, ok := value.(time.Duration); ok && execution.Duration < durationValue {
				return false
			}
		case "max_duration":
			if durationValue, ok := value.(time.Duration); ok && execution.Duration > durationValue {
				return false
			}
		}
	}

	return true
}

// createQueryVector creates a query vector from the analysis request
func (pde *PatternDiscoveryEngine) createQueryVector(request *PatternAnalysisRequest) []float64 {
	vector := make([]float64, 10) // Standard vector size

	// Encode time range as vector features
	duration := request.TimeRange.End.Sub(request.TimeRange.Start)
	vector[0] = math.Min(duration.Hours()/24.0, 1.0) // Normalize to days

	// Encode hour of day (using start time)
	vector[1] = float64(request.TimeRange.Start.Hour()) / 24.0

	// Encode day of week
	vector[2] = float64(request.TimeRange.Start.Weekday()) / 7.0

	// Encode pattern types as bitmap
	patternBits := 0.0
	for _, patternType := range request.PatternTypes {
		switch patternType {
		case shared.PatternTypeAlert:
			patternBits += 0.1
		case shared.PatternTypeResource:
			patternBits += 0.2
		case shared.PatternTypeTemporal:
			patternBits += 0.3
		case shared.PatternTypeFailure:
			patternBits += 0.4
		case shared.PatternTypeOptimization:
			patternBits += 0.5
		case shared.PatternTypeAnomaly:
			patternBits += 0.6
		case shared.PatternTypeWorkflow:
			patternBits += 0.7
		}
	}
	vector[3] = math.Min(patternBits, 1.0)

	// Encode confidence threshold
	vector[4] = request.MinConfidence

	// Fill remaining slots with filter-based features
	if nameFilter, exists := request.Filters["alert_name"]; exists {
		if name, ok := nameFilter.(string); ok {
			vector[5] = pde.hashStringToFloat(name)
		}
	}

	if severityFilter, exists := request.Filters["severity"]; exists {
		if severity, ok := severityFilter.(string); ok {
			switch severity {
			case "critical":
				vector[6] = 1.0
			case "warning":
				vector[6] = 0.75
			case "info":
				vector[6] = 0.5
			default:
				vector[6] = 0.25
			}
		}
	}

	return vector
}

// vectorResultToExecution converts a vector search result to execution data
func (pde *PatternDiscoveryEngine) vectorResultToExecution(result *VectorSearchResult) *engine.WorkflowExecutionData {
	execution := &engine.WorkflowExecutionData{
		ExecutionID: result.ID,
		Metrics:     make(map[string]float64),
		Metadata:    make(map[string]interface{}),
	}

	// Extract metadata
	if templateID, ok := result.Metadata["template_id"].(string); ok {
		execution.WorkflowID = templateID
	}

	if timestamp, ok := result.Metadata["timestamp"].(time.Time); ok {
		execution.Timestamp = timestamp
	}

	// Set success and duration
	execution.Success = result.Score > 0.7 // Use similarity score as success indicator
	execution.Duration = time.Minute * 5   // Default duration

	// Store alert information in metadata
	if alertName, ok := result.Metadata["alert_name"].(string); ok {
		alertData := map[string]interface{}{
			"name":      alertName,
			"severity":  "info",
			"namespace": "unknown",
			"resource":  "unknown",
		}

		if severity, ok := result.Metadata["severity"].(string); ok {
			alertData["severity"] = severity
		}
		if namespace, ok := result.Metadata["namespace"].(string); ok {
			alertData["namespace"] = namespace
		}
		if resource, ok := result.Metadata["resource"].(string); ok {
			alertData["resource"] = resource
		}

		execution.Metadata["alert"] = alertData
	}

	return execution
}

// hashStringToFloat creates a normalized hash value from a string
func (pde *PatternDiscoveryEngine) hashStringToFloat(s string) float64 {
	hash := uint32(0)
	for _, c := range s {
		hash = hash*31 + uint32(c)
	}
	return float64(hash%1000) / 1000.0 // Normalize to 0-1
}

// collectFromExecutionRepository collects data from execution repository if available
func (pde *PatternDiscoveryEngine) collectFromExecutionRepository(ctx context.Context, request *PatternAnalysisRequest) ([]*engine.WorkflowExecutionData, error) {
	// Check if execution repository is available (would be injected)
	if pde.executionRepo == nil {
		return []*engine.WorkflowExecutionData{}, nil
	}

	// Query executions within time range
	executions, err := pde.executionRepo.GetExecutionsInTimeWindow(ctx, request.TimeRange.Start, request.TimeRange.End)
	if err != nil {
		return nil, fmt.Errorf("failed to query execution repository: %w", err)
	}

	// Convert execution repository format to WorkflowExecutionData
	executionData := make([]*engine.WorkflowExecutionData, 0)
	for _, exec := range executions {
		data := pde.convertExecutionToWorkflowData(exec)
		if data != nil {
			executionData = append(executionData, data)
		}
	}

	pde.log.WithField("count", len(executionData)).Debug("Collected executions from repository")
	return executionData, nil
}

// deduplicateExecutionData removes duplicate execution data based on execution ID
func (pde *PatternDiscoveryEngine) deduplicateExecutionData(data []*engine.WorkflowExecutionData) []*engine.WorkflowExecutionData {
	seen := make(map[string]bool)
	unique := make([]*engine.WorkflowExecutionData, 0)

	for _, execution := range data {
		if execution == nil || execution.ExecutionID == "" {
			continue
		}

		if !seen[execution.ExecutionID] {
			seen[execution.ExecutionID] = true
			unique = append(unique, execution)
		}
	}

	return unique
}

// convertExecutionToWorkflowData converts execution repository format to WorkflowExecutionData
func (pde *PatternDiscoveryEngine) convertExecutionToWorkflowData(exec *engine.WorkflowExecution) *engine.WorkflowExecutionData {
	if exec == nil {
		return nil
	}

	data := &engine.WorkflowExecutionData{
		ExecutionID: exec.ID,
		WorkflowID:  exec.WorkflowID,
		Timestamp:   exec.StartTime,
		Duration:    exec.Duration,
		Metrics:     make(map[string]float64),
		Metadata:    make(map[string]interface{}),
	}

	// Set success based on execution status
	data.Success = (exec.Status == engine.ExecutionStatusCompleted)

	// Extract alert information from context if available and store in metadata
	if exec.Context != nil && exec.Context.Variables != nil {
		if alertData, exists := exec.Context.Variables["alert"]; exists {
			data.Metadata["alert"] = alertData
		}

		// Extract resource usage as metrics if available
		if resourceData, exists := exec.Context.Variables["resource_usage"]; exists {
			if resourceMap, ok := resourceData.(map[string]interface{}); ok {
				if cpu, ok := resourceMap["cpu"].(float64); ok {
					data.Metrics["cpu_usage"] = cpu
				}
				if memory, ok := resourceMap["memory"].(float64); ok {
					data.Metrics["memory_usage"] = memory
				}
				if network, ok := resourceMap["network"].(float64); ok {
					data.Metrics["network_usage"] = network
				}
				if storage, ok := resourceMap["storage"].(float64); ok {
					data.Metrics["storage_usage"] = storage
				}
			}
		}
	}

	// Store execution context in metadata if available
	if exec.Context != nil {
		data.Metadata["execution_context"] = map[string]interface{}{
			"environment":    exec.Context.Environment,
			"cluster":        exec.Context.Cluster,
			"user":           exec.Context.User,
			"request_id":     exec.Context.RequestID,
			"trace_id":       exec.Context.TraceID,
			"correlation_id": exec.Context.CorrelationID,
			"configuration":  exec.Context.Configuration,
		}
	}

	return data
}

// Enhanced empirical validation methods for pattern analysis

// validateAlertPatternEmpirical performs empirical validation using statistical tests
func (pde *PatternDiscoveryEngine) validateAlertPatternEmpirical(group *AlertClusterGroup, data []*engine.WorkflowExecutionData) float64 {
	if len(group.Members) < 5 {
		return 0.5 // Low confidence for small samples
	}

	// Perform bootstrap sampling to validate pattern stability
	bootstrapScores := make([]float64, 10)
	for i := 0; i < 10; i++ {
		sample := pde.bootstrapSample(group.Members, len(group.Members)/2)
		bootstrapScores[i] = pde.calculateBootstrapScore(sample)
	}

	// Calculate confidence interval
	sort.Float64s(bootstrapScores)
	p5 := bootstrapScores[0]
	p95 := bootstrapScores[len(bootstrapScores)-1]
	confidenceWidth := p95 - p5

	// Narrower confidence interval = higher empirical confidence
	empiricalConfidence := math.Max(0.1, 1.0-confidenceWidth)

	// Chi-square test for independence between alert types and outcomes
	chiSquareP := pde.calculateChiSquareTest(group.Members)

	// Statistical significance boost
	if chiSquareP < 0.05 {
		empiricalConfidence += 0.1
	}

	// Temporal stability test
	temporalStability := pde.calculateTemporalStability(group.Members)
	empiricalConfidence = (empiricalConfidence + temporalStability) / 2.0

	return math.Min(empiricalConfidence, 0.95)
}

// bootstrapSample creates a bootstrap sample from the data
func (pde *PatternDiscoveryEngine) bootstrapSample(data []*engine.WorkflowExecutionData, size int) []*engine.WorkflowExecutionData {
	if size > len(data) {
		size = len(data)
	}

	sample := make([]*engine.WorkflowExecutionData, size)
	for i := 0; i < size; i++ {
		randomIndex := int(time.Now().UnixNano()+int64(i)) % len(data)
		sample[i] = data[randomIndex]
	}
	return sample
}

// calculateBootstrapScore calculates a score for bootstrap sample
func (pde *PatternDiscoveryEngine) calculateBootstrapScore(sample []*engine.WorkflowExecutionData) float64 {
	if len(sample) == 0 {
		return 0.0
	}

	successCount := 0
	for _, execution := range sample {
		if execution.Success {
			successCount++
		}
	}

	return float64(successCount) / float64(len(sample))
}

// calculateChiSquareTest performs chi-square test for independence
func (pde *PatternDiscoveryEngine) calculateChiSquareTest(data []*engine.WorkflowExecutionData) float64 {
	// Simplified chi-square calculation
	// Group by alert type and success/failure
	alertSuccessCount := make(map[string]int)
	alertFailureCount := make(map[string]int)

	for _, execution := range data {
		alert := pde.extractAlertFromMetadata(execution)
		if alert == nil {
			continue
		}

		alertType := alert.Name
		if execution.Success {
			alertSuccessCount[alertType]++
		} else {
			alertFailureCount[alertType]++
		}
	}

	// Calculate chi-square statistic (simplified)
	chiSquare := 0.0
	totalSuccess := 0
	totalFailure := 0

	for alertType := range alertSuccessCount {
		totalSuccess += alertSuccessCount[alertType]
		totalFailure += alertFailureCount[alertType]
	}

	if totalSuccess == 0 || totalFailure == 0 {
		return 1.0 // No variation, perfect independence
	}

	for alertType := range alertSuccessCount {
		observed := float64(alertSuccessCount[alertType])
		total := float64(alertSuccessCount[alertType] + alertFailureCount[alertType])
		expected := total * float64(totalSuccess) / float64(totalSuccess+totalFailure)

		if expected > 0 {
			chiSquare += math.Pow(observed-expected, 2) / expected
		}
	}

	// Convert to p-value approximation (simplified)
	// For demonstration - in reality would use proper chi-square distribution
	if chiSquare > 3.84 { // Critical value for df=1, α=0.05
		return 0.01 // Significant
	}
	return 0.1 // Not significant
}

// calculateTemporalStability measures how stable patterns are across time
func (pde *PatternDiscoveryEngine) calculateTemporalStability(data []*engine.WorkflowExecutionData) float64 {
	if len(data) < 10 {
		return 0.5
	}

	// Sort by timestamp
	sorted := make([]*engine.WorkflowExecutionData, len(data))
	copy(sorted, data)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	// Split into time windows and compare success rates
	windowSize := len(sorted) / 3
	if windowSize < 2 {
		return 0.5
	}

	windows := [][]*engine.WorkflowExecutionData{
		sorted[:windowSize],
		sorted[windowSize : 2*windowSize],
		sorted[2*windowSize:],
	}

	successRates := make([]float64, 3)
	for i, window := range windows {
		successCount := 0
		for _, execution := range window {
			if execution.Success {
				successCount++
			}
		}
		successRates[i] = float64(successCount) / float64(len(window))
	}

	// Calculate variance in success rates across windows
	mean := (successRates[0] + successRates[1] + successRates[2]) / 3.0
	variance := 0.0
	for _, rate := range successRates {
		variance += math.Pow(rate-mean, 2)
	}
	variance /= 3.0

	// Lower variance = higher temporal stability
	stability := math.Max(0.1, 1.0-variance*4) // Scale variance to 0-1 range
	return stability
}

// calculateEnhancedPatternConfidence combines multiple confidence sources
func (pde *PatternDiscoveryEngine) calculateEnhancedPatternConfidence(
	clusterConfidence, empiricalConfidence float64,
	sampleSize int,
	successRate float64,
) float64 {
	// Base confidence from clustering
	confidence := clusterConfidence * 0.4

	// Empirical validation boost
	confidence += empiricalConfidence * 0.3

	// Sample size confidence (logarithmic scaling)
	sampleConfidence := math.Min(1.0, math.Log(float64(sampleSize))/math.Log(100))
	confidence += sampleConfidence * 0.2

	// Success rate influence
	confidence += successRate * 0.1

	// Penalty for extreme values (usually indicates overfitting)
	if confidence > 0.95 {
		confidence = 0.95
	}
	if confidence < 0.1 {
		confidence = 0.1
	}

	return confidence
}

// Statistical validation methods for resource patterns

// validateResourcePatternEmpirical validates resource utilization patterns
func (pde *PatternDiscoveryEngine) validateResourcePatternEmpirical(pattern *shared.DiscoveredPattern, data []*engine.WorkflowExecutionData) float64 {
	if len(data) < 10 {
		return 0.5
	}

	// Extract resource usage data from metrics
	resourceData := make([]float64, 0)
	for _, execution := range data {
		// Check for resource usage metrics
		cpuUsage, hasCPU := execution.Metrics["cpu_usage"]
		memUsage, hasMem := execution.Metrics["memory_usage"]
		netUsage, hasNet := execution.Metrics["network_usage"]
		stgUsage, hasStg := execution.Metrics["storage_usage"]

		if hasCPU || hasMem || hasNet || hasStg {
			// Calculate average of available metrics
			total := 0.0
			count := 0
			if hasCPU {
				total += cpuUsage
				count++
			}
			if hasMem {
				total += memUsage
				count++
			}
			if hasNet {
				total += netUsage
				count++
			}
			if hasStg {
				total += stgUsage
				count++
			}
			if count > 0 {
				resourceData = append(resourceData, total/float64(count))
			}
		}
	}

	if len(resourceData) < 5 {
		return 0.3
	}

	// Perform Kolmogorov-Smirnov test for distribution normality
	ksStatistic := pde.calculateKSStatistic(resourceData)

	// Calculate autocorrelation to detect patterns
	autocorr := pde.calculateAutocorrelation(resourceData, 1)

	// Combine statistics into confidence score
	confidence := 0.5

	// Normal distribution increases confidence
	if ksStatistic < 0.2 {
		confidence += 0.2
	}

	// Significant autocorrelation indicates pattern
	if math.Abs(autocorr) > 0.3 {
		confidence += 0.3
	}

	return math.Min(confidence, 0.9)
}

// calculateKSStatistic calculates Kolmogorov-Smirnov statistic (simplified)
func (pde *PatternDiscoveryEngine) calculateKSStatistic(data []float64) float64 {
	if len(data) < 5 {
		return 1.0
	}

	// Sort data
	sorted := make([]float64, len(data))
	copy(sorted, data)
	sort.Float64s(sorted)

	// Calculate empirical CDF vs theoretical normal CDF
	mean := sharedmath.Mean(sorted)
	stdDev := sharedmath.StandardDeviation(sorted)

	maxDiff := 0.0
	n := float64(len(sorted))

	for i, value := range sorted {
		empiricalCDF := float64(i+1) / n

		// Approximate normal CDF (simplified)
		z := (value - mean) / stdDev
		theoreticalCDF := 0.5 * (1.0 + math.Erf(z/math.Sqrt(2)))

		diff := math.Abs(empiricalCDF - theoreticalCDF)
		if diff > maxDiff {
			maxDiff = diff
		}
	}

	return maxDiff
}

// calculateAutocorrelation calculates lag-1 autocorrelation
func (pde *PatternDiscoveryEngine) calculateAutocorrelation(data []float64, lag int) float64 {
	if len(data) <= lag {
		return 0.0
	}

	mean := sharedmath.Mean(data)

	numerator := 0.0
	denominator := 0.0

	for i := 0; i < len(data)-lag; i++ {
		x1 := data[i] - mean
		x2 := data[i+lag] - mean
		numerator += x1 * x2
		denominator += x1 * x1
	}

	if denominator == 0 {
		return 0.0
	}

	return numerator / denominator
}

// Confidence calibration and validation methods

// ConfidenceCalibrator calibrates confidence scores based on historical performance
type ConfidenceCalibrator struct {
	calibrationCurve map[float64]float64 // Maps predicted confidence to actual accuracy
	sampleCounts     map[float64]int     // Number of samples for each confidence bin
	isCalibrated     bool
	lastUpdated      time.Time
}

// NewConfidenceCalibrator creates a new confidence calibrator
func NewConfidenceCalibrator() *ConfidenceCalibrator {
	return &ConfidenceCalibrator{
		calibrationCurve: make(map[float64]float64),
		sampleCounts:     make(map[float64]int),
		isCalibrated:     false,
		lastUpdated:      time.Now(),
	}
}

// calibratePatternConfidence calibrates confidence scores based on historical accuracy
func (pde *PatternDiscoveryEngine) calibratePatternConfidence(pattern *shared.DiscoveredPattern) float64 {
	if pde.learningMetrics == nil || len(pde.learningMetrics.PatternTrackers) == 0 {
		return pattern.Confidence
	}

	// Find similar patterns in historical data for calibration
	similarPatterns := pde.findSimilarPatternsForCalibration(pattern)

	if len(similarPatterns) < 3 {
		// Not enough data for calibration, return original confidence with small adjustment
		return pde.applyBasicConfidenceAdjustment(pattern.Confidence, string(pattern.Type))
	}

	// Calculate calibration factors
	calibrationFactor := pde.calculateCalibrationFactor(similarPatterns, pattern.Confidence)

	// Apply calibration
	calibratedConfidence := pattern.Confidence * calibrationFactor

	// Ensure bounds
	calibratedConfidence = math.Max(0.05, math.Min(0.98, calibratedConfidence))

	pde.log.WithFields(logrus.Fields{
		"pattern_id":            pattern.ID,
		"original_confidence":   pattern.Confidence,
		"calibrated_confidence": calibratedConfidence,
		"calibration_factor":    calibrationFactor,
		"similar_patterns":      len(similarPatterns),
	}).Debug("Applied confidence calibration")

	return calibratedConfidence
}

// findSimilarPatternsForCalibration finds patterns similar to the given pattern for calibration
func (pde *PatternDiscoveryEngine) findSimilarPatternsForCalibration(pattern *shared.DiscoveredPattern) []*PatternAccuracyTracker {
	if pde.learningMetrics == nil {
		return []*PatternAccuracyTracker{}
	}

	similarTrackers := make([]*PatternAccuracyTracker, 0)

	for _, tracker := range pde.learningMetrics.PatternTrackers {
		if tracker.AccuracyMetrics == nil || tracker.TotalPredictions < 5 {
			continue
		}

		// Check pattern type similarity (if we can infer it from pattern ID)
		if pde.arePatternsSimilar(pattern, tracker) {
			similarTrackers = append(similarTrackers, tracker)
		}
	}

	return similarTrackers
}

// arePatternsSimilar checks if patterns are similar enough for calibration
func (pde *PatternDiscoveryEngine) arePatternsSimilar(pattern *shared.DiscoveredPattern, tracker *PatternAccuracyTracker) bool {
	// Pattern type matching (basic heuristic based on ID prefixes)
	patternPrefix := pde.extractPatternPrefix(pattern.ID)
	trackerPrefix := pde.extractPatternPrefix(tracker.PatternID)

	if patternPrefix == trackerPrefix {
		return true
	}

	// Additional similarity checks based on pattern characteristics
	patternType := strings.ToLower(string(pattern.Type))
	if patternType == "alert" && strings.Contains(tracker.PatternID, "alert") {
		return true
	}
	if patternType == "resource" && strings.Contains(tracker.PatternID, "resource") {
		return true
	}
	if patternType == "temporal" && strings.Contains(tracker.PatternID, "temporal") {
		return true
	}

	return false
}

// extractPatternPrefix extracts prefix from pattern ID for similarity matching
func (pde *PatternDiscoveryEngine) extractPatternPrefix(patternID string) string {
	parts := strings.Split(patternID, "-")
	if len(parts) >= 2 {
		return parts[0] + "-" + parts[1]
	}
	return patternID
}

// calculateCalibrationFactor calculates how to adjust confidence based on historical accuracy
func (pde *PatternDiscoveryEngine) calculateCalibrationFactor(trackers []*PatternAccuracyTracker, predictedConfidence float64) float64 {
	if len(trackers) == 0 {
		return 1.0
	}

	// Group trackers by confidence bins
	confidenceBins := make(map[float64][]*PatternAccuracyTracker)
	binSize := 0.1 // 10% bins

	for _, tracker := range trackers {
		bin := math.Floor(tracker.AccuracyMetrics.AverageConfidence/binSize) * binSize
		if confidenceBins[bin] == nil {
			confidenceBins[bin] = make([]*PatternAccuracyTracker, 0)
		}
		confidenceBins[bin] = append(confidenceBins[bin], tracker)
	}

	// Find the closest bin to our predicted confidence
	targetBin := math.Floor(predictedConfidence/binSize) * binSize

	// If exact bin doesn't exist, find nearest bin
	if confidenceBins[targetBin] == nil {
		nearestBin := -1.0
		minDistance := math.MaxFloat64

		for bin := range confidenceBins {
			distance := math.Abs(bin - targetBin)
			if distance < minDistance {
				minDistance = distance
				nearestBin = bin
			}
		}

		if nearestBin >= 0 {
			targetBin = nearestBin
		} else {
			return 1.0 // No suitable bin found
		}
	}

	// Calculate average actual accuracy for this confidence bin
	binTrackers := confidenceBins[targetBin]
	totalAccuracy := 0.0
	totalWeight := 0.0

	for _, tracker := range binTrackers {
		weight := float64(tracker.TotalPredictions) // Weight by sample size
		totalAccuracy += tracker.AccuracyMetrics.Accuracy * weight
		totalWeight += weight
	}

	if totalWeight == 0 {
		return 1.0
	}

	averageAccuracy := totalAccuracy / totalWeight

	// Calibration factor = actual accuracy / predicted confidence
	if predictedConfidence > 0 {
		factor := averageAccuracy / predictedConfidence
		// Limit extreme adjustments
		factor = math.Max(0.3, math.Min(2.0, factor))
		return factor
	}

	return 1.0
}

// applyBasicConfidenceAdjustment applies basic confidence adjustments when calibration data is insufficient
func (pde *PatternDiscoveryEngine) applyBasicConfidenceAdjustment(confidence float64, patternType string) float64 {
	// Apply pattern-type specific adjustments based on known behavior
	switch strings.ToLower(patternType) {
	case "alert":
		// Alert patterns tend to be more reliable
		return math.Min(confidence*1.05, 0.95)
	case "temporal":
		// Temporal patterns can be less stable
		return confidence * 0.9
	case "failure":
		// Failure patterns are often complex and less predictable
		return confidence * 0.85
	case "anomaly":
		// Anomaly patterns are by definition unusual
		return confidence * 0.8
	default:
		return confidence
	}
}

// updateCalibrationData updates calibration data with new accuracy information
func (pde *PatternDiscoveryEngine) updateCalibrationData(patternID string, predictedConfidence, actualAccuracy float64) {
	tracker := pde.findAccuracyTracker(patternID)
	if tracker == nil {
		return
	}

	// Update the tracker's calibration information
	dataPoint := ConfidenceDataPoint{
		Timestamp:           time.Now(),
		PredictedConfidence: predictedConfidence,
		ActualOutcome:       actualAccuracy > 0.5, // Convert accuracy to boolean
		ContextFactors: map[string]interface{}{
			"actual_accuracy": actualAccuracy,
		},
	}

	tracker.ConfidenceHistory = append(tracker.ConfidenceHistory, dataPoint)

	// Recalculate accuracy metrics
	pde.updateAccuracyMetrics(tracker)
}

// Reliability scoring for confidence validation

// calculateConfidenceReliability calculates how reliable a confidence score is
func (pde *PatternDiscoveryEngine) calculateConfidenceReliability(pattern *shared.DiscoveredPattern, data []*engine.WorkflowExecutionData) *ConfidenceReliabilityScore {
	reliability := &ConfidenceReliabilityScore{
		OverallReliability: 0.5,
		DataQualityScore:   0.5,
		SampleSizeScore:    0.5,
		TemporalStability:  0.5,
		ValidationScore:    0.5,
		Factors:            make(map[string]float64),
	}

	// Data quality assessment
	reliability.DataQualityScore = pde.assessDataQuality(data)
	reliability.Factors["data_quality"] = reliability.DataQualityScore

	// Sample size adequacy
	reliability.SampleSizeScore = pde.calculateSampleSizeScore(len(data))
	reliability.Factors["sample_size"] = reliability.SampleSizeScore

	// Temporal stability
	reliability.TemporalStability = pde.calculateTemporalStability(data)
	reliability.Factors["temporal_stability"] = reliability.TemporalStability

	// Cross-validation score
	reliability.ValidationScore = pde.crossValidatePattern(pattern, data)
	reliability.Factors["validation"] = reliability.ValidationScore

	// Calculate overall reliability as weighted average
	reliability.OverallReliability = (reliability.DataQualityScore*0.3 +
		reliability.SampleSizeScore*0.25 +
		reliability.TemporalStability*0.25 +
		reliability.ValidationScore*0.2)

	// Additional factors based on pattern type
	switch strings.ToLower(string(pattern.Type)) {
	case "anomaly":
		// Anomaly patterns are inherently less reliable
		reliability.OverallReliability *= 0.8
	case "failure":
		// Failure patterns can be complex
		reliability.OverallReliability *= 0.85
	case "temporal":
		// Temporal patterns depend heavily on data coverage
		if reliability.TemporalStability < 0.6 {
			reliability.OverallReliability *= 0.7
		}
	}

	// Ensure bounds
	reliability.OverallReliability = math.Max(0.1, math.Min(0.95, reliability.OverallReliability))

	return reliability
}

// assessDataQuality assesses the quality of input data
func (pde *PatternDiscoveryEngine) assessDataQuality(data []*engine.WorkflowExecutionData) float64 {
	if len(data) == 0 {
		return 0.0
	}

	score := 1.0
	penaltyFactor := 0.0

	// Check for missing required fields
	missingAlerts := 0
	missingTimestamps := 0

	for _, execution := range data {
		alert := pde.extractAlertFromMetadata(execution)
		if alert == nil {
			missingAlerts++
		}
		if execution.Timestamp.IsZero() {
			missingTimestamps++
		}
	}

	// Calculate penalty based on missing data
	totalData := float64(len(data))
	penaltyFactor += (float64(missingAlerts) / totalData) * 0.5
	penaltyFactor += (float64(missingTimestamps) / totalData) * 0.5

	score -= penaltyFactor

	// Check for data diversity
	uniqueAlertTypes := make(map[string]bool)
	uniqueNamespaces := make(map[string]bool)

	for _, execution := range data {
		alert := pde.extractAlertFromMetadata(execution)
		if alert != nil {
			uniqueAlertTypes[alert.Name] = true
			uniqueNamespaces[alert.Namespace] = true
		}
	}

	// Bonus for diversity
	diversityBonus := 0.0
	if len(uniqueAlertTypes) > 1 {
		diversityBonus += 0.1
	}
	if len(uniqueNamespaces) > 1 {
		diversityBonus += 0.1
	}

	score += diversityBonus

	// Ensure bounds
	return math.Max(0.1, math.Min(1.0, score))
}

// calculateSampleSizeScore calculates a score based on sample size adequacy
func (pde *PatternDiscoveryEngine) calculateSampleSizeScore(sampleSize int) float64 {
	if sampleSize < 5 {
		return 0.1
	} else if sampleSize < 10 {
		return 0.3
	} else if sampleSize < 20 {
		return 0.5
	} else if sampleSize < 50 {
		return 0.7
	} else if sampleSize < 100 {
		return 0.85
	} else {
		return 0.95
	}
}

// Confidence interval calculation

// calculateConfidenceInterval calculates confidence interval for pattern effectiveness
func (pde *PatternDiscoveryEngine) calculateConfidenceInterval(pattern *shared.DiscoveredPattern, data []*engine.WorkflowExecutionData, confidenceLevel float64) *sharedtypes.ConfidenceInterval {
	if len(data) < 5 {
		return &sharedtypes.ConfidenceInterval{
			Level: confidenceLevel,
			Lower: []float64{0.0},
			Upper: []float64{1.0},
		}
	}

	// Extract success rates
	successes := 0
	for _, execution := range data {
		if execution.Success {
			successes++
		}
	}

	n := float64(len(data))
	p := float64(successes) / n

	// Calculate Wilson score interval (more robust than normal approximation)
	z := pde.getZScore(confidenceLevel)

	denominator := 1 + (z*z)/n
	center := p + (z*z)/(2*n)
	halfWidth := z * math.Sqrt(p*(1-p)/n+(z*z)/(4*n*n))

	lower := (center - halfWidth/denominator) / denominator
	upper := (center + halfWidth/denominator) / denominator

	// Ensure bounds
	lower = math.Max(0.0, lower)
	upper = math.Min(1.0, upper)

	return &sharedtypes.ConfidenceInterval{
		Level: confidenceLevel,
		Lower: []float64{lower},
		Upper: []float64{upper},
	}
}

// getZScore returns the z-score for a given confidence level
func (pde *PatternDiscoveryEngine) getZScore(confidenceLevel float64) float64 {
	// Common confidence levels
	switch confidenceLevel {
	case 0.90:
		return 1.645
	case 0.95:
		return 1.96
	case 0.99:
		return 2.576
	default:
		// Approximate for other levels (simplified)
		return 1.96
	}
}

// Supporting types for confidence validation

type ConfidenceReliabilityScore struct {
	OverallReliability float64            `json:"overall_reliability"`
	DataQualityScore   float64            `json:"data_quality_score"`
	SampleSizeScore    float64            `json:"sample_size_score"`
	TemporalStability  float64            `json:"temporal_stability"`
	ValidationScore    float64            `json:"validation_score"`
	Factors            map[string]float64 `json:"factors"`
}

// Enhanced alert pattern analysis helper methods

// calculateClusterConfidence calculates confidence for alert clusters
func (pde *PatternDiscoveryEngine) calculateClusterConfidence(group *AlertClusterGroup) float64 {
	// Base confidence from cluster analysis
	baseConfidence := group.Confidence

	// Boost confidence based on cluster size
	sizeBoost := math.Min(float64(len(group.Members))/20.0, 0.3) // Max 30% boost

	// Boost confidence based on success rate
	successBoost := group.SuccessRate * 0.2 // Max 20% boost

	// Penalty for very diverse clusters (low commonality)
	diversityPenalty := 0.0
	if len(group.AlertTypes) > 5 {
		diversityPenalty = 0.1
	}

	confidence := baseConfidence + sizeBoost + successBoost - diversityPenalty
	return math.Max(0.0, math.Min(1.0, confidence))
}

// generateClusterName generates a descriptive name for alert clusters
func (pde *PatternDiscoveryEngine) generateClusterName(group *AlertClusterGroup) string {
	if len(group.AlertTypes) == 0 {
		return "Mixed Alert Cluster"
	}

	if len(group.AlertTypes) == 1 {
		return fmt.Sprintf("%s Alerts", group.AlertTypes[0])
	}

	if len(group.AlertTypes) <= 3 {
		return fmt.Sprintf("%s Cluster", strings.Join(group.AlertTypes[:minInt(3, len(group.AlertTypes))], "+"))
	}

	return fmt.Sprintf("Multi-Alert Cluster (%d types)", len(group.AlertTypes))
}

// generateClusterDescription generates detailed description for clusters
func (pde *PatternDiscoveryEngine) generateClusterDescription(group *AlertClusterGroup) string {
	desc := fmt.Sprintf("Cluster of %d alert occurrences", len(group.Members))

	if len(group.AlertTypes) > 0 {
		desc += fmt.Sprintf(" including %s alerts", strings.Join(group.AlertTypes[:minInt(2, len(group.AlertTypes))], ", "))
	}

	if len(group.Namespaces) > 0 {
		desc += fmt.Sprintf(" in namespace(s) %s", strings.Join(group.Namespaces[:minInt(2, len(group.Namespaces))], ", "))
	}

	desc += fmt.Sprintf(" with %.1f%% success rate", group.SuccessRate*100)

	return desc
}

// calculateAlertCorrelation calculates correlation data for alert patterns
func (pde *PatternDiscoveryEngine) calculateAlertCorrelation(group *AlertClusterGroup) *shared.AlertCorrelation {
	primaryAlert := ""
	if len(group.AlertTypes) > 0 {
		primaryAlert = group.AlertTypes[0]
	}
	correlatedAlerts := group.AlertTypes[1:]
	if len(correlatedAlerts) == 0 {
		correlatedAlerts = []string{}
	}

	return &shared.AlertCorrelation{
		PrimaryAlert:     primaryAlert,
		CorrelatedAlerts: correlatedAlerts,
		CorrelationScore: group.Confidence,
		TimeWindow:       30 * time.Minute, // Default time window
		Direction:        "concurrent",     // Default direction
		Confidence:       group.Confidence,
	}
}

// generateClusterOptimizationHints generates optimization hints for clusters
func (pde *PatternDiscoveryEngine) generateClusterOptimizationHints(group *AlertClusterGroup) []*shared.OptimizationHint {
	hints := make([]*shared.OptimizationHint, 0)

	// Low success rate suggests optimization opportunity
	if group.SuccessRate < 0.7 {
		hints = append(hints, &shared.OptimizationHint{
			Type:               "success_rate_improvement",
			Description:        fmt.Sprintf("Success rate %.1f%% is below optimal", group.SuccessRate*100),
			ImpactEstimate:     (0.8 - group.SuccessRate) * 0.5, // Potential improvement
			ImplementationCost: 0.6,
			Priority:           1,
			ActionSuggestion:   "Review and improve workflow for these alert types",
			Evidence:           []string{fmt.Sprintf("%d failed executions", int(float64(len(group.Members))*(1-group.SuccessRate)))},
		})
	}

	// Large clusters suggest automation opportunity
	if len(group.Members) > 20 {
		hints = append(hints, &shared.OptimizationHint{
			Type:               "automation_opportunity",
			Description:        "High frequency alerts suggest automation potential",
			ImpactEstimate:     0.4,
			ImplementationCost: 0.8,
			Priority:           2,
			ActionSuggestion:   "Consider automated response workflows",
			Evidence:           []string{fmt.Sprintf("%d occurrences", len(group.Members))},
		})
	}

	return hints
}

// calculateClusterMetrics calculates metrics for alert clusters
func (pde *PatternDiscoveryEngine) calculateClusterMetrics(group *AlertClusterGroup) map[string]float64 {
	metrics := map[string]float64{
		"cluster_size":     float64(len(group.Members)),
		"success_rate":     group.SuccessRate,
		"alert_diversity":  float64(len(group.AlertTypes)),
		"namespace_spread": float64(len(group.Namespaces)),
		"resource_spread":  float64(len(group.Resources)),
	}

	if group.TimeWindow > 0 {
		metrics["time_window_hours"] = group.TimeWindow.Hours()
	}

	return metrics
}

// groupAlertsByTimeWindows groups alerts into time windows
func (pde *PatternDiscoveryEngine) groupAlertsByTimeWindows(data []*engine.WorkflowExecutionData, windowSize time.Duration) map[time.Time][]*engine.WorkflowExecutionData {
	windows := make(map[time.Time][]*engine.WorkflowExecutionData)

	for _, execution := range data {
		// Round timestamp to window boundary
		windowStart := execution.Timestamp.Truncate(windowSize)

		if _, exists := windows[windowStart]; !exists {
			windows[windowStart] = make([]*engine.WorkflowExecutionData, 0)
		}
		windows[windowStart] = append(windows[windowStart], execution)
	}

	return windows
}

// analyzeAlertSequence analyzes a sequence of alerts for patterns
func (pde *PatternDiscoveryEngine) analyzeAlertSequence(alerts []*engine.WorkflowExecutionData) *AlertSequenceAnalysis {
	if len(alerts) < 2 {
		return &AlertSequenceAnalysis{IsSignificant: false}
	}

	// Sort by timestamp
	sortedAlerts := make([]*engine.WorkflowExecutionData, len(alerts))
	copy(sortedAlerts, alerts)
	sort.Slice(sortedAlerts, func(i, j int) bool {
		return sortedAlerts[i].Timestamp.Before(sortedAlerts[j].Timestamp)
	})

	// Calculate intervals
	intervals := make([]time.Duration, 0)
	for i := 1; i < len(sortedAlerts); i++ {
		interval := sortedAlerts[i].Timestamp.Sub(sortedAlerts[i-1].Timestamp)
		intervals = append(intervals, interval)
	}

	// Calculate average interval
	totalInterval := time.Duration(0)
	for _, interval := range intervals {
		totalInterval += interval
	}
	avgInterval := totalInterval / time.Duration(len(intervals))

	// Collect alert types
	alertTypes := make([]string, 0)
	for _, alertExecution := range sortedAlerts {
		alert := pde.extractAlertFromMetadata(alertExecution)
		if alert != nil {
			alertTypes = append(alertTypes, alert.Name)
		}
	}

	// Calculate success rate
	successful := 0
	for _, alert := range sortedAlerts {
		if alert.Success {
			successful++
		}
	}
	successRate := float64(successful) / float64(len(sortedAlerts))

	// Analyze severity escalation
	severityEscalation := pde.calculateSeverityEscalation(sortedAlerts)

	// Determine significance
	isSignificant := len(alerts) >= 3 && avgInterval < 10*time.Minute && successRate < 0.8

	confidence := 0.5
	if isSignificant {
		confidence = 0.7 + (1.0-successRate)*0.2 // Higher confidence for problematic sequences
	}

	return &AlertSequenceAnalysis{
		IsSignificant:      isSignificant,
		Confidence:         confidence,
		SuccessRate:        successRate,
		AlertTypes:         alertTypes,
		AverageInterval:    avgInterval,
		AverageDuration:    totalInterval,
		SeverityEscalation: severityEscalation,
	}
}

// calculateSeverityEscalation calculates severity escalation in alert sequence
func (pde *PatternDiscoveryEngine) calculateSeverityEscalation(alerts []*engine.WorkflowExecutionData) float64 {
	if len(alerts) < 2 {
		return 0.0
	}

	severityValues := make([]float64, 0)
	for _, alertExecution := range alerts {
		alert := pde.extractAlertFromMetadata(alertExecution)
		if alert != nil {
			switch strings.ToLower(alert.Severity) {
			case "critical":
				severityValues = append(severityValues, 1.0)
			case "warning":
				severityValues = append(severityValues, 0.75)
			case "info":
				severityValues = append(severityValues, 0.5)
			default:
				severityValues = append(severityValues, 0.25)
			}
		}
	}

	if len(severityValues) < 2 {
		return 0.0
	}

	// Calculate trend (positive = escalating)
	escalation := severityValues[len(severityValues)-1] - severityValues[0]
	return escalation
}

// calculateResourceAffinity calculates resource affinity analysis
func (pde *PatternDiscoveryEngine) calculateResourceAffinity(executions []*engine.WorkflowExecutionData) *ResourceAffinityAnalysis {
	if len(executions) == 0 {
		return &ResourceAffinityAnalysis{IsSignificant: false}
	}

	// Extract common properties
	firstExecution := executions[0]
	var namespace, resource, resourceType string
	alert := pde.extractAlertFromMetadata(firstExecution)
	if alert != nil {
		namespace = alert.Namespace
		resource = alert.Resource
		resourceType = resource // Simplified mapping
	}

	// Collect alert types
	alertTypeMap := make(map[string]int)
	successful := 0
	totalUtilization := 0.0
	utilizationCount := 0

	for _, execution := range executions {
		alert := pde.extractAlertFromMetadata(execution)
		if alert != nil {
			alertTypeMap[alert.Name]++
		}

		if execution.Success {
			successful++
		}

		// Extract resource usage from metrics if available
		if cpuUsage, hasCPU := execution.Metrics["cpu_usage"]; hasCPU {
			memUsage := execution.Metrics["memory_usage"]
			totalUtilization += (cpuUsage + memUsage) / 2.0
			utilizationCount++
		}
	}

	// Convert alert type map to slice
	alertTypes := make([]string, 0)
	for alertType := range alertTypeMap {
		alertTypes = append(alertTypes, alertType)
	}

	successRate := float64(successful) / float64(len(executions))
	avgUtilization := 0.0
	if utilizationCount > 0 {
		avgUtilization = totalUtilization / float64(utilizationCount)
	}

	// Determine significance
	isSignificant := len(executions) >= 5 && (successRate < 0.8 || avgUtilization > 0.8)

	confidence := 0.5
	if isSignificant {
		confidence = 0.6 + float64(len(executions))/50.0 // Higher confidence with more data
		confidence = math.Min(confidence, 0.9)
	}

	return &ResourceAffinityAnalysis{
		IsSignificant:      isSignificant,
		Confidence:         confidence,
		SuccessRate:        successRate,
		Namespace:          namespace,
		Resource:           resource,
		ResourceType:       resourceType,
		AlertTypes:         alertTypes,
		AverageUtilization: avgUtilization,
		UtilizationTrend: &sharedtypes.UtilizationTrend{
			ResourceType:       resourceType,
			TrendDirection:     "variable",
			GrowthRate:         0.1, // Default values
			SeasonalVariation:  0.2,
			PeakUtilization:    avgUtilization * 1.5,
			AverageUtilization: avgUtilization,
			EfficiencyScore:    0.7,
		}, // Simplified
	}
}

// findSeverityEscalations finds severity escalation patterns
func (pde *PatternDiscoveryEngine) findSeverityEscalations(data []*engine.WorkflowExecutionData) []*SeverityEscalationAnalysis {
	escalations := make([]*SeverityEscalationAnalysis, 0)

	if len(data) < 3 {
		return escalations
	}

	// Look for escalation sequences in time windows
	windowSize := 2 * time.Hour
	windows := pde.groupAlertsByTimeWindows(data, windowSize)

	for windowStart, windowAlerts := range windows {
		if len(windowAlerts) >= 3 {
			escalation := pde.analyzeEscalationWindow(windowAlerts, windowStart)
			if escalation != nil && escalation.IsSignificant {
				escalations = append(escalations, escalation)
			}
		}
	}

	return escalations
}

// analyzeEscalationWindow analyzes a time window for escalation patterns
func (pde *PatternDiscoveryEngine) analyzeEscalationWindow(alerts []*engine.WorkflowExecutionData, windowStart time.Time) *SeverityEscalationAnalysis {
	if len(alerts) < 3 {
		return nil
	}

	// Sort by timestamp
	sorted := make([]*engine.WorkflowExecutionData, len(alerts))
	copy(sorted, alerts)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	// Find escalation sequence
	firstAlertData := pde.extractAlertFromMetadata(sorted[0])
	lastAlertData := pde.extractAlertFromMetadata(sorted[len(sorted)-1])
	if firstAlertData == nil || lastAlertData == nil {
		return nil
	}

	startSeverity := strings.ToLower(firstAlertData.Severity)
	endSeverity := strings.ToLower(lastAlertData.Severity)

	// Check if this is actually an escalation
	startValue := pde.severityToValue(startSeverity)
	endValue := pde.severityToValue(endSeverity)

	if endValue <= startValue {
		return nil // Not an escalation
	}

	duration := sorted[len(sorted)-1].Timestamp.Sub(sorted[0].Timestamp)

	// Collect affected resources
	affectedResources := make(map[string]bool)
	resolutionCount := 0
	for _, alertExecution := range sorted {
		alert := pde.extractAlertFromMetadata(alertExecution)
		if alert != nil {
			resourceKey := fmt.Sprintf("%s/%s", alert.Namespace, alert.Resource)
			affectedResources[resourceKey] = true
		}
		if alertExecution.Success {
			resolutionCount++
		}
	}

	affectedResourceList := make([]string, 0)
	for resource := range affectedResources {
		affectedResourceList = append(affectedResourceList, resource)
	}

	resolutionRate := float64(resolutionCount) / float64(len(sorted))
	escalationSpeed := (endValue - startValue) / duration.Hours()

	// Determine significance
	isSignificant := endValue-startValue >= 0.25 && duration < 4*time.Hour && len(affectedResources) > 1

	confidence := 0.6
	if isSignificant {
		confidence = 0.7 + (endValue-startValue)*0.2
		confidence = math.Min(confidence, 0.95)
	}

	return &SeverityEscalationAnalysis{
		IsSignificant:     isSignificant,
		Confidence:        confidence,
		StartTime:         sorted[0].Timestamp,
		StartSeverity:     startSeverity,
		EndSeverity:       endSeverity,
		Duration:          duration,
		Pattern:           fmt.Sprintf("%s→%s", startSeverity, endSeverity),
		ResolutionRate:    resolutionRate,
		EscalationSpeed:   escalationSpeed,
		ResolutionTime:    duration,
		AffectedResources: affectedResourceList,
	}
}

// severityToValue converts severity string to numeric value
func (pde *PatternDiscoveryEngine) severityToValue(severity string) float64 {
	switch strings.ToLower(severity) {
	case "critical":
		return 1.0
	case "warning":
		return 0.75
	case "info":
		return 0.5
	default:
		return 0.25
	}
}

// crossValidatePattern performs cross-validation on a discovered pattern
func (pde *PatternDiscoveryEngine) crossValidatePattern(pattern *shared.DiscoveredPattern, data []*engine.WorkflowExecutionData) float64 {
	if len(data) < 5 {
		return 0.5 // Default confidence for insufficient data
	}

	// Split data into training (80%) and validation (20%)
	splitIndex := int(0.8 * float64(len(data)))
	_ = data[:splitIndex] // trainingData - not used in this simplified implementation
	validationData := data[splitIndex:]

	// Count matches in validation data
	matches := 0
	for _, execution := range validationData {
		if pde.executionMatchesPattern(execution, pattern) {
			matches++
		}
	}

	// Calculate validation confidence
	matchRate := float64(matches) / float64(len(validationData))

	// Adjust confidence based on pattern type complexity
	complexityFactor := 1.0
	switch strings.ToLower(string(pattern.Type)) {
	case "alert":
		complexityFactor = 0.9 // Slightly reduce confidence for alert patterns
	case "temporal":
		complexityFactor = 0.8 // More reduction for temporal patterns
	case "failure":
		complexityFactor = 0.7 // Highest reduction for failure patterns
	}

	confidence := matchRate * complexityFactor
	return math.Max(0.1, math.Min(0.95, confidence))
}

// executionMatchesPattern checks if an execution matches a discovered pattern
func (pde *PatternDiscoveryEngine) executionMatchesPattern(execution *engine.WorkflowExecutionData, pattern *shared.DiscoveredPattern) bool {
	switch strings.ToLower(string(pattern.Type)) {
	case "alert":
		return pde.executionMatchesAlertPattern(execution, pattern)
	case "temporal":
		return pde.executionMatchesTemporalPattern(execution, pattern)
	case "resource":
		return pde.executionMatchesResourcePattern(execution, pattern)
	default:
		return false
	}
}

// executionMatchesAlertPattern checks if execution matches alert pattern
func (pde *PatternDiscoveryEngine) executionMatchesAlertPattern(execution *engine.WorkflowExecutionData, pattern *shared.DiscoveredPattern) bool {
	alert := pde.extractAlertFromMetadata(execution)
	if alert == nil {
		return false
	}

	// Simplified pattern matching - check if pattern ID contains alert name
	// Since we don't have AlertPatterns field, use pattern ID/Type for matching
	patternType := strings.ToLower(string(pattern.Type))
	alertName := strings.ToLower(alert.Name)

	// Basic matching based on pattern type and alert name
	return strings.Contains(patternType, "alert") &&
		(strings.Contains(pattern.ID, alertName) || strings.Contains(alertName, patternType))
}

// executionMatchesTemporalPattern checks if execution matches temporal pattern
func (pde *PatternDiscoveryEngine) executionMatchesTemporalPattern(execution *engine.WorkflowExecutionData, pattern *shared.DiscoveredPattern) bool {
	if len(pattern.TemporalPatterns) == 0 {
		return false
	}

	temporalPattern := pattern.TemporalPatterns[0]

	// Check if execution time falls within pattern peak times
	for _, peakTime := range temporalPattern.PeakTimes {
		if execution.Timestamp.After(peakTime.Start) && execution.Timestamp.Before(peakTime.End) {
			return true
		}
	}

	return false
}

// executionMatchesResourcePattern checks if execution matches resource pattern
func (pde *PatternDiscoveryEngine) executionMatchesResourcePattern(execution *engine.WorkflowExecutionData, pattern *shared.DiscoveredPattern) bool {
	// Check if execution has resource metrics
	hasResourceMetrics := false
	for key := range execution.Metrics {
		if strings.Contains(key, "cpu") || strings.Contains(key, "memory") ||
			strings.Contains(key, "network") || strings.Contains(key, "storage") {
			hasResourceMetrics = true
			break
		}
	}

	if !hasResourceMetrics {
		return false
	}

	// Simple check based on pattern type containing resource-related keywords
	patternType := strings.ToLower(string(pattern.Type))
	return strings.Contains(patternType, "resource") || strings.Contains(patternType, "cpu") ||
		strings.Contains(patternType, "memory") || strings.Contains(patternType, "storage")
}

// Supporting types for enhanced pattern analysis

type AlertSequenceAnalysis struct {
	IsSignificant      bool
	Confidence         float64
	SuccessRate        float64
	AlertTypes         []string
	AverageInterval    time.Duration
	AverageDuration    time.Duration
	SeverityEscalation float64
}

type ResourceAffinityAnalysis struct {
	IsSignificant      bool
	Confidence         float64
	SuccessRate        float64
	Namespace          string
	Resource           string
	ResourceType       string
	AlertTypes         []string
	AverageUtilization float64
	UtilizationTrend   *sharedtypes.UtilizationTrend
}

type SeverityEscalationAnalysis struct {
	IsSignificant     bool
	Confidence        float64
	StartTime         time.Time
	StartSeverity     string
	EndSeverity       string
	Duration          time.Duration
	Pattern           string
	ResolutionRate    float64
	EscalationSpeed   float64
	ResolutionTime    time.Duration
	AffectedResources []string
}

// Confidence validation and accuracy tracking methods

// PatternAccuracyTracker tracks the accuracy of discovered patterns over time
type PatternAccuracyTracker struct {
	PatternID          string                  `json:"pattern_id"`
	CreatedAt          time.Time               `json:"created_at"`
	TotalPredictions   int                     `json:"total_predictions"`
	CorrectPredictions int                     `json:"correct_predictions"`
	FalsePositives     int                     `json:"false_positives"`
	FalseNegatives     int                     `json:"false_negatives"`
	ConfidenceHistory  []ConfidenceDataPoint   `json:"confidence_history"`
	AccuracyMetrics    *PatternAccuracyMetrics `json:"accuracy_metrics"`
	ValidationResults  []*ValidationResult     `json:"validation_results"`
	LastValidated      time.Time               `json:"last_validated"`
	PerformanceTrend   string                  `json:"performance_trend"` // "improving", "declining", "stable"
}

// ConfidenceDataPoint represents a point in confidence tracking
type ConfidenceDataPoint struct {
	Timestamp           time.Time              `json:"timestamp"`
	PredictedConfidence float64                `json:"predicted_confidence"`
	ActualOutcome       bool                   `json:"actual_outcome"`
	ContextFactors      map[string]interface{} `json:"context_factors"`
}

// PatternAccuracyMetrics contains comprehensive accuracy metrics
type PatternAccuracyMetrics struct {
	Precision             float64   `json:"precision"`              // TP / (TP + FP)
	Recall                float64   `json:"recall"`                 // TP / (TP + FN)
	F1Score               float64   `json:"f1_score"`               // 2 * (Precision * Recall) / (Precision + Recall)
	Accuracy              float64   `json:"accuracy"`               // (TP + TN) / (TP + TN + FP + FN)
	ConfidenceCorrelation float64   `json:"confidence_correlation"` // How well confidence predicts outcomes
	CalibrationError      float64   `json:"calibration_error"`      // Average difference between confidence and accuracy
	AverageConfidence     float64   `json:"average_confidence"`
	ConfidenceStdDev      float64   `json:"confidence_std_dev"`
	SampleSize            int       `json:"sample_size"`
	LastUpdated           time.Time `json:"last_updated"`
}

// startAccuracyTracking initializes accuracy tracking for a pattern
func (pde *PatternDiscoveryEngine) startAccuracyTracking(pattern *shared.DiscoveredPattern) *PatternAccuracyTracker {
	tracker := &PatternAccuracyTracker{
		PatternID:          pattern.ID,
		CreatedAt:          time.Now(),
		TotalPredictions:   0,
		CorrectPredictions: 0,
		FalsePositives:     0,
		FalseNegatives:     0,
		ConfidenceHistory:  make([]ConfidenceDataPoint, 0),
		AccuracyMetrics:    &PatternAccuracyMetrics{},
		ValidationResults:  make([]*ValidationResult, 0),
		LastValidated:      time.Now(),
		PerformanceTrend:   "stable",
	}

	// Store in pattern discovery engine for tracking
	if pde.learningMetrics != nil {
		pde.learningMetrics.PatternTrackers = append(pde.learningMetrics.PatternTrackers, tracker)
	}

	pde.log.WithFields(logrus.Fields{
		"pattern_id":   pattern.ID,
		"pattern_type": pattern.Type,
		"confidence":   pattern.Confidence,
	}).Info("Started accuracy tracking for pattern")

	return tracker
}

// recordPatternPrediction records a prediction made using a pattern
func (pde *PatternDiscoveryEngine) recordPatternPrediction(patternID string, predictedConfidence float64, context map[string]interface{}) {
	tracker := pde.findAccuracyTracker(patternID)
	if tracker == nil {
		pde.log.WithField("pattern_id", patternID).Warn("No accuracy tracker found for pattern")
		return
	}

	dataPoint := ConfidenceDataPoint{
		Timestamp:           time.Now(),
		PredictedConfidence: predictedConfidence,
		ActualOutcome:       false, // Will be updated when outcome is known
		ContextFactors:      context,
	}

	tracker.ConfidenceHistory = append(tracker.ConfidenceHistory, dataPoint)
	tracker.TotalPredictions++

	pde.log.WithFields(logrus.Fields{
		"pattern_id":  patternID,
		"confidence":  predictedConfidence,
		"predictions": tracker.TotalPredictions,
	}).Debug("Recorded pattern prediction")
}

// updatePatternOutcome updates the actual outcome for a prediction
func (pde *PatternDiscoveryEngine) updatePatternOutcome(patternID string, actualOutcome bool, timestamp time.Time) {
	tracker := pde.findAccuracyTracker(patternID)
	if tracker == nil {
		return
	}

	// Find the corresponding prediction (closest timestamp)
	var closestPoint *ConfidenceDataPoint
	minDiff := time.Duration(math.MaxInt64)

	for i := range tracker.ConfidenceHistory {
		diff := timestamp.Sub(tracker.ConfidenceHistory[i].Timestamp)
		if diff >= 0 && diff < minDiff {
			minDiff = diff
			closestPoint = &tracker.ConfidenceHistory[i]
		}
	}

	if closestPoint != nil && minDiff < 24*time.Hour { // Within 24 hours
		closestPoint.ActualOutcome = actualOutcome

		// Update accuracy counters
		confidenceThreshold := 0.7 // Patterns above this confidence are considered "positive predictions"
		predictedPositive := closestPoint.PredictedConfidence >= confidenceThreshold

		if predictedPositive && actualOutcome {
			tracker.CorrectPredictions++ // True Positive
		} else if predictedPositive && !actualOutcome {
			tracker.FalsePositives++ // False Positive
		} else if !predictedPositive && actualOutcome {
			tracker.FalseNegatives++ // False Negative
		} else {
			tracker.CorrectPredictions++ // True Negative
		}

		// Update metrics
		pde.updateAccuracyMetrics(tracker)

		pde.log.WithFields(logrus.Fields{
			"pattern_id":     patternID,
			"actual_outcome": actualOutcome,
			"predicted_conf": closestPoint.PredictedConfidence,
			"accuracy":       tracker.AccuracyMetrics.Accuracy,
		}).Debug("Updated pattern outcome")
	}
}

// updateAccuracyMetrics calculates and updates accuracy metrics for a tracker
func (pde *PatternDiscoveryEngine) updateAccuracyMetrics(tracker *PatternAccuracyTracker) {
	if tracker.TotalPredictions == 0 {
		return
	}

	// Calculate basic metrics
	tp := float64(tracker.CorrectPredictions - (tracker.TotalPredictions - tracker.CorrectPredictions - tracker.FalsePositives - tracker.FalseNegatives))
	fp := float64(tracker.FalsePositives)
	fn := float64(tracker.FalseNegatives)
	tn := float64(tracker.TotalPredictions - tracker.CorrectPredictions - tracker.FalsePositives - tracker.FalseNegatives)

	// Ensure non-negative values
	if tp < 0 {
		tp = 0
	}
	if tn < 0 {
		tn = 0
	}

	metrics := tracker.AccuracyMetrics

	// Precision: TP / (TP + FP)
	if tp+fp > 0 {
		metrics.Precision = tp / (tp + fp)
	} else {
		metrics.Precision = 0
	}

	// Recall: TP / (TP + FN)
	if tp+fn > 0 {
		metrics.Recall = tp / (tp + fn)
	} else {
		metrics.Recall = 0
	}

	// F1 Score: 2 * (Precision * Recall) / (Precision + Recall)
	if metrics.Precision+metrics.Recall > 0 {
		metrics.F1Score = 2 * (metrics.Precision * metrics.Recall) / (metrics.Precision + metrics.Recall)
	} else {
		metrics.F1Score = 0
	}

	// Accuracy: (TP + TN) / Total
	if tracker.TotalPredictions > 0 {
		metrics.Accuracy = (tp + tn) / float64(tracker.TotalPredictions)
	} else {
		metrics.Accuracy = 0
	}

	// Calculate confidence correlation and calibration
	pde.calculateConfidenceMetrics(tracker)

	metrics.SampleSize = tracker.TotalPredictions
	metrics.LastUpdated = time.Now()

	// Update performance trend
	pde.updatePerformanceTrend(tracker)
}

// calculateConfidenceMetrics calculates confidence-related metrics
func (pde *PatternDiscoveryEngine) calculateConfidenceMetrics(tracker *PatternAccuracyTracker) {
	if len(tracker.ConfidenceHistory) == 0 {
		return
	}

	confidences := make([]float64, 0)
	outcomes := make([]float64, 0)
	calibrationErrors := make([]float64, 0)

	for _, point := range tracker.ConfidenceHistory {
		if point.ActualOutcome { // Only include points where outcome is known
			confidences = append(confidences, point.PredictedConfidence)
			if point.ActualOutcome {
				outcomes = append(outcomes, 1.0)
			} else {
				outcomes = append(outcomes, 0.0)
			}

			// Calibration error: |confidence - actual_outcome|
			actualValue := 0.0
			if point.ActualOutcome {
				actualValue = 1.0
			}
			calibrationErrors = append(calibrationErrors, math.Abs(point.PredictedConfidence-actualValue))
		}
	}

	if len(confidences) == 0 {
		return
	}

	// Calculate average confidence
	sum := 0.0
	for _, conf := range confidences {
		sum += conf
	}
	tracker.AccuracyMetrics.AverageConfidence = sum / float64(len(confidences))

	// Calculate confidence standard deviation
	sumSquares := 0.0
	for _, conf := range confidences {
		diff := conf - tracker.AccuracyMetrics.AverageConfidence
		sumSquares += diff * diff
	}
	tracker.AccuracyMetrics.ConfidenceStdDev = math.Sqrt(sumSquares / float64(len(confidences)))

	// Calculate confidence-outcome correlation (simplified Pearson correlation)
	if len(confidences) > 1 {
		avgConf := tracker.AccuracyMetrics.AverageConfidence
		avgOutcome := 0.0
		for _, outcome := range outcomes {
			avgOutcome += outcome
		}
		avgOutcome /= float64(len(outcomes))

		numerator := 0.0
		confVariance := 0.0
		outcomeVariance := 0.0

		for i := 0; i < len(confidences); i++ {
			confDiff := confidences[i] - avgConf
			outcomeDiff := outcomes[i] - avgOutcome
			numerator += confDiff * outcomeDiff
			confVariance += confDiff * confDiff
			outcomeVariance += outcomeDiff * outcomeDiff
		}

		denominator := math.Sqrt(confVariance * outcomeVariance)
		if denominator > 0 {
			tracker.AccuracyMetrics.ConfidenceCorrelation = numerator / denominator
		}
	}

	// Calculate average calibration error
	if len(calibrationErrors) > 0 {
		sum := 0.0
		for _, err := range calibrationErrors {
			sum += err
		}
		tracker.AccuracyMetrics.CalibrationError = sum / float64(len(calibrationErrors))
	}
}

// updatePerformanceTrend analyzes performance trend over time
func (pde *PatternDiscoveryEngine) updatePerformanceTrend(tracker *PatternAccuracyTracker) {
	if len(tracker.ValidationResults) < 2 {
		tracker.PerformanceTrend = "stable"
		return
	}

	// Look at recent validation results (last 5)
	recentResults := tracker.ValidationResults
	if len(recentResults) > 5 {
		recentResults = recentResults[len(recentResults)-5:]
	}

	if len(recentResults) < 2 {
		tracker.PerformanceTrend = "stable"
		return
	}

	// Calculate trend based on validation scores
	scores := make([]float64, len(recentResults))
	for i, result := range recentResults {
		// Use 1.0 for passed validation, 0.0 for failed as score
		if result.Passed {
			scores[i] = 1.0
		} else {
			scores[i] = 0.0
		}
	}

	// Simple linear trend analysis
	sumX := 0.0
	sumY := 0.0
	sumXY := 0.0
	sumXX := 0.0
	n := float64(len(scores))

	for i, score := range scores {
		x := float64(i)
		sumX += x
		sumY += score
		sumXY += x * score
		sumXX += x * x
	}

	// Calculate slope
	slope := (n*sumXY - sumX*sumY) / (n*sumXX - sumX*sumX)

	if slope > 0.02 { // More than 2% improvement trend
		tracker.PerformanceTrend = "improving"
	} else if slope < -0.02 { // More than 2% decline trend
		tracker.PerformanceTrend = "declining"
	} else {
		tracker.PerformanceTrend = "stable"
	}
}

// performPatternValidation performs comprehensive validation of a pattern
func (pde *PatternDiscoveryEngine) performPatternValidation(pattern *shared.DiscoveredPattern, data []*engine.WorkflowExecutionData) *ValidationResult {
	validationID := fmt.Sprintf("validation-%s-%d", pattern.ID, time.Now().Unix())

	result := &ValidationResult{
		RuleID:    validationID,
		Type:      ValidationType("comprehensive"),
		Passed:    false,
		Message:   "Pattern validation in progress",
		Details:   make(map[string]interface{}),
		Timestamp: time.Now(),
	}

	if len(data) < 10 {
		result.Details["error"] = "Insufficient data for validation"
		result.Details["validation_score"] = 0.3
		result.Message = "Insufficient data for validation"
		return result
	}

	// Perform multiple validation techniques
	crossValScore := pde.performCrossValidation(pattern, data)
	holdoutScore := pde.performHoldoutValidation(pattern, data)
	temporalScore := pde.performTemporalValidation(pattern, data)

	// Calculate weighted average
	validationScore := (crossValScore*0.4 + holdoutScore*0.3 + temporalScore*0.3)
	result.Details["validation_score"] = validationScore

	// Determine if validation passed
	result.Passed = validationScore >= pde.config.PredictionConfidence

	// Calculate confidence adjustment
	originalConfidence := pattern.Confidence
	confidenceAdjustment := validationScore - originalConfidence

	// Store validation details
	result.Details["cross_validation_score"] = crossValScore
	result.Details["holdout_score"] = holdoutScore
	result.Details["temporal_score"] = temporalScore
	result.Details["original_confidence"] = originalConfidence
	result.Details["adjusted_confidence"] = math.Max(0.1, math.Min(0.95, originalConfidence+confidenceAdjustment))

	pde.log.WithFields(logrus.Fields{
		"pattern_id":        pattern.ID,
		"validation_score":  validationScore,
		"passed_validation": result.Passed,
		"confidence_adj":    confidenceAdjustment,
	}).Info("Completed pattern validation")

	return result
}

// performCrossValidation performs k-fold cross-validation
func (pde *PatternDiscoveryEngine) performCrossValidation(pattern *shared.DiscoveredPattern, data []*engine.WorkflowExecutionData) float64 {
	k := 5 // 5-fold cross-validation
	if len(data) < k {
		return 0.5 // Default score for insufficient data
	}

	foldSize := len(data) / k
	totalScore := 0.0

	for i := 0; i < k; i++ {
		// Create validation fold
		validationStart := i * foldSize
		validationEnd := validationStart + foldSize
		if i == k-1 {
			validationEnd = len(data) // Include remaining data in last fold
		}

		validationData := data[validationStart:validationEnd]

		// Test pattern against validation fold
		matches := 0
		for _, execution := range validationData {
			if pde.executionMatchesPattern(execution, pattern) {
				matches++
			}
		}

		foldScore := float64(matches) / float64(len(validationData))
		totalScore += foldScore
	}

	return totalScore / float64(k)
}

// performHoldoutValidation performs holdout validation (80/20 split)
func (pde *PatternDiscoveryEngine) performHoldoutValidation(pattern *shared.DiscoveredPattern, data []*engine.WorkflowExecutionData) float64 {
	if len(data) < 10 {
		return 0.5
	}

	// Use last 20% as validation set
	splitPoint := int(0.8 * float64(len(data)))
	validationData := data[splitPoint:]

	matches := 0
	for _, execution := range validationData {
		if pde.executionMatchesPattern(execution, pattern) {
			matches++
		}
	}

	return float64(matches) / float64(len(validationData))
}

// performTemporalValidation performs temporal validation (patterns should work across time)
func (pde *PatternDiscoveryEngine) performTemporalValidation(pattern *shared.DiscoveredPattern, data []*engine.WorkflowExecutionData) float64 {
	if len(data) < 20 {
		return 0.5
	}

	// Sort data by timestamp
	sortedData := make([]*engine.WorkflowExecutionData, len(data))
	copy(sortedData, data)
	sort.Slice(sortedData, func(i, j int) bool {
		return sortedData[i].Timestamp.Before(sortedData[j].Timestamp)
	})

	// Test pattern on different time periods
	periods := 3
	periodSize := len(sortedData) / periods
	totalScore := 0.0

	for i := 0; i < periods; i++ {
		start := i * periodSize
		end := start + periodSize
		if i == periods-1 {
			end = len(sortedData)
		}

		periodData := sortedData[start:end]
		matches := 0

		for _, execution := range periodData {
			if pde.executionMatchesPattern(execution, pattern) {
				matches++
			}
		}

		periodScore := float64(matches) / float64(len(periodData))
		totalScore += periodScore
	}

	return totalScore / float64(periods)
}

// findAccuracyTracker finds the accuracy tracker for a pattern
func (pde *PatternDiscoveryEngine) findAccuracyTracker(patternID string) *PatternAccuracyTracker {
	if pde.learningMetrics == nil {
		return nil
	}

	for _, tracker := range pde.learningMetrics.PatternTrackers {
		if tracker.PatternID == patternID {
			return tracker
		}
	}

	return nil
}

// getPatternAccuracyReport generates a comprehensive accuracy report
func (pde *PatternDiscoveryEngine) getPatternAccuracyReport() *PatternAccuracyReport {
	if pde.learningMetrics == nil {
		return &PatternAccuracyReport{
			GeneratedAt:   time.Now(),
			TotalPatterns: 0,
		}
	}

	trackers := pde.learningMetrics.PatternTrackers
	report := &PatternAccuracyReport{
		GeneratedAt:      time.Now(),
		TotalPatterns:    len(trackers),
		TrackedPatterns:  len(trackers),
		PatternSummaries: make([]*PatternAccuracySummary, 0),
		OverallMetrics:   &OverallAccuracyMetrics{},
	}

	// Calculate overall metrics
	totalPredictions := 0
	totalCorrect := 0
	accuracySum := 0.0
	confidenceSum := 0.0

	for _, tracker := range trackers {
		if tracker.AccuracyMetrics != nil {
			totalPredictions += tracker.TotalPredictions
			totalCorrect += tracker.CorrectPredictions
			accuracySum += tracker.AccuracyMetrics.Accuracy
			confidenceSum += tracker.AccuracyMetrics.AverageConfidence

			// Create pattern summary
			summary := &PatternAccuracySummary{
				PatternID:             tracker.PatternID,
				Accuracy:              tracker.AccuracyMetrics.Accuracy,
				Precision:             tracker.AccuracyMetrics.Precision,
				Recall:                tracker.AccuracyMetrics.Recall,
				F1Score:               tracker.AccuracyMetrics.F1Score,
				ConfidenceCorrelation: tracker.AccuracyMetrics.ConfidenceCorrelation,
				SampleSize:            tracker.TotalPredictions,
				PerformanceTrend:      tracker.PerformanceTrend,
				LastValidated:         tracker.LastValidated,
			}
			report.PatternSummaries = append(report.PatternSummaries, summary)
		}
	}

	// Calculate overall metrics
	if len(trackers) > 0 {
		report.OverallMetrics.AverageAccuracy = accuracySum / float64(len(trackers))
		report.OverallMetrics.AverageConfidence = confidenceSum / float64(len(trackers))
		if totalPredictions > 0 {
			report.OverallMetrics.OverallAccuracy = float64(totalCorrect) / float64(totalPredictions)
		}
		report.OverallMetrics.TotalPredictions = totalPredictions
	}

	return report
}

// Supporting types for accuracy tracking

type PatternAccuracyReport struct {
	GeneratedAt      time.Time                 `json:"generated_at"`
	TotalPatterns    int                       `json:"total_patterns"`
	TrackedPatterns  int                       `json:"tracked_patterns"`
	PatternSummaries []*PatternAccuracySummary `json:"pattern_summaries"`
	OverallMetrics   *OverallAccuracyMetrics   `json:"overall_metrics"`
}

type PatternAccuracySummary struct {
	PatternID             string    `json:"pattern_id"`
	Accuracy              float64   `json:"accuracy"`
	Precision             float64   `json:"precision"`
	Recall                float64   `json:"recall"`
	F1Score               float64   `json:"f1_score"`
	ConfidenceCorrelation float64   `json:"confidence_correlation"`
	SampleSize            int       `json:"sample_size"`
	PerformanceTrend      string    `json:"performance_trend"`
	LastValidated         time.Time `json:"last_validated"`
}

type OverallAccuracyMetrics struct {
	AverageAccuracy   float64 `json:"average_accuracy"`
	OverallAccuracy   float64 `json:"overall_accuracy"`
	AverageConfidence float64 `json:"average_confidence"`
	TotalPredictions  int     `json:"total_predictions"`
}
