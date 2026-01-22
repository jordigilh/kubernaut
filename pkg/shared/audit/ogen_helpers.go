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

// Package audit provides shared audit helpers for ogen-client type conversions.
//
// This file contains reusable helpers for converting internal audit types to
// OpenAPI-generated (ogen) types used by the DataStorage API.
//
// Business Requirement: BR-AUDIT-005 v2.0 (SOC2 Type II + RR Reconstruction)
// Authority: api/openapi/data-storage-v1.yaml (ErrorDetails.component enum)
package audit

import (
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// ComponentMapping maps string component names to OpenAPI ErrorDetails.Component enum values.
// This provides a single source of truth for component enum conversion across all services.
//
// **Authority**: api/openapi/data-storage-v1.yaml (ErrorDetails.component enum)
//
// **Valid Components**:
// - gateway
// - aianalysis
// - workflowexecution
// - authwebhook
// - remediationorchestrator
// - signalprocessing
//
// **Usage**: Services use this map directly or via ToOgenErrorDetailsComponent() helper.
//
// **Refactoring Note**: 2026-01-22 - Created shared mapping to eliminate duplication across services
var ComponentMapping = map[string]ogenclient.ErrorDetailsComponent{
	"gateway":                 ogenclient.ErrorDetailsComponentGateway,
	"aianalysis":              ogenclient.ErrorDetailsComponentAianalysis,
	"workflowexecution":       ogenclient.ErrorDetailsComponentWorkflowexecution,
	"authwebhook":             ogenclient.ErrorDetailsComponentAuthwebhook,
	"remediationorchestrator": ogenclient.ErrorDetailsComponentRemediationorchestrator,
	"signalprocessing":        ogenclient.ErrorDetailsComponentSignalprocessing,
}

// ToOgenErrorDetailsComponent converts a string component name to ogen ErrorDetailsComponent enum.
//
// This helper provides type-safe conversion from internal component strings to OpenAPI enum values.
// If the component string is not found in the mapping, returns empty string (invalid per OpenAPI spec).
//
// **Parameters**:
//   - component: Service component name (e.g., "gateway", "aianalysis", "workflowexecution")
//
// **Returns**:
//   - ogenclient.ErrorDetailsComponent: OpenAPI-generated enum value
//   - Empty string if component not found in mapping (invalid per OpenAPI spec)
//
// **Example**:
//
//	component := sharedaudit.ToOgenErrorDetailsComponent("aianalysis")
//	// component = ogenclient.ErrorDetailsComponentAianalysis
//
// **Authority**: api/openapi/data-storage-v1.yaml (ErrorDetails.component enum)
func ToOgenErrorDetailsComponent(component string) ogenclient.ErrorDetailsComponent {
	if mapped, ok := ComponentMapping[component]; ok {
		return mapped
	}
	return "" // Invalid component: will fail OpenAPI validation
}

// ToOgenOptErrorDetails converts internal ErrorDetails to ogen-generated OptErrorDetails.
//
// This helper bridges the gap between internal audit.ErrorDetails and OpenAPI-generated types
// used by the DataStorage API. It handles nil input gracefully and uses ComponentMapping for
// type-safe enum conversion.
//
// **Parameters**:
//   - errorDetails: Internal error details structure (can be nil)
//
// **Returns**:
//   - ogenclient.OptErrorDetails: OpenAPI-generated optional error details
//   - Empty OptErrorDetails if input is nil
//
// **Example**:
//
//	errorDetails := audit.NewErrorDetails("aianalysis", "ERR_K8S_NOT_FOUND", "Resource not found", false)
//	ogenErrorDetails := sharedaudit.ToOgenOptErrorDetails(errorDetails)
//	// Use ogenErrorDetails in audit event submission
//
// **Authority**: api/openapi/data-storage-v1.yaml (ErrorDetails schema)
func ToOgenOptErrorDetails(errorDetails *ErrorDetails) ogenclient.OptErrorDetails {
	if errorDetails == nil {
		return ogenclient.OptErrorDetails{}
	}

	ogenErrorDetails := ogenclient.ErrorDetails{
		Message:       errorDetails.Message,
		Code:          errorDetails.Code,
		RetryPossible: errorDetails.RetryPossible,
	}

	// Convert Component enum using shared ComponentMapping (type-safe, maintainable)
	if component, ok := ComponentMapping[errorDetails.Component]; ok {
		ogenErrorDetails.Component = component
	}
	// If not found, Component will be zero value (empty string) - invalid per OpenAPI spec

	// Set StackTrace ([]string, not optional)
	if len(errorDetails.StackTrace) > 0 {
		ogenErrorDetails.StackTrace = errorDetails.StackTrace
	}

	var result ogenclient.OptErrorDetails
	result.SetTo(ogenErrorDetails)
	return result
}
