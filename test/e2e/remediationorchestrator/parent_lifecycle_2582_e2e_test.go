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
		deployment := "deployment/remediationorchestrator-controller"
		scale := func(replicas string) {
			cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
				"scale", deployment, "-n", controllerNamespace, "--replicas="+replicas)
			_, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred())
		}
		waitForReplicas := func(expected string) {
			Eventually(func() string {
				cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
					"get", deployment, "-n", controllerNamespace, "-o", "jsonpath={.status.replicas}")
				output, err := cmd.Output()
				if err != nil {
					return ""
				}
				return string(output)
			}, timeout, interval).Should(Equal(expected))
		}

		// Stop RO before creating the failed parent. Otherwise the controller
		// can legitimately observe the just-created RR between Create and the
		// status-subresource update, creating a child before the fixture is
		// authorized as terminal.
		scale("0")
		waitForReplicas("0")

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
		scale("1")
		waitForReplicas("1")

		restart := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"rollout", "restart", deployment, "-n", controllerNamespace)
		_, err := restart.CombinedOutput()
		Expect(err).NotTo(HaveOccurred())
		wait := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"rollout", "status", deployment, "-n", controllerNamespace, "--timeout=120s")
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
