/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kubernautagent_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/transport"
)

// newMockIdPServerWithExpiry returns an httptest.Server that issues tokens
// with a configurable expiry. Useful for testing token refresh behavior.
func newMockIdPServerWithExpiry(accessToken string, expiresIn int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": accessToken,
			"token_type":   "Bearer",
			"expires_in":   expiresIn,
		})
	}))
}

var _ = Describe("OAuth2 Client Credentials Integration — #417", func() {

	Describe("IT-KA-417-010: Full round trip with token acquisition from IdP", func() {
		It("should acquire token from IdP and inject Authorization header into LLM request", func() {
			var capturedAuthHeader string

			llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedAuthHeader = r.Header.Get("Authorization")
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}]}`)
			}))
			defer llmServer.Close()

			idpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal("POST"), "token request must be POST")
				Expect(r.FormValue("grant_type")).To(Equal("client_credentials"),
					"grant_type must be client_credentials")
				Expect(r.FormValue("scope")).To(Equal("openid llm-access"),
					"scopes must be space-separated")

				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"access_token": "integration-jwt-abc123",
					"token_type":   "Bearer",
					"expires_in":   3600,
				})
			}))
			defer idpServer.Close()

			cfg := kaconfig.OAuth2Config{
				Enabled:      true,
				TokenURL:     idpServer.URL,
				ClientID:     "kubernaut-agent",
				ClientSecret: "integration-secret",
				Scopes:       []string{"openid", "llm-access"},
			}

			rt := transport.NewOAuth2ClientCredentialsTransport(cfg, http.DefaultTransport)

			body := `{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"analyze"}]}`
			req, err := http.NewRequest("POST", llmServer.URL+"/v1/messages", strings.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			resp, err := rt.RoundTrip(req)
			Expect(err).NotTo(HaveOccurred())
			_ = resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(capturedAuthHeader).To(Equal("Bearer integration-jwt-abc123"),
				"LLM server must receive the JWT acquired from the IdP")
		})
	})

	Describe("IT-KA-417-011: Token refresh on expiry", func() {
		It("should automatically re-acquire a fresh token after expiry", func() {
			var tokenVersion atomic.Int32

			idpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				version := tokenVersion.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"access_token": fmt.Sprintf("token-v%d", version),
					"token_type":   "Bearer",
					"expires_in":   1,
				})
			}))
			defer idpServer.Close()

			var capturedTokens []string
			llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedTokens = append(capturedTokens, r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}]}`)
			}))
			defer llmServer.Close()

			cfg := kaconfig.OAuth2Config{
				Enabled:      true,
				TokenURL:     idpServer.URL,
				ClientID:     "kubernaut-agent",
				ClientSecret: "refresh-secret",
			}

			rt := transport.NewOAuth2ClientCredentialsTransport(cfg, http.DefaultTransport)

			sendRequest := func() {
				req, err := http.NewRequest("POST", llmServer.URL+"/v1/messages",
					strings.NewReader(`{"messages":[{"role":"user","content":"test"}]}`))
				Expect(err).NotTo(HaveOccurred())
				req.Header.Set("Content-Type", "application/json")
				resp, err := rt.RoundTrip(req)
				Expect(err).NotTo(HaveOccurred())
				_ = resp.Body.Close()
			}

			sendRequest()
			Expect(capturedTokens).To(HaveLen(1))
			firstToken := capturedTokens[0]

			// The oauth2.ReuseTokenSource uses real wall-clock time to determine
			// token expiry. We must wait for the 1s token to actually expire.
			// This is NOT an async polling scenario (Eventually() would not help)
			// — it's deterministic time passage for the stdlib token cache.
			time.Sleep(2 * time.Second)

			sendRequest()
			Expect(capturedTokens).To(HaveLen(2))
			secondToken := capturedTokens[1]

			Expect(firstToken).NotTo(Equal(secondToken),
				"second request must use a refreshed token, not the expired one")
			Expect(firstToken).To(Equal("Bearer token-v1"))
			Expect(secondToken).To(Equal("Bearer token-v2"))
		})
	})

	Describe("IT-KA-417-012: IdP unavailable produces actionable error", func() {
		It("should return a clear error when the token endpoint is unreachable", func() {
			cfg := kaconfig.OAuth2Config{
				Enabled:      true,
				TokenURL:     "http://127.0.0.1:1/nonexistent-idp/token",
				ClientID:     "kubernaut-agent",
				ClientSecret: "secret",
			}

			rt := transport.NewOAuth2ClientCredentialsTransport(cfg, http.DefaultTransport)

			req, err := http.NewRequest("POST", "https://llm.example.com/v1/messages",
				strings.NewReader(`{"messages":[{"role":"user","content":"test"}]}`))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			_, err = rt.RoundTrip(req)
			Expect(err).To(HaveOccurred(), "request must fail when IdP is unreachable")
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("connection refused"),
				ContainSubstring("connect"),
				ContainSubstring("oauth2"),
			), "error message must indicate a connection/oauth2 issue")
		})
	})
})
