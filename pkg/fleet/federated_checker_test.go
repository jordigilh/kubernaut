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

package fleet_test

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// mockLocalChecker is the local-lane stub used by factory tests (the
// federated checker itself no longer accepts a local checker, #2660 m7).
type mockLocalChecker struct {
	managed map[string]bool
}

func (m *mockLocalChecker) Check(_ context.Context, resource scope.ResourceIdentity) (scope.ScopeResult, error) {
	key := resource.Namespace + "/" + resource.Kind + "/" + resource.Name
	return scope.ScopeResult{State: map[bool]scope.ScopeState{true: scope.ScopeVerifiedManaged, false: scope.ScopeVerifiedUnmanaged}[m.managed[key]], Provenance: scope.ScopeSourceResourceLabel}, nil
}

type mockRemoteChecker struct {
	managed map[string]bool
	err     error
}

func (m *mockRemoteChecker) Check(_ context.Context, resource scope.ResourceIdentity) (scope.ScopeResult, error) {
	if m.err != nil {
		return scope.ScopeResult{State: scope.ScopeUnavailable, Provenance: scope.ScopeSourceUnavailable}, m.err
	}
	key := resource.ClusterID + "/" + resource.Namespace + "/" + resource.Kind + "/" + resource.Name
	return scope.ScopeResult{State: map[bool]scope.ScopeState{true: scope.ScopeVerifiedManaged, false: scope.ScopeVerifiedUnmanaged}[m.managed[key]], Provenance: scope.ScopeSourceResourceLabel}, nil
}

var _ = Describe("FederatedScopeChecker (BR-INTEGRATION-065)", func() {
	var (
		ctx    context.Context
		remote *mockRemoteChecker
		fc     *fleet.FederatedScopeChecker
	)

	BeforeEach(func() {
		ctx = context.Background()
		remote = &mockRemoteChecker{managed: map[string]bool{
			"prod-east/default/Deployment/nginx": true,
		}}
		fc = fleet.NewFederatedScopeChecker(remote, logr.Discard())
	})

	// Readiness gate Wave 2 (#1553): GW's readiness wiring needs to reach the
	// remote scope-check backend (fmc.HTTPClient/acm.Client) to build a
	// ScopeCheckerProber. Remote() exposes it without leaking the field.
	It("UT-FLEET-FED-CHK-001 [readiness gate Wave 2]: Remote returns the configured remote checker", func() {
		Expect(fc.Remote()).To(BeIdenticalTo(remote))
	})

	It("UT-FLEET-USI-001 [SI-10]: FederatedScopeChecker implements ScopeChecker interface", func() {
		var checker scope.ScopeChecker = fc
		Expect(checker).ToNot(BeNil())
	})

	It("UT-FLEET-FC-001: Fleet empty ClusterID cannot fall back to local checker", func() {
		managed, err := fc.Check(ctx, scope.ResourceIdentity{
			Namespace: "default",
			Kind:      "Deployment",
			Name:      "nginx",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(managed.State).To(Equal(scope.ScopeInvalid))
	})

	It("UT-FLEET-FC-002: missing Fleet attribution remains invalid for any target", func() {
		managed, err := fc.Check(ctx, scope.ResourceIdentity{
			Namespace: "default",
			Kind:      "Deployment",
			Name:      "redis",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(managed.State).To(Equal(scope.ScopeInvalid))
	})

	It("UT-FLEET-FC-003: non-empty ClusterID routes to remote backend", func() {
		managed, err := fc.Check(ctx, scope.ResourceIdentity{
			ClusterID: "prod-east",
			Group:     "apps",
			Version:   "v1",
			Kind:      "Deployment",
			Namespace: "default",
			Name:      "nginx",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(managed.State).To(Equal(scope.ScopeVerifiedManaged))
	})

	It("UT-FLEET-FC-004: remote miss returns an evaluated unmanaged result", func() {
		managed, err := fc.Check(ctx, scope.ResourceIdentity{
			ClusterID: "prod-west",
			Group:     "apps",
			Version:   "v1",
			Kind:      "Deployment",
			Namespace: "default",
			Name:      "unknown",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(managed.State).To(Equal(scope.ScopeVerifiedUnmanaged))
	})

	It("UT-FLEET-FC-005: remote error is preserved by the canonical checker", func() {
		remote.err = fmt.Errorf("connection refused")
		managed, err := fc.Check(ctx, scope.ResourceIdentity{
			ClusterID: "prod-east",
			Group:     "apps",
			Version:   "v1",
			Kind:      "Deployment",
			Namespace: "default",
			Name:      "nginx",
		})
		Expect(err).To(HaveOccurred())
		Expect(managed.State).To(Equal(scope.ScopeUnavailable))
	})

	It("UT-FLEET-2584-SCOPE-001 [BR-SECURITY-016]: preserves remote errors through the federated checker", func() {
		remote.err = fmt.Errorf("backend unavailable")
		managed, err := fc.Check(ctx, scope.ResourceIdentity{ClusterID: "prod-east", Group: "apps", Version: "v1", Kind: "Deployment", Name: "nginx"})
		Expect(managed.State).To(Equal(scope.ScopeUnavailable))
		Expect(err).To(MatchError(ContainSubstring("backend unavailable")))
	})

	It("UT-FLEET-2592-GVK-001 [SI-10]: rejects a remote identity without explicit Group and Version", func() {
		managed, err := fc.IsManagedResource(ctx, scope.ResourceIdentity{
			ClusterID: "prod-east",
			Kind:      "Deployment",
			Namespace: "default",
			Name:      "nginx",
		})

		Expect(managed).To(BeFalse())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid_remote_gvk"))
	})

	It("UT-FLEET-2592-TYPED-001 [BR-SCOPE-001, AC-4]: preserves a verified remote decision in the typed contract", func() {
		result, err := fc.Check(ctx, scope.ResourceIdentity{
			ClusterID: "prod-east",
			Group:     "apps",
			Version:   "v1",
			Kind:      "Deployment",
			Namespace: "default",
			Name:      "nginx",
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(result.State).To(Equal(scope.ScopeVerifiedManaged))
	})

	It("UT-FLEET-2592-TYPED-002 [BR-SCOPE-001, SI-10]: returns invalid for an ambiguous remote identity", func() {
		result, err := fc.Check(ctx, scope.ResourceIdentity{
			ClusterID: "prod-east",
			Kind:      "Deployment",
			Namespace: "default",
			Name:      "nginx",
		})

		Expect(err).NotTo(HaveOccurred(), "an invalid identity is a completed domain result, not an execution failure")
		Expect(result.State).To(Equal(scope.ScopeInvalid))
		Expect(result.Provenance).To(Equal(scope.ScopeSourceUnavailable))
	})
})

// mockClusterLookup implements fleet.ClusterLookup for testing.
type mockClusterLookup struct {
	known map[string]bool
}

func (m *mockClusterLookup) IsKnownCluster(clusterID string) bool {
	return m.known[clusterID]
}

var _ = Describe("FederatedScopeChecker — Cluster-level precondition (P1)", func() {
	var (
		ctx     context.Context
		remote  *mockRemoteChecker
		cluster *mockClusterLookup
	)

	BeforeEach(func() {
		ctx = context.Background()
		remote = &mockRemoteChecker{managed: map[string]bool{
			"prod-east/default/Deployment/nginx": true,
		}}
		cluster = &mockClusterLookup{known: map[string]bool{
			"prod-east": true,
		}}
	})

	It("UT-SCOPE-P1-001 [AC-3]: rejects resource from unknown cluster", func() {
		fc := fleet.NewFederatedScopeChecker(remote, logr.Discard(),
			fleet.WithClusterLookup(cluster))

		managed, err := fc.Check(ctx, scope.ResourceIdentity{
			ClusterID: "unknown-cluster",
			Group:     "apps",
			Version:   "v1",
			Kind:      "Deployment",
			Namespace: "default",
			Name:      "nginx",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(managed.State).To(Equal(scope.ScopeUnavailable),
			"resource from unknown cluster must be rejected at cluster level")
	})

	It("UT-SCOPE-P1-002 [AC-3]: passes resource from known cluster to remote checker", func() {
		fc := fleet.NewFederatedScopeChecker(remote, logr.Discard(),
			fleet.WithClusterLookup(cluster))

		managed, err := fc.Check(ctx, scope.ResourceIdentity{
			ClusterID: "prod-east",
			Group:     "apps",
			Version:   "v1",
			Kind:      "Deployment",
			Namespace: "default",
			Name:      "nginx",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(managed.State).To(Equal(scope.ScopeVerifiedManaged),
			"resource from known cluster must proceed to resource-level check")
	})

	It("UT-SCOPE-P1-003 [AC-3]: skips cluster check when no ClusterLookup is configured", func() {
		fc := fleet.NewFederatedScopeChecker(remote, logr.Discard())

		managed, err := fc.Check(ctx, scope.ResourceIdentity{
			ClusterID: "any-cluster",
			Group:     "apps",
			Version:   "v1",
			Kind:      "Deployment",
			Namespace: "default",
			Name:      "nginx",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(managed.State).To(Equal(scope.ScopeVerifiedUnmanaged),
			"without cluster lookup, remote check proceeds normally (backward compat)")
	})
})
