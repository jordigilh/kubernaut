package engine

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
)

// Core Workflow Types

// Workflow represents a complex multi-step automation process
type Workflow struct {
	types.BaseVersionedEntity // Embedded: ID, Name, Description, Version, CreatedAt, UpdatedAt, Metadata, CreatedBy

	// Workflow-specific fields
	Template *ExecutableTemplate `json:"template"`
	Status   WorkflowStatus      `json:"status"`
}

// ExecutableTemplate defines the structure and logic of an executable workflow
type ExecutableTemplate struct {
	types.BaseVersionedEntity // Embedded: ID, Name, Description, Version, CreatedAt, UpdatedAt, Metadata, CreatedBy

	// Template-specific fields
	Steps      []*ExecutableWorkflowStep `json:"steps"`
	Conditions []*ExecutableCondition    `json:"conditions"`
	Variables  map[string]interface{}    `json:"variables"`
	Timeouts   *WorkflowTimeouts         `json:"timeouts"`
	Recovery   *RecoveryPolicy           `json:"recovery"`
	Tags       []string                  `json:"tags"`
}

// ExecutableWorkflowStep represents a single step in a workflow with execution capabilities
type ExecutableWorkflowStep struct {
	types.BaseEntity // Embedded: ID, Name, Description, CreatedAt, UpdatedAt, Metadata

	// Step-specific fields
	Type         StepType               `json:"type"`
	Action       *StepAction            `json:"action,omitempty"`
	Condition    *ExecutableCondition   `json:"condition,omitempty"`
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

// ExecutableCondition defines conditional logic in workflows with full execution context
type ExecutableCondition struct {
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

// RuntimeWorkflowExecution represents an active execution of a workflow with full operational state
// It embeds WorkflowExecutionRecord for shared analytics fields and extends with operational fields
type RuntimeWorkflowExecution struct {
	types.WorkflowExecutionRecord // Embedded shared analytics fields (ID, WorkflowID, Status, StartTime, EndTime, Metadata)

	// Operational-specific fields (Status is overridden with enum type)
	OperationalStatus ExecutionStatus   `json:"status"` // Override embedded Status with enum type for operations
	Input             *WorkflowInput    `json:"input"`
	Output            *WorkflowOutput   `json:"output,omitempty"`
	Context           *ExecutionContext `json:"context"`
	Steps             []*StepExecution  `json:"steps"`
	CurrentStep       int               `json:"current_step"`
	Duration          time.Duration     `json:"duration"`
	Error             string            `json:"error,omitempty"`
}

// IsSuccessful returns true if the workflow execution completed successfully
func (rwe *RuntimeWorkflowExecution) IsSuccessful() bool {
	return rwe.OperationalStatus == ExecutionStatusCompleted
}

// IsCompleted returns true if the workflow execution has finished (either successfully or with failure)
func (rwe *RuntimeWorkflowExecution) IsCompleted() bool {
	return rwe.OperationalStatus == ExecutionStatusCompleted ||
		rwe.OperationalStatus == ExecutionStatusFailed ||
		rwe.OperationalStatus == ExecutionStatusCancelled
}

// IsFailed returns true if the workflow execution failed
func (rwe *RuntimeWorkflowExecution) IsFailed() bool {
	return rwe.OperationalStatus == ExecutionStatusFailed
}

// IsRunning returns true if the workflow execution is currently running
func (rwe *RuntimeWorkflowExecution) IsRunning() bool {
	return rwe.OperationalStatus == ExecutionStatusRunning
}

// IsPending returns true if the workflow execution is pending
func (rwe *RuntimeWorkflowExecution) IsPending() bool {
	return rwe.OperationalStatus == ExecutionStatusPending
}

// GetSuccessRate returns the success rate of completed steps
func (rwe *RuntimeWorkflowExecution) GetSuccessRate() float64 {
	if len(rwe.Steps) == 0 {
		return 0.0
	}

	successfulSteps := 0
	for _, step := range rwe.Steps {
		if step.Status == ExecutionStatusCompleted {
			successfulSteps++
		}
	}

	return float64(successfulSteps) / float64(len(rwe.Steps))
}

// GetCompletionStatus returns detailed completion statistics
func (rwe *RuntimeWorkflowExecution) GetCompletionStatus() ExecutionCompletionStatus {
	totalSteps := len(rwe.Steps)
	completedSteps := 0
	failedSteps := 0
	pendingSteps := 0
	runningSteps := 0

	for _, step := range rwe.Steps {
		switch step.Status {
		case ExecutionStatusCompleted:
			completedSteps++
		case ExecutionStatusFailed:
			failedSteps++
		case ExecutionStatusPending:
			pendingSteps++
		case ExecutionStatusRunning:
			runningSteps++
		}
	}

	return ExecutionCompletionStatus{
		TotalSteps:     totalSteps,
		CompletedSteps: completedSteps,
		FailedSteps:    failedSteps,
		PendingSteps:   pendingSteps,
		RunningSteps:   runningSteps,
		SuccessRate:    rwe.GetSuccessRate(),
		IsFinished:     rwe.IsCompleted(),
		IsSuccessful:   rwe.IsSuccessful(),
	}
}

// ExecutionCompletionStatus provides detailed status information about an execution
type ExecutionCompletionStatus struct {
	TotalSteps     int     `json:"total_steps"`
	CompletedSteps int     `json:"completed_steps"`
	FailedSteps    int     `json:"failed_steps"`
	PendingSteps   int     `json:"pending_steps"`
	RunningSteps   int     `json:"running_steps"`
	SuccessRate    float64 `json:"success_rate"`
	IsFinished     bool    `json:"is_finished"`
	IsSuccessful   bool    `json:"is_successful"`
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
	Condition *ExecutableCondition   `json:"condition"`
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
	Conditions []*ExecutableCondition `json:"conditions"`
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
	ID                     string                        `json:"id"`
	WorkflowID             string                        `json:"workflow_id"`
	Type                   OptimizationType              `json:"type"`
	Changes                []*OptimizationChange         `json:"changes"`
	Performance            *PerformanceImprovement       `json:"performance"`
	Confidence             float64                       `json:"confidence"`
	ValidationResult       *WorkflowRuleValidationResult `json:"validation_result"`
	AppliedAt              *time.Time                    `json:"applied_at,omitempty"`
	CreatedAt              time.Time                     `json:"created_at"`
	OptimizationCandidates interface{}                   `json:"optimization_candidates,omitempty"` // Added for BR-ORK-001
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

// OptimizationCandidate represents a potential optimization
// Business Requirements: BR-ORK-001 - Generate 3-5 viable optimization candidates
type OptimizationCandidate struct {
	ID                     string                 `json:"id"`
	Type                   string                 `json:"type"`
	Target                 string                 `json:"target"`
	Description            string                 `json:"description"`
	Impact                 float64                `json:"impact"`                   // Expected performance impact (0.0 to 1.0)
	Confidence             float64                `json:"confidence"`               // Confidence in prediction (0.0 to 1.0)
	PredictedTimeReduction float64                `json:"predicted_time_reduction"` // Expected time reduction percentage
	ROIScore               float64                `json:"roi_score"`                // Return on investment score
	CostReduction          float64                `json:"cost_reduction"`           // Expected cost reduction
	ImplementationEffort   time.Duration          `json:"implementation_effort"`    // Expected implementation time
	Priority               int                    `json:"priority"`                 // Priority ranking
	ApplicableSteps        []string               `json:"applicable_steps"`         // Steps this optimization applies to
	Parameters             map[string]interface{} `json:"parameters"`               // Additional optimization parameters
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
	Workflows     []*ExecutableTemplate   `json:"workflows"`
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
	ID           string                    `json:"id"`
	Type         DecisionType              `json:"type"`
	Action       *AdaptiveAction           `json:"action,omitempty"`
	Workflow     *RuntimeWorkflowExecution `json:"workflow,omitempty"`
	Confidence   float64                   `json:"confidence"`
	Reasoning    string                    `json:"reasoning"`
	Alternatives []*DecisionAlternative    `json:"alternatives"`
	Timestamp    time.Time                 `json:"timestamp"`
	Metadata     map[string]interface{}    `json:"metadata"`
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
type WorkflowTimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Context Types

// AlertContext provides context about an alert
type AlertContext struct {
	types.BaseContext // Embedded: Labels, Annotations, Metadata, Environment, Cluster, Timestamp

	// Alert-specific fields
	Name         string     `json:"name"`
	Severity     Severity   `json:"severity"`
	StartsAt     time.Time  `json:"starts_at"`
	EndsAt       *time.Time `json:"ends_at,omitempty"`
	GeneratorURL string     `json:"generator_url,omitempty"`
	Fingerprint  string     `json:"fingerprint"`
}

// ResourceContext provides context about a Kubernetes resource
type ResourceContext struct {
	types.BaseContext      // Embedded: Labels, Annotations, Metadata, Environment, Cluster, Timestamp
	types.BaseResourceInfo // Embedded: Namespace, Kind, Name

	// Resource-specific fields
	Status map[string]interface{} `json:"status"`
	Spec   map[string]interface{} `json:"spec"`
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
	types.BaseContext // Embedded: Labels, Annotations, Metadata, Environment, Cluster, Timestamp

	// Execution-specific fields
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

// NewStepContext creates a new step context with initialized variables
func NewStepContext() *StepContext {
	return &StepContext{
		Variables: make(map[string]interface{}),
	}
}

// Set sets a variable in the step context
func (sc *StepContext) Set(key string, value interface{}) {
	if sc.Variables == nil {
		sc.Variables = make(map[string]interface{})
	}
	sc.Variables[key] = value
}

// Get gets a variable from the step context
func (sc *StepContext) Get(key string) (interface{}, bool) {
	if sc.Variables == nil {
		return nil, false
	}
	value, exists := sc.Variables[key]
	return value, exists
}

// GetString gets a string variable from the step context
func (sc *StepContext) GetString(key, defaultValue string) string {
	if value, exists := sc.Get(key); exists {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return defaultValue
}

// GetInt gets an integer variable from the step context
func (sc *StepContext) GetInt(key string, defaultValue int) int {
	if value, exists := sc.Get(key); exists {
		switch v := value.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		case string:
			if i, err := strconv.Atoi(v); err == nil {
				return i
			}
		}
	}
	return defaultValue
}

// GetBool gets a boolean variable from the step context
func (sc *StepContext) GetBool(key string, defaultValue bool) bool {
	if value, exists := sc.Get(key); exists {
		if b, ok := value.(bool); ok {
			return b
		}
	}
	return defaultValue
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
	Type       RecoveryType           `json:"type"`
	Conditions []*ExecutableCondition `json:"conditions"`
	Actions    []*RecoveryAction      `json:"actions"`
	Priority   Priority               `json:"priority"`
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
func (w *RuntimeWorkflowExecution) MarshalJSON() ([]byte, error) {
	type Alias RuntimeWorkflowExecution
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

// OptimizationCandidate is already defined above at line 399 - removing duplicate

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
	types.BaseContext // Embedded: Labels, Annotations, Metadata, Environment, Cluster, Timestamp

	// Workflow-specific fields
	WorkflowID string                    `json:"workflow_id"`
	Execution  *RuntimeWorkflowExecution `json:"execution"`
	Namespace  string                    `json:"namespace"`
	Alert      *AlertContext             `json:"alert,omitempty"`
	Resource   *ResourceContext          `json:"resource,omitempty"`
	Variables  map[string]interface{}    `json:"variables,omitempty"`
	History    []*ActionResult           `json:"history,omitempty"`
	Metrics    map[string]float64        `json:"metrics,omitempty"`
	CreatedAt  time.Time                 `json:"created_at"`
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
	ID             string                    `json:"id"`
	Name           string                    `json:"name"`
	Type           string                    `json:"type"`
	Steps          []*ExecutableWorkflowStep `json:"steps"`
	Conditions     []*ActionCondition        `json:"conditions"`
	SuccessRate    float64                   `json:"success_rate"`
	ExecutionCount int                       `json:"execution_count"`
	AverageTime    time.Duration             `json:"average_time"`
	Environments   []string                  `json:"environments"`
	ResourceTypes  []string                  `json:"resource_types"`
	Confidence     float64                   `json:"confidence"`
	LastUsed       time.Time                 `json:"last_used"`
	CreatedAt      time.Time                 `json:"created_at"`
	UpdatedAt      time.Time                 `json:"updated_at"`
}

// Advanced Pattern Testing Types (TDD Implementation)
// Following project guideline #5: Define business contracts to enable test compilation
// Note: ObjectiveAnalysisResult already exists in intelligent_workflow_builder_helpers.go

type ResourceConstraints struct {
	MaxCPUUtilization    float64       `json:"max_cpu_utilization"`
	MaxMemoryUtilization float64       `json:"max_memory_utilization"`
	MaxConcurrentSteps   int           `json:"max_concurrent_steps"`
	TimeoutBuffer        time.Duration `json:"timeout_buffer"`
}

type ResourcePlan struct {
	TotalCPUWeight             float64    `json:"total_cpu_weight"`
	TotalMemoryWeight          float64    `json:"total_memory_weight"`
	MaxConcurrency             int        `json:"max_concurrency"`
	EstimatedCPUUtilization    float64    `json:"estimated_cpu_utilization"`
	EstimatedMemoryUtilization float64    `json:"estimated_memory_utilization"`
	EfficiencyScore            float64    `json:"efficiency_score"`
	OptimalBatches             [][]string `json:"optimal_batches"`
}

type ParallelizationStrategy struct {
	ParallelGroups          [][]string `json:"parallel_groups"`
	EstimatedSpeedup        float64    `json:"estimated_speedup"`
	HasCircularDependencies bool       `json:"has_circular_dependencies"`
	ConflictResolution      string     `json:"conflict_resolution"`
}

type LoopExecutionMetrics struct {
	TotalIterations      int           `json:"total_iterations"`
	SuccessfulIterations int           `json:"successful_iterations"`
	FailedIterations     int           `json:"failed_iterations"`
	AverageIterationTime time.Duration `json:"average_iteration_time"`
	TotalExecutionTime   time.Duration `json:"total_execution_time"`
}

type LoopTerminationResult struct {
	ShouldContinue bool   `json:"should_continue"`
	NextIteration  int    `json:"next_iteration"`
	Reason         string `json:"reason"`
}

type ComplexLoopEvaluation struct {
	BreakConditionMet    bool   `json:"break_condition_met"`
	ContinueConditionMet bool   `json:"continue_condition_met"`
	ConditionEvaluation  string `json:"condition_evaluation"`
}

type LoopPerformanceOptimization struct {
	SuccessRate     float64  `json:"success_rate"`
	EfficiencyScore float64  `json:"efficiency_score"`
	Recommendations []string `json:"recommendations"`
}

type WorkflowComplexity struct {
	OverallScore float64            `json:"overall_score"`
	FactorScores map[string]float64 `json:"factor_scores"`
}

type WorkflowRiskAssessment struct {
	RiskLevel string  `json:"risk_level"`
	RiskScore float64 `json:"risk_score"`
}

type WorkflowExecution struct {
	WorkflowID  string                 `json:"workflow_id"`
	Status      ExecutionStatus        `json:"status"`
	Duration    time.Duration          `json:"duration"`
	StepResults map[string]*StepResult `json:"step_results"`
	StartTime   time.Time              `json:"start_time"`
	EndTime     time.Time              `json:"end_time"`
}

// Note: StepResult already defined in models.go line 287

type ExecutionPattern struct {
	PatternID       string                 `json:"pattern_id"`
	SuccessRate     float64                `json:"success_rate"`
	AverageDuration time.Duration          `json:"average_duration"`
	CommonFailures  []string               `json:"common_failures"`
	ContextFactors  map[string]interface{} `json:"context_factors"`
}

type OptimizationConstraints struct {
	MaxRiskLevel       string        `json:"max_risk_level"`
	MinSuccessRate     float64       `json:"min_success_rate"`
	MaxPerformanceGain float64       `json:"max_performance_gain"`
	PreferReliability  bool          `json:"prefer_reliability"`
	MaxExecutionTime   time.Duration `json:"max_execution_time"`
	MinPerformanceGain float64       `json:"min_performance_gain"`
	RequiredConfidence float64       `json:"required_confidence"`
}

type WorkflowMetrics struct {
	AverageExecutionTime time.Duration `json:"average_execution_time"`
	SuccessRate          float64       `json:"success_rate"`
	ResourceUtilization  float64       `json:"resource_utilization"`
	FailureRate          float64       `json:"failure_rate"`
	ErrorRate            float64       `json:"error_rate"`
}

// OrchestrationEfficiency represents orchestration efficiency metrics
type OrchestrationEfficiency struct {
	OverallEfficiency     float64                `json:"overall_efficiency"`
	ParallelizationRatio  float64                `json:"parallelization_ratio"`
	ResourceUtilization   float64                `json:"resource_utilization"`
	StepDependencyMetrics map[string]interface{} `json:"step_dependency_metrics"`
	OptimizationPotential float64                `json:"optimization_potential"`
}

// OptimizationImpact represents the impact of orchestration optimization
type OptimizationImpact struct {
	ExecutionTimeImprovement float64 `json:"execution_time_improvement"`
	ResourceEfficiencyGain   float64 `json:"resource_efficiency_gain"`
	StepReduction            float64 `json:"step_reduction"`
	OverallImpact            float64 `json:"overall_impact"`
	TimeImprovement          float64 `json:"time_improvement"`        // For production optimization engine compatibility
	ReliabilityImprovement   float64 `json:"reliability_improvement"` // For production optimization engine compatibility
	OverallScore             float64 `json:"overall_score"`           // For production optimization engine compatibility
	ROIAchieved              float64 `json:"roi_achieved"`            // For production optimization engine compatibility
}

type PerformanceThresholds struct {
	MaxExecutionTime time.Duration `json:"max_execution_time"`
	MinSuccessRate   float64       `json:"min_success_rate"`
	MaxResourceUsage float64       `json:"max_resource_usage"`
	MaxErrorRate     float64       `json:"max_error_rate"`
}

type PerformanceAlert struct {
	Severity string `json:"severity"`
	Metric   string `json:"metric"`
	Message  string `json:"message"`
}

type AIOptimizationResult struct {
	OptimizationScore    float64            `json:"optimization_score"`
	Recommendations      []string           `json:"recommendations"`
	EstimatedImprovement map[string]float64 `json:"estimated_improvement"`
}

type LearningResult struct {
	PatternConfidence float64  `json:"pattern_confidence"`
	LearningImpact    string   `json:"learning_impact"`
	UpdatedRules      []string `json:"updated_rules"`
}

type SuccessPrediction struct {
	SuccessProbability float64  `json:"success_probability"`
	RiskFactors        []string `json:"risk_factors"`
	ConfidenceLevel    string   `json:"confidence_level"`
}

type ExecutionOptimization struct {
	EstimatedImprovement float64  `json:"estimated_improvement"`
	OptimizedSteps       []string `json:"optimized_steps"`
	Techniques           []string `json:"techniques"`
}

type SafetyConstraints struct {
	MaxConcurrentOperations int           `json:"max_concurrent_operations"`
	MaxWorkflowDuration     time.Duration `json:"max_workflow_duration"`
	AllowedEnvironments     []string      `json:"allowed_environments"`
	RequiredApprovals       []string      `json:"required_approvals"`
	RequireApproval         bool          `json:"require_approval"`          // For security enhancement compatibility
	AllowDestructiveActions bool          `json:"allow_destructive_actions"` // For security enhancement compatibility
}

type SafetyCheck struct {
	IsSafe      bool     `json:"is_safe"`
	RiskFactors []string `json:"risk_factors"`
	SafetyScore float64  `json:"safety_score"`
}

// SecurityReport represents a comprehensive security analysis report
type SecurityReport struct {
	WorkflowID         string                 `json:"workflow_id"`
	SecurityScore      float64                `json:"security_score"`
	VulnerabilityCount int                    `json:"vulnerability_count"`
	ComplianceStatus   string                 `json:"compliance_status"`
	SecurityFindings   []SecurityFinding      `json:"security_findings"`
	RecommendedActions []string               `json:"recommended_actions"`
	GeneratedAt        time.Time              `json:"generated_at"`
	SecurityMetadata   map[string]interface{} `json:"security_metadata"`
}

// SecurityFinding represents a specific security finding
type SecurityFinding struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Severity    string                 `json:"severity"`
	Description string                 `json:"description"`
	StepID      string                 `json:"step_id,omitempty"`
	Remediation string                 `json:"remediation"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// AdvancedInsights represents comprehensive workflow insights
type AdvancedInsights struct {
	WorkflowID  string                 `json:"workflow_id"`
	InsightType string                 `json:"insight_type"`
	Confidence  float64                `json:"confidence"`
	Insights    []WorkflowInsight      `json:"insights"`
	GeneratedAt time.Time              `json:"generated_at"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// WorkflowInsight represents a specific workflow insight
type WorkflowInsight struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Category    string                 `json:"category"`
	Description string                 `json:"description"`
	Impact      string                 `json:"impact"`
	Confidence  float64                `json:"confidence"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// PredictiveMetrics represents predictive analytics for workflows
type PredictiveMetrics struct {
	WorkflowID             string        `json:"workflow_id"`
	PredictedExecutionTime time.Duration `json:"predicted_execution_time"`
	PredictedSuccessRate   float64       `json:"predicted_success_rate"`
	PredictedResourceUsage float64       `json:"predicted_resource_usage"`
	ConfidenceLevel        float64       `json:"confidence_level"`
	TrendAnalysis          []string      `json:"trend_analysis"`
	PredictionHorizon      time.Duration `json:"prediction_horizon"`
	GeneratedAt            time.Time     `json:"generated_at"`
	PredictiveFactors      []string      `json:"predictive_factors"`
	RiskAssessment         string        `json:"risk_assessment"`
}

// AIRecommendations represents AI-generated workflow recommendations
type AIRecommendations struct {
	WorkflowID         string                 `json:"workflow_id"`
	RecommendationType string                 `json:"recommendation_type"`
	Confidence         float64                `json:"confidence"`
	Recommendations    []AIRecommendation     `json:"recommendations"`
	GeneratedAt        time.Time              `json:"generated_at"`
	ModelVersion       string                 `json:"model_version"`
	Metadata           map[string]interface{} `json:"metadata"`
}

// AIRecommendation represents a specific AI recommendation
type AIRecommendation struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Priority    string                 `json:"priority"`
	Description string                 `json:"description"`
	Impact      string                 `json:"impact"`
	Confidence  float64                `json:"confidence"`
	Parameters  map[string]interface{} `json:"parameters"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// AIOptimizationParams represents parameters for AI optimization
type AIOptimizationParams struct {
	OptimizationType string                 `json:"optimization_type"`
	TargetMetrics    []string               `json:"target_metrics"`
	Confidence       float64                `json:"confidence"`
	ModelVersion     string                 `json:"model_version"`
	LearningData     map[string]interface{} `json:"learning_data"`
	Constraints      map[string]interface{} `json:"constraints"`
	Preferences      map[string]interface{} `json:"preferences"`
}

// MachineLearningContext represents context for machine learning enhancement
type MachineLearningContext struct {
	ModelType       string                 `json:"model_type"`
	TrainingData    []string               `json:"training_data"`
	FeatureSet      []string               `json:"feature_set"`
	LearningRate    float64                `json:"learning_rate"`
	Epochs          int                    `json:"epochs"`
	ValidationSplit float64                `json:"validation_split"`
	ModelAccuracy   float64                `json:"model_accuracy"`
	Hyperparameters map[string]interface{} `json:"hyperparameters"`
	Metadata        map[string]interface{} `json:"metadata"`
}

type SafetyEnforcement struct {
	ConstraintsViolated   []string `json:"constraints_violated"`
	RequiredModifications []string `json:"required_modifications"`
	CanProceed            bool     `json:"can_proceed"`
}

type ConstrainedOptimizationResult struct {
	RiskLevel       string  `json:"risk_level"`
	PerformanceGain float64 `json:"performance_gain"`
}

// Note: OptimizationImpact already defined above with all required fields

// Note: ExecutionMetrics and TrendAnalysis already exist - using existing types

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
	Success  bool          `json:"success"`
	Duration time.Duration `json:"duration"`

	// Simulation-specific fields
	ID         string                 `json:"id"`
	ScenarioID string                 `json:"scenario_id"`
	Results    map[string]interface{} `json:"results"`
	Metrics    map[string]float64     `json:"metrics"`
	Logs       []string               `json:"logs,omitempty"`
	Errors     []string               `json:"errors,omitempty"`
	RunAt      time.Time              `json:"run_at"`
}

type ValidationReport struct {
	ID          string                          `json:"id"`
	WorkflowID  string                          `json:"workflow_id"`
	ExecutionID string                          `json:"execution_id"`
	Type        ValidationType                  `json:"type"`
	Status      string                          `json:"status"`
	Results     []*WorkflowRuleValidationResult `json:"results"`
	Summary     *ValidationSummary              `json:"summary"`
	CreatedAt   time.Time                       `json:"created_at"`
	CompletedAt *time.Time                      `json:"completed_at,omitempty"`
}

// WorkflowRuleValidationResult is defined above

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

type WorkflowRuleValidationResult struct {
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
	ValidationTypeSecurity  ValidationType = "security" // For security enhancement validation
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

// Workflow validation types needed for testing
type WorkflowValidationResult struct {
	Valid            bool                   `json:"valid"`
	ValidationChecks map[string]interface{} `json:"validation_checks"`
	Warnings         []string               `json:"warnings"`
	SafetyScore      float64                `json:"safety_score"`
	CorrectnessScore float64                `json:"correctness_score"`
	SecurityScore    float64                `json:"security_score"`
	OverallScore     float64                `json:"overall_score"`
}

// ExecutionOutcome represents the outcome of a workflow execution for learning
type ExecutionOutcome struct {
	WorkflowID        string                 `json:"workflow_id"`
	Success           bool                   `json:"success"`
	Duration          time.Duration          `json:"duration"`
	EffectivenesScore float64                `json:"effectiveness_score"`
	Feedback          map[string]interface{} `json:"feedback"`
}

// Note: LearningResult already defined above for TDD implementation at line 1141

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

// Missing types for workflow builder
type EffectivenessReport struct {
	ID          string             `json:"id"`
	ExecutionID string             `json:"execution_id"`
	Score       float64            `json:"score"`
	Metrics     map[string]float64 `json:"metrics"`
	Insights    []string           `json:"insights"`
	CreatedAt   time.Time          `json:"created_at"`
}

type PatternInsights struct {
	PatternID     string                 `json:"pattern_id"`
	Effectiveness float64                `json:"effectiveness"`
	UsageCount    int                    `json:"usage_count"`
	Insights      []string               `json:"insights"`
	Metrics       map[string]interface{} `json:"metrics"`
}

type TemplateFactory struct {
	templates map[string]*ExecutableTemplate
}

type PatternMatcher struct {
}

// Note: AnalyticsEngine interface moved to pkg/shared/types/analytics.go for consolidation

type WorkflowValidator interface {
	ValidateWorkflow(ctx context.Context, template *ExecutableTemplate) *ValidationReport
}

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

// BR-ORK-002 Type definitions for adaptive step execution

// ContextAnalysis represents analysis of execution context
type ContextAnalysis struct {
	SystemLoad             map[string]interface{} `json:"system_load"`
	HistoricalPerformance  map[string]interface{} `json:"historical_performance"`
	ClusterHealth          map[string]interface{} `json:"cluster_health"`
	RecommendedAdaptations []string               `json:"recommended_adaptations"`
	AnalyzedAt             time.Time              `json:"analyzed_at"`
}

// ExecutionStrategy represents a strategy for step execution
type ExecutionStrategy struct {
	Name       string                 `json:"name"`
	Parameters map[string]interface{} `json:"parameters"`
	Confidence float64                `json:"confidence"`
	CreatedAt  time.Time              `json:"created_at"`
}

// WorkflowStep represents a workflow step (updated definition)
type WorkflowStep struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Name       string                 `json:"name"`
	Action     string                 `json:"action"`
	Parameters map[string]interface{} `json:"parameters"`
	Timeout    time.Duration          `json:"timeout"`
	Conditions []*ExecutableCondition `json:"conditions,omitempty"`
	DependsOn  []string               `json:"depends_on,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
}

// BR-ORK-003 Type definitions for statistics tracking and analysis

// WorkflowExecutionMetrics represents comprehensive execution metrics
type WorkflowExecutionMetrics struct {
	WorkflowID    string                `json:"workflow_id"`
	ExecutionID   string                `json:"execution_id"`
	Duration      time.Duration         `json:"duration"`
	SuccessRate   float64               `json:"success_rate"`
	StepCount     int                   `json:"step_count"`
	SuccessCount  int                   `json:"success_count"`
	FailureCount  int                   `json:"failure_count"`
	ResourceUsage *ResourceUsageMetrics `json:"resource_usage"`
	Timestamp     time.Time             `json:"timestamp"`
}

// SystemResourceImpact represents system resource impact during orchestration
type SystemResourceImpact struct {
	ExecutionID  string        `json:"execution_id"`
	CPUDelta     float64       `json:"cpu_delta"`
	MemoryDelta  float64       `json:"memory_delta"`
	NetworkDelta float64       `json:"network_delta"`
	DiskDelta    float64       `json:"disk_delta"`
	PeakCPU      float64       `json:"peak_cpu"`
	PeakMemory   float64       `json:"peak_memory"`
	Duration     time.Duration `json:"duration"`
	Timestamp    time.Time     `json:"timestamp"`
}

// Business Requirements: BR-ORCH-002 - Adaptive Resource Allocation Types

// ResourceAllocationResult represents the result of resource allocation optimization
type ResourceAllocationResult struct {
	OptimizationApplied     bool    `json:"optimization_applied"`
	EstimatedEfficiencyGain float64 `json:"estimated_efficiency_gain"`
	AllocatedCPU            float64 `json:"allocated_cpu"`
	AllocatedMemory         float64 `json:"allocated_memory"`
	OptimizationDetails     string  `json:"optimization_details"`
	Confidence              float64 `json:"confidence"`
}

// ClusterCapacity represents the available capacity of a cluster
type ClusterCapacity struct {
	Level           ClusterCapacityLevel `json:"level"`
	AvailableCPU    float64              `json:"available_cpu"`
	AvailableMemory float64              `json:"available_memory"`
	NodeCount       int                  `json:"node_count"`
	Utilization     float64              `json:"utilization"`
}

// ClusterCapacityLevel represents different levels of cluster capacity
type ClusterCapacityLevel string

const (
	ClusterCapacityHigh        ClusterCapacityLevel = "high"
	ClusterCapacityMedium      ClusterCapacityLevel = "medium"
	ClusterCapacityLow         ClusterCapacityLevel = "low"
	ClusterCapacityConstrained ClusterCapacityLevel = "constrained"
)

// ResourcePrediction represents predicted resource requirements
type ResourcePrediction struct {
	PredictedCPU       float64 `json:"predicted_cpu"`
	PredictedMemory    float64 `json:"predicted_memory"`
	ConfidenceLevel    float64 `json:"confidence_level"`
	PredictionAccuracy float64 `json:"prediction_accuracy"`
	BasedOnExecutions  int     `json:"based_on_executions"`
	TimeWindow         string  `json:"time_window"`
}

// AdaptiveResourceMetrics represents resource usage metrics for adaptive allocation
type AdaptiveResourceMetrics struct {
	CPUUtilization    float64 `json:"cpu_utilization"`
	MemoryUtilization float64 `json:"memory_utilization"`
	EstimatedCost     float64 `json:"estimated_cost"`
	Efficiency        float64 `json:"efficiency"`
}

// Business Requirements: BR-ORCH-003 - Execution Scheduling Types

// SchedulingResult represents the result of execution scheduling optimization
type SchedulingResult struct {
	OptimizationApplied     bool                          `json:"optimization_applied"`
	EstimatedThroughputGain float64                       `json:"estimated_throughput_gain"`
	ScheduledWorkflows      []*ScheduledWorkflowExecution `json:"scheduled_workflows"`
	TotalSchedulingTime     time.Duration                 `json:"total_scheduling_time"`
	OptimizationDetails     string                        `json:"optimization_details"`
	Confidence              float64                       `json:"confidence"`
}

// ScheduledWorkflowExecution represents a workflow scheduled for execution
type ScheduledWorkflowExecution struct {
	WorkflowID              string              `json:"workflow_id"`
	ScheduledStartTime      time.Time           `json:"scheduled_start_time"`
	ScheduledExecutionTime  time.Duration       `json:"scheduled_execution_time"`
	Priority                int                 `json:"priority"`
	AllocatedResources      *ResourceAllocation `json:"allocated_resources"`
	Dependencies            []string            `json:"dependencies"`
	EstimatedCompletionTime time.Time           `json:"estimated_completion_time"`
}

// ResourceAllocation represents allocated resources for a scheduled workflow
type ResourceAllocation struct {
	CPU     float64 `json:"cpu"`
	Memory  float64 `json:"memory"`
	Disk    float64 `json:"disk"`
	Network float64 `json:"network"`
}

// SystemLoad represents current system load characteristics
type SystemLoad struct {
	Level       SystemLoadLevel `json:"level"`
	CPULoad     float64         `json:"cpu_load"`
	MemoryLoad  float64         `json:"memory_load"`
	DiskLoad    float64         `json:"disk_load"`
	NetworkLoad float64         `json:"network_load"`
	ActiveTasks int             `json:"active_tasks"`
	QueueLength int             `json:"queue_length"`
}

// SystemLoadLevel represents different levels of system load
type SystemLoadLevel string

const (
	SystemLoadVeryLow  SystemLoadLevel = "very_low"
	SystemLoadLow      SystemLoadLevel = "low"
	SystemLoadMedium   SystemLoadLevel = "medium"
	SystemLoadHigh     SystemLoadLevel = "high"
	SystemLoadCritical SystemLoadLevel = "critical"
)

// SchedulingPrediction represents predicted scheduling optimization
type SchedulingPrediction struct {
	PredictedThroughput float64       `json:"predicted_throughput"`
	PredictedWaitTime   time.Duration `json:"predicted_wait_time"`
	ConfidenceLevel     float64       `json:"confidence_level"`
	AccuracyScore       float64       `json:"accuracy_score"`
	BasedOnExecutions   int           `json:"based_on_executions"`
	TimeWindow          string        `json:"time_window"`
}

// PrioritySchedulingResult represents the result of priority-based scheduling
type PrioritySchedulingResult struct {
	ScheduledWorkflows  []*ScheduledWorkflowExecution `json:"scheduled_workflows"`
	PriorityOrdering    []string                      `json:"priority_ordering"`
	TotalSchedulingTime time.Duration                 `json:"total_scheduling_time"`
	OptimizationApplied bool                          `json:"optimization_applied"`
	BusinessSLAMet      bool                          `json:"business_sla_met"`
}

// SchedulingPerformanceMetrics represents scheduling performance measurements
type SchedulingPerformanceMetrics struct {
	ThroughputWPS   float64       `json:"throughput_wps"` // Workflows per second
	AverageWaitTime time.Duration `json:"average_wait_time"`
	QueueLength     int           `json:"queue_length"`
	UtilizationRate float64       `json:"utilization_rate"`
	SuccessRate     float64       `json:"success_rate"`
}

// Business Requirements: BR-ORCH-001 - Feedback Loop Types

// ExecutionFeedback represents feedback from workflow execution
type ExecutionFeedback struct {
	ExecutionID      string                 `json:"execution_id"`
	WorkflowID       string                 `json:"workflow_id"`
	FeedbackType     FeedbackType           `json:"feedback_type"`
	AccuracyScore    float64                `json:"accuracy_score"`
	PerformanceScore float64                `json:"performance_score"`
	QualityScore     float64                `json:"quality_score"`
	UserSatisfaction float64                `json:"user_satisfaction"`
	Timestamp        time.Time              `json:"timestamp"`
	Context          string                 `json:"context"`
	Metadata         map[string]interface{} `json:"metadata"`
}

// FeedbackType represents different types of feedback
type FeedbackType string

const (
	FeedbackTypePositive    FeedbackType = "positive"
	FeedbackTypeNegative    FeedbackType = "negative"
	FeedbackTypePerformance FeedbackType = "performance"
	FeedbackTypeQuality     FeedbackType = "quality"
	FeedbackTypeUser        FeedbackType = "user"
	FeedbackTypeSystem      FeedbackType = "system"
)

// FeedbackLoopResult represents the result of feedback loop processing
type FeedbackLoopResult struct {
	FeedbackProcessed        bool          `json:"feedback_processed"`
	OptimizationImprovements int           `json:"optimization_improvements"`
	AccuracyImprovement      float64       `json:"accuracy_improvement"`
	PerformanceImprovement   float64       `json:"performance_improvement"`
	LearningRate             float64       `json:"learning_rate"`
	ProcessingTime           time.Duration `json:"processing_time"`
	ConfidenceLevel          float64       `json:"confidence_level"`
}

// PerformanceFeedback represents performance-specific feedback data
type PerformanceFeedback struct {
	FeedbackType          FeedbackType  `json:"feedback_type"`
	SuccessRate           float64       `json:"success_rate"`
	SampleCount           int           `json:"sample_count"`
	AverageResponseTime   time.Duration `json:"average_response_time"`
	ErrorRate             float64       `json:"error_rate"`
	ThroughputImprovement float64       `json:"throughput_improvement"`
	ResourceEfficiency    float64       `json:"resource_efficiency"`
}

// StrategyAdaptationResult represents the result of optimization strategy adaptation
type StrategyAdaptationResult struct {
	StrategyAdjustment      float64                `json:"strategy_adjustment"`
	LearningRate            float64                `json:"learning_rate"`
	CorrectiveActions       int                    `json:"corrective_actions"`
	AdaptationEffectiveness float64                `json:"adaptation_effectiveness"`
	NewStrategyParameters   map[string]interface{} `json:"new_strategy_parameters"`
	ConfidenceLevel         float64                `json:"confidence_level"`
}

// FeedbackConvergenceResult represents the result of feedback convergence cycle processing
type FeedbackConvergenceResult struct {
	ConvergenceAchieved   bool    `json:"convergence_achieved"`
	StabilityScore        float64 `json:"stability_score"`
	ConvergenceRate       float64 `json:"convergence_rate"`
	CycleNumber           int     `json:"cycle_number"`
	OptimizationVariance  float64 `json:"optimization_variance"`
	LearningStabilization float64 `json:"learning_stabilization"`
}

// RealTimeFeedbackAnalysis represents the result of real-time feedback analysis
type RealTimeFeedbackAnalysis struct {
	InsightsGenerated    int                    `json:"insights_generated"`
	AnalysisAccuracy     float64                `json:"analysis_accuracy"`
	ResponseTime         float64                `json:"response_time"` // seconds
	Insights             []*ActionableInsight   `json:"insights"`
	TrendAnalysis        *FeedbackTrendAnalysis `json:"trend_analysis"`
	PredictiveIndicators map[string]float64     `json:"predictive_indicators"`
}

// ActionableInsight represents an actionable insight from feedback analysis
type ActionableInsight struct {
	InsightID                string  `json:"insight_id"`
	ActionableRecommendation string  `json:"actionable_recommendation"`
	ConfidenceScore          float64 `json:"confidence_score"`
	ExpectedImpact           float64 `json:"expected_impact"`
	Priority                 int     `json:"priority"`
	Category                 string  `json:"category"`
	ImplementationComplexity string  `json:"implementation_complexity"`
}

// FeedbackTrendAnalysis represents trend analysis from feedback data
type FeedbackTrendAnalysis struct {
	PerformanceTrend    string  `json:"performance_trend"` // improving, declining, stable
	AccuracyTrend       string  `json:"accuracy_trend"`
	QualityTrend        string  `json:"quality_trend"`
	TrendConfidence     float64 `json:"trend_confidence"`
	PredictedDirection  string  `json:"predicted_direction"`
	TrendStabilityScore float64 `json:"trend_stability_score"`
}

// ConflictResolutionResult represents the result of conflicting feedback resolution
type ConflictResolutionResult struct {
	ResolutionStrategy      string   `json:"resolution_strategy"`
	ConfidenceLevel         float64  `json:"confidence_level"`
	ConflictSeverity        float64  `json:"conflict_severity"`
	ResolutionEffectiveness float64  `json:"resolution_effectiveness"`
	RecommendedAction       string   `json:"recommended_action"`
	AlternativeStrategies   []string `json:"alternative_strategies"`
}

// HighVolumeFeedbackResult represents the result of high-volume feedback processing
type HighVolumeFeedbackResult struct {
	ProcessingThroughput      float64       `json:"processing_throughput"` // items per second
	AccuracyDegradation       float64       `json:"accuracy_degradation"`
	ResourceUtilization       float64       `json:"resource_utilization"`
	ProcessingLatency         time.Duration `json:"processing_latency"`
	BatchProcessingEfficiency float64       `json:"batch_processing_efficiency"`
	MemoryUsagePeak           float64       `json:"memory_usage_peak"`
}

// OptimizationAccuracyMetrics represents optimization accuracy measurement
type OptimizationAccuracyMetrics struct {
	AccuracyScore   float64 `json:"accuracy_score"`
	ConfidenceLevel float64 `json:"confidence_level"`
	SampleCount     int     `json:"sample_count"`
	PrecisionScore  float64 `json:"precision_score"`
	RecallScore     float64 `json:"recall_score"`
	F1Score         float64 `json:"f1_score"`
}
