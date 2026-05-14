package e2e_test

import (
	"io"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// metricsBaseURL is the E2E metrics endpoint. In Kind with port-forward this is
// typically the same host as baseURL but on the API port since the E2E deployment
// exposes /metrics on the main mux (port 8443), not on a separate metrics port.
// If a separate metrics port-forward is available, override via env.
func metricsURL() string {
	u := getEnvOrDefault("AF_E2E_METRICS_URL", "")
	if u != "" {
		return u
	}
	return baseURL + "/metrics"
}

// healthBaseURL is the E2E health endpoint port. In Kind this maps to 8081.
func healthURL() string {
	return getEnvOrDefault("AF_E2E_HEALTH_URL", "http://localhost:18081")
}

func scrapeMetrics() string {
	resp, err := httpClient.Get(metricsURL())
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	defer func() { _ = resp.Body.Close() }()
	ExpectWithOffset(1, resp.StatusCode).To(Equal(http.StatusOK))
	body, err := io.ReadAll(resp.Body)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return string(body)
}

var _ = Describe("Operational Contract", Ordered, Label("e2e", "phase1", "operational"), func() {

	// -----------------------------------------------------------------------
	// TC-A-01e: /readyz on health port must be dependency-aware
	// -----------------------------------------------------------------------
	Context("Readiness Probe Semantics (WIRE-01)", func() {
		It("TC-A-01e: /readyz on health port should include dependency status", func() {
			// The health port readyz must reflect dependency health, not just
			// return static {"status":"ready"}.
			healthClient := &http.Client{}
			resp, err := healthClient.Get(healthURL() + "/readyz")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			// When deps are unavailable (KA not deployed in E2E), the probe
			// should return 503 with a problem+json or structured response.
			// Current behavior: always returns 200 {"status":"ready"} — this
			// test should FAIL on current code.
			if resp.StatusCode == http.StatusOK {
				// If 200, verify it's not the static response
				Expect(string(body)).NotTo(Equal(`{"status":"ready"}`),
					"readyz should be dependency-aware, not static")
			}
		})
	})

	// -----------------------------------------------------------------------
	// TC-A-metrics: Metrics emission after authenticated request
	// -----------------------------------------------------------------------
	Context("Metrics Emission (WIRE-05/06/08)", func() {
		var token string

		BeforeAll(func() {
			var err error
			token, err = fetchDEXToken(dexURL, clientID, clientSecret, username, password)
			Expect(err).NotTo(HaveOccurred())

			// Make an authenticated request to generate metrics
			body := strings.NewReader(`{"jsonrpc":"2.0","method":"initialize","id":"1","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"e2e","version":"1.0"}}}`)
			req, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", body)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := httpClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			_ = resp.Body.Close()
		})

		It("TC-A-metrics-01: should emit af_http_requests_total with status/method/path labels", func() {
			body := scrapeMetrics()
			Expect(body).To(ContainSubstring("af_http_requests_total"),
				"af_http_requests_total metric not found in /metrics")
			Expect(body).To(MatchRegexp(`af_http_requests_total\{.*status=`),
				"af_http_requests_total missing status label")
		})

		It("TC-A-metrics-02: should emit af_circuit_breaker_state with dependency=ka", func() {
			body := scrapeMetrics()
			Expect(body).To(MatchRegexp(`af_circuit_breaker_state\{[^}]*dependency="ka"`),
				"af_circuit_breaker_state{dependency=\"ka\"} not found — WIRE-05 KA metrics not wired")
		})

		It("TC-A-metrics-03: should emit af_circuit_breaker_state with dependency=ds", func() {
			body := scrapeMetrics()
			Expect(body).To(MatchRegexp(`af_circuit_breaker_state\{[^}]*dependency="ds"`),
				"af_circuit_breaker_state{dependency=\"ds\"} not found — WIRE-06 DependencyName not set")
		})

		It("TC-A-metrics-04: should emit af_downstream_request_duration_seconds with dependency label", func() {
			body := scrapeMetrics()
			Expect(body).To(MatchRegexp(`af_downstream_request_duration_seconds_bucket\{[^}]*dependency="(ka|ds)"`),
				"af_downstream_request_duration_seconds missing dependency label")
		})
	})

	// -----------------------------------------------------------------------
	// TC-A-auth-metrics: Auth duration metric
	// -----------------------------------------------------------------------
	Context("Auth Metrics (WIRE-08)", func() {
		It("TC-A-auth-01: should emit af_auth_duration_seconds after authenticated request", func() {
			body := scrapeMetrics()
			Expect(body).To(ContainSubstring("af_auth_duration_seconds"),
				"af_auth_duration_seconds metric not found — auth middleware metrics not wired")
		})
	})
})
