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

// Package ownerchain provides K8s ownership chain traversal for enrichment.
//
// # Business Requirements
//
// BR-SP-100: OwnerChain Traversal - Build K8s ownership chain from signal source resource.
//
// # Design Decisions
//
// DD-WORKFLOW-001 v1.8: OwnerChainEntry schema requires Namespace, Kind, Name ONLY.
// Do NOT include APIVersion or UID - not used by KA validation.
//
// # Algorithm
//
// The Build method traverses K8s ownerReferences following these rules:
//  1. Start from source resource (NOT added to chain - owners only)
//  2. Follow first ownerReference with controller: true at each level
//  3. Stop at MaxOwnerChainDepth (5) or when no more owners exist
//  4. On K8s API errors, return partial chain (graceful degradation)
//
// # Example
//
// For a Pod owned by a ReplicaSet owned by a Deployment:
//
//	chain, _ := builder.Build(ctx, "prod", "Pod", "api-pod-abc12")
//	// chain = [
//	//   {Namespace: "prod", Kind: "ReplicaSet", Name: "api-rs-xyz"},
//	//   {Namespace: "prod", Kind: "Deployment", Name: "api"},
//	// ]
package ownerchain

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// MaxOwnerChainDepth per BR-SP-100: Stop at max depth 5
const MaxOwnerChainDepth = 5

// ErrNoControllerOwner indicates the fetched resource has no owner reference
// with controller: true. Issue #1674: typed sentinel replacing the previous
// ambiguous (nil, nil) return from getControllerOwner — this is the expected
// "chain complete" signal, not a K8s API error.
var ErrNoControllerOwner = errors.New("no controller owner reference found")

// Builder constructs the K8s ownership chain.
// Used for K8s context enrichment in the SignalProcessing pipeline.
// DD-WORKFLOW-001 v1.8: Namespace, Kind, Name ONLY (no APIVersion/UID)
// BR-SP-100: Max depth 5, owners only (source not included)
type Builder struct {
	client client.Reader
	logger logr.Logger
}

// NewBuilder creates a new OwnerChain builder.
// Per BR-SP-100: K8s ownerReference traversal for enrichment.
// Accepts client.Reader (not client.Client) so the builder works with both
// local K8s clients and remote MCP-backed readers (BR-INTEGRATION-054).
func NewBuilder(c client.Reader, logger logr.Logger) *Builder {
	return &Builder{
		client: c,
		logger: logger.WithName("ownerchain"),
	}
}

// Build traverses K8s ownerReferences to construct ownership chain.
// Algorithm: Follow first `controller: true` ownerReference at each level.
// Example: Pod → ReplicaSet → Deployment (Pod is NOT in chain, only owners)
// DD-WORKFLOW-001 v1.8: Chain contains OWNERS ONLY
// BR-SP-100: Max depth 5
func (b *Builder) Build(ctx context.Context, namespace, kind, name string) ([]signalprocessingv1alpha1.OwnerChainEntry, error) {
	return b.build(ctx, namespace, "", kind, name, false)
}

// BuildWithAPIVersion traverses an owner chain using an explicit source
// apiVersion. It is used for remote readers where Kind-only GVK inference is
// unsafe. Each owner reference must carry its own apiVersion before it can be
// used for the next lookup.
func (b *Builder) BuildWithAPIVersion(ctx context.Context, namespace, apiVersion, kind, name string) ([]signalprocessingv1alpha1.OwnerChainEntry, error) {
	return b.build(ctx, namespace, apiVersion, kind, name, true)
}

func (b *Builder) build(ctx context.Context, namespace, apiVersion, kind, name string, requireExplicitGVK bool) ([]signalprocessingv1alpha1.OwnerChainEntry, error) {
	var chain []signalprocessingv1alpha1.OwnerChainEntry

	currentNamespace := namespace
	currentAPIVersion := apiVersion
	currentKind := kind
	currentName := name

	// NOTE: Source resource is NOT added to chain (DD-WORKFLOW-001 v1.8)
	// Chain contains owners only

	// Traverse ownerReferences (max 5 levels per BR-SP-100)
	for i := 0; i < MaxOwnerChainDepth; i++ {
		b.logger.V(1).Info("Traversing owner chain level",
			"level", i, "kind", currentKind, "name", currentName, "namespace", currentNamespace)

		ownerRef, err := b.getControllerOwnerWithGVK(ctx, currentNamespace, currentAPIVersion, currentKind, currentName, requireExplicitGVK)
		if err != nil {
			if errors.Is(err, ErrNoControllerOwner) {
				b.logger.V(1).Info("No controller owner found, chain complete",
					"kind", currentKind, "name", currentName)
				break // No more owners - chain complete
			}
			// Log error but return partial chain (graceful degradation)
			// Per ERROR_HANDLING_PHILOSOPHY.md: Category E - Partial Data
			b.logger.Info("Error fetching owner, returning partial chain",
				"error", err, "currentKind", currentKind, "currentName", currentName)
			break
		}
		b.logger.V(1).Info("Found controller owner",
			"ownerKind", ownerRef.Kind, "ownerName", ownerRef.Name)

		// Cluster-scoped resources have empty namespace
		ownerNamespace := currentNamespace
		if isClusterScoped(ownerRef.Kind) {
			ownerNamespace = ""
		}

		// DD-WORKFLOW-001 v1.8: Namespace, Kind, Name ONLY
		chain = append(chain, signalprocessingv1alpha1.OwnerChainEntry{
			Namespace: ownerNamespace,
			Kind:      ownerRef.Kind,
			Name:      ownerRef.Name,
		})

		currentNamespace = ownerNamespace
		currentAPIVersion = ownerRef.APIVersion
		currentKind = ownerRef.Kind
		currentName = ownerRef.Name
	}

	b.logger.Info("Owner chain built",
		"length", len(chain),
		"source", kind+"/"+name)

	return chain, nil
}

// getControllerOwner fetches a resource and returns its controller owner reference.
// Returns ErrNoControllerOwner if no controller owner found.
// Returns a different error for K8s API errors (RBAC, timeout, not found).
// getControllerOwner preserves the local compatibility seam used by existing
// callers. Remote traversal uses getControllerOwnerWithGVK below so it cannot
// infer a GVK from Kind alone.
func (b *Builder) getControllerOwner(ctx context.Context, namespace, kind, name string) (*metav1.OwnerReference, error) {
	return b.getControllerOwnerWithGVK(ctx, namespace, "", kind, name, false)
}

func (b *Builder) getControllerOwnerWithGVK(ctx context.Context, namespace, apiVersion, kind, name string, requireExplicitGVK bool) (*metav1.OwnerReference, error) {
	// Check context cancellation first
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Determine GVK for unstructured fetch
	gvk, err := gvkForLookup(apiVersion, kind, requireExplicitGVK)
	if err != nil {
		return nil, err
	}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)

	key := client.ObjectKey{Namespace: namespace, Name: name}
	if err := b.client.Get(ctx, key, obj); err != nil {
		return nil, err // Caller handles error (graceful degradation)
	}

	// Find controller owner (controller: true)
	for _, ref := range obj.GetOwnerReferences() {
		if ref.Controller != nil && *ref.Controller {
			return &ref, nil
		}
	}

	return nil, ErrNoControllerOwner
}

func gvkForLookup(apiVersion, kind string, requireExplicitGVK bool) (schema.GroupVersionKind, error) {
	if apiVersion == "" {
		if requireExplicitGVK {
			return schema.GroupVersionKind{}, fmt.Errorf("ownerchain: explicit apiVersion is required for %q", kind)
		}
		return getGVKForKind(kind), nil
	}
	groupVersion, err := schema.ParseGroupVersion(apiVersion)
	if err != nil || groupVersion.Version == "" {
		if err != nil {
			return schema.GroupVersionKind{}, fmt.Errorf("ownerchain: invalid apiVersion %q: %w", apiVersion, err)
		}
		return schema.GroupVersionKind{}, fmt.Errorf("ownerchain: invalid apiVersion %q", apiVersion)
	}
	return groupVersion.WithKind(kind), nil
}

// getGVKForKind returns the GroupVersionKind for a given K8s resource kind.
func getGVKForKind(kind string) schema.GroupVersionKind {
	// Map of kinds to their API groups
	kindToGroup := map[string]string{
		"Pod":         "",
		"Node":        "",
		"Service":     "",
		"ConfigMap":   "",
		"Secret":      "",
		"Namespace":   "",
		"ReplicaSet":  "apps",
		"Deployment":  "apps",
		"StatefulSet": "apps",
		"DaemonSet":   "apps",
		"Job":         "batch",
		"CronJob":     "batch",
	}

	group := ""
	if g, ok := kindToGroup[kind]; ok {
		group = g
	}

	return schema.GroupVersionKind{
		Group:   group,
		Version: "v1",
		Kind:    kind,
	}
}

// isClusterScoped returns true if the resource kind is cluster-scoped.
func isClusterScoped(kind string) bool {
	clusterScoped := map[string]bool{
		"Node":               true,
		"PersistentVolume":   true,
		"Namespace":          true,
		"ClusterRole":        true,
		"ClusterRoleBinding": true,
	}
	return clusterScoped[kind]
}
