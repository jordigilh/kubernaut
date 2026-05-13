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

	dsmetrics "github.com/jordigilh/kubernaut/pkg/datastorage/metrics"
)

// ========================================
// PHASE 7: OBSERVABILITY METRIC CONSTANTS (TP-1088-P1)
// ========================================
//
// Issue: #1088 Phase 7 (Observability & Resilience)
// TDD Phase: RED — these tests FAIL because metric constants are empty stubs
//
// Each metric constant must follow the DD-005 V3.0 naming convention:
//   datastorage_<subsystem>_<metric>_<unit>
//
// ========================================

var _ = Describe("Phase 7: Observability Metric Constants (TP-1088-P1)", func() {

	Describe("Drain-batch counter (7.4)", func() {
		It("UT-DS-1088-P7-004: MetricNameDLQDrainBatchTotal must be a valid metric name", func() {
			// RED: Stub constant is empty string.
			// GREEN will set to "datastorage_dlq_drain_batch_total".

			Expect(dsmetrics.MetricNameDLQDrainBatchTotal).ToNot(BeEmpty(),
				"Drain-batch counter metric name must be defined (DD-005 V3.0)")
			Expect(dsmetrics.MetricNameDLQDrainBatchTotal).To(HavePrefix("datastorage_"),
				"Metric name must follow DD-005 V3.0 naming: datastorage_<subsystem>_<metric>")
		})
	})

	Describe("Retention-purge counter (7.9)", func() {
		It("UT-DS-1088-P7-009: MetricNameRetentionPurgeTotal must be a valid metric name", func() {
			// RED: Stub constant is empty string.

			Expect(dsmetrics.MetricNameRetentionPurgeTotal).ToNot(BeEmpty(),
				"Retention-purge counter metric name must be defined (DD-005 V3.0)")
			Expect(dsmetrics.MetricNameRetentionPurgeTotal).To(HavePrefix("datastorage_"),
				"Metric name must follow DD-005 V3.0 naming")
		})
	})

	Describe("PEL pending gauge (7.10)", func() {
		It("UT-DS-1088-P7-010a: MetricNameDLQPelPending must be a valid metric name", func() {
			// RED: Stub constant is empty string.

			Expect(dsmetrics.MetricNameDLQPelPending).ToNot(BeEmpty(),
				"PEL pending gauge metric name must be defined for XPENDING observability")
			Expect(dsmetrics.MetricNameDLQPelPending).To(HavePrefix("datastorage_"),
				"Metric name must follow DD-005 V3.0 naming")
		})
	})

	Describe("PEL idle age gauge (7.10)", func() {
		It("UT-DS-1088-P7-010b: MetricNameDLQPelMaxIdleSeconds must be a valid metric name", func() {
			// RED: Stub constant is empty string.

			Expect(dsmetrics.MetricNameDLQPelMaxIdleSeconds).ToNot(BeEmpty(),
				"PEL idle age gauge metric name must be defined for stale message detection")
			Expect(dsmetrics.MetricNameDLQPelMaxIdleSeconds).To(HavePrefix("datastorage_"),
				"Metric name must follow DD-005 V3.0 naming")
		})
	})

	Describe("Shutdown DLQ drain error counter (7.6)", func() {
		It("UT-DS-1088-P7-006: MetricNameShutdownDLQDrainError must be a valid metric name", func() {
			// RED: Stub constant is empty string.
			// ARCH-M1: Shutdown must surface DLQ drain errors as metrics.

			Expect(dsmetrics.MetricNameShutdownDLQDrainError).ToNot(BeEmpty(),
				"Shutdown DLQ drain error metric must be defined for ARCH-M1 error surfacing")
			Expect(dsmetrics.MetricNameShutdownDLQDrainError).To(HavePrefix("datastorage_"),
				"Metric name must follow DD-005 V3.0 naming")
		})
	})
})
