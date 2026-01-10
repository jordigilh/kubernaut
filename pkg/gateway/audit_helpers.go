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

	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/shared/audit"
)

// toGatewayAuditPayloadSignalType converts string to api.GatewayAuditPayloadSignalType enum
func toGatewayAuditPayloadSignalType(value string) api.GatewayAuditPayloadSignalType {
	switch value {
	case adapters.SourceTypePrometheusAlert:
		return api.GatewayAuditPayloadSignalTypePrometheusAlert
	case adapters.SourceTypeKubernetesEvent:
		return api.GatewayAuditPayloadSignalTypeKubernetesEvent
	default:
		return "" // ❌ Invalid signal_type: must be [prometheus-alert, kubernetes-event] per OpenAPI spec
	}
}

// toGatewayAuditPayloadSeverity converts string to api.GatewayAuditPayloadSeverity enum
func toGatewayAuditPayloadSeverity(value string) api.GatewayAuditPayloadSeverity {
	switch value {
	case "critical":
		return api.GatewayAuditPayloadSeverityCritical
	case "warning":
		return api.GatewayAuditPayloadSeverityWarning
	case "info":
		return api.GatewayAuditPayloadSeverityInfo
	default:
		return "" // Or handle error
	}
}

// toGatewayAuditPayloadDeduplicationStatus converts string to api.GatewayAuditPayloadDeduplicationStatus enum
func toGatewayAuditPayloadDeduplicationStatus(value string) api.GatewayAuditPayloadDeduplicationStatus {
	switch value {
	case "new":
		return api.GatewayAuditPayloadDeduplicationStatusNew
	case "duplicate":
		return api.GatewayAuditPayloadDeduplicationStatusDuplicate
	default:
		return "" // Or handle error
	}
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

// toAPIErrorDetails converts sharedaudit.ErrorDetails to api.ErrorDetails
func toAPIErrorDetails(errorDetails *sharedaudit.ErrorDetails) api.ErrorDetails {
	if errorDetails == nil {
		return api.ErrorDetails{}
	}

	ogenErrorDetails := api.ErrorDetails{
		Message:       errorDetails.Message,
		Code:          errorDetails.Code,
		RetryPossible: errorDetails.RetryPossible,
	}

	// Convert Component enum
	switch errorDetails.Component {
	case "gateway":
		ogenErrorDetails.Component = api.ErrorDetailsComponentGateway
	case "aianalysis":
		ogenErrorDetails.Component = api.ErrorDetailsComponentAianalysis
	case "workflowexecution":
		ogenErrorDetails.Component = api.ErrorDetailsComponentWorkflowexecution
	case "webhooks":
		ogenErrorDetails.Component = api.ErrorDetailsComponentWebhooks
	case "remediationorchestrator":
		ogenErrorDetails.Component = api.ErrorDetailsComponentRemediationorchestrator
	case "signalprocessing":
		ogenErrorDetails.Component = api.ErrorDetailsComponentSignalprocessing
	}

	// Set StackTrace ([]string, not optional)
	if len(errorDetails.StackTrace) > 0 {
		ogenErrorDetails.StackTrace = errorDetails.StackTrace
	}

	return ogenErrorDetails
}
