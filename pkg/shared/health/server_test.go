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

package health

import (
	"context"
	"io"
	"net"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// listenOnFreePort starts a TCP listener on an OS-assigned loopback port,
// returning it alongside its address, so parallel test runs never collide
// on a fixed port.
func listenOnFreePort() (net.Listener, string) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).ToNot(HaveOccurred())
	return ln, ln.Addr().String()
}

// DD-PLATFORM-009 (#1755): NewBootstrapServer is the sole mechanism by
// which a service's health port answers *anything* during blocking
// dependency wiring (mcpclient.NewResilient, cluster registry cache sync)
// that runs before the service's real health server binds. These IT tests
// exercise the real production wiring helper end-to-end (real listener,
// real HTTP round trip), not a re-implementation -- proving the exact
// contract cmd/fleetmetadatacache/main.go and cmd/gateway/main.go depend on.
var _ = Describe("NewBootstrapServer (DD-PLATFORM-009)", func() {
	It("IT-HEALTH-009-001: /healthz reports 200 OK unconditionally, so kubelet's startupProbe/livenessProbe see a live process during blocking dependency wiring", func() {
		ln, addr := listenOnFreePort()
		srv := NewBootstrapServer(addr)
		go func() { _ = srv.Serve(ln) }()
		defer func() { _ = srv.Close() }()

		resp, err := http.Get("http://" + addr + "/healthz") //nolint:gosec,noctx // test-only probe against a loopback bootstrap server
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		body, err := io.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())
		Expect(string(body)).To(Equal("ok"))
	})

	It("IT-HEALTH-009-002: /readyz honestly reports 503 unconditionally, distinguishing 'still wiring' from a genuinely dead process", func() {
		ln, addr := listenOnFreePort()
		srv := NewBootstrapServer(addr)
		go func() { _ = srv.Serve(ln) }()
		defer func() { _ = srv.Close() }()

		resp, err := http.Get("http://" + addr + "/readyz") //nolint:gosec,noctx // test-only probe against a loopback bootstrap server
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable),
			"a bootstrap server must never claim readiness before real dependency wiring completes")
		body, err := io.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())
		Expect(string(body)).To(Equal("starting up"))
	})

	It("IT-HEALTH-009-003: Shutdown() frees the port so the real health server (NewHealthServer) can bind the same address without conflict", func() {
		ln, addr := listenOnFreePort()
		srv := NewBootstrapServer(addr)
		go func() { _ = srv.Serve(ln) }()

		// Confirm the bootstrap server is actually up before tearing it down.
		Eventually(func() (int, error) {
			resp, err := http.Get("http://" + addr + "/healthz") //nolint:gosec,noctx // test-only probe
			if err != nil {
				return 0, err
			}
			defer func() { _ = resp.Body.Close() }()
			return resp.StatusCode, nil
		}, "2s", "10ms").Should(Equal(http.StatusOK))

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		Expect(srv.Shutdown(shutdownCtx)).To(Succeed())

		// The real handoff pattern (cmd/fleetmetadatacache/main.go,
		// cmd/gateway/main.go): after Shutdown(), a fresh server built by
		// NewHealthServer must be able to bind the exact same address.
		liveness := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
		readiness := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
		realServer := NewHealthServer(addr, liveness, readiness, false)
		realLn, err := net.Listen("tcp", addr)
		Expect(err).ToNot(HaveOccurred(), "the bootstrap server's port must be fully released after Shutdown()")
		go func() { _ = realServer.Serve(realLn) }()
		defer func() { _ = realServer.Close() }()

		Eventually(func() (int, error) {
			resp, err := http.Get("http://" + addr + "/readyz") //nolint:gosec,noctx // test-only probe
			if err != nil {
				return 0, err
			}
			defer func() { _ = resp.Body.Close() }()
			return resp.StatusCode, nil
		}, "2s", "10ms").Should(Equal(http.StatusOK),
			"the real health server's /readyz must now answer 200, proving the handoff left no bind conflict or stale state")
	})
})
