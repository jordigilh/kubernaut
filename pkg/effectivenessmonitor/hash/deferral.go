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
// an EffectivenessAssessment based on the HashComputeAfter timestamp.
//
// Business behavior (BR-EM-010.1, DD-EM-004):
//   - When HashComputeAfter is nil: compute hash immediately (backward compatible)
//   - When HashComputeAfter is in the past: compute hash immediately
//   - When HashComputeAfter is in the future: defer and requeue after the remaining duration
//
// The RO sets HashComputeAfter for async-managed targets (GitOps, operator CRDs)
// so the EM captures the post-remediation spec after the external controller reconciles.
func CheckHashDeferral(ea *eav1.EffectivenessAssessment) DeferralResult {
	if ea.Spec.HashComputeAfter == nil || ea.Spec.HashComputeAfter.IsZero() {
		return DeferralResult{ShouldDefer: false}
	}

	remaining := time.Until(ea.Spec.HashComputeAfter.Time)
	if remaining <= 0 {
		return DeferralResult{ShouldDefer: false}
	}

	return DeferralResult{
		ShouldDefer:  true,
		RequeueAfter: remaining,
	}
}
