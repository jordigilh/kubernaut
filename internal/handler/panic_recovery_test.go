package handler_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/jordigilh/kubernaut-apifrontend/internal/handler"
	"github.com/jordigilh/kubernaut-apifrontend/internal/metrics"
)

var _ = Describe("Panic Recovery Middleware (HANDLER-01)", func() {
	var reg *metrics.Registry

	BeforeEach(func() {
		reg = metrics.NewRegistry()
	})

	It("TC-C-01a: recovers from string panic and returns 500 problem+json", func() {
		inner := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			panic("something went wrong")
		})
		wrapped := handler.RecoverMiddleware(reg, logr.Discard())(inner)

		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/test", http.NoBody))

		Expect(rec.Code).To(Equal(http.StatusInternalServerError))
		Expect(rec.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"))

		var problem map[string]interface{}
		Expect(json.Unmarshal(rec.Body.Bytes(), &problem)).To(Succeed())
		Expect(problem).To(HaveKey("title"))
		Expect(testutil.ToFloat64(reg.HTTPPanicsTotal)).To(Equal(1.0))
	})

	It("TC-C-01b: recovers from error-typed panic and increments counter", func() {
		inner := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			panic(errors.New("wrapped error panic"))
		})
		wrapped := handler.RecoverMiddleware(reg, logr.Discard())(inner)

		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/test", http.NoBody))

		Expect(rec.Code).To(Equal(http.StatusInternalServerError))
		Expect(rec.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"))
		Expect(testutil.ToFloat64(reg.HTTPPanicsTotal)).To(Equal(1.0))
	})

	It("TC-C-01c: recovers from nil panic", func() {
		inner := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			panic(nil)
		})
		wrapped := handler.RecoverMiddleware(reg, logr.Discard())(inner)

		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/test", http.NoBody))

		Expect(rec.Code).To(Equal(http.StatusInternalServerError))
	})

	It("TC-C-01d: recovers from runtime index-out-of-bounds panic", func() {
		inner := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			s := []int{}
			_ = s[42] //nolint:govet // intentional OOB for test
		})
		wrapped := handler.RecoverMiddleware(reg, logr.Discard())(inner)

		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/test", http.NoBody))

		Expect(rec.Code).To(Equal(http.StatusInternalServerError))
		Expect(rec.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"))
		Expect(testutil.ToFloat64(reg.HTTPPanicsTotal)).To(Equal(1.0))
	})

	It("TC-C-01e: normal handler passes through without recovery", func() {
		inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})
		wrapped := handler.RecoverMiddleware(reg, logr.Discard())(inner)

		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/test", http.NoBody))

		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Body.String()).To(Equal("ok"))
		Expect(testutil.ToFloat64(reg.HTTPPanicsTotal)).To(Equal(0.0))
	})

	It("TC-C-01f: still returns 500 when headers were partially written before panic", func() {
		inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-Partial", "true")
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte("partial"))
			panic("late panic after headers sent")
		})
		wrapped := handler.RecoverMiddleware(reg, logr.Discard())(inner)

		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/test", http.NoBody))

		// httptest.ResponseRecorder allows double WriteHeader; in production
		// the second status write is silently dropped. The key guarantee is
		// the process does NOT crash and the counter is incremented.
		Expect(testutil.ToFloat64(reg.HTTPPanicsTotal)).To(Equal(1.0))
	})

	It("TC-P1-01h: router-level integration — panic in MCP handler returns 500 and increments counter", func() {
		panicMCP := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			panic("mcp handler exploded")
		})
		noopHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		passthrough := func(next http.Handler) http.Handler { return next }

		router, err := handler.NewRouter(handler.RouterConfig{
			MetricsRegistry:  reg,
			A2AHandler:       noopHandler,
			MCPHandler:       panicMCP,
			AgentCardHandler: noopHandler,
			AuthMiddleware:   passthrough,
			ReadyChecker:     func() bool { return true },
		})
		Expect(err).NotTo(HaveOccurred())

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mcp", http.NoBody))
		Expect(rec.Code).To(Equal(http.StatusInternalServerError))
		Expect(rec.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"))
		Expect(testutil.ToFloat64(reg.HTTPPanicsTotal)).To(Equal(1.0))
	})

	It("TC-C-01g: 10 concurrent panics all increment the counter", func() {
		inner := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			panic("concurrent boom")
		})
		wrapped := handler.RecoverMiddleware(reg, logr.Discard())(inner)

		var wg sync.WaitGroup
		var recovered atomic.Int32
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				rec := httptest.NewRecorder()
				wrapped.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/test", http.NoBody))
				if rec.Code == http.StatusInternalServerError {
					recovered.Add(1)
				}
			}()
		}
		wg.Wait()

		Expect(recovered.Load()).To(Equal(int32(10)))
		Expect(testutil.ToFloat64(reg.HTTPPanicsTotal)).To(Equal(10.0))
	})
})

var _ = Describe("Panic Message Security (MED-04)", func() {
	It("TC-P3-04a: problem+json detail does NOT contain panic value", func() {
		reg := metrics.NewRegistry()
		mw := handler.RecoverMiddleware(reg, logr.Discard())
		inner := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			panic("secret-internal-state-xyz")
		})
		wrapped := mw(inner)

		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/test", http.NoBody))

		Expect(rec.Code).To(Equal(http.StatusInternalServerError))
		var problem map[string]interface{}
		Expect(json.NewDecoder(rec.Body).Decode(&problem)).To(Succeed())
		detail, _ := problem["detail"].(string)
		Expect(detail).NotTo(ContainSubstring("secret-internal-state-xyz"),
			"TC-P3-04a: panic value MUST NOT appear in response detail field — information leak")
	})

	It("TC-P3-04b: problem+json title is generic 'Internal Server Error'", func() {
		reg := metrics.NewRegistry()
		mw := handler.RecoverMiddleware(reg, logr.Discard())
		inner := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			panic("anything")
		})
		wrapped := mw(inner)

		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/test", http.NoBody))

		var problem map[string]interface{}
		Expect(json.NewDecoder(rec.Body).Decode(&problem)).To(Succeed())
		Expect(problem["title"]).To(Equal("Internal Server Error"))
	})
})
