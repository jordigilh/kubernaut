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
package orchestration

import (
	"time"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/sirupsen/logrus"
)

// **REFACTOR PHASE**: Enhanced types for sophisticated ensemble decision-making
// Business Requirements: BR-ENSEMBLE-001 through BR-ENSEMBLE-004

// Priority defines decision-making priority levels
type Priority string

const (
	CriticalPriority  Priority = "critical"
	StandardPriority  Priority = "standard"
	LowPriority       Priority = "low"
	EmergencyPriority Priority = "emergency"
)

// CostBudget defines cost constraints for model selection
type CostBudget struct {
	MaxCostPerRequest float64 `json:"max_cost_per_request"`
	AccuracyThreshold float64 `json:"accuracy_threshold"`
}

// ConsensusDecision represents the result of ensemble decision-making
type ConsensusDecision struct {
	// Core decision fields
	Action              string  `json:"action"`
	Confidence          float64 `json:"confidence"`
	ParticipatingModels int     `json:"participating_models"`

	// Disagreement and conflict analysis
	DisagreementResolution string  `json:"disagreement_resolution"`
	ConflictScore          float64 `json:"conflict_score"`

	// Performance and timing
	ResponseTime time.Duration `json:"response_time"`
	BypassReason string        `json:"bypass_reason,omitempty"`

	// Model management
	ExcludedModels  []string `json:"excluded_models,omitempty"`
	FailedModels    []string `json:"failed_models,omitempty"`
	FailoverApplied bool     `json:"failover_applied"`
	MaintenanceMode bool     `json:"maintenance_mode"`

	// Cost optimization
	TotalCost          float64  `json:"total_cost"`
	PredictedAccuracy  float64  `json:"predicted_accuracy"`
	CostSavings        float64  `json:"cost_savings"`
	DegradationApplied bool     `json:"degradation_applied"`
	SelectedModels     []string `json:"selected_models"`
}

// ModelPerformance tracks performance metrics for individual models
type ModelPerformance struct {
	AccuracyRate float64       `json:"accuracy_rate"`
	ResponseTime time.Duration `json:"response_time"`
	RequestCount int           `json:"request_count"`
}

// ModelHealth represents health status of a model
type ModelHealth struct {
	IsHealthy    bool          `json:"is_healthy"`
	ResponseTime time.Duration `json:"response_time"`
	ErrorRate    float64       `json:"error_rate"`
	LastChecked  time.Time     `json:"last_checked"`
}

// CostOptimizationRecommendation provides cost optimization guidance
type CostOptimizationRecommendation struct {
	PotentialSavings float64 `json:"potential_savings"`
	AccuracyImpact   float64 `json:"accuracy_impact"`
	Implementation   string  `json:"implementation"`
}

// RecoveryStatus tracks model recovery validation
type RecoveryStatus struct {
	IsRecovered         bool    `json:"is_recovered"`
	ValidationTests     int     `json:"validation_tests"`
	PerformanceBaseline float64 `json:"performance_baseline"`
}

// MultiModelOrchestrator manages ensemble decision-making with multiple models
type MultiModelOrchestrator struct {
	// Core dependencies
	models []llm.Client
	logger *logrus.Logger

	// Performance tracking
	modelAccuracies   map[string]float64
	failedModels      map[string]bool
	maintenanceModels map[string]bool
}

// NewMultiModelOrchestrator creates a new multi-model orchestrator
func NewMultiModelOrchestrator(models []llm.Client, logger *logrus.Logger) *MultiModelOrchestrator {
	return &MultiModelOrchestrator{
		models:            models,
		logger:            logger,
		modelAccuracies:   make(map[string]float64),
		failedModels:      make(map[string]bool),
		maintenanceModels: make(map[string]bool),
	}
}

// EnhancedConsensusDecision represents a sophisticated ensemble decision with rich metadata
type EnhancedConsensusDecision struct {
	// Core decision fields
	Action              string  `json:"action"`
	Confidence          float64 `json:"confidence"`
	ParticipatingModels int     `json:"participating_models"`

	// Disagreement and conflict analysis
	DisagreementScore      float64 `json:"disagreement_score"`
	DisagreementResolution string  `json:"disagreement_resolution"`
	ConflictScore          float64 `json:"conflict_score"`

	// Performance and timing
	ResponseTime   time.Duration `json:"response_time"`
	ProcessingTime time.Duration `json:"processing_time"`
	BypassReason   string        `json:"bypass_reason,omitempty"`

	// Model management
	ExcludedModels  []string `json:"excluded_models,omitempty"`
	FailedModels    []string `json:"failed_models,omitempty"`
	FailoverApplied bool     `json:"failover_applied"`
	MaintenanceMode bool     `json:"maintenance_mode"`

	// Cost optimization
	TotalCost          float64  `json:"total_cost"`
	PredictedAccuracy  float64  `json:"predicted_accuracy"`
	CostSavings        float64  `json:"cost_savings"`
	DegradationApplied bool     `json:"degradation_applied"`
	SelectedModels     []string `json:"selected_models"`

	// Enhanced metadata
	Algorithm              string                 `json:"algorithm"`
	QualityScore           float64                `json:"quality_score"`
	ModelResponses         []interface{}          `json:"model_responses,omitempty"`
	RecommendationMetadata map[string]interface{} `json:"recommendation_metadata,omitempty"`
}

// ConsensusOptions configures ensemble decision-making behavior
type ConsensusOptions struct {
	// Performance requirements
	MinAccuracyThreshold float64       `json:"min_accuracy_threshold"`
	MaxResponseTime      time.Duration `json:"max_response_time"`
	RequiredConfidence   float64       `json:"required_confidence"`

	// Cost constraints
	Budget CostBudget `json:"budget"`

	// Quality requirements
	MinQualityScore      float64 `json:"min_quality_score"`
	MaxDisagreementScore float64 `json:"max_disagreement_score"`

	// Operational settings
	AllowFailover    bool `json:"allow_failover"`
	AllowDegradation bool `json:"allow_degradation"`
	BypassOnTimeout  bool `json:"bypass_on_timeout"`

	// Model selection
	PreferredModels        []string `json:"preferred_models,omitempty"`
	ExcludedModels         []string `json:"excluded_models,omitempty"`
	MinParticipatingModels int      `json:"min_participating_models"`
}

// DefaultConsensusOptions returns sensible defaults for consensus options
func DefaultConsensusOptions() ConsensusOptions {
	return ConsensusOptions{
		MinAccuracyThreshold: 0.7,
		MaxResponseTime:      10 * time.Second,
		RequiredConfidence:   0.8,
		Budget: CostBudget{
			MaxCostPerRequest: 1.0,
			AccuracyThreshold: 0.8,
		},
		MinQualityScore:        0.7,
		MaxDisagreementScore:   0.5,
		AllowFailover:          true,
		AllowDegradation:       true,
		BypassOnTimeout:        true,
		MinParticipatingModels: 1,
	}
}

// CriticalConsensusOptions returns options for critical decisions requiring high confidence
func CriticalConsensusOptions() ConsensusOptions {
	options := DefaultConsensusOptions()
	options.MinAccuracyThreshold = 0.9
	options.RequiredConfidence = 0.95
	options.MinQualityScore = 0.9
	options.MaxDisagreementScore = 0.3
	options.MinParticipatingModels = 2
	options.AllowDegradation = false
	return options
}

// EmergencyConsensusOptions returns options for emergency decisions prioritizing speed
func EmergencyConsensusOptions() ConsensusOptions {
	options := DefaultConsensusOptions()
	options.MaxResponseTime = 2 * time.Second
	options.MinAccuracyThreshold = 0.6
	options.RequiredConfidence = 0.7
	options.MinQualityScore = 0.6
	options.AllowDegradation = true
	options.BypassOnTimeout = true
	options.MinParticipatingModels = 1
	return options
}

// CostOptimizedConsensusOptions returns options for cost-conscious decisions
func CostOptimizedConsensusOptions() ConsensusOptions {
	options := DefaultConsensusOptions()
	options.Budget.MaxCostPerRequest = 0.5
	options.MinAccuracyThreshold = 0.6
	options.AllowDegradation = true
	return options
}
