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
	"sync"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
)

// K8sEnricher fetches Kubernetes context for signal enrichment.
type K8sEnricher struct {
	client  client.Client
	logger  logr.Logger
	metrics *metrics.Metrics
	timeout time.Duration
	cache   *ttlCache
}

// ttlCache provides simple TTL-based caching for namespace lookups.
type ttlCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	ttl     time.Duration
}

type cacheEntry struct {
	value     interface{}
	expiresAt time.Time
}

func newTTLCache(ttl time.Duration) *ttlCache {
	return &ttlCache{
		entries: make(map[string]*cacheEntry),
		ttl:     ttl,
	}
}

func (c *ttlCache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.value, true
}

func (c *ttlCache) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = &cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// NewK8sEnricher creates a new K8s context enricher.
func NewK8sEnricher(c client.Client, logger logr.Logger, m *metrics.Metrics, timeout time.Duration) *K8sEnricher {
	return &K8sEnricher{
		client:  c,
		logger:  logger.WithName("k8s-enricher"),
		metrics: m,
		timeout: timeout,
		cache:   newTTLCache(5 * time.Minute),
	}
}

// Enrich fetches Kubernetes context based on signal type.
func (e *K8sEnricher) Enrich(ctx context.Context, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.KubernetesContext, error) {
	startTime := time.Now()
	defer func() {
		if e.metrics != nil {
			e.metrics.ObserveProcessingDuration("enrichment", time.Since(startTime).Seconds())
		}
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
func (e *K8sEnricher) enrichPodSignal(ctx context.Context, signal *signalprocessingv1alpha1.SignalData, result *signalprocessingv1alpha1.KubernetesContext) (*signalprocessingv1alpha1.KubernetesContext, error) {
	// 1. Fetch namespace (required)
	ns, err := e.getNamespace(ctx, signal.TargetResource.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace %s: %w", signal.TargetResource.Namespace, err)
	}
	result.NamespaceLabels = ensureMap(ns.Labels)
	result.NamespaceAnnotations = ensureMap(ns.Annotations)

	// 2. Fetch pod (optional - continue with partial context if not found)
	pod, err := e.getPod(ctx, signal.TargetResource.Namespace, signal.TargetResource.Name)
	if err != nil {
		e.logger.Info("Pod not found, continuing with partial context", "error", err)
		return result, nil
	}
	result.Pod = e.convertPodDetails(pod)

	// 3. Build owner chain from pod's owner references
	result.OwnerChain = e.buildOwnerChain(pod.OwnerReferences)

	// 4. Fetch node where pod runs (optional)
	if pod.Spec.NodeName != "" {
		node, err := e.getNode(ctx, pod.Spec.NodeName)
		if err == nil {
			result.Node = e.convertNodeDetails(node)
		}
	}

	return result, nil
}

// enrichDeploymentSignal fetches Namespace + Deployment.
func (e *K8sEnricher) enrichDeploymentSignal(ctx context.Context, signal *signalprocessingv1alpha1.SignalData, result *signalprocessingv1alpha1.KubernetesContext) (*signalprocessingv1alpha1.KubernetesContext, error) {
	// 1. Fetch namespace (required)
	ns, err := e.getNamespace(ctx, signal.TargetResource.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace %s: %w", signal.TargetResource.Namespace, err)
	}
	result.NamespaceLabels = ensureMap(ns.Labels)
	result.NamespaceAnnotations = ensureMap(ns.Annotations)

	// 2. Fetch deployment (optional)
	deployment, err := e.getDeployment(ctx, signal.TargetResource.Namespace, signal.TargetResource.Name)
	if err != nil {
		e.logger.Info("Deployment not found, continuing with namespace only", "error", err)
		return result, nil
	}
	result.Deployment = e.convertDeploymentDetails(deployment)

	return result, nil
}

// enrichStatefulSetSignal fetches Namespace + StatefulSet.
func (e *K8sEnricher) enrichStatefulSetSignal(ctx context.Context, signal *signalprocessingv1alpha1.SignalData, result *signalprocessingv1alpha1.KubernetesContext) (*signalprocessingv1alpha1.KubernetesContext, error) {
	// 1. Fetch namespace (required)
	ns, err := e.getNamespace(ctx, signal.TargetResource.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace %s: %w", signal.TargetResource.Namespace, err)
	}
	result.NamespaceLabels = ensureMap(ns.Labels)
	result.NamespaceAnnotations = ensureMap(ns.Annotations)

	// 2. Fetch statefulset (optional)
	statefulset, err := e.getStatefulSet(ctx, signal.TargetResource.Namespace, signal.TargetResource.Name)
	if err != nil {
		e.logger.Info("StatefulSet not found, continuing with namespace only", "error", err)
		return result, nil
	}
	result.StatefulSet = e.convertStatefulSetDetails(statefulset)

	return result, nil
}

// enrichDaemonSetSignal fetches Namespace + DaemonSet.
func (e *K8sEnricher) enrichDaemonSetSignal(ctx context.Context, signal *signalprocessingv1alpha1.SignalData, result *signalprocessingv1alpha1.KubernetesContext) (*signalprocessingv1alpha1.KubernetesContext, error) {
	// 1. Fetch namespace (required)
	ns, err := e.getNamespace(ctx, signal.TargetResource.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace %s: %w", signal.TargetResource.Namespace, err)
	}
	result.NamespaceLabels = ensureMap(ns.Labels)
	result.NamespaceAnnotations = ensureMap(ns.Annotations)

	// 2. Fetch daemonset (optional)
	daemonset, err := e.getDaemonSet(ctx, signal.TargetResource.Namespace, signal.TargetResource.Name)
	if err != nil {
		e.logger.Info("DaemonSet not found, continuing with namespace only", "error", err)
		return result, nil
	}
	result.DaemonSet = e.convertDaemonSetDetails(daemonset)

	return result, nil
}

// enrichReplicaSetSignal fetches Namespace + ReplicaSet.
func (e *K8sEnricher) enrichReplicaSetSignal(ctx context.Context, signal *signalprocessingv1alpha1.SignalData, result *signalprocessingv1alpha1.KubernetesContext) (*signalprocessingv1alpha1.KubernetesContext, error) {
	// 1. Fetch namespace (required)
	ns, err := e.getNamespace(ctx, signal.TargetResource.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace %s: %w", signal.TargetResource.Namespace, err)
	}
	result.NamespaceLabels = ensureMap(ns.Labels)
	result.NamespaceAnnotations = ensureMap(ns.Annotations)

	// 2. Fetch replicaset (optional)
	replicaset, err := e.getReplicaSet(ctx, signal.TargetResource.Namespace, signal.TargetResource.Name)
	if err != nil {
		e.logger.Info("ReplicaSet not found, continuing with namespace only", "error", err)
		return result, nil
	}
	result.ReplicaSet = e.convertReplicaSetDetails(replicaset)

	return result, nil
}

// enrichServiceSignal fetches Namespace + Service.
func (e *K8sEnricher) enrichServiceSignal(ctx context.Context, signal *signalprocessingv1alpha1.SignalData, result *signalprocessingv1alpha1.KubernetesContext) (*signalprocessingv1alpha1.KubernetesContext, error) {
	// 1. Fetch namespace (required)
	ns, err := e.getNamespace(ctx, signal.TargetResource.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace %s: %w", signal.TargetResource.Namespace, err)
	}
	result.NamespaceLabels = ensureMap(ns.Labels)
	result.NamespaceAnnotations = ensureMap(ns.Annotations)

	// 2. Fetch service (optional)
	service, err := e.getService(ctx, signal.TargetResource.Namespace, signal.TargetResource.Name)
	if err != nil {
		e.logger.Info("Service not found, continuing with namespace only", "error", err)
		return result, nil
	}
	result.Service = e.convertServiceDetails(service)

	return result, nil
}

// enrichNodeSignal fetches Node only (no namespace for node signals).
func (e *K8sEnricher) enrichNodeSignal(ctx context.Context, signal *signalprocessingv1alpha1.SignalData, result *signalprocessingv1alpha1.KubernetesContext) (*signalprocessingv1alpha1.KubernetesContext, error) {
	// Node signals have no namespace
	node, err := e.getNode(ctx, signal.TargetResource.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get node %s: %w", signal.TargetResource.Name, err)
	}
	result.Node = e.convertNodeDetails(node)

	return result, nil
}

// enrichNamespaceOnly fetches namespace only for unknown resource types.
func (e *K8sEnricher) enrichNamespaceOnly(ctx context.Context, signal *signalprocessingv1alpha1.SignalData, result *signalprocessingv1alpha1.KubernetesContext) (*signalprocessingv1alpha1.KubernetesContext, error) {
	ns, err := e.getNamespace(ctx, signal.TargetResource.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace %s: %w", signal.TargetResource.Namespace, err)
	}
	result.NamespaceLabels = ensureMap(ns.Labels)
	result.NamespaceAnnotations = ensureMap(ns.Annotations)

	return result, nil
}

// buildOwnerChain builds the owner chain from owner references.
// Returns controller owner first if present.
func (e *K8sEnricher) buildOwnerChain(ownerRefs []metav1.OwnerReference) []signalprocessingv1alpha1.OwnerChainEntry {
	if len(ownerRefs) == 0 {
		return nil
	}

	chain := make([]signalprocessingv1alpha1.OwnerChainEntry, 0, len(ownerRefs))

	// Find controller owner first
	for _, ref := range ownerRefs {
		if ref.Controller != nil && *ref.Controller {
			chain = append(chain, signalprocessingv1alpha1.OwnerChainEntry{
				Kind:       ref.Kind,
				Name:       ref.Name,
				APIVersion: ref.APIVersion,
				UID:        string(ref.UID),
			})
			break
		}
	}

	// Add non-controller owners
	for _, ref := range ownerRefs {
		if ref.Controller == nil || !*ref.Controller {
			chain = append(chain, signalprocessingv1alpha1.OwnerChainEntry{
				Kind:       ref.Kind,
				Name:       ref.Name,
				APIVersion: ref.APIVersion,
				UID:        string(ref.UID),
			})
		}
	}

	return chain
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

// convertPodDetails converts a corev1.Pod to PodDetails.
func (e *K8sEnricher) convertPodDetails(pod *corev1.Pod) *signalprocessingv1alpha1.PodDetails {
	details := &signalprocessingv1alpha1.PodDetails{
		Labels:      ensureMap(pod.Labels),
		Annotations: ensureMap(pod.Annotations),
		Phase:       string(pod.Status.Phase),
		NodeName:    pod.Spec.NodeName,
	}

	// Convert container statuses
	for _, cs := range pod.Status.ContainerStatuses {
		status := signalprocessingv1alpha1.ContainerStatus{
			Name:         cs.Name,
			Ready:        cs.Ready,
			RestartCount: cs.RestartCount,
		}
		if cs.State.Running != nil {
			status.State = "Running"
		} else if cs.State.Waiting != nil {
			status.State = "Waiting"
		} else if cs.State.Terminated != nil {
			status.State = "Terminated"
			status.LastTerminationReason = cs.State.Terminated.Reason
		}
		details.ContainerStatuses = append(details.ContainerStatuses, status)
	}

	return details
}

// convertNodeDetails converts a corev1.Node to NodeDetails.
func (e *K8sEnricher) convertNodeDetails(node *corev1.Node) *signalprocessingv1alpha1.NodeDetails {
	details := &signalprocessingv1alpha1.NodeDetails{
		Labels: ensureMap(node.Labels),
	}

	// Convert conditions
	for _, cond := range node.Status.Conditions {
		details.Conditions = append(details.Conditions, signalprocessingv1alpha1.NodeCondition{
			Type:   string(cond.Type),
			Status: string(cond.Status),
			Reason: cond.Reason,
		})
	}

	// Convert allocatable resources
	details.Allocatable = make(map[string]string)
	for key, val := range node.Status.Allocatable {
		details.Allocatable[string(key)] = val.String()
	}

	return details
}

// convertDeploymentDetails converts an appsv1.Deployment to DeploymentDetails.
func (e *K8sEnricher) convertDeploymentDetails(deployment *appsv1.Deployment) *signalprocessingv1alpha1.DeploymentDetails {
	var replicas int32
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}

	return &signalprocessingv1alpha1.DeploymentDetails{
		Labels:            ensureMap(deployment.Labels),
		Annotations:       ensureMap(deployment.Annotations),
		Replicas:          replicas,
		AvailableReplicas: deployment.Status.AvailableReplicas,
		ReadyReplicas:     deployment.Status.ReadyReplicas,
	}
}

// convertStatefulSetDetails converts an appsv1.StatefulSet to StatefulSetDetails.
func (e *K8sEnricher) convertStatefulSetDetails(statefulset *appsv1.StatefulSet) *signalprocessingv1alpha1.StatefulSetDetails {
	var replicas int32
	if statefulset.Spec.Replicas != nil {
		replicas = *statefulset.Spec.Replicas
	}

	return &signalprocessingv1alpha1.StatefulSetDetails{
		Labels:          ensureMap(statefulset.Labels),
		Annotations:     ensureMap(statefulset.Annotations),
		Replicas:        replicas,
		ReadyReplicas:   statefulset.Status.ReadyReplicas,
		CurrentReplicas: statefulset.Status.CurrentReplicas,
	}
}

// convertDaemonSetDetails converts an appsv1.DaemonSet to DaemonSetDetails.
func (e *K8sEnricher) convertDaemonSetDetails(daemonset *appsv1.DaemonSet) *signalprocessingv1alpha1.DaemonSetDetails {
	return &signalprocessingv1alpha1.DaemonSetDetails{
		Labels:                 ensureMap(daemonset.Labels),
		Annotations:           ensureMap(daemonset.Annotations),
		DesiredNumberScheduled: daemonset.Status.DesiredNumberScheduled,
		CurrentNumberScheduled: daemonset.Status.CurrentNumberScheduled,
		NumberReady:           daemonset.Status.NumberReady,
	}
}

// convertReplicaSetDetails converts an appsv1.ReplicaSet to ReplicaSetDetails.
func (e *K8sEnricher) convertReplicaSetDetails(replicaset *appsv1.ReplicaSet) *signalprocessingv1alpha1.ReplicaSetDetails {
	var replicas int32
	if replicaset.Spec.Replicas != nil {
		replicas = *replicaset.Spec.Replicas
	}

	return &signalprocessingv1alpha1.ReplicaSetDetails{
		Labels:            ensureMap(replicaset.Labels),
		Annotations:       ensureMap(replicaset.Annotations),
		Replicas:          replicas,
		AvailableReplicas: replicaset.Status.AvailableReplicas,
		ReadyReplicas:     replicaset.Status.ReadyReplicas,
	}
}

// convertServiceDetails converts a corev1.Service to ServiceDetails.
func (e *K8sEnricher) convertServiceDetails(service *corev1.Service) *signalprocessingv1alpha1.ServiceDetails {
	details := &signalprocessingv1alpha1.ServiceDetails{
		Labels:      ensureMap(service.Labels),
		Annotations: ensureMap(service.Annotations),
		Type:        string(service.Spec.Type),
		ClusterIP:   service.Spec.ClusterIP,
		ExternalIPs: service.Spec.ExternalIPs,
	}

	// Convert ports
	for _, p := range service.Spec.Ports {
		details.Ports = append(details.Ports, signalprocessingv1alpha1.ServicePort{
			Name:       p.Name,
			Port:       p.Port,
			TargetPort: p.TargetPort.String(),
			Protocol:   string(p.Protocol),
		})
	}

	return details
}
