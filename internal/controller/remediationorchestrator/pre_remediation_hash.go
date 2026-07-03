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

package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	canonicalhash "github.com/jordigilh/kubernaut/pkg/shared/hash"
	k8sutil "github.com/jordigilh/kubernaut/pkg/shared/k8s"
)

// resolveDualTargets resolves both signal and remediation targets for the EA (DD-EM-003).
//
// Signal target: Always from RR.Spec.TargetResource (the resource that triggered the alert).
// Remediation target: Prefers the LLM-identified RemediationTarget from the AIAnalysis
// RootCauseAnalysis. Falls back to RR.Spec.TargetResource when AI analysis is unavailable
// or did not identify a specific resource.
func resolveDualTargets(
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
) *creator.DualTarget {
	signal := eav1.TargetResource{
		Kind:       rr.Spec.TargetResource.Kind,
		Name:       rr.Spec.TargetResource.Name,
		Namespace:  rr.Spec.TargetResource.Namespace,
		APIVersion: rr.Spec.TargetResource.APIVersion, // #1040
	}

	remediation := signal
	if ai != nil && ai.Status.RootCauseAnalysis != nil && ai.Status.RootCauseAnalysis.RemediationTarget != nil {
		ar := ai.Status.RootCauseAnalysis.RemediationTarget
		if ar.Kind != "" && ar.Name != "" {
			remediation = eav1.TargetResource{
				Kind:       ar.Kind,
				Name:       ar.Name,
				Namespace:  ar.Namespace,
				APIVersion: ar.APIVersion, // #1040
			}
		}
	}

	return &creator.DualTarget{Signal: signal, Remediation: remediation}
}

// formatRemediationTargetString builds a "namespace/kind/name" or "kind/name"
// string from an AIAnalysis RemediationTarget. Returns "" if the target is nil.
func formatRemediationTargetString(ai *aianalysisv1.AIAnalysis) string {
	if ai == nil || ai.Status.RootCauseAnalysis == nil || ai.Status.RootCauseAnalysis.RemediationTarget == nil {
		return ""
	}
	ar := ai.Status.RootCauseAnalysis.RemediationTarget
	if ar.Kind == "" || ar.Name == "" {
		return ""
	}
	if ar.Namespace != "" {
		return ar.Namespace + "/" + ar.Kind + "/" + ar.Name
	}
	return ar.Kind + "/" + ar.Name
}

// CapturePreRemediationHash fetches the target resource via an uncached reader
// and computes the canonical resource fingerprint (DD-EM-002 v2.0, #765).
//
// Returns (hash, degradedReason, err) where:
//   - ("sha256:...", "", nil) on success
//   - ("", "", nil) when legitimately no hash: NotFound, unknown GVK
//   - ("", "reason", nil) when degraded: Forbidden, transient API errors (Issue #545 defense-in-depth)
//   - ("", "", err) on hard errors: fingerprint computation failures
//
// This is exported for testability from the test package.
func CapturePreRemediationHash(
	ctx context.Context,
	reader client.Reader,
	restMapper meta.RESTMapper,
	targetKind string,
	targetName string,
	targetNamespace string,
) (string, string, error) {
	logger := log.FromContext(ctx)

	gvk, err := k8sutil.ResolveGVKForKind(restMapper, targetKind)
	if err != nil {
		logger.V(1).Info("Cannot resolve GVK for kind, skipping pre-remediation hash",
			"kind", targetKind, "error", err)
		return "", "", nil
	}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	key := client.ObjectKey{Name: targetName, Namespace: targetNamespace}
	if err := reader.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info("Target resource not found, skipping pre-remediation hash",
				"kind", targetKind, "name", targetName, "namespace", targetNamespace)
			return "", "", nil
		}
		reason := fmt.Sprintf("failed to fetch target resource %s/%s: %v", targetKind, targetName, err)
		logger.Info("Pre-remediation hash capture degraded (soft-fail)",
			"kind", targetKind, "name", targetName, "namespace", targetNamespace, "reason", reason)
		return "", reason, nil
	}

	fingerprint, err := canonicalhash.CanonicalResourceFingerprint(obj.Object)
	if err != nil {
		return "", "", fmt.Errorf("failed to compute resource fingerprint for %s/%s: %w", targetKind, targetName, err)
	}

	spec, _, _ := unstructured.NestedMap(obj.Object, "spec")
	configMapHashes := resolveConfigMapHashes(ctx, reader, spec, targetKind, targetNamespace)

	compositeHash, err := canonicalhash.CompositeResourceFingerprint(fingerprint, configMapHashes)
	if err != nil {
		return "", "", fmt.Errorf("failed to compute composite fingerprint for %s/%s: %w", targetKind, targetName, err)
	}

	if len(configMapHashes) > 0 {
		logger.V(1).Info("Pre-remediation composite fingerprint computed",
			"kind", targetKind, "name", targetName,
			"fingerprint", fingerprint, "configMapCount", len(configMapHashes),
			"compositeHash", compositeHash)
	}

	return compositeHash, "", nil
}

// resolveConfigMapHashes extracts ConfigMap references from the resource spec,
// fetches each ConfigMap's data, and returns a map of name -> content hash.
// Missing/forbidden ConfigMaps produce a deterministic sentinel hash.
// Transient errors are logged and the ConfigMap is skipped (non-fatal).
func resolveConfigMapHashes(
	ctx context.Context,
	reader client.Reader,
	spec map[string]interface{},
	kind string,
	namespace string,
) map[string]string {
	refs := canonicalhash.ExtractConfigMapRefs(spec, kind)
	if len(refs) == 0 {
		return nil
	}

	logger := log.FromContext(ctx)
	configMapHashes := make(map[string]string, len(refs))

	for _, cmName := range refs {
		cm := &corev1.ConfigMap{}
		key := client.ObjectKey{Name: cmName, Namespace: namespace}
		if err := reader.Get(ctx, key, cm); err != nil {
			// All fetch errors (404, 403, transient) use sentinel to ensure deterministic
			// hash computation: the same set of ConfigMap names always contributes to the
			// composite hash, preventing false-positive drift from intermittent failures.
			sentinelData := map[string]string{"__sentinel__": fmt.Sprintf("__absent:%s__", cmName)}
			sentinelHash, hashErr := canonicalhash.ConfigMapDataHash(sentinelData, nil)
			if hashErr != nil {
				logger.Error(hashErr, "Failed to compute sentinel hash for ConfigMap", "configMap", cmName)
				continue
			}
			configMapHashes[cmName] = sentinelHash
			if apierrors.IsNotFound(err) || apierrors.IsForbidden(err) {
				logger.V(1).Info("ConfigMap not accessible, using sentinel hash",
					"configMap", cmName, "namespace", namespace, "reason", err.Error())
			} else {
				logger.Error(err, "Transient ConfigMap fetch error, using sentinel hash",
					"configMap", cmName, "namespace", namespace)
			}
			continue
		}

		cmHash, err := canonicalhash.ConfigMapDataHash(cm.Data, cm.BinaryData)
		if err != nil {
			logger.Error(err, "Failed to compute hash for ConfigMap, skipping", "configMap", cmName)
			continue
		}
		configMapHashes[cmName] = cmHash
	}

	return configMapHashes
}
