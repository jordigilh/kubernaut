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

package orchestration

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// **REFACTOR PHASE**: Enhanced cost optimization for intelligent resource management
// Business Requirements: BR-ENSEMBLE-003

// CostMetrics tracks cost-related performance data
type CostMetrics struct {
	TotalCost        float64   `json:"total_cost"`
	CostPerRequest   float64   `json:"cost_per_request"`
	CostPerAccuracy  float64   `json:"cost_per_accuracy"`
	RequestCount     int       `json:"request_count"`
	LastUpdated      time.Time `json:"last_updated"`
	CostTrend        CostTrend `json:"cost_trend"`
	EfficiencyRating float64   `json:"efficiency_rating"`
}

// CostTrend indicates cost efficiency direction
type CostTrend string

const (
	CostTrendImproving CostTrend = "improving"
	CostTrendStable    CostTrend = "stable"
	CostTrendWorsening CostTrend = "worsening"
)

// CostOptimizer manages cost-aware model selection and optimization
type CostOptimizer struct {
	mu           sync.RWMutex
	logger       *logrus.Logger
	modelCosts   map[string]float64
	costMetrics  map[string]*CostMetrics
	budgetLimits BudgetLimits
}

// BudgetLimits defines cost constraints
type BudgetLimits struct {
	DailyBudget     float64 `json:"daily_budget"`
	RequestBudget   float64 `json:"request_budget"`
	AccuracyBudget  float64 `json:"accuracy_budget"`
	EmergencyBudget float64 `json:"emergency_budget"`
}

// CostOptimizationStrategy defines different cost optimization approaches
type CostOptimizationStrategy string

const (
	StrategyMinimizeCost     CostOptimizationStrategy = "minimize_cost"
	StrategyMaximizeValue    CostOptimizationStrategy = "maximize_value"
	StrategyBalanced         CostOptimizationStrategy = "balanced"
	StrategyPerformanceFirst CostOptimizationStrategy = "performance_first"
)

// NewCostOptimizer creates a new cost optimizer
func NewCostOptimizer(logger *logrus.Logger) *CostOptimizer {
	return &CostOptimizer{
		logger:      logger,
		modelCosts:  make(map[string]float64),
		costMetrics: make(map[string]*CostMetrics),
		budgetLimits: BudgetLimits{
			DailyBudget:     100.0,
			RequestBudget:   1.0,
			AccuracyBudget:  0.5,
			EmergencyBudget: 10.0,
		},
	}
}

// SetModelCost configures the cost for a specific model
// BR-ENSEMBLE-003: Cost configuration and tracking
func (co *CostOptimizer) SetModelCost(modelID string, cost float64) {
	co.mu.Lock()
	defer co.mu.Unlock()

	co.modelCosts[modelID] = cost
	co.logger.WithFields(logrus.Fields{
		"model_id": modelID,
		"cost":     cost,
	}).Debug("BR-ENSEMBLE-003: Model cost configured")
}

// GetModelCost returns the cost for a specific model
func (co *CostOptimizer) GetModelCost(modelID string) float64 {
	co.mu.RLock()
	defer co.mu.RUnlock()

	if cost, exists := co.modelCosts[modelID]; exists {
		return cost
	}
	return 0.1 // Default cost for unknown models
}

// RecordCostUsage records cost usage for a model
func (co *CostOptimizer) RecordCostUsage(modelID string, cost float64, accuracy float64) {
	co.mu.Lock()
	defer co.mu.Unlock()

	metrics := co.getOrCreateCostMetrics(modelID)

	// Update cost metrics
	metrics.RequestCount++
	metrics.TotalCost += cost
	metrics.CostPerRequest = metrics.TotalCost / float64(metrics.RequestCount)

	if accuracy > 0 {
		metrics.CostPerAccuracy = cost / accuracy
	}

	// Calculate efficiency rating (accuracy per unit cost)
	if cost > 0 {
		metrics.EfficiencyRating = accuracy / cost
	}

	metrics.LastUpdated = time.Now()

	co.logger.WithFields(logrus.Fields{
		"model_id":          modelID,
		"cost":              cost,
		"accuracy":          accuracy,
		"efficiency_rating": metrics.EfficiencyRating,
		"cost_per_request":  metrics.CostPerRequest,
	}).Debug("BR-ENSEMBLE-003: Cost usage recorded")
}

// OptimizeModelSelection selects optimal models based on cost-accuracy trade-offs
// BR-ENSEMBLE-003: Intelligent cost-aware selection
func (co *CostOptimizer) OptimizeModelSelection(availableModels []string, budget CostBudget, strategy CostOptimizationStrategy) ([]string, float64, error) {
	co.mu.RLock()
	defer co.mu.RUnlock()

	switch strategy {
	case StrategyMinimizeCost:
		return co.selectMinimumCost(availableModels, budget)
	case StrategyMaximizeValue:
		return co.selectMaximumValue(availableModels, budget)
	case StrategyBalanced:
		return co.selectBalanced(availableModels, budget)
	case StrategyPerformanceFirst:
		return co.selectPerformanceFirst(availableModels, budget)
	default:
		return co.selectBalanced(availableModels, budget)
	}
}

// GenerateOptimizationRecommendations provides cost optimization suggestions
// BR-ENSEMBLE-003: Cost optimization guidance
func (co *CostOptimizer) GenerateOptimizationRecommendations() []CostOptimizationRecommendation {
	co.mu.RLock()
	defer co.mu.RUnlock()

	recommendations := []CostOptimizationRecommendation{}

	// Analyze cost efficiency across models
	for modelID, metrics := range co.costMetrics {
		if metrics.EfficiencyRating < 0.5 {
			recommendations = append(recommendations, CostOptimizationRecommendation{
				PotentialSavings: co.calculatePotentialSavings(modelID),
				AccuracyImpact:   -0.02, // Minimal impact
				Implementation:   fmt.Sprintf("Consider reducing usage of model %s due to low efficiency", modelID),
			})
		}
	}

	// Add general recommendations
	recommendations = append(recommendations, CostOptimizationRecommendation{
		PotentialSavings: 0.25,
		AccuracyImpact:   -0.01,
		Implementation:   "Use cost-optimized consensus for non-critical alerts",
	})

	return recommendations
}

// CalculateCostSavings calculates potential savings from optimization
func (co *CostOptimizer) CalculateCostSavings(originalCost, optimizedCost float64) float64 {
	if originalCost <= 0 {
		return 0.0
	}
	return (originalCost - optimizedCost) / originalCost
}

// IsWithinBudget checks if a cost is within budget constraints
func (co *CostOptimizer) IsWithinBudget(cost float64, budgetType string) bool {
	co.mu.RLock()
	defer co.mu.RUnlock()

	switch budgetType {
	case "request":
		return cost <= co.budgetLimits.RequestBudget
	case "emergency":
		return cost <= co.budgetLimits.EmergencyBudget
	case "accuracy":
		return cost <= co.budgetLimits.AccuracyBudget
	default:
		return cost <= co.budgetLimits.RequestBudget
	}
}

// Private helper methods

func (co *CostOptimizer) getOrCreateCostMetrics(modelID string) *CostMetrics {
	if metrics, exists := co.costMetrics[modelID]; exists {
		return metrics
	}

	metrics := &CostMetrics{
		TotalCost:        0.0,
		CostPerRequest:   0.0,
		CostPerAccuracy:  0.0,
		RequestCount:     0,
		LastUpdated:      time.Now(),
		CostTrend:        CostTrendStable,
		EfficiencyRating: 1.0,
	}
	co.costMetrics[modelID] = metrics
	return metrics
}

func (co *CostOptimizer) selectMinimumCost(models []string, budget CostBudget) ([]string, float64, error) {
	// Sort models by cost (ascending)
	type modelCost struct {
		id   string
		cost float64
	}

	modelCosts := make([]modelCost, 0, len(models))
	for _, modelID := range models {
		cost := co.GetModelCost(modelID)
		modelCosts = append(modelCosts, modelCost{id: modelID, cost: cost})
	}

	// Sort by cost
	for i := 0; i < len(modelCosts)-1; i++ {
		for j := i + 1; j < len(modelCosts); j++ {
			if modelCosts[i].cost > modelCosts[j].cost {
				modelCosts[i], modelCosts[j] = modelCosts[j], modelCosts[i]
			}
		}
	}

	// Select models within budget
	selected := []string{}
	totalCost := 0.0

	for _, mc := range modelCosts {
		if totalCost+mc.cost <= budget.MaxCostPerRequest {
			selected = append(selected, mc.id)
			totalCost += mc.cost
		}
	}

	if len(selected) == 0 && len(modelCosts) > 0 {
		// Use cheapest model if budget is very tight
		selected = []string{modelCosts[0].id}
		totalCost = modelCosts[0].cost
	}

	return selected, totalCost, nil
}

func (co *CostOptimizer) selectMaximumValue(models []string, budget CostBudget) ([]string, float64, error) {
	// Calculate value (efficiency rating / cost) for each model
	type modelValue struct {
		id    string
		cost  float64
		value float64
	}

	modelValues := make([]modelValue, 0, len(models))
	for _, modelID := range models {
		cost := co.GetModelCost(modelID)
		metrics := co.getOrCreateCostMetrics(modelID)
		value := 0.0
		if cost > 0 {
			value = metrics.EfficiencyRating / cost
		}
		modelValues = append(modelValues, modelValue{id: modelID, cost: cost, value: value})
	}

	// Sort by value (descending)
	for i := 0; i < len(modelValues)-1; i++ {
		for j := i + 1; j < len(modelValues); j++ {
			if modelValues[i].value < modelValues[j].value {
				modelValues[i], modelValues[j] = modelValues[j], modelValues[i]
			}
		}
	}

	// Select highest value models within budget
	selected := []string{}
	totalCost := 0.0

	for _, mv := range modelValues {
		if totalCost+mv.cost <= budget.MaxCostPerRequest {
			selected = append(selected, mv.id)
			totalCost += mv.cost
		}
	}

	return selected, totalCost, nil
}

func (co *CostOptimizer) selectBalanced(models []string, budget CostBudget) ([]string, float64, error) {
	// Balance between cost and performance
	type modelScore struct {
		id    string
		cost  float64
		score float64
	}

	modelScores := make([]modelScore, 0, len(models))
	for _, modelID := range models {
		cost := co.GetModelCost(modelID)
		metrics := co.getOrCreateCostMetrics(modelID)

		// Balanced score: efficiency rating weighted by inverse cost
		costFactor := 1.0 / math.Max(cost, 0.001)
		score := metrics.EfficiencyRating*0.7 + costFactor*0.3

		modelScores = append(modelScores, modelScore{id: modelID, cost: cost, score: score})
	}

	// Sort by balanced score (descending)
	for i := 0; i < len(modelScores)-1; i++ {
		for j := i + 1; j < len(modelScores); j++ {
			if modelScores[i].score < modelScores[j].score {
				modelScores[i], modelScores[j] = modelScores[j], modelScores[i]
			}
		}
	}

	// Select best balanced models within budget
	selected := []string{}
	totalCost := 0.0

	for _, ms := range modelScores {
		if totalCost+ms.cost <= budget.MaxCostPerRequest {
			selected = append(selected, ms.id)
			totalCost += ms.cost
		}
	}

	return selected, totalCost, nil
}

func (co *CostOptimizer) selectPerformanceFirst(models []string, budget CostBudget) ([]string, float64, error) {
	// Prioritize performance, then consider cost
	type modelPerformance struct {
		id          string
		cost        float64
		performance float64
	}

	modelPerfs := make([]modelPerformance, 0, len(models))
	for _, modelID := range models {
		cost := co.GetModelCost(modelID)
		metrics := co.getOrCreateCostMetrics(modelID)

		modelPerfs = append(modelPerfs, modelPerformance{
			id:          modelID,
			cost:        cost,
			performance: metrics.EfficiencyRating,
		})
	}

	// Sort by performance (descending)
	for i := 0; i < len(modelPerfs)-1; i++ {
		for j := i + 1; j < len(modelPerfs); j++ {
			if modelPerfs[i].performance < modelPerfs[j].performance {
				modelPerfs[i], modelPerfs[j] = modelPerfs[j], modelPerfs[i]
			}
		}
	}

	// Select highest performing models within budget
	selected := []string{}
	totalCost := 0.0

	for _, mp := range modelPerfs {
		if totalCost+mp.cost <= budget.MaxCostPerRequest {
			selected = append(selected, mp.id)
			totalCost += mp.cost
		}
	}

	return selected, totalCost, nil
}

func (co *CostOptimizer) calculatePotentialSavings(modelID string) float64 {
	metrics, exists := co.costMetrics[modelID]
	if !exists {
		return 0.0
	}

	// Calculate potential savings based on efficiency
	if metrics.EfficiencyRating < 0.5 {
		return 0.3 // 30% potential savings for inefficient models
	} else if metrics.EfficiencyRating < 0.7 {
		return 0.15 // 15% potential savings for moderately efficient models
	}
	return 0.05 // 5% potential savings for efficient models
}
