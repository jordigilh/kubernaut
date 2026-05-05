package handler_test

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/handler"
	"github.com/jordigilh/kubernaut-apifrontend/internal/metrics"
)

var _ = Describe("Router", func() {
	var (
		router     http.Handler
		metricsReg *metrics.Registry
	)

	BeforeEach(func() {
		metricsReg = metrics.NewRegistry()
		var err error
		router, err = handler.NewRouter(handler.RouterConfig{
			MetricsRegistry:  metricsReg,
			A2AHandler:       http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }),
			MCPHandler:       http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }),
			AgentCardHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }),
			AuthMiddleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("Authorization") == "" {
						http.Error(w, "unauthorized", http.StatusUnauthorized)
						return
					}
					next.ServeHTTP(w, r)
				})
			},
			ReadyChecker: func() bool { return true },
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("UT-AF-200-001: NewRouter returns mux with all expected routes", func() {
		routes := []string{"/healthz", "/readyz", "/metrics", "/a2a/invoke", "/mcp", "/.well-known/agent-card.json"}
		for _, path := range routes {
			req := httptest.NewRequest("GET", path, http.NoBody)
			req.Header.Set("Authorization", "Bearer test-token")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			Expect(rec.Code).NotTo(Equal(http.StatusNotFound), "route %s should exist", path)
		}
	})

	It("UT-AF-200-002: /healthz returns 200 ok without auth", func() {
		req := httptest.NewRequest("GET", "/healthz", http.NoBody)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Body.String()).To(Equal("ok"))
	})

	It("UT-AF-200-003: /readyz returns 503 when checker reports not ready", func() {
		notReadyRouter, err := handler.NewRouter(handler.RouterConfig{
			MetricsRegistry:  metricsReg,
			A2AHandler:       http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
			MCPHandler:       http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
			AgentCardHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
			AuthMiddleware:   func(next http.Handler) http.Handler { return next },
			ReadyChecker:     func() bool { return false },
		})
		Expect(err).NotTo(HaveOccurred())

		req := httptest.NewRequest("GET", "/readyz", http.NoBody)
		rec := httptest.NewRecorder()
		notReadyRouter.ServeHTTP(rec, req)
		Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
	})

	It("UT-AF-200-004: /metrics returns Prometheus scrape without auth", func() {
		req := httptest.NewRequest("GET", "/metrics", http.NoBody)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Body.String()).To(ContainSubstring("go_goroutines"))
	})

	It("UT-AF-200-005: /a2a/invoke returns 401 without bearer token", func() {
		req := httptest.NewRequest("POST", "/a2a/invoke", http.NoBody)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		Expect(rec.Code).To(Equal(http.StatusUnauthorized))
	})

	It("UT-AF-200-006: /mcp returns 401 without bearer token", func() {
		req := httptest.NewRequest("POST", "/mcp", http.NoBody)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		Expect(rec.Code).To(Equal(http.StatusUnauthorized))
	})

	It("UT-AF-200-007: /.well-known/agent-card.json returns 200 without auth", func() {
		req := httptest.NewRequest("GET", "/.well-known/agent-card.json", http.NoBody)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		Expect(rec.Code).To(Equal(http.StatusOK))
	})

	It("UT-AF-200-008: HTTP metrics middleware increments af_http_requests_total", func() {
		req := httptest.NewRequest("GET", "/healthz", http.NoBody)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		metricsReq := httptest.NewRequest("GET", "/metrics", http.NoBody)
		metricsRec := httptest.NewRecorder()
		router.ServeHTTP(metricsRec, metricsReq)
		Expect(metricsRec.Body.String()).To(ContainSubstring("af_http_requests_total"))
	})

	It("UT-AF-200-009: HTTP metrics middleware records af_http_request_duration_seconds", func() {
		req := httptest.NewRequest("GET", "/healthz", http.NoBody)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		metricsReq := httptest.NewRequest("GET", "/metrics", http.NoBody)
		metricsRec := httptest.NewRecorder()
		router.ServeHTTP(metricsRec, metricsReq)
		Expect(metricsRec.Body.String()).To(ContainSubstring("af_http_request_duration_seconds"))
	})

	It("UT-AF-200-010: Unknown path returns 404", func() {
		req := httptest.NewRequest("GET", "/nonexistent/path", http.NoBody)
		req.Header.Set("Authorization", "Bearer test-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		Expect(rec.Code).To(Equal(http.StatusNotFound))
	})
})
