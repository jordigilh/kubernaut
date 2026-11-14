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

package fmc_test

import (
	"context"
	"errors"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/observation"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
)

type fencedRegistry struct {
	current  registry.ClusterInfo
	revision uint64
	events   chan registry.ClusterEvent
}

func newFencedRegistry(current registry.ClusterInfo, revision uint64) *fencedRegistry {
	return &fencedRegistry{current: current, revision: revision, events: make(chan registry.ClusterEvent, 1)}
}

func (r *fencedRegistry) List() []registry.ClusterInfo { return []registry.ClusterInfo{r.current} }

func (r *fencedRegistry) Get(clusterID string) (registry.ClusterInfo, bool) {
	if clusterID != r.current.ID {
		return registry.ClusterInfo{}, false
	}
	return r.current, true
}

func (r *fencedRegistry) WatchClusters() <-chan registry.ClusterEvent { return r.events }

func (r *fencedRegistry) Ready() bool { return true }

func (r *fencedRegistry) Start(context.Context) error { return nil }

func (r *fencedRegistry) Stop() {}

func (r *fencedRegistry) Snapshot(clusterID string) (registry.RegistrationSnapshot, bool) {
	if clusterID != r.current.ID {
		return registry.RegistrationSnapshot{}, false
	}
	return registry.RegistrationSnapshot{Info: r.current, Revision: r.revision}, true
}

func (r *fencedRegistry) WithCurrent(_ context.Context, clusterID, registrationUID string, expectedRevision uint64, callback func(registry.ClusterInfo) error) error {
	if clusterID != r.current.ID || registrationUID != r.current.RegistrationUID || expectedRevision != r.revision {
		return context.Canceled
	}
	return callback(r.current)
}

type lifecycleReader struct {
	pageCounts map[string]int
	items      []unstructured.Unstructured
	listDelay  time.Duration
	listErr    error
}

func (r *lifecycleReader) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return nil
}

func (r *lifecycleReader) List(_ context.Context, list client.ObjectList, _ ...client.ListOption) error {
	if target, ok := list.(*unstructured.UnstructuredList); ok {
		target.Items = r.items
	}
	return nil
}

func (r *lifecycleReader) ListObserved(_ context.Context, request observation.ListRequest) (observation.ListObservation, error) {
	if r.listDelay > 0 {
		time.Sleep(r.listDelay)
	}
	if r.listErr != nil {
		return observation.ListObservation{Failure: observation.FailureTransport}, r.listErr
	}
	pageCount := r.pageCounts[request.Kind]
	if pageCount == 0 {
		pageCount = 1
	}
	items := make([]observation.ObservedItem, 0, len(r.items))
	for _, item := range r.items {
		items = append(items, observation.ObservedItem{Object: item})
	}
	return observation.ListObservation{
		Items:     items,
		Complete:  true,
		PageCount: pageCount,
		ItemCount: len(items),
	}, nil
}

type lifecycleStatusStore struct {
	repairRequest *scopecache.RepairRequest
	uncertainErr  error
}

func (s *lifecycleStatusStore) BeginRepair(_ context.Context, request scopecache.BeginRepairRequest) (scopecache.RepairStartResult, error) {
	return scopecache.RepairStartResult{
		Partition:                request.Partition,
		RegistrationUID:          request.RegistrationUID,
		ExpectedRegistryRevision: request.ExpectedRegistryRevision,
		WriterFence:              request.WriterFence,
		Generation:               1,
		Token:                    scopecache.RepairToken{Value: "repair-token"},
	}, nil
}

func (s *lifecycleStatusStore) MarkRepairUncertain(ctx context.Context, _ scopecache.RepairStartResult, _ scopecache.RepairFailure) error {
	s.uncertainErr = ctx.Err()
	return nil
}

func (s *lifecycleStatusStore) Repair(_ context.Context, request scopecache.RepairRequest) (scopecache.RepairResult, error) {
	s.repairRequest = &request
	count := len(request.Observations.Items)
	return scopecache.RepairResult{
		Partition:                request.Partition,
		RegistrationUID:          request.RegistrationUID,
		ExpectedRegistryRevision: request.ExpectedRegistryRevision,
		ProvenRegistryRevision:   request.ExpectedRegistryRevision,
		WriterFence:              request.WriterFence,
		ProvenWriterFence:        request.WriterFence,
		ProvenGeneration:         request.Generation,
		Token:                    request.Token,
		ExpectedCount:            count,
		ObservedCount:            count,
		SourceComplete:           request.Observations.Complete,
		RecordValid:              true,
		IndexValid:               true,
		StatusValid:              true,
		RecordIndexChecksValid:   true,
		RecordTTL:                scopecache.TTLPTTLValidation{Valid: true},
		IndexTTL:                 scopecache.TTLPTTLValidation{Valid: true},
		StatusTTL:                scopecache.TTLPTTLValidation{Valid: true},
		CleanCommitEligible:      true,
	}, nil
}

func (s *lifecycleStatusStore) CommitRepair(_ context.Context, request scopecache.RepairCommitRequest) (scopecache.RepairCommitResult, error) {
	return scopecache.RepairCommitResult{
		Partition:                request.Partition,
		RegistrationUID:          request.RegistrationUID,
		ExpectedRegistryRevision: request.ExpectedRegistryRevision,
		ProvenRegistryRevision:   request.ExpectedRegistryRevision,
		WriterFence:              request.WriterFence,
		ProvenGeneration:         request.Generation,
		ProvenWriterFence:        request.WriterFence,
		Token:                    request.Token,
		Committed:                true,
		CleanCommit:              true,
	}, nil
}

func (s *lifecycleStatusStore) ReadVerified(context.Context, scopecache.FencedReadRequest) (scopecache.FencedReadResult, error) {
	return scopecache.FencedReadResult{}, nil
}

type lifecycleLease struct {
	fence scopecache.WriterFence
	lost  chan struct{}
}

func (l *lifecycleLease) Acquire(context.Context) (scopecache.LeaseSession, error) {
	return scopecache.LeaseSession{Fence: l.fence, Loss: l.lost}, nil
}

func (l *lifecycleLease) Verify(context.Context, scopecache.WriterFence) error { return nil }

func (l *lifecycleLease) Renew(context.Context, scopecache.WriterFence) error { return nil }

func (l *lifecycleLease) Release(context.Context, scopecache.WriterFence) error { return nil }

func newLifecycleSyncer(registryClient *fencedRegistry, readerFactory fmc.ObservedReaderFactory, statusStore *lifecycleStatusStore, lease *lifecycleLease, resourceKinds []string) *fmc.Syncer {
	readiness, err := scopecache.NewFMCReadinessSnapshot(scopecache.LeasePolicyProcessWideRequired)
	Expect(err).NotTo(HaveOccurred())
	metrics := fmc.NewMetrics(prometheus.NewPedanticRegistry())
	return fmc.NewSyncer(fmc.Dependencies{
		Registry:    registryClient,
		Readers:     readerFactory,
		Writer:      &stubWriter{},
		StatusStore: statusStore,
		Lease:       lease,
		Readiness:   readiness,
	}, fmc.Config{KeyTTL: time.Minute, ResourceKinds: resourceKinds}, logr.Discard(), metrics)
}

var _ = Describe("FMC lifecycle handoff (BR-SCOPE-001, BR-INTEGRATION-065)", func() {
	It("UT-FMC-PRES-009 [BR-SCOPE-001]: collects with the registration configuration captured in the repair snapshot", func() {
		current := registry.ClusterInfo{ID: "cluster-a", RegistrationUID: "registration-a", ToolPrefix: "current__"}
		stale := current
		stale.ToolPrefix = "stale__"
		registryClient := newFencedRegistry(current, 7)
		statusStore := &lifecycleStatusStore{}
		lease := &lifecycleLease{
			fence: scopecache.WriterFence{HolderPodUID: "pod-a", HolderSessionUID: "session-a", LeaseEpoch: 1, Fence: 1},
			lost:  make(chan struct{}),
		}
		reader := &lifecycleReader{pageCounts: map[string]int{"Pod": 1}}
		var received registry.ClusterInfo
		syncer := newLifecycleSyncer(registryClient, fmc.ObservedReaderFactoryFunc(func(_ context.Context, cluster registry.ClusterInfo) (fmc.ObservedReader, error) {
			received = cluster
			return reader, nil
		}), statusStore, lease, []string{"Pod"})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		Expect(syncer.SyncCluster(ctx, stale)).To(Succeed())
		Expect(received.ToolPrefix).To(Equal("current__"))
	})

	It("UT-FMC-PRES-010 [BR-SCOPE-001]: bounds repair pages using the sum of source page counts", func() {
		current := registry.ClusterInfo{ID: "cluster-a", RegistrationUID: "registration-a", ToolPrefix: "current__"}
		registryClient := newFencedRegistry(current, 7)
		statusStore := &lifecycleStatusStore{}
		lease := &lifecycleLease{
			fence: scopecache.WriterFence{HolderPodUID: "pod-a", HolderSessionUID: "session-a", LeaseEpoch: 1, Fence: 1},
			lost:  make(chan struct{}),
		}
		reader := &lifecycleReader{pageCounts: map[string]int{"Pod": 2, "Service": 3}}
		syncer := newLifecycleSyncer(registryClient, fmc.ObservedReaderFactoryFunc(func(_ context.Context, _ registry.ClusterInfo) (fmc.ObservedReader, error) {
			return reader, nil
		}), statusStore, lease, []string{"Pod", "Service"})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		Expect(syncer.SyncCluster(ctx, current)).To(Succeed())
		Expect(statusStore.repairRequest).NotTo(BeNil())
		Expect(statusStore.repairRequest.Observations.PageCount).To(Equal(5))
	})

	It("UT-FMC-PRES-011 [BR-SCOPE-001]: starts the failed-repair cleanup deadline when cleanup begins", func() {
		current := registry.ClusterInfo{ID: "cluster-a", RegistrationUID: "registration-a", ToolPrefix: "current__"}
		registryClient := newFencedRegistry(current, 7)
		statusStore := &lifecycleStatusStore{}
		lease := &lifecycleLease{
			fence: scopecache.WriterFence{HolderPodUID: "pod-a", HolderSessionUID: "session-a", LeaseEpoch: 1, Fence: 1},
			lost:  make(chan struct{}),
		}
		reader := &lifecycleReader{
			pageCounts: map[string]int{"Pod": 1},
			listDelay:  5*time.Second + 50*time.Millisecond,
			listErr:    errors.New("source unavailable"),
		}
		syncer := newLifecycleSyncer(registryClient, fmc.ObservedReaderFactoryFunc(func(_ context.Context, _ registry.ClusterInfo) (fmc.ObservedReader, error) {
			return reader, nil
		}), statusStore, lease, []string{"Pod"})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		Expect(syncer.SyncCluster(ctx, current)).To(HaveOccurred())
		Expect(statusStore.uncertainErr).ToNot(HaveOccurred())
	})
})
