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

package registry

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
)

// MCPServerRegistrationGVR is the GroupVersionResource for Kuadrant's MCPServerRegistration CRD.
var MCPServerRegistrationGVR = schema.GroupVersionResource{
	Group:    "mcp.kuadrant.io",
	Version:  "v1alpha1",
	Resource: "mcpserverregistrations",
}

// KuadrantRegistry implements ClusterRegistry by watching Kuadrant MCPServerRegistration CRDs
// via a dynamic informer. Only resources labeled kubernaut.ai/managed=true and not in
// spec.state=Disabled are tracked.
type KuadrantRegistry struct {
	client  dynamic.Interface
	config  EAIGWRegistryConfig
	metrics *Metrics
	logger  logr.Logger

	mu       sync.RWMutex
	clusters map[string]ClusterInfo
	ready    bool

	eventCh chan ClusterEvent
	stopCh  chan struct{}
	stopped bool
}

// NewKuadrantRegistry creates a new KuadrantRegistry.
func NewKuadrantRegistry(client dynamic.Interface, cfg EAIGWRegistryConfig, metrics *Metrics, logger logr.Logger) *KuadrantRegistry {
	if cfg.ResyncPeriod == 0 {
		cfg.ResyncPeriod = defaultResyncPeriod
	}
	if cfg.ChannelSize == 0 {
		cfg.ChannelSize = defaultChannelSize
	}
	return &KuadrantRegistry{
		client:   client,
		config:   cfg,
		metrics:  metrics,
		logger:   logger.WithName("kuadrant-registry"),
		clusters: make(map[string]ClusterInfo),
		eventCh:  make(chan ClusterEvent, cfg.ChannelSize),
		stopCh:   make(chan struct{}),
	}
}

func (w *KuadrantRegistry) List() []ClusterInfo {
	w.mu.RLock()
	defer w.mu.RUnlock()
	result := make([]ClusterInfo, 0, len(w.clusters))
	for _, c := range w.clusters {
		result = append(result, c)
	}
	return result
}

func (w *KuadrantRegistry) Get(clusterID string) (ClusterInfo, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	info, ok := w.clusters[clusterID]
	return info, ok
}

func (w *KuadrantRegistry) WatchClusters() <-chan ClusterEvent {
	return w.eventCh
}

func (w *KuadrantRegistry) Ready() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.ready
}

// Start begins watching Kuadrant MCPServerRegistration CRDs.
func (w *KuadrantRegistry) Start(ctx context.Context) error {
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		w.client,
		w.config.ResyncPeriod,
		w.config.Namespace,
		func(opts *metav1.ListOptions) {
			opts.LabelSelector = ManagedLabel + "=true"
		},
	)

	informer := factory.ForResource(MCPServerRegistrationGVR).Informer()

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    w.onAdd,
		UpdateFunc: w.onUpdate,
		DeleteFunc: w.onDelete,
	})
	if err != nil {
		return fmt.Errorf("failed to add event handler: %w", err)
	}

	go func() {
		informer.Run(w.stopCh)
	}()

	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		return fmt.Errorf("failed to sync informer cache")
	}

	w.mu.Lock()
	w.ready = true
	w.mu.Unlock()

	w.metrics.NilSafeIncReconcile()
	w.logger.Info("KuadrantRegistry started and synced", "clusters", len(w.clusters))
	return nil
}

func (w *KuadrantRegistry) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return
	}
	w.stopped = true
	close(w.stopCh)
	close(w.eventCh)
	w.logger.Info("KuadrantRegistry stopped")
}

func (w *KuadrantRegistry) onAdd(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}

	if isKuadrantDisabled(u) {
		w.logger.V(1).Info("skipping disabled MCPServerRegistration", "name", u.GetName())
		return
	}

	labels := u.GetLabels()
	if labels[ManagedLabel] != "true" {
		return
	}

	info, err := extractKuadrantClusterInfo(u)
	if err != nil {
		w.logger.Error(err, "failed to extract cluster info on add", "name", u.GetName())
		w.metrics.NilSafeIncReconcileError()
		return
	}

	w.mu.Lock()
	w.clusters[info.ID] = info
	w.mu.Unlock()

	w.metrics.NilSafeSetClusters(float64(len(w.clusters)))
	w.metrics.NilSafeIncReconcile()
	w.emit(ClusterEvent{Type: EventAdded, Cluster: info})
	w.logger.Info("cluster added", "id", info.ID, "toolPrefix", info.ToolPrefix)
}

func (w *KuadrantRegistry) onUpdate(oldObj, newObj interface{}) {
	u, ok := newObj.(*unstructured.Unstructured)
	if !ok {
		return
	}

	if isKuadrantDisabled(u) {
		id := u.GetName()
		w.mu.Lock()
		info, existed := w.clusters[id]
		delete(w.clusters, id)
		w.mu.Unlock()
		if existed {
			w.metrics.NilSafeSetClusters(float64(len(w.clusters)))
			w.emit(ClusterEvent{Type: EventDeleted, Cluster: info})
			w.logger.Info("cluster disabled, removing", "id", id)
		}
		return
	}

	info, err := extractKuadrantClusterInfo(u)
	if err != nil {
		w.logger.Error(err, "failed to extract cluster info on update", "name", u.GetName())
		w.metrics.NilSafeIncReconcileError()
		return
	}

	w.mu.Lock()
	w.clusters[info.ID] = info
	w.mu.Unlock()

	w.metrics.NilSafeIncReconcile()
	w.emit(ClusterEvent{Type: EventUpdated, Cluster: info})
	w.logger.V(1).Info("cluster updated", "id", info.ID)
}

func (w *KuadrantRegistry) onDelete(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			return
		}
		u, ok = tombstone.Obj.(*unstructured.Unstructured)
		if !ok {
			return
		}
	}

	id := u.GetName()
	w.mu.Lock()
	info, existed := w.clusters[id]
	delete(w.clusters, id)
	w.mu.Unlock()

	if existed {
		w.metrics.NilSafeSetClusters(float64(len(w.clusters)))
		w.metrics.NilSafeIncReconcile()
		w.emit(ClusterEvent{Type: EventDeleted, Cluster: info})
		w.logger.Info("cluster deleted", "id", id)
	}
}

func (w *KuadrantRegistry) emit(event ClusterEvent) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.stopped {
		return
	}
	select {
	case w.eventCh <- event:
	default:
		w.metrics.NilSafeIncEventDrop()
		w.logger.V(0).Info("event channel full, dropping event",
			"type", event.Type, "cluster", event.Cluster.ID)
	}
}

// extractKuadrantClusterInfo builds ClusterInfo from a Kuadrant MCPServerRegistration.
func extractKuadrantClusterInfo(u *unstructured.Unstructured) (ClusterInfo, error) {
	name := u.GetName()
	if name == "" {
		return ClusterInfo{}, fmt.Errorf("MCPServerRegistration has empty name")
	}

	info := ClusterInfo{
		ID:        name,
		Namespace: u.GetNamespace(),
		Labels:    u.GetLabels(),
	}

	prefix, _, _ := unstructured.NestedString(u.Object, "spec", "prefix")
	info.ToolPrefix = prefix

	return info, nil
}

// isKuadrantDisabled checks if the MCPServerRegistration has spec.state set to "Disabled".
func isKuadrantDisabled(u *unstructured.Unstructured) bool {
	state, found, _ := unstructured.NestedString(u.Object, "spec", "state")
	return found && state == "Disabled"
}
