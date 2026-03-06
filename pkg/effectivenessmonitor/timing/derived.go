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
// - BR-EM-009: Derived timing computation (ValidityDeadline, CheckAfter, AlertCheckAfter)
// - BR-EM-010.4: Stabilization anchored to HashComputeDelay for async targets (#253)
// - Issue #277: AlertCheckDelay additive semantics, Duration-based HashComputeDelay
package timing

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DerivedTiming holds the computed timing fields for an EffectivenessAssessment.
// These are persisted in EA.Status on first reconciliation so operators can
// observe the assessment timeline immediately.
type DerivedTiming struct {
	// CheckAfter is when Prometheus metrics checks should begin.
	// For sync targets: creationTimestamp + StabilizationWindow.
	// For async targets (#253): creationTimestamp + HashComputeDelay + StabilizationWindow.
	CheckAfter metav1.Time

	// AlertCheckAfter is when AlertManager alert resolution checks should begin.
	// Equals CheckAfter when AlertCheckDelay is nil.
	// When AlertCheckDelay is set: CheckAfter + AlertCheckDelay (additive, #277).
	AlertCheckAfter metav1.Time

	// ValidityDeadline is when the assessment expires.
	// The runtime guard ensures the deadline extends to cover all checks:
	// if the latest check offset (stab + alertCheckDelay) >= ValidityWindow,
	// the effective validity is extended accordingly.
	ValidityDeadline metav1.Time

	// EffectiveValidity is the computed validity duration (may be extended).
	EffectiveValidity time.Duration

	// Extended is true when the runtime guard extended the validity window.
	Extended bool
}

// ComputeDerivedTiming calculates the derived timing fields for an EA.
//
// For sync targets (hashComputeDelay nil or zero):
//
//	CheckAfter = creationTimestamp + StabilizationWindow
//	AlertCheckAfter = CheckAfter + AlertCheckDelay (or == CheckAfter if nil)
//	ValidityDeadline = creationTimestamp + effectiveValidity
//	Runtime guard extends validity when totalCheckOffset >= ValidityWindow.
//
// For async targets (hashComputeDelay non-nil, non-zero — Issue #253, #277):
//
//	anchor = creationTimestamp + HashComputeDelay
//	CheckAfter = anchor + StabilizationWindow
//	AlertCheckAfter = CheckAfter + AlertCheckDelay (or == CheckAfter if nil)
//	effectiveValidity = stab + alertCheckDelay + validity (always extended from anchor)
//	ValidityDeadline = anchor + effectiveValidity
//
// Parameters:
//   - creationTimestamp: EA.metadata.creationTimestamp
//   - stabilizationWindow: EA.Spec.Config.StabilizationWindow.Duration
//   - validityWindow: ReconcilerConfig.ValidityWindow (EM-level config)
//   - hashComputeDelay: EA.Spec.Config.HashComputeDelay (nil for sync targets)
//   - alertCheckDelay: EA.Spec.Config.AlertCheckDelay (nil when not proactive)
func ComputeDerivedTiming(creationTimestamp metav1.Time, stabilizationWindow, validityWindow time.Duration, hashComputeDelay, alertCheckDelay *metav1.Duration) DerivedTiming {
	alertDelay := time.Duration(0)
	if alertCheckDelay != nil {
		alertDelay = alertCheckDelay.Duration
	}

	if hashComputeDelay != nil && hashComputeDelay.Duration > 0 {
		anchor := creationTimestamp.Time.Add(hashComputeDelay.Duration)
		effectiveValidity := stabilizationWindow + alertDelay + validityWindow
		checkAfter := metav1.NewTime(anchor.Add(stabilizationWindow))
		alertCheckAfterTime := checkAfter
		if alertDelay > 0 {
			alertCheckAfterTime = metav1.NewTime(checkAfter.Time.Add(alertDelay))
		}
		return DerivedTiming{
			CheckAfter:        checkAfter,
			AlertCheckAfter:   alertCheckAfterTime,
			ValidityDeadline:  metav1.NewTime(anchor.Add(effectiveValidity)),
			EffectiveValidity: effectiveValidity,
			Extended:          true,
		}
	}

	totalCheckOffset := stabilizationWindow + alertDelay
	effectiveValidity := validityWindow
	extended := false
	if totalCheckOffset >= effectiveValidity {
		effectiveValidity = totalCheckOffset + validityWindow
		extended = true
	}

	checkAfter := metav1.NewTime(creationTimestamp.Time.Add(stabilizationWindow))
	alertCheckAfterTime := checkAfter
	if alertDelay > 0 {
		alertCheckAfterTime = metav1.NewTime(checkAfter.Time.Add(alertDelay))
	}

	return DerivedTiming{
		CheckAfter:        checkAfter,
		AlertCheckAfter:   alertCheckAfterTime,
		ValidityDeadline:  metav1.NewTime(creationTimestamp.Time.Add(effectiveValidity)),
		EffectiveValidity: effectiveValidity,
		Extended:          extended,
	}
}
