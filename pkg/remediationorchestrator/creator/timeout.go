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

package creator

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// computeTimesOutAt converts RO's authoritative per-phase timeout duration
// (from RemediationRequest.Status.TimeoutConfig) into an absolute deadline
// (DD-TIMEOUT-002 / Issue #2176), shared by the SignalProcessing, AIAnalysis,
// and WorkflowExecution creators.
//
// An absolute timestamp (rather than a relative duration) is propagated so
// the child controller can self-enforce it without any clock-skew ambiguity
// between RO's reconcile loop (where the duration is known) and the child
// controller's own reconcile loop (where the deadline must be checked,
// potentially much later). Returns nil when no positive duration is
// configured, in which case the child CRD relies on its own configured
// default and/or RO's outer backstop.
func computeTimesOutAt(d *metav1.Duration) *metav1.Time {
	if d == nil || d.Duration <= 0 {
		return nil
	}
	deadline := metav1.NewTime(metav1.Now().Add(d.Duration))
	return &deadline
}
