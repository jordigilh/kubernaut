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

// Package health provides a shared health server for Kubernaut stateless services.
// Issue #753: Dedicated health probe port (:8081) aligns all services with
// CONFIG_STANDARDS.md and enables TLS on the API port without breaking kubelet probes.
package health

import (
	"net/http"
	"net/http/pprof"
	"time"
)

// NewHealthServer creates an http.Server on the given address with /healthz
// (liveness) and /readyz (readiness) endpoints. The server always serves
// plain HTTP — kubelet probes never need TLS.
//
// When enableProfiling is true, /debug/pprof/* handlers are registered for
// runtime profiling (CPU, heap, goroutine, trace). This follows the
// kube-apiserver --profiling pattern: enabled by default, opt-out for
// hardened environments. Profiling has zero overhead when not actively queried.
//
// Callers own the lifecycle: start in a goroutine, graceful shutdown alongside
// the main server.
func NewHealthServer(addr string, liveness, readiness http.HandlerFunc, enableProfiling bool) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", liveness)
	mux.HandleFunc("/readyz", readiness)

	if enableProfiling {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}

	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

// AlwaysReadyLiveness responds 200 OK unconditionally. Suitable for a
// bootstrap health server's /healthz endpoint (see NewBootstrapServer):
// kubelet's startupProbe/livenessProbe only need to know the process itself
// is alive and serving HTTP, not that its downstream dependencies are ready.
func AlwaysReadyLiveness(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// NotYetReady responds 503 Service Unavailable unconditionally. Suitable
// for a bootstrap health server's /readyz endpoint (see NewBootstrapServer):
// correctly reports NotReady while a service's blocking dependency wiring
// is still in progress.
func NotYetReady(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte("starting up"))
}

// NewBootstrapServer creates a minimal health server (AlwaysReadyLiveness on
// /healthz, NotYetReady on /readyz) for use BEFORE a service's real
// dependency wiring (fleet MCP Gateway client, cluster registry cache sync,
// etc.) completes and its real health server (NewHealthServer, gated on
// actual dependency readiness) can be built (DD-PLATFORM-009).
//
// Without this, a service whose main() constructs its blocking dependencies
// before starting any HTTP listener leaves its health port unbound for the
// full duration of that wiring -- which can run for minutes under a
// mcpclient.Resilient-style exponential backoff. kubelet's startupProbe can
// only observe "connection refused" during that window (indistinguishable
// from a hung process), so it eventually kills and restarts the pod,
// repeating indefinitely whenever the dependency wiring is consistently
// slower than the probe's budget -- confirmed during Issue #54/DD-TEST-015
// Fleet E2E validation for cmd/fleetmetadatacache (Gateway showed the same
// blocking-before-listening shape in the same run but didn't need this fix;
// see DD-PLATFORM-009's "Follow-up" note).
//
// Callers own the lifecycle: start the returned server in a goroutine
// immediately at the top of main, then Shutdown() it BEFORE starting the
// real health server (NewHealthServer, or a controller-runtime manager's
// HealthProbeBindAddress) on the same addr, to avoid a bind conflict.
func NewBootstrapServer(addr string) *http.Server {
	return NewHealthServer(addr, AlwaysReadyLiveness, NotYetReady, false)
}
