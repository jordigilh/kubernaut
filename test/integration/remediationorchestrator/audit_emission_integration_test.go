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
	const (
		// Use 127.0.0.1 instead of localhost to force IPv4
		// (macOS sometimes resolves localhost to ::1 IPv6, which may not be accessible)
		dataStorageURL = "http://127.0.0.1:18140" // Data Storage API port from podman-compose
	)

	var (
		testNamespace string
		dsClient      *ogenclient.Client
	)

	BeforeEach(func() {
		// Create Data Storage OpenAPI client (each test needs its own client instance for parallel execution)
		var err error
		dsClient, err = ogenclient.NewClient(dataStorageURL)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Data Storage OpenAPI client")
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
				SignalType:        "prometheus",
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

			correlationID := string(rr.UID)

			// Wait for RO to transition to Processing (creates SignalProcessing CRD)
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

		// Query Data Storage API for lifecycle_started audit event using OpenAPI client
		// Use Eventually because audit events are buffered (FlushInterval: 1s)
		// Timeout: 90s (conservative, timer works at ~1s but allows for infrastructure delays)
		eventType := "orchestrator.lifecycle.started"
		var events []ogenclient.AuditEvent
		Eventually(func() int {
			events = queryAuditEventsOpenAPI(dsClient, correlationID, eventType)
			return len(events)
		}, "90s", "1s").Should(Equal(1), "Expected exactly 1 lifecycle_started audit event after buffer flush")

			// Validate event structure (typed response from OpenAPI client)
			event := events[0]
			Expect(event.EventType).To(Equal("orchestrator.lifecycle.started"))
			Expect(event.EventAction).To(Equal("started"))
			Expect(string(event.EventOutcome)).To(Equal("pending"), "lifecycle.started uses pending outcome - outcome not yet determined")
			Expect(event.CorrelationId).To(Equal(correlationID))

		// Validate event_data (JSONB field as interface{})
		// Note: Integration tests receive data from HTTP API as map[string]interface{} (JSON deserialization)
		Expect(event.EventData).ToNot(BeNil())
		eventData, ok := event.EventData.(map[string]interface{})
		Expect(ok).To(BeTrue(), "event_data should be map[string]interface{} from HTTP API")
		Expect(eventData).To(HaveKey("rr_name"))
		Expect(eventData["rr_name"]).To(Equal("rr-lifecycle-started"))
		Expect(eventData).To(HaveKey("namespace"))
		Expect(eventData["namespace"]).To(Equal(testNamespace))
		})
	})

	Context("AE-INT-2: Phase Transition Audit (Processing→Analyzing)", func() {
		It("should emit 'phase_transition' audit event when RR transitions phases", func() {
			// Create RemediationRequest with unique fingerprint (prevents test pollution)
			fingerprint := GenerateTestFingerprint(testNamespace, "ae-int-2-phase-transition")
			rr := newValidRemediationRequest("rr-phase-transition", fingerprint)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			correlationID := string(rr.UID)

			// Wait for Processing phase (RO creates SignalProcessing CRD)
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

			// Get the RO-created SignalProcessing CRD and update its status to Completed
			spName := fmt.Sprintf("sp-%s", rr.Name)
			sp := &signalprocessingv1.SignalProcessing{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: spName, Namespace: testNamespace}, sp)
			}, timeout, interval).Should(Succeed(), "Expected RO to create SignalProcessing CRD")

			// Update SP status to Completed to trigger transition to Analyzing
			sp.Status.Phase = signalprocessingv1.PhaseCompleted
			Expect(k8sClient.Status().Update(ctx, sp)).To(Succeed())

			// Wait for transition to Analyzing
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseAnalyzing))

		// Query for phase transition audit event using Eventually (accounts for buffer flush)
		// Timeout increased to 10s for consistency with other audit event queries (AE-INT-3, AE-INT-4, AE-INT-8)
		// and to account for audit store flush interval (1s) + network latency + query processing
		eventType := "orchestrator.phase.transitioned"
		var transitionEvent *ogenclient.AuditEvent
		Eventually(func() bool {
			events := queryAuditEventsOpenAPI(dsClient, correlationID, eventType)
		// Find the Processing→Analyzing transition
		for i, e := range events {
			if e.EventData != nil {
				eventData, ok := e.EventData.(map[string]interface{})
				if ok && eventData["from_phase"] == "Processing" && eventData["to_phase"] == "Analyzing" {
					transitionEvent = &events[i]
					return true
				}
			}
		}
		return false
		}, "10s", "500ms").Should(BeTrue(), "Expected Processing→Analyzing transition event after buffer flush")

			// Validate event
			Expect(transitionEvent.EventType).To(Equal("orchestrator.phase.transitioned"))
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

			correlationID := string(rr.UID)

			// Fast-forward through phases: update RO-created child CRDs to completed status
			// Wait for Processing
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

			// Get the RO-created SP and update status to Completed
			spName := fmt.Sprintf("sp-%s", rr.Name)
			sp := &signalprocessingv1.SignalProcessing{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: spName, Namespace: testNamespace}, sp)
			}, timeout, interval).Should(Succeed())
			sp.Status.Phase = signalprocessingv1.PhaseCompleted
			Expect(k8sClient.Status().Update(ctx, sp)).To(Succeed())

			// Wait for Analyzing
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseAnalyzing))

			// Get the RO-created AI and update status to Completed (high confidence - no approval)
			aiName := fmt.Sprintf("ai-%s", rr.Name)
			ai := &aianalysisv1.AIAnalysis{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: aiName, Namespace: testNamespace}, ai)
			}, timeout, interval).Should(Succeed())
			ai.Status.Phase = aianalysisv1.PhaseCompleted
			ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
				WorkflowID:     "test-workflow",
				Version:        "1.0.0",
				ContainerImage: "test-image:latest",
				Confidence:     0.95,
			}
			ai.Status.ApprovalRequired = false
			Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

			// Wait for Executing
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseExecuting))

			// Get the RO-created WE and update status to Completed
			weName := fmt.Sprintf("we-%s", rr.Name)
			we := &workflowexecutionv1.WorkflowExecution{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: weName, Namespace: testNamespace}, we)
			}, timeout, interval).Should(Succeed())
			we.Status.Phase = workflowexecutionv1.PhaseCompleted
			Expect(k8sClient.Status().Update(ctx, we)).To(Succeed())

			// Wait for Completed
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseCompleted))

		// Query for lifecycle completed audit event using Eventually (accounts for buffer flush)
		// Timeout increased to 10s to account for system load during full test suite execution
		// (audit store has 1s flush interval + network latency + query processing)
		eventType := "orchestrator.lifecycle.completed"
		var events []ogenclient.AuditEvent
		Eventually(func() int {
			events = queryAuditEventsOpenAPI(dsClient, correlationID, eventType)
			return len(events)
		}, "10s", "500ms").Should(Equal(1), "Expected exactly 1 lifecycle_completed audit event after buffer flush")

			// Validate event
			event := events[0]
			Expect(event.EventType).To(Equal("orchestrator.lifecycle.completed"))
			Expect(event.EventAction).To(Equal("completed"))
			Expect(string(event.EventOutcome)).To(Equal("success"))

		// Validate event_data
		Expect(event.EventData).ToNot(BeNil())
		eventData, ok := event.EventData.(map[string]interface{})
		Expect(ok).To(BeTrue())
		Expect(eventData).To(HaveKey("outcome"))
		Expect(eventData["outcome"]).To(Equal("Remediated")) // Actual value from controller
		})
	})

	Context("AE-INT-4: Failure Audit (any phase→Failed)", func() {
		It("should emit 'lifecycle_failed' audit event when RR fails", func() {
			// Create RemediationRequest with unique fingerprint (prevents test pollution)
			fingerprint := GenerateTestFingerprint(testNamespace, "ae-int-4-lifecycle-failed")
			rr := newValidRemediationRequest("rr-failure", fingerprint)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			correlationID := string(rr.UID)

			// Wait for Processing
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

			// Get the RO-created SP and update status to Failed
			spName := fmt.Sprintf("sp-%s", rr.Name)
			sp := &signalprocessingv1.SignalProcessing{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: spName, Namespace: testNamespace}, sp)
			}, timeout, interval).Should(Succeed())
			sp.Status.Phase = signalprocessingv1.PhaseFailed
			sp.Status.Error = "Simulated SP failure for testing"
			Expect(k8sClient.Status().Update(ctx, sp)).To(Succeed())

		// Wait for Failed
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseFailed))

		// Query for lifecycle completed audit event with failure outcome (DD-AUDIT-003)
		// Per DD-AUDIT-003: orchestrator.lifecycle.completed has outcome=success OR outcome=failure
		// Timeout increased to 10s for consistency with other audit tests and to account for query/filter overhead
		eventType := "orchestrator.lifecycle.completed"
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
		}, "10s", "500ms").Should(Equal(1), "Expected exactly 1 lifecycle_completed audit event with failure outcome after buffer flush")

		// Validate event
		event := failureEvents[0]
		Expect(event.EventType).To(Equal("orchestrator.lifecycle.completed"))
		Expect(event.EventAction).To(Equal("completed"))
		Expect(string(event.EventOutcome)).To(Equal("failure"))

	// Validate event_data
	Expect(event.EventData).ToNot(BeNil())
	eventData, ok := event.EventData.(map[string]interface{})
	Expect(ok).To(BeTrue())
	Expect(eventData).To(HaveKey("failure_phase"))
	// Per reconciler.go:454 - failurePhase is "signal_processing" when SP fails
	Expect(eventData["failure_phase"]).To(Equal("signal_processing"))
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

			correlationID := string(rr.UID)

			// Fast-forward to Analyzing
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

			// Get the RO-created SP and update status to Completed
			spName := fmt.Sprintf("sp-%s", rr.Name)
			sp := &signalprocessingv1.SignalProcessing{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: spName, Namespace: testNamespace}, sp)
			}, timeout, interval).Should(Succeed())
			sp.Status.Phase = signalprocessingv1.PhaseCompleted
			Expect(k8sClient.Status().Update(ctx, sp)).To(Succeed())

			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseAnalyzing))

			// Get the RO-created AI and update status with LOW confidence (triggers approval)
			aiName := fmt.Sprintf("ai-%s", rr.Name)
			ai := &aianalysisv1.AIAnalysis{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: aiName, Namespace: testNamespace}, ai)
			}, timeout, interval).Should(Succeed())
			ai.Status.Phase = aianalysisv1.PhaseCompleted
			ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
				WorkflowID:     "test-workflow",
				Version:        "1.0.0",
				ContainerImage: "test-image:latest",
				Confidence:     0.65, // Low confidence
			}
			ai.Status.ApprovalRequired = true
			ai.Status.ApprovalReason = "Confidence below threshold (0.65 < 0.80)"
			Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

			// Wait for AwaitingApproval
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseAwaitingApproval))

		// Query for approval requested audit event using Eventually (accounts for buffer flush)
		// Per DataStorage batch flushing: Default 60s flush interval in integration tests
		// Use 90s timeout to account for: 60s flush + 30s safety margin for processing
		eventType := "orchestrator.approval.requested"
		var events []ogenclient.AuditEvent
		Eventually(func() int {
			events = queryAuditEventsOpenAPI(dsClient, correlationID, eventType)
			return len(events)
		}, "90s", "1s").Should(Equal(1), "Expected exactly 1 approval_requested audit event after buffer flush")

		// Validate event
		event := events[0]
		Expect(event.EventType).To(Equal("orchestrator.approval.requested"))
		Expect(event.EventAction).To(Equal(audit.ActionApprovalRequested)) // ✅ Use authoritative constant
		Expect(string(event.EventOutcome)).To(Equal("pending"))

		// Validate event_data
		Expect(event.EventData).ToNot(BeNil())
		eventData, ok := event.EventData.(map[string]interface{})
		Expect(ok).To(BeTrue())
		Expect(eventData).To(HaveKey("approval_reason"))
		})
	})

	Context("AE-INT-8: Audit Metadata Validation", func() {
		It("should include required metadata fields in all audit events (correlation_id, timestamps)", func() {
			// Create RemediationRequest with unique fingerprint (prevents test pollution)
			fingerprint := GenerateTestFingerprint(testNamespace, "ae-int-8-metadata-validation")
			rr := newValidRemediationRequest("rr-metadata", fingerprint)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			correlationID := string(rr.UID)

			// Wait for Processing
			Eventually(func() remediationv1.RemediationPhase {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)
				return rr.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

		// Query for any audit event from this RR using Eventually (accounts for buffer flush)
		// Timeout increased to 10s for consistency with other audit event queries
		eventType := "orchestrator.lifecycle.started"
		var events []ogenclient.AuditEvent
		Eventually(func() int {
			events = queryAuditEventsOpenAPI(dsClient, correlationID, eventType)
			return len(events)
		}, "10s", "500ms").Should(BeNumerically(">", 0), "Expected at least 1 audit event after buffer flush")

			// Validate metadata fields on first event
			event := events[0]

			// Required metadata fields (per DD-AUDIT-003)
			Expect(event.CorrelationId).To(Equal(correlationID), "correlation_id is required")
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
	// Use the OpenAPI client method QueryAuditEventsWithResponse
	// Per ADR-034 v1.2: event_category is MANDATORY for queries
	// Use context.Background() for query context (safe for parallel test execution)
	queryCtx := context.Background()
	eventCategory := "orchestration" // RO audit events use "orchestration" category
	params := &ogenclient.QueryAuditEventsParams{
		CorrelationId: &correlationID,
		EventCategory: &eventCategory, // ✅ Required per ADR-034 v1.2 (matches pkg/remediationorchestrator/audit/audit.go)
		EventType:     &eventType,
	}

	resp, err := client.QueryAuditEventsWithResponse(queryCtx, params)
	if err != nil {
		GinkgoWriter.Printf("Failed to query Data Storage: %v\n", err)
		return nil
	}

	if resp.StatusCode() != 200 {
		GinkgoWriter.Printf("Data Storage returned non-200: %d\n", resp.StatusCode())
		return nil
	}

	if resp.JSON200 == nil || resp.JSON200.Data == nil {
		GinkgoWriter.Printf("No data in response from Data Storage\n")
		return nil
	}

	return *resp.JSON200.Data
}
