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

// Package enricher provides Kubernetes context enrichment for signal processing.
//
// The K8sEnricher fetches Kubernetes resource details based on signal type,
// following a signal-driven enrichment strategy:
//
//   - Pod signals: Namespace + Pod + Node + OwnerChain context
//   - Deployment signals: Namespace + Deployment context
//   - StatefulSet signals: Namespace + StatefulSet context
//   - DaemonSet signals: Namespace + DaemonSet context
//   - ReplicaSet signals: Namespace + ReplicaSet context
//   - Service signals: Namespace + Service context
//   - Node signals: Node context only (no namespace)
//   - Unknown resource types: Namespace context only (graceful fallback)
//
// Design Decisions:
//   - DD-005 v2.0: Uses logr.Logger for unified logging
//   - DD-017: Standard depth fetching (hardcoded, no configuration)
//   - Graceful degradation: Returns partial context on non-critical failures
//
// Business Requirements:
//   - BR-SP-001: K8s Context Enrichment (<2s P95)
//
// Per IMPLEMENTATION_PLAN_V1.21.md Day 3 specification.
package enricher

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/cache"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/ownerchain"
)

// K8sEnricher fetches Kubernetes context for signal enrichment.
type K8sEnricher struct {
	client            client.Client
	logger            logr.Logger
	cache             *cache.TTLCache
	metrics           *metrics.Metrics
	timeout           time.Duration
	ownerChainBuilder *ownerchain.Builder // BR-SP-100: Full owner chain traversal
}

// NewK8sEnricher creates a new K8s context enricher.
// Per IMPLEMENTATION_PLAN_V1.21.md Day 3 specification.
//
// Panics if metrics is nil (metrics are mandatory for observability).
func NewK8sEnricher(c client.Client, logger logr.Logger, m *metrics.Metrics, timeout time.Duration) *K8sEnricher {
	if m == nil {
		panic("metrics cannot be nil: metrics are mandatory for observability")
	}
	return &K8sEnricher{
		client:            c,
		logger:            logger.WithName("k8s-enricher"),
		cache:             cache.NewTTLCache(5 * time.Minute),
		metrics:           m,
		ownerChainBuilder: ownerchain.NewBuilder(c, logger), // BR-SP-100: Full owner chain traversal
		timeout: timeout,
	}
}

// Enrich fetches Kubernetes context based on signal type.
// BR-SP-001: <2 seconds P95
//
// Standard Depth Strategy (no configuration):
//
//	Pod signal    → Namespace + Pod + Node + OwnerChain
//	Deploy signal → Namespace + Deployment
//	SS signal     → Namespace + StatefulSet
//	DS signal     → Namespace + DaemonSet
//	RS signal     → Namespace + ReplicaSet
//	Svc signal    → Namespace + Service
//	Node signal   → Node only (no namespace)
//	Unknown       → Namespace only (graceful fallback)
func (e *K8sEnricher) Enrich(ctx context.Context, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.KubernetesContext, error) {
	startTime := time.Now()
	// Get resource kind for metrics labeling (lowercase to match Prometheus conventions)
	resourceKind := "unknown"
	if signal.TargetResource.Kind != "" {
		resourceKind = strings.ToLower(signal.TargetResource.Kind)
	}
	defer func() {
		// DD-005: Record enrichment duration with actual resource kind
		// Metrics are validated at constructor time (non-nil guaranteed)
		e.metrics.EnrichmentDuration.WithLabelValues(resourceKind).Observe(time.Since(startTime).Seconds())
	}()

	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	// Check if context is already cancelled
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	result := &signalprocessingv1alpha1.KubernetesContext{}

	// Signal-driven enrichment based on resource kind
	switch signal.TargetResource.Kind {
	case "Pod":
		return e.enrichPodSignal(ctx, signal, result)
	case "Deployment":
		return e.enrichDeploymentSignal(ctx, signal, result)
	case "StatefulSet":
		return e.enrichStatefulSetSignal(ctx, signal, result)
	case "DaemonSet":
		return e.enrichDaemonSetSignal(ctx, signal, result)
	case "ReplicaSet":
		return e.enrichReplicaSetSignal(ctx, signal, result)
	case "Service":
		return e.enrichServiceSignal(ctx, signal, result)
	case "Node":
		return e.enrichNodeSignal(ctx, signal, result)
	default:
		// Graceful fallback: namespace only for unknown resource types
		return e.enrichNamespaceOnly(ctx, signal, result)
	}
}

// enrichPodSignal fetches Namespace + Pod + Node + OwnerChain.
// BR-SP-001: Sets DegradedMode=true if target pod not found
func (e *K8sEnricher) enrichPodSignal(ctx context.Context, signal *signalprocessingv1alpha1.SignalData, result *signalprocessingv1alpha1.KubernetesContext) (*signalprocessingv1alpha1.KubernetesContext, error) {
	// 1. Fetch namespace (required)
	ns, err := e.getNamespace(ctx, signal.TargetResource.Namespace)
	if err != nil {
		e.recordEnrichmentResult("failure")
		return nil, fmt.Errorf("failed to get namespace %s: %w", signal.TargetResource.Namespace, err)
	}
	e.populateNamespaceContext(result, ns)

	// 2. Fetch pod - enter degraded mode if not found (BR-SP-001)
	pod, err := e.getPod(ctx, signal.TargetResource.Namespace, signal.TargetResource.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// BR-SP-001: Enter degraded mode when target resource not found
			e.logger.Info("Target pod not found, entering degraded mode", "name", signal.TargetResource.Name)
			result.DegradedMode = true
			e.metrics.RecordEnrichmentError("not_found")  // DD-005: Record error metric
			e.recordEnrichmentResult("degraded")
			return result, nil
		}
		e.logger.Error(err, "Failed to fetch pod", "name", signal.TargetResource.Name)
		e.metrics.RecordEnrichmentError("api_error")  // DD-005: Record error metric
		e.recordEnrichmentResult("failure")
		return nil, fmt.Errorf("failed to fetch pod: %w", err)
	}
	result.Workload = &signalprocessingv1alpha1.WorkloadDetails{
		Kind:        "Pod",
		Name:        pod.Name,
		Labels:      ensureMap(pod.Labels),
		Annotations: ensureMap(pod.Annotations),
	}

	// 3. Build owner chain using full traversal (BR-SP-100)
	// DD-WORKFLOW-001 v1.8: Traverses ownerReferences up the chain (Pod → ReplicaSet → Deployment)
	if e.ownerChainBuilder != nil {
		ownerChain, err := e.ownerChainBuilder.Build(ctx, signal.TargetResource.Namespace, signal.TargetResource.Kind, signal.TargetResource.Name)
		if err != nil {
			e.logger.V(1).Info("Owner chain build failed", "error", err)
		} else {
			result.OwnerChain = ownerChain
		}
	}

	e.recordEnrichmentResult("success")
	return result, nil
}

// enrichDeploymentSignal fetches Namespace + Deployment.
// BR-SP-001: Sets DegradedMode=true if target deployment not found
func (e *K8sEnricher) enrichDeploymentSignal(ctx context.Context, signal *signalprocessingv1alpha1.SignalData, result *signalprocessingv1alpha1.KubernetesContext) (*signalprocessingv1alpha1.KubernetesContext, error) {
	// 1. Fetch namespace (required)
	ns, err := e.getNamespace(ctx, signal.TargetResource.Namespace)
	if err != nil {
		e.recordEnrichmentResult("failure")
		return nil, fmt.Errorf("failed to get namespace %s: %w", signal.TargetResource.Namespace, err)
	}
	e.populateNamespaceContext(result, ns)

	// 2. Fetch deployment - enter degraded mode if not found (BR-SP-001)
	deployment, err := e.getDeployment(ctx, signal.TargetResource.Namespace, signal.TargetResource.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			e.logger.Info("Target deployment not found, entering degraded mode", "name", signal.TargetResource.Name)
			result.DegradedMode = true
			e.metrics.RecordEnrichmentError("not_found")  // DD-005: Record error metric
			e.recordEnrichmentResult("degraded")
			return result, nil
		}
		e.logger.Error(err, "Failed to fetch deployment", "name", signal.TargetResource.Name)
		e.metrics.RecordEnrichmentError("api_error")  // DD-005: Record error metric
		e.recordEnrichmentResult("failure")
		return nil, fmt.Errorf("failed to fetch deployment: %w", err)
	}
	result.Workload = &signalprocessingv1alpha1.WorkloadDetails{
		Kind:        "Deployment",
		Name:        deployment.Name,
		Labels:      ensureMap(deployment.Labels),
		Annotations: ensureMap(deployment.Annotations),
	}

	e.recordEnrichmentResult("success")
	return result, nil
}

// enrichStatefulSetSignal fetches Namespace + StatefulSet.
// BR-SP-001: Sets DegradedMode=true if target statefulset not found
func (e *K8sEnricher) enrichStatefulSetSignal(ctx context.Context, signal *signalprocessingv1alpha1.SignalData, result *signalprocessingv1alpha1.KubernetesContext) (*signalprocessingv1alpha1.KubernetesContext, error) {
	// 1. Fetch namespace (required)
	ns, err := e.getNamespace(ctx, signal.TargetResource.Namespace)
	if err != nil {
		e.recordEnrichmentResult("failure")
		return nil, fmt.Errorf("failed to get namespace %s: %w", signal.TargetResource.Namespace, err)
	}
	e.populateNamespaceContext(result, ns)

	// 2. Fetch statefulset - enter degraded mode if not found (BR-SP-001)
	statefulset, err := e.getStatefulSet(ctx, signal.TargetResource.Namespace, signal.TargetResource.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			e.logger.Info("Target statefulset not found, entering degraded mode", "name", signal.TargetResource.Name)
			result.DegradedMode = true
			e.metrics.RecordEnrichmentError("not_found")  // DD-005: Record error metric
			e.recordEnrichmentResult("degraded")
			return result, nil
		}
		e.logger.Error(err, "Failed to fetch statefulset", "name", signal.TargetResource.Name)
		e.metrics.RecordEnrichmentError("api_error")  // DD-005: Record error metric
		e.recordEnrichmentResult("failure")
		return nil, fmt.Errorf("failed to fetch statefulset: %w", err)
	}
	result.Workload = &signalprocessingv1alpha1.WorkloadDetails{
		Kind:        "StatefulSet",
		Name:        statefulset.Name,
		Labels:      ensureMap(statefulset.Labels),
		Annotations: ensureMap(statefulset.Annotations),
	}

	e.recordEnrichmentResult("success")
	return result, nil
}

// enrichDaemonSetSignal fetches Namespace + DaemonSet.
// BR-SP-001: Sets DegradedMode=true if target daemonset not found
func (e *K8sEnricher) enrichDaemonSetSignal(ctx context.Context, signal *signalprocessingv1alpha1.SignalData, result *signalprocessingv1alpha1.KubernetesContext) (*signalprocessingv1alpha1.KubernetesContext, error) {
	// 1. Fetch namespace (required)
	ns, err := e.getNamespace(ctx, signal.TargetResource.Namespace)
	if err != nil {
		e.recordEnrichmentResult("failure")
		return nil, fmt.Errorf("failed to get namespace %s: %w", signal.TargetResource.Namespace, err)
	}
	e.populateNamespaceContext(result, ns)

	// 2. Fetch daemonset - enter degraded mode if not found (BR-SP-001)
	daemonset, err := e.getDaemonSet(ctx, signal.TargetResource.Namespace, signal.TargetResource.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			e.logger.Info("Target daemonset not found, entering degraded mode", "name", signal.TargetResource.Name)
			result.DegradedMode = true
			e.metrics.RecordEnrichmentError("not_found")  // DD-005: Record error metric
			e.recordEnrichmentResult("degraded")
			return result, nil
		}
		e.logger.Error(err, "Failed to fetch daemonset", "name", signal.TargetResource.Name)
		e.metrics.RecordEnrichmentError("api_error")  // DD-005: Record error metric
		e.recordEnrichmentResult("failure")
		return nil, fmt.Errorf("failed to fetch daemonset: %w", err)
	}
	result.Workload = &signalprocessingv1alpha1.WorkloadDetails{
		Kind:        "DaemonSet",
		Name:        daemonset.Name,
		Labels:      ensureMap(daemonset.Labels),
		Annotations: ensureMap(daemonset.Annotations),
	}

	e.recordEnrichmentResult("success")
	return result, nil
}

// enrichReplicaSetSignal fetches Namespace + ReplicaSet.
// BR-SP-001: Sets DegradedMode=true if target replicaset not found
func (e *K8sEnricher) enrichReplicaSetSignal(ctx context.Context, signal *signalprocessingv1alpha1.SignalData, result *signalprocessingv1alpha1.KubernetesContext) (*signalprocessingv1alpha1.KubernetesContext, error) {
	// 1. Fetch namespace (required)
	ns, err := e.getNamespace(ctx, signal.TargetResource.Namespace)
	if err != nil {
		e.recordEnrichmentResult("failure")
		return nil, fmt.Errorf("failed to get namespace %s: %w", signal.TargetResource.Namespace, err)
	}
	e.populateNamespaceContext(result, ns)

	// 2. Fetch replicaset - enter degraded mode if not found (BR-SP-001)
	replicaset, err := e.getReplicaSet(ctx, signal.TargetResource.Namespace, signal.TargetResource.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			e.logger.Info("Target replicaset not found, entering degraded mode", "name", signal.TargetResource.Name)
			result.DegradedMode = true
			e.metrics.RecordEnrichmentError("not_found")  // DD-005: Record error metric
			e.recordEnrichmentResult("degraded")
			return result, nil
		}
		e.logger.Error(err, "Failed to fetch replicaset", "name", signal.TargetResource.Name)
		e.metrics.RecordEnrichmentError("api_error")  // DD-005: Record error metric
		e.recordEnrichmentResult("failure")
		return nil, fmt.Errorf("failed to fetch replicaset: %w", err)
	}
	result.Workload = &signalprocessingv1alpha1.WorkloadDetails{
		Kind:        "ReplicaSet",
		Name:        replicaset.Name,
		Labels:      ensureMap(replicaset.Labels),
		Annotations: ensureMap(replicaset.Annotations),
	}

	e.recordEnrichmentResult("success")
	return result, nil
}

// enrichServiceSignal fetches Namespace + Service.
// BR-SP-001: Sets DegradedMode=true if target service not found
func (e *K8sEnricher) enrichServiceSignal(ctx context.Context, signal *signalprocessingv1alpha1.SignalData, result *signalprocessingv1alpha1.KubernetesContext) (*signalprocessingv1alpha1.KubernetesContext, error) {
	// 1. Fetch namespace (required)
	ns, err := e.getNamespace(ctx, signal.TargetResource.Namespace)
	if err != nil {
		e.recordEnrichmentResult("failure")
		return nil, fmt.Errorf("failed to get namespace %s: %w", signal.TargetResource.Namespace, err)
	}
	e.populateNamespaceContext(result, ns)

	// 2. Fetch service - enter degraded mode if not found (BR-SP-001)
	service, err := e.getService(ctx, signal.TargetResource.Namespace, signal.TargetResource.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			e.logger.Info("Target service not found, entering degraded mode", "name", signal.TargetResource.Name)
			result.DegradedMode = true
			e.metrics.RecordEnrichmentError("not_found")  // DD-005: Record error metric
			e.recordEnrichmentResult("degraded")
			return result, nil
		}
		e.logger.Error(err, "Failed to fetch service", "name", signal.TargetResource.Name)
		e.metrics.RecordEnrichmentError("api_error")  // DD-005: Record error metric
		e.recordEnrichmentResult("failure")
		return nil, fmt.Errorf("failed to fetch service: %w", err)
	}
	result.Workload = &signalprocessingv1alpha1.WorkloadDetails{
		Kind:        "Service",
		Name:        service.Name,
		Labels:      ensureMap(service.Labels),
		Annotations: ensureMap(service.Annotations),
	}

	e.recordEnrichmentResult("success")
	return result, nil
}

// enrichNodeSignal fetches Node only (no namespace for node signals).
func (e *K8sEnricher) enrichNodeSignal(ctx context.Context, signal *signalprocessingv1alpha1.SignalData, result *signalprocessingv1alpha1.KubernetesContext) (*signalprocessingv1alpha1.KubernetesContext, error) {
	// Node signals have no namespace
	node, err := e.getNode(ctx, signal.TargetResource.Name)
	if err != nil {
		e.recordEnrichmentResult("failure")
		return nil, fmt.Errorf("failed to get node %s: %w", signal.TargetResource.Name, err)
	}
	result.Workload = &signalprocessingv1alpha1.WorkloadDetails{
		Kind:        "Node",
		Name:        node.Name,
		Labels:      ensureMap(node.Labels),
		Annotations: ensureMap(node.Annotations),
	}

	e.recordEnrichmentResult("success")
	return result, nil
}

// enrichNamespaceOnly fetches namespace only for unknown resource types.
func (e *K8sEnricher) enrichNamespaceOnly(ctx context.Context, signal *signalprocessingv1alpha1.SignalData, result *signalprocessingv1alpha1.KubernetesContext) (*signalprocessingv1alpha1.KubernetesContext, error) {
	ns, err := e.getNamespace(ctx, signal.TargetResource.Namespace)
	if err != nil {
		e.recordEnrichmentResult("failure")
		return nil, fmt.Errorf("failed to get namespace %s: %w", signal.TargetResource.Namespace, err)
	}
	e.populateNamespaceContext(result, ns)

	e.recordEnrichmentResult("success")
	return result, nil
}

// ensureMap returns the input map if non-nil, otherwise returns an empty map.
func ensureMap(m map[string]string) map[string]string {
	if m == nil {
		return make(map[string]string)
	}
	return m
}

// getNamespace fetches a namespace by name with caching.
func (e *K8sEnricher) getNamespace(ctx context.Context, name string) (*corev1.Namespace, error) {
	// Check cache first
	cacheKey := "ns:" + name
	if cached, ok := e.cache.Get(cacheKey); ok {
		return cached.(*corev1.Namespace), nil
	}

	ns := &corev1.Namespace{}
	if err := e.client.Get(ctx, types.NamespacedName{Name: name}, ns); err != nil {
		return nil, err
	}

	// Cache the result
	e.cache.Set(cacheKey, ns)
	return ns, nil
}

// getPod fetches a pod by namespace and name.
func (e *K8sEnricher) getPod(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
	pod := &corev1.Pod{}
	if err := e.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, pod); err != nil {
		return nil, err
	}
	return pod, nil
}

// getNode fetches a node by name.
func (e *K8sEnricher) getNode(ctx context.Context, name string) (*corev1.Node, error) {
	node := &corev1.Node{}
	if err := e.client.Get(ctx, types.NamespacedName{Name: name}, node); err != nil {
		return nil, err
	}
	return node, nil
}

// getDeployment fetches a deployment by namespace and name.
func (e *K8sEnricher) getDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error) {
	deployment := &appsv1.Deployment{}
	if err := e.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, deployment); err != nil {
		return nil, err
	}
	return deployment, nil
}

// getStatefulSet fetches a statefulset by namespace and name.
func (e *K8sEnricher) getStatefulSet(ctx context.Context, namespace, name string) (*appsv1.StatefulSet, error) {
	statefulset := &appsv1.StatefulSet{}
	if err := e.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, statefulset); err != nil {
		return nil, err
	}
	return statefulset, nil
}

// getDaemonSet fetches a daemonset by namespace and name.
func (e *K8sEnricher) getDaemonSet(ctx context.Context, namespace, name string) (*appsv1.DaemonSet, error) {
	daemonset := &appsv1.DaemonSet{}
	if err := e.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, daemonset); err != nil {
		return nil, err
	}
	return daemonset, nil
}

// getReplicaSet fetches a replicaset by namespace and name.
func (e *K8sEnricher) getReplicaSet(ctx context.Context, namespace, name string) (*appsv1.ReplicaSet, error) {
	replicaset := &appsv1.ReplicaSet{}
	if err := e.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, replicaset); err != nil {
		return nil, err
	}
	return replicaset, nil
}

// getService fetches a service by namespace and name.
func (e *K8sEnricher) getService(ctx context.Context, namespace, name string) (*corev1.Service, error) {
	service := &corev1.Service{}
	if err := e.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, service); err != nil {
		return nil, err
	}
	return service, nil
}

// populateNamespaceContext populates the Namespace struct with namespace details.
func (e *K8sEnricher) populateNamespaceContext(result *signalprocessingv1alpha1.KubernetesContext, ns *corev1.Namespace) {
	result.Namespace = &signalprocessingv1alpha1.NamespaceContext{
		Name:        ns.Name,
		Labels:      ensureMap(ns.Labels),
		Annotations: ensureMap(ns.Annotations),
	}
}

// recordEnrichmentResult records the enrichment result metric.
// Per plan: direct field access to EnrichmentTotal.WithLabelValues(result).Inc()
// Metrics are validated at constructor time (non-nil guaranteed)
func (e *K8sEnricher) recordEnrichmentResult(result string) {
	e.metrics.EnrichmentTotal.WithLabelValues(result).Inc()
}
