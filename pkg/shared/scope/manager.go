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
// The Manager accepts a client.Reader. Both Gateway and RO inject the cached client
// (informer-backed) because namespace labels are stable — they are set well before
// alerts or reconciliation runs. This avoids unnecessary API server load under high
// alert volume. The Manager implements the ScopeChecker interface for DI (see checker.go).
//
//	mgr := scope.NewManager(cachedClient) // Gateway + RO: informer-backed reads
//	managed, err := mgr.IsManagedResource(ctx, scope.ResourceIdentity{Namespace: "production", Kind: "Deployment", Name: "payment-api"})
package scope

import (
	"context"
	"fmt"
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
	"Node":                           true,
	"PersistentVolume":               true,
	"Namespace":                      true,
	"ClusterRole":                    true,
	"ClusterRoleBinding":             true,
	"StorageClass":                   true,
	"CustomResourceDefinition":       true,
	"ClusterOperator":                true,
	"ClusterVersion":                 true,
	"IngressClass":                   true,
	"PriorityClass":                  true,
	"ValidatingWebhookConfiguration": true,
	"MutatingWebhookConfiguration":   true,
	"CSIDriver":                      true,
	"CSINode":                        true,
	"VolumeAttachment":               true,
	"RuntimeClass":                   true,
	"APIService":                     true,
	"CertificateSigningRequest":      true,
	"FlowSchema":                     true,
	"PriorityLevelConfiguration":     true,
	"ClusterNetwork":                 true,
	"Infrastructure":                 true,
}

// Manager validates resource scope using the 2-level hierarchy defined in ADR-053.
// It is shared by the Gateway (uncached reader for immediate namespace visibility) and
// the Remediation Orchestrator (cached reader, sufficient for reconciliation).
// Implements the ScopeChecker interface for dependency injection.
type Manager struct {
	client client.Reader
}

// LabelState is the bounded representation of the managed-scope label.
// Invalid values are retained for provenance but never count as opt-in.
type LabelState string

const (
	LabelStateTrue    LabelState = "true"
	LabelStateFalse   LabelState = "false"
	LabelStateAbsent  LabelState = "absent"
	LabelStateInvalid LabelState = "invalid"
)

// ScopeSource identifies the label that determined the effective scope.
type ScopeSource string

const (
	ScopeSourceResourceLabel  ScopeSource = "resource-label"
	ScopeSourceNamespaceLabel ScopeSource = "namespace-label"
	ScopeSourceNone           ScopeSource = "none"
	ScopeSourceUnavailable    ScopeSource = "unavailable"
)

// Resolution is the structured scope decision used by sanitized projections.
// Reason is for internal diagnostics and must not be serialized to Console
// responses or audit payloads.
type Resolution struct {
	Included       bool
	ResourceLabel  LabelState
	NamespaceLabel LabelState
	Source         ScopeSource
	ClusterScoped  bool
	Reason         string
}

// ResolveLabels applies the single ADR-053 precedence algorithm to already
// fetched metadata. Local inventory readers use this function so they do not
// maintain a second scope implementation.
func ResolveLabels(resourceLabels, namespaceLabels map[string]string, clusterScoped bool) Resolution {
	return resolveLabelStates(labelState(resourceLabels), labelState(namespaceLabels), clusterScoped)
}

func resolveLabelStates(resourceState, namespaceState LabelState, clusterScoped bool) Resolution {
	resolution := Resolution{
		ResourceLabel:  resourceState,
		NamespaceLabel: namespaceState,
		ClusterScoped:  clusterScoped,
		Source:         ScopeSourceNone,
	}
	if resourceState == LabelStateTrue || resourceState == LabelStateFalse {
		resolution.Included = resourceState == LabelStateTrue
		resolution.Source = ScopeSourceResourceLabel
		return resolution
	}
	if !clusterScoped && namespaceState != LabelStateAbsent && namespaceState != LabelStateInvalid {
		resolution.Included = namespaceState == LabelStateTrue
		resolution.Source = ScopeSourceNamespaceLabel
		return resolution
	}
	return resolution
}

func labelState(labels map[string]string) LabelState {
	value, ok := labels[ManagedLabelKey]
	if !ok || value == "" {
		return LabelStateAbsent
	}
	switch value {
	case ManagedLabelValueTrue:
		return LabelStateTrue
	case ManagedLabelValueFalse:
		return LabelStateFalse
	default:
		return LabelStateInvalid
	}
}

// Resolve reads the local resource and namespace metadata and returns the
// effective managed-scope decision with provenance. Remote identities are
// rejected because the local Manager cannot inspect another cluster.
func (m *Manager) Resolve(ctx context.Context, resource ResourceIdentity) (Resolution, error) {
	if resource.ClusterID != "" {
		return Resolution{}, fmt.Errorf("scope: local Manager cannot resolve remote cluster %q; use a fleet adapter", resource.ClusterID)
	}
	clusterScoped := isClusterScoped(resource.Kind) || resource.Namespace == ""
	if _, known := kindToGroup[resource.Kind]; known {
		obj := &metav1.PartialObjectMetadata{}
		obj.SetGroupVersionKind(InferGVK(resource.Kind))
		lookupCtx, cancel := context.WithTimeout(ctx, scopeLookupTimeout)
		err := m.client.Get(lookupCtx, client.ObjectKey{Namespace: resource.Namespace, Name: resource.Name}, obj)
		cancel()
		if err == nil {
			return m.ResolveWithResourceLabels(ctx, resource, obj.GetLabels())
		}
		if !apierrors.IsNotFound(err) {
			log.FromContext(ctx).WithName("scope").Error(err, "Resource label check failed")
			return Resolution{}, fmt.Errorf("scope: resolve resource metadata: %w", err)
		}
	}
	return m.resolveWithResourceLabels(ctx, resource, nil, clusterScoped)
}

// ResolveWithResourceLabels resolves scope using metadata already fetched for
// the resource. A valid resource label is decisive, so Namespace metadata is
// read only when inheritance is required. This is used by bounded inventory
// readers to avoid a second resource read and to preserve explicit-label
// precedence when Namespace access is unavailable.
func (m *Manager) ResolveWithResourceLabels(ctx context.Context, resource ResourceIdentity, resourceLabels map[string]string) (Resolution, error) {
	if resource.ClusterID != "" {
		return Resolution{}, fmt.Errorf("scope: local Manager cannot resolve remote cluster %q; use a fleet adapter", resource.ClusterID)
	}
	return m.resolveWithResourceLabels(ctx, resource, resourceLabels, isClusterScoped(resource.Kind) || resource.Namespace == "")
}

func (m *Manager) resolveWithResourceLabels(ctx context.Context, resource ResourceIdentity, resourceLabels map[string]string, clusterScoped bool) (Resolution, error) {
	resourceState := labelState(resourceLabels)
	if resourceState == LabelStateTrue || resourceState == LabelStateFalse || clusterScoped {
		return resolveLabelStates(resourceState, LabelStateAbsent, clusterScoped), nil
	}

	namespaceState := LabelStateAbsent
	ns := &metav1.PartialObjectMetadata{}
	ns.SetGroupVersionKind(namespaceGVK)
	lookupCtx, cancel := context.WithTimeout(ctx, scopeLookupTimeout)
	err := m.client.Get(lookupCtx, client.ObjectKey{Name: resource.Namespace}, ns)
	cancel()
	if err == nil {
		namespaceState = labelState(ns.GetLabels())
	} else if !apierrors.IsNotFound(err) {
		return Resolution{}, fmt.Errorf("scope: resolve Namespace metadata: %w", err)
	}

	return resolveLabelStates(resourceState, namespaceState, clusterScoped), nil
}

// NewManager creates a new scope Manager with the given client reader.
// Both Gateway and RO pass their cached client (ctrlClient / mgr.GetClient()),
// which creates metadata-only informers for PartialObjectMetadata lookups.
func NewManager(c client.Reader) *Manager {
	return &Manager{client: c}
}

// IsManagedResource checks whether a Kubernetes resource is managed by Kubernaut.
//
// The 2-level hierarchy (ADR-053):
//  1. Check resource label: "true" → managed, "false" → unmanaged, missing/invalid → continue
//  2. Check namespace label: "true" → managed, "false" → unmanaged, missing → unmanaged
//
// For cluster-scoped resources (namespace==""), only the resource label is checked.
// Resource not found: falls through to namespace check.
// Namespace not found: returns false, nil (unmanaged, not an error).
//
// ClusterID must be empty — the local Manager cannot resolve remote clusters.
// For remote checks, use a fleet adapter (e.g., FederatedScopeChecker).
func (m *Manager) IsManagedResource(ctx context.Context, resource ResourceIdentity) (bool, error) {
	resolution, err := m.Resolve(ctx, resource)
	if err != nil {
		log.FromContext(ctx).WithName("scope").Error(err, "Failed to resolve resource scope")
		return false, err
	}
	return resolution.Included, nil
}

// InferGVK resolves a Kubernetes resource kind string to its GroupVersionKind
// using the static kindToGroup mapping. Exported so that other ScopeChecker
// implementations (e.g. pkg/fleet/scopecache.Client) can honor the
// ResourceIdentity contract documented on checker.go: Group/Version are
// optional and MUST be inferred from Kind when empty.
func InferGVK(kind string) schema.GroupVersionKind {
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

// IsClusterScopedKind returns true if the resource kind is known to be
// cluster-scoped. Used as a static heuristic when a RESTMapper is unavailable.
func IsClusterScopedKind(kind string) bool {
	return clusterScopedKinds[kind]
}
