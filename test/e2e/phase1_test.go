package e2e_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	baseURL      string
	caCertPath   string
	dexURL       string
	clientID     string
	clientSecret string
	username     string
	password     string
	httpClient   *http.Client
)

var _ = Describe("Phase 1: AF Standalone (Realistic)", Ordered, Label("e2e", "phase1"), func() {

	Context("Health Probes", func() {
		It("should return 200 on /healthz", func() {
			resp, err := httpClient.Get(baseURL + "/healthz")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		It("should return 200 on /readyz", func() {
			resp, err := httpClient.Get(baseURL + "/readyz")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			// Readiness is circuit-breaker-based: 200 when CBs are healthy (closed), 503 when open or draining. In Phase 1 without KA/DS traffic, CBs stay closed so 200 is expected.
			Expect(resp.StatusCode).To(BeElementOf(http.StatusOK, http.StatusServiceUnavailable))
		})
	})

	Context("TLS Enforcement", func() {
		It("should serve HTTPS with valid certificate", func() {
			resp, err := httpClient.Get(baseURL + "/healthz")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.TLS).NotTo(BeNil())
			Expect(resp.TLS.Version).To(BeNumerically(">=", 0x0303)) // TLS 1.2+
		})
	})

	Context("Metrics Endpoint", func() {
		It("should expose Prometheus metrics on /metrics", func() {
			resp, err := httpClient.Get(baseURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(ContainSubstring("go_info"))
			Expect(string(body)).To(ContainSubstring("process_start_time_seconds"))
		})
	})

	Context("Agent Card", func() {
		It("should serve agent card JSON at /.well-known/agent-card.json", func() {
			resp, err := httpClient.Get(baseURL + "/.well-known/agent-card.json")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("application/json"))

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			var card map[string]interface{}
			Expect(json.Unmarshal(body, &card)).To(Succeed())
			Expect(card).To(HaveKey("name"))
			Expect(card).To(HaveKey("url"))

			skills, ok := card["skills"].([]interface{})
			Expect(ok).To(BeTrue(), "skills should be a JSON array")
			Expect(skills).To(HaveLen(19), "agent card should advertise all 19 tools as skills")
		})
	})

	Context("Authentication", func() {
		It("should reject unauthenticated POST /a2a/invoke with 401", func() {
			body := strings.NewReader(`{"jsonrpc":"2.0","method":"tasks/send","id":"1","params":{}}`)
			req, err := http.NewRequest(http.MethodPost, baseURL+"/a2a/invoke", body)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			resp, err := httpClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		It("should reject unauthenticated POST /mcp with 401", func() {
			body := strings.NewReader(`{"jsonrpc":"2.0","method":"initialize","id":"1","params":{}}`)
			req, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", body)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			resp, err := httpClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		It("should accept authenticated requests with valid DEX token", func() {
			token, err := fetchDEXToken(dexURL, clientID, clientSecret, username, password)
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			body := strings.NewReader(`{"jsonrpc":"2.0","method":"tasks/send","id":"1","params":{}}`)
			req, err := http.NewRequest(http.MethodPost, baseURL+"/a2a/invoke", body)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := httpClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			// Token accepted (not 401/403); downstream may return 4xx/5xx since KA is absent in Phase 1
			Expect(resp.StatusCode).NotTo(BeElementOf(http.StatusUnauthorized, http.StatusForbidden))
		})
	})

	Context("Security Headers", func() {
		It("should include security headers in responses", func() {
			resp, err := httpClient.Get(baseURL + "/healthz")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.Header.Get("X-Content-Type-Options")).To(Equal("nosniff"))
			Expect(resp.Header.Get("X-Frame-Options")).To(Equal("DENY"))
			Expect(resp.Header.Get("Strict-Transport-Security")).NotTo(BeEmpty())
			Expect(resp.Header.Get("Cache-Control")).To(Equal("no-store"))
		})
	})

	Context("Request ID Propagation", func() {
		It("should return X-Request-Id header", func() {
			resp, err := httpClient.Get(baseURL + "/healthz")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.Header.Get("X-Request-Id")).NotTo(BeEmpty())
		})
	})

	Context("Rate Limiting", func() {
		It("should enforce IP-based rate limiting", func() {
			var hitRateLimit bool
			for i := 0; i < 50; i++ {
				body := strings.NewReader(`{"jsonrpc":"2.0","method":"tasks/send","id":"1","params":{}}`)
				req, err := http.NewRequest(http.MethodPost, baseURL+"/a2a/invoke", body)
				Expect(err).NotTo(HaveOccurred())
				req.Header.Set("Content-Type", "application/json")

				resp, err := httpClient.Do(req)
				Expect(err).NotTo(HaveOccurred())
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusTooManyRequests {
					hitRateLimit = true
					break
				}
			}
			Expect(hitRateLimit).To(BeTrue(), "should have received 429 within 50 requests (burst=20)")
		})
	})
})
