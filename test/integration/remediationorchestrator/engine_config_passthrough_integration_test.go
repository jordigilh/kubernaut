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
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// ============================================================================
// IT-WE-016-003: RO Creator Passes EngineConfig from AI to WFE CRD
// Authority: BR-WE-016 (EngineConfig Discriminator Pattern)
// Test Plan: docs/testing/45/TEST_PLAN.md
// Pattern: envtest with RO controller (full orchestration lifecycle)
//
// Validates that the RO WorkflowExecution creator correctly passes the
// engineConfig from AIAnalysis.Status.SelectedWorkflow to WFE.Spec.WorkflowRef.
// ============================================================================

var _ = Describe("EngineConfig Pass-Through (BR-WE-016)", func() {
	It("IT-WE-016-003: should pass engineConfig from AIAnalysis to WorkflowExecution CRD", func() {
		ns := createTestNamespace("ro-ec-003")
		defer deleteTestNamespace(ns)

		By("Creating a RemediationRequest")
		rr := createRemediationRequest(ns, "rr-ec-003")

		By("Driving RR to Processing phase")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

		By("Completing SignalProcessing")
		spName := fmt.Sprintf("sp-%s", rr.Name)
		sp := &signalprocessingv1.SignalProcessing{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ROControllerNamespace}, sp)
		}, timeout, interval).Should(Succeed())
		Expect(updateSPStatus(ROControllerNamespace, spName, signalprocessingv1.PhaseCompleted, "critical")).To(Succeed())

		By("Waiting for Analyzing phase")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseAnalyzing))

		By("Completing AIAnalysis with ansible engine and engineConfig")
		aiName := fmt.Sprintf("ai-%s", rr.Name)
		ai := &aianalysisv1.AIAnalysis{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: ROControllerNamespace}, ai)
		}, timeout, interval).Should(Succeed())

		ansibleConfig, err := json.Marshal(map[string]interface{}{
			"playbookPath":    "playbooks/restart-deployment.yml",
			"jobTemplateName": "restart-deployment",
			"inventoryName":   "production",
		})
		Expect(err).ToNot(HaveOccurred())

		ai.Status.Phase = aianalysisv1.PhaseCompleted
		ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
			WorkflowID:      "wf-ansible-restart",
			Version:         "v2.0.0",
			ExecutionBundle: "https://github.com/kubernaut/playbooks.git",
			Confidence:      0.92,
			Rationale:       "Ansible playbook for deployment restart",
			ExecutionEngine: "ansible",
			EngineConfig: &apiextensionsv1.JSON{
				Raw: ansibleConfig,
			},
		}
		ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
			Summary:    "Memory leak detected",
			Severity:   "critical",
			SignalType: "alert",
			AffectedResource: &aianalysisv1.AffectedResource{
				Kind:      "Deployment",
				Name:      "leaky-app",
				Namespace: ns,
			},
		}
		now := metav1.Now()
		ai.Status.CompletedAt = &now
		Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

		By("Waiting for Executing phase (RO creates WFE)")
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseExecuting))

		By("Verifying WFE has engineConfig from AIAnalysis")
		weName := fmt.Sprintf("we-%s", rr.Name)
		we := &workflowexecutionv1.WorkflowExecution{}
		Eventually(func() error {
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: weName, Namespace: ROControllerNamespace}, we)
		}, timeout, interval).Should(Succeed())

		Expect(we.Status.ExecutionEngine).To(Equal("ansible"),
			"WFE should have ansible execution engine on status")
		Expect(we.Spec.WorkflowRef.EngineConfig).ToNot(BeNil(),
			"WFE.Spec.WorkflowRef.EngineConfig must be populated from AIAnalysis")

		var parsedConfig map[string]interface{}
		err = json.Unmarshal(we.Spec.WorkflowRef.EngineConfig.Raw, &parsedConfig)
		Expect(err).ToNot(HaveOccurred(), "EngineConfig should be valid JSON")
		Expect(parsedConfig["playbookPath"]).To(Equal("playbooks/restart-deployment.yml"),
			"playbookPath should pass through from AI to WFE")
		Expect(parsedConfig["jobTemplateName"]).To(Equal("restart-deployment"),
			"jobTemplateName should pass through from AI to WFE")
		Expect(parsedConfig["inventoryName"]).To(Equal("production"),
			"inventoryName should pass through from AI to WFE")

		Expect(we.Spec.WorkflowRef.WorkflowID).To(Equal("wf-ansible-restart"))
		Expect(we.Spec.WorkflowRef.Version).To(Equal("v2.0.0"))
		Expect(we.Spec.WorkflowRef.ExecutionBundle).To(Equal("https://github.com/kubernaut/playbooks.git"))

		GinkgoWriter.Printf("✅ IT-WE-016-003: engineConfig passed through from AI to WFE\n")
	})
})
