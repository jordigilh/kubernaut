package testutil

import (
	"time"

	"github.com/google/uuid"
	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/ai/conditions"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// Test data constants - following development guidelines: eliminate magic values
const (
	// Default test values
	DefaultTestNamespace    = "default"
	DefaultProductionNS     = "production"
	DefaultWaringSeverity   = "warning"
	DefaultCriticalSeverity = "critical"
	DefaultFiringStatus     = "firing"
	DefaultTestApp          = "test-app"
	DefaultConfidenceScore  = 0.85
	DefaultCPUThreshold     = 80
	DefaultMemoryThreshold  = 90
	DefaultMaxConcurrent    = 5
	DefaultMaxConditions    = 1000
	DefaultReplicaCount     = 5
	DefaultTimeoutSeconds   = 10
)

// TestDataFactory provides centralized test data creation for comprehensive testing
// Business Requirements: Supports testing for BR-PA-001 through BR-PA-013 (alert processing),
// BR-AP-001 through BR-AP-025 (alert pipeline), BR-WF-001 through BR-WF-020 (workflow engine),
// and BR-AI-001 through BR-AI-020 (AI/ML capabilities)
// Following development guidelines: consolidate test data creation to improve maintainability
type TestDataFactory struct{}

// NewTestDataFactory creates a new test data factory
func NewTestDataFactory() *TestDataFactory {
	return &TestDataFactory{}
}

// =============================================================================
// ALERT CREATION PATTERNS
// Business Requirements: Support testing for BR-PA-001 (alert reception), BR-AP-002 (alert enrichment),
// BR-AP-003 (alert normalization), BR-WH-001 through BR-WH-025 (webhook processing)
// =============================================================================

// CreateStandardAlert creates a standard test alert
// Business Requirements: BR-PA-001 (receive alerts), BR-PA-002 (validate payloads)
// Following development guidelines: use constants instead of magic values
func (f *TestDataFactory) CreateStandardAlert() types.Alert {
	return types.Alert{
		Name:        "TestAlert",
		Description: "Test alert description",
		Severity:    DefaultWaringSeverity,
		Status:      DefaultFiringStatus,
		Namespace:   DefaultTestNamespace,
		Resource:    "test-resource",
		Labels: map[string]string{
			"app":  DefaultTestApp,
			"tier": "backend",
		},
		Annotations: map[string]string{
			"summary":     "Test alert summary",
			"description": "Test alert detailed description",
		},
	}
}

// CreateHighMemoryAlert creates a high memory usage alert
func (f *TestDataFactory) CreateHighMemoryAlert() types.Alert {
	return types.Alert{
		Name:        "HighMemoryUsage",
		Description: "Memory usage is above 90% for pod test-pod",
		Severity:    "warning",
		Status:      "firing",
		Namespace:   "default",
		Resource:    "test-pod",
		Labels: map[string]string{
			"pod":       "test-pod",
			"container": "app",
		},
		Annotations: map[string]string{
			"summary":     "High memory usage detected",
			"description": "Memory usage is consistently above 90%",
		},
	}
}

// CreateHighCPUAlert creates a high CPU usage alert
func (f *TestDataFactory) CreateHighCPUAlert() types.Alert {
	return types.Alert{
		Name:        "HighCPUUsage",
		Description: "CPU usage is above 80% for deployment test-app",
		Severity:    "critical",
		Status:      "firing",
		Namespace:   "production",
		Resource:    "test-app",
		Labels: map[string]string{
			"app":        "test-app",
			"deployment": "test-app",
		},
		Annotations: map[string]string{
			"summary":     "High CPU usage detected",
			"description": "CPU usage is consistently above 80%",
		},
	}
}

// CreatePodCrashingAlert creates a pod crashing alert
func (f *TestDataFactory) CreatePodCrashingAlert() types.Alert {
	return types.Alert{
		Name:        "PodCrashLoopBackoff",
		Description: "Pod is in CrashLoopBackoff state",
		Severity:    "critical",
		Status:      "firing",
		Namespace:   "default",
		Resource:    "crashing-pod",
		Labels: map[string]string{
			"pod": "crashing-pod",
			"app": "unstable-app",
		},
		Annotations: map[string]string{
			"summary":     "Pod is crashing repeatedly",
			"description": "Pod has crashed 5 times in the last 10 minutes",
		},
	}
}

// CreateCustomAlert creates a custom alert with specified parameters
// Following development guidelines: strengthen assertions and validate inputs
func (f *TestDataFactory) CreateCustomAlert(name, severity, namespace, resource string) types.Alert {
	// Validate required parameters - following development guidelines: strengthen assertions
	name = validateStringWithDefault(name, "DefaultTestAlert")
	severity = validateStringWithDefault(severity, DefaultWaringSeverity)
	namespace = validateStringWithDefault(namespace, DefaultTestNamespace)
	resource = validateStringWithDefault(resource, "test-resource")

	return types.Alert{
		Name:        name,
		Description: "Custom test alert for " + resource,
		Severity:    severity,
		Status:      "firing",
		Namespace:   namespace,
		Resource:    resource,
		Labels: map[string]string{
			"resource": resource,
			"custom":   "true",
		},
		Annotations: map[string]string{
			"summary": "Custom alert summary for " + name,
		},
	}
}

// =============================================================================
// ACTION RECOMMENDATION PATTERNS
// Business Requirements: Support testing for BR-PA-006 through BR-PA-010 (AI decision making),
// BR-PA-011 through BR-PA-013 (action execution), BR-AI-001 through BR-AI-005 (analytics)
// =============================================================================

// CreateStandardActionRecommendation creates a standard action recommendation
// Business Requirements: BR-PA-007 (remediation recommendations), BR-PA-009 (confidence scoring)
// Following development guidelines: use constants instead of magic values
func (f *TestDataFactory) CreateStandardActionRecommendation() *types.ActionRecommendation {
	return &types.ActionRecommendation{
		Action:     "scale_deployment",
		Confidence: DefaultConfidenceScore,
		Parameters: map[string]interface{}{
			"replicas": DefaultReplicaCount,
		},
		Reasoning: &types.ReasoningDetails{
			Summary:           "Standard scaling recommendation",
			HistoricalContext: "Previous scaling actions were successful",
			PrimaryReason:     "Resource utilization above threshold",
		},
	}
}

// CreateRestartPodRecommendation creates a restart pod recommendation
func (f *TestDataFactory) CreateRestartPodRecommendation() *types.ActionRecommendation {
	return &types.ActionRecommendation{
		Action:     "restart_pod",
		Confidence: 0.75,
		Parameters: map[string]interface{}{
			"force":       true,
			"gracePeriod": 30,
		},
		Reasoning: &types.ReasoningDetails{
			Summary:       "Pod restart recommended to recover from error state",
			PrimaryReason: "Pod is in CrashLoopBackoff",
		},
	}
}

// CreateIncreaseResourcesRecommendation creates an increase resources recommendation
func (f *TestDataFactory) CreateIncreaseResourcesRecommendation() *types.ActionRecommendation {
	return &types.ActionRecommendation{
		Action:     "increase_resources",
		Confidence: 0.90,
		Parameters: map[string]interface{}{
			"memory_limit": "2Gi",
			"cpu_limit":    "1000m",
		},
		Reasoning: &types.ReasoningDetails{
			Summary:       "Increase resource limits to handle load",
			PrimaryReason: "Resource exhaustion detected",
		},
	}
}

// =============================================================================
// CONFIGURATION PATTERNS
// =============================================================================

// CreateStandardLLMConfig creates a standard LLM configuration
func (f *TestDataFactory) CreateStandardLLMConfig() config.LLMConfig {
	return config.LLMConfig{
		Provider:    "ramalama",
		Endpoint:    "http://192.168.1.169:8080",
		Model:       "oss-gpt:20b",
		Temperature: 0.1,
		MaxTokens:   2048,
		Timeout:     30 * time.Second,
	}
}

// CreateStandardActionsConfig creates a standard actions configuration
func (f *TestDataFactory) CreateStandardActionsConfig() config.ActionsConfig {
	return config.ActionsConfig{
		DryRun:         false,
		MaxConcurrent:  5,
		CooldownPeriod: 5 * time.Minute,
	}
}

// CreateConditionsEngineConfig creates a conditions engine configuration
// Following development guidelines: use existing types, avoid undefined references
func (f *TestDataFactory) CreateConditionsEngineConfig() *conditions.EngineConfig {
	return &conditions.EngineConfig{
		MaxEvaluationTime: 10 * time.Second,
		EnableCaching:     true,
		LogLevel:          "info",
		MaxConditions:     1000,
	}
}

// =============================================================================
// WORKFLOW PATTERNS
// Business Requirements: Support testing for BR-WF-001 through BR-WF-020 (workflow execution),
// BR-IWB-001 through BR-IWB-020 (intelligent workflow building), BR-ORCH-001 through BR-ORCH-008 (orchestration)
// =============================================================================

// CreateStandardConditionSpec creates a standard condition specification
// Following development guidelines: reuse code for ID generation
func (f *TestDataFactory) CreateStandardConditionSpec() *types.ConditionSpec {
	return &types.ConditionSpec{
		ID:          generateConditionID(),
		Type:        "metric",
		Description: "CPU usage threshold condition",
		Parameters: map[string]interface{}{
			"expression": "cpu_usage < 80",
			"threshold":  80,
		},
	}
}

// CreateResourceCondition creates a resource-based workflow condition
// Following development guidelines: reuse code for ID generation
func (f *TestDataFactory) CreateResourceCondition() *types.ConditionSpec {
	return &types.ConditionSpec{
		ID:          generateConditionID(),
		Type:        "resource",
		Description: "Pod readiness check condition",
		Parameters: map[string]interface{}{
			"expression": "namespace=default AND pod_ready=true",
			"namespace":  "default",
		},
	}
}

// CreateTimeCondition creates a time-based workflow condition
// Following development guidelines: reuse code for ID generation
func (f *TestDataFactory) CreateTimeCondition() *types.ConditionSpec {
	return &types.ConditionSpec{
		ID:          generateConditionID(),
		Type:        "time",
		Description: "Business hours check condition",
		Parameters: map[string]interface{}{
			"expression": "business_hours=true",
			"timeout":    "5m",
		},
	}
}

// CreateStepContext creates a standard step context
// Following development guidelines: reuse code for ID generation
func (f *TestDataFactory) CreateStepContext() *engine.StepContext {
	return &engine.StepContext{
		ExecutionID: generateExecutionID(),
		StepID:      generateStepID(),
		Variables: map[string]interface{}{
			"test_var":  "test_value",
			"threshold": 80,
			"namespace": "default",
		},
	}
}

// =============================================================================
// VECTOR DATABASE PATTERNS
// =============================================================================

// CreateStandardActionPattern creates a standard action pattern
// Following development guidelines: reuse code for ID generation
func (f *TestDataFactory) CreateStandardActionPattern() *vector.ActionPattern {
	return &vector.ActionPattern{
		ID:               generatePatternID(),
		ActionType:       "scale_deployment",
		AlertName:        "HighMemoryUsage",
		AlertSeverity:    "warning",
		Namespace:        "default",
		ResourceType:     "Deployment",
		ResourceName:     "test-app",
		ActionParameters: map[string]interface{}{"replicas": 3},
		ContextLabels:    map[string]string{"app": "test-app"},
		Embedding:        []float64{0.1, 0.2, 0.3, 0.4, 0.5},
		CreatedAt:        time.Now(),
		EffectivenessData: &vector.EffectivenessData{
			Score:                0.85,
			SuccessCount:         8,
			FailureCount:         2,
			AverageExecutionTime: 5 * time.Minute,
			SideEffectsCount:     0,
			RecurrenceRate:       0.1,
			LastAssessed:         time.Now(),
		},
	}
}

// CreateTestActionPatterns creates a set of test action patterns
func (f *TestDataFactory) CreateTestActionPatterns() []*vector.ActionPattern {
	baseTime := time.Now().Add(-24 * time.Hour)

	return []*vector.ActionPattern{
		{
			ID:               "pattern-1",
			ActionType:       "scale_deployment",
			AlertName:        "HighMemoryUsage",
			AlertSeverity:    "warning",
			Namespace:        "production",
			ResourceType:     "Deployment",
			ResourceName:     "web-server",
			ActionParameters: map[string]interface{}{"replicas": 5},
			ContextLabels:    map[string]string{"app": "web", "tier": "frontend"},
			Embedding:        []float64{0.1, 0.2, 0.8, 0.5, 0.3},
			CreatedAt:        baseTime,
			EffectivenessData: &vector.EffectivenessData{
				Score:                0.85,
				SuccessCount:         8,
				FailureCount:         2,
				AverageExecutionTime: 5 * time.Minute,
				RecurrenceRate:       0.1,
				LastAssessed:         baseTime.Add(10 * time.Minute),
			},
		},
		{
			ID:               "pattern-2",
			ActionType:       "restart_pod",
			AlertName:        "PodCrashing",
			AlertSeverity:    "critical",
			Namespace:        "production",
			ResourceType:     "Pod",
			ResourceName:     "api-server-123",
			ActionParameters: map[string]interface{}{"force": true},
			ContextLabels:    map[string]string{"app": "api", "tier": "backend"},
			Embedding:        []float64{0.3, 0.7, 0.2, 0.8, 0.4},
			CreatedAt:        baseTime.Add(time.Hour),
			EffectivenessData: &vector.EffectivenessData{
				Score:                0.65,
				SuccessCount:         6,
				FailureCount:         4,
				AverageExecutionTime: 2 * time.Minute,
				SideEffectsCount:     2,
				RecurrenceRate:       0.3,
				LastAssessed:         baseTime.Add(time.Hour + 5*time.Minute),
			},
		},
		{
			ID:               "pattern-3",
			ActionType:       "increase_resources",
			AlertName:        "ResourceExhaustion",
			AlertSeverity:    "critical",
			Namespace:        "production",
			ResourceType:     "Deployment",
			ResourceName:     "database",
			ActionParameters: map[string]interface{}{"cpu": "2000m", "memory": "4Gi"},
			ContextLabels:    map[string]string{"app": "db", "tier": "data"},
			Embedding:        []float64{0.6, 0.1, 0.4, 0.9, 0.2},
			CreatedAt:        baseTime.Add(2 * time.Hour),
			EffectivenessData: &vector.EffectivenessData{
				Score:                0.92,
				SuccessCount:         9,
				FailureCount:         1,
				AverageExecutionTime: 3 * time.Minute,
				RecurrenceRate:       0.05,
				LastAssessed:         baseTime.Add(2*time.Hour + 8*time.Minute),
			},
		},
	}
}

// =============================================================================
// ACTION HISTORY PATTERNS
// =============================================================================

// CreateStandardActionTrace creates a standard resource action trace
// Following development guidelines: reuse code for ID generation
func (f *TestDataFactory) CreateStandardActionTrace() actionhistory.ResourceActionTrace {
	return actionhistory.ResourceActionTrace{
		ID:         123,
		ActionID:   generateTraceID(),
		ActionType: "scale",
		ActionParameters: actionhistory.JSONMap{
			"replicas": 3,
		},
		ActionTimestamp: time.Now(),
		ExecutionStatus: "completed",
		ModelUsed:       "test-model",
		ModelConfidence: 0.85,
	}
}

// CreateMultipleActionTraces creates multiple action traces for testing
func (f *TestDataFactory) CreateMultipleActionTraces() []actionhistory.ResourceActionTrace {
	baseTime := time.Now().Add(-1 * time.Hour)

	return []actionhistory.ResourceActionTrace{
		{
			ID:               1,
			ActionID:         "trace-1",
			ActionType:       "scale",
			ActionParameters: actionhistory.JSONMap{"replicas": 5},
			ActionTimestamp:  baseTime,
			ExecutionStatus:  "completed",
			ModelUsed:        "test-model",
			ModelConfidence:  0.85,
		},
		{
			ID:               2,
			ActionID:         "trace-2",
			ActionType:       "restart",
			ActionParameters: actionhistory.JSONMap{"force": true},
			ActionTimestamp:  baseTime.Add(10 * time.Minute),
			ExecutionStatus:  "completed",
			ModelUsed:        "test-model",
			ModelConfidence:  0.75,
		},
		{
			ID:               3,
			ActionID:         "trace-3",
			ActionType:       "update_resources",
			ActionParameters: actionhistory.JSONMap{"memory": "2Gi"},
			ActionTimestamp:  baseTime.Add(20 * time.Minute),
			ExecutionStatus:  "failed",
			ModelUsed:        "test-model",
			ModelConfidence:  0.60,
		},
	}
}

// =============================================================================
// UTILITY FUNCTIONS - Following development guidelines: reuse code whenever possible
// =============================================================================

// generateUniqueID creates a unique ID with the specified prefix
// Following development guidelines: avoid duplicating structure names and reuse code
func generateUniqueID(prefix string) string {
	return prefix + "-" + uuid.New().String()
}

// ID generation convenience functions using the consolidated approach
func generateConditionID() string { return generateUniqueID("test-condition") }
func generatePatternID() string   { return generateUniqueID("test-pattern") }
func generateExecutionID() string { return generateUniqueID("test-execution") }
func generateStepID() string      { return generateUniqueID("test-step") }
func generateTraceID() string     { return generateUniqueID("test-trace") }
func generateActionID() string    { return generateUniqueID("test-action") }

// Validation helpers - following development guidelines: strengthen assertions and reuse code
func validateStringWithDefault(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}
