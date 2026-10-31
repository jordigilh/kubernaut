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

package effectivenessmonitor

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

var _ = Describe("DD-2582 EffectivenessAssessment parent lifecycle", Label("integration", "issue-2582"), func() {
	It("IT-EM-2582-001: cancels before checks when the persisted RR is Failed [BR-EM-005, AC-4]", func() {
		rrName := helpers.UniqueTestName("rr-em-parent-failed")
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: rrName, Namespace: "default"},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
				SignalName:        "ParentLifecycleFailure",
				Severity:          "critical",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				TargetResource:    remediationv1.ResourceIdentifier{Kind: "Pod", Name: "parent-lifecycle-target", Namespace: "default"},
				FiringTime:        metav1.Now(),
				ReceivedTime:      metav1.Now(),
			},
		}
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, rr) })

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)).To(Succeed())
		rr.Status.OverallPhase = remediationv1.PhaseFailed
		Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      helpers.UniqueTestName("ea-parent-failed"),
				Namespace: "default",
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           rrName,
				RemediationRequestPhase: "Verifying",
				SignalTarget:            eav1.TargetResource{Kind: "Deployment", Name: "parent-lifecycle-target", Namespace: "default"},
				RemediationTarget:       eav1.TargetResource{Kind: "Deployment", Name: "parent-lifecycle-target", Namespace: "default"},
			},
		}
		Expect(k8sClient.Create(ctx, ea)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, ea) })

		Eventually(func() string {
			fetched := &eav1.EffectivenessAssessment{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(ea), fetched); err != nil {
				return ""
			}
			return fetched.Status.Phase
		}, 2*time.Minute, time.Second).Should(Equal(eav1.PhaseCancelled))
	})
})
