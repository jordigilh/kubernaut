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

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
)

// MCPServerRegistrationGVR is the GroupVersionResource for Kuadrant's MCPServerRegistration CRD.
var MCPServerRegistrationGVR = schema.GroupVersionResource{
	Group:    "mcp.kuadrant.io",
	Version:  "v1",
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

	registryWatchState
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
		client: client, config: cfg, metrics: metrics,
		logger:             logger.WithName("kuadrant-registry"),
		registryWatchState: newRegistryWatchState(cfg.ChannelSize),
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

// Snapshot returns the current registration and its per-registration revision.
func (w *KuadrantRegistry) Snapshot(clusterID string) (RegistrationSnapshot, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	info, ok := w.clusters[clusterID]
	if !ok {
		return RegistrationSnapshot{}, false
	}
	info.Labels = cloneLabels(info.Labels)
	return RegistrationSnapshot{Info: info, Revision: w.registrationRevisions[clusterID]}, true
}

func (w *KuadrantRegistry) SnapshotAll() RegistrySnapshot {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return snapshotRegistrations(w.revision, w.clusters, w.registrationRevisions)
}

// WithCurrent runs callback only while the requested registration UID and
// exact registry revision remain current.
func (w *KuadrantRegistry) WithCurrent(ctx context.Context, clusterID, registrationUID string, expectedRevision uint64, callback func(ClusterInfo) error) error {
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
	return w.session.start(ctx, registryWatchConfig{
		client: w.client, gvr: MCPServerRegistrationGVR, config: w.config,
		trackable: w.trackableClusterInfo, logger: w.logger,
		publish: watchPublication{begin: w.begin, install: w.install, invalidate: w.invalidate, apply: w.applyWatchEvent},
	})
}

func (w *KuadrantRegistry) applyWatchEvent(event watch.Event) error {
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
	if isKuadrantDisabled(obj) || obj.GetLabels()[ManagedLabel] != managedLabelValue {
		w.removeIfCurrent(obj)
		return nil
	}
	w.onAdd(obj)
	return nil
}

func (w *KuadrantRegistry) Stop() {
	w.stop()
	w.logger.Info("KuadrantRegistry stopped")
}

func (w *KuadrantRegistry) onAdd(obj interface{}) {
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
	w.logger.Info("cluster added", "id", info.ID, "toolPrefix", info.ToolPrefix)
}

// trackableClusterInfo reports whether u should be tracked in the registry
// (respecting the managed label and disabled state) and, if so, extracts
// its ClusterInfo. Shared by onAdd's async dispatch path and Start()'s
// synchronous seed-from-indexer path (#2299) so cold-start population
// applies the identical inclusion/extraction rules as live events.
func (w *KuadrantRegistry) trackableClusterInfo(u *unstructured.Unstructured) (ClusterInfo, bool) {
	if isKuadrantDisabled(u) {
		w.logger.V(1).Info("skipping disabled MCPServerRegistration", "name", u.GetName())
		return ClusterInfo{}, false
	}

	labels := u.GetLabels()
	if labels[ManagedLabel] != managedLabelValue {
		return ClusterInfo{}, false
	}

	info, err := extractKuadrantClusterInfo(u)
	if err != nil {
		w.logger.Error(err, "failed to extract cluster info", "name", u.GetName())
		w.metrics.NilSafeIncReconcileError()
		return ClusterInfo{}, false
	}

	return info, true
}

func (w *KuadrantRegistry) onUpdate(oldObj, newObj interface{}) {
	u, ok := newObj.(*unstructured.Unstructured)
	if !ok {
		return
	}

	if isKuadrantDisabled(u) {
		w.removeIfCurrent(u)
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

func (w *KuadrantRegistry) onDelete(obj interface{}) {
	handleDeletedCluster(obj, &w.mu, w.currentMaps, &w.revision, w.metrics, w.logger, w.emit)
}

func (w *KuadrantRegistry) removeIfCurrent(obj interface{}) {
	w.onDelete(obj)
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
	registrationUID := string(u.GetUID())
	if registrationUID == "" {
		return ClusterInfo{}, fmt.Errorf("MCPServerRegistration %q has empty metadata.uid", name)
	}

	info := ClusterInfo{
		ID:              name,
		RegistrationUID: registrationUID,
		Namespace:       u.GetNamespace(),
		Labels:          u.GetLabels(),
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
