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

// Package evaluator provides a unified OPA Rego evaluator for SignalProcessing.
//
// ADR-060: Consolidates 5 separate Rego classifiers (environment, severity,
// priority, custom labels, business) into a single policy.rego evaluated by
// one evaluator with per-rule query methods.
//
// # Business Requirements
//
// BR-SP-051: Environment classification
// BR-SP-070: Priority assignment
// BR-SP-102: CustomLabels extraction
// BR-SP-104: Mandatory label protection
// BR-SP-105: Severity determination
//
// # Design Decisions
//
// ADR-060: Unified SignalProcessing Rego Policy
// DD-WORKFLOW-001 v1.9: Sandbox configuration and validation limits
package evaluator

import (
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// PolicyInput is the typed contract between Go and Rego.
// OPA serializes via JSON tags, producing input paths like
// input.namespace.name, input.signal.severity, input.workload.kind.
//
// Reuses shared types from pkg/shared/types/enrichment.go for
// namespace and workload context.
type PolicyInput struct {
	Namespace sharedtypes.NamespaceContext `json:"namespace"`
	Signal    SignalInput                  `json:"signal"`
	Workload  sharedtypes.WorkloadDetails  `json:"workload"`
}

// SignalInput contains signal-specific fields for Rego evaluation.
type SignalInput struct {
	Severity string            `json:"severity"`
	Type     string            `json:"type"`
	Source   string            `json:"source"`
	Labels   map[string]string `json:"labels,omitempty"`
}

// SeverityResult contains the determined severity and source attribution.
// DD-SEVERITY-001: Aligned with HAPI/workflow catalog.
type SeverityResult struct {
	Severity   string
	Source     string
	PolicyHash string
}
