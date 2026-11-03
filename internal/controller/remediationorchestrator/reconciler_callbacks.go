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

// Phase-handler callback helpers (event recording, lock acquisition, RAR
// updates, pre-remediation hash capture) shared across the phase.Registry
// callbacks wired in NewReconciler. Split out of reconciler.go per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520) to keep the file
// under the 700-line convention threshold. Pure structural move — no
// behavior change.
package controller

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sretry "k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationapprovalrequest"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/override"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
	"github.com/jordigilh/kubernaut/pkg/remediationrequest"
)

// recordEvent emits a Kubernetes event if a Recorder is configured. Shared by
// AnalyzingCallbacks and AwaitingApprovalCallbacks (previously duplicated
// closures); extracted per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *Reconciler) recordEvent(rr *remediationv1.RemediationRequest, eventType, reason, message string) {
	if r.Recorder != nil {
		r.Recorder.Event(rr, eventType, reason, message)
	}
}

// fetchFreshRR refetches a RemediationRequest via the cache-bypassed apiReader.
// Extracted from AnalyzingCallbacks.FetchFreshRR per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *Reconciler) fetchFreshRR(ctx context.Context, key client.ObjectKey) (*remediationv1.RemediationRequest, error) {
	freshRR := &remediationv1.RemediationRequest{}
	err := r.apiReader.Get(ctx, key, freshRR)
	return freshRR, err
}

// acquireLock acquires the distributed lock for target when a lockManager is
// configured; otherwise it is a no-op that always succeeds. Shared by
// AnalyzingCallbacks and AwaitingApprovalCallbacks (previously duplicated
// closures); extracted per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *Reconciler) acquireLock(ctx context.Context, target string) (bool, error) {
	if r.lockManager == nil {
		return true, nil
	}
	return r.lockManager.AcquireLock(ctx, target)
}

// releaseLock releases the distributed lock for target when a lockManager is
// configured; otherwise it is a no-op. Shared by AnalyzingCallbacks and
// AwaitingApprovalCallbacks (previously duplicated closures); extracted per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *Reconciler) releaseLock(ctx context.Context, target string) error {
	if r.lockManager == nil {
		return nil
	}
	return r.lockManager.ReleaseLock(ctx, target)
}

// capturePreRemediationHashForCallback resolves the appropriate reader for
// clusterID and captures the pre-remediation resource hash. Shared by
// AnalyzingCallbacks and AwaitingApprovalCallbacks (previously duplicated
// closures); extracted per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *Reconciler) capturePreRemediationHashForCallback(ctx context.Context, kind, name, namespace, clusterID string) (string, string, error) {
	reader, err := r.readerForHash(ctx, clusterID)
	if err != nil {
		return "", "", fmt.Errorf("failed to get reader for cluster %s: %w", clusterID, err)
	}
	return CapturePreRemediationHash(ctx, reader, r.restMapper, kind, name, namespace)
}

// resolveDualTargetsForCallback adapts resolveDualTargets' internal result
// type to the DualTargetResult shape expected by phase-handler callbacks.
// Shared by AnalyzingCallbacks and AwaitingApprovalCallbacks (previously
// duplicated closures); extracted per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2
// (issue #1520).
func resolveDualTargetsForCallback(rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) DualTargetResult {
	dt := resolveDualTargets(rr, ai)
	return DualTargetResult{Remediation: TargetRef{Kind: dt.Remediation.Kind, Name: dt.Remediation.Name, Namespace: dt.Remediation.Namespace}}
}

// persistPreRemediationHash stamps the captured pre-remediation spec hash
// onto the RemediationRequest status. Shared by AnalyzingCallbacks and
// AwaitingApprovalCallbacks (previously duplicated closures); extracted per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *Reconciler) persistPreRemediationHash(ctx context.Context, rr *remediationv1.RemediationRequest, preHash string) error {
	return helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.EnsureOperatorAudit().PreRemediationSpecHash = preHash
		remediationrequest.SetPreRemediationHashCaptured(rr, true,
			"Pre-remediation hash captured", r.Metrics)
		return nil
	})
}

// resolveWorkflowOverride resolves an operator-supplied WorkflowOverride
// against the AI-selected workflow. Extracted from
// AwaitingApprovalCallbacks.ResolveWorkflow per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *Reconciler) resolveWorkflowOverride(ctx context.Context, wo *remediationv1.WorkflowOverride, sw *aianalysisv1.SelectedWorkflow, ns string) (*aianalysisv1.SelectedWorkflow, bool, error) {
	return override.ResolveWorkflow(ctx, r.apiReader, wo, sw, ns)
}

// updateRARConditionsOnDecision records an approve/reject decision on a
// RemediationApprovalRequest's status conditions, retrying on optimistic
// concurrency conflicts. Extracted from
// AwaitingApprovalCallbacks.UpdateRARConditions per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *Reconciler) updateRARConditionsOnDecision(ctx context.Context, _ *remediationv1.RemediationRequest, rar *remediationv1.RemediationApprovalRequest, decision string) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		if err := r.client.Get(ctx, client.ObjectKeyFromObject(rar), rar); err != nil {
			return err
		}
		// Decision-bearing conditions are immutable after AuthWebhook commits
		// the terminal business tuple. AuditRecorded is the only post-terminal
		// condition owned by a controller. Keep this callback idempotent for
		// older callers without rewriting approval conditions.
		if rar.Status.Decision != remediationv1.ApprovalDecisionPending {
			return nil
		}
		remediationapprovalrequest.SetApprovalPending(rar, false, "Decision received", r.Metrics)
		switch decision {
		case "approved":
			remediationapprovalrequest.SetApprovalDecided(rar, true,
				remediationapprovalrequest.ReasonApproved,
				fmt.Sprintf("Approved by %s", rar.Status.DecidedBy), r.Metrics)
		case "rejected":
			remediationapprovalrequest.SetApprovalDecided(rar, true,
				remediationapprovalrequest.ReasonRejected,
				fmt.Sprintf("Rejected by %s: %s", rar.Status.DecidedBy, rar.Status.DecisionMessage), r.Metrics)
		}
		remediationapprovalrequest.SetReady(rar, true, remediationapprovalrequest.ReasonReady, "Approval decided", r.Metrics)
		return r.client.Status().Update(ctx, rar)
	})
}

// removeApprovalRequestFinalizer removes the RAR cleanup finalizer using a
// cache-bypassed read and the current resourceVersion. The expected UID keeps
// a delayed deletion event from finalizing a same-name replacement object.
func (r *Reconciler) removeApprovalRequestFinalizer(ctx context.Context, key client.ObjectKey, expectedUID types.UID) error {
	reader := r.apiReader
	if reader == nil {
		reader = r.client
	}

	return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		current := &remediationv1.RemediationApprovalRequest{}
		if err := reader.Get(ctx, key, current); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("read RemediationApprovalRequest before finalizer removal: %w", err)
		}
		if expectedUID != "" && current.UID != expectedUID {
			return fmt.Errorf("refusing to remove RAR finalizer from UID %q; expected %q", current.UID, expectedUID)
		}
		if !controllerutil.ContainsFinalizer(current, creator.ApprovalRequestFinalizerName) {
			return nil
		}
		controllerutil.RemoveFinalizer(current, creator.ApprovalRequestFinalizerName)
		if err := r.client.Update(ctx, current); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("remove RemediationApprovalRequest finalizer: %w", err)
		}
		return nil
	})
}

// expireRARWithoutDecision marks a RemediationApprovalRequest as expired when
// no decision was received before its deadline, retrying on optimistic
// concurrency conflicts. Extracted from AwaitingApprovalCallbacks.ExpireRAR
// per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *Reconciler) expireRARWithoutDecision(ctx context.Context, rar *remediationv1.RemediationApprovalRequest) (ExpiryOutcome, error) {
	reader := r.apiReader
	if reader == nil {
		reader = r.client
	}

	var authoritative *remediationv1.RemediationApprovalRequest
	outcome := Unavailable
	err := k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		current := &remediationv1.RemediationApprovalRequest{}
		if err := reader.Get(ctx, client.ObjectKeyFromObject(rar), current); err != nil {
			return fmt.Errorf("read approval request before expiry: %w", err)
		}
		authoritative = current

		if current.Status.Decision != remediationv1.ApprovalDecisionPending {
			outcome = classifyExpiryDecision(current, r.clock.Now())
			return nil
		}

		now := r.clock.Now()
		if current.Spec.RequiredBy.IsZero() || now.Before(current.Spec.RequiredBy.Time) {
			return fmt.Errorf("approval request is not authoritatively expired")
		}

		confirmed, err := r.persistAndConfirmRARExpiry(ctx, reader, current, now)
		if err != nil {
			return err
		}
		*current = *confirmed.DeepCopy()
		authoritative = current
		outcome = ExpiredConfirmed
		return nil
	})

	if authoritative != nil {
		*rar = *authoritative.DeepCopy()
	}
	if err != nil {
		return Unavailable, err
	}
	return outcome, nil
}

func classifyExpiryDecision(rar *remediationv1.RemediationApprovalRequest, now time.Time) ExpiryOutcome {
	switch {
	case isConfirmedExpiredAt(rar, now):
		return ExpiredConfirmed
	case isTimelyDecisionAt(rar, now):
		return DecisionWon
	default:
		return Unavailable
	}
}

func (r *Reconciler) persistAndConfirmRARExpiry(ctx context.Context, reader client.Reader, current *remediationv1.RemediationApprovalRequest, now time.Time) (*remediationv1.RemediationApprovalRequest, error) {
	remediationapprovalrequest.SetApprovalPending(current, false, "Expired without decision", r.Metrics)
	remediationapprovalrequest.SetApprovalExpired(current, true,
		fmt.Sprintf("Expired after %v without decision",
			now.Sub(current.ObjectMeta.CreationTimestamp.Time).Round(time.Minute)), r.Metrics)
	remediationapprovalrequest.SetReady(current, false, remediationapprovalrequest.ReasonNotReady, "Approval expired", r.Metrics)
	current.Status.Decision = remediationv1.ApprovalDecisionExpired
	current.Status.DecidedBy = systemDecisionActor
	current.Status.DecidedVia = ""
	current.Status.Expired = true
	current.Status.TimeRemaining = remediationapprovalrequest.ComputeTimeRemaining(current.Spec.RequiredBy.Time, now)
	decidedAt := metav1.NewTime(now)
	current.Status.DecidedAt = &decidedAt
	if err := r.client.Status().Update(ctx, current); err != nil {
		return nil, err
	}

	// Confirm the status write through the authoritative reader before
	// allowing the parent to fail. A successful client call alone does not
	// establish that the terminal expiry is the state now observed by RO.
	confirmed := &remediationv1.RemediationApprovalRequest{}
	if err := reader.Get(ctx, client.ObjectKeyFromObject(current), confirmed); err != nil {
		return nil, fmt.Errorf("read approval request after expiry write: %w", err)
	}
	if !isConfirmedExpiredAt(confirmed, now) {
		return nil, fmt.Errorf("approval request expiry write could not be authoritatively confirmed")
	}
	return confirmed, nil
}

// updateRARTimeRemaining refreshes the TimeRemaining status field on a
// RemediationApprovalRequest, retrying on optimistic concurrency conflicts.
// Extracted from AwaitingApprovalCallbacks.UpdateRARTimeRemaining per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *Reconciler) updateRARTimeRemaining(ctx context.Context, rar *remediationv1.RemediationApprovalRequest) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		if err := r.client.Get(ctx, client.ObjectKeyFromObject(rar), rar); err != nil {
			return err
		}
		rar.Status.TimeRemaining = remediationapprovalrequest.ComputeTimeRemaining(rar.Spec.RequiredBy.Time, r.clock.Now())
		return r.client.Status().Update(ctx, rar)
	})
}

// SetRetentionPeriod configures how long terminal RemediationRequest CRDs persist
// before automatic cleanup. Default: 24h. Issue #265.
// Thread-safe: acquires configMu write lock (#835, DD-INFRA-001).
func (r *Reconciler) SetRetentionPeriod(d time.Duration) {
	if d > 0 {
		r.configMu.Lock()
		r.retentionPeriod = d
		r.configMu.Unlock()
	}
}

// getRetentionPeriod returns the current retention TTL for terminal RRs.
// Thread-safe: acquires configMu read lock (#835).
func (r *Reconciler) getRetentionPeriod() time.Duration {
	r.configMu.RLock()
	defer r.configMu.RUnlock()
	return r.retentionPeriod
}

// SetDSClient wires the DataStorage history querier into the routing engine.
// Must be called after NewReconciler if the default routing engine is used in production.
// Issue #214: Enables CheckIneffectiveRemediationChain to query real DS data.
func (r *Reconciler) SetDSClient(dsClient routing.RemediationHistoryQuerier) {
	if re, ok := r.routingEngine.(*routing.RoutingEngine); ok {
		re.SetDSClient(dsClient)
	} else {
		log.Log.Info("SetDSClient skipped: routing engine is not *routing.RoutingEngine (mock or custom implementation)")
	}
}

// SetDryRun configures dry-run mode for the RO reconciler.
// When enabled, the pipeline stops after AI analysis without creating WFE or EA.
// holdPeriod controls how long the Gateway suppresses re-triggering for the same fingerprint.
// #712, #736: Called from cmd/remediationorchestrator/main.go.
// Thread-safe: acquires configMu write lock (#835, DD-INFRA-001).
func (r *Reconciler) SetDryRun(enabled bool, holdPeriod time.Duration) {
	r.configMu.Lock()
	r.dryRun = enabled
	r.dryRunHoldPeriod = holdPeriod
	r.configMu.Unlock()
}

// isDryRun returns whether dry-run mode is enabled.
// Thread-safe: acquires configMu read lock (#835).
func (r *Reconciler) isDryRun() bool {
	r.configMu.RLock()
	defer r.configMu.RUnlock()
	return r.dryRun
}

// getDryRunHoldPeriod returns the current dry-run suppression window.
// Thread-safe: acquires configMu read lock (#835).
func (r *Reconciler) getDryRunHoldPeriod() time.Duration {
	r.configMu.RLock()
	defer r.configMu.RUnlock()
	return r.dryRunHoldPeriod
}
