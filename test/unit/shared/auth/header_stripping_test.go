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

package auth_test

import (
	"io"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

var _ = Describe("Impersonation Header Stripping — #703", func() {
	var (
		middleware   *auth.Middleware
		recorder     *httptest.ResponseRecorder
		capturedHeaders http.Header
	)

	BeforeEach(func() {
		capturedHeaders = nil
		recorder = httptest.NewRecorder()

		authenticator := &auth.MockAuthenticator{
			ValidUsers: map[string]string{
				"valid-token": "system:serviceaccount:kubernaut-system:test-sa",
			},
		}
		authorizer := &auth.MockAuthorizer{
			AllowedUsers: map[string]bool{
				"system:serviceaccount:kubernaut-system:test-sa": true,
			},
		}

		middleware = auth.NewMiddleware(
			authenticator,
			authorizer,
			auth.MiddlewareConfig{
				Namespace:    "kubernaut-system",
				Resource:     "services",
				ResourceName: "test-service",
				Verb:         "create",
			},
			logr.Discard(),
		)
	})

	handler := func() http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedHeaders = r.Header.Clone()
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "ok")
		})
	}

	Describe("UT-KA-703-C01: Middleware strips Impersonate-User header from incoming requests", func() {
		It("should remove Impersonate-User before reaching the handler", func() {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/test", nil)
			req.Header.Set("Authorization", "Bearer valid-token")
			req.Header.Set("Impersonate-User", "attacker@evil.com")

			middleware.Handler(handler()).ServeHTTP(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusOK))
			Expect(capturedHeaders.Get("Impersonate-User")).To(BeEmpty())
		})
	})

	Describe("UT-KA-703-C02: Middleware strips Impersonate-Group header", func() {
		It("should remove Impersonate-Group before reaching the handler", func() {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/test", nil)
			req.Header.Set("Authorization", "Bearer valid-token")
			req.Header.Set("Impersonate-Group", "system:masters")

			middleware.Handler(handler()).ServeHTTP(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusOK))
			Expect(capturedHeaders.Get("Impersonate-Group")).To(BeEmpty())
		})
	})

	Describe("UT-KA-703-C03: Middleware strips all Impersonate-Extra-* headers (wildcard)", func() {
		It("should remove all Impersonate-Extra- prefixed headers", func() {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/test", nil)
			req.Header.Set("Authorization", "Bearer valid-token")
			req.Header.Set("Impersonate-Extra-Scopes", "admin")
			req.Header.Set("Impersonate-Extra-Tenant", "evil-corp")

			middleware.Handler(handler()).ServeHTTP(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusOK))
			Expect(capturedHeaders.Get("Impersonate-Extra-Scopes")).To(BeEmpty())
			Expect(capturedHeaders.Get("Impersonate-Extra-Tenant")).To(BeEmpty())
		})
	})

	Describe("UT-KA-703-C04: Existing X-Auth-Request-User stripping unchanged (regression)", func() {
		It("should still strip X-Auth-Request-User and set it from authentication", func() {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/test", nil)
			req.Header.Set("Authorization", "Bearer valid-token")
			req.Header.Set("X-Auth-Request-User", "spoofed-user")

			middleware.Handler(handler()).ServeHTTP(recorder, req)

			Expect(recorder.Code).To(Equal(http.StatusOK))
			Expect(capturedHeaders.Get("X-Auth-Request-User")).To(Equal("system:serviceaccount:kubernaut-system:test-sa"))
		})
	})
})
