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

package workflowexecution

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

var _ = Describe("DD-2582 WorkflowExecution parent lifecycle", Label("integration", "issue-2582"), func() {
	It("IT-WE-2582-001: keeps a pre-created WFE from dispatching while RR is still Analyzing [BR-WE-007, AC-4]", func() {
		rrName := "test-rr-2582-parent-wait"
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: rrName, Namespace: DefaultNamespace},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
				SignalName:        "ParentLifecycleWait",
				Severity:          "critical",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				TargetResource:    remediationv1.ResourceIdentifier{Kind: "Pod", Name: "parent-lifecycle-target", Namespace: DefaultNamespace},
				FiringTime:        metav1.Now(),
				ReceivedTime:      metav1.Now(),
			},
		}
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, rr) })

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)).To(Succeed())
		rr.Status.OverallPhase = remediationv1.PhaseAnalyzing
		Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

		wfe := createUniqueWFE("2582-parent-wait", "default/deployment/parent-lifecycle-target")
		wfe.Spec.RemediationRequestRef.Name = rrName
		Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, wfe) })

		Consistently(func(g Gomega) {
			fetched := &workflowexecutionv1alpha1.WorkflowExecution{}
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe), fetched)).To(Succeed())
			g.Expect(fetched.Status.Phase).To(BeElementOf("", workflowexecutionv1alpha1.PhasePending),
				"parent authorization wait must not enter execution")
			g.Expect(fetched.Status.ExecutionRef).To(BeNil(),
				"parent authorization wait must not create a backend execution")
		}, 5*time.Second, 250*time.Millisecond).Should(Succeed())
	})

	It("IT-WE-2582-002: enters child-owned cancellation before claiming backend termination [BR-WE-014, AC-4]", func() {
		rrName := "test-rr-2582-cancel-" + time.Now().Format("150405000")
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: rrName, Namespace: DefaultNamespace},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				SignalName:        "parent-cancel",
				Severity:          "critical",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				TargetResource:    remediationv1.ResourceIdentifier{Kind: "Pod", Name: "parent-cancel-target", Namespace: DefaultNamespace},
				FiringTime:        metav1.Now(),
				ReceivedTime:      metav1.Now(),
			},
		}
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, rr) })
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)).To(Succeed())
		rr.Status.OverallPhase = remediationv1.PhaseFailed
		Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

		wfe := createUniqueWFE("2582-parent-cancel", "default/deployment/parent-cancel-target")
		wfe.Spec.RemediationRequestRef.Name = rrName
		Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, wfe) })

		Eventually(func() string {
			fetched := &workflowexecutionv1alpha1.WorkflowExecution{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe), fetched); err != nil {
				return ""
			}
			return fetched.Status.Phase
		}, 30*time.Second, 250*time.Millisecond).Should(BeElementOf(
			string(workflowexecutionv1alpha1.PhaseCancelled),
			string(workflowexecutionv1alpha1.PhaseCancelling),
		))
	})
})
