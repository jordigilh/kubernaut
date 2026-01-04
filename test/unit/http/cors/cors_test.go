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

package cors

import (
	"net/http"
	"net/http/httptest"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kubecors "github.com/jordigilh/kubernaut/pkg/http/cors"
)

// BR-HTTP-015: CORS Security Policy Enforcement - BEHAVIOR & CORRECTNESS Testing
//
// FOCUS: Test WHAT the CORS middleware does (behavior), NOT HOW it does it (implementation)
// BEHAVIOR: Does it allow/block cross-origin requests? Does it handle preflight?
// CORRECTNESS: Are specific origins/methods correctly authorized? Is security classification accurate?

var _ = Describe("BR-HTTP-015: CORS Security Policy Enforcement", func() {
	var (
		testHandler http.Handler
	)

	BeforeEach(func() {
		// Simple handler that returns 200 OK
		testHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		})
	})

	AfterEach(func() {
		// Cleanup environment variables after each test
		_ = os.Unsetenv("CORS_ALLOWED_ORIGINS")
		_ = os.Unsetenv("CORS_ALLOWED_METHODS")
		_ = os.Unsetenv("CORS_ALLOWED_HEADERS")
		_ = os.Unsetenv("CORS_ALLOW_CREDENTIALS")
		_ = os.Unsetenv("CORS_MAX_AGE")
		_ = os.Unsetenv("CORS_EXPOSED_HEADERS")
	})

	// ==============================================
	// CATEGORY 1: Cross-Origin Request Authorization (BEHAVIOR)
	// BR-HTTP-015: System must enforce CORS security policy
	// ==============================================

	Context("Cross-Origin Request Authorization - BEHAVIOR", func() {

		DescribeTable("should authorize/deny cross-origin requests based on origin whitelist",
			func(configuredOrigins, requestOrigin string, shouldBeAuthorized bool, description string) {
				// BEHAVIOR: Browser either gets CORS permission or doesn't
				_ = os.Setenv("CORS_ALLOWED_ORIGINS", configuredOrigins)
				opts := kubecors.FromEnvironment()
				handler := kubecors.Handler(opts)(testHandler)

				req := httptest.NewRequest("GET", "/api/v1/data", nil)
				req.Header.Set("Origin", requestOrigin)
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				allowOrigin := rec.Header().Get("Access-Control-Allow-Origin")

				if shouldBeAuthorized {
					// BEHAVIOR VALIDATION: Browser CAN make the cross-origin request
					Expect(allowOrigin).To(SatisfyAny(
						Equal(requestOrigin),
						Equal("*"),
					), "%s - should receive CORS authorization", description)
				} else {
					// BEHAVIOR VALIDATION: Browser CANNOT make the cross-origin request
					Expect(allowOrigin).ToNot(Equal(requestOrigin),
						"%s - should NOT receive CORS authorization for this origin", description)
				}
			},
			// CORRECTNESS: Specific origin patterns work correctly
			Entry("exact match from whitelist → authorized",
				"https://app.kubernaut.io",
				"https://app.kubernaut.io",
				true,
				"Whitelisted origin should be authorized"),
			Entry("origin not in whitelist → blocked",
				"https://app.kubernaut.io",
				"https://malicious-site.com",
				false,
				"Non-whitelisted origin should be blocked for security"),
			Entry("first of multiple whitelisted origins → authorized",
				"https://app.kubernaut.io,https://dashboard.kubernaut.io",
				"https://app.kubernaut.io",
				true,
				"First origin in whitelist should be authorized"),
			Entry("second of multiple whitelisted origins → authorized",
				"https://app.kubernaut.io,https://dashboard.kubernaut.io",
				"https://dashboard.kubernaut.io",
				true,
				"Second origin in whitelist should be authorized"),
			Entry("wildcard origin → any origin authorized",
				"*",
				"https://any-site.example.com",
				true,
				"Wildcard should authorize any origin (development mode)"),
		)
	})

	// ==============================================
	// CATEGORY 2: HTTP Method Authorization (BEHAVIOR)
	// BR-HTTP-015: System must authorize specific HTTP methods
	// ==============================================

	Context("HTTP Method Authorization via Preflight - BEHAVIOR", func() {

		DescribeTable("should permit/deny specific HTTP methods for cross-origin requests",
			func(configuredMethods, requestedMethod string, shouldBePermitted bool, description string) {
				// BEHAVIOR: Browser can/cannot use specific HTTP method cross-origin
				_ = os.Setenv("CORS_ALLOWED_ORIGINS", "*")
				_ = os.Setenv("CORS_ALLOWED_METHODS", configuredMethods)
				opts := kubecors.FromEnvironment()
				handler := kubecors.Handler(opts)(testHandler)

				// Preflight request (OPTIONS) asking if method is allowed
				req := httptest.NewRequest("OPTIONS", "/api/v1/data", nil)
				req.Header.Set("Origin", "https://app.kubernaut.io")
				req.Header.Set("Access-Control-Request-Method", requestedMethod)
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				allowMethods := rec.Header().Get("Access-Control-Allow-Methods")

				if shouldBePermitted {
					// BEHAVIOR VALIDATION: Method is permitted for cross-origin requests
					Expect(allowMethods).To(ContainSubstring(requestedMethod),
						"%s - method should be in Access-Control-Allow-Methods", description)
				}
			},
			Entry("POST permitted when configured → can create resources",
				"GET,POST,PUT,DELETE", "POST", true,
				"POST needed for creating resources via cross-origin request"),
			Entry("PUT permitted when configured → can update resources",
				"GET,POST,PUT,DELETE", "PUT", true,
				"PUT needed for updating resources via cross-origin request"),
			Entry("DELETE permitted when configured → can remove resources",
				"GET,POST,PUT,DELETE", "DELETE", true,
				"DELETE needed for removing resources via cross-origin request"),
			Entry("PATCH permitted when configured → can patch resources",
				"GET,POST,PUT,PATCH,DELETE", "PATCH", true,
				"PATCH needed for partial updates via cross-origin request"),
		)
	})

	// ==============================================
	// CATEGORY 3: Development Environment Support (BEHAVIOR)
	// BR-HTTP-015: System must support development workflow
	// ==============================================

	Context("Development Environment Support - BEHAVIOR", func() {

		It("should allow any origin in development mode for rapid iteration", func() {
			// BEHAVIOR: Developers can test from any local environment without configuration
			// No environment variables set = development defaults

			opts := kubecors.FromEnvironment()
			handler := kubecors.Handler(opts)(testHandler)

			// Developer testing from localhost
			req := httptest.NewRequest("GET", "/api/v1/data", nil)
			req.Header.Set("Origin", "http://localhost:3000")
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			// BEHAVIOR VALIDATION: Development origin is authorized
			allowOrigin := rec.Header().Get("Access-Control-Allow-Origin")
			Expect(allowOrigin).To(SatisfyAny(
				Equal("*"),
				Equal("http://localhost:3000"),
			), "Development origin should be authorized for rapid iteration")
		})

		It("should include standard methods in development defaults", func() {
			// BEHAVIOR: Common HTTP methods work out-of-box in development

			opts := kubecors.FromEnvironment()

			// CORRECTNESS: Default methods include what developers need
			Expect(opts.AllowedMethods).To(ContainElements(
				"GET", "POST", "PUT", "DELETE", "OPTIONS",
			), "Development defaults should include common HTTP methods")
		})
	})

	// ==============================================
	// CATEGORY 4: Preflight Request Handling (BEHAVIOR)
	// BR-HTTP-015: System must handle browser preflight requests
	// ==============================================

	Context("Preflight Request Handling - BEHAVIOR", func() {

		It("should respond to preflight OPTIONS request with CORS headers", func() {
			// BEHAVIOR: Browser's preflight check succeeds before actual request
			_ = os.Setenv("CORS_ALLOWED_ORIGINS", "https://app.kubernaut.io")
			opts := kubecors.FromEnvironment()
			handler := kubecors.Handler(opts)(testHandler)

			req := httptest.NewRequest("OPTIONS", "/api/v1/signals/prometheus", nil)
			req.Header.Set("Origin", "https://app.kubernaut.io")
			req.Header.Set("Access-Control-Request-Method", "POST")
			req.Header.Set("Access-Control-Request-Headers", "Content-Type,Authorization")
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			// BEHAVIOR VALIDATION: Preflight response includes required CORS headers
			Expect(rec.Header().Get("Access-Control-Allow-Origin")).ToNot(BeEmpty(),
				"Preflight must include Access-Control-Allow-Origin")
			Expect(rec.Header().Get("Access-Control-Allow-Methods")).ToNot(BeEmpty(),
				"Preflight must include Access-Control-Allow-Methods")
		})

		It("should include Max-Age header for preflight caching", func() {
			// BEHAVIOR: Browsers can cache preflight response to reduce requests
			_ = os.Setenv("CORS_ALLOWED_ORIGINS", "*")
			opts := kubecors.FromEnvironment()
			handler := kubecors.Handler(opts)(testHandler)

			req := httptest.NewRequest("OPTIONS", "/api/v1/data", nil)
			req.Header.Set("Origin", "https://app.kubernaut.io")
			req.Header.Set("Access-Control-Request-Method", "GET")
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			// BEHAVIOR VALIDATION: Max-Age allows browser to cache preflight
			maxAge := rec.Header().Get("Access-Control-Max-Age")
			Expect(maxAge).ToNot(BeEmpty(),
				"Max-Age should be set for preflight caching")
		})
	})

	// ==============================================
	// CATEGORY 5: Configuration Safety Classification (CORRECTNESS)
	// BR-HTTP-015: System must identify insecure configurations
	// ==============================================

	Context("Configuration Safety Classification - CORRECTNESS", func() {

		DescribeTable("should correctly classify configuration security level",
			func(origins []string, isSecure bool, securityRisk string) {
				// CORRECTNESS: Security classification is accurate
				opts := &kubecors.Options{AllowedOrigins: origins}

				Expect(opts.IsProduction()).To(Equal(isSecure), securityRisk)
			},
			Entry("wildcard is INSECURE - allows any website to access API",
				[]string{"*"}, false,
				"Wildcard exposes API to all websites including malicious ones"),
			Entry("explicit single origin is SECURE - restricted access",
				[]string{"https://app.kubernaut.io"}, true,
				"Only whitelisted origin can access the API"),
			Entry("explicit multiple origins is SECURE - restricted access",
				[]string{"https://app.kubernaut.io", "https://dashboard.kubernaut.io"}, true,
				"Only whitelisted origins can access the API"),
			Entry("empty origins is INSECURE - likely misconfiguration",
				[]string{}, false,
				"Empty origin list indicates deployment misconfiguration"),
			Entry("wildcard mixed with specific is INSECURE - wildcard negates restrictions",
				[]string{"https://app.kubernaut.io", "*"}, false,
				"Wildcard in list negates all origin restrictions"),
		)
	})

	// ==============================================
	// CATEGORY 6: Credentials Handling (BEHAVIOR)
	// BR-HTTP-015: System must handle credentials correctly
	// ==============================================

	Context("Credentials Handling - BEHAVIOR", func() {

		It("should include credentials header when configured", func() {
			// BEHAVIOR: Cross-origin requests can include cookies/auth when enabled
			_ = os.Setenv("CORS_ALLOWED_ORIGINS", "https://app.kubernaut.io")
			_ = os.Setenv("CORS_ALLOW_CREDENTIALS", "true")
			opts := kubecors.FromEnvironment()
			handler := kubecors.Handler(opts)(testHandler)

			req := httptest.NewRequest("GET", "/api/v1/data", nil)
			req.Header.Set("Origin", "https://app.kubernaut.io")
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			// BEHAVIOR VALIDATION: Credentials are allowed
			allowCredentials := rec.Header().Get("Access-Control-Allow-Credentials")
			Expect(allowCredentials).To(Equal("true"),
				"Credentials should be allowed when configured")
		})

		It("should NOT include credentials header by default", func() {
			// BEHAVIOR: Credentials disabled by default for security
			_ = os.Setenv("CORS_ALLOWED_ORIGINS", "*")
			opts := kubecors.FromEnvironment()
			handler := kubecors.Handler(opts)(testHandler)

			req := httptest.NewRequest("GET", "/api/v1/data", nil)
			req.Header.Set("Origin", "https://app.kubernaut.io")
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			// BEHAVIOR VALIDATION: Credentials not allowed by default
			allowCredentials := rec.Header().Get("Access-Control-Allow-Credentials")
			Expect(allowCredentials).ToNot(Equal("true"),
				"Credentials should NOT be allowed by default for security")
		})
	})
})
