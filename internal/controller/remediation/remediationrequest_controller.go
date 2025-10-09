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

package remediation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1alpha1"
)

// RemediationRequestReconciler reconciles a RemediationRequest object
type RemediationRequestReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=remediation.kubernaut.io,resources=remediationrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=remediation.kubernaut.io,resources=remediationrequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=remediation.kubernaut.io,resources=remediationrequests/finalizers,verbs=update
// +kubebuilder:rbac:groups=remediationprocessing.kubernaut.io,resources=remediationprocessings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=remediationprocessing.kubernaut.io,resources=remediationprocessings/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// Task 2.2: Creates RemediationProcessing CRD with self-contained data copied from RemediationRequest
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *RemediationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the RemediationRequest instance
	var remediationRequest remediationv1alpha1.RemediationRequest
	if err := r.Get(ctx, req.NamespacedName, &remediationRequest); err != nil {
		// Handle not found or other errors
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if RemediationProcessing CRD already exists
	processingName := fmt.Sprintf("%s-processing", remediationRequest.Name)
	var existingProcessing remediationprocessingv1alpha1.RemediationProcessing
	err := r.Get(ctx, client.ObjectKey{
		Name:      processingName,
		Namespace: remediationRequest.Namespace,
	}, &existingProcessing)

	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to check for existing RemediationProcessing")
		return ctrl.Result{}, err
	}

	// Create RemediationProcessing if it doesn't exist
	if errors.IsNotFound(err) {
		processing := &remediationprocessingv1alpha1.RemediationProcessing{
			ObjectMeta: metav1.ObjectMeta{
				Name:      processingName,
				Namespace: remediationRequest.Namespace,
				Labels: map[string]string{
					"remediation-request": remediationRequest.Name,
					"signal-type":         remediationRequest.Spec.SignalType,
					"environment":         remediationRequest.Spec.Environment,
					"priority":            remediationRequest.Spec.Priority,
				},
				Annotations: map[string]string{
					"signal-fingerprint": remediationRequest.Spec.SignalFingerprint,
					"created-by":         "remediation-orchestrator",
				},
			},
			Spec: mapRemediationRequestToProcessingSpec(&remediationRequest),
		}

		// Set owner reference for cascade deletion
		if err := ctrl.SetControllerReference(&remediationRequest, processing, r.Scheme); err != nil {
			log.Error(err, "Failed to set owner reference")
			return ctrl.Result{}, err
		}

		// Create the RemediationProcessing CRD
		if err := r.Create(ctx, processing); err != nil {
			log.Error(err, "Failed to create RemediationProcessing")
			return ctrl.Result{}, err
		}

		log.Info("Created RemediationProcessing CRD",
			"name", processingName,
			"signal-fingerprint", remediationRequest.Spec.SignalFingerprint)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RemediationRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&remediationv1alpha1.RemediationRequest{}).
		Owns(&remediationprocessingv1alpha1.RemediationProcessing{}). // Watch owned CRDs
		Named("remediation-remediationrequest").
		Complete(r)
}

// ========================================
// FIELD MAPPING FUNCTIONS
// Task 2.2: Self-contained CRD Pattern Implementation
// ========================================

// mapRemediationRequestToProcessingSpec maps RemediationRequest data to RemediationProcessing spec
// Phase 1: Implements self-contained CRD pattern - all data copied, no external reads required
func mapRemediationRequestToProcessingSpec(rr *remediationv1alpha1.RemediationRequest) remediationprocessingv1alpha1.RemediationProcessingSpec {
	return remediationprocessingv1alpha1.RemediationProcessingSpec{
		// ========================================
		// PARENT REFERENCE (Audit/Lineage Only)
		// ========================================
		RemediationRequestRef: corev1.ObjectReference{
			APIVersion: rr.APIVersion,
			Kind:       rr.Kind,
			Name:       rr.Name,
			Namespace:  rr.Namespace,
			UID:        rr.UID,
		},

		// ========================================
		// SIGNAL IDENTIFICATION (From RemediationRequest)
		// ========================================
		SignalFingerprint: rr.Spec.SignalFingerprint,
		SignalName:        rr.Spec.SignalName,
		Severity:          rr.Spec.Severity,

		// ========================================
		// SIGNAL CLASSIFICATION (From RemediationRequest)
		// ========================================
		Environment:  rr.Spec.Environment,
		Priority:     rr.Spec.Priority,
		SignalType:   rr.Spec.SignalType,
		SignalSource: rr.Spec.SignalSource,
		TargetType:   rr.Spec.TargetType,

		// ========================================
		// SIGNAL METADATA (From RemediationRequest)
		// ========================================
		SignalLabels:      deepCopyStringMap(rr.Spec.SignalLabels),
		SignalAnnotations: deepCopyStringMap(rr.Spec.SignalAnnotations),

		// ========================================
		// TARGET RESOURCE (From RemediationRequest)
		// ========================================
		TargetResource: extractTargetResource(rr),

		// ========================================
		// TIMESTAMPS (From RemediationRequest)
		// ========================================
		FiringTime:   rr.Spec.FiringTime,
		ReceivedTime: rr.Spec.ReceivedTime,

		// ========================================
		// DEDUPLICATION (From RemediationRequest)
		// ========================================
		Deduplication: mapDeduplicationInfo(rr.Spec.Deduplication),

		// ========================================
		// PROVIDER DATA (From RemediationRequest)
		// ========================================
		ProviderData:    deepCopyBytes(rr.Spec.ProviderData),
		OriginalPayload: deepCopyBytes(rr.Spec.OriginalPayload),

		// ========================================
		// STORM DETECTION (From RemediationRequest)
		// ========================================
		IsStorm:         rr.Spec.IsStorm,
		StormAlertCount: rr.Spec.StormAlertCount,

		// ========================================
		// CONFIGURATION (Optional, defaults if not specified)
		// ========================================
		EnrichmentConfig: getDefaultEnrichmentConfig(),
	}
}

// ========================================
// HELPER FUNCTIONS
// ========================================

// deepCopyStringMap creates a deep copy of string map
func deepCopyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	copied := make(map[string]string, len(m))
	for k, v := range m {
		copied[k] = v
	}
	return copied
}

// deepCopyBytes creates a deep copy of byte slice
func deepCopyBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	copied := make([]byte, len(b))
	copy(copied, b)
	return copied
}

// extractTargetResource extracts target resource from RemediationRequest
// Phase 1: Parses providerData to identify Kubernetes resource
// Future: Support for AWS/Azure/GCP resources
func extractTargetResource(rr *remediationv1alpha1.RemediationRequest) remediationprocessingv1alpha1.ResourceIdentifier {
	// Default empty resource if extraction fails
	resource := remediationprocessingv1alpha1.ResourceIdentifier{}

	// For Kubernetes signals, parse providerData
	if rr.Spec.TargetType == "kubernetes" {
		// Parse Kubernetes-specific fields from providerData
		var kubernetesData struct {
			Namespace string `json:"namespace"`
			Resource  struct {
				Kind string `json:"kind"`
				Name string `json:"name"`
			} `json:"resource"`
		}

		if err := json.Unmarshal(rr.Spec.ProviderData, &kubernetesData); err == nil {
			resource.Namespace = kubernetesData.Namespace
			resource.Kind = kubernetesData.Resource.Kind
			resource.Name = kubernetesData.Resource.Name
		}
	}

	// Fallback: Try to extract from signal labels
	if resource.Namespace == "" {
		if ns, ok := rr.Spec.SignalLabels["namespace"]; ok {
			resource.Namespace = ns
		}
	}
	if resource.Kind == "" {
		// Common label keys for resource kind
		for _, key := range []string{"kind", "resource_kind", "object_kind"} {
			if kind, ok := rr.Spec.SignalLabels[key]; ok {
				resource.Kind = kind
				break
			}
		}
	}
	if resource.Name == "" {
		// Common label keys for resource name
		for _, key := range []string{"pod", "deployment", "statefulset", "daemonset", "resource_name"} {
			if name, ok := rr.Spec.SignalLabels[key]; ok {
				resource.Name = name
				if resource.Kind == "" {
					// Infer kind from label key
					resource.Kind = strings.Title(key)
				}
				break
			}
		}
	}

	return resource
}

// mapDeduplicationInfo maps RemediationRequest deduplication to RemediationProcessing format
func mapDeduplicationInfo(dedupInfo remediationv1alpha1.DeduplicationInfo) remediationprocessingv1alpha1.DeduplicationContext {
	return remediationprocessingv1alpha1.DeduplicationContext{
		FirstOccurrence: dedupInfo.FirstSeen,
		LastOccurrence:  dedupInfo.LastSeen,
		OccurrenceCount: dedupInfo.OccurrenceCount,
		CorrelationID:   dedupInfo.PreviousRemediationRequestRef, // Optional correlation
	}
}

// getDefaultEnrichmentConfig returns default enrichment configuration
func getDefaultEnrichmentConfig() *remediationprocessingv1alpha1.EnrichmentConfiguration {
	return &remediationprocessingv1alpha1.EnrichmentConfiguration{
		EnableClusterState: true,  // Enable by default for Kubernetes signals
		EnableMetrics:      true,  // Enable by default for context
		EnableHistorical:   false, // Disabled by default (performance)
	}
}
