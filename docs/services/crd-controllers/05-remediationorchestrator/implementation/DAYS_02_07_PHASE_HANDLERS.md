# Days 2-7: Core Business Logic - Phase Handlers (48h)

> **Note (Issue #91):** This document references `kubernaut.ai/*` CRD labels that have since been migrated to immutable spec fields. See [DD-CRD-003](../../../../../architecture/DD-CRD-003-field-selectors-operational-queries.md) for the current field-selector-based approach.

> **📋 Changelog**
> | Version | Date | Changes | Reference |
> |---------|------|---------|-----------|
> | **v1.6** | 2025-12-06 | **Day 6 CRITICAL FIXES**: Corrected package path to `pkg/remediationorchestrator/timeout/`, fixed TimeoutConfig usage (not `GlobalTimeout`), added phase start time fields to API (`ProcessingStartTime`, `AnalyzingStartTime`, `ExecutingStartTime`), added test plan with 14 tests + DescribeTable examples | [BR-ORCH-027](BUSINESS_REQUIREMENTS.md#br-orch-027-global-remediation-timeout), [BR-ORCH-028](BUSINESS_REQUIREMENTS.md#br-orch-028-per-phase-timeouts) |
> | v1.5 | 2025-12-06 | **Day 5 MAJOR UPDATE**: Fixed `rr.Status.Phase` → `rr.Status.OverallPhase`, added `SignalProcessingRef` and `RequiresManualReview` to API, fixed SP `Environment` read from status, added Day 5 test plan with 21 tests | [NOTICE](../../../handoff/NOTICE_RO_REMEDIATIONREQUEST_SCHEMA_UPDATE.md) |
> | v1.4 | 2025-12-06 | **Day 4/5 CRITICAL FIXES**: Fixed API field access errors (`rr.Spec.SignalName` not `rr.Spec.SignalData.SignalName`), added precondition validation, added `NotificationTypeManualReview` enum (BR-ORCH-036), added Day 4 test plan with BR mappings | [BR-ORCH-036](../../../requirements/BR-ORCH-036-manual-review-notification.md) |
> | v1.3 | 2025-12-06 | **Day 5 MAJOR UPDATE**: Added WE failure handling per DD-WE-004. New skip reasons: `ExhaustedRetries`, `PreviousExecutionFailed`. New file: `handler/workflowexecution.go`. Updated BR-ORCH-032 v1.1 | [NOTICE](../../../handoff/NOTICE_WE_EXPONENTIAL_BACKOFF_DD_WE_004.md) |
> | v1.2 | 2025-12-06 | **Day 4 CRITICAL FIX**: Corrected NotificationRequest API usage - `Subject`/`Body` (not `Title`/`Message`), `Metadata` (not `Context`), typed `Channel`/`Priority` enums, added `NotificationTypeApproval` enum | [NOTICE](../../../handoff/NOTICE_NOTIFICATION_TYPE_APPROVAL_ADDITION.md) |
> | v1.1 | 2025-12-06 | **Day 4 Update**: Added BR-ORCH-035 notification reference tracking to all creators; **Day 3 Update**: Added precondition validation for SelectedWorkflow | [BR-ORCH-035](../../../requirements/BR-ORCH-035-notification-reference-tracking.md) |
> | v1.0 | 2025-12-04 | Initial breakout from main plan | - |

**Parent Document**: [IMPLEMENTATION_PLAN_V1.2.md](./IMPLEMENTATION_PLAN_V1.2.md)
**Date**: Days 2-7 of 14-16
**Focus**: Child CRD creators, phase handlers, status aggregation
**Deliverable**: `02-day3-midpoint.md`, `03-day7-complete.md`

---

## 📑 Table of Contents

| Day | Focus | Hours | Key Deliverables |
|-----|-------|-------|------------------|
| [Day 2](#day-2-child-crd-creators-8h) | SignalProcessing & AIAnalysis creators | 8h | Child CRD creation logic |
| [Day 3](#day-3-workflowexecution-creator-8h) | WorkflowExecution creator | 8h | Workflow pass-through (BR-ORCH-025) |
| [Day 4](#day-4-notification-creator-8h) | Notification creators | 8h | Approval (BR-ORCH-001), bulk (BR-ORCH-034), tracking (BR-ORCH-035) |
| [Day 5](#day-5-status-aggregation-and-we-failure-handling-8h) | Status aggregation + WE failure handling | 8h | Multi-CRD status, WE skip/failure (DD-WE-004, BR-ORCH-032) |
| [Day 6](#day-6-timeout-detection-8h) | Timeout detection | 8h | BR-ORCH-027, BR-ORCH-028 |
| [Day 7](#day-7-escalation-manager-8h) | Escalation manager | 8h | Failed/timeout escalation |

---

## Day 2: Child CRD Creators (8h)

### Morning: SignalProcessing Creator (4h)

**File**: `pkg/remediationorchestrator/creator/signalprocessing.go`

**API Contract Alignment** (Updated Dec 2025):
- `SignalProcessingSpec` uses `RemediationRequestRef` + `Signal` structure (per `signalprocessing_types.go`)
- Method signature takes only `RemediationRequest` (SP doesn't need prior CRD data)

```go
package creator

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// SignalProcessingCreator creates SignalProcessing CRDs from RemediationRequests.
type SignalProcessingCreator struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewSignalProcessingCreator creates a new SignalProcessingCreator.
func NewSignalProcessingCreator(c client.Client, s *runtime.Scheme) *SignalProcessingCreator {
	return &SignalProcessingCreator{
		client: c,
		scheme: s,
	}
}

// Create creates a SignalProcessing CRD for the given RemediationRequest.
// It is idempotent - if the CRD already exists, it returns the existing name.
// Reference: BR-ORCH-025 (data pass-through), BR-ORCH-031 (cascade deletion)
func (c *SignalProcessingCreator) Create(ctx context.Context, rr *remediationv1.RemediationRequest) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
	)

	// Generate deterministic name
	name := fmt.Sprintf("sp-%s", rr.Name)

	// Check if already exists (idempotency)
	existing := &signalprocessingv1.SignalProcessing{}
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, existing)
	if err == nil {
		logger.Info("SignalProcessing already exists, reusing", "name", name)
		return name, nil
	}
	if !apierrors.IsNotFound(err) {
		logger.Error(err, "Failed to check existing SignalProcessing")
		return "", fmt.Errorf("failed to check existing SignalProcessing: %w", err)
	}

	// Build SignalProcessing CRD with data pass-through (BR-ORCH-025)
	sp := &signalprocessingv1.SignalProcessing{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
			Labels: map[string]string{
				"kubernaut.ai/remediation-request": rr.Name,
				"kubernaut.ai/component":           "signal-processing",
			},
		},
		Spec: signalprocessingv1.SignalProcessingSpec{
			// Reference to parent RemediationRequest for audit trail
			RemediationRequestRef: signalprocessingv1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rr.Name,
				Namespace:  rr.Namespace,
				UID:        string(rr.UID),
			},
			// Signal data pass-through from RemediationRequest
			Signal: signalprocessingv1.SignalData{
				Fingerprint:    rr.Spec.SignalFingerprint,
				Name:           rr.Spec.SignalName,
				Severity:       rr.Spec.Severity,
				Type:           rr.Spec.SignalType,
				Source:         rr.Spec.SignalSource,
				TargetType:     rr.Spec.TargetType,
				Labels:         rr.Spec.SignalLabels,
				Annotations:    rr.Spec.SignalAnnotations,
				FiringTime:     &rr.Spec.FiringTime,
				ReceivedTime:   rr.Spec.ReceivedTime,
				ProviderData:   rr.Spec.ProviderData,
				TargetResource: c.buildTargetResource(rr),
			},
		},
	}

	// Set owner reference for cascade deletion (BR-ORCH-031)
	if err := controllerutil.SetControllerReference(rr, sp, c.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference")
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create the CRD
	if err := c.client.Create(ctx, sp); err != nil {
		logger.Error(err, "Failed to create SignalProcessing CRD")
		return "", fmt.Errorf("failed to create SignalProcessing: %w", err)
	}

	logger.Info("Created SignalProcessing CRD", "name", name)
	return name, nil
}

// buildTargetResource converts ResourceIdentifier to SignalProcessing format.
func (c *SignalProcessingCreator) buildTargetResource(rr *remediationv1.RemediationRequest) signalprocessingv1.ResourceIdentifier {
	return signalprocessingv1.ResourceIdentifier{
		Kind:      rr.Spec.TargetResource.Kind,
		Name:      rr.Spec.TargetResource.Name,
		Namespace: rr.Spec.TargetResource.Namespace,
	}
}
```

### Afternoon: AIAnalysis Creator (4h)

**File**: `pkg/remediationorchestrator/creator/aianalysis.go`

**API Contract Alignment** (Updated Dec 2025):
- `AIAnalysisSpec` uses `RemediationRequestRef`, `RemediationID`, `AnalysisRequest` structure (per `aianalysis_types.go`)
- Method signature takes SignalProcessing as parameter (consistent with Day 2 pattern, better testability)
- EnrichmentResults passed through from SP status to AI SignalContextInput

```go
package creator

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// AIAnalysisCreator creates AIAnalysis CRDs from RemediationRequests.
type AIAnalysisCreator struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewAIAnalysisCreator creates a new AIAnalysisCreator.
func NewAIAnalysisCreator(c client.Client, s *runtime.Scheme) *AIAnalysisCreator {
	return &AIAnalysisCreator{
		client: c,
		scheme: s,
	}
}

// Create creates an AIAnalysis CRD for the given RemediationRequest.
// It uses enrichment data from the completed SignalProcessing CRD.
// It is idempotent - if the CRD already exists, it returns the existing name.
// Reference: BR-ORCH-025 (data pass-through), BR-ORCH-031 (cascade deletion)
func (c *AIAnalysisCreator) Create(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	sp *signalprocessingv1.SignalProcessing,
) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
		"signalProcessing", sp.Name,
	)

	// Generate deterministic name
	name := fmt.Sprintf("ai-%s", rr.Name)

	// Check if already exists (idempotency)
	existing := &aianalysisv1.AIAnalysis{}
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, existing)
	if err == nil {
		logger.Info("AIAnalysis already exists, reusing", "name", name)
		return name, nil
	}
	if !apierrors.IsNotFound(err) {
		logger.Error(err, "Failed to check existing AIAnalysis")
		return "", fmt.Errorf("failed to check existing AIAnalysis: %w", err)
	}

	// Build AIAnalysis CRD
	ai := &aianalysisv1.AIAnalysis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
			Labels: map[string]string{
				"kubernaut.ai/remediation-request": rr.Name,
				"kubernaut.ai/component":           "ai-analysis",
			},
		},
		Spec: aianalysisv1.AIAnalysisSpec{
			// Parent reference for audit trail
			RemediationRequestRef: corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rr.Name,
				Namespace:  rr.Namespace,
				UID:        rr.UID,
			},
			// Remediation ID for audit correlation
			RemediationID: string(rr.UID),
			// Analysis request with signal context
			AnalysisRequest: aianalysisv1.AnalysisRequest{
				SignalContext: c.buildSignalContext(rr, sp),
				AnalysisTypes: []string{
					"investigation",
					"root-cause",
					"workflow-selection",
				},
			},
			// Recovery fields (false for initial analysis)
			IsRecoveryAttempt:     false,
			RecoveryAttemptNumber: 0,
		},
	}

	// Set owner reference for cascade deletion (BR-ORCH-031)
	if err := controllerutil.SetControllerReference(rr, ai, c.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference")
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create the CRD
	if err := c.client.Create(ctx, ai); err != nil {
		logger.Error(err, "Failed to create AIAnalysis CRD")
		return "", fmt.Errorf("failed to create AIAnalysis: %w", err)
	}

	logger.Info("Created AIAnalysis CRD", "name", name)
	return name, nil
}

// buildSignalContext constructs the SignalContextInput from RemediationRequest and SignalProcessing.
func (c *AIAnalysisCreator) buildSignalContext(
	rr *remediationv1.RemediationRequest,
	sp *signalprocessingv1.SignalProcessing,
) aianalysisv1.SignalContextInput {
	// Get environment and priority from SP status if available, fallback to RR spec
	environment := rr.Spec.Environment
	priority := rr.Spec.Priority
	if sp.Status.EnvironmentClassification != nil {
		environment = sp.Status.EnvironmentClassification.Environment
	}
	if sp.Status.PriorityAssignment != nil {
		priority = sp.Status.PriorityAssignment.Priority
	}

	return aianalysisv1.SignalContextInput{
		Fingerprint:      rr.Spec.SignalFingerprint,
		Severity:         rr.Spec.Severity,
		SignalName:       rr.Spec.SignalName,  // Issue #166: signal name (OOMKilled etc)
		Environment:      environment,
		BusinessPriority: priority,
		TargetResource: aianalysisv1.TargetResource{
			Kind:      rr.Spec.TargetResource.Kind,
			Name:      rr.Spec.TargetResource.Name,
			Namespace: rr.Spec.TargetResource.Namespace,
		},
		// EnrichmentResults from SignalProcessing status (BR-ORCH-025)
		EnrichmentResults: c.buildEnrichmentResults(sp),
	}
}

// buildEnrichmentResults converts SignalProcessing status to shared EnrichmentResults.
// Reference: BR-ORCH-025 (data pass-through from SP enrichment)
func (c *AIAnalysisCreator) buildEnrichmentResults(sp *signalprocessingv1.SignalProcessing) sharedtypes.EnrichmentResults {
	results := sharedtypes.EnrichmentResults{}

	// Pass through KubernetesContext from SP status
	if sp.Status.KubernetesContext != nil {
		results.KubernetesContext = &sharedtypes.KubernetesContext{
			NamespaceLabels: sp.Status.KubernetesContext.NamespaceLabels,
		}
	}

	// Pass through owner chain if available
	if sp.Status.KubernetesContext != nil && len(sp.Status.KubernetesContext.OwnerChain) > 0 {
		results.OwnerChain = make([]sharedtypes.OwnerChainEntry, len(sp.Status.KubernetesContext.OwnerChain))
		for i, entry := range sp.Status.KubernetesContext.OwnerChain {
			results.OwnerChain[i] = sharedtypes.OwnerChainEntry{
				Kind: entry.Kind,
				Name: entry.Name,
			}
		}
	}

	return results
}
```

---

## Day 3: WorkflowExecution Creator (8h)

### WorkflowExecution Creator (BR-ORCH-025)

**File**: `pkg/remediationorchestrator/creator/workflowexecution.go`

**API Contract Alignment** (Updated Dec 2025):
- `TargetResource` is a **string** format: `"namespace/kind/name"` (per `workflowexecution_types.go`)
- `RemediationRequestRef` is **required** (`corev1.ObjectReference`)
- `WorkflowRef.Version` is **required** (from AIAnalysis.Status.SelectedWorkflow)
- `Confidence` and `Rationale` are audit fields (pass-through from AIAnalysis)
- Method signature takes AIAnalysis as parameter (consistent with Day 2 pattern)

```go
package creator

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// WorkflowExecutionCreator creates WorkflowExecution CRDs from RemediationRequests.
type WorkflowExecutionCreator struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewWorkflowExecutionCreator creates a new WorkflowExecutionCreator.
func NewWorkflowExecutionCreator(c client.Client, s *runtime.Scheme) *WorkflowExecutionCreator {
	return &WorkflowExecutionCreator{
		client: c,
		scheme: s,
	}
}

// Create creates a WorkflowExecution CRD for the given RemediationRequest.
// It uses the selected workflow from the completed AIAnalysis CRD.
// It is idempotent - if the CRD already exists, it returns the existing name.
// Reference: BR-ORCH-025 (workflow data pass-through), BR-ORCH-031 (cascade deletion)
func (c *WorkflowExecutionCreator) Create(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
		"aiAnalysis", ai.Name,
	)

	// Validate preconditions (BR-ORCH-025: "Missing selectedWorkflow → RR marked as Failed")
	if ai.Status.SelectedWorkflow == nil {
		return "", fmt.Errorf("AIAnalysis has no selectedWorkflow")
	}
	if ai.Status.SelectedWorkflow.WorkflowID == "" {
		return "", fmt.Errorf("selectedWorkflow.workflowId is required")
	}
	if ai.Status.SelectedWorkflow.ContainerImage == "" {
		return "", fmt.Errorf("selectedWorkflow.containerImage is required")
	}

	// Generate deterministic name
	name := fmt.Sprintf("we-%s", rr.Name)

	// Check if already exists (idempotency)
	existing := &workflowexecutionv1.WorkflowExecution{}
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, existing)
	if err == nil {
		logger.Info("WorkflowExecution already exists, reusing", "name", name)
		return name, nil
	}
	if !apierrors.IsNotFound(err) {
		logger.Error(err, "Failed to check existing WorkflowExecution")
		return "", fmt.Errorf("failed to check existing WorkflowExecution: %w", err)
	}

	// Build WorkflowExecution CRD
	// BR-ORCH-025: Pass-through from AIAnalysis.Status.SelectedWorkflow
	we := &workflowexecutionv1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
			Labels: map[string]string{
				"kubernaut.ai/remediation-request": rr.Name,
				"kubernaut.ai/component":           "workflow-execution",
			},
		},
		Spec: workflowexecutionv1.WorkflowExecutionSpec{
			// Parent reference for audit trail (REQUIRED)
			RemediationRequestRef: corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rr.Name,
				Namespace:  rr.Namespace,
				UID:        rr.UID,
			},
			// WorkflowRef: Direct pass-through from AIAnalysis (BR-ORCH-025)
			WorkflowRef: workflowexecutionv1.WorkflowRef{
				WorkflowID:      ai.Status.SelectedWorkflow.WorkflowID,
				Version:         ai.Status.SelectedWorkflow.Version, // REQUIRED
				ContainerImage:  ai.Status.SelectedWorkflow.ContainerImage,
				ContainerDigest: ai.Status.SelectedWorkflow.ContainerDigest,
			},
			// TargetResource: String format "namespace/kind/name" (per API contract)
			TargetResource: BuildTargetResourceString(rr),
			// Parameters: Direct pass-through from AIAnalysis
			Parameters: ai.Status.SelectedWorkflow.Parameters,
			// Audit fields from AIAnalysis
			Confidence: ai.Status.SelectedWorkflow.Confidence,
			Rationale:  ai.Status.SelectedWorkflow.Rationale,
			// ExecutionConfig: Optional timeout from RemediationRequest
			ExecutionConfig: c.buildExecutionConfig(rr),
		},
	}

	// Set owner reference for cascade deletion (BR-ORCH-031)
	if err := controllerutil.SetControllerReference(rr, we, c.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference")
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create the CRD
	if err := c.client.Create(ctx, we); err != nil {
		logger.Error(err, "Failed to create WorkflowExecution CRD")
		return "", fmt.Errorf("failed to create WorkflowExecution: %w", err)
	}

	logger.Info("Created WorkflowExecution CRD",
		"name", name,
		"workflowId", ai.Status.SelectedWorkflow.WorkflowID,
		"containerImage", ai.Status.SelectedWorkflow.ContainerImage,
		"targetResource", we.Spec.TargetResource,
	)
	return name, nil
}

// BuildTargetResourceString builds the target resource string for WorkflowExecution.
// Format: "namespace/kind/name" for namespaced resources
//         "kind/name" for cluster-scoped resources (e.g., Node)
// This format is used by WorkflowExecution for resource locking (DD-WE-001).
func BuildTargetResourceString(rr *remediationv1.RemediationRequest) string {
	tr := rr.Spec.TargetResource
	if tr.Namespace != "" {
		return fmt.Sprintf("%s/%s/%s", tr.Namespace, tr.Kind, tr.Name)
	}
	return fmt.Sprintf("%s/%s", tr.Kind, tr.Name)
}

// buildExecutionConfig builds ExecutionConfig from RemediationRequest timeouts.
func (c *WorkflowExecutionCreator) buildExecutionConfig(rr *remediationv1.RemediationRequest) *workflowexecutionv1.ExecutionConfig {
	// Use custom timeout if specified in RemediationRequest
	if rr.Status.TimeoutConfig != nil && rr.Status.TimeoutConfig.WorkflowExecutionTimeout.Duration > 0 {
		return &workflowexecutionv1.ExecutionConfig{
			Timeout: &rr.Status.TimeoutConfig.WorkflowExecutionTimeout,
		}
	}
	// Return nil to use WorkflowExecution controller defaults
	return nil
}
```

---

## Day 4: Notification Creator (8h)

### Approval Notification Creator (BR-ORCH-001)

**File**: `pkg/remediationorchestrator/creator/notification.go`

**API Contract Alignment** (Updated Dec 2025):
- `Type` uses `NotificationTypeApproval` enum (added per NOTICE_NOTIFICATION_TYPE_APPROVAL_ADDITION.md)
- `Subject` and `Body` (NOT `Title` and `Message`)
- `Metadata map[string]string` (NOT `Context` struct)
- `Channels []Channel` (typed enum, NOT `[]string`)
- `Priority NotificationPriority` (typed enum)
- Method receives `AIAnalysis` as parameter (consistent with Day 2-3 pattern)

```go
package creator

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// NotificationCreator creates NotificationRequest CRDs for the Remediation Orchestrator.
type NotificationCreator struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewNotificationCreator creates a new NotificationCreator.
func NewNotificationCreator(c client.Client, s *runtime.Scheme) *NotificationCreator {
	return &NotificationCreator{
		client: c,
		scheme: s,
	}
}

// CreateApprovalNotification creates a NotificationRequest for approval (BR-ORCH-001).
// It receives AIAnalysis as a parameter (consistent with Day 2-3 pattern).
// Reference: BR-ORCH-001 (approval notification), BR-ORCH-031 (cascade deletion), BR-ORCH-035 (ref tracking)
func (c *NotificationCreator) CreateApprovalNotification(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
		"aiAnalysis", ai.Name,
	)

	// Precondition validation (per Day 3 pattern - BR-ORCH-001)
	if ai.Status.SelectedWorkflow == nil {
		logger.Error(nil, "AIAnalysis missing SelectedWorkflow for approval notification")
		return "", fmt.Errorf("AIAnalysis %s/%s missing SelectedWorkflow for approval notification", ai.Namespace, ai.Name)
	}
	if ai.Status.SelectedWorkflow.WorkflowID == "" {
		logger.Error(nil, "AIAnalysis SelectedWorkflow missing WorkflowID")
		return "", fmt.Errorf("AIAnalysis %s/%s SelectedWorkflow missing WorkflowID", ai.Namespace, ai.Name)
	}

	// Generate deterministic name
	name := fmt.Sprintf("nr-approval-%s", rr.Name)

	// Check if already exists (idempotency)
	existing := &notificationv1.NotificationRequest{}
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, existing)
	if err == nil {
		logger.Info("Approval notification already exists, reusing", "name", name)
		return name, nil
	}
	if !apierrors.IsNotFound(err) {
		logger.Error(err, "Failed to check existing NotificationRequest")
		return "", fmt.Errorf("failed to check existing NotificationRequest: %w", err)
	}

	// Determine channels based on context
	channels := c.determineApprovalChannels(rr, ai)

	// Build NotificationRequest for approval
	// API Contract: Uses Subject/Body (not Title/Message), Metadata (not Context)
	nr := &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
			Labels: map[string]string{
				"kubernaut.ai/remediation-request": rr.Name,
				"kubernaut.ai/notification-type":   "approval",
				"kubernaut.ai/severity":            "high", // Approval = high priority
				"kubernaut.ai/environment":         rr.Spec.Environment,
				"kubernaut.ai/component":           "remediation-orchestrator",
			},
		},
		Spec: notificationv1.NotificationRequestSpec{
			Type:     notificationv1.NotificationTypeApproval, // NEW enum value
			Priority: c.mapPriority(rr.Spec.Priority),
			Subject:  fmt.Sprintf("Approval Required: %s", rr.Spec.SignalName),
			Body:     c.buildApprovalBody(rr, ai),
			Channels: channels, // []Channel typed enum
			Metadata: map[string]string{
				"remediationRequest": rr.Name,
				"aiAnalysis":         ai.Name,
				"approvalReason":     ai.Status.ApprovalReason,
				"confidence":         fmt.Sprintf("%.2f", ai.Status.SelectedWorkflow.Confidence),
				"selectedWorkflow":   ai.Status.SelectedWorkflow.WorkflowID,
				"environment":        rr.Spec.Environment,
				"severity":           rr.Spec.Severity,
			},
		},
	}

	// Set owner reference for cascade deletion (BR-ORCH-031)
	if err := controllerutil.SetControllerReference(rr, nr, c.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference")
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create the CRD
	if err := c.client.Create(ctx, nr); err != nil {
		logger.Error(err, "Failed to create approval NotificationRequest")
		return "", fmt.Errorf("failed to create NotificationRequest: %w", err)
	}

	logger.Info("Created approval NotificationRequest",
		"name", name,
		"channels", channels,
		"approvalReason", ai.Status.ApprovalReason,
	)

	// BR-ORCH-035: Caller (reconciler) appends to rr.Status.NotificationRequestRefs
	return name, nil
}

// CreateBulkDuplicateNotification creates a NotificationRequest for bulk duplicates (BR-ORCH-034).
// Reference: BR-ORCH-034 (bulk duplicate notification), BR-ORCH-031 (cascade deletion), BR-ORCH-035 (ref tracking)
func (c *NotificationCreator) CreateBulkDuplicateNotification(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
		"duplicateCount", rr.Status.DuplicateCount,
	)

	// Generate deterministic name
	name := fmt.Sprintf("nr-bulk-%s", rr.Name)

	// Check if already exists (idempotency)
	existing := &notificationv1.NotificationRequest{}
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, existing)
	if err == nil {
		logger.Info("Bulk notification already exists, reusing", "name", name)
		return name, nil
	}
	if !apierrors.IsNotFound(err) {
		logger.Error(err, "Failed to check existing NotificationRequest")
		return "", fmt.Errorf("failed to check existing NotificationRequest: %w", err)
	}

	// Build bulk notification
	// API Contract: Uses Subject/Body (not Title/Message), Metadata (not Context)
	nr := &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
			Labels: map[string]string{
				"kubernaut.ai/remediation-request": rr.Name,
				"kubernaut.ai/notification-type":   "bulk-duplicate",
				"kubernaut.ai/severity":            "low", // Informational
				"kubernaut.ai/environment":         rr.Spec.Environment,
				"kubernaut.ai/component":           "remediation-orchestrator",
			},
		},
		Spec: notificationv1.NotificationRequestSpec{
			Type:     notificationv1.NotificationTypeSimple, // Informational
			Priority: notificationv1.NotificationPriorityLow,
			Subject:  fmt.Sprintf("Remediation Completed with %d Duplicates", rr.Status.DuplicateCount),
			Body:     c.buildBulkDuplicateBody(rr),
			Channels: []notificationv1.Channel{notificationv1.ChannelSlack}, // Lower priority channel
			Metadata: map[string]string{
				"remediationRequest": rr.Name,
				"duplicateCount":     fmt.Sprintf("%d", rr.Status.DuplicateCount),
				"environment":        rr.Spec.Environment,
			},
		},
	}

	// Set owner reference for cascade deletion (BR-ORCH-031)
	if err := controllerutil.SetControllerReference(rr, nr, c.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference")
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create the CRD
	if err := c.client.Create(ctx, nr); err != nil {
		logger.Error(err, "Failed to create bulk NotificationRequest")
		return "", fmt.Errorf("failed to create NotificationRequest: %w", err)
	}

	logger.Info("Created bulk duplicate NotificationRequest", "name", name)

	// BR-ORCH-035: Caller (reconciler) appends to rr.Status.NotificationRequestRefs
	return name, nil
}

// determineApprovalChannels determines notification channels based on context.
// Returns typed Channel slice per API contract.
func (c *NotificationCreator) determineApprovalChannels(
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
) []notificationv1.Channel {
	channels := []notificationv1.Channel{notificationv1.ChannelSlack} // Default

	// High-risk actions or production environment get additional channels
	if ai.Status.ApprovalReason == "high_risk_action" {
		channels = append(channels, notificationv1.ChannelEmail)
	} else if rr.Spec.Environment == "production" {
		channels = append(channels, notificationv1.ChannelEmail)
	}

	return channels
}

// mapPriority maps remediation priority string to NotificationPriority enum.
func (c *NotificationCreator) mapPriority(priority string) notificationv1.NotificationPriority {
	switch priority {
	case "P0":
		return notificationv1.NotificationPriorityCritical
	case "P1":
		return notificationv1.NotificationPriorityHigh
	case "P2":
		return notificationv1.NotificationPriorityMedium
	default:
		return notificationv1.NotificationPriorityLow
	}
}

// buildApprovalBody builds the approval notification body.
// API Contract: Uses rr.Spec fields directly (NOT rr.Spec.SignalData)
// Uses ai.Status fields: RootCause, SelectedWorkflow, ApprovalContext
func (c *NotificationCreator) buildApprovalBody(rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) string {
	// Safely get root cause - prefer RootCauseAnalysis.Summary, fall back to RootCause
	rootCause := ai.Status.RootCause
	if ai.Status.RootCauseAnalysis != nil && ai.Status.RootCauseAnalysis.Summary != "" {
		rootCause = ai.Status.RootCauseAnalysis.Summary
	}

	// Safely get approval reason
	approvalReason := ai.Status.ApprovalReason
	if ai.Status.ApprovalContext != nil && ai.Status.ApprovalContext.Reason != "" {
		approvalReason = ai.Status.ApprovalContext.Reason
	}

	return fmt.Sprintf(`
Remediation requires approval:

**Signal**: %s
**Severity**: %s
**Environment**: %s

**Root Cause Analysis**:
%s

**Confidence**: %.0f%%

**Proposed Workflow**: %s

**Approval Reason**: %s

Please review and approve/reject the remediation.
`,
		rr.Spec.SignalName,          // FIXED: Direct field on Spec
		rr.Spec.Severity,            // FIXED: Direct field on Spec
		rr.Spec.Environment,
		rootCause,                   // FIXED: Uses ai.Status.RootCause or RootCauseAnalysis.Summary
		ai.Status.SelectedWorkflow.Confidence*100, // FIXED: Confidence is on SelectedWorkflow
		ai.Status.SelectedWorkflow.WorkflowID,
		approvalReason,              // FIXED: Uses ai.Status.ApprovalReason or ApprovalContext.Reason
	)
}

// buildBulkDuplicateBody builds the bulk duplicate notification body.
// RENAMED from buildBulkDuplicateMessage to match caller (line 699)
// API Contract: Uses rr.Spec fields directly (NOT rr.Spec.SignalData)
func (c *NotificationCreator) buildBulkDuplicateBody(rr *remediationv1.RemediationRequest) string {
	return fmt.Sprintf(`
Remediation completed successfully.

**Signal**: %s
**Result**: %s

**Duplicate Remediations**: %d
The following remediations were skipped as duplicates:
%v

All duplicate signals have been handled by this remediation.
`,
		rr.Spec.SignalName,          // FIXED: Direct field on Spec
		rr.Status.Phase,             // FIXED: Phase, not OverallPhase
		rr.Status.DuplicateCount,
		rr.Status.DuplicateRefs,
	)
}

// calculateRequiredBy calculates approval deadline
func (c *NotificationCreator) calculateRequiredBy(rr *remediationv1.RemediationRequest) *metav1.Time {
	// Default: 1 hour for approval
	deadline := metav1.NewTime(time.Now().Add(1 * time.Hour))
	return &deadline
}
```

---

### Day 4: TDD Test Plan (MANDATORY)

**File**: `test/unit/remediationorchestrator/notification_creator_test.go`

**Test Coverage Matrix** (per `03-testing-strategy.mdc` and `TESTING_GUIDELINES.md`):

| # | Test Case | BR Reference | Type | Priority |
|---|-----------|--------------|------|----------|
| 1 | Constructor returns non-nil `NotificationCreator` | — | Unit | P0 |
| 2 | `CreateApprovalNotification` generates deterministic name `nr-approval-{rr.Name}` | BR-ORCH-001 | Unit | P0 |
| 3 | `CreateApprovalNotification` sets owner reference for cascade deletion | BR-ORCH-031 | Unit | P0 |
| 4 | `CreateApprovalNotification` is idempotent (existing returns same name) | BR-ORCH-001 AC-001-2 | Unit | P0 |
| 5 | `CreateApprovalNotification` uses correct API fields (`rr.Spec.SignalName`, etc.) | BR-ORCH-001, BR-ORCH-025 | Unit | P0 |
| 6 | `CreateApprovalNotification` returns error for nil `SelectedWorkflow` | BR-ORCH-001 | Unit | P0 |
| 7 | `CreateApprovalNotification` returns error for empty `WorkflowID` | BR-ORCH-001 | Unit | P0 |
| 8 | `CreateApprovalNotification` sets correct labels for routing | BR-NOT-065 | Unit | P1 |
| 9 | `CreateApprovalNotification` uses `NotificationTypeApproval` enum | BR-ORCH-001 | Unit | P0 |
| 10 | `CreateBulkDuplicateNotification` generates deterministic name `nr-bulk-{rr.Name}` | BR-ORCH-034 | Unit | P0 |
| 11 | `CreateBulkDuplicateNotification` sets owner reference for cascade deletion | BR-ORCH-031 | Unit | P0 |
| 12 | `CreateBulkDuplicateNotification` is idempotent | BR-ORCH-034 | Unit | P0 |
| 13 | `CreateBulkDuplicateNotification` uses `NotificationTypeSimple` enum | BR-ORCH-034 | Unit | P1 |
| 14 | `mapPriority` maps P0→Critical, P1→High, P2→Medium, default→Low (DescribeTable) | BR-ORCH-001 | Unit | P1 |
| 15 | `determineApprovalChannels` returns Slack for default | BR-ORCH-001 | Unit | P1 |
| 16 | `determineApprovalChannels` adds Email for high_risk_action | BR-ORCH-001 | Unit | P1 |
| 17 | `determineApprovalChannels` adds Email for production environment | BR-ORCH-001 | Unit | P1 |
| 18 | Client Get error propagates correctly | — | Unit | P2 |
| 19 | Client Create error propagates correctly | — | Unit | P2 |

**Test Structure** (Ginkgo/Gomega BDD):

```go
var _ = Describe("NotificationCreator", func() {
    var (
        fakeClient client.Client
        scheme     *runtime.Scheme
        nc         *creator.NotificationCreator
        ctx        context.Context
    )

    BeforeEach(func() {
        scheme = runtime.NewScheme()
        _ = remediationv1.AddToScheme(scheme)
        _ = notificationv1.AddToScheme(scheme)
        _ = aianalysisv1.AddToScheme(scheme)
        fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
        nc = creator.NewNotificationCreator(fakeClient, scheme)
        ctx = context.Background()
    })

    Describe("Constructor", func() {
        It("should return non-nil NotificationCreator", func() {
            Expect(nc).ToNot(BeNil())
        })
    })

    Describe("CreateApprovalNotification", func() {
        Context("BR-ORCH-001: Approval Notification Creation", func() {
            It("should generate deterministic name nr-approval-{rr.Name}", func() {
                rr := testutil.NewRemediationRequest("test-rr", "default")
                ai := testutil.NewCompletedAIAnalysis("test-ai", "default")
                name, err := nc.CreateApprovalNotification(ctx, rr, ai)
                Expect(err).ToNot(HaveOccurred())
                Expect(name).To(Equal("nr-approval-test-rr"))
            })

            It("should be idempotent - return existing name without error", func() {
                // ... idempotency test
            })
        })

        Context("BR-ORCH-031: Cascade Deletion", func() {
            It("should set owner reference to RemediationRequest", func() {
                // ... owner reference test
            })
        })

        Context("Precondition Validation", func() {
            It("should return error when SelectedWorkflow is nil", func() {
                rr := testutil.NewRemediationRequest("test-rr", "default")
                ai := testutil.NewCompletedAIAnalysis("test-ai", "default")
                ai.Status.SelectedWorkflow = nil
                _, err := nc.CreateApprovalNotification(ctx, rr, ai)
                Expect(err).To(HaveOccurred())
                Expect(err.Error()).To(ContainSubstring("missing SelectedWorkflow"))
            })

            It("should return error when WorkflowID is empty", func() {
                rr := testutil.NewRemediationRequest("test-rr", "default")
                ai := testutil.NewCompletedAIAnalysis("test-ai", "default")
                ai.Status.SelectedWorkflow.WorkflowID = ""
                _, err := nc.CreateApprovalNotification(ctx, rr, ai)
                Expect(err).To(HaveOccurred())
                Expect(err.Error()).To(ContainSubstring("missing WorkflowID"))
            })
        })
    })

    Describe("mapPriority", func() {
        DescribeTable("priority to notification priority mapping",
            func(input string, expected notificationv1.NotificationPriority) {
                result := nc.MapPriority(input) // Exported for testing
                Expect(result).To(Equal(expected))
            },
            Entry("P0 → Critical", "P0", notificationv1.NotificationPriorityCritical),
            Entry("P1 → High", "P1", notificationv1.NotificationPriorityHigh),
            Entry("P2 → Medium", "P2", notificationv1.NotificationPriorityMedium),
            Entry("P3 → Low", "P3", notificationv1.NotificationPriorityLow),
            Entry("unknown → Low", "unknown", notificationv1.NotificationPriorityLow),
            Entry("empty → Low", "", notificationv1.NotificationPriorityLow),
        )
    })

    Describe("determineApprovalChannels", func() {
        DescribeTable("channel determination based on context",
            func(environment, approvalReason string, expectedChannels []notificationv1.Channel) {
                rr := testutil.NewRemediationRequest("test-rr", "default")
                rr.Spec.Environment = environment
                ai := testutil.NewCompletedAIAnalysis("test-ai", "default")
                ai.Status.ApprovalReason = approvalReason
                channels := nc.DetermineApprovalChannels(rr, ai) // Exported for testing
                Expect(channels).To(ConsistOf(expectedChannels))
            },
            Entry("default → Slack only", "dev", "low_confidence",
                []notificationv1.Channel{notificationv1.ChannelSlack}),
            Entry("high_risk_action → Slack + Email", "dev", "high_risk_action",
                []notificationv1.Channel{notificationv1.ChannelSlack, notificationv1.ChannelEmail}),
            Entry("production → Slack + Email", "production", "low_confidence",
                []notificationv1.Channel{notificationv1.ChannelSlack, notificationv1.ChannelEmail}),
        )
    })

    Describe("Error Handling", func() {
        DescribeTable("client errors propagate correctly",
            func(operation string, setupError func(), expectedErrorSubstring string) {
                setupError()
                rr := testutil.NewRemediationRequest("test-rr", "default")
                ai := testutil.NewCompletedAIAnalysis("test-ai", "default")
                _, err := nc.CreateApprovalNotification(ctx, rr, ai)
                Expect(err).To(HaveOccurred())
                Expect(err.Error()).To(ContainSubstring(expectedErrorSubstring))
            },
            // Entries for Get error, Create error using interceptor funcs
        )
    })
})
```

---

## Day 5: Status Aggregation and WE Failure Handling (8h)

**Updated**: December 6, 2025 - v1.5: Fixed API field names, added imports, added test plan

> **📋 Day 5 Changelog**
> | Version | Date | Changes |
> |---------|------|---------|
> | v1.5 | 2025-12-06 | **CRITICAL FIXES**: `rr.Status.Phase` → `rr.Status.OverallPhase`; Added `SignalProcessingRef` to API; Added `RequiresManualReview` field; Fixed missing imports; Added test plan with BR mappings |
> | v1.4 | 2025-12-06 | Added WE failure handling per DD-WE-004 |

**Files**:
- `pkg/remediationorchestrator/aggregator/status.go`
- `pkg/remediationorchestrator/handler/workflowexecution.go` (NEW)

**Business Requirements**: BR-ORCH-032 (skip handling), BR-ORCH-033 (duplicate tracking), BR-ORCH-036 (manual review notification)

**Design Decisions**: DD-RO-001 (deduplication), DD-WE-004 (exponential backoff)

**API Updates Required** (added in v1.5):
- `RemediationRequestStatus.SignalProcessingRef` - reference to SP CRD
- `RemediationRequestStatus.RequiresManualReview` - boolean for manual review needed

---

### WorkflowExecution Skip/Failure Handler (DD-WE-004)

**File**: `pkg/remediationorchestrator/handler/workflowexecution.go`

```go
package handler

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// WorkflowExecutionHandler handles WE status changes
// Reference: BR-ORCH-032, BR-ORCH-033, BR-ORCH-036, DD-WE-004
type WorkflowExecutionHandler struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewWorkflowExecutionHandler creates a new handler
func NewWorkflowExecutionHandler(c client.Client, s *runtime.Scheme) *WorkflowExecutionHandler {
	return &WorkflowExecutionHandler{
		client: c,
		scheme: s,
	}
}

// HandleSkipped handles WE Skipped phase per DD-WE-004 and BR-ORCH-032
// NOTE: Uses rr.Status.OverallPhase (NOT Phase)
func (h *WorkflowExecutionHandler) HandleSkipped(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	we *workflowexecutionv1.WorkflowExecution,
	sp *signalprocessingv1.SignalProcessing, // v1.5: Pass SP for environment labels
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"workflowExecution", we.Name,
		"skipReason", we.Status.SkipDetails.Reason,
	)

	reason := we.Status.SkipDetails.Reason

	switch reason {
	case "ResourceBusy":
		// DUPLICATE: Another workflow running - requeue
		logger.Info("WE skipped: ResourceBusy - tracking as duplicate, requeueing")
		if err := h.trackDuplicate(ctx, rr, we); err != nil {
			return ctrl.Result{}, err
		}
		rr.Status.OverallPhase = "Skipped" // v1.5: FIXED from Phase
		rr.Status.SkipReason = reason
		rr.Status.DuplicateOf = we.Status.SkipDetails.ConflictingWorkflow.RemediationRequestRef
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil

	case "RecentlyRemediated":
		// DUPLICATE: Cooldown active - requeue with fixed interval
		// Per WE Team Response Q6: RO should NOT calculate backoff, let WE re-evaluate
		logger.Info("WE skipped: RecentlyRemediated - tracking as duplicate, requeueing")
		if err := h.trackDuplicate(ctx, rr, we); err != nil {
			return ctrl.Result{}, err
		}
		rr.Status.OverallPhase = "Skipped" // v1.5: FIXED from Phase
		rr.Status.SkipReason = reason
		rr.Status.DuplicateOf = we.Status.SkipDetails.RecentRemediation.RemediationRequestRef

		// Fixed interval requeue - WE owns backoff logic (DD-WE-004, Q6)
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil

	case "ExhaustedRetries":
		// NOT A DUPLICATE: Manual review required
		logger.Info("WE skipped: ExhaustedRetries - manual intervention required")
		return h.handleManualReviewRequired(ctx, rr, we, sp, reason,
			"Retry limit exceeded - 5+ consecutive pre-execution failures")

	case "PreviousExecutionFailed":
		// NOT A DUPLICATE: Manual review required (cluster state unknown)
		logger.Info("WE skipped: PreviousExecutionFailed - manual intervention required")
		return h.handleManualReviewRequired(ctx, rr, we, sp, reason,
			"Previous execution failed during workflow run - cluster state may be inconsistent")

	default:
		logger.Error(nil, "Unknown skip reason", "reason", reason)
		return ctrl.Result{}, fmt.Errorf("unknown skip reason: %s", reason)
	}
}

// HandleFailed handles WE Failed phase
// NOTE: Uses rr.Status.OverallPhase (NOT Phase), rr.Status.RequiresManualReview
func (h *WorkflowExecutionHandler) HandleFailed(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	we *workflowexecutionv1.WorkflowExecution,
	sp *signalprocessingv1.SignalProcessing, // v1.5: Pass SP for environment labels
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"workflowExecution", we.Name,
		"wasExecutionFailure", we.Status.FailureDetails.WasExecutionFailure,
	)

	if we.Status.FailureDetails.WasExecutionFailure {
		// EXECUTION FAILURE: Cluster state may be modified - NO auto-retry
		logger.Info("WE failed during execution - manual review required")

		rr.Status.OverallPhase = "Failed"         // v1.5: FIXED from Phase
		rr.Status.RequiresManualReview = true     // v1.5: NEW field added to API
		rr.Status.Message = we.Status.FailureDetails.NaturalLanguageSummary

		// Create escalation notification with naturalLanguageSummary
		if err := h.createExecutionFailureNotification(ctx, rr, we, sp); err != nil {
			logger.Error(err, "Failed to create execution failure notification")
			return ctrl.Result{}, err
		}

		// NO requeue - manual intervention required
		return ctrl.Result{}, nil
	}

	// PRE-EXECUTION FAILURE: May consider recovery
	logger.Info("WE failed during pre-execution - may consider recovery")
	return h.evaluateRecoveryOptions(ctx, rr, we)
}

// handleManualReviewRequired handles skip reasons requiring manual intervention
// v1.5: Pass SP for environment; use OverallPhase; use RequiresManualReview
func (h *WorkflowExecutionHandler) handleManualReviewRequired(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	we *workflowexecutionv1.WorkflowExecution,
	sp *signalprocessingv1.SignalProcessing, // v1.5: Pass SP for environment
	skipReason string,
	message string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// v1.5: Get environment from SP status (per NOTICE_RO_REMEDIATIONREQUEST_SCHEMA_UPDATE)
	environment := "unknown"
	if sp != nil && sp.Status.EnvironmentClassification != nil {
		environment = sp.Status.EnvironmentClassification.Environment
	}

	// Update RR status - FAILED, not Skipped (per BR-ORCH-032 v1.1)
	rr.Status.OverallPhase = "Failed"      // v1.5: FIXED from Phase
	rr.Status.SkipReason = skipReason
	rr.Status.RequiresManualReview = true  // v1.5: NEW field added to API
	rr.Status.DuplicateOf = ""             // NOT a duplicate
	rr.Status.Message = we.Status.SkipDetails.Message

	// Create manual review notification (BR-ORCH-036)
	nr := &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("nr-manual-review-%s", rr.Name),
			Namespace: rr.Namespace,
			Labels: map[string]string{
				"kubernaut.ai/remediation-request": rr.Name,
				"kubernaut.ai/notification-type":   "manual-review",
				"kubernaut.ai/skip-reason":         skipReason,
				"kubernaut.ai/severity":            h.mapSkipReasonToSeverity(skipReason),
				"kubernaut.ai/environment":         environment, // v1.5: FIXED - from SP status
				"kubernaut.ai/component":           "remediation-orchestrator",
			},
		},
		Spec: notificationv1.NotificationRequestSpec{
			Type:     notificationv1.NotificationTypeManualReview, // BR-ORCH-036
			Priority: h.mapSkipReasonToPriority(skipReason),
			Subject:  fmt.Sprintf("⚠️ Manual Review Required: %s - %s", rr.Name, skipReason),
			Body:     h.buildManualReviewBody(rr, we, skipReason, message),
			Channels: []notificationv1.Channel{
				notificationv1.ChannelConsole,
				notificationv1.ChannelSlack,
				notificationv1.ChannelEmail,
			},
			Metadata: map[string]string{
				"remediationRequest": rr.Name,
				"workflowExecution":  we.Name,
				"skipReason":         skipReason,
				"targetResource":     we.Spec.TargetResource,
				"environment":        environment, // v1.5: From SP status
			},
		},
	}

	if err := controllerutil.SetControllerReference(rr, nr, h.scheme); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set owner reference: %w", err)
	}

	if err := h.client.Create(ctx, nr); err != nil {
		logger.Error(err, "Failed to create manual review notification")
		return ctrl.Result{}, err
	}

	logger.Info("Created manual review notification", "notification", nr.Name)

	// NO requeue - manual intervention required
	return ctrl.Result{}, nil
}

// trackDuplicate tracks a duplicate RR on the parent (BR-ORCH-033)
func (h *WorkflowExecutionHandler) trackDuplicate(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	we *workflowexecutionv1.WorkflowExecution,
) error {
	// Implementation as in Day 5 original content
	// ... (get parent RR, increment duplicate count, append to refs)
	return nil
}

// calculateRequeueTime calculates requeue duration from NextAllowedExecution
func (h *WorkflowExecutionHandler) calculateRequeueTime(nextAllowed *metav1.Time) time.Duration {
	if nextAllowed == nil {
		return 1 * time.Minute // Default fallback
	}
	duration := time.Until(nextAllowed.Time)
	if duration < 0 {
		return 0 // Already expired, requeue immediately
	}
	return duration
}

// mapSkipReasonToSeverity maps skip reason to severity label per Notification team guidance
// PreviousExecutionFailed = critical (cluster state unknown)
// ExhaustedRetries = high (infrastructure issue, but state is known)
func (h *WorkflowExecutionHandler) mapSkipReasonToSeverity(skipReason string) string {
	switch skipReason {
	case "PreviousExecutionFailed":
		return "critical"
	case "ExhaustedRetries":
		return "high"
	default:
		return "medium"
	}
}

// mapSkipReasonToPriority maps skip reason to NotificationPriority per Notification team guidance
func (h *WorkflowExecutionHandler) mapSkipReasonToPriority(skipReason string) notificationv1.NotificationPriority {
	switch skipReason {
	case "PreviousExecutionFailed":
		return notificationv1.NotificationPriorityCritical
	case "ExhaustedRetries":
		return notificationv1.NotificationPriorityHigh
	default:
		return notificationv1.NotificationPriorityMedium
	}
}

// buildManualReviewBody builds the notification body for manual review
func (h *WorkflowExecutionHandler) buildManualReviewBody(
	rr *remediationv1.RemediationRequest,
	we *workflowexecutionv1.WorkflowExecution,
	skipReason string,
	message string,
) string {
	return fmt.Sprintf(`
A workflow has been blocked and requires manual review.

**Remediation Request:** %s
**Workflow Execution:** %s
**Target Resource:** %s
**Skip Reason:** %s

**Details:**
%s

**WE Message:**
%s

**Required Action:**
1. Investigate the underlying issue
2. Verify cluster state is correct
3. Clear the block (see documentation)
4. Retry manually if appropriate
`,
		rr.Name,
		we.Name,
		we.Spec.TargetResource,
		skipReason,
		message,
		we.Status.SkipDetails.Message,
	)
}
```

---

---

### Day 5: TDD Test Plan (MANDATORY)

**File**: `test/unit/remediationorchestrator/workflowexecution_handler_test.go`

**Test Coverage Matrix** (per `03-testing-strategy.mdc` and `TESTING_GUIDELINES.md`):

| # | Test Case | BR Reference | Type | Priority |
|---|-----------|--------------|------|----------|
| 1 | Constructor returns non-nil `WorkflowExecutionHandler` | — | Unit | P0 |
| 2 | `HandleSkipped` with ResourceBusy sets OverallPhase="Skipped" and requeues | BR-ORCH-032 | Unit | P0 |
| 3 | `HandleSkipped` with RecentlyRemediated sets OverallPhase="Skipped" and requeues | BR-ORCH-032 | Unit | P0 |
| 4 | `HandleSkipped` with ExhaustedRetries sets OverallPhase="Failed" + RequiresManualReview | BR-ORCH-032, BR-ORCH-036 | Unit | P0 |
| 5 | `HandleSkipped` with PreviousExecutionFailed sets OverallPhase="Failed" + RequiresManualReview | BR-ORCH-032, BR-ORCH-036 | Unit | P0 |
| 6 | `HandleSkipped` creates manual review notification for ExhaustedRetries | BR-ORCH-036 | Unit | P0 |
| 7 | `HandleSkipped` creates manual review notification for PreviousExecutionFailed | BR-ORCH-036 | Unit | P0 |
| 8 | `HandleFailed` with WasExecutionFailure sets RequiresManualReview=true | BR-ORCH-032, DD-WE-004 | Unit | P0 |
| 9 | `HandleFailed` without WasExecutionFailure calls evaluateRecoveryOptions | BR-ORCH-032 | Unit | P1 |
| 10 | `trackDuplicate` increments DuplicateCount on parent RR | BR-ORCH-033 | Unit | P0 |
| 11 | `trackDuplicate` appends to DuplicateRefs | BR-ORCH-033 | Unit | P0 |
| 12 | `mapSkipReasonToSeverity` mapping (DescribeTable) | BR-ORCH-036 | Unit | P1 |
| 13 | `mapSkipReasonToPriority` mapping (DescribeTable) | BR-ORCH-036 | Unit | P1 |
| 14 | Manual review notification uses environment from SP status | BR-ORCH-036, NOTICE | Unit | P0 |
| 15 | Manual review notification sets `NotificationTypeManualReview` | BR-ORCH-036 | Unit | P0 |

**DescribeTable Tests** (per `03-testing-strategy.mdc`):

```go
// Test mapSkipReasonToSeverity mapping
DescribeTable("BR-ORCH-036: Skip reason to severity mapping",
    func(skipReason string, expectedSeverity string) {
        handler := NewWorkflowExecutionHandler(fakeClient, scheme)
        severity := handler.mapSkipReasonToSeverity(skipReason)
        Expect(severity).To(Equal(expectedSeverity))
    },
    Entry("PreviousExecutionFailed → critical", "PreviousExecutionFailed", "critical"),
    Entry("ExhaustedRetries → high", "ExhaustedRetries", "high"),
    Entry("ResourceBusy → medium", "ResourceBusy", "medium"),
    Entry("RecentlyRemediated → medium", "RecentlyRemediated", "medium"),
    Entry("unknown → medium", "unknown", "medium"),
)

// Test mapSkipReasonToPriority mapping
DescribeTable("BR-ORCH-036: Skip reason to priority mapping",
    func(skipReason string, expectedPriority notificationv1.NotificationPriority) {
        handler := NewWorkflowExecutionHandler(fakeClient, scheme)
        priority := handler.mapSkipReasonToPriority(skipReason)
        Expect(priority).To(Equal(expectedPriority))
    },
    Entry("PreviousExecutionFailed → Critical", "PreviousExecutionFailed", notificationv1.NotificationPriorityCritical),
    Entry("ExhaustedRetries → High", "ExhaustedRetries", notificationv1.NotificationPriorityHigh),
    Entry("ResourceBusy → Medium", "ResourceBusy", notificationv1.NotificationPriorityMedium),
)
```

**File**: `test/unit/remediationorchestrator/status_aggregator_test.go`

| # | Test Case | BR Reference | Type | Priority |
|---|-----------|--------------|------|----------|
| 16 | Constructor returns non-nil `StatusAggregator` | — | Unit | P0 |
| 17 | `AggregateStatus` returns SP status when SignalProcessingRef set | BR-ORCH-025 | Unit | P0 |
| 18 | `AggregateStatus` returns AI status when AIAnalysisRef set | BR-ORCH-025 | Unit | P0 |
| 19 | `AggregateStatus` returns WE status when WorkflowExecutionRef set | BR-ORCH-025 | Unit | P0 |
| 20 | `AggregateStatus` handles missing child CRDs gracefully | — | Unit | P1 |
| 21 | `AggregateStatus` aggregates error from child CRDs | — | Unit | P1 |

---

### Status Aggregator

**File**: `pkg/remediationorchestrator/aggregator/status.go`

```go
package aggregator

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// AggregatedStatus holds collected status from all child CRDs
type AggregatedStatus struct {
	SignalProcessingPhase string
	SignalProcessingReady bool
	AIAnalysisPhase       string
	AIAnalysisReady       bool
	WorkflowExecutionPhase string
	WorkflowExecutionReady bool
	EnrichmentResults     interface{}
	Error                 error
}

// StatusAggregator aggregates status from child CRDs
type StatusAggregator struct {
	client client.Client
}

// NewStatusAggregator creates a new status aggregator
func NewStatusAggregator(c client.Client) *StatusAggregator {
	return &StatusAggregator{client: c}
}

// AggregateStatus collects status from all child CRDs
// v1.5: Uses SignalProcessingRef (ObjectReference), not string
func (a *StatusAggregator) AggregateStatus(ctx context.Context, rr *remediationv1.RemediationRequest) (*AggregatedStatus, error) {
	logger := log.FromContext(ctx)

	status := &AggregatedStatus{}

	// Aggregate SignalProcessing status
	// v1.5: SignalProcessingRef is now *corev1.ObjectReference
	if rr.Status.SignalProcessingRef != nil && rr.Status.SignalProcessingRef.Name != "" {
		spStatus, err := a.getSignalProcessingStatus(ctx, rr.Namespace, rr.Status.SignalProcessingRef.Name)
		if err != nil {
			logger.Error(err, "Failed to get SignalProcessing status")
			status.Error = err
		} else {
			status.SignalProcessingPhase = spStatus.Phase
			status.SignalProcessingReady = spStatus.Phase == "Completed"
			status.EnrichmentResults = spStatus.EnrichmentResults
		}
	}

	// Aggregate AIAnalysis status
	if rr.Status.AIAnalysisRef != nil && rr.Status.AIAnalysisRef.Name != "" {
		aiStatus, err := a.getAIAnalysisStatus(ctx, rr.Namespace, rr.Status.AIAnalysisRef.Name)
		if err != nil {
			logger.Error(err, "Failed to get AIAnalysis status")
			status.Error = err
		} else {
			status.AIAnalysisPhase = aiStatus.Phase
			status.AIAnalysisReady = aiStatus.Phase == "Completed" || aiStatus.Phase == "Approved"
			status.RequiresApproval = aiStatus.RequiresApproval
			status.SelectedWorkflow = aiStatus.SelectedWorkflow
		}
	}

	// Aggregate WorkflowExecution status
	if rr.Status.WorkflowExecutionRef != "" {
		weStatus, err := a.getWorkflowExecutionStatus(ctx, rr.Namespace, rr.Status.WorkflowExecutionRef)
		if err != nil {
			log.Error(err, "Failed to get WorkflowExecution status")
			status.Error = err
		} else {
			status.WorkflowExecutionPhase = weStatus.Phase
			status.WorkflowExecutionReady = weStatus.Phase == "Succeeded" || weStatus.Phase == "Failed" || weStatus.Phase == "Skipped"
			status.ExecutionSkipped = weStatus.Phase == "Skipped"
			if weStatus.SkipDetails != nil {
				status.SkipReason = weStatus.SkipDetails.Reason
				status.DuplicateOf = weStatus.SkipDetails.ActiveRemediationRef
			}
		}
	}

	// Calculate overall readiness
	status.OverallReady = a.calculateOverallReadiness(status, rr)

	return status, nil
}

// getSignalProcessingStatus fetches SignalProcessing status
func (a *StatusAggregator) getSignalProcessingStatus(ctx context.Context, namespace, name string) (*signalprocessingv1.SignalProcessingStatus, error) {
	sp := &signalprocessingv1.SignalProcessing{}
	if err := a.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, sp); err != nil {
		return nil, err
	}
	return &sp.Status, nil
}

// getAIAnalysisStatus fetches AIAnalysis status
func (a *StatusAggregator) getAIAnalysisStatus(ctx context.Context, namespace, name string) (*aianalysisv1.AIAnalysisStatus, error) {
	ai := &aianalysisv1.AIAnalysis{}
	if err := a.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, ai); err != nil {
		return nil, err
	}
	return &ai.Status, nil
}

// getWorkflowExecutionStatus fetches WorkflowExecution status
func (a *StatusAggregator) getWorkflowExecutionStatus(ctx context.Context, namespace, name string) (*workflowexecutionv1.WorkflowExecutionStatus, error) {
	we := &workflowexecutionv1.WorkflowExecution{}
	if err := a.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, we); err != nil {
		return nil, err
	}
	return &we.Status, nil
}

// calculateOverallReadiness determines if the remediation is ready for next phase
func (a *StatusAggregator) calculateOverallReadiness(status *orchestrator.AggregatedStatus, rr *remediationv1.RemediationRequest) bool {
	// If there's an error, not ready
	if status.Error != nil {
		return false
	}

	// Based on current phase, check appropriate child readiness
	switch rr.Status.OverallPhase {
	case "Processing":
		return status.SignalProcessingReady
	case "Analyzing":
		return status.AIAnalysisReady
	case "AwaitingApproval":
		return rr.Status.ApprovalDecision != ""
	case "Executing":
		return status.WorkflowExecutionReady
	default:
		return true
	}
}
```

---

## Day 6: Timeout Detection (8h)

> **📋 Changelog**
> | Version | Date | Changes |
> |---------|------|---------|
> | **v1.6** | 2025-12-06 | **CRITICAL FIX**: Corrected package path to `pkg/remediationorchestrator/timeout/`, fixed TimeoutConfig usage, added phase start time fields to API, added test plan with BR mappings, added DescribeTable examples |
> | v1.0 | 2025-12-04 | Initial version |

**File**: `pkg/remediationorchestrator/timeout/detector.go`

**API Prerequisite** (Added in v1.6):
New fields added to `RemediationRequestStatus`:
- `ProcessingStartTime` - When SignalProcessing phase started
- `AnalyzingStartTime` - When AIAnalysis phase started
- `ExecutingStartTime` - When WorkflowExecution phase started

**Per-Remediation Override**:
Uses existing `rr.Status.TimeoutConfig.OverallWorkflowTimeout` (not a separate `GlobalTimeout` field).

### Implementation

```go
package timeout

import (
	"time"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator"
)

// Terminal phases that should skip timeout checks
var terminalPhases = map[string]bool{
	"Completed": true,
	"Failed":    true,
	"Timeout":   true,
	"Skipped":   true,
}

// TimeoutResult contains information about a detected timeout
type TimeoutResult struct {
	TimedOut     bool
	TimedOutPhase string        // "global", "processing", "analyzing", "executing"
	Elapsed      time.Duration
}

// Detector detects phase and global timeouts.
// Reference: BR-ORCH-027 (global timeout), BR-ORCH-028 (per-phase timeout)
type Detector struct {
	config remediationorchestrator.OrchestratorConfig
}

// NewDetector creates a new timeout detector.
func NewDetector(config remediationorchestrator.OrchestratorConfig) *Detector {
	return &Detector{config: config}
}

// CheckTimeout checks if global or phase timeout has been exceeded.
// Global timeout (BR-ORCH-027) is checked first, then per-phase (BR-ORCH-028).
// Returns TimeoutResult with details about the timeout, or TimedOut=false if no timeout.
func (d *Detector) CheckTimeout(rr *remediationv1.RemediationRequest) TimeoutResult {
	currentPhase := rr.Status.OverallPhase

	// Skip if terminal state
	if d.IsTerminalPhase(currentPhase) {
		return TimeoutResult{TimedOut: false}
	}

	// Check global timeout first (BR-ORCH-027)
	if result := d.CheckGlobalTimeout(rr); result.TimedOut {
		return result
	}

	// Check per-phase timeout (BR-ORCH-028)
	return d.CheckPhaseTimeout(rr)
}

// CheckGlobalTimeout checks if global timeout has been exceeded (BR-ORCH-027).
func (d *Detector) CheckGlobalTimeout(rr *remediationv1.RemediationRequest) TimeoutResult {
	elapsed := time.Since(rr.CreationTimestamp.Time)

	// Get global timeout from config or per-remediation override
	globalTimeout := d.config.Timeouts.Global
	if rr.Status.TimeoutConfig != nil && rr.Status.TimeoutConfig.OverallWorkflowTimeout.Duration > 0 {
		globalTimeout = rr.Status.TimeoutConfig.OverallWorkflowTimeout.Duration
	}

	if elapsed > globalTimeout {
		return TimeoutResult{
			TimedOut:      true,
			TimedOutPhase: "global",
			Elapsed:       elapsed,
		}
	}

	return TimeoutResult{TimedOut: false}
}

// CheckPhaseTimeout checks if current phase has timed out (BR-ORCH-028).
func (d *Detector) CheckPhaseTimeout(rr *remediationv1.RemediationRequest) TimeoutResult {
	currentPhase := rr.Status.OverallPhase

	// Get phase start time based on current phase
	var phaseStartTime *time.Time
	switch currentPhase {
	case "Processing":
		if rr.Status.ProcessingStartTime != nil {
			t := rr.Status.ProcessingStartTime.Time
			phaseStartTime = &t
		}
	case "Analyzing", "AwaitingApproval":
		if rr.Status.AnalyzingStartTime != nil {
			t := rr.Status.AnalyzingStartTime.Time
			phaseStartTime = &t
		}
	case "Executing":
		if rr.Status.ExecutingStartTime != nil {
			t := rr.Status.ExecutingStartTime.Time
			phaseStartTime = &t
		}
	}

	if phaseStartTime == nil {
		return TimeoutResult{TimedOut: false}
	}

	// Get timeout for current phase (with per-remediation override)
	timeout := d.GetPhaseTimeout(rr, currentPhase)
	elapsed := time.Since(*phaseStartTime)

	if elapsed > timeout {
		return TimeoutResult{
			TimedOut:      true,
			TimedOutPhase: currentPhase,
			Elapsed:       elapsed,
		}
	}

	return TimeoutResult{TimedOut: false}
}

// GetPhaseTimeout returns the configured timeout for a phase.
// Checks per-remediation override first, then falls back to global config.
// Reference: BR-ORCH-028
func (d *Detector) GetPhaseTimeout(rr *remediationv1.RemediationRequest, phase string) time.Duration {
	// Check per-remediation override first
	if rr.Status.TimeoutConfig != nil {
		switch phase {
		case "Processing":
			if rr.Status.TimeoutConfig.RemediationProcessingTimeout.Duration > 0 {
				return rr.Status.TimeoutConfig.RemediationProcessingTimeout.Duration
			}
		case "Analyzing", "AwaitingApproval":
			if rr.Status.TimeoutConfig.AIAnalysisTimeout.Duration > 0 {
				return rr.Status.TimeoutConfig.AIAnalysisTimeout.Duration
			}
		case "Executing":
			if rr.Status.TimeoutConfig.WorkflowExecutionTimeout.Duration > 0 {
				return rr.Status.TimeoutConfig.WorkflowExecutionTimeout.Duration
			}
		}
	}

	// Fall back to global config defaults
	switch phase {
	case "Processing":
		return d.config.Timeouts.Processing
	case "Analyzing", "AwaitingApproval":
		return d.config.Timeouts.Analyzing
	case "Executing":
		return d.config.Timeouts.Executing
	default:
		return d.config.Timeouts.Global
	}
}

// IsTerminalPhase checks if the phase is terminal (no timeout check needed).
func (d *Detector) IsTerminalPhase(phase string) bool {
	return terminalPhases[phase]
}
```

### Day 6 TDD Test Plan

**Test File**: `test/unit/remediationorchestrator/timeout_detector_test.go`

| # | Test Case | BR | Type |
|---|-----------|-----|------|
| 1 | Constructor returns non-nil Detector | — | Unit |
| 2 | `CheckGlobalTimeout` returns true when exceeded | BR-ORCH-027 | Unit |
| 3 | `CheckGlobalTimeout` returns false when not exceeded | BR-ORCH-027 | Unit |
| 4 | `CheckGlobalTimeout` uses per-RR override when set | BR-ORCH-027 | Unit |
| 5 | `CheckTimeout` skips terminal phases (Completed) | BR-ORCH-028 | Unit |
| 6 | `CheckTimeout` skips terminal phases (Failed) | BR-ORCH-028 | Unit |
| 7 | `CheckPhaseTimeout` detects Processing timeout | BR-ORCH-028 | Unit |
| 8 | `CheckPhaseTimeout` detects Analyzing timeout | BR-ORCH-028 | Unit |
| 9 | `CheckPhaseTimeout` detects Executing timeout | BR-ORCH-028 | Unit |
| 10 | `CheckPhaseTimeout` returns no timeout when not exceeded | BR-ORCH-028 | Unit |
| 11 | `CheckTimeout` returns global timeout when both exceed | BR-ORCH-027/028 | Unit |
| 12 | `GetPhaseTimeout` returns config defaults | BR-ORCH-028 | Unit (DescribeTable) |
| 13 | `GetPhaseTimeout` uses per-RR override | BR-ORCH-028 | Unit (DescribeTable) |
| 14 | `IsTerminalPhase` returns true for terminal phases | — | Unit (DescribeTable) |

**Total**: 14 tests

### DescribeTable Examples

```go
// Test #12, #13: GetPhaseTimeout with defaults and overrides
DescribeTable("BR-ORCH-028: GetPhaseTimeout returns correct timeout",
    func(phase string, rrOverride *time.Duration, expectedTimeout time.Duration) {
        config := remediationorchestrator.DefaultConfig()
        detector := timeout.NewDetector(config)

        rr := testutil.NewRemediationRequest("test-rr", "default")
        if rrOverride != nil {
            rr.Status.TimeoutConfig = &remediationv1.TimeoutConfig{}
            switch phase {
            case "Processing":
                rr.Status.TimeoutConfig.RemediationProcessingTimeout = metav1.Duration{Duration: *rrOverride}
            case "Analyzing":
                rr.Status.TimeoutConfig.AIAnalysisTimeout = metav1.Duration{Duration: *rrOverride}
            case "Executing":
                rr.Status.TimeoutConfig.WorkflowExecutionTimeout = metav1.Duration{Duration: *rrOverride}
            }
        }

        result := detector.GetPhaseTimeout(rr, phase)
        Expect(result).To(Equal(expectedTimeout))
    },
    // Default config values
    Entry("Processing → 5min (default)", "Processing", nil, 5*time.Minute),
    Entry("Analyzing → 10min (default)", "Analyzing", nil, 10*time.Minute),
    Entry("AwaitingApproval → 10min (uses Analyzing)", "AwaitingApproval", nil, 10*time.Minute),
    Entry("Executing → 30min (default)", "Executing", nil, 30*time.Minute),
    // Per-RR override
    Entry("Processing → 2min (override)", "Processing", ptr(2*time.Minute), 2*time.Minute),
    Entry("Analyzing → 15min (override)", "Analyzing", ptr(15*time.Minute), 15*time.Minute),
    Entry("Executing → 45min (override)", "Executing", ptr(45*time.Minute), 45*time.Minute),
)

// Test #14: IsTerminalPhase
DescribeTable("IsTerminalPhase returns correct value",
    func(phase string, expected bool) {
        config := remediationorchestrator.DefaultConfig()
        detector := timeout.NewDetector(config)
        Expect(detector.IsTerminalPhase(phase)).To(Equal(expected))
    },
    Entry("Completed → true", "Completed", true),
    Entry("Failed → true", "Failed", true),
    Entry("Timeout → true", "Timeout", true),
    Entry("Skipped → true", "Skipped", true),
    Entry("Processing → false", "Processing", false),
    Entry("Analyzing → false", "Analyzing", false),
    Entry("Executing → false", "Executing", false),
    Entry("Pending → false", "Pending", false),
)
```

---

## Day 7: Escalation Manager (8h)

**File**: `pkg/remediation/orchestrator/escalation/manager.go`

```go
package escalation

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// Manager handles escalation workflows
type Manager struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewManager creates a new escalation manager
func NewManager(c client.Client, s *runtime.Scheme) *Manager {
	return &Manager{
		client: c,
		scheme: s,
	}
}

// Escalate creates an escalation notification for failed/timed out remediations
func (m *Manager) Escalate(ctx context.Context, rr *remediationv1.RemediationRequest, reason string) error {
	log := log.FromContext(ctx)

	// Generate unique name
	name := fmt.Sprintf("nr-escalation-%s", rr.Name)

	// Check if already exists (idempotency)
	existing := &notificationv1.NotificationRequest{}
	err := m.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, existing)
	if err == nil {
		log.Info("Escalation notification already exists", "name", name)
		return nil
	}

	// Build escalation notification
	nr := &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
			Labels: map[string]string{
				"kubernaut.ai/remediation-request": rr.Name,
				"kubernaut.ai/notification-type":   "escalation",
			},
		},
		Spec: notificationv1.NotificationRequestSpec{
			NotificationType: "remediation_escalation",
			Channels:         []string{"slack", "pagerduty"}, // High priority channels
			Priority:         "critical",
			Title:            fmt.Sprintf("Remediation Escalation: %s", rr.Spec.SignalData.SignalName),
			Message:          m.buildEscalationMessage(rr, reason),
			Context: notificationv1.NotificationContext{
				RemediationRequestRef: rr.Name,
				EscalationReason:      reason,
				CurrentPhase:          rr.Status.OverallPhase,
			},
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(rr, nr, m.scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create the CRD
	if err := m.client.Create(ctx, nr); err != nil {
		log.Error(err, "Failed to create escalation NotificationRequest")
		return fmt.Errorf("failed to create escalation: %w", err)
	}

	// Update remediation status
	rr.Status.EscalationNotificationRef = name

	log.Info("Created escalation NotificationRequest", "name", name, "reason", reason)
	return nil
}

// TrackDuplicate records a duplicate remediation on the parent (BR-ORCH-033)
func (m *Manager) TrackDuplicate(ctx context.Context, rr *remediationv1.RemediationRequest, duplicateOf string) error {
	log := log.FromContext(ctx)

	// Fetch the parent remediation
	parent := &remediationv1.RemediationRequest{}
	if err := m.client.Get(ctx, client.ObjectKey{
		Name:      duplicateOf,
		Namespace: rr.Namespace,
	}, parent); err != nil {
		log.Error(err, "Failed to get parent remediation", "duplicateOf", duplicateOf)
		return err
	}

	// Update parent's duplicate tracking
	parent.Status.DuplicateCount++
	if parent.Status.DuplicateRefs == nil {
		parent.Status.DuplicateRefs = []string{}
	}
	parent.Status.DuplicateRefs = append(parent.Status.DuplicateRefs, rr.Name)

	// Update parent status
	if err := m.client.Status().Update(ctx, parent); err != nil {
		log.Error(err, "Failed to update parent duplicate tracking")
		return err
	}

	log.Info("Tracked duplicate on parent",
		"duplicate", rr.Name,
		"parent", duplicateOf,
		"totalDuplicates", parent.Status.DuplicateCount,
	)
	return nil
}

// buildEscalationMessage builds the escalation notification message
func (m *Manager) buildEscalationMessage(rr *remediationv1.RemediationRequest, reason string) string {
	return fmt.Sprintf(`
⚠️ REMEDIATION ESCALATION ⚠️

**Signal**: %s
**Severity**: %s
**Environment**: %s
**Current Phase**: %s

**Escalation Reason**:
%s

**Timeline**:
- Created: %s
- Last Transition: %s

Manual intervention may be required.
`,
		rr.Spec.SignalData.SignalName,
		rr.Spec.SignalData.Severity,
		rr.Spec.Environment,
		rr.Status.OverallPhase,
		reason,
		rr.CreationTimestamp.Format(time.RFC3339),
		rr.Status.LastTransitionTime.Format(time.RFC3339),
	)
}
```

---

## Validation Checklist

### Day 3 Midpoint (02-day3-midpoint.md)

- [ ] SignalProcessing creator implemented and tested
- [ ] AIAnalysis creator implemented and tested
- [ ] WorkflowExecution creator implemented and tested
- [ ] Owner references correctly set on all child CRDs
- [ ] Idempotency verified for all creators

### Day 7 Complete (03-day7-complete.md)

- [ ] All 4 child CRD creators working
- [ ] Notification creators (approval + bulk) working
- [ ] Status aggregation collecting from all children
- [ ] Timeout detection for all phases
- [ ] Escalation manager creating notifications
- [ ] All unit tests passing
- [ ] Integration patterns verified

---

## Next Steps

**Day 8-10**: [DAYS_08_10_COORDINATION.md](./DAYS_08_10_COORDINATION.md)
- Watch-based coordination
- Multi-CRD watch setup
- Status aggregation optimization


