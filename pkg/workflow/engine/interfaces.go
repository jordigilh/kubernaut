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
	"time"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Core workflow interfaces
type WorkflowEngine interface {
	Execute(ctx context.Context, workflow *Workflow) (*RuntimeWorkflowExecution, error)
	GetExecution(ctx context.Context, executionID string) (*RuntimeWorkflowExecution, error)
	ListExecutions(ctx context.Context, workflowID string) ([]*RuntimeWorkflowExecution, error)

	// BR-WF-ADV-628: Subflow completion monitoring
	WaitForSubflowCompletion(ctx context.Context, executionID string, timeout time.Duration) (*RuntimeWorkflowExecution, error)
}

// Metrics and monitoring interfaces

// SubflowMetricsCollector defines interface for collecting subflow monitoring metrics
// Business Requirement: BR-WF-ADV-628 - Resource optimization during waiting periods
type SubflowMetricsCollector interface {
	RecordSubflowMonitoring(executionID string, monitoringDuration, executionDuration time.Duration, success bool)
	RecordSubflowProgress(executionID string, progressPercent float64, monitoringDuration time.Duration)
	RecordCircuitBreakerActivation(executionID string, consecutiveFailures int, backoffDuration time.Duration)
}

// Storage and data interfaces
type VectorDatabase interface {
	Store(ctx context.Context, id string, vector []float64, metadata map[string]interface{}) error
	Search(ctx context.Context, vector []float64, limit int) ([]*VectorSearchResult, error)
	Delete(ctx context.Context, id string) error
}

type VectorSearchResult struct {
	ID       string                 `json:"id"`
	Score    float64                `json:"score"`
	Vector   []float64              `json:"vector"`
	Metadata map[string]interface{} `json:"metadata"`
}

type PatternStore interface {
	StorePattern(ctx context.Context, pattern *types.DiscoveredPattern) error
	GetPattern(ctx context.Context, patternID string) (*types.DiscoveredPattern, error)
	ListPatterns(ctx context.Context, patternType string) ([]*types.DiscoveredPattern, error)
	DeletePattern(ctx context.Context, patternID string) error
}

// Analytics and ML interfaces
type MachineLearningAnalyzer interface {
	AnalyzePatterns(ctx context.Context, data []*EngineWorkflowExecutionData) ([]*types.DiscoveredPattern, error)
	PredictEffectiveness(ctx context.Context, workflow *Workflow) (float64, error)
	TrainModel(ctx context.Context, trainingData []*EngineWorkflowExecutionData) error
}

type TimeSeriesAnalyzer interface {
	AnalyzeTrends(ctx context.Context, data []*EngineWorkflowExecutionData, timeRange WorkflowTimeRange) (*TimeSeriesTrendAnalysis, error)
	DetectAnomalies(ctx context.Context, data []*EngineWorkflowExecutionData) ([]*AnomalyResult, error)
	ForecastMetrics(ctx context.Context, data []*EngineWorkflowExecutionData, horizonHours int) (*ForecastResult, error)
}

type TimeSeriesTrendAnalysis struct {
	Trend       string                 `json:"trend"`
	Confidence  float64                `json:"confidence"`
	Slope       float64                `json:"slope"`
	Seasonality map[string]interface{} `json:"seasonality"`
	Metadata    map[string]interface{} `json:"metadata"`
}

type AnomalyResult struct {
	Timestamp  time.Time              `json:"timestamp"`
	Value      float64                `json:"value"`
	Expected   float64                `json:"expected"`
	Severity   string                 `json:"severity"`
	Confidence float64                `json:"confidence"`
	Metadata   map[string]interface{} `json:"metadata"`
}

type ForecastResult struct {
	Predictions []PredictionPoint      `json:"predictions"`
	Confidence  float64                `json:"confidence"`
	Method      string                 `json:"method"`
	Metadata    map[string]interface{} `json:"metadata"`
}

type PredictionPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
	Upper     float64   `json:"upper_bound"`
	Lower     float64   `json:"lower_bound"`
}

// @deprecated RULE 12 VIOLATION: Creates AI-specific clustering interface instead of enhancing llm.Client
// Migration: Use enhanced llm.Client.ClusterWorkflows(), llm.Client.FindSimilarWorkflows() methods directly
// Business Requirements: BR-CLUSTER-001 - now served by enhanced llm.Client
//
// REMOVED - use enhanced llm.Client methods:
// - llmClient.ClusterWorkflows(ctx, executionData, config)
// - llmClient.FindSimilarWorkflows(ctx, workflow, limit)

type WorkflowCluster struct {
	ID       string                         `json:"id"`
	Centroid map[string]float64             `json:"centroid"`
	Members  []*EngineWorkflowExecutionData `json:"members"`
	Size     int                            `json:"size"`
	Cohesion float64                        `json:"cohesion"`
	Metadata map[string]interface{}         `json:"metadata"`
}

type SimilarWorkflow struct {
	Workflow   *Workflow `json:"workflow"`
	Similarity float64   `json:"similarity"`
	Reasons    []string  `json:"reasons"`
}

type AnomalyDetector interface {
	DetectAnomalies(ctx context.Context, data []*EngineWorkflowExecutionData, baseline *BaselineStatistics) ([]*AnomalyResult, error)
	UpdateBaseline(ctx context.Context, data []*EngineWorkflowExecutionData) (*BaselineStatistics, error)
	GetBaseline(ctx context.Context, workflowType string) (*BaselineStatistics, error)
}

// @deprecated RULE 12 VIOLATION: Creates dedicated AI interface instead of enhancing existing llm.Client
// Migration: Use enhanced llm.Client.EvaluateCondition(), llm.Client.ValidateCondition() methods directly
// Business Requirements: BR-COND-001 - now served by enhanced llm.Client
//
// REMOVED - use enhanced llm.Client methods:
// - llmClient.EvaluateCondition(ctx, condition, context)
// - llmClient.ValidateCondition(ctx, condition)

// PostConditionValidator evaluates post-conditions after action execution (DEPRECATED: use ValidatorRegistry)
// This interface is kept for backwards compatibility but is no longer used

// @deprecated RULE 12 VIOLATION: Creates AI-specific optimization interface instead of enhancing llm.Client
// Migration: Use enhanced llm.Client.OptimizeWorkflow(), llm.Client.SuggestOptimizations() methods directly
// Business Requirements: BR-ORCH-003 - now served by enhanced llm.Client
//
// REMOVED - use enhanced llm.Client methods:
// - llmClient.OptimizeWorkflow(ctx, workflow, executionHistory)
// - llmClient.SuggestOptimizations(ctx, workflow)

// AdaptiveResourceAllocator provides intelligent resource allocation optimization
// Business Requirements: BR-ORCH-002 - Adaptive Resource Allocation Integration
type AdaptiveResourceAllocator interface {
	// OptimizeResourceAllocation optimizes resource allocation based on execution patterns
	OptimizeResourceAllocation(ctx context.Context, workflow *Workflow, executionHistory []*RuntimeWorkflowExecution) (*ResourceAllocationResult, error)

	// OptimizeForClusterCapacity adapts resource allocation to cluster capacity constraints
	OptimizeForClusterCapacity(ctx context.Context, workflow *Workflow, clusterCapacity *ClusterCapacity) (*ResourceAllocationResult, error)

	// PredictResourceRequirements predicts future resource needs based on historical patterns
	PredictResourceRequirements(ctx context.Context, workflow *Workflow, historicalPatterns []*RuntimeWorkflowExecution) (*ResourcePrediction, error)
}

// ExecutionScheduler provides intelligent workflow execution scheduling optimization
// Business Requirements: BR-ORCH-003 - Execution Scheduling Integration
type ExecutionScheduler interface {
	// OptimizeScheduling optimizes execution scheduling based on workflow patterns
	OptimizeScheduling(ctx context.Context, workflows []*Workflow, executionHistory []*RuntimeWorkflowExecution) (*SchedulingResult, error)

	// ScheduleForSystemLoad adapts scheduling to current system load constraints
	ScheduleForSystemLoad(ctx context.Context, workflows []*Workflow, systemLoad *SystemLoad) (*SchedulingResult, error)

	// PredictOptimalScheduling predicts optimal scheduling based on historical patterns
	PredictOptimalScheduling(ctx context.Context, workflows []*Workflow, historicalPatterns []*RuntimeWorkflowExecution) (*SchedulingPrediction, error)

	// ScheduleWithPriority schedules workflows considering business priority ordering
	ScheduleWithPriority(ctx context.Context, workflows []*Workflow) (*PrioritySchedulingResult, error)
}

// FeedbackProcessor provides intelligent feedback loop processing for continuous optimization
// Business Requirements: BR-ORCH-001 - Feedback Loop Integration
type FeedbackProcessor interface {
	// ProcessFeedbackLoop processes feedback to improve optimization accuracy
	// RULE 12 COMPLIANCE: SelfOptimizer replaced with llm.Client
	ProcessFeedbackLoop(ctx context.Context, workflow *Workflow, feedbackData []*ExecutionFeedback, llmClient llm.Client) (*FeedbackLoopResult, error)

	// AdaptOptimizationStrategy adapts optimization strategies based on performance feedback
	AdaptOptimizationStrategy(ctx context.Context, workflow *Workflow, performanceFeedback *PerformanceFeedback) (*StrategyAdaptationResult, error)

	// ProcessConvergenceCycle processes feedback cycles to achieve optimization convergence
	// RULE 12 COMPLIANCE: SelfOptimizer replaced with llm.Client
	ProcessConvergenceCycle(ctx context.Context, workflow *Workflow, feedbackCycle []*ExecutionFeedback, llmClient llm.Client) (*FeedbackConvergenceResult, error)

	// AnalyzeRealTimeFeedback analyzes real-time feedback for actionable optimization insights
	AnalyzeRealTimeFeedback(ctx context.Context, workflow *Workflow, feedbackStream []*ExecutionFeedback) (*RealTimeFeedbackAnalysis, error)

	// ResolveConflictingFeedback resolves conflicting feedback signals
	ResolveConflictingFeedback(ctx context.Context, workflow *Workflow, conflictingFeedback []*ExecutionFeedback) (*ConflictResolutionResult, error)

	// ProcessHighVolumeFeedback processes high-volume feedback streams efficiently
	ProcessHighVolumeFeedback(ctx context.Context, workflow *Workflow, highVolumeFeedback []*ExecutionFeedback) (*HighVolumeFeedbackResult, error)
}

// PromptVersion represents a versioned prompt with performance tracking
type PromptVersion struct {
	ID             string    `json:"id"`
	Version        string    `json:"version"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	PromptTemplate string    `json:"prompt_template"`
	Requirements   []string  `json:"requirements"`
	OutputFormat   string    `json:"output_format"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	IsActive       bool      `json:"is_active"`

	// Performance metrics
	UsageCount     int64                  `json:"usage_count"`
	SuccessRate    float64                `json:"success_rate"`
	AverageLatency time.Duration          `json:"average_latency"`
	ErrorRate      float64                `json:"error_rate"`
	QualityScore   float64                `json:"quality_score"`
	Template       string                 `json:"template"`
	Variables      map[string]interface{} `json:"variables"`
}

// PromptExperiment represents an A/B test experiment
type PromptExperiment struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	Description   string             `json:"description"`
	ControlPrompt string             `json:"control_prompt"`
	TestPrompts   []string           `json:"test_prompts"`
	TrafficSplit  map[string]float64 `json:"traffic_split"`
	StartTime     time.Time          `json:"start_time"`
	EndTime       *time.Time         `json:"end_time,omitempty"`
	Status        string             `json:"status"` // "running", "completed", "paused"

	// Results
	Results      map[string]*ExperimentResult `json:"results"`
	WinnerPrompt string                       `json:"winner_prompt,omitempty"`
	Confidence   float64                      `json:"confidence"`
}

// ExperimentResult tracks performance metrics for a prompt variant
type ExperimentResult struct {
	PromptID         string        `json:"prompt_id"`
	SampleSize       int64         `json:"sample_size"`
	SuccessRate      float64       `json:"success_rate"`
	AverageLatency   time.Duration `json:"average_latency"`
	ErrorRate        float64       `json:"error_rate"`
	QualityScore     float64       `json:"quality_score"`
	UserSatisfaction float64       `json:"user_satisfaction"`
}

// @deprecated RULE 12 VIOLATION: Creates AI-specific prompt interface instead of enhancing llm.Client
// Migration: Use enhanced llm.Client.RegisterPromptVersion(), llm.Client.GetOptimalPrompt(), llm.Client.StartABTest() methods directly
// Business Requirements: BR-AI-022 - now served by enhanced llm.Client
//
// REMOVED - use enhanced llm.Client methods:
// - llmClient.RegisterPromptVersion(ctx, version)
// - llmClient.GetOptimalPrompt(ctx, objective)
// - llmClient.StartABTest(ctx, experiment)

// @deprecated RULE 12 VIOLATION: Creates AI-specific metrics interface instead of enhancing llm.Client
// Migration: Use enhanced llm.Client.CollectMetrics(), llm.Client.GetAggregatedMetrics() methods directly
// Business Requirements: BR-AI-017, BR-AI-025 - now served by enhanced llm.Client
//
// REMOVED - use enhanced llm.Client methods:
// - llmClient.CollectMetrics(ctx, execution)
// - llmClient.GetAggregatedMetrics(ctx, workflowID, timeRange)
// - llmClient.RecordAIRequest(ctx, requestID, prompt, response)

// @deprecated RULE 12 VIOLATION: Creates AI-specific prompt building interface instead of enhancing llm.Client
// Migration: Use enhanced llm.Client.BuildPrompt(), llm.Client.LearnFromExecution() methods directly
// Business Requirements: BR-PROMPT-001 - now served by enhanced llm.Client
//
// REMOVED - use enhanced llm.Client methods:
// - llmClient.BuildPrompt(ctx, template, context)
// - llmClient.LearnFromExecution(ctx, execution)
// - llmClient.GetOptimizedTemplate(ctx, templateID)

// Analytics package interface
// Note: Analytics types moved to pkg/shared/types/analytics.go to resolve import cycles

type EngineWorkflowExecutionData struct {
	ExecutionID string                 `json:"execution_id"`
	WorkflowID  string                 `json:"workflow_id"`
	Timestamp   time.Time              `json:"timestamp"`
	Duration    time.Duration          `json:"duration"`
	Success     bool                   `json:"success"`
	Metrics     map[string]float64     `json:"metrics"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// Additional missing types for interfaces
type PatternDiscoveryConfig struct {
	MinSupport      float64 `json:"min_support"`
	MinConfidence   float64 `json:"min_confidence"`
	MaxPatterns     int     `json:"max_patterns"`
	TimeWindowHours int     `json:"time_window_hours"`
}

type BaselineStatistics struct {
	Mean   float64 `json:"mean"`
	StdDev float64 `json:"std_dev"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Count  int64   `json:"count"`
}

// Missing AIResponseQuality type for interfaces
type AIResponseQuality struct {
	Score      float64 `json:"score"`
	Confidence float64 `json:"confidence"`
	Relevance  float64 `json:"relevance"`
	Clarity    float64 `json:"clarity"`
}

// LearningEnhancedPromptBuilder interface for AI-driven prompt optimization
// Business Requirements: BR-AI-PROMPT-001 through BR-AI-PROMPT-004
type LearningEnhancedPromptBuilder interface {
	// Core prompt building functionality
	BuildPrompt(ctx context.Context, template string, context map[string]interface{}) (string, error)

	// Learning from execution outcomes
	GetLearnFromExecution(ctx context.Context, execution *RuntimeWorkflowExecution) error

	// Template optimization
	GetOptimizedTemplate(ctx context.Context, templateID string) (string, error)

	// Enhanced prompt building with advanced features
	GetBuildEnhancedPrompt(ctx context.Context, basePrompt string, context map[string]interface{}) (string, error)
}

// IntelligentWorkflowBuilder interface for AI-driven workflow generation
type IntelligentWorkflowBuilder interface {
	GenerateWorkflow(ctx context.Context, objective *WorkflowObjective) (*ExecutableTemplate, error)
	OptimizeWorkflowStructure(ctx context.Context, template *ExecutableTemplate) (*ExecutableTemplate, error)
	FindWorkflowPatterns(ctx context.Context, criteria *PatternCriteria) ([]*WorkflowPattern, error)
	ApplyWorkflowPattern(ctx context.Context, pattern *WorkflowPattern, workflowContext *WorkflowContext) (*ExecutableTemplate, error)
	ValidateWorkflow(ctx context.Context, template *ExecutableTemplate) *ValidationReport
	SimulateWorkflow(ctx context.Context, template *ExecutableTemplate, scenario *SimulationScenario) (*SimulationResult, error)
	LearnFromWorkflowExecution(ctx context.Context, execution *RuntimeWorkflowExecution)
	// Phase 2 TDD Activations - Medium confidence functions
	AnalyzeObjective(description string, constraints map[string]interface{}) *ObjectiveAnalysisResult

	// Resource Constraint Management - Business Requirement: BR-RESOURCE-001
	ApplyResourceConstraintManagement(ctx context.Context, template *ExecutableTemplate, objective *WorkflowObjective) (*ExecutableTemplate, error)

	// Performance Calculation Methods - Business Requirement: BR-SCHEDULING-002
	CalculateTimeImprovement(baseline, optimized *WorkflowMetrics) float64
	CalculateReliabilityImprovement(baseline, optimized *WorkflowMetrics) float64
	CalculateResourceEfficiencyGain(baseline, optimized *WorkflowMetrics) float64
	CalculateOverallOptimizationScore(timeImprovement, reliabilityImprovement, resourceGain float64) float64
}
