package templates

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// WorkflowValidator interface for validating workflow templates
type WorkflowValidator interface {
	ValidateWorkflow(ctx context.Context, template *engine.WorkflowTemplate) (*engine.ValidationReport, error)
}

// WorkflowTemplateFactory creates standardized workflow templates
type WorkflowTemplateFactory struct {
	alertPatterns    map[string]*engine.WorkflowTemplate
	actionSequences  map[string][]*engine.WorkflowStep
	conditionLibrary map[string]*engine.WorkflowCondition
	validator        WorkflowValidator
	log              *logrus.Logger
	config           *TemplateFactoryConfig
}

// TemplateFactoryConfig holds configuration for the template factory
type TemplateFactoryConfig struct {
	DefaultTimeout     time.Duration `yaml:"default_timeout" default:"10m"`
	MaxWorkflowSteps   int           `yaml:"max_workflow_steps" default:"15"`
	EnableSafetyChecks bool          `yaml:"enable_safety_checks" default:"true"`
	DefaultRetries     int           `yaml:"default_retries" default:"3"`
	RetryBackoffBase   time.Duration `yaml:"retry_backoff_base" default:"30s"`
	EnableRollback     bool          `yaml:"enable_rollback" default:"true"`
	SafetyLevel        string        `yaml:"safety_level" default:"high"`
}

// SafetyConstraints defines safety constraints for templates
type SafetyConstraints struct {
	MaxResourceImpact    float64       `json:"max_resource_impact"` // 0.0-1.0
	RequireConfirmation  bool          `json:"require_confirmation"`
	DisruptiveOperations bool          `json:"disruptive_operations"`
	RollbackRequired     bool          `json:"rollback_required"`
	MaxConcurrentActions int           `json:"max_concurrent_actions"`
	CooldownPeriod       time.Duration `json:"cooldown_period"`
}

// NewWorkflowTemplateFactory creates a new template factory instance
func NewWorkflowTemplateFactory(validator WorkflowValidator, log *logrus.Logger) *WorkflowTemplateFactory {
	config := &TemplateFactoryConfig{
		DefaultTimeout:     10 * time.Minute,
		MaxWorkflowSteps:   15,
		EnableSafetyChecks: true,
		DefaultRetries:     3,
		RetryBackoffBase:   30 * time.Second,
		EnableRollback:     true,
		SafetyLevel:        "high",
	}

	factory := &WorkflowTemplateFactory{
		alertPatterns:    make(map[string]*engine.WorkflowTemplate),
		actionSequences:  make(map[string][]*engine.WorkflowStep),
		conditionLibrary: make(map[string]*engine.WorkflowCondition),
		validator:        validator,
		log:              log,
		config:           config,
	}

	// Initialize predefined templates and conditions
	factory.initializePredefinedTemplates()
	factory.initializeConditionLibrary()
	factory.initializeActionSequences()

	return factory
}

// BuildHighMemoryWorkflow creates workflow for high memory usage scenarios
func (wtf *WorkflowTemplateFactory) BuildHighMemoryWorkflow(alert types.Alert) *engine.WorkflowTemplate {
	workflowID := fmt.Sprintf("high-memory-%s", uuid.New().String()[:8])

	template := &engine.WorkflowTemplate{
		ID:          workflowID,
		Name:        "High Memory Usage Remediation",
		Description: fmt.Sprintf("Automated remediation for high memory usage in %s/%s", alert.Namespace, alert.Resource),
		Version:     "1.0.0",
		Tags:        []string{"memory", "performance", "scaling", "automated"},
		CreatedBy:   "template-factory",
		CreatedAt:   time.Now(),
	}

	// Step 1: Check current memory usage and pod state
	checkStep := &engine.WorkflowStep{
		ID:   "check-memory-state",
		Name: "Check Current Memory Usage",
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "kubernetes",
			Parameters: map[string]interface{}{
				"action": "get_resource_metrics",
				"metric": "memory",
			},
			Target: &engine.ActionTarget{
				Type:      "kubernetes",
				Namespace: alert.Namespace,
				Resource:  alert.Resource,
				Name:      getResourceNameFromAlert(alert),
			},
		},
		Timeout:     30 * time.Second,
		RetryPolicy: wtf.createDefaultRetryPolicy(),
	}

	// Step 2: Evaluate memory threshold condition
	memoryThreshold := wtf.conditionLibrary["memory_threshold"]
	evaluateStep := &engine.WorkflowStep{
		ID:           "evaluate-memory-threshold",
		Name:         "Evaluate Memory Threshold",
		Type:         engine.StepTypeCondition,
		Condition:    memoryThreshold,
		Dependencies: []string{"check-memory-state"},
		Timeout:      15 * time.Second,
	}

	// Step 3: Scale deployment (conditional on high memory)
	scaleStep := &engine.WorkflowStep{
		ID:           "scale-deployment",
		Name:         "Scale Deployment",
		Type:         engine.StepTypeAction,
		Dependencies: []string{"evaluate-memory-threshold"},
		Action: &engine.StepAction{
			Type: "kubernetes",
			Parameters: map[string]interface{}{
				"action":   "scale_deployment",
				"replicas": 3, // Conservative scaling
			},
			Target: &engine.ActionTarget{
				Type:      "kubernetes",
				Namespace: alert.Namespace,
				Resource:  "deployment",
				Name:      getResourceNameFromAlert(alert),
			},
			Rollback: &engine.RollbackAction{
				Type: "scale_deployment",
				Parameters: map[string]interface{}{
					"replicas": 1, // Rollback to original
				},
			},
		},
		Timeout:     60 * time.Second,
		RetryPolicy: wtf.createDefaultRetryPolicy(),
		OnSuccess:   []string{"verify-scaling"},
		OnFailure:   []string{"notify-failure"},
	}

	// Step 4: Verify scaling success
	verifyStep := &engine.WorkflowStep{
		ID:           "verify-scaling",
		Name:         "Verify Scaling Success",
		Type:         engine.StepTypeAction,
		Dependencies: []string{"scale-deployment"},
		Action: &engine.StepAction{
			Type: "kubernetes",
			Parameters: map[string]interface{}{
				"action":  "verify_scaling",
				"timeout": "120s",
			},
			Target: &engine.ActionTarget{
				Type:      "kubernetes",
				Namespace: alert.Namespace,
				Resource:  "deployment",
				Name:      getResourceNameFromAlert(alert),
			},
		},
		Timeout:   2 * time.Minute,
		OnFailure: []string{"rollback-scaling"},
	}

	// Step 5: Rollback on failure
	rollbackStep := &engine.WorkflowStep{
		ID:   "rollback-scaling",
		Name: "Rollback Scaling",
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "kubernetes",
			Parameters: map[string]interface{}{
				"action":   "scale_deployment",
				"replicas": 1,
			},
			Target: &engine.ActionTarget{
				Type:      "kubernetes",
				Namespace: alert.Namespace,
				Resource:  "deployment",
				Name:      getResourceNameFromAlert(alert),
			},
		},
		Timeout:   60 * time.Second,
		OnSuccess: []string{"notify-rollback"},
	}

	// Step 6: Notification steps
	notifyFailureStep := &engine.WorkflowStep{
		ID:   "notify-failure",
		Name: "Notify Failure",
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "notification",
			Parameters: map[string]interface{}{
				"action":   "notify_only",
				"message":  "High memory workflow failed to scale deployment",
				"severity": "warning",
			},
		},
		Timeout: 10 * time.Second,
	}

	notifyRollbackStep := &engine.WorkflowStep{
		ID:   "notify-rollback",
		Name: "Notify Rollback",
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "notification",
			Parameters: map[string]interface{}{
				"action":   "notify_only",
				"message":  "High memory workflow rolled back due to scaling failure",
				"severity": "error",
			},
		},
		Timeout: 10 * time.Second,
	}

	template.Steps = []*engine.WorkflowStep{
		checkStep, evaluateStep, scaleStep, verifyStep,
		rollbackStep, notifyFailureStep, notifyRollbackStep,
	}
	template.Conditions = []*engine.WorkflowCondition{memoryThreshold}
	template.Timeouts = &engine.WorkflowTimeouts{
		Execution: wtf.config.DefaultTimeout,
		Step:      60 * time.Second,
	}
	template.Recovery = &engine.RecoveryPolicy{
		Enabled:         true,
		MaxRecoveryTime: wtf.config.DefaultTimeout / 2,
	}

	return template
}

// BuildPodCrashLoopWorkflow creates workflow for pod crash loop scenarios
func (wtf *WorkflowTemplateFactory) BuildPodCrashLoopWorkflow(alert types.Alert) *engine.WorkflowTemplate {
	workflowID := fmt.Sprintf("crash-loop-%s", uuid.New().String()[:8])

	template := &engine.WorkflowTemplate{
		ID:          workflowID,
		Name:        "Pod Crash Loop Recovery",
		Description: fmt.Sprintf("Automated recovery for crash loop in %s/%s", alert.Namespace, alert.Resource),
		Version:     "1.0.0",
		Tags:        []string{"crash-loop", "recovery", "diagnostics", "automated"},
		CreatedBy:   "template-factory",
		CreatedAt:   time.Now(),
	}

	// Step 1: Collect diagnostics
	diagnosticsStep := &engine.WorkflowStep{
		ID:   "collect-diagnostics",
		Name: "Collect Pod Diagnostics",
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "kubernetes",
			Parameters: map[string]interface{}{
				"action":  "collect_diagnostics",
				"include": []string{"logs", "events", "describe"},
			},
			Target: &engine.ActionTarget{
				Type:      "kubernetes",
				Namespace: alert.Namespace,
				Resource:  "pod",
				Name:      getResourceNameFromAlert(alert),
			},
		},
		Timeout:     30 * time.Second,
		RetryPolicy: wtf.createDefaultRetryPolicy(),
	}

	// Step 2: Analyze crash pattern
	analyzeStep := &engine.WorkflowStep{
		ID:           "analyze-crash-pattern",
		Name:         "Analyze Crash Pattern",
		Type:         engine.StepTypeCondition,
		Dependencies: []string{"collect-diagnostics"},
		Condition:    wtf.conditionLibrary["crash_pattern_analysis"],
		Timeout:      20 * time.Second,
	}

	// Step 3: Restart pod (primary recovery action)
	restartStep := &engine.WorkflowStep{
		ID:           "restart-pod",
		Name:         "Restart Pod",
		Type:         engine.StepTypeAction,
		Dependencies: []string{"analyze-crash-pattern"},
		Action: &engine.StepAction{
			Type: "kubernetes",
			Parameters: map[string]interface{}{
				"action": "restart_pod",
				"force":  false,
			},
			Target: &engine.ActionTarget{
				Type:      "kubernetes",
				Namespace: alert.Namespace,
				Resource:  "pod",
				Name:      getResourceNameFromAlert(alert),
			},
		},
		Timeout:     60 * time.Second,
		RetryPolicy: wtf.createDefaultRetryPolicy(),
		OnSuccess:   []string{"verify-restart"},
		OnFailure:   []string{"rollback-deployment"},
	}

	// Step 4: Verify restart success
	verifyStep := &engine.WorkflowStep{
		ID:           "verify-restart",
		Name:         "Verify Pod Restart",
		Type:         engine.StepTypeAction,
		Dependencies: []string{"restart-pod"},
		Action: &engine.StepAction{
			Type: "kubernetes",
			Parameters: map[string]interface{}{
				"action":  "verify_pod_health",
				"timeout": "90s",
			},
			Target: &engine.ActionTarget{
				Type:      "kubernetes",
				Namespace: alert.Namespace,
				Resource:  "pod",
				Name:      getResourceNameFromAlert(alert),
			},
		},
		Timeout:   90 * time.Second,
		OnFailure: []string{"rollback-deployment"},
	}

	// Step 5: Rollback deployment if restart fails
	rollbackStep := &engine.WorkflowStep{
		ID:   "rollback-deployment",
		Name: "Rollback Deployment",
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "kubernetes",
			Parameters: map[string]interface{}{
				"action":   "rollback_deployment",
				"revision": "previous",
			},
			Target: &engine.ActionTarget{
				Type:      "kubernetes",
				Namespace: alert.Namespace,
				Resource:  "deployment",
				Name:      getResourceNameFromAlert(alert),
			},
		},
		Timeout:   2 * time.Minute,
		OnSuccess: []string{"notify-rollback-success"},
		OnFailure: []string{"notify-manual-intervention"},
	}

	// Notification steps
	notifyRollbackStep := &engine.WorkflowStep{
		ID:   "notify-rollback-success",
		Name: "Notify Rollback Success",
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "notification",
			Parameters: map[string]interface{}{
				"action":   "notify_only",
				"message":  "Crash loop resolved via deployment rollback",
				"severity": "info",
			},
		},
		Timeout: 10 * time.Second,
	}

	notifyManualStep := &engine.WorkflowStep{
		ID:   "notify-manual-intervention",
		Name: "Notify Manual Intervention Required",
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "notification",
			Parameters: map[string]interface{}{
				"action":   "notify_only",
				"message":  "Crash loop requires manual intervention - automated recovery failed",
				"severity": "error",
			},
		},
		Timeout: 10 * time.Second,
	}

	template.Steps = []*engine.WorkflowStep{
		diagnosticsStep, analyzeStep, restartStep, verifyStep,
		rollbackStep, notifyRollbackStep, notifyManualStep,
	}
	template.Conditions = []*engine.WorkflowCondition{
		wtf.conditionLibrary["crash_pattern_analysis"],
	}
	template.Timeouts = &engine.WorkflowTimeouts{
		Execution: wtf.config.DefaultTimeout,
		Step:      60 * time.Second,
	}
	template.Recovery = &engine.RecoveryPolicy{
		Enabled: true,
		// More conservative recovery time for crash loops
		MaxRecoveryTime: 5 * time.Minute,
	}

	return template
}

// BuildNodeIssueWorkflow creates workflow for node-level issues
func (wtf *WorkflowTemplateFactory) BuildNodeIssueWorkflow(alert types.Alert) *engine.WorkflowTemplate {
	workflowID := fmt.Sprintf("node-issue-%s", uuid.New().String()[:8])

	template := &engine.WorkflowTemplate{
		ID:          workflowID,
		Name:        "Node Issue Remediation",
		Description: fmt.Sprintf("Automated remediation for node issues in %s", alert.Resource),
		Version:     "1.0.0",
		Tags:        []string{"node", "infrastructure", "maintenance", "automated"},
		CreatedBy:   "template-factory",
		CreatedAt:   time.Now(),
	}

	// Step 1: Assess node health
	assessStep := &engine.WorkflowStep{
		ID:   "assess-node-health",
		Name: "Assess Node Health",
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "kubernetes",
			Parameters: map[string]interface{}{
				"action": "get_node_status",
			},
			Target: &engine.ActionTarget{
				Type:     "kubernetes",
				Resource: "node",
				Name:     getResourceNameFromAlert(alert),
			},
		},
		Timeout:     30 * time.Second,
		RetryPolicy: wtf.createDefaultRetryPolicy(),
	}

	// Step 2: Evaluate node condition
	evaluateStep := &engine.WorkflowStep{
		ID:           "evaluate-node-condition",
		Name:         "Evaluate Node Condition",
		Type:         engine.StepTypeCondition,
		Dependencies: []string{"assess-node-health"},
		Condition:    wtf.conditionLibrary["node_health_condition"],
		Timeout:      15 * time.Second,
	}

	// Step 3: Cordon node (prevent new workloads)
	cordonStep := &engine.WorkflowStep{
		ID:           "cordon-node",
		Name:         "Cordon Node",
		Type:         engine.StepTypeAction,
		Dependencies: []string{"evaluate-node-condition"},
		Action: &engine.StepAction{
			Type: "kubernetes",
			Parameters: map[string]interface{}{
				"action": "cordon_node",
			},
			Target: &engine.ActionTarget{
				Type:     "kubernetes",
				Resource: "node",
				Name:     getResourceNameFromAlert(alert),
			},
			Rollback: &engine.RollbackAction{
				Type: "uncordon_node",
				Parameters: map[string]interface{}{
					"action": "uncordon_node",
				},
			},
		},
		Timeout:   30 * time.Second,
		OnSuccess: []string{"migrate-workloads"},
		OnFailure: []string{"notify-cordon-failure"},
	}

	// Step 4: Migrate workloads
	migrateStep := &engine.WorkflowStep{
		ID:           "migrate-workloads",
		Name:         "Migrate Workloads",
		Type:         engine.StepTypeAction,
		Dependencies: []string{"cordon-node"},
		Action: &engine.StepAction{
			Type: "kubernetes",
			Parameters: map[string]interface{}{
				"action":   "migrate_workload",
				"timeout":  "300s",
				"graceful": true,
			},
			Target: &engine.ActionTarget{
				Type:     "kubernetes",
				Resource: "node",
				Name:     getResourceNameFromAlert(alert),
			},
		},
		Timeout:   5 * time.Minute,
		OnSuccess: []string{"verify-migration"},
		OnFailure: []string{"notify-migration-failure"},
	}

	// Step 5: Verify migration
	verifyStep := &engine.WorkflowStep{
		ID:           "verify-migration",
		Name:         "Verify Migration Success",
		Type:         engine.StepTypeAction,
		Dependencies: []string{"migrate-workloads"},
		Action: &engine.StepAction{
			Type: "kubernetes",
			Parameters: map[string]interface{}{
				"action": "verify_node_empty",
			},
			Target: &engine.ActionTarget{
				Type:     "kubernetes",
				Resource: "node",
				Name:     getResourceNameFromAlert(alert),
			},
		},
		Timeout:   60 * time.Second,
		OnSuccess: []string{"notify-success"},
		OnFailure: []string{"notify-migration-incomplete"},
	}

	// Notification steps
	notifySuccessStep := &engine.WorkflowStep{
		ID:   "notify-success",
		Name: "Notify Success",
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "notification",
			Parameters: map[string]interface{}{
				"action":   "notify_only",
				"message":  "Node issue resolved - workloads migrated successfully",
				"severity": "info",
			},
		},
		Timeout: 10 * time.Second,
	}

	notifyCordonFailureStep := &engine.WorkflowStep{
		ID:   "notify-cordon-failure",
		Name: "Notify Cordon Failure",
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "notification",
			Parameters: map[string]interface{}{
				"action":   "notify_only",
				"message":  "Failed to cordon node - manual intervention required",
				"severity": "warning",
			},
		},
		Timeout: 10 * time.Second,
	}

	notifyMigrationFailureStep := &engine.WorkflowStep{
		ID:   "notify-migration-failure",
		Name: "Notify Migration Failure",
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "notification",
			Parameters: map[string]interface{}{
				"action":   "notify_only",
				"message":  "Workload migration failed - investigate node and workload status",
				"severity": "error",
			},
		},
		Timeout: 10 * time.Second,
	}

	notifyIncompleteStep := &engine.WorkflowStep{
		ID:   "notify-migration-incomplete",
		Name: "Notify Migration Incomplete",
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "notification",
			Parameters: map[string]interface{}{
				"action":   "notify_only",
				"message":  "Node migration completed with some workloads remaining",
				"severity": "warning",
			},
		},
		Timeout: 10 * time.Second,
	}

	template.Steps = []*engine.WorkflowStep{
		assessStep, evaluateStep, cordonStep, migrateStep, verifyStep,
		notifySuccessStep, notifyCordonFailureStep, notifyMigrationFailureStep, notifyIncompleteStep,
	}
	template.Conditions = []*engine.WorkflowCondition{
		wtf.conditionLibrary["node_health_condition"],
	}
	template.Timeouts = &engine.WorkflowTimeouts{
		Execution: 15 * time.Minute, // Longer for node operations
		Step:      60 * time.Second,
	}
	template.Recovery = &engine.RecoveryPolicy{
		Enabled: true,
		// Conservative recovery settings
		MaxRecoveryTime: 8 * time.Minute,
	}

	return template
}

// BuildStorageIssueWorkflow creates workflow for storage-related issues
func (wtf *WorkflowTemplateFactory) BuildStorageIssueWorkflow(alert types.Alert) *engine.WorkflowTemplate {
	workflowID := fmt.Sprintf("storage-issue-%s", uuid.New().String()[:8])

	template := &engine.WorkflowTemplate{
		ID:          workflowID,
		Name:        "Storage Issue Remediation",
		Description: fmt.Sprintf("Automated remediation for storage issues in %s/%s", alert.Namespace, alert.Resource),
		Version:     "1.0.0",
		Tags:        []string{"storage", "disk", "cleanup", "automated"},
		CreatedBy:   "template-factory",
		CreatedAt:   time.Now(),
	}

	// Step 1: Check disk usage
	checkDiskStep := &engine.WorkflowStep{
		ID:   "check-disk-usage",
		Name: "Check Disk Usage",
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "kubernetes",
			Parameters: map[string]interface{}{
				"action": "get_storage_metrics",
			},
			Target: &engine.ActionTarget{
				Type:      "kubernetes",
				Namespace: alert.Namespace,
				Resource:  alert.Resource,
				Name:      getResourceNameFromAlert(alert),
			},
		},
		Timeout:     30 * time.Second,
		RetryPolicy: wtf.createDefaultRetryPolicy(),
	}

	// Step 2: Evaluate storage threshold
	evaluateStep := &engine.WorkflowStep{
		ID:           "evaluate-storage-threshold",
		Name:         "Evaluate Storage Threshold",
		Type:         engine.StepTypeCondition,
		Dependencies: []string{"check-disk-usage"},
		Condition:    wtf.conditionLibrary["storage_threshold"],
		Timeout:      15 * time.Second,
	}

	// Step 3: Cleanup storage
	cleanupStep := &engine.WorkflowStep{
		ID:           "cleanup-storage",
		Name:         "Cleanup Storage",
		Type:         engine.StepTypeAction,
		Dependencies: []string{"evaluate-storage-threshold"},
		Action: &engine.StepAction{
			Type: "kubernetes",
			Parameters: map[string]interface{}{
				"action": "cleanup_storage",
				"types":  []string{"logs", "temp", "cache"},
			},
			Target: &engine.ActionTarget{
				Type:      "kubernetes",
				Namespace: alert.Namespace,
				Resource:  alert.Resource,
				Name:      getResourceNameFromAlert(alert),
			},
		},
		Timeout:     2 * time.Minute,
		RetryPolicy: wtf.createDefaultRetryPolicy(),
		OnSuccess:   []string{"verify-cleanup"},
		OnFailure:   []string{"expand-pvc"},
	}

	// Step 4: Verify cleanup
	verifyStep := &engine.WorkflowStep{
		ID:           "verify-cleanup",
		Name:         "Verify Storage Cleanup",
		Type:         engine.StepTypeAction,
		Dependencies: []string{"cleanup-storage"},
		Action: &engine.StepAction{
			Type: "kubernetes",
			Parameters: map[string]interface{}{
				"action": "verify_storage_usage",
			},
			Target: &engine.ActionTarget{
				Type:      "kubernetes",
				Namespace: alert.Namespace,
				Resource:  alert.Resource,
				Name:      getResourceNameFromAlert(alert),
			},
		},
		Timeout:   30 * time.Second,
		OnSuccess: []string{"notify-cleanup-success"},
		OnFailure: []string{"expand-pvc"},
	}

	// Step 5: Expand PVC (fallback)
	expandStep := &engine.WorkflowStep{
		ID:   "expand-pvc",
		Name: "Expand Persistent Volume",
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "kubernetes",
			Parameters: map[string]interface{}{
				"action":     "expand_pvc",
				"size_delta": "5Gi",
			},
			Target: &engine.ActionTarget{
				Type:      "kubernetes",
				Namespace: alert.Namespace,
				Resource:  "pvc",
				Name:      getResourceNameFromAlert(alert),
			},
		},
		Timeout:   3 * time.Minute,
		OnSuccess: []string{"notify-expand-success"},
		OnFailure: []string{"notify-manual-intervention"},
	}

	// Notification steps
	notifyCleanupStep := &engine.WorkflowStep{
		ID:   "notify-cleanup-success",
		Name: "Notify Cleanup Success",
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "notification",
			Parameters: map[string]interface{}{
				"action":   "notify_only",
				"message":  "Storage issue resolved via cleanup",
				"severity": "info",
			},
		},
		Timeout: 10 * time.Second,
	}

	notifyExpandStep := &engine.WorkflowStep{
		ID:   "notify-expand-success",
		Name: "Notify Expansion Success",
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "notification",
			Parameters: map[string]interface{}{
				"action":   "notify_only",
				"message":  "Storage issue resolved via PVC expansion",
				"severity": "info",
			},
		},
		Timeout: 10 * time.Second,
	}

	notifyManualStep := &engine.WorkflowStep{
		ID:   "notify-manual-intervention",
		Name: "Notify Manual Intervention Required",
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "notification",
			Parameters: map[string]interface{}{
				"action":   "notify_only",
				"message":  "Storage issue requires manual intervention - cleanup and expansion failed",
				"severity": "error",
			},
		},
		Timeout: 10 * time.Second,
	}

	template.Steps = []*engine.WorkflowStep{
		checkDiskStep, evaluateStep, cleanupStep, verifyStep, expandStep,
		notifyCleanupStep, notifyExpandStep, notifyManualStep,
	}
	template.Conditions = []*engine.WorkflowCondition{
		wtf.conditionLibrary["storage_threshold"],
	}
	template.Timeouts = &engine.WorkflowTimeouts{
		Execution: 10 * time.Minute,
		Step:      60 * time.Second,
	}
	template.Recovery = &engine.RecoveryPolicy{
		Enabled:         true,
		MaxRecoveryTime: wtf.config.DefaultTimeout / 2,
	}

	return template
}

// BuildNetworkIssueWorkflow creates workflow for network connectivity issues
func (wtf *WorkflowTemplateFactory) BuildNetworkIssueWorkflow(alert types.Alert) *engine.WorkflowTemplate {
	workflowID := fmt.Sprintf("network-issue-%s", uuid.New().String()[:8])

	template := &engine.WorkflowTemplate{
		ID:          workflowID,
		Name:        "Network Issue Remediation",
		Description: fmt.Sprintf("Automated remediation for network issues in %s/%s", alert.Namespace, alert.Resource),
		Version:     "1.0.0",
		Tags:        []string{"network", "connectivity", "dns", "automated"},
		CreatedBy:   "template-factory",
		CreatedAt:   time.Now(),
	}

	// Step 1: Test network connectivity
	testConnectivityStep := &engine.WorkflowStep{
		ID:   "test-network-connectivity",
		Name: "Test Network Connectivity",
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "kubernetes",
			Parameters: map[string]interface{}{
				"action":  "test_connectivity",
				"targets": []string{"dns", "service", "external"},
			},
			Target: &engine.ActionTarget{
				Type:      "kubernetes",
				Namespace: alert.Namespace,
				Resource:  alert.Resource,
				Name:      getResourceNameFromAlert(alert),
			},
		},
		Timeout:     45 * time.Second,
		RetryPolicy: wtf.createDefaultRetryPolicy(),
	}

	// Step 2: Evaluate network condition
	evaluateStep := &engine.WorkflowStep{
		ID:           "evaluate-network-condition",
		Name:         "Evaluate Network Condition",
		Type:         engine.StepTypeCondition,
		Dependencies: []string{"test-network-connectivity"},
		Condition:    wtf.conditionLibrary["network_connectivity"],
		Timeout:      15 * time.Second,
	}

	// Step 3: Restart network components
	restartNetworkStep := &engine.WorkflowStep{
		ID:           "restart-network-components",
		Name:         "Restart Network Components",
		Type:         engine.StepTypeAction,
		Dependencies: []string{"evaluate-network-condition"},
		Action: &engine.StepAction{
			Type: "kubernetes",
			Parameters: map[string]interface{}{
				"action":     "restart_network",
				"components": []string{"dns", "cni"},
			},
			Target: &engine.ActionTarget{
				Type:      "kubernetes",
				Namespace: alert.Namespace,
				Resource:  "daemonset",
			},
		},
		Timeout:   2 * time.Minute,
		OnSuccess: []string{"verify-network-fix"},
		OnFailure: []string{"update-network-policy"},
	}

	// Step 4: Verify network fix
	verifyStep := &engine.WorkflowStep{
		ID:           "verify-network-fix",
		Name:         "Verify Network Fix",
		Type:         engine.StepTypeAction,
		Dependencies: []string{"restart-network-components"},
		Action: &engine.StepAction{
			Type: "kubernetes",
			Parameters: map[string]interface{}{
				"action":  "verify_connectivity",
				"timeout": "60s",
			},
			Target: &engine.ActionTarget{
				Type:      "kubernetes",
				Namespace: alert.Namespace,
				Resource:  alert.Resource,
				Name:      getResourceNameFromAlert(alert),
			},
		},
		Timeout:   60 * time.Second,
		OnSuccess: []string{"notify-network-success"},
		OnFailure: []string{"update-network-policy"},
	}

	// Step 5: Update network policy (fallback)
	updatePolicyStep := &engine.WorkflowStep{
		ID:   "update-network-policy",
		Name: "Update Network Policy",
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "kubernetes",
			Parameters: map[string]interface{}{
				"action": "update_network_policy",
				"policy": "allow_temporary",
			},
			Target: &engine.ActionTarget{
				Type:      "kubernetes",
				Namespace: alert.Namespace,
				Resource:  "networkpolicy",
			},
		},
		Timeout:   30 * time.Second,
		OnSuccess: []string{"notify-policy-update"},
		OnFailure: []string{"notify-manual-intervention"},
	}

	// Notification steps
	notifyNetworkSuccessStep := &engine.WorkflowStep{
		ID:   "notify-network-success",
		Name: "Notify Network Success",
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "notification",
			Parameters: map[string]interface{}{
				"action":   "notify_only",
				"message":  "Network issue resolved via component restart",
				"severity": "info",
			},
		},
		Timeout: 10 * time.Second,
	}

	notifyPolicyUpdateStep := &engine.WorkflowStep{
		ID:   "notify-policy-update",
		Name: "Notify Policy Update",
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "notification",
			Parameters: map[string]interface{}{
				"action":   "notify_only",
				"message":  "Network issue resolved via policy update",
				"severity": "warning",
			},
		},
		Timeout: 10 * time.Second,
	}

	notifyManualStep := &engine.WorkflowStep{
		ID:   "notify-manual-intervention",
		Name: "Notify Manual Intervention Required",
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "notification",
			Parameters: map[string]interface{}{
				"action":   "notify_only",
				"message":  "Network issue requires manual intervention - automated fixes failed",
				"severity": "error",
			},
		},
		Timeout: 10 * time.Second,
	}

	template.Steps = []*engine.WorkflowStep{
		testConnectivityStep, evaluateStep, restartNetworkStep, verifyStep, updatePolicyStep,
		notifyNetworkSuccessStep, notifyPolicyUpdateStep, notifyManualStep,
	}
	template.Conditions = []*engine.WorkflowCondition{
		wtf.conditionLibrary["network_connectivity"],
	}
	template.Timeouts = &engine.WorkflowTimeouts{
		Execution: 8 * time.Minute,
		Step:      60 * time.Second,
	}
	template.Recovery = &engine.RecoveryPolicy{
		Enabled: true,
		// Conservative recovery settings
		MaxRecoveryTime: 4 * time.Minute,
	}

	return template
}

// BuildFromAlert creates workflow template dynamically from alert context
func (wtf *WorkflowTemplateFactory) BuildFromAlert(ctx context.Context, alert types.Alert) *engine.WorkflowTemplate {
	wtf.log.WithFields(logrus.Fields{
		"alert_name": alert.Name,
		"namespace":  alert.Namespace,
		"severity":   alert.Severity,
		"resource":   alert.Resource,
	}).Info("Building workflow template from alert")

	// Analyze alert to determine appropriate template
	templateType := wtf.determineTemplateType(alert)

	switch templateType {
	case "high_memory":
		return wtf.BuildHighMemoryWorkflow(alert)
	case "crash_loop":
		return wtf.BuildPodCrashLoopWorkflow(alert)
	case "node_issue":
		return wtf.BuildNodeIssueWorkflow(alert)
	case "storage_issue":
		return wtf.BuildStorageIssueWorkflow(alert)
	case "network_issue":
		return wtf.BuildNetworkIssueWorkflow(alert)
	default:
		return wtf.buildGenericWorkflow(alert)
	}
}

// BuildFromObjective creates workflow template from high-level objective
func (wtf *WorkflowTemplateFactory) BuildFromObjective(ctx context.Context, objective *engine.WorkflowObjective) *engine.WorkflowTemplate {
	wtf.log.WithFields(logrus.Fields{
		"objective_id":   objective.ID,
		"objective_type": objective.Type,
	}).Info("Building workflow template from objective")

	workflowID := fmt.Sprintf("objective-%s", uuid.New().String()[:8])

	template := &engine.WorkflowTemplate{
		ID:          workflowID,
		Name:        objective.Description,
		Description: fmt.Sprintf("Workflow for objective: %s", objective.Description),
		Version:     "1.0.0",
		Tags:        []string{"objective-driven", "automated"},
		CreatedBy:   "template-factory",
		CreatedAt:   time.Now(),
		Variables:   objective.Constraints,
	}

	// Build steps based on objective type and targets
	steps := wtf.buildStepsFromObjective(objective)
	template.Steps = steps

	// Set appropriate timeouts based on objective priority
	template.Timeouts = wtf.createTimeoutsFromObjective(objective)
	template.Recovery = wtf.createRecoveryFromObjective(objective)

	return template
}

// BuildFromPattern creates workflow template from existing pattern
func (wtf *WorkflowTemplateFactory) BuildFromPattern(ctx context.Context, pattern *engine.WorkflowPattern, context *engine.WorkflowContext) *engine.WorkflowTemplate {
	wtf.log.WithFields(logrus.Fields{
		"pattern_id":  pattern.ID,
		"workflow_id": context.WorkflowID,
		"environment": context.Environment,
	}).Info("Building workflow template from pattern")

	workflowID := fmt.Sprintf("pattern-%s", uuid.New().String()[:8])

	template := &engine.WorkflowTemplate{
		ID:          workflowID,
		Name:        pattern.Name,
		Description: fmt.Sprintf("Workflow based on pattern: %s", pattern.Name),
		Version:     "1.0.0",
		Tags:        []string{"pattern-based", pattern.Type},
		CreatedBy:   "template-factory",
		CreatedAt:   time.Now(),
	}

	// Adapt pattern steps to current context
	adaptedSteps := wtf.adaptPatternSteps(pattern.Steps, context)
	template.Steps = adaptedSteps

	// Convert ActionConditions to WorkflowConditions
	var workflowConditions []*engine.WorkflowCondition
	for _, actionCond := range pattern.Conditions {
		workflowCond := &engine.WorkflowCondition{
			ID:         actionCond.ID,
			Name:       fmt.Sprintf("Condition %s", actionCond.ID),
			Type:       engine.ConditionType(actionCond.Type),
			Expression: actionCond.Expression,
			Timeout:    15 * time.Second,
		}
		workflowConditions = append(workflowConditions, workflowCond)
	}
	template.Conditions = workflowConditions

	// Apply context-specific modifications
	template.Timeouts = wtf.adaptTimeouts(nil, context)
	template.Recovery = wtf.adaptRecovery(nil, context)

	return template
}

// ComposeWorkflows combines multiple templates into one
func (wtf *WorkflowTemplateFactory) ComposeWorkflows(templates ...*engine.WorkflowTemplate) *engine.WorkflowTemplate {
	if len(templates) == 0 {
		return nil
	}

	if len(templates) == 1 {
		return templates[0]
	}

	compositeID := fmt.Sprintf("composite-%s", uuid.New().String()[:8])
	composite := &engine.WorkflowTemplate{
		ID:          compositeID,
		Name:        "Composite Workflow",
		Description: "Combined workflow from multiple templates",
		Version:     "1.0.0",
		Tags:        []string{"composite", "multi-template"},
		CreatedBy:   "template-factory",
		CreatedAt:   time.Now(),
	}

	// Combine steps from all templates
	var allSteps []*engine.WorkflowStep
	var allConditions []*engine.WorkflowCondition
	var maxTimeout time.Duration

	for i, template := range templates {
		// Prefix step IDs to avoid conflicts
		prefix := fmt.Sprintf("t%d-", i)

		for _, step := range template.Steps {
			adaptedStep := wtf.adaptStepForComposition(step, prefix)
			allSteps = append(allSteps, adaptedStep)
		}

		allConditions = append(allConditions, template.Conditions...)

		if template.Timeouts != nil && template.Timeouts.Execution > maxTimeout {
			maxTimeout = template.Timeouts.Execution
		}
	}

	composite.Steps = allSteps
	composite.Conditions = wtf.deduplicateConditions(allConditions)
	composite.Timeouts = &engine.WorkflowTimeouts{
		Execution: maxTimeout + time.Minute, // Add buffer
		Step:      60 * time.Second,
	}
	composite.Recovery = &engine.RecoveryPolicy{
		Enabled:         true,
		MaxRecoveryTime: wtf.config.DefaultTimeout / 3,
	}

	return composite
}

// CustomizeForEnvironment customizes template for specific environment
func (wtf *WorkflowTemplateFactory) CustomizeForEnvironment(template *engine.WorkflowTemplate, env string) *engine.WorkflowTemplate {
	customized := wtf.deepCopyTemplate(template)
	customized.ID = fmt.Sprintf("%s-%s", template.ID, env)
	customized.Tags = append(customized.Tags, fmt.Sprintf("env-%s", env))

	// Apply environment-specific customizations
	switch strings.ToLower(env) {
	case "production":
		customized = wtf.applyProductionCustomizations(customized)
	case "staging":
		customized = wtf.applyStagingCustomizations(customized)
	case "development":
		customized = wtf.applyDevelopmentCustomizations(customized)
	}

	return customized
}

// AddSafetyConstraints adds safety constraints to template
func (wtf *WorkflowTemplateFactory) AddSafetyConstraints(template *engine.WorkflowTemplate, constraints *SafetyConstraints) *engine.WorkflowTemplate {
	constrained := wtf.deepCopyTemplate(template)

	// Apply safety constraints to each step
	for _, step := range constrained.Steps {
		if step.Action != nil {
			wtf.applySafetyConstraintsToStep(step, constraints)
		}
	}

	// Adjust timeouts based on safety level
	if constraints.CooldownPeriod > 0 {
		constrained.Recovery.MaxRecoveryTime = constraints.CooldownPeriod * 2
	}

	// Add safety metadata
	if constrained.Variables == nil {
		constrained.Variables = make(map[string]interface{})
	}
	constrained.Variables["safety_constraints"] = constraints
	constrained.Tags = append(constrained.Tags, "safety-constrained")

	return constrained
}

// Helper methods

func (wtf *WorkflowTemplateFactory) initializePredefinedTemplates() {
	// Pre-define common alert patterns
	wtf.alertPatterns["HighMemoryUsage"] = nil // Will be created on demand
	wtf.alertPatterns["PodCrashLoop"] = nil
	wtf.alertPatterns["NodeNotReady"] = nil
	wtf.alertPatterns["DiskSpaceCritical"] = nil
	wtf.alertPatterns["NetworkConnectivityIssue"] = nil
}

func (wtf *WorkflowTemplateFactory) initializeConditionLibrary() {
	// Memory threshold condition
	wtf.conditionLibrary["memory_threshold"] = &engine.WorkflowCondition{
		ID:         "memory-threshold",
		Name:       "Memory Usage Threshold",
		Type:       "metric",
		Expression: "memory_usage > 0.85",
		Timeout:    15 * time.Second,
	}

	// Crash pattern analysis condition
	wtf.conditionLibrary["crash_pattern_analysis"] = &engine.WorkflowCondition{
		ID:         "crash-pattern",
		Name:       "Crash Pattern Analysis",
		Type:       "custom",
		Expression: "crash_count > 3 AND crash_interval < 300s",
		Timeout:    20 * time.Second,
	}

	// Node health condition
	wtf.conditionLibrary["node_health_condition"] = &engine.WorkflowCondition{
		ID:         "node-health",
		Name:       "Node Health Check",
		Type:       "resource",
		Expression: "node_ready == false OR disk_pressure == true",
		Timeout:    15 * time.Second,
	}

	// Storage threshold condition
	wtf.conditionLibrary["storage_threshold"] = &engine.WorkflowCondition{
		ID:         "storage-threshold",
		Name:       "Storage Usage Threshold",
		Type:       "metric",
		Expression: "disk_usage > 0.80",
		Timeout:    15 * time.Second,
	}

	// Network connectivity condition
	wtf.conditionLibrary["network_connectivity"] = &engine.WorkflowCondition{
		ID:         "network-connectivity",
		Name:       "Network Connectivity Check",
		Type:       "custom",
		Expression: "dns_resolution == false OR service_unreachable == true",
		Timeout:    15 * time.Second,
	}
}

func (wtf *WorkflowTemplateFactory) initializeActionSequences() {
	// Common action sequences for reuse
	wtf.actionSequences["scale_and_verify"] = []*engine.WorkflowStep{
		{
			ID:   "scale",
			Name: "Scale Resource",
			Type: engine.StepTypeAction,
		},
		{
			ID:           "verify",
			Name:         "Verify Scaling",
			Type:         engine.StepTypeAction,
			Dependencies: []string{"scale"},
		},
	}

	wtf.actionSequences["diagnose_and_restart"] = []*engine.WorkflowStep{
		{
			ID:   "diagnose",
			Name: "Collect Diagnostics",
			Type: engine.StepTypeAction,
		},
		{
			ID:           "restart",
			Name:         "Restart Component",
			Type:         engine.StepTypeAction,
			Dependencies: []string{"diagnose"},
		},
	}
}

func (wtf *WorkflowTemplateFactory) createDefaultRetryPolicy() *engine.RetryPolicy {
	return &engine.RetryPolicy{
		MaxRetries:  wtf.config.DefaultRetries,
		Delay:       wtf.config.RetryBackoffBase,
		Backoff:     "exponential",
		BackoffRate: 2.0,
	}
}

func (wtf *WorkflowTemplateFactory) determineTemplateType(alert types.Alert) string {
	alertName := strings.ToLower(alert.Name)
	description := strings.ToLower(alert.Description)

	// Pattern matching for alert types
	if strings.Contains(alertName, "memory") || strings.Contains(description, "memory") {
		return "high_memory"
	}
	if strings.Contains(alertName, "crash") || strings.Contains(description, "crash") {
		return "crash_loop"
	}
	if strings.Contains(alertName, "node") || alert.Resource == "node" {
		return "node_issue"
	}
	if strings.Contains(alertName, "disk") || strings.Contains(alertName, "storage") {
		return "storage_issue"
	}
	if strings.Contains(alertName, "network") || strings.Contains(description, "connectivity") {
		return "network_issue"
	}

	return "generic"
}

func (wtf *WorkflowTemplateFactory) buildGenericWorkflow(alert types.Alert) *engine.WorkflowTemplate {
	workflowID := fmt.Sprintf("generic-%s", uuid.New().String()[:8])

	template := &engine.WorkflowTemplate{
		ID:          workflowID,
		Name:        "Generic Alert Remediation",
		Description: fmt.Sprintf("Generic workflow for %s in %s/%s", alert.Name, alert.Namespace, alert.Resource),
		Version:     "1.0.0",
		Tags:        []string{"generic", "automated", "fallback"},
		CreatedBy:   "template-factory",
		CreatedAt:   time.Now(),
	}

	// Generic workflow: diagnose -> notify
	diagnoseStep := &engine.WorkflowStep{
		ID:   "diagnose-issue",
		Name: "Diagnose Issue",
		Type: engine.StepTypeAction,
		Action: &engine.StepAction{
			Type: "kubernetes",
			Parameters: map[string]interface{}{
				"action": "collect_diagnostics",
			},
			Target: &engine.ActionTarget{
				Type:      "kubernetes",
				Namespace: alert.Namespace,
				Resource:  alert.Resource,
				Name:      getResourceNameFromAlert(alert),
			},
		},
		Timeout: 30 * time.Second,
	}

	notifyStep := &engine.WorkflowStep{
		ID:           "notify-investigation",
		Name:         "Notify Investigation Required",
		Type:         engine.StepTypeAction,
		Dependencies: []string{"diagnose-issue"},
		Action: &engine.StepAction{
			Type: "notification",
			Parameters: map[string]interface{}{
				"action":   "notify_only",
				"message":  fmt.Sprintf("Alert %s requires investigation - generic workflow applied", alert.Name),
				"severity": "info",
			},
		},
		Timeout: 10 * time.Second,
	}

	template.Steps = []*engine.WorkflowStep{diagnoseStep, notifyStep}
	template.Timeouts = &engine.WorkflowTimeouts{
		Execution: 5 * time.Minute,
		Step:      30 * time.Second,
	}
	template.Recovery = &engine.RecoveryPolicy{
		Enabled:         true,
		MaxRecoveryTime: 4 * time.Minute,
	}

	return template
}

// Utility functions

func getResourceNameFromAlert(alert types.Alert) string {
	if name, exists := alert.Labels["pod"]; exists {
		return name
	}
	if name, exists := alert.Labels["deployment"]; exists {
		return name
	}
	if name, exists := alert.Labels["node"]; exists {
		return name
	}
	if name, exists := alert.Labels["instance"]; exists {
		return name
	}

	// Fallback to resource name from alert
	return alert.Resource
}

func (wtf *WorkflowTemplateFactory) buildStepsFromObjective(objective *engine.WorkflowObjective) []*engine.WorkflowStep {
	var steps []*engine.WorkflowStep

	// Simple implementation - expand based on requirements
	for i, target := range objective.Targets {
		step := &engine.WorkflowStep{
			ID:   fmt.Sprintf("objective-step-%d", i),
			Name: fmt.Sprintf("Execute %s on %s", target.Metric, target.Type),
			Type: engine.StepTypeAction,
			Action: &engine.StepAction{
				Type:       target.Type,
				Parameters: target.Parameters,
			},
			Timeout: wtf.config.DefaultTimeout / time.Duration(len(objective.Targets)),
		}
		steps = append(steps, step)
	}

	return steps
}

func (wtf *WorkflowTemplateFactory) createTimeoutsFromObjective(objective *engine.WorkflowObjective) *engine.WorkflowTimeouts {
	baseDuration := wtf.config.DefaultTimeout

	// Adjust based on priority (higher priority = more time)
	priorityMultiplier := 1.0 + (float64(objective.Priority) / 10.0)
	adjustedDuration := time.Duration(float64(baseDuration) * priorityMultiplier)

	return &engine.WorkflowTimeouts{
		Execution: adjustedDuration,
		Step:      adjustedDuration / time.Duration(len(objective.Targets)+1),
	}
}

func (wtf *WorkflowTemplateFactory) createRecoveryFromObjective(objective *engine.WorkflowObjective) *engine.RecoveryPolicy {
	return &engine.RecoveryPolicy{
		Enabled:         true,
		MaxRecoveryTime: wtf.config.DefaultTimeout / 3,
	}
}

func (wtf *WorkflowTemplateFactory) adaptPatternSteps(steps []*engine.WorkflowStep, context *engine.WorkflowContext) []*engine.WorkflowStep {
	adapted := make([]*engine.WorkflowStep, len(steps))

	for i, step := range steps {
		adaptedStep := wtf.deepCopyStep(step)

		// Apply context-specific modifications
		if context.Environment != "" {
			adaptedStep.Variables = wtf.mergeVariables(adaptedStep.Variables, context.Variables)
		}

		adapted[i] = adaptedStep
	}

	return adapted
}

func (wtf *WorkflowTemplateFactory) adaptTimeouts(timeouts *engine.WorkflowTimeouts, context *engine.WorkflowContext) *engine.WorkflowTimeouts {
	if timeouts == nil {
		return &engine.WorkflowTimeouts{
			Execution: wtf.config.DefaultTimeout,
			Step:      60 * time.Second,
		}
	}

	adapted := &engine.WorkflowTimeouts{
		Execution: timeouts.Execution,
		Step:      timeouts.Step,
		Condition: timeouts.Condition,
		Recovery:  timeouts.Recovery,
	}

	// Adjust for environment
	if context.Environment == "production" {
		adapted.Execution = adapted.Execution * 2 // More conservative in production
	}

	return adapted
}

func (wtf *WorkflowTemplateFactory) adaptRecovery(recovery *engine.RecoveryPolicy, context *engine.WorkflowContext) *engine.RecoveryPolicy {
	if recovery == nil {
		return &engine.RecoveryPolicy{
			Enabled:         true,
			MaxRecoveryTime: wtf.config.DefaultTimeout / 2,
		}
	}

	adapted := &engine.RecoveryPolicy{
		Enabled:         recovery.Enabled,
		MaxRecoveryTime: recovery.MaxRecoveryTime,
	}

	// Adjust for environment
	if context.Environment == "production" {
		adapted.MaxRecoveryTime = adapted.MaxRecoveryTime * 2 // More conservative in production
	}

	return adapted
}

func (wtf *WorkflowTemplateFactory) adaptStepForComposition(step *engine.WorkflowStep, prefix string) *engine.WorkflowStep {
	adapted := wtf.deepCopyStep(step)
	adapted.ID = prefix + adapted.ID

	// Update dependencies with prefix
	for i, dep := range adapted.Dependencies {
		adapted.Dependencies[i] = prefix + dep
	}

	// Update success/failure references
	for i, ref := range adapted.OnSuccess {
		adapted.OnSuccess[i] = prefix + ref
	}
	for i, ref := range adapted.OnFailure {
		adapted.OnFailure[i] = prefix + ref
	}

	return adapted
}

func (wtf *WorkflowTemplateFactory) deduplicateConditions(conditions []*engine.WorkflowCondition) []*engine.WorkflowCondition {
	seen := make(map[string]*engine.WorkflowCondition)
	var result []*engine.WorkflowCondition

	for _, condition := range conditions {
		if _, exists := seen[condition.ID]; !exists {
			seen[condition.ID] = condition
			result = append(result, condition)
		}
	}

	return result
}

func (wtf *WorkflowTemplateFactory) deepCopyTemplate(template *engine.WorkflowTemplate) *engine.WorkflowTemplate {
	// Simple deep copy implementation
	// In production, consider using a more robust deep copy library
	copied := *template

	// Copy steps
	if template.Steps != nil {
		copied.Steps = make([]*engine.WorkflowStep, len(template.Steps))
		for i, step := range template.Steps {
			copied.Steps[i] = wtf.deepCopyStep(step)
		}
	}

	// Copy conditions
	if template.Conditions != nil {
		copied.Conditions = make([]*engine.WorkflowCondition, len(template.Conditions))
		copy(copied.Conditions, template.Conditions)
	}

	// Copy variables
	if template.Variables != nil {
		copied.Variables = make(map[string]interface{})
		for k, v := range template.Variables {
			copied.Variables[k] = v
		}
	}

	// Copy tags
	if template.Tags != nil {
		copied.Tags = make([]string, len(template.Tags))
		copy(copied.Tags, template.Tags)
	}

	return &copied
}

func (wtf *WorkflowTemplateFactory) deepCopyStep(step *engine.WorkflowStep) *engine.WorkflowStep {
	copied := *step

	// Copy dependencies
	if step.Dependencies != nil {
		copied.Dependencies = make([]string, len(step.Dependencies))
		copy(copied.Dependencies, step.Dependencies)
	}

	// Copy success/failure references
	if step.OnSuccess != nil {
		copied.OnSuccess = make([]string, len(step.OnSuccess))
		copy(copied.OnSuccess, step.OnSuccess)
	}
	if step.OnFailure != nil {
		copied.OnFailure = make([]string, len(step.OnFailure))
		copy(copied.OnFailure, step.OnFailure)
	}

	// Copy variables
	if step.Variables != nil {
		copied.Variables = make(map[string]interface{})
		for k, v := range step.Variables {
			copied.Variables[k] = v
		}
	}

	// Copy metadata
	if step.Metadata != nil {
		copied.Metadata = make(map[string]interface{})
		for k, v := range step.Metadata {
			copied.Metadata[k] = v
		}
	}

	return &copied
}

func (wtf *WorkflowTemplateFactory) applyProductionCustomizations(template *engine.WorkflowTemplate) *engine.WorkflowTemplate {
	// Production: more conservative settings
	for _, step := range template.Steps {
		if step.RetryPolicy != nil {
			step.RetryPolicy.MaxRetries = step.RetryPolicy.MaxRetries / 2
			step.RetryPolicy.Delay = step.RetryPolicy.Delay * 2
		}
		step.Timeout = step.Timeout * 2 // Longer timeouts in production
	}

	return template
}

func (wtf *WorkflowTemplateFactory) applyStagingCustomizations(template *engine.WorkflowTemplate) *engine.WorkflowTemplate {
	// Staging: balanced settings
	return template
}

func (wtf *WorkflowTemplateFactory) applyDevelopmentCustomizations(template *engine.WorkflowTemplate) *engine.WorkflowTemplate {
	// Development: faster iterations, more aggressive
	for _, step := range template.Steps {
		step.Timeout = step.Timeout / 2 // Shorter timeouts in development
	}

	return template
}

func (wtf *WorkflowTemplateFactory) applySafetyConstraintsToStep(step *engine.WorkflowStep, constraints *SafetyConstraints) {
	if step.Action == nil {
		return
	}

	// Add safety parameters
	if step.Action.Parameters == nil {
		step.Action.Parameters = make(map[string]interface{})
	}

	step.Action.Parameters["dry_run"] = constraints.RequireConfirmation
	step.Action.Parameters["safety_level"] = "high"

	if !constraints.DisruptiveOperations {
		step.Action.Parameters["disruptive"] = false
	}

	// Extend timeout for safety checks
	step.Timeout = step.Timeout + (30 * time.Second)
}

func (wtf *WorkflowTemplateFactory) mergeVariables(base, additional map[string]interface{}) map[string]interface{} {
	if base == nil {
		base = make(map[string]interface{})
	}

	for k, v := range additional {
		base[k] = v
	}

	return base
}
