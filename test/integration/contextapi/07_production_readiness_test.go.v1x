package contextapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/jordigilh/kubernaut/pkg/contextapi/metrics"
	"github.com/jordigilh/kubernaut/pkg/contextapi/server"
)

var _ = Describe("Production Readiness Integration Tests", func() {
	Context("Metrics Validation", func() {
		It("should expose metrics endpoint with Prometheus format", func() {
			// Day 9 Suite 1 - Test #1 (DO-GREEN Phase - Pure TDD)
			// BR-CONTEXT-006: Observability - Metrics exposure
			//
			// ✅ Pure TDD: Validate metrics endpoint is operational
			// Expected: /metrics endpoint returns valid Prometheus format
			// Note: Specific metric names tested in unit tests

			testServer, _ := createTestServerForProduction()
			defer testServer.Close()

			// Query metrics endpoint
			resp, err := http.Get(testServer.URL + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK),
				"Metrics endpoint should return 200 OK")

			// Read metrics body
			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			metricsOutput := string(body)

			// ✅ Business Value Assertion: Metrics endpoint is operational
			Expect(len(metricsOutput)).To(BeNumerically(">", 0),
				"Metrics output should not be empty")

			// Validate Prometheus format (should have HELP and TYPE comments)
			Expect(metricsOutput).To(ContainSubstring("# HELP"),
				"Metrics should include Prometheus HELP comments")
			Expect(metricsOutput).To(ContainSubstring("# TYPE"),
				"Metrics should include Prometheus TYPE comments")

			// Validate some metrics are exposed (any metrics prove endpoint works)
			Expect(metricsOutput).To(ContainSubstring("_total"),
				"Should contain counter metrics")
			Expect(metricsOutput).To(ContainSubstring("cache"),
				"Should contain cache-related metrics")
		})

		It("should serve metrics consistently across requests", func() {
			// Day 9 Suite 1 - Test #2 (Validation Testing)
			// BR-CONTEXT-006: Metrics endpoint reliability
			//
			// ✅ Validates metrics endpoint responds consistently

			testServer, _ := createTestServerForProduction()
			defer testServer.Close()

			// Execute multiple metrics requests
			for i := 0; i < 3; i++ {
				resp, err := http.Get(testServer.URL + "/metrics")
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusOK),
					"Metrics endpoint should consistently return 200 OK")

				body, err := io.ReadAll(resp.Body)
				Expect(err).ToNot(HaveOccurred())

				Expect(len(body)).To(BeNumerically(">", 0),
					"Metrics should not be empty")
			}

			// ✅ Business Value Assertion: Metrics endpoint is reliable
		})
	})

	Context("Graceful Shutdown", func() {
		It("should shutdown cleanly with context timeout", func() {
			// Day 9 Suite 1 - Test #3 (Integration Testing)
			// BR-CONTEXT-007: Production Readiness - Graceful shutdown
			//
			// ✅ Tests that Shutdown method exists and completes

			// Create server
			_, srv := createTestServerForProduction()

			// Initiate graceful shutdown with timeout
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := srv.Shutdown(shutdownCtx)

			// ✅ Business Value Assertion: Shutdown completes successfully
			Expect(err).ToNot(HaveOccurred(),
				"Graceful shutdown should complete without errors")
		})
	})
})

// Helper Functions

// createTestServerForProduction creates test server for production readiness tests
// Uses Redis DB 6 for parallel test isolation
func createTestServerForProduction() (*httptest.Server, *server.Server) {
	// Create custom registry for this test instance
	customRegistry := prometheus.NewRegistry()

	// Create metrics with custom registry
	metricsInstance := metrics.NewMetricsWithRegistry("contextapi", "test", customRegistry)

	// Create server config
	cfg := &server.Config{
		Port:         8092,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Connection string for test infrastructure (Data Storage Service)
	// DD-SCHEMA-001: Connect to Data Storage Service database
	connStr := "host=localhost port=5432 user=slm_user password=slm_password_dev dbname=action_history sslmode=disable"

	// Use Redis DB 6 for production readiness tests (parallel test isolation)
	redisAddr := "localhost:6379/6"

	// Create server with custom metrics
	srv, err := server.NewServerWithMetrics(connStr, redisAddr, logger, cfg, metricsInstance)
	Expect(err).ToNot(HaveOccurred())

	return httptest.NewServer(srv.Handler()), srv
}
