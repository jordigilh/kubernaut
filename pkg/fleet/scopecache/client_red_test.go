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

package scopecache_test

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

type noOpStatusStore struct{}

func (noOpStatusStore) BeginRepair(context.Context, scopecache.BeginRepairRequest) (scopecache.RepairStartResult, error) {
	return scopecache.RepairStartResult{}, nil
}

func (noOpStatusStore) MarkRepairUncertain(context.Context, scopecache.RepairStartResult, scopecache.RepairFailure) error {
	return nil
}

func (noOpStatusStore) AbandonStaging(context.Context, scopecache.PartitionStateKey, string, uint64, scopecache.WriterFence) error {
	return nil
}

func (noOpStatusStore) Repair(context.Context, scopecache.RepairRequest) (scopecache.RepairResult, error) {
	return scopecache.RepairResult{}, nil
}

func (noOpStatusStore) CommitRepair(context.Context, scopecache.RepairCommitRequest) (scopecache.RepairCommitResult, error) {
	return scopecache.RepairCommitResult{}, nil
}

func (noOpStatusStore) ReadVerified(context.Context, scopecache.FencedReadRequest) (scopecache.FencedReadResult, error) {
	return scopecache.FencedReadResult{}, nil
}

// staticRegistrationGuard serves one immutable registration snapshot. The
// optional onCurrent hook lets a test rotate readiness exactly when the
// client's guarded re-read runs.
type staticRegistrationGuard struct {
	snapshot  registry.RegistrationSnapshot
	found     bool
	onCurrent func()
}

func (g *staticRegistrationGuard) Snapshot(string) (registry.RegistrationSnapshot, bool) {
	return g.snapshot, g.found
}

func (g *staticRegistrationGuard) WithCurrent(ctx context.Context, _ string, _ string, _ uint64, callback func(registry.ClusterInfo) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if g.onCurrent != nil {
		g.onCurrent()
	}
	return callback(g.snapshot.Info)
}

// cleanReadiness publishes one complete clean partition proof, the state a
// healthy FMC writer leaves behind after a commit.
func cleanReadiness(partition scopecache.PartitionStateKey, info registry.ClusterInfo, revision uint64, fence scopecache.WriterFence, generation uint64) *scopecache.FMCReadinessSnapshot {
	readiness, err := scopecache.NewFMCReadinessSnapshot(scopecache.LeasePolicyProcessWideRequired)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	ExpectWithOffset(1, readiness.ReconcileRegistry(registry.RegistrySnapshot{
		Generation:    revision,
		Registrations: []registry.RegistrationSnapshot{{Info: info, Revision: revision}},
	})).To(Succeed())
	ExpectWithOffset(1, readiness.PublishGlobal(scopecache.FMCGlobalReadiness{
		MCPInitialSync:       true,
		RegistryInitialSync:  true,
		RegistryWatchHealthy: true,
		ScriptReady:          true,
		ValkeyReady:          true,
		LeasePolicy:          scopecache.LeasePolicyProcessWideRequired,
	})).To(Succeed())
	lease := scopecache.LeaseView{
		Policy:         scopecache.LeasePolicyProcessWideRequired,
		Active:         true,
		Valid:          true,
		Epoch:          fence.LeaseEpoch,
		SessionUID:     fence.HolderSessionUID,
		WriterFence:    fence,
		HeartbeatAt:    time.Now().UTC(),
		HeartbeatValid: true,
	}
	ExpectWithOffset(1, readiness.PublishWriterLease(scopecache.WriterFence{}, lease)).To(Succeed())
	ExpectWithOffset(1, readiness.BeginPartitionRepair(partition, info.RegistrationUID, revision, lease, generation)).To(Succeed())
	ExpectWithOffset(1, readiness.PublishPartitionClean(partition, scopecache.PartitionReadiness{
		Key:                  partition,
		RegistrationUID:      info.RegistrationUID,
		RegistrationRevision: revision,
		LeaseEpoch:           fence.LeaseEpoch,
		LeaseSessionUID:      fence.HolderSessionUID,
		WriterFence:          fence,
		LeaseHeartbeatAt:     time.Now().UTC(),
		LeaseHeartbeatValid:  true,
		Generation:           generation,
		State:                scopecache.PartitionClean,
		CleanCommit:          true,
		RecordValid:          true,
		IndexValid:           true,
		StatusValid:          true,
		RecordTTLValid:       true,
		IndexTTLValid:        true,
		StatusTTLValid:       true,
	})).To(Succeed())
	return readiness
}

var _ = Describe("Status-store scope cache client (BR-SCOPE-001)", func() {
	DescribeTable("UT-FMC-DEFERRED-001 [BR-SCOPE-001]: cluster-scoped target checks are unsupported", func(kind, namespace string) {
		// A verified-present managed observation must not authorize a deferred target.
		reader := &mockCacheReader{store: make(map[string]bool)}
		key, err := scopecache.BuildKey("cluster", "", "v1", kind, namespace, "target")
		Expect(err).ToNot(HaveOccurred())
		reader.store[key] = true
		result, err := scopecache.NewClient(reader).Check(context.Background(), scope.ResourceIdentity{
			ClusterID: "cluster", Version: "v1", Kind: kind, Namespace: namespace, Name: "target",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.State).To(Equal(scope.ScopeUnsupported))
		Expect(result.Provenance).To(Equal(scope.ScopeSourceUnavailable))
	},
		Entry("Node", "Node", ""),
		Entry("PersistentVolume", "PersistentVolume", ""),
		Entry("Namespace as target, not internal observation", "Namespace", ""),
		Entry("unknown cluster-scoped kind", "CustomClusterResource", ""),
		Entry("known cluster-scoped kind cannot gain scope from a supplied namespace", "Node", "default"),
	)
	It("UT-FMC-DEFERRED-002 [BR-SCOPE-001, #2660 m5]: an invalid readiness proof is not flattened to partition_unavailable", func() {
		info := registry.ClusterInfo{ID: "cluster-a", RegistrationUID: "registration-a", ToolPrefix: "cluster-a__"}
		partition := scopecache.PartitionStateKey{ClusterID: info.ID, RegistrationUID: info.RegistrationUID}
		fence := scopecache.WriterFence{HolderPodUID: "pod-a", HolderSessionUID: "session-a", LeaseEpoch: 1, Fence: 1}

		// Site 1: the registration guard exposes a zero revision, which
		// PartitionReady classifies as ReadinessInvalid (identity defect).
		readiness := cleanReadiness(partition, info, 1, fence, 1)
		guard := &staticRegistrationGuard{snapshot: registry.RegistrationSnapshot{Info: info, Revision: 0}, found: true}
		client, err := scopecache.NewClientWithStatusStore(noOpStatusStore{}, guard, readiness)
		Expect(err).ToNot(HaveOccurred())

		result, checkErr := client.Check(context.Background(), scope.ResourceIdentity{
			ClusterID: info.ID, Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "default", Name: "web",
		})
		Expect(checkErr).To(HaveOccurred(), "an invalid readiness proof must keep its error")
		Expect(result.State).To(Equal(scope.ScopeUnavailable))
		Expect(result.Reason).To(Equal("partition_invalid"))

		// Site 2: the guarded re-read rotates the readiness view, so the
		// callback's proof check is Unavailable (not Invalid) and must stay a
		// nil-error unavailable result with an assigned result value.
		readiness = cleanReadiness(partition, info, 1, fence, 1)
		guard = &staticRegistrationGuard{
			snapshot:  registry.RegistrationSnapshot{Info: info, Revision: 1},
			found:     true,
			onCurrent: func() { Expect(readiness.InvalidatePartition(partition, "rotated")).To(Succeed()) },
		}
		client, err = scopecache.NewClientWithStatusStore(noOpStatusStore{}, guard, readiness)
		Expect(err).ToNot(HaveOccurred())

		result, checkErr = client.Check(context.Background(), scope.ResourceIdentity{
			ClusterID: info.ID, Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "default", Name: "web",
		})
		Expect(checkErr).ToNot(HaveOccurred(), "an unavailable partition is an expected operational state")
		Expect(result.State).To(Equal(scope.ScopeUnavailable))
		Expect(result.Reason).To(Equal("partition_unavailable"))
	})

	It("UT-FMC-CACHE-015 [BR-SCOPE-001]: unknown registration returns unavailable without a reader", func() {
		readiness, err := scopecache.NewFMCReadinessSnapshot(scopecache.LeasePolicyProcessWideRequired)
		Expect(err).ToNot(HaveOccurred())
		registryClient := registry.NewEAIGWRegistry(nil, registry.EAIGWRegistryConfig{}, nil, logr.Discard())
		DeferCleanup(registryClient.Stop)
		client, err := scopecache.NewClientWithStatusStore(noOpStatusStore{}, registryClient, readiness)
		Expect(err).ToNot(HaveOccurred())

		var result scope.ScopeResult
		Expect(func() {
			result, err = client.Check(context.Background(), scope.ResourceIdentity{ClusterID: "cluster-a", Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "default", Name: "web"})
		}).ToNot(Panic())
		Expect(err).NotTo(HaveOccurred())
		Expect(result.State).To(Equal(scope.ScopeUnavailable))
	})
})
