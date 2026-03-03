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

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// E2E-FP-251-001: Async Hash Deferral for CRD Targets
//
// Validates DD-EM-004 / BR-EM-010: When the remediation target is an operator-managed
// CRD (cert-manager Certificate), the RO sets HashComputeAfter on the EA spec and the
// EM defers hash computation until that timestamp.
//
// Pipeline:
//
//	CertManagerCertNotReady alert → AlertManager → Gateway → RR → RO → SP → AA → HAPI(MockLLM) → WE(Job) → EA
//
// Key validations:
//  1. RO detects Certificate (cert-manager.io/v1) as non-built-in CRD via REST mapper
//  2. EA.Spec.HashComputeAfter is set (non-nil, future timestamp)
//  3. Audit trail contains hash_compute_after in assessment.scheduled event
//  4. EA reaches terminal phase after deferral window
//
// Self-contained: cert-manager is installed in BeforeAll and only affects this test.
var _ = Describe("Async Hash Deferral for CRD Targets [DD-EM-004, BR-EM-010]", Ordered, func() {

	var (
		testNamespace string
		testCtx       context.Context
		testCancel    context.CancelFunc
	)

	BeforeAll(func() {
		By("Installing cert-manager for real Certificate CRD registration")
		err := infrastructure.InstallCertManager(kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "Failed to install cert-manager")

		By("Waiting for cert-manager to be ready")
		err = infrastructure.WaitForCertManagerReady(kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "cert-manager not ready in time")

		GinkgoWriter.Println("  ✅ cert-manager installed — Certificate CRD registered in REST mapper")
	})

	BeforeEach(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 10*time.Minute)

		By("Step 0: Seeding test workflows in DataStorage")
		workflows := []infrastructure.TestWorkflow{
			{
				WorkflowID:      "oomkill-increase-memory-v1",
				Name:            "OOMKill Recovery - Increase Memory Limits",
				Description:     "Increases container memory limits to prevent OOMKill recurrence",
				ActionType:      "IncreaseMemoryLimits",
				SignalName:      "OOMKilled",
				Severity:        "critical",
				Component:       "deployment",
				Environment:     "production",
				Priority:        "*",
				SchemaImage:     "quay.io/kubernaut-cicd/test-workflows/oomkill-increase-memory-job:v1.0.0",
				ExecutionEngine: "job",
				SchemaParameters: []models.WorkflowParameter{
					{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target namespace"},
					{Name: "DEPLOYMENT_NAME", Type: "string", Required: true, Description: "Deployment to patch"},
					{Name: "MEMORY_INCREASE_PERCENT", Type: "string", Required: false, Description: "Memory increase percentage", Default: "50"},
				},
			},
		}
		workflowUUIDs, err := infrastructure.SeedWorkflowsInDataStorage(
			dataStorageClient, workflows, "async-hash-deferral-e2e", GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to seed workflows")
		Expect(workflowUUIDs).To(HaveKey("oomkill-increase-memory-v1:production"))

		if os.Getenv("SKIP_MOCK_LLM") == "" {
			By("Step 0b: Updating Mock LLM with seeded workflow UUIDs")
			err = infrastructure.UpdateMockLLMConfigMap(
				testCtx, namespace, kubeconfigPath, workflowUUIDs, GinkgoWriter,
			)
			Expect(err).ToNot(HaveOccurred(), "Failed to update Mock LLM ConfigMap")
		}
	})

	AfterEach(func() {
		if testNamespace != "" {
			By("Cleaning up test namespace")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
			_ = k8sClient.Delete(ctx, ns)
		}
		testCancel()
	})

	// E2E-FP-251-001: Full pipeline with CRD target triggers async hash deferral
	It("E2E-FP-251-001: Pipeline with cert-manager CRD target defers hash computation [BR-EM-010]", func() {
		// ================================================================
		// Step 1: Create managed test namespace
		// ================================================================
		By("Step 1: Creating managed test namespace for cert-async test")
		testNamespace = fmt.Sprintf("fp-cert-%d", time.Now().Unix())
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
		// Step 2: Deploy memory-eater for pipeline flow
		// ================================================================
		By("Step 2: Deploying memory-eater (high usage) for pipeline completion")
		err := infrastructure.DeployMemoryEaterHighUsage(testCtx, testNamespace, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "Failed to deploy memory-eater")

		var memoryEaterPodName string
		Eventually(func() bool {
			pods := &corev1.PodList{}
			if err := apiReader.List(ctx, pods, client.InNamespace(testNamespace),
				client.MatchingLabels{"app": "memory-eater"}); err != nil {
				return false
			}
			for _, pod := range pods.Items {
				for _, cs := range pod.Status.ContainerStatuses {
					if cs.Ready && cs.State.Running != nil {
						memoryEaterPodName = pod.Name
						GinkgoWriter.Printf("  ✅ memory-eater running: %s\n", pod.Name)
						return true
					}
				}
			}
			return false
		}, 2*time.Minute, 2*time.Second).Should(BeTrue(), "memory-eater should be running")

		// ================================================================
		// Step 3: Inject CertManagerCertNotReady alert
		// ================================================================
		By("Step 3: Injecting CertManagerCertNotReady alert into AlertManager")
		alertManagerURL := fmt.Sprintf("http://localhost:%d", infrastructure.AlertManagerHostPort)
		injectErr := infrastructure.InjectAlerts(alertManagerURL, []infrastructure.TestAlert{
			{
				Name: "CertManagerCertNotReady",
				Labels: map[string]string{
					"severity":           "critical",
					"namespace":          testNamespace,
					"exported_namespace": testNamespace,
					"name":               "demo-app-cert",
					"pod":                memoryEaterPodName,
					"container":          "memory-eater",
				},
				Annotations: map[string]string{
					"summary":     "cert-manager Certificate not ready",
					"description": fmt.Sprintf("Certificate demo-app-cert in %s is not ready", testNamespace),
				},
				Status:   "firing",
				StartsAt: time.Now(),
			},
		})
		Expect(injectErr).ToNot(HaveOccurred(), "Failed to inject alert")

		// ================================================================
		// Step 4: Wait for RemediationRequest
		// ================================================================
		By("Step 4: Waiting for RemediationRequest from CertManagerCertNotReady alert")
		var remediationRequest *remediationv1.RemediationRequest
		Eventually(func() bool {
			rrList := &remediationv1.RemediationRequestList{}
			if err := apiReader.List(ctx, rrList, client.InNamespace(namespace)); err != nil {
				return false
			}
			for i := range rrList.Items {
				rr := &rrList.Items[i]
				if rr.Spec.TargetResource.Namespace != testNamespace {
					continue
				}
				remediationRequest = rr
				GinkgoWriter.Printf("  ✅ RemediationRequest found: %s\n", rr.Name)
				return true
			}
			return false
		}, 2*time.Minute, 3*time.Second).Should(BeTrue(),
			"RemediationRequest should be created by Gateway from CertManagerCertNotReady alert")

		// ================================================================
		// Step 5: Wait for AIAnalysis to complete
		// ================================================================
		By("Step 5: Waiting for AIAnalysis to complete")
		var aaName string
		Eventually(func() string {
			aaList := &aianalysisv1.AIAnalysisList{}
			if err := apiReader.List(ctx, aaList, client.InNamespace(namespace)); err != nil {
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
		}, timeout, interval).Should(Equal("Completed"), "AIAnalysis should reach Completed")

		// Verify AIAnalysis has Certificate as affected resource (DD-EM-004 prerequisite)
		By("Step 5b: Verifying AIAnalysis has Certificate as affected resource")
		aa := &aianalysisv1.AIAnalysis{}
		Expect(apiReader.Get(ctx, client.ObjectKey{Name: aaName, Namespace: namespace}, aa)).To(Succeed())
		Expect(aa.Status.RootCauseAnalysis).ToNot(BeNil(), "RCA should be populated")
		Expect(aa.Status.RootCauseAnalysis.AffectedResource).ToNot(BeNil(),
			"AffectedResource should be populated by Mock LLM cert_not_ready scenario")
		Expect(aa.Status.RootCauseAnalysis.AffectedResource.Kind).To(Equal("Certificate"),
			"RCA affected resource kind must be Certificate (CRD trigger for async detection)")
		GinkgoWriter.Printf("  ✅ AIAnalysis AffectedResource: %s/%s/%s\n",
			aa.Status.RootCauseAnalysis.AffectedResource.Namespace,
			aa.Status.RootCauseAnalysis.AffectedResource.Kind,
			aa.Status.RootCauseAnalysis.AffectedResource.Name)

		// ================================================================
		// Step 6: Verify WorkflowExecution and Job
		// ================================================================
		By("Step 6: Waiting for WorkflowExecution to use job engine")
		var weName string
		Eventually(func() string {
			weList := &workflowexecutionv1.WorkflowExecutionList{}
			if err := apiReader.List(ctx, weList, client.InNamespace(namespace)); err != nil {
				return ""
			}
			for _, we := range weList.Items {
				if we.Spec.RemediationRequestRef.Name == remediationRequest.Name {
					weName = we.Name
					return we.Spec.ExecutionEngine
				}
			}
			return ""
		}, timeout, interval).Should(Equal("job"), "WorkflowExecution should use job engine")

		By("Step 6b: Waiting for K8s Job to complete")
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

		By("Step 6c: Waiting for WorkflowExecution to complete")
		Eventually(func() string {
			we := &workflowexecutionv1.WorkflowExecution{}
			if err := apiReader.Get(ctx, client.ObjectKey{Name: weName, Namespace: namespace}, we); err != nil {
				return ""
			}
			return we.Status.Phase
		}, timeout, interval).Should(Equal("Completed"), "WorkflowExecution should complete")

		// ================================================================
		// Step 7: Verify NotificationRequest
		// ================================================================
		By("Step 7: Waiting for completion NotificationRequest")
		Eventually(func() bool {
			nrList := &notificationv1.NotificationRequestList{}
			if listErr := apiReader.List(ctx, nrList, client.InNamespace(namespace)); listErr != nil {
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
		// Step 8: Verify RemediationRequest completed
		// ================================================================
		By("Step 8: Verifying RemediationRequest completed")
		Eventually(func() string {
			rr := &remediationv1.RemediationRequest{}
			if err := apiReader.Get(ctx, client.ObjectKey{
				Name: remediationRequest.Name, Namespace: namespace,
			}, rr); err != nil {
				return ""
			}
			return string(rr.Status.OverallPhase)
		}, timeout, interval).Should(Equal("Completed"), "RR should reach Completed")

		// ================================================================
		// Step 9: CORE VALIDATION — EA has HashComputeAfter [DD-EM-004]
		// ================================================================
		By("Step 9: Verifying EA created with HashComputeAfter set (async CRD detection)")
		eaName := fmt.Sprintf("ea-%s", remediationRequest.Name)
		eaKey := client.ObjectKey{Name: eaName, Namespace: namespace}

		ea := &eav1.EffectivenessAssessment{}
		Eventually(func() error {
			return apiReader.Get(testCtx, eaKey, ea)
		}, 30*time.Second, 2*time.Second).Should(Succeed(),
			"EA with name ea-<RR.Name> should be created by RO")

		Expect(ea.Spec.CorrelationID).To(Equal(remediationRequest.Name),
			"EA correlationID should match RR name")

		// CORE ASSERTION: HashComputeAfter must be set for CRD target
		Expect(ea.Spec.HashComputeAfter).ToNot(BeNil(),
			"EA.Spec.HashComputeAfter MUST be set when remediation target is a CRD (Certificate)")
		Expect(ea.Spec.HashComputeAfter.Time).To(BeTemporally(">", ea.CreationTimestamp.Time),
			"HashComputeAfter should be in the future relative to EA creation")

		GinkgoWriter.Println("  ┌─────────────────────────────────────────────────────────")
		GinkgoWriter.Println("  │ ASYNC HASH DEFERRAL VALIDATION")
		GinkgoWriter.Println("  ├─────────────────────────────────────────────────────────")
		GinkgoWriter.Printf("  │ EA Created:         %s\n", ea.CreationTimestamp.Format("15:04:05"))
		GinkgoWriter.Printf("  │ HashComputeAfter:   %s\n", ea.Spec.HashComputeAfter.Format("15:04:05"))
		GinkgoWriter.Printf("  │ Deferral Duration:  %s\n",
			ea.Spec.HashComputeAfter.Time.Sub(ea.CreationTimestamp.Time).Round(time.Second))
		GinkgoWriter.Printf("  │ Remediation Target: %s/%s\n",
			ea.Spec.RemediationTarget.Kind, ea.Spec.RemediationTarget.Name)
		GinkgoWriter.Println("  └─────────────────────────────────────────────────────────")

		// Verify remediation target is Certificate (from AIAnalysis.AffectedResource)
		Expect(ea.Spec.RemediationTarget.Kind).To(Equal("Certificate"),
			"EA remediation target should be Certificate (from AIAnalysis RCA)")

		// ================================================================
		// Step 10: Verify EA reaches terminal phase (after deferral)
		// ================================================================
		By("Step 10: Waiting for EA to reach terminal phase (after hash deferral window)")
		Eventually(func() string {
			fetched := &eav1.EffectivenessAssessment{}
			if err := apiReader.Get(testCtx, eaKey, fetched); err != nil {
				return ""
			}
			return fetched.Status.Phase
		}, 3*time.Minute, 5*time.Second).Should(
			BeElementOf(eav1.PhaseCompleted, eav1.PhaseFailed),
			"EA should reach terminal phase after hash deferral window")

		finalEA := &eav1.EffectivenessAssessment{}
		Expect(apiReader.Get(testCtx, eaKey, finalEA)).To(Succeed())

		GinkgoWriter.Printf("  ✅ EA terminal phase: %s (reason: %s)\n",
			finalEA.Status.Phase, finalEA.Status.AssessmentReason)

		// ================================================================
		// Step 11: Verify audit trail includes hash_compute_after
		// ================================================================
		By("Step 11: Verifying audit trail contains hash_compute_after in assessment.scheduled")
		correlationID := remediationRequest.Name

		var scheduledEvent *ogenclient.AuditEvent
		Eventually(func() bool {
			resp, err := dataStorageClient.QueryAuditEvents(testCtx, ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(correlationID),
				Limit:         ogenclient.NewOptInt(200),
			})
			if err != nil {
				return false
			}
			for i := range resp.Data {
				if resp.Data[i].EventType == "effectiveness.assessment.scheduled" {
					scheduledEvent = &resp.Data[i]
					return true
				}
			}
			return false
		}, 60*time.Second, 2*time.Second).Should(BeTrue(),
			"effectiveness.assessment.scheduled audit event should exist")

		// Verify the audit payload includes hash_compute_after via typed accessor
		eaPayload, ok := scheduledEvent.EventData.GetEffectivenessAssessmentAuditPayload()
		Expect(ok).To(BeTrue(),
			"assessment.scheduled event should have EffectivenessAssessmentAuditPayload")
		Expect(eaPayload.HashComputeAfter.Set).To(BeTrue(),
			"assessment.scheduled audit payload must have hash_compute_after set for CRD target")
		GinkgoWriter.Printf("  ✅ Audit trail: hash_compute_after = %s\n",
			eaPayload.HashComputeAfter.Value.Format(time.RFC3339))

		// Verify core audit events are present
		var allAuditEvents []ogenclient.AuditEvent
		resp, auditErr := dataStorageClient.QueryAuditEvents(testCtx, ogenclient.QueryAuditEventsParams{
			CorrelationID: ogenclient.NewOptString(correlationID),
			Limit:         ogenclient.NewOptInt(200),
		})
		Expect(auditErr).ToNot(HaveOccurred())
		allAuditEvents = resp.Data

		eventTypeCounts := map[string]int{}
		for _, event := range allAuditEvents {
			eventTypeCounts[event.EventType]++
		}

		coreEvents := []string{
			"gateway.signal.received",
			"gateway.crd.created",
			"orchestrator.lifecycle.completed",
			"effectiveness.assessment.scheduled",
			"effectiveness.assessment.completed",
		}
		for _, eventType := range coreEvents {
			Expect(eventTypeCounts).To(HaveKey(eventType),
				"Audit trail must contain: %s", eventType)
		}

		GinkgoWriter.Printf("  ✅ Audit trail: %d events, %d unique types\n",
			len(allAuditEvents), len(eventTypeCounts))

		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		GinkgoWriter.Println("✅ ASYNC HASH DEFERRAL E2E TEST COMPLETE [E2E-FP-251-001]")
		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		GinkgoWriter.Println("  CertManagerCertNotReady → full pipeline → EA.HashComputeAfter set ✅")
		GinkgoWriter.Println("  Remediation target: Certificate (cert-manager.io CRD) ✅")
		GinkgoWriter.Println("  EM deferred hash computation, EA reached terminal phase ✅")
		GinkgoWriter.Println("  Audit trail: hash_compute_after in assessment.scheduled ✅")
	})
})
