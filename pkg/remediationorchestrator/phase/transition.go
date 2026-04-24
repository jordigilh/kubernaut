/*
Copyright 2025 Jordi Gil.

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

package phase

import (
	"fmt"
	"time"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// TransitionType classifies the kind of outcome a phase handler returns.
//
// Reference: Issue #666 (RO Phase Handler Registry refactoring)
type TransitionType int

const (
	// TransitionNone indicates no phase change. When RequeueAfter > 0,
	// the reconciler should requeue; otherwise it's a no-op terminal return.
	TransitionNone TransitionType = iota

	// TransitionAdvance indicates normal forward progression to TargetPhase.
	TransitionAdvance

	// TransitionFailed indicates the RR should move to Failed with a FailurePhase.
	TransitionFailed

	// TransitionBlocked indicates the RR should be blocked per routing conditions.
	TransitionBlocked

	// TransitionVerifying indicates the RR should transition to the Verifying phase.
	// Maps to reconciler.transitionToVerifying(ctx, rr, outcome).
	TransitionVerifying

	// TransitionInheritedCompleted indicates the RR should inherit completion
	// from an original resource (WFE or RR).
	// Maps to reconciler.transitionToInheritedCompleted(ctx, rr, sourceRef, sourceKind).
	TransitionInheritedCompleted

	// TransitionInheritedFailed indicates the RR should inherit failure
	// from an original resource (WFE or RR).
	// Maps to reconciler.transitionToInheritedFailed(ctx, rr, failureErr, sourceRef, sourceKind).
	TransitionInheritedFailed
)

var transitionTypeNames = map[TransitionType]string{
	TransitionNone:               "None",
	TransitionAdvance:            "Advance",
	TransitionFailed:             "Failed",
	TransitionBlocked:            "Blocked",
	TransitionVerifying:          "Verifying",
	TransitionInheritedCompleted: "InheritedCompleted",
	TransitionInheritedFailed:    "InheritedFailed",
}

// String returns a human-readable representation of the TransitionType.
func (t TransitionType) String() string {
	if name, ok := transitionTypeNames[t]; ok {
		return name
	}
	return fmt.Sprintf("Unknown(%d)", int(t))
}

// BlockMeta carries routing-derived metadata for a blocking transition.
// It mirrors routing.BlockingCondition without importing the routing package,
// keeping the phase package free of cross-package coupling.
type BlockMeta struct {
	Reason                    string
	Message                   string
	RequeueAfter              time.Duration
	BlockedUntil              *time.Time
	BlockingWorkflowExecution string
	DuplicateOf               string
	FromPhase                 Phase
	WorkflowID                string
}

// TransitionIntent captures the full outcome intent of a phase handler.
// The reconciler interprets this to perform the actual status mutations
// and return the appropriate ctrl.Result.
//
// Mapping to ctrl.Result:
//   - TransitionNone + RequeueImmediately → ctrl.Result{Requeue: true}
//   - TransitionNone + RequeueAfter > 0   → ctrl.Result{RequeueAfter: d}
//   - TransitionNone + neither            → ctrl.Result{} (no-op)
//   - All other types                     → reconciler performs transition, then ctrl.Result
//
// Cancelled phase: Not handler-produced. Cancelled is set by external actors
// (e.g., user cancellation webhook) and does not flow through PhaseHandler.
//
// Reference: Issue #666 (RO Phase Handler Registry refactoring)
type TransitionIntent struct {
	Type               TransitionType
	TargetPhase        Phase                          // TransitionAdvance
	FailurePhase       remediationv1.FailurePhase     // TransitionFailed
	FailureErr         error                          // TransitionFailed, TransitionInheritedFailed
	Block              *BlockMeta                     // TransitionBlocked
	Outcome            string                         // TransitionVerifying
	SourceRef          string                         // TransitionInheritedCompleted, TransitionInheritedFailed
	SourceKind         string                         // TransitionInheritedCompleted, TransitionInheritedFailed
	RequeueAfter       time.Duration                  // TransitionNone
	RequeueImmediately bool                           // TransitionNone
	Reason             string                         // all types
}

// Advance creates a TransitionIntent for normal forward progression.
func Advance(target Phase, reason string) TransitionIntent {
	return TransitionIntent{
		Type:        TransitionAdvance,
		TargetPhase: target,
		Reason:      reason,
	}
}

// Fail creates a TransitionIntent for a failure transition.
func Fail(fp remediationv1.FailurePhase, err error, reason string) TransitionIntent {
	return TransitionIntent{
		Type:         TransitionFailed,
		FailurePhase: fp,
		FailureErr:   err,
		Reason:       reason,
	}
}

// Block creates a TransitionIntent for a routing block.
func Block(meta *BlockMeta) TransitionIntent {
	return TransitionIntent{
		Type:  TransitionBlocked,
		Block: meta,
	}
}

// Requeue creates a TransitionIntent that stays in the current phase
// and requeues after the specified duration.
func Requeue(after time.Duration, reason string) TransitionIntent {
	return TransitionIntent{
		Type:         TransitionNone,
		RequeueAfter: after,
		Reason:       reason,
	}
}

// NoOp creates a TransitionIntent indicating no action needed.
func NoOp(reason string) TransitionIntent {
	return TransitionIntent{
		Type:   TransitionNone,
		Reason: reason,
	}
}

// RequeueNow creates a TransitionIntent that stays in the current phase
// and requeues immediately (maps to ctrl.Result{Requeue: true}).
// Used by clearEventBasedBlock and handleUnmanagedResourceExpiry.
func RequeueNow(reason string) TransitionIntent {
	return TransitionIntent{
		Type:               TransitionNone,
		RequeueImmediately: true,
		Reason:             reason,
	}
}

// Verify creates a TransitionIntent for transitioning to the Verifying phase.
// outcome describes the execution result (e.g., "remediationSucceeded").
func Verify(outcome, reason string) TransitionIntent {
	return TransitionIntent{
		Type:    TransitionVerifying,
		Outcome: outcome,
		Reason:  reason,
	}
}

// InheritComplete creates a TransitionIntent for inheriting completion
// from an original resource. sourceRef is the original resource name,
// sourceKind is "WorkflowExecution" or "RemediationRequest".
func InheritComplete(sourceRef, sourceKind, reason string) TransitionIntent {
	return TransitionIntent{
		Type:       TransitionInheritedCompleted,
		SourceRef:  sourceRef,
		SourceKind: sourceKind,
		Reason:     reason,
	}
}

// InheritFail creates a TransitionIntent for inheriting failure
// from an original resource. sourceRef is the original resource name,
// sourceKind is "WorkflowExecution" or "RemediationRequest".
func InheritFail(err error, sourceRef, sourceKind, reason string) TransitionIntent {
	return TransitionIntent{
		Type:       TransitionInheritedFailed,
		FailureErr: err,
		SourceRef:  sourceRef,
		SourceKind: sourceKind,
		Reason:     reason,
	}
}

// Validate checks that a TransitionIntent is internally consistent.
func (t TransitionIntent) Validate() error {
	switch t.Type {
	case TransitionAdvance:
		if t.TargetPhase == "" {
			return fmt.Errorf("TransitionAdvance requires TargetPhase to be set")
		}
	case TransitionFailed:
		if t.FailurePhase == "" {
			return fmt.Errorf("TransitionFailed requires FailurePhase to be set")
		}
	case TransitionBlocked:
		if t.Block == nil {
			return fmt.Errorf("TransitionBlocked requires Block metadata to be set")
		}
	case TransitionVerifying:
		// Outcome is optional; no additional requirements
	case TransitionInheritedCompleted:
		if t.SourceRef == "" || t.SourceKind == "" {
			return fmt.Errorf("TransitionInheritedCompleted requires SourceRef and SourceKind to be set")
		}
	case TransitionInheritedFailed:
		if t.SourceRef == "" || t.SourceKind == "" {
			return fmt.Errorf("TransitionInheritedFailed requires SourceRef and SourceKind to be set")
		}
	case TransitionNone:
		// No additional requirements
	default:
		return fmt.Errorf("unknown TransitionType: %d", int(t.Type))
	}
	return nil
}

// IsNoOp returns true if this intent represents a terminal no-op
// (no phase change and no requeue of any kind).
func (t TransitionIntent) IsNoOp() bool {
	return t.Type == TransitionNone && t.RequeueAfter == 0 && !t.RequeueImmediately
}

// IsRequeue returns true if this intent represents a requeue without phase change.
// Covers both timed requeue (RequeueAfter > 0) and immediate requeue (RequeueImmediately).
func (t TransitionIntent) IsRequeue() bool {
	return t.Type == TransitionNone && (t.RequeueAfter > 0 || t.RequeueImmediately)
}
