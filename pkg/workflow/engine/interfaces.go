package engine

import (
	"context"
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Core workflow interfaces
type WorkflowEngine interface {
	Execute(ctx context.Context, workflow *Workflow) (*RuntimeWorkflowExecution, error)
	GetExecution(ctx context.Context, executionID string) (*RuntimeWorkflowExecution, error)
	ListExecutions(ctx context.Context, workflowID string) ([]*RuntimeWorkflowExecution, error)
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

type ClusteringEngine interface {
	ClusterWorkflows(ctx context.Context, data []*EngineWorkflowExecutionData, config *PatternDiscoveryConfig) ([]*WorkflowCluster, error)
	FindSimilarWorkflows(ctx context.Context, workflow *Workflow, limit int) ([]*SimilarWorkflow, error)
}

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

// AI and optimization interfaces
type AIConditionEvaluator interface {
	EvaluateCondition(ctx context.Context, condition *ExecutableCondition, context *StepContext) (bool, error)
	ValidateCondition(ctx context.Context, condition *ExecutableCondition) error
}

// PostConditionValidator evaluates post-conditions after action execution (DEPRECATED: use ValidatorRegistry)
// This interface is kept for backwards compatibility but is no longer used

type SelfOptimizer interface {
	OptimizeWorkflow(ctx context.Context, workflow *Workflow, executionHistory []*RuntimeWorkflowExecution) (*Workflow, error)
	SuggestImprovements(ctx context.Context, workflow *Workflow) ([]*OptimizationSuggestion, error)
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

type PromptOptimizer interface {
	RegisterPromptVersion(version *PromptVersion) error
	GetOptimalPrompt(ctx context.Context, objective *WorkflowObjective) (*PromptVersion, error)
	StartABTest(experiment *PromptExperiment) error
	RecordPromptMetrics(promptID string, success bool, latency time.Duration, qualityScore float64)
	EvaluateExperiments()
	GetPromptStatistics() map[string]*PromptVersion
	GetRunningExperiments() []*PromptExperiment
}

type AIMetricsCollector interface {
	CollectMetrics(ctx context.Context, execution *RuntimeWorkflowExecution) (map[string]float64, error)
	GetAggregatedMetrics(ctx context.Context, workflowID string, timeRange WorkflowTimeRange) (map[string]float64, error)
	RecordAIRequest(ctx context.Context, requestID string, prompt string, response string) error
	EvaluateResponseQuality(ctx context.Context, response string, context map[string]interface{}) (*AIResponseQuality, error)
}

type LearningEnhancedPromptBuilder interface {
	BuildPrompt(ctx context.Context, template string, context map[string]interface{}) (string, error)
	GetLearnFromExecution(ctx context.Context, execution *RuntimeWorkflowExecution) error
	GetGetOptimizedTemplate(ctx context.Context, templateID string) (string, error)
	GetBuildEnhancedPrompt(ctx context.Context, basePrompt string, context map[string]interface{}) (string, error)
}

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

// IntelligentWorkflowBuilder interface for AI-driven workflow generation
type IntelligentWorkflowBuilder interface {
	GenerateWorkflow(ctx context.Context, objective *WorkflowObjective) (*ExecutableTemplate, error)
	OptimizeWorkflowStructure(ctx context.Context, template *ExecutableTemplate) (*ExecutableTemplate, error)
	FindWorkflowPatterns(ctx context.Context, criteria *PatternCriteria) ([]*WorkflowPattern, error)
	ApplyWorkflowPattern(ctx context.Context, pattern *WorkflowPattern, workflowContext *WorkflowContext) (*ExecutableTemplate, error)
	ValidateWorkflow(ctx context.Context, template *ExecutableTemplate) (*ValidationReport, error)
	SimulateWorkflow(ctx context.Context, template *ExecutableTemplate, scenario *SimulationScenario) (*SimulationResult, error)
	LearnFromWorkflowExecution(ctx context.Context, execution *RuntimeWorkflowExecution) error
}
