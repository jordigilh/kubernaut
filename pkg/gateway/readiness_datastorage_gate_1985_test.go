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
	"sync"
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

// fakeProber1985 is a minimal readiness.Prober test double standing in for
// audit.DataStorageProber, mirroring fakeProber1553's mutex-guarded shape
// (readiness.Gate probes it from a background goroutine started by
// gate.Start while UT-GW-1985-003 mutates err mid-test to simulate
// DataStorage recovering).
type fakeProber1985 struct {
	mu  sync.Mutex
	err error
}

func (f *fakeProber1985) Probe(_ context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.err
}

func (f *fakeProber1985) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

// IT-AUDIT-1985-007: Gateway readiness must fail closed (pod-wide
// NotReady) when DataStorage is unreachable, unlike the Fleet-conditional
// gate (#1553) this is UNCONDITIONAL -- Gateway always writes audit
// (DD-AUDIT-003), so there is no "disabled" state to skip the check for.
// Proves the wiring point (pkg/gateway/server.go's readinessHandler), not
// just DataStorageProber's own branching logic (already proven at UT tier,
// UT-AUDIT-1985-001).
var _ = Describe("BR-AUDIT-005: Readiness DataStorage Dependency Gate (#1985)", func() {
	newReadyServer := func() *gwpkg.Server {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		server := gwpkg.NewMinimalServerForReadinessTest(GinkgoLogr, fakeClient)
		server.SetCacheReadyForTesting(true)
		return server
	}

	It("IT-AUDIT-1985-007a: readiness returns 503 when the DataStorage readiness gate reports NotReady", func() {
		server := newReadyServer()

		prober := &fakeProber1985{err: errors.New("DataStorage health endpoint unreachable: dial tcp: connection refused")}
		gate := readiness.NewGate(time.Hour, GinkgoLogr, prober)
		gate.Start(context.Background())
		defer gate.Stop()
		server.SetDataStorageReadinessGateForTesting(gate)

		handler := server.ReadinessHandler()
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		Expect(w.Code).To(Equal(http.StatusServiceUnavailable),
			"BR-AUDIT-005 / #1985: an unreachable DataStorage must flip the whole pod NotReady, closing the audit-loss window at the root")

		var errResp gwerrors.RFC7807Error
		Expect(json.Unmarshal(w.Body.Bytes(), &errResp)).To(Succeed())
		Expect(errResp.Detail).To(ContainSubstring("DataStorage"))
		Expect(errResp.Detail).To(ContainSubstring("connection refused"))
	})

	It("IT-AUDIT-1985-007b: readiness returns 200 once DataStorage recovers", func() {
		server := newReadyServer()

		prober := &fakeProber1985{err: errors.New("DataStorage health endpoint unreachable")}
		gate := readiness.NewGate(20*time.Millisecond, GinkgoLogr, prober)
		gate.Start(context.Background())
		defer gate.Stop()
		server.SetDataStorageReadinessGateForTesting(gate)

		handler := server.ReadinessHandler()

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		Expect(w.Code).To(Equal(http.StatusServiceUnavailable))

		prober.setErr(nil)

		Eventually(func() int {
			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			return w.Code
		}, "500ms", "10ms").Should(Equal(http.StatusOK),
			"BR-AUDIT-005: readiness must recover to 200 once DataStorage is reachable again")
	})

	It("IT-AUDIT-1985-007c: shutdown priority still takes precedence over a NotReady DataStorage gate", func() {
		server := newReadyServer()
		server.SetShuttingDownForTesting(true)

		prober := &fakeProber1985{err: errors.New("DataStorage health endpoint unreachable")}
		gate := readiness.NewGate(time.Hour, GinkgoLogr, prober)
		gate.Start(context.Background())
		defer gate.Stop()
		server.SetDataStorageReadinessGateForTesting(gate)

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
