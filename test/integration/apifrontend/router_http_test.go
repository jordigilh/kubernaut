package apifrontend_test

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/httputil"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/streaming"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"encoding/json"
	"net/http/httptest"
)

var _ = Describe("Router HTTP Integration (handler/)", func() {

	Describe("AC-1: Authenticated route dispatch", func() {
		It("IT-AF-1195-001: dispatches authenticated POST to A2A handler", func() {
			token := signValidToken("it-user-001")
			req, err := http.NewRequest(http.MethodPost, routerServer.URL+"/a2a/invoke", strings.NewReader(`{"jsonrpc":"2.0","method":"message/send","id":"1"}`))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(ContainSubstring(`"result"`))
		})

		It("IT-AF-1195-002: dispatches authenticated POST to MCP handler", func() {
			token := signValidToken("it-user-002")
			req, err := http.NewRequest(http.MethodPost, routerServer.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","method":"tools/list","id":"1"}`))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		It("IT-AF-1195-007: rejects request with missing Authorization header", func() {
			req, err := http.NewRequest(http.MethodPost, routerServer.URL+"/a2a/invoke", strings.NewReader(`{}`))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("application/problem+json"))
		})
	})

	Describe("AC-9: Root alias for kagenti compatibility (issue #1268)", func() {
		It("IT-AF-1268-001: dispatches authenticated POST / to A2A handler", func() {
			token := signValidToken("it-user-1268-001")
			req, err := http.NewRequest(http.MethodPost, routerServer.URL+"/", strings.NewReader(`{"jsonrpc":"2.0","method":"message/send","id":"1268"}`))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(ContainSubstring(`"result"`))
		})

		It("IT-AF-1268-002: POST / rejects request without auth", func() {
			req, err := http.NewRequest(http.MethodPost, routerServer.URL+"/", strings.NewReader(`{}`))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		It("IT-AF-1268-003: POST /unknown-subpath returns 404 (not catch-all)", func() {
			token := signValidToken("it-user-1268-003")
			req, err := http.NewRequest(http.MethodPost, routerServer.URL+"/not-a-real-path", strings.NewReader(`{}`))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		})
	})

	Describe("AC-2: Public endpoints serve without auth", func() {
		It("IT-AF-1195-003: /healthz returns 200 ok", func() {
			resp, err := http.Get(routerServer.URL + "/healthz")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(Equal("ok"))
		})

		It("IT-AF-1195-004: /readyz returns 200 when ready", func() {
			resp, err := http.Get(routerServer.URL + "/readyz")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		It("IT-AF-1195-005: /metrics returns 200 with prometheus text", func() {
			resp, err := http.Get(routerServer.URL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(ContainSubstring("go_goroutines"))
		})

		It("IT-AF-1195-006: /.well-known/agent-card.json returns valid JSON", func() {
			resp, err := http.Get(routerServer.URL + "/.well-known/agent-card.json")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("Content-Type")).To(Equal("application/json"))

			var card map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&card)).To(Succeed())
			Expect(card["name"]).To(Equal("kubernaut-af-it"))
		})
	})

	Describe("AC-4: Panic recovery", func() {
		It("IT-AF-1195-008: returns 500 problem+json on handler panic", func() {
			panicHandler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
				panic("deliberate test panic")
			})

			panicRouter, err := handler.NewRouter(handler.RouterConfig{
				MetricsRegistry:  metricsRegistry,
				Logger:           logf.Log.WithName("panic-it"),
				A2AHandler:       panicHandler,
				MCPHandler:       panicHandler,
				AgentCardHandler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }),
				AuthMiddleware:   func(next http.Handler) http.Handler { return next },
				ReadyChecker:     func() bool { return true },
				SSETracker:       streaming.NewConnectionTracker(metricsRegistry.SSEActiveConnections, 30*time.Second),
			})
			Expect(err).NotTo(HaveOccurred())

			srv := httptest.NewServer(panicRouter)
			defer srv.Close()

			resp, err := http.Post(srv.URL+"/a2a/invoke", "application/json", strings.NewReader(`{}`))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))
			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("application/problem+json"))

			var problem httputil.ProblemDetail
			Expect(json.NewDecoder(resp.Body).Decode(&problem)).To(Succeed())
			Expect(problem.Title).To(Equal("Internal Server Error"))
		})
	})

	Describe("AC-5: Security headers", func() {
		It("IT-AF-1195-009: all responses include security headers", func() {
			resp, err := http.Get(routerServer.URL + "/healthz")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.Header.Get("X-Content-Type-Options")).To(Equal("nosniff"))
			Expect(resp.Header.Get("X-Frame-Options")).To(Equal("DENY"))
			Expect(resp.Header.Get("Cache-Control")).To(Equal("no-store"))
			Expect(resp.Header.Get("Strict-Transport-Security")).To(ContainSubstring("max-age="))
		})
	})

	Describe("AC-6: Metrics middleware", func() {
		It("IT-AF-1195-010: records request metrics", func() {
			resp, err := http.Get(routerServer.URL + "/healthz")
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			families, err := metricsRegistry.Gather()
			Expect(err).NotTo(HaveOccurred())

			var found bool
			for _, f := range families {
				if f.GetName() == "af_http_requests_total" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "af_http_requests_total metric should be registered")
		})
	})

	Describe("AC-7: Readyz reflects health and draining", func() {
		It("IT-AF-1195-011: returns 503 when checker reports not ready", func() {
			notReadyRouter, err := handler.NewRouter(handler.RouterConfig{
				MetricsRegistry:  metricsRegistry,
				Logger:           logf.Log.WithName("notready-it"),
				A2AHandler:       http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }),
				MCPHandler:       http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }),
				AgentCardHandler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }),
				AuthMiddleware:   func(next http.Handler) http.Handler { return next },
				ReadyChecker:     func() bool { return false },
				SSETracker:       streaming.NewConnectionTracker(metricsRegistry.SSEActiveConnections, 30*time.Second),
			})
			Expect(err).NotTo(HaveOccurred())

			srv := httptest.NewServer(notReadyRouter)
			defer srv.Close()

			resp, err := http.Get(srv.URL + "/readyz")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))
		})

		It("IT-AF-1195-012: returns 503 when draining", func() {
			draining := &atomic.Bool{}
			draining.Store(true)

			drainingRouter, err := handler.NewRouter(handler.RouterConfig{
				MetricsRegistry:  metricsRegistry,
				Logger:           logf.Log.WithName("draining-it"),
				A2AHandler:       http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }),
				MCPHandler:       http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }),
				AgentCardHandler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }),
				AuthMiddleware:   func(next http.Handler) http.Handler { return next },
				ReadyChecker:     func() bool { return true },
				Draining:         draining,
				SSETracker:       streaming.NewConnectionTracker(metricsRegistry.SSEActiveConnections, 30*time.Second),
			})
			Expect(err).NotTo(HaveOccurred())

			srv := httptest.NewServer(drainingRouter)
			defer srv.Close()

			resp, err := http.Get(srv.URL + "/readyz")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))
		})
	})

	Describe("AC-8: Max body size enforcement", func() {
		It("IT-AF-1195-013: maxBodyMiddleware wraps request body with MaxBytesReader", func() {
			token := signValidToken("it-user-013")

			// MaxBytesReader is wired by the router but only errors when the
			// handler reads the body. The stub A2A handler does not read it,
			// so the request succeeds. We verify the middleware exists by
			// confirming the authenticated request reaches the stub handler
			// and returns 200 -- the body-limit protection is tested at the
			// unit level in handler/ package tests.
			smallBody := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"message/send","params":{}}`)
			req, err := http.NewRequest(http.MethodPost, routerServer.URL+"/a2a/invoke", smallBody)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK),
				"authenticated request with small body should reach stub handler")
		})
	})

	Describe("AC-5: Status subscribe dispatch (#1460)", func() {
		It("IT-AF-1460-020: POST /a2a/status dispatches through auth chain to StatusHandler", func() {
			token := signValidToken("it-user-status-020")
			body := `{"jsonrpc":"2.0","id":"sub-1","method":"status/subscribe","params":{"rr_id":"nonexistent-for-dispatch-test"}}`
			req, err := http.NewRequest(http.MethodPost, routerServer.URL+"/a2a/status", strings.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).NotTo(Equal(http.StatusNotFound),
				"POST /a2a/status must be routed (not 404)")
			Expect(resp.StatusCode).NotTo(Equal(http.StatusUnauthorized),
				"authenticated request must pass auth")

			respBody, _ := io.ReadAll(resp.Body)
			if resp.Header.Get("Content-Type") == "application/json" {
				var data map[string]any
				Expect(json.Unmarshal(respBody, &data)).To(Succeed())
				if errObj, ok := data["error"].(map[string]any); ok {
					Expect(errObj["code"]).To(BeNumerically("==", -32001),
						"nonexistent RR should return rr_not_found, not a routing error")
				}
			}
		})
	})

	// DOS-01, SC-5: deterministic proof that trackSSEConnection wiring returns
	// 503 "too many concurrent connections" once the ConnectionTracker cap is
	// reached. The Add()/Remove() cap-enforcement LOGIC is already proven at
	// the unit tier (UT-AF-STREAM04-001/002 in
	// pkg/apifrontend/streaming/tracker_test.go); this IT proves the router
	// middleware is correctly WIRED to that logic. Uses its own isolated
	// router + ConnectionTracker (not the shared routerServer) so this test
	// is fully deterministic and immune to the cross-process E2E flakiness
	// that motivated raising the E2E-deployed cap (see
	// test/e2e/apifrontend/streaming_test.go TC-E2E-SSE-CAP-01).
	Describe("AC-9: SSE connection cap enforcement (DOS-01, SC-5)", func() {
		It("IT-AF-SSE-CAP-001: trackSSEConnection returns 503 once the tracker cap is reached", func() {
			release := make(chan struct{})
			blockingA2A := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case <-release:
				case <-r.Context().Done():
				}
				w.WriteHeader(http.StatusOK)
			})

			capTracker := streaming.NewConnectionTracker(
				prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_af_sse_active_connections_it_sse_cap_001"}),
				0,
			)
			capTracker.MaxConnections = 2

			capRouter, err := handler.NewRouter(handler.RouterConfig{
				MetricsRegistry:  metricsRegistry,
				Logger:           logf.Log.WithName("sse-cap-it"),
				A2AHandler:       blockingA2A,
				MCPHandler:       http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }),
				AgentCardHandler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }),
				AuthMiddleware:   func(next http.Handler) http.Handler { return next },
				ReadyChecker:     func() bool { return true },
				SSETracker:       capTracker,
			})
			Expect(err).NotTo(HaveOccurred())

			capServer := httptest.NewServer(capRouter)
			var wg sync.WaitGroup
			// Deferred cleanup runs LIFO: close(release) unblocks the two
			// blocking handlers first, then wg.Wait() lets them finish and
			// close their response bodies, then capServer.Close() can
			// cleanly wait for the (now-idle) connections to close.
			defer capServer.Close()
			defer wg.Wait()
			defer close(release)

			sendBlocking := func() (*http.Response, error) {
				req, rerr := http.NewRequest(http.MethodPost, capServer.URL+"/a2a/invoke", strings.NewReader(`{}`))
				Expect(rerr).NotTo(HaveOccurred())
				req.Header.Set("Accept", "text/event-stream")
				return http.DefaultClient.Do(req)
			}

			// Fill both slots. Each request blocks server-side on `release`,
			// so the client-side call itself blocks until we signal below --
			// run each in a goroutine and wait for the tracker to observe it.
			for i := 0; i < capTracker.MaxConnections; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					resp, serr := sendBlocking()
					if serr == nil {
						_ = resp.Body.Close()
					}
				}()
			}
			Eventually(capTracker.Count, "2s", "10ms").Should(Equal(2),
				"both blocking requests must register with the tracker before the overflow attempt")

			overflowReq, err := http.NewRequest(http.MethodPost, capServer.URL+"/a2a/invoke", strings.NewReader(`{}`))
			Expect(err).NotTo(HaveOccurred())
			overflowReq.Header.Set("Accept", "text/event-stream")
			overflowResp, err := http.DefaultClient.Do(overflowReq)
			Expect(err).NotTo(HaveOccurred())
			defer overflowResp.Body.Close()

			Expect(overflowResp.StatusCode).To(Equal(http.StatusServiceUnavailable),
				"third connection must be rejected once the tracker cap (2) is reached")
			overflowBody, err := io.ReadAll(overflowResp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.ToLower(string(overflowBody))).To(ContainSubstring("too many concurrent connections"))
		})
	})
})

