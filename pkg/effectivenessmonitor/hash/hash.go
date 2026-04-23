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

// Package hash provides the spec hash computation and comparison for the Effectiveness Monitor.
// It computes a deterministic canonical hash of the target resource's spec to detect
// whether the remediation's changes are still in effect (no drift).
//
// Business Requirements:
// - BR-EM-004: Spec hash comparison to detect configuration drift
//
// Hash Algorithm (DD-EM-002):
//   - Uses pkg/shared/hash.CanonicalResourceFingerprint for cross-service consistency
//   - Both the RO (pre-remediation) and EM (post-remediation) use the same algorithm
//   - Deterministic, map-order independent, slice-order independent
//   - Returns "sha256:<lowercase-hex>" format
//
// Pre/Post Comparison:
//   - When a pre-remediation hash is available (from DS audit trail), the Computer
//     compares it with the post-remediation hash and sets the Match field
//   - Match=true means no spec change (possible drift-back or no-op remediation)
//   - Match=false means spec changed (expected for successful remediations)
//   - Match=nil means no pre-hash available (comparison not possible)
package hash

import (
	"fmt"

	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/types"
	canonicalhash "github.com/jordigilh/kubernaut/pkg/shared/hash"
)

// ComputeResult contains the outcome of a hash computation and optional comparison.
type ComputeResult struct {
	// Hash is the post-remediation canonical hash ("sha256:<64-char-hex>").
	Hash string
	// PreHash is the pre-remediation hash from the DS audit trail.
	// Empty string if not available.
	PreHash string
	// Match indicates whether pre and post hashes are identical.
	// nil if no PreHash was provided (comparison not possible).
	// true if hashes match (no spec change detected).
	// false if hashes differ (spec changed, expected for successful remediations).
	Match *bool
	// Component is the full ComponentResult for audit reporting.
	Component types.ComponentResult
}

// SpecHashInput contains the data needed to compute a resource fingerprint.
type SpecHashInput struct {
	// Spec is the target resource's full obj.Object (for CanonicalResourceFingerprint).
	// When FunctionalState is non-nil, it takes precedence over Spec.
	Spec map[string]interface{}
	// FunctionalState is the full K8s object (obj.Object) for CanonicalResourceFingerprint (#765).
	// When set, the Computer uses CanonicalResourceFingerprint on the full object.
	FunctionalState map[string]interface{}
	// PreHash is the pre-remediation spec hash from the DS audit trail.
	// Format: "sha256:<hex>". Empty string if not available.
	// When provided, the Computer will compare pre and post hashes.
	PreHash string
	// ConfigMapHashes is a map of ConfigMap name -> content hash ("sha256:<hex>").
	// Pre-computed by the caller using pkg/shared/hash.ConfigMapDataHash.
	// When non-empty, the Computer produces a composite hash that incorporates
	// both the fingerprint and the ConfigMap content hashes (#396, BR-EM-004).
	// When nil or empty, the Computer falls back to fingerprint-only hash.
	ConfigMapHashes map[string]string
}

// Computer computes deterministic canonical hashes of Kubernetes resource specs
// and optionally compares them with pre-remediation hashes.
type Computer interface {
	// Compute calculates the canonical SHA-256 hash of the given spec map
	// and optionally compares it with the pre-remediation hash.
	Compute(input SpecHashInput) ComputeResult
}

// computer is the concrete implementation of Computer.
type computer struct{}

// NewComputer creates a new spec hash computer.
func NewComputer() Computer {
	return &computer{}
}

// Compute calculates the canonical resource fingerprint using the DD-EM-002 v2.0 algorithm (#765).
//
// If FunctionalState is provided, uses CanonicalResourceFingerprint on the full object.
// Otherwise falls back to hashing Spec directly for backward compatibility.
//
// If PreHash is provided in the input, it compares the two hashes and sets
// the Match field in the result.
func (c *computer) Compute(input SpecHashInput) ComputeResult {
	var fingerprint string
	var err error

	if input.FunctionalState != nil {
		fingerprint, err = canonicalhash.CanonicalResourceFingerprint(input.FunctionalState)
	} else {
		spec := input.Spec
		if spec == nil {
			spec = map[string]interface{}{}
		}
		fingerprint, err = canonicalhash.CanonicalResourceFingerprint(spec)
	}
	if err != nil {
		return ComputeResult{
			Component: types.ComponentResult{
				Component: types.ComponentHash,
				Assessed:  false,
				Error:     fmt.Errorf("canonical hash computation failed: %w", err),
				Details:   "failed to compute resource fingerprint: " + err.Error(),
			},
		}
	}

	postHash, err := canonicalhash.CompositeResourceFingerprint(fingerprint, input.ConfigMapHashes)
	if err != nil {
		return ComputeResult{
			Component: types.ComponentResult{
				Component: types.ComponentHash,
				Assessed:  false,
				Error:     fmt.Errorf("composite hash computation failed: %w", err),
				Details:   "failed to compute composite fingerprint: " + err.Error(),
			},
		}
	}

	result := ComputeResult{
		Hash:    postHash,
		PreHash: input.PreHash,
		Component: types.ComponentResult{
			Component: types.ComponentHash,
			Assessed:  true,
		},
	}

	// Compare with pre-remediation hash if available
	if input.PreHash != "" {
		match := postHash == input.PreHash
		result.Match = &match
		if match {
			result.Component.Details = fmt.Sprintf("spec hash computed: %s (matches pre-remediation hash — no drift)", postHash[:23]+"...")
		} else {
			result.Component.Details = fmt.Sprintf("spec hash computed: %s (differs from pre-remediation — spec changed)", postHash[:23]+"...")
		}
	} else {
		result.Component.Details = fmt.Sprintf("spec hash computed: %s (no pre-remediation hash for comparison)", postHash[:23]+"...")
	}

	return result
}
