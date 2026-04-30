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

import "fmt"

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
	ErrCodeInvestigationCompleted = &MCPError{
		Code:    "investigation_completed",
		Message: "Investigation has already completed",
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
)
