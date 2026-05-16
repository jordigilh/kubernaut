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
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

var _ = Describe("Multi-Issuer JWT — GAP-11 / DD-AUTH-MCP-001", Label("integration", "jwt"), func() {

	var (
		primaryJWKS   *testauth.MockJWKSServer
		secondaryJWKS *testauth.MockJWKSServer
		jwtAuth       *auth.JWTAuthenticator
		mockK8sAuth   *auth.MockAuthenticator
		mockAuthz     *auth.MockAuthorizer
		composite     *auth.CompositeAuthenticator
		middleware    *auth.Middleware
		server        *httptest.Server

		capturedUser         string
		capturedProviderType string
	)

	BeforeEach(func() {
		var err error

		primaryJWKS, err = testauth.NewMockJWKSServer("https://keycloak.example.com/realms/kubernaut")
		Expect(err).NotTo(HaveOccurred())

		secondaryJWKS, err = testauth.NewMockJWKSServer("https://dex.example.com")
		Expect(err).NotTo(HaveOccurred())

		jwtAuth, err = auth.NewJWTAuthenticator([]auth.JWTProviderEntry{
			{
				Issuer:        "https://keycloak.example.com/realms/kubernaut",
				JWKSURL:       primaryJWKS.JWKSURL(),
				Audience:      "kubernaut-agent",
				UsernameClaim: "preferred_username",
				GroupsClaim:   "groups",
			},
			{
				Issuer:        "https://dex.example.com",
				JWKSURL:       secondaryJWKS.JWKSURL(),
				Audience:      "kubernaut-agent",
				UsernameClaim: "preferred_username",
				GroupsClaim:   "groups",
			},
		}, logr.Discard())
		Expect(err).NotTo(HaveOccurred())

		mockK8sAuth = &auth.MockAuthenticator{
			ValidUsers: map[string]string{
				"sa-token-apifrontend": "system:serviceaccount:kubernaut-system:apifrontend",
			},
			ValidUsersFull: map[string]auth.UserInfo{
				"sa-token-apifrontend": {
					Username:     "system:serviceaccount:kubernaut-system:apifrontend",
					Groups:       []string{"system:serviceaccounts"},
					ProviderType: "k8s:tokenreview",
				},
			},
		}

		mockAuthz = &auth.MockAuthorizer{
			AllowedUsers: map[string]bool{
				"alice@keycloak":                                       true,
				"bob@dex":                                              true,
				"system:serviceaccount:kubernaut-system:apifrontend":   true,
			},
		}

		composite = auth.NewCompositeAuthenticator(jwtAuth, mockK8sAuth)
		middleware = auth.NewMiddleware(composite, mockAuthz, auth.MiddlewareConfig{
			Namespace: "kubernaut-system",
		}, logr.Discard())

		handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedUser = auth.GetUserFromContext(r.Context())
			info := auth.GetUserInfoFromContext(r.Context())
			capturedProviderType = info.ProviderType
			w.WriteHeader(http.StatusOK)
		}))

		server = httptest.NewServer(handler)
	})

	AfterEach(func() {
		server.Close()
		primaryJWKS.Close()
		secondaryJWKS.Close()
		capturedUser = ""
		capturedProviderType = ""
	})

	// ---------------------------------------------------------------
	// IT-KA-JWT-001: Primary issuer JWT accepted
	// BR: DD-AUTH-MCP-001
	// ---------------------------------------------------------------
	Describe("IT-KA-JWT-001: Primary issuer JWT accepted", func() {
		It("should authenticate and authorize JWT from primary Keycloak issuer", func() {
			token, err := primaryJWKS.IssueJWT("alice@keycloak", []string{"sre"}, "kubernaut-agent", 10*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			req, err := http.NewRequest("GET", server.URL+"/api/v1/mcp", nil)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(capturedUser).To(Equal("alice@keycloak"))
			Expect(capturedProviderType).To(Equal("jwt"))

			GinkgoWriter.Println("IT-KA-JWT-001: Primary issuer JWT validated")
		})
	})

	// ---------------------------------------------------------------
	// IT-KA-JWT-002: Secondary issuer JWT accepted
	// BR: DD-AUTH-MCP-001
	// ---------------------------------------------------------------
	Describe("IT-KA-JWT-002: Secondary issuer (DEX) JWT accepted", func() {
		It("should authenticate and authorize JWT from secondary DEX issuer", func() {
			token, err := secondaryJWKS.IssueJWT("bob@dex", []string{"platform"}, "kubernaut-agent", 10*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			req, err := http.NewRequest("GET", server.URL+"/api/v1/mcp", nil)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(capturedUser).To(Equal("bob@dex"))
			Expect(capturedProviderType).To(Equal("jwt"))

			GinkgoWriter.Println("IT-KA-JWT-002: Secondary issuer JWT validated")
		})
	})

	// ---------------------------------------------------------------
	// IT-KA-JWT-003: Unknown issuer JWT rejected, K8s SA fallback works
	// BR: DD-AUTH-MCP-001
	// ---------------------------------------------------------------
	Describe("IT-KA-JWT-003: Unknown issuer JWT rejected with K8s SA fallback", func() {
		It("should reject JWT from unknown issuer and accept valid SA token", func() {
			unknownJWKS, err := testauth.NewMockJWKSServer("https://unknown-provider.com")
			Expect(err).NotTo(HaveOccurred())
			defer unknownJWKS.Close()

			By("Sending JWT from unknown issuer — should be rejected")
			token, err := unknownJWKS.IssueJWT("evil@unknown", []string{"admin"}, "kubernaut-agent", 10*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			req, err := http.NewRequest("GET", server.URL+"/api/v1/mcp", nil)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(SatisfyAny(
				Equal(http.StatusUnauthorized),
				Equal(http.StatusForbidden),
			), "unknown issuer JWT should be rejected")

			By("Sending valid K8s SA token — should be accepted (coexistence)")
			saReq, err := http.NewRequest("GET", server.URL+"/api/v1/mcp", nil)
			Expect(err).NotTo(HaveOccurred())
			saReq.Header.Set("Authorization", "Bearer sa-token-apifrontend")

			saResp, err := http.DefaultClient.Do(saReq)
			Expect(err).NotTo(HaveOccurred())
			defer saResp.Body.Close()

			Expect(saResp.StatusCode).To(Equal(http.StatusOK))
			Expect(capturedUser).To(Equal("system:serviceaccount:kubernaut-system:apifrontend"))
			Expect(capturedProviderType).To(Equal("k8s:tokenreview"))

			GinkgoWriter.Println("IT-KA-JWT-003: Unknown issuer rejected, K8s SA coexistence validated")
		})
	})
})
