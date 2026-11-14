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

// Package fmc implements the Fleet Metadata Cache service.
// It polls remote clusters via MCP Gateway for resources labeled
// kubernaut.ai/managed=true, caches their metadata in Valkey, and
// exposes scope queries to GW and RO services.
package fmc

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/pkg/fleet/observation"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// kindToGVK maps synced resource Kind names to their full GroupVersionKind.
// kube-mcp-server requires apiVersion as a mandatory parameter; omitting it
// causes "missing argument apiVersion" errors.
var kindToGVK = map[string]schema.GroupVersionKind{
	"Deployment":  {Group: "apps", Version: "v1", Kind: "Deployment"},
	"StatefulSet": {Group: "apps", Version: "v1", Kind: "StatefulSet"},
	"DaemonSet":   {Group: "apps", Version: "v1", Kind: "DaemonSet"},
	"Pod":         {Group: "", Version: "v1", Kind: "Pod"},
	"Service":     {Group: "", Version: "v1", Kind: "Service"},
	"Node":        {Group: "", Version: "v1", Kind: "Node"},
	"Namespace":   {Group: "", Version: "v1", Kind: "Namespace"},
}

// ObservedReader is the typed, complete-scan source contract used by FMC.
// A bare client.Reader is intentionally not accepted because it cannot prove
// source completeness or preserve explicit false/invalid label state.
type ObservedReader = observation.ObservedReader

// ObservedReaderFactory creates a typed reader for the current registration.
// Passing the complete ClusterInfo preserves registration identity and tool
// prefix metadata at the FMC boundary.
type ObservedReaderFactory interface {
	ReaderFor(ctx context.Context, cluster registry.ClusterInfo) (ObservedReader, error)
}

// ObservedReaderFactoryFunc adapts a function to ObservedReaderFactory.
type ObservedReaderFactoryFunc func(context.Context, registry.ClusterInfo) (ObservedReader, error)

func (f ObservedReaderFactoryFunc) ReaderFor(ctx context.Context, cluster registry.ClusterInfo) (ObservedReader, error) {
	return f(ctx, cluster)
}

// CacheWriter abstracts Valkey SET operations for writing managed resource keys.
type CacheWriter interface {
	Set(ctx context.Context, key string, ttl time.Duration) error
	Close() error
}

// ObservationWriter is the presence-aware extension used by FMC source
// synchronization. A writer that only implements the legacy Set method cannot
// safely represent explicit false, absent, or invalid label states.
type ObservationWriter interface {
	SetObservation(ctx context.Context, key string, labelState scope.LabelState, ttl time.Duration) error
}

// Config holds FMC Writer configuration.
type Config struct {
	SyncInterval       time.Duration
	KeyTTL             time.Duration
	ResourceKinds      []string
	WaitForBrokerReady bool
	LeaseRenewInterval time.Duration

	// Broker readiness probe backoff settings. Zero values use production defaults.
	BrokerProbeInitialInterval time.Duration
	BrokerProbeMaxInterval     time.Duration
	BrokerProbeTimeout         time.Duration
}

// DefaultResourceKinds lists the resource kinds FMC syncs by default.
// Namespace is included so scopecache.Client.IsManagedResource can fall back
// to a namespace-level kubernaut.ai/managed label when a resource itself is
// unlabeled -- mirroring pkg/shared/scope.Manager's 2-level hierarchy for the
// local cluster (ADR-053, Issue #2311). Without it, fleet scope checks were
// silently stricter than local ones: a namespace-only label worked on the hub
// but never on a managed remote cluster.
//
// Exported so pkg/fleet/fmc/config.DefaultServiceConfig can reuse this exact
// list instead of hand-maintaining a second copy -- the two defaults drifting
// out of sync (one updated, one not) is exactly how Issue #2311 slipped
// through: the config-layer default kept the pre-fix kind list even after a
// first pass at this fix updated only this one.
var DefaultResourceKinds = []string{"Deployment", "StatefulSet", "DaemonSet", "Pod", "Service", "Node", "Namespace"}

// DefaultConfig returns production defaults for FMC Writer.
func DefaultConfig() Config {
	return Config{
		SyncInterval:  30 * time.Second,
		KeyTTL:        45 * time.Second,
		ResourceKinds: DefaultResourceKinds,
	}
}

// Syncer polls remote clusters for managed resources and writes them to the cache.
type Syncer struct {
	registry      registry.ClusterRegistry
	readerFactory ObservedReaderFactory
	writer        CacheWriter
	config        Config
	logger        logr.Logger
	metrics       *Metrics
	statusStore   scopecache.StatusStore
	lease         scopecache.LeaseCoordinator
	readiness     *scopecache.FMCReadinessSnapshot

	mu             sync.Mutex
	running        bool
	lifecycleReady atomic.Bool
	fenceMu        sync.Mutex
	fence          scopecache.WriterFence
	holdingFence   bool
	leaseCancel    context.CancelFunc
	leaseDone      chan struct{}
	leaseSession   scopecache.LeaseSession
}

// Dependencies is the production composition boundary for the FMC writer.
// The lifecycle dependencies are optional only for legacy unit callers; the
// production command supplies both StatusStore and LeaseCoordinator.
type Dependencies struct {
	Registry    registry.ClusterRegistry
	Readers     ObservedReaderFactory
	Writer      CacheWriter
	StatusStore scopecache.StatusStore
	Lease       scopecache.LeaseCoordinator
	Readiness   *scopecache.FMCReadinessSnapshot
}

// Metrics tracks FMC Writer operational metrics.
type Metrics struct {
	SyncTotal    *prometheus.CounterVec
	SyncErrors   *prometheus.CounterVec
	SyncDuration *prometheus.HistogramVec
	KeysWritten  *prometheus.CounterVec
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
func NewSyncerWithReaderFactory(registry registry.ClusterRegistry, factory ObservedReaderFactory, writer CacheWriter, config Config, logger logr.Logger, metrics *Metrics) *Syncer {
	return &Syncer{
		registry:      registry,
		readerFactory: factory,
		writer:        writer,
		config:        config,
		logger:        logger,
		metrics:       metrics,
	}
}

// NewSyncer creates a syncer with the complete production dependency set.
func NewSyncer(deps Dependencies, config Config, logger logr.Logger, metrics *Metrics) *Syncer {
	return &Syncer{
		registry:      deps.Registry,
		readerFactory: deps.Readers,
		writer:        deps.Writer,
		config:        config,
		logger:        logger,
		metrics:       metrics,
		statusStore:   deps.StatusStore,
		lease:         deps.Lease,
		readiness:     deps.Readiness,
	}
}

// Ready reports whether the sync lifecycle has produced an authoritative
// committed state. An empty synced registry is ready because there is no
// remote partition to authorize.
func (s *Syncer) Ready() bool {
	if s.statusStore == nil && s.lease == nil && s.readiness == nil {
		return true
	}
	if s.readiness != nil {
		ready, err := s.readiness.ServiceReady()
		return err == nil && ready
	}
	return s.lifecycleReady.Load()
}

// SyncCluster performs a single sync cycle for one cluster:
// 1. Collect complete, unfiltered observations for every configured kind.
// 2. Validate all identities and label states before any cache mutation.
// 3. Write verified observations to Valkey with TTL.
func (s *Syncer) lifecycleEnabled() (bool, error) {
	enabled := s.statusStore != nil || s.lease != nil || s.readiness != nil
	if enabled && (s.statusStore == nil || s.lease == nil || s.readiness == nil) {
		return false, fmt.Errorf("FMC lifecycle requires status store, lease coordinator, and readiness snapshot")
	}
	return enabled, nil
}

func (s *Syncer) currentRegistrationSnapshot(cluster registry.ClusterInfo) (registry.RegistrationSnapshot, error) {
	guard, err := currentRegistrationGuard(s.registry)
	if err != nil {
		return registry.RegistrationSnapshot{}, err
	}
	snapshot, found := guard.Snapshot(cluster.ID)
	if !found || snapshot.Info.RegistrationUID == "" || snapshot.Info.RegistrationUID != cluster.RegistrationUID {
		return registry.RegistrationSnapshot{}, fmt.Errorf("cluster %s registration is unavailable or stale", cluster.ID)
	}
	return snapshot, nil
}

func currentRegistrationGuard(reg registry.ClusterRegistry) (registry.CurrentRegistrationGuard, error) {
	guard, ok := reg.(registry.CurrentRegistrationGuard)
	if !ok {
		return nil, fmt.Errorf("FMC registry does not provide current-registration fencing")
	}
	return guard, nil
}

func verifyRepairRegistration(guard registry.CurrentRegistrationGuard, cluster registry.ClusterInfo, snapshot registry.RegistrationSnapshot) error {
	latest, found := guard.Snapshot(cluster.ID)
	if !found || latest.Info.RegistrationUID != cluster.RegistrationUID || latest.Revision != snapshot.Revision {
		return fmt.Errorf("cluster %s registration changed during repair collection", cluster.ID)
	}
	return nil
}

func (s *Syncer) commitRepair(ctx context.Context, guard registry.CurrentRegistrationGuard, request scopecache.RepairCommitRequest) (scopecache.RepairCommitResult, error) {
	var result scopecache.RepairCommitResult
	err := guard.WithCurrent(ctx, request.Partition.ClusterID, request.RegistrationUID, request.ExpectedRegistryRevision, func(registry.ClusterInfo) error {
		if err := s.lease.Verify(ctx, request.WriterFence); err != nil {
			return err
		}
		var err error
		result, err = s.statusStore.CommitRepair(ctx, request)
		return err
	})
	return result, err
}

func (s *Syncer) SyncCluster(ctx context.Context, cluster registry.ClusterInfo) error {
	start := time.Now()
	s.metrics.SyncTotal.WithLabelValues(cluster.ID).Inc()

	lifecycleEnabled, err := s.lifecycleEnabled()
	if err != nil {
		return err
	}
	if !lifecycleEnabled {
		return s.syncLegacyCluster(ctx, cluster, start)
	}
	snapshot, err := s.currentRegistrationSnapshot(cluster)
	if err != nil {
		return err
	}
	session, err := s.ensureFence(ctx)
	if err != nil {
		return fmt.Errorf("acquire FMC writer fence for cluster %s: %w", cluster.ID, err)
	}
	if err := s.publishGlobal(session); err != nil {
		return fmt.Errorf("publish FMC global readiness for cluster %s: %w", cluster.ID, err)
	}
	result, err := s.repairPartition(ctx, snapshot.Info, snapshot, session.Fence)
	if err != nil {
		s.metrics.SyncErrors.WithLabelValues(cluster.ID, "repair_failed").Inc()
		return fmt.Errorf("sync cluster %s repair: %w", cluster.ID, err)
	}
	if !result.CleanCommitEligible {
		return fmt.Errorf("sync cluster %s repair returned no clean proof", cluster.ID)
	}

	duration := time.Since(start)
	s.metrics.SyncDuration.WithLabelValues(cluster.ID).Observe(duration.Seconds())
	s.metrics.KeysWritten.WithLabelValues(cluster.ID).Add(float64(result.ObservedCount))
	s.logger.V(1).Info("Sync cycle complete",
		"cluster", cluster.ID, "keys_written", result.ObservedCount, "duration", duration)
	return nil
}

func (s *Syncer) syncLegacyCluster(ctx context.Context, cluster registry.ClusterInfo, start time.Time) error {
	var observations []observedResource
	for _, kind := range s.config.ResourceKinds {
		kindResult, err := s.syncKind(ctx, cluster, kind)
		if err != nil {
			s.metrics.SyncErrors.WithLabelValues(cluster.ID, "list_failed").Inc()
			s.logger.Error(err, "Failed to sync kind", "cluster", cluster.ID, "kind", kind)
			return fmt.Errorf("sync cluster %s incomplete: %w", cluster.ID, err)
		}
		observations = append(observations, kindResult.Resources...)
	}
	seenKeys := make(map[string]struct{}, len(observations))
	for _, observation := range observations {
		if _, exists := seenKeys[observation.key]; exists {
			err := fmt.Errorf("sync cluster %s has duplicate resource identity %q", cluster.ID, observation.key)
			s.metrics.SyncErrors.WithLabelValues(cluster.ID, "duplicate_identity").Inc()
			return err
		}
		seenKeys[observation.key] = struct{}{}
	}
	for _, observed := range observations {
		if err := observed.writer.SetObservation(ctx, observed.key, observed.labelState, s.config.KeyTTL); err != nil {
			s.metrics.SyncErrors.WithLabelValues(cluster.ID, "write_failed").Inc()
			return fmt.Errorf("sync cluster %s write observation %q: %w", cluster.ID, observed.key, err)
		}
	}
	duration := time.Since(start)
	s.metrics.SyncDuration.WithLabelValues(cluster.ID).Observe(duration.Seconds())
	s.metrics.KeysWritten.WithLabelValues(cluster.ID).Add(float64(len(observations)))
	return nil
}

func (s *Syncer) beginRepair(ctx context.Context, partition scopecache.PartitionStateKey, cluster registry.ClusterInfo, snapshot registry.RegistrationSnapshot, fence scopecache.WriterFence) (scopecache.RepairStartResult, error) {
	return s.statusStore.BeginRepair(ctx, scopecache.BeginRepairRequest{
		Partition:                partition,
		RegistrationUID:          cluster.RegistrationUID,
		ExpectedRegistryRevision: snapshot.Revision,
		WriterFence:              fence,
	})
}

func (s *Syncer) markRepairUncertain(ctx context.Context, start scopecache.RepairStartResult, clusterID string, failure scopecache.RepairFailure) {
	cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cleanupCancel()
	if err := s.statusStore.MarkRepairUncertain(cleanupCtx, start, failure); err != nil {
		s.logger.Error(err, "failed to mark FMC repair uncertain", "cluster", clusterID)
	}
}

// repairPartition performs the only production lifecycle that can publish a
// clean partition. BeginRepair runs before any MCP source collection.
func (s *Syncer) repairPartition(ctx context.Context, cluster registry.ClusterInfo, snapshot registry.RegistrationSnapshot, fence scopecache.WriterFence) (scopecache.RepairResult, error) {
	guard, err := currentRegistrationGuard(s.registry)
	if err != nil {
		return scopecache.RepairResult{}, err
	}
	partition := scopecache.PartitionStateKey{ClusterID: cluster.ID, RegistrationUID: cluster.RegistrationUID}
	start, err := s.beginRepair(ctx, partition, cluster, snapshot, fence)
	if err != nil {
		return scopecache.RepairResult{}, err
	}
	leaseView := s.leaseView(fence)
	if err := s.readiness.BeginPartitionRepair(partition, cluster.RegistrationUID, snapshot.Revision, leaseView, start.Generation); err != nil {
		s.markRepairUncertain(ctx, start, cluster.ID, scopecache.RepairFailure{Code: scopecache.RepairFailureInvalidRequest, Message: err.Error()})
		return scopecache.RepairResult{}, err
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		s.markRepairUncertain(ctx, start, cluster.ID, scopecache.RepairFailure{Code: scopecache.RepairFailureSourceIncomplete, Message: "repair did not commit"})
		_ = s.readiness.InvalidatePartition(partition, "repair did not commit")
	}()

	observations, err := s.collectRepairObservations(ctx, cluster)
	if err != nil {
		return scopecache.RepairResult{}, err
	}
	if err := verifyRepairRegistration(guard, cluster, snapshot); err != nil {
		return scopecache.RepairResult{}, err
	}
	if err := s.lease.Verify(ctx, fence); err != nil {
		return scopecache.RepairResult{}, fmt.Errorf("verify FMC writer fence before repair: %w", err)
	}
	repairResult, err := s.statusStore.Repair(ctx, scopecache.RepairRequest{
		Partition:                partition,
		RegistrationUID:          cluster.RegistrationUID,
		ExpectedRegistryRevision: snapshot.Revision,
		WriterFence:              fence,
		Generation:               start.Generation,
		Token:                    start.Token,
		Observations: scopecache.ValidatedObservationSet{
			Items:     observations.Items,
			Complete:  true,
			PageCount: observations.PageCount,
			ItemCount: len(observations.Items),
		},
	})
	if err != nil {
		return repairResult, err
	}
	commitResult, err := s.commitRepair(ctx, guard, scopecache.RepairCommitRequest{
		Partition:                partition,
		RegistrationUID:          cluster.RegistrationUID,
		ExpectedRegistryRevision: snapshot.Revision,
		WriterFence:              fence,
		Generation:               start.Generation,
		Token:                    start.Token,
		ExpectedCount:            len(observations.Items),
		Proof:                    repairResult,
	})
	if err != nil {
		return repairResult, err
	}
	if !commitResult.Committed || !commitResult.CleanCommit {
		return repairResult, fmt.Errorf("repair commit did not publish clean state")
	}
	if err := s.publishCleanPartition(partition, cluster, snapshot, fence, start.Generation, repairResult, commitResult); err != nil {
		return repairResult, err
	}
	committed = true
	return repairResult, nil
}

type repairObservationCollection struct {
	Items     []scopecache.RepairObservation
	PageCount int
}

func (s *Syncer) collectRepairObservations(ctx context.Context, cluster registry.ClusterInfo) (repairObservationCollection, error) {
	var observations []observedResource
	pageCount := 0
	for _, kind := range s.config.ResourceKinds {
		kindResult, err := s.syncKind(ctx, cluster, kind)
		if err != nil {
			return repairObservationCollection{}, fmt.Errorf("collect repair observations for kind %s: %w", kind, err)
		}
		observations = append(observations, kindResult.Resources...)
		pageCount += kindResult.PageCount
	}

	seenKeys := make(map[string]struct{}, len(observations))
	repairItems := make([]scopecache.RepairObservation, 0, len(observations))
	for _, observed := range observations {
		if _, exists := seenKeys[observed.key]; exists {
			return repairObservationCollection{}, fmt.Errorf("repair has duplicate resource identity %q", observed.key)
		}
		seenKeys[observed.key] = struct{}{}
		repairItems = append(repairItems, scopecache.RepairObservation{
			Identity:      observed.cacheObservation.Identity,
			APIVersion:    observed.apiVersion,
			LabelsPresent: observed.labelsPresent,
			LabelState:    observed.labelState,
			TTL:           observed.cacheObservation.TTL,
		})
	}
	return repairObservationCollection{Items: repairItems, PageCount: pageCount}, nil
}

func (s *Syncer) publishCleanPartition(
	partition scopecache.PartitionStateKey,
	cluster registry.ClusterInfo,
	snapshot registry.RegistrationSnapshot,
	fence scopecache.WriterFence,
	generation uint64,
	repairResult scopecache.RepairResult,
	commitResult scopecache.RepairCommitResult,
) error {
	clean := scopecache.PartitionReadiness{
		Key:                  partition,
		RegistrationUID:      cluster.RegistrationUID,
		RegistrationRevision: snapshot.Revision,
		LeaseEpoch:           fence.LeaseEpoch,
		LeaseSessionUID:      fence.HolderSessionUID,
		WriterFence:          fence,
		LeaseHeartbeatAt:     time.Now().UTC(),
		LeaseHeartbeatValid:  true,
		Generation:           generation,
		State:                scopecache.PartitionClean,
		CleanCommit:          commitResult.CleanCommit,
		RecordValid:          repairResult.RecordValid,
		IndexValid:           repairResult.IndexValid,
		StatusValid:          repairResult.StatusValid,
		RecordTTLValid:       repairResult.RecordTTL.Valid,
		IndexTTLValid:        repairResult.IndexTTL.Valid,
		StatusTTLValid:       repairResult.StatusTTL.Valid,
	}
	return s.readiness.PublishPartitionClean(partition, clean)
}

func (s *Syncer) leaseView(fence scopecache.WriterFence) scopecache.LeaseView {
	policy := scopecache.LeasePolicyProcessWideRequired
	if s.readiness != nil && s.readiness.View().Global.LeasePolicy != "" {
		policy = s.readiness.View().Global.LeasePolicy
	}
	return scopecache.LeaseView{
		Policy:         policy,
		Active:         true,
		Valid:          true,
		Epoch:          fence.LeaseEpoch,
		SessionUID:     fence.HolderSessionUID,
		WriterFence:    fence,
		HeartbeatAt:    time.Now().UTC(),
		HeartbeatValid: true,
	}
}

func (s *Syncer) publishGlobal(session scopecache.LeaseSession) error {
	if s.readiness == nil {
		return nil
	}
	global := s.readiness.View().Global
	global.MCPInitialSync = true
	global.RegistryInitialSync = s.registry.Ready()
	global.RegistryWatchHealthy = s.registry.Ready()
	global.ScriptReady = false
	if scriptReady, ok := s.statusStore.(interface{ ScriptReady() bool }); ok {
		global.ScriptReady = scriptReady.ScriptReady()
	}
	global.ValkeyReady = s.statusStore != nil
	global.Lease = s.leaseView(session.Fence)
	global.ActiveLeaseValid = true
	return s.readiness.PublishGlobal(global)
}

type observedResource struct {
	key              string
	labelState       scope.LabelState
	apiVersion       string
	labelsPresent    bool
	writer           ObservationWriter
	cacheObservation scopecache.Observation
}

type kindObservationResult struct {
	Resources []observedResource
	PageCount int
}

func (s *Syncer) syncKind(ctx context.Context, cluster registry.ClusterInfo, kind string) (kindObservationResult, error) {
	reader, err := s.readerFactory.ReaderFor(ctx, cluster)
	if err != nil {
		return kindObservationResult{}, fmt.Errorf("syncKind: create reader for kind=%s cluster=%s: %w", kind, cluster.ID, err)
	}

	gvk, ok := kindToGVK[kind]
	if !ok {
		return kindObservationResult{}, fmt.Errorf("syncKind: unknown resource kind %q (add to kindToGVK map)", kind)
	}

	observed, err := reader.ListObserved(ctx, observation.ListRequest{
		APIVersion: gvk.GroupVersion().String(),
		Kind:       kind,
		Limit:      observation.ObservationPageLimit,
	})
	if err != nil {
		return kindObservationResult{}, fmt.Errorf("syncKind: list kind=%s cluster=%s: %w", kind, cluster.ID, err)
	}
	if !observed.Complete {
		failure := observed.Failure
		if failure == observation.FailureNone {
			failure = observation.FailureMalformedContent
		}
		return kindObservationResult{}, fmt.Errorf("syncKind: incomplete observation kind=%s cluster=%s failure=%s", kind, cluster.ID, failure)
	}

	observationWriter, ok := s.writer.(ObservationWriter)
	if !ok && s.statusStore == nil {
		return kindObservationResult{}, fmt.Errorf("syncKind: writer does not support presence-aware observations")
	}

	resources := make([]observedResource, 0, len(observed.Items))
	for i := range observed.Items {
		resource, itemErr := s.buildObservedResource(cluster, kind, gvk, observationWriter, &observed.Items[i])
		if itemErr != nil {
			return kindObservationResult{}, itemErr
		}
		resources = append(resources, resource)
	}
	return kindObservationResult{Resources: resources, PageCount: observed.PageCount}, nil
}

func (s *Syncer) buildObservedResource(
	cluster registry.ClusterInfo,
	kind string,
	gvk schema.GroupVersionKind,
	writer ObservationWriter,
	item *observation.ObservedItem,
) (observedResource, error) {
	itemGV, parseErr := schema.ParseGroupVersion(item.Object.GetAPIVersion())
	if parseErr != nil || item.Object.GetKind() != kind || itemGV.Group != gvk.Group || itemGV.Version != gvk.Version {
		return observedResource{}, fmt.Errorf("syncKind: item GVK does not match requested %s, Kind=%s", gvk.GroupVersion().String(), kind)
	}
	namespace := item.Object.GetNamespace()
	if scope.IsClusterScopedKind(kind) && namespace != "" {
		return observedResource{}, fmt.Errorf("syncKind: cluster-scoped kind=%s has namespace=%s", kind, namespace)
	}
	if !scope.IsClusterScopedKind(kind) && namespace == "" {
		return observedResource{}, fmt.Errorf("syncKind: namespaced kind=%s has empty namespace", kind)
	}
	if item.LabelState == scope.LabelStateInvalid {
		return observedResource{}, fmt.Errorf("syncKind: invalid managed label for cluster=%s kind=%s namespace=%s name=%s", cluster.ID, item.Object.GetKind(), namespace, item.Object.GetName())
	}
	key, err := scopecache.BuildKey(cluster.ID, itemGV.Group, itemGV.Version, item.Object.GetKind(), namespace, item.Object.GetName())
	if err != nil {
		return observedResource{}, fmt.Errorf("syncKind: invalid resource metadata for cluster=%s kind=%s namespace=%s name=%s: %w",
			cluster.ID, item.Object.GetKind(), namespace, item.Object.GetName(), err)
	}
	return observedResource{
		key:           key,
		labelState:    item.LabelState,
		apiVersion:    item.Object.GetAPIVersion(),
		labelsPresent: item.LabelsPresent,
		writer:        writer,
		cacheObservation: scopecache.Observation{
			Identity: scopecache.ResourceIdentity{
				ClusterID: cluster.ID,
				Group:     itemGV.Group,
				Version:   itemGV.Version,
				Kind:      item.Object.GetKind(),
				Namespace: namespace,
				Name:      item.Object.GetName(),
			},
			LabelState: item.LabelState,
			TTL:        s.config.KeyTTL,
		},
	}, nil
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
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cleanupCancel()
		s.releaseFence(cleanupCtx)
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	if s.config.WaitForBrokerReady {
		s.waitForBrokerReady(ctx)
	}

	if ctx.Err() != nil {
		s.logger.Info("FMC Writer shutting down (cancelled during readiness probe)")
		return nil
	}

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

func (s *Syncer) ensureFence(ctx context.Context) (scopecache.LeaseSession, error) {
	s.fenceMu.Lock()
	if s.holdingFence {
		session := s.leaseSession
		if err := s.lease.Verify(ctx, session.Fence); err == nil {
			s.fenceMu.Unlock()
			return session, nil
		}
		s.holdingFence = false
	}
	session, err := s.lease.Acquire(ctx)
	if err != nil {
		s.fenceMu.Unlock()
		return scopecache.LeaseSession{}, err
	}
	s.fence = session.Fence
	s.leaseSession = session
	s.holdingFence = true
	renewCtx, cancel := context.WithCancel(ctx)
	s.leaseCancel = cancel
	s.leaseDone = make(chan struct{})
	done := s.leaseDone
	s.fenceMu.Unlock()
	go s.renewFenceLoop(renewCtx, session, done)
	return session, nil
}

func (s *Syncer) renewFenceLoop(ctx context.Context, session scopecache.LeaseSession, done chan struct{}) {
	defer close(done)
	interval := s.config.LeaseRenewInterval
	if interval <= 0 {
		interval = 20 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-session.Loss:
			s.markFenceLost(session.Fence, "lease loss notification")
			return
		case <-ticker.C:
			if err := s.lease.Renew(ctx, session.Fence); err != nil {
				s.logger.Error(err, "FMC writer lease renewal failed")
				s.markFenceLost(session.Fence, err.Error())
				return
			}
		}
	}
}

func (s *Syncer) markFenceLost(fence scopecache.WriterFence, reason string) {
	s.fenceMu.Lock()
	owned := s.holdingFence && s.fence == fence
	if owned {
		s.holdingFence = false
	}
	s.fenceMu.Unlock()
	if owned && s.readiness != nil {
		if err := s.readiness.InvalidateLease(fence, reason); err != nil {
			s.logger.Error(err, "failed to invalidate FMC readiness after lease loss")
		}
	}
}

func (s *Syncer) releaseFence(ctx context.Context) {
	s.fenceMu.Lock()
	if !s.holdingFence {
		s.fenceMu.Unlock()
		return
	}
	session := s.leaseSession
	s.holdingFence = false
	if s.leaseCancel != nil {
		s.leaseCancel()
	}
	done := s.leaseDone
	s.fenceMu.Unlock()
	if done != nil {
		<-done
	}
	if err := s.lease.Release(ctx, session.Fence); err != nil && !errors.Is(err, scopecache.ErrStaleFence) {
		s.logger.Error(err, "failed to release FMC writer lease")
	}
}

const (
	defaultBrokerProbeInitialInterval = 2 * time.Second
	defaultBrokerProbeMaxInterval     = 15 * time.Second
	defaultBrokerProbeTimeout         = 2 * time.Minute
)

func (s *Syncer) brokerProbeInterval() time.Duration {
	if s.config.BrokerProbeInitialInterval > 0 {
		return s.config.BrokerProbeInitialInterval
	}
	return defaultBrokerProbeInitialInterval
}

func (s *Syncer) brokerProbeMax() time.Duration {
	if s.config.BrokerProbeMaxInterval > 0 {
		return s.config.BrokerProbeMaxInterval
	}
	return defaultBrokerProbeMaxInterval
}

func (s *Syncer) brokerProbeDeadline() time.Duration {
	if s.config.BrokerProbeTimeout > 0 {
		return s.config.BrokerProbeTimeout
	}
	return defaultBrokerProbeTimeout
}

// waitForBrokerReady probes the ReaderFactory with exponential backoff until
// at least one registered cluster returns a successful reader, or the timeout
// or context cancellation is reached. When no clusters are registered, it
// returns immediately (nothing to probe).
func (s *Syncer) waitForBrokerReady(ctx context.Context) {
	clusters := s.registry.List()
	if len(clusters) == 0 {
		s.logger.Info("No clusters registered, skipping broker readiness probe")
		return
	}

	probeCluster := clusters[0]
	interval := s.brokerProbeInterval()
	maxInterval := s.brokerProbeMax()
	deadline := time.After(s.brokerProbeDeadline())

	s.logger.Info("Waiting for broker readiness", "cluster", probeCluster.ID)

	for {
		_, err := s.readerFactory.ReaderFor(ctx, probeCluster)
		if err == nil {
			s.logger.Info("Broker ready", "cluster", probeCluster.ID)
			return
		}

		s.logger.V(1).Info("Broker not ready, retrying", "cluster", probeCluster.ID, "error", err, "nextRetry", interval)

		select {
		case <-ctx.Done():
			return
		case <-deadline:
			s.logger.Info("Broker readiness probe timed out, proceeding with periodic sync", "cluster", probeCluster.ID)
			return
		case <-time.After(interval):
			interval *= 2
			if interval > maxInterval {
				interval = maxInterval
			}
		}
	}
}

func (s *Syncer) syncAll(ctx context.Context) {
	clusters := s.registry.List()
	if len(clusters) == 0 {
		if s.statusStore != nil || s.lease != nil {
			s.lifecycleReady.Store(true)
		}
		return
	}
	allSucceeded := true
	for _, cluster := range clusters {
		if err := s.SyncCluster(ctx, cluster); err != nil {
			allSucceeded = false
			s.logger.Error(err, "Sync failed", "cluster", cluster.ID)
		}
	}
	if s.statusStore != nil || s.lease != nil {
		s.lifecycleReady.Store(allSucceeded)
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
