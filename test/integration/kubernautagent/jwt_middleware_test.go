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

var _ = Describe("JWT Middleware Pipeline — #1009", func() {
	var (
		mockJWKS    *testauth.MockJWKSServer
		jwtAuth     *auth.JWTAuthenticator
		mockK8sAuth *auth.MockAuthenticator
		mockAuthz   *auth.MockAuthorizer
		composite   *auth.CompositeAuthenticator
		middleware  *auth.Middleware
		server      *httptest.Server
		capturedUser string
		capturedProviderType string
	)

	BeforeEach(func() {
		var err error
		mockJWKS, err = testauth.NewMockJWKSServer("https://keycloak.example.com/realms/kubernaut")
		Expect(err).NotTo(HaveOccurred())

		jwtAuth, err = auth.NewJWTAuthenticator([]auth.JWTProviderEntry{
			{
				Issuer:        "https://keycloak.example.com/realms/kubernaut",
				JWKSURL:       mockJWKS.JWKSURL(),
				Audience:      "kubernaut-agent",
				UsernameClaim: "preferred_username",
				GroupsClaim:   "groups",
			},
		})
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
				"user-a@corp":  true,
				"system:serviceaccount:kubernaut-system:apifrontend": true,
			},
		}

		composite = auth.NewCompositeAuthenticator(jwtAuth, mockK8sAuth)

		middleware = auth.NewMiddleware(composite, mockAuthz, auth.MiddlewareConfig{
			Namespace:    "kubernaut-system",
			Resource:     "services",
			ResourceName: "kubernaut-agent",
			Verb:         "create",
		}, logr.Discard())

		capturedUser = ""
		capturedProviderType = ""

		handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedUser = auth.GetUserFromContext(r.Context())
			info := auth.GetUserInfoFromContext(r.Context())
			capturedProviderType = info.ProviderType
			w.WriteHeader(http.StatusOK)
		}))

		server = httptest.NewServer(handler)
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
		if mockJWKS != nil {
			mockJWKS.Close()
		}
	})

	Describe("IT-KA-1009-001: Full middleware pipeline accepts valid JWT", func() {
		It("should inject correct UserInfo into context for JWT-authenticated user", func() {
			token, err := mockJWKS.IssueJWT("user-a@corp", []string{"interactive-users"}, "kubernaut-agent", 5*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			req, _ := http.NewRequest("POST", server.URL+"/api/v1/investigate", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(capturedUser).To(Equal("user-a@corp"))
			Expect(capturedProviderType).To(HavePrefix("jwt:"))
		})
	})

	Describe("IT-KA-1009-002: Expired JWT rejected with 401", func() {
		It("should return 401 for expired JWT", func() {
			token, err := mockJWKS.IssueJWT("user-a@corp", []string{"g1"}, "kubernaut-agent", -1*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			req, _ := http.NewRequest("POST", server.URL+"/api/v1/investigate", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})
	})

	Describe("IT-KA-1009-004: JWT user fails SAR → 403", func() {
		It("should return 403 when JWT user lacks RBAC permissions", func() {
			token, err := mockJWKS.IssueJWT("unauthorized-user@corp", []string{"g1"}, "kubernaut-agent", 5*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			req, _ := http.NewRequest("POST", server.URL+"/api/v1/investigate", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
		})
	})

	Describe("IT-KA-1009-005: Mixed traffic — JWT + K8s SA token both succeed", func() {
		It("should accept both JWT and SA tokens through the same middleware", func() {
			jwtToken, err := mockJWKS.IssueJWT("user-a@corp", []string{"interactive-users"}, "kubernaut-agent", 5*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			req1, _ := http.NewRequest("POST", server.URL+"/api/v1/investigate", nil)
			req1.Header.Set("Authorization", "Bearer "+jwtToken)
			resp1, err := http.DefaultClient.Do(req1)
			Expect(err).NotTo(HaveOccurred())
			defer resp1.Body.Close()
			Expect(resp1.StatusCode).To(Equal(http.StatusOK))
			Expect(capturedUser).To(Equal("user-a@corp"))
			Expect(capturedProviderType).To(HavePrefix("jwt:"))

			req2, _ := http.NewRequest("POST", server.URL+"/api/v1/investigate", nil)
			req2.Header.Set("Authorization", "Bearer sa-token-apifrontend")
			resp2, err := http.DefaultClient.Do(req2)
			Expect(err).NotTo(HaveOccurred())
			defer resp2.Body.Close()
			Expect(resp2.StatusCode).To(Equal(http.StatusOK))
			Expect(capturedUser).To(Equal("system:serviceaccount:kubernaut-system:apifrontend"))
			Expect(capturedProviderType).To(Equal("k8s:tokenreview"))
		})
	})

	Describe("IT-KA-1009-007: Impersonation headers stripped before JWT validation", func() {
		It("should strip Impersonate-* headers from JWT requests", func() {
			token, err := mockJWKS.IssueJWT("user-a@corp", []string{"interactive-users"}, "kubernaut-agent", 5*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			var capturedHeaders http.Header
			handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedHeaders = r.Header.Clone()
				w.WriteHeader(http.StatusOK)
			}))
			srv := httptest.NewServer(handler)
			defer srv.Close()

			req, _ := http.NewRequest("POST", srv.URL+"/test", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Impersonate-User", "admin")
			req.Header.Set("Impersonate-Group", "cluster-admins")
			req.Header.Set("Impersonate-Extra-Scopes", "all")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(capturedHeaders.Get("Impersonate-User")).To(BeEmpty())
			Expect(capturedHeaders.Get("Impersonate-Group")).To(BeEmpty())
			Expect(capturedHeaders.Get("Impersonate-Extra-Scopes")).To(BeEmpty())
		})
	})
})
