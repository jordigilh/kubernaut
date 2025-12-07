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

// Package rego provides CustomLabels extraction via sandboxed OPA Rego policies.
//
// # Business Requirements
//
// BR-SP-102: CustomLabels Rego Extraction
// BR-SP-104: Mandatory Label Protection
//
// # Design Decisions
//
// DD-WORKFLOW-001 v1.9: Validation limits and sandbox requirements
//
// # Sandbox Configuration
//
//   - Evaluation timeout: 5 seconds
//   - Memory limit: 128 MB
//   - Network access: Disabled
//   - Filesystem access: Disabled
package rego

import (
	"context"

	"github.com/go-logr/logr"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Engine evaluates customer Rego policies for CustomLabels.
// BR-SP-102: CustomLabels Rego Extraction
// BR-SP-104: Mandatory Label Protection (via security wrapper)
// DD-WORKFLOW-001 v1.9: Sandboxed OPA Runtime
type Engine struct {
	logger     logr.Logger
	policyPath string
}

// NewEngine creates a new CustomLabels Rego engine.
// Per BR-SP-102: Extract customer labels via sandboxed OPA policies.
func NewEngine(logger logr.Logger, policyPath string) *Engine {
	return &Engine{
		logger:     logger.WithName("rego"),
		policyPath: policyPath,
	}
}

// RegoInput wraps shared types for Rego policy evaluation.
// Uses sharedtypes.KubernetesContext (authoritative source).
type RegoInput struct {
	Kubernetes     *sharedtypes.KubernetesContext `json:"kubernetes"`
	Signal         SignalContext                  `json:"signal"`
	DetectedLabels *sharedtypes.DetectedLabels    `json:"detected_labels,omitempty"`
}

// SignalContext contains signal-specific data for Rego policies.
type SignalContext struct {
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Source   string `json:"source"`
}

// LoadPolicy loads customer policy from string and wraps with security policy.
// RED PHASE STUB: Returns nil (always succeeds, but policy not applied)
func (e *Engine) LoadPolicy(policyContent string) error {
	// RED PHASE: Stub - policy not stored
	return nil
}

// EvaluatePolicy evaluates the policy and returns CustomLabels.
// Output format: map[string][]string (subdomain → list of values)
// RED PHASE STUB: Returns empty map (tests will fail)
func (e *Engine) EvaluatePolicy(ctx context.Context, input *RegoInput) (map[string][]string, error) {
	// RED PHASE: Stub - returns empty map, tests will fail
	return make(map[string][]string), nil
}

