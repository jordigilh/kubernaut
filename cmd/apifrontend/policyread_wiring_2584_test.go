package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	spv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/policyread"
	"github.com/jordigilh/kubernaut/pkg/fleet"
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

var _ = Describe("IT-AF-2584-001/002/003: policy-read dependency wiring", func() {
	DescribeTable("IT-AF-2593-REGISTRY-003 [BR-SCOPE-001]: matching managed registry and ACM identity permits scope evaluation",
		func(gatewayType registry.MCPGatewayType, gvr schema.GroupVersionResource, apiVersion, kind, listKind string) {
			registration := &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": apiVersion, "kind": kind,
				"metadata": map[string]interface{}{"name": "local-cluster", "namespace": "kubernaut-system", "labels": map[string]interface{}{"kubernaut.ai/managed": "true"}},
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
			strict, ok := deps.ScopeChecker.(scope.StrictScopeChecker)
			Expect(ok).To(BeTrue())
			managed, err := strict.IsManagedResourceStrict(context.Background(), scope.ResourceIdentity{ClusterID: "local-cluster", Version: "v1", Kind: "ConfigMap", Namespace: "payments", Name: "checkout"})
			Expect(err).NotTo(HaveOccurred())
			Expect(managed).To(BeTrue())
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
		Expect(buildScopeCheckerDeps(cfg, deps, logr.Discard())).To(Succeed())
		strict, ok := deps.ScopeChecker.(scope.StrictScopeChecker)
		Expect(ok).To(BeTrue())

		managed, err := strict.IsManagedResourceStrict(context.Background(), scope.ResourceIdentity{ClusterID: "prod-east", Version: "v1", Kind: "ConfigMap", Namespace: "payments", Name: "checkout"})
		Expect(managed).To(BeFalse(), "an unavailable registry must not authorize a cluster")
		Expect(err).To(HaveOccurred())
	})

	It("IT-AF-2593-REGISTRY-001 [BR-SECURITY-016]: rejects an ACM target outside the managed gateway registry", func() {
		backend := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "gateway.envoyproxy.io/v1alpha1", "kind": "Backend",
			"metadata": map[string]interface{}{"name": "local-cluster", "namespace": "kubernaut-system", "labels": map[string]interface{}{"kubernaut.ai/managed": "true"}},
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
		strict, ok := deps.ScopeChecker.(scope.StrictScopeChecker)
		Expect(ok).To(BeTrue())

		target := scope.ResourceIdentity{ClusterID: "other-cluster", Version: "v1", Kind: "ConfigMap", Namespace: "payments", Name: "checkout"}
		managed, err := strict.IsManagedResourceStrict(context.Background(), target)
		Expect(managed).To(BeFalse(), "an unregistered cluster is not an authorized ACM target")
		Expect(err).To(HaveOccurred(), "strict checking must explain the unregistered target")
		managed, err = deps.ScopeChecker.IsManagedResource(context.Background(), target)
		Expect(err).NotTo(HaveOccurred(), "legacy callers retain fail-closed behavior")
		Expect(managed).To(BeFalse())
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
	It("constructs local typed policy/history and inventory sources from AF clients", func() {
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
})

type projectionAllow struct{}

func (projectionAllow) Check(context.Context, string, []string, string) (bool, error) {
	return true, nil
}
func (projectionAllow) CheckTarget(context.Context, string, []string, auth.TargetAccess) (bool, error) {
	return true, nil
}
