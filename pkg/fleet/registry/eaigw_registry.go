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

// EAIGWRegistryConfig configures the EAIGWRegistry.
type EAIGWRegistryConfig struct {
	// Namespace restricts watching to a specific namespace. Empty watches all.
	Namespace string
	// ResyncPeriod for the informer. Defaults to 5 minutes.
	ResyncPeriod time.Duration
	// ChannelSize for subscriber event channels. Defaults to 64.
	ChannelSize int
}

// EAIGWRegistry implements ClusterRegistry by watching Envoy AI Gateway Backend CRDs
// via a dynamic informer. Only resources labeled kubernaut.ai/managed=true are tracked.
type EAIGWRegistry struct {
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

// NewEAIGWRegistry creates a new EAIGWRegistry.
func NewEAIGWRegistry(client dynamic.Interface, cfg EAIGWRegistryConfig, metrics *Metrics, logger logr.Logger) *EAIGWRegistry {
	if cfg.ResyncPeriod == 0 {
		cfg.ResyncPeriod = defaultResyncPeriod
	}
	if cfg.ChannelSize == 0 {
		cfg.ChannelSize = defaultChannelSize
	}
	return &EAIGWRegistry{
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
func (w *EAIGWRegistry) List() []ClusterInfo {
	w.mu.RLock()
	defer w.mu.RUnlock()
	result := make([]ClusterInfo, 0, len(w.clusters))
	for _, c := range w.clusters {
		result = append(result, c)
	}
	return result
}

// Get returns cluster info by ID.
func (w *EAIGWRegistry) Get(clusterID string) (ClusterInfo, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	info, ok := w.clusters[clusterID]
	return info, ok
}

// WatchClusters returns the event channel.
func (w *EAIGWRegistry) WatchClusters() <-chan ClusterEvent {
	return w.eventCh
}

// Ready reports whether the watcher has completed initial sync.
func (w *EAIGWRegistry) Ready() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.ready
}

// Start begins watching Envoy AI Gateway Backend CRDs.
func (w *EAIGWRegistry) Start(ctx context.Context) error {
	seeded, err := startInformerAndSeed(ctx, w.client, w.config, BackendGVR,
		cache.ResourceEventHandlerFuncs{AddFunc: w.onAdd, UpdateFunc: w.onUpdate, DeleteFunc: w.onDelete},
		w.stopCh, w.trackableClusterInfo)
	if err != nil {
		return err
	}

	w.mu.Lock()
	for id, info := range seeded {
		w.clusters[id] = info
	}
	w.ready = true
	clusterCount := len(w.clusters)
	w.mu.Unlock()

	w.metrics.NilSafeIncReconcile()
	w.logger.Info("EAIGWRegistry started and synced", "clusters", clusterCount)
	return nil
}

// startInformerAndSeed builds a filtered dynamic informer for gvr, registers
// handlers, waits for the initial cache sync, then synchronously seeds
// already-present objects via trackable before returning. Shared by
// KuadrantRegistry.Start and EAIGWRegistry.Start (#2299/#2300 dupl fix):
// their startup sequence is otherwise identical and previously duplicated
// wholesale between the two files (golangci-lint dupl finding once the
// #2299 seed-from-indexer fix landed in both).
//
// #2299: cache.WaitForCacheSync only guarantees the reflector's initial
// List has populated informer.GetIndexer(); it does NOT guarantee the
// sharedProcessor has finished dispatching AddFunc to registered handlers
// for every pre-existing object, since that dispatch runs on an
// independent processorListener goroutine with no ordering guarantee
// relative to WaitForCacheSync returning. Seeding synchronously from the
// indexer itself makes the caller's Start() returning success a
// correctness guarantee, not a race against async dispatch.
func startInformerAndSeed(
	ctx context.Context,
	client dynamic.Interface,
	cfg EAIGWRegistryConfig,
	gvr schema.GroupVersionResource,
	handlers cache.ResourceEventHandlerFuncs,
	stopCh chan struct{},
	trackable func(*unstructured.Unstructured) (ClusterInfo, bool),
) (map[string]ClusterInfo, error) {
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		client,
		cfg.ResyncPeriod,
		cfg.Namespace,
		func(opts *metav1.ListOptions) {
			opts.LabelSelector = ManagedLabel + "=true"
		},
	)

	informer := factory.ForResource(gvr).Informer()

	if _, err := informer.AddEventHandler(handlers); err != nil {
		return nil, fmt.Errorf("failed to add event handler: %w", err)
	}

	go func() {
		informer.Run(stopCh)
	}()

	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		return nil, fmt.Errorf("failed to sync informer cache")
	}

	seeded := make(map[string]ClusterInfo)
	for _, obj := range informer.GetIndexer().List() {
		if u, ok := obj.(*unstructured.Unstructured); ok {
			if info, ok := trackable(u); ok {
				seeded[info.ID] = info
			}
		}
	}
	return seeded, nil
}

// Stop halts the watcher and closes the event channel.
func (w *EAIGWRegistry) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return
	}
	w.stopped = true
	close(w.stopCh)
	close(w.eventCh)
	w.logger.Info("EAIGWRegistry stopped")
}

func (w *EAIGWRegistry) onAdd(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	info, trackable := w.trackableClusterInfo(u)
	if !trackable {
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

// trackableClusterInfo extracts ClusterInfo from u, reporting false if
// extraction fails. Shared by onAdd's async dispatch path and Start()'s
// synchronous seed-from-indexer path (#2299) so cold-start population
// applies the identical extraction rules as live events.
func (w *EAIGWRegistry) trackableClusterInfo(u *unstructured.Unstructured) (ClusterInfo, bool) {
	info, err := ExtractClusterInfo(u)
	if err != nil {
		w.logger.Error(err, "failed to extract cluster info", "name", u.GetName())
		w.metrics.NilSafeIncReconcileError()
		return ClusterInfo{}, false
	}
	return info, true
}

func (w *EAIGWRegistry) onUpdate(oldObj, newObj interface{}) {
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

func (w *EAIGWRegistry) onDelete(obj interface{}) {
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

func (w *EAIGWRegistry) emit(event ClusterEvent) {
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
		return ClusterInfo{}, fmt.Errorf("backend CRD has empty name")
	}

	info := ClusterInfo{
		ID:         name,
		Namespace:  u.GetNamespace(),
		Labels:     u.GetLabels(),
		ToolPrefix: name + "__",
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
