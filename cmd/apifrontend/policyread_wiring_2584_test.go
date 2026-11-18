package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	spv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/policyread"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	metadatafake "k8s.io/client-go/metadata/fake"
	crfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("BR-SCOPE-001: policy-read dependency wiring", func() {
	DescribeTable("IT-AF-ROUTE-010 / IT-AF-PRES-007 [BR-SCOPE-001]: production dependency factory, action and exact inventory retain enum authority",
		func(state scope.ScopeState, source scope.ScopeSource, status int, expected inventory.State) {
			resource := inventory.Resource{ClusterID: "hub", Version: "v1", Kind: "Pod", Namespace: "payments", Name: "api",
				Scope: inventory.Scope{Included: true, ResourceLabel: inventory.LabelTrue, NamespaceLabel: inventory.LabelAbsent, Source: inventory.ScopeSourceResourceLabel}}
			resource.ID = inventory.CanonicalResourceID(resource.Identity())
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				defer GinkgoRecover()
				Expect(request.Header.Get("Authorization")).To(Equal("Bearer scope-test"))
				w.Header().Set("Content-Type", "application/json")
				if request.URL.Path == fmc.ScopeCheckPath {
					w.WriteHeader(status)
					Expect(json.NewEncoder(w).Encode(scope.ScopeResult{State: state, Provenance: source})).To(Succeed())
					return
				}
				Expect(json.NewEncoder(w).Encode(resource)).To(Succeed())
			}))
			defer server.Close()
			token := filepath.Join(GinkgoT().TempDir(), "token")
			Expect(os.WriteFile(token, []byte("scope-test"), 0o600)).To(Succeed())
			cfg := &config.Config{Fleet: fleet.FleetConfig{Enabled: true, Backend: fleet.BackendFMC, Endpoint: server.URL, TokenPath: token}}
			scheme := runtime.NewScheme()
			Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
			deps := &backendDeps{k8sTypedClient: crfake.NewClientBuilder().WithScheme(scheme).Build()}
			Expect(buildScopeCheckerDeps(cfg, deps, logr.Discard())).To(Succeed())
			Expect(buildPolicyReadDeps(cfg, deps, logr.Discard())).To(Succeed())
			if expected != inventory.StateSuccess {
				result, err := tools.HandleCreateRR(context.Background(), &tools.ToolDeps{Client: deps.k8sTypedClient, ControllerNS: "kubernaut-system", ScopeChecker: deps.ScopeChecker},
					&tools.CreateRRArgs{ClusterID: "hub", APIVersion: "v1", Kind: "Pod", Namespace: "payments", Name: "api", Description: "scope contract"}, "alice")
				Expect(err).To(HaveOccurred())
				Expect(result.RRID).To(BeEmpty())
				if state == scope.ScopeVerifiedUnmanaged {
					Expect(errors.Is(err, tools.ErrResourceNotManaged)).To(BeTrue())
				} else {
					Expect(errors.Is(err, tools.ErrScopeUnavailable)).To(BeTrue())
					Expect(err.Error()).NotTo(ContainSubstring("Add label"))
					Expect(err.Error()).NotTo(ContainSubstring("not managed"))
				}
				var requests remediationv1.RemediationRequestList
				Expect(deps.k8sTypedClient.List(context.Background(), &requests)).To(Succeed())
				Expect(requests.Items).To(BeEmpty())
			}
			service, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{RemoteInventory: deps.RemoteInventory, ScopeChecker: deps.ScopeChecker, Authorizer: projectionAllow{}})
			Expect(err).NotTo(HaveOccurred())
			result, err := service.GetInventory(context.Background(), &auth.UserIdentity{Username: "alice"}, inventory.Filters{ClusterID: "hub"}, resource.ID)
			if expected == inventory.StateSuccess {
				Expect(err).NotTo(HaveOccurred())
				Expect(result.ID).To(Equal(resource.ID))
			} else {
				Expect(err).To(HaveOccurred())
				Expect(policyread.StateOf(err)).To(Equal(expected))
				Expect(result).To(Equal(inventory.Resource{}))
			}
		},
		Entry("verified managed", scope.ScopeVerifiedManaged, scope.ScopeSourceResourceLabel, 200, inventory.StateSuccess),
		Entry("only verified unmanaged is NotFound", scope.ScopeVerifiedUnmanaged, scope.ScopeSourceResourceLabel, 200, inventory.StateNotFound),
		Entry("unavailable has no row", scope.ScopeUnavailable, scope.ScopeSourceUnavailable, 503, inventory.StateUnavailable),
		Entry("unsupported has no row", scope.ScopeUnsupported, scope.ScopeSourceUnavailable, 422, inventory.StateUnavailable),
		Entry("invalid has no row", scope.ScopeInvalid, scope.ScopeSourceUnavailable, 422, inventory.StateUnavailable),
		Entry("unauthorized has no row", scope.ScopeUnauthorized, scope.ScopeSourceUnavailable, 403, inventory.StateUnavailable),
	)
	DescribeTable("IT-AF-2593-REGISTRY-003 [BR-SCOPE-001]: matching managed registry and ACM identity permits scope evaluation",
		func(gatewayType registry.MCPGatewayType, gvr schema.GroupVersionResource, apiVersion, kind, listKind string) {
			registration := &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": apiVersion, "kind": kind,
				"metadata": map[string]interface{}{"name": "local-cluster", "uid": "registration-local-cluster", "namespace": "kubernaut-system", "labels": map[string]interface{}{"kubernaut.ai/managed": "true"}},
			}}
			dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{gvr: listKind}, registration)
			clusterRegistry, err := registry.NewClusterRegistry(gatewayType, dynamicClient, registry.RegistryConfig{}, nil, logr.Discard())
			Expect(err).NotTo(HaveOccurred())
			Expect(clusterRegistry.Start(context.Background())).To(Succeed())
			DeferCleanup(clusterRegistry.Stop)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if _, err := io.WriteString(w, `{"data":{"searchResult":[{"count":1,"items":[{"cluster":"local-cluster","apiversion":"v1","kind":"ConfigMap","namespace":"payments","name":"checkout","label":"kubernaut.ai/managed=true"}]}]}}`); err != nil {
					GinkgoWriter.Printf("write Search response: %v\n", err)
				}
			}))
			DeferCleanup(server.Close)
			deps := &backendDeps{FleetClusterRegistry: clusterRegistry, k8sTypedClient: crfake.NewClientBuilder().Build()}
			cfg := &config.Config{Fleet: fleet.FleetConfig{Enabled: true, Backend: fleet.BackendACM, Endpoint: server.URL}}
			Expect(buildScopeCheckerDeps(cfg, deps, logr.Discard())).To(Succeed())
			managed, err := deps.ScopeChecker.Check(context.Background(), scope.ResourceIdentity{ClusterID: "local-cluster", Version: "v1", Kind: "ConfigMap", Namespace: "payments", Name: "checkout"})
			Expect(err).NotTo(HaveOccurred())
			Expect(managed.State).To(Equal(scope.ScopeVerifiedManaged))
		},
		Entry("Envoy AI Gateway", registry.GatewayEAIGW, registry.BackendGVR, "gateway.envoyproxy.io/v1alpha1", "Backend", "BackendList"),
		Entry("Kuadrant", registry.GatewayKuadrant, registry.MCPServerRegistrationGVR, "mcp.kuadrant.io/v1", "MCPServerRegistration", "MCPServerRegistrationList"),
	)

	It("IT-AF-2593-REGISTRY-002 [BR-SECURITY-016]: rejects ACM targets when the managed gateway registry is unavailable", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if _, err := io.WriteString(w, `{"data":{"searchResult":[{"count":1,"items":[{"cluster":"prod-east","apiversion":"v1","kind":"ConfigMap","namespace":"payments","name":"checkout","label":"kubernaut.ai/managed=true"}]}]}}`); err != nil {
				GinkgoWriter.Printf("write Search response: %v\n", err)
			}
		}))
		DeferCleanup(server.Close)
		deps := &backendDeps{k8sTypedClient: crfake.NewClientBuilder().Build()}
		cfg := &config.Config{Fleet: fleet.FleetConfig{Enabled: true, Backend: fleet.BackendACM, Endpoint: server.URL}}
		Expect(buildScopeCheckerDeps(cfg, deps, logr.Discard())).To(MatchError("acm scope checking requires a managed gateway registry"))
		Expect(deps.ScopeChecker).To(BeNil())
	})

	It("IT-AF-2593-REGISTRY-001 [BR-SECURITY-016]: rejects an ACM target outside the managed gateway registry", func() {
		backend := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "gateway.envoyproxy.io/v1alpha1", "kind": "Backend",
			"metadata": map[string]interface{}{"name": "local-cluster", "uid": "backend-local-cluster", "namespace": "kubernaut-system", "labels": map[string]interface{}{"kubernaut.ai/managed": "true"}},
		}}
		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{registry.BackendGVR: "BackendList"}, backend)
		clusterRegistry, err := registry.NewClusterRegistry(registry.GatewayEAIGW, dynamicClient, registry.RegistryConfig{}, nil, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		Expect(clusterRegistry.Start(context.Background())).To(Succeed())
		DeferCleanup(clusterRegistry.Stop)
		Expect(clusterRegistry.Ready()).To(BeTrue())
		_, found := clusterRegistry.Get("local-cluster")
		Expect(found).To(BeTrue())

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if _, err := io.WriteString(w, `{"data":{"searchResult":[{"count":1,"items":[{"cluster":"other-cluster","apiversion":"v1","kind":"ConfigMap","namespace":"payments","name":"checkout","label":"kubernaut.ai/managed=true"}]}]}}`); err != nil {
				GinkgoWriter.Printf("write Search response: %v\n", err)
			}
		}))
		DeferCleanup(server.Close)
		deps := &backendDeps{FleetClusterRegistry: clusterRegistry, k8sTypedClient: crfake.NewClientBuilder().Build()}
		cfg := &config.Config{Fleet: fleet.FleetConfig{Enabled: true, Backend: fleet.BackendACM, Endpoint: server.URL}}
		Expect(buildScopeCheckerDeps(cfg, deps, logr.Discard())).To(Succeed())
		target := scope.ResourceIdentity{ClusterID: "other-cluster", Version: "v1", Kind: "ConfigMap", Namespace: "payments", Name: "checkout"}
		managed, err := deps.ScopeChecker.Check(context.Background(), target)
		Expect(managed.State).To(Equal(scope.ScopeUnavailable), "an unregistered cluster is not an observed unmanaged target")
		Expect(err).NotTo(HaveOccurred())
		Expect(managed.Provenance).To(Equal(scope.ScopeSourceUnavailable))
	})
	It("IT-AF-2584-ACM-FAIL-CLOSED [BR-SECURITY-016]: production ACM wiring returns unavailable for Console inventory", func() {
		cfg := &config.Config{Fleet: fleet.FleetConfig{Enabled: true, Backend: fleet.BackendACM, Endpoint: "https://acm.example", TokenPath: "/unused/token"}}
		deps := &backendDeps{}
		Expect(buildPolicyReadDeps(cfg, deps, logr.Discard())).To(Succeed())
		Expect(deps.RemoteInventory).To(BeNil(), "unqualified ACM inventory must not be wired")
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{RemoteInventory: deps.RemoteInventory, Authorizer: projectionAllow{}})
		Expect(err).NotTo(HaveOccurred())
		user := &auth.UserIdentity{Username: "alice"}
		page, err := svc.ListInventory(context.Background(), user, inventory.Filters{ClusterID: "prod-east"})
		Expect(err).To(HaveOccurred())
		Expect(policyread.StateOf(err)).To(Equal(inventory.StateUnavailable))
		Expect(page.Items).To(BeEmpty())
		policies, err := svc.ListPolicyScope(context.Background(), user, inventory.Filters{ClusterID: "prod-east"})
		Expect(err).To(HaveOccurred())
		Expect(policyread.StateOf(err)).To(Equal(inventory.StateUnavailable))
		Expect(policies.Items).To(BeEmpty())
		_, err = svc.GetInventory(context.Background(), user, inventory.Filters{ClusterID: "prod-east"}, "opaque-id")
		Expect(err).To(HaveOccurred())
		Expect(policyread.StateOf(err)).To(Equal(inventory.StateUnavailable))
	})

	It("UT-AF-2584-FMC-READER [BR-SECURITY-016]: retains the FMC Console reader", func() {
		cfg := &config.Config{Fleet: fleet.FleetConfig{Enabled: true, Backend: fleet.BackendFMC, Endpoint: "https://fmc.example"}}
		deps := &backendDeps{}
		Expect(buildPolicyReadDeps(cfg, deps, logr.Discard())).To(Succeed())
		Expect(deps.RemoteInventory).NotTo(BeNil())
	})
	It("IT-AF-2584-001 [BR-API-001]: constructs local typed policy/history and inventory sources from AF clients", func() {
		scheme := runtime.NewScheme()
		Expect(v1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
		Expect(spv1.AddToScheme(scheme)).To(Succeed())
		Expect(metav1.AddMetaToScheme(scheme)).To(Succeed())
		deps := &backendDeps{
			k8sDynClient:      dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()),
			k8sMetadataClient: metadatafake.NewSimpleMetadataClient(scheme),
			k8sPolicyClient:   crfake.NewClientBuilder().WithScheme(scheme).Build(),
		}

		Expect(buildPolicyReadDeps(&config.Config{}, deps, logr.Discard())).To(Succeed())
		Expect(deps.LocalInventory).NotTo(BeNil())
		Expect(deps.PolicySource).NotTo(BeNil())
		Expect(deps.HistorySource).NotTo(BeNil())
		Expect(deps.RemoteInventory).To(BeNil())
	})

	It("IT-AF-2584-009 [BR-API-001]: keeps existing A2A and MCP routes mounted alongside the Console read routes", func() {
		params := testRouterBuildParams(&dualToolConsoleAuthorizer{})
		// Distinct bodies prove each path reaches the handler mounted for it,
		// not merely that some handler answered 200.
		params.MCPHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-Mounted-By", "mcp")
			w.WriteHeader(http.StatusOK)
		})
		params.A2AHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-Mounted-By", "a2a")
			w.WriteHeader(http.StatusOK)
		})
		_, router, err := buildRouterConfig(params)
		Expect(err).NotTo(HaveOccurred())

		// The agentic surface keeps serving after the Console read routes are
		// registered on the same router. This fixture uses a pass-through auth
		// middleware, so it proves mounting and dispatch only; the authenticated
		// chain is covered by the router-wiring container.
		mcpRecorder := httptest.NewRecorder()
		router.ServeHTTP(mcpRecorder, httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody))
		Expect(mcpRecorder.Code).To(Equal(http.StatusOK))
		Expect(mcpRecorder.Header().Get("X-Mounted-By")).To(Equal("mcp"))

		a2aRecorder := httptest.NewRecorder()
		router.ServeHTTP(a2aRecorder, httptest.NewRequest(http.MethodPost, "/a2a/invoke", http.NoBody))
		Expect(a2aRecorder.Code).To(Equal(http.StatusOK))
		Expect(a2aRecorder.Header().Get("X-Mounted-By")).To(Equal("a2a"))

		// The Console routes are additive: they answer on their own paths and
		// reject an unauthenticated caller instead of widening the router.
		consoleRecorder := httptest.NewRecorder()
		router.ServeHTTP(consoleRecorder, httptest.NewRequest(http.MethodGet, "/api/v1/console/inventory/managed-resources?cluster_id=local", http.NoBody))
		Expect(consoleRecorder.Code).To(Equal(http.StatusUnauthorized))
	})

	It("IT-AF-2584-009 [BR-SECURITY-016, FedRAMP AC-4/AC-6]: keeps both surfaces behind the configured authentication middleware", func() {
		params := testRouterBuildParams(&dualToolConsoleAuthorizer{})
		// A distinctive stop code proves the middleware ran: the Console handler
		// would answer 401 on its own, so 401 could not distinguish the two.
		params.AuthMiddleware = func(http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-Auth-Middleware", "applied")
				w.WriteHeader(http.StatusTeapot)
			})
		}
		_, router, err := buildRouterConfig(params)
		Expect(err).NotTo(HaveOccurred())

		for _, probe := range []struct{ method, path string }{
			{http.MethodPost, "/mcp"},
			{http.MethodGet, "/api/v1/console/inventory/managed-resources?cluster_id=local"},
			{http.MethodGet, "/api/v1/console/policies/scope?cluster_id=local"},
		} {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(probe.method, probe.path, http.NoBody))
			Expect(recorder.Code).To(Equal(http.StatusTeapot), "%s %s must not bypass the middleware", probe.method, probe.path)
			Expect(recorder.Header().Get("X-Auth-Middleware")).To(Equal("applied"), probe.path)
		}
	})
})

type projectionAllow struct{}

func (projectionAllow) Check(context.Context, string, []string, string) (bool, error) {
	return true, nil
}
func (projectionAllow) CheckTarget(context.Context, string, []string, auth.TargetAccess) (bool, error) {
	return true, nil
}
