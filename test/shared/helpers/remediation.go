// Package testutil provides test utilities and factories for Kubernaut testing.
//
// Reference: 03-testing-strategy.mdc - Test Data Factory requirement
package helpers

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// ============================================================================
// RemediationRequest Factory (BR-ORCH-025)
// ============================================================================

// RemediationRequestOpts provides optional overrides for test data
type RemediationRequestOpts struct {
	SignalFingerprint string
	SignalName        string
	Severity          string
	// DEPRECATED: Environment removed from RR spec per NOTICE_RO_REMEDIATIONREQUEST_SCHEMA_UPDATE.md
	// Now in SignalProcessingStatus.EnvironmentClassification.Environment
	Environment string
	// DEPRECATED: Priority removed from RR spec per NOTICE_RO_REMEDIATIONREQUEST_SCHEMA_UPDATE.md
	// Now in SignalProcessingStatus.PriorityAssignment.Priority
	Priority        string
	SignalType      string
	TargetKind      string
	TargetName      string
	TargetNamespace string // For cluster-scoped resources, leave empty
	Phase           string
	TimeoutConfig   *remediationv1.TimeoutConfig // BR-ORCH-028: Per-phase timeout configuration
}

// NewRemediationRequest creates a test RemediationRequest with sensible defaults.
// Use opts to override specific fields.
func NewRemediationRequest(name, namespace string, opts ...RemediationRequestOpts) *remediationv1.RemediationRequest {
	// Default values
	fingerprint := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	signalName := "TestSignal"
	severity := "warning"
	// NOTE: Environment and Priority removed per NOTICE_RO_REMEDIATIONREQUEST_SCHEMA_UPDATE.md
	signalType := "prometheus"
	targetKind := "Pod"
	targetName := "test-pod"
	targetNamespace := namespace // Default to RR namespace for namespaced resources

	// Apply overrides if provided
	if len(opts) > 0 {
		opt := opts[0]
		if opt.SignalFingerprint != "" {
			fingerprint = opt.SignalFingerprint
		}
		if opt.SignalName != "" {
			signalName = opt.SignalName
		}
		if opt.Severity != "" {
			severity = opt.Severity
		}
		// NOTE: Environment and Priority overrides removed per NOTICE_RO_REMEDIATIONREQUEST_SCHEMA_UPDATE.md
		if opt.SignalType != "" {
			signalType = opt.SignalType
		}
		if opt.TargetKind != "" {
			targetKind = opt.TargetKind
		}
		if opt.TargetName != "" {
			targetName = opt.TargetName
		}
		// TargetNamespace: explicitly set (even to empty for cluster-scoped)
		if opt.TargetNamespace != "" {
			targetNamespace = opt.TargetNamespace
		} else if opt.TargetKind == "Node" || opt.TargetKind == "PersistentVolume" || opt.TargetKind == "ClusterRole" {
			// Cluster-scoped resources have no namespace
			targetNamespace = ""
		}
	}

	rr := &remediationv1.RemediationRequest{
		TypeMeta: metav1.TypeMeta{
			APIVersion: remediationv1.GroupVersion.String(),
			Kind:       "RemediationRequest",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID(fmt.Sprintf("%s-uid", name)),
		},
		Spec: remediationv1.RemediationRequestSpec{
			SignalFingerprint: fingerprint,
			SignalName:        signalName,
			Severity:          severity,
			// NOTE: Environment and Priority removed per NOTICE_RO_REMEDIATIONREQUEST_SCHEMA_UPDATE.md
			// These are now in SignalProcessingStatus, not RemediationRequestSpec
			SignalType:   signalType,
			SignalSource: "test",
			TargetType:   "kubernetes",
			TargetResource: remediationv1.ResourceIdentifier{
				Kind:      targetKind,
				Name:      targetName,
				Namespace: targetNamespace,
			},
		},
	}

	// Apply phase if provided
	if len(opts) > 0 && opts[0].Phase != "" {
		rr.Status.OverallPhase = remediationv1.RemediationPhase(opts[0].Phase)
	}

	// Apply TimeoutConfig if provided (BR-ORCH-028)
	if len(opts) > 0 && opts[0].TimeoutConfig != nil {
		rr.Status.TimeoutConfig = opts[0].TimeoutConfig
	}

	return rr
}

// ============================================================================
// SignalProcessing Factory (BR-ORCH-025)
// ============================================================================

// SignalProcessingOpts provides optional overrides for test data
type SignalProcessingOpts struct {
	Phase             signalprocessingv1.SignalProcessingPhase
	KubernetesContext *signalprocessingv1.KubernetesContext
}

// NewSignalProcessing creates a test SignalProcessing CRD with sensible defaults.
// Updated to match SignalProcessingSpec v1alpha1 schema (Dec 2025)
func NewSignalProcessing(name, namespace string, opts ...SignalProcessingOpts) *signalprocessingv1.SignalProcessing {
	sp := &signalprocessingv1.SignalProcessing{
		TypeMeta: metav1.TypeMeta{
			APIVersion: signalprocessingv1.GroupVersion.String(),
			Kind:       "SignalProcessing",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID: types.UID(fmt.Sprintf("%s-uid", name)),
		},
		Spec: signalprocessingv1.SignalProcessingSpec{
			RemediationRequestRef: signalprocessingv1.ObjectReference{
				Kind:      "RemediationRequest",
				Name:      fmt.Sprintf("%s-rr", name),
				Namespace: namespace,
			},
			Signal: signalprocessingv1.SignalData{
				Fingerprint: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
				Name:        "TestSignal",
				Severity:    "warning",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: namespace,
				},
			},
		},
	}

	// Apply options
	if len(opts) > 0 {
		opt := opts[0]
		if opt.Phase != "" {
			sp.Status.Phase = opt.Phase
		}
		if opt.KubernetesContext != nil {
			sp.Status.KubernetesContext = opt.KubernetesContext
		}
	}

	return sp
}

// NewCompletedSignalProcessing creates a SignalProcessing in Completed phase with enrichment results.
// Updated to match SignalProcessingStatus v1alpha1 schema (Dec 2025)
// NOTE: Environment and Priority are now owned by SignalProcessing (per NOTICE_RO_REMEDIATIONREQUEST_SCHEMA_UPDATE.md)
func NewCompletedSignalProcessing(name, namespace string) *signalprocessingv1.SignalProcessing {
	sp := NewSignalProcessing(name, namespace, SignalProcessingOpts{
		Phase: signalprocessingv1.PhaseCompleted,
	})
	// Set KubernetesContext directly on status (updated schema)
	sp.Status.KubernetesContext = &signalprocessingv1.KubernetesContext{
		Namespace: &signalprocessingv1.NamespaceContext{
			Name: namespace,
			Labels: map[string]string{
				"kubernetes.io/metadata.name": namespace,
			},
		},
	}
	// Set default environment classification (per V1.0 API)
	now := metav1.Now()
	sp.Status.EnvironmentClassification = &signalprocessingv1.EnvironmentClassification{
		Environment:  "production",
		Source:       "namespace-labels",
		ClassifiedAt: now,
	}
	// Set default priority assignment (per V1.0 API)
	sp.Status.PriorityAssignment = &signalprocessingv1.PriorityAssignment{
		Priority:   "P1",
		Source:     "fallback-matrix",
		AssignedAt: now,
	}
	// BR-SP-106: Set default signal mode and normalized type for downstream consumers
	// Defaults to reactive with Spec.Signal.Type as the normalized type
	sp.Status.SignalMode = "reactive"
	sp.Status.SignalType = sp.Spec.Signal.Type
	return sp
}

// ============================================================================
// AIAnalysis Factory (BR-ORCH-025, BR-ORCH-026)
// ============================================================================

// AIAnalysisOpts provides optional overrides for test data
type AIAnalysisOpts struct {
	Phase            string
	ApprovalRequired bool
	SelectedWorkflow *aianalysisv1.SelectedWorkflow
}

// NewAIAnalysis creates a test AIAnalysis CRD with sensible defaults.
func NewAIAnalysis(name, namespace string, opts ...AIAnalysisOpts) *aianalysisv1.AIAnalysis {
	ai := &aianalysisv1.AIAnalysis{
		TypeMeta: metav1.TypeMeta{
			APIVersion: aianalysisv1.GroupVersion.String(),
			Kind:       "AIAnalysis",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID: types.UID(fmt.Sprintf("%s-uid", name)),
		},
		Spec: aianalysisv1.AIAnalysisSpec{
			RemediationID: "test-remediation",
			AnalysisRequest: aianalysisv1.AnalysisRequest{
				SignalContext: aianalysisv1.SignalContextInput{
					Fingerprint:      "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
					Severity:         "warning",
					Environment:      "production",
					BusinessPriority: "P1",
				},
				AnalysisTypes: []string{"investigation", "root-cause", "workflow-selection"},
			},
		},
	}

	// Apply options
	if len(opts) > 0 {
		opt := opts[0]
		if opt.Phase != "" {
			ai.Status.Phase = opt.Phase
		}
		ai.Status.ApprovalRequired = opt.ApprovalRequired
		if opt.SelectedWorkflow != nil {
			ai.Status.SelectedWorkflow = opt.SelectedWorkflow
		}
	}

	return ai
}

// NewCompletedAIAnalysis creates an AIAnalysis in Completed phase with selected workflow.
func NewCompletedAIAnalysis(name, namespace string) *aianalysisv1.AIAnalysis {
	return NewAIAnalysis(name, namespace, AIAnalysisOpts{
		Phase:            "Completed",
		ApprovalRequired: false,
		SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
			WorkflowID:      "pod-restart-workflow",
			Version:         "v1.0.0",
			ExecutionBundle:  "kubernaut/workflows/pod-restart:v1.0.0",
			ExecutionBundleDigest: "sha256:abc123",
			Confidence:      0.95,
			Rationale:       "High confidence match for pod restart scenario",
			Parameters: map[string]string{
				"TARGET_POD": "test-pod",
			},
		},
	})
}

// NewAIAnalysisRequiringApproval creates an AIAnalysis that requires human approval.
func NewAIAnalysisRequiringApproval(name, namespace, approvalReason string) *aianalysisv1.AIAnalysis {
	ai := NewAIAnalysis(name, namespace, AIAnalysisOpts{
		Phase:            "Completed",
		ApprovalRequired: true,
		SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
			WorkflowID:      "deployment-rollback-workflow",
			Version:         "v1.0.0",
			ExecutionBundle:  "kubernaut/workflows/deployment-rollback:v1.0.0",
			ExecutionBundleDigest: "sha256:def456",
			Confidence:      0.65, // Low confidence triggers approval
			Rationale:       "Moderate confidence - human review recommended",
			Parameters: map[string]string{
				"TARGET_DEPLOYMENT": "test-deployment",
			},
		},
	})
	ai.Status.ApprovalReason = approvalReason
	ai.Status.ApprovalContext = &aianalysisv1.ApprovalContext{
		Reason:               approvalReason,
		ConfidenceScore:      0.65,
		ConfidenceLevel:      "low",
		InvestigationSummary: "Deployment rollback requires human approval",
		WhyApprovalRequired:  approvalReason,
		RecommendedActions: []aianalysisv1.RecommendedAction{
			{
				Action:    "deployment-rollback",
				Rationale: "Moderate confidence suggests human review",
			},
		},
	}
	return ai
}

// ============================================================================
// WorkflowExecution Factory (BR-ORCH-025, BR-ORCH-032)
// ============================================================================

// WorkflowExecutionOpts provides optional overrides for test data
// V1.0: Removed SkipReason and DuplicateOf (Skipped phase removed - RO makes routing decisions)
type WorkflowExecutionOpts struct {
	Phase          string
	FailureDetails *workflowexecutionv1.FailureDetails
}

// NewWorkflowExecution creates a test WorkflowExecution CRD with sensible defaults.
func NewWorkflowExecution(name, namespace string, opts ...WorkflowExecutionOpts) *workflowexecutionv1.WorkflowExecution {
	we := &workflowexecutionv1.WorkflowExecution{
		TypeMeta: metav1.TypeMeta{
			APIVersion: workflowexecutionv1.GroupVersion.String(),
			Kind:       "WorkflowExecution",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID: types.UID(fmt.Sprintf("%s-uid", name)),
		},
		Spec: workflowexecutionv1.WorkflowExecutionSpec{
			WorkflowRef: workflowexecutionv1.WorkflowRef{
				WorkflowID:      "pod-restart-workflow",
				Version:         "v1.0.0",
				ExecutionBundle:  "kubernaut/workflows/pod-restart:v1.0.0",
				ExecutionBundleDigest: "sha256:abc123",
			},
			TargetResource: fmt.Sprintf("%s/Pod/test-pod", namespace),
			Parameters: map[string]string{
				"TARGET_POD": "test-pod",
			},
			Confidence: 0.95,
			Rationale:  "High confidence match for pod restart scenario",
		},
	}

	// Apply options
	if len(opts) > 0 {
		opt := opts[0]
		if opt.Phase != "" {
			we.Status.Phase = opt.Phase
		}
		if opt.FailureDetails != nil {
			we.Status.FailureDetails = opt.FailureDetails
		}
	}

	return we
}

// NewCompletedWorkflowExecution creates a WorkflowExecution in Completed phase.
func NewCompletedWorkflowExecution(name, namespace string) *workflowexecutionv1.WorkflowExecution {
	we := NewWorkflowExecution(name, namespace, WorkflowExecutionOpts{
		Phase: workflowexecutionv1.PhaseCompleted,
	})
	now := metav1.Now()
	we.Status.StartTime = &now
	we.Status.CompletionTime = &now
	we.Status.Duration = "30s"
	return we
}

// NewSkippedWorkflowExecution was removed in V1.0.
// V1.0 Decision: RemediationOrchestrator makes routing decisions BEFORE creating WorkflowExecution.
// Duplicate detection (BR-ORCH-033) is now handled at RO level, not WFE level.
// See: docs/handoff/WE_TEAM_V1.0_API_BREAKING_CHANGES_REQUIRED.md for migration guide

// ============================================================================
// NotificationRequest Factory (BR-ORCH-001, BR-ORCH-034)
// ============================================================================

// NotificationRequestOpts provides optional overrides for test data
type NotificationRequestOpts struct {
	Type     notificationv1.NotificationType
	Priority notificationv1.NotificationPriority
	Channels []notificationv1.Channel
	Phase    notificationv1.NotificationPhase
}

// NewNotificationRequest creates a test NotificationRequest CRD with sensible defaults.
func NewNotificationRequest(name, namespace string, opts ...NotificationRequestOpts) *notificationv1.NotificationRequest {
	// Default values
	notifType := notificationv1.NotificationTypeEscalation
	priority := notificationv1.NotificationPriorityHigh
	channels := []notificationv1.Channel{notificationv1.ChannelSlack, notificationv1.ChannelEmail}

	// Apply options
	if len(opts) > 0 {
		opt := opts[0]
		if opt.Type != "" {
			notifType = opt.Type
		}
		if opt.Priority != "" {
			priority = opt.Priority
		}
		if len(opt.Channels) > 0 {
			channels = opt.Channels
		}
	}

	nr := &notificationv1.NotificationRequest{
		TypeMeta: metav1.TypeMeta{
			APIVersion: notificationv1.GroupVersion.String(),
			Kind:       "NotificationRequest",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID: types.UID(fmt.Sprintf("%s-uid", name)),
		},
		Spec: notificationv1.NotificationRequestSpec{
			Type:     notifType,
			Priority: priority,
			Channels: channels,
			Recipients: []notificationv1.Recipient{
				{
					Email: "oncall@example.com",
					Slack: "#alerts",
				},
			},
			Subject: "Test Notification",
			Body:    "This is a test notification body.",
			Metadata: map[string]string{
				"remediationRequestName": "test-rr",
				"cluster":                "test-cluster",
			},
		},
	}

	// Apply phase if provided
	if len(opts) > 0 && opts[0].Phase != "" {
		nr.Status.Phase = opts[0].Phase
	}

	return nr
}

// NewApprovalNotificationRequest creates a NotificationRequest for approval workflow.
func NewApprovalNotificationRequest(name, namespace, remediationRequestName string) *notificationv1.NotificationRequest {
	nr := NewNotificationRequest(name, namespace, NotificationRequestOpts{
		Type:     notificationv1.NotificationTypeEscalation,
		Priority: notificationv1.NotificationPriorityHigh,
		Channels: []notificationv1.Channel{
			notificationv1.ChannelSlack,
			notificationv1.ChannelEmail,
		},
	})
	nr.Spec.Subject = fmt.Sprintf("Approval Required: %s", remediationRequestName)
	nr.Spec.Body = "A remediation workflow requires human approval before execution."
	nr.Spec.Metadata["remediationRequestName"] = remediationRequestName
	nr.Spec.Metadata["notificationType"] = "approval_required"
	return nr
}

// ============================================================================
// Object Reference Helpers
// ============================================================================

// NewOwnerReference creates an owner reference for cascade deletion testing.
func NewOwnerReference(owner metav1.Object, apiVersion, kind string) metav1.OwnerReference {
	controller := true
	blockOwnerDeletion := true
	return metav1.OwnerReference{
		APIVersion:         apiVersion,
		Kind:               kind,
		Name:               owner.GetName(),
		UID:                owner.GetUID(),
		Controller:         &controller,
		BlockOwnerDeletion: &blockOwnerDeletion,
	}
}

// NewObjectReference creates a corev1.ObjectReference from an object.
func NewObjectReference(obj metav1.Object, apiVersion, kind string) corev1.ObjectReference {
	return corev1.ObjectReference{
		APIVersion: apiVersion,
		Kind:       kind,
		Name:       obj.GetName(),
		Namespace:  obj.GetNamespace(),
		UID:        obj.GetUID(),
	}
}
