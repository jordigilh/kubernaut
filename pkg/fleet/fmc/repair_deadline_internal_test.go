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

package fmc

// This spec is internal (package fmc) because the contract under test is the
// collection deadline the repair path places on the context: deadlineKey is
// deliberately unexported so no caller outside this package can set it
// (DD-FLEET-017 D4.1a rule 7b: exactly one component may put the deadline
// there, and in production that component is repairPartition).

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/fleet/observation"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// deadlineRecordingReader records, per configured kind, the collection
// deadline each ListObserved call received from the repair path.
type deadlineRecordingReader struct {
	mu        sync.Mutex
	deadlines map[string]time.Time
	present   map[string]bool
}

func newDeadlineRecordingReader() *deadlineRecordingReader {
	return &deadlineRecordingReader{
		deadlines: make(map[string]time.Time),
		present:   make(map[string]bool),
	}
}

func (r *deadlineRecordingReader) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return nil
}

func (r *deadlineRecordingReader) List(_ context.Context, list client.ObjectList, _ ...client.ListOption) error {
	if target, ok := list.(*unstructured.UnstructuredList); ok {
		target.Items = nil
	}
	return nil
}

func (r *deadlineRecordingReader) ListObserved(ctx context.Context, request observation.ListRequest) (observation.ListObservation, error) {
	deadline, ok := ctx.Value(deadlineKey{}).(time.Time)
	r.mu.Lock()
	r.deadlines[request.Kind] = deadline
	r.present[request.Kind] = ok && !deadline.IsZero()
	r.mu.Unlock()
	object := unstructured.Unstructured{Object: map[string]any{
		"apiVersion": request.APIVersion,
		"kind":       request.Kind,
		"metadata":   map[string]any{"name": "fixture"},
	}}
	if !scope.IsClusterScopedKind(request.Kind) {
		object.SetNamespace("payments")
	}
	return observation.ListObservation{
		Items:     []observation.ObservedItem{{Object: object, LabelsPresent: true, LabelState: scope.LabelStateTrue}},
		Complete:  true,
		PageCount: 1,
		ItemCount: 1,
	}, nil
}

func (r *deadlineRecordingReader) deadlineFor(kind string) (time.Time, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.deadlines[kind], r.present[kind]
}

// deadlineWiringStatusStore is the minimal happy-path StatusStore: begin,
// repair and commit all succeed so the sync cycle reaches collection and the
// deadline assertions observe the real repairPartition code path.
type deadlineWiringStatusStore struct{}

func (s *deadlineWiringStatusStore) BeginRepair(_ context.Context, request scopecache.BeginRepairRequest) (scopecache.RepairStartResult, error) {
	return scopecache.RepairStartResult{
		Partition:                request.Partition,
		RegistrationUID:          request.RegistrationUID,
		ExpectedRegistryRevision: request.ExpectedRegistryRevision,
		WriterFence:              request.WriterFence,
		Generation:               1,
		Token:                    scopecache.RepairToken{Value: "deadline-wiring-token"},
	}, nil
}

func (s *deadlineWiringStatusStore) MarkRepairUncertain(context.Context, scopecache.RepairStartResult, scopecache.RepairFailure) error {
	return nil
}

func (s *deadlineWiringStatusStore) AbandonStaging(context.Context, scopecache.PartitionStateKey, string, uint64, scopecache.WriterFence) error {
	return nil
}

func (s *deadlineWiringStatusStore) Repair(_ context.Context, request scopecache.RepairRequest) (scopecache.RepairResult, error) {
	return scopecache.RepairResult{
		Partition:                request.Partition,
		RegistrationUID:          request.RegistrationUID,
		ExpectedRegistryRevision: request.ExpectedRegistryRevision,
		ProvenRegistryRevision:   request.ExpectedRegistryRevision,
		WriterFence:              request.WriterFence,
		ProvenWriterFence:        request.WriterFence,
		ProvenGeneration:         request.Generation,
		Token:                    request.Token,
		ExpectedCount:            len(request.Observations.Items),
		ObservedCount:            len(request.Observations.Items),
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

func (s *deadlineWiringStatusStore) CommitRepair(_ context.Context, request scopecache.RepairCommitRequest) (scopecache.RepairCommitResult, error) {
	return scopecache.RepairCommitResult{
		Partition:                request.Partition,
		RegistrationUID:          request.RegistrationUID,
		ExpectedRegistryRevision: request.ExpectedRegistryRevision,
		ProvenRegistryRevision:   request.ExpectedRegistryRevision,
		WriterFence:              request.WriterFence,
		ProvenWriterFence:        request.WriterFence,
		ProvenGeneration:         request.Generation,
		Token:                    request.Token,
		Committed:                true,
		CleanCommit:              true,
	}, nil
}

func (s *deadlineWiringStatusStore) ReadVerified(context.Context, scopecache.FencedReadRequest) (scopecache.FencedReadResult, error) {
	return scopecache.FencedReadResult{}, nil
}

func (s *deadlineWiringStatusStore) ReadInventoryPageHeader(context.Context, scopecache.InventoryPageRequest) (scopecache.InventoryPageHeader, error) {
	return scopecache.InventoryPageHeader{State: scopecache.InventoryReadMissing}, nil
}

func (s *deadlineWiringStatusStore) ReadInventoryPageRecords(context.Context, scopecache.InventoryPageRecordsRequest) (scopecache.InventoryPageRecords, error) {
	return scopecache.InventoryPageRecords{State: scopecache.InventoryReadMissing}, nil
}

func (s *deadlineWiringStatusStore) ReadInventoryResource(context.Context, scopecache.InventoryResourceRequest) (scopecache.InventoryResourceRecord, error) {
	return scopecache.InventoryResourceRecord{State: scopecache.InventoryReadMissing}, nil
}

// deadlineWiringLease always grants and verifies one writer fence.
type deadlineWiringLease struct {
	fence scopecache.WriterFence
}

func (l *deadlineWiringLease) Acquire(context.Context) (scopecache.LeaseSession, error) {
	return scopecache.LeaseSession{Fence: l.fence, Loss: make(chan struct{})}, nil
}

func (l *deadlineWiringLease) Verify(context.Context, scopecache.WriterFence) error  { return nil }
func (l *deadlineWiringLease) Renew(context.Context, scopecache.WriterFence) error   { return nil }
func (l *deadlineWiringLease) Release(context.Context, scopecache.WriterFence) error { return nil }

// deadlineWiringRegistry is a registration-fenced registry for one cluster.
type deadlineWiringRegistry struct {
	current  registry.ClusterInfo
	revision uint64
	events   chan registry.ClusterEvent
}

func (r *deadlineWiringRegistry) List() []registry.ClusterInfo {
	return []registry.ClusterInfo{r.current}
}

func (r *deadlineWiringRegistry) Get(clusterID string) (registry.ClusterInfo, bool) {
	if clusterID != r.current.ID {
		return registry.ClusterInfo{}, false
	}
	return r.current, true
}

func (r *deadlineWiringRegistry) WatchClusters() <-chan registry.ClusterEvent { return r.events }
func (r *deadlineWiringRegistry) Ready() bool                                 { return true }
func (r *deadlineWiringRegistry) Start(context.Context) error                 { return nil }
func (r *deadlineWiringRegistry) Stop()                                       {}

func (r *deadlineWiringRegistry) SnapshotAll() registry.RegistrySnapshot {
	return registry.RegistrySnapshot{
		Generation:    r.revision,
		Registrations: []registry.RegistrationSnapshot{{Info: r.current, Revision: r.revision}},
	}
}

func (r *deadlineWiringRegistry) Snapshot(clusterID string) (registry.RegistrationSnapshot, bool) {
	if clusterID != r.current.ID {
		return registry.RegistrationSnapshot{}, false
	}
	return registry.RegistrationSnapshot{Info: r.current, Revision: r.revision}, true
}

func (r *deadlineWiringRegistry) WithCurrent(_ context.Context, clusterID, registrationUID string, expectedRevision uint64, callback func(registry.ClusterInfo) error) error {
	if clusterID != r.current.ID || registrationUID != r.current.RegistrationUID || expectedRevision != r.revision {
		return fmt.Errorf("%w: cluster %q registration changed", registry.ErrRegistrationChanged, clusterID)
	}
	return callback(r.current)
}

// newDeadlineWiringSyncer assembles a lifecycle-enabled syncer whose only
// real production code under test is repairPartition and its collection
// deadline wiring; everything else is a happy-path stand-in.
func newDeadlineWiringSyncer(reader *deadlineRecordingReader) *Syncer {
	cluster := registry.ClusterInfo{ID: "cluster-a", RegistrationUID: "registration-a"}
	reg := &deadlineWiringRegistry{
		current:  cluster,
		revision: 7,
		events:   make(chan registry.ClusterEvent, 1),
	}
	readiness, err := scopecache.NewFMCReadinessSnapshot(scopecache.LeasePolicyProcessWideRequired)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	syncer := NewSyncer(Dependencies{
		Registry:    reg,
		Readers:     ObservedReaderFactoryFunc(func(_ context.Context, _ registry.ClusterInfo) (ObservedReader, error) { return reader, nil }),
		Writer:      &deadlineWiringWriter{},
		StatusStore: &deadlineWiringStatusStore{},
		Lease: &deadlineWiringLease{
			fence: scopecache.WriterFence{HolderPodUID: "pod-a", HolderSessionUID: "session-a", LeaseEpoch: 1, Fence: 1},
		},
		Readiness: readiness,
	}, Config{KeyTTL: time.Minute, ResourceKinds: []string{"Pod", "Namespace"}}, logr.Discard(), NewMetrics(prometheus.NewRegistry()))
	return syncer
}

// deadlineWiringWriter satisfies CacheWriter; the lifecycle lane never calls it.
type deadlineWiringWriter struct{}

func (w *deadlineWiringWriter) Set(context.Context, string, time.Duration) error { return nil }
func (w *deadlineWiringWriter) Close() error                                     { return nil }

var _ = Describe("FMC repair collection deadline ownership (DD-FLEET-017 D4.1a rule 7b)", func() {
	It("UT-FMC-SRC-2661-024 [BR-SAFETY-001]: the repair path sets one deadline, shared by every kind of the repair", func() {
		reader := newDeadlineRecordingReader()
		syncer := newDeadlineWiringSyncer(reader)
		cluster := registry.ClusterInfo{ID: "cluster-a", RegistrationUID: "registration-a"}

		started := time.Now()
		Expect(syncer.SyncCluster(context.Background(), cluster)).To(Succeed())
		finished := time.Now()

		podDeadline, podSet := reader.deadlineFor("Pod")
		namespaceDeadline, namespaceSet := reader.deadlineFor("Namespace")
		ExpectWithOffset(1, podSet).To(BeTrue(),
			"the repair path must bound every kind's collection; an unset deadline means each kind derives its own fallback and a multi-kind sync can outlive the staged generation")
		ExpectWithOffset(1, namespaceSet).To(BeTrue(),
			"the second kind must run under the repair's deadline, not a fresh one")

		ExpectWithOffset(1, podDeadline.Equal(namespaceDeadline)).To(BeTrue(),
			"exactly one deadline per repair: both kinds must have seen the identical value")
		ExpectWithOffset(1, podDeadline.Sub(started)).To(BeNumerically(">=", RepairCollectionBudget),
			"the deadline is the repair start plus the collection budget")
		ExpectWithOffset(1, podDeadline.Sub(finished)).To(BeNumerically("<=", RepairCollectionBudget),
			"the deadline is set once, at repair start, not per kind")
	})

	It("UT-FMC-SRC-2661-024 [BR-SAFETY-001]: the repair path never overwrites a deadline the caller already set", func() {
		reader := newDeadlineRecordingReader()
		syncer := newDeadlineWiringSyncer(reader)
		cluster := registry.ClusterInfo{ID: "cluster-a", RegistrationUID: "registration-a"}

		callerDeadline := time.Now().Add(20 * time.Second)
		ctx := WithCollectionDeadline(context.Background(), callerDeadline)
		Expect(syncer.SyncCluster(ctx, cluster)).To(Succeed())

		for _, kind := range []string{"Pod", "Namespace"} {
			deadline, set := reader.deadlineFor(kind)
			ExpectWithOffset(1, set).To(BeTrue())
			ExpectWithOffset(1, deadline.Equal(callerDeadline)).To(BeTrue(),
				"rule 7b: exactly one component may set the deadline; the repair path must defer to one already present")
		}
	})
})
