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
)

// AIAnalysisCreator creates AIAnalysis CRDs from RemediationRequests.
type AIAnalysisCreator struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewAIAnalysisCreator creates a new AIAnalysisCreator.
func NewAIAnalysisCreator(c client.Client, s *runtime.Scheme) *AIAnalysisCreator {
	return &AIAnalysisCreator{
		client: c,
		scheme: s,
	}
}

// Create creates an AIAnalysis CRD for the given RemediationRequest.
// It uses enrichment data from the completed SignalProcessing CRD.
// Reference: BR-ORCH-025 (data pass-through), BR-ORCH-031 (cascade deletion)
func (c *AIAnalysisCreator) Create(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	sp *signalprocessingv1.SignalProcessing,
) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
	)

	// Generate deterministic name
	name := fmt.Sprintf("ai-%s", rr.Name)

	// Check if already exists (idempotency)
	existing := &aianalysisv1.AIAnalysis{}
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, existing)
	if err == nil {
		logger.Info("AIAnalysis already exists, reusing", "name", name)
		return name, nil
	}
	if !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("failed to check existing AIAnalysis: %w", err)
	}

	// Build AIAnalysis from RemediationRequest and SignalProcessing
	ai := c.buildAIAnalysis(rr, sp, name)

	// Set owner reference for cascade deletion (BR-ORCH-031)
	if err := controllerutil.SetControllerReference(rr, ai, c.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference")
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create the CRD
	if err := c.client.Create(ctx, ai); err != nil {
		logger.Error(err, "Failed to create AIAnalysis CRD")
		return "", fmt.Errorf("failed to create AIAnalysis: %w", err)
	}

	logger.Info("Created AIAnalysis CRD", "name", name)
	return name, nil
}

// buildAIAnalysis constructs the AIAnalysis CRD from RemediationRequest and SignalProcessing.
func (c *AIAnalysisCreator) buildAIAnalysis(
	rr *remediationv1.RemediationRequest,
	sp *signalprocessingv1.SignalProcessing,
	name string,
) *aianalysisv1.AIAnalysis {
	return &aianalysisv1.AIAnalysis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
			Labels: map[string]string{
				"kubernaut.ai/remediation-request": rr.Name,
				"kubernaut.ai/component":           "ai-analysis",
			},
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

			// Remediation ID for audit correlation
			RemediationID: string(rr.UID),

			// Analysis request with signal context
			AnalysisRequest: aianalysisv1.AnalysisRequest{
				SignalContext: c.buildSignalContext(rr, sp),
				AnalysisTypes: []string{
					"investigation",
					"root-cause",
					"workflow-selection",
				},
			},

			// Recovery fields (false for initial analysis)
			IsRecoveryAttempt:     false,
			RecoveryAttemptNumber: 0,
		},
	}
}

// buildSignalContext constructs the SignalContextInput from RemediationRequest and SignalProcessing.
func (c *AIAnalysisCreator) buildSignalContext(
	rr *remediationv1.RemediationRequest,
	sp *signalprocessingv1.SignalProcessing,
) aianalysisv1.SignalContextInput {
	ctx := aianalysisv1.SignalContextInput{
		Fingerprint:      rr.Spec.SignalFingerprint,
		Severity:         rr.Spec.Severity,
		SignalType:       rr.Spec.SignalType,
		Environment:      rr.Spec.Environment,
		BusinessPriority: rr.Spec.Priority,
	}

	// Pass through enrichment results from SignalProcessing (dereference pointer)
	if sp.Status.EnrichmentResults != nil {
		ctx.EnrichmentResults = *sp.Status.EnrichmentResults
	}

	return ctx
}
