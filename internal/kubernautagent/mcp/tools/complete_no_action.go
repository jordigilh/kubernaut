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

	"github.com/go-logr/logr"
	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// CompleteNoActionInput defines the input schema for the kubernaut_complete_no_action tool.
type CompleteNoActionInput struct {
	RRID             string   `json:"rr_id"`
	Reason           string   `json:"reason,omitempty"`
	ActingUser       string   `json:"acting_user,omitempty"`
	ActingUserGroups []string `json:"acting_user_groups,omitempty"`
}

// CompleteNoActionOutput defines the output schema for the kubernaut_complete_no_action tool.
type CompleteNoActionOutput struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
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
func (t *CompleteNoActionTool) Handle(_ context.Context, input CompleteNoActionInput, user mcpinternal.UserInfo) (CompleteNoActionOutput, error) {
	if input.RRID == "" {
		return CompleteNoActionOutput{}, fmt.Errorf("rr_id is required")
	}

	if t.mutexProvider != nil {
		mu := t.mutexProvider.GetSessionMutex(input.RRID)
		mu.Lock()
		defer mu.Unlock()
	}

	if !t.sessions.IsDriverActive(input.RRID) {
		return CompleteNoActionOutput{}, fmt.Errorf("no active interactive session for rr_id")
	}

	driver, err := t.sessions.GetDriver(input.RRID)
	if err != nil || driver == nil {
		return CompleteNoActionOutput{}, fmt.Errorf("no active interactive session for rr_id")
	}

	if driver.ActingUser.Username != user.Username {
		return CompleteNoActionOutput{}, fmt.Errorf("caller is not the active driver for this session")
	}

	// Build InvestigationResult: use RCA if available, otherwise minimal result.
	var finalResult *katypes.InvestigationResult
	if driver.RCAResult != nil {
		result := *driver.RCAResult
		result.Reason = input.Reason
		if result.Reason == "" {
			result.Reason = "no action needed"
		}
		finalResult = &result
	} else {
		finalResult = &katypes.InvestigationResult{
			RCASummary: "Investigation completed without workflow selection",
			Reason:     input.Reason,
		}
		if finalResult.Reason == "" {
			finalResult.Reason = "no action needed"
		}
	}

	// Signal to AA that no remediation is needed. AA's ProcessIncidentResponse
	// requires both the "Alert not actionable" warning and IsActionable=false
	// to route to Completed/WorkflowNotNeeded/NotActionable instead of
	// Failed/WorkflowResolutionFailed.
	notActionable := false
	finalResult.IsActionable = &notActionable
	finalResult.Warnings = append(finalResult.Warnings, "Alert not actionable")

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

	return CompleteNoActionOutput{
		Status: "completed_no_action",
		Reason: input.Reason,
	}, nil
}
