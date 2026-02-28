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

package audit

import (
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// Enum conversion helpers for SignalProcessing audit payloads

func toSignalProcessingAuditPayloadPhase(value string) api.SignalProcessingAuditPayloadPhase {
	switch value {
	case "Pending":
		return api.SignalProcessingAuditPayloadPhasePending
	case "Enriching":
		return api.SignalProcessingAuditPayloadPhaseEnriching
	case "Classifying":
		return api.SignalProcessingAuditPayloadPhaseClassifying
	case "Categorizing":
		return api.SignalProcessingAuditPayloadPhaseCategorizing
	case "Completed":
		return api.SignalProcessingAuditPayloadPhaseCompleted
	case "Failed":
		return api.SignalProcessingAuditPayloadPhaseFailed
	default:
		return ""
	}
}

func toSignalProcessingAuditPayloadSeverity(value string) api.SignalProcessingAuditPayloadSeverity {
	switch value {
	case "critical":
		return api.SignalProcessingAuditPayloadSeverityCritical
	case "high":
		return api.SignalProcessingAuditPayloadSeverityHigh
	case "medium":
		return api.SignalProcessingAuditPayloadSeverityMedium
	case "low":
		return api.SignalProcessingAuditPayloadSeverityLow
	case "unknown":
		return api.SignalProcessingAuditPayloadSeverityUnknown
	default:
		return api.SignalProcessingAuditPayloadSeverityUnknown // DD-SEVERITY-001 v1.1 fallback
	}
}

// DD-SEVERITY-001 v1.1: Converter for normalized severity from Rego policy
func toSignalProcessingAuditPayloadNormalizedSeverity(value string) api.SignalProcessingAuditPayloadNormalizedSeverity {
	switch value {
	case "critical":
		return api.SignalProcessingAuditPayloadNormalizedSeverityCritical
	case "high":
		return api.SignalProcessingAuditPayloadNormalizedSeverityHigh
	case "medium":
		return api.SignalProcessingAuditPayloadNormalizedSeverityMedium
	case "low":
		return api.SignalProcessingAuditPayloadNormalizedSeverityLow
	case "unknown":
		return api.SignalProcessingAuditPayloadNormalizedSeverityUnknown
	default:
		return api.SignalProcessingAuditPayloadNormalizedSeverityUnknown // DD-SEVERITY-001 v1.1 fallback
	}
}

func toSignalProcessingAuditPayloadEnvironment(value string) api.SignalProcessingAuditPayloadEnvironment {
	switch value {
	case "production":
		return api.SignalProcessingAuditPayloadEnvironmentProduction
	case "staging":
		return api.SignalProcessingAuditPayloadEnvironmentStaging
	case "development":
		return api.SignalProcessingAuditPayloadEnvironmentDevelopment
	default:
		return ""
	}
}

func toSignalProcessingAuditPayloadEnvironmentSource(value string) api.SignalProcessingAuditPayloadEnvironmentSource {
	switch value {
	case "rego":
		return api.SignalProcessingAuditPayloadEnvironmentSourceRego
	case "labels":
		return api.SignalProcessingAuditPayloadEnvironmentSourceLabels
	case "default":
		return api.SignalProcessingAuditPayloadEnvironmentSourceDefault
	default:
		return ""
	}
}

func toSignalProcessingAuditPayloadPriority(value string) api.SignalProcessingAuditPayloadPriority {
	switch value {
	case "P0":
		return api.SignalProcessingAuditPayloadPriorityP0
	case "P1":
		return api.SignalProcessingAuditPayloadPriorityP1
	case "P2":
		return api.SignalProcessingAuditPayloadPriorityP2
	case "P3":
		return api.SignalProcessingAuditPayloadPriorityP3
	case "P4":
		return api.SignalProcessingAuditPayloadPriorityP4
	default:
		return ""
	}
}

func toSignalProcessingAuditPayloadPrioritySource(value string) api.SignalProcessingAuditPayloadPrioritySource {
	switch value {
	case "rego":
		return api.SignalProcessingAuditPayloadPrioritySourceRego
	case "severity":
		return api.SignalProcessingAuditPayloadPrioritySourceSeverity
	case "default":
		return api.SignalProcessingAuditPayloadPrioritySourceDefault
	default:
		return ""
	}
}

// BR-SP-106: Signal mode conversion for audit payloads
func toSignalProcessingAuditPayloadSignalMode(value string) api.SignalProcessingAuditPayloadSignalMode {
	switch value {
	case "reactive":
		return api.SignalProcessingAuditPayloadSignalModeReactive
	case "proactive":
		return api.SignalProcessingAuditPayloadSignalModeProactive
	default:
		return api.SignalProcessingAuditPayloadSignalModeReactive // Default to reactive
	}
}

func toSignalProcessingAuditPayloadCriticality(value string) api.SignalProcessingAuditPayloadCriticality {
	switch value {
	case "critical":
		return api.SignalProcessingAuditPayloadCriticalityCritical
	case "high":
		return api.SignalProcessingAuditPayloadCriticalityHigh
	case "medium":
		return api.SignalProcessingAuditPayloadCriticalityMedium
	case "low":
		return api.SignalProcessingAuditPayloadCriticalityLow
	default:
		return ""
	}
}
