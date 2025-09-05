package engine

import (
	"encoding/json"
	"time"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
)

// Core Workflow Types

// Workflow represents a complex multi-step automation process
type Workflow struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Version     string                 `json:"version"`
	Template    *WorkflowTemplate      `json:"template"`
	Status      WorkflowStatus         `json:"status"`
	Metadata    map[string]interface{} `json:"metadata"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// WorkflowTemplate defines the structure and logic of a workflow
type WorkflowTemplate struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Version     string                 `json:"version"`
	Steps       []*WorkflowStep        `json:"steps"`
	Conditions  []*WorkflowCondition   `json:"conditions"`
	Variables   map[string]interface{} `json:"variables"`
	Timeouts    *WorkflowTimeouts      `json:"timeouts"`
	Recovery    *RecoveryPolicy        `json:"recovery"`
	Tags        []string               `json:"tags"`
	CreatedBy   string                 `json:"created_by"`
	CreatedAt   time.Time              `json:"created_at"`
}

// WorkflowStep represents a single step in a workflow
type WorkflowStep struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Type         StepType               `json:"type"`
	Action       *StepAction            `json:"action,omitempty"`
	Condition    *WorkflowCondition     `json:"condition,omitempty"`
	Dependencies []string               `json:"dependencies"`
	Timeout      time.Duration          `json:"timeout"`
	RetryPolicy  *RetryPolicy           `json:"retry_policy"`
	OnSuccess    []string               `json:"on_success"`
	OnFailure    []string               `json:"on_failure"`
	Variables    map[string]interface{} `json:"variables"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// StepType defines the type of workflow step
type StepType string

const (
	StepTypeAction     StepType = "action"
	StepTypeCondition  StepType = "condition"
	StepTypeParallel   StepType = "parallel"
	StepTypeSequential StepType = "sequential"
	StepTypeLoop       StepType = "loop"
	StepTypeWait       StepType = "wait"
	StepTypeDecision   StepType = "decision"
	StepTypeSubflow    StepType = "subflow"
)

// StepAction defines an action to be performed in a workflow step
type StepAction struct {
	Type       string                 `json:"type"`
	Parameters map[string]interface{} `json:"parameters"`
	Target     *ActionTarget          `json:"target"`
	Validation *ActionValidation      `json:"validation"`
	Rollback   *RollbackAction        `json:"rollback,omitempty"`
}

// ActionTarget specifies the target of an action
type ActionTarget struct {
	Type      string            `json:"type"` // "kubernetes", "prometheus", "custom"
	Namespace string            `json:"namespace"`
	Resource  string            `json:"resource"`
	Name      string            `json:"name"`
	Selector  map[string]string `json:"selector,omitempty"`
	Endpoint  string            `json:"endpoint,omitempty"`
}

// WorkflowCondition defines conditional logic in workflows
type WorkflowCondition struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Type       ConditionType          `json:"type"`
	Expression string                 `json:"expression"`
	Variables  map[string]interface{} `json:"variables"`
	Timeout    time.Duration          `json:"timeout"`
}

// ConditionType defines the type of condition
type ConditionType string

const (
	ConditionTypeMetric     ConditionType = "metric"
	ConditionTypeResource   ConditionType = "resource"
	ConditionTypeTime       ConditionType = "time"
	ConditionTypeCustom     ConditionType = "custom"
	ConditionTypeExpression ConditionType = "expression"
)

// Workflow Execution Types

// WorkflowExecution represents an active execution of a workflow
type WorkflowExecution struct {
	ID          string                 `json:"id"`
	WorkflowID  string                 `json:"workflow_id"`
	Status      ExecutionStatus        `json:"status"`
	Input       *WorkflowInput         `json:"input"`
	Output      *WorkflowOutput        `json:"output,omitempty"`
	Context     *ExecutionContext      `json:"context"`
	Steps       []*StepExecution       `json:"steps"`
	CurrentStep int                    `json:"current_step"`
	StartTime   time.Time              `json:"start_time"`
	EndTime     *time.Time             `json:"end_time,omitempty"`
	Duration    time.Duration          `json:"duration"`
	Error       string                 `json:"error,omitempty"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// ExecutionStatus represents the status of a workflow execution
type ExecutionStatus string

const (
	ExecutionStatusPending     ExecutionStatus = "pending"
	ExecutionStatusRunning     ExecutionStatus = "running"
	ExecutionStatusCompleted   ExecutionStatus = "completed"
	ExecutionStatusFailed      ExecutionStatus = "failed"
	ExecutionStatusCancelled   ExecutionStatus = "cancelled"
	ExecutionStatusPaused      ExecutionStatus = "paused"
	ExecutionStatusRollingBack ExecutionStatus = "rolling_back"
)

// WorkflowInput contains input data for workflow execution
type WorkflowInput struct {
	Alert       *AlertContext          `json:"alert,omitempty"`
	Resource    *ResourceContext       `json:"resource,omitempty"`
	Parameters  map[string]interface{} `json:"parameters"`
	Environment string                 `json:"environment"`
	Priority    Priority               `json:"priority"`
	Requester   string                 `json:"requester"`
	Context     map[string]interface{} `json:"context"`
}

// WorkflowOutput contains output data from workflow execution
type WorkflowOutput struct {
	Success         bool                   `json:"success"`
	Results         map[string]interface{} `json:"results"`
	Actions         []*ActionResult        `json:"actions"`
	Metrics         *ExecutionMetrics      `json:"metrics"`
	Effectiveness   *EffectivenessResult   `json:"effectiveness,omitempty"`
	Recommendations []string               `json:"recommendations"`
}

// ActionResult represents the result of an action execution
type ActionResult struct {
	ActionID  string                             `json:"action_id"`
	Type      string                             `json:"type"`
	Success   bool                               `json:"success"`
	StartTime time.Time                          `json:"start_time"`
	EndTime   time.Time                          `json:"end_time"`
	Duration  time.Duration                      `json:"duration"`
	Output    map[string]interface{}             `json:"output"`
	Error     string                             `json:"error,omitempty"`
	Trace     *actionhistory.ResourceActionTrace `json:"trace,omitempty"`
}

// EffectivenessResult represents effectiveness assessment results
type EffectivenessResult struct {
	Score      float64            `json:"score"`
	Confidence float64            `json:"confidence"`
	Factors    map[string]float64 `json:"factors"`
	Reasoning  string             `json:"reasoning"`
	Assessment string             `json:"assessment"`
	Timestamp  time.Time          `json:"timestamp"`
}

// StepExecution represents the execution state of a single step
type StepExecution struct {
	StepID     string                 `json:"step_id"`
	Status     ExecutionStatus        `json:"status"`
	StartTime  time.Time              `json:"start_time"`
	EndTime    *time.Time             `json:"end_time,omitempty"`
	Duration   time.Duration          `json:"duration"`
	Result     *StepResult            `json:"result,omitempty"`
	Error      string                 `json:"error,omitempty"`
	RetryCount int                    `json:"retry_count"`
	Variables  map[string]interface{} `json:"variables"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// StepResult contains the result of a step execution
type StepResult struct {
	Success     bool                               `json:"success"`
	Error       string                             `json:"error,omitempty"`
	Output      map[string]interface{}             `json:"output"`
	Duration    time.Duration                      `json:"duration"`
	Confidence  float64                            `json:"confidence"`
	NextSteps   []string                           `json:"next_steps"`
	Variables   map[string]interface{}             `json:"variables"`
	Metrics     *StepMetrics                       `json:"metrics"`
	ActionTrace *actionhistory.ResourceActionTrace `json:"action_trace,omitempty"`
	// Legacy Data field for migration (will be removed after full migration)
	Data map[string]interface{} `json:"data,omitempty"`
}

// Adaptation and Optimization Types

// AdaptationRules define how workflows should adapt to changing conditions
type AdaptationRules struct {
	ID          string                  `json:"id"`
	WorkflowID  string                  `json:"workflow_id"`
	Triggers    []*AdaptationTrigger    `json:"triggers"`
	Actions     []*AdaptationAction     `json:"actions"`
	Constraints []*AdaptationConstraint `json:"constraints"`
	Enabled     bool                    `json:"enabled"`
	CreatedAt   time.Time               `json:"created_at"`
	UpdatedAt   time.Time               `json:"updated_at"`
}

// AdaptationTrigger defines when adaptation should occur
type AdaptationTrigger struct {
	Type      TriggerType            `json:"type"`
	Condition *WorkflowCondition     `json:"condition"`
	Threshold float64                `json:"threshold"`
	Variables map[string]interface{} `json:"variables"`
}

// TriggerType defines the type of adaptation trigger
type TriggerType string

const (
	TriggerTypeMetric        TriggerType = "metric"
	TriggerTypePerformance   TriggerType = "performance"
	TriggerTypeEffectiveness TriggerType = "effectiveness"
	TriggerTypeError         TriggerType = "error"
	TriggerTypeTime          TriggerType = "time"
	TriggerTypeCustom        TriggerType = "custom"
)

// AdaptationAction defines what adaptation to perform
type AdaptationAction struct {
	Type       AdaptationActionType   `json:"type"`
	Target     string                 `json:"target"`    // step ID, parameter name, etc.
	Operation  string                 `json:"operation"` // "modify", "add", "remove", "replace"
	Value      interface{}            `json:"value"`
	Conditions []*WorkflowCondition   `json:"conditions"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// AdaptationActionType defines the type of adaptation action
type AdaptationActionType string

const (
	AdaptationActionModifyStep      AdaptationActionType = "modify_step"
	AdaptationActionAddStep         AdaptationActionType = "add_step"
	AdaptationActionRemoveStep      AdaptationActionType = "remove_step"
	AdaptationActionModifyParameter AdaptationActionType = "modify_parameter"
	AdaptationActionModifyTimeout   AdaptationActionType = "modify_timeout"
	AdaptationActionModifyRetry     AdaptationActionType = "modify_retry"
	AdaptationActionChangeFlow      AdaptationActionType = "change_flow"
)

// OptimizationResult contains the result of workflow optimization
type OptimizationResult struct {
	ID               string                  `json:"id"`
	WorkflowID       string                  `json:"workflow_id"`
	Type             OptimizationType        `json:"type"`
	Changes          []*OptimizationChange   `json:"changes"`
	Performance      *PerformanceImprovement `json:"performance"`
	Confidence       float64                 `json:"confidence"`
	ValidationResult *ValidationResult       `json:"validation_result"`
	AppliedAt        *time.Time              `json:"applied_at,omitempty"`
	CreatedAt        time.Time               `json:"created_at"`
}

// OptimizationType defines the type of optimization
type OptimizationType string

const (
	OptimizationTypePerformance   OptimizationType = "performance"
	OptimizationTypeEffectiveness OptimizationType = "effectiveness"
	OptimizationTypeReliability   OptimizationType = "reliability"
	OptimizationTypeCost          OptimizationType = "cost"
	OptimizationTypeResource      OptimizationType = "resource"
)

// OptimizationChange represents a specific change in an optimization
type OptimizationChange struct {
	ID          string      `json:"id"`
	Type        string      `json:"type"`
	Target      string      `json:"target"`
	Description string      `json:"description"`
	OldValue    interface{} `json:"old_value"`
	NewValue    interface{} `json:"new_value"`
	Confidence  float64     `json:"confidence"`
	Reasoning   string      `json:"reasoning"`
	Applied     bool        `json:"applied"`
	CreatedAt   time.Time   `json:"created_at"`
}

// PerformanceImprovement represents performance improvements from optimization
type PerformanceImprovement struct {
	ExecutionTime float64 `json:"execution_time"`
	SuccessRate   float64 `json:"success_rate"`
	ResourceUsage float64 `json:"resource_usage"`
	Effectiveness float64 `json:"effectiveness"`
	OverallScore  float64 `json:"overall_score"`
}

// Cross-Cluster Learning Types

// ClusterKnowledge represents knowledge that can be shared across clusters
type ClusterKnowledge struct {
	ID            string                  `json:"id"`
	SourceCluster string                  `json:"source_cluster"`
	Patterns      []*vector.ActionPattern `json:"patterns"`
	Workflows     []*WorkflowTemplate     `json:"workflows"`
	Optimizations []*OptimizationResult   `json:"optimizations"`
	Metrics       *ClusterMetrics         `json:"metrics"`
	Timestamp     time.Time               `json:"timestamp"`
	Signature     string                  `json:"signature"`
}

// ClusterInfo represents information about a cluster in the federation
type ClusterInfo struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Environment  string                 `json:"environment"`
	Region       string                 `json:"region"`
	Version      string                 `json:"version"`
	Capabilities []string               `json:"capabilities"`
	Endpoint     string                 `json:"endpoint"`
	Status       ClusterStatus          `json:"status"`
	Metadata     map[string]interface{} `json:"metadata"`
	LastContact  time.Time              `json:"last_contact"`
	CreatedAt    time.Time              `json:"created_at"`
}

// ClusterStatus represents the status of a cluster
type ClusterStatus string

const (
	ClusterStatusActive      ClusterStatus = "active"
	ClusterStatusInactive    ClusterStatus = "inactive"
	ClusterStatusUnreachable ClusterStatus = "unreachable"
	ClusterStatusMaintenance ClusterStatus = "maintenance"
)

// Real-Time Adaptation Types

// SystemSituation represents the current state and context of the system
type SystemSituation struct {
	Timestamp   time.Time              `json:"timestamp"`
	Metrics     *SystemMetrics         `json:"metrics"`
	Alerts      []*AlertContext        `json:"alerts"`
	Resources   []*ResourceContext     `json:"resources"`
	Trends      *TrendAnalysis         `json:"trends"`
	Predictions *PredictedState        `json:"predictions"`
	Context     map[string]interface{} `json:"context"`
}

// AdaptiveDecision represents a decision made by the adaptive system
type AdaptiveDecision struct {
	ID           string                 `json:"id"`
	Type         DecisionType           `json:"type"`
	Action       *AdaptiveAction        `json:"action,omitempty"`
	Workflow     *WorkflowExecution     `json:"workflow,omitempty"`
	Confidence   float64                `json:"confidence"`
	Reasoning    string                 `json:"reasoning"`
	Alternatives []*DecisionAlternative `json:"alternatives"`
	Timestamp    time.Time              `json:"timestamp"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// DecisionType defines the type of adaptive decision
type DecisionType string

const (
	DecisionTypeImmediate    DecisionType = "immediate"
	DecisionTypeScheduled    DecisionType = "scheduled"
	DecisionTypeProactive    DecisionType = "proactive"
	DecisionTypePreventive   DecisionType = "preventive"
	DecisionTypeOptimization DecisionType = "optimization"
	DecisionTypeNoAction     DecisionType = "no_action"
)

// Proactive Action Types

// PredictedIssue represents an issue that is predicted to occur
type PredictedIssue struct {
	ID           string             `json:"id"`
	Type         IssueType          `json:"type"`
	Severity     Severity           `json:"severity"`
	Probability  float64            `json:"probability"`
	TimeToImpact time.Duration      `json:"time_to_impact"`
	Impact       *ImpactAssessment  `json:"impact"`
	Indicators   []*IssueIndicator  `json:"indicators"`
	Context      *PredictionContext `json:"context"`
	Confidence   float64            `json:"confidence"`
	CreatedAt    time.Time          `json:"created_at"`
}

// IssueType defines the type of predicted issue
type IssueType string

const (
	IssueTypePerformance   IssueType = "performance"
	IssueTypeReliability   IssueType = "reliability"
	IssueTypeCapacity      IssueType = "capacity"
	IssueTypeSecurity      IssueType = "security"
	IssueTypeCompliance    IssueType = "compliance"
	IssueTypeConfiguration IssueType = "configuration"
)

// ProactiveAction represents an action that can be taken proactively
type ProactiveAction struct {
	ID         string                 `json:"id"`
	Type       ProactiveActionType    `json:"type"`
	Target     *ActionTarget          `json:"target"`
	Parameters map[string]interface{} `json:"parameters"`
	Schedule   *ActionSchedule        `json:"schedule"`
	Risk       *RiskAssessment        `json:"risk"`
	Impact     *ImpactAnalysis        `json:"impact"`
	Validation *ActionValidation      `json:"validation"`
	Rollback   *RollbackAction        `json:"rollback"`
	CreatedAt  time.Time              `json:"created_at"`
}

// ProactiveActionType defines the type of proactive action
type ProactiveActionType string

const (
	ProactiveActionTypePreventive       ProactiveActionType = "preventive"
	ProactiveActionTypeOptimization     ProactiveActionType = "optimization"
	ProactiveActionTypeCapacityPlanning ProactiveActionType = "capacity_planning"
	ProactiveActionTypeMaintenance      ProactiveActionType = "maintenance"
	ProactiveActionTypeConfiguration    ProactiveActionType = "configuration"
)

// Supporting Types

// Priority defines the priority level of an action or workflow
type Priority string

const (
	PriorityLow       Priority = "low"
	PriorityMedium    Priority = "medium"
	PriorityHigh      Priority = "high"
	PriorityCritical  Priority = "critical"
	PriorityEmergency Priority = "emergency"
)

// Severity defines the severity level of an issue or alert
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityError    Severity = "error"
	SeverityCritical Severity = "critical"
)

// TimeRange represents a time range for analysis
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Context Types

// AlertContext provides context about an alert
type AlertContext struct {
	Name         string                 `json:"name"`
	Severity     Severity               `json:"severity"`
	Labels       map[string]string      `json:"labels"`
	Annotations  map[string]string      `json:"annotations"`
	StartsAt     time.Time              `json:"starts_at"`
	EndsAt       *time.Time             `json:"ends_at,omitempty"`
	GeneratorURL string                 `json:"generator_url,omitempty"`
	Fingerprint  string                 `json:"fingerprint"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// ResourceContext provides context about a Kubernetes resource
type ResourceContext struct {
	Namespace   string                 `json:"namespace"`
	Kind        string                 `json:"kind"`
	Name        string                 `json:"name"`
	Labels      map[string]string      `json:"labels"`
	Annotations map[string]string      `json:"annotations"`
	Status      map[string]interface{} `json:"status"`
	Spec        map[string]interface{} `json:"spec"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// ActionContext provides context for action decision making
type ActionContext struct {
	Alert       *AlertContext                        `json:"alert,omitempty"`
	Resource    *ResourceContext                     `json:"resource,omitempty"`
	Environment string                               `json:"environment"`
	Cluster     string                               `json:"cluster"`
	History     []*actionhistory.ResourceActionTrace `json:"history"`
	Metrics     *SystemMetrics                       `json:"metrics"`
	Context     map[string]interface{}               `json:"context"`
}

// ExecutionContext provides context for workflow execution
type ExecutionContext struct {
	Environment   string                 `json:"environment"`
	Cluster       string                 `json:"cluster"`
	User          string                 `json:"user"`
	RequestID     string                 `json:"request_id"`
	TraceID       string                 `json:"trace_id"`
	CorrelationID string                 `json:"correlation_id"`
	Variables     map[string]interface{} `json:"variables"`
	Secrets       map[string]string      `json:"secrets,omitempty"`
	Configuration map[string]interface{} `json:"configuration"`
}

// StepContext provides context for step execution
type StepContext struct {
	ExecutionID   string                 `json:"execution_id"`
	StepID        string                 `json:"step_id"`
	Variables     map[string]interface{} `json:"variables"`
	PreviousSteps []*StepResult          `json:"previous_steps"`
	Environment   *ExecutionContext      `json:"environment"`
	Timeout       time.Duration          `json:"timeout"`
}

// Metrics and Analysis Types

// SystemMetrics represents current system metrics
type SystemMetrics struct {
	Timestamp time.Time              `json:"timestamp"`
	CPU       *ResourceMetrics       `json:"cpu"`
	Memory    *ResourceMetrics       `json:"memory"`
	Network   *NetworkMetrics        `json:"network"`
	Disk      *DiskMetrics           `json:"disk"`
	Custom    map[string]interface{} `json:"custom"`
}

// ResourceMetrics represents metrics for a specific resource type
type ResourceMetrics struct {
	Usage       float64 `json:"usage"`
	Capacity    float64 `json:"capacity"`
	Utilization float64 `json:"utilization"`
	Requests    float64 `json:"requests"`
	Limits      float64 `json:"limits"`
}

// NetworkMetrics represents network-related metrics
type NetworkMetrics struct {
	InboundBytes    float64 `json:"inbound_bytes"`
	OutboundBytes   float64 `json:"outbound_bytes"`
	InboundPackets  float64 `json:"inbound_packets"`
	OutboundPackets float64 `json:"outbound_packets"`
	Connections     int     `json:"connections"`
	Errors          float64 `json:"errors"`
}

// DiskMetrics represents disk-related metrics
type DiskMetrics struct {
	UsedBytes      float64 `json:"used_bytes"`
	AvailableBytes float64 `json:"available_bytes"`
	TotalBytes     float64 `json:"total_bytes"`
	Utilization    float64 `json:"utilization"`
	IOPSRead       float64 `json:"iops_read"`
	IOPSWrite      float64 `json:"iops_write"`
}

// ExecutionMetrics represents metrics from workflow execution
type ExecutionMetrics struct {
	Duration      time.Duration         `json:"duration"`
	StepCount     int                   `json:"step_count"`
	SuccessCount  int                   `json:"success_count"`
	FailureCount  int                   `json:"failure_count"`
	RetryCount    int                   `json:"retry_count"`
	ResourceUsage *ResourceUsageMetrics `json:"resource_usage"`
	Performance   *PerformanceMetrics   `json:"performance"`
}

// StepMetrics represents metrics from step execution
type StepMetrics struct {
	Duration      time.Duration         `json:"duration"`
	RetryCount    int                   `json:"retry_count"`
	ResourceUsage *ResourceUsageMetrics `json:"resource_usage"`
	ApiCalls      int                   `json:"api_calls"`
	DataProcessed int64                 `json:"data_processed"`
}

// Policy and Configuration Types

// RetryPolicy defines retry behavior for steps
type RetryPolicy struct {
	MaxRetries  int           `json:"max_retries"`
	Delay       time.Duration `json:"delay"`
	Backoff     BackoffType   `json:"backoff"`
	BackoffRate float64       `json:"backoff_rate"`
	Conditions  []string      `json:"conditions"`
}

// BackoffType defines the type of backoff strategy
type BackoffType string

const (
	BackoffTypeFixed       BackoffType = "fixed"
	BackoffTypeExponential BackoffType = "exponential"
	BackoffTypeLinear      BackoffType = "linear"
	BackoffTypeRandom      BackoffType = "random"
)

// RecoveryPolicy defines recovery behavior for workflows
type RecoveryPolicy struct {
	Enabled         bool                  `json:"enabled"`
	MaxRecoveryTime time.Duration         `json:"max_recovery_time"`
	Strategies      []*RecoveryStrategy   `json:"strategies"`
	Notifications   []*NotificationConfig `json:"notifications"`
}

// RecoveryStrategy defines a recovery strategy
type RecoveryStrategy struct {
	Type       RecoveryType         `json:"type"`
	Conditions []*WorkflowCondition `json:"conditions"`
	Actions    []*RecoveryAction    `json:"actions"`
	Priority   Priority             `json:"priority"`
}

// RecoveryType defines the type of recovery strategy
type RecoveryType string

const (
	RecoveryTypeRetry     RecoveryType = "retry"
	RecoveryTypeRollback  RecoveryType = "rollback"
	RecoveryTypeSkip      RecoveryType = "skip"
	RecoveryTypeAlternate RecoveryType = "alternate"
	RecoveryTypeManual    RecoveryType = "manual"
)

// WorkflowTimeouts defines timeout configuration for workflows
type WorkflowTimeouts struct {
	Execution time.Duration `json:"execution"`
	Step      time.Duration `json:"step"`
	Condition time.Duration `json:"condition"`
	Recovery  time.Duration `json:"recovery"`
}

// Utility Types

// JSONMap is a type alias for map[string]interface{} for JSON compatibility
type JSONMap = map[string]interface{}

// Serialization helpers

// MarshalJSON customizes JSON marshaling for complex types
func (w *WorkflowExecution) MarshalJSON() ([]byte, error) {
	type Alias WorkflowExecution
	return json.Marshal(&struct {
		*Alias
		DurationMs int64 `json:"duration_ms"`
	}{
		Alias:      (*Alias)(w),
		DurationMs: w.Duration.Milliseconds(),
	})
}

// Performance Analysis Types
type OptimizationTarget struct {
	Type       string                 `json:"type"`
	Metric     string                 `json:"metric"`
	Threshold  float64                `json:"threshold"`
	Priority   int                    `json:"priority"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

type ParameterSet struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Parameters map[string]interface{} `json:"parameters"`
	Versions   []string               `json:"versions,omitempty"`
	Active     bool                   `json:"active"`
	CreatedAt  time.Time              `json:"created_at"`
}

type ValidationCriteria struct {
	Rules      []*ValidationRule `json:"rules"`
	Timeout    time.Duration     `json:"timeout"`
	Retries    int               `json:"retries"`
	StrictMode bool              `json:"strict_mode"`
}

type PerformanceAnalysis struct {
	WorkflowID      string                    `json:"workflow_id"`
	ExecutionTime   time.Duration             `json:"execution_time"`
	ResourceUsage   *ResourceUsageMetrics     `json:"resource_usage"`
	Bottlenecks     []*Bottleneck             `json:"bottlenecks"`
	Optimizations   []*OptimizationCandidate  `json:"optimizations"`
	Effectiveness   float64                   `json:"effectiveness"`
	CostEfficiency  float64                   `json:"cost_efficiency"`
	Recommendations []*OptimizationSuggestion `json:"recommendations"`
	AnalyzedAt      time.Time                 `json:"analyzed_at"`
}

// ResourceUsageMetrics is defined in types.go

type Bottleneck struct {
	ID          string         `json:"id"`
	Type        BottleneckType `json:"type"`
	StepID      string         `json:"step_id"`
	Description string         `json:"description"`
	Impact      float64        `json:"impact"`
	Severity    string         `json:"severity"`
	Suggestion  string         `json:"suggestion"`
}

type BottleneckType string

const (
	BottleneckTypeResource BottleneckType = "resource"
	BottleneckTypeNetwork  BottleneckType = "network"
	BottleneckTypeLogical  BottleneckType = "logical"
	BottleneckTypeTimeout  BottleneckType = "timeout"
)

type OptimizationCandidate struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Target      string                 `json:"target"`
	Description string                 `json:"description"`
	Impact      float64                `json:"impact"`
	Confidence  float64                `json:"confidence"`
	Parameters  map[string]interface{} `json:"parameters"`
	Applied     bool                   `json:"applied"`
}

type OptimizationSuggestion struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Priority    int                    `json:"priority"`
	Impact      float64                `json:"impact"`
	Effort      string                 `json:"effort"`
	Parameters  map[string]interface{} `json:"parameters"`
	Applicable  bool                   `json:"applicable"`
}

// LearningType represents different types of learning
type LearningType string

const (
	LearningTypePerformance   LearningType = "performance"
	LearningTypeEffectiveness LearningType = "effectiveness"
	LearningTypeFailure       LearningType = "failure"
	LearningTypeOptimization  LearningType = "optimization"
	LearningTypeSuccess       LearningType = "success"
	LearningTypePattern       LearningType = "pattern"
)

// LearningActionType represents types of learning actions
type LearningActionType string

const (
	LearningActionUpdateParameter          LearningActionType = "update_parameter"
	LearningActionAddStep                  LearningActionType = "add_step"
	LearningActionRemoveStep               LearningActionType = "remove_step"
	LearningActionTypeOptimizeWorkflow     LearningActionType = "optimize_workflow"
	LearningActionTypeImproveRecovery      LearningActionType = "improve_recovery"
	LearningActionTypeAddValidation        LearningActionType = "add_validation"
	LearningActionTypeUpdateThreshold      LearningActionType = "update_threshold"
	LearningActionTypeCreatePreventionPlan LearningActionType = "create_prevention_plan"
	LearningActionModifyCondition          LearningActionType = "modify_condition"
	LearningActionUpdatePattern            LearningActionType = "update_pattern"
)

type WorkflowLearning struct {
	ID         string                 `json:"id"`
	WorkflowID string                 `json:"workflow_id"`
	Type       LearningType           `json:"type"`
	Trigger    string                 `json:"trigger"`
	Data       map[string]interface{} `json:"data"`
	Actions    []*LearningAction      `json:"actions"`
	Applied    bool                   `json:"applied"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
}

type LearningAction struct {
	ID         string                 `json:"id"`
	Type       LearningActionType     `json:"type"`
	Target     string                 `json:"target"`
	Parameters map[string]interface{} `json:"parameters"`
	Applied    bool                   `json:"applied"`
	Result     string                 `json:"result,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
}

type PreventionPlan struct {
	ID         string                  `json:"id"`
	WorkflowID string                  `json:"workflow_id"`
	Trigger    string                  `json:"trigger"`
	Conditions []*ActionCondition      `json:"conditions"`
	Actions    []*ActionRecommendation `json:"actions"`
	Validation *ValidationEngine       `json:"validation,omitempty"`
	Schedule   string                  `json:"schedule,omitempty"`
	Enabled    bool                    `json:"enabled"`
	CreatedAt  time.Time               `json:"created_at"`
	UpdatedAt  time.Time               `json:"updated_at"`
}

type WorkflowContext struct {
	WorkflowID  string                 `json:"workflow_id"`
	Execution   *WorkflowExecution     `json:"execution"`
	Environment string                 `json:"environment"`
	Cluster     string                 `json:"cluster"`
	Namespace   string                 `json:"namespace"`
	Alert       *AlertContext          `json:"alert,omitempty"`
	Resource    *ResourceContext       `json:"resource,omitempty"`
	Variables   map[string]interface{} `json:"variables,omitempty"`
	History     []*ActionResult        `json:"history,omitempty"`
	Metrics     map[string]float64     `json:"metrics,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
}

type WorkflowObjective struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Description string                 `json:"description"`
	Priority    int                    `json:"priority"`
	Targets     []*OptimizationTarget  `json:"targets"`
	Constraints map[string]interface{} `json:"constraints"`
	Deadline    *time.Time             `json:"deadline,omitempty"`
	Status      string                 `json:"status"`
	Progress    float64                `json:"progress"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

type PatternCriteria struct {
	MinSimilarity     float64           `json:"min_similarity"`
	MinExecutionCount int               `json:"min_execution_count"`
	MinSuccessRate    float64           `json:"min_success_rate"`
	TimeWindow        time.Duration     `json:"time_window"`
	EnvironmentFilter []string          `json:"environment_filter,omitempty"`
	ResourceFilter    map[string]string `json:"resource_filter,omitempty"`
	ExcludePatterns   []string          `json:"exclude_patterns,omitempty"`
}

type WorkflowPattern struct {
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	Type           string             `json:"type"`
	Steps          []*WorkflowStep    `json:"steps"`
	Conditions     []*ActionCondition `json:"conditions"`
	SuccessRate    float64            `json:"success_rate"`
	ExecutionCount int                `json:"execution_count"`
	AverageTime    time.Duration      `json:"average_time"`
	Environments   []string           `json:"environments"`
	ResourceTypes  []string           `json:"resource_types"`
	Confidence     float64            `json:"confidence"`
	LastUsed       time.Time          `json:"last_used"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
}

// Simulation and Validation Types
type SimulationScenario struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	WorkflowID  string                 `json:"workflow_id"`
	Type        SimulationType         `json:"type"`
	Parameters  map[string]interface{} `json:"parameters"`
	Environment string                 `json:"environment"`
	Duration    time.Duration          `json:"duration"`
	Completed   bool                   `json:"completed"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

type SimulationResult struct {
	ID         string                 `json:"id"`
	ScenarioID string                 `json:"scenario_id"`
	Success    bool                   `json:"success"`
	Duration   time.Duration          `json:"duration"`
	Results    map[string]interface{} `json:"results"`
	Metrics    map[string]float64     `json:"metrics"`
	Logs       []string               `json:"logs,omitempty"`
	Errors     []string               `json:"errors,omitempty"`
	RunAt      time.Time              `json:"run_at"`
}

type ValidationReport struct {
	ID          string              `json:"id"`
	WorkflowID  string              `json:"workflow_id"`
	ExecutionID string              `json:"execution_id"`
	Type        ValidationType      `json:"type"`
	Status      string              `json:"status"`
	Results     []*ValidationResult `json:"results"`
	Summary     *ValidationSummary  `json:"summary"`
	CreatedAt   time.Time           `json:"created_at"`
	CompletedAt *time.Time          `json:"completed_at,omitempty"`
}

// ValidationResult is defined in types.go

type ValidationSummary struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

// Additional types will be added as we implement more features...

// Additional missing types for models.go
type WorkflowStatus string

const (
	StatusPending   WorkflowStatus = "pending"
	StatusRunning   WorkflowStatus = "running"
	StatusCompleted WorkflowStatus = "completed"
	StatusFailed    WorkflowStatus = "failed"
)

// PostCondition represents a structured post-condition check
type PostCondition struct {
	Type       PostConditionType `json:"type"`
	Name       string            `json:"name"`
	Expression string            `json:"expression,omitempty"`
	Threshold  *float64          `json:"threshold,omitempty"`
	Expected   interface{}       `json:"expected,omitempty"`
	Timeout    time.Duration     `json:"timeout,omitempty"`
	Critical   bool              `json:"critical"`
	Enabled    bool              `json:"enabled"`
}

// PostConditionType defines the type of post-condition
type PostConditionType string

const (
	PostConditionSuccess    PostConditionType = "success"
	PostConditionConfidence PostConditionType = "confidence"
	PostConditionDuration   PostConditionType = "duration"
	PostConditionOutput     PostConditionType = "output"
	PostConditionNoErrors   PostConditionType = "no_errors"
	PostConditionExpression PostConditionType = "expression"
	PostConditionMetric     PostConditionType = "metric"
	PostConditionResource   PostConditionType = "resource"
)

// ActionValidation contains pre and post-condition validation rules
type ActionValidation struct {
	Valid          bool             `json:"valid"`
	Errors         []string         `json:"errors"`
	Warnings       []string         `json:"warnings"`
	PostConditions []*PostCondition `json:"post_conditions"`
}

// PostConditionResult represents the result of a post-condition evaluation
type PostConditionResult struct {
	Name        string                 `json:"name"`
	Type        PostConditionType      `json:"type"`
	Satisfied   bool                   `json:"satisfied"`
	Value       interface{}            `json:"value"`
	Expected    interface{}            `json:"expected"`
	Message     string                 `json:"message"`
	Critical    bool                   `json:"critical"`
	Duration    time.Duration          `json:"duration"`
	EvaluatedAt time.Time              `json:"evaluated_at"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// PostConditionValidationResult represents the overall post-condition validation result
type PostConditionValidationResult struct {
	Success        bool                   `json:"success"`
	Results        []*PostConditionResult `json:"results"`
	CriticalFailed int                    `json:"critical_failed"`
	TotalFailed    int                    `json:"total_failed"`
	TotalPassed    int                    `json:"total_passed"`
	TotalDuration  time.Duration          `json:"total_duration"`
	Message        string                 `json:"message"`
}

type RollbackAction struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Parameters  map[string]interface{} `json:"parameters"`
	Description string                 `json:"description"`
}

type ResourceUsageMetrics struct {
	CPUUsage    float64 `json:"cpu_usage"`
	MemoryUsage float64 `json:"memory_usage"`
	DiskUsage   float64 `json:"disk_usage"`
	NetworkIO   float64 `json:"network_io"`
}

type PerformanceMetrics struct {
	ResponseTime float64 `json:"response_time"`
	Throughput   float64 `json:"throughput"`
	ErrorRate    float64 `json:"error_rate"`
	Availability float64 `json:"availability"`
}

type NotificationConfig struct {
	Enabled    bool     `json:"enabled"`
	Channels   []string `json:"channels"`
	Recipients []string `json:"recipients"`
	Template   string   `json:"template"`
}

type RecoveryAction struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Trigger    string                 `json:"trigger"`
	Parameters map[string]interface{} `json:"parameters"`
	Timeout    int                    `json:"timeout"`
}

type ActionCondition struct {
	ID         string                 `json:"id"`
	Expression string                 `json:"expression"`
	Type       string                 `json:"type"`
	Variables  map[string]interface{} `json:"variables"`
}

// Additional missing types for models.go
type AdaptationConstraint struct {
	Type        string                 `json:"type"`
	Value       interface{}            `json:"value"`
	Description string                 `json:"description"`
	Metadata    map[string]interface{} `json:"metadata"`
}

type ValidationResult struct {
	RuleID    string                 `json:"rule_id"`
	Type      ValidationType         `json:"type"`
	Passed    bool                   `json:"passed"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details"`
	Timestamp time.Time              `json:"timestamp"`
}

type ClusterMetrics struct{}

// ValidationType for validation results
type ValidationType string

const (
	ValidationTypeIntegrity ValidationType = "integrity"
	ValidationTypeSyntax    ValidationType = "syntax"
	ValidationTypeSemantic  ValidationType = "semantic"
	ValidationTypeRuntime   ValidationType = "runtime"
)

// Missing types referenced in models.go
type TrendAnalysis struct {
	Direction  string  `json:"direction"`
	Strength   float64 `json:"strength"`
	Confidence float64 `json:"confidence"`
	Slope      float64 `json:"slope"`
}

type PredictedState struct {
	State       string                 `json:"state"`
	Confidence  float64                `json:"confidence"`
	Probability float64                `json:"probability"`
	Metadata    map[string]interface{} `json:"metadata"`
}

type AdaptiveAction struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Parameters map[string]interface{} `json:"parameters"`
	Conditions []string               `json:"conditions"`
	Priority   int                    `json:"priority"`
}

type DecisionAlternative struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Score       float64                `json:"score"`
	Metadata    map[string]interface{} `json:"metadata"`
}

type ImpactAssessment struct {
	Type        string  `json:"type"`
	Severity    string  `json:"severity"`
	Probability float64 `json:"probability"`
	Scope       string  `json:"scope"`
}

type IssueIndicator struct {
	Name      string  `json:"name"`
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
	Status    string  `json:"status"`
}

type PredictionContext struct {
	TimeHorizon time.Duration          `json:"time_horizon"`
	Confidence  float64                `json:"confidence"`
	Scenario    string                 `json:"scenario"`
	Variables   map[string]interface{} `json:"variables"`
}

type ActionSchedule struct {
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	Frequency  string    `json:"frequency"`
	MaxRetries int       `json:"max_retries"`
}

type RiskAssessment struct {
	Level      string   `json:"level"`
	Score      float64  `json:"score"`
	Factors    []string `json:"factors"`
	Mitigation string   `json:"mitigation"`
}

type ImpactAnalysis struct {
	Type       string  `json:"type"`
	Scope      string  `json:"scope"`
	Severity   string  `json:"severity"`
	Likelihood float64 `json:"likelihood"`
}

// Final missing types
type ValidationRule struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Expression string `json:"expression"`
	ErrorMsg   string `json:"error_message"`
	Severity   string `json:"severity"`
}

type ActionRecommendation struct {
	Action     string   `json:"action"`
	Priority   string   `json:"priority"`
	Confidence float64  `json:"confidence"`
	Reasoning  string   `json:"reasoning"`
	Benefits   []string `json:"benefits"`
	Risks      []string `json:"risks"`
}

type ValidationEngine struct {
	Rules   []ValidationRule       `json:"rules"`
	Enabled bool                   `json:"enabled"`
	Config  map[string]interface{} `json:"config"`
}

type SimulationType string

const (
	SimulationBasic    SimulationType = "basic"
	SimulationAdvanced SimulationType = "advanced"
	SimulationStress   SimulationType = "stress"
)

// More missing types
type EnhancedPatternConfig struct {
	MinSupport       float64 `json:"min_support"`
	MinConfidence    float64 `json:"min_confidence"`
	MaxPatterns      int     `json:"max_patterns"`
	EnableMLAnalysis bool    `json:"enable_ml_analysis"`
	TimeWindowHours  int     `json:"time_window_hours"`
}

type SimulatedEnvironment struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Config      map[string]interface{} `json:"config"`
	Resources   map[string]interface{} `json:"resources"`
	Constraints map[string]interface{} `json:"constraints"`
}

type RecoveryPlan struct {
	ID       string                 `json:"id"`
	Actions  []RecoveryAction       `json:"actions"`
	Triggers []string               `json:"triggers"`
	Priority int                    `json:"priority"`
	Timeout  time.Duration          `json:"timeout"`
	Metadata map[string]interface{} `json:"metadata"`
}

// Add missing validation type constants
const (
	ValidationTypeResource    ValidationType = "resource"
	ValidationTypePerformance ValidationType = "performance"
)

// Add missing simulation type constant
const (
	SimulationTypeLoad SimulationType = "load"
)

// Add missing simulation constants and fields
const (
	SimulationTypeFailure     SimulationType = "failure"
	SimulationTypePerformance SimulationType = "performance"
)

// Extend SimulatedEnvironment with missing fields
type ExtendedSimulatedEnvironment struct {
	SimulatedEnvironment
	Metrics       map[string]float64 `json:"metrics"`
	FailureMode   string             `json:"failure_mode"`
	ResourceLimit map[string]float64 `json:"resource_limit"`
}

// Add missing simulation type
const (
	SimulationTypeChaos SimulationType = "chaos"
)

// Risk level constants
type RiskLevel string

const (
	RiskLevelLow    RiskLevel = "low"
	RiskLevelMedium RiskLevel = "medium"
	RiskLevelHigh   RiskLevel = "high"
)

// Workflow recommendation type
type WorkflowRecommendation struct {
	WorkflowID    string                 `json:"workflow_id"`
	Name          string                 `json:"name"`
	Description   string                 `json:"description"`
	Confidence    float64                `json:"confidence"`
	Reason        string                 `json:"reason"`
	Parameters    map[string]interface{} `json:"parameters"`
	Priority      Priority               `json:"priority"`
	Effectiveness float64                `json:"effectiveness"`
	Risk          RiskLevel              `json:"risk"`
}
