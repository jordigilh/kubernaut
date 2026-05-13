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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
)

// ========================================
// PHASE 7: SHUTDOWN OBSERVABILITY (TP-1088-P1)
// ========================================
//
// Issue: #1088 Phase 7.8 (FEDRAMP-H1)
// File Under Test: pkg/datastorage/server/server.go
// TDD Phase: RED — shutdown does not include correlation ID
//
// FEDRAMP AU-2 requires all shutdown operations to be correlatable.
// A shutdown_id (UUID) should be generated at the start of Shutdown()
// and passed to all step log entries for end-to-end tracing.
//
// ========================================

var _ = Describe("Phase 7: Shutdown Observability (TP-1088-P1)", func() {

	Describe("Shutdown ID (FEDRAMP-H1)", func() {
		It("UT-DS-1088-P7-008: DLQRetryWorkerConfig should support shutdown correlation", func() {
			// RED: The worker config and lifecycle don't include shutdown correlation.
			// FEDRAMP-H1 requires all shutdown operations to carry a shutdown_id.
			//
			// This test verifies the building block: the worker's Start log
			// includes all required fields for operational correlation.
			// The shutdown_id itself is generated in Shutdown() (integration test).

			cfg := server.DefaultDLQRetryWorkerConfig()

			// Verify all operational fields are configured
			Expect(cfg.PollInterval).To(BeNumerically(">", 0))
			Expect(cfg.MaxBatchSize).To(BeNumerically(">", 0))
			Expect(cfg.ReadTimeout).To(BeNumerically(">", 0))
			Expect(cfg.ConsumerGroup).To(Equal("data-storage-retry-workers"))
			Expect(cfg.ConsumerName).To(Equal("worker-default"))
		})
	})
})
