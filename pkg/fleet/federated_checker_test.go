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
