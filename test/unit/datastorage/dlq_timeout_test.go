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
// PHASE 6: DLQ READ TIMEOUT — BEHAVIORAL TESTS (TP-1088-P1)
// ========================================
//
// Issue: #1088 Phase 6 (Performance)
// File Under Test: pkg/datastorage/server/dlq_retry_worker.go
//
// These tests verify DefaultDLQRetryWorkerConfig returns a config
// with bounded ReadTimeout properties. The ReadTimeout controls the
// XREADGROUP block duration, preventing indefinite blocking.
//
// ========================================

var _ = Describe("Phase 6: DLQ Read Timeout (TP-1088-P1)", func() {

	Describe("DefaultDLQRetryWorkerConfig ReadTimeout", func() {

		It("UT-DS-1088-P6-010: default config sets ReadTimeout to 5s", func() {
			cfg := server.DefaultDLQRetryWorkerConfig()

			Expect(cfg.ReadTimeout).To(Equal(5*time.Second),
				"ReadTimeout must be 5s: balances message latency with shutdown responsiveness")
		})

		It("UT-DS-1088-P6-011: ReadTimeout must not exceed PollInterval", func() {
			cfg := server.DefaultDLQRetryWorkerConfig()

			Expect(cfg.ReadTimeout).To(BeNumerically("<=", cfg.PollInterval),
				"ReadTimeout must not exceed PollInterval to ensure tick responsiveness")
		})

		It("UT-DS-1088-P6-012: ReadTimeout must be positive for bounded XREADGROUP", func() {
			cfg := server.DefaultDLQRetryWorkerConfig()

			Expect(cfg.ReadTimeout).To(BeNumerically(">", 0),
				"ReadTimeout must be positive to prevent indefinite blocking on XREADGROUP")
		})
	})
})
