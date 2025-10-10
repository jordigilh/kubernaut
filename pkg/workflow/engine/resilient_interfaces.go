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
	"time"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
)

// Business Requirements: BR-WF-541, BR-ORCH-001, BR-ORCH-004, BR-ORK-002
// Resilient Workflow Engine interfaces backed by business requirements

// ResilientWorkflowEngine extends WorkflowEngine with resilience capabilities
// BR-WF-541: <10% workflow termination rate, >40% performance improvement
// BR-ORCH-001: Self-optimization with ≥80% confidence, ≥15% performance gains
type ResilientWorkflowEngine struct {
	// Core engine (following guideline #11: reuse existing code)
	defaultEngine *DefaultWorkflowEngine

	// Business requirement components
	failureHandler FailureHandler        // BR-ORCH-004: Learning from execution failures
	healthChecker  WorkflowHealthChecker // BR-ORCH-011: Operational visibility
	// RULE 12 COMPLIANCE: Using enhanced llm.Client instead of deprecated OptimizationEngine
	llmClient           llm.Client          // BR-ORCH-001: Self-optimization via enhanced AI client
	statisticsCollector StatisticsCollector // BR-ORK-003: Performance trend analysis

	// BR-WF-541: Parallel execution configuration
	maxPartialFailures int     // <10% workflow termination rate
	criticalStepRatio  float64 // Min % of critical steps that must succeed

	// BR-ORCH-001: Self-optimization configuration
	optimizationHistory     []*RuntimeWorkflowExecution // Execution history for optimization
	selfOptimizationEnabled bool                        // Enable/disable self-optimization

	// Configuration and logging
	config *ResilientWorkflowConfig
	log    Logger
}

// ResilientWorkflowConfig holds configuration for resilient workflow execution
// Following guideline #20: use configuration settings, never hardcode
type ResilientWorkflowConfig struct {
	// BR-WF-541: Parallel execution resilience configuration
	MaxPartialFailures       int           `yaml:"max_partial_failures" default:"2"`  // <10% termination rate
	CriticalStepRatio        float64       `yaml:"critical_step_ratio" default:"0.8"` // 80% critical steps must succeed
	ParallelExecutionTimeout time.Duration `yaml:"parallel_execution_timeout" default:"10m"`

	// BR-ORCH-001: Self-optimization configuration
	OptimizationConfidenceThreshold float64       `yaml:"optimization_confidence_threshold" default:"0.80"` // ≥80% confidence
	PerformanceGainTarget           float64       `yaml:"performance_gain_target" default:"0.15"`           // ≥15% performance gains
	OptimizationInterval            time.Duration `yaml:"optimization_interval" default:"1h"`

	// BR-ORCH-004: Learning configuration
	LearningEnabled                bool    `yaml:"learning_enabled" default:"true"`
	MinExecutionHistoryForLearning int     `yaml:"min_execution_history_for_learning" default:"10"`
	LearningConfidenceThreshold    float64 `yaml:"learning_confidence_threshold" default:"0.80"`

	// BR-ORK-003: Statistics and monitoring configuration
	StatisticsCollectionEnabled bool          `yaml:"statistics_collection_enabled" default:"true"`
	PerformanceTrendWindow      time.Duration `yaml:"performance_trend_window" default:"7d"`
	HealthCheckInterval         time.Duration `yaml:"health_check_interval" default:"1m"`
}

// FailureHandler manages failure scenarios and learning
// BR-ORCH-004: MUST learn from execution failures and adjust retry strategies
type FailureHandler interface {
	HandleStepFailure(ctx context.Context, step *ExecutableWorkflowStep,
		failure *StepFailure, policy FailurePolicy) (*FailureDecision, error)
	CalculateWorkflowHealth(execution *RuntimeWorkflowExecution) *WorkflowHealth
	ShouldTerminateWorkflow(health *WorkflowHealth) bool

	// BR-ORCH-004: Learning capabilities
	GetLearningMetrics() *LearningMetrics
	GetAdaptiveRetryStrategies() []*AdaptiveRetryStrategy
	CalculateRetryEffectiveness() float64

	// Test configuration methods (following guideline #32: extend existing mocks)
	SetPartialFailureRate(rate float64)
	SetFailurePolicy(policy string)
	SetStepExecutionDelay(delay time.Duration)
	SetExecutionHistory(history []*RuntimeWorkflowExecution)
	EnableLearning(enabled bool)
	SetRetryHistory(history []*RuntimeWorkflowExecution)
	EnableRetryLearning(enabled bool)
}

// WorkflowHealthChecker monitors workflow health and performance
// BR-ORCH-011: MUST provide operational visibility (≥85% system health, ≥90% success rates)
type WorkflowHealthChecker interface {
	CheckHealth(ctx context.Context, execution *RuntimeWorkflowExecution) (*WorkflowHealth, error)
	CalculateSystemHealth(executions []*RuntimeWorkflowExecution) *SystemHealthMetrics
	GenerateHealthRecommendations(health *WorkflowHealth) []HealthRecommendation
}

// OptimizationEngine handles self-optimization capabilities
// BR-ORCH-001: MUST continuously optimize with ≥80% confidence, ≥15% performance gains
// @deprecated RULE 12 VIOLATION: OptimizationEngine interface violates Rule 12 AI/ML methodology
// Migration: Use enhanced llm.Client.OptimizeWorkflow(), llm.Client.SuggestOptimizations() methods directly
// Business Requirements: BR-ORCH-001 - now served by enhanced llm.Client
type OptimizationEngine interface {
	OptimizeOrchestrationStrategies(ctx context.Context, workflow *Workflow,
		history []*RuntimeWorkflowExecution) (*OptimizationResult, error)
	AnalyzeOptimizationOpportunities(workflow *Workflow) ([]*OptimizationCandidate, error)
	ApplyOptimizations(ctx context.Context, workflow *Workflow,
		optimizations []*OptimizationCandidate) (*Workflow, error)
}

// StatisticsCollector gathers performance and trend data
// BR-ORK-003: MUST implement execution metrics collection and performance trend analysis
type StatisticsCollector interface {
	CollectExecutionStatistics(execution *RuntimeWorkflowExecution) error
	AnalyzePerformanceTrends(timeWindow time.Duration) (*PerformanceTrendAnalysis, error)
	DetectFailurePatterns(executions []*RuntimeWorkflowExecution) ([]*FailurePattern, error)
	GeneratePerformanceReport(ctx context.Context) (*PerformanceReport, error)
}

// Business requirement data structures (following guideline #7: structured field values)

// FailurePolicy defines how to handle step failures (BR-WF-541)
type FailurePolicy string

const (
	FailurePolicyFast     FailurePolicy = "fail_fast"            // BR-WF-001-005: Current behavior for critical steps
	FailurePolicyContinue FailurePolicy = "continue"             // BR-WF-541: Continue despite failures (<10% termination)
	FailurePolicyPartial  FailurePolicy = "partial_success"      // BR-WF-541: Accept partial completion
	FailurePolicyGradual  FailurePolicy = "graceful_degradation" // BR-ORCH-002: Reduce scope with resource adaptation
)

// StepFailure represents a step execution failure
type StepFailure struct {
	StepID       string                 `json:"step_id"`
	ErrorMessage string                 `json:"error_message"`
	ErrorType    string                 `json:"error_type"`
	Timestamp    time.Time              `json:"timestamp"`
	Context      map[string]interface{} `json:"context"`
	RetryCount   int                    `json:"retry_count"`
	IsCritical   bool                   `json:"is_critical"`
}

// FailureDecision represents the decision made by failure handler
type FailureDecision struct {
	Action           FailureAction             `json:"action"`
	ShouldRetry      bool                      `json:"should_retry"`
	ShouldContinue   bool                      `json:"should_continue"`
	FallbackSteps    []*ExecutableWorkflowStep `json:"fallback_steps,omitempty"`
	ImpactAssessment *FailureImpact            `json:"impact_assessment"`
	RetryDelay       time.Duration             `json:"retry_delay"`
	Reason           string                    `json:"reason"`
}

// FailureAction defines the action to take on step failure
type FailureAction string

const (
	ActionRetry     FailureAction = "retry"
	ActionFallback  FailureAction = "fallback"
	ActionContinue  FailureAction = "continue"
	ActionTerminate FailureAction = "terminate"
	ActionDegrade   FailureAction = "degrade"
)

// FailureImpact assesses the business impact of a failure
type FailureImpact struct {
	BusinessImpact    string        `json:"business_impact"` // "critical", "major", "minor", "negligible"
	AffectedFunctions []string      `json:"affected_functions"`
	EstimatedDowntime time.Duration `json:"estimated_downtime"`
	RecoveryOptions   []string      `json:"recovery_options"`
}

// WorkflowHealth represents the health status of a workflow execution
type WorkflowHealth struct {
	TotalSteps       int       `json:"total_steps"`
	CompletedSteps   int       `json:"completed_steps"`
	FailedSteps      int       `json:"failed_steps"`
	CriticalFailures int       `json:"critical_failures"`
	HealthScore      float64   `json:"health_score"` // 0.0-1.0
	CanContinue      bool      `json:"can_continue"`
	Recommendations  []string  `json:"recommendations"`
	LastUpdated      time.Time `json:"last_updated"`
}

// SystemHealthMetrics represents overall system health (BR-ORCH-011)
type SystemHealthMetrics struct {
	OverallHealth    float64               `json:"overall_health"` // ≥85% requirement
	SuccessRate      float64               `json:"success_rate"`   // ≥90% requirement
	ActiveWorkflows  int                   `json:"active_workflows"`
	SystemThroughput float64               `json:"system_throughput"`
	ResourceUsage    *ResourceUsageMetrics `json:"resource_usage"`
	AlertsActive     int                   `json:"alerts_active"`
	LastHealthCheck  time.Time             `json:"last_health_check"`
}

// LearningMetrics tracks learning effectiveness (BR-ORCH-004)
type LearningMetrics struct {
	ConfidenceScore         float64   `json:"confidence_score"` // ≥80% requirement
	PatternsLearned         int       `json:"patterns_learned"`
	SuccessfulAdaptations   int       `json:"successful_adaptations"`
	LearningAccuracy        float64   `json:"learning_accuracy"`
	LastLearningUpdate      time.Time `json:"last_learning_update"`
	AdaptationEffectiveness float64   `json:"adaptation_effectiveness"`
	ImprovementTrend        string    `json:"improvement_trend"` // "improving", "declining", "stable"
}

// AdaptiveRetryStrategy represents learned retry patterns (BR-ORCH-004)
type AdaptiveRetryStrategy struct {
	FailureType       string        `json:"failure_type"`
	OptimalRetryCount int           `json:"optimal_retry_count"`
	OptimalRetryDelay time.Duration `json:"optimal_retry_delay"`
	SuccessRate       float64       `json:"success_rate"`
	Confidence        float64       `json:"confidence"`
	LearningSource    string        `json:"learning_source"`
}

// Note: Using existing OptimizationResult and OptimizationCandidate from models.go
// Following guideline #11: reuse existing code

// OptimizationStrategy defines an optimization approach
type OptimizationStrategy struct {
	Name                string                 `json:"name"`
	Type                string                 `json:"type"`
	Parameters          map[string]interface{} `json:"parameters"`
	TargetMetric        string                 `json:"target_metric"`
	ExpectedImprovement float64                `json:"expected_improvement"`
	Confidence          float64                `json:"confidence"`
}

// PerformanceTrendAnalysis analyzes performance over time (BR-ORK-003)
type PerformanceTrendAnalysis struct {
	TimeWindow         time.Duration         `json:"time_window"`
	TrendDirection     string                `json:"trend_direction"` // "improving", "degrading", "stable"
	PerformanceMetrics *PerformanceMetrics   `json:"performance_metrics"`
	SeasonalPatterns   []*SeasonalPattern    `json:"seasonal_patterns"`
	AnomalyDetection   []*PerformanceAnomaly `json:"anomaly_detection"`
	Recommendations    []string              `json:"recommendations"`
}

// FailurePattern represents recurring failure patterns (BR-ORK-003)
type FailurePattern struct {
	PatternType         string    `json:"pattern_type"`
	Frequency           float64   `json:"frequency"`
	AffectedSteps       []string  `json:"affected_steps"`
	CommonCause         string    `json:"common_cause"`
	DetectionConfidence float64   `json:"detection_confidence"`
	FirstOccurrence     time.Time `json:"first_occurrence"`
	LastOccurrence      time.Time `json:"last_occurrence"`
}

// SeasonalPattern represents time-based performance patterns
type SeasonalPattern struct {
	Period            string    `json:"period"` // "hourly", "daily", "weekly"
	PatternStrength   float64   `json:"pattern_strength"`
	PeakPerformance   time.Time `json:"peak_performance"`
	LowestPerformance time.Time `json:"lowest_performance"`
	Reliability       float64   `json:"reliability"`
}

// PerformanceAnomaly represents detected performance anomalies
type PerformanceAnomaly struct {
	Timestamp      time.Time `json:"timestamp"`
	AnomalyType    string    `json:"anomaly_type"`
	Severity       string    `json:"severity"`
	AffectedMetric string    `json:"affected_metric"`
	ExpectedValue  float64   `json:"expected_value"`
	ActualValue    float64   `json:"actual_value"`
	Deviation      float64   `json:"deviation"`
}

// PerformanceReport summarizes overall performance metrics
type PerformanceReport struct {
	ReportPeriod         time.Duration             `json:"report_period"`
	TotalExecutions      int                       `json:"total_executions"`
	SuccessRate          float64                   `json:"success_rate"`
	AverageExecutionTime time.Duration             `json:"average_execution_time"`
	ResourceEfficiency   float64                   `json:"resource_efficiency"`
	TopFailureReasons    []string                  `json:"top_failure_reasons"`
	PerformanceTrends    *PerformanceTrendAnalysis `json:"performance_trends"`
	OptimizationImpact   *OptimizationImpact       `json:"optimization_impact"`
}

// Note: Using existing OptimizationImpact from models.go
// Following guideline #11: reuse existing code

// HealthRecommendation provides actionable health improvement suggestions
type HealthRecommendation struct {
	Type               string  `json:"type"`
	Description        string  `json:"description"`
	Priority           string  `json:"priority"` // "high", "medium", "low"
	EstimatedImpact    float64 `json:"estimated_impact"`
	ImplementationCost string  `json:"implementation_cost"`
	ActionRequired     bool    `json:"action_required"`
}

// Logger interface for structured logging (following guideline #14: handle all errors with logging)
type Logger interface {
	WithField(key string, value interface{}) Logger
	WithFields(fields map[string]interface{}) Logger
	Debug(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})
	WithError(err error) Logger
}
