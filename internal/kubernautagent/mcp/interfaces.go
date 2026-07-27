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

// Package mcp provides the MCP (Model Context Protocol) interactive mode
// infrastructure for Kubernaut Agent. DD-INTERACTIVE-002, BR-INTERACTIVE-001..008.
package mcp

import (
	"context"
	"time"

	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// NotificationType distinguishes notification categories on the bus.
// v1.5: audit events only. v1.6: extends with token streaming.
type NotificationType string

const (
	NotificationAuditEvent NotificationType = "audit_event"
	NotificationToken      NotificationType = "token" // v1.6
)

// Notification is a single event delivered via the NotificationBus.
type Notification struct {
	Type          NotificationType
	CorrelationID string
	SessionID     string
	Payload       interface{}
	Timestamp     time.Time
}

// NotificationBus is a generic pub/sub delivering completed audit events to
// connected observers/drivers. DD-INTERACTIVE-002: bounded channel buffer with
// configurable drop policy (slow consumers don't block publisher).
type NotificationBus interface {
	Subscribe(correlationID, sessionID string) <-chan Notification
	Publish(correlationID string, n Notification)
	Unsubscribe(correlationID, sessionID string)
}

// UserInfo represents a resolved user identity from TokenReview or trusted
// intermediary payload (acting_user). DD-AUTH-MCP-001 v3.0.
type UserInfo struct {
	Username     string
	Groups       []string
	ProviderType string
}

// InteractiveSession represents an active interactive session state.
// DD-INTERACTIVE-002: single-driver, Lease-backed, cancel+reconstruct lifecycle.
type InteractiveSession struct {
	SessionID     string
	MCPSessionID  string
	CorrelationID string
	ActingUser    UserInfo
	StartedAt     time.Time
	CompletedAt   *time.Time
	Reason        string
	// Reconnected is true when Takeover returned an existing session for the
	// same user (e.g., after network loss). Callers use this to skip
	// one-time operations like audit emission and autonomous transition.
	Reconnected bool

	// RCAResult holds the structured RCA extracted from the interactive conversation
	// when discover_workflows is called. Used by select_workflow and complete_no_action
	// to build the final InvestigationResult for the HTTP session store.
	RCAResult *katypes.InvestigationResult

	// DiscoveryResult holds the Phase 3 workflow discovery recommendations.
	// Cleared by any subsequent message to prevent stale recommendations.
	DiscoveryResult *WorkflowDiscoveryResult
}

// DiscoveryTargetInfo identifies a Kubernetes resource involved in workflow
// discovery. Used to surface both the resource that was searched against the
// catalog (searched_target) and the original alert resource (signal_target)
// so the Console can explain cross-resource RCA scenarios (#1437).
type DiscoveryTargetInfo struct {
	APIVersion string `json:"api_version,omitempty"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
}

// WorkflowDiscoveryResult holds the Phase 3 recommendations returned to the user
// by discover_workflows. Contains the selected + alternative workflows along with
// the full InvestigationResult for final assembly in select_workflow.
type WorkflowDiscoveryResult struct {
	// Recommended is the top-ranked workflow.
	Recommended *DiscoveredWorkflow `json:"recommended,omitempty"`
	// Alternatives are additional workflows the user may choose from.
	Alternatives []DiscoveredWorkflow `json:"alternatives,omitempty"`
	// SearchedTarget is the resource that was searched against the workflow
	// catalog. Sourced from workflowResult.RemediationTarget (#1437).
	SearchedTarget *DiscoveryTargetInfo `json:"searched_target,omitempty"`
	// SignalTarget is the original alert resource before any RCA override.
	SignalTarget *DiscoveryTargetInfo `json:"signal_target,omitempty"`
	// FullResult is the complete InvestigationResult from Phase 3 (includes RCA +
	// workflow selection). Used to build the final result for the HTTP session.
	FullResult *katypes.InvestigationResult `json:"-"`
}

// DiscoveredWorkflow represents a single workflow recommendation from Phase 3.
type DiscoveredWorkflow struct {
	WorkflowID      string                 `json:"workflow_id"`
	Name            string                 `json:"name,omitempty"`
	ExecutionBundle string                 `json:"execution_bundle,omitempty"`
	Confidence      float64                `json:"confidence"`
	Rationale       string                 `json:"rationale"`
	Parameters      map[string]interface{} `json:"parameters,omitempty"`
}

// SessionLifecycle manages interactive session takeover/release: the
// Lease-backed, single-driver-guaranteeing mutations. Split out from
// SessionManager for ISP (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 0b) — e.g.
// SelectWorkflowTool only ever needs SessionQuerier below, never mutates
// session lifecycle.
type SessionLifecycle interface {
	// Takeover acquires the Lease for the given RR, cancels autonomous mode,
	// and creates an interactive session for the user.
	// Returns lease_held error if another driver is active.
	Takeover(ctx context.Context, rrID string, user UserInfo) (*InteractiveSession, error)

	// Release ends the interactive session, releases the Lease, and signals
	// autonomous reconstruction. Reason: "disconnect", "timeout", "explicit".
	Release(sessionID string, reason string) error
}

// SessionQuerier provides read-only driver/activity queries for interactive
// sessions. Split out from SessionManager for ISP
// (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 0b).
type SessionQuerier interface {
	// GetDriver returns the current driver session for an RR, or
	// ErrSessionNotFound if no interactive session exists (e.g., autonomous mode).
	GetDriver(rrID string) (*InteractiveSession, error)

	// IsDriverActive returns true if a user is currently driving the given RR.
	IsDriverActive(rrID string) bool

	// TouchActivity updates the last activity timestamp for a session (SEC-04).
	// Called by tool handlers on each interaction to reset the inactivity timer.
	TouchActivity(rrID string)
}

// SessionManager manages interactive session lifecycle: takeover, release, and
// driver querying. Backed by K8s coordination/v1 Lease for single-driver guarantee.
// Adapts the pattern from pkg/remediationorchestrator/locking/distributed_lock.go.
// Kept as a named union of SessionLifecycle + SessionQuerier — rather than
// inlining both at call sites — so existing implementers (LeaseSessionManager)
// and mocks (which already implement every method) need no changes
// (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 0b; see docs/architecture/audits for
// rationale).
type SessionManager interface {
	SessionLifecycle
	SessionQuerier
}

// ConversationTurn represents a single LLM turn reconstructed from DS audit events.
// Used by ContextReconstructor to rebuild conversation history after cancel+reconstruct.
type ConversationTurn struct {
	SessionID  string
	ActingUser string
	Role       string // "assistant" or "user"
	Content    string
	Timestamp  time.Time
}

// ContextReconstructor queries DS audit events and rebuilds conversation history
// for a given remediation. DD-INTERACTIVE-002: auto-inject seeds new session's
// LLM context with prior findings from all sessions (excluding own session_id).
type ContextReconstructor interface {
	// Reconstruct queries DS by correlationID, excludes events from excludeSessionID,
	// and returns conversation turns ordered by timestamp.
	// Returns empty slice (not error) if DS is unavailable (best-effort, BR-INTERACTIVE-008).
	Reconstruct(ctx context.Context, correlationID string, excludeSessionID string) ([]ConversationTurn, error)
}
