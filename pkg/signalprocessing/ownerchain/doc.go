// Package ownerchain provides K8s ownership chain traversal for Signal Processing.
//
// # Purpose
//
// The OwnerChain builder traverses Kubernetes ownerReferences to construct
// a complete ownership chain from any resource back to its root owner.
// This chain is used by HolmesGPT-API to validate that DetectedLabels
// are applicable to the resource being analyzed.
//
// # Business Requirement
//
// BR-SP-100: OwnerChain Traversal
//
// # Design Decision
//
// DD-WORKFLOW-001 v1.8: SignalProcessing populates OwnerChain at incident time
// by traversing metadata.ownerReferences from the signal source resource.
//
// # Algorithm
//
// 1. Start with source resource (e.g., Pod)
// 2. Find ownerReference with controller=true
// 3. Add owner to chain, repeat from step 2
// 4. Stop when no more controller owners or max depth (10) reached
//
// # Example
//
// For a Pod owned by a ReplicaSet owned by a Deployment:
//
//	chain := []OwnerChainEntry{
//	    {Namespace: "prod", Kind: "Pod", Name: "api-7d8f9c6b5-abc"},
//	    {Namespace: "prod", Kind: "ReplicaSet", Name: "api-7d8f9c6b5"},
//	    {Namespace: "prod", Kind: "Deployment", Name: "api"},
//	}
//
// # Usage
//
//	builder := ownerchain.NewBuilder(k8sClient, logger)
//	chain, err := builder.Build(ctx, "prod", "Pod", "api-7d8f9c6b5-abc")
//
// # Cluster-Scoped Resources
//
// For cluster-scoped resources like Nodes, the Namespace field is empty:
//
//	chain := []OwnerChainEntry{
//	    {Namespace: "", Kind: "Node", Name: "worker-1"},
//	}
//
// # Error Handling
//
// The builder uses graceful degradation:
//   - If a resource is not found, the chain stops at that point
//   - If an unknown resource kind is encountered, traversal stops
//   - Errors are logged but do not fail the operation
//
// # Thread Safety
//
// The Builder is safe for concurrent use. Each Build() call is independent.
package ownerchain
