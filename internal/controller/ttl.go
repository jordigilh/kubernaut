// Package controller implements controller-runtime reconcilers for the
// kubernaut API Frontend.
package controller

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/jordigilh/kubernaut-apifrontend/api/apifrontend/v1alpha1"
	"github.com/jordigilh/kubernaut-apifrontend/internal/session"
)

// SessionCleanupReconciler watches InvestigationSession CRDs and enforces
// two TTL policies:
//   - DisconnectTTL: time a session may stay in Disconnected before being
//     auto-cancelled.
//   - RetentionTTL: time a terminal session is kept before deletion.
type SessionCleanupReconciler struct {
	client        client.Client
	disconnectTTL time.Duration
	retentionTTL  time.Duration
	logger        *slog.Logger
}

// NewSessionCleanupReconciler creates a reconciler with the specified TTLs.
func NewSessionCleanupReconciler(c client.Client, disconnectTTL, retentionTTL time.Duration) *SessionCleanupReconciler {
	return &SessionCleanupReconciler{
		client:        c,
		disconnectTTL: disconnectTTL,
		retentionTTL:  retentionTTL,
		logger:        slog.Default().With("component", "session-cleanup"),
	}
}

// Reconcile implements controller-runtime's Reconciler interface.
func (r *SessionCleanupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var sess v1alpha1.InvestigationSession
	if err := r.client.Get(ctx, req.NamespacedName, &sess); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get session: %w", err)
	}

	switch sess.Status.Phase {
	case v1alpha1.SessionPhaseActive:
		return ctrl.Result{}, nil

	case v1alpha1.SessionPhaseDisconnected:
		return r.handleDisconnected(ctx, &sess)

	case v1alpha1.SessionPhaseCompleted, v1alpha1.SessionPhaseCancelled, v1alpha1.SessionPhaseFailed:
		return r.handleTerminal(ctx, &sess)

	default:
		return ctrl.Result{}, nil
	}
}

func (r *SessionCleanupReconciler) handleDisconnected(ctx context.Context, sess *v1alpha1.InvestigationSession) (ctrl.Result, error) {
	if sess.Status.DisconnectedAt == nil {
		return ctrl.Result{}, nil
	}

	elapsed := time.Since(sess.Status.DisconnectedAt.Time)
	if elapsed < r.disconnectTTL {
		remaining := r.disconnectTTL - elapsed
		return ctrl.Result{RequeueAfter: remaining}, nil
	}

	now := metav1.Now()
	sess.Status.Phase = v1alpha1.SessionPhaseCancelled
	sess.Status.CompletedAt = &now
	sess.Status.Message = "auto-cancelled: disconnect TTL expired"
	sess.Labels[session.LabelPhase] = string(v1alpha1.SessionPhaseCancelled)

	if err := r.client.Status().Update(ctx, sess); err != nil {
		return ctrl.Result{}, fmt.Errorf("cancel disconnected session: %w", err)
	}
	if err := r.client.Update(ctx, sess); err != nil {
		return ctrl.Result{}, fmt.Errorf("update disconnected session labels: %w", err)
	}

	r.logger.InfoContext(ctx, "session auto-cancelled",
		"session", sess.Name,
		"elapsed", elapsed.String(),
	)
	return ctrl.Result{}, nil
}

func (r *SessionCleanupReconciler) handleTerminal(ctx context.Context, sess *v1alpha1.InvestigationSession) (ctrl.Result, error) {
	if sess.Status.CompletedAt == nil {
		return ctrl.Result{}, nil
	}

	elapsed := time.Since(sess.Status.CompletedAt.Time)
	if elapsed < r.retentionTTL {
		remaining := r.retentionTTL - elapsed
		return ctrl.Result{RequeueAfter: remaining}, nil
	}

	if err := r.client.Delete(ctx, sess); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("delete expired session: %w", err)
	}

	r.logger.InfoContext(ctx, "session deleted after retention TTL",
		"session", sess.Name,
		"phase", sess.Status.Phase,
		"elapsed", elapsed.String(),
	)
	return ctrl.Result{}, nil
}
