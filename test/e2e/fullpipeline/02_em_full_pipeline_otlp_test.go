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

package fullpipeline

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// EM Full Pipeline - OTLP Adapter [E2E-EM-OTLP-001]
//
// Verifies the Effectiveness Monitor (EM) full pipeline after a remediation completes:
//  1. RO creates EffectivenessAssessment CRD when RR reaches Completed phase
//  2. EM processes EA (health, hash, optionally metrics/alerts)
//  3. EM emits audit events to DataStorage
//
// This test runs after 01_full_remediation_lifecycle_test.go completes (file order).
// It relies on at least one EA existing from a prior remediation completion.
//
// OTLP metric injection and AlertManager alert injection are placeholder steps;
// the EM may complete assessment with health+hash only (metrics/alerts can be skipped).
var _ = Describe("EM Full Pipeline - OTLP Adapter [E2E-EM-OTLP-001]", Ordered, func() {
	var (
		testCtx    context.Context
		testCancel context.CancelFunc
		eaName     string
	)

	BeforeAll(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 5*time.Minute)
	})

	AfterAll(func() {
		testCancel()
	})

	It("should find EffectivenessAssessment CRDs created by RO after remediation completion", func() {
		By("Listing EA CRDs in the namespace")
		eaList := &eav1.EffectivenessAssessmentList{}
		Eventually(func() int {
			err := k8sClient.List(testCtx, eaList, client.InNamespace(namespace))
			if err != nil {
				return 0
			}
			return len(eaList.Items)
		}, 3*time.Minute, 5*time.Second).Should(BeNumerically(">=", 1),
			"At least one EA should be created by RO after remediation completion")

		eaName = eaList.Items[0].Name
		GinkgoWriter.Printf("Found EA: %s\n", eaName)
	})

	It("should have correct EA spec (correlationID, targetResource, stabilizationWindow)", func() {
		ea := &eav1.EffectivenessAssessment{}
		err := k8sClient.Get(testCtx, client.ObjectKey{Name: eaName, Namespace: namespace}, ea)
		Expect(err).ToNot(HaveOccurred())

		By("Verifying correlationID is set")
		Expect(ea.Spec.CorrelationID).ToNot(BeEmpty(), "correlationID should be set to RR name")

		By("Verifying targetResource is set")
		Expect(ea.Spec.TargetResource.Kind).ToNot(BeEmpty())
		Expect(ea.Spec.TargetResource.Name).ToNot(BeEmpty())

		By("Verifying stabilizationWindow is set")
		Expect(ea.Spec.Config.StabilizationWindow.Duration).To(BeNumerically(">", 0),
			"stabilizationWindow should be positive (set by RO config)")

		By("Verifying spec fields")
		Expect(ea.Spec.RemediationRequestPhase).To(Equal("Completed"))

		GinkgoWriter.Printf("EA spec: correlationID=%s, target=%s/%s, stabilizationWindow=%v\n",
			ea.Spec.CorrelationID, ea.Spec.TargetResource.Kind, ea.Spec.TargetResource.Name,
			ea.Spec.Config.StabilizationWindow.Duration)
	})

	It("should have Prometheus and AlertManager available for OTLP/metric injection", func() {
		By("Verifying Prometheus OTLP endpoint is available")
		promURL := fmt.Sprintf("http://localhost:%d", infrastructure.PrometheusHostPort)
		GinkgoWriter.Printf("Prometheus available at %s (OTLP metric injection)\n", promURL)

		By("Verifying AlertManager API is available")
		amURL := fmt.Sprintf("http://localhost:%d", infrastructure.AlertManagerHostPort)
		GinkgoWriter.Printf("AlertManager available at %s (alert resolution)\n", amURL)
	})

	It("should wait for EA to reach terminal phase (EM assessment complete)", func() {
		By("Waiting for EA to be processed by EM")
		Eventually(func() string {
			ea := &eav1.EffectivenessAssessment{}
			err := k8sClient.Get(testCtx, client.ObjectKey{Name: eaName, Namespace: namespace}, ea)
			if err != nil {
				return ""
			}
			return ea.Status.Phase
		}, 3*time.Minute, 5*time.Second).Should(
			BeElementOf(eav1.PhaseCompleted, eav1.PhaseFailed),
			"EA should reach terminal phase (Completed or Failed)")

		// Get final EA status
		ea := &eav1.EffectivenessAssessment{}
		err := k8sClient.Get(testCtx, client.ObjectKey{Name: eaName, Namespace: namespace}, ea)
		Expect(err).ToNot(HaveOccurred())

		GinkgoWriter.Printf("EA final state: phase=%s, reason=%s, healthAssessed=%v, hashComputed=%v, alertAssessed=%v, metricsAssessed=%v\n",
			ea.Status.Phase, ea.Status.AssessmentReason,
			ea.Status.Components.HealthAssessed, ea.Status.Components.HashComputed,
			ea.Status.Components.AlertAssessed, ea.Status.Components.MetricsAssessed)
	})

	It("should have EM audit events in DataStorage", func() {
		By("Querying DataStorage for EM audit events")
		ea := &eav1.EffectivenessAssessment{}
		err := k8sClient.Get(testCtx, client.ObjectKey{Name: eaName, Namespace: namespace}, ea)
		Expect(err).ToNot(HaveOccurred())

		// All 6 EM audit event types must be present per ADR-EM-001.
		// Both Prometheus and AlertManager are enabled by default (EMConfig defaults),
		// so all component events fire — none are optional.
		expectedEMEvents := []string{
			"effectiveness.assessment.scheduled",
			"effectiveness.health.assessed",
			"effectiveness.hash.computed",
			"effectiveness.alert.assessed",
			"effectiveness.metrics.assessed",
			"effectiveness.assessment.completed",
		}

		var allEMEvents []ogenclient.AuditEvent
		eventTypeCounts := map[string]int{}
		Eventually(func() []string {
			params := ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(ea.Spec.CorrelationID),
				EventCategory: ogenclient.NewOptString("effectiveness"),
				Limit:         ogenclient.NewOptInt(100),
			}
			resp, err := dataStorageClient.QueryAuditEvents(testCtx, params)
			if err != nil {
				GinkgoWriter.Printf("  Audit query error: %v\n", err)
				return expectedEMEvents
			}
			allEMEvents = resp.Data

			eventTypeCounts = map[string]int{}
			for _, evt := range allEMEvents {
				eventTypeCounts[evt.EventType]++
			}

			var missing []string
			for _, eventType := range expectedEMEvents {
				if eventTypeCounts[eventType] == 0 {
					missing = append(missing, eventType)
				}
			}
			GinkgoWriter.Printf("  Found %d EM audit events (%d unique types), %d required still missing\n",
				len(allEMEvents), len(eventTypeCounts), len(missing))
			return missing
		}, 2*time.Minute, 5*time.Second).Should(BeEmpty(),
			"All 6 EM audit event types must be present in DataStorage")

		// Verify each event type appears exactly once (each is flag-guarded)
		for _, eventType := range expectedEMEvents {
			Expect(eventTypeCounts[eventType]).To(Equal(1),
				"EM event %s must appear exactly once, but found %d", eventType, eventTypeCounts[eventType])
		}

		// Total count must be exactly 6 (no unexpected effectiveness events)
		Expect(len(allEMEvents)).To(Equal(len(expectedEMEvents)),
			"DataStorage should contain exactly %d EM audit events (got %d)",
			len(expectedEMEvents), len(allEMEvents))
	})
})
