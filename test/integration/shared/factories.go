//go:build integration
// +build integration

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
package shared

import (
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/pkg/intelligence/patterns"
	"github.com/jordigilh/kubernaut/pkg/intelligence/shared"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// =============================================================================
// ALERT FACTORY FUNCTIONS - Consolidated from multiple test files
// =============================================================================

// CreateStandardAlert creates a basic alert for testing with standard fields
func CreateStandardAlert(name, description, severity, namespace, resource string) *types.Alert {
	return &types.Alert{
		Name:        name,
		Description: description,
		Severity:    severity,
		Namespace:   namespace,
		Resource:    resource,
		Labels: map[string]string{
			"environment": namespace,
			"test":        "true",
			"created_by":  "test_factory",
		},
		StartsAt:  time.Now().Add(-5 * time.Minute),
		EndsAt:    nil, // Empty for active alert
		UpdatedAt: time.Now(),
	}
}

// CreateDatabaseAlert creates a database-related alert for testing
func CreateDatabaseAlert() *types.Alert {
	return CreateStandardAlert(
		"DatabaseHighCPU",
		"Database CPU usage at 95%",
		"critical",
		"production",
		"postgres-primary",
	)
}

// CreateMemoryAlert creates a memory-related alert for testing
func CreateMemoryAlert() *types.Alert {
	alert := CreateStandardAlert(
		"HighMemoryUsage",
		"Pod memory usage at 90%",
		"warning",
		"production",
		"app-server",
	)
	alert.Labels["resource_type"] = "memory"
	return alert
}

// CreateOOMAlert creates an out-of-memory alert for testing
func CreateOOMAlert() *types.Alert {
	alert := CreateStandardAlert(
		"OOMKilled",
		"Pod was killed due to out-of-memory condition",
		"critical",
		"production",
		"app-worker",
	)
	alert.Labels["reason"] = "OOMKilled"
	return alert
}

// CreatePerformanceAlert creates a performance-related alert for testing
func CreatePerformanceAlert() *types.Alert {
	return CreateStandardAlert(
		"PerformanceDegradation",
		"System performance degrading over time",
		"warning",
		"production",
		"web-frontend",
	)
}

// CreateSecurityAlert creates a security-related alert for testing
func CreateSecurityAlert() *types.Alert {
	alert := CreateStandardAlert(
		"SecurityThreat",
		"Potential security threat detected",
		"critical",
		"production",
		"auth-service",
	)
	alert.Labels["threat_type"] = "malware"
	alert.Labels["severity"] = "high"
	return alert
}

// CreateNetworkAlert creates a network-related alert for testing
func CreateNetworkAlert() *types.Alert {
	return CreateStandardAlert(
		"NetworkConnectivityIssue",
		"Network connectivity problems detected",
		"warning",
		"production",
		"load-balancer",
	)
}

// CreateStorageAlert creates a storage-related alert for testing
func CreateStorageAlert() *types.Alert {
	alert := CreateStandardAlert(
		"PVCNearFull",
		"Persistent volume claim is 95% full",
		"warning",
		"production",
		"database-pvc",
	)
	alert.Labels["storage_type"] = "persistent_volume"
	return alert
}

// CreateCascadingAlerts creates a set of related alerts for testing correlation
func CreateCascadingAlerts() []*types.Alert {
	baseTime := time.Now().Add(-10 * time.Minute)

	return []*types.Alert{
		{
			Name:        "DatabaseConnectionPoolExhausted",
			Description: "Database connection pool at 98% capacity",
			Severity:    "warning",
			Namespace:   "production",
			Resource:    "postgres-service",
			Labels: map[string]string{
				"component":    "database",
				"test":         "true",
				"cascade_root": "true",
			},
			StartsAt: baseTime,
		},
		{
			Name:        "ApplicationResponseTime",
			Description: "API response time increased to 5s",
			Severity:    "warning",
			Namespace:   "production",
			Resource:    "api-gateway",
			Labels: map[string]string{
				"component":      "application",
				"test":           "true",
				"cascade_effect": "true",
			},
			StartsAt: baseTime.Add(2 * time.Minute),
		},
		{
			Name:        "UserSessionTimeout",
			Description: "Users experiencing session timeouts",
			Severity:    "critical",
			Namespace:   "production",
			Resource:    "user-service",
			Labels: map[string]string{
				"component":      "frontend",
				"test":           "true",
				"cascade_effect": "true",
			},
			StartsAt: baseTime.Add(5 * time.Minute),
		},
	}
}

// CreateTestAlertsForScenario creates alerts for specific test scenarios
func CreateTestAlertsForScenario(scenario string) []*types.Alert {
	switch scenario {
	case "memory_leak":
		return []*types.Alert{
			CreateMemoryAlert(),
			CreateOOMAlert(),
		}
	case "security_breach":
		return []*types.Alert{
			CreateSecurityAlert(),
		}
	case "performance_degradation":
		return []*types.Alert{
			CreatePerformanceAlert(),
			CreateDatabaseAlert(),
		}
	case "storage_exhaustion":
		return []*types.Alert{
			CreateStorageAlert(),
		}
	case "network_issues":
		return []*types.Alert{
			CreateNetworkAlert(),
		}
	case "cascading_failure":
		return CreateCascadingAlerts()
	default:
		return []*types.Alert{
			CreateStandardAlert("GenericAlert", "Generic test alert", "info", "test", "test-resource"),
		}
	}
}

// =============================================================================
// ACTION RECOMMENDATION FACTORY FUNCTIONS
// =============================================================================

// CreateStandardRecommendation creates a standard action recommendation for testing
func CreateStandardRecommendation(action string, confidence float64, reasoning string) *types.ActionRecommendation {
	return &types.ActionRecommendation{
		Action:     action,
		Confidence: confidence,
		Parameters: map[string]interface{}{
			"created_by":   "test_factory",
			"test_context": true,
		},
		Reasoning: &types.ReasoningDetails{
			Summary: reasoning,
		},
	}
}

// CreateHighConfidenceRecommendation creates a high-confidence recommendation
func CreateHighConfidenceRecommendation(action string) *types.ActionRecommendation {
	return CreateStandardRecommendation(
		action,
		0.9,
		fmt.Sprintf("High confidence recommendation to %s based on clear patterns", action),
	)
}

// CreateLowConfidenceRecommendation creates a low-confidence recommendation
func CreateLowConfidenceRecommendation(action string) *types.ActionRecommendation {
	return CreateStandardRecommendation(
		action,
		0.4,
		fmt.Sprintf("Low confidence recommendation to %s - requires manual review", action),
	)
}

// =============================================================================
// WORKFLOW FACTORY FUNCTIONS
// =============================================================================

// CreateStandardWorkflowObjective creates a standard workflow objective for testing
func CreateStandardWorkflowObjective(alert *types.Alert, recommendation *types.ActionRecommendation, objType string) *engine.WorkflowObjective {
	return &engine.WorkflowObjective{
		ID:          fmt.Sprintf("%s-%d", objType, time.Now().UnixNano()),
		Type:        objType,
		Description: fmt.Sprintf("Resolve %s", alert.Name),
		Priority:    5,
		Targets: []*engine.OptimizationTarget{
			{
				Type:     "kubernetes",
				Metric:   "resolution",
				Priority: 1,
				Parameters: map[string]interface{}{
					"namespace":     alert.Namespace,
					"resource_type": alert.Labels["resource_type"],
					"resource":      alert.Resource,
					"action":        recommendation.Action,
				},
			},
		},
		Constraints: map[string]interface{}{
			"max_duration": "10m",
			"safety_level": "high",
		},
	}
}

// CreateSimpleWorkflow creates a simple workflow for testing
func CreateSimpleWorkflow(name string, steps []string) *engine.Workflow {
	var workflowSteps []*engine.ExecutableWorkflowStep

	for i, stepName := range steps {
		workflowSteps = append(workflowSteps, &engine.ExecutableWorkflowStep{
			BaseEntity: types.BaseEntity{
				ID:   fmt.Sprintf("step_%d", i),
				Name: stepName,
			},
			Type: "action",
		})
	}

	return &engine.Workflow{
		BaseVersionedEntity: types.BaseVersionedEntity{
			BaseEntity: types.BaseEntity{
				ID:          fmt.Sprintf("test_workflow_%d", time.Now().UnixNano()),
				Name:        name,
				Description: fmt.Sprintf("Test workflow: %s", name),
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
		},
		Template: &engine.ExecutableTemplate{
			Steps: workflowSteps,
		},
	}
}

// =============================================================================
// PATTERN FACTORY FUNCTIONS
// =============================================================================

// CreateMockPatternResult creates a mock pattern analysis result for testing
func CreateMockPatternResult() *patterns.PatternAnalysisResult {
	return &patterns.PatternAnalysisResult{
		Patterns: []*shared.DiscoveredPattern{
			{
				BasePattern: types.BasePattern{
					BaseEntity: types.BaseEntity{
						ID:          "test-pattern-1",
						Description: "Standard resolution pattern for database issues",
					},
					Confidence: 0.8,
					Frequency:  10,
					LastSeen:   time.Now().Add(-time.Hour),
				},
			},
			{
				BasePattern: types.BasePattern{
					BaseEntity: types.BaseEntity{
						ID:          "test-pattern-2",
						Description: "Preventive pattern for memory issues",
					},
					Confidence: 0.7,
					Frequency:  5,
					LastSeen:   time.Now().Add(-2 * time.Hour),
				},
			},
		},
	}
}

// CreateDiscoveredPattern creates a single discovered pattern for testing
func CreateDiscoveredPattern(id, patternType, description string) *shared.DiscoveredPattern {
	return &shared.DiscoveredPattern{
		BasePattern: types.BasePattern{
			BaseEntity: types.BaseEntity{
				ID:          id,
				Description: description,
			},
			Confidence: 0.8,
			Frequency:  5,
			LastSeen:   time.Now().Add(-time.Hour),
		},
	}
}

// =============================================================================
// EXECUTION DATA FACTORY FUNCTIONS
// =============================================================================

// CreateWorkflowExecutionData creates workflow execution data for testing
func CreateWorkflowExecutionData(workflowID string, success bool, duration time.Duration) *engine.EngineWorkflowExecutionData {
	return &engine.EngineWorkflowExecutionData{
		WorkflowID:  workflowID,
		ExecutionID: fmt.Sprintf("exec_%d", time.Now().UnixNano()),
		Duration:    duration,
		Success:     success,
	}
}

// CreateBatchExecutionData creates multiple execution data records for testing
func CreateBatchExecutionData(workflowID string, count int, successRate float64) []*engine.EngineWorkflowExecutionData {
	var executions []*engine.EngineWorkflowExecutionData

	for i := 0; i < count; i++ {
		// Determine success based on success rate
		success := float64(i)/float64(count) < successRate

		// Vary duration based on success (successful executions are typically faster)
		var duration time.Duration
		if success {
			duration = time.Duration(30+i*5) * time.Second
		} else {
			duration = time.Duration(60+i*10) * time.Second
		}

		execution := CreateWorkflowExecutionData(workflowID, success, duration)
		executions = append(executions, execution)
	}

	return executions
}
