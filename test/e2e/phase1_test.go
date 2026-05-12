package e2e_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

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

var _ = BeforeSuite(func() {
	baseURL = getEnvOrDefault("AF_E2E_BASE_URL", "https://localhost:18443")
	caCertPath = getEnvOrDefault("AF_E2E_CA_CERT", "")
	dexURL = getEnvOrDefault("AF_E2E_DEX_URL", "http://localhost:15556/dex")
	clientID = getEnvOrDefault("AF_E2E_CLIENT_ID", "kubernaut-apifrontend")
	clientSecret = getEnvOrDefault("AF_E2E_CLIENT_SECRET", "e2e-client-secret")
	username = getEnvOrDefault("AF_E2E_USERNAME", "e2e-user@kubernaut.ai")
	password = getEnvOrDefault("AF_E2E_PASSWORD", "password")

	httpClient = newTLSClient(caCertPath)

	Eventually(func() error {
		resp, err := httpClient.Get(baseURL + "/healthz")
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("healthz returned %d", resp.StatusCode)
		}
		return nil
	}, 60*time.Second, 2*time.Second).Should(Succeed(), "AF should become healthy")
})

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
			// May return 503 if deps (KA/DS) are not reachable — acceptable for Phase 1
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
			// Should not be 401 — may be 501 (A2A not configured) or other, but NOT unauthorized
			Expect(resp.StatusCode).NotTo(Equal(http.StatusUnauthorized))
		})
	})

	Context("Security Headers", func() {
		It("should include security headers in responses", func() {
			resp, err := httpClient.Get(baseURL + "/healthz")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.Header.Get("X-Content-Type-Options")).To(Equal("nosniff"))
			Expect(resp.Header.Get("X-Frame-Options")).To(Equal("DENY"))
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
