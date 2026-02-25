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

// Package creator provides child CRD creation logic for the Remediation Orchestrator.
package creator

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	rrconditions "github.com/jordigilh/kubernaut/pkg/remediationrequest"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// AIAnalysisCreator creates AIAnalysis CRDs from RemediationRequests.
type AIAnalysisCreator struct {
	client  client.Client
	scheme  *runtime.Scheme
	metrics *metrics.Metrics // DD-METRICS-001: Dependency-injected metrics
}

// NewAIAnalysisCreator creates a new AIAnalysisCreator.
func NewAIAnalysisCreator(c client.Client, s *runtime.Scheme, m *metrics.Metrics) *AIAnalysisCreator {
	return &AIAnalysisCreator{
		client:  c,
		scheme:  s,
		metrics: m,
	}
}

// Create creates an AIAnalysis CRD for the given RemediationRequest.
// It uses enrichment data from the completed SignalProcessing CRD.
// It is idempotent - if the CRD already exists, it returns the existing name.
// Reference: BR-ORCH-025 (data pass-through), BR-ORCH-031 (cascade deletion)
func (c *AIAnalysisCreator) Create(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	sp *signalprocessingv1.SignalProcessing,
) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
		"signalProcessing", sp.Name,
	)

	// Generate deterministic name
	name := fmt.Sprintf("ai-%s", rr.Name)

	// Check if already exists (idempotency)
	existing := &aianalysisv1.AIAnalysis{}
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, existing)
	if err == nil {
		// Already exists, return existing name
		logger.Info("AIAnalysis already exists, reusing", "name", name)
		return name, nil
	}
	if !apierrors.IsNotFound(err) {
		// Real error (not "not found"), return it
		logger.Error(err, "Failed to check existing AIAnalysis")
		return "", fmt.Errorf("failed to check existing AIAnalysis: %w", err)
	}

	// Build AIAnalysis CRD
	ai := &aianalysisv1.AIAnalysis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
			// Issue #91: labels removed; parent tracked via spec.remediationRequestRef + ownerRef

		},
		Spec: aianalysisv1.AIAnalysisSpec{
			// Parent reference for audit trail
			RemediationRequestRef: corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rr.Name,
				Namespace:  rr.Namespace,
				UID:        rr.UID,
			},
		// DD-AUDIT-CORRELATION-001: Use RR name for consistency
		// TODO(v2): Deprecate RemediationID field (use RemediationRequestRef.Name instead)
		RemediationID: rr.Name,
			// Analysis request with signal context
			AnalysisRequest: aianalysisv1.AnalysisRequest{
				SignalContext: c.buildSignalContext(rr, sp),
				AnalysisTypes: []string{
					"investigation",
					"root-cause",
					"workflow-selection",
				},
			},
		},
	}

	// Validate RemediationRequest has required metadata for owner reference (defensive programming)
	// Gap 2.1: Prevents orphaned child CRDs if RR not properly persisted
	if rr.UID == "" {
		logger.Error(nil, "RemediationRequest has empty UID, cannot set owner reference")
		return "", fmt.Errorf("failed to set owner reference: RemediationRequest UID is required but empty")
	}

	// Set owner reference for cascade deletion (BR-ORCH-031)
	if err := controllerutil.SetControllerReference(rr, ai, c.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference")
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create the CRD
	if err := c.client.Create(ctx, ai); err != nil {
		logger.Error(err, "Failed to create AIAnalysis CRD")
		// DD-CRD-002-RR: Set AIAnalysisReady=False on creation failure
		rrconditions.SetAIAnalysisReady(rr, false, fmt.Sprintf("Failed to create AIAnalysis: %v", err), c.metrics)
		return "", fmt.Errorf("failed to create AIAnalysis: %w", err)
	}

	// DD-CRD-002-RR: Set AIAnalysisReady=True on successful creation
	// Note: Reconciler will handle Status().Update() after this call
	rrconditions.SetAIAnalysisReady(rr, true, fmt.Sprintf("AIAnalysis CRD %s created successfully", name), c.metrics)

	logger.Info("Created AIAnalysis CRD", "name", name)
	return name, nil
}

// buildSignalContext constructs the SignalContextInput from RemediationRequest and SignalProcessing.
// NOTE: Environment, Priority, and SignalType are now owned by SignalProcessing (per NOTICE_RO_REMEDIATIONREQUEST_SCHEMA_UPDATE.md)
// BR-SP-106: SignalType is read from sp.Status.SignalType (normalized) instead of rr.Spec.SignalType
// BR-AI-084: SignalMode is copied from sp.Status.SignalMode to allow HAPI prompt switching
func (c *AIAnalysisCreator) buildSignalContext(
	rr *remediationv1.RemediationRequest,
	sp *signalprocessingv1.SignalProcessing,
) aianalysisv1.SignalContextInput {
	// Environment and Priority come from SP status only (no longer on RR.Spec)
	environment := "unknown"
	priority := "P2" // Default medium priority
	if sp.Status.EnvironmentClassification != nil && sp.Status.EnvironmentClassification.Environment != "" {
		environment = sp.Status.EnvironmentClassification.Environment
	}
	if sp.Status.PriorityAssignment != nil && sp.Status.PriorityAssignment.Priority != "" {
		priority = sp.Status.PriorityAssignment.Priority
	}

	// BR-SP-106: SignalType from SP status (normalized by signal mode classifier)
	// For predictive signals: SP normalizes e.g. "PredictedOOMKill" -> "OOMKilled"
	// For reactive signals: SP copies Spec.Signal.Type unchanged
	// Fallback to RR spec if SP status field is empty (backwards compatibility)
	signalType := sp.Status.SignalName
	if signalType == "" {
		signalType = rr.Spec.SignalType
	}

	return aianalysisv1.SignalContextInput{
		Fingerprint:      rr.Spec.SignalFingerprint,
		Severity:         sp.Status.Severity, // DD-SEVERITY-001: Use normalized severity from SignalProcessing Rego (not external rr.Spec.Severity)
		SignalName:       signalType,          // BR-SP-106: Normalized by SP (not raw from RR)
		SignalMode:       sp.Status.SignalMode, // BR-AI-084: Predictive signal mode for HAPI prompt switching
		Environment:      environment,
		BusinessPriority: priority,
		TargetResource: aianalysisv1.TargetResource{
			Kind:      rr.Spec.TargetResource.Kind,
			Name:      rr.Spec.TargetResource.Name,
			Namespace: rr.Spec.TargetResource.Namespace,
		},
		// EnrichmentResults from SignalProcessing status (BR-ORCH-025)
		EnrichmentResults: c.buildEnrichmentResults(sp),
	}
}

// buildEnrichmentResults converts SignalProcessing status to shared EnrichmentResults.
// Reference: BR-ORCH-025 (data pass-through from SP enrichment)
//
// Issue #113: SP's KubernetesContext and BusinessClassification are now the shared types.
// Direct assignment - no conversion needed.
func (c *AIAnalysisCreator) buildEnrichmentResults(sp *signalprocessingv1.SignalProcessing) sharedtypes.EnrichmentResults {
	results := sharedtypes.EnrichmentResults{}

	// Pass through KubernetesContext from SP status (includes CustomLabels via KubernetesContext.CustomLabels)
	results.KubernetesContext = sp.Status.KubernetesContext

	// Pass through BusinessClassification from SP categorization (BR-SP-002, BR-SP-080, BR-SP-081)
	if sp.Status.BusinessClassification != nil {
		results.BusinessClassification = sp.Status.BusinessClassification
	}

	return results
}
