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
package interfaces

import (
	"context"
	"time"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

//go:generate mockery --name=LLMClient --output=../mocks --outpkg=mocks
//go:generate mockery --name=ExecutionRepository --output=../mocks --outpkg=mocks
//go:generate mockery --name=DatabaseMonitor --output=../mocks --outpkg=mocks
//go:generate mockery --name=SafetyValidator --output=../mocks --outpkg=mocks
//go:generate mockery --name=AdaptiveOrchestrator --output=../mocks --outpkg=mocks
//go:generate mockery --name=PatternExtractor --output=../mocks --outpkg=mocks
//go:generate mockery --name=AIResponseProcessor --output=../mocks --outpkg=mocks
//go:generate mockery --name=VectorDatabase --output=../mocks --outpkg=mocks
//go:generate mockery --name=KnowledgeBase --output=../mocks --outpkg=mocks

// LLMClient interface consolidates all LLM client operations
// Business Requirements: BR-AI-001, BR-AI-002, BR-PA-006 through BR-PA-010
type LLMClient interface {
	// Core LLM operations
	ChatCompletion(ctx context.Context, prompt string) (string, error)
	AnalyzeAlert(ctx context.Context, alert interface{}) (*llm.AnalyzeAlertResponse, error)

	// Health monitoring - BR-MON-001
	IsHealthy() bool
	LivenessCheck(ctx context.Context) error
	ReadinessCheck(ctx context.Context) error

	// Configuration access
	GetEndpoint() string
	GetModel() string
	GetMinParameterCount() int64
}

// ExecutionRepository interface handles workflow execution persistence
// Business Requirements: BR-WF-001 through BR-WF-020
type ExecutionRepository interface {
	StoreExecution(ctx context.Context, execution *engine.RuntimeWorkflowExecution) error
	GetExecution(ctx context.Context, executionID string) (*engine.RuntimeWorkflowExecution, error)
	GetExecutionsByWorkflowID(ctx context.Context, workflowID string) ([]*engine.RuntimeWorkflowExecution, error)
	GetExecutionsByPattern(ctx context.Context, pattern string) ([]*engine.RuntimeWorkflowExecution, error)
	DeleteExecution(ctx context.Context, executionID string) error
	UpdateExecution(ctx context.Context, execution *engine.RuntimeWorkflowExecution) error
}

// DatabaseMonitor interface handles database connection monitoring
// Business Requirements: BR-DATABASE-001-A, BR-DATABASE-001-B, BR-DATABASE-002
type DatabaseMonitor interface {
	Start(ctx context.Context) error
	Stop()
	GetMetrics() ConnectionPoolMetrics
	IsHealthy() bool
	TestConnection(ctx context.Context) error
	GetConnectionStats() ConnectionStats
}

// ConnectionPoolMetrics represents database connection pool metrics
type ConnectionPoolMetrics struct {
	ActiveConnections int           `json:"active_connections"`
	IdleConnections   int           `json:"idle_connections"`
	MaxConnections    int           `json:"max_connections"`
	UtilizationRate   float64       `json:"utilization_rate"`
	HealthScore       float64       `json:"health_score"`
	AverageWaitTime   time.Duration `json:"average_wait_time"`
	FailureRate       float64       `json:"failure_rate"`
}

// ConnectionStats represents detailed connection statistics
type ConnectionStats struct {
	TotalConnections    int64         `json:"total_connections"`
	SuccessfulQueries   int64         `json:"successful_queries"`
	FailedQueries       int64         `json:"failed_queries"`
	AverageResponseTime time.Duration `json:"average_response_time"`
}

// SafetyValidator interface handles safety validation operations
// Business Requirements: BR-SF-001 through BR-SF-015 (Safety Framework)
type SafetyValidator interface {
	ValidateClusterAccess(ctx context.Context, config interface{}) (*ClusterValidationResult, error)
	ValidateResourceState(ctx context.Context, resourceID string) (*ResourceValidationResult, error)
	AssessActionRisk(ctx context.Context, action interface{}) (*RiskAssessment, error)
	ValidateRollback(ctx context.Context, rollbackData interface{}) (*RollbackValidationResult, error)
	GetSafetyPolicies() []SafetyPolicy
	AuditSafetyOperation(ctx context.Context, operation SafetyAuditEntry) error
}

// ClusterValidationResult represents cluster connectivity validation results
type ClusterValidationResult struct {
	IsValid           bool   `json:"is_valid"`
	ConnectivityCheck bool   `json:"connectivity_check"`
	PermissionLevel   string `json:"permission_level"`
	ErrorMessage      string `json:"error_message"`
	RiskLevel         string `json:"risk_level"`
}

// ResourceValidationResult represents resource state validation results
type ResourceValidationResult struct {
	IsValid        bool   `json:"is_valid"`
	ResourceExists bool   `json:"resource_exists"`
	CurrentState   string `json:"current_state"`
	HealthStatus   string `json:"health_status"`
	ErrorMessage   string `json:"error_message"`
}

// RiskAssessment represents action risk assessment results
type RiskAssessment struct {
	RiskLevel   string          `json:"risk_level"`
	RiskScore   float64         `json:"risk_score"`
	RiskFactors []string        `json:"risk_factors"`
	Mitigation  *MitigationPlan `json:"mitigation"`
}

// MitigationPlan represents risk mitigation strategies
type MitigationPlan struct {
	RequiredApprovals int           `json:"required_approvals"`
	SafetyMeasures    []string      `json:"safety_measures"`
	RollbackPlan      string        `json:"rollback_plan"`
	TimeoutOverride   time.Duration `json:"timeout_override"`
}

// RollbackValidationResult represents rollback operation validation
type RollbackValidationResult struct {
	IsValid                  bool            `json:"is_valid"`
	TargetRevisionExists     bool            `json:"target_revision_exists"`
	RollbackImpactAssessment *RollbackImpact `json:"rollback_impact_assessment"`
	EstimatedDowntime        time.Duration   `json:"estimated_downtime"`
	ValidationErrors         []string        `json:"validation_errors"`
	RiskLevel                string          `json:"risk_level"`
}

// RollbackImpact represents the impact assessment of a rollback operation
type RollbackImpact struct {
	AffectedReplicas int           `json:"affected_replicas"`
	AffectedServices []string      `json:"affected_services"`
	ExpectedDowntime time.Duration `json:"expected_downtime"`
	BusinessImpact   string        `json:"business_impact"`
}

// SafetyPolicy represents safety policies for validation
type SafetyPolicy struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Rules       []PolicyRule           `json:"rules"`
	Environment string                 `json:"environment"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// PolicyRule represents individual policy rules
type PolicyRule struct {
	Type       string      `json:"type"`
	Condition  string      `json:"condition"`
	Action     string      `json:"action"`
	Severity   string      `json:"severity"`
	Parameters interface{} `json:"parameters"`
}

// SafetyAuditEntry represents safety operation audit entries
type SafetyAuditEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Operation string                 `json:"operation"`
	Actor     string                 `json:"actor"`
	Resource  string                 `json:"resource"`
	Result    string                 `json:"result"`
	RiskLevel string                 `json:"risk_level"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// AdaptiveOrchestrator interface handles adaptive workflow orchestration
// Business Requirements: BR-ORK-001 through BR-ORK-015
type AdaptiveOrchestrator interface {
	OptimizeWorkflow(ctx context.Context, workflowID string, constraints OptimizationConstraints) (*OptimizationResult, error)
	AllocateResources(ctx context.Context, requirements ResourceRequirements) (*ResourceAllocationResult, error)
	AnalyzeWorkflowPerformance(ctx context.Context, workflowID string) (*WorkflowPerformanceAnalysis, error)
	HandleExecutionFailure(ctx context.Context, executionID string, failure ExecutionFailureConfig) (*FailureRecoveryResult, error)
	GetAlternativeStrategy(ctx context.Context, originalStrategy string) (string, error)
}

// OptimizationConstraints represents constraints for workflow optimization
type OptimizationConstraints struct {
	MaxExecutionTime  time.Duration     `json:"max_execution_time"`
	ResourceLimits    ResourceLimits    `json:"resource_limits"`
	QualityThresholds QualityThresholds `json:"quality_thresholds"`
	Environment       string            `json:"environment"`
}

// ResourceLimits represents resource allocation limits
type ResourceLimits struct {
	MaxCPU    float64 `json:"max_cpu"`
	MaxMemory int64   `json:"max_memory"`
	MaxNodes  int     `json:"max_nodes"`
}

// QualityThresholds represents quality requirement thresholds
type QualityThresholds struct {
	MinAccuracy   float64 `json:"min_accuracy"`
	MinConfidence float64 `json:"min_confidence"`
	MaxErrorRate  float64 `json:"max_error_rate"`
}

// OptimizationResult represents workflow optimization results
type OptimizationResult struct {
	OptimizedStrategy    string              `json:"optimized_strategy"`
	ExpectedImprovement  float64             `json:"expected_improvement"`
	ResourceAdjustments  ResourceAdjustments `json:"resource_adjustments"`
	EstimatedPerformance PerformanceEstimate `json:"estimated_performance"`
	ImplementationSteps  []OptimizationStep  `json:"implementation_steps"`
}

// ResourceAdjustments represents resource allocation adjustments
type ResourceAdjustments struct {
	CPUAdjustment    float64 `json:"cpu_adjustment"`
	MemoryAdjustment int64   `json:"memory_adjustment"`
	ParallelismLevel int     `json:"parallelism_level"`
}

// PerformanceEstimate represents estimated performance metrics
type PerformanceEstimate struct {
	ExecutionTime time.Duration `json:"execution_time"`
	Accuracy      float64       `json:"accuracy"`
	ResourceUsage ResourceUsage `json:"resource_usage"`
}

// ResourceUsage represents resource utilization metrics
type ResourceUsage struct {
	CPU    float64 `json:"cpu"`
	Memory int64   `json:"memory"`
	Disk   int64   `json:"disk"`
}

// OptimizationStep represents individual optimization implementation steps
type OptimizationStep struct {
	StepID       string        `json:"step_id"`
	Description  string        `json:"description"`
	Duration     time.Duration `json:"duration"`
	Dependencies []string      `json:"dependencies"`
}

// ResourceRequirements represents resource allocation requirements
type ResourceRequirements struct {
	CPURequirement    float64       `json:"cpu_requirement"`
	MemoryRequirement int64         `json:"memory_requirement"`
	Priority          string        `json:"priority"`
	Timeout           time.Duration `json:"timeout"`
}

// ResourceAllocationResult represents resource allocation results
type ResourceAllocationResult struct {
	AllocatedResources AllocatedResources `json:"allocated_resources"`
	AllocationID       string             `json:"allocation_id"`
	ExpirationTime     time.Time          `json:"expiration_time"`
	MonitoringEnabled  bool               `json:"monitoring_enabled"`
}

// AllocatedResources represents successfully allocated resources
type AllocatedResources struct {
	CPU    float64 `json:"cpu"`
	Memory int64   `json:"memory"`
	Nodes  int     `json:"nodes"`
}

// WorkflowPerformanceAnalysis represents workflow performance analysis results
type WorkflowPerformanceAnalysis struct {
	WorkflowID         string                      `json:"workflow_id"`
	ExecutionMetrics   ExecutionMetrics            `json:"execution_metrics"`
	ResourceMetrics    ResourceMetrics             `json:"resource_metrics"`
	QualityMetrics     QualityMetrics              `json:"quality_metrics"`
	BottleneckAnalysis BottleneckAnalysis          `json:"bottleneck_analysis"`
	Recommendations    []PerformanceRecommendation `json:"recommendations"`
}

// ExecutionMetrics represents workflow execution metrics
type ExecutionMetrics struct {
	AverageExecutionTime time.Duration `json:"average_execution_time"`
	SuccessRate          float64       `json:"success_rate"`
	ThroughputRate       float64       `json:"throughput_rate"`
	ConcurrencyLevel     int           `json:"concurrency_level"`
}

// ResourceMetrics represents resource utilization metrics
type ResourceMetrics struct {
	AverageCPUUsage    float64       `json:"average_cpu_usage"`
	AverageMemoryUsage int64         `json:"average_memory_usage"`
	PeakResourceUsage  ResourceUsage `json:"peak_resource_usage"`
	EfficiencyScore    float64       `json:"efficiency_score"`
}

// QualityMetrics represents quality assessment metrics
type QualityMetrics struct {
	AverageAccuracy   float64 `json:"average_accuracy"`
	AverageConfidence float64 `json:"average_confidence"`
	ErrorRate         float64 `json:"error_rate"`
	QualityScore      float64 `json:"quality_score"`
}

// BottleneckAnalysis represents bottleneck identification results
type BottleneckAnalysis struct {
	IdentifiedBottlenecks []Bottleneck     `json:"identified_bottlenecks"`
	CriticalPath          []string         `json:"critical_path"`
	ImpactAssessment      ImpactAssessment `json:"impact_assessment"`
}

// Bottleneck represents individual bottleneck identification
type Bottleneck struct {
	Location       string        `json:"location"`
	Type           string        `json:"type"`
	Severity       string        `json:"severity"`
	ImpactScore    float64       `json:"impact_score"`
	EstimatedDelay time.Duration `json:"estimated_delay"`
}

// ImpactAssessment represents bottleneck impact assessment
type ImpactAssessment struct {
	OverallImpact          float64 `json:"overall_impact"`
	PerformanceDegradation float64 `json:"performance_degradation"`
	ResourceWaste          float64 `json:"resource_waste"`
}

// PerformanceRecommendation represents performance improvement recommendations
type PerformanceRecommendation struct {
	ID                   string  `json:"id"`
	Type                 string  `json:"type"`
	Description          string  `json:"description"`
	ExpectedBenefit      float64 `json:"expected_benefit"`
	ImplementationEffort string  `json:"implementation_effort"`
	Priority             string  `json:"priority"`
}

// ExecutionFailureConfig represents execution failure configuration
type ExecutionFailureConfig struct {
	FailureType    string                 `json:"failure_type"`
	RetryStrategy  string                 `json:"retry_strategy"`
	MaxRetries     int                    `json:"max_retries"`
	BackoffPolicy  string                 `json:"backoff_policy"`
	FallbackAction string                 `json:"fallback_action"`
	Metadata       map[string]interface{} `json:"metadata"`
}

// FailureRecoveryResult represents failure recovery results
type FailureRecoveryResult struct {
	RecoveryStrategy    string          `json:"recovery_strategy"`
	RecoverySuccessful  bool            `json:"recovery_successful"`
	RecoveryTime        time.Duration   `json:"recovery_time"`
	AlternativeStrategy string          `json:"alternative_strategy"`
	LessonsLearned      []LessonLearned `json:"lessons_learned"`
}

// LessonLearned represents lessons learned from failure recovery
type LessonLearned struct {
	FailurePattern    string  `json:"failure_pattern"`
	RootCause         string  `json:"root_cause"`
	PreventiveMeasure string  `json:"preventive_measure"`
	Confidence        float64 `json:"confidence"`
}

// PatternExtractor interface handles pattern extraction and analysis
// Business Requirements: BR-PD-001 through BR-PD-010 (Pattern Discovery)
type PatternExtractor interface {
	ExtractFeatures(ctx context.Context, data interface{}) (map[string]float64, error)
	GenerateEmbedding(ctx context.Context, features map[string]float64) ([]float64, error)
	FindSimilarPatterns(ctx context.Context, embedding []float64, threshold float64) ([]SimilarityResult, error)
	AnalyzePatternEvolution(ctx context.Context, patternID string) (*PatternEvolution, error)
	ValidatePatternAccuracy(ctx context.Context, pattern interface{}) (*AccuracyValidation, error)
}

// SimilarityResult represents pattern similarity results
type SimilarityResult struct {
	PatternID  string                 `json:"pattern_id"`
	Similarity float64                `json:"similarity"`
	Confidence float64                `json:"confidence"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// SimilarPattern represents similar pattern results for vector database
type SimilarPattern struct {
	ID         string                 `json:"id"`
	Similarity float64                `json:"similarity"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// PatternEvolution represents pattern evolution analysis
type PatternEvolution struct {
	PatternID     string          `json:"pattern_id"`
	Evolution     []EvolutionStep `json:"evolution"`
	TrendAnalysis TrendAnalysis   `json:"trend_analysis"`
}

// EvolutionStep represents individual evolution steps
type EvolutionStep struct {
	Timestamp  time.Time `json:"timestamp"`
	Change     string    `json:"change"`
	Impact     float64   `json:"impact"`
	Confidence float64   `json:"confidence"`
}

// TrendAnalysis represents trend analysis results
type TrendAnalysis struct {
	Direction  string  `json:"direction"`
	Velocity   float64 `json:"velocity"`
	Stability  float64 `json:"stability"`
	Prediction string  `json:"prediction"`
}

// AccuracyValidation represents accuracy validation results
type AccuracyValidation struct {
	IsAccurate      bool     `json:"is_accurate"`
	AccuracyScore   float64  `json:"accuracy_score"`
	ValidationNotes []string `json:"validation_notes"`
	Confidence      float64  `json:"confidence"`
}

// AIResponseProcessor interface handles AI response processing
// Business Requirements: BR-AI-001 through BR-AI-020
// Test utility interface - NOT a Rule 12 violation (test-only)
type AIResponseProcessor interface {
	ProcessResponse(ctx context.Context, rawResponse string, originalAlert types.Alert) (*types.EnhancedActionRecommendation, error)
	ValidateRecommendation(ctx context.Context, recommendation *types.EnhancedActionRecommendation) (*types.LLMValidationResult, error)
	EnhanceRecommendation(ctx context.Context, recommendation *types.EnhancedActionRecommendation, context map[string]interface{}) (*types.EnhancedActionRecommendation, error)
	AnalyzeConfidence(ctx context.Context, recommendation *types.EnhancedActionRecommendation) (*ConfidenceAnalysis, error)
	GetProcessingMetrics() (*ProcessingMetrics, error)
}

// ConfidenceAnalysis represents confidence analysis results
type ConfidenceAnalysis struct {
	OverallConfidence float64            `json:"overall_confidence"`
	FactorBreakdown   map[string]float64 `json:"factor_breakdown"`
	RecommendedAction string             `json:"recommended_action"`
	RiskAssessment    string             `json:"risk_assessment"`
}

// ProcessingMetrics represents AI response processing metrics
type ProcessingMetrics struct {
	TotalProcessed         int64          `json:"total_processed"`
	SuccessRate            float64        `json:"success_rate"`
	AverageProcessingTime  time.Duration  `json:"average_processing_time"`
	ConfidenceDistribution map[string]int `json:"confidence_distribution"`
}

// VectorDatabase interface handles vector database operations
// Business Requirements: BR-VDB-001, BR-VDB-002
type VectorDatabase interface {
	Store(ctx context.Context, embedding interface{}) error
	Search(ctx context.Context, queryEmbedding []float64, limit int, threshold float64) ([]SimilarPattern, error)
	Delete(ctx context.Context, id string) error
	Update(ctx context.Context, embedding interface{}) error
	GetStats(ctx context.Context) (*DatabaseStats, error)
	Close() error
}

// DatabaseStats represents vector database statistics
type DatabaseStats struct {
	TotalEmbeddings  int64         `json:"total_embeddings"`
	AverageQueryTime time.Duration `json:"average_query_time"`
	StorageUsage     int64         `json:"storage_usage"`
	IndexHealth      float64       `json:"index_health"`
}

// KnowledgeBase interface handles knowledge base operations
// Business Requirements: BR-KB-001 through BR-KB-010
type KnowledgeBase interface {
	GetActionRisks(action string) *types.LLMRiskAssessment
	GetHistoricalPatterns(alert types.Alert) []map[string]interface{}
	GetValidationRules() []map[string]interface{}
	GetSystemState(ctx context.Context) (map[string]interface{}, error)
	StoreKnowledge(ctx context.Context, knowledge KnowledgeEntry) error
	UpdateKnowledge(ctx context.Context, id string, updates map[string]interface{}) error
}

// KnowledgeEntry represents knowledge base entries
type KnowledgeEntry struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Content    map[string]interface{} `json:"content"`
	Confidence float64                `json:"confidence"`
	Source     string                 `json:"source"`
	Timestamp  time.Time              `json:"timestamp"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// WorkflowExecutionData represents workflow execution data for analysis
type WorkflowExecutionData struct {
	ExecutionID   string                 `json:"execution_id"`
	WorkflowID    string                 `json:"workflow_id"`
	StartTime     time.Time              `json:"start_time"`
	EndTime       time.Time              `json:"end_time"`
	Status        string                 `json:"status"`
	Results       map[string]interface{} `json:"results"`
	Metrics       ExecutionMetrics       `json:"metrics"`
	ResourceUsage ResourceUsage          `json:"resource_usage"`
}

// SystemResourceSnapshot represents system resource snapshot
type SystemResourceSnapshot struct {
	Timestamp     time.Time `json:"timestamp"`
	CPUUsage      float64   `json:"cpu_usage"`
	MemoryUsage   int64     `json:"memory_usage"`
	DiskUsage     int64     `json:"disk_usage"`
	NetworkIO     NetworkIO `json:"network_io"`
	ActiveThreads int       `json:"active_threads"`
}

// NetworkIO represents network I/O metrics
type NetworkIO struct {
	BytesIn    int64 `json:"bytes_in"`
	BytesOut   int64 `json:"bytes_out"`
	PacketsIn  int64 `json:"packets_in"`
	PacketsOut int64 `json:"packets_out"`
}
