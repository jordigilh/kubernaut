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

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
		ns := createTestNamespace(ctx, "ro-ec-003")
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
		Expect(updateSPStatus(spName, "critical")).To(Succeed())

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
		ai.Status.EnsureRCAResult().SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
			WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
				WorkflowID:         "wf-ansible-restart",
				WorkflowName:       "wf-ansible-restart",
				ActionType:         "RestartPod",
				Version:            "v2.0.0",
				ExecutionBundle:    "https://github.com/kubernaut/playbooks.git",
				ExecutionEngine:    "ansible",
				ServiceAccountName: "workflow-ansible",
				ExecutionClusterID: "production-east",
				Dependencies: &sharedtypes.WorkflowDependencies{
					Secrets:    []sharedtypes.WorkflowResourceDependency{{Name: "gitea-repo-creds"}},
					ConfigMaps: []sharedtypes.WorkflowResourceDependency{{Name: "workflow-config"}},
				},
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{"cpu": resource.MustParse("100m")},
					Limits:   corev1.ResourceList{"memory": resource.MustParse("256Mi")},
				},
				DeclaredParameterNames: map[string]bool{"TARGET_NAMESPACE": true, "MEMORY_LIMIT_NEW": true},
				EngineConfig: &apiextensionsv1.JSON{
					Raw: ansibleConfig,
				},
			},
			Confidence: 0.92,
			Rationale:  "Ansible playbook for deployment restart",
		}
		ai.Status.EnsureRCAResult().RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
			Summary:    "Memory leak detected",
			Severity:   "critical",
			SignalType: "alert",
			RemediationTarget: &aianalysisv1.RemediationTarget{
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

		// Issue #1661 Change 11d/11f: the RO creator now copies ExecutionEngine
		// (and its siblings, including ActionType as of Change 11f) verbatim
		// onto WorkflowRef from AIAnalysis.Status.SelectedWorkflow -- there is
		// no runtime DS catalog resolution step anymore. This test's actual
		// concern is the EngineConfig pass-through; verify that works correctly.
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
		Expect(we.Spec.WorkflowRef.ServiceAccountName).To(Equal("workflow-ansible"))
		Expect(we.Spec.ClusterID).To(Equal("production-east"))
		Expect(we.Spec.WorkflowRef.Dependencies.Secrets).To(ConsistOf(sharedtypes.WorkflowResourceDependency{Name: "gitea-repo-creds"}))
		Expect(we.Spec.WorkflowRef.Dependencies.ConfigMaps).To(ConsistOf(sharedtypes.WorkflowResourceDependency{Name: "workflow-config"}))
		Expect(we.Spec.WorkflowRef.Resources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("100m")))
		Expect(we.Spec.WorkflowRef.Resources.Limits[corev1.ResourceMemory]).To(Equal(resource.MustParse("256Mi")))
		Expect(we.Spec.WorkflowRef.DeclaredParameterNames).To(Equal(map[string]bool{"TARGET_NAMESPACE": true, "MEMORY_LIMIT_NEW": true}))

		GinkgoWriter.Printf("✅ IT-WE-016-003: engineConfig passed through from AI to WFE\n")
	})
})
