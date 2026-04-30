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
	"fmt"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
)

// LLMMessage represents a single conversation message for the investigator.
type LLMMessage struct {
	Role    string
	Content string
}

// InvestigatorRunner is the interface for executing interactive LLM turns.
// Implemented by the real Investigator.RunInteractiveTurn via an adapter.
type InvestigatorRunner interface {
	RunInteractiveTurn(ctx context.Context, messages []LLMMessage, correlationID string) (string, error)
}

// InvestigateTool handles the kubernaut_investigate MCP tool actions:
// start, message, complete, cancel. BR-INTERACTIVE-001.
type InvestigateTool struct {
	sessions mcpinternal.SessionManager
	runner   InvestigatorRunner
	recon    mcpinternal.ContextReconstructor
}

// NewInvestigateTool creates the tool handler with its dependencies.
func NewInvestigateTool(sessions mcpinternal.SessionManager, runner InvestigatorRunner, recon mcpinternal.ContextReconstructor) *InvestigateTool {
	return &InvestigateTool{
		sessions: sessions,
		runner:   runner,
		recon:    recon,
	}
}

// Handle dispatches the input to the correct action handler.
func (t *InvestigateTool) Handle(ctx context.Context, input InvestigateInput, user mcpinternal.UserInfo) (InvestigateOutput, error) {
	if err := ValidateInput(input); err != nil {
		return InvestigateOutput{}, err
	}

	switch input.Action {
	case ActionStart:
		return t.handleStart(ctx, input, user)
	case ActionMessage:
		return t.handleMessage(ctx, input)
	case ActionComplete:
		return t.handleComplete(input)
	case ActionCancel:
		return t.handleCancel(input)
	default:
		return InvestigateOutput{}, ErrInvalidAction
	}
}

func (t *InvestigateTool) handleStart(ctx context.Context, input InvestigateInput, user mcpinternal.UserInfo) (InvestigateOutput, error) {
	session, err := t.sessions.Takeover(ctx, input.RRID, user)
	if err != nil {
		return InvestigateOutput{}, err
	}

	// Best-effort context reconstruction from prior sessions
	_, _ = t.recon.Reconstruct(ctx, input.RRID, session.SessionID)

	return InvestigateOutput{
		SessionID: session.SessionID,
		Status:    "started",
	}, nil
}

func (t *InvestigateTool) handleMessage(ctx context.Context, input InvestigateInput) (InvestigateOutput, error) {
	if !t.sessions.IsDriverActive(input.RRID) {
		return InvestigateOutput{}, ErrNoActiveSession
	}

	session, err := t.sessions.GetDriver(input.RRID)
	if err != nil || session == nil {
		return InvestigateOutput{}, ErrNoActiveSession
	}

	messages := []LLMMessage{{Role: "user", Content: input.Message}}

	response, err := t.runner.RunInteractiveTurn(ctx, messages, input.RRID)
	if err != nil {
		return InvestigateOutput{}, fmt.Errorf("interactive turn failed: %w", err)
	}

	return InvestigateOutput{
		SessionID: session.SessionID,
		Status:    "message_received",
		Response:  response,
	}, nil
}

func (t *InvestigateTool) handleComplete(input InvestigateInput) (InvestigateOutput, error) {
	if !t.sessions.IsDriverActive(input.RRID) {
		return InvestigateOutput{}, ErrNoActiveSession
	}

	session, err := t.sessions.GetDriver(input.RRID)
	if err != nil || session == nil {
		return InvestigateOutput{}, ErrNoActiveSession
	}

	if err := t.sessions.Release(session.SessionID, "complete"); err != nil {
		return InvestigateOutput{}, fmt.Errorf("release session: %w", err)
	}

	return InvestigateOutput{
		SessionID: session.SessionID,
		Status:    "completed",
	}, nil
}

func (t *InvestigateTool) handleCancel(input InvestigateInput) (InvestigateOutput, error) {
	if !t.sessions.IsDriverActive(input.RRID) {
		return InvestigateOutput{}, ErrNoActiveSession
	}

	session, err := t.sessions.GetDriver(input.RRID)
	if err != nil || session == nil {
		return InvestigateOutput{}, ErrNoActiveSession
	}

	if err := t.sessions.Release(session.SessionID, "explicit"); err != nil {
		return InvestigateOutput{}, fmt.Errorf("release session: %w", err)
	}

	return InvestigateOutput{
		SessionID: session.SessionID,
		Status:    "cancelled",
	}, nil
}
