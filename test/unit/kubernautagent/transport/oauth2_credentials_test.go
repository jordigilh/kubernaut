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

package transport_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/config"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/transport"
)

// newMockIdPServer returns an httptest.Server that responds to OAuth2 token
// requests with the given access token. Caller must defer Close().
func newMockIdPServer(accessToken string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		Expect(json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": accessToken,
			"token_type":   "Bearer",
			"expires_in":   3600,
		})).To(Succeed())
	}))
}

var _ = Describe("OAuth2 Client Credentials Transport — #417", func() {

	Describe("UT-KA-417-026: NewOAuth2ClientCredentialsTransport wraps base transport", func() {
		It("should return an http.RoundTripper that delegates to the base transport after token injection", func() {
			var tokenRequestCount atomic.Int32
			idpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tokenRequestCount.Add(1)
				w.Header().Set("Content-Type", "application/json")
				Expect(json.NewEncoder(w).Encode(map[string]interface{}{
					"access_token": "test-jwt-token",
					"token_type":   "Bearer",
					"expires_in":   3600,
				})).To(Succeed())
			}))
			defer idpServer.Close()

			inner := &capturingTransport{}
			cfg := kaconfig.OAuth2Config{
				Enabled:      true,
				TokenURL:     idpServer.URL,
				ClientID:     "test-client",
				ClientSecret: "test-secret",
				Scopes:       []string{"openid"},
			}

			rt := transport.NewOAuth2ClientCredentialsTransport(cfg, inner)
			Expect(rt).NotTo(BeNil())

			req := httptest.NewRequest("POST", "https://llm.example.com/v1/chat/completions", nil)
			_, err := rt.RoundTrip(req)
			Expect(err).NotTo(HaveOccurred())

			Expect(inner.captured).NotTo(BeNil(), "base transport must be called")
			Expect(inner.captured.Header.Get("Authorization")).To(Equal("Bearer test-jwt-token"),
				"OAuth2 transport must inject the acquired token")
			Expect(tokenRequestCount.Load()).To(BeNumerically(">=", 1),
				"IdP token endpoint must have been called")
		})
	})

	Describe("UT-KA-417-027: NewOAuth2ClientCredentialsTransport with nil base defaults to DefaultTransport", func() {
		It("should not panic when base is nil", func() {
			idpServer := newMockIdPServer("token-nil-base")
			defer idpServer.Close()

			cfg := kaconfig.OAuth2Config{
				Enabled:      true,
				TokenURL:     idpServer.URL,
				ClientID:     "test-client",
				ClientSecret: "test-secret",
			}

			Expect(func() {
				rt := transport.NewOAuth2ClientCredentialsTransport(cfg, nil)
				Expect(rt).NotTo(BeNil())
			}).NotTo(Panic())
		})
	})

	Describe("UT-KA-417-028: Transport chain ordering is correct", func() {
		It("should compose OAuth2 -> AuthHeaders -> StructuredOutput in the correct order", func() {
			var capturedHeaders http.Header
			llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedHeaders = r.Header.Clone()
				w.Header().Set("Content-Type", "application/json")
				_, err := fmt.Fprint(w, `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"{}"}]}`)
				Expect(err).NotTo(HaveOccurred())
			}))
			defer llmServer.Close()

			idpServer := newMockIdPServer("chain-test-jwt")
			defer idpServer.Close()

			oauth2Cfg := kaconfig.OAuth2Config{
				Enabled:      true,
				TokenURL:     idpServer.URL,
				ClientID:     "chain-client",
				ClientSecret: "chain-secret",
			}
			authHeaders := []config.HeaderDefinition{
				{Name: "X-Tenant-Id", Value: "kubernaut-prod"},
			}

			base := http.DefaultTransport
			oauth2RT := transport.NewOAuth2ClientCredentialsTransport(oauth2Cfg, base)
			authRT := transport.NewAuthHeadersTransport(authHeaders, oauth2RT)
			soRT := transport.NewStructuredOutputTransport(json.RawMessage(`{"type":"object"}`), authRT)

			body := `{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"test"}]}`
			req, err := http.NewRequest("POST", llmServer.URL+"/v1/messages", strings.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			resp, err := soRT.RoundTrip(req)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			Expect(capturedHeaders.Get("Authorization")).To(Equal("Bearer chain-test-jwt"),
				"OAuth2 token must reach the LLM server")
			Expect(capturedHeaders.Get("X-Tenant-Id")).To(Equal("kubernaut-prod"),
				"custom auth headers must reach the LLM server")
		})
	})
})
