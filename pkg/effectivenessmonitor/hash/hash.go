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
// It computes a deterministic hash of the target resource's spec to detect
// whether the remediation's changes are still in effect (no drift).
//
// Business Requirements:
// - BR-EM-004: Spec hash comparison to detect configuration drift
//
// Hash Algorithm:
//   - SHA-256 of the canonicalized JSON representation of the target resource's spec
//   - Deterministic: same spec always produces same hash
//   - The hash is compared against the pre-remediation hash to detect drift
package hash

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/types"
)

// ComputeResult contains the outcome of a hash computation.
type ComputeResult struct {
	// Hash is the SHA-256 hex string of the target resource spec.
	Hash string
	// Component is the full ComponentResult for audit reporting.
	Component types.ComponentResult
}

// SpecHashInput contains the data needed to compute a spec hash.
type SpecHashInput struct {
	// SpecJSON is the canonicalized JSON representation of the target resource spec.
	SpecJSON []byte
}

// Computer computes deterministic hashes of Kubernetes resource specs.
type Computer interface {
	// Compute calculates the SHA-256 hash of the given spec JSON.
	// Returns the hash string and a ComponentResult.
	Compute(input SpecHashInput) ComputeResult
}

// computer is the concrete implementation of Computer.
type computer struct{}

// NewComputer creates a new spec hash computer.
func NewComputer() Computer {
	return &computer{}
}

// Compute calculates the SHA-256 hash of the given spec JSON.
// The hash is deterministic: same input always produces the same output.
func (c *computer) Compute(input SpecHashInput) ComputeResult {
	data := input.SpecJSON
	if data == nil {
		data = []byte{}
	}

	h := sha256.Sum256(data)
	hashStr := hex.EncodeToString(h[:])

	return ComputeResult{
		Hash: hashStr,
		Component: types.ComponentResult{
			Component: types.ComponentHash,
			Assessed:  true,
			Details:   "spec hash computed: " + hashStr[:16] + "...",
		},
	}
}
