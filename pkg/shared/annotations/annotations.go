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

// Package annotations defines cross-cutting Kubernaut annotation constants
// used across multiple services (Gateway, RO, API Frontend, etc.).
package annotations

const (
	// InteractiveModeKey is the annotation key set on a RemediationRequest
	// to indicate that the associated AIAnalysis should run in interactive
	// (human-in-the-loop) mode via MCP. The API Frontend sets this annotation
	// when an MCP session initiates a remediation.
	//
	// Value: "true" to enable interactive mode; absent or any other value means autonomous.
	//
	// Reference: DD-INTERACTIVE-001, Enhancement #703
	InteractiveModeKey = "kubernaut.ai/interactive-mode"

	// InteractiveModeValueTrue is the canonical value for InteractiveModeKey.
	InteractiveModeValueTrue = "true"
)

// IsInteractiveMode checks whether the given annotations map indicates
// interactive mode is enabled.
func IsInteractiveMode(annotations map[string]string) bool {
	return annotations[InteractiveModeKey] == InteractiveModeValueTrue
}
