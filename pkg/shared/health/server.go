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
)

// NewHealthServer creates an http.Server on the given address with /healthz
// (liveness) and /readyz (readiness) endpoints. The server always serves
// plain HTTP — kubelet probes never need TLS.
//
// Callers own the lifecycle: start in a goroutine, graceful shutdown alongside
// the main server.
func NewHealthServer(addr string, liveness, readiness http.HandlerFunc) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", liveness)
	mux.HandleFunc("/readyz", readiness)
	return &http.Server{
		Addr:    addr,
		Handler: mux,
	}
}
