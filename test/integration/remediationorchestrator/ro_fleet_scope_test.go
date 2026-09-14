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

package remediationorchestrator

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// staticClusterRegistry implements registry.ClusterRegistry with a fixed set of clusters.
// Used in fleet ITs where we need a real FMC handler backed by known clusters
// without requiring envtest Backend CRDs.
type staticClusterRegistry struct {
	clusters map[string]registry.ClusterInfo
}

func newStaticClusterRegistry(ids ...string) *staticClusterRegistry {
	r := &staticClusterRegistry{clusters: make(map[string]registry.ClusterInfo, len(ids))}
	for _, id := range ids {
		r.clusters[id] = registry.ClusterInfo{ID: id}
	}
	return r
}

func (r *staticClusterRegistry) List() []registry.ClusterInfo {
	out := make([]registry.ClusterInfo, 0, len(r.clusters))
	for _, c := range r.clusters {
		out = append(out, c)
	}
	return out
}

func (r *staticClusterRegistry) Get(clusterID string) (registry.ClusterInfo, bool) {
	c, ok := r.clusters[clusterID]
	return c, ok
}

func (r *staticClusterRegistry) WatchClusters() <-chan registry.ClusterEvent {
	ch := make(chan registry.ClusterEvent)
	close(ch)
	return ch
}

func (r *staticClusterRegistry) Ready() bool                   { return true }
func (r *staticClusterRegistry) Start(_ context.Context) error { return nil }
func (r *staticClusterRegistry) Stop()                         {}

// IT-RO-054-FLEET: RO Routing Engine Fleet Scope Integration Tests
//
// These ITs prove that the RO routing engine correctly dispatches scope checks
// through a real FMC stack backed by the suite's shared Redis (port 16381).
// No mocks — the FederatedScopeChecker delegates remote checks to a real FMC
// handler (httptest) backed by the shared Valkey, and local checks to a real
// scope.Manager backed by envtest.
//
// Architecture follows the same pattern as GW fleet signal ITs (ADR-068):
//   - Seed Redis with kubernaut:managed: keys for test clusters
//   - FMC handler + httptest.Server serving /api/v1/scope/check
//   - fmc.HTTPClient as the remote checker in FederatedScopeChecker
//   - scope.Manager(k8sClient) as the local checker
var _ = Describe("BR-FLEET-054: RO Fleet Scope Routing (Integration)", Ordered, Label("fleet", "scope", "integration"), func() {
	var (
		fmcServer     *httptest.Server
		engine        routing.Engine
		testNamespace string
	)

	BeforeAll(func() {
		testNamespace = helpers.CreateTestNamespace(ctx, k8sClient, "ro-fleet-scope")

		valkeyAddr := fmt.Sprintf("127.0.0.1:%d", roRedisPort)

		By("Seeding shared Redis with managed resources for remote clusters")
		// Cache keys use the real GVK ("apps/v1" for Deployment/StatefulSet) matching
		// what pkg/fleet/fmc/syncer.go writes from the K8s API, and what
		// scopecache.Client.IsManagedResource now infers via scope.InferGVK when
		// the caller's ResourceIdentity leaves Group/Version empty (Issue #54 RCA).
		writer := fmc.NewValkeyWriter(valkeyAddr)
		for _, cluster := range []string{"prod-east", "staging-west", "dr-site-east", "any-cluster"} {
			key, err := scopecache.BuildKey(cluster, "apps", "v1", "Deployment", "production", "api-server")
			Expect(err).ToNot(HaveOccurred())
			Expect(writer.Set(ctx, key, 5*time.Minute)).To(Succeed())

			key2, err := scopecache.BuildKey(cluster, "apps", "v1", "StatefulSet", "databases", "postgres")
			Expect(err).ToNot(HaveOccurred())
			Expect(writer.Set(ctx, key2, 5*time.Minute)).To(Succeed())

			key3, err := scopecache.BuildKey(cluster, "apps", "v1", "Deployment", "ecommerce", "cart-svc")
			Expect(err).ToNot(HaveOccurred())
			Expect(writer.Set(ctx, key3, 5*time.Minute)).To(Succeed())

			key4, err := scopecache.BuildKey(cluster, "apps", "v1", "Deployment", "ns", "app")
			Expect(err).ToNot(HaveOccurred())
			Expect(writer.Set(ctx, key4, 5*time.Minute)).To(Succeed())
		}
		_ = writer.Close()

		By("Creating real FMC HTTP stack (scopecache + handler + httptest)")
		cacheReader := scopecache.NewValkeyCacheReader(valkeyAddr)
		scopeClient := scopecache.NewClient(cacheReader)
		clusterReg := newStaticClusterRegistry("prod-east", "staging-west", "dr-site-east", "any-cluster")
		handler := fmc.NewHandler(scopeClient, clusterReg, ctrl.Log.WithName("fmc-test"))
		mux := http.NewServeMux()
		handler.RegisterRoutes(mux)
		fmcServer = httptest.NewServer(mux)

		By("Creating FederatedScopeChecker backed by real FMC + real scope.Manager")
		localChecker := scope.NewManager(k8sClient)
		remoteChecker := fmc.NewHTTPClient(fmcServer.URL)
		federatedChecker := fleet.NewFederatedScopeChecker(localChecker, remoteChecker, ctrl.Log.WithName("federated-scope-test"))

		By("Creating routing engine with real federated scope checker")
		engine = routing.NewRoutingEngine(
			k8sManager.GetClient(), // Cached client provides the WFE target field index.
			k8sClient,
			"",
			routing.Config{
				ScopeBackoffBase: 5,
				ScopeBackoffMax:  300,
			},
			federatedChecker,
		)
	})

	AfterAll(func() {
		if fmcServer != nil {
			fmcServer.Close()
		}
		if testNamespace != "" {
			helpers.DeleteTestNamespace(ctx, k8sClient, testNamespace)
		}
	})

	It("IT-RO-054-FLEET-001 [AC-3]: should route remote ClusterID to FMC and allow managed resource", func() {
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-remote-managed", Namespace: ROControllerNamespace},
			Spec: remediationv1.RemediationRequestSpec{
				ClusterID: "prod-east",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind: "Deployment", Name: "api-server", Namespace: "production",
				},
			},
		}

		condition := engine.CheckUnmanagedResource(ctx, rr)
		Expect(condition).To(BeNil(),
			"AC-3: managed resource on remote cluster must NOT be blocked")
	})

	It("IT-RO-054-FLEET-002 [AC-3]: should route empty ClusterID to local scope.Manager", func() {
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-local", Namespace: ROControllerNamespace},
			Spec: remediationv1.RemediationRequestSpec{
				TargetResource: remediationv1.ResourceIdentifier{
					Kind: "Deployment", Name: "web-app", Namespace: testNamespace,
				},
			},
		}

		condition := engine.CheckUnmanagedResource(ctx, rr)
		Expect(condition).To(BeNil(),
			"AC-3: managed resource on local cluster (managed namespace) must NOT be blocked")
	})

	It("IT-RO-054-FLEET-003 [AC-3]: should block RR when remote scope returns unmanaged", func() {
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-remote-unmanaged", Namespace: ROControllerNamespace},
			Spec: remediationv1.RemediationRequestSpec{
				ClusterID: "prod-east",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind: "Deployment", Name: "unknown-resource", Namespace: "unknown-ns",
				},
			},
		}

		condition := engine.CheckUnmanagedResource(ctx, rr)
		Expect(condition).ToNot(BeNil(),
			"AC-3: unmanaged resource on remote cluster must be blocked")
		Expect(condition.Reason).To(Equal(string(remediationv1.BlockReasonUnmanagedResource)))
	})

	It("IT-RO-054-FLEET-004 [AC-4]: should propagate ClusterID through scope check identity", func() {
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-lineage", Namespace: ROControllerNamespace},
			Spec: remediationv1.RemediationRequestSpec{
				ClusterID: "dr-site-east",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind: "StatefulSet", Name: "postgres", Namespace: "databases",
				},
			},
		}

		condition := engine.CheckUnmanagedResource(ctx, rr)
		Expect(condition).To(BeNil(),
			"AC-4: managed resource on known remote cluster must NOT be blocked")
	})

	It("IT-RO-054-FLEET-005 [AC-3]: should block RR for unknown remote cluster", func() {
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-unknown-cluster", Namespace: ROControllerNamespace},
			Spec: remediationv1.RemediationRequestSpec{
				ClusterID: "unknown-cluster",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind: "Deployment", Name: "api-server", Namespace: "production",
				},
			},
		}

		condition := engine.CheckUnmanagedResource(ctx, rr)
		Expect(condition).ToNot(BeNil(),
			"AC-3: resource on unknown cluster must be blocked (FMC handler rejects unknown clusters)")
		Expect(condition.Reason).To(Equal(string(remediationv1.BlockReasonUnmanagedResource)))
	})

	It("IT-RO-2396-001 [AC-4, AC-6; ASVS V4.1.1/V4.1.3]: should not block an identical target from another cluster", func() {
		suffix := fmt.Sprintf("%d", time.Now().UnixNano())
		targetResource := "kubernaut-system/Deployment/memory-eater"
		blockerRR := helpers.NewRemediationRequest(
			"rr-prod-west-"+suffix,
			ROControllerNamespace,
			helpers.RemediationRequestOpts{
				ClusterID:       "prod-west",
				TargetKind:      "Deployment",
				TargetName:      "memory-eater",
				TargetNamespace: "kubernaut-system",
			},
		)
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "wfe-prod-west-" + suffix,
				Namespace: ROControllerNamespace,
			},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Name:      blockerRR.Name,
					Namespace: blockerRR.Namespace,
				},
				TargetResource: targetResource,
				ClusterID:      "prod-east", // execution cluster override
			},
			Status: workflowexecutionv1.WorkflowExecutionStatus{
				Phase: workflowexecutionv1.PhaseRunning,
			},
		}
		requestingRR := helpers.NewRemediationRequest(
			"rr-remote-cluster-"+suffix,
			ROControllerNamespace,
			helpers.RemediationRequestOpts{
				ClusterID:       "remote-cluster",
				TargetKind:      "Deployment",
				TargetName:      "memory-eater",
				TargetNamespace: "kubernaut-system",
			},
		)

		Expect(k8sClient.Create(ctx, blockerRR)).To(Succeed())
		Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
		DeferCleanup(func() {
			Expect(k8sClient.Delete(context.Background(), wfe)).To(Succeed())
			Expect(k8sClient.Delete(context.Background(), blockerRR)).To(Succeed())
		})

		Eventually(func() bool {
			wfeList := &workflowexecutionv1.WorkflowExecutionList{}
			if err := k8sManager.GetClient().List(ctx, wfeList, client.MatchingFields{
				"spec.targetResource": targetResource,
			}); err != nil {
				return false
			}
			for i := range wfeList.Items {
				if wfeList.Items[i].Name == wfe.Name {
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue(), "blocking WFE should be visible through the target index")

		blocked, err := engine.CheckResourceBusy(ctx, requestingRR, targetResource)
		Expect(errors.Is(err, routing.ErrNotBlocked)).To(BeTrue(),
			"different-cluster target must not be blocked: err=%v blocked=%+v", err, blocked)
		Expect(blocked).To(BeNil())
	})
})
