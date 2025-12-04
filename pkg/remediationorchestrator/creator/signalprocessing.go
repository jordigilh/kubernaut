// Package creator provides child CRD creation logic for the Remediation Orchestrator.
//
// Business Requirements:
// - BR-ORCH-025: Workflow data pass-through to child CRDs
// - BR-ORCH-031: Cascade deletion via owner references
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

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// SignalProcessingCreator creates SignalProcessing CRDs from RemediationRequests.
type SignalProcessingCreator struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewSignalProcessingCreator creates a new SignalProcessingCreator.
func NewSignalProcessingCreator(c client.Client, s *runtime.Scheme) *SignalProcessingCreator {
	return &SignalProcessingCreator{
		client: c,
		scheme: s,
	}
}

// Create creates a SignalProcessing CRD for the given RemediationRequest.
// It is idempotent - if the CRD already exists, it returns the existing name.
// Reference: BR-ORCH-025 (data pass-through), BR-ORCH-031 (cascade deletion)
func (c *SignalProcessingCreator) Create(ctx context.Context, rr *remediationv1.RemediationRequest) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
	)

	// Generate deterministic name
	name := fmt.Sprintf("sp-%s", rr.Name)

	// Check if already exists (idempotency)
	existing := &signalprocessingv1.SignalProcessing{}
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, existing)
	if err == nil {
		logger.Info("SignalProcessing already exists, reusing", "name", name)
		return name, nil
	}
	if !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("failed to check existing SignalProcessing: %w", err)
	}

	// Build SignalProcessing from RemediationRequest
	sp := c.buildSignalProcessing(rr, name)

	// Set owner reference for cascade deletion (BR-ORCH-031)
	if err := controllerutil.SetControllerReference(rr, sp, c.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference")
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create the CRD
	if err := c.client.Create(ctx, sp); err != nil {
		logger.Error(err, "Failed to create SignalProcessing CRD")
		return "", fmt.Errorf("failed to create SignalProcessing: %w", err)
	}

	logger.Info("Created SignalProcessing CRD", "name", name)
	return name, nil
}

// buildSignalProcessing constructs the SignalProcessing CRD from RemediationRequest.
func (c *SignalProcessingCreator) buildSignalProcessing(rr *remediationv1.RemediationRequest, name string) *signalprocessingv1.SignalProcessing {
	now := metav1.Now()

	return &signalprocessingv1.SignalProcessing{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
			Labels: map[string]string{
				"kubernaut.ai/remediation-request": rr.Name,
				"kubernaut.ai/component":           "signal-processing",
			},
		},
		Spec: signalprocessingv1.SignalProcessingSpec{
			// Parent reference for audit trail
			RemediationRequestRef: corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rr.Name,
				Namespace:  rr.Namespace,
				UID:        rr.UID,
			},

			// Signal identification (pass-through from RR)
			SignalFingerprint: rr.Spec.SignalFingerprint,
			SignalName:        rr.Spec.SignalName,
			Severity:          rr.Spec.Severity,

			// Signal classification (pass-through from RR)
			Environment:  rr.Spec.Environment,
			Priority:     rr.Spec.Priority,
			SignalType:   rr.Spec.SignalType,
			SignalSource: rr.Spec.SignalSource,
			TargetType:   rr.Spec.TargetType,

			// Signal metadata (pass-through from RR)
			SignalLabels:      rr.Spec.SignalLabels,
			SignalAnnotations: rr.Spec.SignalAnnotations,

			// Target resource (pass-through from RR)
			TargetResource: c.buildTargetResource(rr),

			// Timestamps
			ReceivedTime: now,

			// Deduplication info (pass-through from RR)
			Deduplication: rr.Spec.Deduplication,
		},
	}
}

// buildTargetResource converts ResourceIdentifier to SignalProcessing format.
func (c *SignalProcessingCreator) buildTargetResource(rr *remediationv1.RemediationRequest) signalprocessingv1.ResourceIdentifier {
	return signalprocessingv1.ResourceIdentifier{
		Kind:      rr.Spec.TargetResource.Kind,
		Name:      rr.Spec.TargetResource.Name,
		Namespace: rr.Spec.TargetResource.Namespace,
	}
}
