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

package adapters_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gwmetrics "github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

var _ = Describe("Observability Metrics (#1029/#1032)", func() {

	Context("Metric Registration", func() {
		It("UT-GW-1029-028: gateway_owner_resolution_total metric registers without collision", func() {
			reg := prometheus.NewRegistry()
			m := gwmetrics.NewMetricsWithRegistry(reg)
			// Verify metric can be incremented without panic
			m.OwnerResolutionTotal.WithLabelValues("Deployment", "success").Inc()
			m.OwnerResolutionTotal.WithLabelValues("BuildConfig", "skipped_crd").Inc()

			families, err := reg.Gather()
			Expect(err).ToNot(HaveOccurred())

			found := false
			for _, f := range families {
				if f.GetName() == gwmetrics.MetricNameOwnerResolutionTotal {
					found = true
					Expect(f.GetMetric()).To(HaveLen(2))
				}
			}
			Expect(found).To(BeTrue(),
				"gateway_owner_resolution_total metric should be registered and gatherable")
		})

		It("UT-GW-1029-029: gateway_signals_parse_dropped_total metric registers without collision", func() {
			reg := prometheus.NewRegistry()
			m := gwmetrics.NewMetricsWithRegistry(reg)
			m.SignalsParseDroppedTotal.WithLabelValues("owner_resolution_failed").Inc()

			families, err := reg.Gather()
			Expect(err).ToNot(HaveOccurred())

			found := false
			for _, f := range families {
				if f.GetName() == gwmetrics.MetricNameSignalsParseDroppedTotal {
					found = true
					Expect(f.GetMetric()).To(HaveLen(1))
				}
			}
			Expect(found).To(BeTrue(),
				"gateway_signals_parse_dropped_total metric should be registered and gatherable")
		})
	})
})
