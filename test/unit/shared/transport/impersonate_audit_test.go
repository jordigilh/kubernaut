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

package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/transport"
)

type capturedCall struct {
	Info transport.K8sCallInfo
}

type mockK8sCallAuditor struct {
	mu    sync.Mutex
	calls []capturedCall
}

func (m *mockK8sCallAuditor) AuditK8sCall(_ context.Context, info transport.K8sCallInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, capturedCall{Info: info})
}

func (m *mockK8sCallAuditor) getCalls() []capturedCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	dst := make([]capturedCall, len(m.calls))
	copy(dst, m.calls)
	return dst
}

var _ transport.K8sCallAuditor = (*mockK8sCallAuditor)(nil)

var _ = Describe("ImpersonatingRoundTripper Audit — #898, BR-INTERACTIVE-003, BR-AUDIT-005", func() {

	var (
		backend *httptest.Server
		auditor *mockK8sCallAuditor
		rt      http.RoundTripper
	)

	BeforeEach(func() {
		backend = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		auditor = &mockK8sCallAuditor{}
		rt = transport.NewImpersonatingRoundTripper(
			http.DefaultTransport,
			transport.WithAuditor(auditor),
		)
	})

	AfterEach(func() {
		backend.Close()
	})

	Describe("UT-KA-898-001: Impersonated K8s call emits audit event with correct fields", func() {
		It("should emit audit event with acting_user, resource, verb, namespace, resource_name, http_status_code", func() {
			ctx := transport.WithImpersonatedUser(
				context.Background(),
				"jane@example.com",
				[]string{"engineering", "sre"},
			)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, backend.URL+"/api/v1/namespaces/default/pods/my-pod", nil)
			Expect(err).NotTo(HaveOccurred())

			resp, err := rt.RoundTrip(req)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			calls := auditor.getCalls()
			Expect(calls).To(HaveLen(1), "expected exactly one audit call for impersonated request")

			info := calls[0].Info
			Expect(info.User).To(Equal("jane@example.com"))
			Expect(info.Groups).To(ConsistOf("engineering", "sre"))
			Expect(info.Verb).To(Equal("get"))
			Expect(info.Resource).To(Equal("pods"))
			Expect(info.Namespace).To(Equal("default"))
			Expect(info.ResourceName).To(Equal("my-pod"))
			Expect(info.StatusCode).To(Equal(http.StatusOK))
		})
	})

	Describe("UT-KA-898-002: No audit event for autonomous mode (no impersonation context)", func() {
		It("should not emit any audit event when no impersonation context is set", func() {
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, backend.URL+"/api/v1/namespaces/default/pods/my-pod", nil)
			Expect(err).NotTo(HaveOccurred())

			resp, err := rt.RoundTrip(req)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			Expect(auditor.getCalls()).To(BeEmpty(), "no audit event should be emitted for autonomous mode")
		})
	})

	Describe("UT-KA-898-004: Audit failure is fire-and-forget", func() {
		It("should return the HTTP response without error even when auditor panics internally", func() {
			panicAuditor := &panicK8sCallAuditor{}
			rtWithPanic := transport.NewImpersonatingRoundTripper(
				http.DefaultTransport,
				transport.WithAuditor(panicAuditor),
			)

			ctx := transport.WithImpersonatedUser(context.Background(), "alice@example.com", nil)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, backend.URL+"/api/v1/nodes", nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(func() {
				resp, roundTripErr := rtWithPanic.RoundTrip(req)
				Expect(roundTripErr).NotTo(HaveOccurred())
				resp.Body.Close()
			}).NotTo(Panic(), "audit panic must not propagate to RoundTrip caller")
		})
	})

	Describe("UT-KA-898-005: HTTP status code captured in audit event", func() {
		It("should capture 404 status code from upstream response", func() {
			notFoundBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}))
			defer notFoundBackend.Close()

			ctx := transport.WithImpersonatedUser(context.Background(), "bob@example.com", nil)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, notFoundBackend.URL+"/api/v1/namespaces/default/pods/missing-pod", nil)
			Expect(err).NotTo(HaveOccurred())

			resp, err := rt.RoundTrip(req)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			calls := auditor.getCalls()
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].Info.StatusCode).To(Equal(http.StatusNotFound))
		})

		It("should capture 500 status code from upstream response", func() {
			errorBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer errorBackend.Close()

			ctx := transport.WithImpersonatedUser(context.Background(), "bob@example.com", nil)
			req, err := http.NewRequestWithContext(ctx, http.MethodDelete, errorBackend.URL+"/api/v1/namespaces/prod/services/critical-svc", nil)
			Expect(err).NotTo(HaveOccurred())

			resp, err := rt.RoundTrip(req)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			calls := auditor.getCalls()
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].Info.StatusCode).To(Equal(http.StatusInternalServerError))
			Expect(calls[0].Info.Verb).To(Equal("delete"))
		})
	})
})

type panicK8sCallAuditor struct{}

func (p *panicK8sCallAuditor) AuditK8sCall(_ context.Context, _ transport.K8sCallInfo) {
	panic("simulated auditor panic")
}

var _ transport.K8sCallAuditor = (*panicK8sCallAuditor)(nil)
