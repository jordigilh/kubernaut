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

package hash

import (
	"time"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
)

// DeferralResult contains the outcome of a hash deferral check (DD-EM-004).
type DeferralResult struct {
	// ShouldDefer is true when hash computation must be postponed.
	ShouldDefer bool
	// RequeueAfter is the duration to wait before retrying. Zero when no deferral needed.
	RequeueAfter time.Duration
}

// CheckHashDeferral evaluates whether hash computation should be deferred for
// an EffectivenessAssessment based on the HashCheckDelay duration in EAConfig.
//
// Business behavior (BR-EM-010.1, DD-EM-004, Issue #277):
//   - When HashCheckDelay is nil or zero: compute hash immediately (backward compatible)
//   - When creation + HashCheckDelay is in the past: compute hash immediately
//   - When creation + HashCheckDelay is in the future: defer and requeue after remaining duration
//
// The RO sets HashCheckDelay for async-managed targets (GitOps, operator CRDs)
// so the EM captures the post-remediation spec after the external controller reconciles.
func CheckHashDeferral(ea *eav1.EffectivenessAssessment) DeferralResult {
	if ea.Spec.Config.HashCheckDelay == nil || ea.Spec.Config.HashCheckDelay.Duration <= 0 {
		return DeferralResult{ShouldDefer: false}
	}

	deadline := ea.CreationTimestamp.Time.Add(ea.Spec.Config.HashCheckDelay.Duration)
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return DeferralResult{ShouldDefer: false}
	}

	return DeferralResult{
		ShouldDefer:  true,
		RequeueAfter: remaining,
	}
}
