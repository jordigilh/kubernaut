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

// Package enricher provides Kubernetes context enrichment for signals.
// Design Decision: DD-017 - Signal-driven K8s enrichment with standard depth
// Design Decision: ADR-041 - K8s Enricher fetches data, Rego policies evaluate classification
// Business Requirement: BR-SP-001 - K8s Context Enrichment (<2s P95)
package enricher

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
)

// EnrichmentResult holds the enriched Kubernetes context.
// Uses shared types from pkg/shared/types/ for API contract alignment.
type EnrichmentResult struct {
	// Namespace labels for classification
	NamespaceLabels map[string]string

	// Pod details (if applicable)
	Pod *sharedtypes.PodDetails

	// Node details (if pod is scheduled)
	Node *sharedtypes.NodeDetails

	// Deployment details (if applicable)
	Deployment *sharedtypes.DeploymentDetails
}

// K8sEnricher fetches Kubernetes context for signal enrichment.
// DD-005 v2.0: Uses logr.Logger (unified interface for all Kubernaut services)
type K8sEnricher struct {
	client  client.Client
	logger  logr.Logger
	metrics *metrics.Metrics
	timeout time.Duration
}

// NewK8sEnricher creates a new K8s context enricher.
// DD-005 v2.0: Accept logr.Logger from caller (CRD controller passes ctrl.Log)
func NewK8sEnricher(c client.Client, logger logr.Logger, m *metrics.Metrics, timeout time.Duration) *K8sEnricher {
	return &K8sEnricher{
		client:  c,
		logger:  logger.WithName("k8s-enricher"),
		metrics: m,
		timeout: timeout,
	}
}

// EnrichPodSignal fetches Namespace + Pod + Node context (standard depth).
// BR-SP-001: K8s Context Enrichment
// Returns partial results on non-critical failures (graceful degradation).
func (e *K8sEnricher) EnrichPodSignal(ctx context.Context, namespace, podName string) (*EnrichmentResult, error) {
	startTime := time.Now()
	defer func() {
		if e.metrics != nil {
			e.metrics.EnrichmentDuration.WithLabelValues("Pod").Observe(time.Since(startTime).Seconds())
		}
	}()

	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	result := &EnrichmentResult{}

	// 1. Fetch namespace (REQUIRED - error if not found)
	ns := &corev1.Namespace{}
	if err := e.client.Get(ctx, types.NamespacedName{Name: namespace}, ns); err != nil {
		e.logger.Error(err, "Failed to get namespace", "namespace", namespace)
		return nil, fmt.Errorf("failed to get namespace %s: %w", namespace, err)
	}
	result.NamespaceLabels = ns.Labels

	// 2. Fetch pod (optional - graceful degradation if not found)
	pod := &corev1.Pod{}
	if err := e.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: podName}, pod); err != nil {
		e.logger.Info("Pod not found, continuing with partial context", "namespace", namespace, "pod", podName, "error", err.Error())
		// Don't return error - graceful degradation
	} else {
		result.Pod = e.convertPodToDetails(pod)

		// 3. Fetch node if pod is scheduled (optional - graceful degradation)
		if pod.Spec.NodeName != "" {
			node := &corev1.Node{}
			if err := e.client.Get(ctx, types.NamespacedName{Name: pod.Spec.NodeName}, node); err != nil {
				e.logger.Info("Node not found, continuing without node context", "node", pod.Spec.NodeName, "error", err.Error())
			} else {
				result.Node = e.convertNodeToDetails(node)
			}
		}
	}

	return result, nil
}

// EnrichNamespaceOnly fetches only namespace context (fallback for unknown resource types).
func (e *K8sEnricher) EnrichNamespaceOnly(ctx context.Context, namespace string) (*EnrichmentResult, error) {
	startTime := time.Now()
	defer func() {
		if e.metrics != nil {
			e.metrics.EnrichmentDuration.WithLabelValues("Namespace").Observe(time.Since(startTime).Seconds())
		}
	}()

	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	result := &EnrichmentResult{}

	// Fetch namespace
	ns := &corev1.Namespace{}
	if err := e.client.Get(ctx, types.NamespacedName{Name: namespace}, ns); err != nil {
		e.logger.Error(err, "Failed to get namespace", "namespace", namespace)
		return nil, fmt.Errorf("failed to get namespace %s: %w", namespace, err)
	}
	result.NamespaceLabels = ns.Labels

	return result, nil
}

// convertPodToDetails converts a K8s Pod to shared PodDetails type.
func (e *K8sEnricher) convertPodToDetails(pod *corev1.Pod) *sharedtypes.PodDetails {
	// Calculate total restart count
	var totalRestarts int32
	for _, cs := range pod.Status.ContainerStatuses {
		totalRestarts += cs.RestartCount
	}

	details := &sharedtypes.PodDetails{
		Name:              pod.Name,
		Phase:             string(pod.Status.Phase),
		Labels:            pod.Labels,
		Annotations:       pod.Annotations,
		RestartCount:      totalRestarts,
		CreationTimestamp: pod.CreationTimestamp.Format("2006-01-02T15:04:05Z"),
	}

	// Convert container statuses
	for _, cs := range pod.Status.ContainerStatuses {
		state := "unknown"
		if cs.State.Running != nil {
			state = "running"
		} else if cs.State.Waiting != nil {
			state = "waiting"
		} else if cs.State.Terminated != nil {
			state = "terminated"
		}

		details.Containers = append(details.Containers, sharedtypes.ContainerStatus{
			Name:         cs.Name,
			Ready:        cs.Ready,
			RestartCount: cs.RestartCount,
			Image:        cs.Image,
			State:        state,
		})
	}

	return details
}

// convertNodeToDetails converts a K8s Node to shared NodeDetails type.
func (e *K8sEnricher) convertNodeToDetails(node *corev1.Node) *sharedtypes.NodeDetails {
	details := &sharedtypes.NodeDetails{
		Name:   node.Name,
		Labels: node.Labels,
		Capacity: sharedtypes.ResourceList{
			CPU:    node.Status.Capacity.Cpu().String(),
			Memory: node.Status.Capacity.Memory().String(),
		},
		Allocatable: sharedtypes.ResourceList{
			CPU:    node.Status.Allocatable.Cpu().String(),
			Memory: node.Status.Allocatable.Memory().String(),
		},
	}

	// Convert conditions
	for _, cond := range node.Status.Conditions {
		details.Conditions = append(details.Conditions, sharedtypes.NodeCondition{
			Type:   string(cond.Type),
			Status: string(cond.Status),
		})
	}

	return details
}

