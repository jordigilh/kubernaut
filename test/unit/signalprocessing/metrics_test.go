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

// Package signalprocessing contains unit tests for Signal Processing controller.
// Unit tests validate implementation correctness, not business value delivery.
// See docs/development/business-requirements/TESTING_GUIDELINES.md
package signalprocessing

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"

	"github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
)

func TestMetrics(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SignalProcessing Metrics Suite")
}

// Unit Test: Metrics package implementation correctness
var _ = Describe("Metrics", func() {
	var (
		registry *prometheus.Registry
		m        *metrics.Metrics
	)

	BeforeEach(func() {
		registry = prometheus.NewRegistry()
		m = metrics.NewMetrics(registry)
	})

	// Test 1: Metrics creation
	It("should create metrics with all required counters and gauges", func() {
		Expect(m).NotTo(BeNil())
		Expect(m.ProcessingTotal).NotTo(BeNil())
		Expect(m.ProcessingDuration).NotTo(BeNil())
		Expect(m.EnrichmentErrors).NotTo(BeNil())
	})

	// Test 2: Counter increment
	Context("when incrementing counters", func() {
		It("should increment processing total counter", func() {
			m.IncrementProcessingTotal("enriching", "success")
			m.IncrementProcessingTotal("enriching", "success")

			// Verify counter value is 2
			metrics, err := registry.Gather()
			Expect(err).NotTo(HaveOccurred())

			var found bool
			for _, mf := range metrics {
				if mf.GetName() == "signalprocessing_processing_total" {
					for _, metric := range mf.GetMetric() {
						if getLabel(metric, "phase") == "enriching" && getLabel(metric, "result") == "success" {
							Expect(metric.GetCounter().GetValue()).To(Equal(float64(2)))
							found = true
						}
					}
				}
			}
			Expect(found).To(BeTrue(), "expected to find processing_total metric with phase=enriching, result=success")
		})
	})
})

// getLabel extracts a label value from a metric.
func getLabel(metric *io_prometheus_client.Metric, name string) string {
	for _, label := range metric.GetLabel() {
		if label.GetName() == name {
			return label.GetValue()
		}
	}
	return ""
}
