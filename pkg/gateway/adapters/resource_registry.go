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

package adapters

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"golang.org/x/sync/singleflight"
)

var (
	tier1Kinds = map[string]bool{
		"Deployment": true, "StatefulSet": true, "DaemonSet": true,
		"CronJob": true, "DeploymentConfig": true,
	}
	tier3Kinds = map[string]bool{
		"Pod": true, "Node": true,
	}

	coreBatchAppsGroups = map[string]bool{
		"": true, "apps": true, "batch": true,
		"autoscaling": true, "policy": true,
	}
)

type registrySnapshot struct {
	labelToKind    map[string]string
	kindToGVR      map[string]schema.GroupVersionResource
	kindToGroup    map[string]string
	kindNamespaced map[string]bool
}

type existenceCacheEntry struct {
	exists    bool
	expiresAt time.Time
}

// APIResourceRegistry provides discovery-backed label-to-kind and kind-to-GVR
// resolution for the gateway's signal ingestion pipeline. It replaces the
// static resourceCandidates and kindToGroup maps with fully dynamic API
// discovery. Issue #1029.
// DefaultMaxCacheSize is the upper bound on cached existence entries to prevent
// unbounded memory growth between refresh cycles.
const DefaultMaxCacheSize = 10000

type APIResourceRegistry struct {
	dc        discovery.DiscoveryInterface
	dynClient dynamic.Interface
	mu        sync.RWMutex
	snapshot  *registrySnapshot

	cacheMu      sync.Mutex
	cache        map[string]existenceCacheEntry
	cacheTTL     time.Duration
	maxCacheSize int

	sfGroup singleflight.Group

	refreshInterval     time.Duration
	logger              logr.Logger
	refreshErrorCounter prometheus.Counter // optional: tracks discovery refresh failures
}

// RegistryOption configures the APIResourceRegistry.
type RegistryOption func(*APIResourceRegistry)

// WithRefreshInterval sets the periodic discovery refresh interval.
func WithRefreshInterval(d time.Duration) RegistryOption {
	return func(r *APIResourceRegistry) {
		r.refreshInterval = d
	}
}

// WithCacheTTL sets the TTL for existence check cache entries.
func WithCacheTTL(d time.Duration) RegistryOption {
	return func(r *APIResourceRegistry) {
		r.cacheTTL = d
	}
}

// WithDynamicClient sets the dynamic client used for existence checks.
// When nil, CheckExistence returns false (stub behavior).
func WithDynamicClient(client dynamic.Interface) RegistryOption {
	return func(r *APIResourceRegistry) {
		r.dynClient = client
	}
}

// WithRegistryLogger sets the logger for the registry.
func WithRegistryLogger(l logr.Logger) RegistryOption {
	return func(r *APIResourceRegistry) {
		r.logger = l
	}
}

// WithMaxCacheSize sets the upper bound on existence cache entries.
// When the cache exceeds this size, expired entries are evicted first; if still
// over capacity the entire cache is reset. Zero means use DefaultMaxCacheSize.
func WithMaxCacheSize(n int) RegistryOption {
	return func(r *APIResourceRegistry) {
		if n > 0 {
			r.maxCacheSize = n
		}
	}
}

// WithRefreshErrorCounter sets the Prometheus counter incremented on discovery refresh failures.
func WithRefreshErrorCounter(c prometheus.Counter) RegistryOption {
	return func(r *APIResourceRegistry) {
		r.refreshErrorCounter = c
	}
}

// NewAPIResourceRegistry constructs a registry by querying the Kubernetes API
// discovery endpoint. Returns an error if discovery is unavailable (fail-fast).
func NewAPIResourceRegistry(dc discovery.DiscoveryInterface, opts ...RegistryOption) (*APIResourceRegistry, error) {
	r := &APIResourceRegistry{
		dc:              dc,
		cache:           make(map[string]existenceCacheEntry),
		cacheTTL:        30 * time.Second,
		maxCacheSize:    DefaultMaxCacheSize,
		refreshInterval: 5 * time.Minute,
		logger:          logr.Discard(),
	}
	for _, o := range opts {
		o(r)
	}

	snap, err := buildSnapshot(dc)
	if err != nil {
		return nil, fmt.Errorf("discovery failed during gateway startup — verify the gateway "+
			"ServiceAccount has discovery RBAC (system:discovery ClusterRoleBinding or explicit "+
			"non-resource URL rules for /api and /apis): %w", err)
	}
	if len(snap.labelToKind) == 0 {
		return nil, fmt.Errorf("discovery returned zero API resources — verify the gateway "+
			"ServiceAccount has discovery RBAC (system:discovery ClusterRoleBinding or explicit "+
			"non-resource URL rules for /api and /apis)")
	}
	r.snapshot = snap
	r.logger.Info("API resource registry initialized",
		"kind_count", len(snap.kindToGVR))
	return r, nil
}

func buildSnapshot(dc discovery.DiscoveryInterface) (*registrySnapshot, error) {
	_, lists, err := dc.ServerGroupsAndResources()
	if err != nil && lists == nil {
		return nil, err
	}
	// Partial failure is OK — client-go returns partial results + error for
	// groups it can't enumerate (IsGroupDiscoveryFailedError).

	totalResources := 0
	for _, list := range lists {
		if list != nil {
			totalResources += len(list.APIResources)
		}
	}
	snap := &registrySnapshot{
		labelToKind:    make(map[string]string, totalResources),
		kindToGVR:      make(map[string]schema.GroupVersionResource, totalResources),
		kindToGroup:    make(map[string]string, totalResources),
		kindNamespaced: make(map[string]bool, totalResources),
	}

	for _, list := range lists {
		if list == nil {
			continue
		}
		gv, parseErr := schema.ParseGroupVersion(list.GroupVersion)
		if parseErr != nil {
			continue
		}
		for _, res := range list.APIResources {
			snap.addAPIResource(gv, res)
		}
	}

	return snap, nil
}

// addAPIResource registers a single discovered APIResource into the
// snapshot's lookup maps. Extracted from buildSnapshot to keep its cognitive
// complexity low; skips subresources and resources with no Kind.
func (snap *registrySnapshot) addAPIResource(gv schema.GroupVersion, res metav1.APIResource) {
	if res.Kind == "" {
		return
	}
	// Skip subresources (contain '/')
	if strings.Contains(res.Name, "/") {
		return
	}

	gvr := schema.GroupVersionResource{
		Group:    gv.Group,
		Version:  gv.Version,
		Resource: res.Name,
	}

	if _, exists := snap.kindToGVR[res.Kind]; !exists {
		snap.kindToGVR[res.Kind] = gvr
		snap.kindToGroup[res.Kind] = gv.Group
		snap.kindNamespaced[res.Kind] = res.Namespaced
	}

	singular := res.SingularName
	if singular == "" {
		singular = strings.ToLower(res.Kind)
	}
	if _, exists := snap.labelToKind[singular]; !exists {
		snap.labelToKind[singular] = res.Kind
	}

	lowerKind := strings.ToLower(res.Kind)
	if lowerKind != singular {
		if _, exists := snap.labelToKind[lowerKind]; !exists {
			snap.labelToKind[lowerKind] = res.Kind
		}
	}
}

// LabelToKind returns the Kubernetes Kind for a given label key by matching
// against discovered APIResource.SingularName and lowercase Kind.
// Returns empty string if no match found.
func (r *APIResourceRegistry) LabelToKind(labelKey string) string {
	r.mu.RLock()
	snap := r.snapshot
	r.mu.RUnlock()
	if snap == nil {
		return ""
	}
	return snap.labelToKind[labelKey]
}

// KindToGVR returns the GroupVersionResource for a given Kind string.
// Returns zero-value GVR and false if the kind is not in the registry.
func (r *APIResourceRegistry) KindToGVR(kind string) (schema.GroupVersionResource, bool) {
	r.mu.RLock()
	snap := r.snapshot
	r.mu.RUnlock()
	if snap == nil {
		return schema.GroupVersionResource{}, false
	}
	gvr, ok := snap.kindToGVR[kind]
	return gvr, ok
}

// IsNamespacedKind returns true if the given Kind is namespaced according to
// API discovery. Returns true (conservative default) for unknown kinds or when
// the registry snapshot is not yet initialized — this avoids accidentally
// dropping namespace information for resources that actually need it.
func (r *APIResourceRegistry) IsNamespacedKind(kind string) bool {
	r.mu.RLock()
	snap := r.snapshot
	r.mu.RUnlock()
	if snap == nil {
		return true
	}
	namespaced, ok := snap.kindNamespaced[kind]
	if !ok {
		return true
	}
	return namespaced
}

// TierForKind returns the priority tier for a given Kind.
// Tier 1 = controllers (highest priority), Tier 2 = managed, Tier 3 = leaf.
// Unknown kinds default to Tier 2.
func (r *APIResourceRegistry) TierForKind(kind string) int {
	if tier1Kinds[kind] {
		return 1
	}
	if tier3Kinds[kind] {
		return 3
	}
	return 2
}

// IsCoreBatchAppsKind returns true if the kind belongs to core, apps, batch,
// autoscaling, or policy API groups (used to determine whether owner chain
// traversal is appropriate).
func (r *APIResourceRegistry) IsCoreBatchAppsKind(kind string) bool {
	r.mu.RLock()
	snap := r.snapshot
	r.mu.RUnlock()
	if snap == nil {
		return false
	}
	group, ok := snap.kindToGroup[kind]
	if !ok {
		return false
	}
	return coreBatchAppsGroups[group]
}

// Refresh re-queries the discovery API and atomically swaps the internal maps.
// Preserves the previous good map on failure.
func (r *APIResourceRegistry) Refresh(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	snap, err := buildSnapshot(r.dc)
	if err != nil {
		r.logger.Error(err, "Discovery refresh failed, preserving previous map")
		return err
	}
	if len(snap.labelToKind) == 0 {
		refreshErr := fmt.Errorf("discovery refresh returned zero resources, preserving previous map")
		r.logger.Error(refreshErr, "Discovery refresh returned empty results")
		return refreshErr
	}

	r.mu.Lock()
	r.snapshot = snap
	r.mu.Unlock()

	r.cacheMu.Lock()
	r.cache = make(map[string]existenceCacheEntry)
	r.cacheMu.Unlock()

	r.logger.Info("API resource registry refreshed",
		"kind_count", len(snap.kindToGVR))
	return nil
}

// StartRefreshLoop starts a goroutine that periodically refreshes the registry.
// Stops when the context is cancelled. If a panic occurs, the loop re-enters
// after a brief backoff. After 3 consecutive panics within 1 minute, the loop
// terminates permanently to prevent infinite crash loops.
func (r *APIResourceRegistry) StartRefreshLoop(ctx context.Context) {
	go func() {
		const maxConsecutivePanics = 3
		const panicWindow = time.Minute
		var panicTimestamps []time.Time

		for {
			exited := r.runRefreshTicker(ctx, &panicTimestamps, panicWindow, maxConsecutivePanics)
			if exited {
				return
			}
			// Backoff before re-entering after panic recovery
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}()
}

// runRefreshTicker runs the ticker loop. Returns true if the loop should stop
// permanently (context cancelled or panic budget exhausted), false to re-enter.
func (r *APIResourceRegistry) runRefreshTicker(ctx context.Context, panicTimestamps *[]time.Time, panicWindow time.Duration, maxPanics int) (stop bool) {
	defer func() {
		if p := recover(); p != nil {
			now := time.Now()
			r.logger.Error(fmt.Errorf("panic: %v", p),
				"API resource registry refresh loop panicked, will attempt re-entry")

			cutoff := now.Add(-panicWindow)
			filtered := (*panicTimestamps)[:0]
			for _, ts := range *panicTimestamps {
				if ts.After(cutoff) {
					filtered = append(filtered, ts)
				}
			}
			*panicTimestamps = append(filtered, now)

			if len(*panicTimestamps) >= maxPanics {
				r.logger.Error(fmt.Errorf("%d panics in %v", maxPanics, panicWindow),
					"API resource registry refresh loop exceeded panic budget, stopping permanently")
				stop = true
				return
			}
			stop = false
			return
		}
	}()

	ticker := time.NewTicker(r.refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return true
		case <-ticker.C:
			if err := r.Refresh(ctx); err != nil && r.refreshErrorCounter != nil {
				r.refreshErrorCounter.Inc()
			}
		}
	}
}

func existenceCacheKey(gvr schema.GroupVersionResource, namespace, name string) string {
	var b strings.Builder
	b.Grow(len(namespace) + len(gvr.Group) + len(gvr.Version) + len(gvr.Resource) + len(name) + 4)
	b.WriteString(namespace)
	b.WriteByte(':')
	b.WriteString(gvr.Group)
	b.WriteByte('/')
	b.WriteString(gvr.Version)
	b.WriteByte('/')
	b.WriteString(gvr.Resource)
	b.WriteByte(':')
	b.WriteString(name)
	return b.String()
}

// CheckExistence verifies whether a resource exists in the given namespace.
// Uses a short-lived TTL cache to bound API server load and a singleflight
// group to coalesce concurrent lookups for the same key.
// Returns true if the resource exists, false otherwise.
// Errors (403, timeout) are treated as "does not exist" with a warning log.
func (r *APIResourceRegistry) CheckExistence(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string) bool {
	key := existenceCacheKey(gvr, namespace, name)

	r.cacheMu.Lock()
	entry, found := r.cache[key]
	if found && time.Now().Before(entry.expiresAt) {
		r.cacheMu.Unlock()
		return entry.exists
	}
	r.cacheMu.Unlock()

	v, _, _ := r.sfGroup.Do(key, func() (interface{}, error) {
		exists := false
		transient := false
		if r.dynClient != nil {
			_, err := r.dynClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
			if err == nil {
				exists = true
			} else if apierrors.IsNotFound(err) {
				exists = false
			} else {
				r.logger.V(1).Info("CheckExistence transient error — not caching",
					"gvr", gvr.String(), "namespace", namespace, "name", name, "error", err)
				transient = true
			}
		}

		if !transient {
			r.cacheMu.Lock()
			r.evictExpiredLocked()
			r.cache[key] = existenceCacheEntry{
				exists:    exists,
				expiresAt: time.Now().Add(r.cacheTTL),
			}
			r.cacheMu.Unlock()
		}

		return exists, nil
	})

	return v.(bool)
}

// evictExpiredLocked removes expired cache entries and, if still over capacity,
// resets the cache. Must be called while cacheMu is held.
func (r *APIResourceRegistry) evictExpiredLocked() {
	if len(r.cache) < r.maxCacheSize {
		return
	}
	now := time.Now()
	for k, e := range r.cache {
		if now.After(e.expiresAt) {
			delete(r.cache, k)
		}
	}
	if len(r.cache) >= r.maxCacheSize {
		r.logger.Info("Existence cache exceeded max size after eviction, resetting",
			"max_size", r.maxCacheSize, "current_size", len(r.cache))
		r.cache = make(map[string]existenceCacheEntry)
	}
}

// KindCount returns the number of discovered kinds in the registry.
func (r *APIResourceRegistry) KindCount() int {
	r.mu.RLock()
	snap := r.snapshot
	r.mu.RUnlock()
	if snap == nil {
		return 0
	}
	return len(snap.kindToGVR)
}
