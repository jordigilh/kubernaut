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
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
)

const (
	// ManagedLabel is the label that marks an MCP Gateway Backend as managed by Kubernaut.
	ManagedLabel      = "kubernaut.ai/managed"
	managedLabelValue = "true"

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
	// ResyncPeriod is retained as registry configuration; owned WATCH streams
	// rotate on the bounded five-minute DD-FLEET-015 lifecycle.
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

	registryWatchState
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
		client: client, config: cfg, metrics: metrics,
		logger:             logger.WithName("crd-watcher"),
		registryWatchState: newRegistryWatchState(cfg.ChannelSize),
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
	if w.stopped || !w.ready {
		return fmt.Errorf("fleet cluster registry is not healthy")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	info, ok := w.clusters[clusterID]
	if !ok {
		return fmt.Errorf("%w: cluster %q is not registered", ErrRegistrationChanged, clusterID)
	}
	if info.RegistrationUID != registrationUID || w.registrationRevisions[clusterID] != expectedRevision {
		return fmt.Errorf("%w: cluster %q registration changed", ErrRegistrationChanged, clusterID)
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

// Start begins watching Envoy AI Gateway Backend CRDs.
func (w *EAIGWRegistry) Start(ctx context.Context) error {
	return w.session.start(ctx, registryWatchConfig{
		client: w.client, gvr: BackendGVR, config: w.config,
		trackable: w.trackableClusterInfo, logger: w.logger,
		publish: watchPublication{begin: w.begin, install: w.install, invalidate: w.invalidate, apply: w.applyWatchEvent},
	})
}

func (w *EAIGWRegistry) applyWatchEvent(event watch.Event) error {
	if event.Type == watch.Bookmark {
		return nil
	}
	if event.Type == watch.Deleted {
		w.onDelete(event.Object)
		return nil
	}
	obj, ok := event.Object.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("fleet registry event has invalid object type")
	}
	if obj.GetLabels()[ManagedLabel] != managedLabelValue {
		w.removeIfCurrent(obj)
		return nil
	}
	w.onAdd(obj)
	return nil
}

// Stop halts the watcher and closes the event channel.
func (w *EAIGWRegistry) Stop() {
	w.stop()
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

	added := addCluster(registryState{
		mu:         &w.mu,
		clusters:   &w.clusters,
		revisions:  w.registrationRevisions,
		generation: &w.revision,
		stopped:    &w.stopped,
	}, info)
	if !added.Changed {
		return
	}
	emitAddedCluster(info, added.Existed, added.Count, w.metrics, w.emit)
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
	handleDeletedCluster(obj, &w.mu, w.currentMaps, &w.revision, w.metrics, w.logger, w.emit)
}

func (w *EAIGWRegistry) removeIfCurrent(obj interface{}) {
	w.onDelete(obj)
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
