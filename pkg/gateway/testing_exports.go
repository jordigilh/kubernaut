/*
Copyright 2025 Jordi Gil.

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

package gateway

// Test-only helpers. Separated from server.go to keep production code clean.
// These remain exported because the unit test package (test/unit/gateway)
// imports this package externally and needs access to them.

import (
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/fleet/readiness"
)

// SetCacheReadyForTesting allows tests to control the cache-ready state.
// Production code must use MarkCacheReady instead.
func (s *Server) SetCacheReadyForTesting(ready bool) {
	s.cacheReady.Store(ready)
}

// SetShuttingDownForTesting allows tests to control the shutdown state.
func (s *Server) SetShuttingDownForTesting(shuttingDown bool) {
	s.isShuttingDown.Store(shuttingDown)
}

// SetFleetReadinessGateForTesting allows tests to inject a fleet readiness
// gate. Production code must use SetFleetReadinessGate instead (#1553).
func (s *Server) SetFleetReadinessGateForTesting(gate *readiness.Gate) {
	s.fleetReadinessGate = gate
}

// NewMinimalServerForReadinessTest creates a lightweight Server suitable for
// readiness handler unit tests. If apiReaders are provided, the first is used
// as the apiReader for the K8s connectivity check; otherwise the K8s check
// will panic (acceptable for tests that return before reaching it).
func NewMinimalServerForReadinessTest(logger logr.Logger, apiReaders ...client.Reader) *Server {
	return &Server{
		logger:    logger,
		apiReader: firstReader(apiReaders),
	}
}

func firstReader(readers []client.Reader) client.Reader {
	if len(readers) > 0 {
		return readers[0]
	}
	return nil
}
