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
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// CompleteNoActionInput defines the input schema for the kubernaut_complete_no_action tool.
type CompleteNoActionInput struct {
	RRID             string   `json:"rr_id"`
	Reason           string   `json:"reason,omitempty"`
	EscalationReason string   `json:"escalation_reason,omitempty"`
	ActingUser       string   `json:"acting_user,omitempty"`
	ActingUserGroups []string `json:"acting_user_groups,omitempty"`
}

// actingUserOverride implements actingUserInput (registration.go).
func (i CompleteNoActionInput) actingUserOverride() (string, []string) {
	return i.ActingUser, i.ActingUserGroups
}

// CompleteNoActionOutput defines the output schema for the kubernaut_complete_no_action tool.
type CompleteNoActionOutput struct {
	Status           string `json:"status"`
	Reason           string `json:"reason,omitempty"`
	EscalationReason string `json:"escalation_reason,omitempty"`
}

// CompleteNoActionTool handles the kubernaut_complete_no_action MCP tool.
// Allows the user to explicitly conclude an investigation without selecting
// a workflow. No discovery gate — can be called at any point in the session.
type CompleteNoActionTool struct {
	sessions       mcpinternal.SessionManager
	httpCompleter  HTTPSessionCompleter
	mutexProvider  SessionMutexProvider
	timeoutTracker TimeoutTracker
	logger         logr.Logger
}

// CompleteNoActionOption configures optional dependencies.
type CompleteNoActionOption func(*CompleteNoActionTool)

// WithCompleteNoActionLogger sets the logger.
func WithCompleteNoActionLogger(logger logr.Logger) CompleteNoActionOption {
	return func(t *CompleteNoActionTool) { t.logger = logger }
}

// WithCompleteNoActionHTTPCompleter sets the HTTP session completer.
func WithCompleteNoActionHTTPCompleter(completer HTTPSessionCompleter) CompleteNoActionOption {
	return func(t *CompleteNoActionTool) {
		if completer != nil {
			t.httpCompleter = completer
		}
	}
}

// WithCompleteNoActionMutexProvider sets the mutex provider.
func WithCompleteNoActionMutexProvider(provider SessionMutexProvider) CompleteNoActionOption {
	return func(t *CompleteNoActionTool) {
		if provider != nil {
			t.mutexProvider = provider
		}
	}
}

// WithCompleteNoActionTimeoutTracker sets the timeout tracker for session cleanup.
func WithCompleteNoActionTimeoutTracker(tt TimeoutTracker) CompleteNoActionOption {
	return func(t *CompleteNoActionTool) {
		if tt != nil {
			t.timeoutTracker = tt
		}
	}
}

// NewCompleteNoActionTool creates the tool handler with its dependencies.
func NewCompleteNoActionTool(sessions mcpinternal.SessionManager, opts ...CompleteNoActionOption) *CompleteNoActionTool {
	t := &CompleteNoActionTool{sessions: sessions, logger: logr.Discard()}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// Handle validates the session and completes the investigation with no workflow.
// validateCompleteNoActionInput validates the request-level fields of a
// complete_no_action call: rr_id must be present, and when supplied,
// escalation_reason must be non-blank and within the length limit.
func validateCompleteNoActionInput(input CompleteNoActionInput) error {
	if input.RRID == "" {
		return fmt.Errorf("rr_id is required")
	}
	if input.EscalationReason != "" && strings.TrimSpace(input.EscalationReason) == "" {
		return fmt.Errorf("escalation_reason must not be whitespace-only")
	}
	if input.EscalationReason != "" && len(input.EscalationReason) > 1024 {
		return fmt.Errorf("escalation_reason exceeds maximum length of 1024")
	}
	return nil
}

// authorizeCompleteNoActionDriver verifies there is an active interactive
// session for rrID and that user is its current driver.
func (t *CompleteNoActionTool) authorizeCompleteNoActionDriver(rrID string, user mcpinternal.UserInfo) (*mcpinternal.InteractiveSession, error) {
	if !t.sessions.IsDriverActive(rrID) {
		return nil, fmt.Errorf("no active interactive session for rr_id")
	}
	driver, err := t.sessions.GetDriver(rrID)
	if err != nil || driver == nil {
		return nil, fmt.Errorf("no active interactive session for rr_id")
	}
	if driver.ActingUser.Username != user.Username {
		return nil, fmt.Errorf("caller is not the active driver for this session")
	}
	return driver, nil
}

// buildNoActionResult constructs the final InvestigationResult for a
// complete_no_action call, branching on whether the caller is escalating
// (operator_escalation, human review required) or dismissing (not
// actionable, no human review needed). Starts from the driver's RCA result
// when available, or a minimal placeholder otherwise.
func buildNoActionResult(driver *mcpinternal.InteractiveSession, input CompleteNoActionInput) *katypes.InvestigationResult {
	var finalResult *katypes.InvestigationResult
	if driver.RCAResult != nil {
		result := *driver.RCAResult
		finalResult = &result
	} else {
		finalResult = &katypes.InvestigationResult{
			RCASummary: "Investigation completed without workflow selection",
		}
	}

	if input.EscalationReason != "" {
		finalResult.HumanReviewNeeded = true
		finalResult.HumanReviewReason = "operator_escalation"
		finalResult.Reason = input.EscalationReason
	} else {
		finalResult.HumanReviewNeeded = false
		finalResult.HumanReviewReason = ""
		finalResult.Reason = input.Reason
		if finalResult.Reason == "" {
			finalResult.Reason = "no action needed"
		}
		notActionable := false
		finalResult.IsActionable = &notActionable
		finalResult.Warnings = append(finalResult.Warnings, "Alert not actionable")
	}

	return finalResult
}

func (t *CompleteNoActionTool) Handle(_ context.Context, input CompleteNoActionInput, user mcpinternal.UserInfo) (CompleteNoActionOutput, error) {
	if err := validateCompleteNoActionInput(input); err != nil {
		return CompleteNoActionOutput{}, err
	}

	if t.mutexProvider != nil {
		mu := t.mutexProvider.GetSessionMutex(input.RRID)
		mu.Lock()
		defer mu.Unlock()
	}

	driver, err := t.authorizeCompleteNoActionDriver(input.RRID, user)
	if err != nil {
		return CompleteNoActionOutput{}, err
	}

	finalResult := buildNoActionResult(driver, input)

	if t.timeoutTracker != nil {
		t.timeoutTracker.StopTracking(driver.SessionID)
	}

	CompleteHTTPSession(t.httpCompleter, input.RRID, finalResult, t.logger, "complete_no_action")

	// Release the MCP interactive lease.
	if releaseErr := t.sessions.Release(driver.SessionID, "complete_no_action"); releaseErr != nil {
		if !errors.Is(releaseErr, mcpinternal.ErrSessionNotFound) {
			t.logger.Error(releaseErr, "failed to release MCP lease", "session_id", driver.SessionID)
		}
	}

	// Determine output status
	status := "completed_no_action"
	if input.EscalationReason != "" {
		status = "escalated"
	}

	return CompleteNoActionOutput{
		Status:           status,
		Reason:           input.Reason,
		EscalationReason: input.EscalationReason,
	}, nil
}
