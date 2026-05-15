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
	"context"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/transport"
)

var _ = Describe("ImpersonatingRoundTripper — #703, DD-AUTH-MCP-001", func() {

	Describe("Context helpers", func() {

		Describe("UT-KA-703-F01: WithImpersonatedUser stores user identity in context", func() {
			It("should return the stored username and groups from context", func() {
				ctx := transport.WithImpersonatedUser(context.Background(), "alice@company.com", []string{"engineering", "sre"})

				username, groups := transport.ImpersonatedUserFromContext(ctx)
				Expect(username).To(Equal("alice@company.com"))
				Expect(groups).To(ConsistOf("engineering", "sre"))
			})
		})

		Describe("UT-KA-703-F02: ImpersonatedUserFromContext returns empty for bare context", func() {
			It("should return empty username and nil groups when no impersonation set", func() {
				username, groups := transport.ImpersonatedUserFromContext(context.Background())
				Expect(username).To(BeEmpty())
				Expect(groups).To(BeNil())
			})
		})

		Describe("UT-KA-703-F03: WithImpersonatedUser accepts nil groups", func() {
			It("should store username with nil groups", func() {
				ctx := transport.WithImpersonatedUser(context.Background(), "bob@company.com", nil)

				username, groups := transport.ImpersonatedUserFromContext(ctx)
				Expect(username).To(Equal("bob@company.com"))
				Expect(groups).To(BeNil())
			})
		})
	})

	Describe("RoundTripper behavior", func() {

		var (
			capturedHeaders http.Header
			backend         *httptest.Server
			rt              http.RoundTripper
		)

		BeforeEach(func() {
			capturedHeaders = nil
			backend = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedHeaders = r.Header.Clone()
				w.WriteHeader(http.StatusOK)
			}))
			rt = transport.NewImpersonatingRoundTripper(http.DefaultTransport)
		})

		AfterEach(func() {
			backend.Close()
		})

		Describe("UT-KA-703-F04: Injects Impersonate-User header when context has user", func() {
			It("should add the impersonation header to the request", func() {
				ctx := transport.WithImpersonatedUser(context.Background(), "alice@company.com", []string{"engineering"})
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, backend.URL, nil)
				Expect(err).NotTo(HaveOccurred())

				resp, err := rt.RoundTrip(req)
				Expect(err).NotTo(HaveOccurred())
				resp.Body.Close()

				Expect(capturedHeaders.Get("Impersonate-User")).To(Equal("alice@company.com"))
			})
		})

		Describe("UT-KA-703-F05: Injects Impersonate-Group headers for all groups", func() {
			It("should add one Impersonate-Group header per group", func() {
				ctx := transport.WithImpersonatedUser(context.Background(), "alice@company.com", []string{"engineering", "sre", "oncall"})
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, backend.URL, nil)
				Expect(err).NotTo(HaveOccurred())

				resp, err := rt.RoundTrip(req)
				Expect(err).NotTo(HaveOccurred())
				resp.Body.Close()

				Expect(capturedHeaders.Values("Impersonate-Group")).To(ConsistOf("engineering", "sre", "oncall"))
			})
		})

		Describe("UT-KA-703-F06: No impersonation headers when context has no user", func() {
			It("should not add any Impersonate-* headers for bare context", func() {
				req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, backend.URL, nil)
				Expect(err).NotTo(HaveOccurred())

				resp, err := rt.RoundTrip(req)
				Expect(err).NotTo(HaveOccurred())
				resp.Body.Close()

				Expect(capturedHeaders.Get("Impersonate-User")).To(BeEmpty())
				Expect(capturedHeaders.Values("Impersonate-Group")).To(BeEmpty())
			})
		})

		Describe("UT-KA-703-F07: Does not mutate original request", func() {
			It("should clone the request before adding headers", func() {
				ctx := transport.WithImpersonatedUser(context.Background(), "alice@company.com", []string{"engineering"})
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, backend.URL, nil)
				Expect(err).NotTo(HaveOccurred())

				resp, err := rt.RoundTrip(req)
				Expect(err).NotTo(HaveOccurred())
				resp.Body.Close()

				Expect(req.Header.Get("Impersonate-User")).To(BeEmpty(),
					"original request must not be mutated")
			})
		})

		Describe("UT-KA-703-F08: Preserves existing request headers", func() {
			It("should not remove pre-existing headers from the request", func() {
				ctx := transport.WithImpersonatedUser(context.Background(), "alice@company.com", nil)
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, backend.URL, nil)
				Expect(err).NotTo(HaveOccurred())
				req.Header.Set("X-Custom-Header", "preserved-value")

				resp, err := rt.RoundTrip(req)
				Expect(err).NotTo(HaveOccurred())
				resp.Body.Close()

				Expect(capturedHeaders.Get("X-Custom-Header")).To(Equal("preserved-value"))
				Expect(capturedHeaders.Get("Impersonate-User")).To(Equal("alice@company.com"))
			})
		})

		Describe("UT-KA-703-F09: Delegates to underlying transport", func() {
			It("should propagate transport errors from the delegate", func() {
				ctx := transport.WithImpersonatedUser(context.Background(), "alice@company.com", nil)
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:1", nil)
				Expect(err).NotTo(HaveOccurred())

				_, err = rt.RoundTrip(req)
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
