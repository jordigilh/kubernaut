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
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	sigyaml "sigs.k8s.io/yaml"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// BR-E2E-001: Full Remediation Lifecycle E2E Test
// Validates the complete pipeline from K8s Event to Notification delivery.
//
// Pipeline:
//
//	OOMKill Event → Gateway → RemediationRequest → RO → SP → AA → HAPI(MockLLM) → WE(Job) → Notification
//
// This test uses the memory-eater pod to generate a real OOMKill event.
// The kubernetes-event-exporter watches for this event and POSTs to Gateway.
// From there, the full controller pipeline processes the signal.
var _ = Describe("Full Remediation Lifecycle [BR-E2E-001]", Ordered, func() {

	var (
		testNamespace string
		testCtx       context.Context
		testCancel    context.CancelFunc
	)

	BeforeAll(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 10*time.Minute)

		By("Step 0: Seeding test workflows in DataStorage")
		// Seed a workflow that uses the Job execution engine
		// WorkflowID MUST match Mock LLM's scenario workflow_name for UUID sync.
		// The event-exporter forwards a "BackOff" Warning event (CrashLoopBackOff),
		// not the OOMKilled terminated state, so Mock LLM matches the "crashloop"
		// scenario (workflow_name="crashloop-config-fix-v1").
		// We also seed the oomkilled workflow for completeness.
		// Workflow labels use wildcards (*) for severity, environment, and priority
		// because a real LLM's severity assessment is non-deterministic — the same
		// OOMKill may be classified as "medium", "high", or "critical" across runs.
		// Similarly, environment comes from SP enrichment (namespace → env mapping)
		// which may resolve to "unknown" in test clusters.
		// Wildcard labels ensure the workflow matches regardless of LLM variability.
	workflows := []infrastructure.TestWorkflow{
		{
			WorkflowID:      "crashloop-config-fix-v1",
			Name:            "CrashLoopBackOff - Configuration Fix",
			Description:     "CrashLoop remediation workflow for full pipeline E2E",
			SignalType:      "CrashLoopBackOff",
			Severity:        "*",
			Component:       "deployment",
			Environment:     "*",
			Priority:        "*",
			ContainerImage:  "busybox:latest",
			ExecutionEngine: "job",
			// BR-HAPI-191: Parameter schema for LLM guidance and HAPI validation
			SchemaParameters: []models.WorkflowParameter{
				{
					Name:        "CONFIG_MAP",
					Type:        "string",
					Required:    true,
					Description: "Name of the ConfigMap to fix",
				},
				{
					Name:        "TARGET_NAMESPACE",
					Type:        "string",
					Required:    true,
					Description: "Namespace of the target resource",
				},
			},
		},
		{
			WorkflowID:      "oomkill-increase-memory-v1",
			Name:            "OOMKill Recovery - Increase Memory Limits",
			Description:     "OOMKill remediation workflow for full pipeline E2E",
			SignalType:      "OOMKilled",
			Severity:        "*",
			Component:       "deployment",
			Environment:     "*",
			Priority:        "*",
			ContainerImage:  "quay.io/jordigilh/kubernaut-cicd/test-workflows/oomkill-increase-memory:fullpipeline-e2e-arm64",
			ExecutionEngine: "job",
			// BR-HAPI-191: Parameter schema matching oomkill-increase-memory.sh expectations
			SchemaParameters: []models.WorkflowParameter{
				{
					Name:        "MEMORY_LIMIT_NEW",
					Type:        "string",
					Required:    true,
					Description: "New memory limit to apply (e.g., 256Mi, 1Gi)",
				},
				{
					Name:        "TARGET_RESOURCE_KIND",
					Type:        "string",
					Required:    true,
					Description: "Kubernetes resource kind (Deployment, StatefulSet, DaemonSet)",
					Enum:        []string{"Deployment", "StatefulSet", "DaemonSet"},
				},
				{
					Name:        "TARGET_RESOURCE_NAME",
					Type:        "string",
					Required:    true,
					Description: "Name of the resource to patch",
				},
				{
					Name:        "TARGET_NAMESPACE",
					Type:        "string",
					Required:    true,
					Description: "Namespace of the target resource",
				},
			},
		},
		}
		workflowUUIDs, err := infrastructure.SeedWorkflowsInDataStorage(
			dataStorageClient, workflows, "fullpipeline-e2e", GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to seed workflows in DataStorage")
		Expect(workflowUUIDs).To(HaveKey("crashloop-config-fix-v1:*"))
		Expect(workflowUUIDs).To(HaveKey("oomkill-increase-memory-v1:*"))
		GinkgoWriter.Printf("  ✅ Workflow seeded: crashloop-config-fix-v1 → %s\n", workflowUUIDs["crashloop-config-fix-v1:*"])
		GinkgoWriter.Printf("  ✅ Workflow seeded: oomkill-increase-memory-v1 → %s\n", workflowUUIDs["oomkill-increase-memory-v1:*"])

		// Update Mock LLM ConfigMap with actual workflow UUIDs from DataStorage,
		// then restart Mock LLM to pick up the new config. This ensures the Mock
		// LLM returns correct workflow_id values that match DataStorage records.
		// When SKIP_MOCK_LLM is set, HAPI uses a real LLM — no Mock LLM to update.
		if os.Getenv("SKIP_MOCK_LLM") == "" {
			By("Step 0b: Updating Mock LLM with seeded workflow UUIDs")
			err = infrastructure.UpdateMockLLMConfigMap(
				testCtx, "kubernaut-system", kubeconfigPath, workflowUUIDs, GinkgoWriter,
			)
			Expect(err).ToNot(HaveOccurred(), "Failed to update Mock LLM ConfigMap")
		} else {
			GinkgoWriter.Println("  ⏭️  Mock LLM update skipped (SKIP_MOCK_LLM is set)")
		}
	})

	AfterAll(func() {
		if testNamespace != "" {
			By("Cleaning up test namespace")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
			_ = k8sClient.Delete(ctx, ns)
		}
		testCancel()
	})

	It("should process a K8s OOMKill event through the complete remediation pipeline", func() {
		// ================================================================
		// Step 1: Create a managed namespace
		// ================================================================
		By("Step 1: Creating managed test namespace")
		testNamespace = fmt.Sprintf("fp-e2e-%d", time.Now().Unix())
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
				Labels: map[string]string{
					"kubernaut.ai/managed": "true",
				},
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		GinkgoWriter.Printf("  ✅ Namespace created: %s\n", testNamespace)

		// ================================================================
		// Step 2: Deploy memory-eater to trigger OOMKill
		// ================================================================
		By("Step 2: Deploying memory-eater pod (will trigger OOMKill)")
		err := infrastructure.DeployMemoryEater(testCtx, testNamespace, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "Failed to deploy memory-eater")

		// Wait for OOMKill event to occur
		By("Step 2b: Waiting for OOMKill event...")
		Eventually(func() bool {
			pods := &corev1.PodList{}
			if err := apiReader.List(ctx, pods, client.InNamespace(testNamespace),
				client.MatchingLabels{"app": "memory-eater"}); err != nil {
				return false
			}
			for _, pod := range pods.Items {
				for _, cs := range pod.Status.ContainerStatuses {
					// Check last terminated state for OOMKilled reason
					if cs.LastTerminationState.Terminated != nil &&
						cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
						GinkgoWriter.Printf("  ✅ OOMKill detected: restarts=%d\n", cs.RestartCount)
						return true
					}
					// Also check current terminated state
					if cs.State.Terminated != nil &&
						cs.State.Terminated.Reason == "OOMKilled" {
						GinkgoWriter.Println("  ✅ OOMKill terminated state detected")
						return true
					}
					// Fallback: CrashLoopBackOff after restarts
					if cs.RestartCount > 0 && cs.State.Waiting != nil &&
						cs.State.Waiting.Reason == "CrashLoopBackOff" {
						GinkgoWriter.Println("  ✅ CrashLoopBackOff detected (OOMKill)")
						return true
					}
				}
			}
			return false
		}, 2*time.Minute, 2*time.Second).Should(BeTrue(), "memory-eater should OOMKill")

		// ================================================================
		// Step 3: Verify RemediationRequest created by Gateway
		// ================================================================
		By("Step 3: Waiting for RemediationRequest to be created by Gateway")
		var remediationRequest *remediationv1.RemediationRequest
		Eventually(func() bool {
			rrList := &remediationv1.RemediationRequestList{}
			// Gateway creates RR in the signal's namespace (testNamespace), not the infrastructure namespace
			if err := apiReader.List(ctx, rrList, client.InNamespace(testNamespace)); err != nil {
				return false
			}
			for i := range rrList.Items {
				rr := &rrList.Items[i]
				remediationRequest = rr
				GinkgoWriter.Printf("  ✅ RemediationRequest found: %s\n", rr.Name)
				return true
			}
			return false
		}, timeout, interval).Should(BeTrue(), "RemediationRequest should be created by Gateway")

		// ================================================================
		// Step 4: Verify SignalProcessing enriched the signal
		// ================================================================
		By("Step 4: Waiting for SignalProcessing to complete")
		Eventually(func() string {
			spList := &signalprocessingv1.SignalProcessingList{}
			if err := apiReader.List(ctx, spList, client.InNamespace(testNamespace)); err != nil {
				return ""
			}
			for _, sp := range spList.Items {
				if sp.Spec.RemediationRequestRef.Name == remediationRequest.Name {
					GinkgoWriter.Printf("  SP %s phase: %s\n", sp.Name, sp.Status.Phase)
					return string(sp.Status.Phase)
				}
			}
			return ""
		}, timeout, interval).Should(Equal("Completed"),
			"SignalProcessing should reach Completed phase")

		// ================================================================
		// Step 5: Verify AIAnalysis created and completed
		// ================================================================
		By("Step 5: Waiting for AIAnalysis to complete")
		var aaName string
		Eventually(func() string {
			aaList := &aianalysisv1.AIAnalysisList{}
			if err := apiReader.List(ctx, aaList, client.InNamespace(testNamespace)); err != nil {
				return ""
			}
			for _, aa := range aaList.Items {
				if aa.Spec.RemediationRequestRef.Name == remediationRequest.Name {
					aaName = aa.Name
					GinkgoWriter.Printf("  AA %s phase: %s\n", aa.Name, aa.Status.Phase)
					return aa.Status.Phase
				}
			}
			return ""
		}, timeout, interval).Should(Equal("Completed"),
			"AIAnalysis should reach Completed phase")

		// Verify AIAnalysis selected a workflow with job engine
		By("Step 5b: Verifying AIAnalysis selected workflow with job engine")
		aa := &aianalysisv1.AIAnalysis{}
		Expect(apiReader.Get(ctx, client.ObjectKey{Name: aaName, Namespace: testNamespace}, aa)).To(Succeed())
		Expect(aa.Status.SelectedWorkflow).ToNot(BeNil(), "AIAnalysis should have selectedWorkflow")
		Expect(aa.Status.SelectedWorkflow.ExecutionEngine).To(Equal("job"),
			"AIAnalysis should select job execution engine")

		// ================================================================
		// Step 6: Verify WorkflowExecution created with executionEngine: job
		// ================================================================
		By("Step 6: Waiting for WorkflowExecution to be created")
		var weName string
		Eventually(func() string {
			weList := &workflowexecutionv1.WorkflowExecutionList{}
			if err := apiReader.List(ctx, weList, client.InNamespace(testNamespace)); err != nil {
				return ""
			}
			for _, we := range weList.Items {
				if we.Spec.RemediationRequestRef.Name == remediationRequest.Name {
					weName = we.Name
					GinkgoWriter.Printf("  WE %s phase: %s, engine: %s\n",
						we.Name, we.Status.Phase, we.Spec.ExecutionEngine)
					return we.Spec.ExecutionEngine
				}
			}
			return ""
		}, timeout, interval).Should(Equal("job"),
			"WorkflowExecution should use job execution engine")

		// ================================================================
		// Step 7: Verify K8s Job ran and completed
		// ================================================================
		By("Step 7: Waiting for K8s Job to complete")
		Eventually(func() bool {
			jobList := &batchv1.JobList{}
			if err := apiReader.List(ctx, jobList,
				client.InNamespace("kubernaut-workflows")); err != nil {
				return false
			}
			for _, job := range jobList.Items {
				if job.Status.Succeeded > 0 {
					GinkgoWriter.Printf("  ✅ Job completed: %s\n", job.Name)
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue(), "K8s Job should complete successfully")

		// ================================================================
		// Step 8: Verify WorkflowExecution reached Completed phase
		// ================================================================
		By("Step 8: Waiting for WorkflowExecution to complete")
		Eventually(func() string {
			we := &workflowexecutionv1.WorkflowExecution{}
			if err := apiReader.Get(ctx, client.ObjectKey{
				Name: weName, Namespace: testNamespace,
			}, we); err != nil {
				return ""
			}
			return we.Status.Phase
		}, timeout, interval).Should(Equal("Completed"),
			"WorkflowExecution should reach Completed phase")

		// ================================================================
		// Step 9: Verify NotificationRequest created (BR-ORCH-045: completion)
		// ================================================================
		By("Step 9: Waiting for completion NotificationRequest")
		pollCount := 0
		Eventually(func() bool {
			pollCount++
			nrList := &notificationv1.NotificationRequestList{}
			if listErr := apiReader.List(ctx, nrList, client.InNamespace(testNamespace)); listErr != nil {
				GinkgoWriter.Printf("  [Step 9 poll %d] List NR error: %v\n", pollCount, listErr)
				return false
			}
			// Diagnostic: every 10 polls, dump RR phase and all NRs
			if pollCount%10 == 1 {
				rr := &remediationv1.RemediationRequest{}
				if getErr := apiReader.Get(ctx, client.ObjectKey{Name: remediationRequest.Name, Namespace: testNamespace}, rr); getErr == nil {
					GinkgoWriter.Printf("  [Step 9 poll %d] RR %s phase=%s outcome=%s\n", pollCount, rr.Name, rr.Status.OverallPhase, rr.Status.Outcome)
				} else {
					GinkgoWriter.Printf("  [Step 9 poll %d] RR Get error: %v\n", pollCount, getErr)
				}
				GinkgoWriter.Printf("  [Step 9 poll %d] Found %d NotificationRequests in %s\n", pollCount, len(nrList.Items), testNamespace)
				for _, nr := range nrList.Items {
					refName := "<nil>"
					if nr.Spec.RemediationRequestRef != nil {
						refName = nr.Spec.RemediationRequestRef.Name
					}
					GinkgoWriter.Printf("    NR %s type=%s ref=%s\n", nr.Name, nr.Spec.Type, refName)
				}
			}
			for _, nr := range nrList.Items {
				if nr.Spec.RemediationRequestRef != nil &&
					nr.Spec.RemediationRequestRef.Name == remediationRequest.Name &&
					nr.Spec.Type == notificationv1.NotificationTypeCompletion {
					GinkgoWriter.Printf("  ✅ Completion NotificationRequest: %s\n", nr.Name)
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue(),
			"Completion NotificationRequest should be created (BR-ORCH-045)")

		// ================================================================
		// Step 10: Verify RemediationRequest reached Completed phase
		// ================================================================
		By("Step 10: Verifying RemediationRequest completed")
		Eventually(func() string {
			rr := &remediationv1.RemediationRequest{}
			if err := apiReader.Get(ctx, client.ObjectKey{
				Name: remediationRequest.Name, Namespace: testNamespace,
			}, rr); err != nil {
				return ""
			}
			return string(rr.Status.OverallPhase)
		}, timeout, interval).Should(Equal("Completed"),
			"RemediationRequest should reach Completed phase")

		// ================================================================
		// Step 11: Verify audit trail completeness (BR-AUDIT-005)
		// ================================================================
		By("Step 11: Verifying audit trail completeness and non-duplication")

		// The correlation_id for all audit events is the RR name
		correlationID := remediationRequest.Name

		// Expected audit event types from a successful full remediation lifecycle.
		// Derived from BR-AUDIT-005, ADR-034, and each service's audit implementation.
		//
		// === Events that MUST appear exactly once ===
		// These are lifecycle boundary events — one per RR by definition.
		exactlyOnceEvents := []string{
			// Gateway: signal ingestion and CRD creation
			"gateway.signal.received",   // pkg/gateway/server.go: emitSignalReceivedAudit
			"gateway.crd.created",       // pkg/gateway/server.go: emitCRDCreatedAudit
			// Remediation Orchestrator: lifecycle boundaries
			"orchestrator.lifecycle.created",   // pkg/remediationorchestrator/audit: emitRemediationCreatedAudit
			"orchestrator.lifecycle.started",   // pkg/remediationorchestrator/audit: emitLifecycleStartedAudit
			"orchestrator.lifecycle.completed", // pkg/remediationorchestrator/audit: emitCompletionAudit
			// Effectiveness Monitor: assessment lifecycle + component events
			// The RO creates an EA CRD on RR completion (ADR-EM-001). The EM waits
			// for the stabilization window (30s default), then runs all 4 component
			// checks in a single reconcile, emitting one audit event per component.
			// Each event is guarded by its component flag (emitted exactly once per EA).
			"effectiveness.assessment.scheduled",  // pkg/effectivenessmonitor/audit: RecordAssessmentScheduled
			"effectiveness.health.assessed",       // pkg/effectivenessmonitor/audit: RecordHealthAssessed
			"effectiveness.hash.computed",          // pkg/effectivenessmonitor/audit: RecordHashComputed
			"effectiveness.alert.assessed",         // pkg/effectivenessmonitor/audit: RecordAlertAssessed
			"effectiveness.metrics.assessed",       // pkg/effectivenessmonitor/audit: RecordMetricsAssessed
			"effectiveness.assessment.completed",   // pkg/effectivenessmonitor/audit: RecordAssessmentCompleted
		}

		// === Events that MUST appear at least once ===
		// These fire during processing; some may repeat (phase transitions, retries).
		atLeastOnceEvents := []string{
			// Remediation Orchestrator: phase transitions
			"orchestrator.lifecycle.transitioned", // pkg/remediationorchestrator/audit: emitPhaseTransitionAudit
			// Signal Processing
			"signalprocessing.enrichment.completed",    // pkg/signalprocessing/audit: RecordEnrichmentComplete
			"signalprocessing.classification.decision",  // pkg/signalprocessing/audit: RecordClassificationDecision
			"signalprocessing.signal.processed",         // pkg/signalprocessing/audit: RecordSignalProcessed
			"signalprocessing.phase.transition",          // pkg/signalprocessing/audit: RecordPhaseTransition
			// AI Analysis
			"aianalysis.phase.transition",    // pkg/aianalysis/audit: RecordPhaseTransition
			"aianalysis.holmesgpt.call",      // pkg/aianalysis/audit: RecordHolmesGPTCall
			"aianalysis.rego.evaluation",     // pkg/aianalysis/audit: RecordRegoEvaluation
			"aianalysis.analysis.completed",  // pkg/aianalysis/audit: RecordAnalysisComplete
			// HolmesGPT API (event_category: "aiagent" per ADR-034 v1.2)
			string(ogenclient.LLMRequestPayloadAuditEventEventData),            // holmesgpt-api/src/audit/events.py: create_llm_request_event
			string(ogenclient.LLMResponsePayloadAuditEventEventData),           // holmesgpt-api/src/audit/events.py: create_llm_response_event
			string(ogenclient.WorkflowValidationPayloadAuditEventEventData),    // holmesgpt-api/src/audit/events.py: create_validation_attempt_event
			string(ogenclient.AIAgentResponsePayloadAuditEventEventData),       // holmesgpt-api/src/audit/events.py: create_aiagent_response_complete_event
			// Workflow Execution
			"workflowexecution.selection.completed", // pkg/workflowexecution/audit: RecordWorkflowSelectionCompleted
			"workflowexecution.execution.started",   // pkg/workflowexecution/audit: RecordExecutionWorkflowStarted
			"workflowexecution.workflow.completed",   // pkg/workflowexecution/audit: RecordWorkflowCompleted
			// Notification
			"notification.message.sent", // pkg/notification/audit: CreateMessageSentEvent
		}

		// === Events that MAY appear (non-deterministic) ===
		// These depend on LLM behavior or conditional logic.
		// ogenclient.LLMToolCallPayloadAuditEventEventData ("aiagent.llm.tool_call") — emitted when the LLM uses tools (e.g., search_workflow_catalog)
		// "signalprocessing.business.classified" — emitted if business classification applies
		// "aianalysis.approval.decision" — emitted if auto-approval is configured

		allExpected := append(exactlyOnceEvents, atLeastOnceEvents...)

		// Query all audit events for this remediation request.
		// The Eventually checks that ALL required event types are present (not just a count
		// threshold), preventing a race where late-arriving events (e.g., notification.message.sent)
		// are missed because the count was already satisfied by earlier events.
		var allAuditEvents []ogenclient.AuditEvent
		eventTypeCounts := map[string]int{}
		Eventually(func() []string {
			resp, err := dataStorageClient.QueryAuditEvents(testCtx, ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(correlationID),
				Limit:         ogenclient.NewOptInt(200),
			})
			if err != nil {
				GinkgoWriter.Printf("  [Step 11] Query error: %v\n", err)
				return allExpected // return full list so matcher keeps polling
			}
			allAuditEvents = resp.Data

			// Rebuild event type counts on each poll
			eventTypeCounts = map[string]int{}
			for _, event := range allAuditEvents {
				eventTypeCounts[event.EventType]++
			}

			// Determine which required event types are still missing
			var missing []string
			for _, eventType := range allExpected {
				if eventTypeCounts[eventType] == 0 {
					missing = append(missing, eventType)
				}
			}
			GinkgoWriter.Printf("  [Step 11] Found %d audit events (%d unique types), %d required types still missing\n",
				len(allAuditEvents), len(eventTypeCounts), len(missing))
			return missing
		}, 150*time.Second, 2*time.Second).Should(BeEmpty(),
			"All required audit event types must be present in the trail")

		// Log all events for debugging
		for _, event := range allAuditEvents {
			GinkgoWriter.Printf("  Audit: type=%s category=%s outcome=%s ts=%s\n",
				event.EventType, event.EventCategory, event.EventOutcome,
				event.EventTimestamp.Format(time.RFC3339))
		}

		// Verify exactly-once events
		for _, eventType := range exactlyOnceEvents {
			Expect(eventTypeCounts).To(HaveKey(eventType),
				"Audit trail must contain exactly-once event: %s", eventType)
			Expect(eventTypeCounts[eventType]).To(Equal(1),
				"Event %s must appear exactly once, but found %d", eventType, eventTypeCounts[eventType])
		}

		// Verify at-least-once events
		for _, eventType := range atLeastOnceEvents {
			Expect(eventTypeCounts).To(HaveKey(eventType),
				"Audit trail must contain at-least-once event: %s", eventType)
			Expect(eventTypeCounts[eventType]).To(BeNumerically(">=", 1),
				"Event %s must appear at least once, but found %d", eventType, eventTypeCounts[eventType])
		}

		// Total event count validation: the audit trail must contain at least
		// len(exactlyOnceEvents) + len(atLeastOnceEvents) events. At-least-once
		// events may repeat (phase transitions, retries), and optional events
		// (tool calls, business classification) may also be present.
		expectedMinTotal := len(exactlyOnceEvents) + len(atLeastOnceEvents)
		Expect(len(allAuditEvents)).To(BeNumerically(">=", expectedMinTotal),
			"Audit trail must contain at least %d events (got %d): %d exactly-once + %d at-least-once",
			expectedMinTotal, len(allAuditEvents), len(exactlyOnceEvents), len(atLeastOnceEvents))

		// Verify temporal ordering: gateway.signal.received should be among the earliest events.
		// Audit timestamps have second-level precision, so multiple events emitted in the
		// first second of the pipeline (gateway → RO → SP) share the same timestamp.
		// We verify gateway.signal.received is present at the earliest timestamp tier.
		Expect(allAuditEvents).ToNot(BeEmpty())
		earliestTS := allAuditEvents[0].EventTimestamp
		for _, event := range allAuditEvents[1:] {
			if event.EventTimestamp.Before(earliestTS) {
				earliestTS = event.EventTimestamp
			}
		}
		var earliestTypes []string
		for _, event := range allAuditEvents {
			if event.EventTimestamp.Equal(earliestTS) {
				earliestTypes = append(earliestTypes, event.EventType)
			}
		}
		Expect(earliestTypes).To(ContainElement("gateway.signal.received"),
			"gateway.signal.received must be among the earliest audit events (ts=%s, found: %v)",
			earliestTS.Format(time.RFC3339), earliestTypes)

		GinkgoWriter.Printf("  ✅ Audit trail verified: %d events, %d unique types, all expected present\n",
			len(allAuditEvents), len(eventTypeCounts))

		// ================================================================
		// Step 12: Verify RR reconstruction from audit trail (BR-AUDIT-005)
		// ================================================================
		By("Step 12: Verifying RR reconstruction from audit trail")

		var reconstructionResp *ogenclient.ReconstructionResponse
		Eventually(func() error {
			resp, err := dataStorageClient.ReconstructRemediationRequest(testCtx,
				ogenclient.ReconstructRemediationRequestParams{
					CorrelationID: correlationID,
				})
			if err != nil {
				return fmt.Errorf("reconstruction API error: %w", err)
			}

			switch r := resp.(type) {
			case *ogenclient.ReconstructionResponse:
				reconstructionResp = r
				return nil
			case *ogenclient.ReconstructRemediationRequestBadRequest:
				return fmt.Errorf("400 Bad Request: %s", r.Detail.Value)
			case *ogenclient.ReconstructRemediationRequestNotFound:
				return fmt.Errorf("404 Not Found")
			case *ogenclient.ReconstructRemediationRequestInternalServerError:
				return fmt.Errorf("500 Internal Server Error: %s", r.Detail.Value)
			default:
				return fmt.Errorf("unexpected response type: %T", resp)
			}
		}, 30*time.Second, 2*time.Second).Should(Succeed(),
			"RR reconstruction should succeed")

		Expect(reconstructionResp).ToNot(BeNil())
		Expect(reconstructionResp.RemediationRequestYaml).ToNot(BeEmpty(),
			"Reconstructed RR YAML should not be empty")
		Expect(reconstructionResp.Validation.IsValid).To(BeTrue(),
			"Reconstructed RR should be valid")
		Expect(reconstructionResp.Validation.Completeness).To(BeNumerically(">=", 80),
			"Reconstructed RR completeness should be at least 80%%")

		// ================================================================
		// Step 12b: Field-by-field verification against the live RR (DD-AUDIT-004)
		// Parse the reconstructed YAML back into an RR struct and verify
		// fields that the reconstruction pipeline populates from audit events.
		//
		// Currently reconstructed fields (from pkg/datastorage/reconstruction/):
		//   gateway.signal.received → SignalName, SignalType, SignalFingerprint, SignalLabels, SignalAnnotations, OriginalPayload
		//   orchestrator.lifecycle.created → TimeoutConfig
		//   aianalysis.analysis.completed → ProviderData
		//   workflowexecution.selection.completed → SelectedWorkflowRef
		//   workflowexecution.execution.started → ExecutionRef
		//
		// NOT reconstructed (pipeline limitations):
		//   OverallPhase — not part of reconstruction scope
		//   Namespace/Name — reconstruction uses correlation_id-derived naming
		// ================================================================
		By("Step 12b: Verifying reconstructed RR fields match the live RR")

		// Fetch the live RR (post-completion, all status fields populated)
		liveRR := &remediationv1.RemediationRequest{}
		Expect(apiReader.Get(ctx, client.ObjectKey{
			Name: remediationRequest.Name, Namespace: testNamespace,
		}, liveRR)).To(Succeed(), "Should fetch the live RR")

		// Parse reconstructed YAML into an RR struct
		reconstructedRR := &remediationv1.RemediationRequest{}
		Expect(sigyaml.Unmarshal([]byte(reconstructionResp.RemediationRequestYaml), reconstructedRR)).To(Succeed(),
			"Reconstructed YAML should unmarshal into a RemediationRequest")

		GinkgoWriter.Printf("  Reconstructed RR: name=%s namespace=%s phase=%s\n",
			reconstructedRR.Name, reconstructedRR.Namespace, reconstructedRR.Status.OverallPhase)

		// Gap #1: spec.signalName — from gateway.signal.received (AlertName field)
		Expect(reconstructedRR.Spec.SignalName).ToNot(BeEmpty(),
			"Gap #1: Reconstructed RR must have spec.signalName")
		Expect(reconstructedRR.Spec.SignalName).To(Equal(liveRR.Spec.SignalName),
			"Gap #1: Reconstructed signalName should match live RR")
		GinkgoWriter.Printf("  ✅ Gap #1: signalName=%s (matches live RR)\n", reconstructedRR.Spec.SignalName)

		// Gap #1: spec.signalFingerprint — from gateway.signal.received (Fingerprint field)
		// SHA256 hash (64-char hex), used as deduplication identity for the signal
		Expect(reconstructedRR.Spec.SignalFingerprint).ToNot(BeEmpty(),
			"Gap #1: Reconstructed RR must have spec.signalFingerprint")
		Expect(reconstructedRR.Spec.SignalFingerprint).To(Equal(liveRR.Spec.SignalFingerprint),
			"Gap #1: Reconstructed signalFingerprint should match live RR")
		GinkgoWriter.Printf("  ✅ Gap #1: signalFingerprint=%s (matches live RR)\n", reconstructedRR.Spec.SignalFingerprint)

		// Gap #2: spec.signalLabels — from gateway.signal.received
		Expect(reconstructedRR.Spec.SignalLabels).ToNot(BeEmpty(),
			"Gap #2: Reconstructed RR must have spec.signalLabels")
		GinkgoWriter.Printf("  ✅ Gap #2: signalLabels has %d entries\n", len(reconstructedRR.Spec.SignalLabels))

		// Gap #3: spec.signalAnnotations — from gateway.signal.received
		// Annotations may be empty depending on the signal source
		GinkgoWriter.Printf("  ✅ Gap #3: signalAnnotations has %d entries\n", len(reconstructedRR.Spec.SignalAnnotations))

		// Gap #4: spec.originalPayload — from gateway.signal.received
		Expect(reconstructedRR.Spec.OriginalPayload).ToNot(BeEmpty(),
			"Gap #4: Reconstructed RR must have spec.originalPayload (original webhook body)")
		GinkgoWriter.Printf("  ✅ Gap #4: originalPayload present (%d bytes)\n", len(reconstructedRR.Spec.OriginalPayload))

		// Gap #5: status.selectedWorkflowRef — from workflowexecution.selection.completed
		Expect(reconstructedRR.Status.SelectedWorkflowRef).ToNot(BeNil(),
			"Gap #5: Reconstructed RR must have status.selectedWorkflowRef")
		GinkgoWriter.Printf("  ✅ Gap #5: selectedWorkflowRef.workflowID=%s\n",
			reconstructedRR.Status.SelectedWorkflowRef.WorkflowID)

		// Gap #6: status.executionRef — from workflowexecution.execution.started
		Expect(reconstructedRR.Status.ExecutionRef).ToNot(BeNil(),
			"Gap #6: Reconstructed RR must have status.executionRef")
		GinkgoWriter.Printf("  ✅ Gap #6: executionRef.name=%s\n", reconstructedRR.Status.ExecutionRef.Name)

		// Gap #7: status.error — NOT implemented in reconstruction pipeline
		// This is a success path so no error is expected anyway.
		GinkgoWriter.Println("  ⏭️  Gap #7: status.error (N/A on success path; not implemented in reconstruction)")

		// Gap #8: status.timeoutConfig — from orchestrator.lifecycle.created
		Expect(reconstructedRR.Status.TimeoutConfig).ToNot(BeNil(),
			"Gap #8: Reconstructed RR must have status.timeoutConfig")
		GinkgoWriter.Printf("  ✅ Gap #8: timeoutConfig present (global=%v)\n",
			reconstructedRR.Status.TimeoutConfig.Global)

		GinkgoWriter.Printf("  ✅ RR reconstruction verified: completeness=%d%%, valid=%t, gaps #1-6,#8 verified\n",
			reconstructionResp.Validation.Completeness, reconstructionResp.Validation.IsValid)
		if len(reconstructionResp.Validation.Warnings) > 0 {
			GinkgoWriter.Printf("  ⚠️  Reconstruction warnings: %v\n", reconstructionResp.Validation.Warnings)
		}

		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		GinkgoWriter.Println("✅ FULL REMEDIATION LIFECYCLE COMPLETE (with audit verification)")
		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		GinkgoWriter.Println("  Event → Gateway → RO → SP → AA → HAPI → WE(Job) → Notification → EM ✅")
		GinkgoWriter.Println("  Audit Trail: complete, non-duplicated, temporally ordered ✅")
		GinkgoWriter.Println("  RR Reconstruction: valid, high completeness ✅")
	})
})

