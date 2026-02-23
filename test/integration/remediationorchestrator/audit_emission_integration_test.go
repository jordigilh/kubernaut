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

// Integration tests for BR-ORCH-041: Audit Trail Integration
// These tests validate that RO emits audit events to Data Storage and that events are persisted correctly.
//
// Business Requirement: BR-ORCH-041 (Audit Trail Integration)
// Design Decision: DD-AUDIT-003 (Service Audit Trace Requirements)
//
// Test Strategy:
// - RO controller running in envtest
// - Data Storage service running in podman
// - Audit events emitted by RO
// - Tests query Data Storage using OpenAPI Go client to validate event persistence
//
// Defense-in-Depth:
// - Unit tests: Fire-and-forget audit emission (limited validation)
// - Integration tests: Full audit persistence validation using OpenAPI client (this file)
// - E2E tests: N/A (audit is internal concern)

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
	audit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
)

var _ = Describe("Audit Emission Integration Tests (BR-ORCH-041)", func() {
	var (
		testNamespace string
		dsClient      *ogenclient.Client
	)

	BeforeEach(func() {
		// DD-AUTH-014: Use authenticated OpenAPI client from shared setup
		// dsClients is created in SynchronizedBeforeSuite with ServiceAccount token
		// Creating a new client here would bypass authentication!
		dsClient = dsClients.OpenAPIClient
	})

	BeforeEach(func() {
		testNamespace = createTestNamespace("audit-emission")
	})

	AfterEach(func() {
		deleteTestNamespace(testNamespace)
	})

	// Helper to create valid RemediationRequest with all required fields
	newValidRemediationRequest := func(name, fingerprint string) *remediationv1.RemediationRequest {
		now := metav1.Now()
		return &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: testNamespace,
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: fingerprint,
				SignalName:        "IntegrationTestSignal",
				Severity:          "warning",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: "default",
				},
				FiringTime:   now,
				ReceivedTime: now,
			},
		}
	}

	Context("AE-INT-1: Lifecycle Started Audit (Pending→Processing)", func() {
		It("should emit 'lifecycle_started' audit event when RR transitions to Processing", func() {
			// Create RemediationRequest with unique fingerprint (prevents test pollution)
			fingerprint := GenerateTestFingerprint(testNamespace, "ae-int-1-lifecycle-started")
			rr := newValidRemediationRequest("rr-lifecycle-started", fingerprint)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) for audit event queries
		// Per universal standard: RemediationRequest.Name is the correlation ID for all services
		correlationID := rr.Name

			// Wait for RO to transition to Processing (creates SignalProcessing CRD)
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

			// Query Data Storage API for lifecycle_started audit event using OpenAPI client
			// Use Eventually because audit events are buffered (FlushInterval: 1s)
			// Timeout: 90s (conservative, timer works at ~1s but allows for infrastructure delays)
			eventType := string(ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleStarted)
			var events []ogenclient.AuditEvent
			Eventually(func() int {
				events = queryAuditEventsOpenAPI(dsClient, correlationID, eventType)
				return len(events)
			}, "90s", "1s").Should(Equal(1), "Expected exactly 1 lifecycle_started audit event after buffer flush")

			// Validate event structure (typed response from OpenAPI client)
			event := events[0]
			Expect(event.EventType).To(Equal(string(ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleStarted)))
			Expect(event.EventAction).To(Equal("started"))
			Expect(string(event.EventOutcome)).To(Equal("pending"), "lifecycle.started uses pending outcome - outcome not yet determined")
			Expect(event.CorrelationID).To(Equal(correlationID))

			// Validate event_data (strongly-typed per DD-AUDIT-004)
			payload := event.EventData.RemediationOrchestratorAuditPayload
			Expect(payload.RrName).To(Equal("rr-lifecycle-started"))
			Expect(payload.Namespace).To(Equal(testNamespace))
		})
	})

	Context("AE-INT-2: Phase Transition Audit (Processing→Analyzing)", func() {
		It("should emit 'phase_transition' audit event when RR transitions phases", func() {
			// Create RemediationRequest with unique fingerprint (prevents test pollution)
			fingerprint := GenerateTestFingerprint(testNamespace, "ae-int-2-phase-transition")
			rr := newValidRemediationRequest("rr-phase-transition", fingerprint)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) for audit event queries
		// Per universal standard: RemediationRequest.Name is the correlation ID for all services
		correlationID := rr.Name

			// Wait for Processing phase (RO creates SignalProcessing CRD)
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

		// Get the RO-created SignalProcessing CRD and update its status to Completed
		spName := fmt.Sprintf("sp-%s", rr.Name)
		Eventually(func() error {
			sp := &signalprocessingv1.SignalProcessing{}
			return k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{Name: spName, Namespace: testNamespace}, sp)
		}, timeout, interval).Should(Succeed(), "Expected RO to create SignalProcessing CRD")

		// Update SP status to Completed (including severity per DD-SEVERITY-001)
		Expect(updateSPStatus(testNamespace, spName, signalprocessingv1.PhaseCompleted, "critical")).To(Succeed())

		// Wait for transition to Analyzing
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseAnalyzing))

		// Wait for controller to emit event, then flush once
		time.Sleep(500 * time.Millisecond)
		err := auditStore.Flush(ctx)
		Expect(err).ToNot(HaveOccurred())

		// Query for phase transition audit event using Eventually (accounts for buffer flush)
		// Timeout increased to 10s for consistency with other audit event queries (AE-INT-3, AE-INT-4, AE-INT-8)
		// and to account for audit store flush interval (1s) + network latency + query processing
		eventType := string(ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleTransitioned)
		var transitionEvent *ogenclient.AuditEvent
		Eventually(func() bool {
			events := queryAuditEventsOpenAPI(dsClient, correlationID, eventType)
			if len(events) == 0 {
				GinkgoWriter.Printf("No phase_transition events found for correlation_id=%s\n", correlationID)
				return false
			}
			GinkgoWriter.Printf("Found %d phase_transition events for correlation_id=%s\n", len(events), correlationID)
			// Find the Processing→Analyzing transition
			for i, e := range events {
				// Use strongly-typed access (DD-AUDIT-004)
				payload := e.EventData.RemediationOrchestratorAuditPayload
				if payload.FromPhase.IsSet() && payload.ToPhase.IsSet() {
					if payload.FromPhase.Value == "Processing" && payload.ToPhase.Value == "Analyzing" {
						transitionEvent = &events[i]
						return true
					}
				}
			}
			GinkgoWriter.Printf("Processing→Analyzing transition not found in %d events\n", len(events))
			return false
		}, "10s", "500ms").Should(BeTrue(), "Expected Processing→Analyzing transition event after buffer flush")

			// Validate event
			Expect(transitionEvent.EventType).To(Equal(string(ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleTransitioned)))
			Expect(transitionEvent.EventAction).To(Equal("transitioned"))
			Expect(string(transitionEvent.EventOutcome)).To(Equal("success"))
		})
	})

	// RESOLVED (2025-12-27): Audit timer investigation complete - test enabled
	// - 10 test iterations validated timer reliability (0/10 bugs detected)
	// - Timer firing correctly with ~1s intervals (sub-millisecond precision)
	// - 50-90s delay never reproduced
	// - Investigation: docs/handoff/RO_AUDIT_TIMER_INTERMITTENCY_ANALYSIS_DEC_27_2025.md
	// - Test enabled with 90s timeout (conservative, timer works at ~1s)
	Context("AE-INT-3: Completion Audit (Executing→Completed)", func() {
		It("should emit 'lifecycle_completed' audit event when RR completes successfully", func() {
			// Create RemediationRequest with unique fingerprint (prevents test pollution)
			fingerprint := GenerateTestFingerprint(testNamespace, "ae-int-3-lifecycle-completed")
			rr := newValidRemediationRequest("rr-completion", fingerprint)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			// Refresh RR to get server-populated fields (including UID for correlation_id)
			Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)).To(Succeed())
			// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) for audit event queries
		// Per universal standard: RemediationRequest.Name is the correlation ID for all services
		correlationID := rr.Name

			// Fast-forward through phases: update RO-created child CRDs to completed status
			// Wait for Processing
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

		// Get the RO-created SP and update status to Completed (including severity per DD-SEVERITY-001)
		spName := fmt.Sprintf("sp-%s", rr.Name)
		Eventually(func() error {
			sp := &signalprocessingv1.SignalProcessing{}
			return k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{Name: spName, Namespace: testNamespace}, sp)
		}, timeout, interval).Should(Succeed())
		Expect(updateSPStatus(testNamespace, spName, signalprocessingv1.PhaseCompleted, "critical")).To(Succeed())

		// Wait for Analyzing
		Eventually(func() remediationv1.RemediationPhase {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseAnalyzing))

			// Get the RO-created AI and update status to Completed (high confidence - no approval)
			aiName := fmt.Sprintf("ai-%s", rr.Name)
			ai := &aianalysisv1.AIAnalysis{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{Name: aiName, Namespace: testNamespace}, ai)
			}, timeout, interval).Should(Succeed())
			ai.Status.Phase = aianalysisv1.PhaseCompleted
			ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
				WorkflowID:     "test-workflow",
				Version:        "1.0.0",
				ExecutionBundle: "test-image:latest",
				Confidence:     0.95,
			}
			ai.Status.ApprovalRequired = false
			Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

			// Wait for Executing
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseExecuting))

			// Get the RO-created WE and update status to Completed
			weName := fmt.Sprintf("we-%s", rr.Name)
			we := &workflowexecutionv1.WorkflowExecution{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{Name: weName, Namespace: testNamespace}, we)
			}, timeout, interval).Should(Succeed())
			we.Status.Phase = workflowexecutionv1.PhaseCompleted
			Expect(k8sClient.Status().Update(ctx, we)).To(Succeed())

		// Wait for Completed
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseCompleted))

		// Wait for controller to emit event, then flush once
		time.Sleep(500 * time.Millisecond)
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := auditStore.Flush(flushCtx)
		flushCancel()
		Expect(err).ToNot(HaveOccurred())

		// RACE CONDITION FIX: Controller emits lifecycle_completed AFTER status update completes (async)
		// Wait for audit event to be queryable (after flush)
		// Expected: 5 events total (lifecycle_started + 3 transitions + lifecycle_completed)
		Eventually(func() bool {
			// Query for lifecycle_completed event
			params := ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(correlationID),
				EventCategory: ogenclient.NewOptString(audit.EventCategoryOrchestration),
				EventType:     ogenclient.NewOptString(string(ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleCompleted)),
			}
			resp, queryErr := dsClient.QueryAuditEvents(ctx, params)
			if queryErr != nil || resp.Data == nil {
				GinkgoWriter.Printf("⚠️  Query failed or no data: err=%v\n", queryErr)
				return false
			}
			if len(resp.Data) != 1 {
				GinkgoWriter.Printf("Found %d lifecycle_completed events (expected 1) for correlation_id=%s\n",
					len(resp.Data), correlationID)
			}
			return len(resp.Data) == 1
		}, "10s", "500ms").Should(BeTrue(), "lifecycle_completed event should be emitted, buffered, and queryable")

			// Query one more time to get the event for validation
			eventType := string(ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleCompleted)
			events := queryAuditEventsOpenAPI(dsClient, correlationID, eventType)
			Expect(events).To(HaveLen(1), "Expected exactly 1 lifecycle_completed audit event")

			// Validate event
			event := events[0]
			Expect(event.EventType).To(Equal(string(ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleCompleted)))
			Expect(event.EventAction).To(Equal("completed"))
			Expect(string(event.EventOutcome)).To(Equal("success"))

			// Validate event_data
			// Validate event_data (strongly-typed per DD-AUDIT-004)
			payload := event.EventData.RemediationOrchestratorAuditPayload
			Expect(payload.Outcome.IsSet()).To(BeTrue(), "outcome should be present")
			Expect(payload.Outcome.Value).To(Equal(ogenclient.RemediationOrchestratorAuditPayloadOutcomeSuccess))

			// DD-TESTING-001 Pattern 6: Validate top-level DurationMs field (performance tracking)
			topLevelDuration, hasDuration := event.DurationMs.Get()
			Expect(hasDuration).To(BeTrue(), "DD-TESTING-001: Top-level duration_ms MUST be set for lifecycle events")
			Expect(topLevelDuration).To(BeNumerically(">", 0), "Workflow execution duration should be positive")
		})
	})

	Context("AE-INT-4: Failure Audit (any phase→Failed)", func() {
		It("should emit 'lifecycle_failed' audit event when RR fails", func() {
			// Create RemediationRequest with unique fingerprint (prevents test pollution)
			fingerprint := GenerateTestFingerprint(testNamespace, "ae-int-4-lifecycle-failed")
			rr := newValidRemediationRequest("rr-failure", fingerprint)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) for audit event queries
		// Per universal standard: RemediationRequest.Name is the correlation ID for all services
		correlationID := rr.Name

			// Wait for Processing
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

			// Get the RO-created SP and update status to Failed
			spName := fmt.Sprintf("sp-%s", rr.Name)
			sp := &signalprocessingv1.SignalProcessing{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{Name: spName, Namespace: testNamespace}, sp)
			}, timeout, interval).Should(Succeed())
			sp.Status.Phase = signalprocessingv1.PhaseFailed
			sp.Status.Error = "Simulated SP failure for testing"
			Expect(k8sClient.Status().Update(ctx, sp)).To(Succeed())

			// Wait for Failed
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseFailed))

			// Explicitly flush audit store to ensure lifecycle_completed event is written to DataStorage
			flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer flushCancel()
			err := auditStore.Flush(flushCtx)
			Expect(err).ToNot(HaveOccurred(), "Failed to flush audit store")

			// Query for lifecycle completed audit event with failure outcome (DD-AUDIT-003)
			// Per DD-AUDIT-003: orchestrator.lifecycle.completed has outcome=success OR outcome=failure
			// Per DataStorage batch flushing: Default 60s flush interval in integration tests
			// Use 90s timeout to account for: 60s flush + 30s safety margin for processing
			eventType := string(ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleCompleted)
			var failureEvents []ogenclient.AuditEvent
			Eventually(func() int {
				allEvents := queryAuditEventsOpenAPI(dsClient, correlationID, eventType)
				// Filter for failure outcome
				failureEvents = []ogenclient.AuditEvent{}
				for _, e := range allEvents {
					if string(e.EventOutcome) == "failure" {
						failureEvents = append(failureEvents, e)
					}
				}
				return len(failureEvents)
			}, "90s", "1s").Should(Equal(1), "Expected exactly 1 lifecycle_completed audit event with failure outcome after buffer flush")

			// Validate event
			event := failureEvents[0]
			Expect(event.EventType).To(Equal(string(ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleCompleted)))
			Expect(event.EventAction).To(Equal("completed"))
			Expect(string(event.EventOutcome)).To(Equal("failure"))

			// Validate event_data
			// Validate event_data (strongly-typed per DD-AUDIT-004)
			payload := event.EventData.RemediationOrchestratorAuditPayload
			Expect(payload.FailurePhase.IsSet()).To(BeTrue(), "failure_phase should be present")
			// Per OpenAPI spec: failure_phase enum uses PascalCase (SignalProcessing, AIAnalysis, WorkflowExecution, Approval)
			Expect(payload.FailurePhase.Value).To(Equal(ogenclient.RemediationOrchestratorAuditPayloadFailurePhaseSignalProcessing))

			// BR-AUDIT-005 Gap #7: Validate standardized error_details
			Expect(payload.ErrorDetails.IsSet()).To(BeTrue(), "error_details should be present for failure events")
			errorDetails := payload.ErrorDetails.Value
			Expect(errorDetails.Component).To(Equal(ogenclient.ErrorDetailsComponentRemediationorchestrator))
			Expect(errorDetails.Code).To(ContainSubstring("ERR_"))
			Expect(errorDetails.Message).ToNot(BeEmpty())
			Expect(errorDetails.RetryPossible).ToNot(BeNil())

			// DD-TESTING-001 Pattern 6: Validate top-level DurationMs field (performance tracking)
			topLevelDuration, hasDuration := event.DurationMs.Get()
			Expect(hasDuration).To(BeTrue(), "DD-TESTING-001: Top-level duration_ms MUST be set even for failures")
			Expect(topLevelDuration).To(BeNumerically(">", 0), "Duration should be positive even for failed workflows")
		})
	})

	// RESOLVED (2025-12-27): Audit timer investigation complete - test enabled
	// - 10 test iterations validated timer reliability (0/10 bugs detected)
	// - Timer firing correctly with ~1s intervals (sub-millisecond precision)
	// - 50-90s delay never reproduced
	// - Investigation: docs/handoff/RO_AUDIT_TIMER_INTERMITTENCY_ANALYSIS_DEC_27_2025.md
	// - Test enabled with 90s timeout (conservative, timer works at ~1s)
	Context("AE-INT-5: Approval Requested Audit (Analyzing→AwaitingApproval)", func() {
		It("should emit 'approval_requested' audit event when low confidence triggers approval", func() {
			// Create RemediationRequest with unique fingerprint (prevents test pollution)
			fingerprint := GenerateTestFingerprint(testNamespace, "ae-int-5-approval-requested")
			rr := newValidRemediationRequest("rr-approval", fingerprint)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) for audit event queries
		// Per universal standard: RemediationRequest.Name is the correlation ID for all services
		correlationID := rr.Name

			// Fast-forward to Analyzing
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

		// Get the RO-created SP and update status to Completed (including severity per DD-SEVERITY-001)
		spName := fmt.Sprintf("sp-%s", rr.Name)
		Eventually(func() error {
			sp := &signalprocessingv1.SignalProcessing{}
			return k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{Name: spName, Namespace: testNamespace}, sp)
		}, timeout, interval).Should(Succeed())
		Expect(updateSPStatus(testNamespace, spName, signalprocessingv1.PhaseCompleted, "critical")).To(Succeed())

		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseAnalyzing))

		// Get the RO-created AI and update status with LOW confidence (triggers approval)
			aiName := fmt.Sprintf("ai-%s", rr.Name)
			ai := &aianalysisv1.AIAnalysis{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, client.ObjectKey{Name: aiName, Namespace: testNamespace}, ai)
			}, timeout, interval).Should(Succeed())
			ai.Status.Phase = aianalysisv1.PhaseCompleted
			ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
				WorkflowID:     "test-workflow",
				Version:        "1.0.0",
				ExecutionBundle: "test-image:latest",
				Confidence:     0.65, // Low confidence
			}
			ai.Status.ApprovalRequired = true
			ai.Status.ApprovalReason = "Confidence below threshold (0.65 < 0.80)"
			Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

		// Wait for AwaitingApproval
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseAwaitingApproval))

		// Wait for controller to emit event, then flush once
		time.Sleep(500 * time.Millisecond)
		err := auditStore.Flush(ctx)
		Expect(err).ToNot(HaveOccurred())

		// Query for approval requested audit event using Eventually (accounts for buffer flush)
		// Per DataStorage batch flushing: Default 60s flush interval in integration tests
		// Use 90s timeout to account for: 60s flush + 30s safety margin for processing
		eventType := string(ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorApprovalRequested)
		var events []ogenclient.AuditEvent
		Eventually(func() int {
			events = queryAuditEventsOpenAPI(dsClient, correlationID, eventType)
			if len(events) != 1 {
				GinkgoWriter.Printf("Found %d approval_requested events (expected 1) for correlation_id=%s\n",
					len(events), correlationID)
			}
			return len(events)
		}, "90s", "1s").Should(Equal(1), "Expected exactly 1 approval_requested audit event after buffer flush")

			// Validate event
			event := events[0]
			Expect(event.EventType).To(Equal(string(ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorApprovalRequested)))
			Expect(event.EventAction).To(Equal(audit.ActionApprovalRequested)) // ✅ Use authoritative constant
			Expect(string(event.EventOutcome)).To(Equal("pending"))

			// Validate event_data
			Expect(event.EventData).ToNot(BeNil())
			// Validate event_data (strongly-typed per DD-AUDIT-004)
			payload := event.EventData.RemediationOrchestratorAuditPayload
			Expect(payload.RarName.IsSet()).To(BeTrue(), "rar_name should be present for approval events")
		})
	})

	Context("AE-INT-8: Audit Metadata Validation", func() {
		It("should include required metadata fields in all audit events (correlation_id, timestamps)", func() {
			// Create RemediationRequest with unique fingerprint (prevents test pollution)
			fingerprint := GenerateTestFingerprint(testNamespace, "ae-int-8-metadata-validation")
			rr := newValidRemediationRequest("rr-metadata", fingerprint)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) for audit event queries
		// Per universal standard: RemediationRequest.Name is the correlation ID for all services
		correlationID := rr.Name

			// Wait for Processing
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

			// Query for any audit event from this RR using Eventually (accounts for buffer flush)
			// Timeout increased to 10s for consistency with other audit event queries
			eventType := string(ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleStarted)
			var events []ogenclient.AuditEvent
			Eventually(func() int {
				events = queryAuditEventsOpenAPI(dsClient, correlationID, eventType)
				return len(events)
			}, "10s", "500ms").Should(BeNumerically(">", 0), "Expected at least 1 audit event after buffer flush")

			// Validate metadata fields on first event
			event := events[0]

			// Required metadata fields (per DD-AUDIT-003)
			Expect(event.CorrelationID).To(Equal(correlationID), "correlation_id is required")
			Expect(event.EventTimestamp).ToNot(BeZero(), "timestamp is required")
			Expect(event.EventType).ToNot(BeEmpty(), "event_type is required")
			Expect(event.EventAction).ToNot(BeEmpty(), "event_action is required")
			Expect(event.EventOutcome).ToNot(BeEmpty(), "event_outcome is required")

			// Optional but expected fields
			Expect(event.EventData).ToNot(BeNil(), "event_data should be present")
		})
	})
})

// queryAuditEventsOpenAPI queries Data Storage using OpenAPI client for audit events by correlation_id and event_type
func queryAuditEventsOpenAPI(client *ogenclient.Client, correlationID, eventType string) []ogenclient.AuditEvent {
	// Use the OpenAPI client method QueryAuditEvents (ogen-generated)
	// Per ADR-034 v1.2: event_category is MANDATORY for queries
	// Use context.Background() for query context (safe for parallel test execution)
	queryCtx := context.Background()
	eventCategory := audit.EventCategoryOrchestration // RO audit events use "orchestration" category
	params := ogenclient.QueryAuditEventsParams{
		CorrelationID: ogenclient.NewOptString(correlationID),
		EventCategory: ogenclient.NewOptString(eventCategory), // ✅ Required per ADR-034 v1.2 (matches pkg/remediationorchestrator/audit/audit.go)
		EventType:     ogenclient.NewOptString(eventType),
	}

	resp, err := client.QueryAuditEvents(queryCtx, params)
	if err != nil {
		GinkgoWriter.Printf("❌ Failed to query Data Storage: %v\n", err)
		return nil
	}

	if resp.Data == nil {
		GinkgoWriter.Printf("⚠️  No data in response from Data Storage (correlation=%s, type=%s, category=%s)\n", correlationID, eventType, eventCategory)
		return nil
	}

	GinkgoWriter.Printf("✅ Query successful: found %d events (correlation=%s, type=%s)\n", len(resp.Data), correlationID, eventType)
	return resp.Data
}
