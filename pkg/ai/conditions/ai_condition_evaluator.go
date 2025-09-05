package conditions

import (
	"context"
	"time"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
	"github.com/jordigilh/kubernaut/pkg/platform/monitoring"
	"github.com/jordigilh/kubernaut/pkg/workflow/types"
)

// AIConditionEvaluator provides AI-powered condition evaluation for workflows
type AIConditionEvaluator interface {
	// EvaluateMetricCondition uses AI to evaluate metric-based conditions
	EvaluateMetricCondition(ctx context.Context, condition *types.WorkflowCondition, stepContext *types.StepContext) (*ConditionResult, error)

	// EvaluateResourceCondition uses AI to evaluate Kubernetes resource conditions
	EvaluateResourceCondition(ctx context.Context, condition *types.WorkflowCondition, stepContext *types.StepContext) (*ConditionResult, error)

	// EvaluateTimeCondition uses AI to evaluate time-based conditions
	EvaluateTimeCondition(ctx context.Context, condition *types.WorkflowCondition, stepContext *types.StepContext) (*ConditionResult, error)

	// EvaluateExpressionCondition uses AI to parse and evaluate complex expressions
	EvaluateExpressionCondition(ctx context.Context, condition *types.WorkflowCondition, stepContext *types.StepContext) (*ConditionResult, error)

	// EvaluateCustomCondition uses AI for custom condition logic
	EvaluateCustomCondition(ctx context.Context, condition *types.WorkflowCondition, stepContext *types.StepContext) (*ConditionResult, error)

	// IsHealthy returns the health status of the AI condition evaluator
	IsHealthy() bool
}

// ConditionResult represents the result of AI condition evaluation
type ConditionResult struct {
	Satisfied   bool                   `json:"satisfied"`
	Confidence  float64                `json:"confidence"`
	Reasoning   string                 `json:"reasoning"`
	Metadata    map[string]interface{} `json:"metadata"`
	NextActions []string               `json:"next_actions,omitempty"`
	Warnings    []string               `json:"warnings,omitempty"`
	EvaluatedAt time.Time              `json:"evaluated_at"`
}

// AIConditionEvaluatorConfig holds configuration for the AI condition evaluator
type AIConditionEvaluatorConfig struct {
	MaxEvaluationTime       time.Duration `yaml:"max_evaluation_time" default:"15s"`
	ConfidenceThreshold     float64       `yaml:"confidence_threshold" default:"0.75"`
	EnableDetailedLogging   bool          `yaml:"enable_detailed_logging" default:"false"`
	FallbackOnLowConfidence bool          `yaml:"fallback_on_low_confidence" default:"true"`
	UseContextualAnalysis   bool          `yaml:"use_contextual_analysis" default:"true"`
}

// DefaultAIConditionEvaluator implements AIConditionEvaluator using SLM client
type DefaultAIConditionEvaluator struct {
	slmClient         llm.Client
	k8sClient         k8s.Client
	monitoringClients *monitoring.MonitoringClients
	config            *AIConditionEvaluatorConfig
	healthy           bool
}

// NewDefaultAIConditionEvaluator creates a new AI condition evaluator
func NewDefaultAIConditionEvaluator(
	slmClient llm.Client,
	k8sClient k8s.Client,
	monitoringClients *monitoring.MonitoringClients,
	config *AIConditionEvaluatorConfig,
) *DefaultAIConditionEvaluator {
	if config == nil {
		config = &AIConditionEvaluatorConfig{
			MaxEvaluationTime:       15 * time.Second,
			ConfidenceThreshold:     0.75,
			EnableDetailedLogging:   false,
			FallbackOnLowConfidence: true,
			UseContextualAnalysis:   true,
		}
	}

	return &DefaultAIConditionEvaluator{
		slmClient:         slmClient,
		k8sClient:         k8sClient,
		monitoringClients: monitoringClients,
		config:            config,
		healthy:           true,
	}
}

// IsHealthy returns the health status of the AI condition evaluator
func (ace *DefaultAIConditionEvaluator) IsHealthy() bool {
	return ace.healthy && ace.slmClient != nil
}

// ConditionEvaluationContext represents contextual information for condition evaluation
type ConditionEvaluationContext struct {
	CurrentMetrics     map[string]interface{} `json:"current_metrics,omitempty"`
	ResourceStates     map[string]interface{} `json:"resource_states,omitempty"`
	RecentEvents       []interface{}          `json:"recent_events,omitempty"`
	SystemLoad         map[string]float64     `json:"system_load,omitempty"`
	AlertHistory       []interface{}          `json:"alert_history,omitempty"`
	WorkflowHistory    []interface{}          `json:"workflow_history,omitempty"`
	EnvironmentContext map[string]string      `json:"environment_context,omitempty"`
}

// ConditionAnalysisRequest represents a request for AI condition analysis
type ConditionAnalysisRequest struct {
	ConditionType      types.ConditionType         `json:"condition_type"`
	Expression         string                      `json:"expression"`
	Variables          map[string]interface{}      `json:"variables"`
	Context            *ConditionEvaluationContext `json:"context"`
	WorkflowID         string                      `json:"workflow_id"`
	StepID             string                      `json:"step_id"`
	ExecutionID        string                      `json:"execution_id"`
	Timestamp          time.Time                   `json:"timestamp"`
	RequiredConfidence float64                     `json:"required_confidence"`
}

// ConditionAnalysisResponse represents the AI response for condition evaluation
type ConditionAnalysisResponse struct {
	Satisfied          bool                   `json:"satisfied"`
	Confidence         float64                `json:"confidence"`
	Reasoning          string                 `json:"reasoning"`
	DetailedAnalysis   map[string]interface{} `json:"detailed_analysis"`
	Recommendations    []string               `json:"recommendations"`
	Warnings           []string               `json:"warnings"`
	AlternativeActions []string               `json:"alternative_actions"`
	Metadata           map[string]interface{} `json:"metadata"`
}
