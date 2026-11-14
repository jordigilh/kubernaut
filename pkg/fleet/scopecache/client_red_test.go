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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
)

type noOpStatusStore struct{}

func (noOpStatusStore) BeginRepair(context.Context, scopecache.BeginRepairRequest) (scopecache.RepairStartResult, error) {
	return scopecache.RepairStartResult{}, nil
}

func (noOpStatusStore) MarkRepairUncertain(context.Context, scopecache.RepairStartResult, scopecache.RepairFailure) error {
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

var _ = Describe("Status-store scope cache client (BR-SCOPE-001)", func() {
	It("UT-FMC-CACHE-015 [BR-SCOPE-001]: does not panic when legacy IsManaged is called without a reader", func() {
		readiness, err := scopecache.NewFMCReadinessSnapshot(scopecache.LeasePolicyProcessWideRequired)
		Expect(err).ToNot(HaveOccurred())
		registryClient := registry.NewEAIGWRegistry(nil, registry.EAIGWRegistryConfig{}, nil, logr.Discard())
		DeferCleanup(registryClient.Stop)
		client, err := scopecache.NewClientWithStatusStore(noOpStatusStore{}, registryClient, readiness)
		Expect(err).ToNot(HaveOccurred())

		var callErr error
		Expect(func() {
			_, callErr = client.IsManaged(context.Background(), "cluster-a", "apps", "v1", "Deployment", "default", "web")
		}).ToNot(Panic())
		Expect(callErr).To(MatchError(ContainSubstring("unsupported")))
	})
})
