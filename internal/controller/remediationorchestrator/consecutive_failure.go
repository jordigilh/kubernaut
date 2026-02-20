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

package controller

import (
	"context"
	"fmt"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// ════════════════════════════════════════════════════════════════════════════
// BR-ORCH-042: Consecutive Failure Blocking with Automatic Cooldown
// ════════════════════════════════════════════════════════════════════════════
//
// Business Logic Component for preventing infinite remediation loops by
// blocking signals that fail 3+ times consecutively.
//
// Design Decision: RO owns this logic (not Gateway) because:
// - RO knows *why* failures happened (timeout, workflow failure, approval rejection)
// - RO already tracks recovery attempts
// - Routing decisions are orchestration responsibility
// - Gateway should be a "dumb pipe" for signal ingestion
//
// Key Features:
// - Count consecutive failures per signal fingerprint
// - Block after threshold (default: 3 failures)
// - Auto-expire after cooldown (default: 1 hour)
// - Create NotificationRequest for operator awareness
// - Use field selectors on spec.signalFingerprint (immutable)
//
// See: docs/requirements/BR-ORCH-042-consecutive-failure-blocking.md
// ════════════════════════════════════════════════════════════════════════════

// ConsecutiveFailureBlocker handles consecutive failure detection and blocking logic.
// This is a separate component from the reconciler to enable clean testing and reuse.
type ConsecutiveFailureBlocker struct {
	client        client.Client
	threshold     int           // Number of consecutive failures before blocking (default: 3)
	cooldown      time.Duration // How long to block before auto-retry (default: 1 hour)
	notifyOnBlock bool          // Whether to create NotificationRequest when blocking
}

// NewConsecutiveFailureBlocker creates a new ConsecutiveFailureBlocker.
// BR-ORCH-042.1, BR-ORCH-042.3
func NewConsecutiveFailureBlocker(
	client client.Client,
	threshold int,
	cooldown time.Duration,
	notifyOnBlock bool,
) *ConsecutiveFailureBlocker {
	return &ConsecutiveFailureBlocker{
		client:        client,
		threshold:     threshold,
		cooldown:      cooldown,
		notifyOnBlock: notifyOnBlock,
	}
}

// CountConsecutiveFailures counts how many times the same signal fingerprint has failed consecutively.
// It stops counting when it encounters a Completed RR (success resets the counter).
// Uses field selector on spec.signalFingerprint (immutable) per BR-ORCH-042.1.
//
// AC-042-1-1: Count consecutive Failed RRs for same fingerprint
// AC-042-1-2: Count resets on any Completed RR
// AC-042-1-3: Count uses chronological order (most recent first)
// AC-042-1-4: Use field selector on spec.signalFingerprint (not labels)
func (b *ConsecutiveFailureBlocker) CountConsecutiveFailures(ctx context.Context, fingerprint string) (int, error) {
	rrList := &remediationv1.RemediationRequestList{}

	// Use field selector on immutable spec.signalFingerprint (full 64-char SHA256)
	// NOT labels (mutable, truncated to 63 chars)
	err := b.client.List(ctx, rrList,
		client.MatchingFields{"spec.signalFingerprint": fingerprint},
	)
	if err != nil {
		return 0, fmt.Errorf("failed to list RemediationRequests for fingerprint %s: %w", fingerprint, err)
	}

	if len(rrList.Items) == 0 {
		return 0, nil
	}

	// Sort by creation time (most recent first)
	sort.Slice(rrList.Items, func(i, j int) bool {
		return rrList.Items[i].CreationTimestamp.After(rrList.Items[j].CreationTimestamp.Time)
	})

	// Count consecutive failures from most recent backwards
	consecutiveCount := 0
	for _, rr := range rrList.Items {
		if rr.Status.OverallPhase == remediationv1.PhaseFailed {
			consecutiveCount++
		} else if rr.Status.OverallPhase == remediationv1.PhaseCompleted {
			// Success resets the counter - stop counting
			break
		}
		// Ignore other phases (Pending, Processing, etc.) - they don't reset count
	}

	return consecutiveCount, nil
}

// BlockIfNeeded checks if the RR should be blocked due to consecutive failures.
// If threshold is reached, transitions RR to Blocked phase with cooldown.
// Creates NotificationRequest if notifyOnBlock is enabled.
//
// AC-042-1-1: RO detects when ≥3 consecutive failures
// AC-042-2-1: Blocked is non-terminal phase
// AC-042-3-1: RO sets BlockedUntil when blocking
// AC-042-5-1: NotificationRequest created when blocking
func (b *ConsecutiveFailureBlocker) BlockIfNeeded(ctx context.Context, rr *remediationv1.RemediationRequest) error {
	logger := ctrl.LoggerFrom(ctx).WithName("consecutive-failure-blocker")

	// Count consecutive failures for this fingerprint
	count, err := b.CountConsecutiveFailures(ctx, rr.Spec.SignalFingerprint)
	if err != nil {
		return fmt.Errorf("failed to count consecutive failures: %w", err)
	}

	// Check if threshold exceeded
	if count < b.threshold {
		logger.Info("Consecutive failure threshold not reached",
			"fingerprint", rr.Spec.SignalFingerprint,
			"count", count,
			"threshold", b.threshold,
		)
		return nil
	}

	// Threshold exceeded - block this RR
	logger.Info("Consecutive failure threshold exceeded - blocking",
		"fingerprint", rr.Spec.SignalFingerprint,
		"count", count,
		"threshold", b.threshold,
		"cooldown", b.cooldown,
	)

	// Set BlockedUntil (now + cooldown duration)
	blockedUntil := metav1.NewTime(time.Now().Add(b.cooldown))
	rr.Status.BlockedUntil = &blockedUntil

	// Set BlockReason (using CRD constant)
	rr.Status.BlockReason = string(remediationv1.BlockReasonConsecutiveFailures)

	// Set phase to Blocked (non-terminal)
	rr.Status.OverallPhase = remediationv1.PhaseBlocked
	rr.Status.Message = fmt.Sprintf("%d consecutive failures detected - cooldown until %s",
		count, blockedUntil.Format(time.RFC3339))

	// Create NotificationRequest if enabled
	if b.notifyOnBlock {
		if err := b.createBlockNotification(ctx, rr, count); err != nil {
			// Non-fatal error - log and continue
			logger.Error(err, "Failed to create block notification")
		}
	}

	return nil
}

// createBlockNotification creates a NotificationRequest when blocking a fingerprint.
// AC-042-5-1, AC-042-5-2
func (b *ConsecutiveFailureBlocker) createBlockNotification(ctx context.Context, rr *remediationv1.RemediationRequest, failureCount int) error {
	notif := &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("consecutive-failure-%s", rr.Name),
			Namespace: rr.Namespace,
			// Owner reference for cascade deletion
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         rr.APIVersion,
					Kind:               rr.Kind,
					Name:               rr.Name,
					UID:                rr.UID,
					Controller:         boolPtr(true),
					BlockOwnerDeletion: boolPtr(false),
				},
			},
		},
		Spec: notificationv1.NotificationRequestSpec{
			// Set RemediationRequestRef for audit correlation (BR-NOT-064)
			RemediationRequestRef: &corev1.ObjectReference{
				APIVersion: rr.APIVersion,
				Kind:       rr.Kind,
				Name:       rr.Name,
				Namespace:  rr.Namespace,
				UID:        rr.UID,
			},
			Type:     notificationv1.NotificationType("consecutive_failures_blocked"),
			Priority: "high",
			Severity: rr.Spec.Severity,
			Subject:  fmt.Sprintf("⚠️ Remediation Blocked: %s (Consecutive Failures)", rr.Spec.SignalName),
			Body: fmt.Sprintf(`The remediation for signal "%s" has been blocked due to %d consecutive failures.

Signal Fingerprint: %s
Cooldown Duration: %s
Blocked Until: %s

Manual intervention is required to investigate the root cause. The signal will be automatically
retried after the cooldown period expires.

Actions:
1. Investigate why remediation repeatedly failed
2. Fix underlying issues (RBAC, infrastructure, workflow logic)
3. Optionally delete this RemediationRequest to manually unblock sooner

Failed RemediationRequest: %s/%s`,
				rr.Spec.SignalName,
				failureCount,
				rr.Spec.SignalFingerprint,
				b.cooldown.String(),
				rr.Status.BlockedUntil.Format(time.RFC3339),
				rr.Namespace,
				rr.Name,
			),
		},
	}

	if err := b.client.Create(ctx, notif); err != nil {
		return fmt.Errorf("failed to create NotificationRequest: %w", err)
	}

	// Add notification reference to RR status (BR-ORCH-035)
	if rr.Status.NotificationRequestRefs == nil {
		rr.Status.NotificationRequestRefs = []corev1.ObjectReference{}
	}
	notifRef := corev1.ObjectReference{
		Kind:       "NotificationRequest",
		Name:       notif.Name,
		Namespace:  notif.Namespace,
		UID:        notif.UID,
		APIVersion: notificationv1.GroupVersion.String(),
	}
	rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs, notifRef)

	return nil
}

// Helper functions

func boolPtr(b bool) *bool {
	return &b
}
