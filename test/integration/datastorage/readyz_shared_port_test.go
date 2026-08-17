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

package datastorage

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// DD-PLATFORM-010: cross-service readiness check now lives on the main API
// port (8080), unauthenticated, alongside the dedicated kubelet-only health
// port (8081) which is unaffected. This suite proves both halves of the
// contract on the SAME real chi router used in production
// (createTestServerWithAccess's testServer wraps srv.Handler()):
// /readyz bypasses DD-AUTH-014 entirely, while /api/v1/* still enforces it.
var _ = Describe("BR-AUDIT-005 v2.0 / DD-PLATFORM-010: /readyz on the shared API port", Label("integration", "readyz-shared-port", "p0"), func() {
	It("GET :8080/readyz succeeds with no Authorization header", func() {
		testServer, healthServer, _ := createTestServerWithAccess()
		defer testServer.Close()
		defer healthServer.Close()

		req, err := http.NewRequest(http.MethodGet, testServer.URL+"/readyz", nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(req.Header.Get("Authorization")).To(BeEmpty(), "sanity: no auth header set")

		resp, err := http.DefaultClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusOK),
			"DD-PLATFORM-010: /readyz on the main API port must be reachable without a bearer token, "+
				"exactly like the dedicated health port it mirrors")
	})

	It("GET :8080/api/v1/audit/events still requires auth (no accidental exemption of the whole router)", func() {
		testServer, healthServer, _ := createTestServerWithAccess()
		defer testServer.Close()
		defer healthServer.Close()

		req, err := http.NewRequest(http.MethodGet, testServer.URL+"/api/v1/audit/events", nil)
		Expect(err).ToNot(HaveOccurred())

		resp, err := http.DefaultClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
			"DD-PLATFORM-010 must scope the auth exemption to /readyz only -- the real audit API "+
				"must still reject unauthenticated requests")
	})

	It("GET :8081/readyz on the dedicated health port is unaffected (kubelet probe path unchanged)", func() {
		testServer, healthServer, _ := createTestServerWithAccess()
		defer testServer.Close()
		defer healthServer.Close()

		resp, err := http.Get(healthServer.URL + "/readyz") //nolint:gosec,noctx // test-only probe, fixed loopback URL
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusOK),
			"the dedicated kubelet-facing health port must keep working exactly as before this change")
	})
})
