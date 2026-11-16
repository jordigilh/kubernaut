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

	// onWatchError is installed by each registry instance to fail readiness
	// closed when its informer loses the watch connection.
	onWatchError func(error)
	// seed publishes an indexer snapshot with mutation-generation fencing after
	// informer sync, preserving asynchronous initial Add callbacks.
	seed func(cache.Indexer) error
}

// EAIGWRegistry implements ClusterRegistry by watching Envoy AI Gateway Backend CRDs
// via a dynamic informer. Only resources labeled kubernaut.ai/managed=true are tracked.
type EAIGWRegistry struct {
	client  dynamic.Interface
	config  EAIGWRegistryConfig
	metrics *Metrics
	logger  logr.Logger

	mu                    sync.RWMutex
	clusters              map[string]ClusterInfo
	revision              uint64 // global refresh mutation generation, not a registration proof
	registrationRevisions map[string]uint64
	ready                 bool
	watchError            error
	started               bool

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
		client:                client,
		config:                cfg,
		metrics:               metrics,
		logger:                logger.WithName("crd-watcher"),
		clusters:              make(map[string]ClusterInfo),
		registrationRevisions: make(map[string]uint64),
		eventCh:               make(chan ClusterEvent, cfg.ChannelSize),
		stopCh:                make(chan struct{}),
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

// Snapshot returns the current registration and its per-registration revision.
// Unrelated and extracted-data no-op events affect refresh generation only.
func (w *EAIGWRegistry) Snapshot(clusterID string) (RegistrationSnapshot, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	info, ok := w.clusters[clusterID]
	if !ok {
		return RegistrationSnapshot{}, false
	}
	info.Labels = cloneLabels(info.Labels)
	return RegistrationSnapshot{Info: info, Revision: w.registrationRevisions[clusterID]}, true
}

func (w *EAIGWRegistry) SnapshotAll() RegistrySnapshot {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return snapshotRegistrations(w.revision, w.clusters, w.registrationRevisions)
}

// WithCurrent runs callback while holding the registry update exclusion after
// verifying both the registration UID and the exact snapshot revision.
func (w *EAIGWRegistry) WithCurrent(ctx context.Context, clusterID, registrationUID string, expectedRevision uint64, callback func(ClusterInfo) error) error {
	if callback == nil {
		return fmt.Errorf("registry current callback is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	info, ok := w.clusters[clusterID]
	if !ok {
		return fmt.Errorf("cluster %q is not registered", clusterID)
	}
	if info.RegistrationUID != registrationUID || w.registrationRevisions[clusterID] != expectedRevision {
		return fmt.Errorf("cluster %q registration changed", clusterID)
	}
	return callback(info)
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

func (w *EAIGWRegistry) markWatchUnhealthy(err error) {
	w.mu.Lock()
	w.ready = false
	w.watchError = err
	w.revision++
	w.mu.Unlock()
	w.logger.Error(err, "fleet cluster registry watch failed")
}

// Probe refreshes the registry from the authoritative API and restores
// readiness after a transient informer/watch failure.
func (w *EAIGWRegistry) Probe(ctx context.Context) error {
	w.mu.RLock()
	started, stopped, generation := w.started, w.stopped, w.revision
	w.mu.RUnlock()
	refreshed, err := probeClusters(ctx, w.client, w.config.Namespace, BackendGVR, started, stopped, w.trackableClusterInfo)
	if err != nil {
		w.markWatchUnhealthy(err)
		return err
	}
	w.mu.Lock()
	publish, err := refreshMayPublish(generation, w.revision, w.stopped, w.ready)
	if !publish {
		w.mu.Unlock()
		return err
	}
	publishRegistrations(&w.clusters, refreshed, w.registrationRevisions, &w.revision)
	w.ready = true
	w.watchError = nil
	w.mu.Unlock()
	return nil
}

// Start begins watching Envoy AI Gateway Backend CRDs.
func (w *EAIGWRegistry) Start(ctx context.Context) error {
	config := w.config
	config.onWatchError = w.markWatchUnhealthy
	config.seed = func(indexer cache.Indexer) error { return w.seedFromIndexer(ctx, indexer) }
	err := startInformerAndSeed(ctx, w.client, config, BackendGVR,
		cache.ResourceEventHandlerFuncs{AddFunc: w.onAdd, UpdateFunc: w.onUpdate, DeleteFunc: w.onDelete},
		w.stopCh)
	if err != nil {
		return err
	}

	w.metrics.NilSafeIncReconcile()
	w.logger.Info("EAIGWRegistry started and synced", "clusters", len(w.List()))
	return nil
}

func (w *EAIGWRegistry) seedFromIndexer(ctx context.Context, indexer cache.Indexer) error {
	return seedRegistry(ctx, indexer, &w.mu, &w.revision, &w.stopped, w.trackableClusterInfo, func(seeded map[string]ClusterInfo) error {
		if w.watchError != nil {
			return fmt.Errorf("registry watch failed during startup: %w", w.watchError)
		}
		publishRegistrations(&w.clusters, seeded, w.registrationRevisions, &w.revision)
		w.ready, w.started = true, true
		return nil
	})
}

func seedRegistry(ctx context.Context, indexer cache.Indexer, mu *sync.RWMutex, revision *uint64, stopped *bool, trackable func(*unstructured.Unstructured) (ClusterInfo, bool), publish func(map[string]ClusterInfo) error) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		mu.RLock()
		generation := *revision
		mu.RUnlock()
		seeded := seedClusters(indexer, trackable)
		mu.Lock()
		if *stopped {
			mu.Unlock()
			return fmt.Errorf("fleet cluster registry stopped")
		}
		if *revision != generation {
			mu.Unlock()
			continue
		}
		err := publish(seeded)
		mu.Unlock()
		return err
	}
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
) error {
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
		return fmt.Errorf("failed to add event handler: %w", err)
	}
	if cfg.onWatchError != nil {
		if err := informer.SetWatchErrorHandler(func(_ *cache.Reflector, watchErr error) {
			cfg.onWatchError(watchErr)
		}); err != nil {
			return fmt.Errorf("failed to set informer watch error handler: %w", err)
		}
	}

	go func() {
		informer.Run(stopCh)
	}()

	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		return fmt.Errorf("failed to sync informer cache")
	}
	if cfg.seed != nil {
		return cfg.seed(informer.GetIndexer())
	}
	return nil
}

func seedClusters(indexer cache.Indexer, trackable func(*unstructured.Unstructured) (ClusterInfo, bool)) map[string]ClusterInfo {
	seeded := make(map[string]ClusterInfo)
	for _, obj := range indexer.List() {
		if u, ok := obj.(*unstructured.Unstructured); ok {
			if info, ok := trackable(u); ok {
				seeded[info.ID] = info
			}
		}
	}
	return seeded
}

// Stop halts the watcher and closes the event channel.
func (w *EAIGWRegistry) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return
	}
	w.stopped = true
	w.ready = false
	w.revision++
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

	changed, existed, count := addCluster(info, &w.mu, &w.clusters, w.registrationRevisions, &w.revision, &w.stopped)
	if !changed {
		return
	}
	emitAddedCluster(info, existed, count, w.metrics, w.emit)
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
	info, trackable := w.trackableClusterInfo(u)
	if !trackable {
		w.removeIfCurrent(u)
		return
	}

	w.mu.Lock()
	current, existed := w.clusters[info.ID]
	if w.stopped {
		w.mu.Unlock()
		return
	}
	if existed && clusterInfoEqual(current, info) {
		w.revision++
		w.mu.Unlock()
		return
	}
	if existed && current.RegistrationUID != info.RegistrationUID {
		// The informer can deliver an old update after a delete/recreate. Never
		// allow that old incarnation to overwrite the current registration.
		if oldUID := objectUID(oldObj); oldUID == "" || oldUID != current.RegistrationUID {
			w.mu.Unlock()
			return
		}
	}
	w.clusters[info.ID] = info
	w.revision++
	w.registrationRevisions[info.ID] = w.revision
	w.mu.Unlock()

	w.metrics.NilSafeIncReconcile()
	w.emit(ClusterEvent{Type: EventUpdated, Cluster: info})
	w.logger.V(1).Info("cluster updated", "id", info.ID)
}

func (w *EAIGWRegistry) onDelete(obj interface{}) {
	handleDeletedCluster(obj, &w.mu, func() (map[string]ClusterInfo, map[string]uint64) { return w.clusters, w.registrationRevisions }, &w.revision, w.metrics, w.logger, w.emit)
}

func (w *EAIGWRegistry) removeIfCurrent(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	id := u.GetName()
	uid := string(u.GetUID())
	w.mu.Lock()
	info, existed := w.clusters[id]
	if !existed || uid == "" || info.RegistrationUID != uid {
		w.mu.Unlock()
		return
	}
	delete(w.clusters, id)
	delete(w.registrationRevisions, id)
	w.revision++
	clusterCount := len(w.clusters)
	w.mu.Unlock()
	w.metrics.NilSafeSetClusters(float64(clusterCount))
	w.metrics.NilSafeIncReconcile()
	w.emit(ClusterEvent{Type: EventDeleted, Cluster: info})
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
		ID:              name,
		RegistrationUID: string(u.GetUID()),
		Namespace:       u.GetNamespace(),
		Labels:          u.GetLabels(),
		ToolPrefix:      name + "__",
	}
	if info.RegistrationUID == "" {
		return ClusterInfo{}, fmt.Errorf("backend CRD %q has empty metadata.uid", name)
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
