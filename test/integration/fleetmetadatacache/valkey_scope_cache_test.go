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

package fleetmetadatacache_test

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// IT-FLEET-VALKEY-001 through IT-FLEET-VALKEY-007
//
// Business Outcome: The Fleet Metadata Cache (Valkey) provides low-latency
// scope checking for remote clusters. These tests prove the production
// ValkeyWriter and ValkeyCacheReader correctly write and read keys from
// a real Redis/Valkey instance, validating the end-to-end data path
// that FMC Writer -> Valkey -> FederatedScopeChecker depends on, including
// the resource/namespace 2-level hierarchy (ADR-053, BR-SCOPE-001) that
// enforces AC-4 (NIST 800-53, information flow control): GW/RO/AF may only
// act on resources FMC reports as managed, whether that status comes from
// the resource's own label or an inherited namespace label (Issue #2311).
//
// Wiring Manifest:
//
//	ValkeyWriter        -> cmd/fleetmetadatacache/main.go
//	ValkeyCacheReader   -> pkg/fleet/scope_factory.go (transitional, will be FMC HTTP client)
//	FederatedScopeChecker -> pkg/fleet/federated_checker.go
var _ = Describe("Fleet Scope Cache Valkey Integration (BR-INTEGRATION-065)", Ordered, Label("fmc", "valkey", "integration"), func() {
	var (
		ctx    context.Context
		writer *fmc.ValkeyWriter
		reader *scopecache.ValkeyCacheReader
		client *scopecache.Client
		fc     *fleet.FederatedScopeChecker
	)

	BeforeAll(func() {
		ctx = context.Background()
		writer = fmc.NewValkeyWriter(valkeyAddr)
		reader = scopecache.NewValkeyCacheReader(valkeyAddr)
		client = scopecache.NewClient(reader)

		local := &localAlwaysFalse{}
		fc = fleet.NewFederatedScopeChecker(local, client, logr.Discard())
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
		// Issue #54 SOC2 gap RCA: pkg/fleet/fmc/syncer.go always writes cache
		// keys using the resource's real GVK read from the K8s API ("apps/v1"
		// for StatefulSet), never an empty group/version. This fixture must
		// match that production write path -- scope.ResourceIdentity below
		// intentionally leaves Group/Version empty (as real callers such as
		// Gateway's validateScope do) and relies on
		// scopecache.Client.IsManagedResource inferring "apps/v1" from Kind
		// (see pkg/shared/scope.InferGVK), exactly as scope.Manager already
		// does for local checks.
		key, err := scopecache.BuildKey("prod-east", "apps", "v1", "StatefulSet", "data", "redis-master")
		Expect(err).ToNot(HaveOccurred())
		Expect(writer.Set(ctx, key, 30*time.Second)).To(Succeed())

		managed, err := fc.IsManagedResource(ctx, scope.ResourceIdentity{
			ClusterID: "prod-east",
			Kind:      "StatefulSet",
			Namespace: "data",
			Name:      "redis-master",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(managed).To(BeTrue(),
			"IT-FLEET-VALKEY-004: FederatedScopeChecker must find managed resource via Valkey")
	})

	It("IT-FLEET-VALKEY-005: FederatedScopeChecker returns false for unmanaged remote resource", func() {
		managed, err := fc.IsManagedResource(ctx, scope.ResourceIdentity{
			ClusterID: "prod-east",
			Kind:      "Deployment",
			Namespace: "orphan-ns",
			Name:      "no-such-deploy",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(managed).To(BeFalse(),
			"IT-FLEET-VALKEY-005: Unmanaged resource must return false from FederatedScopeChecker")
	})

	// Issue #2311: pkg/shared/scope.Manager's 2-level hierarchy (ADR-053,
	// BR-SCOPE-001) -- resource label, then namespace label, then unmanaged --
	// was only ever proven for the LOCAL path (pkg/shared/scope/manager_test.go
	// UT-SCOPE-001-004 through 013). The FLEET path exercised here had zero
	// equivalent coverage: every existing FMC E2E fixture
	// (test/e2e/fleetmetadatacache/shared/*) labels the resource itself
	// directly, never "namespace labeled, resource unlabeled" -- so the fleet
	// scope chain silently regressed to a *stricter*, undocumented 1-level
	// variant (resource-only) without any test noticing. This IT closes that
	// gap by driving the real production wiring end to end -- FMC's
	// ValkeyWriter, a real Valkey instance, and FederatedScopeChecker's
	// AC-4 (NIST 800-53, information flow control: GW/RO/AF may only act on
	// resources FMC reports as managed) enforcement path -- not the
	// in-memory mocks used by pkg/fleet/scopecache and pkg/fleet/fmc's unit
	// tests.
	It("IT-FLEET-VALKEY-006 [Issue #2311, AC-4]: FederatedScopeChecker honors a namespace-level managed label for an unlabeled remote resource", func() {
		nsKey, err := scopecache.BuildKey("prod-west", "", "v1", "Namespace", "", "demo-checkout")
		Expect(err).ToNot(HaveOccurred())
		Expect(writer.Set(ctx, nsKey, 30*time.Second)).To(Succeed())
		// Deliberately do NOT write a resource-level key -- the pod itself
		// carries no label in this scenario, only its namespace does.

		managed, err := fc.IsManagedResource(ctx, scope.ResourceIdentity{
			ClusterID: "prod-west",
			Kind:      "Pod",
			Namespace: "demo-checkout",
			Name:      "worker-5f74bb54f7-28jpv",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(managed).To(BeTrue(),
			"IT-FLEET-VALKEY-006: a namespace-level kubernaut.ai/managed=true label must be honored for fleet resources through the real Valkey-backed chain, same as local (ADR-053 parity)")
	})

	It("IT-FLEET-VALKEY-007 [Issue #2311, AC-4]: FederatedScopeChecker does not fall back to namespace for cluster-scoped kinds", func() {
		// Mirrors UT-SCOPE-001-013's cluster-scoped guarantee for the fleet
		// path: a Node has no namespace to inherit from, so a coincidentally
		// matching Namespace-shaped key must never be consulted.
		nsKey, err := scopecache.BuildKey("prod-west", "", "v1", "Namespace", "", "worker-node-7")
		Expect(err).ToNot(HaveOccurred())
		Expect(writer.Set(ctx, nsKey, 30*time.Second)).To(Succeed())

		managed, err := fc.IsManagedResource(ctx, scope.ResourceIdentity{
			ClusterID: "prod-west",
			Kind:      "Node",
			Namespace: "",
			Name:      "worker-node-7",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(managed).To(BeFalse(),
			"IT-FLEET-VALKEY-007: Node is cluster-scoped and must only ever be resolved from its own resource-level label")
	})
})

