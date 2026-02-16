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

package effectivenessmonitor

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/prometheus/client_golang/prometheus"

	emmetrics "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/metrics"
)

var _ = Describe("Controller Metrics (BR-EM-009, DD-METRICS-001)", func() {

	// ========================================
	// UT-EM-OM-001: Metrics registration
	// ========================================
	Describe("NewMetricsWithRegistry (UT-EM-OM-001)", func() {

		It("should register all metrics without panic", func() {
			registry := prometheus.NewPedanticRegistry()
			Expect(func() {
				emmetrics.NewMetricsWithRegistry(registry)
			}).ToNot(Panic())
		})

		It("should create non-nil metric collectors", func() {
			registry := prometheus.NewPedanticRegistry()
			m := emmetrics.NewMetricsWithRegistry(registry)

			Expect(m.ReconcileTotal).ToNot(BeNil())
			Expect(m.ReconcileDuration).ToNot(BeNil())
			Expect(m.PhaseTransitionsTotal).ToNot(BeNil())
			Expect(m.ComponentAssessmentsTotal).ToNot(BeNil())
			Expect(m.ComponentAssessmentDuration).ToNot(BeNil())
			Expect(m.ComponentScores).ToNot(BeNil())
			Expect(m.AssessmentsCompletedTotal).ToNot(BeNil())
			Expect(m.ExternalCallsTotal).ToNot(BeNil())
			Expect(m.ExternalCallDuration).ToNot(BeNil())
			Expect(m.ExternalCallErrors).ToNot(BeNil())
			Expect(m.ValidityExpirationsTotal).ToNot(BeNil())
			Expect(m.StabilizationWaitsTotal).ToNot(BeNil())
			Expect(m.AuditEventsTotal).ToNot(BeNil())
			Expect(m.K8sEventsTotal).ToNot(BeNil())
		})

		It("should not allow duplicate registration", func() {
			registry := prometheus.NewPedanticRegistry()
			emmetrics.NewMetricsWithRegistry(registry)

			// Second registration should panic (duplicate collector)
			Expect(func() {
				emmetrics.NewMetricsWithRegistry(registry)
			}).To(Panic())
		})
	})

	// ========================================
	// UT-EM-OM-002: Metric name constants
	// ========================================
	Describe("Metric Name Constants (UT-EM-OM-002)", func() {

		It("should follow DD-005 naming convention", func() {
			// All metrics must start with kubernaut_effectivenessmonitor_
			Expect(emmetrics.MetricNameReconcileTotal).To(HavePrefix("kubernaut_effectivenessmonitor_"))
			Expect(emmetrics.MetricNameReconcileDuration).To(HavePrefix("kubernaut_effectivenessmonitor_"))
			Expect(emmetrics.MetricNamePhaseTransitionsTotal).To(HavePrefix("kubernaut_effectivenessmonitor_"))
			Expect(emmetrics.MetricNameComponentAssessmentsTotal).To(HavePrefix("kubernaut_effectivenessmonitor_"))
			Expect(emmetrics.MetricNameAssessmentsCompletedTotal).To(HavePrefix("kubernaut_effectivenessmonitor_"))
			Expect(emmetrics.MetricNameExternalCallsTotal).To(HavePrefix("kubernaut_effectivenessmonitor_"))
			Expect(emmetrics.MetricNameAuditEventsTotal).To(HavePrefix("kubernaut_effectivenessmonitor_"))
			Expect(emmetrics.MetricNameK8sEventsTotal).To(HavePrefix("kubernaut_effectivenessmonitor_"))
		})
	})

	// ========================================
	// UT-EM-OM-003: Helper methods don't panic
	// ========================================
	Describe("Helper Methods (UT-EM-OM-003)", func() {

		var m *emmetrics.Metrics

		BeforeEach(func() {
			registry := prometheus.NewPedanticRegistry()
			m = emmetrics.NewMetricsWithRegistry(registry)
		})

		It("RecordReconcile should not panic", func() {
			Expect(func() {
				m.RecordReconcile("success", 0.5)
			}).ToNot(Panic())
		})

		It("RecordPhaseTransition should not panic", func() {
			Expect(func() {
				m.RecordPhaseTransition("Pending", "Assessing")
			}).ToNot(Panic())
		})

		It("RecordComponentAssessment should not panic with non-nil score", func() {
			score := 1.0
			Expect(func() {
				m.RecordComponentAssessment("health", "success", 0.1, &score)
			}).ToNot(Panic())
		})

		It("RecordComponentAssessment should not panic with nil score", func() {
			Expect(func() {
				m.RecordComponentAssessment("metrics", "error", 0.5, nil)
			}).ToNot(Panic())
		})

		It("RecordAssessmentCompleted should not panic", func() {
			Expect(func() {
				m.RecordAssessmentCompleted("full")
			}).ToNot(Panic())
		})

		It("RecordExternalCall should not panic", func() {
			Expect(func() {
				m.RecordExternalCall("prometheus", "query", 0.2)
			}).ToNot(Panic())
		})

		It("RecordExternalCallError should not panic", func() {
			Expect(func() {
				m.RecordExternalCallError("alertmanager", "alerts", "timeout")
			}).ToNot(Panic())
		})

		It("RecordValidityExpiration should not panic", func() {
			Expect(func() {
				m.RecordValidityExpiration()
			}).ToNot(Panic())
		})

		It("RecordStabilizationWait should not panic", func() {
			Expect(func() {
				m.RecordStabilizationWait()
			}).ToNot(Panic())
		})

		It("RecordAuditEvent should not panic", func() {
			Expect(func() {
				m.RecordAuditEvent("effectiveness.health.assessed", "success")
			}).ToNot(Panic())
		})

		It("RecordK8sEvent should not panic", func() {
			Expect(func() {
				m.RecordK8sEvent("Normal", "AssessmentStarted")
			}).ToNot(Panic())
		})
	})
})
