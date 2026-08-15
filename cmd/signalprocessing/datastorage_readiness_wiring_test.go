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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/signalprocessing/config"
)

// IT-AUDIT-1985-007: cmd/signalprocessing must wire an independent,
// always-on readiness.Gate carrying a DataStorageProber into
// mgr.AddReadyzCheck("datastorage", ...) -- the actual production entry
// point -- so that SignalProcessing's own /readyz fails closed while
// DataStorage is unreachable (#1985, BR-AUDIT-005 v2.0), instead of
// accepting traffic (and generating audit events that would be silently
// lost) before DataStorage is confirmed reachable.

func TestWireDataStorageReadinessGate_SP_Reachable_ReadyImmediately(t *testing.T) {
	ds := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ds.Close)

	cfg := config.DefaultConfig()
	cfg.DataStorage.HealthURL = ds.URL

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	gate := wireDataStorageReadinessGate(ctx, cfg, logr.Discard())
	if gate == nil {
		t.Fatal("IT-AUDIT-1985-007a: DataStorage readiness gate must always be wired (unconditional, unlike the Fleet gate)")
	}
	t.Cleanup(gate.Stop)

	if err := gate.Check(httptest.NewRequest("GET", "/readyz", nil)); err != nil {
		t.Fatalf("IT-AUDIT-1985-007a: gate must report ready immediately after Start when DataStorage's health endpoint is reachable, got error: %v", err)
	}
}

func TestWireDataStorageReadinessGate_SP_Unreachable_NotReady(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DataStorage.HealthURL = "http://127.0.0.1:1/unreachable"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gate := wireDataStorageReadinessGate(ctx, cfg, logr.Discard())
	if gate == nil {
		t.Fatal("IT-AUDIT-1985-007b: gate must still be wired (and report NotReady) when DataStorage is currently unreachable")
	}
	t.Cleanup(gate.Stop)

	if err := gate.Check(httptest.NewRequest("GET", "/readyz", nil)); err == nil {
		t.Fatal("BR-AUDIT-005 / #1985: gate must report NotReady when DataStorage's health endpoint is unreachable, " +
			"so Kubernetes removes the pod from Service endpoints (pod-wide fail closed) before any audit event can be lost")
	}
}
