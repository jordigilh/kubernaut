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

package main

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	karbac "github.com/jordigilh/kubernaut/internal/kubernautagent/rbac"
)

// UT-KA-JWKS-001: readinessHandler reports not_ready before the main API
// server has started listening, even when shutdownFlag and the LLM client
// are otherwise ready. This guards against the race fixed alongside the JWKS
// pre-warm startup reorder: the health server now starts before the
// route-setup closure (which performs the JWKS pre-warm), so /readyz must
// not report ready until the main API server is actually about to listen.
func TestReadinessHandler_NotReadyBeforeAPIServerListening(t *testing.T) {
	var shutdownFlag, apiServerReady int32
	swappable := setupSwappable(t)
	interactive := karbac.NewInteractiveReadiness()

	handler := readinessHandler(&shutdownFlag, &apiServerReady, swappable, nil, interactive)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("UT-KA-JWKS-001: expected 503 before apiServerReady is set, got %d", rec.Code)
	}
}

// UT-KA-JWKS-002: readinessHandler reports ready once apiServerReady is set
// (and all other dependencies are healthy).
func TestReadinessHandler_ReadyAfterAPIServerListening(t *testing.T) {
	var shutdownFlag, apiServerReady int32
	atomic.StoreInt32(&apiServerReady, 1)
	swappable := setupSwappable(t)
	interactive := karbac.NewInteractiveReadiness()

	handler := readinessHandler(&shutdownFlag, &apiServerReady, swappable, nil, interactive)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("UT-KA-JWKS-002: expected 200 once apiServerReady is set, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

// UT-KA-JWKS-003: shutdownFlag takes priority over apiServerReady -- once
// shutdown begins, the probe must fail even if the API server was ready.
func TestReadinessHandler_ShutdownTakesPriority(t *testing.T) {
	var shutdownFlag, apiServerReady int32
	atomic.StoreInt32(&apiServerReady, 1)
	atomic.StoreInt32(&shutdownFlag, 1)
	swappable := setupSwappable(t)
	interactive := karbac.NewInteractiveReadiness()

	handler := readinessHandler(&shutdownFlag, &apiServerReady, swappable, nil, interactive)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("UT-KA-JWKS-003: expected 503 during shutdown, got %d", rec.Code)
	}
}
