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
	"strings"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"k8s.io/client-go/util/workqueue"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

// cacheSyncTimeout overrides controller-runtime's 2-minute default for this
// controller's own EventSource cache sync (CI RCA, PR #2222, run
// 32491684227, E2E apifrontend must-gather): under a fully-loaded Kind E2E
// apiserver (dozens of parallel Ginkgo procs hammering it with CRD churn),
// AgentSession's informer -- newly introduced by this controller, so it has
// no warm cache to reuse -- did not complete its initial List/Watch within
// the 2-minute default, and controller-runtime treats that as fatal to the
// whole manager (session_infra.go's shared Healthy flag flips false,
// /readyz returns 503, kube-proxy drops the pod from Service endpoints --
// breaking every other AF request in flight, unrelated to session/IS
// closure). A generous timeout gives the informer room to finish under load
// instead of failing fast and taking down pod readiness for an unrelated
// reason.
const cacheSyncTimeout = 5 * time.Minute

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
// closure from this reconciler; see handleDelete.
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

// Reconcile handles Create/Update events for AgentSession: if Status.Phase is
// a mapped terminal phase (Completed/Failed), it closes the correlated IS.
// No-op for Pending/Investigating/Cancelled and for a not-found AgentSession
// (already deleted -- handled by the Delete-event path instead).
func (r *AgentSessionTerminalCloseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var as agentsessionv1.AgentSession
	if err := r.client.Get(ctx, req.NamespacedName, &as); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
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

// handleDelete closes the correlated IS to Cancelled when an AgentSession is
// deleted, regardless of who deleted it (AA's explicit cascade-cancel on
// ParentCancelled being the one production caller today). Best-effort and
// detached from any live request context, mirroring KA's own
// Dispatcher.cancelOnDelete framing for the same reason: by the time a
// Delete event is observed, the object is already gone and there is no
// requester waiting on the result.
func (r *AgentSessionTerminalCloseReconciler) handleDelete(ctx context.Context, obj client.Object) {
	as, ok := obj.(*agentsessionv1.AgentSession)
	if !ok {
		r.logger.Info("delete event for unexpected object type, skipping", "type", obj.GetObjectKind().GroupVersionKind().String())
		return
	}

	rrName := as.Spec.RemediationRequestRef.Name
	if rrName == "" {
		return
	}

	// Best-effort: the delete path has no requester waiting on the result,
	// so a failure is logged (inside closeIS) and swallowed here.
	_ = r.closeIS(ctx, as.Namespace, as.Name, rrName, isv1alpha1.SessionPhaseCancelled, "Delete")
}

// closeIS calls FinalizeSessionByRR and logs the outcome consistently for
// both the Update-driven (Reconcile) and Delete-driven (handleDelete) paths,
// treating a redundant already-terminal delivery as a benign no-op rather
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

// SetupWithManager registers the reconciler with mgr. Uses Watches (not For)
// with a custom handler.Funcs: the DeleteFunc reads Spec.RemediationRequestRef
// directly off the informer's last-known object (event.DeleteEvent.Object)
// and closes IS immediately, since Reconcile's Get() would 404 after
// deletion and lose the RR name needed to find the correlated IS -- the same
// structural reason KA's own raw-watch Dispatcher.cancelOnDelete reads off
// the watch event rather than re-fetching.
func (r *AgentSessionTerminalCloseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	enqueue := func(obj client.Object, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
		}})
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named("agentsession-terminal-close").
		WithOptions(controller.Options{CacheSyncTimeout: cacheSyncTimeout}).
		Watches(&agentsessionv1.AgentSession{}, handler.Funcs{
			CreateFunc: func(_ context.Context, e event.CreateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				enqueue(e.Object, q)
			},
			UpdateFunc: func(_ context.Context, e event.UpdateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				enqueue(e.ObjectNew, q)
			},
			DeleteFunc: func(ctx context.Context, e event.DeleteEvent, _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				r.handleDelete(ctx, e.Object)
			},
		}).
		Complete(r)
}
