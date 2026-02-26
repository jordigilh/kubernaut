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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	crdvalidators "github.com/jordigilh/kubernaut/test/shared/validators"
)

// BR-ORCH-026, BR-ORCH-029: Approval Lifecycle E2E Test
// Validates the complete pipeline with human approval gate:
//
//	OOMKill Event → Gateway → RO → SP → AA(approval required) → RAR → Approval NR
//	                                                                 → Operator Approves → WE(Job) → Completion NR → EM
//
// Approval is triggered by the "production" environment classification via Rego policy.
var _ = Describe("Approval Lifecycle [BR-ORCH-026]", func() {

	var (
		testNamespace string
		testCtx       context.Context
		testCancel    context.CancelFunc
	)

	BeforeEach(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 10*time.Minute)

		By("Step 0: Seeding test workflows in DataStorage")
		workflows := []infrastructure.TestWorkflow{
			{
				WorkflowID:      "crashloop-config-fix-v1",
				Name:            "CrashLoopBackOff - Configuration Fix",
				Description:     "CrashLoop remediation workflow for approval E2E",
				SignalName:      "CrashLoopBackOff",
				Severity:        "high",
				Component:       "deployment",
				Environment:     "production",
				Priority:        "*",
				SchemaImage:     "quay.io/kubernaut-cicd/test-workflows/crashloop-config-fix-job:v1.0.0",
				ExecutionEngine: "job",
				SchemaParameters: []models.WorkflowParameter{
					{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target namespace"},
					{Name: "DEPLOYMENT_NAME", Type: "string", Required: true, Description: "Name of the deployment to restart"},
					{Name: "GRACE_PERIOD_SECONDS", Type: "integer", Required: false, Description: "Graceful shutdown period"},
				},
			},
			{
				WorkflowID:      "oomkill-increase-memory-v1",
				Name:            "OOMKill Recovery - Increase Memory Limits",
				Description:     "OOMKill remediation workflow for approval E2E",
				SignalName:      "OOMKilled",
				Severity:        "critical",
				Component:       "deployment",
				Environment:     "production",
				Priority:        "*",
				SchemaImage:     "quay.io/kubernaut-cicd/test-workflows/oomkill-increase-memory-job:v1.0.0",
				ExecutionEngine: "job",
				SchemaParameters: []models.WorkflowParameter{
					{Name: "TARGET_RESOURCE_KIND", Type: "string", Required: true, Description: "Kubernetes resource kind"},
					{Name: "TARGET_RESOURCE_NAME", Type: "string", Required: true, Description: "Name of the resource to patch"},
					{Name: "TARGET_NAMESPACE", Type: "string", Required: true, Description: "Namespace of the resource"},
					{Name: "MEMORY_LIMIT_NEW", Type: "string", Required: true, Description: "New memory limit"},
				},
			},
		}
		workflowUUIDs, err := infrastructure.SeedWorkflowsInDataStorage(
			dataStorageClient, workflows, "approval-e2e", GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to seed workflows in DataStorage")
		Expect(workflowUUIDs).To(HaveKey("crashloop-config-fix-v1:production"))

		if os.Getenv("SKIP_MOCK_LLM") == "" {
			By("Step 0b: Updating Mock LLM with seeded workflow UUIDs")
			err = infrastructure.UpdateMockLLMConfigMap(
				testCtx, "kubernaut-system", kubeconfigPath, workflowUUIDs, GinkgoWriter,
			)
			Expect(err).ToNot(HaveOccurred(), "Failed to update Mock LLM ConfigMap")
		}
	})

	AfterEach(func() {
		if testNamespace != "" {
			By("Cleaning up approval test namespace")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
			_ = k8sClient.Delete(ctx, ns)
		}
		testCancel()
	})

	It("should require human approval for production environment and resume pipeline after approval [E2E-FP-118-003]", func() {
		// ================================================================
		// Step 1: Create a production namespace (triggers approval via Rego)
		// ================================================================
		By("Step 1: Creating managed namespace with production environment label")
		testNamespace = fmt.Sprintf("fp-approval-%d", time.Now().Unix())
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
				Labels: map[string]string{
					"kubernaut.ai/managed":     "true",
					"kubernaut.ai/environment": "production",
				},
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		GinkgoWriter.Printf("  ✅ Namespace created: %s (production environment)\n", testNamespace)

		// ================================================================
		// Step 2: Deploy memory-eater to trigger OOMKill
		// ================================================================
		By("Step 2: Deploying memory-eater pod (will trigger OOMKill)")
		err := infrastructure.DeployMemoryEater(testCtx, testNamespace, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		By("Step 2b: Waiting for OOMKill/CrashLoopBackOff event")
		Eventually(func() bool {
			pods := &corev1.PodList{}
			if err := apiReader.List(ctx, pods, client.InNamespace(testNamespace),
				client.MatchingLabels{"app": "memory-eater"}); err != nil {
				return false
			}
			for _, pod := range pods.Items {
				for _, cs := range pod.Status.ContainerStatuses {
					if cs.LastTerminationState.Terminated != nil &&
						cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
						GinkgoWriter.Printf("  ✅ OOMKill detected: restarts=%d\n", cs.RestartCount)
						return true
					}
					if cs.State.Terminated != nil &&
						cs.State.Terminated.Reason == "OOMKilled" {
						return true
					}
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
		// Step 3: Wait for RemediationRequest creation
		// ================================================================
		By("Step 3: Waiting for RemediationRequest to be created by Gateway")
		var remediationRequest *remediationv1.RemediationRequest
		Eventually(func() bool {
			rrList := &remediationv1.RemediationRequestList{}
			if err := apiReader.List(ctx, rrList, client.InNamespace(namespace)); err != nil {
				return false
			}
			for i := range rrList.Items {
				remediationRequest = &rrList.Items[i]
				GinkgoWriter.Printf("  ✅ RemediationRequest found: %s\n", remediationRequest.Name)
				return true
			}
			return false
		}, timeout, interval).Should(BeTrue(), "RemediationRequest should be created")

		// ================================================================
		// Step 4: Wait for SignalProcessing to complete
		// ================================================================
		By("Step 4: Waiting for SignalProcessing to complete")
		Eventually(func() string {
			spList := &signalprocessingv1.SignalProcessingList{}
			if err := apiReader.List(ctx, spList, client.InNamespace(namespace)); err != nil {
				return ""
			}
			for _, sp := range spList.Items {
				if sp.Spec.RemediationRequestRef.Name == remediationRequest.Name {
					GinkgoWriter.Printf("  SP %s phase: %s\n", sp.Name, sp.Status.Phase)
					return string(sp.Status.Phase)
				}
			}
			return ""
		}, timeout, interval).Should(Equal("Completed"))

		// Scenario-specific: verify production environment classification
		By("Step 4b: Verifying SP classified environment as production")
		spList := &signalprocessingv1.SignalProcessingList{}
		Expect(apiReader.List(ctx, spList, client.InNamespace(namespace))).To(Succeed())
		for i := range spList.Items {
			sp := &spList.Items[i]
			if sp.Spec.RemediationRequestRef.Name == remediationRequest.Name {
				Expect(sp.Status.EnvironmentClassification.Environment).To(Equal("production"),
					"SP should classify namespace as production (kubernaut.ai/environment label)")
				Expect(sp.Status.EnvironmentClassification.Source).To(Equal("namespace-labels"),
					"SP should classify from namespace labels when kubernaut.ai/environment is set")
				GinkgoWriter.Printf("  ✅ SP environment: %s (source: %s)\n",
					sp.Status.EnvironmentClassification.Environment, sp.Status.EnvironmentClassification.Source)
				break
			}
		}

		// ================================================================
		// Step 5: Wait for AIAnalysis to complete WITH approval required
		// ================================================================
		By("Step 5: Waiting for AIAnalysis to complete with approval required")
		var aaObj *aianalysisv1.AIAnalysis
		Eventually(func() bool {
			aaList := &aianalysisv1.AIAnalysisList{}
			if err := apiReader.List(ctx, aaList, client.InNamespace(namespace)); err != nil {
				return false
			}
			for i := range aaList.Items {
				aa := &aaList.Items[i]
				if aa.Spec.RemediationRequestRef.Name == remediationRequest.Name &&
					aa.Status.Phase == aianalysisv1.PhaseCompleted {
					aaObj = aa
					GinkgoWriter.Printf("  AA %s completed: approvalRequired=%v\n", aa.Name, aa.Status.ApprovalRequired)
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue(), "AIAnalysis should reach Completed phase")

		Expect(aaObj.Status.ApprovalRequired).To(BeTrue(),
			"AIAnalysis should require approval for production environment (Rego policy)")
		Expect(aaObj.Status.ApprovalReason).ToNot(BeEmpty(),
			"AIAnalysis approval reason should be populated")
		GinkgoWriter.Printf("  ✅ Approval required: reason=%s\n", aaObj.Status.ApprovalReason)

		// ================================================================
		// Step 6: Wait for RemediationApprovalRequest to be created
		// ================================================================
		By("Step 6: Waiting for RemediationApprovalRequest creation (BR-ORCH-026)")
		var rarObj *remediationv1.RemediationApprovalRequest
		rarName := fmt.Sprintf("rar-%s", remediationRequest.Name)
		Eventually(func() bool {
			rar := &remediationv1.RemediationApprovalRequest{}
			if err := apiReader.Get(ctx, client.ObjectKey{
				Name: rarName, Namespace: namespace,
			}, rar); err != nil {
				return false
			}
			rarObj = rar
			GinkgoWriter.Printf("  ✅ RAR found: %s (confidence=%.2f, level=%s)\n",
				rar.Name, rar.Spec.Confidence, rar.Spec.ConfidenceLevel)
			return true
		}, timeout, interval).Should(BeTrue(),
			"RemediationApprovalRequest should be created by RO")

		// ================================================================
		// Step 7: Wait for approval NotificationRequest (Type=approval)
		// ================================================================
		By("Step 7: Waiting for approval NotificationRequest (BR-ORCH-029)")
		Eventually(func() bool {
			nrList := &notificationv1.NotificationRequestList{}
			if err := apiReader.List(ctx, nrList, client.InNamespace(namespace)); err != nil {
				return false
			}
			for _, nr := range nrList.Items {
				if nr.Spec.RemediationRequestRef != nil &&
					nr.Spec.RemediationRequestRef.Name == remediationRequest.Name &&
					nr.Spec.Type == notificationv1.NotificationTypeApproval {
					GinkgoWriter.Printf("  ✅ Approval NotificationRequest found: %s\n", nr.Name)
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue(),
			"Approval NotificationRequest should be created (Type=approval)")

		// Verify RR is in AwaitingApproval phase before we approve
		By("Step 7b: Verifying RR is in AwaitingApproval phase")
		Eventually(func() string {
			rr := &remediationv1.RemediationRequest{}
			if err := apiReader.Get(ctx, client.ObjectKey{
				Name: remediationRequest.Name, Namespace: namespace,
			}, rr); err != nil {
				return ""
			}
			return string(rr.Status.OverallPhase)
		}, 30*time.Second, interval).Should(Equal("AwaitingApproval"),
			"RR should be in AwaitingApproval phase before approval")

		// ================================================================
		// Step 8: Approve the RAR (simulates operator approval)
		// ================================================================
		By("Step 8: Approving RemediationApprovalRequest (simulating operator)")
		Expect(apiReader.Get(ctx, client.ObjectKey{
			Name: rarName, Namespace: namespace,
		}, rarObj)).To(Succeed())

		rarObj.Status.Decision = remediationv1.ApprovalDecisionApproved
		rarObj.Status.DecidedBy = "e2e-test-admin@kubernaut.ai"
		rarObj.Status.DecisionMessage = "Approved by E2E test"
		decidedAt := metav1.Now()
		rarObj.Status.DecidedAt = &decidedAt
		Expect(k8sClient.Status().Update(ctx, rarObj)).To(Succeed())
		GinkgoWriter.Println("  ✅ RAR approved by e2e-test-admin@kubernaut.ai")

		// ================================================================
		// Step 9: Wait for WorkflowExecution creation (pipeline resumes)
		// ================================================================
		By("Step 9: Waiting for WorkflowExecution after approval")
		var weName string
		Eventually(func() string {
			weList := &workflowexecutionv1.WorkflowExecutionList{}
			if err := apiReader.List(ctx, weList, client.InNamespace(namespace)); err != nil {
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
			"WorkflowExecution should be created with job engine after approval")

		// ================================================================
		// Step 10: Wait for K8s Job completion
		// ================================================================
		By("Step 10: Waiting for K8s Job to complete")
		Eventually(func() bool {
			jobList := &batchv1.JobList{}
			if err := apiReader.List(ctx, jobList,
				client.InNamespace("kubernaut-workflows")); err != nil {
				return false
			}
			for _, job := range jobList.Items {
				if job.CreationTimestamp.After(remediationRequest.CreationTimestamp.Time.Add(-10*time.Second)) &&
					job.Status.Succeeded > 0 {
					GinkgoWriter.Printf("  ✅ Job completed: %s\n", job.Name)
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue(), "K8s Job should complete")

		// ================================================================
		// Step 11: Wait for WorkflowExecution completion
		// ================================================================
		By("Step 11: Waiting for WorkflowExecution to complete")
		Eventually(func() string {
			we := &workflowexecutionv1.WorkflowExecution{}
			if err := apiReader.Get(ctx, client.ObjectKey{
				Name: weName, Namespace: namespace,
			}, we); err != nil {
				return ""
			}
			return we.Status.Phase
		}, timeout, interval).Should(Equal("Completed"))

		// ================================================================
		// Step 12: Wait for completion NotificationRequest
		// ================================================================
		By("Step 12: Waiting for completion NotificationRequest")
		Eventually(func() bool {
			nrList := &notificationv1.NotificationRequestList{}
			if err := apiReader.List(ctx, nrList, client.InNamespace(namespace)); err != nil {
				return false
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
		}, timeout, interval).Should(BeTrue(), "Completion NotificationRequest should be created")

		// ================================================================
		// Step 13: Wait for RR to reach Completed
		// ================================================================
		By("Step 13: Verifying RemediationRequest completed")
		Eventually(func() string {
			rr := &remediationv1.RemediationRequest{}
			if err := apiReader.Get(ctx, client.ObjectKey{
				Name: remediationRequest.Name, Namespace: namespace,
			}, rr); err != nil {
				return ""
			}
			return string(rr.Status.OverallPhase)
		}, timeout, interval).Should(Equal("Completed"))

		// ================================================================
		// Step 14: Wait for EffectivenessAssessment
		// ================================================================
		By("Step 14: Waiting for EffectivenessAssessment to reach terminal phase")
		eaKey := client.ObjectKey{Name: fmt.Sprintf("ea-%s", remediationRequest.Name), Namespace: namespace}
		finalEA := &eav1.EffectivenessAssessment{}
		Eventually(func() string {
			if err := apiReader.Get(testCtx, eaKey, finalEA); err != nil {
				return ""
			}
			return finalEA.Status.Phase
		}, 3*time.Minute, 5*time.Second).Should(
			BeElementOf(eav1.PhaseCompleted, eav1.PhaseFailed),
			"EA should reach terminal phase")
		GinkgoWriter.Printf("  EA %s phase: %s\n", finalEA.Name, finalEA.Status.Phase)

		// ================================================================
		// Step 15: CRD Status Validation with Approval Flow [E2E-FP-118-003..006]
		// Validates all 7 CRDs with approval-specific fields.
		// ================================================================
		By("Step 15: Validating CRD status fields with approval flow [E2E-FP-118-003..006]")

		var allFailures []string

		// SP
		spList = &signalprocessingv1.SignalProcessingList{}
		Expect(apiReader.List(ctx, spList, client.InNamespace(namespace))).To(Succeed())
		for i := range spList.Items {
			sp := &spList.Items[i]
			if sp.Spec.RemediationRequestRef.Name == remediationRequest.Name {
				allFailures = append(allFailures, crdvalidators.ValidateSPStatus(sp)...)
				break
			}
		}

		// AA (with approval)
		freshAA := &aianalysisv1.AIAnalysis{}
		Expect(apiReader.Get(ctx, client.ObjectKey{Name: aaObj.Name, Namespace: namespace}, freshAA)).To(Succeed())
		allFailures = append(allFailures, crdvalidators.ValidateAAStatus(freshAA, crdvalidators.WithApprovalFlow())...)

		// WE
		weObj := &workflowexecutionv1.WorkflowExecution{}
		Expect(apiReader.Get(ctx, client.ObjectKey{Name: weName, Namespace: namespace}, weObj)).To(Succeed())
		allFailures = append(allFailures, crdvalidators.ValidateWEStatus(weObj)...)

		// NT: validate both approval and completion notifications
		nrList := &notificationv1.NotificationRequestList{}
		Expect(apiReader.List(ctx, nrList, client.InNamespace(namespace))).To(Succeed())
		for i := range nrList.Items {
			nr := &nrList.Items[i]
			if nr.Spec.RemediationRequestRef != nil &&
				nr.Spec.RemediationRequestRef.Name == remediationRequest.Name {
				allFailures = append(allFailures, crdvalidators.ValidateNTStatus(nr)...)
			}
		}

		// RR (with approval)
		finalRR := &remediationv1.RemediationRequest{}
		Expect(apiReader.Get(ctx, client.ObjectKey{
			Name: remediationRequest.Name, Namespace: namespace,
		}, finalRR)).To(Succeed())
		allFailures = append(allFailures, crdvalidators.ValidateRRStatus(finalRR, crdvalidators.WithApprovalFlow())...)

		// EA (guaranteed non-nil by the Eventually block above)
		Expect(finalEA.Status.Phase).To(Or(Equal(eav1.PhaseCompleted), Equal(eav1.PhaseFailed)),
			"EA should be in terminal phase after pipeline completes")
		allFailures = append(allFailures, crdvalidators.ValidateEAStatus(finalEA)...)

		// RAR status
		freshRAR := &remediationv1.RemediationApprovalRequest{}
		Expect(apiReader.Get(ctx, client.ObjectKey{Name: rarName, Namespace: namespace}, freshRAR)).To(Succeed())
		allFailures = append(allFailures, crdvalidators.ValidateRARStatus(freshRAR)...)

		// RAR spec
		allFailures = append(allFailures, crdvalidators.ValidateRARSpec(freshRAR)...)

		if len(allFailures) > 0 {
			GinkgoWriter.Println("  ⚠️  CRD Status Validation Failures (approval flow):")
			for _, f := range allFailures {
				GinkgoWriter.Printf("    - %s\n", f)
			}
		}
		Expect(allFailures).To(BeEmpty(),
			"All pipeline CRDs (including RAR) should have complete status fields. Failures:\n%s",
			strings.Join(allFailures, "\n"))

		GinkgoWriter.Println("  ✅ CRD status validation passed (all 7 CRDs + approval fields)")

		// ================================================================
		// Step 16: Audit trail validation for approval events [E2E-FP-118-006]
		// Verifies approval-specific audit events are emitted alongside
		// standard pipeline events.
		// ================================================================
		By("Step 16: Verifying audit trail contains approval events [E2E-FP-118-006]")

		correlationID := remediationRequest.Name

		approvalAuditEvents := []string{
			"orchestrator.approval.requested",
			"orchestrator.approval.approved",
		}

		standardPipelineEvents := []string{
			"gateway.signal.received",
			"gateway.crd.created",
			"orchestrator.lifecycle.created",
			"orchestrator.lifecycle.started",
			"orchestrator.lifecycle.transitioned",
			"orchestrator.lifecycle.completed",
			"signalprocessing.signal.processed",
			"aianalysis.analysis.completed",
			"workflowexecution.workflow.completed",
			"notification.message.sent",
		}

		allExpectedEvents := append(approvalAuditEvents, standardPipelineEvents...)

		eventTypeCounts := map[string]int{}
		Eventually(func() []string {
			resp, err := dataStorageClient.QueryAuditEvents(testCtx, ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(correlationID),
				Limit:         ogenclient.NewOptInt(200),
			})
			if err != nil {
				return allExpectedEvents
			}

			eventTypeCounts = map[string]int{}
			for _, event := range resp.Data {
				eventTypeCounts[event.EventType]++
			}

			var missing []string
			for _, et := range allExpectedEvents {
				if eventTypeCounts[et] == 0 {
					missing = append(missing, et)
				}
			}
			GinkgoWriter.Printf("  [Step 16] %d audit events, %d missing\n", len(resp.Data), len(missing))
			return missing
		}, 150*time.Second, 2*time.Second).Should(BeEmpty(),
			"Audit trail must contain approval-specific and standard pipeline events")

		// Verify approval events appeared exactly once
		for _, et := range approvalAuditEvents {
			Expect(eventTypeCounts[et]).To(Equal(1),
				"Approval event %s must appear exactly once, got %d", et, eventTypeCounts[et])
		}
		GinkgoWriter.Println("  ✅ Audit trail verified: approval events present (requested + approved)")

		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		GinkgoWriter.Println("✅ APPROVAL LIFECYCLE COMPLETE")
		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		GinkgoWriter.Println("  Event → Gateway → RO → SP → AA(approval) → RAR → Approval NR ✅")
		GinkgoWriter.Println("  Operator Approval → WE(Job) → Completion NR → EM ✅")
		GinkgoWriter.Println("  CRD Status Fields: all populated (with approval) [E2E-FP-118-003..006] ✅")
	})
})
