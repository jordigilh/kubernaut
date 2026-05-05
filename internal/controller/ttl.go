// Package controller implements controller-runtime reconcilers for the
// kubernaut API Frontend.
package controller

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/jordigilh/kubernaut-apifrontend/api/apifrontend/v1alpha1"
	"github.com/jordigilh/kubernaut-apifrontend/internal/audit"
	"github.com/jordigilh/kubernaut-apifrontend/internal/session"
)

// MinRetentionTTL is the minimum retention period for terminal sessions,
// enforcing NIST AU-11 (30-day minimum audit record retention).
const MinRetentionTTL = 30 * 24 * time.Hour

// SessionCleanupReconciler watches InvestigationSession CRDs and enforces
// two TTL policies:
//   - DisconnectTTL: time a session may stay in Disconnected before being
//     auto-cancelled.
//   - RetentionTTL: time a terminal session is kept before deletion.
type SessionCleanupReconciler struct {
	client         client.Client
	disconnectTTL  time.Duration
	retentionTTL   time.Duration
	logger         *slog.Logger
	auditor        audit.Emitter
	ttlActions     *prometheus.CounterVec
	sessionService *session.CRDSessionService
}

// NewSessionCleanupReconciler creates a reconciler with the specified TTLs.
// If retentionTTL is below MinRetentionTTL, it is clamped and a warning is logged.
// The auditor may be nil to disable audit emission (e.g. in tests).
// The sessionService may be nil; when provided, PruneTerminalEntries is called
// after each successful terminal deletion to bound crdIndex growth.
func NewSessionCleanupReconciler(c client.Client, disconnectTTL, retentionTTL time.Duration, auditor audit.Emitter, ttlActions *prometheus.CounterVec, sessionService *session.CRDSessionService) *SessionCleanupReconciler {
	logger := slog.Default().With("component", "session-cleanup")
	if retentionTTL < MinRetentionTTL {
		logger.Warn("retentionTTL below NIST AU-11 minimum, clamping",
			"configured", retentionTTL.String(),
			"minimum", MinRetentionTTL.String(),
		)
		retentionTTL = MinRetentionTTL
	}
	return &SessionCleanupReconciler{
		client:         c,
		disconnectTTL:  disconnectTTL,
		retentionTTL:   retentionTTL,
		logger:         logger,
		auditor:        auditor,
		ttlActions:     ttlActions,
		sessionService: sessionService,
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
		result, err := r.handleDisconnected(ctx, &sess)
		if err != nil {
			r.logger.ErrorContext(ctx, "disconnect TTL reconcile failed",
				"session", sess.Name,
				"namespace", sess.Namespace,
				"phase", sess.Status.Phase,
				"error", err,
			)
		}
		return result, err

	case v1alpha1.SessionPhaseCompleted, v1alpha1.SessionPhaseCancelled, v1alpha1.SessionPhaseFailed:
		result, err := r.handleTerminal(ctx, &sess)
		if err != nil {
			r.logger.ErrorContext(ctx, "retention TTL reconcile failed",
				"session", sess.Name,
				"namespace", sess.Namespace,
				"phase", sess.Status.Phase,
				"error", err,
			)
		}
		return result, err

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

	if err := session.ValidateTransition(sess.Status.Phase, v1alpha1.SessionPhaseCancelled); err != nil {
		return ctrl.Result{}, fmt.Errorf("validate disconnect TTL transition: %w", err)
	}

	now := metav1.Now()
	sess.Status.Phase = v1alpha1.SessionPhaseCancelled
	sess.Status.CompletedAt = &now
	sess.Status.Message = "auto-cancelled: disconnect TTL expired"
	if sess.Labels == nil {
		sess.Labels = make(map[string]string)
	}
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
	r.emitAudit(ctx, audit.EventSessionAutoCancelled, map[string]string{
		"session": sess.Name,
		"phase":   string(v1alpha1.SessionPhaseCancelled),
		"elapsed": elapsed.String(),
	})
	r.incTTLAction("cancel")
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
	r.emitAudit(ctx, audit.EventSessionRetentionDeleted, map[string]string{
		"session": sess.Name,
		"phase":   string(sess.Status.Phase),
		"elapsed": elapsed.String(),
	})
	r.incTTLAction("delete")

	if r.sessionService != nil {
		r.sessionService.PruneTerminalEntries(ctx)
	}

	return ctrl.Result{}, nil
}

func (r *SessionCleanupReconciler) incTTLAction(action string) {
	if r.ttlActions != nil {
		r.ttlActions.WithLabelValues(action).Inc()
	}
}

func (r *SessionCleanupReconciler) emitAudit(ctx context.Context, eventType audit.EventType, detail map[string]string) {
	if r.auditor == nil {
		return
	}
	r.auditor.Emit(ctx, &audit.Event{
		Type:   eventType,
		Detail: detail,
	})
}
