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

package audit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/jordigilh/kubernaut/pkg/audit"
	fleetreadiness "github.com/jordigilh/kubernaut/pkg/fleet/readiness"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ========================================
// DATASTORAGE READINESS PROBER UNIT TESTS
// BR-AUDIT-005 v2.0 | Issue #1985 | DD-AUDIT-003
// Test ID: UT-AUDIT-1985-001
//
// DataStorageProber lets every audit-writing service gate its own
// /readyz on DataStorage's real health endpoint (pkg/fleet/readiness.Gate/
// Prober, reused as-is -- no Fleet-specific coupling), closing the
// audit-loss window where a service starts serving traffic before
// DataStorage is reachable (#1985).
//
// Business-level assertion per tier (Pyramid Invariant): this UT proves
// the Probe *decision* in isolation against a real httptest.Server
// standing in for DataStorage's health endpoint -- not the wiring (that
// is proven at IT tier, IT-AUDIT-1985-003..012).
// ========================================

var _ = Describe("DataStorageProber (UT-AUDIT-1985-001, BR-AUDIT-005)", func() {
	var server *httptest.Server

	AfterEach(func() {
		if server != nil {
			server.Close()
			server = nil
		}
	})

	Context("when DataStorage's health endpoint is reachable and healthy", func() {
		It("reports the dependency ready (Probe returns nil)", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			prober := &audit.DataStorageProber{HealthURL: server.URL}

			err := prober.Probe(context.Background())

			Expect(err).NotTo(HaveOccurred(), "a healthy DataStorage health endpoint must report the dependency ready")
		})

		It("treats any 2xx status as healthy, not only 200", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))
			prober := &audit.DataStorageProber{HealthURL: server.URL}

			err := prober.Probe(context.Background())

			Expect(err).NotTo(HaveOccurred(), "204 No Content is a successful health response")
		})
	})

	Context("when DataStorage's health endpoint reports unhealthy", func() {
		It("fails closed when the health endpoint returns 503 (e.g. Postgres unreachable)", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			prober := &audit.DataStorageProber{HealthURL: server.URL}

			err := prober.Probe(context.Background())

			Expect(err).To(HaveOccurred(), "a 503 from DataStorage's readyz must fail the probe so the dependent service also reports NotReady")
			Expect(err.Error()).To(ContainSubstring("503"), "the error should surface the underlying status for operator diagnosis")
		})

		It("fails closed on any non-2xx status", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			prober := &audit.DataStorageProber{HealthURL: server.URL}

			err := prober.Probe(context.Background())

			Expect(err).To(HaveOccurred())
		})
	})

	Context("when DataStorage is completely unreachable", func() {
		It("fails closed when the connection is refused", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			unreachableURL := server.URL
			server.Close()
			server = nil // already closed, avoid double-close in AfterEach

			prober := &audit.DataStorageProber{HealthURL: unreachableURL}

			err := prober.Probe(context.Background())

			Expect(err).To(HaveOccurred(), "an unreachable DataStorage must fail closed, not be silently treated as ready")
		})

		It("fails closed when the probe exceeds its timeout", func() {
			blockCh := make(chan struct{})
			defer close(blockCh)
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case <-blockCh:
				case <-r.Context().Done():
				}
			}))
			prober := &audit.DataStorageProber{HealthURL: server.URL, Client: &http.Client{Timeout: 50 * time.Millisecond}}

			err := prober.Probe(context.Background())

			Expect(err).To(HaveOccurred(), "a health check that never responds must not hang the readiness Gate's probe cycle indefinitely")
		})
	})

	Context("construction defaults", func() {
		It("does not panic and still performs a real HTTP call when Client is left nil", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			prober := &audit.DataStorageProber{HealthURL: server.URL}

			Expect(func() { _ = prober.Probe(context.Background()) }).NotTo(Panic())
		})
	})

	Context("Prober interface contract (wiring precondition for #1985)", func() {
		It("satisfies pkg/fleet/readiness.Prober so it can be aggregated by an existing, proven Gate", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			var prober fleetreadiness.Prober = &audit.DataStorageProber{HealthURL: server.URL}

			Expect(prober.Probe(context.Background())).NotTo(HaveOccurred())
		})
	})
})
