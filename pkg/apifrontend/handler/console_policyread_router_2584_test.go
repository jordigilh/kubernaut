package handler_test

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/metrics"
)

var _ = Describe("IT-AF-2584-001/002/005: Console read router wiring", func() {
	It("registers all three Console routes in the authenticated middleware tier", func() {
		consoleHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
		router, err := handler.NewRouter(handler.RouterConfig{
			MetricsRegistry: metrics.NewRegistry(),
			A2AHandler:      http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
			MCPHandler:      http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
			PublicRoutes: handler.PublicRouteConfig{
				AgentCardHandler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
				BackendContract:  handler.BackendContractConfig{Incarnation: testBackendIncarnation},
			},
			AuthMiddleware:           func(next http.Handler) http.Handler { return next },
			ReadyChecker:             func() bool { return true },
			ConsolePolicyReadHandler: consoleHandler,
		})
		Expect(err).NotTo(HaveOccurred())

		for _, path := range []string{
			"/api/v1/console/policies/scope?cluster_id=local",
			"/api/v1/console/inventory/managed-resources?cluster_id=local",
			"/api/v1/console/inventory/managed-resources/r-opaque?cluster_id=local",
		} {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, http.NoBody))
			Expect(recorder.Code).To(Equal(http.StatusNoContent), "route %s should dispatch", path)
		}
	})

	It("keeps the Console routes behind the configured authentication middleware", func() {
		var authCalls int
		router, err := handler.NewRouter(handler.RouterConfig{
			MetricsRegistry: metrics.NewRegistry(),
			A2AHandler:      http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
			MCPHandler:      http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
			PublicRoutes: handler.PublicRouteConfig{
				AgentCardHandler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
				BackendContract:  handler.BackendContractConfig{Incarnation: testBackendIncarnation},
			},
			AuthMiddleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					authCalls++
					next.ServeHTTP(w, r)
				})
			},
			ReadyChecker:             func() bool { return true },
			ConsolePolicyReadHandler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
		})
		Expect(err).NotTo(HaveOccurred())

		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/console/inventory/managed-resources?cluster_id=local", http.NoBody))
		Expect(recorder.Code).To(Equal(http.StatusOK))
		Expect(authCalls).To(Equal(1))
	})
})
