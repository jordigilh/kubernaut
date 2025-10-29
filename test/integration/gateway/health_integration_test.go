package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Health Endpoints Integration Tests", func() {
	var (
		ctx         context.Context
		testServer  *httptest.Server
		redisClient *RedisTestClient
		k8sClient   *K8sTestClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		redisClient = SetupRedisTestClient(ctx)
		k8sClient = SetupK8sTestClient(ctx)

		gatewayServer, err := StartTestGateway(ctx, redisClient, k8sClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Gateway server")
		testServer = httptest.NewServer(gatewayServer.Handler())
	})

	AfterEach(func() {
		if testServer != nil {
			testServer.Close()
		}
	})

	Context("BR-GATEWAY-024: Basic Health Endpoint", func() {
		It("should return 200 OK when all dependencies are healthy", func() {
			// DD-GATEWAY-004: K8s API health check removed (network-level security)
			// Health endpoint now only checks Redis connectivity

			// Act: Call /health endpoint
			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Get(testServer.URL + "/health")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Assert: Should return 200 with healthy status
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var health map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&health)
			Expect(err).ToNot(HaveOccurred())

			// Validate response structure per HEALTH_CHECK_STANDARD.md
			Expect(health["status"]).To(Equal("healthy"))
			Expect(health["timestamp"]).ToNot(BeEmpty())
		})
	})

	Context("BR-GATEWAY-024: Readiness Endpoint", func() {
		It("should return 200 OK when Gateway is ready to accept requests", func() {
			// DD-GATEWAY-004: K8s API readiness check removed (network-level security)
			// Readiness endpoint now only checks Redis connectivity

			// Act: Call /health/ready endpoint
			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Get(testServer.URL + "/health/ready")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Assert: Should return 200 with ready status
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var readiness map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&readiness)
			Expect(err).ToNot(HaveOccurred())

			// Validate response structure
			Expect(readiness["time"]).ToNot(BeEmpty())

			// DD-GATEWAY-004: Redis check + K8s marked as "not_applicable"
			Expect(readiness["status"]).To(Equal("ready"))
			Expect(readiness["redis"]).To(Equal("healthy"))
			Expect(readiness["kubernetes"]).To(Equal("not_applicable"))
		})
	})

	Context("BR-GATEWAY-024: Liveness Endpoint", func() {
		It("should return 200 OK for liveness probe", func() {
			// Liveness probe is simple - just checks if process is alive
			// No dependency checks needed

			// Act: Call /health/live endpoint
			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Get(testServer.URL + "/health/live")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Assert: Should return 200 OK
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})

	Context("Response Format Validation", func() {
		It("should return valid JSON for all health endpoints", func() {
			// Validate that all health endpoints return valid JSON

			endpoints := []string{
				"/health",
				"/health/ready",
			}

			client := &http.Client{Timeout: 10 * time.Second}

			for _, endpoint := range endpoints {
				resp, err := client.Get(testServer.URL + endpoint)
				Expect(err).ToNot(HaveOccurred(), "Should successfully call "+endpoint)
				defer resp.Body.Close()

				// Should return valid JSON
				var result map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&result)
				Expect(err).ToNot(HaveOccurred(), endpoint+" should return valid JSON")

				// Should have at least a time field
				Expect(result["time"]).ToNot(BeEmpty(), endpoint+" should include timestamp")
			}
		})
	})
})
