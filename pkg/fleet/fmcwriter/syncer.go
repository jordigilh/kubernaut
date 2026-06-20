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

// Package fmcwriter implements the Fleet Metadata Cache writer service.
// It polls remote clusters via MCP Gateway for resources labeled
// kubernaut.ai/managed=true and writes their metadata to Valkey for
// low-latency scope checking by GW and RO services.
package fmcwriter

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// ReaderFactory creates a cluster-scoped client.Reader for the given clusterID.
// The FMC Writer calls this once per cluster per sync cycle, enabling per-cluster
// session reuse via NewFromSession without creating separate MCP connections.
type ReaderFactory func(ctx context.Context, clusterID string) (client.Reader, error)

// CacheWriter abstracts Valkey SET operations for writing managed resource keys.
type CacheWriter interface {
	Set(ctx context.Context, key string, ttl time.Duration) error
	Close() error
}

// Config holds FMC Writer configuration.
type Config struct {
	SyncInterval time.Duration
	KeyTTL       time.Duration
	ResourceKinds []string
}

// DefaultConfig returns production defaults for FMC Writer.
func DefaultConfig() Config {
	return Config{
		SyncInterval:  30 * time.Second,
		KeyTTL:        45 * time.Second,
		ResourceKinds: []string{"Deployment", "StatefulSet", "DaemonSet", "Pod", "Service", "Node"},
	}
}

// Syncer polls remote clusters for managed resources and writes them to the cache.
type Syncer struct {
	registry      registry.ClusterRegistry
	readerFactory ReaderFactory
	writer        CacheWriter
	config        Config
	logger        logr.Logger
	metrics       *Metrics

	mu      sync.Mutex
	running bool
}

// Metrics tracks FMC Writer operational metrics.
type Metrics struct {
	SyncTotal     *prometheus.CounterVec
	SyncErrors    *prometheus.CounterVec
	SyncDuration  *prometheus.HistogramVec
	KeysWritten   *prometheus.CounterVec
}

// NewMetrics creates FMC Writer Prometheus metrics.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		SyncTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fmc_sync_total",
			Help: "Total number of sync cycles per cluster",
		}, []string{"cluster_id"}),
		SyncErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fmc_sync_errors_total",
			Help: "Total number of sync errors per cluster",
		}, []string{"cluster_id", "reason"}),
		SyncDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "fmc_sync_duration_seconds",
			Help:    "Duration of sync cycles per cluster",
			Buckets: prometheus.DefBuckets,
		}, []string{"cluster_id"}),
		KeysWritten: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fmc_keys_written_total",
			Help: "Total number of keys written to Valkey per cluster",
		}, []string{"cluster_id"}),
	}
	reg.MustRegister(m.SyncTotal, m.SyncErrors, m.SyncDuration, m.KeysWritten)
	return m
}

// NewSyncerWithReaderFactory creates a new FMC Writer syncer backed by a ReaderFactory.
// The factory creates per-cluster client.Reader instances each sync cycle.
func NewSyncerWithReaderFactory(registry registry.ClusterRegistry, factory ReaderFactory, writer CacheWriter, config Config, logger logr.Logger, metrics *Metrics) *Syncer {
	return &Syncer{
		registry:      registry,
		readerFactory: factory,
		writer:        writer,
		config:        config,
		logger:        logger,
		metrics:       metrics,
	}
}

// SyncCluster performs a single sync cycle for one cluster:
// 1. List resources with kubernaut.ai/managed=true label via MCP
// 2. Parse response to extract resource metadata
// 3. Write keys to Valkey with TTL
func (s *Syncer) SyncCluster(ctx context.Context, cluster registry.ClusterInfo) error {
	start := time.Now()
	s.metrics.SyncTotal.WithLabelValues(cluster.ID).Inc()

	var totalKeys int
	for _, kind := range s.config.ResourceKinds {
		keys, err := s.syncKind(ctx, cluster, kind)
		if err != nil {
			s.metrics.SyncErrors.WithLabelValues(cluster.ID, "list_failed").Inc()
			s.logger.Error(err, "Failed to sync kind",
				"cluster", cluster.ID, "kind", kind)
			continue
		}
		totalKeys += keys
	}

	duration := time.Since(start)
	s.metrics.SyncDuration.WithLabelValues(cluster.ID).Observe(duration.Seconds())
	s.metrics.KeysWritten.WithLabelValues(cluster.ID).Add(float64(totalKeys))

	s.logger.V(1).Info("Sync cycle complete",
		"cluster", cluster.ID, "keys_written", totalKeys, "duration", duration)
	return nil
}

func (s *Syncer) syncKind(ctx context.Context, cluster registry.ClusterInfo, kind string) (int, error) {
	reader, err := s.readerFactory(ctx, cluster.ID)
	if err != nil {
		return 0, fmt.Errorf("syncKind: create reader for kind=%s cluster=%s: %w", kind, cluster.ID, err)
	}

	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{Kind: kind + "List"})

	err = reader.List(ctx, list,
		client.MatchingLabels{scope.ManagedLabelKey: scope.ManagedLabelValueTrue},
	)
	if err != nil {
		return 0, fmt.Errorf("syncKind: list kind=%s cluster=%s with label %s=%s: %w",
			kind, cluster.ID, scope.ManagedLabelKey, scope.ManagedLabelValueTrue, err)
	}

	var written int
	for i := range list.Items {
		item := &list.Items[i]
		gvk := item.GroupVersionKind()
		key, keyErr := scopecache.BuildKey(cluster.ID, gvk.Group, gvk.Version, item.GetKind(), item.GetNamespace(), item.GetName())
		if keyErr != nil {
			s.logger.Error(keyErr, "Invalid resource metadata, skipping",
				"cluster", cluster.ID, "kind", item.GetKind(),
				"namespace", item.GetNamespace(), "name", item.GetName())
			continue
		}
		if err := s.writer.Set(ctx, key, s.config.KeyTTL); err != nil {
			s.logger.Error(err, "Failed to write key",
				"cluster", cluster.ID, "key", key)
			continue
		}
		written++
	}
	return written, nil
}

// Run starts the FMC Writer main loop. It syncs all known clusters at the configured interval
// and reacts to cluster add/remove events from the registry.
func (s *Syncer) Run(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("syncer already running")
	}
	s.running = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	ticker := time.NewTicker(s.config.SyncInterval)
	defer ticker.Stop()

	events := s.registry.WatchClusters()

	s.syncAll(ctx)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("FMC Writer shutting down")
			return nil
		case <-ticker.C:
			s.syncAll(ctx)
		case event, ok := <-events:
			if !ok {
				s.logger.Info("Cluster registry channel closed")
				return nil
			}
			s.handleClusterEvent(ctx, event)
		}
	}
}

func (s *Syncer) syncAll(ctx context.Context) {
	clusters := s.registry.List()
	for _, cluster := range clusters {
		if err := s.SyncCluster(ctx, cluster); err != nil {
			s.logger.Error(err, "Sync failed", "cluster", cluster.ID)
		}
	}
}

func (s *Syncer) handleClusterEvent(ctx context.Context, event registry.ClusterEvent) {
	switch event.Type {
	case registry.EventAdded:
		s.logger.Info("New cluster discovered, syncing immediately", "cluster", event.Cluster.ID)
		if err := s.SyncCluster(ctx, event.Cluster); err != nil {
			s.logger.Error(err, "Initial sync for new cluster failed", "cluster", event.Cluster.ID)
		}
	case registry.EventDeleted:
		s.logger.Info("Cluster removed from registry", "cluster", event.Cluster.ID)
	case registry.EventUpdated:
		s.logger.V(1).Info("Cluster updated", "cluster", event.Cluster.ID)
	}
}

