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

package gateway_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/jordigilh/kubernaut/pkg/fleet/readiness"
	gwpkg "github.com/jordigilh/kubernaut/pkg/gateway"
	gwerrors "github.com/jordigilh/kubernaut/pkg/gateway/errors"
)

// fakeProber1553 is a minimal readiness.Prober test double.
type fakeProber1553 struct{ err error }

func (f *fakeProber1553) Probe(_ context.Context) error { return f.err }

// BR-INTEGRATION-065 / #1553: Gateway readiness must fail closed
// (pod-wide NotReady) when a configured Fleet dependency (MCP Gateway or
// scope-check backend) is unreachable, instead of the previous fail-open
// behavior where a connection failure only logged an error and left
// /readyz unaffected.
//
// Test Plan: Wave 2 of the fail-closed Fleet readiness gate rollout (#1553).
var _ = Describe("BR-INTEGRATION-065: Readiness Fleet Dependency Gate (#1553)", func() {
	newReadyServer := func() *gwpkg.Server {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		server := gwpkg.NewMinimalServerForReadinessTest(GinkgoLogr, fakeClient)
		server.SetCacheReadyForTesting(true)
		return server
	}

	It("UT-GW-1553-001: readiness ignores fleet entirely when no gate is set (fleet disabled)", func() {
		server := newReadyServer()

		handler := server.ReadinessHandler()
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		Expect(w.Code).To(Equal(http.StatusOK))
	})

	It("UT-GW-1553-002: readiness returns 503 when the fleet readiness gate reports NotReady", func() {
		server := newReadyServer()

		prober := &fakeProber1553{err: errors.New("MCP Gateway unreachable: dial tcp: connection refused")}
		gate := readiness.NewGate(time.Hour, GinkgoLogr, prober)
		gate.Start(context.Background())
		defer gate.Stop()
		server.SetFleetReadinessGateForTesting(gate)

		handler := server.ReadinessHandler()
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		Expect(w.Code).To(Equal(http.StatusServiceUnavailable),
			"BR-INTEGRATION-065: an unreachable Fleet dependency must flip the whole pod NotReady")

		var errResp gwerrors.RFC7807Error
		Expect(json.Unmarshal(w.Body.Bytes(), &errResp)).To(Succeed())
		Expect(errResp.Detail).To(ContainSubstring("Fleet"))
		Expect(errResp.Detail).To(ContainSubstring("connection refused"))
	})

	It("UT-GW-1553-003: readiness returns 200 once the fleet dependency recovers", func() {
		server := newReadyServer()

		prober := &fakeProber1553{err: errors.New("MCP Gateway unreachable")}
		gate := readiness.NewGate(20*time.Millisecond, GinkgoLogr, prober)
		gate.Start(context.Background())
		defer gate.Stop()
		server.SetFleetReadinessGateForTesting(gate)

		handler := server.ReadinessHandler()

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		Expect(w.Code).To(Equal(http.StatusServiceUnavailable))

		prober.err = nil

		Eventually(func() int {
			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			return w.Code
		}, "500ms", "10ms").Should(Equal(http.StatusOK),
			"BR-INTEGRATION-065: readiness must recover to 200 once the Fleet dependency is reachable again")
	})

	It("UT-GW-1553-004: shutdown priority still takes precedence over a NotReady fleet gate", func() {
		server := newReadyServer()
		server.SetShuttingDownForTesting(true)

		prober := &fakeProber1553{err: errors.New("MCP Gateway unreachable")}
		gate := readiness.NewGate(time.Hour, GinkgoLogr, prober)
		gate.Start(context.Background())
		defer gate.Stop()
		server.SetFleetReadinessGateForTesting(gate)

		handler := server.ReadinessHandler()
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		Expect(w.Code).To(Equal(http.StatusServiceUnavailable))

		var errResp gwerrors.RFC7807Error
		Expect(json.Unmarshal(w.Body.Bytes(), &errResp)).To(Succeed())
		Expect(errResp.Detail).To(ContainSubstring("shutting down"))
	})
})
