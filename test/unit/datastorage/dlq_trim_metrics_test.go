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

package datastorage

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/jordigilh/kubernaut/pkg/datastorage/metrics"
)

// #1048 Phase 5 / AU-11 / BR-STORAGE-019: DLQ XADD vs MAXLEN~ trimming observability.
var _ = Describe("UT-DS-1048-P5: DLQ Trim Metrics (AU-11)", func() {
	newIsolatedMetrics := func() *metrics.Metrics {
		reg := prometheus.NewRegistry()
		return metrics.NewMetricsWithRegistry("datastorage", "", reg)
	}

	Describe("UT-DS-1048-P5-070: DLQStreamXAddTotal metric exists", func() {
		It("should expose datastorage_dlq_stream_xadd_total on the Metrics bundle", func() {
			m := newIsolatedMetrics()
			Expect(m.DLQStreamXAddTotal).NotTo(BeNil())

			var metric dto.Metric
			err := m.DLQStreamXAddTotal.WithLabelValues("notifications").Write(&metric)
			Expect(err).NotTo(HaveOccurred())
			Expect(metric.GetCounter().GetValue()).To(Equal(float64(0)))
		})
	})

	Describe("UT-DS-1048-P5-071: Counter has stream label", func() {
		It("should separate time series per stream label", func() {
			m := newIsolatedMetrics()
			m.DLQStreamXAddTotal.WithLabelValues("notifications").Inc()
			m.DLQStreamXAddTotal.WithLabelValues("audit_events").Inc()
			m.DLQStreamXAddTotal.WithLabelValues("audit_events").Inc()

			var metric dto.Metric
			err := m.DLQStreamXAddTotal.WithLabelValues("notifications").Write(&metric)
			Expect(err).NotTo(HaveOccurred())
			Expect(metric.GetCounter().GetValue()).To(Equal(float64(1)))

			err = m.DLQStreamXAddTotal.WithLabelValues("audit_events").Write(&metric)
			Expect(err).NotTo(HaveOccurred())
			Expect(metric.GetCounter().GetValue()).To(Equal(float64(2)))
		})
	})
})
