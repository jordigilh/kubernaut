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

package fmc_test

import (
	"context"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

var _ = Describe("FMC HTTP Client (BR-INTEGRATION-065, ADR-068)", func() {
	var (
		server *httptest.Server
		client *fmc.HTTPClient
	)

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	It("UT-FMC-HC-001 [AC-4]: returns managed=true when FMC responds with managed=true", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"managed":true}`))
		}))
		client = fmc.NewHTTPClient(server.URL)

		managed, err := client.IsManagedResource(context.Background(), scope.ResourceIdentity{
			ClusterID: "prod-east", Group: "apps", Version: "v1",
			Kind: "Deployment", Namespace: "default", Name: "nginx",
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(managed).To(BeTrue())
	})

	It("UT-FMC-HC-002 [AC-4]: returns managed=false when FMC responds with managed=false", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"managed":false}`))
		}))
		client = fmc.NewHTTPClient(server.URL)

		managed, err := client.IsManagedResource(context.Background(), scope.ResourceIdentity{
			ClusterID: "prod-east", Group: "apps", Version: "v1",
			Kind: "Deployment", Namespace: "default", Name: "missing",
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(managed).To(BeFalse())
	})

	DescribeTable("UT-FMC-HC-003..005 [SC-7]: fail-safe returns managed=false on backend errors",
		func(setupServer func() string) {
			client = fmc.NewHTTPClient(setupServer())

			managed, err := client.IsManagedResource(context.Background(), scope.ResourceIdentity{
				ClusterID: "prod-east", Kind: "Deployment", Name: "nginx",
			})

			Expect(err).ToNot(HaveOccurred(),
				"errors must be absorbed (fail-safe), not propagated")
			Expect(managed).To(BeFalse())
		},
		Entry("connection refused", func() string {
			return "http://127.0.0.1:1"
		}),
		Entry("HTTP 500", func() string {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			return server.URL
		}),
		Entry("malformed JSON body", func() string {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`not-json`))
			}))
			return server.URL
		}),
	)

	It("UT-FMC-HC-006 [SI-10]: query parameters are URL-encoded correctly", func() {
		var receivedQuery string
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedQuery = r.URL.RawQuery
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"managed":false}`))
		}))
		client = fmc.NewHTTPClient(server.URL)

		_, _ = client.IsManagedResource(context.Background(), scope.ResourceIdentity{
			ClusterID: "cluster with spaces",
			Kind:      "Deployment",
			Name:      "name/with/slashes",
			Namespace: "ns",
		})

		Expect(receivedQuery).To(ContainSubstring("cluster=cluster+with+spaces"))
		Expect(receivedQuery).To(ContainSubstring("name=name%2Fwith%2Fslashes"))
	})

	It("UT-FMC-HC-007: HTTPClient satisfies scope.ScopeChecker interface", func() {
		var checker scope.ScopeChecker = fmc.NewHTTPClient("http://localhost:8080")
		Expect(checker).ToNot(BeNil())
	})

	Describe("Ping [readiness gate Wave 0]", func() {
		It("UT-FMC-HC-008: succeeds when /healthz responds 200", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal(fmc.HealthzPath))
				w.WriteHeader(http.StatusOK)
			}))
			client = fmc.NewHTTPClient(server.URL)

			Expect(client.Ping(context.Background())).To(Succeed())
		})

		It("UT-FMC-HC-009: returns an error (does not swallow it) when the endpoint is unreachable", func() {
			client = fmc.NewHTTPClient("http://127.0.0.1:1")
			err := client.Ping(context.Background())
			Expect(err).To(HaveOccurred(),
				"unlike IsManagedResource, Ping must surface the transport error for the readiness gate")
		})

		It("UT-FMC-HC-010: returns an error when /healthz responds with a non-200 status", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			client = fmc.NewHTTPClient(server.URL)

			err := client.Ping(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("503"))
		})
	})
})
