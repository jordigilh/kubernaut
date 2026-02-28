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

// Integration tests for ADR-032 §1: Phase Transition & Lifecycle Completion Audit Events
// These tests validate that RO emits MANDATORY audit events for SOC2 compliance and RR reconstruction.
//
// Business Requirement: BR-AUDIT-005 v2.0 (RR Reconstruction)
// SOC2 Compliance: ADR-032 §1 Item #7 ("Every orchestration phase transition")
// Design Decision: DD-AUDIT-003 (Service Audit Trace Requirements)
//
// Test Strategy (TDD RED-GREEN-REFACTOR):
// - RED: Write failing tests that validate phase transition and completion audit events
// - GREEN: Wire audit emission in RO reconciler
// - REFACTOR: Optimize audit emission logic
//
// Defense-in-Depth:
// - Unit tests: Audit event builders (already exist in pkg/remediationorchestrator/audit/)
// - Integration tests: Full audit persistence validation (this file - MANDATORY)
// - E2E tests: End-to-end remediation flow validation (already exists)

package remediationorchestrator

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	roaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
)

var _ = Describe("Phase Transition & Lifecycle Completion Audit Events (ADR-032 §1)", func() {
	var (
		testNamespace string
	)

	BeforeEach(func() {
		testNamespace = createTestNamespace("audit-phase-lifecycle")
		// DD-AUTH-014: dsClients initialized at suite level with ServiceAccount authentication
	})

	AfterEach(func() {
		deleteTestNamespace(testNamespace)
	})

	// Helper to query audit events by correlation_id and event_type
	queryAuditEvents := func(correlationID, eventType string) ([]ogenclient.AuditEvent, error) {
		// DD-AUTH-014: Fail fast if authenticated client is not initialized
		if dsClients == nil || dsClients.OpenAPIClient == nil {
			return nil, fmt.Errorf("dsClients is nil - DataStorage client not initialized in test suite")
		}

		eventCategory := roaudit.EventCategoryOrchestration

		// PAGINATION FIX (2026-01-24): Fetch ALL pages to avoid missing events under concurrent load
		// Root Cause: Under high load (12 procs), 100+ events can exist, causing first-page-only
		//             queries to miss events beyond position 100.
		// Reference: test/AUDIT_QUERY_PAGINATION_STANDARDS.md
		var allEvents []ogenclient.AuditEvent
		offset := 0
		limit := 100

		for {
			resp, err := dsClients.OpenAPIClient.QueryAuditEvents(context.Background(), ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(correlationID),
				EventCategory: ogenclient.NewOptString(eventCategory),
				EventType:     ogenclient.NewOptString(eventType),
				Limit:         ogenclient.NewOptInt(limit),
				Offset:        ogenclient.NewOptInt(offset),
			})

			if err != nil {
				return nil, err
			}

			if len(resp.Data) == 0 {
				break
			}

			allEvents = append(allEvents, resp.Data...)

			if len(resp.Data) < limit {
				break
			}

			offset += limit
		}

		return allEvents, nil
	}

	// Helper to create valid RemediationRequest
	// ADR-057: RR must be in ROControllerNamespace; controller only watches this NS
	newValidRemediationRequest := func(name, fingerprint string) *remediationv1.RemediationRequest {
		now := metav1.Now()
		return &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ROControllerNamespace,
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: fingerprint,
				SignalName:        "PhaseTransitionTest",
				Severity:          "medium",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: testNamespace,
				},
				FiringTime:   now,
				ReceivedTime: now,
			},
		}
	}

	Context("Phase Transition Audit Events (orchestrator.lifecycle.transitioned)", func() {
		It("IT-AUDIT-PHASE-001: should emit audit event when transitioning from Pending to Processing", func() {
			// TDD RED: This test will FAIL until we wire phase transition audit emission in reconciler
			// ADR-032 §1 Item #7: "Every orchestration phase transition"
			// DD-AUDIT-003: Event type = "orchestrator.lifecycle.transitioned" (formerly "orchestrator.phase.transitioned")

			fingerprint := GenerateTestFingerprint(testNamespace, "it-audit-phase-001")
			rr := newValidRemediationRequest("it-audit-phase-001", fingerprint)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

		// Wait for RO to create SignalProcessing (this triggers Pending → Processing)
		spName := "sp-" + rr.Name
		sp := &signalprocessingv1.SignalProcessing{}
		Eventually(func() bool {
			err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{
				Name:      spName,
				Namespace: ROControllerNamespace,
			}, sp)
			return err == nil
		}, timeout, interval).Should(BeTrue(), "SignalProcessing should be created")

		// Wait for RR to transition to Processing phase
		// Note: We DON'T complete SignalProcessing here - that would trigger Processing → Analyzing
		// This test only validates the Pending → Processing transition audit event
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{
					Name:      rr.Name, // Fixed: was incorrectly using spName
					Namespace: ROControllerNamespace,
				}, rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing),
				"RR should transition to Processing phase")

		// Query DataStorage for phase transition audit event
		// Explicit flush to ensure buffered events are persisted
		err := auditStore.Flush(ctx)
		Expect(err).ToNot(HaveOccurred(), "Failed to flush audit store")

		correlationID := rr.Name
		var events []ogenclient.AuditEvent
		var queryErr error

		Eventually(func() bool {
			events, queryErr = queryAuditEvents(correlationID, roaudit.EventTypeLifecycleTransitioned)
			if queryErr != nil {
				GinkgoWriter.Printf("⏳ Waiting for phase transition audit event (error: %v)\n", queryErr)
				return false
			}
			if len(events) > 0 {
				GinkgoWriter.Printf("✅ Found %d transition event(s)\n", len(events))
			}
			return len(events) > 0
		}, 30*time.Second, 2*time.Second).Should(BeTrue(),
			"Phase transition audit event should be persisted in DataStorage")

		// Validate we have exactly 1 event (Pending → Processing)
		// Since we don't complete SignalProcessing, there should be no additional transitions
		Expect(events).To(HaveLen(1), "Should have exactly 1 phase transition event (Pending→Processing)")
		event := events[0]

		// Validate event details
		Expect(event.EventType).To(Equal(roaudit.EventTypeLifecycleTransitioned))
		Expect(string(event.EventCategory)).To(Equal(roaudit.EventCategoryOrchestration))
		Expect(event.EventAction).To(Equal("transitioned"))
		Expect(event.CorrelationID).To(Equal(correlationID))
		Expect(string(event.EventOutcome)).To(Equal("success"))

		// Validate the phase transition in event_data
		payload, ok := event.EventData.GetRemediationOrchestratorAuditPayload()
		Expect(ok).To(BeTrue(), "EventData should be RemediationOrchestratorAuditPayload")
		Expect(payload.FromPhase.Value).To(Equal("Pending"), "FromPhase should be Pending")
		Expect(payload.ToPhase.Value).To(Equal("Processing"), "ToPhase should be Processing")
		})

		It("IT-AUDIT-PHASE-002: should emit audit event when transitioning from Processing to Analyzing", func() {
			// TDD RED: This test will FAIL until phase transition audit is wired

			fingerprint := GenerateTestFingerprint(testNamespace, "it-audit-phase-002")
			rr := newValidRemediationRequest("it-audit-phase-002", fingerprint)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())
			spName := "sp-" + rr.Name
			aiName := "ai-" + rr.Name

			// Wait for SignalProcessing and complete it
			sp := &signalprocessingv1.SignalProcessing{}
			Eventually(func() bool {
				err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{
					Name:      spName,
					Namespace: ROControllerNamespace,
				}, sp)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			Expect(updateSPStatus(ROControllerNamespace, sp.Name, signalprocessingv1.PhaseCompleted, "medium")).To(Succeed())

			// Wait for RO to create AIAnalysis
			ai := &aianalysisv1.AIAnalysis{}
			Eventually(func() bool {
				err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{
					Name:      aiName,
					Namespace: ROControllerNamespace,
				}, ai)
				return err == nil
			}, timeout, interval).Should(BeTrue(), "AIAnalysis should be created")

			// Complete AIAnalysis to trigger phase transition to Analyzing
			ai.Status.Phase = aianalysisv1.PhaseCompleted
			Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

			// Wait for RR to transition to Analyzing phase
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{
					Name:      rr.Name, // Fixed: was incorrectly using spName
					Namespace: ROControllerNamespace,
				}, rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseAnalyzing),
				"RR should transition to Analyzing phase")

			// Trigger async flush and wait for events to be persisted
			err := auditStore.Flush(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Query DataStorage for phase transition audit events
			// Wait for async flush to complete
			correlationID := rr.Name
			var events []ogenclient.AuditEvent

			Eventually(func() bool {
				events, err = queryAuditEvents(correlationID, roaudit.EventTypeLifecycleTransitioned)
				if err != nil {
					return false
				}
				// Should have 2 events: Pending→Processing and Processing→Analyzing
				return len(events) >= 2
			}, 30*time.Second, 2*time.Second).Should(BeTrue(),
				"Should have 2 phase transition audit events")

			// Find the Processing→Analyzing transition
			var analyzingEvent *ogenclient.AuditEvent
			for i := range events {
				payload := events[i].EventData.RemediationOrchestratorAuditPayload
				if payload.ToPhase.IsSet() && payload.ToPhase.Value == "Analyzing" {
					analyzingEvent = &events[i]
					break
				}
			}

			Expect(analyzingEvent).ToNot(BeNil(), "Should find Processing→Analyzing transition event")
			payload := analyzingEvent.EventData.RemediationOrchestratorAuditPayload
			Expect(payload.FromPhase.Value).To(Equal("Processing"))
			Expect(payload.ToPhase.Value).To(Equal("Analyzing"))
		})
	})

	Context("Lifecycle Completion Audit Events (orchestrator.lifecycle.completed)", func() {
		It("IT-AUDIT-COMPLETION-001: should emit success completion audit event when remediation succeeds", func() {
			// TDD RED: This test will FAIL until we wire completion audit emission in reconciler
			// ADR-032 §1 Item #7: Lifecycle completion is part of phase transition tracking
			// DD-AUDIT-003: Event type = "orchestrator.lifecycle.completed" with outcome=success

			fingerprint := GenerateTestFingerprint(testNamespace, "it-audit-completion-001")
			rr := newValidRemediationRequest("it-audit-completion-001", fingerprint)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())
			spName := "sp-" + rr.Name
			aiName := "ai-" + rr.Name
			weName := "we-" + rr.Name

			// Complete SignalProcessing
			sp := &signalprocessingv1.SignalProcessing{}
			Eventually(func() bool {
				err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{
					Name:      spName,
					Namespace: ROControllerNamespace,
				}, sp)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			Expect(updateSPStatus(ROControllerNamespace, sp.Name, signalprocessingv1.PhaseCompleted, "medium")).To(Succeed())

			// Complete AIAnalysis
			ai := &aianalysisv1.AIAnalysis{}
			Eventually(func() bool {
				err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{
					Name:      aiName,
					Namespace: ROControllerNamespace,
				}, ai)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			ai.Status.Phase = aianalysisv1.PhaseCompleted
			ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
				WorkflowID:     "test-workflow",
				Version:        "1.0.0",
				ExecutionBundle: "test-image:latest",
			}
			// DD-HAPI-006: AffectedResource is required for routing to WorkflowExecution
			ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
				Summary:    "Test root cause",
				Severity:   "medium",
				SignalType: "alert",
				AffectedResource: &aianalysisv1.AffectedResource{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: ROControllerNamespace,
				},
			}
			Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

			// Complete WorkflowExecution
			we := &workflowexecutionv1.WorkflowExecution{}
			Eventually(func() bool {
				err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{
					Name:      weName,
					Namespace: ROControllerNamespace,
				}, we)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			we.Status.Phase = workflowexecutionv1.PhaseCompleted
			Expect(k8sClient.Status().Update(ctx, we)).To(Succeed())

			// Wait for RR to complete successfully
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{
					Name:      rr.Name, // Fixed: was incorrectly using spName
					Namespace: ROControllerNamespace,
				}, rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseCompleted),
				"RR should transition to Completed phase")

			// Trigger async flush and wait for events to be persisted
			err := auditStore.Flush(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Query DataStorage for lifecycle completion audit event
			// Wait for async flush to complete
			correlationID := rr.Name
			var events []ogenclient.AuditEvent

			Eventually(func() bool {
			events, err = queryAuditEvents(correlationID, roaudit.EventTypeLifecycleCompleted)
			if err != nil {
				GinkgoWriter.Printf("⏳ Waiting for completion audit event (error: %v)\n", err)
				return false
			}
			return len(events) > 0
		}, 30*time.Second, 2*time.Second).Should(BeTrue(),
			"Lifecycle completion audit event should be persisted in DataStorage")

		// Validate event details
		Expect(events).To(HaveLen(1), "Should have exactly 1 completion event")
		event := events[0]

		Expect(event.EventType).To(Equal(roaudit.EventTypeLifecycleCompleted))
		Expect(string(event.EventCategory)).To(Equal(roaudit.EventCategoryOrchestration)) // Convert enum to string
		Expect(event.EventAction).To(Equal(roaudit.ActionCompleted))
			Expect(event.CorrelationID).To(Equal(correlationID))
			Expect(string(event.EventOutcome)).To(Equal("success"))

			// Validate event_data contains completion details
			// Note: Specific payload field validation will be added after GREEN phase implementation
			payload := event.EventData.RemediationOrchestratorAuditPayload
			Expect(payload.Outcome.IsSet()).To(BeTrue(), "Should capture outcome")
			Expect(payload.Outcome.Value).To(Equal(ogenclient.RemediationOrchestratorAuditPayloadOutcomeSuccess))
		})

		It("IT-AUDIT-COMPLETION-002: should emit failure completion audit event when remediation fails", func() {
			// TDD RED: This test will FAIL until we wire failure completion audit emission
			// ADR-032 §1 Item #7: Lifecycle failure tracking for SOC2
			// DD-AUDIT-003: Event type = "orchestrator.lifecycle.completed" with outcome=failure

			fingerprint := GenerateTestFingerprint(testNamespace, "it-audit-completion-002")
			rr := newValidRemediationRequest("it-audit-completion-002", fingerprint)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())
			spName := "sp-" + rr.Name
			aiName := "ai-" + rr.Name

			// Complete SignalProcessing
			sp := &signalprocessingv1.SignalProcessing{}
			Eventually(func() bool {
				err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{
					Name:      spName,
					Namespace: ROControllerNamespace,
				}, sp)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			Expect(updateSPStatus(ROControllerNamespace, sp.Name, signalprocessingv1.PhaseCompleted, "medium")).To(Succeed())

			// Fail AIAnalysis to trigger remediation failure
			ai := &aianalysisv1.AIAnalysis{}
			Eventually(func() bool {
				err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{
					Name:      aiName,
					Namespace: ROControllerNamespace,
				}, ai)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			ai.Status.Phase = aianalysisv1.PhaseFailed
			ai.Status.Reason = "AIAnalysisInternalError"
			Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

			// Wait for RR to fail
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{
					Name:      rr.Name, // Fixed: was incorrectly using spName
					Namespace: ROControllerNamespace,
				}, rr)
				GinkgoWriter.Printf("⏳ RR phase: %s, AIPhase: %s\n", rr.Status.OverallPhase, ai.Status.Phase)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseFailed),
				"RR should transition to Failed phase")

			// Trigger async flush and wait for events to be persisted
			err := auditStore.Flush(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Query DataStorage for lifecycle failure completion audit event
			// Wait for async flush to complete
			correlationID := rr.Name
			var events []ogenclient.AuditEvent

			Eventually(func() bool {
			events, err = queryAuditEvents(correlationID, roaudit.EventTypeLifecycleCompleted)
			if err != nil {
				GinkgoWriter.Printf("⏳ Waiting for failure completion audit event (error: %v)\n", err)
				return false
			}
			if len(events) > 0 {
				GinkgoWriter.Printf("✅ Found %d completion events, first event: EventType=%s, EventCategory=%s, EventOutcome=%s\n",
					len(events), events[0].EventType, events[0].EventCategory, string(events[0].EventOutcome))
			} else {
				GinkgoWriter.Printf("⏳ No completion events found yet (correlation_id=%s, event_type=%s)\n", correlationID, roaudit.EventTypeLifecycleCompleted)
			}
			return len(events) > 0
		}, 30*time.Second, 2*time.Second).Should(BeTrue(),
			"Lifecycle failure completion audit event should be persisted in DataStorage")

		// Validate event details
		Expect(events).To(HaveLen(1), "Should have exactly 1 completion event")
		event := events[0]

		Expect(event.EventType).To(Equal(roaudit.EventTypeLifecycleCompleted))
		Expect(string(event.EventCategory)).To(Equal(roaudit.EventCategoryOrchestration)) // Convert enum to string
		Expect(event.EventAction).To(Equal(roaudit.ActionCompleted))
			Expect(event.CorrelationID).To(Equal(correlationID))
			Expect(string(event.EventOutcome)).To(Equal("failure"))

			// Validate event_data contains failure details
			payload := event.EventData.RemediationOrchestratorAuditPayload
			Expect(payload.Outcome.IsSet()).To(BeTrue(), "Should capture outcome")
			Expect(payload.Outcome.Value).To(Equal(ogenclient.RemediationOrchestratorAuditPayloadOutcomeFailed))
			Expect(payload.FailurePhase.IsSet()).To(BeTrue(), "Should capture failure_phase for SOC2/RR reconstruction")
		})
	})
})
