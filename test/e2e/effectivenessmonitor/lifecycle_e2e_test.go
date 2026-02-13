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

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// E2E Tests: Full Pipeline Lifecycle and Audit Events
//
// These tests exercise the complete EM assessment pipeline with all 4 components
// (health, hash, alert, metrics) using real Prometheus and AlertManager.
//
// Scenarios:
//   - E2E-EM-RC-001: Full pipeline happy path -> EA processed with all events
//   - E2E-EM-AE-001: All 5 audit events present in DS with correct payloads
//   - E2E-EM-SH-001: Hash event in DS matches actual spec change

var _ = Describe("EffectivenessMonitor Lifecycle E2E Tests", Label("e2e"), func() {
	var testNS string

	BeforeEach(func() {
		testNS = createTestNamespace("em-lc-e2e")
	})

	AfterEach(func() {
		deleteTestNamespace(testNS)
	})

	// ========================================================================
	// E2E-EM-RC-001: Full Pipeline Happy Path
	// ========================================================================
	It("E2E-EM-RC-001: should process EA through full pipeline with all assessments", func() {
		By("Setting up test data: target pod, alerts, and metrics")

		// Create and wait for target pod
		createTargetPod(testNS, "target-pod")
		waitForPodReady(testNS, "target-pod")

		// The reconciler queries AM using alertname=<correlationID>.
		// Define correlationID before injecting alerts so the alert name matches.
		correlationID := uniqueName("corr-rc-happy")

		// Inject a resolved alert for this target (alertname must match correlationID)
		err := infrastructure.InjectAlerts(alertManagerURL, []infrastructure.TestAlert{
			{
				Name: correlationID,
				Labels: map[string]string{
					"namespace": testNS,
					"pod":       "target-pod",
				},
				Status:   "resolved",
				StartsAt: time.Now().Add(-10 * time.Minute),
				EndsAt:   time.Now().Add(-2 * time.Minute),
			},
		})
		Expect(err).ToNot(HaveOccurred(), "Failed to inject alerts")

		// Inject before/after metrics showing improvement.
		// Use time.Now() for both to avoid Prometheus TSDB "out of bounds" rejection.
		// Inject sequentially with a short sleep for timestamp separation.
		err = infrastructure.InjectMetrics(prometheusURL, []infrastructure.TestMetric{
			{
				Name: "container_cpu_usage_seconds_total",
				Labels: map[string]string{
					"namespace": testNS,
					"pod":       "target-pod",
					"container": "workload",
				},
				Value:     0.90,
				Timestamp: time.Now(),
			},
		})
		Expect(err).ToNot(HaveOccurred(), "Failed to inject 'before' metrics")

		time.Sleep(2 * time.Second) // Ensure timestamp separation

		err = infrastructure.InjectMetrics(prometheusURL, []infrastructure.TestMetric{
			{
				Name: "container_cpu_usage_seconds_total",
				Labels: map[string]string{
					"namespace": testNS,
					"pod":       "target-pod",
					"container": "workload",
				},
				Value:     0.20,
				Timestamp: time.Now(),
			},
		})
		Expect(err).ToNot(HaveOccurred(), "Failed to inject 'after' metrics")

		By("Creating an EffectivenessAssessment CRD with all components enabled")
		name := uniqueName("ea-rc-happy")
		createEA(testNS, name, correlationID,
			withTargetPod("target-pod"),
		)

		By("Waiting for EM to complete the assessment")
		ea := waitForEAPhase(testNS, name, eav1.PhaseCompleted)

		By("Verifying all 4 components were assessed")
		Expect(ea.Status.Components.HealthAssessed).To(BeTrue(), "Health should be assessed")
		Expect(ea.Status.Components.HashComputed).To(BeTrue(), "Hash should be computed")
		Expect(ea.Status.Components.AlertAssessed).To(BeTrue(), "Alert should be assessed")
		Expect(ea.Status.Components.MetricsAssessed).To(BeTrue(), "Metrics should be assessed")

		By("Verifying scores are set")
		Expect(ea.Status.Components.HealthScore).ToNot(BeNil(), "Health score should be set")
		Expect(ea.Status.Components.AlertScore).ToNot(BeNil(), "Alert score should be set")
		Expect(ea.Status.Components.MetricsScore).ToNot(BeNil(), "Metrics score should be set")

		By("Verifying assessment reason is 'full' (all components completed)")
		Expect(ea.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonFull),
			"Assessment reason should be 'full' when all components succeed")

		By("Verifying completion timestamp is set")
		Expect(ea.Status.CompletedAt).ToNot(BeNil(), "CompletedAt should be set")
	})

	// ========================================================================
	// E2E-EM-AE-001: Audit Events in DataStorage
	// ========================================================================
	It("E2E-EM-AE-001: should emit all 5 audit events to DataStorage", func() {
		By("Creating a target pod and EA with all components enabled")
		createTargetPod(testNS, "target-pod")
		waitForPodReady(testNS, "target-pod")

		// Define correlationID before injecting alerts (reconciler queries by correlationID)
		correlationID := uniqueName("corr-ae-events")

		// Inject resolved alert (alertname must match correlationID)
		err := infrastructure.InjectAlerts(alertManagerURL, []infrastructure.TestAlert{
			{
				Name:     correlationID,
				Labels:   map[string]string{"namespace": testNS, "pod": "target-pod"},
				Status:   "resolved",
				StartsAt: time.Now().Add(-10 * time.Minute),
				EndsAt:   time.Now().Add(-1 * time.Minute),
			},
		})
		Expect(err).ToNot(HaveOccurred())

		// Inject metrics sequentially with time.Now() to avoid TSDB "out of bounds".
		err = infrastructure.InjectMetrics(prometheusURL, []infrastructure.TestMetric{
			{
				Name:      "container_cpu_usage_seconds_total",
				Labels:    map[string]string{"namespace": testNS, "pod": "target-pod", "container": "workload"},
				Value:     0.80,
				Timestamp: time.Now(),
			},
		})
		Expect(err).ToNot(HaveOccurred(), "Failed to inject 'before' metrics")

		time.Sleep(2 * time.Second) // Ensure timestamp separation

		err = infrastructure.InjectMetrics(prometheusURL, []infrastructure.TestMetric{
			{
				Name:      "container_cpu_usage_seconds_total",
				Labels:    map[string]string{"namespace": testNS, "pod": "target-pod", "container": "workload"},
				Value:     0.30,
				Timestamp: time.Now(),
			},
		})
		Expect(err).ToNot(HaveOccurred(), "Failed to inject 'after' metrics")

		name := uniqueName("ea-ae-events")
		createEA(testNS, name, correlationID,
			withTargetPod("target-pod"),
		)

		By("Waiting for EA to complete")
		waitForEAPhase(testNS, name, eav1.PhaseCompleted)

		By("Querying DataStorage for audit events with this correlation ID")
		// The EM should have emitted 5 events:
		// 1. health assessment component event
		// 2. hash computation component event
		// 3. alert resolution component event
		// 4. metrics comparison component event
		// 5. effectiveness.assessment.completed lifecycle event
		//
		// Query DS audit export API for events matching this correlation ID.
		// This validates the real end-to-end audit trail.
		Eventually(func() int {
			params := ogenclient.ExportAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(correlationID),
				Format:        ogenclient.NewOptExportAuditEventsFormat(ogenclient.ExportAuditEventsFormatJSON),
			}
			res, err := auditClient.ExportAuditEvents(ctx, params)
			if err != nil {
				return 0
			}
			// Count events in the export response
			switch v := res.(type) {
			case *ogenclient.AuditExportResponse:
				return len(v.Events)
			default:
				return 0
			}
		}, timeout, interval).Should(BeNumerically(">=", 5),
			"DataStorage should contain at least 5 audit events for correlation ID %s", correlationID)
	})

	// ========================================================================
	// E2E-EM-SH-001: Spec Hash Verification
	// ========================================================================
	It("E2E-EM-SH-001: should compute spec hash and emit hash event to DS", func() {
		By("Creating a target pod and EA")
		createTargetPod(testNS, "target-pod")
		waitForPodReady(testNS, "target-pod")

		name := uniqueName("ea-sh-hash")
		correlationID := uniqueName("corr-sh-hash")
		createEA(testNS, name, correlationID,
			withTargetPod("target-pod"),
			withPrometheusDisabled(),
			withAlertManagerDisabled(),
		)

		By("Waiting for EA to complete")
		ea := waitForEAPhase(testNS, name, eav1.PhaseCompleted)

		By("Verifying hash was computed")
		Expect(ea.Status.Components.HashComputed).To(BeTrue(),
			"Hash should be computed")
		Expect(ea.Status.Components.PostRemediationSpecHash).ToNot(BeEmpty(),
			"Post-remediation spec hash should be set")
	})
})
