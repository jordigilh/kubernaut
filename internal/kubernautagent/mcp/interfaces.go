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

// UserInfo represents a resolved user identity from TokenReview or impersonation.
// DD-AUTH-MCP-001: Pattern A (TokenReview) or Pattern B (SAR-verified delegation).
type UserInfo struct {
	Username string
	Groups   []string
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
}

// SessionManager manages interactive session lifecycle: takeover, release, and
// driver querying. Backed by K8s coordination/v1 Lease for single-driver guarantee.
// Adapts the pattern from pkg/remediationorchestrator/locking/distributed_lock.go.
type SessionManager interface {
	// Takeover acquires the Lease for the given RR, cancels autonomous mode,
	// and creates an interactive session for the user.
	// Returns lease_held error if another driver is active.
	Takeover(ctx context.Context, rrID string, user UserInfo) (*InteractiveSession, error)

	// Release ends the interactive session, releases the Lease, and signals
	// autonomous reconstruction. Reason: "disconnect", "timeout", "explicit".
	Release(sessionID string, reason string) error

	// GetDriver returns the current driver session for an RR, or nil if autonomous.
	GetDriver(rrID string) (*InteractiveSession, error)

	// IsDriverActive returns true if a user is currently driving the given RR.
	IsDriverActive(rrID string) bool
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
