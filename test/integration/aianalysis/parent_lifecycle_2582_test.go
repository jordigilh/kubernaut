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

package aianalysis

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

var _ = Describe("DD-2582 AIAnalysis parent lifecycle", Label("integration", "issue-2582"), func() {
	It("IT-AI-2582-001: cancels before AgentSession creation when the persisted RR is Failed [BR-AI-023, AC-4]", func() {
		rrName := helpers.UniqueTestName("rr-ai-parent-failed")
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: rrName, Namespace: testNamespace},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
				SignalName:        "ParentLifecycleFailure",
				Severity:          "critical",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				TargetResource:    remediationv1.ResourceIdentifier{Kind: "Pod", Name: "parent-lifecycle-target", Namespace: testNamespace},
				FiringTime:        metav1.Now(),
				ReceivedTime:      metav1.Now(),
			},
		}
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, rr) })

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)).To(Succeed())
		rr.Status.OverallPhase = remediationv1.PhaseFailed
		Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

		analysis := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      helpers.UniqueTestName("ai-parent-failed"),
				Namespace: testNamespace,
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: rrName, Namespace: testNamespace},
				RemediationID:         rrName,
				AnalysisRequest: aianalysisv1.AnalysisRequest{
					SignalContext: aianalysisv1.SignalContextInput{
						Fingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
						Severity:    "critical",
						SignalName:  "ParentLifecycleFailure",
						TargetResource: aianalysisv1.TargetResource{
							Kind:      "Pod",
							Name:      "parent-lifecycle-target",
							Namespace: testNamespace,
						},
					},
					AnalysisTypes: []aianalysisv1.AnalysisType{aianalysisv1.AnalysisTypeInvestigation},
				},
			},
		}
		Expect(k8sClient.Create(ctx, analysis)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, analysis) })

		Eventually(func() string {
			fetched := &aianalysisv1.AIAnalysis{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), fetched); err != nil {
				return ""
			}
			return fetched.Status.Phase
		}, 2*time.Minute, time.Second).Should(Equal(aianalysisv1.PhaseCancelled))

		agentSession := &agentsessionv1.AgentSession{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "as-" + rrName, Namespace: testNamespace}, agentSession)).ToNot(Succeed(),
			"a failed parent must prevent the AIA production path from creating AgentSession work")
	})
})
