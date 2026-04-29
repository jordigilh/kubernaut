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

package remediationorchestrator

import (
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// E2E Tests for #712, #736: Dry-Run Mode
//
// Business Requirements:
// - ADR-RO-001: Dry-Run Mode — EA Policy Decoupling from Remediation Pipeline
// - BR-ORCH-037: Dry-run outcome in RemediationRequest
//
// When dryRun is enabled in the RO config, the pipeline stops after AI analysis
// completes — no WFE, RAR, or EA CRDs are created. The RemediationRequest
// completes immediately with outcome "DryRun".
//
// IMPORTANT: This test is Serial because it modifies the RO ConfigMap and
// restarts the controller. It must not run in parallel with other RO E2E tests.
var _ = Describe("ADR-RO-001: Dry-Run Mode E2E", Serial, Label("e2e", "dry-run"), func() {
	const (
		roConfigMapName = "remediationorchestrator-config"
		roDeployment    = "remediationorchestrator-controller"
	)

	var (
		testNS          string
		originalConfig  string
		configPatched   bool
	)

	BeforeEach(func() {
		configPatched = false
		testNS = createTestNamespace("ro-dryrun-e2e")

		By("Saving original RO ConfigMap for restoration")
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"get", "configmap", roConfigMapName,
			"-n", controllerNamespace,
			"-o", "jsonpath={.data.remediationorchestrator\\.yaml}")
		out, err := cmd.CombinedOutput()
		Expect(err).ToNot(HaveOccurred(), "Failed to read RO ConfigMap: %s", string(out))
		originalConfig = string(out)

		By("Patching RO ConfigMap to enable dryRun mode (#712, #736)")
		dryRunConfig := originalConfig + "\ndryRun: true\ndryRunHoldPeriod: 5m\n"
		patchJSON := fmt.Sprintf(`{"data":{"remediationorchestrator.yaml":%q}}`, dryRunConfig)
		patchCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"patch", "configmap", roConfigMapName,
			"-n", controllerNamespace,
			"--type=merge", "-p", patchJSON)
		patchOut, patchErr := patchCmd.CombinedOutput()
		Expect(patchErr).ToNot(HaveOccurred(), "Failed to patch RO ConfigMap: %s", string(patchOut))
		configPatched = true

		By("Restarting RO controller to pick up dryRun config")
		restartCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"rollout", "restart", "deployment/"+roDeployment,
			"-n", controllerNamespace)
		restartOut, restartErr := restartCmd.CombinedOutput()
		Expect(restartErr).ToNot(HaveOccurred(), "Failed to restart RO: %s", string(restartOut))

		By("Waiting for RO rollout to complete")
		waitCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"rollout", "status", "deployment/"+roDeployment,
			"-n", controllerNamespace,
			"--timeout=120s")
		waitOut, waitErr := waitCmd.CombinedOutput()
		Expect(waitErr).ToNot(HaveOccurred(), "RO rollout did not complete: %s", string(waitOut))
	})

	AfterEach(func() {
		if configPatched {
			By("Restoring original RO ConfigMap (disable dryRun)")
			patchJSON := fmt.Sprintf(`{"data":{"remediationorchestrator.yaml":%q}}`, originalConfig)
			patchCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
				"patch", "configmap", roConfigMapName,
				"-n", controllerNamespace,
				"--type=merge", "-p", patchJSON)
			patchOut, patchErr := patchCmd.CombinedOutput()
			if patchErr != nil {
				GinkgoWriter.Printf("Warning: failed to restore RO ConfigMap: %v\n%s\n", patchErr, string(patchOut))
			}

			By("Restarting RO controller to restore normal operation")
			restartCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
				"rollout", "restart", "deployment/"+roDeployment,
				"-n", controllerNamespace)
			_, _ = restartCmd.CombinedOutput()

			waitCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
				"rollout", "status", "deployment/"+roDeployment,
				"-n", controllerNamespace,
				"--timeout=120s")
			_, _ = waitCmd.CombinedOutput()
		}

		deleteTestNamespace(testNS)
	})

	// ========================================
	// E2E-RO-712-001: Dry-run completes without execution
	// ========================================
	Describe("E2E-RO-712-001: Pipeline stops after AI analysis in dry-run mode", func() {
		It("should complete RR with outcome DryRun and NOT create WorkflowExecution", func() {
			By("Creating RemediationRequest")
			now := metav1.Now()
			fingerprint := "e2edryrun1111111111111111111111111111111111111111111111111111001"
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-e2e-dryrun-001",
					Namespace: controllerNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
					SignalName:        "DryRunTestOOMKilled",
					Severity:          "critical",
					SignalType:        "oomkilled",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "test-pod-dryrun",
						Namespace: testNS,
					},
					FiringTime:   now,
					ReceivedTime: now,
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rr) })

			By("Waiting for RO to create SignalProcessing CRD")
			var sp *signalprocessingv1.SignalProcessing
			Eventually(func() bool {
				spList := &signalprocessingv1.SignalProcessingList{}
				_ = k8sClient.List(ctx, spList, client.InNamespace(controllerNamespace))
				for i := range spList.Items {
					if len(spList.Items[i].OwnerReferences) > 0 &&
						spList.Items[i].OwnerReferences[0].Kind == "RemediationRequest" &&
						spList.Items[i].OwnerReferences[0].Name == rr.Name {
						sp = &spList.Items[i]
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(), "SignalProcessing should be created by RO")

			By("Simulating SP completion")
			sp.Status.Phase = signalprocessingv1.PhaseCompleted
			sp.Status.Severity = "critical"
			sp.Status.SignalMode = "reactive"
			sp.Status.SignalName = sp.Spec.Signal.Name
			sp.Status.EnvironmentClassification = &signalprocessingv1.EnvironmentClassification{
				Environment:  signalprocessingv1.EnvironmentProduction,
				Source:       "namespace-labels",
				ClassifiedAt: metav1.Now(),
			}
			sp.Status.PriorityAssignment = &signalprocessingv1.PriorityAssignment{
				Priority:   signalprocessingv1.PriorityP1,
				Source:     "rego-policy",
				AssignedAt: metav1.Now(),
			}
			Expect(k8sClient.Status().Update(ctx, sp)).To(Succeed())

			By("Waiting for RO to create AIAnalysis CRD")
			var analysis *aianalysisv1.AIAnalysis
			Eventually(func() bool {
				analysisList := &aianalysisv1.AIAnalysisList{}
				_ = k8sClient.List(ctx, analysisList, client.InNamespace(controllerNamespace))
				for i := range analysisList.Items {
					if len(analysisList.Items[i].OwnerReferences) > 0 &&
						analysisList.Items[i].OwnerReferences[0].Kind == "RemediationRequest" &&
						analysisList.Items[i].OwnerReferences[0].Name == rr.Name {
						analysis = &analysisList.Items[i]
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(), "AIAnalysis should be created by RO")

			By("Simulating AI completion with workflow recommendation (dry-run intercepts before WFE creation)")
			analysis.Status.Phase = aianalysisv1.PhaseCompleted
			analysis.Status.Reason = aianalysisv1.ReasonAnalysisCompleted
			analysis.Status.NeedsHumanReview = false
			analysis.Status.Message = "Workflow recommended: restart-pod-v1"
			analysis.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
				WorkflowID:      "restart-pod-v1",
				Version:         "1.0.0",
				ExecutionBundle: "quay.io/kubernaut/restart-pod:v1",
				Confidence:      0.95,
				Rationale:       "High confidence workflow match for pod restart scenario",
			}
			analysis.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
				Summary:    "OOM kill detected on pod",
				Severity:   "critical",
				SignalType: "alert",
				RemediationTarget: &aianalysisv1.RemediationTarget{
					Kind:      "Pod",
					Name:      "test-pod-dryrun",
					Namespace: testNS,
				},
			}
			Expect(k8sClient.Status().Update(ctx, analysis)).To(Succeed())

			By("Waiting for RR to reach Completed phase with DryRun outcome")
			updatedRR := &remediationv1.RemediationRequest{}
			Eventually(func() remediationv1.RemediationPhase {
				_ = apiReader.Get(ctx, client.ObjectKeyFromObject(rr), updatedRR)
				return updatedRR.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseCompleted),
				"RR should reach Completed phase (dry-run intercept in AnalyzingHandler)")

			Expect(updatedRR.Status.Outcome).To(Equal("DryRun"),
				"Outcome must be DryRun per ADR-RO-001")

			By("Verifying NO WorkflowExecution was created (dry-run stops pipeline)")
			Consistently(func() int {
				weList := &workflowexecutionv1.WorkflowExecutionList{}
				_ = k8sClient.List(ctx, weList, client.InNamespace(controllerNamespace))
				count := 0
				for _, item := range weList.Items {
					if len(item.OwnerReferences) > 0 &&
						item.OwnerReferences[0].Kind == "RemediationRequest" &&
						item.OwnerReferences[0].Name == rr.Name {
						count++
					}
				}
				return count
			}, 10*time.Second, interval).Should(Equal(0),
				"NO WorkflowExecution should exist — dry-run intercepts before WFE creation")

			By("Verifying NextAllowedExecution is set for Gateway dedup suppression")
			Expect(updatedRR.Status.NextAllowedExecution).ToNot(BeNil(),
				"NextAllowedExecution must be set per ADR-RO-001 Gateway dedup")
		})
	})
})
