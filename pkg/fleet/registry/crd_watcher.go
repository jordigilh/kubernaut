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
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
)

const (
	// ManagedLabel is the label that marks an MCP Gateway Backend as managed by Kubernaut.
	ManagedLabel = "kubernaut.ai/managed"

	// defaultChannelSize for event subscriber channels.
	defaultChannelSize = 64

	// defaultResyncPeriod for the dynamic informer.
	defaultResyncPeriod = 5 * time.Minute
)

// BackendGVR is the GroupVersionResource for Envoy AI Gateway's Backend CRD.
// Each Backend represents a managed cluster's K8s MCP Server endpoint.
// The Backend name serves as the cluster ID and tool name prefix ({backendName}__{toolName}).
var BackendGVR = schema.GroupVersionResource{
	Group:    "gateway.envoyproxy.io",
	Version:  "v1alpha1",
	Resource: "backends",
}

// MCPRouteGVR is the GroupVersionResource for Envoy AI Gateway's MCPRoute CRD.
// MCPRoute aggregates multiple Backends into a single MCP endpoint with
// tool prefixing, OAuth, and CEL authorization.
var MCPRouteGVR = schema.GroupVersionResource{
	Group:    "aigateway.envoyproxy.io",
	Version:  "v1beta1",
	Resource: "mcproutes",
}

// CRDWatcherConfig configures the CRDWatcher.
type CRDWatcherConfig struct {
	// Namespace restricts watching to a specific namespace. Empty watches all.
	Namespace string
	// ResyncPeriod for the informer. Defaults to 5 minutes.
	ResyncPeriod time.Duration
	// ChannelSize for subscriber event channels. Defaults to 64.
	ChannelSize int
}

// CRDWatcher implements ClusterRegistry by watching Envoy AI Gateway Backend CRDs
// via a dynamic informer. Only resources labeled kubernaut.ai/managed=true are tracked.
type CRDWatcher struct {
	client  dynamic.Interface
	config  CRDWatcherConfig
	metrics *Metrics
	logger  logr.Logger

	mu       sync.RWMutex
	clusters map[string]ClusterInfo
	ready    bool

	eventCh chan ClusterEvent
	stopCh  chan struct{}
	stopped bool
}

// NewCRDWatcher creates a new CRDWatcher.
func NewCRDWatcher(client dynamic.Interface, cfg CRDWatcherConfig, metrics *Metrics, logger logr.Logger) *CRDWatcher {
	if cfg.ResyncPeriod == 0 {
		cfg.ResyncPeriod = defaultResyncPeriod
	}
	if cfg.ChannelSize == 0 {
		cfg.ChannelSize = defaultChannelSize
	}
	return &CRDWatcher{
		client:   client,
		config:   cfg,
		metrics:  metrics,
		logger:   logger.WithName("crd-watcher"),
		clusters: make(map[string]ClusterInfo),
		eventCh:  make(chan ClusterEvent, cfg.ChannelSize),
		stopCh:   make(chan struct{}),
	}
}

// List returns all known managed clusters.
func (w *CRDWatcher) List() []ClusterInfo {
	w.mu.RLock()
	defer w.mu.RUnlock()
	result := make([]ClusterInfo, 0, len(w.clusters))
	for _, c := range w.clusters {
		result = append(result, c)
	}
	return result
}

// Get returns cluster info by ID.
func (w *CRDWatcher) Get(clusterID string) (ClusterInfo, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	info, ok := w.clusters[clusterID]
	return info, ok
}

// WatchClusters returns the event channel.
func (w *CRDWatcher) WatchClusters() <-chan ClusterEvent {
	return w.eventCh
}

// Ready reports whether the watcher has completed initial sync.
func (w *CRDWatcher) Ready() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.ready
}

// Start begins watching Envoy AI Gateway Backend CRDs.
func (w *CRDWatcher) Start(ctx context.Context) error {
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		w.client,
		w.config.ResyncPeriod,
		w.config.Namespace,
		func(opts *metav1.ListOptions) {
			opts.LabelSelector = ManagedLabel + "=true"
		},
	)

	informer := factory.ForResource(BackendGVR).Informer()

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
	w.logger.Info("CRDWatcher started and synced", "clusters", len(w.clusters))
	return nil
}

// Stop halts the watcher and closes the event channel.
func (w *CRDWatcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return
	}
	w.stopped = true
	close(w.stopCh)
	close(w.eventCh)
	w.logger.Info("CRDWatcher stopped")
}

func (w *CRDWatcher) onAdd(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	info, err := ExtractClusterInfo(u)
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
	w.logger.Info("cluster added", "id", info.ID, "endpoint", info.MCPEndpoint)
}

func (w *CRDWatcher) onUpdate(oldObj, newObj interface{}) {
	u, ok := newObj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	info, err := ExtractClusterInfo(u)
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

func (w *CRDWatcher) onDelete(obj interface{}) {
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

func (w *CRDWatcher) emit(event ClusterEvent) {
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

// ExtractClusterInfo extracts ClusterInfo from an unstructured Envoy AI Gateway Backend CRD.
// The MCP endpoint is derived from spec.endpoints[0].fqdn if present,
// otherwise falls back to status.endpoint or spec.endpoint for compatibility.
func ExtractClusterInfo(u *unstructured.Unstructured) (ClusterInfo, error) {
	name := u.GetName()
	if name == "" {
		return ClusterInfo{}, fmt.Errorf("Backend CRD has empty name")
	}

	info := ClusterInfo{
		ID:        name,
		Namespace: u.GetNamespace(),
		Labels:    u.GetLabels(),
	}

	// Extract display name from annotation or label.
	annotations := u.GetAnnotations()
	if displayName, ok := annotations["kubernaut.ai/cluster-name"]; ok {
		info.Name = displayName
	} else {
		info.Name = name
	}

	// Extract MCP endpoint from status.endpoint or spec.
	endpoint, found, _ := unstructured.NestedString(u.Object, "status", "endpoint")
	if found && endpoint != "" {
		info.MCPEndpoint = endpoint
	} else {
		// Fallback: derive from spec.targetRef or name convention.
		specEndpoint, found, _ := unstructured.NestedString(u.Object, "spec", "endpoint")
		if found && specEndpoint != "" {
			info.MCPEndpoint = specEndpoint
		}
	}

	return info, nil
}
