package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// BusinessThresholds represents the complete business requirements threshold configuration
// Business Requirements: Support configuration-driven validation for all BR-XXX-### requirements
// Following project guidelines: structured field values, avoid interface{} where possible
type BusinessThresholds struct {
	Database    DatabaseThresholds    `yaml:"database"`
	Performance PerformanceThresholds `yaml:"performance"`
	Monitoring  MonitoringThresholds  `yaml:"monitoring"`
	AI          AIThresholds          `yaml:"ai"`
	Workflow    WorkflowThresholds    `yaml:"workflow"`
	Safety      SafetyThresholds      `yaml:"safety"`
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
	AccuracyThreshold   float64       `yaml:"accuracy_threshold"`
	ErrorRateThreshold  float64       `yaml:"error_rate_threshold"`
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
	UptimeRequirement         float64       `yaml:"uptime_requirement"`
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
	AccuracyThreshold      float64       `yaml:"accuracy_threshold"`
}

// AIRecommendationRequirements for BR-AI-002
type AIRecommendationRequirements struct {
	RecommendationConfidence float64       `yaml:"recommendation_confidence"`
	ActionValidationTime     time.Duration `yaml:"action_validation_time"`
	MinSuccessRate           float64       `yaml:"min_success_rate"`
}

// WorkflowThresholds represents workflow-related business requirement thresholds
type WorkflowThresholds struct {
	BRWF001 WorkflowExecutionRequirements   `yaml:"BR-WF-001"`
	BRWF002 WorkflowConcurrencyRequirements `yaml:"BR-WF-002"`
}

// WorkflowExecutionRequirements for BR-WF-001
type WorkflowExecutionRequirements struct {
	MaxExecutionTime            time.Duration `yaml:"max_execution_time"`
	MinSuccessRate              float64       `yaml:"min_success_rate"`
	ResourceEfficiencyThreshold float64       `yaml:"resource_efficiency_threshold"`
}

// WorkflowConcurrencyRequirements for BR-WF-002
type WorkflowConcurrencyRequirements struct {
	ParallelExecutionThreshold int `yaml:"parallel_execution_threshold"`
	MaxConcurrentWorkflows     int `yaml:"max_concurrent_workflows"`
	QueueDepthLimit            int `yaml:"queue_depth_limit"`
}

// SafetyThresholds represents safety-related business requirement thresholds
type SafetyThresholds struct {
	BRSF001 SafetyValidationRequirements `yaml:"BR-SF-001"`
	BRSF002 SafetyOperationRequirements  `yaml:"BR-SF-002"`
}

// SafetyValidationRequirements for BR-SF-001
type SafetyValidationRequirements struct {
	MaxRiskScore      float64 `yaml:"max_risk_score"`
	ApprovalThreshold float64 `yaml:"approval_threshold"`
	AutoApprovalLimit float64 `yaml:"auto_approval_limit"`
}

// SafetyOperationRequirements for BR-SF-002
type SafetyOperationRequirements struct {
	RollbackTimeLimit   time.Duration `yaml:"rollback_time_limit"`
	ValidationTimeout   time.Duration `yaml:"validation_timeout"`
	SafetyCheckInterval time.Duration `yaml:"safety_check_interval"`
}

// Configuration holds the complete configuration structure
type Configuration struct {
	BusinessRequirements BusinessThresholds              `yaml:"business_requirements"`
	Environments         map[string]EnvironmentOverrides `yaml:"environments"`
}

// EnvironmentOverrides represents environment-specific threshold overrides
type EnvironmentOverrides struct {
	Database    *DatabaseThresholds    `yaml:"database,omitempty"`
	Performance *PerformanceThresholds `yaml:"performance,omitempty"`
	Monitoring  *MonitoringThresholds  `yaml:"monitoring,omitempty"`
	AI          *AIThresholds          `yaml:"ai,omitempty"`
	Workflow    *WorkflowThresholds    `yaml:"workflow,omitempty"`
	Safety      *SafetyThresholds      `yaml:"safety,omitempty"`
}

// Global configuration cache
var (
	globalThresholds *BusinessThresholds
	thresholdsOnce   sync.Once
	configMutex      sync.RWMutex
)

// LoadThresholds loads business requirement thresholds for the specified environment
// Following project guidelines: clear error handling, no ignored errors
func LoadThresholds(env string) (*BusinessThresholds, error) {
	var loadError error

	thresholdsOnce.Do(func() {
		configMutex.Lock()
		defer configMutex.Unlock()

		// Find configuration file
		configPath := findConfigFile()
		if configPath == "" {
			// Fallback to embedded defaults
			globalThresholds = getDefaultThresholds()
			return
		}

		// Read configuration file
		data, err := os.ReadFile(configPath)
		if err != nil {
			loadError = fmt.Errorf("failed to read configuration file %s: %w", configPath, err)
			globalThresholds = getDefaultThresholds()
			return
		}

		// Parse configuration
		var config Configuration
		if err := yaml.Unmarshal(data, &config); err != nil {
			loadError = fmt.Errorf("failed to parse configuration file %s: %w", configPath, err)
			globalThresholds = getDefaultThresholds()
			return
		}

		// Start with base configuration
		globalThresholds = &config.BusinessRequirements

		// Apply environment-specific overrides
		if envOverrides, exists := config.Environments[env]; exists {
			applyEnvironmentOverrides(globalThresholds, &envOverrides)
		}

		// Validate configuration
		if err := validateThresholds(globalThresholds); err != nil {
			loadError = fmt.Errorf("configuration validation failed: %w", err)
			globalThresholds = getDefaultThresholds()
			return
		}
	})

	if loadError != nil {
		return globalThresholds, loadError
	}

	configMutex.RLock()
	defer configMutex.RUnlock()
	return globalThresholds, nil
}

// GetDatabaseThresholds returns database-specific thresholds for the environment
func GetDatabaseThresholds(env string) (*DatabaseThresholds, error) {
	thresholds, err := LoadThresholds(env)
	if err != nil {
		return nil, err
	}
	return &thresholds.Database, nil
}

// GetPerformanceThresholds returns performance-specific thresholds for the environment
func GetPerformanceThresholds(env string) (*PerformanceThresholds, error) {
	thresholds, err := LoadThresholds(env)
	if err != nil {
		return nil, err
	}
	return &thresholds.Performance, nil
}

// GetAIThresholds returns AI-specific thresholds for the environment
func GetAIThresholds(env string) (*AIThresholds, error) {
	thresholds, err := LoadThresholds(env)
	if err != nil {
		return nil, err
	}
	return &thresholds.AI, nil
}

// GetWorkflowThresholds returns workflow-specific thresholds for the environment
func GetWorkflowThresholds(env string) (*WorkflowThresholds, error) {
	thresholds, err := LoadThresholds(env)
	if err != nil {
		return nil, err
	}
	return &thresholds.Workflow, nil
}

// GetSafetyThresholds returns safety-specific thresholds for the environment
func GetSafetyThresholds(env string) (*SafetyThresholds, error) {
	thresholds, err := LoadThresholds(env)
	if err != nil {
		return nil, err
	}
	return &thresholds.Safety, nil
}

// ReloadConfiguration forces a reload of the configuration
// Following project guidelines: provide clear functionality for testing scenarios
func ReloadConfiguration() {
	configMutex.Lock()
	defer configMutex.Unlock()
	globalThresholds = nil
	thresholdsOnce = sync.Once{}
}

// findConfigFile locates the configuration file using multiple search paths
func findConfigFile() string {
	// Search paths in order of preference
	searchPaths := []string{
		"test/config/thresholds.yaml",
		"../test/config/thresholds.yaml",
		"../../test/config/thresholds.yaml",
		"../../../test/config/thresholds.yaml",
		filepath.Join("test", "config", "thresholds.yaml"),
	}

	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// applyEnvironmentOverrides applies environment-specific overrides to base configuration
func applyEnvironmentOverrides(base *BusinessThresholds, overrides *EnvironmentOverrides) {
	if overrides.Database != nil {
		mergeDatabase(&base.Database, overrides.Database)
	}
	if overrides.Performance != nil {
		mergePerformance(&base.Performance, overrides.Performance)
	}
	if overrides.Monitoring != nil {
		mergeMonitoring(&base.Monitoring, overrides.Monitoring)
	}
	if overrides.AI != nil {
		mergeAI(&base.AI, overrides.AI)
	}
	if overrides.Workflow != nil {
		mergeWorkflow(&base.Workflow, overrides.Workflow)
	}
	if overrides.Safety != nil {
		mergeSafety(&base.Safety, overrides.Safety)
	}
}

// Helper functions for merging environment overrides
func mergeDatabase(base *DatabaseThresholds, override *DatabaseThresholds) {
	if override.BRDatabase001A.UtilizationThreshold > 0 {
		base.BRDatabase001A.UtilizationThreshold = override.BRDatabase001A.UtilizationThreshold
	}
	if override.BRDatabase001A.MaxOpenConnections > 0 {
		base.BRDatabase001A.MaxOpenConnections = override.BRDatabase001A.MaxOpenConnections
	}
	if override.BRDatabase001A.MaxIdleConnections > 0 {
		base.BRDatabase001A.MaxIdleConnections = override.BRDatabase001A.MaxIdleConnections
	}

	if override.BRDatabase001B.HealthScoreThreshold > 0 {
		base.BRDatabase001B.HealthScoreThreshold = override.BRDatabase001B.HealthScoreThreshold
	}
	if override.BRDatabase001B.HealthyScore > 0 {
		base.BRDatabase001B.HealthyScore = override.BRDatabase001B.HealthyScore
	}
	if override.BRDatabase001B.DegradedScore > 0 {
		base.BRDatabase001B.DegradedScore = override.BRDatabase001B.DegradedScore
	}
	if override.BRDatabase001B.FailureRateThreshold > 0 {
		base.BRDatabase001B.FailureRateThreshold = override.BRDatabase001B.FailureRateThreshold
	}
	if override.BRDatabase001B.WaitTimeThreshold > 0 {
		base.BRDatabase001B.WaitTimeThreshold = override.BRDatabase001B.WaitTimeThreshold
	}

	if override.BRDatabase002.ExhaustionRecoveryTime > 0 {
		base.BRDatabase002.ExhaustionRecoveryTime = override.BRDatabase002.ExhaustionRecoveryTime
	}
	if override.BRDatabase002.RecoveryHealthThreshold > 0 {
		base.BRDatabase002.RecoveryHealthThreshold = override.BRDatabase002.RecoveryHealthThreshold
	}
}

func mergePerformance(base *PerformanceThresholds, override *PerformanceThresholds) {
	if override.BRPERF001.MaxResponseTime > 0 {
		base.BRPERF001.MaxResponseTime = override.BRPERF001.MaxResponseTime
	}
	if override.BRPERF001.MinThroughput > 0 {
		base.BRPERF001.MinThroughput = override.BRPERF001.MinThroughput
	}
	if override.BRPERF001.LatencyPercentile95 > 0 {
		base.BRPERF001.LatencyPercentile95 = override.BRPERF001.LatencyPercentile95
	}
	if override.BRPERF001.LatencyPercentile99 > 0 {
		base.BRPERF001.LatencyPercentile99 = override.BRPERF001.LatencyPercentile99
	}
	if override.BRPERF001.AccuracyThreshold > 0 {
		base.BRPERF001.AccuracyThreshold = override.BRPERF001.AccuracyThreshold
	}
	if override.BRPERF001.ErrorRateThreshold > 0 {
		base.BRPERF001.ErrorRateThreshold = override.BRPERF001.ErrorRateThreshold
	}
}

func mergeMonitoring(base *MonitoringThresholds, override *MonitoringThresholds) {
	if override.BRMON001.AlertThreshold > 0 {
		base.BRMON001.AlertThreshold = override.BRMON001.AlertThreshold
	}
	if override.BRMON001.MetricsCollectionInterval > 0 {
		base.BRMON001.MetricsCollectionInterval = override.BRMON001.MetricsCollectionInterval
	}
	if override.BRMON001.HealthCheckInterval > 0 {
		base.BRMON001.HealthCheckInterval = override.BRMON001.HealthCheckInterval
	}
	if override.BRMON001.UptimeRequirement > 0 {
		base.BRMON001.UptimeRequirement = override.BRMON001.UptimeRequirement
	}
}

func mergeAI(base *AIThresholds, override *AIThresholds) {
	if override.BRAI001.MinConfidenceScore > 0 {
		base.BRAI001.MinConfidenceScore = override.BRAI001.MinConfidenceScore
	}
	if override.BRAI001.MaxAnalysisTime > 0 {
		base.BRAI001.MaxAnalysisTime = override.BRAI001.MaxAnalysisTime
	}
	if override.BRAI001.WorkflowGenerationTime > 0 {
		base.BRAI001.WorkflowGenerationTime = override.BRAI001.WorkflowGenerationTime
	}
	if override.BRAI001.AccuracyThreshold > 0 {
		base.BRAI001.AccuracyThreshold = override.BRAI001.AccuracyThreshold
	}

	if override.BRAI002.RecommendationConfidence > 0 {
		base.BRAI002.RecommendationConfidence = override.BRAI002.RecommendationConfidence
	}
	if override.BRAI002.ActionValidationTime > 0 {
		base.BRAI002.ActionValidationTime = override.BRAI002.ActionValidationTime
	}
	if override.BRAI002.MinSuccessRate > 0 {
		base.BRAI002.MinSuccessRate = override.BRAI002.MinSuccessRate
	}
}

func mergeWorkflow(base *WorkflowThresholds, override *WorkflowThresholds) {
	if override.BRWF001.MaxExecutionTime > 0 {
		base.BRWF001.MaxExecutionTime = override.BRWF001.MaxExecutionTime
	}
	if override.BRWF001.MinSuccessRate > 0 {
		base.BRWF001.MinSuccessRate = override.BRWF001.MinSuccessRate
	}
	if override.BRWF001.ResourceEfficiencyThreshold > 0 {
		base.BRWF001.ResourceEfficiencyThreshold = override.BRWF001.ResourceEfficiencyThreshold
	}

	if override.BRWF002.ParallelExecutionThreshold > 0 {
		base.BRWF002.ParallelExecutionThreshold = override.BRWF002.ParallelExecutionThreshold
	}
	if override.BRWF002.MaxConcurrentWorkflows > 0 {
		base.BRWF002.MaxConcurrentWorkflows = override.BRWF002.MaxConcurrentWorkflows
	}
	if override.BRWF002.QueueDepthLimit > 0 {
		base.BRWF002.QueueDepthLimit = override.BRWF002.QueueDepthLimit
	}
}

func mergeSafety(base *SafetyThresholds, override *SafetyThresholds) {
	if override.BRSF001.MaxRiskScore > 0 {
		base.BRSF001.MaxRiskScore = override.BRSF001.MaxRiskScore
	}
	if override.BRSF001.ApprovalThreshold > 0 {
		base.BRSF001.ApprovalThreshold = override.BRSF001.ApprovalThreshold
	}
	if override.BRSF001.AutoApprovalLimit > 0 {
		base.BRSF001.AutoApprovalLimit = override.BRSF001.AutoApprovalLimit
	}

	if override.BRSF002.RollbackTimeLimit > 0 {
		base.BRSF002.RollbackTimeLimit = override.BRSF002.RollbackTimeLimit
	}
	if override.BRSF002.ValidationTimeout > 0 {
		base.BRSF002.ValidationTimeout = override.BRSF002.ValidationTimeout
	}
	if override.BRSF002.SafetyCheckInterval > 0 {
		base.BRSF002.SafetyCheckInterval = override.BRSF002.SafetyCheckInterval
	}
}

// validateThresholds validates the loaded configuration for consistency
func validateThresholds(thresholds *BusinessThresholds) error {
	// Validate database thresholds
	if thresholds.Database.BRDatabase001A.UtilizationThreshold <= 0 || thresholds.Database.BRDatabase001A.UtilizationThreshold > 1 {
		return fmt.Errorf("invalid database utilization threshold: %f", thresholds.Database.BRDatabase001A.UtilizationThreshold)
	}

	if thresholds.Database.BRDatabase001B.HealthScoreThreshold <= 0 || thresholds.Database.BRDatabase001B.HealthScoreThreshold > 1 {
		return fmt.Errorf("invalid database health score threshold: %f", thresholds.Database.BRDatabase001B.HealthScoreThreshold)
	}

	// Validate AI thresholds
	if thresholds.AI.BRAI001.MinConfidenceScore < 0 || thresholds.AI.BRAI001.MinConfidenceScore > 1 {
		return fmt.Errorf("invalid AI confidence threshold: %f", thresholds.AI.BRAI001.MinConfidenceScore)
	}

	// Validate performance thresholds
	if thresholds.Performance.BRPERF001.MaxResponseTime <= 0 {
		return fmt.Errorf("invalid max response time: %v", thresholds.Performance.BRPERF001.MaxResponseTime)
	}

	return nil
}

// getDefaultThresholds provides fallback default thresholds
// Following project guidelines: provide realistic defaults that align with business requirements
func getDefaultThresholds() *BusinessThresholds {
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
				WaitTimeThreshold:    50 * time.Millisecond,
			},
			BRDatabase002: DatabaseRecoveryThresholds{
				ExhaustionRecoveryTime:  200 * time.Millisecond,
				RecoveryHealthThreshold: 0.7,
			},
		},
		Performance: PerformanceThresholds{
			BRPERF001: PerformanceRequirements{
				MaxResponseTime:     2 * time.Second,
				MinThroughput:       1000,
				LatencyPercentile95: 1500 * time.Millisecond,
				LatencyPercentile99: 1800 * time.Millisecond,
				AccuracyThreshold:   0.9,
				ErrorRateThreshold:  0.05,
			},
		},
		Monitoring: MonitoringThresholds{
			BRMON001: MonitoringRequirements{
				AlertThreshold:            0.95,
				MetricsCollectionInterval: 100 * time.Millisecond,
				HealthCheckInterval:       30 * time.Second,
				UptimeRequirement:         0.999,
			},
		},
		AI: AIThresholds{
			BRAI001: AIAnalysisRequirements{
				MinConfidenceScore:     0.5,
				MaxAnalysisTime:        10 * time.Second,
				WorkflowGenerationTime: 20 * time.Second,
				AccuracyThreshold:      0.8,
			},
			BRAI002: AIRecommendationRequirements{
				RecommendationConfidence: 0.7,
				ActionValidationTime:     5 * time.Second,
				MinSuccessRate:           0.85,
			},
		},
		Workflow: WorkflowThresholds{
			BRWF001: WorkflowExecutionRequirements{
				MaxExecutionTime:            300 * time.Second,
				MinSuccessRate:              0.9,
				ResourceEfficiencyThreshold: 0.75,
			},
			BRWF002: WorkflowConcurrencyRequirements{
				ParallelExecutionThreshold: 3,
				MaxConcurrentWorkflows:     10,
				QueueDepthLimit:            50,
			},
		},
		Safety: SafetyThresholds{
			BRSF001: SafetyValidationRequirements{
				MaxRiskScore:      0.3,
				ApprovalThreshold: 0.5,
				AutoApprovalLimit: 0.2,
			},
			BRSF002: SafetyOperationRequirements{
				RollbackTimeLimit:   60 * time.Second,
				ValidationTimeout:   30 * time.Second,
				SafetyCheckInterval: 10 * time.Second,
			},
		},
	}
}
