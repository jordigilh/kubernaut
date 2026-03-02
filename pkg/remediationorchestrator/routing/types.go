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

// Package routing provides routing decision logic for RemediationOrchestrator.
//
// The routing engine determines if a RemediationRequest should proceed to
// workflow execution or be blocked due to:
// - Consecutive failures (BR-ORCH-042)
// - Duplicate signals (DD-RO-002-ADDENDUM)
// - Resource locks (DD-RO-002)
// - Cooldown periods (DD-WE-001)
// - Exponential backoff (DD-WE-004)
//
// All routing decisions use Kubernetes field indexes for O(1) query performance.
//
// Reference: DD-RO-002 (Centralized Routing Responsibility)
// Reference: DD-RO-002-ADDENDUM (Blocked Phase Semantics)
package routing

import (
	"context"
	"fmt"
	"time"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// BlockingCondition represents a blocking scenario that prevents
// a RemediationRequest from proceeding to workflow execution.
//
// When a blocking condition is found, the RR enters PhaseBlocked with
// the appropriate BlockReason and will be requeued after RequeueAfter duration.
//
// Reference: DD-RO-002-ADDENDUM (Blocked Phase Semantics)
type BlockingCondition struct {
	// Blocked indicates if the RR is blocked (true) or can proceed (false)
	Blocked bool

	// Reason is the BlockReason enum value explaining why blocked.
	// Valid values: "ConsecutiveFailures", "DuplicateInProgress",
	//               "ResourceBusy", "RecentlyRemediated", "ExponentialBackoff"
	Reason string

	// Message is a human-readable explanation of the blocking condition
	Message string

	// RequeueAfter specifies when to check the blocking condition again
	RequeueAfter time.Duration

	// BlockedUntil is the absolute time when the blocking condition expires.
	// Used for time-based blocks (ConsecutiveFailures, RecentlyRemediated, ExponentialBackoff).
	// Optional: only set when blocking has a time-based expiration.
	BlockedUntil *time.Time

	// BlockingWorkflowExecution references the WorkflowExecution CRD that is
	// currently blocking this RemediationRequest.
	// Used for WFE-based blocks (ResourceBusy, DuplicateInProgress).
	// Optional: only set when blocking is due to an active WFE.
	BlockingWorkflowExecution string

	// DuplicateOf references the original RemediationRequest that this RR
	// is a duplicate of.
	// Used only for DuplicateInProgress blocks.
	// Optional: only set when Reason = "DuplicateInProgress".
	DuplicateOf string
}

// TargetResource identifies a Kubernetes resource for routing decisions.
// Issue #214: Replaces the raw `targetResource string` parameter with a typed struct
// so that CheckIneffectiveRemediationChain can query DataStorage with structured fields.
type TargetResource struct {
	Kind      string
	Name      string
	Namespace string
}

// String returns the formatted target resource string used in logging and WFE matching.
func (t TargetResource) String() string {
	return fmt.Sprintf("%s/%s/%s", t.Namespace, t.Kind, t.Name)
}

// RemediationHistoryQuerier abstracts DataStorage queries for remediation history.
// Issue #214: Injected into RoutingEngine to enable unit testing with mocks.
type RemediationHistoryQuerier interface {
	GetRemediationHistory(
		ctx context.Context,
		target TargetResource,
		currentSpecHash string,
		window time.Duration,
	) ([]ogenclient.RemediationHistoryEntry, error)
}

// IsTerminalPhase checks if a RemediationRequest phase is terminal.
// Terminal phases indicate the RR has finished processing and will not
// transition to any other phase.
//
// Terminal phases: Completed, Failed, TimedOut, Skipped, Cancelled
// Non-terminal phases: Pending, Processing, Analyzing, AwaitingApproval, Executing, Blocked
//
// Reference: DD-RO-002-ADDENDUM (Blocked Phase Semantics)
func IsTerminalPhase(phase remediationv1.RemediationPhase) bool {
	switch phase {
	case remediationv1.PhaseCompleted,
		remediationv1.PhaseFailed,
		remediationv1.PhaseTimedOut,
		remediationv1.PhaseSkipped,
		remediationv1.PhaseCancelled:
		return true
	default:
		return false
	}
}
