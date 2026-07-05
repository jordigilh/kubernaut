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
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"

	karbac "github.com/jordigilh/kubernaut/internal/kubernautagent/rbac"
	"github.com/jordigilh/kubernaut/pkg/fleet/readiness"
)

// fakeUnreadyProber is a readiness.Prober test double that always reports
// its dependency as unreachable, used to exercise the fleetGate branch of
// readinessHandler without depending on a real MCP Gateway connection.
type fakeUnreadyProber struct{}

func (fakeUnreadyProber) Probe(context.Context) error {
	return errors.New("fake: fleet dependency unreachable")
}

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

	handler := readinessHandler(&shutdownFlag, &apiServerReady, swappable, nil, interactive, nil)

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

	handler := readinessHandler(&shutdownFlag, &apiServerReady, swappable, nil, interactive, nil)

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

	handler := readinessHandler(&shutdownFlag, &apiServerReady, swappable, nil, interactive, nil)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("UT-KA-JWKS-003: expected 503 during shutdown, got %d", rec.Code)
	}
}

// UT-KA-1553-001: readinessHandler must report 503 when the injected Fleet
// readiness gate is NotReady, even though every other dependency (API
// server, LLM client, interactive mode) is healthy. This is the pod-wide
// fail-closed behavior mandated by ADR-068 decision #11 / BR-INTEGRATION-054
// (#1553): an unreachable Fleet MCP Gateway must remove the whole pod from
// Service endpoints, not just silently degrade fleet-scoped functionality.
func TestReadinessHandler_FleetGateNotReady_ReturnsUnavailable(t *testing.T) {
	var shutdownFlag, apiServerReady int32
	atomic.StoreInt32(&apiServerReady, 1)
	swappable := setupSwappable(t)
	interactive := karbac.NewInteractiveReadiness()

	gate := readiness.NewGate(time.Hour, logr.Discard(), fakeUnreadyProber{})
	gate.Start(context.Background())
	t.Cleanup(gate.Stop)

	handler := readinessHandler(&shutdownFlag, &apiServerReady, swappable, nil, interactive, gate)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("UT-KA-1553-001: expected 503 when the fleet readiness gate is NotReady, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

// UT-KA-1553-002: readinessHandler must report 200 when the injected Fleet
// readiness gate is nil (fleet mode not configured), matching the existing
// soft-dependency convention used for ds.
func TestReadinessHandler_NilFleetGate_DoesNotBlockReady(t *testing.T) {
	var shutdownFlag, apiServerReady int32
	atomic.StoreInt32(&apiServerReady, 1)
	swappable := setupSwappable(t)
	interactive := karbac.NewInteractiveReadiness()

	handler := readinessHandler(&shutdownFlag, &apiServerReady, swappable, nil, interactive, nil)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("UT-KA-1553-002: expected 200 when fleet mode is not configured (nil gate), got %d (body: %s)", rec.Code, rec.Body.String())
	}
}
