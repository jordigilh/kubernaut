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

package controller

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// ============================================================================
// TEST ACCESSORS
// These methods expose private reconciler methods for unit testing.
// They follow the existing pattern in the codebase for testable controller logic.
// ============================================================================

// TransitionToCompletedForTest exposes transitionToCompleted for unit tests.
func (r *Reconciler) TransitionToCompletedForTest(ctx context.Context, rr *remediationv1.RemediationRequest, outcome string) (ctrl.Result, error) {
	return r.transitionToCompleted(ctx, rr, outcome)
}

// TransitionToFailedForTest exposes transitionToFailed for unit tests.
func (r *Reconciler) TransitionToFailedForTest(ctx context.Context, rr *remediationv1.RemediationRequest, failurePhase string, failureErr error) (ctrl.Result, error) {
	return r.transitionToFailed(ctx, rr, failurePhase, failureErr)
}

// HandleGlobalTimeoutForTest exposes handleGlobalTimeout for unit tests.
func (r *Reconciler) HandleGlobalTimeoutForTest(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
	return r.handleGlobalTimeout(ctx, rr)
}
