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

package creator

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
)

// EffectivenessAssessmentCreator creates EffectivenessAssessment CRDs for the Remediation Orchestrator.
// Reference: ADR-EM-001 (Effectiveness Monitor Service Integration)
// The RO creates an EA CRD when a RemediationRequest reaches a terminal phase (Completed).
// The EA spec contains only StabilizationWindow (set by the RO from its config).
// All other assessment parameters (PrometheusEnabled, AlertManagerEnabled, ValidityWindow)
// are EM-internal config read from effectivenessmonitor.Config.
type EffectivenessAssessmentCreator struct {
	client              client.Client
	scheme              *runtime.Scheme
	metrics             *metrics.Metrics
	recorder            record.EventRecorder
	stabilizationWindow time.Duration
}

// NewEffectivenessAssessmentCreator creates a new EffectivenessAssessmentCreator.
// The stabilizationWindow parameter comes from the RO's EACreationConfig.
// The recorder parameter is used to emit K8s events on EA creation (DD-EVENT-001).
func NewEffectivenessAssessmentCreator(c client.Client, s *runtime.Scheme, m *metrics.Metrics, recorder record.EventRecorder, stabilizationWindow time.Duration) *EffectivenessAssessmentCreator {
	// DD-METRICS-001: Metrics are REQUIRED (dependency injection pattern)
	if m == nil {
		panic("DD-METRICS-001 violation: EffectivenessAssessmentCreator requires non-nil metrics (authoritative mandate)")
	}
	return &EffectivenessAssessmentCreator{
		client:              c,
		scheme:              s,
		metrics:             m,
		recorder:            recorder,
		stabilizationWindow: stabilizationWindow,
	}
}

// DualTarget carries both the signal-sourced and remediation-sourced targets for
// effectiveness assessment (DD-EM-003).
//
// Signal: The resource that triggered the alert (from RR.Spec.TargetResource).
// Remediation: The resource the workflow actually modified (from AA.Status.RootCauseAnalysis.AffectedResource).
// These may differ (e.g., alert on Deployment, workflow patches HPA).
type DualTarget struct {
	Signal      eav1.TargetResource
	Remediation eav1.TargetResource
}

// CreateEffectivenessAssessment creates an EffectivenessAssessment CRD for a completed remediation.
// ADR-EM-001: The EA is created with:
//   - CorrelationID: RR.Name (used for audit trail correlation)
//   - SignalTarget: from dualTarget.Signal (signal-sourced resource)
//   - RemediationTarget: from dualTarget.Remediation (AI-identified resource)
//   - Config.StabilizationWindow: from RO's EACreationConfig
//   - RemediationRequestPhase: RR.Status.OverallPhase at creation time (immutable spec field)
//   - OwnerReference: RR (for cascade deletion, BR-ORCH-031)
//
// The dualTarget parameter is optional. When non-nil, it provides both signal and remediation
// targets (DD-EM-003). When nil, falls back to RR.Spec.TargetResource for both.
//
// Returns the EA name if created (or already exists), or an error.
func (c *EffectivenessAssessmentCreator) CreateEffectivenessAssessment(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	dualTarget *DualTarget,
) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
	)

	// Generate deterministic name (one EA per RR)
	name := fmt.Sprintf("ea-%s", rr.Name)

	// Check if already exists (idempotency)
	existing := &eav1.EffectivenessAssessment{}
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, existing)
	if err == nil {
		logger.Info("EffectivenessAssessment already exists, reusing", "name", name)
		return name, nil
	}
	if !apierrors.IsNotFound(err) {
		logger.Error(err, "Failed to check existing EffectivenessAssessment")
		return "", fmt.Errorf("failed to check existing EffectivenessAssessment: %w", err)
	}

	// DD-EM-003: Resolve signal and remediation targets.
	signalTarget := eav1.TargetResource{
		Kind:      rr.Spec.TargetResource.Kind,
		Name:      rr.Spec.TargetResource.Name,
		Namespace: rr.Spec.TargetResource.Namespace,
	}
	remediationTarget := signalTarget
	if dualTarget != nil {
		signalTarget = dualTarget.Signal
		remediationTarget = dualTarget.Remediation
	}

	// Build EffectivenessAssessment CRD
	rrCreatedAt := rr.CreationTimestamp.DeepCopy()
	ea := &eav1.EffectivenessAssessment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
		},
		Spec: eav1.EffectivenessAssessmentSpec{
			CorrelationID:           rr.Name,
			RemediationRequestPhase: string(rr.Status.OverallPhase),
			SignalTarget:            signalTarget,
			RemediationTarget:       remediationTarget,
			Config: eav1.EAConfig{
				StabilizationWindow: metav1.Duration{Duration: c.stabilizationWindow},
			},
			RemediationCreatedAt:   rrCreatedAt,
			SignalName:             rr.Spec.SignalName,             // OBS-1: Propagate actual alert name for audit
			PreRemediationSpecHash: rr.Status.PreRemediationSpecHash, // DD-EM-002: Propagate from RR status
		},
	}

	// Validate RemediationRequest has required metadata for owner reference (defensive programming)
	// Gap 2.1: Prevents orphaned child CRDs if RR not properly persisted
	if rr.UID == "" {
		logger.Error(nil, "RemediationRequest has empty UID, cannot set owner reference")
		return "", fmt.Errorf("failed to set owner reference: RemediationRequest UID is required but empty")
	}

	// Set owner reference for cascade deletion (BR-ORCH-031)
	if err := controllerutil.SetControllerReference(rr, ea, c.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference")
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}
	// ADR-EM-001 Section 8: blockOwnerDeletion must be false to prevent
	// RR deletion from blocking on EA finalizers. SetControllerReference
	// defaults to true; override it here.
	for i := range ea.OwnerReferences {
		if ea.OwnerReferences[i].Controller != nil && *ea.OwnerReferences[i].Controller {
			blockOwnerDeletion := false
			ea.OwnerReferences[i].BlockOwnerDeletion = &blockOwnerDeletion
		}
	}

	// Create the CRD
	if err := c.client.Create(ctx, ea); err != nil {
		logger.Error(err, "Failed to create EffectivenessAssessment")
		return "", fmt.Errorf("failed to create EffectivenessAssessment: %w", err)
	}

	logger.Info("Created EffectivenessAssessment",
		"name", name,
		"correlationID", rr.Name,
		"signalTarget", fmt.Sprintf("%s/%s/%s", signalTarget.Namespace, signalTarget.Kind, signalTarget.Name),
		"remediationTarget", fmt.Sprintf("%s/%s/%s", remediationTarget.Namespace, remediationTarget.Kind, remediationTarget.Name),
		"stabilizationWindow", c.stabilizationWindow,
	)

	// DD-EVENT-001: Emit K8s event for EA creation (observable via kubectl describe rr)
	if c.recorder != nil {
		c.recorder.Eventf(rr, corev1.EventTypeNormal, events.EventReasonEffectivenessAssessmentCreated,
			"Created EffectivenessAssessment %s (correlationID: %s)", name, rr.Name)
	}

	// Track EA creation metric (DD-METRICS-001)
	c.metrics.EffectivenessAssessmentsCreatedTotal.WithLabelValues(rr.Namespace).Inc()

	return name, nil
}
