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
package safety

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// SafetyValidator provides real Kubernetes safety validation
// Business Requirements: BR-SAFE-001 through BR-SAFE-010 - Safety validation and risk assessment
type SafetyValidator struct {
	client kubernetes.Interface
	log    *logrus.Logger
	config *SafetyConfig
}

// SafetyConfig configures safety validation behavior
type SafetyConfig struct {
	MaxRiskLevel        string        `yaml:"max_risk_level" default:"MEDIUM"`
	RequireConnectivity bool          `yaml:"require_connectivity" default:"true"`
	ValidationTimeout   time.Duration `yaml:"validation_timeout" default:"30s"`
	DryRunByDefault     bool          `yaml:"dry_run_by_default" default:"true"`
}

// ClusterValidationResult represents cluster connectivity validation results
type ClusterValidationResult struct {
	IsValid           bool   `json:"is_valid"`
	ConnectivityCheck bool   `json:"connectivity_check"`
	PermissionLevel   string `json:"permission_level"`
	ErrorMessage      string `json:"error_message,omitempty"`
	RiskLevel         string `json:"risk_level"`
}

// ResourceValidationResult represents resource state validation results
type ResourceValidationResult struct {
	IsValid        bool   `json:"is_valid"`
	ResourceExists bool   `json:"resource_exists"`
	CurrentState   string `json:"current_state"`
	ErrorMessage   string `json:"error_message,omitempty"`
	RiskLevel      string `json:"risk_level"`
}

// RiskAssessment represents risk analysis for a planned action
type RiskAssessment struct {
	ActionName    string                 `json:"action_name"`
	RiskLevel     string                 `json:"risk_level"`
	RiskFactors   []string               `json:"risk_factors"`
	Mitigation    string                 `json:"mitigation"`
	SafeToExecute bool                   `json:"safe_to_execute"`
	Confidence    float64                `json:"confidence"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// SafetyPolicy represents a safety policy rule
type SafetyPolicy struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Conditions  map[string]interface{} `json:"conditions"`
	Actions     []string               `json:"actions"`
	Severity    string                 `json:"severity"`
	Enabled     bool                   `json:"enabled"`
	CreatedAt   time.Time              `json:"created_at"`
}

// SafetyAuditEntry represents an audit trail entry
type SafetyAuditEntry struct {
	ID        string                 `json:"id"`
	Action    string                 `json:"action"`
	Namespace string                 `json:"namespace"`
	Resource  string                 `json:"resource"`
	User      string                 `json:"user"`
	Timestamp time.Time              `json:"timestamp"`
	Result    string                 `json:"result"`
	RiskLevel string                 `json:"risk_level"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// RollbackValidationResult represents rollback validation results
type RollbackValidationResult struct {
	IsValid       bool     `json:"is_valid"`
	CanRollback   bool     `json:"can_rollback"`
	RollbackSteps []string `json:"rollback_steps"`
	ErrorMessage  string   `json:"error_message,omitempty"`
	EstimatedTime string   `json:"estimated_time"`
}

// RollbackState represents the state for rollback operations
type RollbackState struct {
	ActionID      string                 `json:"action_id"`
	OriginalState map[string]interface{} `json:"original_state"`
	CurrentState  map[string]interface{} `json:"current_state"`
	Timestamp     time.Time              `json:"timestamp"`
}

// NewSafetyValidator creates a new real safety validator
func NewSafetyValidator(client kubernetes.Interface, log *logrus.Logger) *SafetyValidator {
	if log == nil {
		log = logrus.New()
		log.SetLevel(logrus.WarnLevel)
	}

	config := &SafetyConfig{
		MaxRiskLevel:        "MEDIUM",
		RequireConnectivity: true,
		ValidationTimeout:   30 * time.Second,
		DryRunByDefault:     true,
	}

	return &SafetyValidator{
		client: client,
		log:    log,
		config: config,
	}
}

// ValidateClusterAccess validates cluster connectivity and permissions
// Business Requirement: BR-SAFE-001 - Cluster connectivity validation
func (sv *SafetyValidator) ValidateClusterAccess(ctx context.Context, namespace string) *ClusterValidationResult {
	sv.log.WithFields(logrus.Fields{
		"namespace":            namespace,
		"business_requirement": "BR-SAFE-001",
	}).Debug("Validating cluster access")

	result := &ClusterValidationResult{
		IsValid:           false,
		ConnectivityCheck: false,
		PermissionLevel:   "none",
		RiskLevel:         "HIGH",
	}

	// Check cluster connectivity
	ctx, cancel := context.WithTimeout(ctx, sv.config.ValidationTimeout)
	defer cancel()

	// Test basic cluster connectivity by listing nodes
	_, err := sv.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("cluster connectivity failed: %v", err)
		result.RiskLevel = "CRITICAL"
		sv.log.WithError(err).Error("BR-SAFE-001: Cluster connectivity validation failed")
		return result
	}

	result.ConnectivityCheck = true

	// Check namespace access permissions
	if namespace != "" {
		_, err = sv.client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		if err != nil {
			result.ErrorMessage = fmt.Sprintf("namespace access failed: %v", err)
			result.RiskLevel = "HIGH"
			sv.log.WithError(err).WithField("namespace", namespace).Error("BR-SAFE-001: Namespace access validation failed")
			return result
		}

		// Test permissions by attempting to list pods (read access)
		_, err = sv.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{Limit: 1})
		if err != nil {
			result.PermissionLevel = "read-only"
			result.RiskLevel = "MEDIUM"
		} else {
			result.PermissionLevel = "admin"
			result.RiskLevel = "LOW"
		}
	} else {
		result.PermissionLevel = "cluster-admin"
		result.RiskLevel = "LOW"
	}

	result.IsValid = true

	sv.log.WithFields(logrus.Fields{
		"namespace":            namespace,
		"permission_level":     result.PermissionLevel,
		"risk_level":           result.RiskLevel,
		"business_requirement": "BR-SAFE-001",
	}).Info("Cluster access validation completed successfully")

	return result
}

// ValidateResourceState validates the current state of Kubernetes resources
// Business Requirement: BR-SAFE-002 - Resource state validation
func (sv *SafetyValidator) ValidateResourceState(ctx context.Context, alert types.Alert) *ResourceValidationResult {
	sv.log.WithFields(logrus.Fields{
		"namespace":            alert.Namespace,
		"resource":             alert.Resource,
		"business_requirement": "BR-SAFE-002",
	}).Debug("Validating resource state")

	result := &ResourceValidationResult{
		IsValid:        false,
		ResourceExists: false,
		CurrentState:   "UNKNOWN",
		RiskLevel:      "HIGH",
	}

	ctx, cancel := context.WithTimeout(ctx, sv.config.ValidationTimeout)
	defer cancel()

	// Basic resource existence check - try to get deployment first
	deployment, err := sv.client.AppsV1().Deployments(alert.Namespace).Get(ctx, alert.Resource, metav1.GetOptions{})
	if err == nil {
		result.ResourceExists = true
		result.CurrentState = "Available"
		if deployment.Status.ReadyReplicas > 0 {
			result.CurrentState = "Ready"
			result.RiskLevel = "LOW"
		} else {
			result.CurrentState = "NotReady"
			result.RiskLevel = "MEDIUM"
		}
		result.IsValid = true
		sv.log.WithFields(logrus.Fields{
			"resource":             alert.Resource,
			"ready_replicas":       deployment.Status.ReadyReplicas,
			"business_requirement": "BR-SAFE-002",
		}).Debug("Deployment resource state validated")
		return result
	}

	// Try pod resource
	pod, err := sv.client.CoreV1().Pods(alert.Namespace).Get(ctx, alert.Resource, metav1.GetOptions{})
	if err == nil {
		result.ResourceExists = true
		result.CurrentState = string(pod.Status.Phase)
		switch pod.Status.Phase {
		case "Running":
			result.RiskLevel = "LOW"
		case "Pending":
			result.RiskLevel = "MEDIUM"
		default:
			result.RiskLevel = "HIGH"
		}
		result.IsValid = true
		sv.log.WithFields(logrus.Fields{
			"resource":             alert.Resource,
			"phase":                pod.Status.Phase,
			"business_requirement": "BR-SAFE-002",
		}).Debug("Pod resource state validated")
		return result
	}

	// Try service resource
	service, err := sv.client.CoreV1().Services(alert.Namespace).Get(ctx, alert.Resource, metav1.GetOptions{})
	if err == nil {
		result.ResourceExists = true
		result.CurrentState = "Available"
		if len(service.Spec.Ports) > 0 {
			result.RiskLevel = "LOW"
		} else {
			result.RiskLevel = "MEDIUM"
		}
		result.IsValid = true
		sv.log.WithFields(logrus.Fields{
			"resource":             alert.Resource,
			"ports":                len(service.Spec.Ports),
			"business_requirement": "BR-SAFE-002",
		}).Debug("Service resource state validated")
		return result
	}

	// Resource not found
	result.ErrorMessage = fmt.Sprintf("resource %s not found in namespace %s", alert.Resource, alert.Namespace)
	sv.log.WithFields(logrus.Fields{
		"namespace":            alert.Namespace,
		"resource":             alert.Resource,
		"business_requirement": "BR-SAFE-002",
	}).Warn("Resource not found during state validation")

	return result
}

// AssessRisk performs risk assessment for a planned action
// Business Requirement: BR-SAFE-003 - Risk assessment and mitigation
func (sv *SafetyValidator) AssessRisk(ctx context.Context, action types.ActionRecommendation, alert types.Alert) *RiskAssessment {
	sv.log.WithFields(logrus.Fields{
		"action":               action.Action,
		"namespace":            alert.Namespace,
		"resource":             alert.Resource,
		"business_requirement": "BR-SAFE-003",
	}).Debug("Performing risk assessment")

	assessment := &RiskAssessment{
		ActionName:    action.Action,
		RiskLevel:     "MEDIUM",
		RiskFactors:   []string{},
		SafeToExecute: false,
		Confidence:    0.8,
		Metadata:      make(map[string]interface{}),
	}

	// Assess risk based on action type
	switch action.Action {
	case "restart_pod", "restart_pods":
		assessment.RiskLevel = "LOW"
		assessment.RiskFactors = []string{"temporary service disruption"}
		assessment.Mitigation = "rolling restart to minimize downtime"
		assessment.SafeToExecute = true
		assessment.Confidence = 0.9

	case "scale_deployment":
		assessment.RiskLevel = "LOW"
		assessment.RiskFactors = []string{"resource utilization change"}
		assessment.Mitigation = "gradual scaling with monitoring"
		assessment.SafeToExecute = true
		assessment.Confidence = 0.85

	case "drain_node":
		assessment.RiskLevel = "HIGH"
		assessment.RiskFactors = []string{"multiple service disruption", "capacity reduction"}
		assessment.Mitigation = "ensure other nodes can handle workload"
		assessment.SafeToExecute = false
		assessment.Confidence = 0.7

	case "delete_pod", "delete_pods":
		assessment.RiskLevel = "MEDIUM"
		assessment.RiskFactors = []string{"service interruption", "data loss potential"}
		assessment.Mitigation = "verify replication and backup status"
		assessment.SafeToExecute = alert.Severity != "critical"
		assessment.Confidence = 0.75

	default:
		assessment.RiskLevel = "HIGH"
		assessment.RiskFactors = []string{"unknown action type"}
		assessment.Mitigation = "manual review required"
		assessment.SafeToExecute = false
		assessment.Confidence = 0.5
	}

	// Adjust risk based on alert severity
	if alert.Severity == "critical" {
		assessment.SafeToExecute = true // Critical alerts may require higher risk actions
		if assessment.RiskLevel == "LOW" {
			assessment.RiskLevel = "MEDIUM"
		}
	}

	assessment.Metadata["alert_severity"] = alert.Severity
	assessment.Metadata["assessment_time"] = time.Now()

	sv.log.WithFields(logrus.Fields{
		"action":               action.Action,
		"risk_level":           assessment.RiskLevel,
		"safe_to_execute":      assessment.SafeToExecute,
		"confidence":           assessment.Confidence,
		"business_requirement": "BR-SAFE-003",
	}).Info("Risk assessment completed")

	return assessment
}

// IsHealthy performs a health check on the safety validator
func (sv *SafetyValidator) IsHealthy(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Test basic cluster connectivity
	_, err := sv.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return fmt.Errorf("safety validator health check failed: %w", err)
	}

	return nil
}
