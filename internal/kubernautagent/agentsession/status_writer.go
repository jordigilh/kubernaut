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

package agentsession

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// maxStatusUpdateRetries bounds retry-on-conflict for Status subresource
// updates. KA is the sole writer of AgentSession.Status (BR-AA-KA-065.9),
// so a conflict here means a concurrent write by this same Dispatcher (e.g.
// a renewal race), never a cross-service writer collision.
const maxStatusUpdateRetries = 3

// updateStatus fetches the current AgentSession, applies mutate, and
// updates the Status subresource, retrying on Conflict. mutate observes and
// may adjust the current Status in place; it is responsible for its own
// idempotency (e.g. refusing to regress a terminal phase).
func (d *Dispatcher) updateStatus(ctx context.Context, key crclient.ObjectKey, mutate func(*agentsessionv1.AgentSession)) {
	for attempt := 0; attempt < maxStatusUpdateRetries; attempt++ {
		// Bounded per-call deadline (same rationale as dispatcher.go's
		// listTimeout uses): callers such as OnTerminal/OnInteractiveUpgrade
		// pass context.Background() with no deadline of its own, and this
		// Dispatcher's client deliberately carries no rest.Config.Timeout.
		callCtx, cancel := context.WithTimeout(ctx, listTimeout)

		as := &agentsessionv1.AgentSession{}
		if err := d.client.Get(callCtx, key, as); err != nil {
			cancel()
			if apierrors.IsNotFound(err) {
				d.logger.Info("agentsession no longer exists, skipping status write", "agentSession", key.Name, "namespace", key.Namespace)
				return
			}
			d.logger.Error(err, "failed to get agentsession for status update", "agentSession", key.Name)
			return
		}
		mutate(as)
		err := d.client.Status().Update(callCtx, as)
		cancel()
		if err != nil {
			if apierrors.IsConflict(err) {
				continue
			}
			d.logger.Error(err, "failed to update agentsession status", "agentSession", key.Name)
			return
		}
		return
	}
	d.logger.Error(nil, "agentsession status update: retries exhausted", "agentSession", key.Name, "namespace", key.Namespace)
}

// writeDispatchedStatus records that this replica won the dispatch Lease
// and started (or, for BR-INTERACTIVE-010 deferred-interactive, registered
// but did not yet launch) the investigation. interactive reflects the
// dispatcher's own dispatch-time InvestigationSession-existence check
// (DD-AA-KA-001 Amendment Gap 1) -- never a Spec field, since Spec is
// immutable and cannot hold a trustworthy snapshot of a fact that can
// become true after Create. Never regresses a Status that has already
// advanced past Pending (e.g. a duplicate dispatch attempt racing a
// terminal write).
func (d *Dispatcher) writeDispatchedStatus(ctx context.Context, key crclient.ObjectKey, sessionID string, interactive bool) {
	d.updateStatus(ctx, key, func(as *agentsessionv1.AgentSession) {
		if as.Status.Phase != "" && as.Status.Phase != agentsessionv1.AgentSessionPhasePending {
			return
		}
		as.Status.SessionID = sessionID
		if interactive {
			// BR-INTERACTIVE-010: session registered in pending state,
			// awaiting MCP action=start -- Phase intentionally stays
			// Pending; DispatchedAt is set once investigation actually
			// launches (LaunchDeferredInvestigation), not here.
			as.Status.Phase = agentsessionv1.AgentSessionPhasePending
			as.Status.Interactive = true
			return
		}
		as.Status.Phase = agentsessionv1.AgentSessionPhaseInvestigating
		now := metav1.Now()
		as.Status.DispatchedAt = &now
	})
}

// writeFailedStatus records a curated (SI-11), user-facing failure message
// when dispatch itself could not start (e.g. session.Manager capacity
// exhausted) -- distinct from an investigation that started and later
// failed, which writeTerminalStatus handles. reason is a curated,
// machine-readable classification (AgentSessionStatus.Reason, e.g.
// AgentSessionReasonCapacityExceeded) for a dispatch-start failure whose
// cause AA needs to act on differently than a generic one; pass "" when no
// such classification applies.
func (d *Dispatcher) writeFailedStatus(ctx context.Context, key crclient.ObjectKey, curatedMessage, reason string) {
	d.updateStatus(ctx, key, func(as *agentsessionv1.AgentSession) {
		if isTerminalPhase(as.Status.Phase) {
			return
		}
		as.Status.Phase = agentsessionv1.AgentSessionPhaseFailed
		as.Status.Error = curatedMessage
		as.Status.Reason = reason
		now := metav1.Now()
		as.Status.CompletedAt = &now
	})
}

// writeTerminalStatus records a session.TerminalSnapshot's outcome
// (DD-AA-KA-001 Amendment Gap 2): status is the already-resolved
// session.Status the hook observed as actually committed --
// StatusCompleted/StatusFailed/StatusCancelled for a genuine terminal
// outcome, or StatusUserDriving for an autonomous InteractiveHold (an
// investigation reached RCA-complete and is holding for a human decision
// via MCP, no acting user yet). This is now the SOLE status-writing path
// for post-dispatch outcomes -- called only from Dispatcher.OnTerminal,
// itself only ever invoked by session.Manager's TerminalHook, which fires
// exactly once for whichever call site actually won the transition
// (investigateFn's own goroutine, or an out-of-band
// CompleteUserDriving/ForceCompleteByRemediationID/CancelInvestigation).
//
// StatusUserDriving intentionally does NOT write a terminal phase here --
// doing so would let AA observe a premature Completed/Result before the
// eventual human decision (select_workflow/complete_no_action) actually
// concludes the session. That final write arrives as a second OnTerminal
// call once CompleteUserDriving/ForceCompleteByRemediationID commits.
func (d *Dispatcher) writeTerminalStatus(ctx context.Context, key crclient.ObjectKey, status session.Status, result *katypes.InvestigationResult, fnErr error) {
	if status == session.StatusUserDriving {
		d.updateStatus(ctx, key, func(as *agentsessionv1.AgentSession) {
			if isTerminalPhase(as.Status.Phase) {
				return
			}
			as.Status.Interactive = true
		})
		return
	}

	d.updateStatus(ctx, key, func(as *agentsessionv1.AgentSession) {
		if isTerminalPhase(as.Status.Phase) {
			return
		}
		now := metav1.Now()
		as.Status.CompletedAt = &now
		switch status {
		case session.StatusCancelled:
			as.Status.Phase = agentsessionv1.AgentSessionPhaseCancelled
		case session.StatusFailed:
			as.Status.Phase = agentsessionv1.AgentSessionPhaseFailed
			as.Status.Error = curatedFailureMessage(fnErr)
		default: // session.StatusCompleted
			as.Status.Phase = agentsessionv1.AgentSessionPhaseCompleted
			as.Status.Result = MapInvestigationResultToAgentSessionResult(d.logger, result, as.Spec.IncidentID)
		}
	})
}

// curatedFailureMessage returns a user-facing message for Status.Error
// (SI-11: never a raw internal error string -- same curation boundary as
// the rest of AgentSessionResult).
func curatedFailureMessage(err error) string {
	if err == nil {
		return "investigation failed"
	}
	return "investigation failed: " + err.Error()
}
