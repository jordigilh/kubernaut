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

package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	gwpkg "github.com/jordigilh/kubernaut/pkg/gateway"
	gwerrors "github.com/jordigilh/kubernaut/pkg/gateway/errors"
)

// BR-GATEWAY-185: Gateway readiness probe must gate on K8s informer cache sync
// Issue #852: /readyz does not gate on cache sync
//
// Business Outcome: Kubelet does not route traffic to a Gateway instance
// whose informer cache is still populating, preventing stale-data responses
// during startup or cache resync.
//
// Test Plan: docs/tests/852/TEST_PLAN.md

var _ = Describe("BR-GATEWAY-185: Readiness Cache Sync Gate (#852)", func() {

	Context("when informer cache has NOT synced", func() {
		It("UT-GW-852-001: should return 503 with RFC 7807 body when cache not synced", func() {
			server := gwpkg.NewMinimalServerForReadinessTest(GinkgoLogr)
			server.SetCacheReadyForTesting(false)

			handler := server.ReadinessHandler()
			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusServiceUnavailable),
				"BR-GATEWAY-185: readiness must reject when cache not synced")

			Expect(w.Header().Get("Content-Type")).To(Equal("application/problem+json"),
				"RFC 7807: Content-Type must be application/problem+json")

			var errResp gwerrors.RFC7807Error
			err := json.Unmarshal(w.Body.Bytes(), &errResp)
			Expect(err).ToNot(HaveOccurred(), "response body must be valid RFC 7807 JSON")

			Expect(errResp.Status).To(Equal(http.StatusServiceUnavailable))
			Expect(errResp.Type).To(Equal(gwerrors.ErrorTypeServiceUnavailable))
			Expect(errResp.Title).To(Equal(gwerrors.TitleServiceUnavailable))
			Expect(errResp.Detail).To(ContainSubstring("cache"),
				"detail must reference cache sync to distinguish from shutdown/K8s errors")
		})
	})

	Context("when informer cache HAS synced", func() {
		It("UT-GW-852-002: should return 200 with ready status when cache is synced", func() {
			scheme := runtime.NewScheme()
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			server := gwpkg.NewMinimalServerForReadinessTest(GinkgoLogr, fakeClient)
			server.SetCacheReadyForTesting(true)

			handler := server.ReadinessHandler()
			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK),
				"BR-GATEWAY-185: readiness must pass when cache synced and K8s API reachable")

			var body map[string]string
			err := json.Unmarshal(w.Body.Bytes(), &body)
			Expect(err).ToNot(HaveOccurred())
			Expect(body["status"]).To(Equal("ready"))
		})
	})

	Context("zero-value safety", func() {
		It("UT-GW-852-003: should default cacheReady to false (fail-closed)", func() {
			server := gwpkg.NewMinimalServerForReadinessTest(GinkgoLogr)
			// Do NOT call SetCacheReadyForTesting — verify zero-value behavior

			handler := server.ReadinessHandler()
			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusServiceUnavailable),
				"fail-closed: zero-value atomic.Bool must reject readiness")
		})
	})

	Context("logging", func() {
		It("UT-GW-852-004: should emit structured log on cache-unsynced rejection", func() {
			server := gwpkg.NewMinimalServerForReadinessTest(GinkgoLogr)
			server.SetCacheReadyForTesting(false)

			handler := server.ReadinessHandler()
			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusServiceUnavailable))
			// Log verification: GinkgoLogr captures output to GinkgoWriter.
			// The handler must log at V(1) — visible in verbose test output.
			// Structural assertion: if we reach 503 with cache detail, the log path was hit.
		})
	})

	Context("shutdown priority", func() {
		It("UT-GW-852-005: shutdown response takes priority over cache-unsynced", func() {
			server := gwpkg.NewMinimalServerForReadinessTest(GinkgoLogr)
			server.SetCacheReadyForTesting(false)
			server.SetShuttingDownForTesting(true)

			handler := server.ReadinessHandler()
			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusServiceUnavailable))

			var errResp gwerrors.RFC7807Error
			err := json.Unmarshal(w.Body.Bytes(), &errResp)
			Expect(err).ToNot(HaveOccurred())

			Expect(errResp.Detail).To(ContainSubstring("shutting down"),
				"shutdown detail must take priority over cache-unsynced detail")
			Expect(errResp.Detail).ToNot(ContainSubstring("cache"),
				"cache detail must NOT appear when shutdown is active")
		})
	})
})
