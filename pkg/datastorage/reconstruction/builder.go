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

package reconstruction

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

const (
	// ReconstructedNamePrefix is the prefix for reconstructed RemediationRequest names
	ReconstructedNamePrefix = "rr-reconstructed-"

	// ReconstructedNamespace is the namespace for reconstructed RemediationRequests
	ReconstructedNamespace = "kubernaut-system"

	// LabelManagedBy indicates the component managing this CRD
	LabelManagedBy = "app.kubernetes.io/managed-by"

	// LabelReconstructed marks this RR as reconstructed from audit trail
	LabelReconstructed = "kubernaut.ai/reconstructed"

	// LabelCorrelationID stores the correlation ID for audit trail lookup
	LabelCorrelationID = "kubernaut.ai/correlation-id"

	// AnnotationReconstructedAt stores the timestamp when reconstruction occurred
	AnnotationReconstructedAt = "kubernaut.ai/reconstructed-at"

	// AnnotationReconstructionSource indicates the source of reconstruction data
	AnnotationReconstructionSource = "kubernaut.ai/reconstruction-source"

	// FinalizerAuditRetention prevents deletion until audit trail is complete
	FinalizerAuditRetention = "kubernaut.ai/audit-retention"
)

// BuildRemediationRequest constructs a complete RemediationRequest CRD from reconstructed fields.
// This function creates a Kubernetes-compliant CRD with proper TypeMeta, ObjectMeta, Spec, and Status.
// TDD GREEN: Minimal implementation to pass current builder tests.
func BuildRemediationRequest(correlationID string, rrFields *ReconstructedRRFields) (*remediationv1.RemediationRequest, error) {
	// Validate inputs
	if correlationID == "" {
		return nil, fmt.Errorf("correlation ID is required for RR reconstruction")
	}
	if rrFields == nil {
		return nil, fmt.Errorf("rrFields cannot be nil")
	}
	if rrFields.Spec == nil {
		return nil, fmt.Errorf("rrFields.Spec cannot be nil")
	}

	// Validate required Spec fields
	if rrFields.Spec.SignalName == "" {
		return nil, fmt.Errorf("SignalName is required in rrFields.Spec")
	}

	// Generate unique name with prefix
	timestamp := time.Now().Unix()
	name := fmt.Sprintf("%s%s-%d", ReconstructedNamePrefix, correlationID, timestamp)

	// Create RemediationRequest with TypeMeta and ObjectMeta
	rr := &remediationv1.RemediationRequest{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "remediation.kubernaut.ai/v1alpha1",
			Kind:       "RemediationRequest",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ReconstructedNamespace,
			Labels: map[string]string{
				LabelManagedBy:     "kubernaut-datastorage",
				LabelReconstructed: "true",
				LabelCorrelationID: correlationID,
			},
			Annotations: map[string]string{
				AnnotationReconstructedAt:      time.Now().UTC().Format(time.RFC3339),
				AnnotationReconstructionSource: "audit-trail",
				LabelCorrelationID:             correlationID,
			},
			Finalizers: []string{
				FinalizerAuditRetention,
			},
		},
		Spec: *rrFields.Spec,
	}

	// Populate Status if provided (Status fields are optional)
	if rrFields.Status != nil {
		rr.Status = *rrFields.Status
	}

	return rr, nil
}
