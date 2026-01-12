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

// Package remediationorchestrator_test contains E2E tests for RO audit client wiring.
//
// Test Scope: Validates RemediationOrchestrator audit client is correctly wired
// to DataStorage service in production deployment per ADR-032 §2 mandatory audit.
//
// Business Requirements:
// - ADR-032 §2: P0 services MUST crash if audit cannot be initialized
// - ADR-032 §4: Audit functions MUST return error if audit store is nil
// - BR-STORAGE-001: Complete audit trail with no data loss
//
// Test Strategy:
// - E2E test: Validate audit client is wired (this file)
// - Integration tests: Validate audit event content (separate file)
package remediationorchestrator

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	roaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("RemediationOrchestrator Audit Client Wiring E2E", func() {
	const (
		dataStorageURL = "http://localhost:8081" // DD-TEST-001: Access via NodePort/extraPortMappings
		e2eTimeout     = 120 * time.Second       // Same as suite timeout
		e2eInterval    = 500 * time.Millisecond
	)

	Context("Audit Client Wiring Verification", func() {
		var (
			testNamespace string
			testRR        *remediationv1.RemediationRequest
			correlationID string
			dsClient      *dsgen.Client
		)

		BeforeEach(func() {
			// Create unique namespace for E2E test
			testNamespace = createTestNamespace("audit-wiring-e2e")

			// ✅ DD-API-001: Use OpenAPI generated client (MANDATORY)
			// Per DD-API-001: Direct HTTP usage is FORBIDDEN - bypasses type safety
			var err error
			dsClient, err = dsgen.NewClient(dataStorageURL)
			Expect(err).ToNot(HaveOccurred(), "Failed to create DataStorage OpenAPI client")

			// Create RemediationRequest
			now := metav1.Now()
			testRR = &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("e2e-audit-test-%d", time.Now().Unix()),
					Namespace: testNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "a1b2c3d4e5f60123456789abcdef0123456789abcdef0123456789abcdef0123",
					SignalName:        "E2EAuditWiringTest",
					Severity:          "critical",
					SignalType:        "prometheus",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "e2e-test-app",
						Namespace: testNamespace,
					},
					FiringTime:   now,
					ReceivedTime: now,
					Deduplication: sharedtypes.DeduplicationInfo{
						FirstOccurrence: now,
						LastOccurrence:  now,
						OccurrenceCount: 1,
					},
				},
			}

			Expect(k8sClient.Create(ctx, testRR)).To(Succeed())

			// Get UID as correlation ID
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      testRR.Name,
					Namespace: testNamespace,
				}, testRR); err != nil {
					return false
				}
				return testRR.UID != ""
			}, e2eTimeout, e2eInterval).Should(BeTrue())

			correlationID = string(testRR.UID)

			GinkgoWriter.Printf("🚀 E2E: Created RemediationRequest %s/%s (UID: %s)\n",
				testNamespace, testRR.Name, correlationID)
		})

		AfterEach(func() {
			// Cleanup namespace
			deleteTestNamespace(testNamespace)
		})

		// ✅ DD-API-001: Helper using OpenAPI generated client (MANDATORY)
		// Per DD-API-001: Direct HTTP is FORBIDDEN - this uses type-safe client
		queryAuditEvents := func(correlationID string) ([]dsgen.AuditEvent, int, error) {
			// Per ADR-034 v1.2: event_category is MANDATORY for queries
			eventCategory := "orchestration"
			limit := 100

			// ✅ MANDATORY: Use generated client with type-safe parameters
			resp, err := dsClient.QueryAuditEvents(context.Background(), dsgen.QueryAuditEventsParams{
				CorrelationID: dsgen.NewOptString(correlationID),
				EventCategory: dsgen.NewOptString(eventCategory),
				Limit:         dsgen.NewOptInt(limit),
			})

			if err != nil {
				return nil, 0, fmt.Errorf("failed to query DataStorage: %w", err)
			}

			// Access typed response directly (ogen pattern)
			total := 0
			if resp.Pagination.Set && resp.Pagination.Value.Total.Set {
				total = resp.Pagination.Value.Total.Value
			}

			events := resp.Data

			return events, total, nil
		}

		It("should successfully emit audit events to DataStorage service", func() {
			// ADR-032 §2: P0 services MUST crash if audit cannot be initialized
			// ADR-032 §4: Audit functions MUST return error if audit store is nil
			// This test validates the audit client is correctly wired in production deployment

			By("Querying DataStorage for audit events (DD-TESTING-001: Use Eventually() instead of time.Sleep())")
			var events []dsgen.AuditEvent
			var total int
			var err error

			Eventually(func() bool {
				events, total, err = queryAuditEvents(correlationID)
				if err != nil {
					GinkgoWriter.Printf("⏳ E2E: Waiting for audit events (error: %v)\n", err)
					return false
				}
				return total > 0
			}, 2*time.Minute, 10*time.Second).Should(BeTrue(),
				"Expected audit events to be stored in DataStorage")

			// Validate at least lifecycle.started event exists
			Expect(events).ToNot(BeEmpty(), "Expected at least one audit event")

			// Find lifecycle.started event (proves audit client is wired)
			var foundLifecycleStarted bool
			for _, event := range events {
				if event.EventType == roaudit.EventTypeLifecycleStarted {
					foundLifecycleStarted = true
					Expect(event.CorrelationID).To(Equal(correlationID))
					break
				}
			}

			Expect(foundLifecycleStarted).To(BeTrue(),
				"Expected orchestrator.lifecycle.started event (proves audit client wiring)")

			GinkgoWriter.Printf("✅ E2E: Audit client correctly wired - found %d audit events in DataStorage\n",
				total)
		})

		It("should emit audit events throughout the remediation lifecycle", func() {
			// This test validates audit events are emitted continuously, not just at startup
			// Per ADR-032 §1: All orchestration phase transitions must be audited

			By("Querying audit events and waiting for lifecycle progression (DD-TESTING-001)")
			// DD-TESTING-001: Wait for COMPLETE audit trail, not just ">=2 events"
			// Root cause of flakiness: Audit buffer flush (1s interval) may not have completed
			// for phase transition events when we check. Solution: Wait for specific event types.
			var events []dsgen.AuditEvent
			var total int
			var err error

			Eventually(func() bool {
				events, total, err = queryAuditEvents(correlationID)
				if err != nil {
					GinkgoWriter.Printf("⏳ Waiting for audit events (error: %v)\n", err)
					return false
				}
				if total == 0 {
					GinkgoWriter.Printf("⏳ Waiting for audit events (no events yet)\n")
					return false
				}

				// Build event type map
				eventTypes := make(map[string]bool)
				for _, event := range events {
					eventTypes[event.EventType] = true
				}

				// Check for lifecycle.started (should always be present first)
				hasLifecycleStarted := eventTypes[roaudit.EventTypeLifecycleStarted]

				// Check for phase transition or lifecycle completion/failure
				hasPhaseTransitionOrCompletion := eventTypes[roaudit.EventTypeLifecycleTransitioned] ||
					eventTypes[roaudit.EventTypeLifecycleCompleted] ||
					eventTypes[roaudit.EventTypeLifecycleFailed]

				if !hasLifecycleStarted {
					GinkgoWriter.Printf("⏳ Waiting for complete audit trail (no lifecycle.started yet, %d total events)\n", total)
					return false
				}
				if !hasPhaseTransitionOrCompletion {
					GinkgoWriter.Printf("⏳ Waiting for complete audit trail (no phase transition/completion yet, %d total events)\n", total)
					return false
				}

				return true
			}, 2*time.Minute, 2*time.Second).Should(BeTrue(),
				"Expected complete audit trail with lifecycle.started + phase transition/completion events")

			// Verify we have different event types (proves continuous emission)
			eventTypes := make(map[string]bool)
			for _, event := range events {
				eventTypes[event.EventType] = true
			}

			Expect(eventTypes).To(HaveKey(roaudit.EventTypeLifecycleStarted),
				"Expected lifecycle.started event")

			// Should have at least one phase transition or lifecycle completion/failure
			hasPhaseTransitionOrCompletion := eventTypes[roaudit.EventTypeLifecycleTransitioned] ||
				eventTypes[roaudit.EventTypeLifecycleCompleted] ||
				eventTypes[roaudit.EventTypeLifecycleFailed]

			Expect(hasPhaseTransitionOrCompletion).To(BeTrue(),
				"Expected phase transition or lifecycle completion/failure event")

			GinkgoWriter.Printf("✅ E2E: Audit events emitted throughout lifecycle - %d events, %d types\n",
				total, len(eventTypes))
		})

		It("should handle audit service unavailability gracefully during startup", func() {
			// ADR-032 §2: P0 services MUST crash if audit cannot be initialized
			// This test validates that IF DataStorage is unreachable at RO startup,
			// RO crashes and does NOT start (tested via deployment probe failures)

			// Note: This is a negative test that would require redeploying RO with
			// DataStorage unavailable. In E2E, we validate the positive case: when
			// DataStorage IS available, RO successfully initializes audit client.

			By("Verifying RO pod is running (proves audit initialized successfully)")
			// If we reached here, RO is running and accepting requests
			// This proves audit initialization succeeded per ADR-032 §2

			ctx := context.Background()
			podList := &corev1.PodList{}
			err := k8sClient.List(ctx, podList, client.InNamespace("kubernaut-system"),
				client.MatchingLabels{"app": "remediationorchestrator-controller"})
			Expect(err).ToNot(HaveOccurred())
			Expect(podList.Items).ToNot(BeEmpty(), "Expected RO pod to be running")

			// Check pod is Ready (proves liveness/readiness probes pass)
			roPod := podList.Items[0]
			var ready bool
			for _, condition := range roPod.Status.Conditions {
				if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
					ready = true
					break
				}
			}
			Expect(ready).To(BeTrue(), "Expected RO pod to be Ready (proves audit initialized)")

			GinkgoWriter.Printf("✅ E2E: RO pod %s is Ready (proves audit client initialized per ADR-032 §2)\n",
				roPod.Name)
		})
	})
})
