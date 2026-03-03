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

// Package timing provides pure functions for computing derived timing fields
// in the EffectivenessAssessment lifecycle.
//
// These functions are extracted from the EM reconciler to enable unit testing
// of timing logic independently from K8s API interactions.
//
// Business Requirements:
// - BR-EM-009: Derived timing computation (ValidityDeadline, CheckAfter)
// - BR-EM-010.4: Stabilization anchored to HashComputeAfter (#253)
package timing

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DerivedTiming holds the computed timing fields for an EffectivenessAssessment.
// These are persisted in EA.Status on first reconciliation so operators can
// observe the assessment timeline immediately.
type DerivedTiming struct {
	// CheckAfter is when Prometheus/AlertManager checks should begin.
	// For sync targets: EA.creationTimestamp + StabilizationWindow.
	// For async targets (#253): HashComputeAfter + StabilizationWindow.
	CheckAfter metav1.Time

	// ValidityDeadline is when the assessment expires.
	// Includes the runtime guard: if StabilizationWindow >= ValidityWindow,
	// the effective validity is extended to StabilizationWindow + ValidityWindow.
	ValidityDeadline metav1.Time

	// EffectiveValidity is the computed validity duration (may be extended).
	EffectiveValidity time.Duration

	// Extended is true when the runtime guard extended the validity window
	// because StabilizationWindow >= ValidityWindow.
	Extended bool
}

// ComputeDerivedTiming calculates the derived timing fields for an EA.
//
// For sync targets (hashComputeAfter nil or zero):
//
//	The runtime guard (Issue #188) ensures that when StabilizationWindow >= ValidityWindow,
//	the effective validity is extended to StabilizationWindow + ValidityWindow.
//	CheckAfter = creationTimestamp + StabilizationWindow
//	ValidityDeadline = creationTimestamp + effectiveValidity
//
// For async targets (hashComputeAfter non-nil, non-zero — Issue #253, DD-EM-004 v2.0):
//
//	The stabilization anchor shifts to HashComputeAfter (propagation complete).
//	effectiveValidity is always StabilizationWindow + ValidityWindow because the full
//	assessment window must be available after propagation + stabilization.
//	CheckAfter = HashComputeAfter + StabilizationWindow
//	ValidityDeadline = HashComputeAfter + effectiveValidity
//
// Parameters:
//   - creationTimestamp: EA.metadata.creationTimestamp
//   - stabilizationWindow: EA.Spec.Config.StabilizationWindow.Duration
//   - validityWindow: ReconcilerConfig.ValidityWindow (EM-level config)
//   - hashComputeAfter: EA.Spec.HashComputeAfter (nil for sync targets)
func ComputeDerivedTiming(creationTimestamp metav1.Time, stabilizationWindow, validityWindow time.Duration, hashComputeAfter *metav1.Time) DerivedTiming {
	if hashComputeAfter != nil && !hashComputeAfter.IsZero() {
		effectiveValidity := stabilizationWindow + validityWindow
		return DerivedTiming{
			CheckAfter:        metav1.NewTime(hashComputeAfter.Add(stabilizationWindow)),
			ValidityDeadline:  metav1.NewTime(hashComputeAfter.Add(effectiveValidity)),
			EffectiveValidity: effectiveValidity,
			Extended:          true,
		}
	}

	effectiveValidity := validityWindow
	extended := false
	if stabilizationWindow >= effectiveValidity {
		effectiveValidity = stabilizationWindow + validityWindow
		extended = true
	}
	return DerivedTiming{
		CheckAfter:        metav1.NewTime(creationTimestamp.Time.Add(stabilizationWindow)),
		ValidityDeadline:  metav1.NewTime(creationTimestamp.Time.Add(effectiveValidity)),
		EffectiveValidity: effectiveValidity,
		Extended:          extended,
	}
}
