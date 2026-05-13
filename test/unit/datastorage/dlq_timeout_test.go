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

package datastorage

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
)

// ========================================
// PHASE 6: DLQ READ TIMEOUT TESTS (TP-1088-P1)
// ========================================
//
// Issue: #1088 Phase 6 (Performance)
// File Under Test: pkg/datastorage/server/dlq_retry_worker.go:219
// TDD Phase: RED — tests verify the worker uses a bounded read timeout
//
// Current implementation: ReadMessages is called with timeout = -1 (infinite block).
// Expected: ReadMessages should use a configurable positive timeout (e.g., 5s)
// so the worker can respond to context cancellation and PEL janitor ticks.
//
// Stub: ReadTimeout field added to DLQRetryWorkerConfig (zero value).
// DefaultDLQRetryWorkerConfig() does not set it yet → test FAILS.
//
// ========================================

var _ = Describe("Phase 6: DLQ Read Timeout (TP-1088-P1)", func() {

	Describe("DLQRetryWorkerConfig", func() {

		It("UT-DS-1088-P6-010: default config must set a positive ReadTimeout", func() {
			// RED: DefaultDLQRetryWorkerConfig() does not set ReadTimeout.
			// The zero value (0s) means the worker would use -1 (infinite block)
			// for XREADGROUP, preventing periodic PEL recovery checks.

			cfg := server.DefaultDLQRetryWorkerConfig()

			Expect(cfg.ReadTimeout).To(BeNumerically(">", 0),
				"ReadTimeout must be positive to prevent indefinite blocking on XREADGROUP; "+
					"recommended: 5s (allows PEL recovery every tick)")
		})

		It("UT-DS-1088-P6-011: ReadTimeout must not exceed PollInterval", func() {
			// RED: ReadTimeout is 0 (not yet wired), so this trivially passes
			// unless we also assert it's positive. Paired with P6-010 above.

			cfg := server.DefaultDLQRetryWorkerConfig()

			Expect(cfg.ReadTimeout).To(BeNumerically(">", 0),
				"ReadTimeout must be set before comparing to PollInterval")
			Expect(cfg.ReadTimeout).To(BeNumerically("<=", cfg.PollInterval),
				"ReadTimeout must not exceed PollInterval to ensure tick responsiveness")
		})

		It("UT-DS-1088-P6-012: ReadTimeout should be 5 seconds for balanced throughput", func() {
			// RED: ReadTimeout is 0 (stub). GREEN will set it to 5s.

			cfg := server.DefaultDLQRetryWorkerConfig()

			Expect(cfg.ReadTimeout).To(Equal(5*time.Second),
				"5s read timeout balances message latency with shutdown responsiveness")
		})
	})
})
