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

package server_test

import (
	"time"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
)

// testHarness gives IT-WIRE-SIGTERM (sigterm_test.go) a bare session.Manager
// to drive Shutdown() against.
//
// #2190: previously built the entire retired HTTP route stack (chi router,
// ogen handler, rate limiter, auth middleware) via httptest.Server, even
// though this test never touches h.Server -- only h.Manager. Now that the
// HTTP transport is gone (pkg/agentclient, internal/kubernautagent/server's
// ogen Handler), this harness shrinks to exactly what the test exercises:
// Manager.Shutdown()'s effect on in-flight investigations.
type testHarness struct {
	Manager *session.Manager
}

// newTestHarness builds a session.Manager backed by a fresh in-memory Store
// and a no-op audit sink (this test asserts on session state, not audit
// events).
func newTestHarness() *testHarness {
	store := session.NewStore(30 * time.Minute)
	mgr := session.NewManager(store, logr.Discard(), audit.NopAuditStore{}, nil)
	return &testHarness{Manager: mgr}
}

// Close is a no-op today (no external resources to release) but kept so
// sigterm_test.go's `defer h.Close()` needs no change if the harness grows
// resources again later.
func (h *testHarness) Close() {}
