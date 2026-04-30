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

import "errors"

// InvestigateInput is the JSON schema for the kubernaut_investigate MCP tool.
type InvestigateInput struct {
	RRID    string `json:"rr_id"`
	Action  string `json:"action"` // start, message, complete, cancel
	Message string `json:"message,omitempty"`
}

// InvestigateOutput is the response returned by the kubernaut_investigate tool.
type InvestigateOutput struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	Response  string `json:"response,omitempty"`
}

// Valid actions for the kubernaut_investigate tool.
const (
	ActionStart    = "start"
	ActionMessage  = "message"
	ActionComplete = "complete"
	ActionCancel   = "cancel"
	ActionTakeover = "takeover"
)

var (
	// ErrInvalidAction indicates the action field is not a recognised value.
	ErrInvalidAction = errors.New("invalid action")

	// ErrNoActiveSession indicates no interactive session exists for the operation.
	ErrNoActiveSession = errors.New("no active session for this remediation")

	// ErrMissingRRID indicates rr_id was not provided.
	ErrMissingRRID = errors.New("rr_id is required")

	// ErrMissingMessage indicates message was not provided for a message action.
	ErrMissingMessage = errors.New("message is required for action=message")
)

// ValidateInput checks that all required fields are present for the given action.
func ValidateInput(input InvestigateInput) error {
	if input.RRID == "" {
		return ErrMissingRRID
	}
	switch input.Action {
	case ActionStart, ActionComplete, ActionCancel, ActionTakeover:
		return nil
	case ActionMessage:
		if input.Message == "" {
			return ErrMissingMessage
		}
		return nil
	default:
		return ErrInvalidAction
	}
}
