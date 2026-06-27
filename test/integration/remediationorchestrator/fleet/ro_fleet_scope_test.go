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
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

// recordingScopeChecker records which ResourceIdentity it was called with.
type recordingScopeChecker struct {
	mu        sync.Mutex
	calls     []scope.ResourceIdentity
	managed   bool
	returnErr error
}

func (r *recordingScopeChecker) IsManagedResource(_ context.Context, identity scope.ResourceIdentity) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, identity)
	return r.managed, r.returnErr
}

func (r *recordingScopeChecker) getCalls() []scope.ResourceIdentity {
	r.mu.Lock()
	defer r.mu.Unlock()
	dst := make([]scope.ResourceIdentity, len(r.calls))
	copy(dst, r.calls)
	return dst
}

// IT-RO-054-FLEET: RO Routing Engine Fleet Scope Integration Tests
//
// Component-level ITs that verify the wiring of:
//
//	RoutingEngine → FederatedScopeChecker → (local/remote) ScopeChecker
//
// They prove that:
//   - RR with ClusterID="" routes to the local ScopeChecker
//   - RR with ClusterID="prod-east" routes to the remote ScopeChecker
//   - Remote scope returning false blocks the RR (AC-3)
//   - Remote scope returning true allows the RR to proceed (AC-3)
//   - ClusterID lineage is preserved in scope check identity (AC-4)
var _ = Describe("BR-FLEET-054: RO Fleet Scope Routing (Integration)", Label("fleet", "scope", "integration"), func() {
	var (
		ctx           context.Context
		localChecker  *recordingScopeChecker
		remoteChecker *recordingScopeChecker
		engine        routing.Engine
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	buildEngine := func(localManaged, remoteManaged bool) {
		localChecker = &recordingScopeChecker{managed: localManaged}
		remoteChecker = &recordingScopeChecker{managed: remoteManaged}
		logger := zap.New(zap.UseDevMode(true))
		federated := fleet.NewFederatedScopeChecker(localChecker, remoteChecker, logger)

		fakeClient := fake.NewClientBuilder().Build()
		engine = routing.NewRoutingEngine(
			fakeClient,
			fakeClient,
			"",
			routing.Config{
				ScopeBackoffBase: 5,
				ScopeBackoffMax:  300,
			},
			federated,
		)
	}

	It("IT-RO-054-FLEET-001 [AC-3]: should route remote ClusterID to remote ScopeChecker", func() {
		buildEngine(true, false)

		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-remote", Namespace: "kubernaut-system"},
			Spec: remediationv1.RemediationRequestSpec{
				ClusterID: "prod-east",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind: "Deployment", Name: "api-server", Namespace: "production",
				},
			},
		}

		condition := engine.CheckUnmanagedResource(ctx, rr)
		Expect(condition).ToNot(BeNil(),
			"AC-3: remote scope returning false must block the RR")
		Expect(condition.Reason).To(Equal(string(remediationv1.BlockReasonUnmanagedResource)))

		remoteCalls := remoteChecker.getCalls()
		Expect(remoteCalls).To(HaveLen(1),
			"AC-3: FederatedScopeChecker must route to remote checker for non-empty ClusterID")
		Expect(remoteCalls[0].ClusterID).To(Equal("prod-east"))
		Expect(remoteCalls[0].Kind).To(Equal("Deployment"))
		Expect(remoteCalls[0].Namespace).To(Equal("production"))
		Expect(remoteCalls[0].Name).To(Equal("api-server"))

		localCalls := localChecker.getCalls()
		Expect(localCalls).To(BeEmpty(),
			"AC-3: local checker must NOT be called for remote ClusterID")
	})

	It("IT-RO-054-FLEET-002 [AC-3]: should route empty ClusterID to local ScopeChecker", func() {
		buildEngine(false, true)

		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-local", Namespace: "kubernaut-system"},
			Spec: remediationv1.RemediationRequestSpec{
				TargetResource: remediationv1.ResourceIdentifier{
					Kind: "Deployment", Name: "web-app", Namespace: "default",
				},
			},
		}

		condition := engine.CheckUnmanagedResource(ctx, rr)
		Expect(condition).ToNot(BeNil(),
			"Local scope returning false must block the RR")
		Expect(condition.Reason).To(Equal(string(remediationv1.BlockReasonUnmanagedResource)))

		localCalls := localChecker.getCalls()
		Expect(localCalls).To(HaveLen(1),
			"AC-3: FederatedScopeChecker must route to local checker for empty ClusterID")
		Expect(localCalls[0].ClusterID).To(BeEmpty())
		Expect(localCalls[0].Kind).To(Equal("Deployment"))
		Expect(localCalls[0].Namespace).To(Equal("default"))

		remoteCalls := remoteChecker.getCalls()
		Expect(remoteCalls).To(BeEmpty(),
			"AC-3: remote checker must NOT be called for empty ClusterID")
	})

	It("IT-RO-054-FLEET-003 [AC-3]: should allow RR when remote scope returns managed=true", func() {
		buildEngine(true, true)

		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-managed-remote", Namespace: "kubernaut-system"},
			Spec: remediationv1.RemediationRequestSpec{
				ClusterID: "staging-west",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind: "Deployment", Name: "cart-svc", Namespace: "ecommerce",
				},
			},
		}

		condition := engine.CheckUnmanagedResource(ctx, rr)
		Expect(condition).To(BeNil(),
			"AC-3: remote scope returning true must NOT block the RR")

		remoteCalls := remoteChecker.getCalls()
		Expect(remoteCalls).To(HaveLen(1))
		Expect(remoteCalls[0].ClusterID).To(Equal("staging-west"))
	})

	It("IT-RO-054-FLEET-004 [AC-4]: should propagate ClusterID through scope check identity", func() {
		buildEngine(true, true)

		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-lineage", Namespace: "kubernaut-system"},
			Spec: remediationv1.RemediationRequestSpec{
				ClusterID: "dr-site-east",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind: "StatefulSet", Name: "postgres", Namespace: "databases",
				},
			},
		}

		condition := engine.CheckUnmanagedResource(ctx, rr)
		Expect(condition).To(BeNil())

		remoteCalls := remoteChecker.getCalls()
		Expect(remoteCalls).To(HaveLen(1))
		identity := remoteCalls[0]
		Expect(identity.ClusterID).To(Equal("dr-site-east"),
			"AC-4: ClusterID lineage must be preserved in scope check identity")
		Expect(identity.Kind).To(Equal("StatefulSet"))
		Expect(identity.Namespace).To(Equal("databases"))
		Expect(identity.Name).To(Equal("postgres"))
	})

	When("using AlwaysManagedScopeChecker as remote backend", func() {
		It("IT-RO-054-FLEET-005 [AC-3]: should allow all remote RRs through", func() {
			logger := zap.New(zap.UseDevMode(true))
			federated := fleet.NewFederatedScopeChecker(
				&mocks.AlwaysManagedScopeChecker{},
				&mocks.AlwaysManagedScopeChecker{},
				logger,
			)

			fakeClient := fake.NewClientBuilder().Build()
			eng := routing.NewRoutingEngine(
				fakeClient, fakeClient, "",
				routing.Config{ScopeBackoffBase: 5, ScopeBackoffMax: 300},
				federated,
			)

			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "rr-always-managed", Namespace: "kubernaut-system"},
				Spec: remediationv1.RemediationRequestSpec{
					ClusterID: "any-cluster",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind: "Deployment", Name: "app", Namespace: "ns",
					},
				},
			}

			condition := eng.CheckUnmanagedResource(ctx, rr)
			Expect(condition).To(BeNil(),
				"AC-3: AlwaysManagedScopeChecker must let all RRs through")
		})
	})
})
