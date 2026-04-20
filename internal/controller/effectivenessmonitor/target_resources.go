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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/health"
	canonicalhash "github.com/jordigilh/kubernaut/pkg/shared/hash"
	k8sutil "github.com/jordigilh/kubernaut/pkg/shared/k8s"
)

// getTargetHealthStatus queries the K8s API for a target resource's health.
// Kind-aware: uses label-based listing for workload resources (Deployment,
// ReplicaSet, StatefulSet, DaemonSet) and direct pod lookup for Pod targets.
// Non-pod-owning resources (ConfigMap, Secret, Node, etc.) are checked for
// existence only -- they have no pod health to assess.
// DD-EM-003: Health uses RemediationTarget (#275), hash uses RemediationTarget.
func (r *Reconciler) getTargetHealthStatus(ctx context.Context, target eav1.TargetResource, remediationStartedAt *metav1.Time) health.TargetStatus {
	logger := log.FromContext(ctx)

	targetKind := target.Kind
	targetName := target.Name
	targetNs := target.Namespace

	var podList *corev1.PodList

	switch targetKind {
	case "Deployment", "ReplicaSet", "StatefulSet", "DaemonSet":
		podList = &corev1.PodList{}
		err := r.List(ctx, podList,
			client.InNamespace(targetNs),
			client.MatchingLabels{"app": targetName},
		)
		if err != nil {
			logger.Error(err, "Failed to list pods for target resource",
				"kind", targetKind, "name", targetName)
			return health.TargetStatus{TargetExists: false}
		}

	case "Pod":
		pod := &corev1.Pod{}
		err := r.Get(ctx, client.ObjectKey{Name: targetName, Namespace: targetNs}, pod)
		if err != nil {
			logger.V(1).Info("Target pod not found", "name", targetName, "error", err)
			return health.TargetStatus{TargetExists: false}
		}
		podList = &corev1.PodList{Items: []corev1.Pod{*pod}}

	default:
		logger.V(1).Info("Target resource kind has no pod health to assess",
			"kind", targetKind, "name", targetName)
		return health.TargetStatus{
			TargetExists:        true,
			HealthNotApplicable: true,
		}
	}

	activePods := FilterActivePods(podList.Items)
	if len(activePods) == 0 {
		return health.TargetStatus{TargetExists: false}
	}

	return ComputePodHealthStats(activePods, remediationStartedAt)
}

// listActivePodNames returns the names of currently running pods for a workload
// target (Deployment, ReplicaSet, StatefulSet, DaemonSet). Returns nil for non-
// workload kinds or when the listing fails (#269: stale alert pod correlation).
func (r *Reconciler) listActivePodNames(ctx context.Context, target eav1.TargetResource) []string {
	switch target.Kind {
	case "Deployment", "ReplicaSet", "StatefulSet", "DaemonSet":
	default:
		return nil
	}

	podList := &corev1.PodList{}
	if err := r.List(ctx, podList,
		client.InNamespace(target.Namespace),
		client.MatchingLabels{"app": target.Name},
	); err != nil {
		log.FromContext(ctx).V(1).Info("Failed to list pods for alert correlation, skipping filter",
			"kind", target.Kind, "name", target.Name, "error", err)
		return nil
	}

	active := FilterActivePods(podList.Items)
	if len(active) == 0 {
		return nil
	}

	names := make([]string, 0, len(active))
	for _, pod := range active {
		names = append(names, pod.Name)
	}
	return names
}

// getTargetFunctionalState fetches the target resource and returns both:
//   - functionalState: full obj.Object (for CanonicalResourceFingerprint)
//   - spec: obj.Object["spec"] (for ExtractConfigMapRefs which needs pod template structure)
//
// Returns (functionalState, spec, degradedReason) where:
//   - (obj, specMap, "") on success
//   - (emptyMap, emptyMap, "") when not applicable: NotFound, unknown GVK, nil RESTMapper
//   - (emptyMap, emptyMap, "reason") when degraded: Forbidden, transient API errors (Issue #546)
//
// DD-EM-002 v2.0 (#765): Returns full object for resource fingerprint.
func (r *Reconciler) getTargetFunctionalState(ctx context.Context, target eav1.TargetResource) (map[string]interface{}, map[string]interface{}, string) {
	logger := log.FromContext(ctx)

	if r.restMapper == nil {
		logger.V(1).Info("RESTMapper not configured, falling back to metadata spec")
		fallback := map[string]interface{}{
			"kind":      target.Kind,
			"name":      target.Name,
			"namespace": target.Namespace,
		}
		return fallback, fallback, ""
	}

	gvk, err := k8sutil.ResolveGVKForKind(r.restMapper, target.Kind)
	if err != nil {
		logger.Error(err, "Failed to resolve GVK for target resource kind",
			"kind", target.Kind)
		return map[string]interface{}{}, map[string]interface{}{}, ""
	}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	key := client.ObjectKey{
		Namespace: target.Namespace,
		Name:      target.Name,
	}
	if err := r.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Target resource not found, computing hash from empty spec",
				"kind", target.Kind,
				"name", target.Name)
			return map[string]interface{}{}, map[string]interface{}{}, ""
		}
		logger.Error(err, "Failed to fetch target resource")
		return map[string]interface{}{}, map[string]interface{}{}, fmt.Sprintf("failed to fetch target resource %s/%s: %v", target.Kind, target.Name, err)
	}

	spec, _, _ := unstructured.NestedMap(obj.Object, "spec")
	if spec == nil {
		spec = map[string]interface{}{}
	}

	logger.V(2).Info("Target functional state retrieved",
		"kind", target.Kind,
		"name", target.Name)
	return obj.Object, spec, ""
}

// getTargetSpec is a backward-compatible wrapper that returns only the spec map.
// Used by callers that don't need the full functional state.
func (r *Reconciler) getTargetSpec(ctx context.Context, target eav1.TargetResource) (map[string]interface{}, string) {
	_, spec, degradedReason := r.getTargetFunctionalState(ctx, target)
	return spec, degradedReason
}

// queryPreRemediationHash queries DataStorage for the pre-remediation spec hash
// from the RO's remediation.workflow_created audit event.
// Returns empty string if DS is unavailable or no pre-hash exists (graceful degradation).
func (r *Reconciler) queryPreRemediationHash(ctx context.Context, correlationID string) string {
	if r.DSQuerier == nil {
		log.FromContext(ctx).V(1).Info("DSQuerier not configured, skipping pre-remediation hash lookup")
		return ""
	}

	preHash, err := r.DSQuerier.QueryPreRemediationHash(ctx, correlationID)
	if err != nil {
		log.FromContext(ctx).Error(err, "Failed to query pre-remediation hash from DataStorage",
			"correlationID", correlationID)
		r.Metrics.RecordExternalCallError("datastorage", "query_pre_hash", "query_error")
		return ""
	}

	if preHash != "" {
		log.FromContext(ctx).V(1).Info("Pre-remediation hash retrieved from DataStorage",
			"correlationID", correlationID,
			"preHash", preHash[:min(23, len(preHash))]+"...")
	}
	return preHash
}

// resolveConfigMapHashes extracts ConfigMap references from the resource spec,
// fetches each ConfigMap via the uncached apiReader (bypassing the informer cache),
// and returns a map of name -> content hash.
// Missing/forbidden ConfigMaps produce a deterministic sentinel hash.
func (r *Reconciler) resolveConfigMapHashes(
	ctx context.Context,
	spec map[string]interface{},
	target eav1.TargetResource,
) map[string]string {
	refs := canonicalhash.ExtractConfigMapRefs(spec, target.Kind)
	if len(refs) == 0 {
		return nil
	}

	logger := log.FromContext(ctx)
	configMapHashes := make(map[string]string, len(refs))

	for _, cmName := range refs {
		cm := &corev1.ConfigMap{}
		key := client.ObjectKey{Name: cmName, Namespace: target.Namespace}
		if err := r.apiReader.Get(ctx, key, cm); err != nil {
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
					"configMap", cmName, "namespace", target.Namespace, "reason", err.Error())
			} else {
				logger.Error(err, "Transient ConfigMap fetch error, using sentinel hash",
					"configMap", cmName, "namespace", target.Namespace)
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
