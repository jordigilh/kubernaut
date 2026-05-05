package session

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	v1alpha1 "github.com/jordigilh/kubernaut-apifrontend/api/apifrontend/v1alpha1"
	"github.com/jordigilh/kubernaut-apifrontend/internal/audit"
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
func (s *CRDSessionService) UpdatePhase(ctx context.Context, sessionID string, to v1alpha1.SessionPhase, message string) error {
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
	var crd v1alpha1.InvestigationSession
	if err := s.client.Get(ctx, nn, &crd); err != nil {
		return fmt.Errorf("get session for phase update: %w", err)
	}

	from := crd.Status.Phase
	if err := ValidateTransition(from, to); err != nil {
		return fmt.Errorf("phase transition: %w", err)
	}

	now := metav1.Now()

	crd.Status.Phase = to
	crd.Status.Message = message
	crd.Labels[LabelPhase] = string(to)

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

	if err := s.client.Status().Update(ctx, &crd); err != nil {
		return fmt.Errorf("update session status: %w", err)
	}

	// Re-read to get updated resourceVersion, then update labels
	if err := s.client.Get(ctx, nn, &crd); err != nil {
		return fmt.Errorf("re-read session for label update: %w", err)
	}
	crd.Labels[LabelPhase] = string(to)
	if err := s.client.Update(ctx, &crd); err != nil {
		return fmt.Errorf("update session labels: %w", err)
	}

	s.logger.InfoContext(ctx, "session phase updated",
		"session_id", sessionID,
		"from", from,
		"to", to,
	)
	s.emitAudit(ctx, audit.EventSessionPhaseChanged, "", map[string]string{
		"session_id": sessionID,
		"from":       string(from),
		"to":         string(to),
	})

	s.decSessionGauge(string(from))
	s.incSessionGauge(string(to))

	if IsTerminal(to) {
		s.mu.Lock()
		delete(s.crdIndex, sessionID)
		s.mu.Unlock()
	}
	return nil
}

