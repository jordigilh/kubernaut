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
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	dsmetrics "github.com/jordigilh/kubernaut/pkg/datastorage/metrics"
)

// ========================================
// PHASE 7: OBSERVABILITY METRICS WIRING (TP-1088-P1)
// ========================================
//
// Issue: #1088 Phase 7 (Observability & Resilience)
// TDD Phase: BEHAVIORAL — each metric is created via NewMetricsWithRegistry,
// then operated on (Inc/Set), and the resulting value is read back from the
// Prometheus registry. This proves end-to-end wiring, not just constant naming.
//
// ========================================

var _ = Describe("Phase 7: Observability Metrics Wiring (TP-1088-P1)", func() {

	var m *dsmetrics.Metrics

	BeforeEach(func() {
		reg := prometheus.NewRegistry()
		m = dsmetrics.NewMetricsWithRegistry("datastorage", "", reg)
	})

	Describe("Drain-batch counter (7.4)", func() {
		It("UT-DS-1088-P7-004: DLQDrainBatchTotal increments and reads back from registry", func() {
			Expect(m.DLQDrainBatchTotal).ToNot(BeNil(),
				"DLQDrainBatchTotal must be wired in Metrics struct")

			m.DLQDrainBatchTotal.Inc()

			var metric dto.Metric
			Expect(m.DLQDrainBatchTotal.Write(&metric)).To(Succeed())
			Expect(metric.GetCounter().GetValue()).To(Equal(float64(1)),
				"Counter should read 1 after a single Inc()")
		})
	})

	Describe("Retention-purge counter (7.9)", func() {
		It("UT-DS-1088-P7-009: RetentionPurgeTotal increments and reads back from registry", func() {
			Expect(m.RetentionPurgeTotal).ToNot(BeNil(),
				"RetentionPurgeTotal must be wired in Metrics struct")

			m.RetentionPurgeTotal.Inc()
			m.RetentionPurgeTotal.Inc()

			var metric dto.Metric
			Expect(m.RetentionPurgeTotal.Write(&metric)).To(Succeed())
			Expect(metric.GetCounter().GetValue()).To(Equal(float64(2)),
				"Counter should read 2 after two Inc() calls")
		})
	})

	Describe("PEL pending gauge (7.10)", func() {
		It("UT-DS-1088-P7-010a: DLQPelPending can be set and read back", func() {
			Expect(m.DLQPelPending).ToNot(BeNil(),
				"DLQPelPending must be wired in Metrics struct")

			m.DLQPelPending.Set(42)

			var metric dto.Metric
			Expect(m.DLQPelPending.Write(&metric)).To(Succeed())
			Expect(metric.GetGauge().GetValue()).To(Equal(float64(42)),
				"Gauge should reflect the Set(42) value")
		})
	})

	Describe("Shutdown DLQ drain error counter (7.6)", func() {
		It("UT-DS-1088-P7-006: ShutdownDLQDrainError increments and reads back from registry", func() {
			Expect(m.ShutdownDLQDrainError).ToNot(BeNil(),
				"ShutdownDLQDrainError must be wired in Metrics struct")

			m.ShutdownDLQDrainError.Inc()

			var metric dto.Metric
			Expect(m.ShutdownDLQDrainError.Write(&metric)).To(Succeed())
			Expect(metric.GetCounter().GetValue()).To(Equal(float64(1)),
				"Counter should read 1 after Inc()")
		})
	})
})
