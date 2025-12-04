// Package ownerchain provides K8s ownership chain traversal for Signal Processing.
// BR-SP-100: OwnerChain Traversal
// DD-WORKFLOW-001 v1.8: SignalProcessing traverses ownerReferences to build chain
package ownerchain

import (
	"context"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

const (
	// MaxChainDepth prevents infinite loops in cyclic ownership (shouldn't happen in K8s but safety first)
	MaxChainDepth = 10
)

// Builder constructs the K8s ownership chain.
// Used by HolmesGPT-API to validate DetectedLabels applicability.
// See: DD-WORKFLOW-001 v1.8
type Builder struct {
	client client.Client
	logger logr.Logger
}

// NewBuilder creates a new OwnerChain builder.
func NewBuilder(c client.Client, logger logr.Logger) *Builder {
	return &Builder{
		client: c,
		logger: logger.WithName("ownerchain"),
	}
}

// Build traverses K8s ownerReferences to construct ownership chain.
// Algorithm: Follow first `controller: true` ownerReference at each level.
// Example: Pod → ReplicaSet → Deployment
// Returns chain with source resource as first entry.
func (b *Builder) Build(ctx context.Context, namespace, kind, name string) ([]sharedtypes.OwnerChainEntry, error) {
	var chain []sharedtypes.OwnerChainEntry

	currentNamespace := namespace
	currentKind := kind
	currentName := name

	// Add source resource as first entry
	chain = append(chain, sharedtypes.OwnerChainEntry{
		Namespace: currentNamespace,
		Kind:      currentKind,
		Name:      currentName,
	})

	// Traverse ownerReferences (max 10 levels to prevent infinite loops)
	for i := 0; i < MaxChainDepth-1; i++ { // -1 because source already added
		ownerRef, err := b.getControllerOwner(ctx, currentNamespace, currentKind, currentName)
		if err != nil {
			b.logger.V(1).Info("Error getting owner, stopping chain traversal",
				"error", err, "kind", currentKind, "name", currentName)
			break
		}
		if ownerRef == nil {
			// No more owners
			break
		}

		// Cluster-scoped resources have empty namespace
		ownerNamespace := currentNamespace
		if isClusterScoped(ownerRef.Kind) {
			ownerNamespace = ""
		}

		chain = append(chain, sharedtypes.OwnerChainEntry{
			Namespace: ownerNamespace,
			Kind:      ownerRef.Kind,
			Name:      ownerRef.Name,
		})

		currentNamespace = ownerNamespace
		currentKind = ownerRef.Kind
		currentName = ownerRef.Name
	}

	b.logger.Info("Owner chain built",
		"length", len(chain),
		"source", kind+"/"+name)

	return chain, nil
}

// getControllerOwner fetches a resource and returns its controller owner reference.
// Returns nil if no controller owner found.
func (b *Builder) getControllerOwner(ctx context.Context, namespace, kind, name string) (*metav1.OwnerReference, error) {
	// Get the resource's metadata to find ownerReferences
	var ownerRefs []metav1.OwnerReference

	switch kind {
	case "Pod":
		pod := &corev1.Pod{}
		if err := b.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, pod); err != nil {
			return nil, nil // Resource not found, graceful degradation
		}
		ownerRefs = pod.OwnerReferences

	case "ReplicaSet":
		rs := &appsv1.ReplicaSet{}
		if err := b.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, rs); err != nil {
			return nil, nil
		}
		ownerRefs = rs.OwnerReferences

	case "Deployment":
		deploy := &appsv1.Deployment{}
		if err := b.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, deploy); err != nil {
			return nil, nil
		}
		ownerRefs = deploy.OwnerReferences

	case "StatefulSet":
		sts := &appsv1.StatefulSet{}
		if err := b.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, sts); err != nil {
			return nil, nil
		}
		ownerRefs = sts.OwnerReferences

	case "DaemonSet":
		ds := &appsv1.DaemonSet{}
		if err := b.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, ds); err != nil {
			return nil, nil
		}
		ownerRefs = ds.OwnerReferences

	case "Node":
		// Nodes are cluster-scoped and don't have owners
		return nil, nil

	case "ConfigMap":
		cm := &corev1.ConfigMap{}
		if err := b.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, cm); err != nil {
			return nil, nil
		}
		ownerRefs = cm.OwnerReferences

	default:
		// Unknown kind, can't traverse
		b.logger.V(1).Info("Unknown kind, cannot traverse", "kind", kind)
		return nil, nil
	}

	// Find the controller owner (controller: true)
	for i := range ownerRefs {
		if ownerRefs[i].Controller != nil && *ownerRefs[i].Controller {
			return &ownerRefs[i], nil
		}
	}

	// No controller owner found
	return nil, nil
}

// isClusterScoped returns true for cluster-scoped resource kinds.
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
