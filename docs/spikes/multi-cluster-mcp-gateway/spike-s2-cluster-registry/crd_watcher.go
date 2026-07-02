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

// Package registry provides the ClusterRegistry interface and a CRD-based
// implementation that watches MCPServerRegistration resources labeled with
// kubernaut.ai/managed=true.
//
// Authority: Issue #54 (Multi-cluster federation), WS4 (Cluster Lifecycle)
// Design: Kubernaut is consumer-only — it does NOT create MCPServerRegistration
// CRDs. Those are owned by platform admins, ACM, or GitOps.
package registry

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	managedLabel       = "kubernaut.ai/managed"
	jwtAudienceAnnot   = "kubernaut.ai/jwt-audience"
	mcpServerRegGVR    = "mcp.kuadrant.io/v1alpha1"
	mcpServerRegKind   = "MCPServerRegistration"
)

// MCPServerRegistrationGVR is the GroupVersionResource for MCPServerRegistration.
var MCPServerRegistrationGVR = schema.GroupVersionResource{
	Group:    "mcp.kuadrant.io",
	Version:  "v1alpha1",
	Resource: "mcpserverregistrations",
}

// CRDWatcher implements ClusterRegistry by watching MCPServerRegistration CRDs
// labeled with kubernaut.ai/managed=true. It extracts ClusterInfo from the CRD
// spec and emits events when the managed set changes.
//
// This is a read-only watcher. Kubernaut never creates, updates, or deletes
// MCPServerRegistration resources — that responsibility belongs to the
// platform team (via ACM, GitOps, or manual kubectl).
type CRDWatcher struct {
	client    client.Reader
	namespace string
	logger    logr.Logger

	mu          sync.RWMutex
	clusters    map[string]ClusterInfo
	subscribers []chan<- ClusterEvent
}

// NewCRDWatcher creates a watcher that discovers clusters from labeled
// MCPServerRegistration CRDs in the given namespace.
func NewCRDWatcher(reader client.Reader, namespace string, logger logr.Logger) *CRDWatcher {
	return &CRDWatcher{
		client:    reader,
		namespace: namespace,
		logger:    logger.WithName("crd-watcher"),
		clusters:  make(map[string]ClusterInfo),
	}
}

// ListClusters returns the current set of managed clusters.
func (w *CRDWatcher) ListClusters(_ context.Context) ([]ClusterInfo, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	result := make([]ClusterInfo, 0, len(w.clusters))
	for _, c := range w.clusters {
		result = append(result, c)
	}
	return result, nil
}

// GetCluster returns a specific cluster by ID (metadata.name of the CRD).
func (w *CRDWatcher) GetCluster(_ context.Context, id string) (ClusterInfo, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	cluster, ok := w.clusters[id]
	if !ok {
		return ClusterInfo{}, fmt.Errorf("cluster %q not found in registry", id)
	}
	return cluster, nil
}

// WatchClusters returns a channel that receives cluster lifecycle events.
func (w *CRDWatcher) WatchClusters(_ context.Context) (<-chan ClusterEvent, error) {
	ch := make(chan ClusterEvent, 16)
	w.mu.Lock()
	w.subscribers = append(w.subscribers, ch)
	w.mu.Unlock()
	return ch, nil
}

// Reconcile is called when the informer detects changes to MCPServerRegistration
// resources. It computes the diff and emits events.
func (w *CRDWatcher) Reconcile(ctx context.Context) error {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "mcp.kuadrant.io",
		Version: "v1alpha1",
		Kind:    "MCPServerRegistrationList",
	})

	opts := []client.ListOption{
		client.InNamespace(w.namespace),
		client.MatchingLabels{managedLabel: "true"},
	}

	if err := w.client.List(ctx, list, opts...); err != nil {
		return fmt.Errorf("listing MCPServerRegistrations: %w", err)
	}

	current := make(map[string]ClusterInfo, len(list.Items))
	for i := range list.Items {
		info := extractClusterInfo(&list.Items[i])
		current[info.ID] = info
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Detect additions and updates
	for id, newInfo := range current {
		if old, exists := w.clusters[id]; !exists {
			w.emit(ClusterEvent{Type: ClusterAdded, Cluster: newInfo})
		} else if !reflect.DeepEqual(old, newInfo) {
			w.emit(ClusterEvent{Type: ClusterUpdated, Cluster: newInfo})
		}
	}

	// Detect removals
	for id, oldInfo := range w.clusters {
		if _, exists := current[id]; !exists {
			w.emit(ClusterEvent{Type: ClusterRemoved, Cluster: oldInfo})
		}
	}

	w.clusters = current
	w.logger.Info("reconciled cluster registry", "clusters", len(current))
	return nil
}

func (w *CRDWatcher) emit(event ClusterEvent) {
	for _, ch := range w.subscribers {
		select {
		case ch <- event:
		default:
			w.logger.Info("dropping cluster event (subscriber channel full)",
				"type", event.Type, "cluster", event.Cluster.ID)
		}
	}
}

// extractClusterInfo builds a ClusterInfo from an unstructured MCPServerRegistration.
func extractClusterInfo(obj *unstructured.Unstructured) ClusterInfo {
	info := ClusterInfo{
		ID:     obj.GetName(),
		Name:   obj.GetName(),
		Labels: obj.GetLabels(),
	}

	annotations := obj.GetAnnotations()
	if annotations != nil {
		info.JWTAudience = annotations[jwtAudienceAnnot]
	}

	spec, _ := obj.Object["spec"].(map[string]any)
	if spec != nil {
		if prefix, ok := spec["prefix"].(string); ok {
			info.ToolPrefix = prefix
		}
	}

	info.Status = deriveStatus(obj)
	return info
}

// deriveStatus extracts cluster health from .status.conditions.
// If a condition with type=Ready and status=True exists -> Ready.
// If conditions exist but none is Ready -> Degraded.
// If no status/conditions -> Offline (CRD exists but gateway hasn't reconciled).
func deriveStatus(obj *unstructured.Unstructured) ClusterStatus {
	status, _ := obj.Object["status"].(map[string]any)
	if status == nil {
		return ClusterStatusOffline
	}

	conditions, _ := status["conditions"].([]any)
	if len(conditions) == 0 {
		return ClusterStatusOffline
	}

	for _, c := range conditions {
		cond, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if cond["type"] == "Ready" && cond["status"] == "True" {
			return ClusterStatusReady
		}
	}
	return ClusterStatusDegraded
}

// Compile-time interface compliance.
var _ ClusterRegistry = (*CRDWatcher)(nil)
