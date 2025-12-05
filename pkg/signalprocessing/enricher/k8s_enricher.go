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

// Package enricher provides K8s context enrichment for signal processing.
package enricher

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
}

// NewK8sEnricher creates a new K8s context enricher.
func NewK8sEnricher(c client.Client, logger logr.Logger, m *metrics.Metrics, timeout time.Duration) *K8sEnricher {
	return &K8sEnricher{
		client:  c,
		logger:  logger.WithName("k8s-enricher"),
		metrics: m,
		timeout: timeout,
	}
}

// Enrich fetches Kubernetes context based on signal type.
func (e *K8sEnricher) Enrich(ctx context.Context, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.KubernetesContext, error) {
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
	case "Node":
		return e.enrichNodeSignal(ctx, signal, result)
	default:
		// Graceful fallback: namespace only for unknown resource types
		return e.enrichNamespaceOnly(ctx, signal, result)
	}
}

// enrichPodSignal fetches Namespace + Pod + Node.
func (e *K8sEnricher) enrichPodSignal(ctx context.Context, signal *signalprocessingv1alpha1.SignalData, result *signalprocessingv1alpha1.KubernetesContext) (*signalprocessingv1alpha1.KubernetesContext, error) {
	// 1. Fetch namespace (required)
	ns, err := e.getNamespace(ctx, signal.TargetResource.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace %s: %w", signal.TargetResource.Namespace, err)
	}
	// Always return non-nil maps (even if empty)
	result.NamespaceLabels = ensureMap(ns.Labels)
	result.NamespaceAnnotations = ensureMap(ns.Annotations)

	// 2. Fetch pod (optional - continue with partial context if not found)
	pod, err := e.getPod(ctx, signal.TargetResource.Namespace, signal.TargetResource.Name)
	if err != nil {
		e.logger.Info("Pod not found, continuing with partial context", "error", err)
		return result, nil
	}
	result.Pod = e.convertPodDetails(pod)

	// 3. Fetch node where pod runs (optional)
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

// ensureMap returns the input map if non-nil, otherwise returns an empty map.
func ensureMap(m map[string]string) map[string]string {
	if m == nil {
		return make(map[string]string)
	}
	return m
}

// getNamespace fetches a namespace by name.
func (e *K8sEnricher) getNamespace(ctx context.Context, name string) (*corev1.Namespace, error) {
	ns := &corev1.Namespace{}
	if err := e.client.Get(ctx, types.NamespacedName{Name: name}, ns); err != nil {
		return nil, err
	}
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

// convertPodDetails converts a corev1.Pod to PodDetails.
func (e *K8sEnricher) convertPodDetails(pod *corev1.Pod) *signalprocessingv1alpha1.PodDetails {
	details := &signalprocessingv1alpha1.PodDetails{
		Labels:      pod.Labels,
		Annotations: pod.Annotations,
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
		Labels: node.Labels,
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
		Labels:            deployment.Labels,
		Annotations:       deployment.Annotations,
		Replicas:          replicas,
		AvailableReplicas: deployment.Status.AvailableReplicas,
		ReadyReplicas:     deployment.Status.ReadyReplicas,
	}
}
