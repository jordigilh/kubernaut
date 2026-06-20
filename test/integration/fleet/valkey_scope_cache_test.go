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

package fleet

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmcwriter"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
)

// IT-FLEET-VALKEY-001 through IT-FLEET-VALKEY-005
//
// Business Outcome: The Fleet Metadata Cache (Valkey) provides low-latency
// scope checking for remote clusters. These tests prove the production
// ValkeyWriter and ValkeyCacheReader correctly write and read keys from
// a real Redis/Valkey instance, validating the end-to-end data path
// that FMC Writer → Valkey → FederatedScopeChecker depends on.
//
// Wiring Manifest:
//   ValkeyWriter     → cmd/fmcwriter/main.go
//   ValkeyCacheReader → pkg/gateway/server.go (via FederatedScopeChecker)
//   FederatedScopeChecker → pkg/fleet/scopecache/federated_checker.go
var _ = Describe("Fleet Scope Cache Valkey Integration (BR-INTEGRATION-065)", Ordered, Label("fleet", "valkey", "integration"), func() {
	var (
		ctx    context.Context
		writer *fmcwriter.ValkeyWriter
		reader *scopecache.ValkeyCacheReader
		client *scopecache.Client
		fc     *scopecache.FederatedScopeChecker
	)

	BeforeAll(func() {
		ctx = context.Background()
		writer = fmcwriter.NewValkeyWriter(valkeyAddr)
		reader = scopecache.NewValkeyCacheReader(valkeyAddr)
		client = scopecache.NewClient(reader)

		local := &localAlwaysFalse{}
		fc = scopecache.NewFederatedScopeChecker(local, client, logr.Discard())
	})

	AfterAll(func() {
		if writer != nil {
			_ = writer.Close()
		}
		if reader != nil {
			_ = reader.Close()
		}
	})

	It("IT-FLEET-VALKEY-001: ValkeyWriter.Set writes key readable by ValkeyCacheReader", func() {
		key, err := scopecache.BuildKey("prod-east", "apps", "v1", "Deployment", "default", "nginx")
		Expect(err).ToNot(HaveOccurred())

		Expect(writer.Set(ctx, key, 30*time.Second)).To(Succeed())

		exists, err := reader.Exists(ctx, key)
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeTrue(),
			"IT-FLEET-VALKEY-001: Key written by ValkeyWriter must be readable by ValkeyCacheReader")
	})

	It("IT-FLEET-VALKEY-002: ValkeyCacheReader.Exists returns false for missing key", func() {
		key, err := scopecache.BuildKey("nonexistent-cluster", "apps", "v1", "Deployment", "ns", "missing")
		Expect(err).ToNot(HaveOccurred())

		exists, err := reader.Exists(ctx, key)
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeFalse(),
			"IT-FLEET-VALKEY-002: Non-existent key must return false")
	})

	It("IT-FLEET-VALKEY-003: TTL expiry removes key from cache", func() {
		key, err := scopecache.BuildKey("staging-west", "", "v1", "Pod", "jobs", "worker")
		Expect(err).ToNot(HaveOccurred())

		Expect(writer.Set(ctx, key, 1*time.Second)).To(Succeed())

		exists, err := reader.Exists(ctx, key)
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeTrue(), "Key must exist immediately after write")

		time.Sleep(1500 * time.Millisecond)

		exists, err = reader.Exists(ctx, key)
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeFalse(),
			"IT-FLEET-VALKEY-003: Key must expire after TTL elapses")
	})

	It("IT-FLEET-VALKEY-004: FederatedScopeChecker reads through to Valkey for remote cluster", func() {
		key, err := scopecache.BuildKey("prod-east", "", "", "StatefulSet", "data", "redis-master")
		Expect(err).ToNot(HaveOccurred())
		Expect(writer.Set(ctx, key, 30*time.Second)).To(Succeed())

		managed, err := fc.IsManagedOnCluster(ctx, "prod-east", "data", "StatefulSet", "redis-master")
		Expect(err).ToNot(HaveOccurred())
		Expect(managed).To(BeTrue(),
			"IT-FLEET-VALKEY-004: FederatedScopeChecker must find managed resource via Valkey")
	})

	It("IT-FLEET-VALKEY-005: FederatedScopeChecker returns false for unmanaged remote resource", func() {
		managed, err := fc.IsManagedOnCluster(ctx, "prod-east", "orphan-ns", "Deployment", "no-such-deploy")
		Expect(err).ToNot(HaveOccurred())
		Expect(managed).To(BeFalse(),
			"IT-FLEET-VALKEY-005: Unmanaged resource must return false from FederatedScopeChecker")
	})
})

// localAlwaysFalse is a mock local scope checker that always returns false.
// In these tests we only validate the remote (Valkey) path.
type localAlwaysFalse struct{}

func (l *localAlwaysFalse) IsManaged(_ context.Context, _, _, _ string) (bool, error) {
	return false, nil
}
