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
	"io"
	"net/http"
	"net/http/httptest"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/transport"
)

type integrationAuditor struct {
	mu    sync.Mutex
	calls []transport.K8sCallInfo
}

func (a *integrationAuditor) AuditK8sCall(_ context.Context, info transport.K8sCallInfo) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = append(a.calls, info)
}

func (a *integrationAuditor) getCalls() []transport.K8sCallInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	dst := make([]transport.K8sCallInfo, len(a.calls))
	copy(dst, a.calls)
	return dst
}

var _ = Describe("IT-KA-898: ImpersonatingRoundTripper audit integration — BR-INTERACTIVE-003", func() {

	Describe("IT-KA-898-001: K8sCallAuditor is wired into ImpersonatingRoundTripper", func() {
		It("should accept the auditor via WithAuditor and make it available during RoundTrip", func() {
			auditor := &integrationAuditor{}
			rt := transport.NewImpersonatingRoundTripper(
				http.DefaultTransport,
				transport.WithAuditor(auditor),
			)

			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer backend.Close()

			ctx := transport.WithImpersonatedUser(context.Background(), "test@example.com", []string{"team-a"})
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, backend.URL+"/api/v1/namespaces/test-ns/pods/test-pod", nil)
			Expect(err).NotTo(HaveOccurred())

			resp, err := rt.RoundTrip(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			calls := auditor.getCalls()
			Expect(calls).To(HaveLen(1), "auditor should have been invoked exactly once")
			Expect(calls[0].User).To(Equal("test@example.com"))
		})
	})

	Describe("IT-KA-898-002: Full round-trip: impersonated HTTP request -> delegate -> audit event", func() {
		It("should perform a real HTTP round-trip and capture all audit fields", func() {
			var capturedHeaders http.Header
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedHeaders = r.Header.Clone()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(w, `{"kind":"Service","metadata":{"name":"my-svc"}}`)
			}))
			defer backend.Close()

			auditor := &integrationAuditor{}
			rt := transport.NewImpersonatingRoundTripper(
				http.DefaultTransport,
				transport.WithAuditor(auditor),
			)

			ctx := transport.WithImpersonatedUser(
				context.Background(),
				"operator@corp.io",
				[]string{"platform-team", "oncall"},
			)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, backend.URL+"/api/v1/namespaces/production/services", nil)
			Expect(err).NotTo(HaveOccurred())

			resp, err := rt.RoundTrip(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			By("verifying impersonation headers were injected")
			Expect(capturedHeaders.Get("Impersonate-User")).To(Equal("operator@corp.io"))
			Expect(capturedHeaders.Values("Impersonate-Group")).To(ConsistOf("platform-team", "oncall"))

			By("verifying HTTP response is returned unchanged")
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))

			By("verifying audit event was captured with correct fields")
			calls := auditor.getCalls()
			Expect(calls).To(HaveLen(1))
			info := calls[0]
			Expect(info.User).To(Equal("operator@corp.io"))
			Expect(info.Groups).To(ConsistOf("platform-team", "oncall"))
			Expect(info.Verb).To(Equal("create"))
			Expect(info.Resource).To(Equal("services"))
			Expect(info.Namespace).To(Equal("production"))
			Expect(info.ResourceName).To(BeEmpty(), "POST creates — no resource name in URL")
			Expect(info.StatusCode).To(Equal(http.StatusCreated))
		})
	})
})
