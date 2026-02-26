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

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
)

// E2E Tests: Health Check Assessment
//
// Scenarios:
//   - E2E-EM-HC-001: Target pod not running -> health score 0.0 in DS

var _ = Describe("EffectivenessMonitor Health Check E2E Tests", Label("e2e"), func() {
	var testNS string

	BeforeEach(func() {
		testNS = createTestNamespace("em-hc-e2e")
	})

	AfterEach(func() {
		deleteTestNamespace(testNS)
	})

	// ========================================================================
	// E2E-EM-HC-001: Unhealthy Target
	// ========================================================================
	It("E2E-EM-HC-001: should produce health score 0.0 when target pod is not running", func() {
		By("Creating an EA that references a non-existent target pod")
		name := uniqueName("ea-hc-missing")
		correlationID := uniqueName("corr-hc")

		// The target pod "nonexistent-pod" does not exist in the namespace,
		// so the health check should produce a score of 0.0.
		// ADR-EM-001 v1.4: Component isolation is at EM config level, not per-EA.
		// All components run; health check is the focus of this test's assertions.
		createEA(testNS, name, correlationID,
			withTargetPod("nonexistent-pod"),
		)

		By("Waiting for EM to complete the assessment")
		ea := waitForEAPhase(name, eav1.PhaseCompleted)

		By("Verifying health was assessed with score 0.0")
		Expect(ea.Status.Components.HealthAssessed).To(BeTrue(),
			"Health component should be assessed")
		Expect(ea.Status.Components.HealthScore).ToNot(BeNil(),
			"Health score should be set")
		Expect(*ea.Status.Components.HealthScore).To(Equal(0.0),
			"Health score should be 0.0 for non-existent pod")
	})
})
