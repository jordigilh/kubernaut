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

package config

import "fmt"

// ConfigError provides detailed configuration validation errors with actionable guidance.
// GAP-8: Enhanced Configuration Validation
//
// This structured error type helps operators quickly identify and fix configuration issues
// by providing:
// - Field: The exact configuration field path (e.g., "processing.deduplication.ttl")
// - Value: The invalid value provided
// - Reason: Why the value is invalid
// - Suggestion: Recommended value or fix
// - Impact: What happens if the issue is not fixed
// - Documentation: Link to relevant documentation
//
// Example:
//
//	&ConfigError{
//	    Field:         "processing.deduplication.ttl",
//	    Value:         "5s",
//	    Reason:        "below minimum threshold (< 10s)",
//	    Suggestion:    "Use 5m for production, minimum 10s",
//	    Impact:        "May cause duplicate RemediationRequest CRDs",
//	    Documentation: "docs/services/stateless/gateway-service/configuration.md#deduplication",
//	}
type ConfigError struct {
	Field         string // Configuration field path (e.g., "server.listen_addr")
	Value         string // Invalid value provided (human-readable)
	Reason        string // Why the value is invalid
	Suggestion    string // Recommended value or action
	Impact        string // What happens if not fixed
	Documentation string // Link to relevant documentation (optional)
}

// Error implements the error interface with rich, actionable error messages.
// Format:
//
//	Configuration error in 'field': reason (got: value)
//	  Suggestion: recommended value or action
//	  Impact: description of consequences
//	  Documentation: link to docs
func (e *ConfigError) Error() string {
	msg := fmt.Sprintf("configuration error in '%s': %s", e.Field, e.Reason)

	if e.Value != "" {
		msg += fmt.Sprintf(" (got: %s)", e.Value)
	}

	if e.Suggestion != "" {
		msg += fmt.Sprintf("\n  Suggestion: %s", e.Suggestion)
	}

	if e.Impact != "" {
		msg += fmt.Sprintf("\n  Impact: %s", e.Impact)
	}

	if e.Documentation != "" {
		msg += fmt.Sprintf("\n  Documentation: %s", e.Documentation)
	}

	return msg
}

// NewConfigError is a convenience function to create a ConfigError with required fields.
// Optional fields (Impact, Documentation) can be set after creation.
func NewConfigError(field, value, reason, suggestion string) *ConfigError {
	return &ConfigError{
		Field:      field,
		Value:      value,
		Reason:     reason,
		Suggestion: suggestion,
	}
}
