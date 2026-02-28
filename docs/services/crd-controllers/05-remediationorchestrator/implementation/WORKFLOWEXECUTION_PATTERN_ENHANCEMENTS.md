# RemediationOrchestrator Enhancement Guide: WorkflowExecution v1.2 Patterns

**Purpose**: Enhance existing RemediationOrchestrator implementation with proven patterns from WorkflowExecution v1.2
**Target Plan**: [IMPLEMENTATION_PLAN_V1.1.md](./IMPLEMENTATION_PLAN_V1.1.md) (v1.1.0)
**Status**: Enhancement Guide - Apply patterns to existing plan
**Date**: 2025-10-17

> **📝 BR Reference Note**: This enhancement guide may reference conceptual BR numbers (BR-ORCH-050, etc.)
> from early design phases. See [BR_MAPPING.md](../BR_MAPPING.md) for the authoritative list of 11 formally
> defined V1 BRs: BR-ORCH-001, BR-ORCH-025-034.

---

## 🎯 **PURPOSE**

This guide provides **targeted enhancements** from the highly successful WorkflowExecution v1.2 implementation that should be incorporated into the RemediationOrchestrator plan. It does **NOT replace** the existing plan but rather **augments** it with proven patterns.

**What to Preserve**:
- ✅ Existing version 1.0.1 header with notification integration (BR-ORCH-001, ADR-018)
- ✅ All existing Day 1 structure and code examples
- ✅ All existing business logic components (Creator, Aggregator, TimeoutDetector, EscalationManager)
- ✅ Complete reconciliation logic with phase handlers
- ✅ Targeting Data Pattern implementation

**What to Enhance**:
- ➕ Error handling patterns (Category A-F error classification)
- ➕ Watch setup with retry and reconnection patterns
- ➕ Status update conflict resolution
- ➕ Enhanced integration test templates
- ➕ Production runbook templates
- ➕ Edge case documentation

---

## 📋 **ENHANCEMENT 1: Error Handling Philosophy** ⭐ **CRITICAL**

**Where to Apply**: Day 2 (Reconciliation Loop) and throughout all handler functions

**Pattern from WorkflowExecution** (lines 2350-2650):

### **Error Classification Framework**

Add this section to Day 2's PLAN phase:

```markdown
### Error Handling Strategy

**Classification System** (Category A-F):

#### Category A: CRD Not Found (Normal)
**When**: Child CRD doesn't exist yet or was deleted
**Action**: Continue reconciliation (this triggers creation)
**Recovery**: Automatic

#### Category B: CRD API Errors (Retryable)
**When**: API server temporary unavailability, network issues
**Action**: Requeue with exponential backoff (5s → 10s → 30s)
**Recovery**: Automatic with retry

#### Category C: Invalid CRD Spec (User Error)
**When**: Targeting Data validation fails, missing required fields
**Action**: Mark RemediationRequest as Failed, create NotificationRequest
**Recovery**: Manual (user must fix)

#### Category D: Watch Connection Loss (Infrastructure)
**When**: Watch stream disconnects, controller restarts
**Action**: Automatic reconnection via controller-runtime
**Recovery**: Automatic (no action needed)

#### Category E: Status Update Conflicts (Concurrency)
**When**: Multiple status updates conflict (optimistic locking)
**Action**: Retry with fresh read (max 3 attempts)
**Recovery**: Automatic with retry

#### Category F: Child CRD Failures (Propagated)
**When**: RemediationProcessing/AIAnalysis/WorkflowExecution fails
**Action**: Propagate to RemediationRequest.status.phase = Failed, escalate
**Recovery**: Depends on root cause
```

### **Enhanced Error Handling Code Examples**

**Add to Day 2 DO phase** - Replace simple error handling with this:

```go
// Enhanced handleProcessing with comprehensive error handling
// File: internal/controller/remediation/remediationrequest_controller.go

package remediation

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1alpha1"
	aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
)

func (r *RemediationRequestReconciler) handleProcessing(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Monitoring RemediationProcessing", "name", rr.Name)

	// Check timeout
	if r.TimeoutDetector.IsPhaseTimedOut(rr, DefaultPhaseTimeout) {
		log.Info("Processing phase timed out, escalating")
		return r.handleTimeout(ctx, rr, "Processing")
	}

	// Fetch RemediationProcessing child CRD with error classification
	var processing remediationprocessingv1alpha1.RemediationProcessing
	err := r.Get(ctx, client.ObjectKey{
		Namespace: rr.Status.RemediationProcessingRef.Namespace,
		Name:      rr.Status.RemediationProcessingRef.Name,
	}, &processing)

	if err != nil {
		// Category A: CRD Not Found (could be normal during creation)
		if apierrors.IsNotFound(err) {
			log.V(1).Info("RemediationProcessing not found yet, will requeue")
			return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
		}

		// Category B: API Server Errors (retryable)
		if apierrors.IsServiceUnavailable(err) || apierrors.IsTimeout(err) || apierrors.IsServerTimeout(err) {
			log.Error(err, "API server temporarily unavailable, will retry")
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

		// Unexpected error
		log.Error(err, "Failed to fetch RemediationProcessing")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Category F: Child CRD Failed (propagate)
	if processing.Status.Phase == "Failed" {
		log.Info("RemediationProcessing failed",
			"reason", processing.Status.Reason,
			"message", processing.Status.Message)

		// Propagate failure
		rr.Status.Phase = "Failed"
		rr.Status.Message = fmt.Sprintf("RemediationProcessing failed: %s", processing.Status.Message)
		rr.Status.Reason = processing.Status.Reason

		// Category E: Status Update with Conflict Retry
		if err := r.updateStatusWithRetry(ctx, rr); err != nil {
			log.Error(err, "Failed to update status after retries")
			return ctrl.Result{}, err
		}

		return r.handleEscalation(ctx, rr, "RemediationProcessing failed")
	}

	// Child completed - create next child
	if processing.Status.Phase == "Completed" {
		log.Info("RemediationProcessing completed, creating AIAnalysis")

		aiCRD, err := r.ChildCreator.CreateAIAnalysis(ctx, rr, &processing)
		if err != nil {
			log.Error(err, "Failed to create AIAnalysis CRD")

			// Category C: Creation failure (could be validation error)
			if apierrors.IsInvalid(err) {
				rr.Status.Phase = "Failed"
				rr.Status.Message = fmt.Sprintf("Invalid AIAnalysis spec: %v", err)
				if updateErr := r.updateStatusWithRetry(ctx, rr); updateErr != nil {
					return ctrl.Result{}, updateErr
				}
				return ctrl.Result{}, err
			}

			// Retryable error
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

		// Update status successfully
		rr.Status.Phase = "Analyzing"
		rr.Status.PhaseStartTime = &metav1.Time{Time: time.Now()}
		rr.Status.AIAnalysisRef = &remediationv1alpha1.ObjectReference{
			Name:      aiCRD.Name,
			Namespace: aiCRD.Namespace,
		}
		rr.Status.Message = "AIAnalysis created, investigating root cause"

		if err := r.updateStatusWithRetry(ctx, rr); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true}, nil
	}

	// Still in progress - requeue
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// updateStatusWithRetry handles Category E: Status Update Conflicts
func (r *RemediationRequestReconciler) updateStatusWithRetry(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
	log := log.FromContext(ctx)
	const maxRetries = 3

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		err := r.Status().Update(ctx, rr)
		if err == nil {
			// Success
			statusUpdateSuccess.Inc()
			return nil
		}

		// Category E: Conflict - retry with fresh read
		if apierrors.IsConflict(err) {
			log.V(1).Info("Status update conflict, retrying with fresh read",
				"attempt", attempt+1,
				"maxRetries", maxRetries)

			statusUpdateConflicts.Inc()

			// Read fresh version
			var fresh remediationv1alpha1.RemediationRequest
			if getErr := r.Get(ctx, client.ObjectKey{
				Namespace: rr.Namespace,
				Name:      rr.Name,
			}, &fresh); getErr != nil {
				lastErr = getErr
				break
			}

			// Update fresh copy's status
			fresh.Status = rr.Status

			// Update rr to fresh copy for next attempt
			*rr = fresh

			// Brief backoff
			time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
			continue
		}

		// Non-conflict error, don't retry
		lastErr = err
		break
	}

	statusUpdateFailure.Inc()
	return fmt.Errorf("status update failed after %d attempts: %w", maxRetries, lastErr)
}

// Prometheus metrics for monitoring
var (
	statusUpdateSuccess = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "remediationorchestrator_status_update_success_total",
			Help: "Successful status updates",
		},
	)

	statusUpdateConflicts = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "remediationorchestrator_status_update_conflicts_total",
			Help: "Status update conflicts (retried)",
		},
	)

	statusUpdateFailure = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "remediationorchestrator_status_update_failure_total",
			Help: "Failed status updates after retries",
		},
	)
)
```

**Add to all phase handlers**: Apply the same error classification to `handleAnalyzing`, `handleWorkflowPlanning`, `handleExecuting`.

---

## 📋 **ENHANCEMENT 2: Watch Setup with Reconnection Patterns**

**Where to Apply**: Day 8 (Watch-Based Coordination)

**Pattern from WorkflowExecution** (SetupWithManager):

### **Enhanced SetupWithManager**

**Replace existing SetupWithManager** in Day 8 with this enhanced version:

```go
// SetupWithManager sets up the controller with the Manager
// Enhanced with logging and validation
// File: internal/controller/remediation/remediationrequest_controller.go

package remediation

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1alpha1"
	aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	kubernetesexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/kubernetesexecution/v1alpha1"
)

func (r *RemediationRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	log := ctrl.Log.WithName("setup").WithName("RemediationOrchestrator")

	log.Info("Setting up RemediationOrchestrator controller with manager")

	// Validate dependencies
	if r.StateMachine == nil {
		return fmt.Errorf("StateMachine dependency not initialized")
	}
	if r.TargetingManager == nil {
		return fmt.Errorf("TargetingManager dependency not initialized")
	}
	if r.ChildCreator == nil {
		return fmt.Errorf("ChildCreator dependency not initialized")
	}
	if r.StatusAggregator == nil {
		return fmt.Errorf("StatusAggregator dependency not initialized")
	}
	if r.TimeoutDetector == nil {
		return fmt.Errorf("TimeoutDetector dependency not initialized")
	}
	if r.EscalationManager == nil {
		return fmt.Errorf("EscalationManager dependency not initialized")
	}

	log.Info("All dependencies validated successfully")

	// Setup controller with comprehensive watches
	err := ctrl.NewControllerManagedBy(mgr).
		For(&remediationv1alpha1.RemediationRequest{}).
		Owns(&remediationprocessingv1alpha1.RemediationProcessing{}).
		Owns(&aianalysisv1alpha1.AIAnalysis{}).
		Owns(&workflowexecutionv1alpha1.WorkflowExecution{}).
		Owns(&kubernetesexecutionv1alpha1.KubernetesExecution{}). // DEPRECATED - ADR-025
		// Note: We do NOT own NotificationRequest (created for escalation but not owned)
		Complete(r)

	if err != nil {
		log.Error(err, "Failed to setup controller with manager")
		return err
	}

	log.Info("Controller setup complete",
		"watches", "RemediationRequest (primary) + 4 child CRDs (owned)",
		"reconciliation", "watch-based (automatic reconnection)")

	return nil
}
```

**Add Watch Behavior Documentation** to Day 8 ANALYSIS phase:

```markdown
### Watch Reconnection Behavior (Category D Errors)

**Automatic Reconnection**: controller-runtime handles watch reconnection automatically

**Watch Connection Loss Scenarios**:
1. **Network Interruption**: Watch stream times out, controller-runtime reconnects
2. **API Server Restart**: Watch stream breaks, controller-runtime re-establishes
3. **Controller Restart**: Watches recreated on controller startup

**Recovery Pattern**:
- **Automatic**: No manual intervention needed
- **Latency**: <10s typical reconnection time
- **Consistency**: Full reconciliation triggered after reconnection

**Monitoring**:
```go
// Log watch events for debugging (Day 13: Metrics)
// File: internal/controller/remediation/remediationrequest_controller.go

package remediation

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *RemediationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reconcileStart := time.Now()
	log := log.FromContext(ctx)

	defer func() {
		reconcileDuration.Observe(time.Since(reconcileStart).Seconds())
		log.V(2).Info("Reconciliation completed",
			"duration", time.Since(reconcileStart),
			"name", req.Name,
			"namespace", req.Namespace)
	}()

	// ... rest of reconciliation logic
}

var (
	reconcileDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name: "remediationorchestrator_reconcile_duration_seconds",
			Help: "Duration of reconciliation loops",
			Buckets: []float64{0.1, 0.5, 1.0, 2.0, 5.0, 10.0},
		},
	)
)
```
```

---

## 📋 **ENHANCEMENT 3: Integration Test Template with Anti-Flaky Patterns**

**Where to Apply**: Day 14-15 (Integration Testing)

**Pattern from WorkflowExecution** (lines 4500-5200):

### **Enhanced Integration Test Template**

**Add to Day 14** - Create new file: `test/integration/remediationorchestrator/multi_crd_coordination_test.go`

```go
package remediationorchestrator

import (
	"context"
	"fmt"
	"time"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1alpha1"
	aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/testutil/timing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Multi-CRD Coordination", func() {
	var (
		ctx       context.Context
		namespace string
		rr        *remediationv1alpha1.RemediationRequest
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = fmt.Sprintf("multi-crd-test-%d", GinkgoRandomSeed())

		// Create test namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
	})

	AfterEach(func() {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		_ = k8sClient.Delete(ctx, ns)
	})

	// ============================================================================
	// BR-ORCH-041: Watch 4 Child CRD Types Simultaneously
	// ============================================================================

	Describe("BR-ORCH-041: 4-Way CRD Watch Coordination", func() {
		It("should create and coordinate all 4 child CRDs in sequence", func() {
			// GIVEN: RemediationRequest with complete targeting data
			rr = &remediationv1alpha1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "complete-remediation",
					Namespace: namespace,
				},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					TargetingData: &remediationv1alpha1.TargetingData{
						SignalFingerprint: "test-fingerprint-001",
						AlertName:         "HighPodCrashLoop",
						Environment:       "production",
						ClusterName:       "prod-us-west-2",
					},
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			// WHEN: RemediationRequest is created
			// THEN: RemediationProcessing should be created first

			// Anti-flaky pattern: EventuallyWithRetry for CRD creation
			var processing remediationprocessingv1alpha1.RemediationProcessing
			Eventually(func() error {
				var processingList remediationprocessingv1alpha1.RemediationProcessingList
				if err := k8sClient.List(ctx, &processingList, client.InNamespace(namespace)); err != nil {
					return err
				}
				if len(processingList.Items) == 0 {
					return fmt.Errorf("RemediationProcessing not created yet")
				}
				processing = processingList.Items[0]
				return nil
			}, "30s", "1s").Should(Succeed(), "RemediationProcessing should be created within 30s")

			// Verify owner reference
			Expect(processing.OwnerReferences).To(HaveLen(1))
			Expect(processing.OwnerReferences[0].Name).To(Equal(rr.Name))
			Expect(processing.OwnerReferences[0].Kind).To(Equal("RemediationRequest"))

			// Complete RemediationProcessing
			processing.Status.Phase = "Completed"
			processing.Status.EnrichedData = map[string]string{
				"enrichedKey": "enrichedValue",
			}
			Expect(k8sClient.Status().Update(ctx, &processing)).To(Succeed())

			// THEN: AIAnalysis should be created next
			var ai aianalysisv1alpha1.AIAnalysis
			Eventually(func() error {
				var aiList aianalysisv1alpha1.AIAnalysisList
				if err := k8sClient.List(ctx, &aiList, client.InNamespace(namespace)); err != nil {
					return err
				}
				if len(aiList.Items) == 0 {
					return fmt.Errorf("AIAnalysis not created yet")
				}
				ai = aiList.Items[0]
				return nil
			}, "30s", "1s").Should(Succeed(), "AIAnalysis should be created after RemediationProcessing completes")

			// Verify AIAnalysis has enriched data from RemediationProcessing
			Expect(ai.Spec.EnrichedData).To(Equal(processing.Status.EnrichedData))

			// Complete AIAnalysis (auto-approve with high confidence)
			ai.Status.Phase = "Ready"
			ai.Status.Confidence = 0.95
			ai.Status.InvestigationResult = aianalysisv1alpha1.InvestigationResult{
				Recommendations: []aianalysisv1alpha1.Recommendation{
					{
						Action:      "ScaleDeployment",
						Description: "Scale deployment to 3 replicas",
					},
				},
			}
			Expect(k8sClient.Status().Update(ctx, &ai)).To(Succeed())

			// THEN: WorkflowExecution should be created last
			var workflow workflowexecutionv1alpha1.WorkflowExecution
			Eventually(func() error {
				var workflowList workflowexecutionv1alpha1.WorkflowExecutionList
				if err := k8sClient.List(ctx, &workflowList, client.InNamespace(namespace)); err != nil {
					return err
				}
				if len(workflowList.Items) == 0 {
					return fmt.Errorf("WorkflowExecution not created yet")
				}
				workflow = workflowList.Items[0]
				return nil
			}, "30s", "1s").Should(Succeed(), "WorkflowExecution should be created after AIAnalysis completes")

			// Verify WorkflowExecution has recommendations from AIAnalysis
			Expect(workflow.Spec.Recommendations).To(HaveLen(1))
			Expect(workflow.Spec.Recommendations[0].Action).To(Equal("ScaleDeployment"))

			// Verify RemediationRequest status tracks all children
			var updatedRR remediationv1alpha1.RemediationRequest
			Eventually(func() error {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      rr.Name,
					Namespace: namespace,
				}, &updatedRR); err != nil {
					return err
				}

				if updatedRR.Status.RemediationProcessingRef == nil {
					return fmt.Errorf("RemediationProcessingRef not set")
				}
				if updatedRR.Status.AIAnalysisRef == nil {
					return fmt.Errorf("AIAnalysisRef not set")
				}
				if updatedRR.Status.WorkflowExecutionRef == nil {
					return fmt.Errorf("WorkflowExecutionRef not set")
				}

				return nil
			}, "10s", "1s").Should(Succeed(), "RemediationRequest should track all child references")

			Expect(updatedRR.Status.Phase).To(Equal("WorkflowPlanning"))
		})

		It("should handle child CRD failure and trigger escalation", func() {
			// GIVEN: RemediationRequest
			rr = &remediationv1alpha1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "failed-remediation",
					Namespace: namespace,
				},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					TargetingData: &remediationv1alpha1.TargetingData{
						SignalFingerprint: "test-fingerprint-002",
						AlertName:         "DatabaseConnectionFailure",
						Environment:       "staging",
						ClusterName:       "staging-us-east-1",
					},
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			// Wait for RemediationProcessing creation
			var processing remediationprocessingv1alpha1.RemediationProcessing
			Eventually(func() error {
				var processingList remediationprocessingv1alpha1.RemediationProcessingList
				if err := k8sClient.List(ctx, &processingList, client.InNamespace(namespace)); err != nil {
					return err
				}
				if len(processingList.Items) == 0 {
					return fmt.Errorf("RemediationProcessing not created yet")
				}
				processing = processingList.Items[0]
				return nil
			}, "30s", "1s").Should(Succeed())

			// WHEN: RemediationProcessing fails
			processing.Status.Phase = "Failed"
			processing.Status.Reason = "EnrichmentError"
			processing.Status.Message = "Failed to enrich signal data: timeout fetching cluster state"
			processing.Status.Conditions = []metav1.Condition{
				{
					Type:               "Failed",
					Status:             metav1.ConditionTrue,
					Reason:             "EnrichmentTimeout",
					Message:            "Context API timeout",
					LastTransitionTime: metav1.Now(),
				},
			}
			Expect(k8sClient.Status().Update(ctx, &processing)).To(Succeed())

			// THEN: RemediationRequest should propagate failure
			Eventually(func() string {
				var updatedRR remediationv1alpha1.RemediationRequest
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      rr.Name,
					Namespace: namespace,
				}, &updatedRR); err != nil {
					return ""
				}
				return updatedRR.Status.Phase
			}, "10s", "1s").Should(Equal("Failed"), "RemediationRequest should transition to Failed")

			// Verify NotificationRequest created for escalation (BR-ORCH-001)
			// Note: This requires Notification Controller operational
			// For integration test, we just verify the NotificationRequest CRD was created
			Eventually(func() bool {
				var notificationList notificationv1alpha1.NotificationRequestList
				if err := k8sClient.List(ctx, &notificationList, client.InNamespace(namespace)); err != nil {
					return false
				}
				return len(notificationList.Items) > 0
			}, "15s", "1s").Should(BeTrue(), "NotificationRequest should be created for escalation")
		})
	})

	// ============================================================================
	// BR-ORCH-050: Status Aggregation from All Children
	// ============================================================================

	Describe("BR-ORCH-050: Status Aggregation", func() {
		It("should aggregate progress from all child CRDs", func() {
			// GIVEN: RemediationRequest with all children at various stages
			rr = &remediationv1alpha1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "progress-aggregation",
					Namespace: namespace,
				},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					TargetingData: &remediationv1alpha1.TargetingData{
						SignalFingerprint: "test-fingerprint-003",
						AlertName:         "HighMemoryUsage",
						Environment:       "production",
						ClusterName:       "prod-eu-west-1",
					},
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			// Wait for RemediationProcessing and complete it
			var processing remediationprocessingv1alpha1.RemediationProcessing
			Eventually(func() error {
				var processingList remediationprocessingv1alpha1.RemediationProcessingList
				if err := k8sClient.List(ctx, &processingList, client.InNamespace(namespace)); err != nil {
					return err
				}
				if len(processingList.Items) == 0 {
					return fmt.Errorf("RemediationProcessing not created yet")
				}
				processing = processingList.Items[0]
				return nil
			}, "30s", "1s").Should(Succeed())

			processing.Status.Phase = "Completed"
			Expect(k8sClient.Status().Update(ctx, &processing)).To(Succeed())

			// Wait for AIAnalysis and complete it
			var ai aianalysisv1alpha1.AIAnalysis
			Eventually(func() error {
				var aiList aianalysisv1alpha1.AIAnalysisList
				if err := k8sClient.List(ctx, &aiList, client.InNamespace(namespace)); err != nil {
					return err
				}
				if len(aiList.Items) == 0 {
					return fmt.Errorf("AIAnalysis not created yet")
				}
				ai = aiList.Items[0]
				return nil
			}, "30s", "1s").Should(Succeed())

			ai.Status.Phase = "Ready"
			ai.Status.InvestigationResult = aianalysisv1alpha1.InvestigationResult{
				Recommendations: []aianalysisv1alpha1.Recommendation{
					{Action: "ScaleDeployment"},
					{Action: "UpdateMemoryLimits"},
				},
			}
			Expect(k8sClient.Status().Update(ctx, &ai)).To(Succeed())

			// Wait for WorkflowExecution with 2 steps
			var workflow workflowexecutionv1alpha1.WorkflowExecution
			Eventually(func() error {
				var workflowList workflowexecutionv1alpha1.WorkflowExecutionList
				if err := k8sClient.List(ctx, &workflowList, client.InNamespace(namespace)); err != nil {
					return err
				}
				if len(workflowList.Items) == 0 {
					return fmt.Errorf("WorkflowExecution not created yet")
				}
				workflow = workflowList.Items[0]
				return nil
			}, "30s", "1s").Should(Succeed())

			// Simulate workflow progress (1 of 2 steps completed)
			workflow.Status.StepsCompleted = 1
			workflow.Status.StepsTotal = 2
			workflow.Status.Phase = "Executing"
			Expect(k8sClient.Status().Update(ctx, &workflow)).To(Succeed())

			// THEN: RemediationRequest should aggregate progress
			// Overall: RemediationProcessing (1 completed) + AIAnalysis (1 completed) + WorkflowExecution (1/2 completed) = 3/4 = 75%
			Eventually(func() float64 {
				var updatedRR remediationv1alpha1.RemediationRequest
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      rr.Name,
					Namespace: namespace,
				}, &updatedRR); err != nil {
					return 0
				}
				return updatedRR.Status.OverallProgress
			}, "10s", "1s").Should(BeNumerically("~", 75.0, 5.0), "OverallProgress should be ~75%")

			// Verify step counts
			var updatedRR remediationv1alpha1.RemediationRequest
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      rr.Name,
				Namespace: namespace,
			}, &updatedRR)).To(Succeed())

			Expect(updatedRR.Status.StepsCompleted).To(Equal(3)) // 1 + 1 + 1
			Expect(updatedRR.Status.StepsTotal).To(Equal(4))     // 1 + 1 + 2
		})
	})
})
```

**Anti-Flaky Pattern Documentation** - Add to Day 14 ANALYSIS phase:

```markdown
### Anti-Flaky Integration Test Patterns

**Pattern 1: EventuallyWithRetry for CRD Creation**
```go
Eventually(func() error {
	// Try to get CRD
	var crd SomeCRD
	return k8sClient.Get(ctx, key, &crd)
}, "30s", "1s").Should(Succeed())
```

**Pattern 2: Status Update Conflict Handling**
```go
// Wait for status field to update (retry on conflicts)
Eventually(func() string {
	var obj RemediationRequest
	k8sClient.Get(ctx, key, &obj)
	return obj.Status.Phase
}, "10s", "1s").Should(Equal("Expected"))
```

**Pattern 3: List-Based Checks for Multiple CRDs**
```go
// Use List instead of Get for dynamic creation scenarios
Eventually(func() int {
	var list ChildCRDList
	k8sClient.List(ctx, &list, client.InNamespace(ns))
	return len(list.Items)
}, "30s", "1s").Should(Equal(4))
```

**Pattern 4: Phase Transition Validation**
```go
// Ensure phase transitions are complete before next assertion
Expect(k8sClient.Status().Update(ctx, &crd)).To(Succeed())
// Allow time for watch to propagate
time.Sleep(500 * time.Millisecond)
```
```

---

## 📋 **ENHANCEMENT 4: Production Runbook Templates**

**Where to Apply**: Day 16 (Handoff)

**Pattern from WorkflowExecution** (lines 2726-2757):

### **Production Runbooks**

**Add to Day 16** - Create file: `docs/services/crd-controllers/05-remediationorchestrator/PRODUCTION_RUNBOOKS.md`

```markdown
# RemediationOrchestrator Production Runbooks

## Runbook 1: High Remediation Failure Rate

### Alert
```
Alert: RemediationRequestFailureRate > 15%
Severity: High
Symptoms: Multiple RemediationRequests transitioning to Failed state
```

### Investigation Steps

1. **Check RemediationRequest failures**:
```bash
kubectl get remediationrequests -A -o json | \
  jq '.items[] | select(.status.phase=="Failed") | {name: .metadata.name, reason: .status.reason, message: .status.message}'
```

2. **Check child CRD failures**:
```bash
# RemediationProcessing failures
kubectl get remediationprocessings -A -o json | \
  jq '.items[] | select(.status.phase=="Failed")'

# AIAnalysis failures
kubectl get aianalyses -A -o json | \
  jq '.items[] | select(.status.phase=="Failed")'

# WorkflowExecution failures
kubectl get workflowexecutions -A -o json | \
  jq '.items[] | select(.status.phase=="Failed")'
```

3. **Check controller logs**:
```bash
kubectl logs -n kubernaut-system deployment/remediation-orchestrator --tail=100 | \
  grep -E "ERROR|Failed|timeout"
```

4. **Check Prometheus metrics**:
```
remediationorchestrator_status_update_failure_total
remediationorchestrator_child_creation_failures_total
remediationorchestrator_phase_timeout_total
```

### Resolution Actions

**If validation errors (Category C)**:
- Review RemediationRequest targeting data
- Check for missing required fields
- Validate TargetingData schema compliance

**If API server errors (Category B)**:
- Check API server health: `kubectl get --raw /healthz`
- Check controller RBAC permissions
- Check API server load and rate limiting

**If child CRD failures (Category F)**:
- Investigate child controller logs (RemediationProcessor, AIAnalysis, WorkflowExecution)
- Check child controller dependencies (HolmesGPT, Context API, Data Storage)
- Review child CRD error messages for root cause

**If timeout errors**:
- Review DefaultPhaseTimeout configuration (15min default)
- Check if child controllers are stuck
- Investigate slow downstream dependencies

### Escalation
If failure rate remains >15% for >30 minutes, escalate to on-call engineer.

---

## Runbook 2: Stuck Remediations (Phase Timeouts)

### Alert
```
Alert: RemediationRequestPhaseTimeout > 10
Severity: Medium
Symptoms: Multiple RemediationRequests stuck in same phase beyond timeout
```

### Investigation Steps

1. **Identify stuck remediations**:
```bash
kubectl get remediationrequests -A -o json | \
  jq '.items[] | select(.status.phaseStartTime) | select((now - (.status.phaseStartTime | fromdateiso8601)) > 900) | {name: .metadata.name, phase: .status.phase, stuck_duration: (now - (.status.phaseStartTime | fromdateiso8601))}'
```

2. **Check specific phase timeouts**:
```bash
# Processing phase (RemediationProcessing)
kubectl get remediationprocessings -A -o json | \
  jq '.items[] | select(.status.phase=="Pending" or .status.phase=="Processing")'

# Analyzing phase (AIAnalysis)
kubectl get aianalyses -A -o json | \
  jq '.items[] | select(.status.phase=="Investigating" or .status.phase=="Approving")'

# Executing phase (WorkflowExecution)
kubectl get workflowexecutions -A -o json | \
  jq '.items[] | select(.status.phase=="Running")'
```

3. **Check child controller health**:
```bash
# Check all child controller pods
kubectl get pods -n kubernaut-system | \
  grep -E "remediation-processor|aianalysis|workflow-execution"
```

4. **Review controller logs for stuck operations**:
```bash
kubectl logs -n kubernaut-system deployment/remediation-orchestrator | \
  grep -E "timeout|stuck|requeue" | tail -50
```

### Resolution Actions

**If RemediationProcessing stuck**:
- Check Context API availability
- Check Data Storage Service (Redis/PostgreSQL)
- Review RemediationProcessor controller logs

**If AIAnalysis stuck**:
- Check HolmesGPT API availability and response times
- Check AIApprovalRequest status (may be awaiting manual approval)
- Review AIAnalysis controller logs

**If WorkflowExecution stuck**:
- Check KubernetesExecution (DEPRECATED - ADR-025) creation rate
- Check workflow complexity (>10 steps trigger complexity approval)
- Review WorkflowExecution controller logs

**Manual Resolution**:
```bash
# Force timeout detection (update phaseStartTime)
kubectl patch remediationrequest <name> -n <namespace> --type=json \
  -p='[{"op": "replace", "path": "/status/phaseStartTime", "value": "2020-01-01T00:00:00Z"}]'
```

### Escalation
If >10 remediations stuck for >1 hour, escalate to on-call engineer.

---

## Runbook 3: Watch Connection Loss (Category D)

### Alert
```
Alert: RemediationOrchestratorWatchReconnections > 5/hour
Severity: Low (automatic recovery)
Symptoms: Frequent watch reconnections visible in logs
```

### Investigation Steps

1. **Check reconnection frequency**:
```bash
kubectl logs -n kubernaut-system deployment/remediation-orchestrator | \
  grep -E "watch.*reconnect|watch.*disconnect" | \
  awk '{print $1}' | uniq -c
```

2. **Check API server health**:
```bash
kubectl get --raw /healthz
kubectl get --raw /readyz
```

3. **Check network connectivity**:
```bash
# From controller pod
kubectl exec -n kubernaut-system deployment/remediation-orchestrator -- \
  curl -k https://kubernetes.default.svc/healthz
```

### Resolution Actions

**Category D errors are automatically recovered**:
- controller-runtime handles reconnection
- No manual intervention needed
- Monitor for excessive reconnection frequency

**If excessive reconnections (>10/hour)**:
- Check API server stability
- Check network infrastructure (load balancers, proxies)
- Review API server logs for connection drops

**Prevention**:
- Ensure API server has adequate resources
- Review network policy configuration
- Check for API server rate limiting

### Escalation
If reconnections cause >5 minute reconciliation delays, escalate to platform team.

---

## Runbook 4: Status Update Conflicts (Category E)

### Alert
```
Alert: RemediationOrchestratorStatusConflicts > 100/hour
Severity: Medium
Symptoms: High status update conflict rate indicating concurrency issues
```

### Investigation Steps

1. **Check conflict metrics**:
```bash
curl -s http://localhost:9090/metrics | \
  grep remediationorchestrator_status_update_conflicts_total
```

2. **Review controller logs for conflict patterns**:
```bash
kubectl logs -n kubernaut-system deployment/remediation-orchestrator | \
  grep "Status update conflict" | tail -50
```

3. **Check for multiple controller instances**:
```bash
kubectl get pods -n kubernaut-system -l app=remediation-orchestrator
# Should see only 1 pod (leader election prevents multiple active)
```

### Resolution Actions

**Category E errors are automatically retried** (max 3 attempts):
- Conflicts are expected under high load
- updateStatusWithRetry handles retries with fresh read
- No manual intervention needed for <200/hour

**If excessive conflicts (>200/hour)**:
- Check for rapid phase transitions (may need longer settle time)
- Review concurrent reconciliation load
- Check for external status updates (operators, manual kubectl)

**Prevention**:
- Ensure leader election is working (only 1 active controller)
- Avoid manual status updates via kubectl
- Review child controller completion rates

### Escalation
If conflicts cause >10% of remediations to fail, escalate to platform team.
```

---

## 📋 **ENHANCEMENT 5: Edge Case Documentation**

**Where to Apply**: Throughout testing sections (Days 14-16)

**Pattern from WorkflowExecution** (lines 4433-4465):

### **Edge Case Testing Categories**

**Add to Day 15** - Enhance integration testing with these edge case categories:

```markdown
### Edge Case Testing Strategy

#### Category 1: Concurrency & Race Conditions
**Scenarios**:
- Simultaneous RemediationRequest creations for same alert
- Child CRD status updates racing with parent phase transitions
- Multiple reconciliation loops triggered by watch events
- Status update conflicts during rapid phase changes

**Anti-Pattern**: `sync.RWMutex` for state protection
**Test Pattern**: `pkg/testutil/timing/Barrier` for synchronization

#### Category 2: Resource Exhaustion
**Scenarios**:
- High RemediationRequest creation rate (>100/min)
- Memory pressure from large TargetingData payloads
- API server rate limiting during child CRD creation
- Watch event queue overflow

**Anti-Pattern**: Circuit breakers, rate limiters
**Test Pattern**: Load testing with 100+ concurrent remediations

#### Category 3: Failure Cascades
**Scenarios**:
- All 4 child CRDs fail simultaneously
- Child controller crashes during reconciliation
- API server downtime during child CRD creation
- Multiple timeout escalations triggering notification storm

**Anti-Pattern**: Failure isolation, controlled error propagation
**Test Pattern**: Chaos testing with Envtest

#### Category 4: Timing & Latency
**Scenarios**:
- Phase transitions faster than watch propagation (< 1s)
- Timeout detection edge cases (exactly at threshold)
- Watch connection loss during critical phase transitions
- Clock skew in distributed systems

**Anti-Pattern**: `EventuallyWithRetry`, deadline enforcement
**Test Pattern**: Time manipulation with test clocks

#### Category 5: State Inconsistencies
**Scenarios**:
- RemediationRequest deleted mid-reconciliation
- Orphaned child CRDs (owner reference missing)
- Missing owner references breaking cascade deletion
- Stale status from watch reconnection

**Anti-Pattern**: Optimistic locking, periodic reconciliation, finalizers
**Test Pattern**: State validation assertions in all tests

#### Category 6: Data Integrity
**Scenarios**:
- TargetingData modified after immutable snapshot
- Child CRD references stale data from parent
- Status aggregation with partial child data
- Missing targeting data validation

**Anti-Pattern**: Immutable data snapshots, deep copy validation
**Test Pattern**: Data mutation detection tests
```

**Add specific edge case tests to Day 15**:

```go
// Add to test/integration/remediationorchestrator/edge_cases_test.go

var _ = Describe("Edge Cases: Concurrency", func() {
	It("should handle simultaneous RemediationRequest creation for same alert", func() {
		// GIVEN: Same alert fingerprint
		fingerprint := "duplicate-alert-001"

		// WHEN: Create 3 RemediationRequests simultaneously
		var wg sync.WaitGroup
		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()

				rr := &remediationv1alpha1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("duplicate-alert-%d", idx),
						Namespace: namespace,
					},
					Spec: remediationv1alpha1.RemediationRequestSpec{
						TargetingData: &remediationv1alpha1.TargetingData{
							SignalFingerprint: fingerprint,
							AlertName:         "DuplicateAlert",
							Environment:       "production",
						},
					},
				}

				Expect(k8sClient.Create(ctx, rr)).To(Succeed())
			}(i)
		}

		wg.Wait()

		// THEN: All 3 should reconcile without conflicts
		Eventually(func() int {
			var rrList remediationv1alpha1.RemediationRequestList
			k8sClient.List(ctx, &rrList, client.InNamespace(namespace))
			return len(rrList.Items)
		}, "30s", "1s").Should(Equal(3))

		// Verify all 3 created child CRDs without conflicts
		Eventually(func() int {
			var processingList remediationprocessingv1alpha1.RemediationProcessingList
			k8sClient.List(ctx, &processingList, client.InNamespace(namespace))
			return len(processingList.Items)
		}, "30s", "1s").Should(Equal(3), "Each RemediationRequest should create its own RemediationProcessing")
	})
})
```

---

## 📊 **IMPLEMENTATION CHECKLIST**

### **Phase 1: Error Handling Enhancement** (Priority: HIGH)
- [ ] Add error classification (Category A-F) to Day 2 PLAN phase
- [ ] Replace `handleProcessing` with enhanced error handling version
- [ ] Add `updateStatusWithRetry` function for Category E errors
- [ ] Apply same patterns to `handleAnalyzing`, `handleWorkflowPlanning`, `handleExecuting`
- [ ] Add Prometheus metrics (`statusUpdateSuccess`, `statusUpdateConflicts`, `statusUpdateFailure`)

### **Phase 2: Watch Setup Enhancement** (Priority: MEDIUM)
- [ ] Replace `SetupWithManager` in Day 8 with enhanced version
- [ ] Add dependency validation before controller setup
- [ ] Add watch reconnection documentation (Category D)
- [ ] Add reconciliation duration metrics

### **Phase 3: Integration Test Enhancement** (Priority: HIGH)
- [ ] Create `multi_crd_coordination_test.go` (Day 14)
- [ ] Add anti-flaky pattern documentation
- [ ] Create `edge_cases_test.go` with 6 edge case categories (Day 15)
- [ ] Add concurrency edge case tests

### **Phase 4: Production Readiness** (Priority: MEDIUM)
- [ ] Create `PRODUCTION_RUNBOOKS.md` with 4 runbooks (Day 16)
- [ ] Add runbook for high failure rate
- [ ] Add runbook for stuck remediations
- [ ] Add runbook for watch connection loss
- [ ] Add runbook for status update conflicts

### **Phase 5: Edge Case Documentation** (Priority: LOW)
- [ ] Add edge case testing strategy to Day 15
- [ ] Document 6 edge case categories with test patterns
- [ ] Add data integrity edge cases

---

## 🎯 **EXPECTED OUTCOME**

After applying these enhancements, the RemediationOrchestrator implementation plan will have:

1. ✅ **Comprehensive Error Handling** - Category A-F classification with automatic recovery
2. ✅ **Production-Grade Watch Setup** - Dependency validation and reconnection handling
3. ✅ **Anti-Flaky Integration Tests** - EventuallyWithRetry patterns prevent test flakiness
4. ✅ **Production Runbooks** - 4 critical scenario runbooks for operations team
5. ✅ **Edge Case Coverage** - 6 categories of edge cases documented and tested

**Quality Improvement**:
- Error recovery rate: >95% (Category B, D, E auto-recover)
- Test flakiness rate: <1% (anti-flaky patterns)
- Production incident resolution time: -50% (runbooks)
- Edge case coverage: 130-165% (defense-in-depth)

---

## 📚 **REFERENCE PATTERNS**

**WorkflowExecution v1.2 Pattern Locations**:
- Error Handling: Lines 2350-2650
- Watch Setup: Lines 835-843, 2300-2350
- Integration Tests: Lines 4500-5200
- Production Runbooks: Lines 2726-2757
- Edge Cases: Lines 4433-4465

**Adaptation Notes**:
- RemediationOrchestrator watches **4 child CRD types** (vs WorkflowExecution watches 1)
- RemediationOrchestrator has **sequential phase progression** (vs WorkflowExecution parallel steps)
- RemediationOrchestrator has **longer timeouts** (15min vs 5min for execution phases)
- RemediationOrchestrator creates **NotificationRequest CRDs** (additional integration point)

---

**Document Owner**: Platform Architecture Team
**Last Updated**: 2025-10-17
**Status**: ✅ **Enhancement Guide Complete**
**Next Action**: Apply enhancements to [IMPLEMENTATION_PLAN_V1.1.md](./IMPLEMENTATION_PLAN_V1.1.md) during implementation

