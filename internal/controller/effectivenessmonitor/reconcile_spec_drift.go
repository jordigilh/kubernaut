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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/conditions"
	canonicalhash "github.com/jordigilh/kubernaut/pkg/shared/hash"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
)

// handleSpecDrift implements Step 6.5: Spec Drift Guard (DD-EM-002 v1.1).
//
// After the post-remediation hash is computed (HashComputed=true), re-hash
// the target resource spec on every reconcile. If it differs, the resource
// was modified (likely by another remediation) and the assessment is invalid.
// Returns (result, true, err) when spec drift is detected and assessment completes.
func (r *Reconciler) handleSpecDrift(ctx context.Context, rctx *reconcileContext) (ctrl.Result, bool, error) {
	ea := rctx.ea

	if !ea.Status.Components.HashComputed || ea.Status.Components.PostRemediationSpecHash == "" {
		return ctrl.Result{}, false, nil
	}

	logger := log.FromContext(ctx)

	functionalState, spec, _ := r.getTargetFunctionalState(ctx, ea.Spec.RemediationTarget)
	fingerprint, fpErr := canonicalhash.CanonicalResourceFingerprint(functionalState)
	if fpErr != nil {
		logger.Error(fpErr, "Failed to compute current resource fingerprint for drift check")
		return ctrl.Result{}, false, nil
	}

	driftConfigMapHashes := r.resolveConfigMapHashes(ctx, spec, ea.Spec.RemediationTarget)
	if len(driftConfigMapHashes) > 0 {
		logger.V(2).Info("Drift guard resolved ConfigMap hashes",
			"configMapCount", len(driftConfigMapHashes),
			"correlationID", ea.Spec.CorrelationID)
	}

	currentHash, compositeErr := canonicalhash.CompositeResourceFingerprint(fingerprint, driftConfigMapHashes)
	if compositeErr != nil {
		logger.Error(compositeErr, "Failed to compute composite fingerprint for drift check")
		return ctrl.Result{}, false, nil
	}

	ea.Status.Components.CurrentSpecHash = currentHash

	if currentHash != ea.Status.Components.PostRemediationSpecHash {
		logger.Info("Spec drift detected — resource modified since post-remediation hash (DD-EM-002 v1.1)",
			"postRemediationHash", ea.Status.Components.PostRemediationSpecHash,
			"currentHash", currentHash,
			"correlationID", ea.Spec.CorrelationID,
		)

		conditions.SetCondition(ea, conditions.ConditionSpecIntegrity,
			metav1.ConditionFalse, conditions.ReasonSpecDrifted,
			fmt.Sprintf("Target spec hash changed: %s -> %s",
				ea.Status.Components.PostRemediationSpecHash, currentHash))

		conditions.SetCondition(ea, conditions.ConditionAssessmentComplete,
			metav1.ConditionTrue, conditions.ReasonSpecDrift,
			"Assessment invalidated: target resource spec was modified (spec drift detected)")

		r.Recorder.Event(ea, corev1.EventTypeWarning, events.EventReasonSpecDriftDetected,
			fmt.Sprintf("Target resource spec modified during assessment (correlation: %s)", ea.Spec.CorrelationID))

		result, err := r.completeAssessmentWithReason(ctx, ea, eav1.AssessmentReasonSpecDrift)
		return result, true, err
	}

	conditions.SetCondition(ea, conditions.ConditionSpecIntegrity,
		metav1.ConditionTrue, conditions.ReasonSpecUnchanged,
		"Target resource spec unchanged since post-remediation hash")

	return ctrl.Result{}, false, nil
}
