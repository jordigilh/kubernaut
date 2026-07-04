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

type mockLocalChecker struct {
	managed map[string]bool
}

func (m *mockLocalChecker) IsManagedResource(_ context.Context, resource scope.ResourceIdentity) (bool, error) {
	key := resource.Namespace + "/" + resource.Kind + "/" + resource.Name
	return m.managed[key], nil
}

type mockRemoteChecker struct {
	managed map[string]bool
	err     error
}

func (m *mockRemoteChecker) IsManagedResource(_ context.Context, resource scope.ResourceIdentity) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	key := resource.ClusterID + "/" + resource.Namespace + "/" + resource.Kind + "/" + resource.Name
	return m.managed[key], nil
}

var _ = Describe("FederatedScopeChecker (BR-INTEGRATION-065)", func() {
	var (
		ctx    context.Context
		local  *mockLocalChecker
		remote *mockRemoteChecker
		fc     *fleet.FederatedScopeChecker
	)

	BeforeEach(func() {
		ctx = context.Background()
		local = &mockLocalChecker{managed: map[string]bool{
			"default/Deployment/nginx": true,
		}}
		remote = &mockRemoteChecker{managed: map[string]bool{
			"prod-east/default/Deployment/nginx": true,
		}}
		fc = fleet.NewFederatedScopeChecker(local, remote, logr.Discard())
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

	It("UT-FLEET-FC-001: empty ClusterID delegates to local checker", func() {
		managed, err := fc.IsManagedResource(ctx, scope.ResourceIdentity{
			Namespace: "default",
			Kind:      "Deployment",
			Name:      "nginx",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(managed).To(BeTrue())
	})

	It("UT-FLEET-FC-002: returns false for unmanaged local resource", func() {
		managed, err := fc.IsManagedResource(ctx, scope.ResourceIdentity{
			Namespace: "default",
			Kind:      "Deployment",
			Name:      "redis",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(managed).To(BeFalse())
	})

	It("UT-FLEET-FC-003: non-empty ClusterID routes to remote backend", func() {
		managed, err := fc.IsManagedResource(ctx, scope.ResourceIdentity{
			ClusterID: "prod-east",
			Kind:      "Deployment",
			Namespace: "default",
			Name:      "nginx",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(managed).To(BeTrue())
	})

	It("UT-FLEET-FC-004: remote miss returns false (fail-safe)", func() {
		managed, err := fc.IsManagedResource(ctx, scope.ResourceIdentity{
			ClusterID: "prod-west",
			Kind:      "Deployment",
			Namespace: "default",
			Name:      "unknown",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(managed).To(BeFalse())
	})

	It("UT-FLEET-FC-005: remote error falls back to unmanaged", func() {
		remote.err = fmt.Errorf("connection refused")
		managed, err := fc.IsManagedResource(ctx, scope.ResourceIdentity{
			ClusterID: "prod-east",
			Kind:      "Deployment",
			Namespace: "default",
			Name:      "nginx",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(managed).To(BeFalse(), "remote error should fall back to unmanaged (fail-safe)")
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
		local   *mockLocalChecker
		remote  *mockRemoteChecker
		cluster *mockClusterLookup
	)

	BeforeEach(func() {
		ctx = context.Background()
		local = &mockLocalChecker{managed: map[string]bool{
			"default/Deployment/nginx": true,
		}}
		remote = &mockRemoteChecker{managed: map[string]bool{
			"prod-east/default/Deployment/nginx": true,
		}}
		cluster = &mockClusterLookup{known: map[string]bool{
			"prod-east": true,
		}}
	})

	It("UT-SCOPE-P1-001 [AC-3]: rejects resource from unknown cluster", func() {
		fc := fleet.NewFederatedScopeChecker(local, remote, logr.Discard(),
			fleet.WithClusterLookup(cluster))

		managed, err := fc.IsManagedResource(ctx, scope.ResourceIdentity{
			ClusterID: "unknown-cluster",
			Kind:      "Deployment",
			Namespace: "default",
			Name:      "nginx",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(managed).To(BeFalse(),
			"resource from unknown cluster must be rejected at cluster level")
	})

	It("UT-SCOPE-P1-002 [AC-3]: passes resource from known cluster to remote checker", func() {
		fc := fleet.NewFederatedScopeChecker(local, remote, logr.Discard(),
			fleet.WithClusterLookup(cluster))

		managed, err := fc.IsManagedResource(ctx, scope.ResourceIdentity{
			ClusterID: "prod-east",
			Kind:      "Deployment",
			Namespace: "default",
			Name:      "nginx",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(managed).To(BeTrue(),
			"resource from known cluster must proceed to resource-level check")
	})

	It("UT-SCOPE-P1-003 [AC-3]: skips cluster check when no ClusterLookup is configured", func() {
		fc := fleet.NewFederatedScopeChecker(local, remote, logr.Discard())

		managed, err := fc.IsManagedResource(ctx, scope.ResourceIdentity{
			ClusterID: "any-cluster",
			Kind:      "Deployment",
			Namespace: "default",
			Name:      "nginx",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(managed).To(BeFalse(),
			"without cluster lookup, remote check proceeds normally (backward compat)")
	})
})
