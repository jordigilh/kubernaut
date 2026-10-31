// Package parentlifecycle contains typed, cache-bypassed lifecycle gates shared
// by child controllers. A gate read is an authorization check, not an
// observation: transient read errors never authorize work and parent NotFound
// is represented as cancellation.
package parentlifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// ChildCollisionError identifies a deterministic child that already exists
// outside the current creator's successful Create operation. It is shared by
// RO and AIAnalysis child creators so controllers can fail closed uniformly.
type ChildCollisionError struct {
	ChildKind string
	Namespace string
	Name      string
	Cause     error
}

func (e *ChildCollisionError) Error() string {
	if e.Cause == nil {
		return fmt.Sprintf("%s %s/%s already exists and cannot be adopted", e.ChildKind, e.Namespace, e.Name)
	}
	return fmt.Sprintf("%s %s/%s collision: %v", e.ChildKind, e.Namespace, e.Name, e.Cause)
}

// Unwrap preserves Kubernetes API error classification.
func (e *ChildCollisionError) Unwrap() error { return e.Cause }

// IsChildCollision reports whether err contains a typed provenance collision.
func IsChildCollision(err error) bool {
	var collision *ChildCollisionError
	return errors.As(err, &collision)
}

// NewChildCollisionError constructs a typed collision for a child creator.
func NewChildCollisionError(kind, namespace, name string, cause error) error {
	return &ChildCollisionError{ChildKind: kind, Namespace: namespace, Name: name, Cause: cause}
}

// GateDecision is the result of a fresh parent lifecycle read.
type GateDecision string

const (
	// GateAllowed means the persisted parent phase authorizes consequential work.
	GateAllowed GateDecision = "Allowed"
	// GateWaiting means the child is in the legitimate create-before-phase window
	// or the parent is still progressing. The caller should requeue without work.
	GateWaiting GateDecision = "Waiting"
	// GateCancelled means the parent is missing or has failed/cancelled. The
	// child controller owns its cancellation transition.
	GateCancelled GateDecision = "Cancelled"
)

// GateResult carries the decision and the phase observed by the direct read.
// Cause is populated for a NotFound cancellation so callers can distinguish it
// from a persisted parent failure without treating it as a retryable read error.
type GateResult struct {
	Decision    GateDecision
	ParentPhase string
	Reason      string
	Cause       error
}

// DefaultHandoffTimeout bounds the legitimate create-before-parent-phase
// window. A successful child create followed by a lost parent status write is
// intentionally fail-closed after this interval rather than waiting forever.
const DefaultHandoffTimeout = 2 * time.Minute

// HandoffExpired reports whether a child has waited longer than the bounded
// create-before-parent-phase window. Zero creation timestamps are treated as
// unknown so legacy fixtures do not receive a false timeout.
func HandoffExpired(createdAt, now time.Time, timeout time.Duration) bool {
	return timeout > 0 && !createdAt.IsZero() && !now.Before(createdAt.Add(timeout))
}

// RemediationRequestGate performs one fresh read of a RemediationRequest and
// evaluates it against the allowed persisted phases. The reader must be an
// API-server reader, not an informer cache, at consequential-work boundaries.
func RemediationRequestGate(
	ctx context.Context,
	reader client.Reader,
	namespace, name string,
	allowed ...remediationv1.RemediationPhase,
) (GateResult, error) {
	parent := &remediationv1.RemediationRequest{}
	if err := reader.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, parent); err != nil {
		if apierrors.IsNotFound(err) {
			return GateResult{
				Decision: GateCancelled,
				Reason:   "parent-not-found",
				Cause:    err,
			}, nil
		}
		return GateResult{}, fmt.Errorf("read RemediationRequest %s/%s for parent gate: %w", namespace, name, err)
	}

	phase := parent.Status.OverallPhase
	if phase == remediationv1.PhaseFailed || phase == remediationv1.PhaseCancelled {
		return GateResult{Decision: GateCancelled, ParentPhase: string(phase), Reason: "parent-terminal-failure"}, nil
	}
	if containsRemediationPhase(allowed, phase) {
		return GateResult{Decision: GateAllowed, ParentPhase: string(phase), Reason: "authorized-parent-phase"}, nil
	}
	return GateResult{Decision: GateWaiting, ParentPhase: string(phase), Reason: "parent-phase-not-yet-authorized"}, nil
}

// AIAnalysisGate performs one fresh read of an AIAnalysis and evaluates its
// persisted phase. A failed/cancelled analysis is a cancellation signal for
// downstream AgentSession work.
func AIAnalysisGate(
	ctx context.Context,
	reader client.Reader,
	namespace, name string,
	allowed ...string,
) (GateResult, error) {
	analysis := &aianalysisv1.AIAnalysis{}
	if err := reader.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, analysis); err != nil {
		if apierrors.IsNotFound(err) {
			return GateResult{Decision: GateCancelled, Reason: "parent-not-found", Cause: err}, nil
		}
		return GateResult{}, fmt.Errorf("read AIAnalysis %s/%s for parent gate: %w", namespace, name, err)
	}

	phase := analysis.Status.Phase
	if phase == aianalysisv1.PhaseFailed || phase == aianalysisv1.PhaseCancelled {
		return GateResult{Decision: GateCancelled, ParentPhase: phase, Reason: "parent-terminal-failure"}, nil
	}
	if containsString(allowed, phase) {
		return GateResult{Decision: GateAllowed, ParentPhase: phase, Reason: "authorized-parent-phase"}, nil
	}
	return GateResult{Decision: GateWaiting, ParentPhase: phase, Reason: "parent-phase-not-yet-authorized"}, nil
}

func containsRemediationPhase(phases []remediationv1.RemediationPhase, want remediationv1.RemediationPhase) bool {
	for _, phase := range phases {
		if phase == want {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
