package handler_test

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/metrics"
)

const testBackendIncarnation = "01234567-89ab-4cde-8123-456789abcdef"

func backendContractRouterConfig(incarnation string) handler.RouterConfig {
	passAuth := func(next http.Handler) http.Handler { return next }
	return handler.RouterConfig{
		MetricsRegistry: metrics.NewRegistry(),
		A2AHandler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{}}`))
		}),
		MCPHandler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{}}`))
		}),
		PublicRoutes: handler.PublicRouteConfig{
			AgentCardHandler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}),
			BackendContract: handler.BackendContractConfig{Incarnation: incarnation},
		},
		AuthMiddleware: passAuth,
		ReadyChecker:   func() bool { return true },
	}
}

var _ = Describe("backend incarnation router contract", func() {
	It("UT-AF-2614-006: rejects missing or malformed direct router configuration", func() {
		for _, value := range []string{"", "01234567-89ab-3cde-8123-456789abcdef", "01234567-89ab-4cde-8123-456789abcde\r"} {
			router, err := handler.NewRouter(backendContractRouterConfig(value))
			Expect(router).To(BeNil())
			Expect(err).To(MatchError(ContainSubstring("backend incarnation")))
		}
	})

	It("IT-AF-2614-001/006: serves the public contract and authenticated lifecycle header", func() {
		router, err := handler.NewRouter(backendContractRouterConfig(testBackendIncarnation))
		Expect(err).NotTo(HaveOccurred())

		public := httptest.NewRecorder()
		router.ServeHTTP(public, httptest.NewRequest(http.MethodGet, "/.well-known/kubernaut-console-contract.json", http.NoBody))
		Expect(public.Code).To(Equal(http.StatusOK))
		Expect(public.Body.String()).To(Equal(`{"backend_incarnation":"` + testBackendIncarnation + `"}`))
		Expect(public.Header().Get("X-Kubernaut-Backend-Incarnation")).To(BeEmpty())

		invoke := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/a2a/invoke", http.NoBody)
		// The test auth middleware is intentionally pass-through; the route
		// boundary is proven here and real auth remains covered by IT fixtures.
		router.ServeHTTP(invoke, req)
		Expect(invoke.Header().Get("X-Kubernaut-Backend-Incarnation")).To(Equal(testBackendIncarnation))
	})

	It("IT-AF-2614-008/009: places the lifecycle boundary after auth and before post-auth outcomes", func() {
		postAuth := func(_ http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusTooManyRequests)
			})
		}
		postAuthConfig := backendContractRouterConfig(testBackendIncarnation)
		postAuthConfig.PostAuthMiddleware = postAuth
		router, err := handler.NewRouter(postAuthConfig)
		Expect(err).NotTo(HaveOccurred())

		postAuthResponse := httptest.NewRecorder()
		router.ServeHTTP(postAuthResponse, httptest.NewRequest(http.MethodPost, "/a2a/invoke", http.NoBody))
		Expect(postAuthResponse.Code).To(Equal(http.StatusTooManyRequests))
		Expect(postAuthResponse.Header().Get("X-Kubernaut-Backend-Incarnation")).To(Equal(testBackendIncarnation))

		preAuthConfig := backendContractRouterConfig(testBackendIncarnation)
		preAuthConfig.PreAuthMiddleware = func(_ http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
			})
		}
		preAuthRouter, err := handler.NewRouter(preAuthConfig)
		Expect(err).NotTo(HaveOccurred())

		preAuthResponse := httptest.NewRecorder()
		preAuthRouter.ServeHTTP(preAuthResponse, httptest.NewRequest(http.MethodPost, "/a2a/invoke", http.NoBody))
		Expect(preAuthResponse.Code).To(Equal(http.StatusTooManyRequests))
		Expect(preAuthResponse.Header().Get("Retry-After")).To(Equal("1"))
		Expect(preAuthResponse.Header().Get("X-Kubernaut-Backend-Incarnation")).To(BeEmpty())
	})

	It("IT-AF-2614-008: excludes public and unrelated authenticated routes", func() {
		config := backendContractRouterConfig(testBackendIncarnation)
		config.ConsoleAccessHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		router, err := handler.NewRouter(config)
		Expect(err).NotTo(HaveOccurred())

		for _, request := range []*http.Request{
			httptest.NewRequest(http.MethodGet, "/healthz", http.NoBody),
			httptest.NewRequest(http.MethodGet, "/.well-known/agent-card.json", http.NoBody),
			httptest.NewRequest(http.MethodGet, "/a2a/access", http.NoBody),
		} {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			Expect(response.Header().Get("X-Kubernaut-Backend-Incarnation")).To(BeEmpty(), request.URL.Path)
		}
	})
})
