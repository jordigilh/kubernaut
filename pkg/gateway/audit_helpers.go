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

package gateway

import (
	"encoding/json"
	"strings"

	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/shared/audit"
)

// ========================================
// TDD REFACTOR Phase 3: Data-Driven Enum Mapping Tables
// 📋 Refactoring: Replace switch statements with map lookups
// Authority: 00-core-development-methodology.mdc
// ========================================
//
// **Refactoring Rationale**:
//   - ✅ Maintainability: Adding new enums only requires updating mapping table
//   - ✅ Readability: Intent is clear from table structure
//   - ✅ Performance: Map lookup is O(1) vs switch O(n)
//   - ✅ Testability: Mapping tables can be validated independently
//
// **Before**: 80 lines of switch statements (3 functions)
// **After**: 3 mapping tables + 3 lookup functions (~50 lines)
// **Reduction**: 30 lines saved (38% reduction)
// ========================================

// signalTypeMapping maps string signal types to OpenAPI enum values.
// Used by toGatewayAuditPayloadSignalType() for audit event emission.
//
// **Authority**: api/openapi/data-storage-v1.yaml (GatewayAuditPayload.signal_type enum)
var signalTypeMapping = map[string]api.GatewayAuditPayloadSignalType{
	adapters.SourceTypePrometheusAlert:  api.GatewayAuditPayloadSignalTypePrometheusAlert,
	adapters.SourceTypeKubernetesEvent:  api.GatewayAuditPayloadSignalTypeKubernetesEvent,
}

// severityMapping maps string severity values to OpenAPI enum values.
// Used by toGatewayAuditPayloadSeverity() for audit event emission.
//
// **Authority**: api/openapi/data-storage-v1.yaml (GatewayAuditPayload.severity enum)
//
// **Mappings**:
//   - "warning" → "high" (OpenAPI compatibility)
//   - "info" → "low" (OpenAPI compatibility)
var severityMapping = map[string]api.GatewayAuditPayloadSeverity{
	"critical": api.GatewayAuditPayloadSeverityCritical,
	"high":     api.GatewayAuditPayloadSeverityHigh,
	"warning":  api.GatewayAuditPayloadSeverityHigh, // Map "warning" to "high"
	"medium":   api.GatewayAuditPayloadSeverityMedium,
	"info":     api.GatewayAuditPayloadSeverityLow, // Map "info" to "low"
	"low":      api.GatewayAuditPayloadSeverityLow,
}

// deduplicationStatusMapping maps string deduplication status to OpenAPI enum values.
// Used by toGatewayAuditPayloadDeduplicationStatus() for audit event emission.
//
// **Authority**: api/openapi/data-storage-v1.yaml (GatewayAuditPayload.deduplication_status enum)
var deduplicationStatusMapping = map[string]api.GatewayAuditPayloadDeduplicationStatus{
	"new":       api.GatewayAuditPayloadDeduplicationStatusNew,
	"duplicate": api.GatewayAuditPayloadDeduplicationStatusDuplicate,
}

// componentMapping maps string component names to OpenAPI ErrorDetails.Component enum values.
// Used by toAPIErrorDetails() for error context in audit events.
//
// **Authority**: api/openapi/data-storage-v1.yaml (ErrorDetails.component enum)
var componentMapping = map[string]api.ErrorDetailsComponent{
	"gateway":                 api.ErrorDetailsComponentGateway,
	"aianalysis":              api.ErrorDetailsComponentAianalysis,
	"workflowexecution":       api.ErrorDetailsComponentWorkflowexecution,
	"webhooks":                api.ErrorDetailsComponentWebhooks,
	"remediationorchestrator": api.ErrorDetailsComponentRemediationorchestrator,
	"signalprocessing":        api.ErrorDetailsComponentSignalprocessing,
}

// toGatewayAuditPayloadSignalType converts string to api.GatewayAuditPayloadSignalType enum.
//
// **Refactoring**: Replaced switch statement with map lookup (Phase 3).
//
// **Valid Values**: prometheus-alert, kubernetes-event
// **Returns**: Empty string if value not in mapping (invalid signal type)
func toGatewayAuditPayloadSignalType(value string) api.GatewayAuditPayloadSignalType {
	if mapped, ok := signalTypeMapping[value]; ok {
		return mapped
	}
	return "" // ❌ Invalid signal_type: must be [prometheus-alert, kubernetes-event] per OpenAPI spec
}

// toGatewayAuditPayloadSeverity converts string to api.GatewayAuditPayloadSeverity enum.
//
// **Refactoring**: Replaced switch statement with map lookup (Phase 3).
//
// **Valid Values**: critical, high, warning, medium, info, low
// **Mappings**: warning→high, info→low (OpenAPI compatibility)
// **Returns**: "unknown" if value not in mapping
func toGatewayAuditPayloadSeverity(value string) api.GatewayAuditPayloadSeverity {
	normalized := strings.ToLower(value)
	if mapped, ok := severityMapping[normalized]; ok {
		return mapped
	}
	return api.GatewayAuditPayloadSeverityUnknown
}

// toGatewayAuditPayloadDeduplicationStatus converts string to api.GatewayAuditPayloadDeduplicationStatus enum.
//
// **Refactoring**: Replaced switch statement with map lookup (Phase 3).
//
// **Valid Values**: new, duplicate
// **Returns**: Empty string if value not in mapping
func toGatewayAuditPayloadDeduplicationStatus(value string) api.GatewayAuditPayloadDeduplicationStatus {
	if mapped, ok := deduplicationStatusMapping[value]; ok {
		return mapped
	}
	return "" // Invalid deduplication status
}

// convertMapToJxRaw converts map[string]interface{} to api.GatewayAuditPayloadOriginalPayload (map[string]jx.Raw)
func convertMapToJxRaw(m map[string]interface{}) api.GatewayAuditPayloadOriginalPayload {
	result := make(api.GatewayAuditPayloadOriginalPayload)
	for k, v := range m {
		// Marshal each value to JSON bytes (jx.Raw)
		jsonBytes, _ := json.Marshal(v)
		result[k] = jsonBytes
	}
	return result
}

// toAPIErrorDetails converts sharedaudit.ErrorDetails to api.ErrorDetails.
//
// **Refactoring**: Replaced switch statement with map lookup (Phase 3).
//
// **Component Mapping**: Uses componentMapping table for enum conversion.
// **Returns**: api.ErrorDetails with converted component enum (empty if not mapped)
func toAPIErrorDetails(errorDetails *sharedaudit.ErrorDetails) api.ErrorDetails {
	if errorDetails == nil {
		return api.ErrorDetails{}
	}

	result := api.ErrorDetails{
		Message:       errorDetails.Message,
		Code:          errorDetails.Code,
		RetryPossible: errorDetails.RetryPossible,
	}

	// Convert Component enum using mapping table (refactored from switch)
	if mapped, ok := componentMapping[errorDetails.Component]; ok {
		result.Component = mapped
	}

	// Set StackTrace ([]string, not optional)
	if len(errorDetails.StackTrace) > 0 {
		result.StackTrace = errorDetails.StackTrace
	}

	return result
}
