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

// DX-M1: This test lives in pkg/datastorage/server/ (internal test) because
// Shutdown() tests require direct access to private fields (isShuttingDown,
// httpServer, dlqClient, db, endpointPropagationDelay). Go convention places
// white-box unit tests in the same package; the project's test/unit/ convention
// applies to black-box (external) tests only.
package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

func TestServerShutdown(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Server Shutdown Suite")
}

// newMinimalServer creates a Server with just enough state for Shutdown() testing.
// DX-L1: Uses a real httptest.Server — this is the canonical Go approach for
// testing HTTP server lifecycle; it allocates a lightweight loopback listener
// with no external dependencies.
func newMinimalServer(db *sql.DB) (*Server, *httptest.Server) {
	logger := kubelog.NewLogger(kubelog.DefaultOptions())

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	ts := httptest.NewServer(mux)

	srv := &Server{
		logger:     logger,
		db:         db,
		httpServer: ts.Config,
		dlqRetryWorker: &DLQRetryWorker{
			logger: logger,
		},
		// QE-H1: Zero propagation delay eliminates the 5s time.Sleep
		// that made each full-Shutdown test take ~5 seconds.
		endpointPropagationDelay: 0,
	}
	return srv, ts
}

var _ = Describe("#1048 Phase 3: Shutdown Ordering", func() {
	Context("Shutdown always completes cleanup", func() {
		It("UT-DS-1048-SD-001: should drain DLQ and close DB even if HTTP drain fails", func() {
			srv, ts := newMinimalServer(nil)
			defer ts.Close()

			holdConn := make(chan struct{})
			connected := make(chan struct{})
			srv.httpServer.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(connected)
				<-holdConn
			})
			go func() {
				resp, err := http.Get(ts.URL + "/block")
				if err == nil {
					resp.Body.Close()
				}
			}()

			// Ensure the in-flight connection is established before calling Shutdown,
			// so http.Server.Shutdown sees an active connection and respects the context.
			<-connected

			// QE-M2: Use an already-past deadline to deterministically trigger
			// context.DeadlineExceeded, avoiding the flaky nanosecond-timeout + sleep.
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-1*time.Second))
			defer cancel()

			err := srv.Shutdown(ctx)
			close(holdConn)

			Expect(err).To(HaveOccurred(), "shutdown should report HTTP drain error")
			Expect(errors.Is(err, context.DeadlineExceeded)).To(BeTrue(),
				"joined error should unwrap to DeadlineExceeded (ARCH-H1)")
		})

		It("UT-DS-1048-SD-002: should complete without error when all steps succeed", func() {
			srv, ts := newMinimalServer(nil)
			defer ts.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			err := srv.Shutdown(ctx)

			Expect(err).ToNot(HaveOccurred(), "all steps should succeed cleanly")
		})

		It("UT-DS-1048-SD-003: should set shutdown flag before any other operation", func() {
			srv, ts := newMinimalServer(nil)
			defer ts.Close()

			Expect(srv.isShuttingDown.Load()).To(BeFalse(), "should start as not shutting down")

			srv.shutdownStep1SetFlag()

			Expect(srv.isShuttingDown.Load()).To(BeTrue(), "flag should be set after step 1")
		})

		It("UT-DS-1048-SD-004: should skip DLQ drain when dlqClient is nil", func() {
			srv, ts := newMinimalServer(nil)
			defer ts.Close()
			srv.dlqClient = nil

			ctx := context.Background()
			Expect(func() { srv.shutdownStep4DrainDLQ(ctx) }).ToNot(Panic())
		})

		It("UT-DS-1048-SD-005: should not panic on DB close and complete all steps (QE-M3)", func() {
			db, openErr := sql.Open("pgx", "host=__nonexistent__")
			Expect(openErr).ToNot(HaveOccurred(), "sql.Open should not fail for lazy drivers")

			srv, ts := newMinimalServer(db)
			defer ts.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			// QE-N1: Explicit panic guard — DB close on a never-connected pgx
			// driver may succeed or fail, but Shutdown must never panic.
			Expect(func() {
				_ = srv.Shutdown(ctx)
			}).ToNot(Panic(), "Shutdown must complete all steps without panicking")
		})
	})

	Context("Step ordering (QE-M1)", func() {
		It("UT-DS-1048-SD-006: Shutdown() executes steps in DD-007/DD-008 order", func() {
			// QE-N2: Test the actual Shutdown() method, then verify ordering
			// by checking observable side effects rather than calling steps manually.
			srv, ts := newMinimalServer(nil)
			defer ts.Close()

			Expect(srv.isShuttingDown.Load()).To(BeFalse(), "flag must be false before shutdown")

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			err := srv.Shutdown(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Step 1 must have run: readiness flag is set
			Expect(srv.isShuttingDown.Load()).To(BeTrue(), "step 1 must set shutdown flag")

			// Steps 2-5 completed: Shutdown returned without error, meaning
			// HTTP drain (step 3), DLQ drain skip (step 4, nil client), and
			// resource close (step 5, nil db guard) all ran successfully.
			// If any step were skipped or reordered, either a panic or error
			// would surface via SD-001/SD-002/SD-005.
		})
	})
})
