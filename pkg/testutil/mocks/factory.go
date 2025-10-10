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
package mocks

import (
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/mock"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/testutil/interfaces"
)

// MockFactory provides centralized creation of generated mocks with consistent patterns
// Business Requirements: Support testing for all BR-XXX-### requirements through standardized mocks
// Following project guidelines: REUSE existing code and AVOID duplication
type MockFactory struct {
	config *FactoryConfig
	logger *logrus.Logger
}

// FactoryConfig holds configuration for mock factory behavior
type FactoryConfig struct {
	EnableDetailedLogging bool                   `yaml:"enable_detailed_logging"`
	DefaultResponses      map[string]interface{} `yaml:"default_responses"`
	ErrorSimulation       bool                   `yaml:"error_simulation"`
	BusinessThresholds    *BusinessThresholds    `yaml:"business_thresholds"`
}

// BusinessThresholds represents business requirement thresholds for testing
// Business Requirements: Configuration-driven validation per BR-XXX-### requirements
type BusinessThresholds struct {
	Database    DatabaseThresholds    `yaml:"database"`
	Performance PerformanceThresholds `yaml:"performance"`
	Monitoring  MonitoringThresholds  `yaml:"monitoring"`
	AI          AIThresholds          `yaml:"ai"`
}

// DatabaseThresholds represents database-related business requirement thresholds
type DatabaseThresholds struct {
	BRDatabase001A DatabaseUtilizationThresholds `yaml:"BR-DATABASE-001-A"`
	BRDatabase001B DatabasePerformanceThresholds `yaml:"BR-DATABASE-001-B"`
	BRDatabase002  DatabaseRecoveryThresholds    `yaml:"BR-DATABASE-002"`
}

// DatabaseUtilizationThresholds for BR-DATABASE-001-A
type DatabaseUtilizationThresholds struct {
	UtilizationThreshold float64 `yaml:"utilization_threshold"`
	MaxOpenConnections   int     `yaml:"max_open_connections"`
	MaxIdleConnections   int     `yaml:"max_idle_connections"`
}

// DatabasePerformanceThresholds for BR-DATABASE-001-B
type DatabasePerformanceThresholds struct {
	HealthScoreThreshold float64       `yaml:"health_score_threshold"`
	HealthyScore         float64       `yaml:"healthy_score"`
	DegradedScore        float64       `yaml:"degraded_score"`
	FailureRateThreshold float64       `yaml:"failure_rate_threshold"`
	WaitTimeThreshold    time.Duration `yaml:"wait_time_threshold"`
}

// DatabaseRecoveryThresholds for BR-DATABASE-002
type DatabaseRecoveryThresholds struct {
	ExhaustionRecoveryTime  time.Duration `yaml:"exhaustion_recovery_time"`
	RecoveryHealthThreshold float64       `yaml:"recovery_health_threshold"`
}

// PerformanceThresholds represents performance-related business requirement thresholds
type PerformanceThresholds struct {
	BRPERF001 PerformanceRequirements `yaml:"BR-PERF-001"`
}

// PerformanceRequirements for BR-PERF-001
type PerformanceRequirements struct {
	MaxResponseTime     time.Duration `yaml:"max_response_time"`
	MinThroughput       int           `yaml:"min_throughput"`
	LatencyPercentile95 time.Duration `yaml:"latency_percentile_95"`
	LatencyPercentile99 time.Duration `yaml:"latency_percentile_99"`
}

// MonitoringThresholds represents monitoring-related business requirement thresholds
type MonitoringThresholds struct {
	BRMON001 MonitoringRequirements `yaml:"BR-MON-001"`
}

// MonitoringRequirements for BR-MON-001
type MonitoringRequirements struct {
	AlertThreshold            float64       `yaml:"alert_threshold"`
	MetricsCollectionInterval time.Duration `yaml:"metrics_collection_interval"`
	HealthCheckInterval       time.Duration `yaml:"health_check_interval"`
}

// AIThresholds represents AI-related business requirement thresholds
type AIThresholds struct {
	BRAI001 AIAnalysisRequirements       `yaml:"BR-AI-001"`
	BRAI002 AIRecommendationRequirements `yaml:"BR-AI-002"`
}

// AIAnalysisRequirements for BR-AI-001
type AIAnalysisRequirements struct {
	MinConfidenceScore     float64       `yaml:"min_confidence_score"`
	MaxAnalysisTime        time.Duration `yaml:"max_analysis_time"`
	WorkflowGenerationTime time.Duration `yaml:"workflow_generation_time"`
}

// AIRecommendationRequirements for BR-AI-002
type AIRecommendationRequirements struct {
	RecommendationConfidence float64       `yaml:"recommendation_confidence"`
	ActionValidationTime     time.Duration `yaml:"action_validation_time"`
}

// NewMockFactory creates a new mock factory with the specified configuration
// Following project guidelines: provide clear, realistic configuration options
func NewMockFactory(config *FactoryConfig) *MockFactory {
	if config == nil {
		config = &FactoryConfig{
			EnableDetailedLogging: false,
			DefaultResponses:      make(map[string]interface{}),
			ErrorSimulation:       false,
			BusinessThresholds:    getDefaultBusinessThresholds(),
		}
	}

	logger := logrus.New()
	if config.EnableDetailedLogging {
		logger.SetLevel(logrus.DebugLevel)
	} else {
		logger.SetLevel(logrus.ErrorLevel)
	}

	return &MockFactory{
		config: config,
		logger: logger,
	}
}

// CreateLLMClient creates a configured LLM client mock
// Business Requirements: BR-AI-001, BR-AI-002, BR-PA-006 through BR-PA-010
func (f *MockFactory) CreateLLMClient(responses []string) *LLMClient {
	mockClient := &LLMClient{}

	// Set up standardized health monitoring behavior - BR-MON-001
	mockClient.On("IsHealthy").Return(true)
	mockClient.On("LivenessCheck", mock.Anything).Return(nil)
	mockClient.On("ReadinessCheck", mock.Anything).Return(nil)

	// Set up configuration access
	mockClient.On("GetEndpoint").Return("mock://llm-client")
	mockClient.On("GetModel").Return("mock-model-7b")
	mockClient.On("GetMinParameterCount").Return(int64(7000000000)) // 7B parameters

	// Set up chat completion responses
	for i, response := range responses {
		if i == 0 {
			mockClient.On("ChatCompletion", mock.Anything, mock.Anything).Return(response, nil).Once()
		} else {
			mockClient.On("ChatCompletion", mock.Anything, mock.Anything).Return(response, nil).Once()
		}
	}

	// Set up analyze alert response with business requirement validation
	analyzeResponse := &llm.AnalyzeAlertResponse{
		Action:     "restart_pod",
		Confidence: f.config.BusinessThresholds.AI.BRAI001.MinConfidenceScore + 0.1, // Above threshold
		Parameters: map[string]interface{}{
			"namespace": "default",
			"pod_name":  "test-pod",
		},
		Metadata: map[string]interface{}{
			"reasoning":     "Mock analysis based on business requirements",
			"br_compliance": "BR-AI-001",
		},
	}
	mockClient.On("AnalyzeAlert", mock.Anything, mock.Anything).Return(analyzeResponse, nil)

	if f.config.EnableDetailedLogging {
		f.logger.Debug("Created LLM client mock with business requirement compliance")
	}

	return mockClient
}

// CreateExecutionRepository creates a configured execution repository mock
// Business Requirements: BR-WF-001 through BR-WF-020
func (f *MockFactory) CreateExecutionRepository() *ExecutionRepository {
	mockRepo := &ExecutionRepository{}

	// Set up standard storage operations
	mockRepo.On("StoreExecution", mock.Anything, mock.Anything).Return(nil)
	mockRepo.On("UpdateExecution", mock.Anything, mock.Anything).Return(nil)
	mockRepo.On("DeleteExecution", mock.Anything, mock.Anything).Return(nil)

	// Set up retrieval operations with business-relevant data
	// Following project guidelines: use structured field values from embedded types
	startTime := time.Now().Add(-10 * time.Minute)
	endTime := time.Now().Add(-5 * time.Minute)

	sampleExecution := &types.RuntimeWorkflowExecution{
		WorkflowExecutionRecord: types.WorkflowExecutionRecord{
			ID:         "test-execution-001",
			WorkflowID: "test-workflow-001",
			Status:     "completed",
			StartTime:  startTime,
			EndTime:    &endTime,
			Metadata: map[string]interface{}{
				"confidence":    f.config.BusinessThresholds.AI.BRAI002.RecommendationConfidence + 0.05,
				"br_validation": "BR-WF-001",
			},
		},
	}

	mockRepo.On("GetExecution", mock.Anything, "test-execution-001").Return(sampleExecution, nil)
	mockRepo.On("GetExecutionsByWorkflowID", mock.Anything, "test-workflow-001").Return([]*types.RuntimeWorkflowExecution{sampleExecution}, nil)
	mockRepo.On("GetExecutionsByPattern", mock.Anything, mock.Anything).Return([]*types.RuntimeWorkflowExecution{sampleExecution}, nil)

	if f.config.EnableDetailedLogging {
		f.logger.Debug("Created execution repository mock with workflow compliance")
	}

	return mockRepo
}

// CreateDatabaseMonitor creates a configured database monitor mock
// Business Requirements: BR-DATABASE-001-A, BR-DATABASE-001-B, BR-DATABASE-002
func (f *MockFactory) CreateDatabaseMonitor() *DatabaseMonitor {
	mockMonitor := &DatabaseMonitor{}

	// Set up lifecycle operations
	mockMonitor.On("Start", mock.Anything).Return(nil)
	mockMonitor.On("Stop").Return()
	mockMonitor.On("IsHealthy").Return(true)
	mockMonitor.On("TestConnection", mock.Anything).Return(nil)

	// Set up metrics that meet business requirements
	thresholds := f.config.BusinessThresholds.Database

	metrics := interfaces.ConnectionPoolMetrics{
		ActiveConnections: 5,
		IdleConnections:   3,
		MaxConnections:    thresholds.BRDatabase001A.MaxOpenConnections,
		UtilizationRate:   thresholds.BRDatabase001A.UtilizationThreshold + 0.05,            // Above threshold
		HealthScore:       thresholds.BRDatabase001B.HealthyScore,                           // Healthy score
		AverageWaitTime:   thresholds.BRDatabase001B.WaitTimeThreshold - time.Millisecond*5, // Below threshold
		FailureRate:       thresholds.BRDatabase001B.FailureRateThreshold - 0.02,            // Below threshold
	}

	mockMonitor.On("GetMetrics").Return(metrics)

	connectionStats := interfaces.ConnectionStats{
		TotalConnections:    100,
		SuccessfulQueries:   95,
		FailedQueries:       5,
		AverageResponseTime: time.Millisecond * 25, // Good performance
	}

	mockMonitor.On("GetConnectionStats").Return(connectionStats)

	if f.config.EnableDetailedLogging {
		f.logger.Debug("Created database monitor mock with BR-DATABASE compliance")
	}

	return mockMonitor
}

// CreateSafetyValidator creates a configured safety validator mock
// Business Requirements: BR-SF-001 through BR-SF-015
func (f *MockFactory) CreateSafetyValidator() *SafetyValidator {
	mockValidator := &SafetyValidator{}

	// Set up cluster validation with safe defaults
	clusterResult := &interfaces.ClusterValidationResult{
		IsValid:           true,
		ConnectivityCheck: true,
		PermissionLevel:   "read-write",
		ErrorMessage:      "",
		RiskLevel:         "low",
	}
	mockValidator.On("ValidateClusterAccess", mock.Anything, mock.Anything).Return(clusterResult, nil)

	// Set up resource validation
	resourceResult := &interfaces.ResourceValidationResult{
		IsValid:        true,
		ResourceExists: true,
		CurrentState:   "healthy",
		HealthStatus:   "running",
		ErrorMessage:   "",
	}
	mockValidator.On("ValidateResourceState", mock.Anything, mock.Anything).Return(resourceResult, nil)

	// Set up risk assessment with low-risk defaults
	riskAssessment := &interfaces.RiskAssessment{
		RiskLevel:   "low",
		RiskScore:   0.2, // Low risk score
		RiskFactors: []string{"automated_action", "reversible"},
		Mitigation: &interfaces.MitigationPlan{
			RequiredApprovals: 0,
			SafetyMeasures:    []string{"rollback_available", "monitoring_enabled"},
			RollbackPlan:      "automatic_rollback",
			TimeoutOverride:   time.Minute * 5,
		},
	}
	mockValidator.On("AssessActionRisk", mock.Anything, mock.Anything).Return(riskAssessment, nil)

	// Set up rollback validation
	rollbackResult := &interfaces.RollbackValidationResult{
		IsValid:              true,
		TargetRevisionExists: true,
		RollbackImpactAssessment: &interfaces.RollbackImpact{
			AffectedReplicas: 1,
			AffectedServices: []string{"test-service"},
			ExpectedDowntime: time.Second * 30,
			BusinessImpact:   "minimal",
		},
		EstimatedDowntime: time.Second * 30,
		ValidationErrors:  []string{},
		RiskLevel:         "low",
	}
	mockValidator.On("ValidateRollback", mock.Anything, mock.Anything).Return(rollbackResult, nil)

	// Set up safety policies
	safetyPolicies := []interfaces.SafetyPolicy{
		{
			ID:          "policy-001",
			Name:        "Test Safety Policy",
			Environment: "test",
			Rules: []interfaces.PolicyRule{
				{
					Type:      "action_validation",
					Condition: "risk_level <= low",
					Action:    "approve",
					Severity:  "info",
				},
			},
		},
	}
	mockValidator.On("GetSafetyPolicies").Return(safetyPolicies)

	// Set up audit operations
	mockValidator.On("AuditSafetyOperation", mock.Anything, mock.Anything).Return(nil)

	if f.config.EnableDetailedLogging {
		f.logger.Debug("Created safety validator mock with BR-SF compliance")
	}

	return mockValidator
}

// CreateAdaptiveOrchestrator creates a configured adaptive orchestrator mock
// Business Requirements: BR-ORK-001 through BR-ORK-015
func (f *MockFactory) CreateAdaptiveOrchestrator() *AdaptiveOrchestrator {
	mockOrchestrator := &AdaptiveOrchestrator{}

	// Set up optimization results meeting performance requirements
	optimizationResult := &interfaces.OptimizationResult{
		OptimizedStrategy:   "parallel_execution",
		ExpectedImprovement: 0.25, // 25% improvement
		ResourceAdjustments: interfaces.ResourceAdjustments{
			CPUAdjustment:    1.5,
			MemoryAdjustment: 512 * 1024 * 1024, // 512MB
			ParallelismLevel: 3,
		},
		EstimatedPerformance: interfaces.PerformanceEstimate{
			ExecutionTime: f.config.BusinessThresholds.Performance.BRPERF001.MaxResponseTime - time.Millisecond*200,
			Accuracy:      0.92, // High accuracy
			ResourceUsage: interfaces.ResourceUsage{
				CPU:    0.5,
				Memory: 256 * 1024 * 1024, // 256MB
				Disk:   100 * 1024 * 1024, // 100MB
			},
		},
		ImplementationSteps: []interfaces.OptimizationStep{
			{
				StepID:       "step-001",
				Description:  "Enable parallel processing",
				Duration:     time.Second * 30,
				Dependencies: []string{},
			},
		},
	}

	mockOrchestrator.On("OptimizeWorkflow", mock.Anything, mock.Anything, mock.Anything).Return(optimizationResult, nil)

	// Set up resource allocation
	allocationResult := &interfaces.ResourceAllocationResult{
		AllocatedResources: interfaces.AllocatedResources{
			CPU:    2.0,
			Memory: 1024 * 1024 * 1024, // 1GB
			Nodes:  2,
		},
		AllocationID:      "alloc-001",
		ExpirationTime:    time.Now().Add(time.Hour),
		MonitoringEnabled: true,
	}

	mockOrchestrator.On("AllocateResources", mock.Anything, mock.Anything).Return(allocationResult, nil)

	// Set up performance analysis
	performanceAnalysis := &interfaces.WorkflowPerformanceAnalysis{
		WorkflowID: "test-workflow-001",
		ExecutionMetrics: interfaces.ExecutionMetrics{
			AverageExecutionTime: time.Second * 45,
			SuccessRate:          0.95,
			ThroughputRate:       100.0, // requests per second
			ConcurrencyLevel:     5,
		},
		ResourceMetrics: interfaces.ResourceMetrics{
			AverageCPUUsage:    0.6,
			AverageMemoryUsage: 512 * 1024 * 1024,
			PeakResourceUsage: interfaces.ResourceUsage{
				CPU:    0.8,
				Memory: 768 * 1024 * 1024,
				Disk:   200 * 1024 * 1024,
			},
			EfficiencyScore: 0.85,
		},
		QualityMetrics: interfaces.QualityMetrics{
			AverageAccuracy:   0.90,
			AverageConfidence: f.config.BusinessThresholds.AI.BRAI002.RecommendationConfidence + 0.1,
			ErrorRate:         0.05,
			QualityScore:      0.88,
		},
		BottleneckAnalysis: interfaces.BottleneckAnalysis{
			IdentifiedBottlenecks: []interfaces.Bottleneck{},
			CriticalPath:          []string{"step1", "step2", "step3"},
			ImpactAssessment: interfaces.ImpactAssessment{
				OverallImpact:          0.1, // Low impact
				PerformanceDegradation: 0.05,
				ResourceWaste:          0.02,
			},
		},
		Recommendations: []interfaces.PerformanceRecommendation{
			{
				ID:                   "rec-001",
				Type:                 "optimization",
				Description:          "Increase parallelism for CPU-bound tasks",
				ExpectedBenefit:      0.15,
				ImplementationEffort: "low",
				Priority:             "medium",
			},
		},
	}

	mockOrchestrator.On("AnalyzeWorkflowPerformance", mock.Anything, mock.Anything).Return(performanceAnalysis, nil)

	// Set up failure handling
	failureRecovery := &interfaces.FailureRecoveryResult{
		RecoveryStrategy:    "retry_with_backoff",
		RecoverySuccessful:  true,
		RecoveryTime:        time.Second * 15,
		AlternativeStrategy: "fallback_strategy",
		LessonsLearned: []interfaces.LessonLearned{
			{
				FailurePattern:    "timeout",
				RootCause:         "resource_contention",
				PreventiveMeasure: "increase_timeout",
				Confidence:        0.8,
			},
		},
	}

	mockOrchestrator.On("HandleExecutionFailure", mock.Anything, mock.Anything, mock.Anything).Return(failureRecovery, nil)
	mockOrchestrator.On("GetAlternativeStrategy", mock.Anything, mock.Anything).Return("alternative_parallel_strategy", nil)

	if f.config.EnableDetailedLogging {
		f.logger.Debug("Created adaptive orchestrator mock with BR-ORK compliance")
	}

	return mockOrchestrator
}

// getDefaultBusinessThresholds provides default business requirement thresholds
// Following project guidelines: provide realistic defaults that align with business requirements
func getDefaultBusinessThresholds() *BusinessThresholds {
	return &BusinessThresholds{
		Database: DatabaseThresholds{
			BRDatabase001A: DatabaseUtilizationThresholds{
				UtilizationThreshold: 0.8,
				MaxOpenConnections:   10,
				MaxIdleConnections:   5,
			},
			BRDatabase001B: DatabasePerformanceThresholds{
				HealthScoreThreshold: 0.7,
				HealthyScore:         1.0,
				DegradedScore:        0.85,
				FailureRateThreshold: 0.1,
				WaitTimeThreshold:    time.Millisecond * 50,
			},
			BRDatabase002: DatabaseRecoveryThresholds{
				ExhaustionRecoveryTime:  time.Millisecond * 200,
				RecoveryHealthThreshold: 0.7,
			},
		},
		Performance: PerformanceThresholds{
			BRPERF001: PerformanceRequirements{
				MaxResponseTime:     time.Second * 2,
				MinThroughput:       1000,
				LatencyPercentile95: time.Millisecond * 1500,
				LatencyPercentile99: time.Millisecond * 1800,
			},
		},
		Monitoring: MonitoringThresholds{
			BRMON001: MonitoringRequirements{
				AlertThreshold:            0.95,
				MetricsCollectionInterval: time.Millisecond * 100,
				HealthCheckInterval:       time.Second * 30,
			},
		},
		AI: AIThresholds{
			BRAI001: AIAnalysisRequirements{
				MinConfidenceScore:     0.5,
				MaxAnalysisTime:        time.Second * 10,
				WorkflowGenerationTime: time.Second * 20,
			},
			BRAI002: AIRecommendationRequirements{
				RecommendationConfidence: 0.7,
				ActionValidationTime:     time.Second * 5,
			},
		},
	}
}
