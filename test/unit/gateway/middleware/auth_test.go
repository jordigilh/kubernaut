/*
Copyright 2025 Jordi Gil.

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

package middleware

// BR-GATEWAY-036: TokenReview Authentication Middleware Unit Tests
// BR-GATEWAY-037: SubjectAccessReview Authorization Middleware Unit Tests
// Authority: pkg/shared/auth/middleware.go (DD-AUTH-014)
//
// Tests validate business outcomes:
// - Unauthenticated callers are rejected (401)
// - Unauthorized callers are rejected (403)
// - Authorized callers pass through with user identity in context
// - API failures produce 500 (not 401/403)
// - RFC 7807 error response format

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kubelog "github.com/jordigilh/kubernaut/pkg/log"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

var _ = Describe("Auth Middleware (BR-GATEWAY-036, BR-GATEWAY-037)", func() {

	var (
		logger = kubelog.NewLogger(kubelog.Options{
			Development: true,
			Level:       -10,
			ServiceName: "gateway-auth-test",
		})

		authConfig = auth.MiddlewareConfig{
			Namespace:    "kubernaut-system",
			Resource:     "services",
			ResourceName: "gateway-service",
			Verb:         "create",
		}

		nextHandlerCalled bool
		capturedUser      string

		nextHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextHandlerCalled = true
			capturedUser = auth.GetUserFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		})
	)

	BeforeEach(func() {
		nextHandlerCalled = false
		capturedUser = ""
	})

	Context("BR-GATEWAY-036: TokenReview Authentication", func() {

		It("UT-GW-036-001: Missing Authorization header returns 401 Unauthorized with RFC 7807 body", func() {
			authenticator := &auth.MockAuthenticator{ValidUsers: map[string]string{}}
			authorizer := &auth.MockAuthorizer{AllowedUsers: map[string]bool{}}
			authMiddleware := auth.NewMiddleware(authenticator, authorizer, authConfig, logger)

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			rr := httptest.NewRecorder()

			authMiddleware.Handler(nextHandler).ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusUnauthorized),
				"UT-GW-036-001: Missing auth header must return 401")
			Expect(rr.Header().Get("Content-Type")).To(Equal("application/problem+json"),
				"UT-GW-036-001: Error response must be RFC 7807")
			Expect(nextHandlerCalled).To(BeFalse(),
				"UT-GW-036-001: Next handler must NOT be called")

			var problem map[string]interface{}
			Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())
			Expect(problem["title"]).To(Equal("Unauthorized"))
			Expect(problem["status"]).To(BeEquivalentTo(401))
		})

		It("UT-GW-036-002: Non-Bearer Authorization scheme returns 401 Unauthorized", func() {
			authenticator := &auth.MockAuthenticator{ValidUsers: map[string]string{}}
			authorizer := &auth.MockAuthorizer{AllowedUsers: map[string]bool{}}
			authMiddleware := auth.NewMiddleware(authenticator, authorizer, authConfig, logger)

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
			rr := httptest.NewRecorder()

			authMiddleware.Handler(nextHandler).ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusUnauthorized),
				"UT-GW-036-002: Non-Bearer scheme must return 401")
			Expect(nextHandlerCalled).To(BeFalse(),
				"UT-GW-036-002: Next handler must NOT be called")
		})

		It("UT-GW-036-003: Empty Bearer token returns 401 Unauthorized", func() {
			authenticator := &auth.MockAuthenticator{ValidUsers: map[string]string{}}
			authorizer := &auth.MockAuthorizer{AllowedUsers: map[string]bool{}}
			authMiddleware := auth.NewMiddleware(authenticator, authorizer, authConfig, logger)

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			req.Header.Set("Authorization", "Bearer ")
			rr := httptest.NewRecorder()

			authMiddleware.Handler(nextHandler).ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusUnauthorized),
				"UT-GW-036-003: Empty Bearer token must return 401")
			Expect(nextHandlerCalled).To(BeFalse(),
				"UT-GW-036-003: Next handler must NOT be called")
		})

		It("UT-GW-036-004: Invalid token (authentication failure) returns 401 Unauthorized", func() {
			authenticator := &auth.MockAuthenticator{
				ValidUsers: map[string]string{
					"valid-token": "system:serviceaccount:kubernaut-system:alertmanager",
				},
			}
			authorizer := &auth.MockAuthorizer{AllowedUsers: map[string]bool{}}
			authMiddleware := auth.NewMiddleware(authenticator, authorizer, authConfig, logger)

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			req.Header.Set("Authorization", "Bearer invalid-token-xyz")
			rr := httptest.NewRecorder()

			authMiddleware.Handler(nextHandler).ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusUnauthorized),
				"UT-GW-036-004: Invalid token must return 401")
			Expect(authenticator.CallCount).To(Equal(1),
				"UT-GW-036-004: Authenticator must be called exactly once")
			Expect(nextHandlerCalled).To(BeFalse(),
				"UT-GW-036-004: Next handler must NOT be called")
		})

		It("UT-GW-036-005: TokenReview API error returns 500 Internal Server Error", func() {
			authenticator := &auth.MockAuthenticator{
				ErrorToReturn: fmt.Errorf("connection refused"),
			}
			authorizer := &auth.MockAuthorizer{AllowedUsers: map[string]bool{}}
			authMiddleware := auth.NewMiddleware(authenticator, authorizer, authConfig, logger)

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			req.Header.Set("Authorization", "Bearer some-token")
			rr := httptest.NewRecorder()

			authMiddleware.Handler(nextHandler).ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusInternalServerError),
				"UT-GW-036-005: API error must return 500, not 401")
			Expect(rr.Header().Get("Content-Type")).To(Equal("application/problem+json"))
			Expect(nextHandlerCalled).To(BeFalse(),
				"UT-GW-036-005: Next handler must NOT be called")
		})

		It("UT-GW-036-006: Valid token is extracted and passed to authenticator without Bearer prefix", func() {
			authenticator := &auth.MockAuthenticator{
				ValidUsers: map[string]string{
					"valid-token-abc": "system:serviceaccount:kubernaut-system:alertmanager",
				},
			}
			authorizer := &auth.MockAuthorizer{
				AllowedUsers: map[string]bool{
					"system:serviceaccount:kubernaut-system:alertmanager": true,
				},
			}
			authMiddleware := auth.NewMiddleware(authenticator, authorizer, authConfig, logger)

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			req.Header.Set("Authorization", "Bearer valid-token-abc")
			rr := httptest.NewRecorder()

			authMiddleware.Handler(nextHandler).ServeHTTP(rr, req)

			Expect(authenticator.CallCount).To(Equal(1),
				"UT-GW-036-006: Authenticator must be called exactly once")
			Expect(authorizer.CallCount).To(Equal(1),
				"UT-GW-036-006: Authorizer must be invoked after successful authentication")
			Expect(nextHandlerCalled).To(BeTrue(),
				"UT-GW-036-006: Next handler must be called for valid, authorized request")
		})
	})

	Context("BR-GATEWAY-037: SubjectAccessReview Authorization", func() {

		It("UT-GW-037-001: Authorized user passes through with user identity in context", func() {
			authenticator := &auth.MockAuthenticator{
				ValidUsers: map[string]string{
					"authorized-token": "system:serviceaccount:kubernaut-system:alertmanager",
				},
			}
			authorizer := &auth.MockAuthorizer{
				AllowedUsers: map[string]bool{
					"system:serviceaccount:kubernaut-system:alertmanager": true,
				},
			}
			authMiddleware := auth.NewMiddleware(authenticator, authorizer, authConfig, logger)

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			req.Header.Set("Authorization", "Bearer authorized-token")
			rr := httptest.NewRecorder()

			authMiddleware.Handler(nextHandler).ServeHTTP(rr, req)

			Expect(nextHandlerCalled).To(BeTrue(),
				"UT-GW-037-001: Next handler must be called for authorized user")
			Expect(capturedUser).To(Equal("system:serviceaccount:kubernaut-system:alertmanager"),
				"UT-GW-037-001: User identity must be available in request context")
		})

		It("UT-GW-037-002: Unauthorized user (SAR denied) returns 403 Forbidden", func() {
			authenticator := &auth.MockAuthenticator{
				ValidUsers: map[string]string{
					"readonly-token": "system:serviceaccount:test:readonly-sa",
				},
			}
			authorizer := &auth.MockAuthorizer{
				AllowedUsers: map[string]bool{
					"system:serviceaccount:test:readonly-sa": false,
				},
			}
			authMiddleware := auth.NewMiddleware(authenticator, authorizer, authConfig, logger)

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			req.Header.Set("Authorization", "Bearer readonly-token")
			rr := httptest.NewRecorder()

			authMiddleware.Handler(nextHandler).ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusForbidden),
				"UT-GW-037-002: Unauthorized user must get 403")
			Expect(rr.Header().Get("Content-Type")).To(Equal("application/problem+json"))
			Expect(nextHandlerCalled).To(BeFalse(),
				"UT-GW-037-002: Next handler must NOT be called")

			var problem map[string]interface{}
			Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())
			Expect(problem["title"]).To(Equal("Forbidden"))
		})

		It("UT-GW-037-003: SAR API error returns 500 Internal Server Error", func() {
			authenticator := &auth.MockAuthenticator{
				ValidUsers: map[string]string{
					"some-token": "system:serviceaccount:test:some-sa",
				},
			}
			authorizer := &auth.MockAuthorizer{
				ErrorToReturn: fmt.Errorf("SAR API unavailable"),
			}
			authMiddleware := auth.NewMiddleware(authenticator, authorizer, authConfig, logger)

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			req.Header.Set("Authorization", "Bearer some-token")
			rr := httptest.NewRecorder()

			authMiddleware.Handler(nextHandler).ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusInternalServerError),
				"UT-GW-037-003: SAR API error must return 500, not 403")
			Expect(nextHandlerCalled).To(BeFalse(),
				"UT-GW-037-003: Next handler must NOT be called")
		})

		It("UT-GW-037-004: X-Auth-Request-User header set for SOC2 attribution", func() {
			expectedUser := "system:serviceaccount:kubernaut-system:alertmanager"
			authenticator := &auth.MockAuthenticator{
				ValidUsers: map[string]string{
					"token-for-header-test": expectedUser,
				},
			}
			authorizer := &auth.MockAuthorizer{
				AllowedUsers: map[string]bool{expectedUser: true},
			}
			authMiddleware := auth.NewMiddleware(authenticator, authorizer, authConfig, logger)

			var capturedHeader string
			headerCheckHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedHeader = r.Header.Get("X-Auth-Request-User")
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			req.Header.Set("Authorization", "Bearer token-for-header-test")
			rr := httptest.NewRecorder()

			authMiddleware.Handler(headerCheckHandler).ServeHTTP(rr, req)

			Expect(capturedHeader).To(Equal(expectedUser),
				"UT-GW-037-004: X-Auth-Request-User header must contain authenticated user identity")
		})

		It("UT-GW-037-005: GetUserFromContext returns authenticated user from context", func() {
			ctx := context.WithValue(context.Background(), auth.UserContextKey, "system:serviceaccount:ns:sa")
			user := auth.GetUserFromContext(ctx)
			Expect(user).To(Equal("system:serviceaccount:ns:sa"),
				"UT-GW-037-005: GetUserFromContext must return the stored user identity")
		})

		It("UT-GW-037-006: GetUserFromContext returns empty string when no user in context", func() {
			user := auth.GetUserFromContext(context.Background())
			Expect(user).To(BeEmpty(),
				"UT-GW-037-006: GetUserFromContext must return empty string for missing user")
		})
	})

	// Issue #673: Security hardening tests
	Context("Issue #673: Generic Error Responses (C-2/M-3)", func() {

		It("UT-GW-673-001: TokenReview API error returns generic detail without internals", func() {
			authenticator := &auth.MockAuthenticator{
				ErrorToReturn: fmt.Errorf("connection refused"),
			}
			authorizer := &auth.MockAuthorizer{}
			authMiddleware := auth.NewMiddleware(authenticator, authorizer, authConfig, logger)

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			req.Header.Set("Authorization", "Bearer valid-token")
			rr := httptest.NewRecorder()

			authMiddleware.Handler(nextHandler).ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusInternalServerError),
				"UT-GW-673-001: API error must return 500")
			var problem map[string]interface{}
			Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())
			Expect(problem["detail"]).To(Equal("Authentication service unavailable"),
				"UT-GW-673-001: Detail must be generic, not leak internals")
			Expect(problem["detail"]).NotTo(ContainSubstring("connection refused"),
				"UT-GW-673-001: Internal error text must not appear in response")
			Expect(nextHandlerCalled).To(BeFalse())
		})

		It("UT-GW-673-002: SAR API error returns generic detail without internals", func() {
			authenticator := &auth.MockAuthenticator{
				ValidUsers: map[string]string{"sar-error-token": "system:serviceaccount:ns:sa"},
			}
			authorizer := &auth.MockAuthorizer{
				ErrorToReturn: fmt.Errorf("etcd timeout"),
			}
			authMiddleware := auth.NewMiddleware(authenticator, authorizer, authConfig, logger)

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			req.Header.Set("Authorization", "Bearer sar-error-token")
			rr := httptest.NewRecorder()

			authMiddleware.Handler(nextHandler).ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusInternalServerError),
				"UT-GW-673-002: SAR API error must return 500")
			var problem map[string]interface{}
			Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())
			Expect(problem["detail"]).To(Equal("Authorization service unavailable"),
				"UT-GW-673-002: Detail must be generic")
			Expect(problem["detail"]).NotTo(ContainSubstring("etcd timeout"),
				"UT-GW-673-002: Internal error text must not appear in response")
			Expect(nextHandlerCalled).To(BeFalse())
		})

		It("UT-GW-673-003: SAR denial returns generic 403 without RBAC details", func() {
			authenticator := &auth.MockAuthenticator{
				ValidUsers: map[string]string{"denied-token": "system:serviceaccount:ns:denied-sa"},
			}
			authorizer := &auth.MockAuthorizer{
				AllowedUsers: map[string]bool{},
			}
			authMiddleware := auth.NewMiddleware(authenticator, authorizer, authConfig, logger)

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			req.Header.Set("Authorization", "Bearer denied-token")
			rr := httptest.NewRecorder()

			authMiddleware.Handler(nextHandler).ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusForbidden),
				"UT-GW-673-003: SAR denial must return 403")
			var problem map[string]interface{}
			Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())
			Expect(problem["detail"]).To(Equal("Insufficient permissions"),
				"UT-GW-673-003: Detail must be generic")
			detail := problem["detail"].(string)
			Expect(detail).NotTo(ContainSubstring("verb:"),
				"UT-GW-673-003: Verb must not appear in response")
			Expect(detail).NotTo(ContainSubstring("remediationrequests"),
				"UT-GW-673-003: Resource name must not appear in response")
			Expect(nextHandlerCalled).To(BeFalse())
		})

		It("UT-GW-673-006: Invalid auth scheme returns generic format error without hint", func() {
			authenticator := &auth.MockAuthenticator{}
			authorizer := &auth.MockAuthorizer{}
			authMiddleware := auth.NewMiddleware(authenticator, authorizer, authConfig, logger)

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
			rr := httptest.NewRecorder()

			authMiddleware.Handler(nextHandler).ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusUnauthorized),
				"UT-GW-673-006: Wrong auth scheme must return 401")
			var problem map[string]interface{}
			Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())
			Expect(problem["detail"]).To(Equal("Invalid Authorization header format"),
				"UT-GW-673-006: Detail must not hint at expected format")
			Expect(problem["detail"]).NotTo(ContainSubstring("Bearer"),
				"UT-GW-673-006: 'Bearer' must not appear as format hint")
			Expect(problem["detail"]).NotTo(ContainSubstring("expected"),
				"UT-GW-673-006: 'expected' must not appear as format hint")
		})
	})

	Context("Issue #673: Identity Header Stripping (H-3)", func() {

		It("UT-GW-673-004: Client-supplied X-Auth-Request-User is replaced with authenticated identity", func() {
			expectedUser := "system:serviceaccount:ns:legitimate-sa"
			authenticator := &auth.MockAuthenticator{
				ValidUsers: map[string]string{"legit-token": expectedUser},
			}
			authorizer := &auth.MockAuthorizer{
				AllowedUsers: map[string]bool{expectedUser: true},
			}
			authMiddleware := auth.NewMiddleware(authenticator, authorizer, authConfig, logger)

			var capturedHeader string
			var capturedContextUser string
			headerCheckHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedHeader = r.Header.Get("X-Auth-Request-User")
				capturedContextUser = auth.GetUserFromContext(r.Context())
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			req.Header.Set("Authorization", "Bearer legit-token")
			req.Header.Set("X-Auth-Request-User", "attacker-spoofed")
			rr := httptest.NewRecorder()

			authMiddleware.Handler(headerCheckHandler).ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK),
				"UT-GW-673-004: Authorized request must pass through")
			Expect(capturedHeader).To(Equal(expectedUser),
				"UT-GW-673-004: X-Auth-Request-User must be authenticated identity, not spoofed value")
			Expect(capturedHeader).NotTo(Equal("attacker-spoofed"),
				"UT-GW-673-004: Spoofed header value must not reach next handler")
			Expect(capturedContextUser).To(Equal(expectedUser),
				"UT-GW-673-004: Context user must match authenticated identity")
		})

		It("UT-GW-673-005: Spoofed header not propagated on auth failure", func() {
			authenticator := &auth.MockAuthenticator{ValidUsers: map[string]string{}}
			authorizer := &auth.MockAuthorizer{}
			authMiddleware := auth.NewMiddleware(authenticator, authorizer, authConfig, logger)

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			req.Header.Set("X-Auth-Request-User", "attacker-spoofed")
			rr := httptest.NewRecorder()

			authMiddleware.Handler(nextHandler).ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusUnauthorized),
				"UT-GW-673-005: Missing auth must return 401")
			Expect(nextHandlerCalled).To(BeFalse(),
				"UT-GW-673-005: Next handler must NOT be called with spoofed header")
		})

		It("UT-GW-673-015: Multi-value X-Auth-Request-User headers are all removed", func() {
			expectedUser := "system:serviceaccount:ns:real-sa"
			authenticator := &auth.MockAuthenticator{
				ValidUsers: map[string]string{"good-token": expectedUser},
			}
			authorizer := &auth.MockAuthorizer{
				AllowedUsers: map[string]bool{expectedUser: true},
			}
			authMiddleware := auth.NewMiddleware(authenticator, authorizer, authConfig, logger)

			var capturedValues []string
			inspectHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedValues = r.Header.Values("X-Auth-Request-User")
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			req.Header.Set("Authorization", "Bearer good-token")
			req.Header.Add("X-Auth-Request-User", "spoofed-val-1")
			req.Header.Add("X-Auth-Request-User", "spoofed-val-2")
			rr := httptest.NewRecorder()

			authMiddleware.Handler(inspectHandler).ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK))
			Expect(capturedValues).To(HaveLen(1),
				"UT-GW-673-015: Exactly one header value must survive (the authenticated identity)")
			Expect(capturedValues[0]).To(Equal(expectedUser),
				"UT-GW-673-015: Surviving value must be the authenticated identity")
		})
	})

	Context("Issue #673 L-ADV-3: Empty Authorization header (BR-GATEWAY-182)", func() {

		It("UT-GW-673-016: Empty Authorization header returns 401", func() {
			authenticator := &auth.MockAuthenticator{ValidUsers: map[string]string{}}
			authorizer := &auth.MockAuthorizer{}
			authMiddleware := auth.NewMiddleware(authenticator, authorizer, authConfig, logger)

			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", nil)
			req.Header.Set("Authorization", "")
			rr := httptest.NewRecorder()

			authMiddleware.Handler(nextHandler).ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusUnauthorized),
				"UT-GW-673-016: Empty Authorization header must return 401")
			Expect(nextHandlerCalled).To(BeFalse(),
				"UT-GW-673-016: Handler must not be called with empty auth")

			body := rr.Body.Bytes()
			var problem map[string]interface{}
			Expect(json.Unmarshal(body, &problem)).To(Succeed())
			Expect(problem["title"]).To(Equal("Unauthorized"),
				"UT-GW-673-016: RFC 7807 title must indicate 401")
		})
	})
})
