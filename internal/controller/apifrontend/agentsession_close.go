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
	"strings"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

// terminalCloseFinalizer is agentsessionv1.TerminalCloseFinalizer, aliased
// locally for readability. See that constant's doc comment for the full CI
// RCA (PR #2222, run 32513171970, reproduced 3/13 locally on an amd64 host
// loaded with the exact same CI-built images under --race --procs=4): the
// previous design used a raw handler.Funcs.DeleteFunc to capture
// Spec.RemediationRequestRef off the informer's last-known object before
// Reconcile's own Get() would 404 after deletion -- best-effort, at-most-once
// delivery that can be dropped entirely under CPU contention, with no retry
// short of the informer's next full relist (a client-go/apiserver
// watch-bounce, typically ~5-10 minutes, far outside any reasonable
// detection budget). The finalizer defers the actual delete until this
// reconciler removes it, so deletion is handled by the SAME workqueue-
// backed, controller-runtime-retried Reconcile() path already proven
// reliable for the Completed/Failed case, eliminating the race by
// construction rather than widening the test's Eventually window.
//
// This reconciler adds the finalizer defensively on its own first
// Create/Update reconcile too (below), but AA's AgentSessionCreator.GetOrCreate
// is the primary, race-free source: it sets it synchronously in the same
// Create call that brings the object into existence, closing a narrower
// bootstrap race this reconciler's own reactive add cannot (a delete
// landing before this reconciler's first reconcile of a brand new object).
const terminalCloseFinalizer = agentsessionv1.TerminalCloseFinalizer

// alreadyTerminalErrSubstring matches session.ValidateTransition's error text
// for an IS that is already in a terminal phase. client-go informers may
// redeliver Create/Update/Delete events for the same object (e.g. after a
// watch reconnect triggers a relist) -- controller-runtime reconcilers are
// required to tolerate this. A redundant FinalizeSessionByRR call landing on
// an IS that a prior (redundant) delivery already closed is exactly this
// case: idempotent no-op, not a real failure. Matched by substring rather
// than a sentinel error because ValidateTransition is shared, existing
// session-service code (pkg/apifrontend/session/statemachine.go) with other
// callers (MCP complete/cancel tools); exporting a new error type there is
// out of scope for #2214, which only relocates the closure caller.
const alreadyTerminalErrSubstring = "no transitions from terminal phase"

func isAlreadyTerminalErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), alreadyTerminalErrSubstring)
}

// agentSessionPhaseToIS maps a terminal AgentSessionPhase (KA-driven) to the
// InvestigationSession terminal phase AF closes the correlated IS to.
// Deliberately excludes AgentSessionPhaseCancelled: that value is reached via
// a KA-driven Status.Phase Update (interactive session ended without
// takeover) and is not, today, correlated with IS closure -- a pre-existing
// gap this change does not widen or narrow. AgentSession *deletion* (AA's
// explicit cascade-cancel, #2214) is the only source of a Cancelled IS
// closure from this reconciler; see reconcileDelete.
var agentSessionPhaseToIS = map[agentsessionv1.AgentSessionPhase]isv1alpha1.SessionPhase{
	agentsessionv1.AgentSessionPhaseCompleted: isv1alpha1.SessionPhaseCompleted,
	agentsessionv1.AgentSessionPhaseFailed:    isv1alpha1.SessionPhaseFailed,
}

// AgentSessionTerminalCloseReconciler watches AgentSession and closes the
// correlated InvestigationSession (IS) to a terminal phase whenever the
// AgentSession reaches a terminal state -- either a KA-written
// Completed/Failed Status.Phase, or deletion (AA's explicit cascade-cancel
// on external ParentCancelled, #2214 / DD-AA-KA-001 Amendment).
//
// This is AF's replacement for AA's retired K8sISPhaseUpdater: IS
// terminal-phase closure is now owned exclusively by AF, matching AF's
// existing ownership of IS creation (MaterializeCRD) and TTL-driven cleanup
// (SessionCleanupReconciler). AA has zero read or write interaction with IS
// after this change.
//
// BR-INTERACTIVE-010 SC-9.
type AgentSessionTerminalCloseReconciler struct {
	client         client.Client
	sessionService *session.CRDSessionService
	logger         logr.Logger
}

// NewAgentSessionTerminalCloseReconciler creates a reconciler that closes the
// InvestigationSession correlated (by RemediationRequestRef.Name) to each
// AgentSession reaching a terminal state.
func NewAgentSessionTerminalCloseReconciler(c client.Client, svc *session.CRDSessionService, logger logr.Logger) *AgentSessionTerminalCloseReconciler {
	if logger.GetSink() == nil {
		logger = logr.Discard()
	}
	return &AgentSessionTerminalCloseReconciler{client: c, sessionService: svc, logger: logger}
}

// Reconcile handles AgentSession Create/Update/Delete via the finalizer
// pattern (see terminalCloseFinalizer): a DeletionTimestamp-set object is
// routed to reconcileDelete; otherwise the finalizer is ensured present and,
// if Status.Phase is a mapped terminal phase (Completed/Failed), the
// correlated IS is closed. No-op for Pending/Investigating/Cancelled and for
// a not-found AgentSession (fully deleted -- finalizer already removed).
func (r *AgentSessionTerminalCloseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var as agentsessionv1.AgentSession
	if err := r.client.Get(ctx, req.NamespacedName, &as); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !as.GetDeletionTimestamp().IsZero() {
		return r.reconcileDelete(ctx, &as)
	}

	if !controllerutil.ContainsFinalizer(&as, terminalCloseFinalizer) {
		controllerutil.AddFinalizer(&as, terminalCloseFinalizer)
		if err := r.client.Update(ctx, &as); err != nil {
			return ctrl.Result{}, fmt.Errorf("add %s finalizer: %w", terminalCloseFinalizer, err)
		}
	}

	isPhase, ok := agentSessionPhaseToIS[as.Status.Phase]
	if !ok {
		return ctrl.Result{}, nil
	}

	if err := r.closeIS(ctx, as.Namespace, as.Name, as.Spec.RemediationRequestRef.Name, isPhase, "Update"); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// reconcileDelete closes the correlated IS to Cancelled when an AgentSession
// carrying terminalCloseFinalizer is being deleted (regardless of who
// deleted it -- AA's explicit cascade-cancel on ParentCancelled being the
// one production caller today), then removes the finalizer to let the
// actual delete proceed.
//
// An AgentSession without the finalizer (e.g. one created by a
// pre-upgrade replica before this reconciler first observed it) falls
// through untouched -- a narrow, self-healing transitional gap bounded to
// objects already in flight at upgrade time, accepted rather than widened
// scope further; the existing IS TTL reaper remains the backstop for that
// window, same as it always has been for any missed closure.
func (r *AgentSessionTerminalCloseReconciler) reconcileDelete(ctx context.Context, as *agentsessionv1.AgentSession) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(as, terminalCloseFinalizer) {
		return ctrl.Result{}, nil
	}

	if rrName := as.Spec.RemediationRequestRef.Name; rrName != "" {
		if err := r.closeIS(ctx, as.Namespace, as.Name, rrName, isv1alpha1.SessionPhaseCancelled, "Delete"); err != nil {
			return ctrl.Result{}, err
		}
	}

	controllerutil.RemoveFinalizer(as, terminalCloseFinalizer)
	if err := r.client.Update(ctx, as); err != nil {
		return ctrl.Result{}, fmt.Errorf("remove %s finalizer: %w", terminalCloseFinalizer, err)
	}
	return ctrl.Result{}, nil
}

// closeIS calls FinalizeSessionByRR and logs the outcome consistently for
// both the Update-driven and Delete-driven (reconcileDelete) paths, treating
// a redundant already-terminal delivery as a benign no-op rather
// than an error (client-go informers may redeliver events for the same
// object).
func (r *AgentSessionTerminalCloseReconciler) closeIS(ctx context.Context, namespace, asName, rrName string, isPhase isv1alpha1.SessionPhase, trigger string) error {
	if err := r.sessionService.FinalizeSessionByRR(ctx, namespace, rrName, isPhase); err != nil {
		if isAlreadyTerminalErr(err) {
			r.logger.Info("InvestigationSession already closed for this AgentSession (redundant event delivery, no-op)",
				"agentSession", asName, "rrName", rrName, "isPhase", isPhase, "trigger", trigger)
			return nil
		}
		r.logger.Error(err, "failed to close InvestigationSession for terminal AgentSession",
			"agentSession", asName, "rrName", rrName, "isPhase", isPhase, "trigger", trigger)
		return err
	}
	r.logger.Info("closed InvestigationSession for terminal AgentSession",
		"agentSession", asName, "rrName", rrName, "isPhase", isPhase, "trigger", trigger)
	return nil
}

// SetupWithManager registers the reconciler with mgr. Uses the standard For()
// watch: terminalCloseFinalizer defers actual AgentSession removal until
// Reconcile has observed the DeletionTimestamp and closed the correlated IS,
// so the delete path is handled by the same reliable, controller-runtime-
// retried Reconcile() dispatch as Create/Update -- no custom event capture
// needed (see terminalCloseFinalizer's doc comment for why the prior
// handler.Funcs.DeleteFunc-capture design was replaced).
//
// No per-controller CacheSyncTimeout override here: cmd/apifrontend's
// newSessionControllerManager sets a manager-wide default (config.Controller)
// covering this controller's AgentSession informer alongside its sibling
// session-cleanup/lease-sync controllers' InvestigationSession informer --
// see that function's doc comment for the CI RCA this addresses.
func (r *AgentSessionTerminalCloseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("agentsession-terminal-close").
		For(&agentsessionv1.AgentSession{}).
		Complete(r)
}
