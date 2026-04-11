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

// BR-SP-008: Prometheus Metrics - Global registry path coverage
package signalprocessing

import (
	"github.com/prometheus/client_golang/prometheus"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
)

var _ = Describe("UT-SP-668-005: NewMetrics registry path", func() {
	var m *metrics.Metrics

	BeforeEach(func() {
		m = metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
	})

	It("BR-SP-100: should create metrics instance with functional collectors", func() {
		Expect(m.ProcessingTotal).ToNot(BeZero())
		Expect(m.ProcessingDuration).ToNot(BeZero())
		Expect(m.EnrichmentErrors).ToNot(BeZero())
	})

	It("BR-SP-100: should be usable for recording metrics after creation", func() {
		Expect(func() {
			m.IncrementProcessingTotal("enriching", "success")
			m.ObserveProcessingDuration("enriching", 0.1)
			m.RecordEnrichmentError("test")
		}).NotTo(Panic())
	})
})
