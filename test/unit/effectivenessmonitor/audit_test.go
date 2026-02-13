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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/audit"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/types"
)

var _ = Describe("Audit Event Builder (BR-AUDIT-006)", func() {

	var builder audit.Builder

	BeforeEach(func() {
		builder = audit.NewBuilder()
	})

	baseEventData := func() audit.EventData {
		return audit.EventData{
			CorrelationID:  "rr-test-001",
			AssessmentName: "ea-test-001",
			Namespace:      "default",
			Timestamp:      time.Now(),
		}
	}

	// ========================================
	// UT-EM-AE-001: Build health audit event
	// ========================================
	Describe("BuildHealthEvent (UT-EM-AE-001)", func() {

		It("should build health audit event with all fields populated", func() {
			data := baseEventData()
			score := 1.0

			event := builder.BuildHealthEvent(data, &score, 3, 3, 0)

			Expect(event.CorrelationID).To(Equal("rr-test-001"))
			Expect(event.AssessmentName).To(Equal("ea-test-001"))
			Expect(event.Namespace).To(Equal("default"))
			Expect(event.Score).ToNot(BeNil())
			Expect(*event.Score).To(Equal(1.0))
			Expect(event.TotalReplicas).To(Equal(int32(3)))
			Expect(event.ReadyReplicas).To(Equal(int32(3)))
			Expect(event.RestartsSinceRemediation).To(Equal(int32(0)))
		})

		It("should handle nil score (health not assessed)", func() {
			data := baseEventData()

			event := builder.BuildHealthEvent(data, nil, 0, 0, 0)

			Expect(event.Score).To(BeNil())
			Expect(event.CorrelationID).To(Equal("rr-test-001"))
		})
	})

	// ========================================
	// UT-EM-AE-002: Build hash audit event
	// ========================================
	Describe("BuildHashEvent (UT-EM-AE-002)", func() {

		It("should build hash audit event with computed hash", func() {
			data := baseEventData()

			event := builder.BuildHashEvent(data, "abc123def456")

			Expect(event.CorrelationID).To(Equal("rr-test-001"))
			Expect(event.PostRemediationSpecHash).To(Equal("abc123def456"))
		})

		It("should handle empty hash", func() {
			data := baseEventData()

			event := builder.BuildHashEvent(data, "")

			Expect(event.PostRemediationSpecHash).To(BeEmpty())
		})
	})

	// ========================================
	// UT-EM-AE-003: Build alert audit event
	// ========================================
	Describe("BuildAlertEvent (UT-EM-AE-003)", func() {

		It("should build alert audit event for resolved alert", func() {
			data := baseEventData()
			score := 1.0

			event := builder.BuildAlertEvent(data, &score, "HighLatency", true)

			Expect(event.Score).ToNot(BeNil())
			Expect(*event.Score).To(Equal(1.0))
			Expect(event.AlertName).To(Equal("HighLatency"))
			Expect(event.AlertResolved).To(BeTrue())
		})

		It("should build alert audit event for active alert", func() {
			data := baseEventData()
			score := 0.0

			event := builder.BuildAlertEvent(data, &score, "HighLatency", false)

			Expect(*event.Score).To(Equal(0.0))
			Expect(event.AlertResolved).To(BeFalse())
		})
	})

	// ========================================
	// UT-EM-AE-004: Build metrics audit event
	// ========================================
	Describe("BuildMetricsEvent (UT-EM-AE-004)", func() {

		It("should build metrics audit event with query details", func() {
			data := baseEventData()
			score := 0.85

			event := builder.BuildMetricsEvent(data, &score, 5, "3 of 5 metrics improved")

			Expect(*event.Score).To(BeNumerically("~", 0.85, 0.001))
			Expect(event.QueriesExecuted).To(Equal(5))
			Expect(event.Details).To(Equal("3 of 5 metrics improved"))
		})
	})

	// ========================================
	// UT-EM-AE-008: Build scheduled audit event (BR-EM-009.4)
	// ========================================
	Describe("BuildScheduledEvent (UT-EM-AE-008)", func() {

		It("should build scheduled audit event with all derived timing fields", func() {
			data := baseEventData()
			creationTime := time.Now()
			validityWindow := 30 * time.Minute
			stabilizationWindow := 5 * time.Minute

			validityDeadline := creationTime.Add(validityWindow)
			prometheusCheckAfter := creationTime.Add(stabilizationWindow)
			alertManagerCheckAfter := creationTime.Add(stabilizationWindow)

			event := builder.BuildScheduledEvent(data,
				validityDeadline, prometheusCheckAfter, alertManagerCheckAfter,
				validityWindow, stabilizationWindow)

			Expect(event.CorrelationID).To(Equal("rr-test-001"))
			Expect(event.AssessmentName).To(Equal("ea-test-001"))
			Expect(event.Namespace).To(Equal("default"))

			// Verify derived timing fields
			Expect(event.ValidityDeadline).To(BeTemporally("~", validityDeadline, time.Millisecond))
			Expect(event.PrometheusCheckAfter).To(BeTemporally("~", prometheusCheckAfter, time.Millisecond))
			Expect(event.AlertManagerCheckAfter).To(BeTemporally("~", alertManagerCheckAfter, time.Millisecond))

			// Verify config observability fields
			Expect(event.ValidityWindow).To(Equal(30 * time.Minute))
			Expect(event.StabilizationWindow).To(Equal(5 * time.Minute))

			// Invariant: ValidityDeadline > PrometheusCheckAfter
			Expect(event.ValidityDeadline.After(event.PrometheusCheckAfter)).To(BeTrue(),
				"ValidityDeadline must be after PrometheusCheckAfter")

			// PrometheusCheckAfter == AlertManagerCheckAfter (same stabilization window)
			Expect(event.PrometheusCheckAfter).To(Equal(event.AlertManagerCheckAfter),
				"PrometheusCheckAfter and AlertManagerCheckAfter should be identical")
		})

		It("should correctly capture custom timing values", func() {
			data := baseEventData()
			creationTime := time.Now()
			validityWindow := 1 * time.Hour
			stabilizationWindow := 10 * time.Minute

			event := builder.BuildScheduledEvent(data,
				creationTime.Add(validityWindow),
				creationTime.Add(stabilizationWindow),
				creationTime.Add(stabilizationWindow),
				validityWindow, stabilizationWindow)

			Expect(event.ValidityWindow).To(Equal(1 * time.Hour))
			Expect(event.StabilizationWindow).To(Equal(10 * time.Minute))
		})
	})

	// ========================================
	// UT-EM-AE-005: Build completed audit event
	// ========================================
	Describe("BuildCompletedEvent (UT-EM-AE-005)", func() {

		It("should build completed audit event with all components", func() {
			data := baseEventData()
			healthScore := 1.0
			alertScore := 1.0
			components := []types.ComponentResult{
				{Component: types.ComponentHealth, Assessed: true, Score: &healthScore},
				{Component: types.ComponentAlert, Assessed: true, Score: &alertScore},
			}

			event := builder.BuildCompletedEvent(data, components, "full", "Assessment completed successfully")

			Expect(event.Reason).To(Equal("full"))
			Expect(event.Message).To(Equal("Assessment completed successfully"))
			Expect(event.Components).To(HaveLen(2))
		})

		It("should build completed audit event for partial assessment", func() {
			data := baseEventData()
			healthScore := 0.5
			components := []types.ComponentResult{
				{Component: types.ComponentHealth, Assessed: true, Score: &healthScore},
				{Component: types.ComponentMetrics, Assessed: false, Score: nil},
			}

			event := builder.BuildCompletedEvent(data, components, "partial", "Metrics timed out")

			Expect(event.Reason).To(Equal("partial"))
			Expect(event.Components).To(HaveLen(2))
		})

		It("should build completed audit event for expired assessment", func() {
			data := baseEventData()

			event := builder.BuildCompletedEvent(data, nil, "expired", "Validity window expired")

			Expect(event.Reason).To(Equal("expired"))
			Expect(event.Components).To(BeNil())
		})
	})
})
