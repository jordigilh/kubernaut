# Days 2-7: Core Business Logic - Phase Handlers (48h)

**Parent Document**: [IMPLEMENTATION_PLAN_V1.1.md](./IMPLEMENTATION_PLAN_V1.1.md)
**Date**: Days 2-7 of 14-16
**Focus**: Child CRD creators, phase handlers, status aggregation
**Deliverable**: `02-day3-midpoint.md`, `03-day7-complete.md`

---

## 📑 Table of Contents

| Day | Focus | Hours | Key Deliverables |
|-----|-------|-------|------------------|
| [Day 2](#day-2-child-crd-creators-8h) | SignalProcessing & AIAnalysis creators | 8h | Child CRD creation logic |
| [Day 3](#day-3-workflowexecution-creator-8h) | WorkflowExecution creator | 8h | Workflow pass-through (BR-ORCH-025) |
| [Day 4](#day-4-notification-creator-8h) | Notification creators | 8h | Approval (BR-ORCH-001), bulk (BR-ORCH-034) |
| [Day 5](#day-5-status-aggregation-8h) | Status aggregation | 8h | Multi-CRD status collection |
| [Day 6](#day-6-timeout-detection-8h) | Timeout detection | 8h | BR-ORCH-027, BR-ORCH-028 |
| [Day 7](#day-7-escalation-manager-8h) | Escalation manager | 8h | Failed/timeout escalation |

---

## Day 2: Child CRD Creators (8h)

### Morning: SignalProcessing Creator (4h)

**File**: `pkg/remediation/orchestrator/creator/signalprocessing.go`

```go
package creator

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// SignalProcessingCreator creates SignalProcessing CRDs
type SignalProcessingCreator struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewSignalProcessingCreator creates a new creator
func NewSignalProcessingCreator(c client.Client, s *runtime.Scheme) *SignalProcessingCreator {
	return &SignalProcessingCreator{
		client: c,
		scheme: s,
	}
}

// Create creates a SignalProcessing CRD for the given RemediationRequest
func (c *SignalProcessingCreator) Create(ctx context.Context, rr *remediationv1.RemediationRequest) (string, error) {
	log := log.FromContext(ctx)

	// Generate unique name
	name := fmt.Sprintf("sp-%s", rr.Name)

	// Check if already exists (idempotency)
	existing := &signalprocessingv1.SignalProcessing{}
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, existing)
	if err == nil {
		log.Info("SignalProcessing already exists, reusing", "name", name)
		return name, nil
	}

	// Build SignalProcessing spec from RemediationRequest
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
			// Pass through from RemediationRequest.Spec
			SignalFingerprint: rr.Spec.SignalFingerprint,
			TargetResource:    c.buildTargetResource(rr),
			SignalContext:     c.buildSignalContext(rr),
			Deduplication:     c.buildDeduplication(rr),
			Priority:          rr.Spec.Priority,
			Environment:       rr.Spec.Environment,
		},
	}

	// Set owner reference for cascade deletion (BR-ORCH-031)
	if err := controllerutil.SetControllerReference(rr, sp, c.scheme); err != nil {
		log.Error(err, "Failed to set owner reference")
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create the CRD
	if err := c.client.Create(ctx, sp); err != nil {
		log.Error(err, "Failed to create SignalProcessing CRD")
		return "", fmt.Errorf("failed to create SignalProcessing: %w", err)
	}

	log.Info("Created SignalProcessing CRD", "name", name)
	return name, nil
}

// buildTargetResource builds TargetResource from RemediationRequest
func (c *SignalProcessingCreator) buildTargetResource(rr *remediationv1.RemediationRequest) *signalprocessingv1.TargetResource {
	if rr.Spec.TargetResource == nil {
		return nil
	}
	return &signalprocessingv1.TargetResource{
		Kind:      rr.Spec.TargetResource.Kind,
		Name:      rr.Spec.TargetResource.Name,
		Namespace: rr.Spec.TargetResource.Namespace,
	}
}

// buildSignalContext builds SignalContext from RemediationRequest
func (c *SignalProcessingCreator) buildSignalContext(rr *remediationv1.RemediationRequest) signalprocessingv1.SignalContext {
	return signalprocessingv1.SignalContext{
		SignalName:     rr.Spec.SignalData.SignalName,
		Severity:       rr.Spec.SignalData.Severity,
		Description:    rr.Spec.SignalData.Description,
		Labels:         rr.Spec.SignalData.Labels,
		Annotations:    rr.Spec.SignalData.Annotations,
		StartsAt:       rr.Spec.SignalData.StartsAt,
		EndsAt:         rr.Spec.SignalData.EndsAt,
		GeneratorURL:   rr.Spec.SignalData.GeneratorURL,
	}
}

// buildDeduplication builds DeduplicationInfo from RemediationRequest
func (c *SignalProcessingCreator) buildDeduplication(rr *remediationv1.RemediationRequest) *sharedtypes.DeduplicationInfo {
	if rr.Spec.Deduplication == nil {
		return nil
	}
	return rr.Spec.Deduplication
}
```

### Afternoon: AIAnalysis Creator (4h)

**File**: `pkg/remediation/orchestrator/creator/aianalysis.go`

```go
package creator

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// AIAnalysisCreator creates AIAnalysis CRDs
type AIAnalysisCreator struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewAIAnalysisCreator creates a new creator
func NewAIAnalysisCreator(c client.Client, s *runtime.Scheme) *AIAnalysisCreator {
	return &AIAnalysisCreator{
		client: c,
		scheme: s,
	}
}

// Create creates an AIAnalysis CRD for the given RemediationRequest
// Requires SignalProcessing to be completed first
func (c *AIAnalysisCreator) Create(ctx context.Context, rr *remediationv1.RemediationRequest) (string, error) {
	log := log.FromContext(ctx)

	// Validate precondition: SignalProcessing must be complete
	if rr.Status.SignalProcessingRef == "" {
		return "", fmt.Errorf("SignalProcessing not yet created")
	}

	// Fetch SignalProcessing to get enrichment results
	sp := &signalprocessingv1.SignalProcessing{}
	if err := c.client.Get(ctx, client.ObjectKey{
		Name:      rr.Status.SignalProcessingRef,
		Namespace: rr.Namespace,
	}, sp); err != nil {
		return "", fmt.Errorf("failed to get SignalProcessing: %w", err)
	}

	// Validate SignalProcessing is complete
	if sp.Status.Phase != "Completed" {
		return "", fmt.Errorf("SignalProcessing not yet completed")
	}

	// Generate unique name
	name := fmt.Sprintf("ai-%s", rr.Name)

	// Check if already exists (idempotency)
	existing := &aianalysisv1.AIAnalysis{}
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, existing)
	if err == nil {
		log.Info("AIAnalysis already exists, reusing", "name", name)
		return name, nil
	}

	// Build AIAnalysis spec with data from SignalProcessing.status
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
			// Pass through from RemediationRequest
			SignalFingerprint: rr.Spec.SignalFingerprint,
			TargetResource:    c.buildTargetResource(rr),
			Priority:          sp.Status.DetectedLabels.Priority,      // From SP enrichment
			Environment:       sp.Status.DetectedLabels.Environment,   // From SP enrichment

			// Analysis request context
			AnalysisRequest: aianalysisv1.AnalysisRequest{
				SignalContext: aianalysisv1.SignalContext{
					SignalName:        rr.Spec.SignalData.SignalName,
					Severity:          rr.Spec.SignalData.Severity,
					Description:       rr.Spec.SignalData.Description,
					Labels:            rr.Spec.SignalData.Labels,
					EnrichmentResults: c.buildEnrichmentResults(sp),
				},
				CustomLabels: sp.Status.DetectedLabels.CustomLabels,
			},
		},
	}

	// Set owner reference for cascade deletion (BR-ORCH-031)
	if err := controllerutil.SetControllerReference(rr, ai, c.scheme); err != nil {
		log.Error(err, "Failed to set owner reference")
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create the CRD
	if err := c.client.Create(ctx, ai); err != nil {
		log.Error(err, "Failed to create AIAnalysis CRD")
		return "", fmt.Errorf("failed to create AIAnalysis: %w", err)
	}

	log.Info("Created AIAnalysis CRD", "name", name)
	return name, nil
}

// buildTargetResource builds TargetResource for AIAnalysis
func (c *AIAnalysisCreator) buildTargetResource(rr *remediationv1.RemediationRequest) *aianalysisv1.TargetResource {
	if rr.Spec.TargetResource == nil {
		return nil
	}
	return &aianalysisv1.TargetResource{
		Kind:      rr.Spec.TargetResource.Kind,
		Name:      rr.Spec.TargetResource.Name,
		Namespace: rr.Spec.TargetResource.Namespace,
	}
}

// buildEnrichmentResults converts SP enrichment to AI format
func (c *AIAnalysisCreator) buildEnrichmentResults(sp *signalprocessingv1.SignalProcessing) *sharedtypes.EnrichmentResults {
	if sp.Status.EnrichmentResults == nil {
		return nil
	}
	// Direct pass-through since we use shared types
	return sp.Status.EnrichmentResults
}
```

---

## Day 3: WorkflowExecution Creator (8h)

### WorkflowExecution Creator (BR-ORCH-025)

**File**: `pkg/remediation/orchestrator/creator/workflowexecution.go`

```go
package creator

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// WorkflowExecutionCreator creates WorkflowExecution CRDs
type WorkflowExecutionCreator struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewWorkflowExecutionCreator creates a new creator
func NewWorkflowExecutionCreator(c client.Client, s *runtime.Scheme) *WorkflowExecutionCreator {
	return &WorkflowExecutionCreator{
		client: c,
		scheme: s,
	}
}

// Create creates a WorkflowExecution CRD for the given RemediationRequest
// Implements BR-ORCH-025: Workflow data pass-through from AIAnalysis
func (c *WorkflowExecutionCreator) Create(ctx context.Context, rr *remediationv1.RemediationRequest) (string, error) {
	log := log.FromContext(ctx)

	// Validate precondition: AIAnalysis must be complete
	if rr.Status.AIAnalysisRef == "" {
		return "", fmt.Errorf("AIAnalysis not yet created")
	}

	// Fetch AIAnalysis to get selected workflow
	ai := &aianalysisv1.AIAnalysis{}
	if err := c.client.Get(ctx, client.ObjectKey{
		Name:      rr.Status.AIAnalysisRef,
		Namespace: rr.Namespace,
	}, ai); err != nil {
		return "", fmt.Errorf("failed to get AIAnalysis: %w", err)
	}

	// Validate AIAnalysis is complete and has selected workflow
	if ai.Status.Phase != "Completed" && ai.Status.Phase != "Approved" {
		return "", fmt.Errorf("AIAnalysis not yet completed")
	}
	if ai.Status.SelectedWorkflow == nil {
		return "", fmt.Errorf("AIAnalysis has no selected workflow")
	}

	// Generate unique name
	name := fmt.Sprintf("we-%s", rr.Name)

	// Check if already exists (idempotency)
	existing := &workflowexecutionv1.WorkflowExecution{}
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, existing)
	if err == nil {
		log.Info("WorkflowExecution already exists, reusing", "name", name)
		return name, nil
	}

	// Build WorkflowExecution spec
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
			// WorkflowRef: Direct pass-through from AIAnalysis
			WorkflowRef: workflowexecutionv1.WorkflowRef{
				WorkflowID:      ai.Status.SelectedWorkflow.WorkflowID,
				ContainerImage:  ai.Status.SelectedWorkflow.ContainerImage,
				ContainerDigest: ai.Status.SelectedWorkflow.ContainerDigest,
			},
			// Parameters: Direct pass-through from AIAnalysis
			Parameters: ai.Status.SelectedWorkflow.Parameters,
			// TargetResource: From RemediationRequest
			TargetResource: c.buildTargetResource(rr),
			// ExecutionConfig
			ExecutionConfig: workflowexecutionv1.ExecutionConfig{
				Timeout:            rr.Spec.ExecutionTimeout,
				ServiceAccountName: rr.Spec.ServiceAccountName,
			},
		},
	}

	// Set owner reference for cascade deletion (BR-ORCH-031)
	if err := controllerutil.SetControllerReference(rr, we, c.scheme); err != nil {
		log.Error(err, "Failed to set owner reference")
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create the CRD
	if err := c.client.Create(ctx, we); err != nil {
		log.Error(err, "Failed to create WorkflowExecution CRD")
		return "", fmt.Errorf("failed to create WorkflowExecution: %w", err)
	}

	log.Info("Created WorkflowExecution CRD",
		"name", name,
		"workflowId", ai.Status.SelectedWorkflow.WorkflowID,
		"containerImage", ai.Status.SelectedWorkflow.ContainerImage,
	)
	return name, nil
}

// buildTargetResource builds TargetResource for WorkflowExecution
// Format: "namespace/Kind/name" or "Kind/name" for cluster-scoped
func (c *WorkflowExecutionCreator) buildTargetResource(rr *remediationv1.RemediationRequest) workflowexecutionv1.TargetResource {
	if rr.Spec.TargetResource == nil {
		return workflowexecutionv1.TargetResource{}
	}
	return workflowexecutionv1.TargetResource{
		Kind:      rr.Spec.TargetResource.Kind,
		Name:      rr.Spec.TargetResource.Name,
		Namespace: rr.Spec.TargetResource.Namespace,
	}
}
```

---

## Day 4: Notification Creator (8h)

### Approval Notification Creator (BR-ORCH-001)

**File**: `pkg/remediation/orchestrator/creator/notification.go`

```go
package creator

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// NotificationCreator creates NotificationRequest CRDs
type NotificationCreator struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewNotificationCreator creates a new creator
func NewNotificationCreator(c client.Client, s *runtime.Scheme) *NotificationCreator {
	return &NotificationCreator{
		client: c,
		scheme: s,
	}
}

// CreateApprovalNotification creates a NotificationRequest for approval (BR-ORCH-001)
func (c *NotificationCreator) CreateApprovalNotification(ctx context.Context, rr *remediationv1.RemediationRequest) (string, error) {
	log := log.FromContext(ctx)

	// Fetch AIAnalysis for approval context
	ai := &aianalysisv1.AIAnalysis{}
	if err := c.client.Get(ctx, client.ObjectKey{
		Name:      rr.Status.AIAnalysisRef,
		Namespace: rr.Namespace,
	}, ai); err != nil {
		return "", fmt.Errorf("failed to get AIAnalysis: %w", err)
	}

	// Generate unique name
	name := fmt.Sprintf("nr-approval-%s", rr.Name)

	// Check if already exists (idempotency)
	existing := &notificationv1.NotificationRequest{}
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, existing)
	if err == nil {
		log.Info("Approval notification already exists, reusing", "name", name)
		return name, nil
	}

	// Determine channels based on urgency
	channels := c.determineApprovalChannels(rr, ai)

	// Build NotificationRequest for approval
	nr := &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
			Labels: map[string]string{
				"kubernaut.ai/remediation-request": rr.Name,
				"kubernaut.ai/notification-type":   "approval",
			},
		},
		Spec: notificationv1.NotificationRequestSpec{
			NotificationType: "approval_required",
			Channels:         channels,
			Priority:         c.mapPriorityToNotification(rr.Spec.Priority),
			Title:            fmt.Sprintf("Approval Required: %s", rr.Spec.SignalData.SignalName),
			Message:          c.buildApprovalMessage(rr, ai),
			Context: notificationv1.NotificationContext{
				RemediationRequestRef: rr.Name,
				AIAnalysisRef:         ai.Name,
				ApprovalReason:        ai.Status.ApprovalContext.Reason,
				RootCause:             ai.Status.InvestigationResults.RootCause,
				Confidence:            ai.Status.InvestigationResults.Confidence,
				SelectedWorkflow:      ai.Status.SelectedWorkflow.WorkflowID,
			},
			RequiredBy: c.calculateRequiredBy(rr),
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(rr, nr, c.scheme); err != nil {
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create the CRD
	if err := c.client.Create(ctx, nr); err != nil {
		log.Error(err, "Failed to create approval NotificationRequest")
		return "", fmt.Errorf("failed to create NotificationRequest: %w", err)
	}

	log.Info("Created approval NotificationRequest", "name", name, "channels", channels)
	return name, nil
}

// CreateBulkDuplicateNotification creates a NotificationRequest for bulk duplicates (BR-ORCH-034)
func (c *NotificationCreator) CreateBulkDuplicateNotification(ctx context.Context, rr *remediationv1.RemediationRequest) (string, error) {
	log := log.FromContext(ctx)

	// Generate unique name
	name := fmt.Sprintf("nr-bulk-%s", rr.Name)

	// Check if already exists (idempotency)
	existing := &notificationv1.NotificationRequest{}
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, existing)
	if err == nil {
		log.Info("Bulk notification already exists, reusing", "name", name)
		return name, nil
	}

	// Build bulk notification
	nr := &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
			Labels: map[string]string{
				"kubernaut.ai/remediation-request": rr.Name,
				"kubernaut.ai/notification-type":   "bulk-duplicate",
			},
		},
		Spec: notificationv1.NotificationRequestSpec{
			NotificationType: "remediation_completed_with_duplicates",
			Channels:         []string{"slack"}, // Lower priority
			Priority:         "low",
			Title:            fmt.Sprintf("Remediation Completed with %d Duplicates", rr.Status.DuplicateCount),
			Message:          c.buildBulkDuplicateMessage(rr),
			Context: notificationv1.NotificationContext{
				RemediationRequestRef: rr.Name,
				DuplicateCount:        rr.Status.DuplicateCount,
				DuplicateRefs:         rr.Status.DuplicateRefs,
			},
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(rr, nr, c.scheme); err != nil {
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create the CRD
	if err := c.client.Create(ctx, nr); err != nil {
		log.Error(err, "Failed to create bulk NotificationRequest")
		return "", fmt.Errorf("failed to create NotificationRequest: %w", err)
	}

	log.Info("Created bulk duplicate NotificationRequest",
		"name", name,
		"duplicateCount", rr.Status.DuplicateCount,
	)
	return name, nil
}

// determineApprovalChannels determines notification channels based on context
func (c *NotificationCreator) determineApprovalChannels(rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) []string {
	channels := []string{"slack"} // Default

	// High-risk actions or production environment
	if ai.Status.ApprovalContext.Reason == "high_risk_action" {
		channels = append(channels, "email", "pagerduty")
	} else if rr.Spec.Environment == "production" {
		channels = append(channels, "email")
	}

	return channels
}

// mapPriorityToNotification maps remediation priority to notification priority
func (c *NotificationCreator) mapPriorityToNotification(priority string) string {
	switch priority {
	case "P0":
		return "critical"
	case "P1":
		return "high"
	case "P2":
		return "medium"
	default:
		return "low"
	}
}

// buildApprovalMessage builds the approval notification message
func (c *NotificationCreator) buildApprovalMessage(rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) string {
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
		rr.Spec.SignalData.SignalName,
		rr.Spec.SignalData.Severity,
		rr.Spec.Environment,
		ai.Status.InvestigationResults.RootCause,
		ai.Status.InvestigationResults.Confidence*100,
		ai.Status.SelectedWorkflow.WorkflowID,
		ai.Status.ApprovalContext.Reason,
	)
}

// buildBulkDuplicateMessage builds the bulk duplicate notification message
func (c *NotificationCreator) buildBulkDuplicateMessage(rr *remediationv1.RemediationRequest) string {
	return fmt.Sprintf(`
Remediation completed successfully.

**Signal**: %s
**Result**: %s

**Duplicate Remediations**: %d
The following remediations were skipped as duplicates:
%v

All duplicate signals have been handled by this remediation.
`,
		rr.Spec.SignalData.SignalName,
		rr.Status.OverallPhase,
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

## Day 5: Status Aggregation (8h)

**File**: `pkg/remediation/orchestrator/aggregator/status.go`

```go
package aggregator

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediation/orchestrator"
)

// StatusAggregator aggregates status from child CRDs
type StatusAggregator struct {
	client client.Client
}

// NewStatusAggregator creates a new status aggregator
func NewStatusAggregator(c client.Client) *StatusAggregator {
	return &StatusAggregator{client: c}
}

// AggregateStatus collects status from all child CRDs
func (a *StatusAggregator) AggregateStatus(ctx context.Context, rr *remediationv1.RemediationRequest) (*orchestrator.AggregatedStatus, error) {
	log := log.FromContext(ctx)

	status := &orchestrator.AggregatedStatus{}

	// Aggregate SignalProcessing status
	if rr.Status.SignalProcessingRef != "" {
		spStatus, err := a.getSignalProcessingStatus(ctx, rr.Namespace, rr.Status.SignalProcessingRef)
		if err != nil {
			log.Error(err, "Failed to get SignalProcessing status")
			status.Error = err
		} else {
			status.SignalProcessingPhase = spStatus.Phase
			status.SignalProcessingReady = spStatus.Phase == "Completed"
			status.EnrichmentResults = spStatus.EnrichmentResults
		}
	}

	// Aggregate AIAnalysis status
	if rr.Status.AIAnalysisRef != "" {
		aiStatus, err := a.getAIAnalysisStatus(ctx, rr.Namespace, rr.Status.AIAnalysisRef)
		if err != nil {
			log.Error(err, "Failed to get AIAnalysis status")
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

**File**: `pkg/remediation/orchestrator/timeout/detector.go`

```go
package timeout

import (
	"time"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediation/orchestrator"
	"github.com/jordigilh/kubernaut/pkg/remediation/orchestrator/phase"
)

// Detector detects phase and global timeouts
type Detector struct {
	config orchestrator.OrchestratorConfig
}

// NewDetector creates a new timeout detector
func NewDetector(config orchestrator.OrchestratorConfig) *Detector {
	return &Detector{config: config}
}

// CheckTimeout checks if the current phase has timed out (BR-ORCH-028)
func (d *Detector) CheckTimeout(rr *remediationv1.RemediationRequest) (timedOut bool, timedOutPhase phase.Phase, duration time.Duration) {
	currentPhase := phase.Phase(rr.Status.OverallPhase)

	// Skip if terminal state
	if phase.IsTerminal(currentPhase) {
		return false, "", 0
	}

	// Check global timeout first (BR-ORCH-027)
	if globalTimedOut, globalDuration := d.CheckGlobalTimeout(rr); globalTimedOut {
		return true, "global", globalDuration
	}

	// Get phase start time
	var phaseStartTime *time.Time
	switch currentPhase {
	case phase.Processing:
		if rr.Status.ProcessingStartTime != nil {
			t := rr.Status.ProcessingStartTime.Time
			phaseStartTime = &t
		}
	case phase.Analyzing:
		if rr.Status.AnalyzingStartTime != nil {
			t := rr.Status.AnalyzingStartTime.Time
			phaseStartTime = &t
		}
	case phase.AwaitingApproval:
		if rr.Status.AnalyzingStartTime != nil {
			t := rr.Status.AnalyzingStartTime.Time
			phaseStartTime = &t
		}
	case phase.Executing:
		if rr.Status.ExecutingStartTime != nil {
			t := rr.Status.ExecutingStartTime.Time
			phaseStartTime = &t
		}
	}

	if phaseStartTime == nil {
		return false, "", 0
	}

	// Get timeout for current phase
	timeout := d.getPhaseTimeout(currentPhase)
	elapsed := time.Since(*phaseStartTime)

	if elapsed > timeout {
		return true, currentPhase, elapsed
	}

	return false, "", 0
}

// CheckGlobalTimeout checks if global timeout has been exceeded (BR-ORCH-027)
func (d *Detector) CheckGlobalTimeout(rr *remediationv1.RemediationRequest) (timedOut bool, duration time.Duration) {
	// Use creation timestamp as start time
	elapsed := time.Since(rr.CreationTimestamp.Time)

	// Check against global timeout
	globalTimeout := d.config.Timeouts.Global
	if rr.Spec.GlobalTimeout != nil && *rr.Spec.GlobalTimeout > 0 {
		globalTimeout = *rr.Spec.GlobalTimeout
	}

	if elapsed > globalTimeout {
		return true, elapsed
	}

	return false, 0
}

// getPhaseTimeout returns the configured timeout for a phase
func (d *Detector) getPhaseTimeout(p phase.Phase) time.Duration {
	switch p {
	case phase.Processing:
		return d.config.Timeouts.Processing
	case phase.Analyzing, phase.AwaitingApproval:
		return d.config.Timeouts.Analyzing
	case phase.Executing:
		return d.config.Timeouts.Executing
	default:
		return d.config.Timeouts.Global
	}
}
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

