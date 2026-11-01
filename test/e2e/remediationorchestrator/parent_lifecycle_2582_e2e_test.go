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
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

var _ = Describe("E2E-2582-005: fail closed across RO restart", Serial, Label("e2e", "issue-2582"), func() {
	It("does not create children after a failed parent survives a controller restart", func() {
		ns := createTestNamespace(ctx, "ro-parent-restart")
		DeferCleanup(func() { deleteTestNamespace(ns) })

		rrName := helpers.UniqueTestName("rr-parent-restart")
		now := metav1.Now()
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: rrName, Namespace: controllerNamespace},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
				SignalName:        "ParentRestart",
				Severity:          "critical",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				TargetResource:    remediationv1.ResourceIdentifier{Kind: "Pod", Name: "parent-restart-target", Namespace: ns},
				FiringTime:        now,
				ReceivedTime:      now,
			},
		}
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, rr) })
		Eventually(func() error {
			fresh := &remediationv1.RemediationRequest{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), fresh); err != nil {
				return err
			}
			fresh.Status.OverallPhase = remediationv1.PhaseFailed
			return k8sClient.Status().Update(ctx, fresh)
		}, timeout, interval).Should(Succeed(), "persist failed parent phase without losing a concurrent RO status update")

		restart := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"rollout", "restart", "deployment/remediationorchestrator-controller", "-n", controllerNamespace)
		_, err := restart.CombinedOutput()
		Expect(err).NotTo(HaveOccurred())
		wait := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"rollout", "status", "deployment/remediationorchestrator-controller", "-n", controllerNamespace, "--timeout=120s")
		_, err = wait.CombinedOutput()
		Expect(err).NotTo(HaveOccurred())

		Consistently(func(g Gomega) {
			spList := &signalprocessingv1.SignalProcessingList{}
			g.Expect(k8sClient.List(ctx, spList, client.InNamespace(controllerNamespace))).To(Succeed())
			for i := range spList.Items {
				g.Expect(spList.Items[i].Spec.RemediationRequestRef.Name).NotTo(Equal(rrName))
			}
			aiList := &aianalysisv1.AIAnalysisList{}
			g.Expect(k8sClient.List(ctx, aiList, client.InNamespace(controllerNamespace))).To(Succeed())
			for i := range aiList.Items {
				g.Expect(aiList.Items[i].Spec.RemediationRequestRef.Name).NotTo(Equal(rrName))
			}
			wfeList := &workflowexecutionv1.WorkflowExecutionList{}
			g.Expect(k8sClient.List(ctx, wfeList, client.InNamespace(controllerNamespace))).To(Succeed())
			for i := range wfeList.Items {
				g.Expect(wfeList.Items[i].Spec.RemediationRequestRef.Name).NotTo(Equal(rrName))
			}
		}, 10*time.Second, interval).Should(Succeed())
	})
})
