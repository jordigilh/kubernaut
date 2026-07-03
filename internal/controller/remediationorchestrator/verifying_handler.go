/*
Copyright 2026 Jordi Gil.

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
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/config"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
)

// VerifyingCallbacks provides the reconciler methods that the VerifyingHandler
// delegates to. This callback injection pattern isolates the handler from
// heavy reconciler dependencies while preserving exact behavioral fidelity.
//
// Reference: Issue #666, TP-666-v1 §4.3
type VerifyingCallbacks struct {
	EnsureNotificationsCreated            func(ctx context.Context, rr *remediationv1.RemediationRequest)
	CreateEffectivenessAssessmentIfNeeded func(ctx context.Context, rr *remediationv1.RemediationRequest)
	TrackEffectivenessStatus              func(ctx context.Context, rr *remediationv1.RemediationRequest) error
	EmitVerificationTimedOutAudit         func(ctx context.Context, rr *remediationv1.RemediationRequest)
	EmitVerificationCompletedAudit        func(ctx context.Context, rr *remediationv1.RemediationRequest)
	EmitCompletionAudit                   func(ctx context.Context, rr *remediationv1.RemediationRequest, outcome string, durationMs int64)
}

// VerifyingHandler encapsulates the reconcile logic for the Verifying phase.
//
// Internalizes logic from reconciler.handleVerifyingPhase.
//
// Reference: Issue #666, TP-666-v1 §8.1
type VerifyingHandler struct {
	k8sClient client.Client
	m         *metrics.Metrics
	timeouts  TimeoutConfig
	callbacks VerifyingCallbacks
}

func NewVerifyingHandler(
	k8sClient client.Client,
	m *metrics.Metrics,
	timeouts TimeoutConfig,
	callbacks VerifyingCallbacks,
) *VerifyingHandler {
	return &VerifyingHandler{
		k8sClient: k8sClient,
		m:         m,
		timeouts:  timeouts,
		callbacks: callbacks,
	}
}

func (h *VerifyingHandler) Phase() phase.Phase {
	return phase.Verifying
}

func (h *VerifyingHandler) Handle(ctx context.Context, rr *remediationv1.RemediationRequest) (phase.TransitionIntent, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	if rr.Status.Outcome != "" {
		h.callbacks.EnsureNotificationsCreated(ctx, rr)
	}

	if rr.Status.EffectivenessAssessmentRef == nil {
		logger.Info("EffectivenessAssessmentRef is nil in Verifying phase, retrying EA creation")
		h.callbacks.CreateEffectivenessAssessmentIfNeeded(ctx, rr)

		if rr.Status.EffectivenessAssessmentRef == nil {
			logger.Info("EA creation still pending, requeueing")
			return phase.Requeue(config.RequeueResourceBusy, "EA creation pending"), nil
		}
	}

	if rr.Status.VerificationDeadline == nil {
		intent, done, err := h.ensureVerificationDeadline(ctx, rr, logger)
		if done {
			return intent, err
		}
	}

	if rr.Status.VerificationDeadline != nil && time.Now().After(rr.Status.VerificationDeadline.Time) {
		logger.Info("VerificationDeadline expired, timing out verification",
			"deadline", rr.Status.VerificationDeadline.Format(time.RFC3339))
		return h.completeAsVerificationTimedOut(ctx, rr, logger, "verification deadline expired, completed")
	}

	if err := h.callbacks.TrackEffectivenessStatus(ctx, rr); err != nil {
		logger.Error(err, "Failed to track EA status during Verifying phase")
	}

	if rr.Status.OverallPhase == phase.Completed {
		h.callbacks.EnsureNotificationsCreated(ctx, rr)
		h.callbacks.EmitVerificationCompletedAudit(ctx, rr)
		if rr.Status.StartTime != nil {
			h.callbacks.EmitCompletionAudit(ctx, rr, rr.Status.Outcome, time.Since(rr.Status.StartTime.Time).Milliseconds())
		}
		return phase.NoOp("EA terminal, verification completed"), nil
	}

	return phase.Requeue(config.RequeueResourceBusy, "EA still in progress"), nil
}

// ensureVerificationDeadline computes and persists VerificationDeadline from
// the EA's ValidityDeadline (plus buffer) on first entry to Verifying with no
// deadline set yet. If the EA hasn't computed ValidityDeadline and the RR has
// exceeded the configured Verifying timeout, this applies a safety-net
// completion (VerificationTimedOut) so the RR is never stuck indefinitely.
// Extracted from Handle per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
// Returns done=true when the caller must return (intent, err) immediately.
func (h *VerifyingHandler) ensureVerificationDeadline(ctx context.Context, rr *remediationv1.RemediationRequest, logger logr.Logger) (phase.TransitionIntent, bool, error) {
	eaName := rr.Status.EffectivenessAssessmentRef.Name
	ea := &eav1.EffectivenessAssessment{}
	if err := h.k8sClient.Get(ctx, client.ObjectKey{Name: eaName, Namespace: rr.Namespace}, ea); err != nil {
		logger.Error(err, "Failed to fetch EA for VerificationDeadline computation", "ea", eaName)
		return phase.Requeue(config.RequeueResourceBusy, "EA fetch error"), true, nil
	}

	if ea.Status.ValidityDeadline != nil {
		deadline := metav1.NewTime(ea.Status.ValidityDeadline.Add(VerificationDeadlineBuffer))
		if err := helpers.UpdateRemediationRequestStatus(ctx, h.k8sClient, rr, func(rr *remediationv1.RemediationRequest) error {
			rr.Status.VerificationDeadline = &deadline
			return nil
		}); err != nil {
			logger.Error(err, "Failed to persist VerificationDeadline")
			return phase.TransitionIntent{}, true, err
		}
		logger.Info("VerificationDeadline set", "deadline", deadline.Format(time.RFC3339))
		return phase.TransitionIntent{}, false, nil
	}

	if time.Since(rr.CreationTimestamp.Time) > h.timeouts.Verifying {
		logger.Info("Safety-net timeout: VerificationDeadline never set, RR exceeded verifying timeout",
			"age", time.Since(rr.CreationTimestamp.Time).String(),
			"verifyingTimeout", h.timeouts.Verifying.String())
		intent, err := h.completeAsVerificationTimedOut(ctx, rr, logger, "safety-net timeout completed")
		return intent, true, err
	}

	logger.V(1).Info("EA.Status.ValidityDeadline not yet set, requeueing")
	return phase.Requeue(config.RequeueResourceBusy, "ValidityDeadline not set"), true, nil
}

// completeAsVerificationTimedOut transitions the RR to Completed with
// outcome VerificationTimedOut, shared by the safety-net timeout (no
// VerificationDeadline ever computed) and the normal deadline-expiry path.
// Extracted from Handle per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2
// (issue #1520).
func (h *VerifyingHandler) completeAsVerificationTimedOut(ctx context.Context, rr *remediationv1.RemediationRequest, logger logr.Logger, noOpReason string) (phase.TransitionIntent, error) {
	if err := helpers.UpdateRemediationRequestStatus(ctx, h.k8sClient, rr, func(rr *remediationv1.RemediationRequest) error {
		now := metav1.Now()
		rr.Status.OverallPhase = phase.Completed
		rr.Status.Outcome = "VerificationTimedOut"
		rr.Status.CompletedAt = &now
		rr.Status.ObservedGeneration = rr.Generation
		return nil
	}); err != nil {
		logger.Error(err, "Failed to transition to Completed on verification timeout")
		return phase.TransitionIntent{}, err
	}
	if h.m != nil {
		h.m.PhaseTransitionsTotal.WithLabelValues(string(phase.Verifying), string(phase.Completed), rr.Namespace).Inc()
	}
	h.callbacks.EnsureNotificationsCreated(ctx, rr)
	h.callbacks.EmitVerificationTimedOutAudit(ctx, rr)
	if rr.Status.StartTime != nil {
		h.callbacks.EmitCompletionAudit(ctx, rr, rr.Status.Outcome, time.Since(rr.Status.StartTime.Time).Milliseconds())
	}
	return phase.NoOp(noOpReason), nil
}
