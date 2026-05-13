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
// PHASE 7: DLQ WORKER CONFIG DEFAULTS (TP-1088-P1)
// ========================================
//
// Issue: #1088 Phase 7 (Observability & Resilience)
// File Under Test: pkg/datastorage/server/dlq_retry_worker.go
//
// BEHAVIORAL: Exercises DefaultDLQRetryWorkerConfig() factory and asserts
// the returned config contains all required operational fields for DLQ
// worker lifecycle and shutdown correlation support.
//
// ========================================

var _ = Describe("Phase 7: DLQ Worker Config Defaults (TP-1088-P1)", func() {

	Describe("DefaultDLQRetryWorkerConfig", func() {
		It("UT-DS-1088-P7-008: factory returns config with all operational fields populated", func() {
			cfg := server.DefaultDLQRetryWorkerConfig()

			Expect(cfg.PollInterval).To(Equal(30*time.Second),
				"PollInterval must be 30s for balanced throughput")
			Expect(cfg.MaxBatchSize).To(Equal(int64(10)),
				"MaxBatchSize must be 10 per DD-009")
			Expect(cfg.ReadTimeout).To(Equal(5*time.Second),
				"ReadTimeout must be 5s for bounded XREADGROUP")
			Expect(cfg.ConsumerGroup).To(Equal("data-storage-retry-workers"),
				"ConsumerGroup must match the Redis stream consumer group name")
			Expect(cfg.ConsumerName).To(Equal("worker-default"),
				"ConsumerName must identify this worker instance")
		})
	})
})
