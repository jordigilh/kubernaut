package session

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sretry "k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
)

// validTransitions defines the allowed phase transitions for an InvestigationSession.
// Terminal phases (Completed, Cancelled, Failed) have no outgoing edges.
var validTransitions = map[v1alpha1.SessionPhase][]v1alpha1.SessionPhase{
	v1alpha1.SessionPhaseActive: {
		v1alpha1.SessionPhaseCompleted,
		v1alpha1.SessionPhaseCancelled,
		v1alpha1.SessionPhaseFailed,
		v1alpha1.SessionPhaseDisconnected,
	},
	v1alpha1.SessionPhaseDisconnected: {
		v1alpha1.SessionPhaseActive,
		v1alpha1.SessionPhaseCancelled,
		v1alpha1.SessionPhaseFailed,
	},
}

// terminalPhases are phases with no outgoing transitions.
var terminalPhases = map[v1alpha1.SessionPhase]bool{
	v1alpha1.SessionPhaseCompleted: true,
	v1alpha1.SessionPhaseCancelled: true,
	v1alpha1.SessionPhaseFailed:    true,
}

// ValidateTransition checks whether the transition from -> to is allowed
// by the InvestigationSession state machine. Returns an error for invalid
// transitions including self-transitions and transitions from terminal states.
func ValidateTransition(from, to v1alpha1.SessionPhase) error {
	allowed, ok := validTransitions[from]
	if !ok {
		return fmt.Errorf("no transitions from terminal phase %q", from)
	}
	for _, a := range allowed {
		if a == to {
			return nil
		}
	}
	return fmt.Errorf("invalid transition %q -> %q", from, to)
}

// IsTerminal returns true if the phase is a terminal (no further transitions).
func IsTerminal(phase v1alpha1.SessionPhase) bool {
	return terminalPhases[phase]
}

// maxPhaseMessageLen caps the length of status.message to prevent PII leakage
// into etcd. Callers MUST pass operator-defined static strings only; user input
// must never be passed as the message parameter (see ADR-017).
const maxPhaseMessageLen = 256

// UpdatePhase transitions the InvestigationSession CRD to a new phase,
// validating the transition and setting appropriate timestamps.
//
// The message parameter MUST be an operator-defined static string describing
// the reason for the transition (e.g. "investigation complete", "user cancelled").
// It MUST NOT contain user-originated content or PII. The message is truncated
// to maxPhaseMessageLen (256 chars) as a defense-in-depth measure per ADR-017.
//
// The userID parameter is included in the audit event for traceability (AU-12).
// Pass an empty string for system-initiated transitions (e.g. TTL expiry).
func (s *CRDSessionService) UpdatePhase(ctx context.Context, sessionID string, to v1alpha1.SessionPhase, message, userID string) error {
	if len(message) > maxPhaseMessageLen {
		message = message[:maxPhaseMessageLen]
	}
	s.mu.RLock()
	crdName, ok := s.crdIndex[sessionID]
	s.mu.RUnlock()
	if !ok {
		crdName = sessionID
	}

	nn := types.NamespacedName{Name: crdName, Namespace: s.namespace}
	reader := s.getReader()

	var from v1alpha1.SessionPhase
	err := k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
		var crd v1alpha1.InvestigationSession
		if err := reader.Get(ctx, nn, &crd); err != nil {
			return fmt.Errorf("get session for phase update: %w", err)
		}

		from = crd.Status.Phase
		if err := ValidateTransition(from, to); err != nil {
			return fmt.Errorf("phase transition: %w", err)
		}

		applyPhaseStatus(&crd, from, to, message)
		if err := s.client.Status().Update(ctx, &crd); err != nil {
			return err
		}

		return s.updatePhaseLabel(ctx, reader, nn, to)
	})
	if err != nil {
		return err
	}

	s.logger.Info("session phase updated",
		"session_id", sessionID,
		"from", from,
		"to", to,
	)
	s.emitAudit(ctx, audit.EventSessionPhaseChanged, userID, map[string]string{
		"session_id": sessionID,
		"from_phase": string(from),
		"to_phase":   string(to),
	})

	if IsTerminal(to) {
		s.emitSessionCompletedAudit(ctx, nn, sessionID, userID, to)
	}

	s.decSessionGauge(string(from))
	s.incSessionGauge(string(to))

	if IsTerminal(to) {
		s.mu.Lock()
		delete(s.crdIndex, sessionID)
		s.mu.Unlock()
	}
	return nil
}

// applyPhaseStatus sets crd.Status.Phase/Message and the phase-specific
// timestamp/connection-state fields for the from->to transition.
func applyPhaseStatus(crd *v1alpha1.InvestigationSession, from, to v1alpha1.SessionPhase, message string) {
	now := metav1.Now()
	crd.Status.Phase = to
	crd.Status.Message = message

	switch {
	case IsTerminal(to):
		crd.Status.CompletedAt = &now
	case to == v1alpha1.SessionPhaseDisconnected:
		crd.Status.DisconnectedAt = &now
		crd.Status.ConnectionState = v1alpha1.ConnectionStateDisconnected
	case from == v1alpha1.SessionPhaseDisconnected && to == v1alpha1.SessionPhaseActive:
		crd.Status.ReconnectedAt = &now
		crd.Status.ConnectionState = v1alpha1.ConnectionStateConnected
	}
}

// updatePhaseLabel re-reads the CRD and updates its phase label. A separate
// Update is required because the status subresource split means the earlier
// Status().Update call does not persist label changes.
func (s *CRDSessionService) updatePhaseLabel(ctx context.Context, reader client.Reader, nn types.NamespacedName, to v1alpha1.SessionPhase) error {
	var crd v1alpha1.InvestigationSession
	if err := reader.Get(ctx, nn, &crd); err != nil {
		return fmt.Errorf("re-read session for label update: %w", err)
	}
	if crd.Labels == nil {
		crd.Labels = make(map[string]string)
	}
	crd.Labels[LabelPhase] = string(to)
	return s.client.Update(ctx, &crd)
}

// emitSessionCompletedAudit emits the EventSessionCompleted audit event for
// a terminal phase transition, including total investigation duration when
// the CRD's creation timestamp is available.
func (s *CRDSessionService) emitSessionCompletedAudit(ctx context.Context, nn types.NamespacedName, sessionID, userID string, to v1alpha1.SessionPhase) {
	detail := map[string]string{
		"session_id":     sessionID,
		"terminal_phase": string(to),
	}
	var crdForDuration v1alpha1.InvestigationSession
	if err := s.getReader().Get(ctx, nn, &crdForDuration); err == nil {
		created := crdForDuration.CreationTimestamp.Time
		if !created.IsZero() {
			detail["total_duration_ms"] = fmt.Sprintf("%d", time.Since(created).Milliseconds())
		}
	}
	s.emitAudit(ctx, audit.EventSessionCompleted, userID, detail)
}
