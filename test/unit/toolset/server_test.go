package toolset

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/jordigilh/kubernaut/pkg/toolset/server"
)

// DD-TOOLSET-001: REST API Deprecation
// =====================================
// This test file has been simplified to only test the remaining active endpoints:
// - /health (health check)
// - /ready (readiness probe)
// - /metrics (Prometheus metrics)
//
// All REST API endpoint tests (GET /api/v1/toolsets, POST /api/v1/discover, etc.)
// have been removed as these endpoints were deprecated per DD-TOOLSET-001.
//
// See: docs/architecture/decisions/DD-TOOLSET-001-REST-API-Deprecation.md
// =====================================

var _ = Describe("BR-TOOLSET-033: HTTP Server", func() {
	var (
		srv        *server.Server
		fakeClient *fake.Clientset
	)

	BeforeEach(func() {
		fakeClient = fake.NewSimpleClientset()

		// Create server with test configuration
		config := &server.Config{
			Port:              8080,
			MetricsPort:       9090,
			ShutdownTimeout:   1 * time.Second,
			DiscoveryInterval: 5 * time.Minute,
		}

		var err error
		srv, err = server.NewServer(config, fakeClient)
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("Health Endpoints", func() {
		Context("GET /health", func() {
			It("should return 200 OK", func() {
				req := httptest.NewRequest("GET", "/health", nil)
				rr := httptest.NewRecorder()

				srv.ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusOK))
				Expect(rr.Body.String()).To(ContainSubstring("ok"))
			})

			It("should include service status", func() {
				req := httptest.NewRequest("GET", "/health", nil)
				rr := httptest.NewRecorder()

				srv.ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusOK))

				var response map[string]interface{}
				err := json.Unmarshal(rr.Body.Bytes(), &response)
				Expect(err).ToNot(HaveOccurred())

				Expect(response["status"]).To(Equal("ok"))
			})
		})

		Context("GET /ready", func() {
			It("should return 200 OK when ready", func() {
				req := httptest.NewRequest("GET", "/ready", nil)
				rr := httptest.NewRecorder()

				srv.ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusOK))
			})

			It("should include kubernetes readiness status", func() {
				req := httptest.NewRequest("GET", "/ready", nil)
				rr := httptest.NewRecorder()

				srv.ServeHTTP(rr, req)

				var response map[string]interface{}
				err := json.Unmarshal(rr.Body.Bytes(), &response)
				Expect(err).ToNot(HaveOccurred())

				Expect(response).To(HaveKey("kubernetes"))
				Expect(response["kubernetes"]).To(BeTrue())
			})
		})
	})

	Describe("Metrics Endpoint", func() {
		Context("GET /metrics", func() {
			It("should return Prometheus metrics", func() {
				req := httptest.NewRequest("GET", "/metrics", nil)
				rr := httptest.NewRecorder()

				srv.ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusOK))
				Expect(rr.Header().Get("Content-Type")).To(ContainSubstring("text/plain"))
			})
		})
	})
})
