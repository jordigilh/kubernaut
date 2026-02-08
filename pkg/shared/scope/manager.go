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

// Package scope provides resource scope management for Kubernaut.
//
// # Business Requirements
//
// BR-SCOPE-001: Resource Scope Management - Opt-In Model
// ADR-053: Resource Scope Management Architecture
//
// # Overview
//
// Kubernaut only remediates resources that are explicitly opted-in by operators
// using the kubernaut.ai/managed label. The Manager validates scope using a
// 2-level hierarchy:
//
//  1. Resource label (if resource exists and has label)
//  2. Namespace label (for namespaced resources)
//  3. Default: unmanaged (safe default)
//
// For cluster-scoped resources (namespace==""), only the resource label is checked.
//
// # Usage
//
// The Manager accepts a client.Reader. Both Gateway and RO inject a cached client
// (controller-runtime's metadata-only informer cache) for 0 direct API calls.
// The Manager implements the ScopeChecker interface for DI (see checker.go).
//
//	mgr := scope.NewManager(cachedClient) // ADR-053: metadata-only cache
//	managed, err := mgr.IsManaged(ctx, "production", "Deployment", "payment-api")
package scope

import (
	"context"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// scopeLookupTimeout is the maximum time to wait for a cached metadata lookup.
// This prevents indefinite blocking when the informer cache cannot sync
// (e.g., missing RBAC for the target resource kind). ADR-053 §Resilience.
const scopeLookupTimeout = 5 * time.Second

const (
	// ManagedLabelKey is the canonical label key for Kubernaut resource scope management.
	// Resources/namespaces with this label set to "true" are managed by Kubernaut;
	// those with "false" or without the label are unmanaged (safe default).
	//
	// Reference: BR-SCOPE-001, ADR-053
	ManagedLabelKey = "kubernaut.ai/managed"

	// ManagedLabelValueTrue indicates the resource is managed by Kubernaut.
	ManagedLabelValueTrue = "true"

	// ManagedLabelValueFalse indicates the resource is explicitly unmanaged.
	ManagedLabelValueFalse = "false"
)

// namespaceGVK is the GroupVersionKind for Namespace, used for metadata-only lookups.
var namespaceGVK = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}

// kindToGroup maps Kubernetes resource kinds to their API groups.
// This static mapping covers core resource types managed by Kubernaut.
// The set is intentionally limited — no collision risk between groups.
// Same pattern as pkg/signalprocessing/ownerchain/builder.go:getGVKForKind()
var kindToGroup = map[string]string{
	"Pod":              "",
	"Node":             "",
	"Service":          "",
	"ConfigMap":        "",
	"Secret":           "",
	"Namespace":        "",
	"PersistentVolume": "",
	"ReplicaSet":       "apps",
	"Deployment":       "apps",
	"StatefulSet":      "apps",
	"DaemonSet":        "apps",
	"Job":              "batch",
	"CronJob":          "batch",
}

// clusterScopedKinds identifies cluster-scoped resource kinds.
// Cluster-scoped resources have no namespace fallback — only their own label is checked.
var clusterScopedKinds = map[string]bool{
	"Node":               true,
	"PersistentVolume":   true,
	"Namespace":          true,
	"ClusterRole":        true,
	"ClusterRoleBinding": true,
}

// Manager validates resource scope using the 2-level hierarchy defined in ADR-053.
// It is shared by the Gateway and the Remediation Orchestrator, both using cached readers
// (controller-runtime metadata-only informers, ADR-053 Decision #5).
// Implements the ScopeChecker interface for dependency injection.
type Manager struct {
	client client.Reader
}

// NewManager creates a new scope Manager with the given client reader.
// Both Gateway and RO pass their cached client (ctrlClient / mgr.GetClient()),
// which creates metadata-only informers for PartialObjectMetadata lookups.
func NewManager(c client.Reader) *Manager {
	return &Manager{client: c}
}

// IsManaged checks whether a Kubernetes resource is managed by Kubernaut.
//
// The 2-level hierarchy (ADR-053):
//  1. Check resource label: "true" → managed, "false" → unmanaged, missing/invalid → continue
//  2. Check namespace label: "true" → managed, "false" → unmanaged, missing → unmanaged
//
// For cluster-scoped resources (namespace==""), only the resource label is checked.
// Resource not found: falls through to namespace check.
// Namespace not found: returns false, nil (unmanaged, not an error).
//
// Parameters:
//   - namespace: the resource's namespace (empty for cluster-scoped resources like Node, PV)
//   - kind: the Kubernetes resource kind (e.g., "Pod", "Deployment", "Node")
//   - name: the resource instance name
func (m *Manager) IsManaged(ctx context.Context, namespace, kind, name string) (bool, error) {
	logger := log.FromContext(ctx).WithName("scope").WithValues(
		"namespace", namespace, "kind", kind, "name", name,
	)

	// Step 1: Check resource label (Level 1)
	resourceManaged, found, err := m.checkResourceLabel(ctx, namespace, kind, name)
	if err != nil {
		logger.Error(err, "Failed to check resource label")
		return false, err
	}
	if found {
		logger.V(1).Info("Scope resolved from resource label", "managed", resourceManaged)
		return resourceManaged, nil
	}

	// Step 2: For cluster-scoped resources (namespace==""), no namespace fallback
	if isClusterScoped(kind) || namespace == "" {
		logger.V(1).Info("Cluster-scoped resource without label — unmanaged (safe default)")
		return false, nil
	}

	// Step 3: Check namespace label (Level 2)
	nsManaged, found, err := m.checkNamespaceLabel(ctx, namespace)
	if err != nil {
		logger.Error(err, "Failed to check namespace label")
		return false, err
	}
	if found {
		logger.V(1).Info("Scope inherited from namespace label", "managed", nsManaged)
		return nsManaged, nil
	}

	// Step 4: Default — unmanaged (safe default per ADR-053)
	logger.V(1).Info("No scope labels found — unmanaged (safe default)")
	return false, nil
}

// checkResourceLabel fetches the resource metadata and checks its managed label.
// Returns (managed, found, error) where found=true means a valid label value was present.
//
// Resilience behavior (ADR-053):
//   - Unknown kind (not in kindToGroup): skips resource check entirely, falls through to namespace.
//   - Resource not found: falls through to namespace check (not an error).
//   - Non-NotFound errors (Forbidden, "no matches for kind", etc.): graceful fallthrough to
//     namespace check with Info-level log. This ensures scope validation degrades to namespace-level
//     when resource-level validation is not possible (e.g., RBAC limitations, unknown CRDs).
func (m *Manager) checkResourceLabel(ctx context.Context, namespace, kind, name string) (bool, bool, error) {
	// Skip resource check for unknown kinds — fall through to namespace
	if _, known := kindToGroup[kind]; !known {
		log.FromContext(ctx).WithName("scope").V(1).Info(
			"Unknown resource kind — skipping resource label check",
			"kind", kind)
		return false, false, nil
	}

	gvk := kindToGVK(kind)

	obj := &metav1.PartialObjectMetadata{}
	obj.SetGroupVersionKind(gvk)

	// Defensive timeout: prevents indefinite blocking if the informer cache
	// cannot sync for this resource kind (e.g., missing RBAC for list/watch).
	// Controller-runtime's cached client blocks Get() until the cache is synced;
	// without RBAC the sync never completes, hanging the request forever.
	lookupCtx, cancel := context.WithTimeout(ctx, scopeLookupTimeout)
	defer cancel()

	key := client.ObjectKey{Namespace: namespace, Name: name}
	if err := m.client.Get(lookupCtx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return false, false, nil // resource not found — fall through
		}
		// Graceful degradation for non-NotFound errors (Forbidden, timeout, connection errors, etc.)
		// Info-level (not V(1)) so operators notice RBAC/infra degradation in production
		log.FromContext(ctx).WithName("scope").Info(
			"Resource label check failed — falling through to namespace check. "+
				"This may indicate missing RBAC permissions for scope validation.",
			"kind", kind, "name", name, "error", err)
		return false, false, nil
	}

	return checkLabelValue(obj.Labels)
}

// checkNamespaceLabel fetches the namespace metadata and checks its managed label.
// Returns (managed, found, error) where found=true means a valid label value was present.
// Namespace not found is NOT an error — returns (false, false, nil) to fall through.
func (m *Manager) checkNamespaceLabel(ctx context.Context, namespace string) (bool, bool, error) {
	obj := &metav1.PartialObjectMetadata{}
	obj.SetGroupVersionKind(namespaceGVK)

	// Same defensive timeout as checkResourceLabel — see scopeLookupTimeout docs.
	lookupCtx, cancel := context.WithTimeout(ctx, scopeLookupTimeout)
	defer cancel()

	key := client.ObjectKey{Name: namespace}
	if err := m.client.Get(lookupCtx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return false, false, nil // namespace not found — fall through to default
		}
		return false, false, err
	}

	return checkLabelValue(obj.Labels)
}

// checkLabelValue inspects the kubernaut.ai/managed label and returns (managed, found).
// "true" → (true, true), "false" → (false, true), missing or invalid → (false, false).
func checkLabelValue(labels map[string]string) (bool, bool, error) {
	val, exists := labels[ManagedLabelKey]
	if !exists {
		return false, false, nil // no label — fall through
	}
	switch val {
	case ManagedLabelValueTrue:
		return true, true, nil
	case ManagedLabelValueFalse:
		return false, true, nil
	default:
		return false, false, nil // invalid value — treat as unset
	}
}

// kindToGVK resolves a Kubernetes resource kind string to its GroupVersionKind.
func kindToGVK(kind string) schema.GroupVersionKind {
	group := ""
	if g, ok := kindToGroup[kind]; ok {
		group = g
	}
	return schema.GroupVersionKind{Group: group, Version: "v1", Kind: kind}
}

// isClusterScoped returns true if the resource kind is cluster-scoped.
func isClusterScoped(kind string) bool {
	return clusterScopedKinds[kind]
}
