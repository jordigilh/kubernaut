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
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// E2E Tests: Alert Resolution with Real AlertManager
//
// These tests inject alerts into the real AlertManager deployed in the Kind cluster
// and verify that the EM correctly scores them.
//
// Scenarios:
//   - E2E-EM-AR-001: Resolved alert -> alert score 1.0 in DS
//   - E2E-EM-AR-002: Active alerts -> alert score 0.0 in DS

var _ = Describe("EffectivenessMonitor Alert Resolution E2E Tests", Label("e2e"), func() {
	var testNS string

	BeforeEach(func() {
		testNS = createTestNamespace("em-ar-e2e")
	})

	AfterEach(func() {
		deleteTestNamespace(testNS)
	})

	// ========================================================================
	// E2E-EM-AR-001: Resolved Alert
	// ========================================================================
	It("E2E-EM-AR-001: should produce alert score 1.0 when AlertManager alert is resolved", func() {
		// The reconciler queries AlertManager using alertname=<correlationID> (see reconciler.go:assessAlert).
		// We must inject the alert with Name matching the EA's correlation ID.
		correlationID := uniqueName("corr-ar-resolved")

		By("Injecting a resolved alert into the real AlertManager")
		alerts := []infrastructure.TestAlert{
			{
				Name: correlationID, // Must match correlationID used by the reconciler
				Labels: map[string]string{
					"namespace": testNS,
					"pod":       "target-pod",
					"severity":  "warning",
				},
				Annotations: map[string]string{
					"summary": "High CPU usage detected (test)",
				},
				Status:   "resolved",
				StartsAt: time.Now().Add(-10 * time.Minute),
				EndsAt:   time.Now().Add(-1 * time.Minute),
			},
		}
		err := infrastructure.InjectAlerts(alertManagerURL, alerts)
		Expect(err).ToNot(HaveOccurred(), "Failed to inject resolved alert into AlertManager")

		By("Creating a target pod and EA")
		createTargetPod(testNS, "target-pod")
		waitForPodReady(testNS, "target-pod")

		name := uniqueName("ea-ar-resolved")
		// ADR-EM-001 v1.4: Component isolation is at EM config level, not per-EA.
		createEA(testNS, name, correlationID,
			withTargetPod("target-pod"),
		)

		By("Waiting for EM to complete the assessment")
		ea := waitForEAPhase(testNS, name, eav1.PhaseCompleted)

		By("Verifying alert was assessed with score 1.0 (resolved)")
		Expect(ea.Status.Components.AlertAssessed).To(BeTrue(),
			"Alert component should be assessed")
		Expect(ea.Status.Components.AlertScore).ToNot(BeNil(),
			"Alert score should be set")
		Expect(*ea.Status.Components.AlertScore).To(Equal(1.0),
			"Alert score should be 1.0 for resolved alert")
	})

	// ========================================================================
	// E2E-EM-AR-002: Active (Firing) Alert
	// ========================================================================
	It("E2E-EM-AR-002: should produce alert score 0.0 when AlertManager has active alerts", func() {
		// The reconciler queries AlertManager using alertname=<correlationID>.
		// We must inject the alert with Name matching the EA's correlation ID.
		correlationID := uniqueName("corr-ar-firing")

		By("Injecting a firing alert into the real AlertManager")
		alerts := []infrastructure.TestAlert{
			{
				Name: correlationID, // Must match correlationID used by the reconciler
				Labels: map[string]string{
					"namespace": testNS,
					"pod":       "target-pod",
					"severity":  "critical",
				},
				Annotations: map[string]string{
					"summary": "High memory usage (test - still firing)",
				},
				Status:   "firing",
				StartsAt: time.Now(),
				// No EndsAt = still firing
			},
		}
		err := infrastructure.InjectAlerts(alertManagerURL, alerts)
		Expect(err).ToNot(HaveOccurred(), "Failed to inject firing alert into AlertManager")

		By("Creating a target pod and EA")
		createTargetPod(testNS, "target-pod")
		waitForPodReady(testNS, "target-pod")

		name := uniqueName("ea-ar-firing")
		// ADR-EM-001 v1.4: Component isolation is at EM config level, not per-EA.
		createEA(testNS, name, correlationID,
			withTargetPod("target-pod"),
		)

		By("Waiting for EM to complete the assessment")
		ea := waitForEAPhase(testNS, name, eav1.PhaseCompleted)

		By("Verifying alert was assessed with score 0.0 (still firing)")
		Expect(ea.Status.Components.AlertAssessed).To(BeTrue(),
			"Alert component should be assessed")
		Expect(ea.Status.Components.AlertScore).ToNot(BeNil(),
			"Alert score should be set")
		Expect(*ea.Status.Components.AlertScore).To(Equal(0.0),
			"Alert score should be 0.0 for active/firing alert")
	})
})
