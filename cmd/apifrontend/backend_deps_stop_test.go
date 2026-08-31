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
	"testing"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

// spyClusterRegistry is a minimal registry.ClusterRegistry test double that
// only tracks whether Stop() was called. It never actually starts a watcher,
// so it carries no goroutine/channel of its own to leak -- its purpose is to
// prove *stopBackendDeps calls Stop()*, not to re-test EAIGWRegistry's own
// Stop() behavior (already covered by pkg/fleet/registry's own test suite).
type spyClusterRegistry struct {
	stopCalled bool
}

func (s *spyClusterRegistry) List() []registry.ClusterInfo { return nil }
func (s *spyClusterRegistry) Get(string) (registry.ClusterInfo, bool) {
	return registry.ClusterInfo{}, false
}
func (s *spyClusterRegistry) WatchClusters() <-chan registry.ClusterEvent { return nil }
func (s *spyClusterRegistry) Ready() bool                                 { return true }
func (s *spyClusterRegistry) Start(context.Context) error                 { return nil }
func (s *spyClusterRegistry) Stop()                                       { s.stopCalled = true }

// TestStopBackendDeps_StopsFleetClusterRegistry (Issue #2313, BR-FLEET-054,
// GA Readiness Dimension 12): stopBackendDeps is AF's only shutdown path for
// backendDeps -- every lifecycle-owning field must be released there or it
// leaks for the life of the process restart cycle. FleetClusterRegistry
// backs a live K8s dynamic-informer goroutine (EAIGWRegistry/KuadrantRegistry)
// once fleet mode is enabled; failing to call its Stop() here leaks that
// goroutine plus its unclosed eventCh/stopCh channels on every AF shutdown,
// unlike every sibling field (fleetReadinessGate, dataStorageReadinessGate,
// FleetResilientClient) which are already stopped/closed below.
func TestStopBackendDeps_StopsFleetClusterRegistry(t *testing.T) {
	t.Parallel()

	spy := &spyClusterRegistry{}
	deps := &backendDeps{FleetClusterRegistry: spy}

	stopBackendDeps(deps, logr.Discard())

	if !spy.stopCalled {
		t.Error("Issue #2313: stopBackendDeps must call FleetClusterRegistry.Stop() on shutdown -- " +
			"otherwise the registry's background informer goroutine and its eventCh/stopCh channels " +
			"leak for the life of the process, unlike every other lifecycle-owning backendDeps field")
	}
}

// TestStopBackendDeps_NilFleetClusterRegistry_NoPanic pins the existing
// nil-safe convention every other backendDeps field follows (fleet disabled
// -> nil field -> stopBackendDeps must still be safe to call unconditionally
// via defer, per its own doc comment).
func TestStopBackendDeps_NilFleetClusterRegistry_NoPanic(t *testing.T) {
	t.Parallel()

	deps := &backendDeps{FleetClusterRegistry: nil}

	stopBackendDeps(deps, logr.Discard())
}
