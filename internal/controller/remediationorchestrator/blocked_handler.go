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
	"fmt"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
)

// BlockedCallbacks provides the reconciler methods that the BlockedHandler
// delegates to. These are heavy methods with deep reconciler dependencies
// (apiReader, routingEngine, handleBlocked, transitionToInherited*).
//
// Reference: Issue #666, TP-666-v1 §8.2
type BlockedCallbacks struct {
	RecheckResourceBusyBlock      func(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error)
	RecheckDuplicateBlock         func(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error)
	HandleUnmanagedResourceExpiry func(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error)
	TransitionToFailedTerminal    func(ctx context.Context, rr *remediationv1.RemediationRequest, failurePhase remediationv1.FailurePhase, failureErr error) (ctrl.Result, error)
}

// BlockedHandler encapsulates the reconcile logic for the Blocked phase.
//
// Internalizes logic from reconciler.handleBlockedPhase (blocking.go).
//
// Reference: Issue #666, TP-666-v1 §8.2
type BlockedHandler struct {
	m         *metrics.Metrics
	callbacks BlockedCallbacks
}

func NewBlockedHandler(
	m *metrics.Metrics,
	callbacks BlockedCallbacks,
) *BlockedHandler {
	return &BlockedHandler{
		m:         m,
		callbacks: callbacks,
	}
}

func (h *BlockedHandler) Phase() phase.Phase {
	return phase.Blocked
}

func (h *BlockedHandler) Handle(ctx context.Context, rr *remediationv1.RemediationRequest) (phase.TransitionIntent, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	if rr.Status.BlockedUntil == nil {
		switch remediationv1.BlockReason(rr.Status.BlockReason) {
		case remediationv1.BlockReasonResourceBusy:
			result, err := h.callbacks.RecheckResourceBusyBlock(ctx, rr)
			if err != nil {
				return phase.TransitionIntent{}, err
			}
			return resultToIntent(result, "recheckResourceBusy"), nil
		case remediationv1.BlockReasonDuplicateInProgress:
			result, err := h.callbacks.RecheckDuplicateBlock(ctx, rr)
			if err != nil {
				return phase.TransitionIntent{}, err
			}
			return resultToIntent(result, "recheckDuplicateBlock"), nil
		default:
			logger.V(1).Info("RR is manually blocked, no auto-expiry")
			return phase.NoOp("manually blocked"), nil
		}
	}

	if time.Now().After(rr.Status.BlockedUntil.Time) {
		if remediationv1.BlockReason(rr.Status.BlockReason) == remediationv1.BlockReasonUnmanagedResource {
			result, err := h.callbacks.HandleUnmanagedResourceExpiry(ctx, rr)
			if err != nil {
				return phase.TransitionIntent{}, err
			}
			return resultToIntent(result, "unmanagedResourceExpiry"), nil
		}

		logger.Info("Blocked cooldown expired, transitioning to terminal Failed")
		if h.m != nil {
			h.m.CurrentBlockedGauge.WithLabelValues(rr.Namespace).Dec()
		}

		blockReason := "unknown"
		if rr.Status.BlockReason != "" {
			blockReason = string(rr.Status.BlockReason)
		}

		result, err := h.callbacks.TransitionToFailedTerminal(ctx, rr, remediationv1.FailurePhaseBlocked,
			fmt.Errorf("cooldown expired after blocking due to %s", blockReason))
		if err != nil {
			return phase.TransitionIntent{}, err
		}
		return resultToIntent(result, "cooldown expired"), nil
	}

	requeueAfter := time.Until(rr.Status.BlockedUntil.Time)
	logger.V(1).Info("Still blocked, requeueing at expiry",
		"blockedUntil", rr.Status.BlockedUntil.Format(time.RFC3339),
		"requeueAfter", requeueAfter)
	return phase.Requeue(requeueAfter, "blocked cooldown active"), nil
}

// resultToIntent converts a ctrl.Result from a legacy callback into a TransitionIntent.
func resultToIntent(result ctrl.Result, reason string) phase.TransitionIntent {
	if result.Requeue {
		return phase.RequeueNow(reason)
	}
	if result.RequeueAfter > 0 {
		return phase.Requeue(result.RequeueAfter, reason)
	}
	return phase.NoOp(reason)
}
