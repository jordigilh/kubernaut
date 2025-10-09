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

package remediation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1alpha1"
	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// RemediationRequestReconciler reconciles a RemediationRequest object
type RemediationRequestReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=remediation.kubernaut.io,resources=remediationrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=remediation.kubernaut.io,resources=remediationrequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=remediation.kubernaut.io,resources=remediationrequests/finalizers,verbs=update
// +kubebuilder:rbac:groups=remediationprocessing.kubernaut.io,resources=remediationprocessings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=remediationprocessing.kubernaut.io,resources=remediationprocessings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aianalysis.kubernaut.io,resources=aianalyses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aianalysis.kubernaut.io,resources=aianalyses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=workflowexecution.kubernaut.io,resources=workflowexecutions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=workflowexecution.kubernaut.io,resources=workflowexecutions/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// Task 1.3: Phase progression state machine for multi-CRD orchestration
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *RemediationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Fetch RemediationRequest
	var remediationRequest remediationv1alpha1.RemediationRequest
	if err := r.Get(ctx, req.NamespacedName, &remediationRequest); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Initialize status if new
	if remediationRequest.Status.OverallPhase == "" {
		remediationRequest.Status.OverallPhase = "pending"
		remediationRequest.Status.StartTime = &metav1.Time{Time: time.Now()}
		if err := r.Status().Update(ctx, &remediationRequest); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Terminal states
	if remediationRequest.Status.OverallPhase == "completed" ||
		remediationRequest.Status.OverallPhase == "failed" {
		return ctrl.Result{}, nil
	}

	// Check for timeout before orchestrating
	if r.IsPhaseTimedOut(&remediationRequest) {
		return r.handleTimeout(ctx, &remediationRequest)
	}

	// Orchestrate based on phase
	return r.orchestratePhase(ctx, &remediationRequest)
}

// SetupWithManager sets up the controller with the Manager.
// Task 1.4: Watch downstream CRDs for orchestration
func (r *RemediationRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&remediationv1alpha1.RemediationRequest{}).
		Owns(&remediationprocessingv1alpha1.RemediationProcessing{}).
		Owns(&aianalysisv1alpha1.AIAnalysis{}).
		Owns(&workflowexecutionv1alpha1.WorkflowExecution{}).
		Named("remediation-remediationrequest").
		Complete(r)
}

// ========================================
// FIELD MAPPING FUNCTIONS
// Task 2.2: Self-contained CRD Pattern Implementation
// ========================================

// mapRemediationRequestToProcessingSpec maps RemediationRequest data to RemediationProcessing spec
// Phase 1: Implements self-contained CRD pattern - all data copied, no external reads required
func mapRemediationRequestToProcessingSpec(rr *remediationv1alpha1.RemediationRequest) remediationprocessingv1alpha1.RemediationProcessingSpec {
	return remediationprocessingv1alpha1.RemediationProcessingSpec{
		// ========================================
		// PARENT REFERENCE (Audit/Lineage Only)
		// ========================================
		RemediationRequestRef: corev1.ObjectReference{
			APIVersion: rr.APIVersion,
			Kind:       rr.Kind,
			Name:       rr.Name,
			Namespace:  rr.Namespace,
			UID:        rr.UID,
		},

		// ========================================
		// SIGNAL IDENTIFICATION (From RemediationRequest)
		// ========================================
		SignalFingerprint: rr.Spec.SignalFingerprint,
		SignalName:        rr.Spec.SignalName,
		Severity:          rr.Spec.Severity,

		// ========================================
		// SIGNAL CLASSIFICATION (From RemediationRequest)
		// ========================================
		Environment:  rr.Spec.Environment,
		Priority:     rr.Spec.Priority,
		SignalType:   rr.Spec.SignalType,
		SignalSource: rr.Spec.SignalSource,
		TargetType:   rr.Spec.TargetType,

		// ========================================
		// SIGNAL METADATA (From RemediationRequest)
		// ========================================
		SignalLabels:      deepCopyStringMap(rr.Spec.SignalLabels),
		SignalAnnotations: deepCopyStringMap(rr.Spec.SignalAnnotations),

		// ========================================
		// TARGET RESOURCE (From RemediationRequest)
		// ========================================
		TargetResource: extractTargetResource(rr),

		// ========================================
		// TIMESTAMPS (From RemediationRequest)
		// ========================================
		FiringTime:   rr.Spec.FiringTime,
		ReceivedTime: rr.Spec.ReceivedTime,

		// ========================================
		// DEDUPLICATION (From RemediationRequest)
		// ========================================
		Deduplication: mapDeduplicationInfo(rr.Spec.Deduplication),

		// ========================================
		// PROVIDER DATA (From RemediationRequest)
		// ========================================
		ProviderData:    deepCopyBytes(rr.Spec.ProviderData),
		OriginalPayload: deepCopyBytes(rr.Spec.OriginalPayload),

		// ========================================
		// STORM DETECTION (From RemediationRequest)
		// ========================================
		IsStorm:         rr.Spec.IsStorm,
		StormAlertCount: rr.Spec.StormAlertCount,

		// ========================================
		// CONFIGURATION (Optional, defaults if not specified)
		// ========================================
		EnrichmentConfig: getDefaultEnrichmentConfig(),
	}
}

// ========================================
// HELPER FUNCTIONS
// ========================================

// deepCopyStringMap creates a deep copy of string map
func deepCopyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	copied := make(map[string]string, len(m))
	for k, v := range m {
		copied[k] = v
	}
	return copied
}

// deepCopyBytes creates a deep copy of byte slice
func deepCopyBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	copied := make([]byte, len(b))
	copy(copied, b)
	return copied
}

// extractTargetResource extracts target resource from RemediationRequest
// Phase 1: Parses providerData to identify Kubernetes resource
// Future: Support for AWS/Azure/GCP resources
func extractTargetResource(rr *remediationv1alpha1.RemediationRequest) remediationprocessingv1alpha1.ResourceIdentifier {
	// Default empty resource if extraction fails
	resource := remediationprocessingv1alpha1.ResourceIdentifier{}

	// For Kubernetes signals, parse providerData
	if rr.Spec.TargetType == "kubernetes" {
		// Parse Kubernetes-specific fields from providerData
		var kubernetesData struct {
			Namespace string `json:"namespace"`
			Resource  struct {
				Kind string `json:"kind"`
				Name string `json:"name"`
			} `json:"resource"`
		}

		if err := json.Unmarshal(rr.Spec.ProviderData, &kubernetesData); err == nil {
			resource.Namespace = kubernetesData.Namespace
			resource.Kind = kubernetesData.Resource.Kind
			resource.Name = kubernetesData.Resource.Name
		}
	}

	// Fallback: Try to extract from signal labels
	if resource.Namespace == "" {
		if ns, ok := rr.Spec.SignalLabels["namespace"]; ok {
			resource.Namespace = ns
		}
	}
	if resource.Kind == "" {
		// Common label keys for resource kind
		for _, key := range []string{"kind", "resource_kind", "object_kind"} {
			if kind, ok := rr.Spec.SignalLabels[key]; ok {
				resource.Kind = kind
				break
			}
		}
	}
	if resource.Name == "" {
		// Common label keys for resource name
		for _, key := range []string{"pod", "deployment", "statefulset", "daemonset", "resource_name"} {
			if name, ok := rr.Spec.SignalLabels[key]; ok {
				resource.Name = name
				if resource.Kind == "" {
					// Infer kind from label key
					resource.Kind = strings.Title(key)
				}
				break
			}
		}
	}

	return resource
}

// mapDeduplicationInfo maps RemediationRequest deduplication to RemediationProcessing format
func mapDeduplicationInfo(dedupInfo remediationv1alpha1.DeduplicationInfo) remediationprocessingv1alpha1.DeduplicationContext {
	return remediationprocessingv1alpha1.DeduplicationContext{
		FirstOccurrence: dedupInfo.FirstSeen,
		LastOccurrence:  dedupInfo.LastSeen,
		OccurrenceCount: dedupInfo.OccurrenceCount,
		CorrelationID:   dedupInfo.PreviousRemediationRequestRef, // Optional correlation
	}
}

// getDefaultEnrichmentConfig returns default enrichment configuration
func getDefaultEnrichmentConfig() *remediationprocessingv1alpha1.EnrichmentConfiguration {
	return &remediationprocessingv1alpha1.EnrichmentConfiguration{
		EnableClusterState: true,  // Enable by default for Kubernetes signals
		EnableMetrics:      true,  // Enable by default for context
		EnableHistorical:   false, // Disabled by default (performance)
	}
}

// ========================================
// PHASE ORCHESTRATION - CRD CREATION FUNCTIONS
// Task 1.1-1.2: AIAnalysis and WorkflowExecution CRD creation
// ========================================

// createAIAnalysis creates AIAnalysis CRD when RemediationProcessing completes
func (r *RemediationRequestReconciler) createAIAnalysis(
	ctx context.Context,
	remediation *remediationv1alpha1.RemediationRequest,
	processing *remediationprocessingv1alpha1.RemediationProcessing,
) error {
	log := logf.FromContext(ctx)

	aiAnalysisName := fmt.Sprintf("%s-aianalysis", remediation.Name)

	// Map enriched context from RemediationProcessing.Status
	aiAnalysis := &aianalysisv1alpha1.AIAnalysis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      aiAnalysisName,
			Namespace: remediation.Namespace,
			Labels: map[string]string{
				"remediation-request": remediation.Name,
				"signal-type":         remediation.Spec.SignalType,
			},
		},
		Spec: aianalysisv1alpha1.AIAnalysisSpec{
			RemediationRequestRef: remediation.Name, // String reference, not ObjectReference
			SignalType:            remediation.Spec.SignalType,
			SignalContext:         processing.Status.ContextData, // Enriched data
			LLMProvider:           "holmesgpt",
			LLMModel:              "gpt-4",
			MaxTokens:             4000,
			Temperature:           0.7,
			IncludeHistory:        true,
		},
	}

	// Set owner reference
	if err := ctrl.SetControllerReference(remediation, aiAnalysis, r.Scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create AIAnalysis CRD
	if err := r.Create(ctx, aiAnalysis); err != nil {
		return fmt.Errorf("failed to create AIAnalysis: %w", err)
	}

	log.Info("Created AIAnalysis CRD", "name", aiAnalysisName)
	return nil
}

// createWorkflowExecution creates WorkflowExecution CRD when AIAnalysis completes
func (r *RemediationRequestReconciler) createWorkflowExecution(
	ctx context.Context,
	remediation *remediationv1alpha1.RemediationRequest,
	analysis *aianalysisv1alpha1.AIAnalysis,
) error {
	log := logf.FromContext(ctx)

	workflowName := fmt.Sprintf("%s-workflow", remediation.Name)

	// Create workflow from AI recommendations
	workflow := &workflowexecutionv1alpha1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workflowName,
			Namespace: remediation.Namespace,
			Labels: map[string]string{
				"remediation-request": remediation.Name,
			},
		},
		Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
			RemediationRequestRef: corev1.ObjectReference{
				Name:      remediation.Name,
				Namespace: remediation.Namespace,
				UID:       remediation.UID,
			},
			WorkflowDefinition: workflowexecutionv1alpha1.WorkflowDefinition{
				Name:    "ai-generated-workflow",
				Version: "1.0",
				Steps: []workflowexecutionv1alpha1.WorkflowStep{
					{
						StepNumber: 1,
						Name:       "AI Recommended Action",
						Action:     analysis.Status.RecommendedAction,
						Parameters: &workflowexecutionv1alpha1.StepParameters{}, // Pointer type
						MaxRetries: 3,
						Timeout:    "5m",
					},
				},
			},
			ExecutionStrategy: workflowexecutionv1alpha1.ExecutionStrategy{
				ApprovalRequired: false,
				DryRunFirst:      true,
				RollbackStrategy: "automatic",
				MaxRetries:       3,
			},
		},
	}

	// Set owner reference
	if err := ctrl.SetControllerReference(remediation, workflow, r.Scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create WorkflowExecution CRD
	if err := r.Create(ctx, workflow); err != nil {
		return fmt.Errorf("failed to create WorkflowExecution: %w", err)
	}

	log.Info("Created WorkflowExecution CRD", "name", workflowName)
	return nil
}

// ========================================
// PHASE ORCHESTRATION - STATE MACHINE
// Task 1.3: Phase progression handlers
// ========================================

// orchestratePhase implements phase progression state machine
func (r *RemediationRequestReconciler) orchestratePhase(
	ctx context.Context,
	remediation *remediationv1alpha1.RemediationRequest,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	switch remediation.Status.OverallPhase {
	case "pending":
		return r.handlePendingPhase(ctx, remediation)

	case "processing":
		return r.handleProcessingPhase(ctx, remediation)

	case "analyzing":
		return r.handleAnalyzingPhase(ctx, remediation)

	case "executing":
		return r.handleExecutingPhase(ctx, remediation)

	default:
		log.Info("Unknown phase", "phase", remediation.Status.OverallPhase)
		return ctrl.Result{}, nil
	}
}

// handlePendingPhase creates RemediationProcessing CRD
func (r *RemediationRequestReconciler) handlePendingPhase(
	ctx context.Context,
	remediation *remediationv1alpha1.RemediationRequest,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Check if RemediationProcessing exists
	processingName := fmt.Sprintf("%s-processing", remediation.Name)
	var processing remediationprocessingv1alpha1.RemediationProcessing
	err := r.Get(ctx, client.ObjectKey{Name: processingName, Namespace: remediation.Namespace}, &processing)

	if errors.IsNotFound(err) {
		// Create RemediationProcessing
		processing := &remediationprocessingv1alpha1.RemediationProcessing{
			ObjectMeta: metav1.ObjectMeta{
				Name:      processingName,
				Namespace: remediation.Namespace,
				Labels: map[string]string{
					"remediation-request": remediation.Name,
					"signal-type":         remediation.Spec.SignalType,
					"environment":         remediation.Spec.Environment,
					"priority":            remediation.Spec.Priority,
				},
				Annotations: map[string]string{
					"signal-fingerprint": remediation.Spec.SignalFingerprint,
					"created-by":         "remediation-orchestrator",
				},
			},
			Spec: mapRemediationRequestToProcessingSpec(remediation),
		}

		// Set owner reference for cascade deletion
		if err := ctrl.SetControllerReference(remediation, processing, r.Scheme); err != nil {
			log.Error(err, "Failed to set owner reference")
			return ctrl.Result{}, err
		}

		// Create the RemediationProcessing CRD
		if err := r.Create(ctx, processing); err != nil {
			log.Error(err, "Failed to create RemediationProcessing")
			return ctrl.Result{}, err
		}

		log.Info("Created RemediationProcessing CRD",
			"name", processingName,
			"signal-fingerprint", remediation.Spec.SignalFingerprint)
	} else if err != nil {
		return ctrl.Result{}, err
	}

	// Update phase to processing
	remediation.Status.OverallPhase = "processing"
	remediation.Status.RemediationProcessingRef = &corev1.ObjectReference{
		Name:      processingName,
		Namespace: remediation.Namespace,
	}
	return ctrl.Result{}, r.Status().Update(ctx, remediation)
}

// handleProcessingPhase waits for RemediationProcessing completion
func (r *RemediationRequestReconciler) handleProcessingPhase(
	ctx context.Context,
	remediation *remediationv1alpha1.RemediationRequest,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Get RemediationProcessing
	var processing remediationprocessingv1alpha1.RemediationProcessing
	err := r.Get(ctx, client.ObjectKey{
		Name:      remediation.Status.RemediationProcessingRef.Name,
		Namespace: remediation.Namespace,
	}, &processing)

	if err != nil {
		return ctrl.Result{}, err
	}

	// Check for failure
	if r.IsPhaseInFailedState(processing.Status.Phase) {
		log.Info("RemediationProcessing failed, transitioning to failed state",
			"remediation", remediation.Name,
			"processing", processing.Name)
		return r.handleFailure(ctx, remediation, "processing", "RemediationProcessing", processing.Status.Phase)
	}

	// Check if completed
	if processing.Status.Phase == "completed" {
		// Create AIAnalysis
		if err := r.createAIAnalysis(ctx, remediation, &processing); err != nil {
			return ctrl.Result{}, err
		}

		// Update phase
		remediation.Status.OverallPhase = "analyzing"
		remediation.Status.AIAnalysisRef = &corev1.ObjectReference{
			Name:      fmt.Sprintf("%s-aianalysis", remediation.Name),
			Namespace: remediation.Namespace,
		}

		log.Info("Phase transition: processing → analyzing", "remediation", remediation.Name)
		return ctrl.Result{}, r.Status().Update(ctx, remediation)
	}

	// Still processing - requeue
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// handleAnalyzingPhase waits for AIAnalysis completion
func (r *RemediationRequestReconciler) handleAnalyzingPhase(
	ctx context.Context,
	remediation *remediationv1alpha1.RemediationRequest,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Get AIAnalysis
	var analysis aianalysisv1alpha1.AIAnalysis
	err := r.Get(ctx, client.ObjectKey{
		Name:      remediation.Status.AIAnalysisRef.Name,
		Namespace: remediation.Namespace,
	}, &analysis)

	if err != nil {
		return ctrl.Result{}, err
	}

	// Check for failure
	if r.IsPhaseInFailedState(analysis.Status.Phase) {
		log.Info("AIAnalysis failed, transitioning to failed state",
			"remediation", remediation.Name,
			"analysis", analysis.Name)
		return r.handleFailure(ctx, remediation, "analyzing", "AIAnalysis", analysis.Status.Phase)
	}

	// Check if completed
	if analysis.Status.Phase == "Completed" {
		// Create WorkflowExecution
		if err := r.createWorkflowExecution(ctx, remediation, &analysis); err != nil {
			return ctrl.Result{}, err
		}

		// Update phase
		remediation.Status.OverallPhase = "executing"
		remediation.Status.WorkflowExecutionRef = &corev1.ObjectReference{
			Name:      fmt.Sprintf("%s-workflow", remediation.Name),
			Namespace: remediation.Namespace,
		}

		log.Info("Phase transition: analyzing → executing", "remediation", remediation.Name)
		return ctrl.Result{}, r.Status().Update(ctx, remediation)
	}

	// Still analyzing - requeue
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// handleExecutingPhase waits for WorkflowExecution completion
func (r *RemediationRequestReconciler) handleExecutingPhase(
	ctx context.Context,
	remediation *remediationv1alpha1.RemediationRequest,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Get WorkflowExecution
	var workflow workflowexecutionv1alpha1.WorkflowExecution
	err := r.Get(ctx, client.ObjectKey{
		Name:      remediation.Status.WorkflowExecutionRef.Name,
		Namespace: remediation.Namespace,
	}, &workflow)

	if err != nil {
		return ctrl.Result{}, err
	}

	// Check for failure
	if r.IsPhaseInFailedState(workflow.Status.Phase) {
		log.Info("WorkflowExecution failed, transitioning to failed state",
			"remediation", remediation.Name,
			"workflow", workflow.Name)
		return r.handleFailure(ctx, remediation, "executing", "WorkflowExecution", workflow.Status.Phase)
	}

	// Check if completed
	if workflow.Status.Phase == "completed" {
		// Update to completed
		remediation.Status.OverallPhase = "completed"
		remediation.Status.CompletedAt = &metav1.Time{Time: time.Now()}

		log.Info("Phase transition: executing → completed", "remediation", remediation.Name)
		return ctrl.Result{}, r.Status().Update(ctx, remediation)
	}

	// Still executing - requeue
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// ========================================
// FAILURE HANDLING
// Phase 2.2: Failure detection and handling
// ========================================

// IsPhaseInFailedState checks if a child CRD phase indicates failure
// Supports phases from RemediationProcessing, AIAnalysis, and WorkflowExecution
func (r *RemediationRequestReconciler) IsPhaseInFailedState(phase string) bool {
	// Normalize to lowercase for comparison
	phaseLower := strings.ToLower(phase)
	return phaseLower == "failed"
}

// BuildFailureReason constructs a descriptive failure reason
func (r *RemediationRequestReconciler) BuildFailureReason(
	currentPhase string,
	childCRDType string,
	errorMessage string,
) string {
	if errorMessage == "" {
		return fmt.Sprintf("%s phase failed: %s reported failure", currentPhase, childCRDType)
	}
	return fmt.Sprintf("%s phase failed: %s - %s", currentPhase, childCRDType, errorMessage)
}

// ShouldTransitionToFailed determines if RemediationRequest should transition to failed state
func (r *RemediationRequestReconciler) ShouldTransitionToFailed(
	remediation *remediationv1alpha1.RemediationRequest,
	childFailed bool,
) bool {
	// Don't transition if child didn't fail
	if !childFailed {
		return false
	}

	// Don't transition if already in a terminal state
	if remediation.Status.OverallPhase == "completed" ||
		remediation.Status.OverallPhase == "failed" {
		return false
	}

	// Don't transition if still pending (no child CRD yet)
	if remediation.Status.OverallPhase == "pending" ||
		remediation.Status.OverallPhase == "" {
		return false
	}

	// Transition to failed for active phases with child failures
	return true
}

// handleFailure marks the RemediationRequest as failed with appropriate metadata
// Business Requirement: BR-ORCHESTRATION-004
func (r *RemediationRequestReconciler) handleFailure(
	ctx context.Context,
	remediation *remediationv1alpha1.RemediationRequest,
	phase string,
	childCRDType string,
	childPhase string,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Build failure reason
	reason := r.BuildFailureReason(phase, childCRDType, childPhase)

	// Update status to failed
	remediation.Status.OverallPhase = "failed"
	remediation.Status.CompletedAt = &metav1.Time{Time: time.Now()}

	log.Info("RemediationRequest marked as failed",
		"name", remediation.Name,
		"phase", phase,
		"reason", reason)

	// TODO Phase 3.1: Emit Kubernetes event
	// TODO Phase 3.2: Record audit entry
	// TODO Phase 3.3: Trigger notification/escalation

	if err := r.Status().Update(ctx, remediation); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil // Terminal state, no requeue
}

// ========================================
// TIMEOUT HANDLING
// Phase 2.1: Timeout detection and handling
// ========================================

// IsPhaseTimedOut checks if the current phase has exceeded its timeout threshold
// Timeout thresholds per phase (from reconciliation-phases.md):
// - pending: 30 seconds
// - processing: 5 minutes
// - analyzing: 10 minutes
// - executing: 30 minutes
func (r *RemediationRequestReconciler) IsPhaseTimedOut(remediation *remediationv1alpha1.RemediationRequest) bool {
	if remediation.Status.StartTime == nil {
		return false
	}

	elapsed := time.Since(remediation.Status.StartTime.Time)

	switch remediation.Status.OverallPhase {
	case "pending":
		return elapsed > 30*time.Second
	case "processing":
		return elapsed > 5*time.Minute
	case "analyzing":
		return elapsed > 10*time.Minute
	case "executing":
		return elapsed > 30*time.Minute
	default:
		return false
	}
}

// handleTimeout marks the remediation as failed due to timeout
// Business Requirement: BR-ORCHESTRATION-003 (Timeout Handling)
func (r *RemediationRequestReconciler) handleTimeout(
	ctx context.Context,
	remediation *remediationv1alpha1.RemediationRequest,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Mark as failed with timeout information
	remediation.Status.OverallPhase = "failed"
	remediation.Status.CompletedAt = &metav1.Time{Time: time.Now()}

	log.Info("Phase timeout detected",
		"remediation", remediation.Name,
		"phase", remediation.Status.OverallPhase,
		"elapsed", time.Since(remediation.Status.StartTime.Time),
	)

	if err := r.Status().Update(ctx, remediation); err != nil {
		return ctrl.Result{}, err
	}

	// Terminal state - no requeue
	return ctrl.Result{}, nil
}
