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
package mockllm_test

import (
	"github.com/prometheus/client_golang/prometheus"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mockmetrics "github.com/jordigilh/kubernaut/test/services/mock-llm/metrics"
)

var _ = Describe("Mock LLM Metrics", func() {

	var (
		reg *prometheus.Registry
		m   *mockmetrics.Metrics
	)

	BeforeEach(func() {
		reg = prometheus.NewRegistry()
		m = mockmetrics.NewMetricsWithRegistry(reg)
	})

	Describe("UT-MOCK-568-001: Metrics registration", func() {
		It("should register all collectors without panic", func() {
			Expect(m).NotTo(BeNil())
			mfs, err := reg.Gather()
			Expect(err).NotTo(HaveOccurred())
			Expect(mfs).NotTo(BeNil())
		})
	})

	Describe("UT-MOCK-568-002: RecordRequest increments counter", func() {
		It("should increment request counter with endpoint and status labels", func() {
			m.RecordRequest("/v1/chat/completions", 200, "oomkilled", 0.005)

			mfs, err := reg.Gather()
			Expect(err).NotTo(HaveOccurred())

			found := false
			for _, mf := range mfs {
				if mf.GetName() == "mock_llm_requests_total" {
					found = true
					Expect(mf.GetMetric()).To(HaveLen(1))
					metric := mf.GetMetric()[0]
					Expect(metric.GetCounter().GetValue()).To(Equal(1.0))
				}
			}
			Expect(found).To(BeTrue(), "mock_llm_requests_total not found")
		})
	})

	Describe("UT-MOCK-568-003: RecordScenarioDetection increments counter", func() {
		It("should increment scenario detection counter with correct labels", func() {
			m.RecordScenarioDetection("oomkilled", "signal")

			mfs, err := reg.Gather()
			Expect(err).NotTo(HaveOccurred())

			found := false
			for _, mf := range mfs {
				if mf.GetName() == "mock_llm_scenario_detection_total" {
					found = true
					Expect(mf.GetMetric()).To(HaveLen(1))
					metric := mf.GetMetric()[0]
					Expect(metric.GetCounter().GetValue()).To(Equal(1.0))
				}
			}
			Expect(found).To(BeTrue(), "mock_llm_scenario_detection_total not found")
		})
	})

	Describe("UT-MOCK-568-004: RecordDAGTransition increments counter", func() {
		It("should increment DAG transition counter with from/to labels", func() {
			m.RecordDAGTransition("dispatch", "search_workflow_catalog")

			mfs, err := reg.Gather()
			Expect(err).NotTo(HaveOccurred())

			found := false
			for _, mf := range mfs {
				if mf.GetName() == "mock_llm_dag_phase_transitions_total" {
					found = true
					Expect(mf.GetMetric()).To(HaveLen(1))
					metric := mf.GetMetric()[0]
					Expect(metric.GetCounter().GetValue()).To(Equal(1.0))
				}
			}
			Expect(found).To(BeTrue(), "mock_llm_dag_phase_transitions_total not found")
		})
	})
})
