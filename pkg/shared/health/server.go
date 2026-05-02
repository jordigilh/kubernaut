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
	}
}
