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

package alert

import (
	"time"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
)

// AlertDeferralResult contains the outcome of an alert deferral check (#277).
type AlertDeferralResult struct {
	// ShouldDefer is true when the alert resolution check must be postponed.
	ShouldDefer bool
	// RequeueAfter is the duration to wait before retrying. Zero when no deferral needed.
	RequeueAfter time.Duration
}

// CheckAlertDeferral evaluates whether alert assessment should be deferred for
// an EffectivenessAssessment based on the AlertManagerCheckAfter status field.
//
// Business behavior (BR-EM-009, Issue #277):
//   - When AlertManagerCheckAfter is nil: check immediately (no alert-specific delay)
//   - When AlertManagerCheckAfter is in the past: check immediately
//   - When AlertManagerCheckAfter is in the future: defer and requeue after remaining duration
//
// Unlike hash deferral (which blocks all checks), alert deferral only blocks the
// alert resolution check — health and metrics assessments proceed independently.
func CheckAlertDeferral(ea *eav1.EffectivenessAssessment) AlertDeferralResult {
	if ea.Status.AlertManagerCheckAfter == nil {
		return AlertDeferralResult{ShouldDefer: false}
	}

	remaining := time.Until(ea.Status.AlertManagerCheckAfter.Time)
	if remaining <= 0 {
		return AlertDeferralResult{ShouldDefer: false}
	}

	return AlertDeferralResult{
		ShouldDefer:  true,
		RequeueAfter: remaining,
	}
}
