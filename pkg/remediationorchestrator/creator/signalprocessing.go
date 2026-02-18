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
//
// Business Requirements:
// - BR-ORCH-025: Workflow data pass-through to child CRDs
// - BR-ORCH-031: Cascade deletion via owner references
package creator

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	rrconditions "github.com/jordigilh/kubernaut/pkg/remediationrequest"
)

// SignalProcessingCreator creates SignalProcessing CRDs from RemediationRequests.
type SignalProcessingCreator struct {
	client  client.Client
	scheme  *runtime.Scheme
	metrics *metrics.Metrics // DD-METRICS-001: Dependency-injected metrics
}

// NewSignalProcessingCreator creates a new SignalProcessingCreator.
func NewSignalProcessingCreator(c client.Client, s *runtime.Scheme, m *metrics.Metrics) *SignalProcessingCreator {
	return &SignalProcessingCreator{
		client:  c,
		scheme:  s,
		metrics: m,
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
		// Already exists, return existing name
		logger.Info("SignalProcessing already exists, reusing", "name", name)
		return name, nil
	}
	if !apierrors.IsNotFound(err) {
		// Real error (not "not found"), return it
		logger.Error(err, "Failed to check existing SignalProcessing")
		return "", fmt.Errorf("failed to check existing SignalProcessing: %w", err)
	}

	// Build SignalProcessing CRD with data pass-through (BR-ORCH-025)
	sp := &signalprocessingv1.SignalProcessing{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
			// Issue #91: labels removed; parent tracked via spec.remediationRequestRef + ownerRef

		},
		Spec: signalprocessingv1.SignalProcessingSpec{
			// Reference to parent RemediationRequest for audit trail
			RemediationRequestRef: signalprocessingv1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rr.Name,
				Namespace:  rr.Namespace,
				UID:        string(rr.UID),
			},
			// Signal data pass-through from RemediationRequest
			Signal: signalprocessingv1.SignalData{
				Fingerprint:    rr.Spec.SignalFingerprint,
				Name:           rr.Spec.SignalName,
				Severity:       rr.Spec.Severity,
				Type:           rr.Spec.SignalType,
				Source:         rr.Spec.SignalSource,
				TargetType:     rr.Spec.TargetType,
				Labels:         rr.Spec.SignalLabels,
				Annotations:    rr.Spec.SignalAnnotations,
				FiringTime:     &rr.Spec.FiringTime,
				ReceivedTime:   rr.Spec.ReceivedTime,
				ProviderData:   rr.Spec.ProviderData,
				TargetResource: c.buildTargetResource(rr),
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
	if err := controllerutil.SetControllerReference(rr, sp, c.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference")
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create the CRD
	if err := c.client.Create(ctx, sp); err != nil {
		logger.Error(err, "Failed to create SignalProcessing CRD")
		// DD-CRD-002-RR: Set SignalProcessingReady=False on creation failure
		rrconditions.SetSignalProcessingReady(rr, false, fmt.Sprintf("Failed to create SignalProcessing: %v", err), c.metrics)
		return "", fmt.Errorf("failed to create SignalProcessing: %w", err)
	}

	// DD-CRD-002-RR: Set SignalProcessingReady=True on successful creation
	// Note: Reconciler will handle Status().Update() after this call
	rrconditions.SetSignalProcessingReady(rr, true, fmt.Sprintf("SignalProcessing CRD %s created successfully", name), c.metrics)

	logger.Info("Created SignalProcessing CRD", "name", name)
	return name, nil
}

// buildTargetResource converts ResourceIdentifier to SignalProcessing format.
func (c *SignalProcessingCreator) buildTargetResource(rr *remediationv1.RemediationRequest) signalprocessingv1.ResourceIdentifier {
	return signalprocessingv1.ResourceIdentifier{
		Kind:      rr.Spec.TargetResource.Kind,
		Name:      rr.Spec.TargetResource.Name,
		Namespace: rr.Spec.TargetResource.Namespace,
	}
}
