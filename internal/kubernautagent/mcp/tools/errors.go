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

package tools

import (
	"errors"
	"fmt"

	"github.com/go-logr/logr"
)

// MCPError represents a structured error returned to MCP clients.
// Contains a machine-readable code, a human-readable message, and optional
// contextual details. Satisfies the error interface for seamless propagation
// through the tool handler chain. BR-INTERACTIVE-004, PROD-02.
type MCPError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

func (e *MCPError) Error() string {
	if len(e.Details) > 0 {
		return fmt.Sprintf("%s: %s (%v)", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// WithDetail returns a copy of the error with an additional detail key-value pair.
func (e *MCPError) WithDetail(key, value string) *MCPError {
	cp := &MCPError{
		Code:    e.Code,
		Message: e.Message,
		Details: make(map[string]string, len(e.Details)+1),
	}
	for k, v := range e.Details {
		cp.Details[k] = v
	}
	cp.Details[key] = value
	return cp
}

var (
	ErrCodeSessionActive = &MCPError{
		Code:    "session_active",
		Message: "Investigation is being driven by another user",
	}
	ErrCodeNotDriving = &MCPError{
		Code:    "not_driving",
		Message: "You must send action=takeover before sending messages",
	}
	ErrCodeNotFound = &MCPError{
		Code:    "not_found",
		Message: "No active investigation found for this remediation",
	}
	ErrCodeRateLimited = &MCPError{
		Code:    "rate_limited",
		Message: "Too many requests. Please slow down.",
	}
	ErrCodeSessionExpired = &MCPError{
		Code:    "session_expired",
		Message: "Session has expired due to TTL or inactivity. Start a new session.",
	}
	ErrCodeRRNotFound = &MCPError{
		Code:    "rr_not_found",
		Message: "RemediationRequest not found",
	}
	ErrCodeForbidden = &MCPError{
		Code:    "forbidden",
		Message: "Access denied: insufficient RBAC permissions",
	}
	ErrCodeInternalError = &MCPError{
		Code:    "internal_error",
		Message: "Internal service error",
	}
	ErrCodeMaxInvestigations = &MCPError{
		Code:    "max_investigations",
		Message: "Maximum concurrent investigations reached, retry later",
	}
	ErrCodeDiscoveryRequired = &MCPError{
		Code:    "discovery_required",
		Message: "discover_workflows must be called before select_workflow",
	}
	ErrCodeInvalidWorkflow = &MCPError{
		Code:    "invalid_workflow",
		Message: "workflow_id not found in discovery results; call discover_workflows to refresh",
	}
	ErrCodeInvalidInput = &MCPError{
		Code:    "invalid_input",
		Message: "Missing required input field",
	}
	ErrCodeNoSession = &MCPError{
		Code:    "no_session",
		Message: "No active interactive session for this remediation",
	}
	ErrCodeNotDriver = &MCPError{
		Code:    "not_driver",
		Message: "Caller is not the active driver for this session",
	}
	// ErrCodeToolBudgetExhausted is returned when the AnomalyDetector's
	// total tool-call budget trips mid-investigation (#1889 gap 1). Before
	// this fix, ExtractContent wrapped ExhaustedResult{Reason:
	// ReasonToolBudgetExhausted} in a plain fmt.Errorf, which ErrorBoundary
	// then redacted to the generic ErrCodeInternalError — leaving the MCP
	// client unable to distinguish "hit a real safety limit" from "server bug".
	ErrCodeToolBudgetExhausted = &MCPError{
		Code:    "tool_budget_exhausted",
		Message: "Investigation exceeded the maximum number of tool calls allowed",
	}
	// ErrCodeNoConversationContext is returned by discover_workflows when
	// neither the session's stored RCA result, the live conversation, nor
	// audit-trail reconstruction produced any content to extract an RCA
	// from (#2023). Before this fix, this case returned a plain fmt.Errorf,
	// which ErrorBoundary redacted to the generic ErrCodeInternalError —
	// leaving the calling agent unable to distinguish "there is genuinely
	// nothing to analyze" from "server bug", which in turn left room for
	// the LLM to fabricate a plausible-sounding RCA/audit-trail narrative
	// rather than surfacing the gap (SI-11).
	ErrCodeNoConversationContext = &MCPError{
		Code:    "no_conversation_context",
		Message: "No investigation content available: no stored RCA, live conversation, or audit trail was found for this remediation",
	}
	// ErrCodeNoInvestigationAvailable is returned by handleStart when every
	// fallback path for attaching an investigation to a newly-acquired
	// interactive Lease has been exhausted: no running/terminal autonomous
	// session exists for the RR, reattachOrCreateFallback's own fallback
	// session creation failed, AND ForceTransitionToUserDriving also failed
	// (#2100). Before this fix, this case fell through to a silent
	// "started" response with an empty InvestigationSessionID -- the
	// client held a Lease (eroding interactive.maxConcurrentSessions
	// capacity) with nothing behind it, reclaimed only incidentally by
	// TimeoutManager's ~10-minute inactivity window. Failing closed here
	// (with an immediate lease Release) surfaces the genuine "nothing to
	// drive" condition to the caller right away, instead of returning a
	// misleading success the client has no way to detect.
	ErrCodeNoInvestigationAvailable = &MCPError{
		Code:    "no_investigation_available",
		Message: "No investigation is available to attach an interactive session to for this remediation",
	}
)

// ErrorBoundary wraps a tool handler error: if the error is already an MCPError
// it passes through unchanged; otherwise the raw error is logged and replaced
// with a generic internal error to prevent information leakage (H3/SEC-5).
func ErrorBoundary(logger logr.Logger, toolName string, err error) error {
	if err == nil {
		return nil
	}
	var mcpErr *MCPError
	if errors.As(err, &mcpErr) {
		return err
	}
	logger.Error(err, "tool handler error redacted", "tool", toolName)
	return ErrCodeInternalError
}
