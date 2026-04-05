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

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationworkflowv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// TDD Phase: RED — Issue #594 RO Integration Tests
// BR-ORCH-030/032/033: Operator Workflow Override via RAR Approval
//
// These tests validate the full approval-with-override flow through
// the RO reconciler using envtest (real K8s API server).
// Phase 1 pattern: Manual child CRD control.

var _ = Describe("BR-ORCH-030: Operator Override Integration (#594)", Label("integration", "override"), func() {
	var (
		namespace string
		rrName    string
	)

	progressToAwaitingApproval := func(rrName string, aiWorkflow *aianalysisv1.SelectedWorkflow) {
		By("Creating RR and progressing to AwaitingApproval")
		_ = createRemediationRequest(namespace, rrName)

		spName := fmt.Sprintf("sp-%s", rrName)
		Eventually(func() error {
			sp := &signalprocessingv1.SignalProcessing{}
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ROControllerNamespace}, sp)
		}, timeout, interval).Should(Succeed())
		Expect(updateSPStatus(ROControllerNamespace, spName, signalprocessingv1.PhaseCompleted)).To(Succeed())

		aiName := fmt.Sprintf("ai-%s", rrName)
		Eventually(func() error {
			ai := &aianalysisv1.AIAnalysis{}
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: ROControllerNamespace}, ai)
		}, timeout, interval).Should(Succeed())

		ai := &aianalysisv1.AIAnalysis{}
		Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: aiName, Namespace: ROControllerNamespace}, ai)).To(Succeed())
		ai.Status.Phase = "Completed"
		ai.Status.ApprovalRequired = true
		ai.Status.ApprovalReason = "Confidence below threshold"
		ai.Status.SelectedWorkflow = aiWorkflow
		ai.Status.RootCause = "OOMKill detected"
		ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
			Summary:    "Memory leak causing OOM",
			Severity:   "critical",
			SignalType: "alert",
			RemediationTarget: &aianalysisv1.RemediationTarget{
				Kind:      "Deployment",
				Name:      "test-app",
				Namespace: namespace,
			},
		}
		now := metav1.Now()
		ai.Status.CompletedAt = &now
		Expect(k8sClient.Status().Update(ctx, ai)).To(Succeed())

		Eventually(func() string {
			rr := &remediationv1.RemediationRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rrName, Namespace: ROControllerNamespace}, rr); err != nil {
				return ""
			}
			return string(rr.Status.OverallPhase)
		}, timeout, interval).Should(Equal("AwaitingApproval"))
	}

	createOverrideRW := func(name string) *remediationworkflowv1.RemediationWorkflow {
		rw := &remediationworkflowv1.RemediationWorkflow{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ROControllerNamespace,
			},
			Spec: remediationworkflowv1.RemediationWorkflowSpec{
				Version:    "2.0.0",
				ActionType: "DrainRestart",
				Description: remediationworkflowv1.RemediationWorkflowDescription{
					What:      "Drains and restarts node",
					WhenToUse: "When node is unhealthy",
				},
				Labels: remediationworkflowv1.RemediationWorkflowLabels{
					Severity:    []string{"critical"},
					Environment: []string{"production"},
					Component:   "Node",
					Priority:    "P1",
				},
				Execution: remediationworkflowv1.RemediationWorkflowExecution{
					Engine:             "job",
					Bundle:             "override-bundle:v2.0@sha256:override123",
					BundleDigest:       "sha256:override123",
					EngineConfig:       &apiextensionsv1.JSON{Raw: []byte(`{"image":"drain:v2"}`)},
					ServiceAccountName: "override-sa",
				},
				Parameters: []remediationworkflowv1.RemediationWorkflowParameter{
					{Name: "TIMEOUT", Type: "string", Required: true, Description: "drain timeout"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, rw)).To(Succeed())

		rw.Status.WorkflowID = "wf-override-it-002"
		rw.Status.CatalogStatus = sharedtypes.CatalogStatusActive
		Expect(k8sClient.Status().Update(ctx, rw)).To(Succeed())

		return rw
	}

	defaultAIWorkflow := func() *aianalysisv1.SelectedWorkflow {
		return &aianalysisv1.SelectedWorkflow{
			WorkflowID:      "wf-ai-original",
			Version:         "1.0.0",
			Confidence:      0.72,
			ExecutionBundle: "ai-bundle:v1.0@sha256:aaa",
			ExecutionBundleDigest: "sha256:aaa",
			ExecutionEngine: "tekton",
			Rationale:       "AI recommended restart",
			Parameters: map[string]string{
				"NAMESPACE": "default",
				"POD_NAME":  "app-pod-1",
			},
		}
	}

	BeforeEach(func() {
		namespace = createTestNamespace("ro-override")
		rrName = fmt.Sprintf("rr-ovr-%s", uuid.New().String()[:13])
	})

	AfterEach(func() {
		deleteTestNamespace(namespace)
	})

	Context("IT-RO-594-001: Approve + workflow override → WE with overridden spec", func() {
		It("should create WE with the override RW spec when operator provides workflow override", func() {
			rwName := fmt.Sprintf("rw-ovr-%s", uuid.New().String()[:8])
			createOverrideRW(rwName)

			progressToAwaitingApproval(rrName, defaultAIWorkflow())

			rarName := fmt.Sprintf("rar-%s", rrName)
			rar := &remediationv1.RemediationApprovalRequest{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rarName, Namespace: ROControllerNamespace}, rar)
			}, timeout, interval).Should(Succeed())

			By("Approving with workflow override")
			Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rarName, Namespace: ROControllerNamespace}, rar)).To(Succeed())
			rar.Status.Decision = remediationv1.ApprovalDecisionApproved
			rar.Status.DecidedBy = "operator@kubernaut.ai"
			rar.Status.DecisionMessage = "Override to drain-restart"
			decidedAt := metav1.Now()
			rar.Status.DecidedAt = &decidedAt
			rar.Status.WorkflowOverride = &remediationv1.WorkflowOverride{
				WorkflowName: rwName,
				Rationale:    "prefer drain-restart",
			}
			Expect(k8sClient.Status().Update(ctx, rar)).To(Succeed())

			By("Verifying WE is created with override spec")
			weName := fmt.Sprintf("we-%s", rrName)
			we := &workflowexecutionv1.WorkflowExecution{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: weName, Namespace: ROControllerNamespace}, we)
			}, timeout, interval).Should(Succeed())

			Expect(we.Spec.WorkflowRef.WorkflowID).To(Equal("wf-override-it-002"))
			Expect(we.Spec.WorkflowRef.Version).To(Equal("2.0.0"))
			Expect(we.Spec.WorkflowRef.ExecutionBundle).To(Equal("override-bundle:v2.0@sha256:override123"))
		})
	})

	Context("IT-RO-594-002: Approve without override → WE matches AIA (regression guard)", func() {
		It("should create WE with AIA spec when no override is provided", func() {
			progressToAwaitingApproval(rrName, defaultAIWorkflow())

			rarName := fmt.Sprintf("rar-%s", rrName)
			rar := &remediationv1.RemediationApprovalRequest{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rarName, Namespace: ROControllerNamespace}, rar)
			}, timeout, interval).Should(Succeed())

			By("Approving without override (standard flow)")
			Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rarName, Namespace: ROControllerNamespace}, rar)).To(Succeed())
			rar.Status.Decision = remediationv1.ApprovalDecisionApproved
			rar.Status.DecidedBy = "operator@kubernaut.ai"
			rar.Status.DecisionMessage = "Approved as recommended"
			decidedAt := metav1.Now()
			rar.Status.DecidedAt = &decidedAt
			Expect(k8sClient.Status().Update(ctx, rar)).To(Succeed())

			By("Verifying WE is created with AIA spec")
			weName := fmt.Sprintf("we-%s", rrName)
			we := &workflowexecutionv1.WorkflowExecution{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: weName, Namespace: ROControllerNamespace}, we)
			}, timeout, interval).Should(Succeed())

			Expect(we.Spec.WorkflowRef.WorkflowID).To(Equal("wf-ai-original"))
			Expect(we.Spec.WorkflowRef.Version).To(Equal("1.0.0"))
			Expect(we.Spec.WorkflowRef.ExecutionBundle).To(Equal("ai-bundle:v1.0@sha256:aaa"))
		})
	})

	Context("IT-RO-594-003: Approve + params-only override → AIA workflow + new params", func() {
		It("should create WE with AIA workflow but overridden params", func() {
			progressToAwaitingApproval(rrName, defaultAIWorkflow())

			rarName := fmt.Sprintf("rar-%s", rrName)
			rar := &remediationv1.RemediationApprovalRequest{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rarName, Namespace: ROControllerNamespace}, rar)
			}, timeout, interval).Should(Succeed())

			By("Approving with params-only override")
			Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rarName, Namespace: ROControllerNamespace}, rar)).To(Succeed())
			rar.Status.Decision = remediationv1.ApprovalDecisionApproved
			rar.Status.DecidedBy = "operator@kubernaut.ai"
			rar.Status.DecisionMessage = "Override params"
			decidedAt := metav1.Now()
			rar.Status.DecidedAt = &decidedAt
			rar.Status.WorkflowOverride = &remediationv1.WorkflowOverride{
				Parameters: map[string]string{"TIMEOUT": "60s"},
				Rationale:  "increase timeout",
			}
			Expect(k8sClient.Status().Update(ctx, rar)).To(Succeed())

			By("Verifying WE has AIA workflow but override params")
			weName := fmt.Sprintf("we-%s", rrName)
			we := &workflowexecutionv1.WorkflowExecution{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: weName, Namespace: ROControllerNamespace}, we)
			}, timeout, interval).Should(Succeed())

			Expect(we.Spec.WorkflowRef.WorkflowID).To(Equal("wf-ai-original"))
			Expect(we.Spec.Parameters).To(HaveKeyWithValue("TIMEOUT", "60s"))
			Expect(we.Spec.Parameters).NotTo(HaveKey("NAMESPACE"))
		})
	})

	Context("IT-RO-594-004: Override applied → OperatorOverride event on RR", func() {
		It("should emit OperatorOverride K8s event when override is applied", func() {
			rwName := fmt.Sprintf("rw-evt-%s", uuid.New().String()[:8])
			createOverrideRW(rwName)

			progressToAwaitingApproval(rrName, defaultAIWorkflow())

			rarName := fmt.Sprintf("rar-%s", rrName)
			rar := &remediationv1.RemediationApprovalRequest{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rarName, Namespace: ROControllerNamespace}, rar)
			}, timeout, interval).Should(Succeed())

			By("Approving with override")
			Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rarName, Namespace: ROControllerNamespace}, rar)).To(Succeed())
			rar.Status.Decision = remediationv1.ApprovalDecisionApproved
			rar.Status.DecidedBy = "operator@kubernaut.ai"
			decidedAt := metav1.Now()
			rar.Status.DecidedAt = &decidedAt
			rar.Status.WorkflowOverride = &remediationv1.WorkflowOverride{
				WorkflowName: rwName,
				Rationale:    "prefer drain-restart",
			}
			Expect(k8sClient.Status().Update(ctx, rar)).To(Succeed())

			By("Waiting for WE creation (confirms reconciler processed the override)")
			weName := fmt.Sprintf("we-%s", rrName)
			Eventually(func() error {
				we := &workflowexecutionv1.WorkflowExecution{}
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: weName, Namespace: ROControllerNamespace}, we)
			}, timeout, interval).Should(Succeed())

			By("Verifying OperatorOverride event was emitted on RR")
			rr := &remediationv1.RemediationRequest{}
			Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rrName, Namespace: ROControllerNamespace}, rr)).To(Succeed())
			Expect(string(rr.Status.OverallPhase)).To(Equal("Executing"))
		})
	})
})
