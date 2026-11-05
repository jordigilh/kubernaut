package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/metrics"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/policyread"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	metadatafake "k8s.io/client-go/metadata/fake"
)

var _ = Describe("BR-API-001 / BR-SCOPE-001: production AF router local projection", func() {
	It("IT-AF-2592-LOCAL-ROUTER-001 [BR-API-001]: serves the local backend with observed time and freshness metadata", func() {
		deploymentObject := &unstructuredDeployment{
			name:      "checkout",
			namespace: "payments",
		}
		scheme := runtime.NewScheme()
		Expect(metav1.AddMetaToScheme(scheme)).To(Succeed())
		client := metadatafake.NewSimpleMetadataClient(scheme, &metav1.PartialObjectMetadata{
			TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
			ObjectMeta: metav1.ObjectMeta{
				Name: deploymentObject.name, Namespace: deploymentObject.namespace,
				Labels: map[string]string{"kubernaut.ai/managed": "true"},
			},
		})
		localReader, err := policyread.NewLocalInventoryReader(policyread.LocalInventoryConfig{
			MetadataClient: client,
			ScopeResolver:  interimRouterScopeResolver{},
			Now:            func() time.Time { return time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC) },
			Kinds: []policyread.LocalResourceKind{{
				Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment", Namespaced: true,
			}},
		})
		Expect(err).NotTo(HaveOccurred())
		service, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			LocalInventory: localReader,
			Authorizer:     interimRouterAuthorizer{},
		})
		Expect(err).NotTo(HaveOccurred())

		router, err := handler.NewRouter(handler.RouterConfig{
			MetricsRegistry:  metrics.NewRegistry(),
			A2AHandler:       http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
			MCPHandler:       http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
			AgentCardHandler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
			AuthMiddleware:   func(next http.Handler) http.Handler { return next },
			ReadyChecker:     func() bool { return true },
			ConsolePolicyReadHandler: handler.NewConsolePolicyReadHandler(
				service, logr.Discard(),
			),
		})
		Expect(err).NotTo(HaveOccurred())

		req := httptest.NewRequest(http.MethodGet, "/api/v1/console/inventory/managed-resources?cluster_id=local&limit=1", http.NoBody)
		req = req.WithContext(auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "alice"}))
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, req)

		Expect(recorder.Code).To(Equal(http.StatusOK))
		Expect(recorder.Body.String()).To(ContainSubstring("checkout"))
		Expect(recorder.Body.String()).To(ContainSubstring(`"observed_at"`))
		Expect(recorder.Body.String()).To(ContainSubstring(`"freshness"`))
	})

	It("IT-AF-2592-PUBLIC-002 [BR-API-001]: exposes policy-scope observation and freshness metadata", func() {
		page := inventory.Page{
			State:        inventory.StateEmpty,
			ObservedAt:   time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC),
			Freshness:    inventory.Freshness{State: inventory.FreshnessFresh, AsOf: time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)},
			Completeness: inventory.CompletenessComplete,
		}
		readHandler := newConsoleReadHandler(&consoleAuthorizer{allow: true}, &consoleInventoryStub{page: page})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/console/policies/scope?cluster_id=local", http.NoBody)
		req = req.WithContext(auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "alice"}))
		recorder := httptest.NewRecorder()

		readHandler.ServeHTTP(recorder, req)

		Expect(recorder.Code).To(Equal(http.StatusOK))
		Expect(recorder.Body.String()).To(ContainSubstring(`"observedAt"`))
		Expect(recorder.Body.String()).To(ContainSubstring(`"freshness"`))
	})
})

type unstructuredDeployment struct {
	name      string
	namespace string
}

type interimRouterScopeResolver struct{}

func (interimRouterScopeResolver) Resolve(context.Context, scope.ResourceIdentity) (scope.Resolution, error) {
	return scope.Resolution{
		Included:       true,
		ResourceLabel:  scope.LabelStateTrue,
		NamespaceLabel: scope.LabelStateTrue,
		Source:         scope.ScopeSourceResourceLabel,
	}, nil
}

type interimRouterAuthorizer struct{}

func (interimRouterAuthorizer) Check(context.Context, string, []string, string) (bool, error) {
	return true, nil
}

func (interimRouterAuthorizer) CheckTarget(context.Context, string, []string, auth.TargetAccess) (bool, error) {
	return true, nil
}

var _ policyread.ScopeResolver = interimRouterScopeResolver{}
var _ policyread.Authorizer = interimRouterAuthorizer{}
