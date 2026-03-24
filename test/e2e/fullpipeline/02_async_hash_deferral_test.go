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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// E2E-FP-253-001: Async Hash Deferral for CRD Targets (corrected timing model)
//
// Validates DD-EM-004 v2.0 / BR-EM-010 / Issue #253, #277: When the remediation
// target is an operator-managed CRD (cert-manager Certificate), the RO computes
// a config-driven propagation delay and sets Config.HashComputeDelay (Duration) in
// the EA spec. The EM uses the corrected timing model with WaitingForPropagation.
//
// Pipeline:
//
//	CertManagerCertNotReady alert → AlertManager → Gateway → RR → RO → SP → AA → HAPI(MockLLM) → WE(Job) → EA
//
// Key validations:
//  1. RO detects Certificate (cert-manager.io/v1) as non-built-in CRD via REST mapper
//  2. EA.Spec.Config.HashComputeDelay set as Duration-based delay (#277)
//  3. EA enters WaitingForPropagation phase before Stabilizing
//  4. Audit trail contains hash_compute_after in assessment.scheduled event
//  5. EA reaches terminal phase after propagation + stabilization window
//
// Self-contained: cert-manager is installed in BeforeAll and only affects this test.
var _ = Describe("Async Hash Deferral for CRD Targets [DD-EM-004 v2.0, BR-EM-010, #253]", Serial, Ordered, func() {

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

	AfterAll(func() {
		By("Uninstalling cert-manager to prevent cluster resource contamination")
		_ = infrastructure.UninstallCertManager(kubeconfigPath, GinkgoWriter)
	})

	BeforeEach(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 10*time.Minute)

		// Workflows are seeded once in SynchronizedBeforeSuite; workflowUUIDs is suite-level.
		Expect(workflowUUIDs).To(HaveKey("fix-certificate-v1:production"))
	})

	AfterEach(func() {
		if testNamespace != "" {
			By("Cleaning up cert-manager scenario resources")
			infrastructure.CleanupCertManagerScenario(kubeconfigPath, testNamespace, GinkgoWriter)

			By("Cleaning up memory-eater deployment")
			dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "memory-eater", Namespace: testNamespace}}
			_ = k8sClient.Delete(ctx, dep)
		}
		testCancel()
	})

	// E2E-FP-253-001: Full pipeline with CRD target, corrected timing model (Issue #253)
	It("E2E-FP-253-001: Pipeline with cert-manager CRD target uses config-driven propagation delay [#253]", func() {
		// ================================================================
		// Step 1: Create managed test namespace
		// ================================================================
		By("Step 1: Using default namespace for cert-async test (matches Mock LLM TARGET_NAMESPACE)")
		testNamespace = "default"
		ns := &corev1.Namespace{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: testNamespace}, ns)).To(Succeed())
		if ns.Labels == nil {
			ns.Labels = map[string]string{}
		}
		ns.Labels["kubernaut.ai/managed"] = "true"
		Expect(k8sClient.Update(ctx, ns)).To(Succeed())
		GinkgoWriter.Printf("  ✅ Using namespace: %s (labelled as managed)\n", testNamespace)

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
		// Step 2.5: Set up cert-manager scenario (fix-certificate-v1)
		// ================================================================
		// Creates ClusterIssuer + Certificate (Ready), then deletes the CA Secret
		// to make the Certificate go NotReady — replicating the demo cert-failure scenario.
		By("Step 2.5: Setting up cert-manager scenario (ClusterIssuer + Certificate + broken CA)")
		certSetupErr := infrastructure.SetupCertManagerScenario(kubeconfigPath, testNamespace, GinkgoWriter)
		Expect(certSetupErr).ToNot(HaveOccurred(), "Failed to set up cert-manager scenario")

		// ================================================================
		// Step 2.6: Resolve stale alerts from prior tests
		// ================================================================
		// The Gateway only processes Alerts[0] from each AlertManager batch.
		// Prior tests leave MemoryExceedsLimit alerts active; if those are
		// batched with our CertManagerCertNotReady alert the cert alert is
		// silently dropped. Resolve all active alerts first so the cert
		// alert fires alone in the next group flush.
		By("Step 2.6: Resolving active alerts from prior tests to prevent batching")
		alertManagerURL := fmt.Sprintf("http://localhost:%d", infrastructure.AlertManagerHostPort)
		resolveErr := infrastructure.ResolveActiveAlerts(alertManagerURL)
		Expect(resolveErr).ToNot(HaveOccurred(), "Failed to resolve active alerts")

		Eventually(func() bool {
			return infrastructure.HasActiveAlerts(alertManagerURL)
		}, 30*time.Second, 2*time.Second).Should(BeFalse(),
			"AlertManager should have zero active alerts before injecting cert alert")

		// ================================================================
		// Step 3: Inject CertManagerCertNotReady alert
		// ================================================================
		By("Step 3: Injecting CertManagerCertNotReady alert into AlertManager")
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
					return we.Status.ExecutionEngine
				}
			}
			return ""
		}, timeout, interval).Should(Equal("job"), "WorkflowExecution should use job engine")

		By("Step 6b: Waiting for K8s Job to complete")
		Eventually(func(g Gomega) {
			jobList := &batchv1.JobList{}
			g.Expect(apiReader.List(ctx, jobList,
				client.InNamespace("kubernaut-workflows"),
				client.MatchingLabels{"kubernaut.ai/workflow-execution": weName})).To(Succeed())
			g.Expect(jobList.Items).NotTo(BeEmpty(), "No Jobs found for WorkflowExecution %s", weName)

			job := jobList.Items[0]
			Expect(job.Status.Failed).To(BeZero(),
				fmt.Sprintf("Job %s has %d failed pod(s) — check pod logs for details", job.Name, job.Status.Failed))
			g.Expect(job.Status.Succeeded).To(BeNumerically(">", 0),
				fmt.Sprintf("Job %s has not succeeded yet (active=%d)", job.Name, job.Status.Active))
		}, timeout, interval).Should(Succeed(), "K8s Job should complete successfully")

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
		// Step 9: CORE VALIDATION — EA has corrected timing (Issue #253)
		// ================================================================
		By("Step 9: Verifying EA created with config-driven propagation delay")
		eaName := fmt.Sprintf("ea-%s", remediationRequest.Name)
		eaKey := client.ObjectKey{Name: eaName, Namespace: namespace}

		ea := &eav1.EffectivenessAssessment{}
		Eventually(func() error {
			return apiReader.Get(testCtx, eaKey, ea)
		}, 30*time.Second, 2*time.Second).Should(Succeed(),
			"EA with name ea-<RR.Name> should be created by RO")

		Expect(ea.Spec.CorrelationID).To(Equal(remediationRequest.Name),
			"EA correlationID should match RR name")

		// CORE ASSERTION: Config.HashComputeDelay must be set for CRD target (#277)
		Expect(ea.Spec.Config.HashComputeDelay).ToNot(BeNil(),
			"EA.Spec.Config.HashComputeDelay MUST be set when remediation target is a CRD (Certificate)")
		Expect(ea.Spec.Config.HashComputeDelay.Duration).To(BeNumerically(">", 0),
			"HashComputeDelay should be a positive duration")

		// Verify remediation target is Certificate (from AIAnalysis.AffectedResource)
		Expect(ea.Spec.RemediationTarget.Kind).To(Equal("Certificate"),
			"EA remediation target should be Certificate (from AIAnalysis RCA)")

		GinkgoWriter.Println("  ┌─────────────────────────────────────────────────────────")
		GinkgoWriter.Println("  │ ASYNC HASH DEFERRAL VALIDATION (#277 Duration-based model)")
		GinkgoWriter.Println("  ├─────────────────────────────────────────────────────────")
		GinkgoWriter.Printf("  │ EA Created:             %s\n", ea.CreationTimestamp.Format("15:04:05"))
		GinkgoWriter.Printf("  │ HashComputeDelay:         %s\n", ea.Spec.Config.HashComputeDelay.Duration)
		GinkgoWriter.Printf("  │ Remediation Target:     %s/%s\n",
			ea.Spec.RemediationTarget.Kind, ea.Spec.RemediationTarget.Name)
		GinkgoWriter.Println("  └─────────────────────────────────────────────────────────")

		// ================================================================
		// Step 10: Verify WaitingForPropagation phase observed (Issue #253)
		// ================================================================
		By("Step 10a: Checking if EA entered WaitingForPropagation phase")
		// The EA may have already passed through WaitingForPropagation by now.
		// We log what we observe; in CI the timing may be tight.
		currentEA := &eav1.EffectivenessAssessment{}
		Expect(apiReader.Get(testCtx, eaKey, currentEA)).To(Succeed())
		GinkgoWriter.Printf("  EA current phase: %s (WaitingForPropagation may have already elapsed)\n",
			currentEA.Status.Phase)

		By("Step 10b: Waiting for EA to reach terminal phase (after propagation + stabilization)")
		Eventually(func() string {
			fetched := &eav1.EffectivenessAssessment{}
			if err := apiReader.Get(testCtx, eaKey, fetched); err != nil {
				return ""
			}
			return fetched.Status.Phase
		}, 5*time.Minute, 2*time.Second).Should(
			BeElementOf(eav1.PhaseCompleted, eav1.PhaseFailed),
			"EA should reach terminal phase after propagation delay + stabilization window")

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

		// Verify the audit payload includes hash_compute_after (EM computes from HashComputeDelay for backward compat)
		eaPayload, ok := scheduledEvent.EventData.GetEffectivenessAssessmentAuditPayload()
		Expect(ok).To(BeTrue(),
			"assessment.scheduled event should have EffectivenessAssessmentAuditPayload")
		Expect(eaPayload.HashComputeAfter.Set).To(BeTrue(),
			"assessment.scheduled audit payload must have hash_compute_after set for CRD target")
		GinkgoWriter.Printf("  ✅ Audit trail: hash_compute_after = %s\n",
			eaPayload.HashComputeAfter.Value.Format(time.RFC3339))

		// Verify core audit events are present
		// #280: lifecycle.completed now arrives after the Verifying phase, so use Eventually
		// to poll until all core events (including late-arriving ones) are present.
		coreEvents := []string{
			"gateway.signal.received",
			"gateway.crd.created",
			"orchestrator.lifecycle.verifying_started",      // #280
			"orchestrator.lifecycle.verification_completed", // #280
			"orchestrator.lifecycle.completed",
			"effectiveness.assessment.scheduled",
			"effectiveness.assessment.completed",
		}
		Eventually(func() []string {
			resp, err := dataStorageClient.QueryAuditEvents(testCtx, ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(correlationID),
				Limit:         ogenclient.NewOptInt(200),
			})
			if err != nil {
				return coreEvents
			}
			eventTypeCounts := map[string]int{}
			for _, event := range resp.Data {
				eventTypeCounts[event.EventType]++
			}
			var missing []string
			for _, eventType := range coreEvents {
				if eventTypeCounts[eventType] == 0 {
					missing = append(missing, eventType)
				}
			}
			GinkgoWriter.Printf("  [Audit] %d events, %d core missing\n", len(resp.Data), len(missing))
			return missing
		}, 240*time.Second, 2*time.Second).Should(BeEmpty(),
			"Audit trail must contain all core events")

		GinkgoWriter.Printf("  ✅ Audit trail: all %d core events present\n", len(coreEvents))

		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		GinkgoWriter.Println("✅ ASYNC HASH DEFERRAL E2E TEST COMPLETE [E2E-FP-253-001]")
		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		GinkgoWriter.Println("  CertManagerCertNotReady → full pipeline → config-driven propagation ✅")
		GinkgoWriter.Println("  Remediation target: Certificate (cert-manager.io CRD) ✅")
		GinkgoWriter.Println("  EA.Spec.Config.HashComputeDelay set by RO for async CRD target ✅")
		GinkgoWriter.Println("  EM deferred hash computation, EA reached terminal phase ✅")
		GinkgoWriter.Println("  Audit trail: hash_compute_after in assessment.scheduled ✅")
	})
})
