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
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// finalizeReconcile handles Steps 8-10: completion check, atomic status update,
// post-update events, and requeue for remaining components.
func (r *Reconciler) finalizeReconcile(ctx context.Context, rctx *reconcileContext) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	ea := rctx.ea

	// Step 8: Check if all components are done and prepare completion fields in-memory.
	completing := false
	if r.allComponentsDone(ea) {
		completing = true
		reason := r.determineAssessmentReason(ea)
		r.setCompletionFields(ea, reason)
		logger.Info("All components done, preparing completion",
			"reason", reason, "componentsChanged", rctx.componentsChanged,
			"pendingTransition", rctx.pendingTransition,
		)
	}

	// Step 9: Single atomic status update (phase transition + timing + components + completion).
	if rctx.componentsChanged || rctx.pendingTransition || completing {
		if err := r.Status().Update(ctx, ea); err != nil {
			logger.Error(err, "Failed to update EA status",
				"phase", ea.Status.Phase, "completing", completing,
				"resourceVersion", ea.ResourceVersion,
			)
			return ctrl.Result{RequeueAfter: r.Config.RequeueGenericError}, err
		}

		if rctx.pendingTransition {
			r.emitAssessingTransitionEvents(ctx, ea)
		}
		if completing {
			r.emitCompletionMetricsAndEvents(ctx, ea, ea.Status.AssessmentReason)
			logger.Info("Assessment completed",
				"reason", ea.Status.AssessmentReason,
				"correlationID", ea.Spec.CorrelationID,
			)
			return ctrl.Result{}, nil
		}
	}

	// Step 10: Requeue for remaining components (BR-EM-007, Issue #591).
	return ctrl.Result{RequeueAfter: r.capRequeueAtDeadline(ea, r.Config.RequeueAssessmentInProgress)}, nil
}
