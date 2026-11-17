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

package shared

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jordigilh/kubernaut/pkg/shared/scope"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSharedHelpers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "FMC shared helper contracts")
}

var _ = Describe("BR-SCOPE-001: missing-target isolation assertions", func() {
	DescribeTable("UT-FMC-E2E-004: preserves verified managed decisions",
		func(body string, managed bool) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				defer GinkgoRecover()
				w.Header().Set("Content-Type", "application/json")
				_, err := w.Write([]byte(body))
				Expect(err).NotTo(HaveOccurred())
			}))
			defer server.Close()
			h := &Harness{Ctx: context.Background(), FMCHTTPClient: server.Client(), FMCAPIBaseURL: server.URL}
			Expect(ScopeCheck(Default, h, "prod-east", "", "v1", "Service", "test", "present")).To(Equal(managed))
		},
		Entry("present managed", `{"state":"verified_managed","provenance":"resource-label"}`, true),
		Entry("present unmanaged", `{"state":"verified_unmanaged","provenance":"resource-label"}`, false),
	)

	DescribeTable("UT-FMC-E2E-003: requires unavailable without a managed decision",
		func(status int, body string, accepted bool) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()
				Expect(r.URL.Query().Get("cluster")).To(Equal("prod-east"))
				Expect(r.URL.Query().Get("name")).To(Equal("absent"))
				w.WriteHeader(status)
				_, err := w.Write([]byte(body))
				Expect(err).NotTo(HaveOccurred())
			}))
			defer server.Close()
			h := &Harness{Ctx: context.Background(), FMCHTTPClient: server.Client(), FMCAPIBaseURL: server.URL}
			failures := InterceptGomegaFailures(func() {
				ScopeCheckUnavailable(Default, h, scope.ResourceIdentity{ClusterID: "prod-east", Version: "v1", Kind: "Service", Namespace: "test", Name: "absent"})
			})
			if accepted {
				Expect(failures).To(BeEmpty())
			} else {
				Expect(failures).NotTo(BeEmpty())
			}
		},
		Entry("missing target", http.StatusServiceUnavailable, `{"state":"unavailable","provenance":"unavailable"}`, true),
		Entry("legacy http.Error body rejected", http.StatusServiceUnavailable, "scope check temporarily unavailable\n", false),
		Entry("unrecognized plain text is not the unavailable contract", http.StatusServiceUnavailable, "unexpected proxy error", false),
		Entry("verified false is not absence", http.StatusOK, `{"managed":false}`, false),
		Entry("verified true is not absence", http.StatusOK, `{"managed":true}`, false),
		Entry("auth failure is not absence", http.StatusForbidden, `{"error":"forbidden"}`, false),
		Entry("unavailable must not carry a managed decision", http.StatusServiceUnavailable, `{"managed":false}`, false),
	)
})
